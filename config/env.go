package config

import (
	"log"

	"github.com/caarlos0/env/v10"
	"github.com/joho/godotenv"
)

type EnvVars struct {
	ClientID        string `env:"CLIENT_ID"`
	ClientSecret    string `env:"CLIENT_SECRET"`
	DBUrl           string `env:"DB_URL"`
	Debug           bool   `env:"DEBUG"`
	Endpoint        string `env:"S3_ENDPOINT"`
	AccessKeyid     string `env:"S3_ACCESS_KEY_ID"`
	ENV             string `env:"ENV"`
	LOG_LEVEL       string `env:"LOG_LEVEL"`
	SecretAccessKey string `env:"S3_SECRET_ACCESS_KEY"`
}

func LoadEnv() *EnvVars {
	_ = godotenv.Load()
	cfg := EnvVars{}
	if err := env.Parse(&cfg); err != nil {
		log.Fatal(err)
	}
	return &cfg
}
