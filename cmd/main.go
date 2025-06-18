package main

import (
	"context"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"log/slog"
	"mangadex/db"
	"mangadex/parser/proxy"
	"mangadex/parser/tasks"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
)

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

	db, err := db.DBConn(ctx)
	if err != nil {
		log.Fatalf("DB conn %w", err)
	}
	// url := "https://cdn4.mangaclash.com/chainsaw-man-20461/chapter-205/15.jpg"

	// resp, err := http.Get(url)
	// if err != nil {
	// 	slog.Warn("http get failed", "err", err)
	// 	return
	// }
	// defer resp.Body.Close()

	// imgBytes, contentType, err := filterImg(resp, url)
	// if err != nil {
	// 	slog.Warn("image processing failed", "err", err)
	// 	return
	// }

	// bucketName := "mangapark"
	// objectName := "55150-en-jujutsu-kaisen/8218084-chapter-24v8.webp"

	// uploadInfo, err := s3bucket.PutObject(ctx, bucketName, objectName,
	// 	bytes.NewReader(imgBytes), int64(len(imgBytes)), minio.PutObjectOptions{
	// 		ContentType: contentType,
	// 	})
	// if err != nil {
	// 	slog.Warn("bucket upload failed", "err", err)
	// 	return
	// }

	// log.Printf("Uploaded to bucket: %+v", uploadInfo)

	times := time.Now()

	proxyManager := proxy.NewProxyManager(700)
	go proxyManager.InitProxyManager(ctx)
	go proxyManager.AutoCleanup(ctx, 3*time.Second)

	for {
		if proxyManager.GetProxyCount() >= 20 {
			break
		}
		slog.Info("Waiting for proxies to be ready...",
			"current", proxyManager.GetProxyCount(),
			"required", proxyManager.MaxConn)
		time.Sleep(30 * time.Second)
	}

	taskMng := tasks.NewTaskManager(ctx, proxyManager, db, s3bucket)
	taskMng.Start()

	log.Printf("nextIND %v of %v", proxyManager.NextIndexAddres, len(proxyManager.AllAddresses))
	log.Printf("elapsed %v", time.Since(times))
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
