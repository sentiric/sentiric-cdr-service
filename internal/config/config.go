// sentiric-cdr-service/internal/config/config.go
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Env            string
	LogLevel       string
	LogFormat      string
	NodeHostname   string
	ServiceVersion string
	PostgresURL    string
	RabbitMQURL    string
	MetricsPort    string
}

func Load(version string) (*Config, error) {
	_ = godotenv.Load()

	// ServiceVersion artık build-time'dan geliyor, eğer boşsa default kullanılıyor.
	if version == "" {
		version = "0.0.0-dev"
	}

	cfg := &Config{
		Env:            getEnvWithDefault("ENV", "production"),
		LogLevel:       getEnvWithDefault("LOG_LEVEL", "info"),
		LogFormat:      getEnvWithDefault("LOG_FORMAT", "json"),
		NodeHostname:   getEnvWithDefault("NODE_HOSTNAME", "localhost"),
		ServiceVersion: version,
		PostgresURL:    getEnv("POSTGRES_URL"),
		RabbitMQURL:    getEnv("RABBITMQ_URL"),
		MetricsPort:    getEnvWithDefault("CDR_SERVICE_METRICS_PORT", "12052"),
	}

	if cfg.PostgresURL == "" || cfg.RabbitMQURL == "" {
		missingVars := ""
		if cfg.PostgresURL == "" {
			missingVars += " POSTGRES_URL"
		}
		if cfg.RabbitMQURL == "" {
			missingVars += " RABBITMQ_URL"
		}
		return nil, fmt.Errorf("kritik ortam değişkenleri eksik:%s", missingVars)
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
