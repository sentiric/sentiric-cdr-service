// ========== FILE: sentiric-cdr-service/cmd/cdr-service/main.go ==========
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sentiric/sentiric-cdr-service/internal/client"
	"github.com/sentiric/sentiric-cdr-service/internal/config"
	"github.com/sentiric/sentiric-cdr-service/internal/database"
	"github.com/sentiric/sentiric-cdr-service/internal/handler"
	"github.com/sentiric/sentiric-cdr-service/internal/logger"
	"github.com/sentiric/sentiric-cdr-service/internal/metrics"
	"github.com/sentiric/sentiric-cdr-service/internal/queue"
)

const serviceName = "cdr-service"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Konfigürasyon yüklenemedi: %v", err)
	}

	appLog := logger.New(serviceName, cfg.Env)
	appLog.Info().Msg("Konfigürasyon başarıyla yüklendi.")

	go metrics.StartServer(cfg.MetricsPort, appLog)

	db, err := database.Connect(cfg.PostgresURL, appLog)
	if err != nil {
		appLog.Fatal().Err(err).Msg("Veritabanı bağlantısı kurulamadı")
	}
	defer db.Close()

	userClient, err := client.NewUserServiceClient(cfg)
	if err != nil {
		appLog.Fatal().Err(err).Msg("User Service gRPC istemcisi oluşturulamadı")
	}

	eventHandler := handler.NewEventHandler(db, userClient, appLog, metrics.EventsProcessed, metrics.EventsFailed)

	rabbitCh, closeChan := queue.Connect(cfg.RabbitMQURL, appLog)
	if rabbitCh != nil {
		defer rabbitCh.Close()
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// DÜZELTME: Artık kullanılmayan cfg.QueueName'i kaldırıyoruz.
	go queue.StartConsumer(ctx, rabbitCh, eventHandler.HandleEvent, appLog, &wg)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		appLog.Info().Str("signal", sig.String()).Msg("Kapatma sinyali alındı, servis durduruluyor...")
	case err := <-closeChan:
		if err != nil {
			appLog.Error().Err(err).Msg("RabbitMQ bağlantısı koptu.")
		}
	}

	cancel()

	appLog.Info().Msg("Mevcut işlemlerin bitmesi bekleniyor...")
	waitChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitChan)
	}()

	select {
	case <-waitChan:
		appLog.Info().Msg("Tüm işlemler başarıyla tamamlandı. Çıkış yapılıyor.")
	case <-time.After(10 * time.Second):
		appLog.Warn().Msg("Graceful shutdown zaman aşımına uğradı. Çıkış yapılıyor.")
	}
}
