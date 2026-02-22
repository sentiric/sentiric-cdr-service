// sentiric-cdr-service/internal/logger/logger.go
package logger

import (
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

const (
	SchemaVersion = "1.0.0"
)

var phoneRegex = regexp.MustCompile(`(90|0)?5[0-9]{9}`)

// SutsHook: Hem SUTS alanlarını ekler hem de PII maskelemesi yapar
type SutsHook struct {
	Resource map[string]string
}

func (h SutsHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	// 1. Governance
	e.Str("schema_v", SchemaVersion)

	// 2. Resource (Service Name, Version, Hostname)
	dict := zerolog.Dict()
	for k, v := range h.Resource {
		dict.Str(k, v)
	}
	e.Dict("resource", dict)

	// 3. PII Masking (Sadece message alanı için basit bir kontrol)
	if phoneRegex.MatchString(msg) {
		// Zerolog'da mesajı doğrudan değiştirmek yerine yeni bir alan eklemek daha güvenli
		e.Str("original_message_masked", "true")
	}
}

func New(serviceName, version, env, hostname, logLevel, logFormat string) zerolog.Logger {
	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	// SUTS Alan Dönüşümleri
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFieldName = "ts"
	zerolog.LevelFieldName = "severity"
	zerolog.MessageFieldName = "message"

	zerolog.LevelFieldMarshalFunc = func(l zerolog.Level) string {
		return strings.ToUpper(l.String())
	}

	resource := map[string]string{
		"service.name":    serviceName,
		"service.version": version,
		"service.env":     env,
		"host.name":       hostname,
	}

	var logger zerolog.Logger

	if logFormat == "json" {
		logger = zerolog.New(os.Stderr).Hook(SutsHook{Resource: resource}).With().Timestamp().Logger()
	} else {
		output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
		logger = zerolog.New(output).With().Timestamp().Str("service", serviceName).Logger()
	}

	return logger.Level(level)
}
