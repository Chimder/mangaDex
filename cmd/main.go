package main

import (
	"context"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mangadex/db"
	"mangadex/parser/mangapark"
	"mangadex/parser/metrics"
	"mangadex/parser/proxy"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

var (
	counter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "devbulls_counter",
		Help: "Counting the total number of requests handled",
	})
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

	proxyManager := proxy.NewProxyManager(800)
	go proxyManager.InitProxyManager(ctx)

	// go func() {
	// 	for {
	// 		select {
	// 		case <-ctx.Done():
	// 			return
	// 		default:
	// 			log.Info().
	// 				Int("active", proxyManager.GetProxyCount()).
	// 				Int("testing", int(proxyManager.GetCurrentlyTesting())).
	// 				Int("next_idx", proxyManager.NextIndexAddres).
	// 				Int("total", len(proxyManager.AllAddresses)).
	// 				Msg("Proxy testing status")
	// 			time.Sleep(30 * time.Second)
	// 		}
	// 	}
	// }()

	metric := metrics.NewTracerMetrics()

	taskMng := mangapark.NewTaskManager(ctx, proxyManager, dbConn, s3bucket, metric)
	taskMng.StartWorkers()

	/////////////////////////////////////////////////////////////////////////////////////////////////
	router := gin.Default()

	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	router.Run(":8080")

	log.Info().Msg("Server is running...")
	<-ctx.Done()
	log.Info().Msg("Shutting down server...")

	log.Info().Msg("Server stopped gracefully")
}
