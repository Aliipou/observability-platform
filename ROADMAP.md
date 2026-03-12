# Observability Platform — Roadmap

A practical checklist for evolving this platform from a single-node observability
backend into a distributed, scalable, enterprise-grade system.

---

## Phase 1 — Foundation (complete ✓)

- [x] In-memory ring-buffer time series (5760 pts/series, 24h retention)
- [x] Automatic downsampling to 1-min resolution after 1h
- [x] PromQL-inspired query language (avg, sum, rate, p99, min, max, count, last)
- [x] Structured log ingestion with service/level/trace filtering + full-text search
- [x] Distributed trace storage with span parent linkage and trace reconstruction
- [x] Alert engine — YAML rules, 30s evaluation, firing/resolved state machine
- [x] Alert history persisted to PostgreSQL
- [x] Graceful degradation — fully operational without PostgreSQL
- [x] Dark web dashboard — stats, SVG line chart, log viewer, alert feed, 10s refresh
- [x] GitHub Actions CI with Postgres service container

---

## Phase 2 — Storage Scaling

### TimescaleDB Migration
- [ ] Replace plain PostgreSQL `metrics_*` tables with TimescaleDB hypertable
  `metric_samples(time TIMESTAMPTZ, name TEXT, labels JSONB, value FLOAT8)`
- [ ] Automatic time-based chunk compression (compress chunks older than 2h)
- [ ] Continuous aggregates: 1-min and 1-hour rollups for fast range queries
- [ ] Retention policy: auto-drop raw samples older than 30 days
- [ ] Keep in-memory ring buffer as L1 cache; TimescaleDB as durable L2

### Distributed In-Memory Store
- [ ] Extract `TimeSeriesStore` to an interface
- [ ] Redis Streams as optional distributed backend (for multi-instance deployments)
- [ ] Hash ring partitioning of series across Redis nodes (consistent hashing)
- [ ] Local ring buffer kept as a read-through cache on top of Redis

### Log Storage
- [ ] Migrate log storage from single `logs` table to a partitioned table by day
- [ ] Full-text search index (PostgreSQL `tsvector` or optional OpenSearch adapter)
- [ ] Log compression: LZ4 on `message` field in PostgreSQL storage

---

## Phase 3 — Query Engine

### Distributed Query
- [ ] Fan-out query to multiple platform instances, merge results
- [ ] Query federation endpoint `GET /api/v1/federate?targets=...`
- [ ] Streaming query results via chunked HTTP response for large time ranges

### PromQL Compatibility Layer
- [ ] Implement `range_vector` selector `metric[5m]`
- [ ] `increase()`, `irate()`, `histogram_quantile()` functions
- [ ] Label matching: `=~` regex and `!~` negative regex operators
- [ ] Recording rules: pre-compute expensive queries on a schedule

### Query Caching
- [ ] LRU cache (512 entries) for repeated identical query expressions
- [ ] Cache invalidation on new data ingestion for affected series
- [ ] Cache hit/miss metrics exposed in `/api/v1/overview`

---

## Phase 4 — Alerting

### Webhook & Slack Integration
- [ ] Alert notification channel config in `configs/alerts.yaml`:
  ```yaml
  notifications:
    slack:
      webhook_url: "${SLACK_WEBHOOK_URL}"
      channel: "#alerts"
    webhook:
      url: "${ALERT_WEBHOOK_URL}"
      headers: {"X-API-Key": "${WEBHOOK_KEY}"}
  ```
- [ ] Alert manager goroutine fans out to configured channels on state change
- [ ] Exponential-backoff retry (3 attempts: 1s, 2s, 4s) on delivery failure
- [ ] Notification payload includes: rule name, severity, current value, threshold, timestamp, dashboard link

### Alert Deduplication & Flap Detection
- [ ] Suppress re-notification for a firing alert within a configurable `repeat_interval` (default: 1h)
- [ ] Flap detection: mark alert as `flapping` if state changes > 3 times in 10 minutes
- [ ] `flapping` state shown separately on dashboard with distinct badge

### Alert Routing
- [ ] `routes` block in YAML — route rules by severity and label matchers to specific channels
- [ ] Silence rules: `POST /api/v1/alerts/silences` with duration + matcher
- [ ] Inhibition rules: suppress child alerts when a parent alert is firing

---

## Phase 5 — Advanced Observability

### Distributed Tracing Upgrades
- [ ] OpenTelemetry collector endpoint (`POST /v1/traces` OTLP format)
- [ ] Trace sampling: head-based 1% for high-volume services, 100% for errors
- [ ] Span attribute indexing for fast lookup by `http.method`, `db.statement`, `error`
- [ ] Flamegraph visualisation for critical-path trace rendering

### Metrics Cardinality Control
- [ ] Per-series cardinality limit (default: 10,000 unique label sets per metric name)
- [ ] Top-N cardinality report: `GET /api/v1/metrics/cardinality`
- [ ] Drop new series that exceed the cardinality limit; log dropped series count

### Self-Monitoring
- [ ] Platform exposes its own metrics to its own ingestion endpoint
  - `obs_ingestion_rate`, `obs_query_latency_p99`, `obs_series_count`, `obs_alert_firing_total`
- [ ] Watchdog alert: fire if `obs_ingestion_rate` drops to zero for > 60s

---

## Phase 6 — Cloud & Operations

### Kubernetes Deployment (AKS)
- [ ] Helm chart: single-binary server Deployment + HPA (CPU + custom metric: series count)
- [ ] Persistent Volume Claim for in-memory store snapshot (crash-safe restart)
- [ ] ConfigMap for `alerts.yaml`; Secret for DB DSN via Key Vault CSI driver
- [ ] Liveness probe: `GET /api/v1/overview` must return 200
- [ ] Readiness probe: Postgres connectivity check

### Multi-Tenant Support
- [ ] Namespace-per-tenant label on all series, logs, and traces
- [ ] Tenant isolation: queries scoped to `X-Tenant-ID` header
- [ ] Per-tenant ingestion rate limits enforced at API layer
- [ ] Tenant-specific alert rule sets loaded from DB (vs global YAML)

### Data Retention & Archival
- [ ] Configurable per-metric retention via label `__retention__`
- [ ] Archive cold series to Azure Blob Storage (Parquet via `parquet-go`)
- [ ] Restore archived series on demand for historical analysis
- [ ] Retention policy UI on dashboard (view / override per metric)

### CI/CD Improvements
- [ ] Add integration tests with Testcontainers (TimescaleDB)
- [ ] Trivy vulnerability scan on Docker image
- [ ] Automated semver tagging + release notes on `main` merge
- [ ] Helm chart publish to GitHub Pages OCI registry
