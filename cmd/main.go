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
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

	////////////////////////////////////////////////////////////////////////////////////////////////
	proxyManager := proxy.NewProxyManager(800)
	go proxyManager.InitProxyManager(ctx)

	metric := metrics.NewTracerMetrics()

	taskMng := mangapark.NewTaskManager(ctx, proxyManager, dbConn, s3bucket, metric)
	taskMng.StartWorkers()

	/////////////////////////////////////////////////////////////////////////////////////////////////
	router := gin.New()
	router.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/metrics"},
	}))
	router.Use(gin.Recovery())

	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	go func() {
		if err := router.Run(":8080"); err != nil {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	log.Info().Msg("Server is running...")
	<-ctx.Done()
	log.Info().Msg("Shutting down server...")

	log.Info().Msg("Server stopped gracefully")
}
