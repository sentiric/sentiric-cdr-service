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
	// Bu değişkenler derleme zamanında ldflags ile doldurulur.
	ServiceVersion string
	GitCommit      string
	BuildDate      string
)

const serviceName = "cdr-service"

func main() {
	// GÜNCELLEME: ServiceVersion artık config'den değil, build-time'dan geliyor.
	cfg, err := config.Load(ServiceVersion)
	if err != nil {
		// Henüz logger olmadığı için standart log kullanıyoruz.
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

	// SUTS v4.0 UYUMLU BAŞLANGIÇ LOGU
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

		db, rabbitCh, rabbitCloseChan := setupInfrastructure(ctx, cfg, appLog)
		if ctx.Err() != nil {
			return
		}
		if db != nil {
			defer db.Close()
		}
		if rabbitCh != nil {
			defer rabbitCh.Close()
		}

		eventHandler := handler.NewEventHandler(db, appLog, metrics.EventsProcessed, metrics.EventsFailed)

		var consumerWg sync.WaitGroup
		go queue.StartConsumer(ctx, rabbitCh, eventHandler.HandleEvent, appLog, &consumerWg)

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
	rabbitCh *amqp091.Channel,
	closeChan <-chan *amqp091.Error,
) {
	var infraWg sync.WaitGroup
	infraWg.Add(2)

	go func() {
		defer infraWg.Done()
		var err error
		db, err = database.Connect(ctx, cfg.PostgresURL, appLog)
		if err != nil && ctx.Err() == nil {
			appLog.Error().Err(err).Msg("Veritabanı bağlantı denemeleri başarısız oldu.")
		}
	}()

	go func() {
		defer infraWg.Done()
		var err error
		rabbitCh, closeChan, err = queue.Connect(ctx, cfg.RabbitMQURL, appLog)
		if err != nil && ctx.Err() == nil {
			appLog.Error().Err(err).Msg("RabbitMQ bağlantı denemeleri başarısız oldu.")
		}
	}()

	infraWg.Wait()
	if ctx.Err() != nil {
		appLog.Info().Msg("Altyapı kurulumu, servis kapatıldığı için iptal edildi.")
		return
	}
	appLog.Info().Str("event", logger.EventInfraReady).Msg("Tüm altyapı bağlantıları başarıyla kuruldu.")
	return
}
