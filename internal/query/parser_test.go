package query_test

import (
	"testing"
	"time"

	"github.com/aliipou/observability-platform/internal/query"
)

// ── Parse tests ───────────────────────────────────────────────────────────────

func TestParse_SimpleMetric(t *testing.T) {
	q, err := query.Parse("cpu_usage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Function != "last" {
		t.Errorf("function: got %q, want %q", q.Function, "last")
	}
	if q.Metric != "cpu_usage" {
		t.Errorf("metric: got %q, want %q", q.Metric, "cpu_usage")
	}
}

func TestParse_FunctionWithMetric(t *testing.T) {
	q, err := query.Parse("avg(cpu_usage{}, 5m)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Function != "avg" {
		t.Errorf("function: got %q, want avg", q.Function)
	}
	if q.Metric != "cpu_usage" {
		t.Errorf("metric: got %q, want cpu_usage", q.Metric)
	}
	if q.Duration != 5*time.Minute {
		t.Errorf("duration: got %v, want 5m", q.Duration)
	}
}

func TestParse_WithLabels(t *testing.T) {
	q, err := query.Parse(`rate(http_requests_total{service="api",env="prod"}, 1m)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Function != "rate" {
		t.Errorf("function: got %q, want rate", q.Function)
	}
	if q.Metric != "http_requests_total" {
		t.Errorf("metric: got %q, want http_requests_total", q.Metric)
	}
	if q.Labels["service"] != "api" {
		t.Errorf("service label: got %q, want api", q.Labels["service"])
	}
	if q.Labels["env"] != "prod" {
		t.Errorf("env label: got %q, want prod", q.Labels["env"])
	}
	if q.Duration != 1*time.Minute {
		t.Errorf("duration: got %v, want 1m", q.Duration)
	}
}

func TestParse_WithCondition(t *testing.T) {
	tests := []struct {
		input    string
		op       string
		val      float64
	}{
		{`sum(error_count{}, 5m) > 100`, ">", 100},
		{`avg(latency_ms{}, 1m) >= 500`, ">=", 500},
		{`count(requests{}, 10m) < 1000`, "<", 1000},
		{`max(cpu{}, 5m) <= 90`, "<=", 90},
		{`min(mem{}, 5m) != 0`, "!=", 0},
		{`last(health{}) == 1`, "==", 1},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			q, err := query.Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Condition == nil {
				t.Fatal("expected condition, got nil")
			}
			if q.Condition.Operator != tc.op {
				t.Errorf("op: got %q, want %q", q.Condition.Operator, tc.op)
			}
			if q.Condition.Value != tc.val {
				t.Errorf("val: got %v, want %v", q.Condition.Value, tc.val)
			}
		})
	}
}

func TestParse_AllFunctions(t *testing.T) {
	fns := []string{"rate", "avg", "avg_over_time", "sum", "max", "min", "p99", "count", "last"}
	for _, fn := range fns {
		t.Run(fn, func(t *testing.T) {
			input := fn + `(metric{}, 5m)`
			q, err := query.Parse(input)
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", fn, err)
			}
			if q.Function != fn {
				t.Errorf("function: got %q, want %q", q.Function, fn)
			}
		})
	}
}

func TestParse_DurationVariants(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{`avg(m{}, 30s)`, 30 * time.Second},
		{`avg(m{}, 5m)`, 5 * time.Minute},
		{`avg(m{}, 1h)`, time.Hour},
		{`avg(m{}, 2h30m)`, 2*time.Hour + 30*time.Minute},
		{`avg(m{}, 1d)`, 24 * time.Hour},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			q, err := query.Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Duration != tc.expected {
				t.Errorf("duration: got %v, want %v", q.Duration, tc.expected)
			}
		})
	}
}

func TestParse_DefaultDuration(t *testing.T) {
	q, err := query.Parse("avg(metric{})")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Duration != 5*time.Minute {
		t.Errorf("default duration: got %v, want 5m", q.Duration)
	}
}

func TestParse_EmptyLabels(t *testing.T) {
	q, err := query.Parse("avg(cpu_usage{}, 1m)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Labels) != 0 {
		t.Errorf("expected empty labels, got %v", q.Labels)
	}
}

// ── Error cases ───────────────────────────────────────────────────────────────

func TestParse_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"unknown_function", "unknown(cpu{}, 1m)"},
		{"missing_close_paren", "avg(cpu{}, 1m"},
		{"invalid_condition_value", "avg(cpu{}, 1m) > abc"},
		{"missing_brace_close", "avg(cpu{service=\"api\", 1m)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := query.Parse(tc.input)
			if err == nil {
				t.Errorf("expected error for input %q", tc.input)
			}
		})
	}
}

func TestParse_SingleLabel(t *testing.T) {
	q, err := query.Parse(`p99(request_duration_ms{service="checkout"}, 1h)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Labels["service"] != "checkout" {
		t.Errorf("service label: got %q, want checkout", q.Labels["service"])
	}
	if q.Duration != time.Hour {
		t.Errorf("duration: got %v, want 1h", q.Duration)
	}
}

// ── EvalCondition tests ───────────────────────────────────────────────────────

func TestEvalCondition(t *testing.T) {
	tests := []struct {
		cond  *query.Condition
		value float64
		want  bool
	}{
		{&query.Condition{Operator: ">", Value: 100}, 150, true},
		{&query.Condition{Operator: ">", Value: 100}, 100, false},
		{&query.Condition{Operator: ">=", Value: 100}, 100, true},
		{&query.Condition{Operator: "<", Value: 50}, 30, true},
		{&query.Condition{Operator: "<", Value: 50}, 50, false},
		{&query.Condition{Operator: "<=", Value: 50}, 50, true},
		{&query.Condition{Operator: "==", Value: 42}, 42, true},
		{&query.Condition{Operator: "==", Value: 42}, 43, false},
		{&query.Condition{Operator: "!=", Value: 42}, 43, true},
		{&query.Condition{Operator: "!=", Value: 42}, 42, false},
		{&query.Condition{Operator: "???", Value: 0}, 0, false}, // unknown op
		{nil, 100, false}, // nil condition
	}
	for _, tc := range tests {
		got := query.EvalCondition(tc.cond, tc.value)
		if got != tc.want {
			var op string
			if tc.cond != nil {
				op = tc.cond.Operator
			}
			t.Errorf("EvalCondition(%s, %v, %v): got %v, want %v", op, tc.cond, tc.value, got, tc.want)
		}
	}
}

func TestEvalCondition_NilCondition(t *testing.T) {
	if query.EvalCondition(nil, 100) {
		t.Error("nil condition should return false")
	}
}

// ── Round-trip test ───────────────────────────────────────────────────────────

func TestParse_ConditionRoundTrip(t *testing.T) {
	input := `rate(http_errors_total{service="payment",env="prod"}, 5m) > 10`
	q, err := query.Parse(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.Function != "rate" {
		t.Errorf("function: got %q", q.Function)
	}
	if q.Metric != "http_errors_total" {
		t.Errorf("metric: got %q", q.Metric)
	}
	if q.Labels["service"] != "payment" || q.Labels["env"] != "prod" {
		t.Errorf("labels: got %v", q.Labels)
	}
	if q.Duration != 5*time.Minute {
		t.Errorf("duration: got %v", q.Duration)
	}
	if q.Condition == nil || q.Condition.Operator != ">" || q.Condition.Value != 10 {
		t.Errorf("condition: got %+v", q.Condition)
	}
}
