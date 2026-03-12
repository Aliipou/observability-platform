# Observability Platform

A lightweight observability backend built in Go — inspired by Prometheus and Datadog. Collects metrics, logs, and traces; evaluates alert rules; and serves a real-time dashboard.

## Architecture

```
┌──────────────────────────────────────────────────────┐
│                  Applications / Services             │
│         POST /api/v1/metrics  /logs  /traces         │
└──────────────────┬───────────────────────────────────┘
                   │
       ┌───────────▼────────────┐
       │      Gin REST API      │
       │  metrics · logs ·      │
       │  traces · alerts ·     │
       │  query · overview      │
       └──────┬─────────┬───────┘
              │         │
   ┌──────────▼──┐  ┌───▼──────────────┐
   │  TimeSeries │  │   PostgreSQL      │
   │   Store     │  │  logs · traces ·  │
   │  (in-memory │  │  alert_history    │
   │  ring buf)  │  └───────────────────┘
   └──────┬──────┘
          │
   ┌──────▼──────────────┐
   │    Alert Engine     │  evaluates rules every 30s
   │  YAML rule config   │  firing / resolved lifecycle
   │  avg·sum·p99·rate   │  persists events to Postgres
   └─────────────────────┘
          │
   ┌──────▼──────────────┐
   │   Web Dashboard     │  auto-refresh every 10s
   │  metrics · logs ·   │  SVG line charts
   │  alerts · history   │  log search + filter
   └─────────────────────┘
```

## Features

- **Metrics ingestion** — Counter, Gauge, Histogram with label sets
- **In-memory time series** — ring-buffer storage (24h, 5760pts/series), automatic downsampling to 1-min resolution after 1h
- **PromQL-inspired query language** — `avg(cpu_usage{host="web-1"}, 5m)`, `p99(latency{}, 1h) > 2000`
- **Structured log ingestion** — service/level/trace filtering, full-text search
- **Distributed trace storage** — span storage with parent linkage, trace reconstruction
- **Alert engine** — YAML-defined rules, firing/resolved lifecycle, alert history
- **Dashboard** — live stats, metric line chart, log viewer, alert feed
- **Graceful degradation** — runs fully in-memory if PostgreSQL is unavailable

## Quick Start

```bash
# Start Postgres
docker compose -f deployments/docker-compose.yml up -d postgres

# Run server
go run ./cmd/server
```

Dashboard: http://localhost:9090
API: http://localhost:9090/api/v1

## Pushing Metrics

```bash
curl -X POST http://localhost:9090/api/v1/metrics \
  -H 'Content-Type: application/json' \
  -d '{
    "metrics": [
      {
        "name": "http_requests_total",
        "type": "counter",
        "value": 1,
        "labels": {"service": "api", "method": "GET", "status": "200"}
      },
      {
        "name": "cpu_usage_percent",
        "type": "gauge",
        "value": 73.5,
        "labels": {"host": "web-1"}
      }
    ]
  }'
```

## Alert Rules (configs/alerts.yaml)

```yaml
rules:
  - name: high-error-rate
    query: "rate(http_errors_total{}, 5m) > 10"
    severity: critical
    message: "HTTP error rate exceeds 10/sec"

  - name: high-cpu
    query: "avg(cpu_usage_percent{}, 5m) > 85"
    severity: warning
    message: "CPU above 85%"
```

Supported functions: `rate`, `avg`, `avg_over_time`, `sum`, `max`, `min`, `p99`, `count`, `last`
Operators: `>`, `>=`, `<`, `<=`, `==`, `!=`

## API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/metrics` | Ingest metric batch |
| `GET` | `/api/v1/metrics/query` | Query time series |
| `GET` | `/api/v1/metrics/names` | List metric names |
| `POST` | `/api/v1/logs` | Ingest log batch |
| `GET` | `/api/v1/logs` | Query logs |
| `POST` | `/api/v1/traces` | Ingest spans |
| `GET` | `/api/v1/traces/:trace_id` | Get full trace |
| `GET` | `/api/v1/alerts` | Firing alerts |
| `GET` | `/api/v1/alerts/history` | Alert history |
| `GET` | `/api/v1/alerts/rules` | Loaded rules |
| `GET` | `/api/v1/overview` | Dashboard summary |
| `GET` | `/api/v1/query?q=` | Eval query expression |

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `OBS_SERVER_PORT` | `9090` | HTTP port |
| `OBS_DATABASE_URL` | `postgres://obsuser:obspass@localhost:5432/observability` | PostgreSQL DSN |
| `OBS_ALERT_RULES` | `configs/alerts.yaml` | Alert rules file path |
| `OBS_LOG_LEVEL` | `info` | Log verbosity |

## Running Tests

```bash
go test ./... -race -count=1
```

## Tech Stack

- **Go 1.22** — in-memory ring buffers, context-based lifecycle
- **PostgreSQL 16** — logs, traces, alert history
- **Gin** — HTTP framework
- **go.uber.org/zap** — structured logging
- **gopkg.in/yaml.v3** — alert rule parsing
