// sentiric-cdr-service/internal/logger/logger.go
package logger

import (
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

var (
	// Türkiye telefon formatı regex
	phoneRegex = regexp.MustCompile(`(90|0)?5[0-9]{9}`)
)

// PIIHook, hassas verileri otomatik maskeler
type PIIHook struct{}

func (h PIIHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	if msg == "" {
		return
	}

	// Mesaj içinde telefon numarası varsa maskele
	if phoneRegex.MatchString(msg) {
		masked := phoneRegex.ReplaceAllStringFunc(msg, func(phone string) string {
			if len(phone) < 7 {
				return "****"
			}
			// Örn: 905548777858 -> 90554***58
			return phone[:5] + "***" + phone[len(phone)-2:]
		})
		e.Str("masked_msg", masked)
	}
}

// New, Sentiric standartlarına uygun konfigüre edilmiş bir logger döner
func New(serviceName, env, logLevel string) zerolog.Logger {
	level, err := zerolog.ParseLevel(strings.ToLower(logLevel))
	if err != nil {
		level = zerolog.InfoLevel
	}

	zerolog.TimeFieldFormat = time.RFC3339
	zerolog.MessageFieldName = "msg"

	var logger zerolog.Logger
	if env == "development" {
		output := zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: "15:04:05",
			NoColor:    false,
		}
		logger = zerolog.New(output).With().Timestamp().Str("svc", serviceName).Logger()
	} else {
		// Üretim ortamında saf JSON
		logger = zerolog.New(os.Stderr).With().Timestamp().Str("svc", serviceName).Logger()
	}

	// PII Maskeleme ve Global Hookları ekle
	return logger.Level(level).Hook(PIIHook{})
}

// Mask, manuel maskeleme gerektiren durumlar için yardımcı fonksiyon
func Mask(input string) string {
	if len(input) < 7 {
		return "****"
	}
	return input[:5] + "***" + input[len(input)-2:]
}
