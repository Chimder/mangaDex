package db

import (
	"context"
	"log"
	"mangadex/config"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
)

func DBConn(ctx context.Context) (*pgxpool.Pool, error) {
	url := config.LoadEnv().DBUrl

	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		log.Fatalf("Failed to parse config DBPOOL: %v", err)
	}

	config.MaxConns = 5
	config.MinConns = 1
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 15 * time.Minute
	config.HealthCheckPeriod = 2 * time.Minute
	config.ConnConfig.ConnectTimeout = 5 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Println("Database pool created successfully")
	return pool, nil
}