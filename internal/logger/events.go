// sentiric-cdr-service/internal/logger/events.go
package logger

// SUTS v4.0 Standard Event IDs
const (
	EventSystemStartup   = "SYSTEM_STARTUP"
	EventInfraReady      = "INFRASTRUCTURE_READY"
	EventShutdown        = "SYSTEM_SHUTDOWN"
	EventMessageReceived = "MESSAGE_RECEIVED"
	EventCdrProcessed    = "CDR_PROCESSED"
	EventCdrIgnored      = "CDR_IGNORED"
	EventDbWriteFail     = "DB_WRITE_FAILURE"
	EventRawLogFail      = "RAW_EVENT_LOG_FAILURE"
	EventRawLogSuccess   = "RAW_EVENT_LOG_SUCCESS"
)
