package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
)

const (
	exchangeName = "sentiric_events"
	cdrQueueName = "sentiric.cdr_service.events"
	// Worker sayısı kadar eşzamanlı işlem limiti
	maxConcurrentWorkers = 10
)

// Connect, RabbitMQ bağlantısını yönetir ve kopma durumunda yeniden bağlanmayı bekler.
func Connect(ctx context.Context, url string, log zerolog.Logger) (*amqp091.Channel, <-chan *amqp091.Error, error) {
	var conn *amqp091.Connection
	var err error
	
	// Retry backoff policy
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}

		log.Debug().Msg("RabbitMQ bağlantısı deneniyor...")
		conn, err = amqp091.Dial(url)
		if err == nil {
			log.Info().Msg("RabbitMQ bağlantısı başarılı.")
			
			ch, chErr := conn.Channel()
			if chErr != nil {
				conn.Close() // Kanal açılmazsa bağlantıyı da kapat
				return nil, nil, fmt.Errorf("RabbitMQ kanalı oluşturulamadı: %w", chErr)
			}
			
			closeChan := make(chan *amqp091.Error)
			conn.NotifyClose(closeChan)
			
			return ch, closeChan, nil
		}

		if ctx.Err() == nil {
			log.Warn().Err(err).Dur("retry_in", backoff).Msg("RabbitMQ'ya bağlanılamadı.")
		}

		select {
		case <-time.After(backoff):
			// Exponential backoff
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
}

// StartConsumer, kuyruğu dinler ve mesajları worker pool mantığıyla işler.
func StartConsumer(ctx context.Context, ch *amqp091.Channel, handlerFunc func([]byte), log zerolog.Logger, wg *sync.WaitGroup) {
	// 1. Exchange Tanımla
	err := ch.ExchangeDeclare(
		exchangeName,
		"topic",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	)
	if err != nil {
		log.Error().Err(err).Str("exchange", exchangeName).Msg("Exchange deklare edilemedi")
		return
	}

	// 2. Kuyruk Tanımla
	q, err := ch.QueueDeclare(
		cdrQueueName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,
	)
	if err != nil {
		log.Error().Err(err).Msg("Kalıcı CDR kuyruğu oluşturulamadı")
		return
	}

	// 3. Bind İşlemi
	err = ch.QueueBind(
		q.Name,
		"#", // Tüm eventleri dinle
		exchangeName,
		false,
		nil,
	)
	if err != nil {
		log.Error().Err(err).Str("queue", q.Name).Str("exchange", exchangeName).Msg("Kuyruk exchange'e bağlanamadı")
		return
	}
	log.Info().Str("queue", q.Name).Str("exchange", exchangeName).Msg("Kalıcı kuyruk başarıyla exchange'e bağlandı.")

	// 4. QoS Ayarı (Worker sayısı kadar prefetch)
	// Global false: Her consumer instance için ayrı limit
	err = ch.Qos(maxConcurrentWorkers, 0, false)
	if err != nil {
		log.Error().Err(err).Msg("QoS ayarı yapılamadı.")
		return
	}

	// 5. Tüketimi Başlat
	msgs, err := ch.Consume(
		q.Name,
		"",    // consumer tag (auto)
		false, // auto-ack (MANUEL ACK KULLANIYORUZ)
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		log.Error().Err(err).Msg("Mesajlar tüketilemedi")
		return
	}

	log.Info().
		Str("queue", q.Name).
		Int("workers", maxConcurrentWorkers).
		Msg("Tüketici başlatıldı, mesajlar bekleniyor...")

	// Semaphore pattern: maxConcurrentWorkers kadar eşzamanlı işlem
	sem := make(chan struct{}, maxConcurrentWorkers)

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Context iptal edildi, yeni mesaj alımı durduruluyor.")
			return
			
		case d, ok := <-msgs:
			if !ok {
				log.Info().Msg("RabbitMQ mesaj kanalı kapandı.")
				return
			}

			// Worker slotu al (Bloklar eğer doluysa)
			sem <- struct{}{}
			wg.Add(1)

			go func(msg amqp091.Delivery) {
				defer wg.Done()
				defer func() { <-sem }() // İş bitince slotu bırak

				// Panic Recovery: Eğer handler panic olursa kuyruğu kilitlemesin
				defer func() {
					if r := recover(); r != nil {
						log.Error().Interface("panic", r).Msg("CRITICAL: Message handler panikledi! Mesaj Nack ediliyor.")
						// Tekrar kuyruğa atma (requeue=false), Dead Letter Exchange'e gitmeli
						_ = msg.Nack(false, false) 
					}
				}()

				handlerFunc(msg.Body)
				
				// Başarılı işleme sonrası Ack
				if err := msg.Ack(false); err != nil {
					log.Error().Err(err).Msg("Mesaj Ack edilemedi")
				}
			}(d)
		}
	}
}