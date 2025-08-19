// ========== FILE: sentiric-cdr-service/internal/config/config.go ==========
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Env         string
	PostgresURL string
	RabbitMQURL string
	// DÜZELTME: QueueName artık kod içinde sabit olduğu için bu alana gerek kalmadı.
	// QueueName   string
	MetricsPort string

	// gRPC İstemci Ayarları
	UserServiceGrpcURL string
	CdrServiceCertPath string
	CdrServiceKeyPath  string
	GrpcTlsCaPath      string
}

func Load() (*Config, error) {
	godotenv.Load()

	cfg := &Config{
		Env:         getEnvWithDefault("ENV", "production"),
		PostgresURL: getEnv("POSTGRES_URL"),
		RabbitMQURL: getEnv("RABBITMQ_URL"),
		// DÜZELTME: Artık bu satıra gerek yok.
		// QueueName:          getEnvWithDefault("CDR_QUEUE_NAME", "call.events"),
		MetricsPort:        getEnvWithDefault("METRICS_PORT", "9092"),
		UserServiceGrpcURL: getEnv("USER_SERVICE_GRPC_URL"),
		CdrServiceCertPath: getEnv("CDR_SERVICE_CERT_PATH"),
		CdrServiceKeyPath:  getEnv("CDR_SERVICE_KEY_PATH"),
		GrpcTlsCaPath:      getEnv("GRPC_TLS_CA_PATH"),
	}

	if cfg.PostgresURL == "" || cfg.RabbitMQURL == "" || cfg.UserServiceGrpcURL == "" {
		return nil, fmt.Errorf("kritik ortam değişkenleri eksik: POSTGRES_URL, RABBITMQ_URL, USER_SERVICE_GRPC_URL")
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
