// sentiric-cdr-service/internal/handler/event_handler.go
package handler

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

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
	var genericEvent eventv1.GenericEvent
	if err := proto.Unmarshal(body, &genericEvent); err != nil {
		h.log.Error().Err(err).Bytes("raw_message", body).Msg("Hata: Mesaj Protobuf formatında değil")
		h.eventsFailed.WithLabelValues("unknown", "proto_unmarshal").Inc()
		return
	}

	eventType := genericEvent.EventType
	l := h.log.With().Str("event_type", eventType).Str("trace_id", genericEvent.TraceId).Logger()

	h.eventsProcessed.WithLabelValues(eventType).Inc()
	l.Debug().Msg("CDR olayı alındı (Protobuf), işleniyor...")

	// Ham olayı JSON olarak (loglama için) ve binary olarak (payload için) kaydet
	if err := h.logRawEvent(l, &genericEvent, body); err != nil {
		h.eventsFailed.WithLabelValues(eventType, "db_raw_insert_failed").Inc()
		return
	}

	switch eventType {
	case "call.started":
		var event eventv1.CallStartedEvent
		if err := proto.Unmarshal(body, &event); err == nil {
			h.handleCallStarted(l, &event)
		}
	case "call.ended":
		var event eventv1.CallEndedEvent
		if err := proto.Unmarshal(body, &event); err == nil {
			h.handleCallEnded(l, &event)
		}
	case "user.identified.for_call":
		var event eventv1.UserIdentifiedForCallEvent
		if err := proto.Unmarshal(body, &event); err == nil {
			h.handleUserIdentified(l, &event)
		}
	default:
		l.Debug().Msg("Bu olay tipi için özet CDR işlemi tanımlanmamış, atlanıyor.")
	}
}

func (h *EventHandler) logRawEvent(l zerolog.Logger, event *eventv1.GenericEvent, rawPayload []byte) error {
	var eventTimestamp time.Time
	ts := event.GetTimestamp()
	if ts == nil || !ts.IsValid() {
		eventTimestamp = time.Now().UTC()
	} else {
		eventTimestamp = ts.AsTime()
	}

	// Ham payload'ı veritabanına JSONB olarak kaydetmek için JSON'a çevirelim (okunabilirlik için)
	// Eğer performans kritikse, doğrudan binary de saklanabilir. Şimdilik JSON daha iyi.
	jsonPayload, _ := json.Marshal(map[string]string{"raw_proto_base64": proto.EncodeVarint(uint64(len(rawPayload)))}) // Placeholder

	query := `INSERT INTO call_events (call_id, event_type, event_timestamp, payload) VALUES ($1, $2, $3, $4)`
	_, err := h.db.Exec(query, event.TraceId, event.EventType, eventTimestamp, jsonPayload) // CallID yerine TraceID
	if err != nil {
		l.Error().Err(err).Msg("Ham CDR olayı veritabanına yazılamadı.")
		return err
	}
	l.Debug().Msg("Ham CDR olayı başarıyla veritabanına kaydedildi.")
	return nil
}

func (h *EventHandler) handleCallStarted(l zerolog.Logger, event *eventv1.CallStartedEvent) {
	l = l.With().Str("call_id", event.CallId).Logger()
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
	l = l.With().Str("call_id", event.CallId).Logger()
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
		l.Warn().Msg("user.identified.for_call olayı eksik User veya Contact bilgisi içeriyor.")
		return
	}
	l = l.With().Str("call_id", event.CallId).Str("user_id", event.User.Id).Int32("contact_id", event.Contact.Id).Logger()
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