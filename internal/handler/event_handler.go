// sentiric-cdr-service/internal/handler/event_handler.go
package handler

import (
	"context"
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
	return &EventHandler{db: db, log: log, eventsProcessed: processed, eventsFailed: failed}
}

func (h *EventHandler) HandleEvent(body []byte) queue.HandlerResult {
	var callStarted eventv1.CallStartedEvent
	if err := proto.Unmarshal(body, &callStarted); err == nil && callStarted.EventType == "call.started" {
		return h.processCallStarted(body, &callStarted)
	}

	var callEnded eventv1.CallEndedEvent
	if err := proto.Unmarshal(body, &callEnded); err == nil && callEnded.EventType == "call.ended" {
		return h.processCallEnded(body, &callEnded)
	}

	var userIdentified eventv1.UserIdentifiedForCallEvent
	if err := proto.Unmarshal(body, &userIdentified); err == nil && userIdentified.EventType == "user.identified.for.call" {
		return queue.Ack
	}

	var recordingEvent eventv1.CallRecordingAvailableEvent
	if err := proto.Unmarshal(body, &recordingEvent); err == nil && recordingEvent.EventType == "call.recording.available" {
		return h.processRecordingAvailable(recordingEvent.CallId, recordingEvent.RecordingUri)
	}

	var jsonEvent map[string]interface{}
	if err := json.Unmarshal(body, &jsonEvent); err == nil {
		if uri, ok := jsonEvent["uri"].(string); ok {
			if callId, ok := jsonEvent["callId"].(string); ok {
				return h.processRecordingAvailable(callId, uri)
			}
		}
	}

	var genericEvent eventv1.GenericEvent
	if err := proto.Unmarshal(body, &genericEvent); err == nil && genericEvent.EventType != "" {
		return h.handleGenericEvent(&genericEvent)
	}

	h.log.Warn().Str("event", logger.EventCdrIgnored).Msg("Bilinmeyen mesaj formatı. Discard ediliyor.")
	h.eventsFailed.WithLabelValues("unknown", "format_error").Inc()
	return queue.NackDiscard // Kalıcı hata, tekrar deneme.
}

func (h *EventHandler) processCallStarted(body []byte, event *eventv1.CallStartedEvent) queue.HandlerResult {
	tenantID := "system"
	if event.DialplanResolution != nil && event.DialplanResolution.TenantId != "" {
		tenantID = event.DialplanResolution.TenantId
	}

	l := h.log.With().Str("call_id", event.CallId).Logger()

	// Raw event kaydı idempotent'tir (aynı payload tekrar yazılabilir log amaçlı)
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

	// İDempotency: UPSERT yapısı zaten idempotent'tir.
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
		return queue.NackRetry // Veritabanı hatası, geçicidir. Tekrar dene.
	}

	h.eventsProcessed.WithLabelValues(event.EventType).Inc()
	return queue.Ack
}

func (h *EventHandler) processCallEnded(body []byte, event *eventv1.CallEndedEvent) queue.HandlerResult {
	l := h.log.With().Str("call_id", event.CallId).Logger()
	_ = h.logRawEvent(l, event.CallId, event.EventType, event.Timestamp.AsTime(), body)

	var startTime, answerTime sql.NullTime
	var tenantID string

	err := h.db.QueryRow("SELECT start_time, answer_time, tenant_id FROM calls WHERE call_id = $1", event.CallId).
		Scan(&startTime, &answerTime, &tenantID)

	if err != nil && err != sql.ErrNoRows {
		l.Error().Err(err).Msg("Çağrı kaydı DB'den okunamadı.")
		return queue.NackRetry // DB hatası
	}

	endTime := event.Timestamp.AsTime()
	duration := 0
	finalDisposition := "NO_ANSWER"

	if answerTime.Valid {
		duration = int(endTime.Sub(answerTime.Time).Seconds())
		finalDisposition = "ANSWERED"
	} else if startTime.Valid {
		duration = int(endTime.Sub(startTime.Time).Seconds())
	}
	if duration < 0 {
		duration = 0
	}

	if !answerTime.Valid && duration > 0 && event.Reason == "normal_clearing" {
		finalDisposition = "ANSWERED"
	} else if event.Reason == "busy" || event.Reason == "user_busy" {
		finalDisposition = "BUSY"
	} else if event.Reason == "failure" || event.Reason == "network_failure" {
		finalDisposition = "FAILED"
	}

	// FATURA KESİMİ (Idempotency Garantisi eklendi)
	if finalDisposition == "ANSWERED" && duration > 0 {
		err := h.calculateAndRecordUsage(context.Background(), event.CallId, tenantID, duration)
		if err != nil {
			return queue.NackRetry
		}
	}

	// Idempotent UPDATE
	query := `UPDATE calls SET end_time = $1, duration_seconds = $2, status = 'COMPLETED', disposition = $3, updated_at = NOW() WHERE call_id = $4`
	_, err = h.db.Exec(query, endTime, duration, finalDisposition, event.CallId)
	if err != nil {
		l.Error().Err(err).Msg("DB Write Error (CallEnded)")
		return queue.NackRetry
	}

	h.eventsProcessed.WithLabelValues(event.EventType).Inc()
	return queue.Ack
}

func (h *EventHandler) calculateAndRecordUsage(ctx context.Context, callID, tenantID string, duration int) error {
	if duration <= 0 {
		return nil
	}

	// IDEMPOTENCY KONTROLÜ: Aynı çağrı için fatura zaten kesilmiş mi?
	var exists bool
	checkErr := h.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM usage_records WHERE call_id = $1 AND resource_type = 'telephony_minute')", callID).Scan(&exists)
	if checkErr != nil {
		h.log.Error().Err(checkErr).Msg("Fatura mükerrerlik kontrolü başarısız")
		return checkErr
	}
	if exists {
		h.log.Info().Str("call_id", callID).Msg("💰 Bu çağrı için fatura zaten kesilmiş. Mükerrer işlem atlanıyor (Idempotent).")
		return nil
	}

	var costPerUnit float64
	err := h.db.QueryRowContext(ctx, "SELECT cost_per_unit FROM cost_models WHERE id = 'CORE_CALL_MINUTE'").Scan(&costPerUnit)
	if err != nil {
		costPerUnit = 0.005
	}

	minutes := float64(duration) / 60.0
	totalCost := minutes * costPerUnit

	usageQuery := `INSERT INTO usage_records (tenant_id, call_id, service_name, resource_type, quantity, calculated_cost) VALUES ($1, $2, 'telephony-core', 'telephony_minute', $3, $4)`
	_, err = h.db.ExecContext(ctx, usageQuery, tenantID, callID, minutes, totalCost)
	if err != nil {
		h.log.Error().Err(err).Msg("Usage record oluşturulamadı!")
		return err
	}

	h.log.Info().Str("call_id", callID).Float64("cost", totalCost).Msg("💰 Fatura kaydı oluşturuldu.")
	_, _ = h.db.ExecContext(ctx, "UPDATE calls SET total_cost = $1 WHERE call_id = $2", totalCost, callID)
	return nil
}

func (h *EventHandler) processRecordingAvailable(callId string, uri string) queue.HandlerResult {
	l := h.log.With().Str("call_id", callId).Logger()

	// Idempotent UPDATE
	query := `UPDATE calls SET recording_url = $1, updated_at = NOW() WHERE call_id = $2`
	_, err := h.db.Exec(query, uri, callId)
	if err != nil {
		l.Error().Err(err).Msg("DB Update Error (Recording)")
		return queue.NackRetry
	}

	l.Info().Str("uri", uri).Msg("🎙️ Ses kaydı DB'ye işleniyor.")
	h.eventsProcessed.WithLabelValues("call.recording.available").Inc()
	return queue.Ack
}

func (h *EventHandler) handleGenericEvent(event *eventv1.GenericEvent) queue.HandlerResult {
	if event.EventType == "call.answered" {
		query := `UPDATE calls SET answer_time = $1, status = 'ANSWERED', updated_at = NOW() WHERE call_id = $2`
		_, err := h.db.Exec(query, event.Timestamp.AsTime(), event.TraceId)
		if err != nil {
			return queue.NackRetry
		}
	}

	// IDEMPOTENCY: Aynı event daha önce loglanmış mı?
	var exists bool
	h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM call_events WHERE call_id = $1 AND event_type = $2 AND event_timestamp = $3)", event.TraceId, event.EventType, event.Timestamp.AsTime()).Scan(&exists)
	if !exists {
		query := `INSERT INTO call_events (call_id, event_type, event_timestamp, payload) VALUES ($1, $2, $3, $4::jsonb)`
		_, err := h.db.Exec(query, event.TraceId, event.EventType, event.Timestamp.AsTime(), event.PayloadJson)
		if err != nil {
			return queue.NackRetry
		}
	}

	return queue.Ack
}

func (h *EventHandler) logRawEvent(l zerolog.Logger, callID, eventType string, ts time.Time, rawPayload []byte) error {
	payloadMap := map[string]string{"raw_proto_base64": base64.StdEncoding.EncodeToString(rawPayload)}
	jsonPayload, _ := json.Marshal(payloadMap)
	// Sadece denetim logu, mükerrer olsa da çok kritik değil ama failsafe için.
	_, err := h.db.Exec(`INSERT INTO call_events (call_id, event_type, event_timestamp, payload) VALUES ($1, $2, $3, $4)`, callID, eventType, ts, string(jsonPayload))
	return err
}
