package storage_test

import (
	"testing"
	"time"

	"github.com/aliipou/observability-platform/internal/models"
	"github.com/aliipou/observability-platform/internal/storage"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func point(name string, value float64, labels map[string]string, t time.Time) models.MetricPoint {
	return models.MetricPoint{
		Name:      name,
		Type:      models.Gauge,
		Value:     value,
		Labels:    labels,
		Timestamp: t,
	}
}

func makeStore() *storage.TimeSeriesStore {
	return storage.NewTimeSeriesStore()
}

// ── Write + Query ─────────────────────────────────────────────────────────────

func TestWrite_SinglePoint(t *testing.T) {
	ts := makeStore()
	now := time.Now()
	ts.Write(point("cpu", 75.0, map[string]string{"host": "web-1"}, now))

	result := ts.Query("cpu", map[string]string{"host": "web-1"}, now.Add(-time.Minute), now.Add(time.Minute))
	if len(result) != 1 {
		t.Fatalf("expected 1 point, got %d", len(result))
	}
	if result[0].Value != 75.0 {
		t.Errorf("value: got %v, want 75.0", result[0].Value)
	}
}

func TestWrite_MultipleSeries(t *testing.T) {
	ts := makeStore()
	now := time.Now()

	ts.Write(point("cpu", 50.0, map[string]string{"host": "web-1"}, now))
	ts.Write(point("cpu", 80.0, map[string]string{"host": "web-2"}, now))
	ts.Write(point("mem", 60.0, map[string]string{"host": "web-1"}, now))

	// Query cpu for web-1 only
	result := ts.Query("cpu", map[string]string{"host": "web-1"}, now.Add(-time.Minute), now.Add(time.Minute))
	if len(result) != 1 || result[0].Value != 50.0 {
		t.Errorf("expected 1 cpu point for web-1 with value 50, got %v", result)
	}

	// Query all cpu
	resultAll := ts.Query("cpu", nil, now.Add(-time.Minute), now.Add(time.Minute))
	if len(resultAll) != 2 {
		t.Errorf("expected 2 cpu points total, got %d", len(resultAll))
	}
}

func TestQuery_TimeRange(t *testing.T) {
	ts := makeStore()
	base := time.Now().Truncate(time.Second)

	ts.Write(point("req", 1, nil, base.Add(-10*time.Minute)))
	ts.Write(point("req", 2, nil, base.Add(-5*time.Minute)))
	ts.Write(point("req", 3, nil, base))

	// Only last 6 minutes
	result := ts.Query("req", nil, base.Add(-6*time.Minute), base.Add(time.Minute))
	if len(result) != 2 {
		t.Errorf("expected 2 points in range, got %d", len(result))
	}
}

func TestQuery_EmptyResult(t *testing.T) {
	ts := makeStore()
	now := time.Now()
	result := ts.Query("nonexistent", nil, now.Add(-time.Hour), now)
	if result != nil && len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestQuery_LabelFilter(t *testing.T) {
	ts := makeStore()
	now := time.Now()

	ts.Write(point("http_req", 10, map[string]string{"service": "api", "env": "prod"}, now))
	ts.Write(point("http_req", 20, map[string]string{"service": "api", "env": "dev"}, now))
	ts.Write(point("http_req", 30, map[string]string{"service": "web", "env": "prod"}, now))

	result := ts.Query("http_req", map[string]string{"service": "api", "env": "prod"}, now.Add(-time.Minute), now.Add(time.Minute))
	if len(result) != 1 || result[0].Value != 10 {
		t.Errorf("expected 1 point with value 10, got %v", result)
	}
}

// ── WriteBatch ────────────────────────────────────────────────────────────────

func TestWriteBatch(t *testing.T) {
	ts := makeStore()
	now := time.Now()
	points := []models.MetricPoint{
		point("m1", 1, nil, now),
		point("m2", 2, nil, now),
		point("m3", 3, nil, now),
	}
	ts.WriteBatch(points)

	for _, name := range []string{"m1", "m2", "m3"} {
		r := ts.Query(name, nil, now.Add(-time.Minute), now.Add(time.Minute))
		if len(r) != 1 {
			t.Errorf("expected 1 point for %s, got %d", name, len(r))
		}
	}
}

// ── GetLatest ─────────────────────────────────────────────────────────────────

func TestGetLatest_Found(t *testing.T) {
	ts := makeStore()
	now := time.Now()
	labels := map[string]string{"host": "db-1"}

	ts.Write(point("cpu", 30, labels, now.Add(-2*time.Minute)))
	ts.Write(point("cpu", 50, labels, now.Add(-time.Minute)))
	ts.Write(point("cpu", 70, labels, now))

	latest, ok := ts.GetLatest("cpu", labels)
	if !ok {
		t.Fatal("expected to find latest point")
	}
	if latest.Value != 70 {
		t.Errorf("latest value: got %v, want 70", latest.Value)
	}
}

func TestGetLatest_NotFound(t *testing.T) {
	ts := makeStore()
	_, ok := ts.GetLatest("nonexistent", nil)
	if ok {
		t.Error("expected not found")
	}
}

// ── ListMetricNames ───────────────────────────────────────────────────────────

func TestListMetricNames(t *testing.T) {
	ts := makeStore()
	now := time.Now()
	ts.Write(point("alpha", 1, nil, now))
	ts.Write(point("beta", 2, nil, now))
	ts.Write(point("alpha", 3, nil, now)) // duplicate name, same series

	names := ts.ListMetricNames()
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["alpha"] || !nameSet["beta"] {
		t.Errorf("expected alpha and beta in names, got %v", names)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 unique names, got %d: %v", len(names), names)
	}
}

// ── ListLabels ────────────────────────────────────────────────────────────────

func TestListLabels(t *testing.T) {
	ts := makeStore()
	now := time.Now()
	ts.Write(point("cpu", 1, map[string]string{"host": "web-1", "region": "eu"}, now))
	ts.Write(point("cpu", 2, map[string]string{"host": "web-2", "region": "us"}, now))

	labels := ts.ListLabels("cpu")
	labelSet := make(map[string]bool)
	for _, l := range labels {
		labelSet[l] = true
	}
	if !labelSet["host"] || !labelSet["region"] {
		t.Errorf("expected host and region labels, got %v", labels)
	}
}

func TestListLabels_UnknownMetric(t *testing.T) {
	ts := makeStore()
	labels := ts.ListLabels("unknown")
	if len(labels) != 0 {
		t.Errorf("expected empty labels for unknown metric, got %v", labels)
	}
}

// ── ListServices ──────────────────────────────────────────────────────────────

func TestListServices(t *testing.T) {
	ts := makeStore()
	now := time.Now()
	ts.Write(point("req", 1, map[string]string{"service": "api"}, now))
	ts.Write(point("req", 2, map[string]string{"service": "worker"}, now))
	ts.Write(point("req", 3, map[string]string{"service": "api"}, now)) // duplicate

	services := ts.ListServices()
	svcSet := make(map[string]bool)
	for _, s := range services {
		svcSet[s] = true
	}
	if !svcSet["api"] || !svcSet["worker"] {
		t.Errorf("expected api and worker, got %v", services)
	}
	if len(services) != 2 {
		t.Errorf("expected 2 unique services, got %d", len(services))
	}
}

// ── EventsPerSecond ───────────────────────────────────────────────────────────

func TestEventsPerSecond_Empty(t *testing.T) {
	ts := makeStore()
	eps := ts.EventsPerSecond()
	if eps != 0 {
		t.Errorf("expected 0 eps for empty store, got %v", eps)
	}
}

func TestEventsPerSecond_RecentPoints(t *testing.T) {
	ts := makeStore()
	now := time.Now()
	// Add 60 points within the last minute → ~1 eps
	for i := 0; i < 60; i++ {
		ts.Write(point("m", float64(i), nil, now.Add(-time.Duration(i)*time.Second)))
	}
	eps := ts.EventsPerSecond()
	if eps <= 0 {
		t.Errorf("expected positive eps, got %v", eps)
	}
}

func TestEventsPerSecond_OldPoints(t *testing.T) {
	ts := makeStore()
	// Points from 2 hours ago — should not count
	old := time.Now().Add(-2 * time.Hour)
	for i := 0; i < 100; i++ {
		ts.Write(point("m", float64(i), nil, old))
	}
	eps := ts.EventsPerSecond()
	if eps != 0 {
		t.Errorf("expected 0 eps for old points, got %v", eps)
	}
}

// ── QuerySeries ───────────────────────────────────────────────────────────────

func TestQuerySeries(t *testing.T) {
	ts := makeStore()
	now := time.Now()
	labels1 := map[string]string{"host": "a"}
	labels2 := map[string]string{"host": "b"}

	ts.Write(point("cpu", 10, labels1, now.Add(-2*time.Minute)))
	ts.Write(point("cpu", 20, labels1, now.Add(-time.Minute)))
	ts.Write(point("cpu", 30, labels2, now))

	series := ts.QuerySeries("cpu", nil, now.Add(-10*time.Minute), now.Add(time.Minute))
	if len(series) != 2 {
		t.Errorf("expected 2 series, got %d", len(series))
	}
}

func TestQuerySeries_WithFilter(t *testing.T) {
	ts := makeStore()
	now := time.Now()
	ts.Write(point("cpu", 10, map[string]string{"host": "a"}, now))
	ts.Write(point("cpu", 20, map[string]string{"host": "b"}, now))

	series := ts.QuerySeries("cpu", map[string]string{"host": "a"}, now.Add(-time.Minute), now.Add(time.Minute))
	if len(series) != 1 {
		t.Errorf("expected 1 series, got %d", len(series))
	}
}

// ── Ring buffer overflow ──────────────────────────────────────────────────────

func TestRingBuffer_OverflowRetainsLatest(t *testing.T) {
	ts := makeStore()
	base := time.Now().Add(-2 * time.Hour)

	// Write more than RingSize (5760) points
	const extra = 100
	total := storage.RingSize + extra
	for i := 0; i < total; i++ {
		ts.Write(point("ring", float64(i), nil, base.Add(time.Duration(i)*time.Second)))
	}

	// Latest value should be total-1
	latest, ok := ts.GetLatest("ring", nil)
	if !ok {
		t.Fatal("expected to find latest point")
	}
	if latest.Value != float64(total-1) {
		t.Errorf("latest value: got %v, want %v", latest.Value, float64(total-1))
	}
}

// ── Cleanup ───────────────────────────────────────────────────────────────────

func TestCleanup_RemovesOldData(t *testing.T) {
	ts := makeStore()

	// Add old data (beyond MaxAge)
	old := time.Now().Add(-25 * time.Hour)
	ts.Write(point("old_metric", 1, nil, old))

	ts.Cleanup()

	// Old series should be removed
	names := ts.ListMetricNames()
	for _, n := range names {
		if n == "old_metric" {
			t.Error("old_metric should have been cleaned up")
		}
	}
}

func TestCleanup_KeepsRecentData(t *testing.T) {
	ts := makeStore()
	now := time.Now()
	ts.Write(point("recent", 42, nil, now))
	ts.Cleanup()

	result := ts.Query("recent", nil, now.Add(-time.Minute), now.Add(time.Minute))
	if len(result) != 1 {
		t.Errorf("recent data should survive cleanup, got %d points", len(result))
	}
}

// ── Concurrency ───────────────────────────────────────────────────────────────

func TestConcurrentWrites(t *testing.T) {
	ts := makeStore()
	now := time.Now()
	done := make(chan struct{})

	for g := 0; g < 10; g++ {
		go func(id int) {
			for i := 0; i < 100; i++ {
				ts.Write(point("concurrent", float64(i), map[string]string{"goroutine": string(rune('0' + id))}, now))
			}
			done <- struct{}{}
		}(g)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
	// Should not panic or deadlock
}

func TestConcurrentReadWrite(t *testing.T) {
	ts := makeStore()
	now := time.Now()
	done := make(chan struct{}, 2)

	go func() {
		for i := 0; i < 500; i++ {
			ts.Write(point("m", float64(i), nil, now.Add(time.Duration(i)*time.Millisecond)))
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 500; i++ {
			ts.Query("m", nil, now.Add(-time.Hour), now.Add(time.Hour))
		}
		done <- struct{}{}
	}()

	<-done
	<-done
	// No race conditions
}
