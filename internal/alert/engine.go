package alert

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/aliipou/observability-platform/internal/models"
	"github.com/aliipou/observability-platform/internal/query"
	"github.com/aliipou/observability-platform/internal/storage"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// MetricsReader is the subset of TimeSeriesStore needed by the alert engine.
type MetricsReader interface {
	Query(name string, labels map[string]string, from, to time.Time) []models.MetricPoint
}

// Engine evaluates alert rules against live metrics.
type Engine struct {
	mu      sync.RWMutex
	rules   []models.AlertRule
	firing  map[string]*models.AlertEvent // key = rule name
	history []models.AlertEvent
	ts      MetricsReader
	pg      *storage.PostgresStore
	log     *zap.Logger
	maxHist int
}

// New creates a new alert engine.
func New(ts MetricsReader, pg *storage.PostgresStore, log *zap.Logger) *Engine {
	return &Engine{
		ts:      ts,
		pg:      pg,
		log:     log,
		firing:  make(map[string]*models.AlertEvent),
		maxHist: 1000,
	}
}

// LoadRules reads alert rules from a YAML file.
func (e *Engine) LoadRules(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read rules file: %w", err)
	}
	var cfg models.AlertRulesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse rules: %w", err)
	}
	e.mu.Lock()
	e.rules = cfg.Rules
	e.mu.Unlock()
	e.log.Info("alert rules loaded", zap.Int("count", len(cfg.Rules)))
	return nil
}

// SetRules replaces the rule set (used in tests and API).
func (e *Engine) SetRules(rules []models.AlertRule) {
	e.mu.Lock()
	e.rules = rules
	e.mu.Unlock()
}

// GetRules returns the current rule set.
func (e *Engine) GetRules() []models.AlertRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]models.AlertRule, len(e.rules))
	copy(out, e.rules)
	return out
}

// GetFiringAlerts returns currently firing alerts.
func (e *Engine) GetFiringAlerts() []models.AlertEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]models.AlertEvent, 0, len(e.firing))
	for _, ev := range e.firing {
		out = append(out, *ev)
	}
	return out
}

// GetAlertHistory returns recent alert events.
func (e *Engine) GetAlertHistory(limit int) []models.AlertEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if limit <= 0 || limit > len(e.history) {
		limit = len(e.history)
	}
	// Return most recent first
	out := make([]models.AlertEvent, limit)
	for i, j := len(e.history)-1, 0; j < limit; i, j = i-1, j+1 {
		out[j] = e.history[i]
	}
	return out
}

// Run starts the alert evaluation loop.
func (e *Engine) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	e.log.Info("alert engine started", zap.Duration("interval", interval))
	for {
		select {
		case <-ctx.Done():
			e.log.Info("alert engine stopped")
			return
		case <-ticker.C:
			e.evaluate(ctx)
		}
	}
}

func (e *Engine) evaluate(ctx context.Context) {
	e.mu.RLock()
	rules := make([]models.AlertRule, len(e.rules))
	copy(rules, e.rules)
	e.mu.RUnlock()

	now := time.Now()

	for _, rule := range rules {
		value, err := e.evalQuery(rule.Query, now)
		if err != nil {
			e.log.Warn("eval query error", zap.String("rule", rule.Name), zap.Error(err))
			continue
		}

		q, err := query.Parse(rule.Query)
		if err != nil || q.Condition == nil {
			continue
		}

		triggered := query.EvalCondition(q.Condition, value)
		e.handleResult(ctx, rule, triggered, value, now)
	}
}

func (e *Engine) handleResult(ctx context.Context, rule models.AlertRule, triggered bool, value float64, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	existing, wasFiring := e.firing[rule.Name]

	if triggered && !wasFiring {
		// New alert firing
		ev := &models.AlertEvent{
			ID:       uuid.New().String(),
			RuleName: rule.Name,
			Severity: rule.Severity,
			State:    models.AlertFiring,
			Message:  rule.Message,
			Value:    value,
			FiredAt:  now,
		}
		e.firing[rule.Name] = ev
		e.appendHistory(*ev)
		e.log.Warn("alert firing",
			zap.String("rule", rule.Name),
			zap.String("severity", string(rule.Severity)),
			zap.Float64("value", value),
		)
		if e.pg != nil {
			_ = e.pg.InsertAlertEvent(ctx, *ev)
		}
	} else if !triggered && wasFiring {
		// Alert resolved
		resolved := now
		existing.State = models.AlertResolved
		existing.ResolvedAt = &resolved
		e.appendHistory(*existing)
		delete(e.firing, rule.Name)
		e.log.Info("alert resolved", zap.String("rule", rule.Name))
		if e.pg != nil {
			_ = e.pg.ResolveAlert(ctx, existing.ID, resolved)
		}
	}
}

func (e *Engine) appendHistory(ev models.AlertEvent) {
	e.history = append(e.history, ev)
	if len(e.history) > e.maxHist {
		e.history = e.history[len(e.history)-e.maxHist:]
	}
}

// evalQuery computes a scalar value for a query expression.
func (e *Engine) evalQuery(expr string, now time.Time) (float64, error) {
	q, err := query.Parse(expr)
	if err != nil {
		return 0, err
	}

	from := now.Add(-q.Duration)
	points := e.ts.Query(q.Metric, q.Labels, from, now)
	if len(points) == 0 {
		return 0, nil
	}

	switch q.Function {
	case "avg", "avg_over_time":
		sum := 0.0
		for _, p := range points {
			sum += p.Value
		}
		return sum / float64(len(points)), nil
	case "sum":
		sum := 0.0
		for _, p := range points {
			sum += p.Value
		}
		return sum, nil
	case "max":
		max := points[0].Value
		for _, p := range points[1:] {
			if p.Value > max {
				max = p.Value
			}
		}
		return max, nil
	case "min":
		min := points[0].Value
		for _, p := range points[1:] {
			if p.Value < min {
				min = p.Value
			}
		}
		return min, nil
	case "count":
		return float64(len(points)), nil
	case "rate":
		if len(points) < 2 {
			return 0, nil
		}
		delta := points[len(points)-1].Value - points[0].Value
		dur := points[len(points)-1].Timestamp.Sub(points[0].Timestamp).Seconds()
		if dur == 0 {
			return 0, nil
		}
		return delta / dur, nil
	case "p99":
		vals := make([]float64, len(points))
		for i, p := range points {
			vals[i] = p.Value
		}
		sortFloats(vals)
		idx := int(float64(len(vals)) * 0.99)
		if idx >= len(vals) {
			idx = len(vals) - 1
		}
		return vals[idx], nil
	case "last":
		return points[len(points)-1].Value, nil
	default:
		return 0, fmt.Errorf("unknown function: %s", q.Function)
	}
}

func sortFloats(vals []float64) {
	for i := 0; i < len(vals); i++ {
		for j := i + 1; j < len(vals); j++ {
			if vals[i] > vals[j] {
				vals[i], vals[j] = vals[j], vals[i]
			}
		}
	}
}
