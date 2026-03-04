// sentiric-cdr-service/cmd/cdr-service/main.go
package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"

	"github.com/sentiric/sentiric-cdr-service/internal/config"
	"github.com/sentiric/sentiric-cdr-service/internal/database"
	"github.com/sentiric/sentiric-cdr-service/internal/handler"
	"github.com/sentiric/sentiric-cdr-service/internal/logger"
	"github.com/sentiric/sentiric-cdr-service/internal/metrics"
	"github.com/sentiric/sentiric-cdr-service/internal/queue"
)

var (
	ServiceVersion string
	GitCommit      string
	BuildDate      string
)

const serviceName = "cdr-service"

func main() {
	cfg, err := config.Load(ServiceVersion)
	if err != nil {
		log.Fatalf("Kritik Hata: Konfigürasyon yüklenemedi: %v", err)
	}

	appLog := logger.New(
		serviceName,
		cfg.ServiceVersion,
		cfg.Env,
		cfg.NodeHostname,
		cfg.LogLevel,
		cfg.LogFormat,
	)

	appLog.Info().
		Str("event", logger.EventSystemStartup).
		Dict("attributes", zerolog.Dict().
			Str("commit", GitCommit).
			Str("build_date", BuildDate)).
		Msg("🚀 cdr-service başlatılıyor (SUTS v4.0)...")

	go metrics.StartServer(cfg.MetricsPort, appLog)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		db, rabbitConn, rabbitCloseChan := setupInfrastructure(ctx, cfg, appLog)
		if ctx.Err() != nil {
			return
		}
		if db != nil {
			defer db.Close()
		}
		if rabbitConn != nil {
			defer rabbitConn.Close()
		}

		// [GÜNCELLEME]: NewEventHandler artık database.DB nesnesini alıyor.
		eventHandler := handler.NewEventHandler(db, appLog, metrics.EventsProcessed, metrics.EventsFailed)

		var consumerWg sync.WaitGroup
		go queue.StartConsumer(ctx, rabbitConn, eventHandler.HandleEvent, appLog, &consumerWg)

		select {
		case <-ctx.Done():
		case err := <-rabbitCloseChan:
			if err != nil {
				appLog.Error().Err(err).Msg("RabbitMQ bağlantısı koptu, servis durduruluyor.")
			}
			cancel()
		}

		appLog.Info().Str("event", logger.EventShutdown).Msg("RabbitMQ tüketicisinin bitmesi bekleniyor...")
		consumerWg.Wait()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	appLog.Warn().Str("event", logger.EventShutdown).Msg("Kapatma sinyali alındı...")
	cancel()

	wg.Wait()
	appLog.Info().Msg("Tüm servisler başarıyla durduruldu. Çıkış yapılıyor.")
}

func setupInfrastructure(ctx context.Context, cfg *config.Config, appLog zerolog.Logger) (
	db *sql.DB,
	rabbitConn *amqp091.Connection,
	closeChan <-chan *amqp091.Error,
) {
	var infraWg sync.WaitGroup
	infraWg.Add(2)

	go func() {
		defer infraWg.Done()
		var err error
		// Veritabanı bağlantısı
		db, err = database.Connect(ctx, cfg.PostgresURL, appLog)
		if err != nil && ctx.Err() == nil {
			appLog.Error().Err(err).Msg("Veritabanı bağlantı denemeleri başarısız oldu.")
		}
	}()

	go func() {
		defer infraWg.Done()
		var err error
		// RabbitMQ bağlantısı
		rabbitConn, closeChan, err = queue.Connect(ctx, cfg.RabbitMQURL, appLog)
		if err != nil && ctx.Err() == nil {
			appLog.Error().Err(err).Msg("RabbitMQ bağlantı denemeleri başarısız oldu.")
		}
	}()

	infraWg.Wait()
	if ctx.Err() != nil {
		appLog.Info().Msg("Altyapı kurulumu iptal edildi.")
		return
	}
	appLog.Info().Str("event", logger.EventInfraReady).Msg("Tüm altyapı bağlantıları başarıyla kuruldu.")
	return
}
