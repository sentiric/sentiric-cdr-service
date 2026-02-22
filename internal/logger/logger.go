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

// New: SUTS v4.0 uyumlu, production-ready bir Logger oluşturur.
func New(serviceName, version, env, hostname, logLevel, logFormat string) zerolog.Logger {
	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	// --- ZEROLOG GLOBAL AYARLARI (SUTS v4.0 UYUMLULUĞU) ---
	zerolog.TimeFieldFormat = time.RFC3339Nano // SUTS standardı
	zerolog.TimestampFieldName = "ts"
	zerolog.LevelFieldName = "severity"
	zerolog.MessageFieldName = "message"

	// Severity değerlerini standart gereği büyük harf yap (örn: info -> INFO)
	zerolog.LevelFieldMarshalFunc = func(l zerolog.Level) string {
		return strings.ToUpper(l.String())
	}

	// Resource objesi, her log kaydına eklenecek olan servis kimliğidir.
	resourceContext := zerolog.Dict().
		Str("service.name", serviceName).
		Str("service.version", version).
		Str("service.env", env).
		Str("host.name", hostname)

	var logger zerolog.Logger

	if logFormat == "json" {
		// Production Ortamı: Standart JSON çıktısı
		logger = zerolog.New(os.Stderr).With().
			Timestamp().
			Str("schema_v", SchemaVersion).
			Dict("resource", resourceContext).
			Logger()
	} else {
		// Geliştirme Ortamı: Okunabilir konsol çıktısı
		output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
		logger = zerolog.New(output).With().
			Timestamp().
			Str("service", serviceName). // Geliştirme ortamında da servis adını görmek faydalıdır.
			Logger()
	}

	return logger.Level(level)
}
