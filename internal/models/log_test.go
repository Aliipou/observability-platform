package models_test

import (
	"testing"

	"github.com/aliipou/observability-platform/internal/models"
)

func TestLogLevelConstants(t *testing.T) {
	levels := []models.LogLevel{
		models.DEBUG, models.INFO, models.WARN, models.ERROR, models.FATAL,
	}
	seen := make(map[models.LogLevel]bool)
	for _, l := range levels {
		if l == "" {
			t.Error("log level should not be empty string")
		}
		if seen[l] {
			t.Errorf("duplicate log level: %q", l)
		}
		seen[l] = true
	}
}

func TestValidLevel_ValidLevels(t *testing.T) {
	valid := []models.LogLevel{
		models.DEBUG, models.INFO, models.WARN, models.ERROR, models.FATAL,
	}
	for _, l := range valid {
		if !models.ValidLevel(l) {
			t.Errorf("expected %q to be valid", l)
		}
	}
}

func TestValidLevel_InvalidLevels(t *testing.T) {
	invalid := []models.LogLevel{
		"", "trace", "CRITICAL", "verbose", "WARNING",
	}
	for _, l := range invalid {
		if models.ValidLevel(l) {
			t.Errorf("expected %q to be invalid", l)
		}
	}
}

func TestLogQuery_ZeroValue(t *testing.T) {
	q := models.LogQuery{}
	if q.Limit != 0 {
		t.Error("zero-value limit should be 0")
	}
	if q.Level != "" {
		t.Error("zero-value level should be empty")
	}
}

func TestLogBatch_MultipleEntries(t *testing.T) {
	batch := models.LogBatch{
		Logs: []models.LogEntry{
			{Service: "api", Level: models.INFO, Message: "started"},
			{Service: "api", Level: models.ERROR, Message: "failed"},
		},
	}
	if len(batch.Logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(batch.Logs))
	}
}
