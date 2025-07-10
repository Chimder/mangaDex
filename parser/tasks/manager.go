package tasks

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"mangadex/parser/proxy"
	"mangadex/parser/query"
	"net/http"
	"strconv"
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
	mangaRepo    query.MangaRepository
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
		mangaRepo:    query.NewMangaRepository(db),
		maxWorkers:   32,
		currentPage:  1,
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (tm *TaskManager) ProcessPages() {
	slog.Info("Starting task manager")

	for {
		select {
		case <-tm.ctx.Done():
			slog.Info("Page processing context stop.")
			return
		default:
			client := tm.proxyManager.GetAvailableProxyClient()
			if client == nil {
				slog.Error("No proxies for fetch")
				time.Sleep(1 * time.Second)
				continue
			}

			parser := query.NewParserManager(client.Addr)
			pageStr := strconv.Itoa(tm.currentPage)
			mangaLists, err := parser.GetMangaList(pageStr)
			if err != nil {
				client.MarkAsBad()
				slog.Error("Failed to get manga list", "page", tm.currentPage, "error", err)
				time.Sleep(1 * time.Second)
				continue
			}
			client.MarkAsNotBusy()

			if len(mangaLists) == 0 {
				slog.Info("No manga found.", "page", tm.currentPage)
				tm.cancel()
				return
			}

			slog.Info("Got manga list", "count", len(mangaLists), "page", tm.currentPage)

			for _, manga := range mangaLists {
				mangaId, err := tm.mangaRepo.ExistsMangaByTitle(tm.ctx, manga.Title)
				if err != nil && mangaId == "" {
					tm.handleMangaInfo(manga.URL)
				} else {
					slog.Warn("Update Chap Skip")
					// tm.handleChapterUpdateInfo(mangaId, manga.URL)
				}
			}

			tm.currentPage++
		}
	}
}

func (tm *TaskManager) handleMangaInfo(URL string) {
	var (
		client    *proxy.ProxyClient
		mangaInfo *query.MangaInfoParserResp
		errParse  error
		errDb     error
		mangaId   string
		chapWg    sync.WaitGroup
	)

	for i := range maxRetries {
		client = tm.proxyManager.GetAvailableProxyClient()
		if client == nil {
			slog.Error("No available proxy", "url", URL, "retrie", i)
			continue
		}
		parser := query.NewParserManager(client.Addr)
		mangaInfo, errParse = parser.GetMangaInfo(URL)
		if errParse != nil {
			client.MarkAsBad()
			slog.Error("Err to get manga info", "url", URL, "error", errParse, "retries", i)
			continue
		}
		mangaId, errDb = tm.mangaRepo.InsertManga(tm.ctx, query.MangaDB{
			Title:       mangaInfo.Title,
			CoverUrl:    mangaInfo.CoverURL,
			AltTitles:   mangaInfo.AltTitles,
			Authors:     mangaInfo.Authors,
			Status:      mangaInfo.Status,
			Genres:      mangaInfo.Genres,
			Description: mangaInfo.Description,
		})
		if errDb != nil || mangaId == "" {
			client.MarkAsBad()
			slog.Info("Failed insert manga info", "title", mangaInfo.Title, "chapters", len(mangaInfo.Chapters), "retry", i)
			continue
		}
		break
	}

	for _, url := range mangaInfo.Chapters {
		chapWg.Add(1)
		go func(url string) {
			defer chapWg.Done()
			tm.handleMangaChapter(url, mangaId)
		}(url)
	}

	chapWg.Wait()
	client.MarkAsNotBusy()
	slog.Info("Get manga info", "title", mangaInfo.Title, "chapters", len(mangaInfo.Chapters))
}

func (tm *TaskManager) handleMangaChapter(URL string, mangaId string) {
	var client *proxy.ProxyClient
	var chapterInfo query.ChapterInfo
	var httpClient *http.Client
	var err error
	for i := range maxRetries {
		client = tm.proxyManager.GetAvailableProxyClient()
		if client == nil {
			client.MarkAsBad()
			slog.Error("No available proxy for chap img", "url", URL, "retry", i)
			continue
		}

		parser := query.NewParserManager(client.Addr)
		chapterInfo, err = parser.GetImgFromChapter(URL)
		if err != nil {
			client.MarkAsBad()
			slog.Error("Err to get chap imgs", "url", URL, "error", err, "retry", i)
			continue
		}

		httpClient, err = client.GetProxyHttpClient()
		if err != nil {
			client.MarkAsBad()
			slog.Error("Http client is nil", "url", URL, "error", err, "retry", i)
			continue
		}
		client.MarkAsNotBusy()
		break
	}

	reqCtx, cancel := context.WithTimeout(tm.ctx, 60*time.Second)
	defer cancel()
	var imgWG sync.WaitGroup

	for i, url := range chapterInfo.Images {
		imgWG.Add(1)
		go func(url string, i int) {
			defer imgWG.Done()
			var imgBytes []byte
			var contentType string
			var ext string
			var err error

			for i := range maxRetries {
				if i > 0 {
					httpClient = tm.proxyManager.GetAvailableProxyClient().Client
					slog.Warn("Get New Client Img", "Idx", i, "nil", httpClient)
				}

				req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
				if err != nil {
					slog.Debug("Failed to create request", "url", url, "error", err)
					continue
				}

				resp, err := httpClient.Do(req)
				if err != nil {
					slog.Debug("Request failed", "url", url, "error", err)
					continue
				}

				if resp == nil || resp.Body == nil {
					slog.Debug("Nil response or body", "url", url)
					continue
				}

				if resp.ContentLength == 0 {
					resp.Body.Close()
					slog.Debug("Empty content", "url", url)
					continue
				}

				imgBytes, contentType, ext, err = query.FilterImg(resp, url)
				resp.Body.Close()
				if err != nil || len(imgBytes) == 0 {
					slog.Warn("skip upload: empty image bytes", "url", url)
					continue
				}
				break
			}
			if len(imgBytes) == 0 {
				slog.Warn("skip upload: empty image bytes", "url", url)
				return
			}

			bucketName := "mangadex"
			objectName := fmt.Sprintf(`%s/%s/%02d%s`, mangaId, chapterInfo.Name, i+1, ext)

			_, err = tm.bucket.PutObject(tm.ctx, bucketName, objectName,
				bytes.NewReader(imgBytes), int64(len(imgBytes)), minio.PutObjectOptions{
					ContentType: contentType,
				})
			if err != nil {
				slog.Error("bucket upload failed", "err", err)
				return
			}
		}(url, i)
	}

	imgWG.Wait()
	if client != nil {
		client.MarkAsNotBusy()
	}
	slog.Warn("Download and save all chapter imgs", "url", URL, "imgs", len(chapterInfo.Images))
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
