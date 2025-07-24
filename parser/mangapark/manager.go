package mangapark

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"mangadex/parser/proxy"
	"mangadex/parser/query"
	"net/http"
	"strconv"
	"strings"
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

const maxRetries = 7

type TaskManager struct {
	mangaRepo    MangaRepository
	proxyManager *proxy.ProxyManager
	maxWorkers   int
	currentPage  int
	ctx          context.Context
	cancel       context.CancelFunc
	bucket       *minio.Client
}

func NewTaskManager(ctx context.Context, proxyMng *proxy.ProxyManager, db *pgxpool.Pool, bucket *minio.Client) *TaskManager {
	ctx, cancel := context.WithCancel(ctx)
	return &TaskManager{
		proxyManager: proxyMng,
		bucket:       bucket,
		mangaRepo:    NewMangaRepository(db),
		maxWorkers:   32,
		currentPage:  1,
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (tm *TaskManager) StartPageParseWorker() {
	slog.Info("Starting task manager")
	sem := make(chan struct{}, 2)

	for {
		select {
		case <-tm.ctx.Done():
			close(sem)
			slog.Info("Stopping processing")
			return
		default:
			client := tm.proxyManager.GetAvailableProxyClient(tm.ctx)
			if client == nil {
				slog.Error("No proxies available")
				continue
			}

			mangaLists, err := NewParserManager(client.Addr).GetMangaList(strconv.Itoa(tm.currentPage))
			if err != nil {
				client.MarkAsBad(tm.proxyManager)
				continue
			}
			client.MarkAsNotBusy()

			if len(mangaLists) == 0 {
				if tm.currentPage > 50 {
					continue
				}
				slog.Info("No manga found - stopping", "page", tm.currentPage)
				tm.cancel()
				close(sem)
				return
			}

			slog.Info("Processing manga list", "count", len(mangaLists), "page", tm.currentPage)
			var wg sync.WaitGroup

			for _, m := range mangaLists {
				sem <- struct{}{}
				wg.Add(1)

				go func(m MangaList) {
					defer func() {
						<-sem
						wg.Done()
					}()

					mangaId, _ := tm.mangaRepo.ExistsMangaByTitle(tm.ctx, m.Title)
					if mangaId == "" {
						tm.handleMangaInfo(m.URL)
					} else {
						slog.Warn("update manga", "title", m.Title)
						tm.handleChapterUpdateInfo(mangaId, m.URL)
					}
				}(m)
			}

			wg.Wait()
			tm.currentPage++
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (tm *TaskManager) handleMangaInfo(URL string) {
	var (
		client    *proxy.ProxyClient
		mangaInfo *MangaInfoParserResp
		errParse  error
		errDb     error
		mangaId   string
		chapWg    sync.WaitGroup
	)

	for i := range maxRetries {
		client = tm.proxyManager.GetAvailableProxyClient(tm.ctx)
		if client == nil {
			slog.Error("No available proxy", "url", URL, "retrie", i)
			continue
		}
		parser := NewParserManager(client.Addr)
		mangaInfo, errParse = parser.GetMangaInfo(URL)
		if errParse != nil {
			client.MarkAsBad(tm.proxyManager)
			slog.Error("Err to get manga info", "url", URL, "error", errParse, "retries", i)
			continue
		}
		mangaId, errDb = tm.mangaRepo.InsertManga(tm.ctx, MangaDB{
			Title:       mangaInfo.Title,
			CoverUrl:    mangaInfo.CoverURL,
			AltTitles:   mangaInfo.AltTitles,
			Authors:     mangaInfo.Authors,
			Status:      mangaInfo.Status,
			Genres:      mangaInfo.Genres,
			Description: mangaInfo.Description,
		})
		if errDb != nil || mangaId == "" {
			client.MarkAsBad(tm.proxyManager)
			slog.Info("Failed insert manga info", "title", mangaInfo.Title, "chapters", len(mangaInfo.Chapters), "retry", i)
			continue
		}
		break
	}
	if mangaInfo == nil {
		slog.Error("Max try parse manga info", "err:", errParse)
		return
	}

	for _, ch := range mangaInfo.Chapters {
		chapWg.Add(1)
		go func(ch Chapter) {
			defer chapWg.Done()
			tm.handleMangaChapter(ch.URL, mangaId)
		}(ch)
	}

	chapWg.Wait()
	client.MarkAsNotBusy()
	slog.Info("Get manga info", "title", mangaInfo.Title, "chapters", len(mangaInfo.Chapters))
}

func (tm *TaskManager) handleMangaChapter(URL string, mangaId string) {
	var client *proxy.ProxyClient
	var chapterInfo ChapterInfo
	var err error
	for i := range maxRetries {
		client = tm.proxyManager.GetAvailableProxyClient(tm.ctx)
		if client == nil {
			client.MarkAsBad(tm.proxyManager)
			slog.Error("No available proxy for chap img", "url", URL, "retry", i)
			continue
		}

		parser := NewParserManager(client.Addr)
		chapterInfo, err = parser.GetImgFromChapter(URL)
		if err == nil && (len(chapterInfo.Images) == 0 || strings.TrimSpace(chapterInfo.Name) == "") {
			err = fmt.Errorf("parsed chapter has empty name or no images")
			client.MarkAsBad(tm.proxyManager)
			slog.Error("Invalid parsed chapter data", "url", URL, "retry", i)
			continue
		}

		client.MarkAsNotBusy()
		break
	}
	if err == nil && (len(chapterInfo.Images) == 0 || strings.TrimSpace(chapterInfo.Name) == "") {
		err = fmt.Errorf("parsed chapter has empty name or no images")
		client.MarkAsBad(tm.proxyManager)
		return
	}

	var imgWG sync.WaitGroup
	imgWG.Add(len(chapterInfo.Images))

	for i, url := range chapterInfo.Images {
		go func(url string, idx int) {
			defer imgWG.Done()

			created, errDB := tm.mangaRepo.CreateImgTask(tm.ctx, ImgInfoToChan{
				Url: url, Idx: idx + 1, MangaId: mangaId, ChapterName: chapterInfo.Name,
			})
			if !created {
				slog.Error("Err create img_task to DB", ":", errDB)
				return
			}
		}(url, i)
	}

	imgWG.Wait()

	created, errDB := tm.mangaRepo.CreateChapter(tm.ctx, CreateChapterArg{
		MangaID: mangaId,
		Name:    chapterInfo.Name,
	})
	if !created {
		slog.Error("Failed Create Chapter to DB", "err", errDB)
		return
	}
}

func (tm *TaskManager) StartImgWorkerLoop() {
	slog.Info("StartImgWorkerLoop: started")

	interval := 15 * time.Second

	for {
		select {
		case <-tm.ctx.Done():
			slog.Info("ImgWorker ctx cancelled")
			return
		default:
		}

		items, err := tm.mangaRepo.GetImgTasks(tm.ctx)
		if err != nil {
			slog.Error("GetImgTasks failed", "err", err)
			time.Sleep(interval)
			continue
		}

		if len(items) == 0 {
			time.Sleep(interval)
			continue
		}

		slog.Debug("Start IMG Process", "tasks", len(items))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 100)

		wg.Add(len(items))
		for i, img := range items {
			go func(i int, img ImgInfoToChan) {
				defer func() {
					<-sem
					wg.Done()
				}()
				sem <- struct{}{}
				tm.handleImageRetry(img)
			}(i, img)
		}
		wg.Wait()
	}
}

func (tm *TaskManager) handleImageRetry(img ImgInfoToChan) {
	var (
		imgBytes    []byte
		ext         string
		contentType string
		errImg      error
	)
	delay := time.Second

	for range 10 {
		time.Sleep(delay)

		reqCtx, cancel := context.WithTimeout(tm.ctx, 50*time.Second)
		defer cancel()

		imgClient := tm.proxyManager.GetAvailableProxyClient(tm.ctx)
		if imgClient == nil {
			delay++
			continue
		}

		req, err := http.NewRequestWithContext(reqCtx, "GET", img.Url, nil)
		if err != nil {
			slog.Error("img request", "err", err)
			imgClient.MarkAsBad(tm.proxyManager)
			delay++
			continue
		}

		resp, err := imgClient.Client.Do(req)
		if err != nil {
			imgClient.MarkAsBad(tm.proxyManager)
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
			delay++
			continue
		}

		imgBytes, ext, contentType, errImg = query.FilterImg(resp, img.Url)
		if errImg != nil || len(imgBytes) <= 0 {
			slog.Error("img filter", "err", errImg, "", ext, "", contentType)
			imgClient.MarkAsBad(tm.proxyManager)
			delay++
			continue
		}
		imgClient.MarkAsNotBusy()
		break
	}
	if errImg != nil || len(imgBytes) <= 0 {
		slog.Error("img all tries spent", "err", errImg)
		return
	}

	safeName := query.SafeChapterNameToS3(img.ChapterName)
	err := tm.uploadImgToS3(imgBytes, img.MangaId, safeName, img.Idx, ext, contentType)
	if err != nil {
		slog.Error("retry uploadToS3", "err:", err)
		return
	}

	imgUrl := fmt.Sprintf("%s/%s/%s/%02d%s", "localhost:9000/mangapark", img.MangaId, safeName, img.Idx, ext)
	err = tm.mangaRepo.UpdateChapterImgByIndex(tm.ctx, img.MangaId, img.ChapterName, img.Idx, imgUrl)
	if err != nil {
		slog.Error("failed to update chapter img", "err", err)
	}

	del, errDel := tm.mangaRepo.DeleteImgTaskByURL(tm.ctx, img.Url)
	if !del {
		slog.Error("err delete img DB", "err:", errDel)
		return
	}
}

func (tm *TaskManager) handleChapterUpdateInfo(mangaId string, URL string) {
	oldChapters, err := tm.mangaRepo.GetChaptersNamesByMangaId(tm.ctx, mangaId)
	if err != nil {
		slog.Error("No manga chapters info", "url", URL)
		return
	}

	var client *proxy.ProxyClient
	var chapters *MangaInfoParserResp
	for i := range maxRetries {
		client = tm.proxyManager.GetAvailableProxyClient(tm.ctx)
		if client == nil {
			client.MarkAsBad(tm.proxyManager)
			slog.Error("No available proxy for chap img", "url", URL, "retry", i)
			continue
		}

		parser := NewParserManager(client.Addr)
		chapters, err = parser.GetMangaInfo(URL)
		if err != nil {
			client.MarkAsBad(tm.proxyManager)
			slog.Error("Err to get chap imgs", "url", URL, "error", err, "retry", i)
			continue
		}

		client.MarkAsNotBusy()
		break
	}
	if err != nil || len(chapters.Chapters) <= 0 {
		slog.Error("Failed to get chapters after retries", "url", URL)
		client.MarkAsBad(tm.proxyManager)
		return
	}

	oldChapMap := make(map[string]struct{})
	for _, old := range oldChapters {
		oldChapMap[old.Name] = struct{}{}
	}

	var chapWg sync.WaitGroup
	for _, ch := range chapters.Chapters {
		if _, exists := oldChapMap[ch.Name]; !exists {
			chapWg.Add(1)
			go func(ch Chapter) {
				defer chapWg.Done()
				tm.handleMangaChapter(ch.URL, mangaId)
			}(ch)
		}
	}
	chapWg.Wait()
	slog.Info("update Chapter", "manga", mangaId)
}

func (tm *TaskManager) uploadImgToS3(imgBytes []byte, mangaId, chapterName string, idx int, ext, contentType string) error {
	bucketName := "mangapark"
	objectName := fmt.Sprintf(`%s/%s/%02d%s`, mangaId, chapterName, idx, ext)
	s3Retry := 4

	var errS3 error
	for i := range s3Retry {
		reqCtx, cancel := context.WithTimeout(tm.ctx, 45*time.Second)

		_, errS3 = tm.bucket.PutObject(
			reqCtx,
			bucketName,
			objectName,
			bytes.NewReader(imgBytes),
			int64(len(imgBytes)),
			minio.PutObjectOptions{ContentType: contentType},
		)
		cancel()

		if errS3 != nil {
			slog.Error("upload s3", "err", errS3, "try", i)
			time.Sleep(time.Second * time.Duration(i))
			continue
		}

		return nil
	}

	return fmt.Errorf("S3 upload failed after %d retries", s3Retry)
}
func (tm *TaskManager) Stop() {
	tm.cancel()
	time.Sleep(500 * time.Millisecond)
	slog.Info("Stop TaskManager.")
}
