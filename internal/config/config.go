package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config, servis için tüm yapılandırma değerlerini tutar.
type Config struct {
	Env         string
	PostgresURL string
	RabbitMQURL string
	QueueName   string
	MetricsPort string
}

// Load, ortam değişkenlerini doğru öncelik sırasıyla yükler.
func Load() (*Config, error) {
	godotenv.Load() // Yerel geliştirme için .env dosyasını arar

	cfg := &Config{
		Env:         getEnvWithDefault("ENV", "production"),
		PostgresURL: getEnv("POSTGRES_URL"),
		RabbitMQURL: getEnv("RABBITMQ_URL"),
		QueueName:   getEnvWithDefault("CDR_QUEUE_NAME", "call.events"),
		MetricsPort: getEnvWithDefault("METRICS_PORT", "9092"),
	}

	if cfg.PostgresURL == "" || cfg.RabbitMQURL == "" {
		return nil, fmt.Errorf("kritik ortam değişkenleri eksik: POSTGRES_URL, RABBITMQ_URL")
	}

	return cfg, nil
}

func getEnv(key string) string {
	return os.Getenv(key)
}

func getEnvWithDefault(key, defaultValue string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	return val
}
