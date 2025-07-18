package mangadex

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"mangadex/parser/proxy"
	"mangadex/parser/query"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
)

type taskType int

const (
	MangaListType taskType = iota
	MangaInfoType
	MangaChapterType
	MangaChapterUpdateType
)

const maxRetries = 10

type TaskManager struct {
	mangaRepo    MangaRepository
	proxyManager *proxy.ProxyManager
	maxWorkers   int
	currentPage  int
	ctx          context.Context
	cancel       context.CancelFunc
	query        *Query
	bucket       *minio.Client
}

func NewTaskManager(ctx context.Context, proxyMng *proxy.ProxyManager, db *pgxpool.Pool, bucket *minio.Client) *TaskManager {
	ctx, cancel := context.WithCancel(ctx)
	return &TaskManager{
		proxyManager: proxyMng,
		bucket:       bucket,
		query:        NewQueryManager(),
		mangaRepo:    NewMangaRepository(db),
		maxWorkers:   32,
		currentPage:  1,
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (tm *TaskManager) ProcessPages() {
	slog.Info("Starting task manager")
	sem := make(chan struct{}, 1)
	limit := 34
	offset := 0

	for {
		select {
		case <-tm.ctx.Done():
			slog.Info("Stopping processing")
			return
		default:
			client := tm.proxyManager.GetAvailableProxyClient(tm.ctx)
			if client == nil {
				slog.Error("No proxies available")
				continue
			}

			mangaLists, err := tm.query.GetMangaList(tm.ctx, limit, offset, client)
			if err != nil {
				slog.Error("GetList", ":", err)
				client.MarkAsBad(tm.proxyManager)
				continue
			}
			client.MarkAsNotBusy()
			if len(mangaLists.Data) == 0 {
				slog.Info("No manga found - stopping", "page", tm.currentPage)
				tm.cancel()
				return
			}

			slog.Info("Processing manga list", "count", len(mangaLists.Data), "page", tm.currentPage)
			var wg sync.WaitGroup

			for _, manga := range mangaLists.Data {
				sem <- struct{}{}
				wg.Add(1)

				go func(m *MangaData) {
					defer func() {
						<-sem
						wg.Done()
					}()
					mangaId, _ := tm.mangaRepo.ExistsMangaByTitle(tm.ctx, m.Attributes.Title["en"])
					if mangaId == "" {
						tm.handleMangaInfo(&manga)
					} else {
						slog.Warn("Skipping existing manga", "title", m.Attributes.Title["en"])
					}
				}(&manga)
			}

			wg.Wait()
			offset += limit
			tm.currentPage++
		}
	}
}

func (tm *TaskManager) handleMangaInfo(manga *MangaData) {
	var (
		client      *proxy.ProxyClient
		chapterData []ChapterList
		errParse    error

		errDb   error
		mangaId string
		chapWg  sync.WaitGroup
	)

	mangaId, errDb = tm.mangaRepo.InsertManga(tm.ctx, manga)
	if errDb != nil || mangaId == "" {
		// client.MarkAsBad(tm.proxyManager)
		slog.Info("Failed insert manga info", "err:", errDb, "title", manga.Attributes.Title["en"])
		return
	}

	for i := range maxRetries {
		client = tm.proxyManager.GetAvailableProxyClient(tm.ctx)
		if client == nil {
			slog.Error("No available proxy", "manga", manga.Attributes.Title["en"], "retrie", i)
			continue
		}
		chapterData, errParse = tm.query.GetChapterListById(tm.ctx, manga.ID, client)
		if errParse != nil {
			client.MarkAsBad(tm.proxyManager)
			slog.Error("Err to get manga info", "manga", manga.Attributes.Title["en"], "error", errParse, "retries", i)
			continue
		}
		break
	}
	if chapterData == nil {
		slog.Error("Max try parse manga info", "err:", errParse)
		return
	}

	for _, c := range chapterData {
		chapWg.Add(1)
		go func(c ChapterList) {
			defer chapWg.Done()
			tm.handleMangaChapter(mangaId, c.ID, c.ChapterNumber)
		}(c)
	}

	chapWg.Wait()
	client.MarkAsNotBusy()
	slog.Info("Get manga info", "title", manga.Attributes.Title["en"], "chapters", len(chapterData))
}

func (tm *TaskManager) handleMangaChapter(mangaID string, chapterID string, chapterNum string) {
	var client *proxy.ProxyClient
	var chapterImgs *ChapterImgsData
	var err error
	for i := range maxRetries {
		client = tm.proxyManager.GetAvailableProxyClient(tm.ctx)
		if client == nil {
			client.MarkAsBad(tm.proxyManager)
			slog.Error("No available proxy for chap img", "id", chapterID, "retry", i)
			continue
		}

		// parser := NewParserManager(client.Addr)
		chapterImgs, err = tm.query.GetChapterImgsById(tm.ctx, chapterID, client)
		if err != nil {
			client.MarkAsBad(tm.proxyManager)
			slog.Error("Err to get chap imgs", "id", chapterID, "error", err, "retry", i)
			continue
		}

		client.MarkAsNotBusy()
		break
	}

	var imgWG sync.WaitGroup
	imgWG.Add(len(chapterImgs.imgLinks))

	for i, url := range chapterImgs.imgLinks {
		go func(idx int, url string) {
			defer imgWG.Done()
			tm.processImage(url, idx, mangaID, chapterNum)
		}(i, url)
	}

	imgWG.Wait()
}
func (tm *TaskManager) processImage(url string, idx int, mangaId, chapterName string) {
	var (
		imgBytes    []byte
		ext         string
		contentType string
		errFilter   error
	)
	for range maxRetries {
		imgClient := tm.proxyManager.GetAvailableProxyClient(tm.ctx)
		if imgClient == nil {
			continue
		}

		resp, err := tm.query.DownloadImage(tm.ctx, url, imgClient)
		if err != nil {
			slog.Error("dowload img", "err", err)
			imgClient.MarkAsBad(tm.proxyManager)
			continue
		}

		imgBytes, ext, contentType, errFilter = query.FilterImg(resp, url)
		if errFilter != nil {
			slog.Error("filter Img", "err", errFilter)
			imgClient.MarkAsBad(tm.proxyManager)
			continue
		}
		if len(imgBytes) <= 0 {
			slog.Error("filter bytes", "err", errFilter)
			imgClient.MarkAsBad(tm.proxyManager)
			continue
		}
		slog.Info("Filter img Ok", ":", url)
		imgClient.MarkAsNotBusy()
		break
	}
	if len(imgBytes) <= 0 {
		slog.Error("final filter bytes max try")
		return
	}

	err := tm.uploadToS3WithRetry(imgBytes, mangaId, chapterName, idx, ext, contentType)
	if err != nil {
		slog.Error("upload failed", "url", url, "err", err)
		return
	}
}

func (tm *TaskManager) uploadToS3WithRetry(imgBytes []byte, mangaId, chapterName string, idx int, ext, contentType string) error {
	slog.Info("Start S3", "chaN", chapterName)

	putCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	bucketName := "mangapark"
	objectName := fmt.Sprintf(`%s/%s/%02d%s`, mangaId, chapterName, idx+1, ext)

	for i := range maxRetries {
		_, err := tm.bucket.PutObject(putCtx, bucketName, objectName,
			bytes.NewReader(imgBytes), int64(len(imgBytes)),
			minio.PutObjectOptions{ContentType: contentType},
		)
		if err == nil {
			return nil
		}

		slog.Warn("upload retry", "attempt", i+1, "err", err)
		time.Sleep(time.Second * time.Duration(i+1))
	}

	return fmt.Errorf("S3 upload failed after %d retries", maxRetries)
}

// func (tm *TaskManager) handleChapterUpdateInfo(mangaId string, URL string) {
// 	oldChapters, err := tm.mangaRepo.GetMangaChaptersById(tm.ctx, mangaId)
// 	if err != nil {
// 		slog.Error("No manga chapters info", "url", URL)
// 		return
// 	}
// 	if oldChapters.ChaptersAmount <= 150 {
// 		slog.Warn("chapters max amount", "url", mangaId)
// 		return
// 	}

// 	client := tm.proxyManager.GetAvailableProxyClient()
// 	if client == nil {
// 		slog.Error("No available proxy", "url", URL)
// 		return
// 	}

// 	parser := query.NewParserManager(client.Addr)
// 	newChapters, err := parser.GetMangaChapters(URL)
// 	if err != nil {
// 		client.MarkAsBad()
// 		slog.Error("Err to get manga info", "url", URL, "error", err)
// 		return
// 	}
// 	if len(newChapters) <= oldChapters.ChaptersAmount {

// 	}
// }

func (tm *TaskManager) Stop() {
	tm.cancel()
	time.Sleep(500 * time.Millisecond)
	slog.Info("Stop TaskManager.")
}
