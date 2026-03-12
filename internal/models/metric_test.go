package models_test

import (
	"testing"

	"github.com/aliipou/observability-platform/internal/models"
)

// ── LabelKey ──────────────────────────────────────────────────────────────────

func TestLabelKey_Empty(t *testing.T) {
	key := models.LabelKey(nil)
	if key != "{}" {
		t.Errorf("empty labels: got %q, want {}", key)
	}
}

func TestLabelKey_Single(t *testing.T) {
	key := models.LabelKey(map[string]string{"host": "web-1"})
	if key != `{host="web-1"}` {
		t.Errorf("single label: got %q, want {host=\"web-1\"}", key)
	}
}

func TestLabelKey_Deterministic(t *testing.T) {
	labels := map[string]string{"b": "2", "a": "1", "c": "3"}
	k1 := models.LabelKey(labels)
	k2 := models.LabelKey(labels)
	if k1 != k2 {
		t.Errorf("LabelKey not deterministic: %q vs %q", k1, k2)
	}
}

func TestLabelKey_OrderIndependent(t *testing.T) {
	// Same labels in different insertion order should produce same key
	// (Go maps don't have insertion order, but we test with same content)
	l1 := map[string]string{"env": "prod", "service": "api"}
	l2 := map[string]string{"service": "api", "env": "prod"}
	if models.LabelKey(l1) != models.LabelKey(l2) {
		t.Errorf("label order should not affect key: %q vs %q", models.LabelKey(l1), models.LabelKey(l2))
	}
}

// ── SeriesKey ─────────────────────────────────────────────────────────────────

func TestSeriesKey_Unique(t *testing.T) {
	k1 := models.SeriesKey("cpu", map[string]string{"host": "a"})
	k2 := models.SeriesKey("cpu", map[string]string{"host": "b"})
	k3 := models.SeriesKey("mem", map[string]string{"host": "a"})

	if k1 == k2 {
		t.Error("different label values should produce different keys")
	}
	if k1 == k3 {
		t.Error("different metric names should produce different keys")
	}
}

func TestSeriesKey_Consistent(t *testing.T) {
	labels := map[string]string{"service": "checkout", "region": "eu-north-1"}
	k1 := models.SeriesKey("latency", labels)
	k2 := models.SeriesKey("latency", labels)
	if k1 != k2 {
		t.Errorf("SeriesKey not consistent: %q vs %q", k1, k2)
	}
}

// ── MatchLabels ───────────────────────────────────────────────────────────────

func TestMatchLabels_EmptyFilter(t *testing.T) {
	labels := map[string]string{"host": "web-1", "env": "prod"}
	if !models.MatchLabels(labels, nil) {
		t.Error("empty filter should match everything")
	}
	if !models.MatchLabels(labels, map[string]string{}) {
		t.Error("empty filter map should match everything")
	}
}

func TestMatchLabels_ExactMatch(t *testing.T) {
	labels := map[string]string{"host": "web-1", "env": "prod"}
	filter := map[string]string{"host": "web-1"}
	if !models.MatchLabels(labels, filter) {
		t.Error("subset filter should match")
	}
}

func TestMatchLabels_NoMatch(t *testing.T) {
	labels := map[string]string{"host": "web-1", "env": "prod"}
	filter := map[string]string{"host": "web-2"}
	if models.MatchLabels(labels, filter) {
		t.Error("different value should not match")
	}
}

func TestMatchLabels_MissingKey(t *testing.T) {
	labels := map[string]string{"host": "web-1"}
	filter := map[string]string{"env": "prod"}
	if models.MatchLabels(labels, filter) {
		t.Error("missing key should not match")
	}
}

func TestMatchLabels_MultipleFilters(t *testing.T) {
	labels := map[string]string{"host": "web-1", "env": "prod", "region": "eu"}
	if !models.MatchLabels(labels, map[string]string{"host": "web-1", "env": "prod"}) {
		t.Error("all matching filters should succeed")
	}
	if models.MatchLabels(labels, map[string]string{"host": "web-1", "env": "dev"}) {
		t.Error("one mismatching filter should fail")
	}
}

// ── MetricType constants ──────────────────────────────────────────────────────

func TestMetricTypeConstants(t *testing.T) {
	types := []models.MetricType{models.Counter, models.Gauge, models.Histogram}
	for _, mt := range types {
		if mt == "" {
			t.Error("metric type should not be empty")
		}
	}
	if models.Counter == models.Gauge {
		t.Error("Counter and Gauge should be distinct")
	}
}
