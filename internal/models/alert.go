package models

import "time"

// AlertSeverity represents the severity of an alert.
type AlertSeverity string

const (
	SeverityCritical AlertSeverity = "critical"
	SeverityWarning  AlertSeverity = "warning"
	SeverityInfo     AlertSeverity = "info"
)

// AlertState represents the current state of an alert.
type AlertState string

const (
	AlertFiring   AlertState = "firing"
	AlertResolved AlertState = "resolved"
)

// AlertRule defines a condition that triggers an alert.
type AlertRule struct {
	Name      string        `json:"name" yaml:"name"`
	Query     string        `json:"query" yaml:"query"`
	Condition string        `json:"condition" yaml:"condition"`
	Severity  AlertSeverity `json:"severity" yaml:"severity"`
	Message   string        `json:"message" yaml:"message"`
}

// AlertRulesConfig is the top-level YAML structure for alert rules.
type AlertRulesConfig struct {
	Rules []AlertRule `yaml:"rules"`
}

// AlertEvent represents a fired or resolved alert instance.
type AlertEvent struct {
	ID         string            `json:"id"`
	RuleName   string            `json:"rule_name"`
	Severity   AlertSeverity     `json:"severity"`
	State      AlertState        `json:"state"`
	Message    string            `json:"message"`
	Labels     map[string]string `json:"labels,omitempty"`
	Value      float64           `json:"value"`
	FiredAt    time.Time         `json:"fired_at"`
	ResolvedAt *time.Time        `json:"resolved_at,omitempty"`
}

// DashboardOverview contains summary stats for the dashboard homepage.
type DashboardOverview struct {
	TotalServices  int     `json:"total_services"`
	ActiveAlerts   int     `json:"active_alerts"`
	EventsPerSec   float64 `json:"events_per_sec"`
	ErrorRate      float64 `json:"error_rate"`
	TotalMetrics   int     `json:"total_metrics"`
	TotalLogs      int64   `json:"total_logs"`
	TotalTraces    int64   `json:"total_traces"`
}
