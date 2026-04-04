// File: internal/queue/rabbitmq.go
// [ARCH-COMPLIANCE] Channel seviyesinde reconnect (Yeniden Bağlanma) kısıtlamasına uyum sağlandı.
// [ARCH-COMPLIANCE] SUTS v4.0 event logları eklendi.
package queue

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
	"github.com/sentiric/sentiric-cdr-service/internal/logger"
)

const (
	exchangeName    = "sentiric_events"
	dlxExchangeName = "sentiric_events.failed"
	cdrQueueName    = "sentiric.cdr_service.events"
	cdrErrorQueue   = "sentiric.cdr_service.failed"
	maxConcurrent   = 10
	maxRetries      = 3
)

type HandlerResult int

const (
	Ack HandlerResult = iota
	NackRetry
	NackDiscard
)

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
			log.Warn().Err(err).Str("event", logger.EventRabbitMQFail).Msg("RabbitMQ bağlantısı bekleniyor...")
			time.Sleep(5 * time.Second)
		}
	}
}

func StartConsumer(ctx context.Context, conn *amqp091.Connection, handlerFunc func([]byte) HandlerResult, log zerolog.Logger, wg *sync.WaitGroup) {
	// Reconnect döngüsü
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			ch, err := conn.Channel()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Error().Err(err).Str("event", logger.EventRabbitMQFail).Msg("Channel oluşturulamadı, 5 saniye sonra tekrar denenecek.")
				time.Sleep(5 * time.Second)
				continue
			}

			_ = ch.ExchangeDeclare(dlxExchangeName, "topic", true, false, false, false, nil)
			_, _ = ch.QueueDeclare(cdrErrorQueue, true, false, false, false, nil)
			_ = ch.QueueBind(cdrErrorQueue, "#", dlxExchangeName, false, nil)

			args := amqp091.Table{"x-dead-letter-exchange": dlxExchangeName}
			q, err := ch.QueueDeclare(cdrQueueName, true, false, false, false, args)
			if err != nil {
				log.Error().Err(err).Str("event", logger.EventRabbitMQFail).Msg("Kuyruk oluşturulamadı, yeniden deneniyor.")
				ch.Close()
				time.Sleep(5 * time.Second)
				continue
			}

			_ = ch.ExchangeDeclare(exchangeName, "topic", true, false, false, false, nil)
			_ = ch.QueueBind(q.Name, "#", exchangeName, false, nil)

			retryCh, err := conn.Channel()
			if err != nil {
				log.Error().Err(err).Str("event", logger.EventRabbitMQFail).Msg("Retry kanalı oluşturulamadı, yeniden deneniyor.")
				ch.Close()
				time.Sleep(5 * time.Second)
				continue
			}
			if err := retryCh.Confirm(false); err != nil {
				log.Error().Err(err).Str("event", logger.EventRabbitMQFail).Msg("Confirm modu aktifleştirilemedi.")
				ch.Close()
				retryCh.Close()
				time.Sleep(5 * time.Second)
				continue
			}

			_ = ch.Qos(maxConcurrent, 0, false)
			msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
			if err != nil {
				log.Error().Err(err).Str("event", logger.EventRabbitMQFail).Msg("Consume başlatılamadı.")
				ch.Close()
				retryCh.Close()
				time.Sleep(5 * time.Second)
				continue
			}

			sem := make(chan struct{}, maxConcurrent)
			log.Info().Str("event", logger.EventInfraReady).Msg("🚀 CDR Consumer aktif (SRE Resilient Mode)")

			channelClosed := make(chan struct{})

			go func() {
				for d := range msgs {
					sem <- struct{}{}
					wg.Add(1)
					go func(msg amqp091.Delivery) {
						defer wg.Done()
						defer func() { <-sem }()

						defer func() {
							if r := recover(); r != nil {
								log.Error().Interface("panic", r).Str("event", logger.EventRabbitMQFail).Msg("Zehirli mesaj (Panic)! DLX'e gönderiliyor.")
								_ = msg.Nack(false, false)
							}
						}()

						result := handlerFunc(msg.Body)
						switch result {
						case Ack:
							_ = msg.Ack(false)
						case NackDiscard:
							_ = msg.Nack(false, false)
						case NackRetry:
							handleRetry(ctx, retryCh, msg, log)
						}
					}(d)
				}
				close(channelClosed)
			}()

			select {
			case <-ctx.Done():
				ch.Close()
				retryCh.Close()
				return
			case <-channelClosed:
				log.Warn().Str("event", logger.EventRabbitMQChannelDrop).Msg("RabbitMQ channel kapandı, yeniden bağlanılıyor...")
				ch.Close()
				retryCh.Close()
				time.Sleep(2 * time.Second)
			}
		}
	}()
}

func handleRetry(ctx context.Context, retryCh *amqp091.Channel, msg amqp091.Delivery, log zerolog.Logger) {
	var count int32 = 0
	if ret, ok := msg.Headers["x-retry-count"].(int32); ok {
		count = ret
	}

	if count >= maxRetries {
		log.Warn().Int32("retry_count", count).Str("routing_key", msg.RoutingKey).Str("event", logger.EventRabbitMQFail).Msg("Maksimum retry limitine ulaşıldı. Mesaj DLX'e atılıyor.")
		_ = msg.Nack(false, false)
		return
	}

	baseDelay := math.Pow(2, float64(count)) * 500
	jitter := rand.Float64() * 500
	delay := time.Duration(baseDelay+jitter) * time.Millisecond

	log.Info().Int32("attempt", count+1).Dur("delay", delay).Str("event", logger.EventRabbitMQRetry).Msg("Geçici hata alındı. Backoff sonrası yeniden yayınlanacak.")
	time.Sleep(delay)

	headers := make(amqp091.Table)
	for k, v := range msg.Headers {
		if k == "x-death" || k == "x-first-death-exchange" || k == "x-first-death-queue" || k == "x-first-death-reason" {
			continue
		}
		headers[k] = v
	}
	headers["x-retry-count"] = count + 1

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
		log.Error().Err(err).Str("event", logger.EventRabbitMQFail).Msg("Retry mesajı RabbitMQ'ya yazılamadı, Nack fallback yapılıyor.")
		_ = msg.Nack(false, true)
		return
	}

	select {
	case confirmed := <-confirms:
		if confirmed.Ack {
			_ = msg.Ack(false)
		} else {
			log.Error().Str("event", logger.EventRabbitMQFail).Msg("Broker mesajı Nack etti (Disk full vb.), fallback yapılıyor.")
			_ = msg.Nack(false, true)
		}
	case <-time.After(5 * time.Second):
		log.Error().Str("event", logger.EventRabbitMQFail).Msg("Publish confirm zaman aşımına uğradı, fallback yapılıyor.")
		_ = msg.Nack(false, true)
	}
}
