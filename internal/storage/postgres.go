package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aliipou/observability-platform/internal/models"
)

// PostgresStore handles persistent storage for logs, traces, and alert history.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgreSQL connection pool.
func NewPostgresStore(databaseURL string) (*PostgresStore, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}

	config.MaxConns = 20
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &PostgresStore{pool: pool}, nil
}

// RunMigrations executes the SQL migration files.
func (ps *PostgresStore) RunMigrations(migrationsPath string) error {
	data, err := os.ReadFile(migrationsPath)
	if err != nil {
		return fmt.Errorf("read migration file: %w", err)
	}

	_, err = ps.pool.Exec(context.Background(), string(data))
	if err != nil {
		return fmt.Errorf("execute migration: %w", err)
	}

	return nil
}

// Close shuts down the connection pool.
func (ps *PostgresStore) Close() {
	ps.pool.Close()
}

// InsertLog stores a log entry in PostgreSQL.
func (ps *PostgresStore) InsertLog(ctx context.Context, entry models.LogEntry) error {
	fieldsJSON, err := json.Marshal(entry.Fields)
	if err != nil {
		fieldsJSON = []byte("{}")
	}

	_, err = ps.pool.Exec(ctx,
		`INSERT INTO logs (id, service, level, message, fields, trace_id, span_id, timestamp)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		entry.ID, entry.Service, string(entry.Level), entry.Message,
		fieldsJSON, entry.TraceID, entry.SpanID, entry.Timestamp,
	)
	return err
}

// InsertLogBatch stores multiple log entries.
func (ps *PostgresStore) InsertLogBatch(ctx context.Context, entries []models.LogEntry) error {
	for _, entry := range entries {
		if err := ps.InsertLog(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

// QueryLogs returns log entries matching the given filters.
func (ps *PostgresStore) QueryLogs(ctx context.Context, q models.LogQuery) ([]models.LogEntry, error) {
	query := "SELECT id, service, level, message, fields, trace_id, span_id, timestamp FROM logs WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if q.Service != "" {
		query += fmt.Sprintf(" AND service = $%d", argIdx)
		args = append(args, q.Service)
		argIdx++
	}
	if q.Level != "" {
		query += fmt.Sprintf(" AND level = $%d", argIdx)
		args = append(args, string(q.Level))
		argIdx++
	}
	if q.TraceID != "" {
		query += fmt.Sprintf(" AND trace_id = $%d", argIdx)
		args = append(args, q.TraceID)
		argIdx++
	}
	if q.Search != "" {
		query += fmt.Sprintf(" AND message ILIKE '%%' || $%d || '%%'", argIdx)
		args = append(args, q.Search)
		argIdx++
	}
	if q.From != "" {
		t, err := time.Parse(time.RFC3339, q.From)
		if err == nil {
			query += fmt.Sprintf(" AND timestamp >= $%d", argIdx)
			args = append(args, t)
			argIdx++
		}
	}
	if q.To != "" {
		t, err := time.Parse(time.RFC3339, q.To)
		if err == nil {
			query += fmt.Sprintf(" AND timestamp <= $%d", argIdx)
			args = append(args, t)
			argIdx++
		}
	}

	query += " ORDER BY timestamp DESC"

	limit := q.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT $%d", argIdx)
	args = append(args, limit)

	rows, err := ps.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.LogEntry
	for rows.Next() {
		var entry models.LogEntry
		var fieldsJSON []byte
		var level string
		err := rows.Scan(&entry.ID, &entry.Service, &level, &entry.Message,
			&fieldsJSON, &entry.TraceID, &entry.SpanID, &entry.Timestamp)
		if err != nil {
			return nil, err
		}
		entry.Level = models.LogLevel(level)
		if len(fieldsJSON) > 0 {
			_ = json.Unmarshal(fieldsJSON, &entry.Fields)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// CountLogs returns the total number of log entries.
func (ps *PostgresStore) CountLogs(ctx context.Context) (int64, error) {
	var count int64
	err := ps.pool.QueryRow(ctx, "SELECT COUNT(*) FROM logs").Scan(&count)
	return count, err
}

// InsertSpan stores a span in PostgreSQL.
func (ps *PostgresStore) InsertSpan(ctx context.Context, span models.Span) error {
	tagsJSON, err := json.Marshal(span.Tags)
	if err != nil {
		tagsJSON = []byte("{}")
	}

	_, err = ps.pool.Exec(ctx,
		`INSERT INTO traces (trace_id, span_id, parent_id, service, operation, start_time, duration_ms, status, tags, error)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (span_id) DO NOTHING`,
		span.TraceID, span.SpanID, span.ParentID, span.Service, span.Operation,
		span.StartTime, float64(span.Duration.Milliseconds()), span.Status,
		tagsJSON, span.Error,
	)
	return err
}

// InsertSpanBatch stores multiple spans.
func (ps *PostgresStore) InsertSpanBatch(ctx context.Context, spans []models.Span) error {
	for _, span := range spans {
		if err := ps.InsertSpan(ctx, span); err != nil {
			return err
		}
	}
	return nil
}

// GetTrace returns all spans for a given trace ID.
func (ps *PostgresStore) GetTrace(ctx context.Context, traceID string) ([]models.Span, error) {
	rows, err := ps.pool.Query(ctx,
		`SELECT trace_id, span_id, parent_id, service, operation, start_time, duration_ms, status, tags, error
		 FROM traces WHERE trace_id = $1 ORDER BY start_time ASC`, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSpans(rows)
}

// QueryTraces returns recent traces matching the given filters.
func (ps *PostgresStore) QueryTraces(ctx context.Context, q models.TraceQuery) ([]models.Span, error) {
	query := `SELECT trace_id, span_id, parent_id, service, operation, start_time, duration_ms, status, tags, error
	          FROM traces WHERE parent_id = '' OR parent_id IS NULL`
	args := []interface{}{}
	argIdx := 1

	if q.Service != "" {
		query += fmt.Sprintf(" AND service = $%d", argIdx)
		args = append(args, q.Service)
		argIdx++
	}
	if q.From != "" {
		t, err := time.Parse(time.RFC3339, q.From)
		if err == nil {
			query += fmt.Sprintf(" AND start_time >= $%d", argIdx)
			args = append(args, t)
			argIdx++
		}
	}
	if q.MinDurMs > 0 {
		query += fmt.Sprintf(" AND duration_ms >= $%d", argIdx)
		args = append(args, float64(q.MinDurMs))
		argIdx++
	}

	query += " ORDER BY start_time DESC"
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	query += fmt.Sprintf(" LIMIT $%d", argIdx)
	args = append(args, limit)

	rows, err := ps.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSpans(rows)
}

// CountTraces returns the total number of distinct traces.
func (ps *PostgresStore) CountTraces(ctx context.Context) (int64, error) {
	var count int64
	err := ps.pool.QueryRow(ctx, "SELECT COUNT(DISTINCT trace_id) FROM traces").Scan(&count)
	return count, err
}

// InsertAlertEvent stores an alert event.
func (ps *PostgresStore) InsertAlertEvent(ctx context.Context, event models.AlertEvent) error {
	labelsJSON, err := json.Marshal(event.Labels)
	if err != nil {
		labelsJSON = []byte("{}")
	}

	_, err = ps.pool.Exec(ctx,
		`INSERT INTO alert_history (id, rule_name, severity, message, labels, fired_at, resolved_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		event.ID, event.RuleName, string(event.Severity), event.Message,
		labelsJSON, event.FiredAt, event.ResolvedAt,
	)
	return err
}

// ResolveAlert marks an alert as resolved.
func (ps *PostgresStore) ResolveAlert(ctx context.Context, id string, resolvedAt time.Time) error {
	_, err := ps.pool.Exec(ctx,
		`UPDATE alert_history SET resolved_at = $1 WHERE id = $2`,
		resolvedAt, id,
	)
	return err
}

// GetAlertHistory returns recent alert events.
func (ps *PostgresStore) GetAlertHistory(ctx context.Context, limit int) ([]models.AlertEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := ps.pool.Query(ctx,
		`SELECT id, rule_name, severity, message, labels, fired_at, resolved_at
		 FROM alert_history ORDER BY fired_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.AlertEvent
	for rows.Next() {
		var event models.AlertEvent
		var severity string
		var labelsJSON []byte
		err := rows.Scan(&event.ID, &event.RuleName, &severity, &event.Message,
			&labelsJSON, &event.FiredAt, &event.ResolvedAt)
		if err != nil {
			return nil, err
		}
		event.Severity = models.AlertSeverity(severity)
		if len(labelsJSON) > 0 {
			_ = json.Unmarshal(labelsJSON, &event.Labels)
		}
		if event.ResolvedAt == nil {
			event.State = models.AlertFiring
		} else {
			event.State = models.AlertResolved
		}
		events = append(events, event)
	}

	return events, nil
}

// scanSpans is a helper to read span rows from a query result.
func scanSpans(rows interface {
	Next() bool
	Scan(dest ...interface{}) error
}) ([]models.Span, error) {
	var spans []models.Span
	for rows.Next() {
		var span models.Span
		var durationMs float64
		var tagsJSON []byte
		var parentID *string
		var errStr *string

		err := rows.Scan(&span.TraceID, &span.SpanID, &parentID, &span.Service,
			&span.Operation, &span.StartTime, &durationMs, &span.Status,
			&tagsJSON, &errStr)
		if err != nil {
			return nil, err
		}

		span.Duration = time.Duration(durationMs) * time.Millisecond
		if parentID != nil {
			span.ParentID = *parentID
		}
		if errStr != nil {
			span.Error = *errStr
		}
		if len(tagsJSON) > 0 {
			_ = json.Unmarshal(tagsJSON, &span.Tags)
		}
		spans = append(spans, span)
	}
	return spans, nil
}
