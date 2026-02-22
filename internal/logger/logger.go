// sentiric-cdr-service/internal/logger/logger.go
package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

const (
	SchemaVersion = "1.0.0"
)

// (PIIHook ve phoneRegex tanımları aynı kalır)

// New: SUTS v4.0 uyumlu Logger oluşturur.
func New(serviceName, version, env, hostname, logLevel, logFormat string) zerolog.Logger {
	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFieldName = "ts"
	zerolog.LevelFieldName = "severity"
	zerolog.MessageFieldName = "message"

	zerolog.LevelFieldMarshalFunc = func(l zerolog.Level) string {
		return strings.ToUpper(l.String())
	}

	resourceContext := zerolog.Dict().
		Str("service.name", serviceName).
		Str("service.version", version).
		Str("service.env", env).
		Str("host.name", hostname)

	var logger zerolog.Logger

	if logFormat == "json" {
		logger = zerolog.New(os.Stderr).With().
			Timestamp().
			Str("schema_v", SchemaVersion).
			Dict("resource", resourceContext).
			Logger()
	} else {
		output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
		logger = zerolog.New(output).With().Timestamp().Str("service", serviceName).Logger()
	}

	return logger.Level(level)
}
