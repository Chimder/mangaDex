package db

import (
	"context"
	"log"
	"log/slog"
	"mangadex/config"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
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
		log.Panicf("MinIO connect error: %v", err)
	}

	ctx := context.Background()
	bucketName := "mangapark"

	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		log.Printf("Error checking bucket: %v", err)
	} else {
		log.Printf("Bucket exists: %v", exists)
	}

	slog.Info("MinIO connection OK")
	return minioClient
}
