// sentiric-cdr-service/internal/queue/rabbitmq.go
package queue

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
)

const (
	exchangeName    = "sentiric_events"
	dlxExchangeName = "sentiric_events.failed"
	cdrQueueName    = "sentiric.cdr_service.events"
	cdrErrorQueue   = "sentiric.cdr_service.failed"
	maxConcurrent   = 10
	maxRetries      = 3 // SRE: Retry Storm Protection Limit
)

type HandlerResult int

const (
	Ack         HandlerResult = iota
	NackRetry                 // Geçici Hatalar (DB Connection, Timeout)
	NackDiscard               // Kalıcı Hatalar (Parse Fail, Validation Error)
)

// DEĞİŞİKLİK: Artık Channel yerine Connection dönüyoruz ki, publish için ayrı kanal açabilelim.
func Connect(ctx context.Context, url string, log zerolog.Logger) (*amqp091.Connection, <-chan *amqp091.Error, error) {
	var conn *amqp091.Connection
	var err error

	for {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
			conn, err = amqp091.Dial(url)
			if err == nil {
				closeChan := make(chan *amqp091.Error)
				conn.NotifyClose(closeChan)
				return conn, closeChan, nil
			}
			log.Warn().Err(err).Msg("RabbitMQ bağlantısı bekleniyor...")
			time.Sleep(5 * time.Second)
		}
	}
}

func StartConsumer(ctx context.Context, conn *amqp091.Connection, handlerFunc func([]byte) HandlerResult, log zerolog.Logger, wg *sync.WaitGroup) {
	ch, err := conn.Channel()
	if err != nil {
		log.Fatal().Err(err).Msg("RabbitMQ tüketici kanalı oluşturulamadı")
	}

	// 1. DLX (Dead Letter Exchange) Tanımla
	_ = ch.ExchangeDeclare(dlxExchangeName, "topic", true, false, false, false, nil)
	_, _ = ch.QueueDeclare(cdrErrorQueue, true, false, false, false, nil)
	_ = ch.QueueBind(cdrErrorQueue, "#", dlxExchangeName, false, nil)

	// 2. Ana Kuyruk (DLX Ayarlı)
	args := amqp091.Table{"x-dead-letter-exchange": dlxExchangeName}
	q, err := ch.QueueDeclare(cdrQueueName, true, false, false, false, args)
	if err != nil {
		log.Fatal().Err(err).Msg("Kuyruk oluşturulamadı")
	}

	_ = ch.ExchangeDeclare(exchangeName, "topic", true, false, false, false, nil)
	_ = ch.QueueBind(q.Name, "#", exchangeName, false, nil)

	// 3. Retry İçin Ayrı Bir Publish Kanalı Hazırla (Publish Confirm Mode)
	retryCh, err := conn.Channel()
	if err != nil {
		log.Fatal().Err(err).Msg("Retry yayın kanalı oluşturulamadı")
	}
	if err := retryCh.Confirm(false); err != nil {
		log.Fatal().Err(err).Msg("Publish confirm modu aktifleştirilemedi")
	}

	_ = ch.Qos(maxConcurrent, 0, false)
	msgs, _ := ch.Consume(q.Name, "", false, false, false, false, nil)

	sem := make(chan struct{}, maxConcurrent)
	log.Info().Msg("🚀 CDR Consumer aktif (SRE Resilient Mode)")

	for {
		select {
		case <-ctx.Done():
			ch.Close()
			retryCh.Close()
			return
		case d, ok := <-msgs:
			if !ok {
				return
			}
			sem <- struct{}{}
			wg.Add(1)
			go func(msg amqp091.Delivery) {
				defer wg.Done()
				defer func() { <-sem }()

				// Panic Recovery
				defer func() {
					if r := recover(); r != nil {
						log.Error().Interface("panic", r).Msg("Zehirli mesaj (Panic)! DLX'e gönderiliyor.")
						_ = msg.Nack(false, false)
					}
				}()

				result := handlerFunc(msg.Body)
				switch result {
				case Ack:
					_ = msg.Ack(false)
				case NackDiscard:
					_ = msg.Nack(false, false) // Doğrudan DLX'e düşer
				case NackRetry:
					handleRetry(ctx, retryCh, msg, log)
				}
			}(d)
		}
	}
}

// handleRetry: Stateless Exponential Backoff, Jitter ve Publish Confirm uygular.
func handleRetry(ctx context.Context, retryCh *amqp091.Channel, msg amqp091.Delivery, log zerolog.Logger) {
	var count int32 = 0
	if ret, ok := msg.Headers["x-retry-count"].(int32); ok {
		count = ret
	}

	// Limit aşımı -> DLX
	if count >= maxRetries {
		log.Warn().Int32("retry_count", count).Str("routing_key", msg.RoutingKey).Msg("Maksimum retry limitine ulaşıldı. Mesaj DLX'e atılıyor.")
		_ = msg.Nack(false, false)
		return
	}

	// 1. BACKOFF JITTER: Üstel bekleme + rastgele sapma (Thundering Herd koruması)
	baseDelay := math.Pow(2, float64(count)) * 500 // 500ms, 1000ms, 2000ms
	jitter := rand.Float64() * 500                 // 0-500ms arası rastgele sapma
	delay := time.Duration(baseDelay+jitter) * time.Millisecond

	log.Info().Int32("attempt", count+1).Dur("delay", delay).Msg("Geçici hata alındı. Backoff sonrası yeniden yayınlanacak.")
	time.Sleep(delay)

	// 2. HEADER SANITIZATION: x-death kirliliğini temizle ve sayacı artır
	headers := make(amqp091.Table)
	for k, v := range msg.Headers {
		if k == "x-death" || k == "x-first-death-exchange" || k == "x-first-death-queue" || k == "x-first-death-reason" {
			continue // Zehirli/gereksiz boyut kaplayan header'ları temizle
		}
		headers[k] = v
	}
	headers["x-retry-count"] = count + 1

	// 3. PUBLISH CONFIRM MODE
	confirms := retryCh.NotifyPublish(make(chan amqp091.Confirmation, 1))

	err := retryCh.PublishWithContext(
		ctx,
		exchangeName,
		msg.RoutingKey,
		false,
		false,
		amqp091.Publishing{
			Headers:      headers,
			ContentType:  msg.ContentType,
			Body:         msg.Body,
			DeliveryMode: amqp091.Persistent,
		},
	)

	if err != nil {
		log.Error().Err(err).Msg("Retry mesajı RabbitMQ'ya yazılamadı, Nack fallback yapılıyor.")
		_ = msg.Nack(false, true)
		return
	}

	// Broker'dan diske yazıldığına dair onay bekle
	select {
	case confirmed := <-confirms:
		if confirmed.Ack {
			_ = msg.Ack(false) // Güvenli! Yeni mesaj yazıldı, eskisini silebiliriz.
		} else {
			log.Error().Msg("Broker mesajı Nack etti (Disk full vb.), fallback yapılıyor.")
			_ = msg.Nack(false, true)
		}
	case <-time.After(5 * time.Second):
		log.Error().Msg("Publish confirm zaman aşımına uğradı, fallback yapılıyor.")
		_ = msg.Nack(false, true)
	}
}
