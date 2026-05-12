# 13 — Observability

> **Goal:** Make a Gin service operable — structured logs with `slog`, Prometheus metrics, OpenTelemetry traces, request IDs that thread through everything, and proper health/readiness endpoints.

---

## 1. The three pillars — mental model + working code

The phrase "logs, metrics, traces" is overused but the model is real:

- **Logs** — discrete events with structure. "What happened to *this* request?"
- **Metrics** — aggregated counters/gauges/histograms. "What's happening to *all* requests?"
- **Traces** — causal chains across services. "Where did *this* request spend its time?"

A production-grade Gin service emits all three, correlated by a **request ID** that is generated (or accepted) at the edge and threaded into every log line, every metric label that makes sense, and every span.

### A request that emits all three

```go
// Middleware order:
r.Use(
    middleware.RequestID(),       // generate or accept X-Request-ID
    otelgin.Middleware("api"),    // trace span around the request
    middleware.Metrics(),         // Prometheus counter + histogram
    middleware.SlogLogger(log),   // structured request log
    middleware.Recovery(log),
)
```

A single request hitting `GET /users/42` produces:

- **Log:** `{"time":"...","level":"INFO","msg":"http","method":"GET","path":"/users/:id","status":200,"duration_ms":12,"request_id":"abc123","trace_id":"..."}`
- **Metric tick:** `http_requests_total{method="GET",path="/users/:id",status="200"}` incremented; `http_request_duration_seconds` histogram observed.
- **Trace span:** named `GET /users/:id`, with child spans for DB queries and downstream calls, tagged with `request_id` and `user_id`.

That correlation is the entire game.

---

## 2. Structured logging — `slog` (stdlib) or `zap`

### `slog` — the modern stdlib choice (Go 1.21+)

`log/slog` shipped in Go 1.21 and is the new default. JSON or text handlers, structured fields, levels, contexts.

```go
import (
    "log/slog"
    "os"
)

log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
}))
log.Info("server starting", "port", 8080, "env", "prod")
```

```json
{"time":"2026-05-13T10:00:00Z","level":"INFO","msg":"server starting","port":8080,"env":"prod"}
```

### Why structured?

Because production log search is grep-on-text vs. queries on fields. With JSON logs in a system like Loki, Cloud Logging, Datadog Logs, you write `status >= 500 AND path = "/users/:id" AND duration_ms > 100`. That's worth more than any amount of cleverness in the message string.

### Request-scoped logger

The senior pattern: every middleware/handler in a request gets a logger with the request ID and any other context attached.

```go
// internal/http/middleware/slog.go
package middleware

import (
    "log/slog"
    "time"

    "github.com/gin-gonic/gin"
)

const loggerKey = "logger"

func SlogLogger(base *slog.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        rid := c.GetString(RequestIDKey)
        l := base.With("request_id", rid)
        c.Set(loggerKey, l)

        start := time.Now()
        path := c.Request.URL.Path
        c.Next()

        l.LogAttrs(c.Request.Context(),
            slog.LevelInfo, "http",
            slog.String("method", c.Request.Method),
            slog.String("path", path),
            slog.String("route", c.FullPath()),
            slog.Int("status", c.Writer.Status()),
            slog.Int("bytes", c.Writer.Size()),
            slog.Int64("duration_ms", time.Since(start).Milliseconds()),
            slog.String("client_ip", c.ClientIP()),
        )
    }
}

func LoggerFrom(c *gin.Context) *slog.Logger {
    if v, ok := c.Get(loggerKey); ok {
        if l, ok := v.(*slog.Logger); ok {
            return l
        }
    }
    return slog.Default()
}
```

Handlers call `middleware.LoggerFrom(c).Info(...)`; the request ID is already in every line. Pass the logger into service calls via `context.Context` (`slog`'s `LogAttrs` already takes `ctx`).

### `zap` — when you want maximum throughput

For very high-RPS services, `go.uber.org/zap` is faster than `slog` due to zero-allocation field encoders. Configure once, use everywhere. The API is similar in spirit. `slog` is good enough for 99% of services; pick `zap` only after profiling shows logging overhead matters.

### `c.FullPath()` vs `c.Request.URL.Path`

For metrics and logs, **use `c.FullPath()`**. It returns the registered template (e.g., `/users/:id`), not the realized path (`/users/42`). Metrics labels with realized paths blow up cardinality and crash Prometheus.

---

## 3. Metrics, traces, health, and the wiring

### Prometheus metrics

The classic RED method: **Rate, Errors, Duration** per endpoint.

```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

```go
// internal/http/middleware/metrics.go
package middleware

import (
    "strconv"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "http_requests_total",
        Help: "HTTP requests by method, path, status.",
    }, []string{"method", "path", "status"})

    httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "http_request_duration_seconds",
        Help:    "HTTP request duration.",
        Buckets: prometheus.DefBuckets,
    }, []string{"method", "path"})

    inflight = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "http_in_flight_requests",
        Help: "In-flight HTTP requests.",
    })
)

func Metrics() gin.HandlerFunc {
    return func(c *gin.Context) {
        inflight.Inc()
        defer inflight.Dec()

        start := time.Now()
        c.Next()

        path := c.FullPath()
        if path == "" {
            path = "unknown"                // requests that didn't match a route
        }
        status := strconv.Itoa(c.Writer.Status())
        httpRequests.WithLabelValues(c.Request.Method, path, status).Inc()
        httpDuration.WithLabelValues(c.Request.Method, path).Observe(time.Since(start).Seconds())
    }
}
```

Expose `/metrics`:

```go
import "github.com/prometheus/client_golang/prometheus/promhttp"

r.GET("/metrics", gin.WrapH(promhttp.Handler()))
```

`gin.WrapH` adapts a `http.Handler` to a `gin.HandlerFunc`. Same with `gin.WrapF` for `http.HandlerFunc`.

```bash
curl http://localhost:8080/metrics | grep http_requests_total
# http_requests_total{method="GET",path="/users/:id",status="200"} 17
```

### Cardinality discipline

The single most common metric mistake: high-cardinality labels. **Bad:** `path="/users/42"`, `user_id="42"`, `error_message="..."`. **Good:** `path="/users/:id"`, label by route template, log details, don't label them.

### OpenTelemetry tracing

```bash
go get go.opentelemetry.io/otel
go get go.opentelemetry.io/otel/sdk
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc
go get go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin
```

```go
// internal/obs/tracing.go
package obs

import (
    "context"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

func InitTracer(ctx context.Context, service, version string) (func(context.Context) error, error) {
    exp, err := otlptracegrpc.New(ctx)        // reads OTEL_EXPORTER_OTLP_ENDPOINT env
    if err != nil {
        return nil, err
    }
    res, _ := resource.New(ctx,
        resource.WithAttributes(
            semconv.ServiceName(service),
            semconv.ServiceVersion(version),
        ),
    )
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exp),
        sdktrace.WithResource(res),
    )
    otel.SetTracerProvider(tp)
    return tp.Shutdown, nil
}
```

Wire the Gin middleware:

```go
import "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

r.Use(otelgin.Middleware("hello-gin"))
```

Now every request has a span, child spans flow from downstream calls (DB, HTTP) when those are instrumented (`otelsql`, `otelhttp`). Traces ship to whatever OTLP collector you point at (Jaeger, Tempo, Honeycomb, Datadog, etc.).

To correlate logs with traces, add `trace_id` to log fields:

```go
sc := trace.SpanContextFromContext(c.Request.Context())
if sc.IsValid() {
    l = l.With("trace_id", sc.TraceID().String())
}
```

This is the wiring that turns a JSON log line into a clickable jump to the trace UI.

### Health and readiness

Two separate endpoints, two different jobs.

```go
r.GET("/healthz", func(c *gin.Context) {
    c.JSON(200, gin.H{"status": "ok"})
})

r.GET("/readyz", func(c *gin.Context) {
    ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
    defer cancel()
    if err := db.PingContext(ctx); err != nil {
        c.JSON(503, gin.H{"status": "degraded", "db": err.Error()})
        return
    }
    c.JSON(200, gin.H{"status": "ready"})
})
```

- **`/healthz` (liveness)** — "is the process responsive?" Should be cheap (no DB call), used by Kubernetes to decide whether to restart the pod.
- **`/readyz` (readiness)** — "can I serve traffic?" Checks dependencies. K8s uses it to gate traffic.

Don't conflate them. A briefly-broken DB shouldn't kill the pod (liveness fails → restart loop) — it should just remove it from rotation (readiness fails → no new traffic).

### Excluding health checks from logs and metrics

Health endpoints get hit every second by load balancers and Kubernetes. Logging them swamps real signal; metering them inflates request counts. Exclude:

```go
func Metrics() gin.HandlerFunc {
    return func(c *gin.Context) {
        if c.FullPath() == "/healthz" || c.FullPath() == "/readyz" {
            c.Next()
            return
        }
        // ... normal path
    }
}
```

Same for the logger.

---

## 4. Practical application — observable Gin service in ~80 lines

```go
// cmd/api/main.go
package main

import (
    "context"
    "database/sql"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

    "github.com/you/hello-gin/internal/http/middleware"
    "github.com/you/hello-gin/internal/obs"
)

func main() {
    log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
    slog.SetDefault(log)

    rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    shutdownTrace, err := obs.InitTracer(rootCtx, "hello-gin", "v1")
    if err != nil {
        log.Warn("tracer init failed", "err", err)
    } else {
        defer shutdownTrace(context.Background())
    }

    db := mustOpenDB()
    defer db.Close()

    r := gin.New()
    r.Use(
        middleware.RequestID(),
        otelgin.Middleware("hello-gin"),
        middleware.Metrics(),
        middleware.SlogLogger(log),
        middleware.Recovery(log),
    )

    r.GET("/metrics", gin.WrapH(promhttp.Handler()))
    r.GET("/healthz", liveness)
    r.GET("/readyz",  readiness(db))

    // ... business routes ...

    srv := &http.Server{
        Addr:              ":8080",
        Handler:           r,
        ReadHeaderTimeout: 5 * time.Second,
        ReadTimeout:       15 * time.Second,
        WriteTimeout:      30 * time.Second,
        IdleTimeout:       60 * time.Second,
    }

    go func() {
        log.Info("listening", "addr", srv.Addr)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Error("server crashed", "err", err)
        }
    }()

    <-rootCtx.Done()
    log.Info("shutting down")
    shutCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
    defer cancel()
    _ = srv.Shutdown(shutCtx)
}

func liveness(c *gin.Context) {
    c.JSON(200, gin.H{"status": "ok"})
}

func readiness(db *sql.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
        defer cancel()
        if err := db.PingContext(ctx); err != nil {
            c.JSON(503, gin.H{"status": "degraded", "db": err.Error()})
            return
        }
        c.JSON(200, gin.H{"status": "ready"})
    }
}

func mustOpenDB() *sql.DB { /* ... */ return nil }
```

This is a real shape — about 80 lines of glue produce a service that's logged, metered, traced, healthchecked, and gracefully shut-downable. Add handlers; everything else is wiring.

### Try it

```bash
GIN_MODE=release \
OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317" \
go run ./cmd/api

# in another terminal
curl http://localhost:8080/api/v1/users/1
# logs include trace_id and request_id; trace appears in your collector

curl http://localhost:8080/metrics | grep -E '^http_'
# http_requests_total{...} 1
# http_request_duration_seconds_bucket{...} ...
```

---

## 5. Common mistakes & gotchas

- **Using `c.Request.URL.Path` as a metric label.** Cardinality explodes (`/users/1`, `/users/2`, ...). Always use `c.FullPath()` (the route template) for metric labels.
- **Logging the health-check on every probe.** Kubernetes hits `/healthz` every 10 seconds. That's 8,640 log lines a day per pod. Skip them.
- **Logging request bodies.** Often contains PII or secrets. Don't, unless you've redacted or you have explicit consent. Even then, redact on the way in.
- **Metrics endpoint behind auth.** It needs to be scrape-able by Prometheus. Either expose it on a separate admin port, or allow-list the scraper's source IP. Don't put `RequireJWT` on `/metrics`.
- **Trace IDs in logs without a span context.** If you log `trace_id` from a goroutine that didn't get the context, it will be empty. Pass `context.Context` (and use `slog`'s `LogAttrs(ctx, ...)` form) everywhere.
- **Health check that talks to every dependency.** A `/healthz` that pings the DB and three downstream services means any flake kills the pod. Keep `/healthz` cheap; put dependency checks in `/readyz` (which only affects traffic, not the pod).
- **No `slog.SetDefault(log)`.** Libraries that call `slog.Info(...)` directly use the default logger. If you haven't set it, they go to stderr in the default text format. Always set the default.
- **`time.Now()` per request without a fast clock.** For >10k RPS, the cost of `time.Now()` shows up in profiles. Use a clock cache (`atomic.Pointer[time.Time]` updated by a ticker) only if profiling demands it.
- **Forgetting graceful tracer shutdown.** OTel's batch exporter flushes on `tp.Shutdown(ctx)`. If you don't call it, the last few seconds of traces are lost. Always `defer shutdownTrace(...)`.

---

## 🎯 Key Takeaways

1. **Logs, metrics, and traces share one identifier — `request_id` (and `trace_id`).** That correlation is the difference between "I can find the problem in 60 seconds" and "I'll need to grep three systems for an hour."
2. **`c.FullPath()` for metric labels, not `c.Request.URL.Path`.** Cardinality discipline is the most important metrics rule and the one beginners always violate.
3. **Liveness vs readiness are different things.** `/healthz` is cheap and only fails when the process should be killed. `/readyz` checks dependencies and only gates traffic. K8s uses both differently; conflating them creates restart loops.
4. **`slog` is the new stdlib default.** Use `JSONHandler` to stdout, attach the request logger via middleware, let your platform (Cloud Logging, Loki, etc.) handle the rest. `zap` only if profiling proves you need it.
5. **OpenTelemetry's `otelgin.Middleware` plus instrumented DB and HTTP clients give you distributed traces almost for free.** This is the single biggest senior-vs-junior delta in operational maturity for a Go HTTP service.

*← [12 — Concurrency Patterns](./12_concurrency.md) | [14 — API Design with Gin →](./14_api_design.md)*
