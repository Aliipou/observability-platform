package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/aliipou/observability-platform/internal/models"
)

func TestSpan_JSONRoundTrip(t *testing.T) {
	span := models.Span{
		TraceID:   "trace-abc",
		SpanID:    "span-001",
		ParentID:  "span-000",
		Service:   "checkout",
		Operation: "process_payment",
		StartTime: time.Now().Truncate(time.Millisecond),
		Duration:  45 * time.Millisecond,
		Status:    "ok",
		Tags:      map[string]string{"user_id": "42"},
	}

	data, err := json.Marshal(span)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded models.Span
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.TraceID != span.TraceID {
		t.Errorf("TraceID: got %q, want %q", decoded.TraceID, span.TraceID)
	}
	if decoded.Service != span.Service {
		t.Errorf("Service: got %q, want %q", decoded.Service, span.Service)
	}
	if decoded.Status != "ok" {
		t.Errorf("Status: got %q, want ok", decoded.Status)
	}
}

func TestSpan_ErrorStatus(t *testing.T) {
	span := models.Span{
		TraceID:   "t1",
		SpanID:    "s1",
		Service:   "api",
		Operation: "db_query",
		Status:    "error",
		Error:     "connection timeout",
	}
	if span.Status != "error" {
		t.Errorf("expected error status, got %q", span.Status)
	}
	if span.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestSpanBatch_MultipleSpans(t *testing.T) {
	batch := models.SpanBatch{
		Spans: []models.Span{
			{TraceID: "t1", SpanID: "s1", Service: "api", Operation: "get", Status: "ok"},
			{TraceID: "t1", SpanID: "s2", Service: "db", Operation: "query", Status: "ok"},
		},
	}
	if len(batch.Spans) != 2 {
		t.Errorf("expected 2 spans, got %d", len(batch.Spans))
	}
}

func TestTrace_Grouping(t *testing.T) {
	trace := models.Trace{
		TraceID: "trace-xyz",
		Spans: []models.Span{
			{TraceID: "trace-xyz", SpanID: "root", Service: "gateway"},
			{TraceID: "trace-xyz", SpanID: "child", ParentID: "root", Service: "auth"},
		},
	}
	if trace.TraceID != trace.Spans[0].TraceID {
		t.Error("trace ID should match span trace ID")
	}
	if trace.Spans[1].ParentID != "root" {
		t.Errorf("child span should have parent, got %q", trace.Spans[1].ParentID)
	}
}

func TestTraceQuery_ZeroValue(t *testing.T) {
	q := models.TraceQuery{}
	if q.Limit != 0 || q.Service != "" || q.MinDurMs != 0 {
		t.Error("zero-value TraceQuery should have zero fields")
	}
}
