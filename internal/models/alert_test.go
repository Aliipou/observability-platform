package models_test

import (
	"testing"

	"github.com/aliipou/observability-platform/internal/models"
)

func TestAlertSeverityConstants(t *testing.T) {
	severities := []models.AlertSeverity{
		models.SeverityCritical, models.SeverityWarning, models.SeverityInfo,
	}
	seen := make(map[models.AlertSeverity]bool)
	for _, s := range severities {
		if s == "" {
			t.Error("severity should not be empty")
		}
		if seen[s] {
			t.Errorf("duplicate severity: %q", s)
		}
		seen[s] = true
	}
}

func TestAlertStateConstants(t *testing.T) {
	states := []models.AlertState{models.AlertFiring, models.AlertResolved}
	if models.AlertFiring == models.AlertResolved {
		t.Error("Firing and Resolved should be distinct")
	}
	for _, s := range states {
		if s == "" {
			t.Error("state should not be empty")
		}
	}
}

func TestAlertRule_Fields(t *testing.T) {
	rule := models.AlertRule{
		Name:      "high-cpu",
		Query:     "avg(cpu_usage{}, 5m) > 90",
		Condition: "> 90",
		Severity:  models.SeverityCritical,
		Message:   "CPU usage critically high",
	}
	if rule.Name == "" || rule.Query == "" || rule.Severity == "" {
		t.Error("AlertRule fields should not be empty")
	}
}

func TestAlertEvent_ResolvedAt_Optional(t *testing.T) {
	event := models.AlertEvent{
		ID:       "ev-1",
		RuleName: "mem-high",
		Severity: models.SeverityWarning,
		State:    models.AlertFiring,
	}
	if event.ResolvedAt != nil {
		t.Error("firing event should have nil ResolvedAt")
	}
}

func TestDashboardOverview_ZeroValue(t *testing.T) {
	d := models.DashboardOverview{}
	if d.TotalServices != 0 || d.ActiveAlerts != 0 || d.ErrorRate != 0 {
		t.Error("zero-value DashboardOverview should have zero fields")
	}
}
