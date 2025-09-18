package handler

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	userv1 "github.com/sentiric/sentiric-contracts/gen/go/sentiric/user/v1"
)

type EventPayload struct {
	EventType  string          `json:"eventType"`
	TraceID    string          `json:"traceId"`
	CallID     string          `json:"callId"`
	From       string          `json:"from"`
	To         string          `json:"to"`
	Timestamp  time.Time       `json:"timestamp"`
	RawPayload json.RawMessage `json:"-"`
}

type UserIdentifiedPayload struct {
	EventType string    `json:"eventType"`
	TraceID   string    `json:"traceId"`
	CallID    string    `json:"callId"`
	UserID    string    `json:"userId"`
	ContactID int32     `json:"contactId"`
	TenantID  string    `json:"tenantId"`
	Timestamp time.Time `json:"timestamp"`
}

type CallAnsweredPayload struct {
	Timestamp time.Time `json:"timestamp"`
}
type CallRecordingAvailablePayload struct {
	RecordingURI string `json:"recordingUri"`
}

type EventHandler struct {
	db              *sql.DB
	userClient      userv1.UserServiceClient
	log             zerolog.Logger
	eventsProcessed *prometheus.CounterVec
	eventsFailed    *prometheus.CounterVec
}

func NewEventHandler(db *sql.DB, uc userv1.UserServiceClient, log zerolog.Logger, processed, failed *prometheus.CounterVec) *EventHandler {
	return &EventHandler{
		db:              db,
		userClient:      uc,
		log:             log,
		eventsProcessed: processed,
		eventsFailed:    failed,
	}
}

func (h *EventHandler) HandleEvent(body []byte) {
	var event EventPayload
	if err := json.Unmarshal(body, &event); err != nil {
		h.log.Error().Err(err).Bytes("raw_message", body).Msg("Hata: Mesaj JSON formatında değil")
		h.eventsFailed.WithLabelValues("unknown", "json_unmarshal").Inc()
		return
	}
	event.RawPayload = json.RawMessage(body)

	l := h.log.With().Str("call_id", event.CallID).Str("trace_id", event.TraceID).Str("event_type", event.EventType).Logger()
	h.eventsProcessed.WithLabelValues(event.EventType).Inc()

	// Bu log, bir iş akışının başlangıcı olduğu için INFO olarak kalmalı.
	l.Info().Msg("CDR olayı alındı, işleniyor...")

	if err := h.logRawEvent(l, &event); err != nil {
		h.eventsFailed.WithLabelValues(event.EventType, "db_raw_insert_failed").Inc()
		return
	}

	switch event.EventType {
	case "call.started":
		h.handleCallStarted(l, &event)
	case "call.ended":
		h.handleCallEnded(l, &event)
	case "user.identified.for_call":
		h.handleUserIdentified(l, body)
	case "call.answered":
		h.handleCallAnswered(l, &event)
	case "call.recording.available":
		h.handleRecordingAvailable(l, &event)
	default:
		// Bu bir hata değil, sadece işlenmeyen bir olay. DEBUG seviyesi daha uygun.
		l.Debug().Msg("Bu olay tipi için özet CDR işlemi tanımlanmamış, atlanıyor.")
	}
}

func (h *EventHandler) logRawEvent(l zerolog.Logger, event *EventPayload) error {
	var eventTimestamp time.Time
	if event.Timestamp.IsZero() {
		l.Warn().Msg("Olayda zaman damgası bulunamadı, şimdiki zaman kullanılıyor.")
		eventTimestamp = time.Now().UTC()
	} else {
		eventTimestamp = event.Timestamp
	}
	query := `INSERT INTO call_events (call_id, event_type, event_timestamp, payload) VALUES ($1, $2, $3, $4)`
	_, err := h.db.Exec(query, event.CallID, event.EventType, eventTimestamp, event.RawPayload)
	if err != nil {
		l.Error().Err(err).Msg("Ham CDR olayı veritabanına yazılamadı.")
		return err
	}
	// Bu log gereksiz gürültü yaratıyor, DEBUG seviyesine alıyoruz.
	l.Debug().Msg("Ham CDR olayı başarıyla veritabanına kaydedildi.")
	return nil
}

func (h *EventHandler) handleCallStarted(l zerolog.Logger, event *EventPayload) {
	// Bu log bir kilometre taşı, INFO olarak kalması doğru.
	l.Info().Msg("Özet çağrı kaydı (CDR) başlangıç verisi oluşturuluyor/güncelleniyor (UPSERT).")

	query := `
		INSERT INTO calls (call_id, start_time, status)
		VALUES ($1, $2, 'STARTED')
		ON CONFLICT (call_id) DO UPDATE SET
			start_time = COALESCE(calls.start_time, EXCLUDED.start_time),
			updated_at = NOW()
	`
	res, err := h.db.Exec(query, event.CallID, event.Timestamp)
	if err != nil {
		l.Error().Err(err).Msg("Özet çağrı kaydı (CDR) başlangıç verisi yazılamadı.")
		h.eventsFailed.WithLabelValues(event.EventType, "db_summary_upsert_failed").Inc()
		return
	}

	if rows, _ := res.RowsAffected(); rows > 0 {
		// Bu log da gereksiz, DEBUG'a çekiyoruz.
		l.Debug().Msg("Özet çağrı kaydı (CDR) başlangıç verisi başarıyla yazıldı/güncellendi.")
	} else {
		l.Warn().Msg("Yinelenen 'call.started' olayı işlendi, mevcut kayıt güncellenmedi (zaten aynı).")
	}
}

func (h *EventHandler) handleCallEnded(l zerolog.Logger, event *EventPayload) {
	var startTime, answerTime sql.NullTime
	err := h.db.QueryRow("SELECT start_time, answer_time FROM calls WHERE call_id = $1", event.CallID).Scan(&startTime, &answerTime)
	if err != nil {
		l.Warn().Err(err).Msg("Çağrı sonlandırma olayı için başlangıç kaydı bulunamadı. Güncelleme atlanıyor.")
		return
	}

	var duration int
	if answerTime.Valid {
		duration = int(event.Timestamp.Sub(answerTime.Time).Seconds())
	} else if startTime.Valid {
		duration = int(event.Timestamp.Sub(startTime.Time).Seconds())
	}
	if duration < 0 {
		duration = 0
	}

	disposition := "NO_ANSWER"
	if answerTime.Valid {
		disposition = "ANSWERED"
	}

	query := `
		UPDATE calls
		SET end_time = $1, duration_seconds = $2, status = 'COMPLETED', disposition = $3, updated_at = NOW()
		WHERE call_id = $4
	`
	res, err := h.db.Exec(query, event.Timestamp, duration, disposition, event.CallID)
	if err != nil {
		l.Error().Err(err).Msg("Özet çağrı kaydı (CDR) güncellenemedi.")
		h.eventsFailed.WithLabelValues(event.EventType, "db_summary_update_failed").Inc()
		return
	}
	if rows, _ := res.RowsAffected(); rows > 0 {
		// Bu bir kilometre taşı, INFO olarak kalması doğru.
		l.Info().Int("duration", duration).Str("disposition", disposition).Msg("Özet çağrı kaydı (CDR) başarıyla sonlandırıldı.")
	}
}

func (h *EventHandler) handleCallAnswered(l zerolog.Logger, event *EventPayload) {
	var payload CallAnsweredPayload
	if err := json.Unmarshal(event.RawPayload, &payload); err != nil {
		l.Error().Err(err).Msg("call.answered olayı parse edilemedi.")
		return
	}

	query := `UPDATE calls SET answer_time = $1, disposition = 'ANSWERED', updated_at = NOW() WHERE call_id = $2 AND status != 'COMPLETED'`
	_, err := h.db.Exec(query, payload.Timestamp, event.CallID)
	if err != nil {
		l.Error().Err(err).Msg("CDR 'answer_time' ile güncellenemedi.")
		return
	}
	l.Debug().Msg("CDR 'answer_time' ile güncellendi.")
}

func (h *EventHandler) handleRecordingAvailable(l zerolog.Logger, event *EventPayload) {
	var payload CallRecordingAvailablePayload
	if err := json.Unmarshal(event.RawPayload, &payload); err != nil {
		l.Error().Err(err).Msg("call.recording.available olayı parse edilemedi.")
		return
	}

	query := `UPDATE calls SET recording_url = $1, updated_at = NOW() WHERE call_id = $2`
	_, err := h.db.Exec(query, payload.RecordingURI, event.CallID)
	if err != nil {
		l.Error().Err(err).Msg("CDR 'recording_url' ile güncellenemedi.")
		return
	}
	l.Debug().Msg("CDR 'recording_url' ile güncellendi.")
}

func (h *EventHandler) handleUserIdentified(l zerolog.Logger, body []byte) {
	var payload UserIdentifiedPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		l.Error().Err(err).Msg("user.identified.for_call olayı parse edilemedi.")
		h.eventsFailed.WithLabelValues("user.identified.for_call", "json_unmarshal").Inc()
		return
	}

	l = l.With().Str("user_id", payload.UserID).Int32("contact_id", payload.ContactID).Logger()
	l.Info().Msg("Kullanıcı kimliği bilgisi alındı, CDR güncelleniyor (UPSERT).")

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
	res, err := h.db.Exec(query, payload.CallID, payload.UserID, payload.ContactID, payload.TenantID)
	if err != nil {
		l.Error().Err(err).Msg("CDR kullanıcı bilgileriyle güncellenemedi (UPSERT).")
		h.eventsFailed.WithLabelValues(payload.EventType, "db_summary_upsert_failed").Inc()
		return
	}
	if rows, _ := res.RowsAffected(); rows > 0 {
		l.Debug().Msg("Özet çağrı kaydı (CDR) kullanıcı bilgileriyle başarıyla yazıldı/güncellendi.")
	}
}
