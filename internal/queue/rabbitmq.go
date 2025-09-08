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
)

// DEĞİŞİKLİK: Fonksiyon artık context alıyor ve context-aware bekleme yapıyor.
func Connect(ctx context.Context, url string, log zerolog.Logger) (*amqp091.Channel, <-chan *amqp091.Error, error) {
	var conn *amqp091.Connection
	var err error
	for {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}

		conn, err = amqp091.Dial(url)
		if err == nil {
			log.Info().Msg("RabbitMQ bağlantısı başarılı.")
			ch, chErr := conn.Channel()
			if chErr != nil {
				return nil, nil, fmt.Errorf("RabbitMQ kanalı oluşturulamadı: %w", chErr)
			}
			closeChan := make(chan *amqp091.Error)
			conn.NotifyClose(closeChan)
			return ch, closeChan, nil
		}

		if ctx.Err() == nil {
			log.Warn().Err(err).Msg("RabbitMQ'ya bağlanılamadı, 5 saniye sonra tekrar denenecek...")
		}

		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
}

func StartConsumer(ctx context.Context, ch *amqp091.Channel, handlerFunc func([]byte), log zerolog.Logger, wg *sync.WaitGroup) {
	err := ch.ExchangeDeclare(
		exchangeName,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Error().Err(err).Str("exchange", exchangeName).Msg("Exchange deklare edilemedi")
		return
	}

	q, err := ch.QueueDeclare(
		cdrQueueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Error().Err(err).Msg("Kalıcı CDR kuyruğu oluşturulamadı")
		return
	}

	err = ch.QueueBind(
		q.Name,
		"#",
		exchangeName,
		false,
		nil,
	)
	if err != nil {
		log.Error().Err(err).Str("queue", q.Name).Str("exchange", exchangeName).Msg("Kuyruk exchange'e bağlanamadı")
		return
	}

	log.Info().Str("queue", q.Name).Str("exchange", exchangeName).Msg("Kalıcı kuyruk başarıyla exchange'e bağlandı.")

	err = ch.Qos(1, 0, false)
	if err != nil {
		log.Error().Err(err).Msg("QoS ayarı yapılamadı.")
		return
	}

	msgs, err := ch.Consume(
		q.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Error().Err(err).Msg("Mesajlar tüketilemedi")
		return
	}

	log.Info().Str("queue", q.Name).Msg("Kuyruk dinleniyor, mesajlar bekleniyor...")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Tüketici döngüsü durduruluyor, yeni mesajlar alınmayacak.")
			return
		case d, ok := <-msgs:
			if !ok {
				log.Info().Msg("RabbitMQ mesaj kanalı kapandı.")
				return
			}
			wg.Add(1)
			go func(msg amqp091.Delivery) {
				defer wg.Done()
				handlerFunc(msg.Body)
				_ = msg.Ack(false)
			}(d)
		}
	}
}
