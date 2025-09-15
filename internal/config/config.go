// ========== FILE: sentiric-cdr-service/internal/config/config.go ==========
package config

import (
	"fmt"
	"os"
)

type Config struct {
	Env         string
	PostgresURL string
	RabbitMQURL string
	MetricsPort string

	// gRPC İstemci Ayarları
	UserServiceGrpcURL string
	CdrServiceCertPath string
	CdrServiceKeyPath  string
	GrpcTlsCaPath      string
}

func Load() (*Config, error) {
	// godotenv.Load() çağrısı kaldırıldı.

	cfg := &Config{
		Env:                getEnvWithDefault("ENV", "production"),
		PostgresURL:        getEnv("POSTGRES_URL"),
		RabbitMQURL:        getEnv("RABBITMQ_URL"),
		MetricsPort:        getEnvWithDefault("CDR_SERVICE_METRICS_PORT", "12052"),
		UserServiceGrpcURL: getEnv("USER_SERVICE_GRPC_URL"),
		CdrServiceCertPath: getEnv("CDR_SERVICE_CERT_PATH"),
		CdrServiceKeyPath:  getEnv("CDR_SERVICE_KEY_PATH"),
		GrpcTlsCaPath:      getEnv("GRPC_TLS_CA_PATH"),
	}

	if cfg.PostgresURL == "" || cfg.RabbitMQURL == "" || cfg.UserServiceGrpcURL == "" {
		// Hata mesajını daha anlaşılır hale getirelim.
		missingVars := ""
		if cfg.PostgresURL == "" { missingVars += " POSTGRES_URL" }
		if cfg.RabbitMQURL == "" { missingVars += " RABBITMQ_URL" }
		if cfg.UserServiceGrpcURL == "" { missingVars += " USER_SERVICE_GRPC_URL" }
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