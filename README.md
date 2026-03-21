<div align="center">

<img src="https://capsule-render.vercel.app/api?type=waving&amp;color=gradient&amp;customColorList=6,14,20&amp;height=180&amp;section=header&amp;text=Observability%20Platform&amp;fontSize=40&amp;fontColor=fff&amp;animation=twinkling&amp;fontAlignY=38" />

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&amp;logo=go&amp;logoColor=white)](https://golang.org)
[![OpenTelemetry](https://img.shields.io/badge/OpenTelemetry-1.x-000000?style=flat&amp;logo=opentelemetry)](https://opentelemetry.io)
[![Prometheus](https://img.shields.io/badge/Prometheus-2.x-E6522C?style=flat&amp;logo=prometheus)](https://prometheus.io)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat)](LICENSE)

**Cloud-native observability platform with distributed tracing, metrics aggregation, and real-time alerting.**

</div>

## Why This Exists

Running microservices without observability is flying blind. Most teams bolt on monitoring as an afterthought and end up with disconnected dashboards, alert fatigue, and slow incident response. This platform was designed from the start to give you a coherent view across all services.

## What It Provides

**Distributed Tracing**
Every request is traced end-to-end across service boundaries using OpenTelemetry. You can see exactly where latency comes from and which service is causing errors.

**Metrics Aggregation**
Prometheus scrapes metrics from all services on a unified schedule. Pre-built dashboards cover the RED method (Rate, Errors, Duration) for every service automatically.

**Real-Time Alerting**
Alert rules fire within seconds of a threshold breach. Routes go to PagerDuty, Slack, or email depending on severity.

**Correlation**
Traces link to logs and metrics via trace IDs. When an alert fires, you can jump straight to the relevant traces without manually correlating timestamps.

## Architecture

```
Services (instrumented with OpenTelemetry SDK)
         |
         v
  [Collector]      Receives spans, metrics, and logs via OTLP
         |
    _____|_____
   |           |
   v           v
[Jaeger]   [Prometheus]     Storage backends
   |           |
   v           v
[Grafana]               Unified dashboards and alerting
         |
         v
  [Alert Manager]        Routes to Slack, PagerDuty, email
```

## Quick Start

```bash
git clone https://github.com/Aliipou/observability-platform.git
cd observability-platform
docker compose up -d
```

This starts the full stack: OpenTelemetry Collector, Prometheus, Grafana, Jaeger, and AlertManager.

**Grafana** at `http://localhost:3000` (admin / admin)
**Jaeger** at `http://localhost:16686`
**Prometheus** at `http://localhost:9090`

## Instrumenting Your Service

```go
package main

import (
    "context"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/sdk/trace"
)

func initTracer(ctx context.Context) (*trace.TracerProvider, error) {
    exporter, err := otlptracegrpc.New(ctx,
        otlptracegrpc.WithEndpoint("otel-collector:4317"),
        otlptracegrpc.WithInsecure(),
    )
    if err != nil {
        return nil, err
    }
    tp := trace.NewTracerProvider(
        trace.WithBatcher(exporter),
        trace.WithSampler(trace.AlwaysSample()),
    )
    otel.SetTracerProvider(tp)
    return tp, nil
}

func handleRequest(ctx context.Context) {
    tracer := otel.Tracer("my-service")
    ctx, span := tracer.Start(ctx, "handleRequest")
    defer span.End()
    // your logic here
}
```

## Alerting Rules

Pre-configured alerts for common failure modes:

| Alert | Condition | Severity |
|-------|-----------|----------|
| HighErrorRate | Error rate > 1% for 5 min | Critical |
| HighLatency | p99 latency > 500ms for 10 min | Warning |
| ServiceDown | No scrape for 2 min | Critical |
| DiskUsageHigh | Disk > 85% | Warning |

## Configuration

All configuration lives in `config/`. Key files:

```
config/
  otel-collector.yaml     OpenTelemetry Collector pipeline config
  prometheus.yml          Scrape targets and rule files
  alertmanager.yml        Routing and receiver config
  grafana/
    dashboards/           Pre-built dashboard JSON
    datasources/          Prometheus and Jaeger data sources
```

## License

MIT
