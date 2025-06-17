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
	Endpoint        string `env:"2B_ENDPOINT"`
	AccessKeyid     string `env:"2B_ACCESS_KEY_ID"`
	SecretAccessKey string `env:"2B_SECRET_ACCESS_KEY"`
}

func LoadEnv() *EnvVars {
	_ = godotenv.Load()
	cfg := EnvVars{}
	if err := env.Parse(&cfg); err != nil {
		log.Fatal(err)
	}
	return &cfg
}
