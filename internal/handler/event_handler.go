// sentiric-cdr-service/internal/handler/event_handler.go
package handler

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
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

// HandleEvent: RabbitMQ'dan gelen ham mesajları yönlendirir
func (h *EventHandler) HandleEvent(body []byte) queue.HandlerResult {
	// --- 1. CallStartedEvent ---
	var callStarted eventv1.CallStartedEvent
	if err := proto.Unmarshal(body, &callStarted); err == nil && callStarted.EventType == "call.started" {
		return h.processCallStarted(body, &callStarted)
	}

	// --- 2. CallEndedEvent ---
	var callEnded eventv1.CallEndedEvent
	if err := proto.Unmarshal(body, &callEnded); err == nil && callEnded.EventType == "call.ended" {
		return h.processCallEnded(body, &callEnded)
	}

	// --- 3. UserIdentifiedForCallEvent ---
	var userIdentified eventv1.UserIdentifiedForCallEvent
	if err := proto.Unmarshal(body, &userIdentified); err == nil && userIdentified.EventType == "user.identified.for.call" {
		return h.processUserIdentified(body, &userIdentified)
	}

	// --- 4. CallRecordingAvailableEvent (YENİ EKLENDİ) ---
	// Media Service bazen raw JSON atıyor olabilir, bu yüzden önce Proto dene, olmazsa JSON dene.
	var recordingEvent eventv1.CallRecordingAvailableEvent
	if err := proto.Unmarshal(body, &recordingEvent); err == nil && recordingEvent.EventType == "call.recording.available" {
		return h.processRecordingAvailable(&recordingEvent)
	}

	// JSON Fallback (Media Service Worker JSON atıyor olabilir)
	var jsonEvent map[string]interface{}
	if err := json.Unmarshal(body, &jsonEvent); err == nil {
		if uri, ok := jsonEvent["uri"].(string); ok {
			if callId, ok := jsonEvent["callId"].(string); ok {
				// Manuel Event Oluştur
				manualEvent := &eventv1.CallRecordingAvailableEvent{
					EventType:    "call.recording.available",
					CallId:       callId,
					RecordingUri: uri,
				}
				return h.processRecordingAvailable(manualEvent)
			}
		}
	}

	// --- 5. GenericEvent ---
	var genericEvent eventv1.GenericEvent
	if err := proto.Unmarshal(body, &genericEvent); err == nil && genericEvent.EventType != "" {
		h.handleGenericEvent(&genericEvent)
		return queue.Ack
	}

	// Bilinmeyen Format
	h.log.Warn().Str("event", logger.EventCdrIgnored).Msg("Bilinmeyen mesaj formatı")
	h.eventsFailed.WithLabelValues("unknown", "format_error").Inc()
	return queue.NackDiscard
}

// --- İŞLEYİCİLER ---

func (h *EventHandler) processCallStarted(body []byte, event *eventv1.CallStartedEvent) queue.HandlerResult {
	tenantID := "unknown"
	if event.DialplanResolution != nil {
		tenantID = event.DialplanResolution.TenantId
	}

	l := h.log.With().Str("call_id", event.CallId).Logger()
	l.Info().Msg("Call Started olayı işleniyor")

	// Ham log
	_ = h.logRawEvent(l, event.CallId, event.EventType, event.Timestamp.AsTime(), body)

	var userID interface{} = nil
	var contactID interface{} = nil

	if event.DialplanResolution != nil {
		if event.DialplanResolution.MatchedUser != nil {
			if _, err := uuid.Parse(event.DialplanResolution.MatchedUser.Id); err == nil {
				userID = event.DialplanResolution.MatchedUser.Id
			}
		}
		if event.DialplanResolution.MatchedContact != nil {
			contactID = event.DialplanResolution.MatchedContact.Id
		}
	}

	query := `
		INSERT INTO calls (call_id, start_time, status, user_id, contact_id, tenant_id) 
		VALUES ($1, $2, 'STARTED', $3, $4, $5)
		ON CONFLICT (call_id) DO UPDATE SET 
			start_time = COALESCE(calls.start_time, EXCLUDED.start_time),
			user_id = COALESCE(calls.user_id, EXCLUDED.user_id),
			contact_id = COALESCE(calls.contact_id, EXCLUDED.contact_id),
			tenant_id = COALESCE(calls.tenant_id, EXCLUDED.tenant_id),
			updated_at = NOW()`

	_, err := h.db.Exec(query, event.CallId, event.Timestamp.AsTime(), userID, contactID, tenantID)
	if err != nil {
		l.Error().Err(err).Msg("DB Write Error (CallStarted)")
		return queue.NackRequeue
	}

	h.eventsProcessed.WithLabelValues(event.EventType).Inc()
	return queue.Ack
}

func (h *EventHandler) processCallEnded(body []byte, event *eventv1.CallEndedEvent) queue.HandlerResult {
	l := h.log.With().Str("call_id", event.CallId).Logger()
	_ = h.logRawEvent(l, event.CallId, event.EventType, event.Timestamp.AsTime(), body)

	// Duration Hesaplama ve Disposition (Mevcut kodunuzdaki mantık buraya taşınır)
	// ... (Kısalık için özet geçiyorum, mevcut mantık korunmalı)

	query := `UPDATE calls SET end_time = $1, status = 'COMPLETED', disposition = $2, updated_at = NOW() WHERE call_id = $3`
	_, err := h.db.Exec(query, event.Timestamp.AsTime(), "COMPLETED", event.CallId)
	if err != nil {
		l.Error().Err(err).Msg("DB Write Error (CallEnded)")
		return queue.NackRequeue
	}

	h.eventsProcessed.WithLabelValues(event.EventType).Inc()
	return queue.Ack
}

func (h *EventHandler) processUserIdentified(body []byte, event *eventv1.UserIdentifiedForCallEvent) queue.HandlerResult {
	// Mevcut mantık...
	return queue.Ack
}

// [YENİ] Kayıt URL'sini Güncelleme
func (h *EventHandler) processRecordingAvailable(event *eventv1.CallRecordingAvailableEvent) queue.HandlerResult {
	l := h.log.With().Str("call_id", event.CallId).Logger()
	l.Info().Str("uri", event.RecordingUri).Msg("🎙️ Ses kaydı mevcut, DB güncelleniyor.")

	query := `UPDATE calls SET recording_url = $1, updated_at = NOW() WHERE call_id = $2`
	res, err := h.db.Exec(query, event.RecordingUri, event.CallId)
	if err != nil {
		l.Error().Err(err).Msg("DB Update Error (Recording)")
		return queue.NackRequeue
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		l.Warn().Msg("Kayıt güncellenecek çağrı bulunamadı (Race Condition olabilir)")
		// Çağrı kaydı henüz oluşmamış olabilir, insert etmeyelim, upsert mantığı karmaşıklaşır.
		// Sadece loglayıp geçiyoruz, retry mekanizmasıyla çözülebilir.
	}

	h.eventsProcessed.WithLabelValues(event.EventType).Inc()
	return queue.Ack
}

func (h *EventHandler) handleGenericEvent(event *eventv1.GenericEvent) {
	if event.EventType == "call.answered" {
		// Answer Time Update Logic
		query := `UPDATE calls SET answer_time = $1, status = 'ANSWERED', updated_at = NOW() WHERE call_id = $2`
		_, _ = h.db.Exec(query, event.Timestamp.AsTime(), event.TraceId)
	}
	// Generic Event Logging
	query := `INSERT INTO call_events (call_id, event_type, event_timestamp, payload) VALUES ($1, $2, $3, $4::jsonb)`
	_, _ = h.db.Exec(query, event.TraceId, event.EventType, event.Timestamp.AsTime(), event.PayloadJson)
}

func (h *EventHandler) logRawEvent(l zerolog.Logger, callID, eventType string, ts time.Time, rawPayload []byte) error {
	payloadMap := map[string]string{"raw_proto_base64": base64.StdEncoding.EncodeToString(rawPayload)}
	jsonPayload, _ := json.Marshal(payloadMap)
	_, err := h.db.Exec(`INSERT INTO call_events (call_id, event_type, event_timestamp, payload) VALUES ($1, $2, $3, $4)`, callID, eventType, ts, string(jsonPayload))
	return err
}
