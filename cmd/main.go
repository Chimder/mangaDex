package main

import (
	"context"
	"log"
	"log/slog"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Print("Start")
	times := time.Now()

	proxyManager := proxy.NewProxyManager(18)
	go proxyManager.InitProxyManager(ctx)
	go proxyManager.AutoCleanup(ctx, 10*time.Second)

	for {
		if proxyManager.GetProxyCount() >= proxyManager.MaxConn {
			break
		}
		slog.Info("Waiting for proxies to be ready...",
			"current", proxyManager.GetProxyCount(),
			"required", proxyManager.MaxConn)
		time.Sleep(2 * time.Minute)
	}

	taskMng := tasks.NewTaskManager(proxyManager, ctx)
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

	defer taskMng.Stop()
	// shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	// defer cancel()

	// if err := srv.Shutdown(shutdownCtx); err != nil {
	// 	slog.Error("Server shutdown error", "error", err)
	// } else {
	slog.Info("Server stopped gracefully")
	// }
}
