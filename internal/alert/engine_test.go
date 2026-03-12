package alert_test

import (
	"context"
	"testing"
	"time"

	"github.com/aliipou/observability-platform/internal/alert"
	"github.com/aliipou/observability-platform/internal/models"
	"github.com/aliipou/observability-platform/internal/storage"
	"go.uber.org/zap"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

func newEngine() (*alert.Engine, *storage.TimeSeriesStore) {
	ts := storage.NewTimeSeriesStore()
	eng := alert.New(ts, nil, zap.NewNop())
	return eng, ts
}

func writeMetric(ts *storage.TimeSeriesStore, name string, value float64, labels map[string]string) {
	ts.Write(models.MetricPoint{
		Name:      name,
		Type:      models.Gauge,
		Value:     value,
		Labels:    labels,
		Timestamp: time.Now(),
	})
}

func runFor(t *testing.T, eng *alert.Engine, dur time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), dur)
	t.Cleanup(cancel)
	go eng.Run(ctx, 50*time.Millisecond)
	time.Sleep(dur)
}

// ── SetRules / GetRules ───────────────────────────────────────────────────────

func TestEngine_SetGetRules(t *testing.T) {
	eng, _ := newEngine()
	rules := []models.AlertRule{
		{Name: "r1", Query: "avg(cpu{}, 1m) > 80", Severity: models.SeverityWarning},
		{Name: "r2", Query: "sum(errors{}, 5m) > 100", Severity: models.SeverityCritical},
	}
	eng.SetRules(rules)

	got := eng.GetRules()
	if len(got) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(got))
	}
	if got[0].Name != "r1" || got[1].Name != "r2" {
		t.Errorf("rules not preserved: %+v", got)
	}
}

func TestEngine_EmptyRulesInitially(t *testing.T) {
	eng, _ := newEngine()
	if len(eng.GetRules()) != 0 {
		t.Error("expected empty rules initially")
	}
}

// ── Alert firing ──────────────────────────────────────────────────────────────

func TestEngine_FiresAlertWhenThresholdExceeded(t *testing.T) {
	eng, ts := newEngine()
	eng.SetRules([]models.AlertRule{
		{Name: "high-cpu", Query: "avg(cpu_pct{}, 5m) > 80", Severity: models.SeverityCritical, Message: "CPU critical"},
	})

	for i := 0; i < 10; i++ {
		writeMetric(ts, "cpu_pct", 95.0, nil)
	}

	runFor(t, eng, 200*time.Millisecond)

	firing := eng.GetFiringAlerts()
	if len(firing) == 0 {
		t.Error("expected alert to be firing")
		return
	}
	if firing[0].RuleName != "high-cpu" {
		t.Errorf("wrong rule: got %q", firing[0].RuleName)
	}
	if firing[0].State != models.AlertFiring {
		t.Errorf("expected firing state, got %q", firing[0].State)
	}
	if firing[0].Severity != models.SeverityCritical {
		t.Errorf("expected critical severity, got %q", firing[0].Severity)
	}
}

func TestEngine_NoAlertBelowThreshold(t *testing.T) {
	eng, ts := newEngine()
	eng.SetRules([]models.AlertRule{
		{Name: "high-cpu", Query: "avg(cpu_pct{}, 5m) > 80", Severity: models.SeverityWarning},
	})

	for i := 0; i < 10; i++ {
		writeMetric(ts, "cpu_pct", 50.0, nil)
	}

	runFor(t, eng, 200*time.Millisecond)

	if len(eng.GetFiringAlerts()) != 0 {
		t.Error("expected no firing alerts below threshold")
	}
}

func TestEngine_MultipleRulesIndependent(t *testing.T) {
	eng, ts := newEngine()
	eng.SetRules([]models.AlertRule{
		{Name: "cpu-high", Query: "avg(cpu{}, 5m) > 80", Severity: models.SeverityCritical},
		{Name: "mem-high", Query: "avg(mem{}, 5m) > 90", Severity: models.SeverityWarning},
	})

	for i := 0; i < 10; i++ {
		writeMetric(ts, "cpu", 90.0, nil)
		writeMetric(ts, "mem", 60.0, nil)
	}

	runFor(t, eng, 200*time.Millisecond)

	firingNames := make(map[string]bool)
	for _, f := range eng.GetFiringAlerts() {
		firingNames[f.RuleName] = true
	}

	if !firingNames["cpu-high"] {
		t.Error("expected cpu-high to be firing")
	}
	if firingNames["mem-high"] {
		t.Error("expected mem-high to NOT be firing")
	}
}

// ── GetFiringAlerts when no alerts ───────────────────────────────────────────

func TestEngine_GetFiringAlerts_Empty(t *testing.T) {
	eng, _ := newEngine()
	if len(eng.GetFiringAlerts()) != 0 {
		t.Error("expected no firing alerts initially")
	}
}

// ── GetAlertHistory ───────────────────────────────────────────────────────────

func TestEngine_GetAlertHistory_InitiallyEmpty(t *testing.T) {
	eng, _ := newEngine()
	if len(eng.GetAlertHistory(100)) != 0 {
		t.Error("expected empty history initially")
	}
}

func TestEngine_GetAlertHistory_AfterFire(t *testing.T) {
	eng, ts := newEngine()
	eng.SetRules([]models.AlertRule{
		{Name: "test-rule", Query: "avg(x{}, 5m) > 1", Severity: models.SeverityInfo},
	})

	for i := 0; i < 5; i++ {
		writeMetric(ts, "x", 5.0, nil)
	}

	runFor(t, eng, 200*time.Millisecond)

	history := eng.GetAlertHistory(100)
	if len(history) == 0 {
		t.Error("expected at least one history entry after alert fires")
	}
	if history[0].RuleName != "test-rule" {
		t.Errorf("expected test-rule in history, got %q", history[0].RuleName)
	}
}

func TestEngine_GetAlertHistory_Limit(t *testing.T) {
	eng, _ := newEngine()
	history := eng.GetAlertHistory(5)
	if len(history) > 5 {
		t.Errorf("history should respect limit, got %d", len(history))
	}
}

// ── Condition types ───────────────────────────────────────────────────────────

func TestEngine_LessThanCondition_NoData(t *testing.T) {
	eng, _ := newEngine()
	eng.SetRules([]models.AlertRule{
		{Name: "low-rate", Query: "count(req{}, 5m) < 1", Severity: models.SeverityWarning},
	})

	// No metrics → count = 0 → triggers "< 1"
	runFor(t, eng, 200*time.Millisecond)

	if len(eng.GetFiringAlerts()) == 0 {
		t.Error("expected low-rate to fire when no data (count < 1)")
	}
}

func TestEngine_NoAlertWithNoCondition(t *testing.T) {
	eng, ts := newEngine()
	eng.SetRules([]models.AlertRule{
		{Name: "no-cond", Query: "avg(cpu{}, 5m)", Severity: models.SeverityInfo},
	})
	writeMetric(ts, "cpu", 100.0, nil)

	runFor(t, eng, 200*time.Millisecond)

	if len(eng.GetFiringAlerts()) != 0 {
		t.Error("query without condition should never fire")
	}
}

// ── Graceful stop ─────────────────────────────────────────────────────────────

func TestEngine_StopsGracefully(t *testing.T) {
	eng, _ := newEngine()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		eng.Run(ctx, 10*time.Millisecond)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("engine did not stop within 1 second")
	}
}
