package main

import (
	"context"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mangadex/db"
	"mangadex/parser/mangapark"
	"mangadex/parser/proxy"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
)

//		@title			Unofficial MangaDex API
//		@version		0.1
//		@description This is an unofficial REST API for interacting with MangaDex website functionality.
//	  @BasePath	/
func main() {
	SetupLogger()

	log.Info().Msg("Start server")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s3bucket := db.StorageBucket()

	dbConn, err := db.DBConn(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("DB connection failed")
	}

	// startTime := time.Now()

	proxyManager := proxy.NewProxyManager(800)
	go proxyManager.InitProxyManager(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				log.Info().
					Int("active", proxyManager.GetProxyCount()).
					Int("testing", int(proxyManager.GetCurrentlyTesting())).
					Int("next_idx", proxyManager.NextIndexAddres).
					Int("total", len(proxyManager.AllAddresses)).
					Msg("Proxy testing status")
				time.Sleep(30 * time.Second)
			}
		}
	}()

	// for {
	// 	if proxyManager.GetProxyCount() >= 40 {
	// 		break
	// 	}
	// 	time.Sleep(40 * time.Second)
	// }

	taskMng := mangapark.NewTaskManager(ctx, proxyManager, dbConn, s3bucket)
	taskMng.StartWorkers()

	log.Info().Msg("Server is running...")
	<-ctx.Done()
	log.Info().Msg("Shutting down server...")

	log.Info().Msg("Server stopped gracefully")
}
