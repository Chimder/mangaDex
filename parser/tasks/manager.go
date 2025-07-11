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
	sem := make(chan struct{}, 10)

	for {
		select {
		case <-tm.ctx.Done():
			slog.Info("Stopping processing")
			return
		default:
			client := tm.proxyManager.GetAvailableProxyClient()
			if client == nil {
				slog.Error("No proxies available")
				time.Sleep(time.Second)
				continue
			}

			mangaLists, err := query.NewParserManager(client.Addr).GetMangaList(strconv.Itoa(tm.currentPage))
			client.MarkAsNotBusy()

			if err != nil {
				client.MarkAsBad()
				slog.Error("Failed to get manga list", "page", tm.currentPage, "error", err)
				time.Sleep(time.Second)
				continue
			}

			if len(mangaLists) == 0 {
				slog.Info("No manga found - stopping", "page", tm.currentPage)
				tm.cancel()
				return
			}

			slog.Info("Processing manga list", "count", len(mangaLists), "page", tm.currentPage)
			var wg sync.WaitGroup

			for _, manga := range mangaLists {
				sem <- struct{}{}
				wg.Add(1)

				go func(m query.MangaList) {
					defer func() {
						<-sem
						wg.Done()
					}()

					mangaId, _ := tm.mangaRepo.ExistsMangaByTitle(tm.ctx, m.Title)
					if mangaId == "" {
						tm.handleMangaInfo(m.URL)
					} else {
						slog.Warn("Skipping existing manga", "title", m.Title)
					}
				}(manga)
			}

			wg.Wait()
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
	if mangaInfo == nil {
		slog.Error("Max try parse manga info", "err:", errParse)
		return
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

	reqCtx, cancel := context.WithTimeout(tm.ctx, 45*time.Second)
	defer cancel()
	var imgWG sync.WaitGroup

	for i, url := range chapterInfo.Images {
		imgWG.Add(1)
		go func(url string, idx int) {
			defer imgWG.Done()
			var imgBytes []byte
			var contentType string
			var ext string
			var err error

			for i := range maxRetries {
				if i > 0 {
					httpClient = tm.proxyManager.GetAvailableProxyClient().Client
				}

				req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
				if err != nil {
					continue
				}

				resp, err := httpClient.Do(req)
				if err != nil {
					continue
				}
				defer func ()  {
					resp.Body.Close()
				}()

				if resp == nil || resp.Body == nil {
					// resp.Body.Close()
					continue
				}

				if resp.ContentLength == 0 {
					// resp.Body.Close()
					continue
				}

				imgBytes, ext, contentType, err = query.FilterImg(resp, url)
				// resp.Body.Close()
				if err != nil || len(imgBytes) == 0 {
					continue
				}
				break
			}
			if len(imgBytes) == 0 {
				slog.Error("skip upload: empty image bytes", "url", url)
				return
			}

			bucketName := "mangapark"
			objectName := fmt.Sprintf(`%s/%s/%02d%s`, mangaId, chapterInfo.Name, idx+1, ext)

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
