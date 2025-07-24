package main

import (
	"context"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"log/slog"
	"mangadex/db"
	"mangadex/parser/mangapark"
	"mangadex/parser/proxy"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s3bucket := db.StorageBucket()

	/////////////////////////////////////////////////////
	// url := "https://mangapark.io/title/10749-en-one-punch-man/8596918-punch-202"
	// parser.TestMangaChapterTask(ctx, url, s3bucket)
	/////////////////////////////////////////////////////
	db, err := db.DBConn(ctx)
	if err != nil {
		log.Fatalf("DB conn %v", err)
	}
	times := time.Now()

	proxyManager := proxy.NewProxyManager(800)
	go proxyManager.InitProxyManager(ctx)

	for {
		if proxyManager.GetProxyCount() >= 35 {
			break
		}
		slog.Info("Waiting for proxies to be ready...",
			"current", proxyManager.GetProxyCount(),
			"required", proxyManager.MaxConn)
		time.Sleep(30 * time.Second)
	}

	taskMng := mangapark.NewTaskManager(ctx, proxyManager, db, s3bucket)
	go taskMng.StartPageParseWorker()
	taskMng.StartImgWorkerLoop()

	log.Printf("nextIND %v of %v", proxyManager.NextIndexAddres, len(proxyManager.AllAddresses))
	log.Printf("elapsed %v", time.Since(times))
	//////////////////////////////////////////////////////////////////////////////////////////

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
