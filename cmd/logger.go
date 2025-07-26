package main

import (
	"mangadex/config"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func SetupLogger() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	env := config.LoadEnv()
	if env.ENV == "dev" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if lvl, err := zerolog.ParseLevel(env.LOG_LEVEL); err == nil {
		zerolog.SetGlobalLevel(lvl)
	}
}
