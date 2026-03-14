# Observability Platform

> Lightweight observability backend in Go — metrics, logs, traces, and alerting in a single binary.

![Go](https://img.shields.io/badge/Go-1.23-00ADD8?logo=go)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?logo=postgresql)
![License](https://img.shields.io/badge/license-MIT-green)
![CI](https://github.com/aliipou/observability-platform/actions/workflows/ci.yml/badge.svg)

---

## Problem & Requirements

### Functional Requirements

| Signal | Capability |
|--------|-----------|
| Metrics | Ingest counter / gauge / histogram with arbitrary label sets |
| Metrics | PromQL-inspired query language — `avg`, `sum`, `rate`, `p99`, `min`, `max`, `count`, `last` |
| Logs | Structured ingestion with service / level / trace-id filtering and full-text search |
| Traces | Span storage with parent linkage; full trace reconstruction by trace ID |
| Alerts | YAML-defined rules evaluated on a 30 s cycle; firing / resolved state machine |
| Dashboard | Real-time web dashboard with SVG charts and 10 s auto-refresh |

### Non-Functional Requirements

| Requirement | Target |
|-------------|--------|
| Metric query latency (last 1 h, L1 hit) | < 10 ms |
| Series capacity | 10,000 series in RAM |
| Graceful degradation | Fully operational without PostgreSQL |
| Alert evaluation cycle | 30 s |
| Data retention (raw) | 24 h in ring buffer; indefinite in PostgreSQL |

### Capacity Estimates

```
Services:       100 services × 50 metrics each = 5,000 active series
Ingestion rate: 1 data point / second / series  = 5,000 pts/sec

Ring buffer sizing:
  RingSize = 5,760 pts/series  (24 h × 3600 s/h ÷ 15 s/pt = 5,760)
  Total points: 5,760 × 5,000 = 28.8 M points
  Memory at 16 B/pt: 28.8 M × 16 B ≈ 461 MB

Cold storage (PostgreSQL):
  Logs + traces + alert_history
  Alert history capped at 1,000 events in-process; remainder in Postgres
```

---

## Architecture

```
Services / Apps
     │  POST /api/v1/metrics | /logs | /traces
     ▼
┌────────────────────────────────────────────┐
│               Gin REST API                 │
│  metrics · logs · traces · alerts · query  │
└──────┬──────────────────┬──────────────────┘
       │                  │
┌──────▼──────┐   ┌───────▼─────────────────┐
│ TimeSeries  │   │       PostgreSQL         │
│   Store     │   │  logs · traces ·         │
│             │   │  alert_history           │
│  L1: ring   │   └─────────────────────────┘
│  buffer     │
│  5,760 pts  │
│  per series │        ┌─────────────────────┐
│  24 h @ 15s ├───────►│   Alert Engine      │
│             │        │  YAML rules         │
│  1-min      │        │  30 s eval cycle    │
│  downsample │        │  firing / resolved  │
│  after 1 h  │        └─────────────────────┘
└─────────────┘
       │
┌──────▼───────────────┐
│    Query Engine      │
│  lexer → AST →       │
│  evaluator           │
│  avg·sum·p99·rate    │
│  label matching      │
└──────────────────────┘
       │
┌──────▼───────────────┐
│   Web Dashboard      │
│  10 s auto-refresh   │
│  SVG line charts     │
└──────────────────────┘
```

---

## Key Design Decisions

Each decision is presented as **Choice → Rationale → Trade-off**.

### 1. In-memory ring buffer (L1) + PostgreSQL (L2)

Hot queries against the last hour hit RAM at sub-millisecond latency. Cold queries (logs, traces, alert history) fall through to Postgres.

**Trade-off:** The ring buffer is lost on process restart. This is acceptable because the buffer covers only the last 24 h of raw samples and Postgres holds durable history.

### 2. Fixed-size ring buffer (5,760 points per series)

`O(1)` write — advance a head pointer and overwrite. `O(1)` eviction — the oldest point is implicitly replaced. Memory per series is constant and predictable at allocation time (`make([]MetricPoint, 5760)`).

**Trade-off:** Series ingested at sub-second cadence evict data faster than 24 h. A 1 s/pt series fills the buffer in 96 minutes. Documented in [ROADMAP.md](ROADMAP.md) — configurable ring size is Phase 2.

### 3. Automatic downsampling after 1 h

Points older than 1 h are aggregated into 1-minute buckets (averaged) on read, not on write. This gives 60× storage reduction for cold data while keeping recent data at full resolution.

**Trade-off:** Sub-minute precision is lost after 1 h. Acceptable for operational monitoring; raw precision is available for the first hour.

### 4. PromQL-inspired query language (not full PromQL)

Supports `avg`, `avg_over_time`, `sum`, `rate`, `p99`, `min`, `max`, `count`, `last` with label filters and optional comparison conditions. The parser is a hand-written lexer → AST → evaluator in ~300 lines with zero dependencies.

**Trade-off:** No range vectors (`metric[5m]`), no `irate()`, no `histogram_quantile()`, no recording rules. These cover ~90% of operational monitoring use cases. Full PromQL compatibility is in Phase 3 of the roadmap.

### 5. Alert engine as a long-running goroutine with a `time.Ticker`

The engine runs in a separate goroutine, wakes on a 30 s ticker, copies the rule list under an `RWMutex`, evaluates each rule, and transitions state. Rules reload from YAML at startup; hot-reload is available via `SetRules`. State is held in a `map[string]*AlertEvent` (one entry per firing rule name).

**Trade-off:** Minimum alert resolution is 30 s. Fine for infrastructure alerting; not suitable for sub-second SLO breach detection.

### 6. Graceful degradation without PostgreSQL

`TimeSeriesStore` operates entirely in memory. PostgreSQL is used only for logs, traces, and alert history. All metric ingestion and querying, and all alert evaluation, continue normally if the Postgres connection is absent. Log and trace endpoints return `503` with an explanatory message.

**Trade-off:** Alert history and traces are lost on restart when Postgres is unavailable.

### 7. Single binary

API server, time series store, alert engine, and query evaluator run in one process under a shared context. No service mesh, no inter-process serialization overhead.

**Trade-off:** Cannot scale individual components independently. Acceptable below ~10,000 series. The [ROADMAP.md](ROADMAP.md) describes extracting `TimeSeriesStore` to an interface backed by Redis Streams for multi-instance deployments.

---

## Storage Design

### Ring Buffer

```
Series key:  "metric_name{label1=v1,label2=v2}"  (canonical sorted string)

struct series {
    Points []MetricPoint  // length = RingSize (5760), allocated once
    head   int            // next write slot
    count  int            // number of valid points (≤ RingSize)
}

Write:   Points[head] = point; head = (head + 1) % RingSize   → O(1)
Evict:   implicit — head overwrites the oldest slot            → O(1)
Read:    walk from (head - count) to head, filter by timestamp → O(n)
```

### Label Index

```
TimeSeriesStore.seriesDB: map[string]*series
                                │
                        key = SeriesKey(name, labels)
                              = "cpu_usage{host=web-1,region=eu}"

Lookup by name + exact label set: O(1) map get
Lookup by name + partial labels:  O(series_count) linear scan with MatchLabels
```

The linear scan is acceptable at ≤10,000 series. A Phase 2 inverted index (label value → series IDs) would reduce this to O(matching series).

### PostgreSQL Schema

```sql
-- Logs
CREATE TABLE logs (
    id UUID PRIMARY KEY,
    service TEXT, level TEXT, message TEXT,
    trace_id TEXT, timestamp TIMESTAMPTZ
);
CREATE INDEX idx_logs_service_time ON logs(service, timestamp DESC);
CREATE INDEX idx_logs_trace_id    ON logs(trace_id);

-- Traces
CREATE TABLE spans (
    span_id TEXT PRIMARY KEY, trace_id TEXT, parent_span_id TEXT,
    service TEXT, operation TEXT,
    start_time TIMESTAMPTZ, end_time TIMESTAMPTZ,
    status TEXT, attributes JSONB
);
CREATE INDEX idx_spans_trace_id ON spans(trace_id);

-- Alert history
CREATE TABLE alert_events (
    id UUID PRIMARY KEY, rule_name TEXT, severity TEXT,
    state TEXT, message TEXT, value FLOAT8,
    fired_at TIMESTAMPTZ, resolved_at TIMESTAMPTZ
);
```

---

## Query Engine

### Syntax

```
query     = function "(" metric_selector ["," duration] ")" [condition]
selector  = metric_name "{" [label_pairs] "}"
label_pairs = key "=" "\"" value "\"" ["," ...]
duration  = number ("s" | "m" | "h" | "d")
condition = (">" | ">=" | "<" | "<=" | "==" | "!=") number
```

### Examples

```
avg(cpu_usage{host="web-1"}, 5m)           -- average over last 5 minutes
p99(latency_ms{service="api"}, 1h)         -- 99th percentile over last hour
rate(http_requests_total{}, 1m)            -- per-second rate
avg(cpu_usage{}, 5m) > 85                  -- alert condition form
```

### Pipeline

```
input string
    │
    ▼  condition extraction (depth-aware scan for operators outside parens)
    │
    ▼  function name extraction
    │
    ▼  metric + label selector parsing
    │
    ▼  duration parsing (custom: supports "5m", "1h30m", "90s")
    │
    ▼  Query{Function, Metric, Labels, Duration, Condition}
    │
    ▼  evaluator: ts.Query(metric, labels, now-duration, now)
                  → applies function over returned []MetricPoint
                  → returns scalar float64
```

---

## Alert State Machine

```
                  query > threshold
INACTIVE ─────────────────────────────► FIRING
                                           │
                                           │  persisted to Postgres
                                           │  alert_events table
                                           ▼
                  query ≤ threshold     RESOLVED ──► INACTIVE
```

- State is keyed by rule name in `map[string]*AlertEvent`.
- A rule transitions `INACTIVE → FIRING` on the first tick where the condition holds.
- It transitions `FIRING → RESOLVED` on the first tick where the condition no longer holds.
- Both transitions append to the in-process history ring (capped at 1,000 events) and, when Postgres is available, call `InsertAlertEvent` / `ResolveAlert`.
- Rules are re-read from the slice copy each tick; a write to the rule set takes effect on the next evaluation without restarting the goroutine.

---

## Failure Modes

| Failure | Observed Behaviour |
|---------|--------------------|
| PostgreSQL unreachable at startup | Server starts; metrics and alerting fully operational; log/trace endpoints return `503` |
| PostgreSQL lost mid-operation | `pg.Insert*` calls return error, logged at warn; in-process alert history continues normally |
| Alert engine rule evaluation error | Rule skipped for this tick; error logged with rule name and expression; engine continues |
| Ring buffer full | Oldest point silently overwritten — expected, not an error condition |
| Large ingest batch with no body-size limit | Server will accept arbitrarily large JSON bodies (tracked as known limitation — `MaxBodySize` middleware is in the roadmap) |
| Process restart | Ring buffer cleared; Postgres-backed history intact; alert rules reload from YAML |

---

## Scalability

| Component | Current Ceiling | Scale Path |
|-----------|----------------|------------|
| Series count | ~10,000 (RAM) | Extract `TimeSeriesStore` to interface; Redis Streams as distributed backend with consistent hash ring |
| Ingestion rate | ~50,000 pts/sec (benchmark estimate) | Batch endpoint (`WriteBatch`) amortises lock contention; async channel-based writer as next step |
| Query latency | < 1 ms L1, < 10 ms L2 | LRU query cache (512 entries) — Phase 3 roadmap |
| Alert rule count | 100s of rules per 30 s tick | Parallelise evaluation with a worker pool of goroutines |
| Log / trace storage | Single Postgres table | Partition `logs` by day; migrate to TimescaleDB hypertable for metrics — Phase 2 roadmap |

---

## Running Locally

```bash
# Start PostgreSQL (optional — platform runs without it)
docker compose -f deployments/docker-compose.yml up -d postgres

# Run server (with Postgres)
OBS_DATABASE_URL=postgres://obsuser:obspass@localhost:5432/observability \
  go run ./cmd/server

# Run server (in-memory only, no Postgres required)
go run ./cmd/server
```

Dashboard: [http://localhost:9090](http://localhost:9090)
API base: [http://localhost:9090/api/v1](http://localhost:9090/api/v1)

---

## API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/metrics` | Ingest metric batch |
| `GET` | `/api/v1/metrics/query` | Query time series (`?name=&labels=&from=&to=`) |
| `GET` | `/api/v1/metrics/names` | List all metric names |
| `POST` | `/api/v1/logs` | Ingest log batch |
| `GET` | `/api/v1/logs` | Query logs (`?service=&level=&search=`) |
| `POST` | `/api/v1/traces` | Ingest spans |
| `GET` | `/api/v1/traces/:trace_id` | Reconstruct full trace |
| `GET` | `/api/v1/alerts` | Currently firing alerts |
| `GET` | `/api/v1/alerts/history` | Alert history |
| `GET` | `/api/v1/alerts/rules` | Loaded alert rules |
| `GET` | `/api/v1/overview` | Dashboard summary (series count, ingestion rate, firing alert count) |
| `GET` | `/api/v1/query?q=` | Evaluate a query expression and return scalar result |

### Ingest Example

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

### Alert Rules (`configs/alerts.yaml`)

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

  - name: high-p99-latency
    query: "p99(request_duration_ms{service=\"checkout\"}, 1h) > 2000"
    severity: warning
    message: "p99 latency above 2 s on checkout service"
```

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `OBS_SERVER_PORT` | `9090` | HTTP listen port |
| `OBS_DATABASE_URL` | `postgres://obsuser:obspass@localhost:5432/observability` | PostgreSQL DSN; omit to run in-memory only |
| `OBS_ALERT_RULES` | `configs/alerts.yaml` | Path to alert rules YAML |
| `OBS_LOG_LEVEL` | `info` | Zap log level (`debug`, `info`, `warn`, `error`) |

---

## Testing

```bash
# Full test suite with race detector
go test ./... -race -count=1

# Individual packages
go test ./internal/storage/...     # ring buffer: write, read, downsample, eviction
go test ./internal/query/...       # parser: all functions, label selectors, conditions, durations
go test ./internal/alert/...       # engine: state machine transitions, YAML loading
go test ./internal/models/...      # model validation, SeriesKey canonicalisation
```

Test coverage targets:
- `storage`: ring buffer write / read / downsample / cleanup / concurrent access under `-race`
- `query`: every supported function, label filter combinations, all six comparison operators, invalid inputs
- `alert`: `INACTIVE → FIRING → RESOLVED` transitions, rule hot-reload, Postgres-absent path
- `models`: `SeriesKey` canonical form, `MatchLabels` subset matching, metric type validation

---

## Tech Stack

| Layer | Technology | Purpose |
|-------|-----------|---------|
| Language | Go 1.23 | In-memory ring buffers, goroutine-based alert engine, context lifecycle |
| HTTP | Gin v1.9 | REST API routing and middleware |
| Database | PostgreSQL 16 (jackc/pgx v5) | Logs, traces, alert history (cold storage) |
| Logging | go.uber.org/zap v1.27 | Structured, levelled logging |
| Config | gopkg.in/yaml.v3 | Alert rule parsing |
| Identity | github.com/google/uuid v1.6 | Alert event IDs |
| Container | Docker + Compose | Local development with Postgres |
| CI | GitHub Actions | Test matrix with Postgres service container |

---

## Roadmap

See [ROADMAP.md](ROADMAP.md) for the full phased plan. Highlights:

- **Phase 2** — TimescaleDB hypertable as durable L2; Redis Streams for distributed ring buffer; per-day log partitioning
- **Phase 3** — Distributed query fan-out; LRU query cache; range vectors and recording rules for full PromQL compatibility
- **Phase 4** — Slack / webhook alert notifications; flap detection; silence and inhibition rules
- **Phase 5** — OpenTelemetry OTLP endpoint; cardinality limits; self-monitoring metrics
- **Phase 6** — Helm chart for AKS; multi-tenant namespace isolation; cold archive to Azure Blob (Parquet)
