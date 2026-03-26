// File: internal/handler/event_handler.go
package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"

	"github.com/sentiric/sentiric-cdr-service/internal/logger"
	"github.com/sentiric/sentiric-cdr-service/internal/queue"
	"github.com/sentiric/sentiric-cdr-service/internal/repository"
	"github.com/sentiric/sentiric-cdr-service/internal/utils"
	eventv1 "github.com/sentiric/sentiric-contracts/gen/go/sentiric/event/v1"
)

type EventHandler struct {
	repo            *repository.CallRepository
	log             zerolog.Logger
	eventsProcessed *prometheus.CounterVec
	eventsFailed    *prometheus.CounterVec
}

func NewEventHandler(db *sql.DB, log zerolog.Logger, processed, failed *prometheus.CounterVec) *EventHandler {
	return &EventHandler{
		repo:            repository.NewCallRepository(db, log),
		log:             log,
		eventsProcessed: processed,
		eventsFailed:    failed,
	}
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
		return h.processRecordingAvailable(&recordingEvent)
	}

	var genericEvent eventv1.GenericEvent
	if err := proto.Unmarshal(body, &genericEvent); err == nil && genericEvent.EventType != "" {
		return h.handleGenericEvent(&genericEvent, body)
	}

	var jsonEvent map[string]interface{}
	if err := json.Unmarshal(body, &jsonEvent); err == nil {
		if reason, ok := jsonEvent["reason"].(string); ok && reason == "workflow_hangup" {
			// [ARCH-COMPLIANCE] SUTS v4.0 'event' zorunluluğu
			h.log.Debug().Str("event", logger.EventCdrIgnored).Msg("Workflow hangup event safely ignored by CDR.")
			return queue.Ack
		}
		if uri, ok := jsonEvent["uri"].(string); ok {
			if callId, ok := jsonEvent["callId"].(string); ok {
				dummyEvent := &eventv1.CallRecordingAvailableEvent{
					CallId:       callId,
					RecordingUri: uri,
					TraceId:      callId,
				}
				return h.processRecordingAvailable(dummyEvent)
			}
		}
	}

	h.log.Warn().Str("event", logger.EventCdrIgnored).Msg("Bilinmeyen mesaj formatı. Discard ediliyor.")
	h.eventsFailed.WithLabelValues("unknown", "format_error").Inc()
	return queue.NackDiscard
}

func (h *EventHandler) processCallStarted(body []byte, event *eventv1.CallStartedEvent) queue.HandlerResult {
	// [ARCH-COMPLIANCE] SUTS v4.0 trace_id ve span_id eklendi.
	l := h.log.With().
		Str("trace_id", event.CallId).
		Str("span_id", uuid.New().String()).
		Logger()

	tenantID := "system"
	if event.DialplanResolution != nil && event.DialplanResolution.TenantId != "" {
		tenantID = event.DialplanResolution.TenantId
	}

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

	callerNum := utils.ParseSipUri(event.FromUri)
	calleeNum := utils.ParseSipUri(event.ToUri)
	direction := utils.DetermineDirection(callerNum, calleeNum)

	data := repository.CallStartData{
		CallID:       event.CallId,
		TenantID:     tenantID,
		CallerNumber: callerNum,
		CalleeNumber: calleeNum,
		Direction:    direction,
		StartTime:    event.Timestamp.AsTime(),
		UserID:       userID,
		ContactID:    contactID,
	}

	//[ARCH-COMPLIANCE] ARCH-004 İhlali Giderildi: context.Background() yerine Timeout context eklendi
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.repo.UpsertCallStart(ctx, data); err != nil {
		l.Error().Err(err).Str("event", logger.EventDbWriteFail).Msg("DB Write Error (CallStarted)")
		return queue.NackRetry
	}

	_ = h.repo.LogEvent(ctx, event.CallId, event.EventType, event.Timestamp.AsTime(), "{}")

	h.eventsProcessed.WithLabelValues(event.EventType).Inc()
	return queue.Ack
}

func (h *EventHandler) processCallEnded(body []byte, event *eventv1.CallEndedEvent) queue.HandlerResult {
	l := h.log.With().
		Str("trace_id", event.CallId).
		Str("span_id", uuid.New().String()).
		Logger()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	startTime, answerTime, tenantID, err := h.repo.GetCallDates(ctx, event.CallId)
	if err != nil {
		if err == sql.ErrNoRows {
			l.Warn().Str("event", logger.EventDbReadFail).Msg("Çağrı kaydı DB'de yok, CallStarted gecikmiş olabilir. Retry ediliyor.")
			return queue.NackRetry
		}
		l.Error().Err(err).Str("event", logger.EventDbReadFail).Msg("Çağrı kaydı okunamadı.")
		return queue.NackRetry
	}

	endTime := event.Timestamp.AsTime()
	duration := 0
	disposition := "NO_ANSWER"

	if answerTime.Valid {
		duration = int(endTime.Sub(answerTime.Time).Seconds())
		disposition = "ANSWERED"
	} else if startTime.Valid {
		duration = int(endTime.Sub(startTime.Time).Seconds())
	}
	if duration < 0 {
		duration = 0
	}

	if !answerTime.Valid && duration > 0 && event.Reason == "normal_clearing" {
		disposition = "ANSWERED"
	} else if event.Reason == "busy" || event.Reason == "user_busy" {
		disposition = "BUSY"
	} else if event.Reason == "failure" || event.Reason == "network_failure" {
		disposition = "FAILED"
	}

	hangupSource := "UNKNOWN"
	if event.Reason == "normal_clearing" {
		hangupSource = "CALLER"
	} else if event.Reason == "system_terminated" {
		hangupSource = "APP"
		disposition = "ANSWERED"
	}

	if disposition == "ANSWERED" && duration > 0 {
		if err := h.calculateAndRecordUsage(ctx, event.CallId, tenantID, duration, l); err != nil {
			return queue.NackRetry
		}
	}

	updateData := repository.CallEndData{
		CallID:          event.CallId,
		EndTime:         endTime,
		DurationSeconds: duration,
		Disposition:     disposition,
		HangupSource:    hangupSource,
		SipCode:         0,
	}

	if err := h.repo.UpdateCallEnd(ctx, updateData); err != nil {
		l.Error().Err(err).Str("event", logger.EventDbWriteFail).Msg("DB Write Error (CallEnded)")
		return queue.NackRetry
	}

	h.eventsProcessed.WithLabelValues(event.EventType).Inc()
	return queue.Ack
}

func (h *EventHandler) calculateAndRecordUsage(ctx context.Context, callID, tenantID string, duration int, l zerolog.Logger) error {
	if duration <= 0 {
		return nil
	}

	exists, err := h.repo.CheckUsageExists(ctx, callID, "telephony_minute")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	costPerUnit := 0.005
	minutes := float64(duration) / 60.0
	totalCost := minutes * costPerUnit

	if err := h.repo.CreateUsageRecord(ctx, tenantID, callID, "telephony-core", "telephony_minute", minutes, totalCost); err != nil {
		l.Error().Err(err).Str("event", logger.EventBillingFail).Msg("Usage record oluşturulamadı!")
		return err
	}

	_ = h.repo.UpdateCost(ctx, callID, totalCost)

	l.Info().Float64("cost", totalCost).Str("event", logger.EventBillingSuccess).Msg("💰 Fatura kaydı oluşturuldu.")
	return nil
}

func (h *EventHandler) processRecordingAvailable(event *eventv1.CallRecordingAvailableEvent) queue.HandlerResult {
	l := h.log.With().
		Str("trace_id", event.CallId).
		Str("span_id", uuid.New().String()).
		Logger()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.repo.UpdateRecording(ctx, event.CallId, event.RecordingUri); err != nil {
		l.Error().Err(err).Str("event", logger.EventRecordingFail).Msg("Recording Update Error")
		return queue.NackRetry
	}
	l.Info().Str("uri", event.RecordingUri).Str("event", logger.EventRecordingSuccess).Msg("🎙️ Ses kaydı DB'ye işlendi.")
	h.eventsProcessed.WithLabelValues("call.recording.available").Inc()
	return queue.Ack
}

func (h *EventHandler) handleGenericEvent(event *eventv1.GenericEvent, rawBody []byte) queue.HandlerResult {
	l := h.log.With().
		Str("trace_id", event.TraceId).
		Str("span_id", uuid.New().String()).
		Logger()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if event.EventType == "call.answered" {
		if err := h.repo.SetAnswerTime(ctx, event.TraceId, event.Timestamp.AsTime()); err != nil {
			l.Error().Err(err).Str("event", logger.EventDbWriteFail).Msg("SetAnswerTime failed")
			return queue.NackRetry
		}
	}

	payloadStr := "{}"
	if len(event.PayloadJson) > 0 {
		payloadStr = event.PayloadJson
	}

	err := h.repo.LogEvent(ctx, event.TraceId, event.EventType, event.Timestamp.AsTime(), payloadStr)
	if err != nil {
		l.Error().Err(err).Str("event", logger.EventRawLogFail).Msg("LogEvent DB'ye yazılamadı")
	}

	return queue.Ack
}
