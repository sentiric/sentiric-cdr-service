package handler

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

type GenericEvent struct {
	EventType string          `json:"eventType"`
	CallID    string          `json:"callId"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type EventHandler struct {
	db              *sql.DB // DÜZELTME: Tekrar *sql.DB tipine döndük.
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
	var event GenericEvent
	event.Timestamp = time.Now().UTC()

	if err := json.Unmarshal(body, &event); err != nil {
		h.log.Error().Err(err).Bytes("raw_message", body).Msg("Hata: Mesaj JSON formatında değil")
		h.eventsFailed.WithLabelValues("unknown", "json_unmarshal").Inc()
		return
	}

	h.eventsProcessed.WithLabelValues(event.EventType).Inc()
	l := h.log.With().Str("call_id", event.CallID).Str("event_type", event.EventType).Logger()

	l.Info().Msg("CDR olayı alındı, veritabanına işleniyor...")

	query := `INSERT INTO call_events (call_id, event_type, event_timestamp, payload) VALUES ($1, $2, $3, $4)`
	_, err := h.db.Exec(query, event.CallID, event.EventType, event.Timestamp, event.Payload)

	if err != nil {
		l.Error().Err(err).Msg("CDR olayı veritabanına yazılamadı.")
		h.eventsFailed.WithLabelValues(event.EventType, "db_insert_failed").Inc()
		return
	}

	l.Info().Msg("CDR olayı başarıyla veritabanına kaydedildi.")
}
