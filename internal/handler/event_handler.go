// ========== FILE: sentiric-cdr-service/internal/handler/event_handler.go ==========
package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	userv1 "github.com/sentiric/sentiric-contracts/gen/go/sentiric/user/v1"
	"google.golang.org/grpc/metadata"
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

// YENİ: SIP URI'sinden telefon numarasını çıkarmak ve normalize etmek için.
var fromUserRegex = regexp.MustCompile(`sip:(\+?\d+)@`)

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
		h.handleCallEnded(l, &event) // <-- YENİ DURUM
	default:
		l.Debug().Msg("Bu olay tipi için özet CDR işlemi tanımlanmamış, atlanıyor.")
	}
}

func (h *EventHandler) logRawEvent(l zerolog.Logger, event *EventPayload) error {
	query := `INSERT INTO call_events (call_id, event_type, event_timestamp, payload) VALUES ($1, $2, $3, $4)`
	_, err := h.db.Exec(query, event.CallID, event.EventType, event.Timestamp, event.RawPayload)
	if err != nil {
		l.Error().Err(err).Msg("Ham CDR olayı veritabanına yazılamadı.")
		return err
	}
	l.Info().Msg("Ham CDR olayı başarıyla veritabanına kaydedildi.")
	return nil
}

func (h *EventHandler) handleCallStarted(l zerolog.Logger, event *EventPayload) {
	// YENİ: `sip-signaling` ile aynı normalizasyon mantığını kullanıyoruz.
	callerNumber := extractAndNormalizePhoneNumber(event.From)
	if callerNumber == "" {
		l.Warn().Msg("Arayan numarası 'From' URI'sinden çıkarılamadı, özet CDR oluşturulmayacak.")
		return
	}

	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-trace-id", event.TraceID)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	userRes, err := h.userClient.FindUserByContact(ctx, &userv1.FindUserByContactRequest{
		ContactType:  "phone",
		ContactValue: callerNumber,
	})

	var userID, tenantID sql.NullString
	var contactID sql.NullInt32

	if err != nil {
		l.Warn().Err(err).Msg("Arayan, user-service'de bulunamadı. Çağrı kaydı kullanıcıyla ilişkilendirilmeyecek.")
	} else if userRes.User != nil {
		userID = sql.NullString{String: userRes.User.Id, Valid: true}
		tenantID = sql.NullString{String: userRes.User.TenantId, Valid: true}
		for _, c := range userRes.User.Contacts {
			if c.ContactValue == callerNumber {
				contactID = sql.NullInt32{Int32: c.Id, Valid: true}
				break
			}
		}
		l.Info().Str("user_id", userID.String).Msg("Arayan kullanıcı başarıyla bulundu.")
	}

	query := `
		INSERT INTO calls (call_id, user_id, contact_id, tenant_id, start_time, status)
		VALUES ($1, $2, $3, $4, $5, 'STARTED')
		ON CONFLICT (call_id) DO NOTHING
	`
	_, err = h.db.Exec(query, event.CallID, userID, contactID, tenantID, event.Timestamp)
	if err != nil {
		l.Error().Err(err).Msg("Özet çağrı kaydı (CDR) veritabanına yazılamadı.")
		h.eventsFailed.WithLabelValues(event.EventType, "db_summary_insert_failed").Inc()
		return
	}
	l.Info().Msg("Özet çağrı kaydı (CDR) başarıyla oluşturuldu.")
}

// YENİ FONKSİYON: Çağrıyı sonlandırır.
func (h *EventHandler) handleCallEnded(l zerolog.Logger, event *EventPayload) {
	var startTime sql.NullTime
	// Önce çağrının başlangıç zamanını bul
	err := h.db.QueryRow("SELECT start_time FROM calls WHERE call_id = $1", event.CallID).Scan(&startTime)
	if err != nil {
		l.Warn().Err(err).Msg("Çağrı sonlandırma olayı için başlangıç kaydı bulunamadı. Güncelleme atlanıyor.")
		return
	}

	duration := int(event.Timestamp.Sub(startTime.Time).Seconds())

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

// YENİ FONKSİYON: SIP URI'sinden telefon numarasını çıkarır ve normalize eder.
func extractAndNormalizePhoneNumber(uri string) string {
	matches := fromUserRegex.FindStringSubmatch(uri)
	if len(matches) < 2 {
		return ""
	}

	originalNum := matches[1]
	var num []rune
	for _, r := range originalNum {
		if r >= '0' && r <= '9' {
			num = append(num, r)
		}
	}

	numStr := string(num)
	if len(numStr) == 11 && numStr[0] == '0' {
		return "90" + numStr[1:]
	}
	if len(numStr) == 10 && numStr[0] != '9' {
		return "90" + numStr
	}
	return numStr
}
