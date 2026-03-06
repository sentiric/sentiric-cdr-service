// sentiric-cdr-service/internal/repository/call_repository.go
package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/rs/zerolog"
)

type CallRepository struct {
	db  *sql.DB
	log zerolog.Logger
}

func NewCallRepository(db *sql.DB, log zerolog.Logger) *CallRepository {
	return &CallRepository{db: db, log: log}
}

type CallStartData struct {
	CallID       string
	TenantID     string
	CallerNumber string
	CalleeNumber string
	Direction    string
	StartTime    time.Time
	UserID       interface{} // uuid or nil
	ContactID    interface{} // int or nil
}

func (r *CallRepository) UpsertCallStart(ctx context.Context, data CallStartData) error {
	// [KRİTİK DÜZELTME]: DO UPDATE kısmından 'recording_url' çıkarıldı.
	// Artık başlangıç event'i asla kayıt URL'ini ezemez.
	query := `
		INSERT INTO calls (
			call_id, tenant_id, caller_number, callee_number, direction, 
			start_time, status, user_id, contact_id
		) 
		VALUES ($1, $2, $3, $4, $5, $6, 'STARTED', $7, $8)
		ON CONFLICT (call_id) DO UPDATE SET 
			tenant_id = COALESCE(calls.tenant_id, EXCLUDED.tenant_id),
			caller_number = COALESCE(calls.caller_number, EXCLUDED.caller_number),
			callee_number = COALESCE(calls.callee_number, EXCLUDED.callee_number),
			user_id = COALESCE(calls.user_id, EXCLUDED.user_id),
			updated_at = NOW()`

	_, err := r.db.ExecContext(ctx, query,
		data.CallID, data.TenantID, data.CallerNumber, data.CalleeNumber, data.Direction,
		data.StartTime, data.UserID, data.ContactID,
	)
	return err
}

func (r *CallRepository) SetAnswerTime(ctx context.Context, callID string, answerTime time.Time) error {
	query := `
		UPDATE calls SET 
			answer_time = $1, 
			status = 'ANSWERED', 
			updated_at = NOW() 
		WHERE call_id = $2`
	_, err := r.db.ExecContext(ctx, query, answerTime, callID)
	return err
}

type CallEndData struct {
	CallID          string
	EndTime         time.Time
	DurationSeconds int
	Disposition     string
	HangupSource    string
	SipCode         int32
}

func (r *CallRepository) UpdateCallEnd(ctx context.Context, data CallEndData) error {
	// [KRİTİK DÜZELTME]: Sadece bitişle ilgili alanlar güncelleniyor.
	// recording_url ve total_cost BURADA GÜNCELLENMEZ.
	query := `
		UPDATE calls SET 
			end_time = $1, 
			duration_seconds = $2, 
			status = 'COMPLETED', 
			disposition = $3,
			hangup_source = $4,
			sip_hangup_cause = $5,
			updated_at = NOW() 
		WHERE call_id = $6`

	_, err := r.db.ExecContext(ctx, query,
		data.EndTime, data.DurationSeconds, data.Disposition,
		data.HangupSource, data.SipCode, data.CallID,
	)
	return err
}

func (r *CallRepository) UpdateCost(ctx context.Context, callID string, cost float64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE calls SET total_cost = $1 WHERE call_id = $2", cost, callID)
	return err
}

func (r *CallRepository) GetCallDates(ctx context.Context, callID string) (startTime, answerTime sql.NullTime, tenantID string, err error) {
	err = r.db.QueryRowContext(ctx, "SELECT start_time, answer_time, tenant_id FROM calls WHERE call_id = $1", callID).
		Scan(&startTime, &answerTime, &tenantID)
	return
}

func (r *CallRepository) CheckUsageExists(ctx context.Context, callID, resourceType string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM usage_records WHERE call_id = $1 AND resource_type = $2)", callID, resourceType).Scan(&exists)
	return exists, err
}

func (r *CallRepository) CreateUsageRecord(ctx context.Context, tenantID, callID, service, resource string, qty, cost float64) error {
	query := `INSERT INTO usage_records (tenant_id, call_id, service_name, resource_type, quantity, calculated_cost) VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.db.ExecContext(ctx, query, tenantID, callID, service, resource, qty, cost)
	return err
}

func (r *CallRepository) LogEvent(ctx context.Context, callID, eventType string, ts time.Time, payloadJsonString string) error {
	var exists bool
	_ = r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM call_events WHERE call_id = $1 AND event_type = $2 AND event_timestamp = $3)", callID, eventType, ts).Scan(&exists)
	if exists {
		return nil
	}

	query := `INSERT INTO call_events (call_id, event_type, event_timestamp, payload) VALUES ($1, $2, $3, $4::jsonb)`
	_, err := r.db.ExecContext(ctx, query, callID, eventType, ts, payloadJsonString)
	return err
}

func (r *CallRepository) UpdateRecording(ctx context.Context, callID, uri string) error {
	// [DEBUG]: RowsAffected kontrolü eklendi.
	query := `UPDATE calls SET recording_url = $1, updated_at = NOW() WHERE call_id = $2`
	res, err := r.db.ExecContext(ctx, query, uri, callID)
	if err != nil {
		return err
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		r.log.Warn().Str("call_id", callID).Msg("⚠️ UpdateRecording: Kayıt güncellenemedi çünkü Call ID bulunamadı.")
	} else {
		r.log.Info().Str("call_id", callID).Msg("✅ UpdateRecording: Veritabanı güncellendi.")
	}
	return nil
}
