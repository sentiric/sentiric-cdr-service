package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func New(serviceName, env string) zerolog.Logger {
	var logger zerolog.Logger

	// DEĞİŞİKLİK: Zaman Damgası Standardizasyonu
	zerolog.TimeFieldFormat = time.RFC3339

	if env == "development" {
		output := zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
		}
		logger = log.Output(output).With().Timestamp().Str("service", serviceName).Logger()
	} else {
		logger = zerolog.New(os.Stderr).With().Timestamp().Str("service", serviceName).Logger()
	}

	return logger
}
