// sentiric-cdr-service/internal/logger/logger.go
package logger

import (
	"os"
	"regexp"
	"time"

	"github.com/rs/zerolog"
)

var (
	// PII Maskeleme için Regex'ler
	phoneRegex = regexp.MustCompile(`(\+?90|0)?5[0-9]{9}`)
)

type PiiFilter struct{}

func (f PiiFilter) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	// Mesaj içinde telefon numarası varsa maskele
	if phoneRegex.MatchString(msg) {
		masked := phoneRegex.ReplaceAllString(msg, "905XXXXXXXXX")
		e.Str("masked_msg", masked)
	}
}

func New(serviceName, env, logLevel string) zerolog.Logger {
	level, _ := zerolog.ParseLevel(logLevel)
	zerolog.TimeFieldFormat = time.RFC3339

	var output zerolog.ConsoleWriter
	if env == "development" {
		output = zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	}

	var logger zerolog.Logger
	if env == "development" {
		logger = zerolog.New(output).With().Timestamp().Str("service", serviceName).Logger()
	} else {
		logger = zerolog.New(os.Stderr).With().Timestamp().Str("service", serviceName).Logger()
	}

	// PII Filtresini Uygula
	return logger.Level(level).Hook(PiiFilter{})
}
