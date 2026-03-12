package models

import "time"

// Span represents a single unit of work within a distributed trace.
type Span struct {
	TraceID   string            `json:"trace_id"`
	SpanID    string            `json:"span_id"`
	ParentID  string            `json:"parent_id,omitempty"`
	Service   string            `json:"service"`
	Operation string            `json:"operation"`
	StartTime time.Time         `json:"start_time"`
	Duration  time.Duration     `json:"duration"`
	Status    string            `json:"status"` // "ok" or "error"
	Tags      map[string]string `json:"tags,omitempty"`
	Error     string            `json:"error,omitempty"`
}

// SpanBatch is used for batch ingestion of spans.
type SpanBatch struct {
	Spans []Span `json:"spans"`
}

// Trace represents a collection of spans that form a complete trace.
type Trace struct {
	TraceID string `json:"trace_id"`
	Spans   []Span `json:"spans"`
}

// TraceQuery represents filters for querying traces.
type TraceQuery struct {
	Service   string `form:"service"`
	Operation string `form:"operation"`
	From      string `form:"from"`
	To        string `form:"to"`
	Limit     int    `form:"limit"`
	MinDurMs  int    `form:"min_duration_ms"`
}
