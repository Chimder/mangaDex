package parser

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"log/slog"
	"mangadex/parser/query"
	"net/http"
	"sync"

	"github.com/minio/minio-go/v7"
)

type task struct {
	ID string
	// Type       taskType
	URL        string
	RetryCount int
	MangaId    string
}

func TestMangaChapterTask(ctx context.Context, urll string, bucket *minio.Client) {
	// proxyManager := proxy.NewProxyManager(10)
	parser := query.NewParserManager("socks5://192.111.129.150:4145")
	log.Print("start parse")
	chapterInfo, err := parser.GetImgFromChapter(urll)
	if err != nil {
		slog.Error("Err to get chap imgs", "url", urll, "error", err)
		return
	}

	// reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	// defer cancel()
	var imgWG sync.WaitGroup

	for i, url := range chapterInfo.Images {
		if url == "" {
			slog.Info("Url is empty", "I:", i, "URL", url)
		}
		imgWG.Add(1)
		// slog.Info("add img", "I", i, ":", url)
		go func(i int, url string) {
			defer imgWG.Done()
			var imgBytes []byte
			var contentType string
			// var err error

			var tryCount int
			for tryCount <= 20 {

				resp, err := http.Get(url)
				if err != nil {
					tryCount++
					continue
				}
				defer resp.Body.Close()

				slog.Info("Succ down img", "I", i)
				// imgBytes, contentType, err = query.FilterImg(resp, url)
				if err!= nil || len(imgBytes) == 0 {
					tryCount++
					slog.Warn("image processing failed", "err", err)
					continue
				}
				break
			}

			bucketName := "mangapark"
			objectName := fmt.Sprintf(`%s/%s/%02d%s`, "test3", chapterInfo.Name, i+1, contentType)

			if len(imgBytes) == 0 {
				slog.Warn("skip upload: empty image bytes", "url", url)
				return
			}
			slog.Info("start backet", ":", url)
			_, err = bucket.PutObject(ctx, bucketName, objectName,
				bytes.NewReader(imgBytes), int64(len(imgBytes)), minio.PutObjectOptions{
					ContentType: contentType,
				})
			if err != nil {
				slog.Error("bucket upload failed", "err", err)
				return
			}
		}(i, url)
	}

	slog.Warn("Download and save all chapter imgs", "url", urll, "imgs", len(chapterInfo.Images))
	imgWG.Wait()
}
