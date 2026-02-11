// sentiric-cdr-service/internal/queue/rabbitmq.go
package queue

import (
	"context"
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
)

type HandlerResult int

const (
	Ack HandlerResult = iota
	NackRequeue
	NackDiscard
)

func Connect(ctx context.Context, url string, log zerolog.Logger) (*amqp091.Channel, <-chan *amqp091.Error, error) {
	var conn *amqp091.Connection
	var err error

	for {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
			conn, err = amqp091.Dial(url)
			if err == nil {
				ch, _ := conn.Channel()
				closeChan := make(chan *amqp091.Error)
				conn.NotifyClose(closeChan)
				return ch, closeChan, nil
			}
			log.Warn().Err(err).Msg("RabbitMQ bağlantısı bekleniyor...")
			time.Sleep(5 * time.Second)
		}
	}
}

func StartConsumer(ctx context.Context, ch *amqp091.Channel, handlerFunc func([]byte) HandlerResult, log zerolog.Logger, wg *sync.WaitGroup) {
	// 1. DLX (Dead Letter Exchange) Tanımla
	_ = ch.ExchangeDeclare(dlxExchangeName, "topic", true, false, false, false, nil)
	_, _ = ch.QueueDeclare(cdrErrorQueue, true, false, false, false, nil)
	_ = ch.QueueBind(cdrErrorQueue, "#", dlxExchangeName, false, nil)

	// 2. Ana Kuyruk (DLX Ayarlı)
	args := amqp091.Table{
		"x-dead-letter-exchange": dlxExchangeName,
	}
	q, err := ch.QueueDeclare(cdrQueueName, true, false, false, false, args)
	if err != nil {
		log.Fatal().Err(err).Msg("Kuyruk oluşturulamadı")
	}

	_ = ch.ExchangeDeclare(exchangeName, "topic", true, false, false, false, nil)
	_ = ch.QueueBind(q.Name, "#", exchangeName, false, nil)

	_ = ch.Qos(maxConcurrent, 0, false)
	msgs, _ := ch.Consume(q.Name, "", false, false, false, false, nil)

	sem := make(chan struct{}, maxConcurrent)
	log.Info().Msg("🚀 CDR Consumer aktif (Resilient Mode)")

	for {
		select {
		case <-ctx.Done():
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
						log.Error().Interface("panic", r).Msg("Zehirli mesaj! DLX'e gönderiliyor.")
						_ = msg.Nack(false, false) // Requeue=false -> DLX'e gider
					}
				}()

				result := handlerFunc(msg.Body)
				switch result {
				case Ack:
					_ = msg.Ack(false)
				case NackRequeue:
					_ = msg.Nack(false, true)
				case NackDiscard:
					_ = msg.Nack(false, false) // DLX'e düşer
				}
			}(d)
		}
	}
}
