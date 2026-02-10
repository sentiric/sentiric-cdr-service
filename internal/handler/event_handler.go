// sentiric-cdr-service/internal/handler/event_handler.go
package handler

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"

	"github.com/sentiric/sentiric-cdr-service/internal/queue"
	eventv1 "github.com/sentiric/sentiric-contracts/gen/go/sentiric/event/v1"
)

type EventHandler struct {
	db              *sql.DB
	log             zerolog.Logger
	eventsProcessed *prometheus.CounterVec
	eventsFailed    *prometheus.CounterVec
}

func NewEventHandler(db *sql.DB, log zerolog.Logger, processed, failed *prometheus.CounterVec) *EventHandler {
	return &EventHandler{
		db:              db,
		log:             log,
		eventsProcessed: processed,
		eventsFailed:    failed,
	}
}

// HandleEvent artık queue.HandlerResult dönüyor.
func (h *EventHandler) HandleEvent(body []byte) queue.HandlerResult {
	// 1. CallStartedEvent
	var callStarted eventv1.CallStartedEvent
	if err := proto.Unmarshal(body, &callStarted); err == nil && callStarted.EventType == "call.started" {
		l := h.log.With().Str("call_id", callStarted.CallId).Str("event_type", callStarted.EventType).Logger()
		h.eventsProcessed.WithLabelValues(callStarted.EventType).Inc()
		l.Info().Msg("Protobuf 'call.started' olayı işleniyor.")

		if err := h.logRawEvent(l, callStarted.CallId, callStarted.EventType, callStarted.Timestamp.AsTime(), body); err != nil {
			return queue.NackRequeue // DB hatası -> Requeue
		}

		if err := h.handleCallStarted(l, &callStarted); err != nil {
			return queue.NackRequeue // DB hatası -> Requeue
		}
		return queue.Ack
	}

	// 2. CallEndedEvent
	var callEnded eventv1.CallEndedEvent
	if err := proto.Unmarshal(body, &callEnded); err == nil && callEnded.EventType == "call.ended" {
		l := h.log.With().Str("call_id", callEnded.CallId).Str("event_type", callEnded.EventType).Logger()
		h.eventsProcessed.WithLabelValues(callEnded.EventType).Inc()
		l.Info().Msg("Protobuf 'call.ended' olayı işleniyor.")

		if err := h.logRawEvent(l, callEnded.CallId, callEnded.EventType, callEnded.Timestamp.AsTime(), body); err != nil {
			return queue.NackRequeue
		}

		if err := h.handleCallEnded(l, &callEnded); err != nil {
			return queue.NackRequeue
		}
		return queue.Ack
	}

	// 3. UserIdentifiedForCallEvent
	var userIdentified eventv1.UserIdentifiedForCallEvent
	if err := proto.Unmarshal(body, &userIdentified); err == nil && userIdentified.EventType == "user.identified.for.call" {
		l := h.log.With().Str("call_id", userIdentified.CallId).Str("event_type", userIdentified.EventType).Logger()
		h.eventsProcessed.WithLabelValues(userIdentified.EventType).Inc()
		l.Info().Msg("Protobuf 'user.identified.for.call' olayı işleniyor.")

		if err := h.logRawEvent(l, userIdentified.CallId, userIdentified.EventType, userIdentified.Timestamp.AsTime(), body); err != nil {
			return queue.NackRequeue
		}

		if err := h.handleUserIdentified(l, &userIdentified); err != nil {
			return queue.NackRequeue
		}
		return queue.Ack
	}

	// 4. GenericEvent
	var genericEvent eventv1.GenericEvent
	if err := proto.Unmarshal(body, &genericEvent); err == nil && genericEvent.EventType != "" {
		l := h.log.With().Str("trace_id", genericEvent.TraceId).Str("event_type", genericEvent.EventType).Logger()
		h.eventsProcessed.WithLabelValues(genericEvent.EventType).Inc()

		callID := genericEvent.TraceId
		l.Debug().Msgf("Protobuf GenericEvent (%s) alındı.", genericEvent.EventType)

		query := `INSERT INTO call_events (call_id, event_type, event_timestamp, payload) VALUES ($1, $2, $3, $4::jsonb)`
		_, dbErr := h.db.Exec(query, callID, genericEvent.EventType, genericEvent.Timestamp.AsTime(), genericEvent.PayloadJson)

		if dbErr != nil {
			l.Error().Err(dbErr).Msg("GenericEvent veritabanına yazılamadı.")
			h.eventsFailed.WithLabelValues(genericEvent.EventType, "db_insert_failed").Inc()
			return queue.NackRequeue // DB hatası -> Requeue
		}
		return queue.Ack
	}

	// 5. Bilinmeyen Format (Protobuf parse hatası)
	// Bu durumda Requeue yapmak mantıksızdır, çünkü veri bozuktur ve düzelmeyecektir.
	h.log.Warn().Bytes("raw_message", body).Msg("Hata: Mesaj bilinen bir Protobuf formatında değil veya 'eventType' alanı eksik/yanlış.")
	h.eventsFailed.WithLabelValues("unknown", "proto_unmarshal_unknown_type").Inc()
	return queue.NackDiscard // Kalıcı hata -> Discard
}

func (h *EventHandler) logRawEvent(l zerolog.Logger, callID, eventType string, ts time.Time, rawPayload []byte) error {
	payloadMap := map[string]string{
		"raw_proto_base64": base64.StdEncoding.EncodeToString(rawPayload),
	}
	jsonPayload, err := json.Marshal(payloadMap)
	if err != nil {
		l.Error().Err(err).Msg("Raw payload JSON'a çevrilemedi.")
		// JSON hatası kritik bir veri kaybı riski değildir ama loglanmalı.
		// Ancak DB'ye yazamazsak sorun.
		// Burada JSON hatasını yutuyoruz çünkü raw data bozuksa DB'ye yazmanın anlamı yok.
		return nil
	}

	query := `INSERT INTO call_events (call_id, event_type, event_timestamp, payload) VALUES ($1, $2, $3, $4)`
	_, err = h.db.Exec(query, callID, eventType, ts, string(jsonPayload))
	if err != nil {
		l.Error().Err(err).Msg("Ham CDR olayı veritabanına yazılamadı.")
		return err // DB Hatası döner
	}
	l.Debug().Msg("Ham CDR olayı başarıyla veritabanına kaydedildi.")
	return nil
}

func (h *EventHandler) handleCallStarted(l zerolog.Logger, event *eventv1.CallStartedEvent) error {
	l.Debug().Msg("Özet çağrı kaydı (CDR) başlangıç verisi oluşturuluyor/güncelleniyor (UPSERT).")
	query := `
		INSERT INTO calls (call_id, start_time, status)
		VALUES ($1, $2, 'STARTED')
		ON CONFLICT (call_id) DO UPDATE SET
			start_time = COALESCE(calls.start_time, EXCLUDED.start_time),
			updated_at = NOW()
	`
	_, err := h.db.Exec(query, event.CallId, event.Timestamp.AsTime())
	if err != nil {
		l.Error().Err(err).Msg("Özet çağrı kaydı (CDR) başlangıç verisi yazılamadı.")
		h.eventsFailed.WithLabelValues(event.EventType, "db_summary_upsert_failed").Inc()
		return err
	}
	l.Debug().Msg("Özet çağrı kaydı (CDR) başlangıç verisi başarıyla yazıldı/güncellendi.")
	return nil
}

func (h *EventHandler) handleCallEnded(l zerolog.Logger, event *eventv1.CallEndedEvent) error {
	var startTime, answerTime sql.NullTime
	// Bu sorgu hatası, kaydın olmaması durumunda (ErrNoRows) normaldir, bu yüzden hata dönmeyiz.
	// Ancak DB bağlantı hatası varsa dönmeliyiz.
	err := h.db.QueryRow("SELECT start_time, answer_time FROM calls WHERE call_id = $1", event.CallId).Scan(&startTime, &answerTime)
	if err != nil && err != sql.ErrNoRows {
		l.Error().Err(err).Msg("Çağrı kaydı okunamadı (DB hatası).")
		return err
	} else if err == sql.ErrNoRows {
		l.Warn().Msg("Çağrı sonlandırma olayı için başlangıç kaydı bulunamadı (Olası sıra hatası).")
		// Kayıt yoksa UPDATE çalışmaz ama bu bir DB hatası değildir.
		// Ancak, CallStarted mesajı henüz gelmemiş olabilir (Race Condition).
		// Bu durumda Requeue yapmak mantıklı olabilir mi?
		// Evet, çünkü CallStarted birazdan gelebilir.
		// Ancak sonsuz döngüye girmemesi için dikkatli olunmalı.
		// Şimdilik Requeue yapmıyoruz, çünkü sadece istatistik kaybı olur.
	}

	var duration int
	endTime := event.Timestamp.AsTime()
	if answerTime.Valid {
		duration = int(endTime.Sub(answerTime.Time).Seconds())
	} else if startTime.Valid {
		duration = int(endTime.Sub(startTime.Time).Seconds())
	}
	if duration < 0 {
		duration = 0
	}

	disposition := "NO_ANSWER"
	if answerTime.Valid {
		disposition = "ANSWERED"
	}

	query := ` UPDATE calls SET end_time = $1, duration_seconds = $2, status = 'COMPLETED', disposition = $3, updated_at = NOW() WHERE call_id = $4 `
	res, err := h.db.Exec(query, endTime, duration, disposition, event.CallId)
	if err != nil {
		l.Error().Err(err).Msg("Özet çağrı kaydı (CDR) güncellenemedi.")
		h.eventsFailed.WithLabelValues(event.EventType, "db_summary_update_failed").Inc()
		return err
	}
	if rows, _ := res.RowsAffected(); rows > 0 {
		l.Info().Int("duration", duration).Str("disposition", disposition).Msg("Özet çağrı kaydı (CDR) başarıyla sonlandırıldı.")
	}
	return nil
}

func (h *EventHandler) handleUserIdentified(l zerolog.Logger, event *eventv1.UserIdentifiedForCallEvent) error {
	if event.User == nil || event.Contact == nil {
		l.Warn().Msg("user.identified.for.call olayı eksik User veya Contact bilgisi içeriyor.")
		// Veri eksikse yapacak bir şey yok, Discard (Ack)
		return nil
	}
	l = l.With().Str("user_id", event.User.Id).Int32("contact_id", event.Contact.Id).Logger()
	l.Debug().Msg("Kullanıcı kimliği bilgisi alındı, CDR güncelleniyor (UPSERT).")
	query := `
		INSERT INTO calls (call_id, user_id, contact_id, tenant_id, status)
		VALUES ($1, $2, $3, $4, 'IDENTIFIED')
		ON CONFLICT (call_id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			contact_id = EXCLUDED.contact_id,
			tenant_id = EXCLUDED.tenant_id,
			status = COALESCE(calls.status, 'IDENTIFIED'),
			updated_at = NOW()
	`
	_, err := h.db.Exec(query, event.CallId, event.User.Id, event.Contact.Id, event.User.TenantId)
	if err != nil {
		l.Error().Err(err).Msg("CDR kullanıcı bilgileriyle güncellenemedi (UPSERT).")
		h.eventsFailed.WithLabelValues(event.EventType, "db_summary_upsert_failed").Inc()
		return err
	}
	l.Debug().Msg("Özet çağrı kaydı (CDR) kullanıcı bilgileriyle başarıyla yazıldı/güncellendi.")
	return nil
}
