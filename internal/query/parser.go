package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Query represents a parsed query.
type Query struct {
	Function  string            // e.g., "rate", "avg", "sum", "max", "min", "p99", "count"
	Metric    string            // e.g., "http_requests_total"
	Labels    map[string]string // e.g., {"service": "api"}
	Duration  time.Duration     // e.g., 5m
	Condition *Condition        // optional threshold condition
}

// Condition represents a comparison for alert evaluation.
type Condition struct {
	Operator string  // ">", "<", ">=", "<=", "==", "!="
	Value    float64 // threshold value
}

// Parse parses a query string into a Query struct.
// Examples:
//
//	rate(http_requests_total{service="api"}, 5m)
//	avg(cpu_usage{host="web-1"}, 10m)
//	sum(error_count{}) > 100
//	p99(request_duration_ms{service="checkout"}, 1h)
func Parse(input string) (*Query, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty query")
	}

	q := &Query{
		Labels: make(map[string]string),
	}

	// Check for condition suffix: e.g., "> 100"
	conditionPart := ""
	mainPart := input
	for _, op := range []string{">=", "<=", "!=", "==", ">", "<"} {
		// Find operator outside parentheses
		depth := 0
		for i := 0; i < len(input); i++ {
			switch input[i] {
			case '(':
				depth++
			case ')':
				depth--
			}
			if depth == 0 && i+len(op) <= len(input) && input[i:i+len(op)] == op {
				mainPart = strings.TrimSpace(input[:i])
				conditionPart = strings.TrimSpace(input[i:])
				break
			}
		}
		if conditionPart != "" {
			break
		}
	}

	if conditionPart != "" {
		cond, err := parseCondition(conditionPart)
		if err != nil {
			return nil, fmt.Errorf("parse condition: %w", err)
		}
		q.Condition = cond
	}

	// Parse function call: func(metric{labels}, duration)
	parenOpen := strings.Index(mainPart, "(")
	if parenOpen == -1 {
		// No function, treat as raw metric name
		q.Function = "last"
		q.Metric = mainPart
		q.Duration = 5 * time.Minute
		return q, nil
	}

	q.Function = strings.TrimSpace(mainPart[:parenOpen])
	if !isValidFunction(q.Function) {
		return nil, fmt.Errorf("unknown function: %s", q.Function)
	}

	// Find matching close paren
	parenClose := strings.LastIndex(mainPart, ")")
	if parenClose == -1 {
		return nil, fmt.Errorf("missing closing parenthesis")
	}

	inner := mainPart[parenOpen+1 : parenClose]

	// Split inner by comma, but respect braces
	parts := splitRespectingBraces(inner)

	if len(parts) == 0 {
		return nil, fmt.Errorf("missing metric name")
	}

	// Parse metric{labels}
	metricPart := strings.TrimSpace(parts[0])
	braceOpen := strings.Index(metricPart, "{")
	if braceOpen == -1 {
		q.Metric = metricPart
	} else {
		q.Metric = strings.TrimSpace(metricPart[:braceOpen])
		braceClose := strings.LastIndex(metricPart, "}")
		if braceClose == -1 {
			return nil, fmt.Errorf("missing closing brace in label selector")
		}
		labelStr := metricPart[braceOpen+1 : braceClose]
		if labelStr != "" {
			labels, err := parseLabels(labelStr)
			if err != nil {
				return nil, err
			}
			q.Labels = labels
		}
	}

	// Parse duration (second argument)
	if len(parts) >= 2 {
		durStr := strings.TrimSpace(parts[1])
		dur, err := parseDuration(durStr)
		if err != nil {
			return nil, fmt.Errorf("parse duration %q: %w", durStr, err)
		}
		q.Duration = dur
	} else {
		q.Duration = 5 * time.Minute
	}

	return q, nil
}

// parseLabels parses a comma-separated list of key="value" pairs.
func parseLabels(s string) (map[string]string, error) {
	labels := make(map[string]string)
	pairs := strings.Split(s, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		eqIdx := strings.Index(pair, "=")
		if eqIdx == -1 {
			return nil, fmt.Errorf("invalid label: %q (missing =)", pair)
		}
		key := strings.TrimSpace(pair[:eqIdx])
		val := strings.TrimSpace(pair[eqIdx+1:])
		val = strings.Trim(val, "\"'")
		labels[key] = val
	}
	return labels, nil
}

// parseCondition parses a condition string like "> 10" or "<= 500".
func parseCondition(s string) (*Condition, error) {
	s = strings.TrimSpace(s)
	for _, op := range []string{">=", "<=", "!=", "==", ">", "<"} {
		if strings.HasPrefix(s, op) {
			valStr := strings.TrimSpace(s[len(op):])
			val, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid threshold value %q: %w", valStr, err)
			}
			return &Condition{Operator: op, Value: val}, nil
		}
	}
	return nil, fmt.Errorf("invalid condition: %q", s)
}

// parseDuration parses duration strings like "5m", "1h", "30s", "2h30m".
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 5 * time.Minute, nil
	}

	totalMs := int64(0)
	current := ""

	for _, ch := range s {
		switch ch {
		case 's':
			v, err := strconv.ParseInt(current, 10, 64)
			if err != nil {
				return 0, err
			}
			totalMs += v * 1000
			current = ""
		case 'm':
			v, err := strconv.ParseInt(current, 10, 64)
			if err != nil {
				return 0, err
			}
			totalMs += v * 60 * 1000
			current = ""
		case 'h':
			v, err := strconv.ParseInt(current, 10, 64)
			if err != nil {
				return 0, err
			}
			totalMs += v * 3600 * 1000
			current = ""
		case 'd':
			v, err := strconv.ParseInt(current, 10, 64)
			if err != nil {
				return 0, err
			}
			totalMs += v * 86400 * 1000
			current = ""
		default:
			current += string(ch)
		}
	}

	if totalMs == 0 {
		return 0, fmt.Errorf("invalid duration: %s", s)
	}

	return time.Duration(totalMs) * time.Millisecond, nil
}

// splitRespectingBraces splits by comma but respects {} braces.
func splitRespectingBraces(s string) []string {
	var parts []string
	depth := 0
	current := ""

	for _, ch := range s {
		switch ch {
		case '{':
			depth++
			current += string(ch)
		case '}':
			depth--
			current += string(ch)
		case ',':
			if depth == 0 {
				parts = append(parts, current)
				current = ""
			} else {
				current += string(ch)
			}
		default:
			current += string(ch)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// isValidFunction checks if the function name is supported.
func isValidFunction(name string) bool {
	switch name {
	case "rate", "avg", "avg_over_time", "sum", "max", "min", "p99", "count", "last":
		return true
	default:
		return false
	}
}

// EvalCondition evaluates a condition against a value.
func EvalCondition(cond *Condition, value float64) bool {
	if cond == nil {
		return false
	}
	switch cond.Operator {
	case ">":
		return value > cond.Value
	case "<":
		return value < cond.Value
	case ">=":
		return value >= cond.Value
	case "<=":
		return value <= cond.Value
	case "==":
		return value == cond.Value
	case "!=":
		return value != cond.Value
	default:
		return false
	}
}
