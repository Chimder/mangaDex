package parser

type task struct {
	ID string
	// Type       taskType
	URL        string
	RetryCount int
	MangaId    string
}

// func TestMangaChapterTask(ctx context.Context, url string, bucket *minio.Client) {
// 	parser := query.NewParserManager()
// 	log.Print("start parse")
// 	chapterInfo, err := parser.GetImgFromChapter(url)
// 	if err != nil {
// 		slog.Error("Err to get chap imgs", "url", url, "error", err)
// 		return
// 	}
// 	// reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
// 	// defer cancel()
// 	var imgWG sync.WaitGroup

// 	for i, url := range chapterInfo.Images {
// 		imgWG.Add(1)
// 		go func(url string) {
// 			defer imgWG.Done()
// 			var imgBytes []byte
// 			var contentType string
// 			var err error

// 			var tryCount int

// 			for tryCount <= 100 {
// 				resp, err := http.Get(url)
// 				if err != nil {
// 					tryCount++
// 					continue
// 				}

// 				imgBytes, contentType, err = query.FilterImg(resp, url)
// 				resp.Body.Close()
// 				if err != nil {
// 					tryCount++
// 					slog.Warn("image processing failed", "err", err)
// 					continue
// 				}
// 				break
// 			}

// 			bucketName := "mangapark"
// 			objectName := fmt.Sprintf(`%s/%s/%02d%s`, "test", chapterInfo.Name, i+1, contentType)

// 			_, err = bucket.PutObject(ctx, bucketName, objectName,
// 				bytes.NewReader(imgBytes), int64(len(imgBytes)), minio.PutObjectOptions{
// 					ContentType: contentType,
// 				})
// 			if err != nil {
// 				slog.Error("bucket upload failed", "err", err)
// 				return
// 			}
// 		}(url)
// 	}

// 	imgWG.Wait()
// 	slog.Warn("Download and save all chapter imgs", "url", url, "imgs", len(chapterInfo.Images))
// }
