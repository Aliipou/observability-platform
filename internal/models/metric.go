package models

import "time"

// MetricType represents the type of metric being collected.
type MetricType string

const (
	Counter   MetricType = "counter"
	Gauge     MetricType = "gauge"
	Histogram MetricType = "histogram"
)

// MetricPoint represents a single data point for a metric.
type MetricPoint struct {
	Name      string            `json:"name"`
	Type      MetricType        `json:"type"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels"`
	Timestamp time.Time         `json:"timestamp"`
}

// MetricSeries represents a time series of metric data points sharing the same name and labels.
type MetricSeries struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
	Points []MetricPoint     `json:"points"`
}

// MetricBatch is used for batch ingestion of metric points.
type MetricBatch struct {
	Metrics []MetricPoint `json:"metrics"`
}

// LabelKey generates a canonical string key from a label set for indexing.
func LabelKey(labels map[string]string) string {
	if len(labels) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	// Sort keys for consistent ordering
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	result := "{"
	for i, k := range keys {
		if i > 0 {
			result += ","
		}
		result += k + "=\"" + labels[k] + "\""
	}
	result += "}"
	return result
}

// SeriesKey generates a unique key for a metric series (name + labels).
func SeriesKey(name string, labels map[string]string) string {
	return name + LabelKey(labels)
}

// MatchLabels checks if the given labels match the filter.
// An empty filter matches everything.
func MatchLabels(labels, filter map[string]string) bool {
	for k, v := range filter {
		if labels[k] != v {
			return false
		}
	}
	return true
}
