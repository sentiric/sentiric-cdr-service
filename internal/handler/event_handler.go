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

func (h *EventHandler) HandleEvent(body []byte) {
	// [YENİ BASİTLEŞTİRİLMİŞ MANTIK]
	// Gelen mesajı sırayla bildiğimiz event tiplerine decode etmeye çalış.

	// 1. CallStartedEvent mi?
	var callStarted eventv1.CallStartedEvent
	if err := proto.Unmarshal(body, &callStarted); err == nil && callStarted.EventType == "call.started" {
		l := h.log.With().Str("call_id", callStarted.CallId).Str("event_type", callStarted.EventType).Logger()
		h.eventsProcessed.WithLabelValues(callStarted.EventType).Inc()
		l.Info().Msg("Protobuf 'call.started' olayı işleniyor.")
		if err := h.logRawEvent(l, callStarted.CallId, callStarted.EventType, callStarted.Timestamp.AsTime(), body); err == nil {
			h.handleCallStarted(l, &callStarted)
		}
		return
	}

	// 2. CallEndedEvent mi?
	var callEnded eventv1.CallEndedEvent
	if err := proto.Unmarshal(body, &callEnded); err == nil && callEnded.EventType == "call.ended" {
		l := h.log.With().Str("call_id", callEnded.CallId).Str("event_type", callEnded.EventType).Logger()
		h.eventsProcessed.WithLabelValues(callEnded.EventType).Inc()
		l.Info().Msg("Protobuf 'call.ended' olayı işleniyor.")
		if err := h.logRawEvent(l, callEnded.CallId, callEnded.EventType, callEnded.Timestamp.AsTime(), body); err == nil {
			h.handleCallEnded(l, &callEnded)
		}
		return
	}

	// 3. UserIdentified mi?
	var userIdentified eventv1.UserIdentifiedForCallEvent
	if err := proto.Unmarshal(body, &userIdentified); err == nil && userIdentified.EventType == "user.identified.for.call" {
		l := h.log.With().Str("call_id", userIdentified.CallId).Str("event_type", userIdentified.EventType).Logger()
		h.eventsProcessed.WithLabelValues(userIdentified.EventType).Inc()
		l.Info().Msg("Protobuf 'user.identified.for.call' olayı işleniyor.")
		if err := h.logRawEvent(l, userIdentified.CallId, userIdentified.EventType, userIdentified.Timestamp.AsTime(), body); err == nil {
			h.handleUserIdentified(l, &userIdentified)
		}
		return
	}

	// Hiçbiri değilse hata ver.
	h.log.Error().Bytes("raw_message", body).Msg("Hata: Mesaj bilinen bir Protobuf formatında değil veya 'eventType' alanı eksik/yanlış.")
	h.eventsFailed.WithLabelValues("unknown", "proto_unmarshal_unknown_type").Inc()
}

func (h *EventHandler) logRawEvent(l zerolog.Logger, callID, eventType string, ts time.Time, rawPayload []byte) error {
	jsonPayload, _ := json.Marshal(map[string]string{
		"raw_proto_base64": base64.StdEncoding.EncodeToString(rawPayload),
	})

	query := `INSERT INTO call_events (call_id, event_type, event_timestamp, payload) VALUES ($1, $2, $3, $4)`
	_, err := h.db.Exec(query, callID, eventType, ts, jsonPayload)
	if err != nil {
		l.Error().Err(err).Msg("Ham CDR olayı veritabanına yazılamadı.")
		return err
	}
	l.Debug().Msg("Ham CDR olayı başarıyla veritabanına kaydedildi.")
	return nil
}

func (h *EventHandler) handleCallStarted(l zerolog.Logger, event *eventv1.CallStartedEvent) {
	l.Debug().Msg("Özet çağrı kaydı (CDR) başlangıç verisi oluşturuluyor/güncelleniyor (UPSERT).")
	query := `
		INSERT INTO calls (call_id, start_time, status)
		VALUES ($1, $2, 'STARTED')
		ON CONFLICT (call_id) DO UPDATE SET
			start_time = COALESCE(calls.start_time, EXCLUDED.start_time),
			updated_at = NOW()
	`
	res, err := h.db.Exec(query, event.CallId, event.Timestamp.AsTime())
	if err != nil {
		l.Error().Err(err).Msg("Özet çağrı kaydı (CDR) başlangıç verisi yazılamadı.")
		h.eventsFailed.WithLabelValues(event.EventType, "db_summary_upsert_failed").Inc()
	} else if rows, _ := res.RowsAffected(); rows > 0 {
		l.Debug().Msg("Özet çağrı kaydı (CDR) başlangıç verisi başarıyla yazıldı/güncellendi.")
	}
}

func (h *EventHandler) handleCallEnded(l zerolog.Logger, event *eventv1.CallEndedEvent) {
	var startTime, answerTime sql.NullTime
	err := h.db.QueryRow("SELECT start_time, answer_time FROM calls WHERE call_id = $1", event.CallId).Scan(&startTime, &answerTime)
	if err != nil {
		l.Warn().Err(err).Msg("Çağrı sonlandırma olayı için başlangıç kaydı bulunamadı.")
		return
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
		return
	}
	if rows, _ := res.RowsAffected(); rows > 0 {
		l.Info().Int("duration", duration).Str("disposition", disposition).Msg("Özet çağrı kaydı (CDR) başarıyla sonlandırıldı.")
	}
}

func (h *EventHandler) handleUserIdentified(l zerolog.Logger, event *eventv1.UserIdentifiedForCallEvent) {
	if event.User == nil || event.Contact == nil {
		l.Warn().Msg("user.identified.for.call olayı eksik User veya Contact bilgisi içeriyor.")
		return
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
	res, err := h.db.Exec(query, event.CallId, event.User.Id, event.Contact.Id, event.User.TenantId)
	if err != nil {
		l.Error().Err(err).Msg("CDR kullanıcı bilgileriyle güncellenemedi (UPSERT).")
		h.eventsFailed.WithLabelValues(event.EventType, "db_summary_upsert_failed").Inc()
		return
	}
	if rows, _ := res.RowsAffected(); rows > 0 {
		l.Debug().Msg("Özet çağrı kaydı (CDR) kullanıcı bilgileriyle başarıyla yazıldı/güncellendi.")
	}
}