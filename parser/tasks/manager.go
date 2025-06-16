package tasks

import (
	"context"
	"fmt"
	"log/slog"
	"mangadex/parser/proxy"
	"mangadex/parser/query"
	"strconv"
	"sync"
	"time"
)

type taskType int

const (
	MangaListType taskType = iota
	MangaInfoType
	MangaChapterImgType
)

const maxRetries = 7

type task struct {
	ID         string
	Type       taskType
	URL        string
	RetryCount int
}

type TaskManager struct {
	taskChan     chan *task
	proxyManager *proxy.ProxyManager
	maxWorkers   int
	currentPage  int
	ctx          context.Context
	cancel       context.CancelFunc

	pageProcessing sync.WaitGroup
}

func NewTaskManager(proxyMng *proxy.ProxyManager, parentCtx context.Context) *TaskManager {
	ctx, cancel := context.WithCancel(parentCtx)
	return &TaskManager{
		taskChan:     make(chan *task, 4000),
		proxyManager: proxyMng,
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

				workerSemaphore <- struct{}{}
				go func(taskToProcess *task) {
					defer func() {
						<-workerSemaphore
						tm.pageProcessing.Done()
						if r := recover(); r != nil {
							slog.Error("Panic in task processing", "task", taskToProcess.ID, "error", r)
						}
					}()

					switch t.Type {
					case MangaInfoType:
						tm.handleMangaInfoTask(t)
					case MangaChapterImgType:
						tm.handleMangaChapterImgTask(t)
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
			tm.pageProcessing.Wait()

			slog.Info("Starting page processing", "page", tm.currentPage)

			client := tm.proxyManager.GetAvailableProxyClient()
			if client == nil {
				slog.Error("No proxies for fetch")
				time.Sleep(10 * time.Second)
				continue
			}

			parser := query.NewParserManager(client.Addr)
			pageStr := strconv.Itoa(tm.currentPage)
			mangaURLs, err := parser.GetMangaList(pageStr)
			if err != nil {
				client.MarkAsBad()
				slog.Error("Failed to get manga list", "page", tm.currentPage, "error", err)
				time.Sleep(10 * time.Second)
				continue
			}
			client.MarkAsNotBusy()

			if len(mangaURLs) == 0 {
				slog.Info("No manga found.", "page", tm.currentPage)
				tm.cancel()
				return
			}

			// tm.processedTasks.Store(0)
			slog.Info("Got manga list", "count", len(mangaURLs), "page", tm.currentPage)

			for _, url := range mangaURLs {
				infoTask := &task{
					ID:   fmt.Sprintf("manga_info:%s", url),
					Type: MangaInfoType,
					URL:  url,
				}

				tm.pageProcessing.Add(1)

				select {
				case tm.taskChan <- infoTask:
				case <-time.After(5 * time.Second):
					slog.Warn("Timeout send task to channel ", "url", url)
					tm.pageProcessing.Done()
				case <-tm.ctx.Done():
					slog.Info("Err while queuing tasks", "page", tm.currentPage)
					tm.pageProcessing.Done()
					return
				}
			}
			tm.currentPage++
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
	for _, img := range mangaInfo.Chapters {
		chapterImgTask := &task{
			ID:   fmt.Sprintf("Chapter_img:%s", img),
			Type: MangaChapterImgType,
			URL:  img,
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

func (tm *TaskManager) handleMangaChapterImgTask(t *task) {
	client := tm.proxyManager.GetAvailableProxyClient()
	if client == nil {
		slog.Error("No available proxy for chap img", "url", t.URL, "retry", t.RetryCount)
		tm.retryTask(t)
		return
	}

	parser := query.NewParserManager(client.Addr)
	images, err := parser.GetImgFromChapter(t.URL)

	if err != nil {
		client.MarkAsBad()
		slog.Error("Err to get chap imgs", "url", t.URL, "error", err, "retry", t.RetryCount)
		tm.retryTask(t)
		return
	}

	client.MarkAsNotBusy()
	slog.Info("Get chapters img", "url", t.URL, "imgs", len(images))
}

func (tm *TaskManager) retryTask(t *task) {
	if t.RetryCount >= maxRetries {
		slog.Error("Max retries over.", "id", t.ID, "url", t.URL)
		return
	}

	t.RetryCount++
	delay := time.Duration(t.RetryCount) * 2 * time.Second

	slog.Info("Retrying task", "task", t.ID, "retry", t.RetryCount)

	tm.pageProcessing.Add(1)
	go func() {
		select {
		case <-tm.ctx.Done():
			slog.Info("Ctx retry cancel1", "task", t.ID)
			tm.pageProcessing.Done()
		case <-time.After(delay):
			select {
			case tm.taskChan <- t:
			case <-tm.ctx.Done():
				slog.Info("Ctx retry cancel2", "task", t.ID)
				tm.pageProcessing.Done()
				// case <-time.After(5 * time.Second):
				// 	slog.Warn("Time retry cancel3", "task", t.ID)
				// 	tm.pageProcessing.Done()
			}
		}
	}()
}

func (tm *TaskManager) Stop() {
	tm.cancel()
	tm.pageProcessing.Wait()
	time.Sleep(500 * time.Millisecond)
	close(tm.taskChan)
	slog.Info("Stop TaskManager.")
}

// func (tm *TaskManager) GetStats() (int, int32, int32) {
// 	return tm.currentPage, tm.activeTasks.Load(), tm.processedTasks.Load()
// }
