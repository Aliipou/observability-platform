package models

import "time"

// LogLevel represents the severity level of a log entry.
type LogLevel string

const (
	DEBUG LogLevel = "debug"
	INFO  LogLevel = "info"
	WARN  LogLevel = "warn"
	ERROR LogLevel = "error"
	FATAL LogLevel = "fatal"
)

// LogEntry represents a single log entry from an application.
type LogEntry struct {
	ID        string                 `json:"id"`
	Service   string                 `json:"service"`
	Level     LogLevel               `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	TraceID   string                 `json:"trace_id,omitempty"`
	SpanID    string                 `json:"span_id,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// LogBatch is used for batch ingestion of log entries.
type LogBatch struct {
	Logs []LogEntry `json:"logs"`
}

// LogQuery represents filters for querying logs.
type LogQuery struct {
	Service string   `form:"service"`
	Level   LogLevel `form:"level"`
	Search  string   `form:"q"`
	TraceID string   `form:"trace_id"`
	From    string   `form:"from"`
	To      string   `form:"to"`
	Limit   int      `form:"limit"`
}

// ValidLevel checks if a LogLevel is valid.
func ValidLevel(l LogLevel) bool {
	switch l {
	case DEBUG, INFO, WARN, ERROR, FATAL:
		return true
	default:
		return false
	}
}
