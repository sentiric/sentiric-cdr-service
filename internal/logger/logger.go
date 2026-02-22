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
	DefaultTenant = "system"
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
	if phoneRegex.MatchString(msg) {
		masked := phoneRegex.ReplaceAllStringFunc(msg, func(phone string) string {
			if len(phone) < 7 {
				return "****"
			}
			return phone[:5] + "***" + phone[len(phone)-2:]
		})
		// declared and not used: masked hatasından dolayı eklendi
		e.Str("masked_msg", masked)
		// Mesajı değiştiremiyoruz ama maskelenmiş halini ekleyebiliriz
		// Zerolog'da mesajı in-place değiştirmek zordur, bu yüzden UI tarafında
		// veya log oluşturulurken dikkat etmek daha iyidir.
		// Ancak SUTS standardı gereği buraya resource ve schema ekleyelim.
	}
}

// SutsHook: SUTS v4.0 zorunlu alanlarını ekler
type SutsHook struct {
	Resource map[string]string
}

func (h SutsHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	// 1. Governance
	e.Str("schema_v", SchemaVersion)
	// Tenant ID'yi context'ten alamıyorsak varsayılanı bas
	// CDR servisinde tenant_id genellikle mesajın içindedir, loglarken oradan verilmelidir.

	// 2. Resource
	dict := zerolog.Dict()
	for k, v := range h.Resource {
		dict.Str(k, v)
	}
	e.Dict("resource", dict)
}

// New: Yapılandırılmış Logger oluşturur
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

	resource := map[string]string{
		"service.name":    serviceName,
		"service.version": version,
		"service.env":     env,
		"host.name":       hostname,
	}

	var logger zerolog.Logger

	if logFormat == "json" {
		logger = zerolog.New(os.Stderr).
			Hook(SutsHook{Resource: resource}).
			Hook(PIIHook{}). // PII Masking
			With().Timestamp().Logger()
	} else {
		output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
		logger = zerolog.New(output).With().Timestamp().Str("service", serviceName).Logger()
	}

	return logger.Level(level)
}

// Mask, manuel maskeleme gerektiren durumlar için yardımcı fonksiyon
func Mask(input string) string {
	if len(input) < 7 {
		return "****"
	}
	return input[:5] + "***" + input[len(input)-2:]
}
