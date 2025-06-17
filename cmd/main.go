package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"log/slog"
	"mangadex/db"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/chai2010/webp"
	_ "github.com/lib/pq"
	"github.com/minio/minio-go/v7"
)

func ConvertToWebp(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	err := webp.Encode(&buf, img, &webp.Options{Lossless: false, Quality: 78})
	if err != nil {
		return nil, fmt.Errorf("webp encode failed: %w", err)
	}
	return buf.Bytes(), nil
}

func filterImg(resp *http.Response, url string) ([]byte, string, error) {
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		ext := filepath.Ext(url)
		contentType = mime.TypeByExtension(ext)
	}
	slog.Info("Processing image", "contentType", contentType, "url", url)

	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, contentType, fmt.Errorf("read body: %w", err)
	}
	if len(imgBytes) == 0 {
		return nil, contentType, fmt.Errorf("empty image data")
	}

	switch contentType {
	case "image/webp":
		return imgBytes, "image/webp", nil

	case "image/jpeg", "image/jpg", "image/png", "image/gif":
		img, _, err := image.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			return nil, contentType, fmt.Errorf("image decode failed: %w", err)
		}
		imgBytes, err := ConvertToWebp(img)
		if err != nil {
			return nil, "image/webp", fmt.Errorf("convert to webp failed: %w", err)
		}
		return imgBytes, "image/webp", nil

	default:
		return nil, "", fmt.Errorf("unsupported image format: %s", contentType)
	}
}

//		@title			Unofficial MangaDex API
//		@version		0.1
//		@description This is an unofficial REST API for interacting with MangaDex website functionality.
//	  @BasePath	/
func main() {
	LoggerInit()
	slog.Info("Start server")

	s3bucket := db.StorageBucket()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	url := "https://cdn4.mangaclash.com/chainsaw-man-20461/chapter-205/15.jpg"

	resp, err := http.Get(url)
	if err != nil {
		slog.Warn("http get failed", "err", err)
		return
	}
	defer resp.Body.Close()

	imgBytes, contentType, err := filterImg(resp, url)
	if err != nil {
		slog.Warn("image processing failed", "err", err)
		return
	}

	bucketName := "mangaParse"
	objectName := "55150-en-jujutsu-kaisen/8218084-chapter-24v5.webp"

	uploadInfo, err := s3bucket.PutObject(ctx, bucketName, objectName,
		bytes.NewReader(imgBytes), int64(len(imgBytes)), minio.PutObjectOptions{
			ContentType: contentType,
		})
	if err != nil {
		slog.Warn("bucket upload failed", "err", err)
		return
	}

	log.Printf("Uploaded to bucket: %+v", uploadInfo)
	// times := time.Now()

	// proxyManager := proxy.NewProxyManager(500)
	// go proxyManager.InitProxyManager(ctx)
	// go proxyManager.AutoCleanup(ctx, 10*time.Second)

	// for {
	// 	if proxyManager.GetProxyCount() >= 25 {
	// 		break
	// 	}
	// 	slog.Info("Waiting for proxies to be ready...",
	// 		"current", proxyManager.GetProxyCount(),
	// 		"required", proxyManager.MaxConn)
	// 	time.Sleep(2 * time.Minute)
	// }

	// taskMng := tasks.NewTaskManager(ctx, proxyManager, s3bucket)
	// taskMng.Start()

	// log.Printf("nextIND %v of %v", proxyManager.NextIndexAddres, len(proxyManager.AllAddresses))
	// log.Printf("elapsed %v", time.Since(times))
	////////////////////
	////////////////////
	//////////////////////
	//////////////////////
	////////////////////
	//////////////////////
	// r := handler.Init()

	// srv := &http.Server{
	// 	Addr:         ":8080",
	// 	Handler:      r,
	// 	ReadTimeout:  5 * time.Second,
	// 	WriteTimeout: 10 * time.Second,
	// 	IdleTimeout:  120 * time.Second,
	// }

	// go func() {
	// 	if err := srv.ListenAndServe(); err != nil {
	// 		log.Fatalf("Server error: %v", err)
	// 	}
	// }()

	slog.Info("Server is running...")
	<-ctx.Done()
	slog.Info("Shutting down server...")

	// defer taskMng.Stop()
	// shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	// defer cancel()

	// if err := srv.Shutdown(shutdownCtx); err != nil {
	// 	slog.Error("Server shutdown error", "error", err)
	// } else {
	slog.Info("Server stopped gracefully")
	// }
}
