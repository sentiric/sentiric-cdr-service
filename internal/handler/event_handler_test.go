package handler

import (
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestHandleEvent_Success(t *testing.T) {
	// --- Kurulum (Arrange) ---
	db, mock, err := sqlmock.New() // sqlmock ile sahte DB bağlantısı oluştur
	assert.NoError(t, err)
	defer db.Close()

	log := zerolog.New(io.Discard)

	processed := prometheus.NewCounterVec(prometheus.CounterOpts{}, []string{"event_type"})
	failed := prometheus.NewCounterVec(prometheus.CounterOpts{}, []string{"event_type", "reason"})

	handler := NewEventHandler(db, log, processed, failed)

	// Gelen olay
	eventPayload := `{"from":"123","to":"456"}`
	body := []byte(`{"eventType":"call.started","callId":"call-123","payload":` + eventPayload + `}`)

	// Mock beklentisini ayarla: Belirli bir SQL sorgusunun çalıştırılmasını bekle.
	// sqlmock, sorguları normal ifade (regex) ile eşleştirir.
	expectedSQL := "INSERT INTO call_events \\(call_id, event_type, event_timestamp, payload\\) VALUES \\(\\$1, \\$2, \\$3, \\$4\\)"
	mock.ExpectExec(regexp.QuoteMeta(expectedSQL)).
		WithArgs("call-123", "call.started", sqlmock.AnyArg(), json.RawMessage(eventPayload)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// --- Eylem (Act) ---
	handler.HandleEvent(body)

	// --- Doğrulama (Assert) ---
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.Equal(t, float64(1), testutil.ToFloat64(processed.WithLabelValues("call.started")))
}

func TestHandleEvent_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	log := zerolog.New(io.Discard)
	processed := prometheus.NewCounterVec(prometheus.CounterOpts{}, []string{"event_type"})
	failed := prometheus.NewCounterVec(prometheus.CounterOpts{}, []string{"event_type", "reason"})
	handler := NewEventHandler(db, log, processed, failed)

	body := []byte(`{"eventType":"call.ended","callId":"call-456","payload":{}}`)
	dbError := errors.New("database connection lost")

	// Beklenti: Exec çağrıldığında bir hata dönecek.
	mock.ExpectExec("INSERT INTO call_events").WillReturnError(dbError)

	handler.HandleEvent(body)

	assert.NoError(t, mock.ExpectationsWereMet())
	assert.Equal(t, float64(1), testutil.ToFloat64(processed.WithLabelValues("call.ended")))
	assert.Equal(t, float64(1), testutil.ToFloat64(failed.WithLabelValues("call.ended", "db_insert_failed")))
}

func TestHandleEvent_JsonError(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	log := zerolog.New(io.Discard)
	processed := prometheus.NewCounterVec(prometheus.CounterOpts{}, []string{"event_type"})
	failed := prometheus.NewCounterVec(prometheus.CounterOpts{}, []string{"event_type", "reason"})
	handler := NewEventHandler(db, log, processed, failed)

	body := []byte(`{"eventType": "bad"`)

	handler.HandleEvent(body)

	// DB'nin hiç çağrılmadığını doğrula
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.Equal(t, float64(1), testutil.ToFloat64(failed.WithLabelValues("unknown", "json_unmarshal")))
}
