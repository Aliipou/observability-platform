package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/aliipou/observability-platform/internal/alert"
	"github.com/aliipou/observability-platform/internal/models"
	"github.com/aliipou/observability-platform/internal/query"
	"github.com/aliipou/observability-platform/internal/storage"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Handler holds all dependencies for the HTTP API.
type Handler struct {
	ts     *storage.TimeSeriesStore
	pg     *storage.PostgresStore
	alerts *alert.Engine
	log    *zap.Logger
}

// New creates a new API handler.
func New(ts *storage.TimeSeriesStore, pg *storage.PostgresStore, ae *alert.Engine, log *zap.Logger) *Handler {
	return &Handler{ts: ts, pg: pg, alerts: ae, log: log}
}

// RegisterRoutes attaches all routes to the given engine.
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/v1")
	{
		// Metrics ingestion + query
		api.POST("/metrics", h.IngestMetrics)
		api.GET("/metrics/query", h.QueryMetrics)
		api.GET("/metrics/names", h.ListMetricNames)
		api.GET("/metrics/series", h.ListSeries)

		// Logs
		api.POST("/logs", h.IngestLogs)
		api.GET("/logs", h.QueryLogs)

		// Traces
		api.POST("/traces", h.IngestTraces)
		api.GET("/traces", h.QueryTraces)
		api.GET("/traces/:trace_id", h.GetTrace)

		// Alerts
		api.GET("/alerts", h.GetAlerts)
		api.GET("/alerts/history", h.GetAlertHistory)
		api.GET("/alerts/rules", h.GetAlertRules)

		// Dashboard overview
		api.GET("/overview", h.GetOverview)

		// Query expression evaluation
		api.GET("/query", h.EvalQuery)
	}

	r.Static("/web", "./web")
	r.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/web/index.html") })
}

// ── Metrics ───────────────────────────────────────────────────────────────────

func (h *Handler) IngestMetrics(c *gin.Context) {
	var batch models.MetricBatch
	if err := c.ShouldBindJSON(&batch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	now := time.Now()
	for i := range batch.Metrics {
		if batch.Metrics[i].Timestamp.IsZero() {
			batch.Metrics[i].Timestamp = now
		}
	}
	h.ts.WriteBatch(batch.Metrics)
	c.JSON(http.StatusOK, gin.H{"ingested": len(batch.Metrics)})
}

func (h *Handler) QueryMetrics(c *gin.Context) {
	name := c.Query("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	from := time.Now().Add(-1 * time.Hour)
	to := time.Now()

	if f := c.Query("from"); f != "" {
		if t, err := time.Parse(time.RFC3339, f); err == nil {
			from = t
		}
	}
	if t := c.Query("to"); t != "" {
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			to = parsed
		}
	}

	// Parse label filters: ?label[service]=api
	labels := make(map[string]string)
	for k, vals := range c.Request.URL.Query() {
		if len(k) > 6 && k[:6] == "label[" && k[len(k)-1] == ']' {
			key := k[6 : len(k)-1]
			labels[key] = vals[0]
		}
	}
	if len(labels) == 0 {
		labels = nil
	}

	series := h.ts.QuerySeries(name, labels, from, to)
	c.JSON(http.StatusOK, gin.H{"name": name, "series": series, "from": from, "to": to})
}

func (h *Handler) ListMetricNames(c *gin.Context) {
	names := h.ts.ListMetricNames()
	c.JSON(http.StatusOK, gin.H{"names": names})
}

func (h *Handler) ListSeries(c *gin.Context) {
	name := c.Query("name")
	series := h.ts.ListSeries(name, nil)
	c.JSON(http.StatusOK, gin.H{"series": series})
}

// ── Logs ──────────────────────────────────────────────────────────────────────

func (h *Handler) IngestLogs(c *gin.Context) {
	var batch models.LogBatch
	if err := c.ShouldBindJSON(&batch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if h.pg == nil {
		c.JSON(http.StatusOK, gin.H{"ingested": len(batch.Logs), "stored": false})
		return
	}
	if err := h.pg.InsertLogBatch(c.Request.Context(), batch.Logs); err != nil {
		h.log.Error("insert logs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store logs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ingested": len(batch.Logs)})
}

func (h *Handler) QueryLogs(c *gin.Context) {
	if h.pg == nil {
		c.JSON(http.StatusOK, gin.H{"logs": []interface{}{}})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	q := models.LogQuery{
		Service: c.Query("service"),
		Level:   models.LogLevel(c.Query("level")),
		Search:  c.Query("q"),
		TraceID: c.Query("trace_id"),
		From:    c.Query("from"),
		To:      c.Query("to"),
		Limit:   limit,
	}
	logs, err := h.pg.QueryLogs(c.Request.Context(), q)
	if err != nil {
		h.log.Error("query logs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// ── Traces ────────────────────────────────────────────────────────────────────

func (h *Handler) IngestTraces(c *gin.Context) {
	var batch models.SpanBatch
	if err := c.ShouldBindJSON(&batch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if h.pg == nil {
		c.JSON(http.StatusOK, gin.H{"ingested": len(batch.Spans), "stored": false})
		return
	}
	if err := h.pg.InsertSpanBatch(c.Request.Context(), batch.Spans); err != nil {
		h.log.Error("insert spans", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store traces"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ingested": len(batch.Spans)})
}

func (h *Handler) QueryTraces(c *gin.Context) {
	if h.pg == nil {
		c.JSON(http.StatusOK, gin.H{"traces": []interface{}{}})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	q := models.TraceQuery{
		Service:   c.Query("service"),
		Operation: c.Query("operation"),
		From:      c.Query("from"),
		To:        c.Query("to"),
		Limit:     limit,
	}
	if minDur := c.Query("min_duration_ms"); minDur != "" {
		q.MinDurMs, _ = strconv.Atoi(minDur)
	}
	spans, err := h.pg.QueryTraces(c.Request.Context(), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"traces": spans})
}

func (h *Handler) GetTrace(c *gin.Context) {
	if h.pg == nil {
		c.JSON(http.StatusOK, gin.H{"spans": []interface{}{}})
		return
	}
	traceID := c.Param("trace_id")
	spans, err := h.pg.GetTrace(c.Request.Context(), traceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"trace_id": traceID, "spans": spans})
}

// ── Alerts ────────────────────────────────────────────────────────────────────

func (h *Handler) GetAlerts(c *gin.Context) {
	firing := h.alerts.GetFiringAlerts()
	c.JSON(http.StatusOK, gin.H{"alerts": firing, "count": len(firing)})
}

func (h *Handler) GetAlertHistory(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	history := h.alerts.GetAlertHistory(limit)
	c.JSON(http.StatusOK, gin.H{"history": history})
}

func (h *Handler) GetAlertRules(c *gin.Context) {
	rules := h.alerts.GetRules()
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// ── Overview ──────────────────────────────────────────────────────────────────

func (h *Handler) GetOverview(c *gin.Context) {
	overview := models.DashboardOverview{
		ActiveAlerts: len(h.alerts.GetFiringAlerts()),
		TotalMetrics: len(h.ts.ListMetricNames()),
		EventsPerSec: h.ts.EventsPerSecond(),
		TotalServices: len(h.ts.ListServices()),
	}
	if h.pg != nil {
		if count, err := h.pg.CountLogs(c.Request.Context()); err == nil {
			overview.TotalLogs = count
		}
		if count, err := h.pg.CountTraces(c.Request.Context()); err == nil {
			overview.TotalTraces = count
		}
	}
	c.JSON(http.StatusOK, overview)
}

// ── Query eval ────────────────────────────────────────────────────────────────

func (h *Handler) EvalQuery(c *gin.Context) {
	expr := c.Query("q")
	if expr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q parameter required"})
		return
	}

	q, err := query.Parse(expr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	from := time.Now().Add(-q.Duration)
	to := time.Now()
	points := h.ts.Query(q.Metric, q.Labels, from, to)

	c.JSON(http.StatusOK, gin.H{
		"query":  q,
		"points": len(points),
		"from":   from,
		"to":     to,
	})
}

// ── Middleware ────────────────────────────────────────────────────────────────

func Logger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
		)
	}
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
