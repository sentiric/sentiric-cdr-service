// File: internal/logger/events.go
// [ARCH-COMPLIANCE] Eksik event log ID'leri SUTS v4.0 zorunluluğu için eklendi.
package logger

// SUTS v4.0 Standard Event IDs
const (
	EventSystemStartup       = "SYSTEM_STARTUP"
	EventInfraReady          = "INFRASTRUCTURE_READY"
	EventShutdown            = "SYSTEM_SHUTDOWN"
	EventMessageReceived     = "MESSAGE_RECEIVED"
	EventCdrProcessed        = "CDR_PROCESSED"
	EventCdrIgnored          = "CDR_IGNORED"
	EventDbWriteFail         = "DB_WRITE_FAILURE"
	EventDbReadFail          = "DB_READ_FAILURE"
	EventRawLogFail          = "RAW_EVENT_LOG_FAILURE"
	EventRawLogSuccess       = "RAW_EVENT_LOG_SUCCESS"
	EventBillingFail         = "BILLING_RECORD_FAILURE"
	EventBillingSuccess      = "BILLING_RECORD_SUCCESS"
	EventRecordingFail       = "RECORDING_UPDATE_FAILURE"
	EventRecordingSuccess    = "RECORDING_UPDATE_SUCCESS"
	EventRabbitMQChannelDrop = "RABBITMQ_CHANNEL_DROPPED"
	EventRabbitMQRetry       = "RABBITMQ_MESSAGE_RETRY"
	EventRabbitMQFail        = "RABBITMQ_INFRA_FAILURE"
)
