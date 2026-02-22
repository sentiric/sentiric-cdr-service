// sentiric-cdr-service/internal/logger/events.go
package logger

// SUTS v4.0 Standard Event IDs
const (
	EventSystemStartup = "SYSTEM_STARTUP"
	EventInfraReady    = "INFRASTRUCTURE_READY"
	EventShutdown      = "SYSTEM_SHUTDOWN"

	EventMessageReceived = "MQ_MESSAGE_RECEIVED"
	EventCdrProcessed    = "CDR_PROCESSED"
	EventCdrIgnored      = "CDR_IGNORED"
	EventCdrFailed       = "CDR_PROCESSING_FAILED"

	EventDbWriteSuccess = "DB_WRITE_SUCCESS"
	EventDbWriteFail    = "DB_WRITE_FAIL"
	EventRawLogSuccess  = "RAW_EVENT_LOGGED"
	EventRawLogFail     = "RAW_EVENT_LOG_FAIL"
)
