// File: internal/handler/event_handler.go (TAM VE NİHAİ SON HALİ)
package handler

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	userv1 "github.com/sentiric/sentiric-contracts/gen/go/sentiric/user/v1"
)

// === DEĞİŞİKLİK: EventPayload'a Timestamp eklendi ===
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
	default:
		l.Debug().Msg("Bu olay tipi için özet CDR işlemi tanımlanmamış, atlanıyor.")
	}
}

func (h *EventHandler) logRawEvent(l zerolog.Logger, event *EventPayload) error {
	// === DEĞİŞİKLİK: Timestamp'in boş olup olmadığını kontrol et ===
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
	l.Info().Msg("Ham CDR olayı başarıyla veritabanına kaydedildi.")
	return nil
}

func (h *EventHandler) handleCallStarted(l zerolog.Logger, event *EventPayload) {
	// ARTIK SADECE BAŞLANGIÇ KAYDINI ATIYORUZ, USER-SERVICE ÇAĞRISI YOK.
	l.Info().Msg("Özet çağrı kaydı (CDR) başlangıç verisi oluşturuluyor.")
	query := `
		INSERT INTO calls (call_id, start_time, status)
		VALUES ($1, $2, 'STARTED')
		ON CONFLICT (call_id) DO NOTHING
	`
	_, err := h.db.Exec(query, event.CallID, event.Timestamp)
	if err != nil {
		l.Error().Err(err).Msg("Özet çağrı kaydı (CDR) başlangıç verisi veritabanına yazılamadı.")
		h.eventsFailed.WithLabelValues(event.EventType, "db_summary_insert_failed").Inc()
		return
	}
	l.Info().Msg("Özet çağrı kaydı (CDR) başlangıç verisi başarıyla oluşturuldu.")
}

func (h *EventHandler) handleCallEnded(l zerolog.Logger, event *EventPayload) {
	var startTime sql.NullTime
	err := h.db.QueryRow("SELECT start_time FROM calls WHERE call_id = $1", event.CallID).Scan(&startTime)
	if err != nil {
		l.Warn().Err(err).Msg("Çağrı sonlandırma olayı için başlangıç kaydı bulunamadı. Güncelleme atlanıyor.")
		return
	}

	duration := int(event.Timestamp.Sub(startTime.Time).Seconds())
	if duration < 0 {
		duration = 0
	} // Negatif süre olmasını engelle

	query := `
		UPDATE calls
		SET end_time = $1, duration_seconds = $2, status = 'COMPLETED', updated_at = NOW()
		WHERE call_id = $3
	`
	res, err := h.db.Exec(query, event.Timestamp, duration, event.CallID)
	if err != nil {
		l.Error().Err(err).Msg("Özet çağrı kaydı (CDR) güncellenemedi.")
		h.eventsFailed.WithLabelValues(event.EventType, "db_summary_update_failed").Inc()
		return
	}
	if rows, _ := res.RowsAffected(); rows > 0 {
		l.Info().Int("duration", duration).Msg("Özet çağrı kaydı (CDR) başarıyla sonlandırıldı ve güncellendi.")
	}
}

// === YENİ FONKSİYON (CDR-REFACTOR-01) ===
func (h *EventHandler) handleUserIdentified(l zerolog.Logger, body []byte) {
	var payload UserIdentifiedPayload // Bu struct daha önce tanımlanmıştı
	if err := json.Unmarshal(body, &payload); err != nil {
		l.Error().Err(err).Msg("user.identified.for_call olayı parse edilemedi.")
		h.eventsFailed.WithLabelValues("user.identified.for_call", "json_unmarshal").Inc()
		return
	}

	l = l.With().Str("user_id", payload.UserID).Int32("contact_id", payload.ContactID).Logger()
	l.Info().Msg("Kullanıcı kimliği bilgisi alındı, CDR güncelleniyor.")

	query := `
		UPDATE calls
		SET user_id = $1, contact_id = $2, tenant_id = $3, updated_at = NOW()
		WHERE call_id = $4
	`
	res, err := h.db.Exec(query, payload.UserID, payload.ContactID, payload.TenantID, payload.CallID)
	if err != nil {
		l.Error().Err(err).Msg("CDR kullanıcı bilgileriyle güncellenemedi.")
		h.eventsFailed.WithLabelValues(payload.EventType, "db_summary_update_failed").Inc()
		return
	}
	if rows, _ := res.RowsAffected(); rows > 0 {
		l.Info().Msg("Özet çağrı kaydı (CDR) kullanıcı bilgileriyle başarıyla güncellendi.")
	} else {
		l.Warn().Msg("Kullanıcı bilgisiyle güncellenecek CDR kaydı bulunamadı.")
	}
}
