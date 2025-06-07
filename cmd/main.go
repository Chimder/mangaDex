package main

import (
	"context"
	"log"
	"log/slog"
	"mangadex/parser/query"
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

	// proxy := []string{"http://164.68.101.70:8888", "http://57.129.81.201:8081"}

	log.Print("Start")
	times := time.Now()
	// urlChapters := "https://mangapark.io/title/105625-en-isekai-meikyuu-de-harem-o/7241726-chapter-59-genghis-khan-5"
	urlInfo := "https://mangapark.io/title/10749-en-one-punch-man"

	parser := query.NewParserManager()
	genres, chapters := parser.GetGenres(urlInfo)

	log.Printf("Gen %s\n", genres)
	log.Printf("Chap %s\n", chapters)
	log.Printf("time %v", time.Since(times))
	///////////////////////////////////////////////////////////////////////////
	// proxyManager := proxy.NewProxyManager(20)
	// go proxyManager.InitProxyManager(ctx)
	// go proxyManager.AutoCleanup(ctx, 5*time.Second)

	// for {
	// 	if proxyManager.GetProxyCount() >= proxyManager.MaxConn {
	// 		break
	// 	}
	// 	slog.Info("Waiting for proxies to be ready...",
	// 		"current", proxyManager.GetProxyCount(),
	// 		"required", proxyManager.MaxConn)
	// 	time.Sleep(5 * time.Second)
	// }
	// urlToCheck := "https://api.mangadex.org/manga?includes[]=cover_art&contentRating[]=safe&contentRating[]=suggestive&contentRating[]=erotica&order[rating]=desc&limit=18"
	// var wg sync.WaitGroup

	// for _, v := range proxyManager.ProxyClients {
	// 	wg.Add(1)
	// 	go func(v *proxy.ProxyClient) {
	// 		defer wg.Done()

	// 		resp, err := v.Client.Get(urlToCheck)
	// 		if err != nil {
	// 			slog.Warn("Request failed", "proxy", v.Addr, "error", err)
	// 			v.MarkAsBad()
	// 			return
	// 		}
	// 		defer resp.Body.Close()

	// 		if resp.StatusCode == http.StatusOK {
	// 			slog.Info("Proxy works", "proxy", v.Addr)
	// 		} else {
	// 			slog.Warn("Bad status code", "proxy", v.Addr, "code", resp.StatusCode)
	// 			v.MarkAsBad()
	// 		}
	// 	}(v)
	// }
	// wg.Wait()
	////////////////////////////////////////////////////

	// total, working := pm.GetStats()
	// log.Printf("Final result: %d working proxies out of %d total", working, total)
	// for addr, client := range proxyManager.ProxyClients {

	// }
	// prox.GetTxtProxy()
	// prox.TestProxy()
	// log.Fatalf("added is %v", len(prox.Addresses))
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

	// shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	// defer cancel()

	// if err := srv.Shutdown(shutdownCtx); err != nil {
	// 	slog.Error("Server shutdown error", "error", err)
	// } else {
	slog.Info("Server stopped gracefully")
	// }
}
