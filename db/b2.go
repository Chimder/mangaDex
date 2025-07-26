package db

import (
	"context"
	"mangadex/config"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rs/zerolog/log"
)

func StorageBucket() *minio.Client {
	url := config.LoadEnv()

	secure := true
	if strings.Contains(url.Endpoint, "localhost") ||
		strings.Contains(url.Endpoint, "127.0.0.1") {
		secure = false
	}

	minioClient, err := minio.New(url.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(url.AccessKeyid, url.SecretAccessKey, ""),
		Secure: secure,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("MinIO connection failed")
	}

	ctx := context.Background()
	bucketName := "mangapark"

	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to check bucket")
	} else {
		log.Debug().Bool("exists", exists).Str("bucket", bucketName).Msg("Bucket status checked")
	}

	log.Info().Msg("MinIO connection OK")
	return minioClient
}
