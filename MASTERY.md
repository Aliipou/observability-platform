# Mastery Engineering Audit — Observability Platform

> "Simplicity under pressure = mastery"

## Project Summary
A cloud-native observability platform built in Go. Ingests spans, metrics, and logs via OpenTelemetry Collector; stores metrics in an in-memory ring-buffer time series store (24h retention); persists alert events in PostgreSQL; evaluates alert rules on a configurable interval; and routes alerts to Slack, PagerDuty, or email. Grafana and Jaeger provide dashboards and trace UIs.

## Failure Mode Analysis

### What breaks when the main service goes down?
The Go backend process hosts the metrics ingestion endpoint, alert engine, and API in a single binary. All three stop simultaneously. Prometheus continues scraping (it buffers remotely) but metrics written via OTLP to the Go backend are lost during downtime — the in-memory ring buffer is not persisted to disk. Alert evaluation also stops, meaning threshold breaches during downtime generate no alerts.

### What breaks when the database/storage slows down?
PostgreSQL is used only for alert event persistence. The in-memory TimeSeriesStore is the hot path and is unaffected by Postgres latency. However, alert events that fire during a Postgres slowdown will fail to persist — the engine logs the error but the firing state is only held in the in-memory `firing` map. On process restart, all firing alert state is lost regardless of DB health.

### What breaks when the network is partitioned?
If the OpenTelemetry Collector cannot reach the backend, spans and metrics are buffered in the Collector retry queue (up to configured limits) and replayed when connectivity restores. If Prometheus cannot reach the backend /metrics, scrape intervals are missed. The alert engine is self-contained and continues evaluating against the in-memory store. Alertmanager webhook routes to Slack/PagerDuty will fail silently if those external endpoints are unreachable.

### What breaks under duplicate execution?
Alert engine runs in a single goroutine per rule evaluation cycle protected by a `sync.RWMutex`. The `firing` map deduplicates: a rule already in the firing map does not generate a second alert event. This is correct. However, if the process restarts while an alert is firing, the alert will re-fire on the next evaluation as if new — the resolved state is not recovered from the database on startup.

## Checklist Status

| Check | Status | Notes |
|-------|--------|-------|
| Invariants defined | ✅ | Alert states (firing/resolved) are well-defined; ring buffer RingSize cap prevents unbounded memory growth |
| Idempotency | ⚠️ | Alert deduplication works in-process, but restart loses firing state — alerts re-fire spuriously after restart |
| Race conditions handled | ✅ | TimeSeriesStore and alert engine both use sync.RWMutex correctly; ring buffer head/count are mutex-protected |
| State consistency | ⚠️ | In-memory metric store is the source of truth but is not durable; a crash loses up to 24h of metric history |
| Structured logging | ✅ | zap structured logging with rule name, severity, and metric values on alert events |
| Metrics (not just logs) | ✅ | Prometheus scraping, Grafana dashboards, and RED method coverage are core features of the platform itself |
| Distributed tracing | ✅ | OpenTelemetry + Jaeger is the central capability; trace-to-log correlation via trace IDs is documented |
| Rollback strategy | ⚠️ | Alert rules are loaded from YAML file — changing a rule file and restarting is the only deploy mechanism; no versioning |
| Safe migrations | ⚠️ | Postgres migrations exist but the migration runner strategy is not visible; no golang-migrate version table confirmed |
| 10x traffic plan | ⚠️ | In-memory ring buffer is bounded at RingSize=5760 points/series and 24h, but unbounded series count (one per label set) will cause memory growth at 10x service count |
| Bottleneck identified | ⚠️ | The in-memory TimeSeriesStore global write lock (sync.Mutex on every Write) becomes contention under high-frequency metric ingestion from many services |
| Simplicity test passed | ✅ | Core value (OTel + Prometheus + Jaeger + AlertManager) is delivered via Docker Compose — genuinely simple to run |

## Critical Gaps (Must Fix)

1. **In-memory metric store is not durable**: a process crash loses all metric history. For production use, the backend should write metrics to a persistent store (Prometheus remote-write to Thanos/Cortex, or at minimum persist the ring buffer to disk periodically).
2. **Alert firing state lost on restart**: the `firing` map is not persisted to PostgreSQL at shutdown or restarted from DB on startup. Alerts re-fire spuriously after every process restart, causing alert fatigue.
3. **Unbounded series count**: the TimeSeriesStore allocates a new `series` struct for every unique (name, labels) combination with no eviction policy. At 10x service count with high-cardinality labels this causes unbounded memory growth.
4. **No self-observability metrics**: the platform observes other services but exposes no /metrics about itself — ingestion rate, evaluation latency, alert counts, and memory usage of the ring buffer are invisible.
5. **Alert rule hot-reload**: changing alert rules requires a process restart, dropping all in-flight state. Implement a SIGHUP or API-triggered reload that preserves the firing map.

## What is Already Mastery-Level

- **Ring buffer with downsampling**: the TimeSeriesStore uses a fixed-size ring buffer (5760 slots) and downsamples data older than 1 hour to 1-minute resolution — memory is bounded and query performance is predictable.
- **Alert deduplication**: the `firing` map prevents duplicate alert events for the same rule within a process lifetime.
- **Full OTel stack**: OpenTelemetry Collector, Prometheus, Grafana, Jaeger, and AlertManager all compose cleanly via Docker Compose — the platform is genuinely useful out of the box.
- **YAML-driven alert rules**: rules are data, not code — operators can add rules without redeploying.
- **Trace-to-log correlation**: trace IDs propagated through structured log fields enable jump-from-alert-to-trace workflows.

## Recommended Next Steps

1. **Persist alert firing state**: on alert fire/resolve, write state to Postgres; on startup, reload firing state from Postgres to restore pre-crash context.
2. **Add series eviction**: implement an LRU or TTL-based eviction policy in TimeSeriesStore so series for decommissioned services do not accumulate indefinitely.
3. **Self-observability /metrics**: expose platform internals — ingestion events/s, alert evaluation duration, ring buffer utilization, active series count — as Prometheus metrics so the platform can monitor itself.
4. **Persistent metric backend**: add optional Prometheus remote-write support so metrics survive process restarts without requiring external infrastructure changes.
5. **Alert rule hot-reload via API**: expose a `PUT /api/v1/rules` endpoint (or SIGHUP handler) that atomically swaps the rule set without losing firing state.
