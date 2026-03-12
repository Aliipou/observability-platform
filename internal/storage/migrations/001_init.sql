-- Observability Platform Schema

CREATE TABLE IF NOT EXISTS logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service VARCHAR(100) NOT NULL,
    level VARCHAR(20) NOT NULL,
    message TEXT NOT NULL,
    fields JSONB DEFAULT '{}',
    trace_id VARCHAR(64),
    span_id VARCHAR(32),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_logs_service_time ON logs(service, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_logs_level_time ON logs(level, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_logs_trace ON logs(trace_id);

CREATE TABLE IF NOT EXISTS traces (
    trace_id VARCHAR(64) NOT NULL,
    span_id VARCHAR(64) PRIMARY KEY,
    parent_id VARCHAR(64),
    service VARCHAR(100) NOT NULL,
    operation VARCHAR(255) NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    duration_ms DOUBLE PRECISION NOT NULL,
    status VARCHAR(20),
    tags JSONB DEFAULT '{}',
    error TEXT
);

CREATE INDEX IF NOT EXISTS idx_traces_trace ON traces(trace_id);
CREATE INDEX IF NOT EXISTS idx_traces_service ON traces(service, start_time DESC);

CREATE TABLE IF NOT EXISTS alert_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_name VARCHAR(255) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    message TEXT,
    labels JSONB DEFAULT '{}',
    fired_at TIMESTAMPTZ DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_alert_history_rule ON alert_history(rule_name, fired_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_history_severity ON alert_history(severity, fired_at DESC);
