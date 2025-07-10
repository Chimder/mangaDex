package db

import (
	"context"
	"log"
	"log/slog"
	"mangadex/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func StorageBucket() *minio.Client {
	url := config.LoadEnv()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	minioClient, err := minio.New(url.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(url.AccessKeyid, url.SecretAccessKey, ""),
		Secure: true,
	})
	if err != nil {
		log.Panicf("Err b2 minit conn: %v", err)
	}

	bucketName := "mangadex"
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		log.Printf("Err bucket exists: %v", err)
	}
	log.Printf("ISS %v", exists)
	// if !exists {
	// 	err := minioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
	// 	if err != nil {
	// 		log.Fatalf("Err create Bucket: %v", err)
	// 	}
	// 	log.Printf("Created: %s", bucketName)
	// }
	slog.Info("B2 Bucket Ok")
	return minioClient
}
