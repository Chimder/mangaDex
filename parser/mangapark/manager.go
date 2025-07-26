package mangapark

import (
	"bytes"
	"context"
	"fmt"
	"mangadex/parser/proxy"
	"mangadex/parser/query"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/rs/zerolog/log"
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
	log.Info().Msg("Starting task manager")
	sem := make(chan struct{}, 36)

	for {
		select {
		case <-tm.ctx.Done():
			close(sem)
			log.Info().Msg("Stopping processing")
			return
		default:
			client := tm.proxyManager.GetAvailableProxyClient(tm.ctx)
			if client == nil {
				log.Error().Msg("No proxies available")
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
				log.Info().Int("page", tm.currentPage).Msg("No manga found - stopping")
				tm.cancel()
				close(sem)
				return
			}

			log.Info().Int("count", len(mangaLists)).Int("page", tm.currentPage).Msg("Processing manga list")
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
						log.Warn().Str("title", m.Title).Msg("update manga")
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
			log.Error().Str("url", URL).Int("retry", i).Msg("No available proxy")
			continue
		}
		parser := NewParserManager(client.Addr)
		mangaInfo, errParse = parser.GetMangaInfo(URL)
		if errParse != nil {
			client.MarkAsBad(tm.proxyManager)
			log.Error().Str("url", URL).Err(errParse).Int("retry", i).Msg("Err to get manga info")
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
			log.Info().Str("title", mangaInfo.Title).Int("chapters", len(mangaInfo.Chapters)).Int("retry", i).Msg("Failed insert manga info")
			continue
		}
		break
	}
	if mangaInfo == nil {
		log.Error().Err(errParse).Msg("Max try parse manga info")
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
	log.Info().Str("title", mangaInfo.Title).Int("chapters", len(mangaInfo.Chapters)).Msg("Get manga info")
}

func (tm *TaskManager) handleMangaChapter(URL string, mangaId string) {
	var client *proxy.ProxyClient
	var chapterInfo ChapterInfo
	var err error
	for i := range maxRetries {
		client = tm.proxyManager.GetAvailableProxyClient(tm.ctx)
		if client == nil {
			client.MarkAsBad(tm.proxyManager)
			log.Error().Str("url", URL).Int("retry", i).Msg("No available proxy for chap img")
			continue
		}

		parser := NewParserManager(client.Addr)
		chapterInfo, err = parser.GetImgFromChapter(URL)
		if err != nil {
			log.Error().Str("url", URL).Int("retry", i).Msg("Invalid parsed chapter data")
			client.MarkAsBad(tm.proxyManager)
			continue
		}

		client.MarkAsNotBusy()
		break
	}
	if err != nil || chapterInfo.Name == "" {
		err = fmt.Errorf("parsed chapter has empty name or no images")
		client.MarkAsBad(tm.proxyManager)
		return
	}

	var imgWG sync.WaitGroup
	imgWG.Add(len(chapterInfo.Images))

	safeName := query.SafeChapterNameToS3(chapterInfo.Name)

	for i, url := range chapterInfo.Images {
		go func(url string, idx int) {
			defer imgWG.Done()

			created, errDB := tm.mangaRepo.CreateImgTask(tm.ctx, ImgInfoToChan{
				Url: url, Idx: idx + 1, MangaId: mangaId, ChapterName: safeName,
			})
			if !created {
				log.Error().Err(errDB).Msg("Err create img_task to DB")
				return
			}
		}(url, i)
	}
	imgWG.Wait()

	created, errDB := tm.mangaRepo.CreateChapter(tm.ctx, CreateChapterArg{
		MangaID: mangaId,
		Name:    safeName,
	})
	if !created {
		log.Error().Err(errDB).Str("manga", mangaId).Str("name", safeName).Msg("Failed Create Chapter to DB")
		return
	}
}

func (tm *TaskManager) StartImgWorkerLoop() {
	log.Info().Msg("StartImgWorkerLoop: started")

	interval := 15 * time.Second

	for {
		select {
		case <-tm.ctx.Done():
			log.Info().Msg("ImgWorker ctx cancelled")
			return
		default:
		}

		items, err := tm.mangaRepo.GetImgTasks(tm.ctx)
		if err != nil {
			log.Error().Err(err).Msg("GetImgTasks failed")
			time.Sleep(interval)
			continue
		}

		if len(items) == 0 {
			time.Sleep(interval)
			continue
		}

		log.Debug().Int("tasks", len(items)).Msg("Start IMG Process")
		var wg sync.WaitGroup
		sem := make(chan struct{}, 200)

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
			log.Error().Err(err).Msg("img request")
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
			log.Error().Err(errImg).Str("ext", ext).Str("contentType", contentType).Msg("img filter")
			imgClient.MarkAsBad(tm.proxyManager)
			delay++
			continue
		}
		imgClient.MarkAsNotBusy()
		break
	}
	if errImg != nil || len(imgBytes) <= 0 {
		log.Error().Err(errImg).Msg("img all tries spent")
		return
	}

	err := tm.uploadImgToS3(imgBytes, img.MangaId, img.ChapterName, img.Idx, ext, contentType)
	if err != nil {
		log.Error().Err(err).Msg("retry uploadToS3")
		return
	}

	imgUrl := fmt.Sprintf("%s/%s/%s/%02d%s", "localhost:9000/mangapark", img.MangaId, img.ChapterName, img.Idx, ext)
	err = tm.mangaRepo.UpdateChapterImgByIndex(tm.ctx, img.MangaId, img.ChapterName, img.Idx, imgUrl)
	if err != nil {
		log.Error().Err(err).Msg("failed to update chapter img")
	}

	del, errDel := tm.mangaRepo.DeleteImgTaskByURL(tm.ctx, img.Url)
	if !del {
		log.Error().Err(errDel).Msg("err delete img DB")
		return
	}
}

func (tm *TaskManager) handleChapterUpdateInfo(mangaId string, URL string) {
	oldChapters, err := tm.mangaRepo.GetChaptersNamesByMangaId(tm.ctx, mangaId)
	if err != nil {
		log.Error().Str("url", URL).Msg("No manga chapters info")
		return
	}

	var client *proxy.ProxyClient
	var chapters *MangaInfoParserResp
	for i := range maxRetries {
		client = tm.proxyManager.GetAvailableProxyClient(tm.ctx)
		if client == nil {
			client.MarkAsBad(tm.proxyManager)
			log.Error().Str("url", URL).Int("retry", i).Msg("No available proxy for chap img")
			continue
		}

		parser := NewParserManager(client.Addr)
		chapters, err = parser.GetMangaInfo(URL)
		if err != nil {
			client.MarkAsBad(tm.proxyManager)
			log.Error().Str("url", URL).Err(err).Int("retry", i).Msg("Err to get chap imgs")
			continue
		}

		client.MarkAsNotBusy()
		break
	}
	if err != nil || len(chapters.Chapters) <= 0 {
		log.Error().Str("url", URL).Msg("Failed to get chapters after retries")
		client.MarkAsBad(tm.proxyManager)
		return
	}

	oldChapMap := make(map[string]struct{})
	for _, old := range oldChapters {
		oldChapMap[old.Name] = struct{}{}
	}

	var chapWg sync.WaitGroup
	for _, ch := range chapters.Chapters {
		safeName := query.SafeChapterNameToS3(ch.Name)
		if _, exists := oldChapMap[safeName]; !exists {
			chapWg.Add(1)
			go func(ch Chapter) {
				defer chapWg.Done()
				tm.handleMangaChapter(ch.URL, mangaId)
			}(ch)
		}
	}
	chapWg.Wait()
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
			log.Error().Err(errS3).Int("try", i).Msg("upload s3")
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
	log.Info().Msg("Stop TaskManager.")
}
