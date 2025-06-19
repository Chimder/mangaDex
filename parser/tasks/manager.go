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

type task struct {
	ID         string
	Type       taskType
	URL        string
	RetryCount int
	MangaId    string
}

type TaskManager struct {
	taskChan     chan *task
	mangaRepo    query.MangaRepository
	proxyManager *proxy.ProxyManager
	maxWorkers   int
	currentPage  int
	ctx          context.Context
	cancel       context.CancelFunc
	bucket       *minio.Client

	pageWaitGroup sync.WaitGroup
}

func NewTaskManager(ctx context.Context, proxyMng *proxy.ProxyManager, db *pgxpool.Pool, bucket *minio.Client) *TaskManager {
	ctx, cancel := context.WithCancel(ctx)
	return &TaskManager{
		taskChan:     make(chan *task, 4000),
		proxyManager: proxyMng,
		bucket:       bucket,
		mangaRepo:    query.NewMangaRepository(db),
		maxWorkers:   20,
		currentPage:  1,
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (tm *TaskManager) Start() {
	slog.Info("Starting task manager")

	workerSemaphore := make(chan struct{}, tm.maxWorkers)

	go func() {
		for {
			select {
			case <-tm.ctx.Done():
				slog.Info("Task manager ctx cancel")
				return
			case t := <-tm.taskChan:
				if t == nil {
					slog.Info("Task channel closed.")
					return
				}

				tm.pageWaitGroup.Add(1)
				workerSemaphore <- struct{}{}
				go func(taskToProcess *task) {
					defer func() {
						<-workerSemaphore
						tm.pageWaitGroup.Done()
						if r := recover(); r != nil {
							slog.Error("Panic in task processing", "task", taskToProcess.ID, "error", r)
						}
					}()

					switch t.Type {
					case MangaInfoType:
						tm.handleMangaInfoTask(t)
					case MangaChapterType:
						tm.handleMangaChapterTask(t)
					case MangaChapterUpdateType:
						tm.handleChapterUpdateInfoTask(t)
					default:
						slog.Warn("Unknown task type", "task_id", t.ID, "type", t.Type)
					}
				}(t)
			}
		}
	}()

	go tm.processPages()
}

func (tm *TaskManager) processPages() {
	for {
		select {
		case <-tm.ctx.Done():
			slog.Info("Page processing context stop.")
			return
		default:
			tm.pageWaitGroup.Wait()

			// slog.Info("Starting page processing", "page", tm.currentPage)

			client := tm.proxyManager.GetAvailableProxyClient()
			if client == nil {
				slog.Error("No proxies for fetch")
				time.Sleep(10 * time.Second)
				continue
			}

			parser := query.NewParserManager(client.Addr)
			pageStr := strconv.Itoa(tm.currentPage)
			mangaLists, err := parser.GetMangaList(pageStr)
			if err != nil {
				client.MarkAsBad()
				slog.Error("Failed to get manga list", "page", tm.currentPage, "error", err)
				time.Sleep(10 * time.Second)
				continue
			}
			client.MarkAsNotBusy()

			if len(mangaLists) == 0 {
				slog.Info("No manga found.", "page", tm.currentPage)
				tm.cancel()
				return
			}

			// tm.processedTasks.Store(0)
			slog.Info("Got manga list", "count", len(mangaLists), "page", tm.currentPage)
			var infoTask *task

			for _, manga := range mangaLists {
				mangaId, err := tm.mangaRepo.ExistsMangaByTitle(tm.ctx, manga.Title)
				if err != nil && mangaId == "" {
					slog.Error("Manga not exist in DB", "title", manga.Title, "err", err)
					infoTask = &task{
						ID:   fmt.Sprintf("manga_info:%s", manga.URL),
						Type: MangaInfoType,
						URL:  manga.URL,
					}
				} else {
					slog.Info("Manga found, check for chapter updates", "title", manga.Title)
					infoTask = &task{
						ID:      fmt.Sprintf("manga_info:%s", manga.URL),
						Type:    MangaChapterUpdateType,
						URL:     manga.URL,
						MangaId: mangaId,
					}
				}

				tm.pageWaitGroup.Add(1)

				select {
				case tm.taskChan <- infoTask:
				case <-time.After(5 * time.Second):
					slog.Warn("Timeout send task to channel ", "url", manga.URL)
					tm.pageWaitGroup.Done()
				case <-tm.ctx.Done():
					slog.Info("Err while queuing tasks", "page", tm.currentPage)
					tm.pageWaitGroup.Done()
					return
				}
			}
			tm.currentPage++
		}
	}
}

func (tm *TaskManager) handleChapterUpdateInfoTask(t *task) {
	oldChapters, err := tm.mangaRepo.GetMangaChaptersById(tm.ctx, t.MangaId)
	if err != nil {
		slog.Error("No manga chapters info", "url", t.URL, "retry_count", t.RetryCount)
		tm.retryTask(t)
		return
	}
	if oldChapters.ChaptersAmount <= 150 {
		slog.Warn("chapters max amount", "url", t.MangaId)
		return
	}

	client := tm.proxyManager.GetAvailableProxyClient()
	if client == nil {
		slog.Error("No available proxy", "url", t.URL, "retry_count", t.RetryCount)
		tm.retryTask(t)
		return
	}

	parser := query.NewParserManager(client.Addr)
	newChapters, err := parser.GetMangaChapters(t.URL)
	if err != nil {
		client.MarkAsBad()
		slog.Error("Err to get manga info", "url", t.URL, "error", err, "retry", t.RetryCount)
		tm.retryTask(t)
		return
	}
	if len(newChapters) <= oldChapters.ChaptersAmount {
		for _, img := range newChapters[0:oldChapters.ChaptersAmount] {
			chapterImgTask := &task{
				ID:      fmt.Sprintf("Chapter_img:%s", img),
				Type:    MangaChapterType,
				URL:     img,
				MangaId: oldChapters.MangaID.String(),
			}
			select {
			case tm.taskChan <- chapterImgTask:
			case <-tm.ctx.Done():
				slog.Info("Err while queuing chapterImg tasks", "url", chapterImgTask.URL)
				return
			}
		}
	}
}
func (tm *TaskManager) handleMangaInfoTask(t *task) {
	client := tm.proxyManager.GetAvailableProxyClient()
	if client == nil {
		slog.Error("No available proxy", "url", t.URL, "retry_count", t.RetryCount)
		tm.retryTask(t)
		return
	}

	parser := query.NewParserManager(client.Addr)
	mangaInfo, err := parser.GetMangaInfo(t.URL)
	if err != nil {
		client.MarkAsBad()
		slog.Error("Err to get manga info", "url", t.URL, "error", err, "retry", t.RetryCount)
		tm.retryTask(t)
		return
	}
	mangaId, err := tm.mangaRepo.InsertManga(tm.ctx, query.MangaDB{
		Title:       mangaInfo.Title,
		CoverUrl:    mangaInfo.CoverURL,
		AltTitles:   mangaInfo.AltTitles,
		Authors:     mangaInfo.Authors,
		Status:      mangaInfo.Status,
		Genres:      mangaInfo.Genres,
		Description: mangaInfo.Description,
	})
	if err != nil || mangaId == "" {
		client.MarkAsNotBusy()
		slog.Info("Failed insert manga info", "title", mangaInfo.Title, "chapters", len(mangaInfo.Chapters))
		return
	}
	for _, img := range mangaInfo.Chapters {
		chapterImgTask := &task{
			ID:      fmt.Sprintf("Chapter_img:%s", img),
			Type:    MangaChapterType,
			URL:     img,
			MangaId: mangaId,
		}
		select {
		case tm.taskChan <- chapterImgTask:
		case <-tm.ctx.Done():
			slog.Info("Err while queuing chapterImg tasks", "url", chapterImgTask.URL)
			return
		}
	}

	client.MarkAsNotBusy()
	slog.Info("Get manga info", "title", mangaInfo.Title, "chapters", len(mangaInfo.Chapters))
}

func (tm *TaskManager) handleMangaChapterTask(t *task) {
	client := tm.proxyManager.GetAvailableProxyClient()
	if client == nil {
		slog.Error("No available proxy for chap img", "url", t.URL, "retry", t.RetryCount)
		tm.retryTask(t)
		return
	}

	parser := query.NewParserManager(client.Addr)
	chapterInfo, err := parser.GetImgFromChapter(t.URL)
	if err != nil {
		client.MarkAsBad()
		slog.Error("Err to get chap imgs", "url", t.URL, "error", err, "retry", t.RetryCount)
		tm.retryTask(t)
		return
	}

	var httpClient *http.Client
	httpClient, err = client.GetProxyHttpClient()
	if err != nil {
		client.MarkAsBad()
		slog.Error("Http client is nil", "url", t.URL, "error", err, "retry", t.RetryCount)
		tm.retryTask(t)
		return
	}

	reqCtx, cancel := context.WithTimeout(tm.ctx, 8*time.Second)
	defer cancel()
	var imgWG sync.WaitGroup

	for i, url := range chapterInfo.Images {
		imgWG.Add(1)
		go func(url string) {
			defer imgWG.Done()
			var imgBytes []byte
			var contentType string
			var err error

			var tryCount int

			for tryCount <= 20 {
				if tryCount > 0 {
					httpClient, err = tm.proxyManager.GetRandomProxyHttpClient()
					if err != nil {
						tryCount++
						continue
					}
				}
				req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
				if err != nil {
					tryCount++
					continue
				}
				resp, err := httpClient.Do(req)
				if err != nil {
					tryCount++
					continue
				}
				defer resp.Body.Close()

				imgBytes, contentType, err = query.FilterImg(resp, url)
				if err != nil || len(imgBytes) == 0 {
					tryCount++
					slog.Warn("image processing failed", "err", err)
					continue
				}
				break
			}

			bucketName := "mangapark"
			objectName := fmt.Sprintf(`%s/%s/%02d%s`, t.MangaId, chapterInfo.Name, i+1, contentType)
			// objectName := fmt.Sprintf(`%s/%s/%s%s`, t.MangaId, chapterInfo.Name, strconv.Itoa(i), contentType)

			if len(imgBytes) == 0 {
				slog.Warn("skip upload: empty image bytes", "url", url)
				return
			}
			_, err = tm.bucket.PutObject(tm.ctx, bucketName, objectName,
				bytes.NewReader(imgBytes), int64(len(imgBytes)), minio.PutObjectOptions{
					ContentType: contentType,
				})
			if err != nil {
				slog.Error("bucket upload failed", "err", err)
				return
			}
		}(url)
	}

	imgWG.Wait()
	client.MarkAsNotBusy()
	slog.Warn("Download and save all chapter imgs", "url", t.URL, "imgs", len(chapterInfo.Images))
}

func (tm *TaskManager) retryTask(t *task) {
	if t.RetryCount >= maxRetries {
		slog.Error("Max retries over.", "id", t.ID, "url", t.URL)
		return
	}

	t.RetryCount++
	delay := time.Duration(t.RetryCount) * 2 * time.Second

	slog.Info("Retrying task", "task", t.ID, "retry", t.RetryCount)

	go func() {
		select {
		case <-tm.ctx.Done():
			slog.Info("Ctx retry cancel1", "task", t.ID)
			tm.pageWaitGroup.Done()
		case <-time.After(delay):
			select {
			case tm.taskChan <- t:
			case <-tm.ctx.Done():
				slog.Info("Ctx retry cancel2", "task", t.ID)
				tm.pageWaitGroup.Done()
				// case <-time.After(5 * time.Second):
				// 	slog.Warn("Time retry cancel3", "task", t.ID)
				// 	tm.pageWaitGroup.Done()
			}
		}
	}()
}

func (tm *TaskManager) Stop() {
	tm.cancel()
	tm.pageWaitGroup.Wait()
	time.Sleep(500 * time.Millisecond)
	close(tm.taskChan)
	slog.Info("Stop TaskManager.")
}

// func (tm *TaskManager) GetStats() (int, int32, int32) {
// 	return tm.currentPage, tm.activeTasks.Load(), tm.processedTasks.Load()
// }
