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

	"github.com/sentiric/sentiric-cdr-service/internal/logger"
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

// HandleEvent artık SUTS v4.0 standartlarına göre loglama yapıyor
func (h *EventHandler) HandleEvent(body []byte) queue.HandlerResult {
	// 1. CallStartedEvent
	var callStarted eventv1.CallStartedEvent
	if err := proto.Unmarshal(body, &callStarted); err == nil && callStarted.EventType == "call.started" {
		// [SUTS v4.0]: Contextual Logger
		l := h.log.With().
			Str("trace_id", callStarted.TraceId). // Event'ten gelen TraceID
			Str("sip.call_id", callStarted.CallId).
			Logger()

		h.eventsProcessed.WithLabelValues(callStarted.EventType).Inc()
		l.Info().
			Str("event", logger.EventMessageReceived).
			Str("message_type", callStarted.EventType).
			Msg("Call Started olayı alındı")

		if err := h.logRawEvent(l, callStarted.CallId, callStarted.EventType, callStarted.Timestamp.AsTime(), body); err != nil {
			return queue.NackRequeue
		}

		if err := h.handleCallStarted(l, &callStarted); err != nil {
			return queue.NackRequeue
		}
		return queue.Ack
	}

	// 2. CallEndedEvent
	var callEnded eventv1.CallEndedEvent
	if err := proto.Unmarshal(body, &callEnded); err == nil && callEnded.EventType == "call.ended" {
		l := h.log.With().
			Str("trace_id", callEnded.TraceId).
			Str("sip.call_id", callEnded.CallId).
			Logger()

		h.eventsProcessed.WithLabelValues(callEnded.EventType).Inc()
		l.Info().
			Str("event", logger.EventMessageReceived).
			Str("message_type", callEnded.EventType).
			Msg("Call Ended olayı alındı")

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
		l := h.log.With().
			Str("trace_id", userIdentified.TraceId).
			Str("sip.call_id", userIdentified.CallId).
			Logger()

		h.eventsProcessed.WithLabelValues(userIdentified.EventType).Inc()
		l.Info().
			Str("event", logger.EventMessageReceived).
			Str("message_type", userIdentified.EventType).
			Msg("User Identified olayı alındı")

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
		l := h.log.With().
			Str("trace_id", genericEvent.TraceId).
			Str("tenant_id", genericEvent.TenantId).
			Logger()

		h.eventsProcessed.WithLabelValues(genericEvent.EventType).Inc()

		query := `INSERT INTO call_events (call_id, event_type, event_timestamp, payload) VALUES ($1, $2, $3, $4::jsonb)`
		_, dbErr := h.db.Exec(query, genericEvent.TraceId, genericEvent.EventType, genericEvent.Timestamp.AsTime(), genericEvent.PayloadJson)

		if dbErr != nil {
			l.Error().
				Str("event", logger.EventDbWriteFail).
				Err(dbErr).
				Msg("GenericEvent veritabanına yazılamadı.")
			h.eventsFailed.WithLabelValues(genericEvent.EventType, "db_insert_failed").Inc()
			return queue.NackRequeue
		}

		l.Debug().Str("event", logger.EventCdrProcessed).Msg("Generic Event kaydedildi")
		return queue.Ack
	}

	// 5. Bilinmeyen Format
	h.log.Warn().
		Str("event", logger.EventCdrIgnored).
		Int("body_size", len(body)).
		Msg("Bilinmeyen veya desteklenmeyen mesaj formatı")

	h.eventsFailed.WithLabelValues("unknown", "proto_unmarshal_unknown_type").Inc()
	return queue.NackDiscard
}

func (h *EventHandler) logRawEvent(l zerolog.Logger, callID, eventType string, ts time.Time, rawPayload []byte) error {
	payloadMap := map[string]string{
		"raw_proto_base64": base64.StdEncoding.EncodeToString(rawPayload),
	}
	jsonPayload, err := json.Marshal(payloadMap)
	if err != nil {
		l.Error().Err(err).Msg("Raw payload JSON'a çevrilemedi.")
		return nil
	}

	query := `INSERT INTO call_events (call_id, event_type, event_timestamp, payload) VALUES ($1, $2, $3, $4)`
	_, err = h.db.Exec(query, callID, eventType, ts, string(jsonPayload))
	if err != nil {
		l.Error().Str("event", logger.EventRawLogFail).Err(err).Msg("Ham CDR olayı DB'ye yazılamadı.")
		return err
	}
	l.Debug().Str("event", logger.EventRawLogSuccess).Msg("Ham CDR olayı kaydedildi.")
	return nil
}

func (h *EventHandler) handleCallStarted(l zerolog.Logger, event *eventv1.CallStartedEvent) error {
	query := `
		INSERT INTO calls (call_id, start_time, status)
		VALUES ($1, $2, 'STARTED')
		ON CONFLICT (call_id) DO UPDATE SET
			start_time = COALESCE(calls.start_time, EXCLUDED.start_time),
			updated_at = NOW()
	`
	_, err := h.db.Exec(query, event.CallId, event.Timestamp.AsTime())
	if err != nil {
		l.Error().Str("event", logger.EventDbWriteFail).Err(err).Msg("Call Started DB'ye yazılamadı.")
		h.eventsFailed.WithLabelValues(event.EventType, "db_summary_upsert_failed").Inc()
		return err
	}
	l.Info().Str("event", logger.EventCdrProcessed).Msg("Call Started işlendi (UPSERT).")
	return nil
}

func (h *EventHandler) handleCallEnded(l zerolog.Logger, event *eventv1.CallEndedEvent) error {
	var startTime, answerTime sql.NullTime
	err := h.db.QueryRow("SELECT start_time, answer_time FROM calls WHERE call_id = $1", event.CallId).Scan(&startTime, &answerTime)
	if err != nil && err != sql.ErrNoRows {
		l.Error().Err(err).Msg("Çağrı kaydı okunamadı.")
		return err
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

	query := `UPDATE calls SET end_time = $1, duration_seconds = $2, status = 'COMPLETED', disposition = $3, updated_at = NOW() WHERE call_id = $4`
	_, err = h.db.Exec(query, endTime, duration, disposition, event.CallId)
	if err != nil {
		l.Error().Str("event", logger.EventDbWriteFail).Err(err).Msg("Call Ended DB'ye yazılamadı.")
		h.eventsFailed.WithLabelValues(event.EventType, "db_summary_update_failed").Inc()
		return err
	}

	l.Info().
		Str("event", logger.EventCdrProcessed).
		Int("duration_sec", duration).
		Str("disposition", disposition).
		Msg("Çağrı tamamlandı ve CDR güncellendi.")
	return nil
}

func (h *EventHandler) handleUserIdentified(l zerolog.Logger, event *eventv1.UserIdentifiedForCallEvent) error {
	if event.User == nil || event.Contact == nil {
		l.Warn().Str("event", logger.EventCdrIgnored).Msg("User/Contact bilgisi eksik.")
		return nil
	}

	l = l.With().
		Str("user_id", event.User.Id).
		Str("tenant_id", event.User.TenantId).
		Logger()

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
		l.Error().Str("event", logger.EventDbWriteFail).Err(err).Msg("User Identified DB'ye yazılamadı.")
		h.eventsFailed.WithLabelValues(event.EventType, "db_summary_upsert_failed").Inc()
		return err
	}
	l.Info().Str("event", logger.EventCdrProcessed).Msg("CDR kullanıcı bilgileriyle zenginleştirildi.")
	return nil
}
