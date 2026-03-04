// sentiric-cdr-service/internal/handler/event_handler.go
package handler

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"

	"github.com/sentiric/sentiric-cdr-service/internal/logger"
	"github.com/sentiric/sentiric-cdr-service/internal/queue"
	"github.com/sentiric/sentiric-cdr-service/internal/repository" // YENİ
	"github.com/sentiric/sentiric-cdr-service/internal/utils"      // YENİ
	eventv1 "github.com/sentiric/sentiric-contracts/gen/go/sentiric/event/v1"
)

type EventHandler struct {
	repo            *repository.CallRepository // SQL DB yerine Repo kullanılıyor
	log             zerolog.Logger
	eventsProcessed *prometheus.CounterVec
	eventsFailed    *prometheus.CounterVec
}

// NewEventHandler constructor güncellendi
func NewEventHandler(db *sql.DB, log zerolog.Logger, processed, failed *prometheus.CounterVec) *EventHandler {
	return &EventHandler{
		repo:            repository.NewCallRepository(db, log),
		log:             log,
		eventsProcessed: processed,
		eventsFailed:    failed,
	}
}

func (h *EventHandler) HandleEvent(body []byte) queue.HandlerResult {
	// Olayları sırayla dene ve uygun olanı işle

	// 1. Call Started
	var callStarted eventv1.CallStartedEvent
	if err := proto.Unmarshal(body, &callStarted); err == nil && callStarted.EventType == "call.started" {
		return h.processCallStarted(body, &callStarted)
	}

	// 2. Call Ended
	var callEnded eventv1.CallEndedEvent
	if err := proto.Unmarshal(body, &callEnded); err == nil && callEnded.EventType == "call.ended" {
		return h.processCallEnded(body, &callEnded)
	}

	// 3. User Identified
	var userIdentified eventv1.UserIdentifiedForCallEvent
	if err := proto.Unmarshal(body, &userIdentified); err == nil && userIdentified.EventType == "user.identified.for.call" {
		return queue.Ack // Şu anki tabloda bu veri update edilebilir, şimdilik ACK.
	}

	// 4. Recording Available
	var recordingEvent eventv1.CallRecordingAvailableEvent
	if err := proto.Unmarshal(body, &recordingEvent); err == nil && recordingEvent.EventType == "call.recording.available" {
		return h.processRecordingAvailable(recordingEvent.CallId, recordingEvent.RecordingUri)
	}

	// 5. Generic Events (Call Answered vb.)
	var genericEvent eventv1.GenericEvent
	if err := proto.Unmarshal(body, &genericEvent); err == nil && genericEvent.EventType != "" {
		return h.handleGenericEvent(&genericEvent, body)
	}

	// Fallback JSON handling (Legacy systems)
	var jsonEvent map[string]interface{}
	if err := json.Unmarshal(body, &jsonEvent); err == nil {
		if uri, ok := jsonEvent["uri"].(string); ok {
			if callId, ok := jsonEvent["callId"].(string); ok {
				return h.processRecordingAvailable(callId, uri)
			}
		}
	}

	h.log.Warn().Str("event", logger.EventCdrIgnored).Msg("Bilinmeyen mesaj formatı. Discard ediliyor.")
	h.eventsFailed.WithLabelValues("unknown", "format_error").Inc()
	return queue.NackDiscard
}

func (h *EventHandler) processCallStarted(body []byte, event *eventv1.CallStartedEvent) queue.HandlerResult {
	l := h.log.With().Str("call_id", event.CallId).Logger()

	// Tenant ID Belirle
	tenantID := "system"
	if event.DialplanResolution != nil && event.DialplanResolution.TenantId != "" {
		tenantID = event.DialplanResolution.TenantId
	}

	// Kullanıcı ve Kontak ID'lerini çözümle
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

	// [YENİ]: Numara ve Yön Analizi
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

	if err := h.repo.UpsertCallStart(context.Background(), data); err != nil {
		l.Error().Err(err).Msg("DB Write Error (CallStarted)")
		return queue.NackRetry
	}

	// Audit Log
	_ = h.repo.LogEvent(context.Background(), event.CallId, event.EventType, event.Timestamp.AsTime(), nil)

	h.eventsProcessed.WithLabelValues(event.EventType).Inc()
	return queue.Ack
}

func (h *EventHandler) processCallEnded(body []byte, event *eventv1.CallEndedEvent) queue.HandlerResult {
	l := h.log.With().Str("call_id", event.CallId).Logger()

	// Mevcut durumu çek
	startTime, answerTime, tenantID, err := h.repo.GetCallDates(context.Background(), event.CallId)
	if err != nil {
		if err == sql.ErrNoRows {
			// Başlangıç event'i henüz gelmemiş olabilir, Retry mantıklı
			l.Warn().Msg("Çağrı kaydı DB'de yok, CallStarted gecikmiş olabilir. Retry ediliyor.")
			return queue.NackRetry
		}
		l.Error().Err(err).Msg("Çağrı kaydı okunamadı.")
		return queue.NackRetry
	}

	endTime := event.Timestamp.AsTime()
	duration := 0
	disposition := "NO_ANSWER"

	// [YENİ]: Hassas Süre Hesabı
	if answerTime.Valid {
		duration = int(endTime.Sub(answerTime.Time).Seconds())
		disposition = "ANSWERED"
	} else if startTime.Valid {
		// Hiç açılmadıysa çalma süresi
		duration = int(endTime.Sub(startTime.Time).Seconds())
	}
	if duration < 0 {
		duration = 0
	}

	// [YENİ]: Disposition Mantığı
	if !answerTime.Valid && duration > 0 && event.Reason == "normal_clearing" {
		// Bazen ANSWER sinyali kaçabilir, süre varsa ANSWERED kabul et (Fail-safe)
		disposition = "ANSWERED"
	} else if event.Reason == "busy" || event.Reason == "user_busy" {
		disposition = "BUSY"
	} else if event.Reason == "failure" || event.Reason == "network_failure" {
		disposition = "FAILED"
	}

	// [YENİ]: Hangup Source Tahmini (Basit)
	hangupSource := "UNKNOWN"
	if event.Reason == "normal_clearing" {
		hangupSource = "CALLER" // Varsayım
	}

	// Fatura Kesimi
	if disposition == "ANSWERED" && duration > 0 {
		if err := h.calculateAndRecordUsage(context.Background(), event.CallId, tenantID, duration); err != nil {
			return queue.NackRetry
		}
	}

	updateData := repository.CallEndData{
		CallID:          event.CallId,
		EndTime:         endTime,
		DurationSeconds: duration,
		Disposition:     disposition,
		HangupSource:    hangupSource,
		SipCode:         0, // GenericEvent içinden çıkarılabilir ileride
	}

	if err := h.repo.UpdateCallEnd(context.Background(), updateData); err != nil {
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

	exists, err := h.repo.CheckUsageExists(ctx, callID, "telephony_minute")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	costPerUnit := 0.005 // Varsayılan maliyet (DB'den çekilebilir)
	minutes := float64(duration) / 60.0
	totalCost := minutes * costPerUnit

	if err := h.repo.CreateUsageRecord(ctx, tenantID, callID, "telephony-core", "telephony_minute", minutes, totalCost); err != nil {
		h.log.Error().Err(err).Msg("Usage record oluşturulamadı!")
		return err
	}

	// Ana tabloyu güncelle
	_ = h.repo.UpdateCost(ctx, callID, totalCost)

	h.log.Info().Str("call_id", callID).Float64("cost", totalCost).Msg("💰 Fatura kaydı oluşturuldu.")
	return nil
}

func (h *EventHandler) processRecordingAvailable(callId string, uri string) queue.HandlerResult {
	if err := h.repo.UpdateRecording(context.Background(), callId, uri); err != nil {
		h.log.Error().Err(err).Msg("Recording Update Error")
		return queue.NackRetry
	}
	h.log.Info().Str("uri", uri).Msg("🎙️ Ses kaydı DB'ye işlendi.")
	h.eventsProcessed.WithLabelValues("call.recording.available").Inc()
	return queue.Ack
}

func (h *EventHandler) handleGenericEvent(event *eventv1.GenericEvent, rawBody []byte) queue.HandlerResult {
	// [KRİTİK]: Call Answered Yakalama
	if event.EventType == "call.answered" {
		if err := h.repo.SetAnswerTime(context.Background(), event.TraceId, event.Timestamp.AsTime()); err != nil {
			return queue.NackRetry
		}
	}

	// [DÜZELTME]: Raw Protobuf byte'ları yerine, event içindeki JSON payload'u kullan.
	// Eğer payload yoksa boş bir JSON nesnesi {} yaz, PostgreSQL kızmasın.
	payloadBytes := []byte("{}")
	if event.PayloadJson != "" {
		payloadBytes = []byte(event.PayloadJson)
	}

	// Raw Event Loglama (Denetim için)
	_ = h.repo.LogEvent(context.Background(), event.TraceId, event.EventType, event.Timestamp.AsTime(), payloadBytes)

	return queue.Ack
}
