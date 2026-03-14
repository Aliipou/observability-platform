package storage

import (
	"sync"
	"time"

	"github.com/aliipou/observability-platform/internal/models"
)

const (
	// MaxAge is how long data is kept in the ring buffer (24 hours).
	MaxAge = 24 * time.Hour
	// DownsampleAfter is the threshold after which data is downsampled to 1-minute resolution.
	DownsampleAfter = 1 * time.Hour
	// RingSize is the maximum number of points per series (24h at 15s = 5760).
	RingSize = 5760
)

// series holds a ring buffer of metric points for a single time series.
type series struct {
	Name   string
	Labels map[string]string
	Points []models.MetricPoint
	head   int
	count  int
}

// TimeSeriesStore is an in-memory store for metric time series data.
type TimeSeriesStore struct {
	mu       sync.RWMutex
	seriesDB map[string]*series // key = SeriesKey(name, labels)
	names    map[string]bool    // set of all metric names
	services map[string]bool    // set of all services
}

// NewTimeSeriesStore creates a new in-memory time series store.
func NewTimeSeriesStore() *TimeSeriesStore {
	return &TimeSeriesStore{
		seriesDB: make(map[string]*series),
		names:    make(map[string]bool),
		services: make(map[string]bool),
	}
}

// Write adds a metric point to the store.
func (ts *TimeSeriesStore) Write(point models.MetricPoint) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	key := models.SeriesKey(point.Name, point.Labels)
	s, ok := ts.seriesDB[key]
	if !ok {
		s = &series{
			Name:   point.Name,
			Labels: point.Labels,
			Points: make([]models.MetricPoint, RingSize),
			head:   0,
			count:  0,
		}
		ts.seriesDB[key] = s
	}

	// Write to ring buffer
	s.Points[s.head] = point
	s.head = (s.head + 1) % RingSize
	if s.count < RingSize {
		s.count++
	}

	ts.names[point.Name] = true
	if svc, ok := point.Labels["service"]; ok {
		ts.services[svc] = true
	}
}

// WriteBatch writes multiple metric points to the store.
func (ts *TimeSeriesStore) WriteBatch(points []models.MetricPoint) {
	for _, p := range points {
		ts.Write(p)
	}
}

// Query returns metric points matching the given name, labels, and time range.
func (ts *TimeSeriesStore) Query(name string, labels map[string]string, from, to time.Time) []models.MetricPoint {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	var result []models.MetricPoint

	for _, s := range ts.seriesDB {
		if s.Name != name {
			continue
		}
		if !models.MatchLabels(s.Labels, labels) {
			continue
		}

		points := ts.getOrderedPoints(s)
		for _, p := range points {
			if !p.Timestamp.Before(from) && !p.Timestamp.After(to) {
				result = append(result, p)
			}
		}
	}

	// Downsample old data to 1-minute resolution
	result = ts.downsample(result, from)

	return result
}

// QuerySeries returns all matching series with their points in a time range.
func (ts *TimeSeriesStore) QuerySeries(name string, labels map[string]string, from, to time.Time) []models.MetricSeries {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	var result []models.MetricSeries

	for _, s := range ts.seriesDB {
		if s.Name != name {
			continue
		}
		if !models.MatchLabels(s.Labels, labels) {
			continue
		}

		var points []models.MetricPoint
		ordered := ts.getOrderedPoints(s)
		for _, p := range ordered {
			if !p.Timestamp.Before(from) && !p.Timestamp.After(to) {
				points = append(points, p)
			}
		}

		if len(points) > 0 {
			result = append(result, models.MetricSeries{
				Name:   s.Name,
				Labels: s.Labels,
				Points: ts.downsample(points, from),
			})
		}
	}

	return result
}

// GetLatest returns the most recent point for a metric name and label set.
func (ts *TimeSeriesStore) GetLatest(name string, labels map[string]string) (models.MetricPoint, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	key := models.SeriesKey(name, labels)
	s, ok := ts.seriesDB[key]
	if !ok || s.count == 0 {
		return models.MetricPoint{}, false
	}

	idx := (s.head - 1 + RingSize) % RingSize
	return s.Points[idx], true
}

// ListMetricNames returns all known metric names.
func (ts *TimeSeriesStore) ListMetricNames() []string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	names := make([]string, 0, len(ts.names))
	for name := range ts.names {
		names = append(names, name)
	}
	return names
}

// ListLabels returns all unique label keys for a given metric name.
func (ts *TimeSeriesStore) ListLabels(name string) []string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	labelSet := make(map[string]bool)
	for _, s := range ts.seriesDB {
		if s.Name == name {
			for k := range s.Labels {
				labelSet[k] = true
			}
		}
	}

	labels := make([]string, 0, len(labelSet))
	for k := range labelSet {
		labels = append(labels, k)
	}
	return labels
}

// ListSeries returns all series matching a metric name and optional label filter.
func (ts *TimeSeriesStore) ListSeries(name string, labels map[string]string) []models.MetricSeries {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	var result []models.MetricSeries
	for _, s := range ts.seriesDB {
		if s.Name != name {
			continue
		}
		if !models.MatchLabels(s.Labels, labels) {
			continue
		}
		result = append(result, models.MetricSeries{
			Name:   s.Name,
			Labels: s.Labels,
		})
	}
	return result
}

// ListServices returns all known service names from labels.
func (ts *TimeSeriesStore) ListServices() []string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	services := make([]string, 0, len(ts.services))
	for svc := range ts.services {
		services = append(services, svc)
	}
	return services
}

// EventsPerSecond estimates the current ingestion rate over the last minute.
func (ts *TimeSeriesStore) EventsPerSecond() float64 {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	now := time.Now()
	oneMinAgo := now.Add(-1 * time.Minute)
	count := 0

	for _, s := range ts.seriesDB {
		points := ts.getOrderedPoints(s)
		for _, p := range points {
			if p.Timestamp.After(oneMinAgo) {
				count++
			}
		}
	}

	if count == 0 {
		return 0
	}
	return float64(count) / 60.0
}

// Cleanup removes data points older than MaxAge.
func (ts *TimeSeriesStore) Cleanup() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	cutoff := time.Now().Add(-MaxAge)
	for key, s := range ts.seriesDB {
		hasValid := false
		for i := 0; i < s.count; i++ {
			idx := (s.head - s.count + i + RingSize) % RingSize
			if s.Points[idx].Timestamp.After(cutoff) {
				hasValid = true
				break
			}
		}
		if !hasValid {
			delete(ts.seriesDB, key)
			delete(ts.names, s.Name)
		}
	}
}

// getOrderedPoints returns points in chronological order from a ring buffer.
func (ts *TimeSeriesStore) getOrderedPoints(s *series) []models.MetricPoint {
	result := make([]models.MetricPoint, 0, s.count)
	for i := 0; i < s.count; i++ {
		idx := (s.head - s.count + i + RingSize) % RingSize
		if !s.Points[idx].Timestamp.IsZero() {
			result = append(result, s.Points[idx])
		}
	}
	return result
}

// downsample reduces resolution of old data to 1-minute intervals.
func (ts *TimeSeriesStore) downsample(points []models.MetricPoint, from time.Time) []models.MetricPoint {
	if len(points) == 0 {
		return points
	}

	boundary := time.Now().Add(-DownsampleAfter)
	var recent, old []models.MetricPoint

	for _, p := range points {
		if p.Timestamp.After(boundary) {
			recent = append(recent, p)
		} else {
			old = append(old, p)
		}
	}

	if len(old) == 0 {
		return recent
	}

	// Group old data into 1-minute buckets and average
	buckets := make(map[int64][]models.MetricPoint)
	for _, p := range old {
		bucket := p.Timestamp.Truncate(time.Minute).Unix()
		buckets[bucket] = append(buckets[bucket], p)
	}

	var downsampled []models.MetricPoint
	for _, bucket := range buckets {
		if len(bucket) == 0 {
			continue
		}
		sum := 0.0
		for _, p := range bucket {
			sum += p.Value
		}
		avg := sum / float64(len(bucket))
		downsampled = append(downsampled, models.MetricPoint{
			Name:      bucket[0].Name,
			Type:      bucket[0].Type,
			Value:     avg,
			Labels:    bucket[0].Labels,
			Timestamp: bucket[0].Timestamp.Truncate(time.Minute),
		})
	}

	result := append(downsampled, recent...)

	// Sort by timestamp
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Timestamp.After(result[j].Timestamp) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}
