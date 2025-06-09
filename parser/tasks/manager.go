package tasks

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"mangadex/parser/proxy"
	"mangadex/parser/query"
	"strconv"
	"sync"
	"time"
)

type taskStatus int
type taskType int

const (
	MangaListType taskType = iota
	MangaInfoType
	MangaChapterImgType
)

type task struct {
	ID   string
	Type taskType
	Url  string
}

type TaskManager struct {
	taskChan       chan task
	proxyManager   *proxy.ProxyManager
	maxWorkers     int
	currentPage    int
	isProcessing   bool
	ctx            context.Context
	cancel         context.CancelFunc
	activeTasks    sync.Map
	currentMangaWg sync.WaitGroup
}

func NewTaskManager(proxyMng *proxy.ProxyManager, parentCtx context.Context) *TaskManager {
	ctx, cancel := context.WithCancel(parentCtx)
	return &TaskManager{
		taskChan:     make(chan task, 1200),
		proxyManager: proxyMng,
		maxWorkers:   10,
		currentPage:  1,
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (tm *TaskManager) Start() {
	sem := make(chan struct{}, tm.maxWorkers)
	slog.Info("Start tasks mg")

	go func() {
		for {
			select {
			case <-tm.ctx.Done():
				return
			case t := <-tm.taskChan:
				sem <- struct{}{}
				defer func() { <-sem }()

				go func(t task) {
					slog.Info("TASK", "type", t.Type, "url", t.Url)

					switch t.Type {
					case MangaInfoType:
						tm.processMangaInfo(t)
					case MangaChapterImgType:
						tm.processChapterImg(t)
					}
				}(t)
			}
		}

	}()

	go func() {
		for {
			select {
			case <-tm.ctx.Done():
				return
			default:
				tm.currentMangaWg.Wait()

				slog.Info("Start take first mangas")
				t := task{
					ID:   fmt.Sprintf("page_%d", tm.currentPage),
					Type: MangaListType,
					Url:  fmt.Sprintf("https://mangapark.io/search?sortby=field_follow&page=%s", strconv.Itoa(tm.currentPage)),
				}

				client := tm.proxyManager.GetAvailableProxyClient()
				if client == nil {
					client.MarkAsBad()
					continue
				}

				slog.Info("Get first client", "addr", client.Addr, "status", client.Status, "isBusy", client.Busy)

				parser := query.NewParserManager(client.Addr)
				mangas, err := parser.GetMangaList(t.Url)
				if err != nil {
					client.MarkAsBad()
					continue
				}
				slog.Info("Get manga list", ":", len(mangas), "page:", tm.currentPage)


				tm.currentMangaWg.Add(len(mangas))
				for _, url := range mangas {
					infoTask := task{
						ID:   fmt.Sprintf("manga_info:%s", url),
						Type: MangaInfoType,
						Url:  url,
					}
					tm.taskChan <- infoTask
				}

				tm.currentPage++
				time.Sleep(2 * time.Second)
			}
		}
	}()

}

func (tm *TaskManager) processMangaInfo(t task) {
	defer tm.currentMangaWg.Done()

	client := tm.proxyManager.GetAvailableProxyClient()
	if client == nil {
		tm.retryTask(t, client)
		return
	}

	slog.Info("Processing manga info", "url", t.Url, "proxy", client.Addr)

	defer func() {
		client.Busy = false
		if err := recover(); err != nil {
			slog.Error("Recovered panic", "url", t.Url, "err", err)
			client.MarkAsBad()
		}
	}()

	parser := query.NewParserManager(client.Addr)
	mangaInfo, err := parser.GetMangaInfo(t.Url)
	if err != nil {
		tm.retryTask(t, client)
		return
	}

	for _, chapterURL := range mangaInfo.Chapters {
		imgTask := task{
			ID:   fmt.Sprintf("chapter_img:%s", chapterURL),
			Type: MangaChapterImgType,
			Url:  chapterURL,
		}
		tm.currentMangaWg.Add(1)
		tm.taskChan <- imgTask
	}
}

func (tm *TaskManager) processChapterImg(t task) {
	defer tm.currentMangaWg.Done()

	client := tm.proxyManager.GetAvailableProxyClient()
	if client == nil {
		tm.retryTask(t, client)
		return
	}

	defer func() {
		client.Busy = false
		if err := recover(); err != nil {
			slog.Error("Recovered panic in chapter img", "url", t.Url, "err", err)
			client.MarkAsBad()
		}
	}()

	parser := query.NewParserManager(client.Addr)
	images, err := parser.GetImgFromChapter(t.Url)
	if err != nil {
		tm.retryTask(t, client)
		return
	}

	log.Printf("Found %d images for chapter %s", len(images), t.Url)
}

func (tm *TaskManager) retryTask(t task, client *proxy.ProxyClient) {
	if client != nil {
		client.MarkAsBad()
	}
	time.Sleep(5 * time.Second)
	tm.taskChan <- t
}

func (tm *TaskManager) Stop() {
	tm.cancel()
	close(tm.taskChan)
}
