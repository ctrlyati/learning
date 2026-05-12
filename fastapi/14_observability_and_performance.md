# 14 — Observability & Performance

> **Goal:** Instrument FastAPI with structured logs, Prometheus metrics, OpenTelemetry traces, and have the tools to find — and fix — performance bottlenecks before users do.

---

## 1. Concept — You can't fix what you can't see

The three pillars:

- **Logs** — discrete events. "user 42 logged in." Useful for forensic debugging.
- **Metrics** — aggregated numbers over time. "p99 latency, request rate, error rate." Useful for alerting and trends.
- **Traces** — request-scoped causal chains. "this 800ms request was 50ms in the app, 600ms in the DB, 150ms in the LLM call." Useful for finding the *where* of slowness.

In FastAPI, all three are wired in as middleware or instrumentation libraries, with little code change. The patterns below are what mature shops run.

```python
import structlog
logger = structlog.get_logger()

logger.info("user_login", user_id=42, method="oauth", duration_ms=120)
# {"event": "user_login", "user_id": 42, "method": "oauth", "duration_ms": 120,
#  "timestamp": "2026-05-13T10:00:00Z", "level": "info"}
```

That's a structured log line. Searchable in any log platform, alertable in Datadog/Grafana Loki, joinable on `request_id`.

---

## 2. Mechanism — Each pillar has a canonical Python stack

| Pillar  | Library           | What it does                                                 |
| ------- | ----------------- | ------------------------------------------------------------ |
| Logs    | `structlog`       | Structured, context-aware, ships JSON                        |
| Metrics | `prometheus-client` + `prometheus-fastapi-instrumentator` | Counters/histograms, `/metrics` endpoint |
| Traces  | OpenTelemetry SDK + `opentelemetry-instrumentation-fastapi` | Spans, exporters to Jaeger/Tempo/etc. |
| APM     | Sentry / Datadog APM / New Relic | One-line integration, paid platforms          |

They don't conflict — most teams run all three.

---

## 3. Variations & Depth

### Structured logging with `structlog`

```python
# app/core/logging.py
import logging
import structlog


def configure_logging(level: str = "INFO") -> None:
    timestamper = structlog.processors.TimeStamper(fmt="iso")
    shared = [
        structlog.contextvars.merge_contextvars,
        structlog.stdlib.add_log_level,
        structlog.stdlib.add_logger_name,
        timestamper,
        structlog.processors.StackInfoRenderer(),
        structlog.processors.format_exc_info,
    ]
    structlog.configure(
        processors=shared + [structlog.processors.JSONRenderer()],
        wrapper_class=structlog.make_filtering_bound_logger(getattr(logging, level)),
        context_class=dict,
        logger_factory=structlog.PrintLoggerFactory(),
        cache_logger_on_first_use=True,
    )
    logging.basicConfig(level=level, format="%(message)s")
```

In `main.py`:

```python
configure_logging("INFO")
```

In any module:

```python
import structlog
logger = structlog.get_logger("orders")

logger.info("order_created", order_id=42, total_cents=1999, user_id=1)
```

Bind request-scoped context in middleware (we did this in module 10) — every subsequent log line inside that request inherits `request_id`, `user_id`, etc.

```python
structlog.contextvars.bind_contextvars(request_id=rid, user_id=user_id)
# ... endpoint runs, logs include both ...
structlog.contextvars.clear_contextvars()
```

### Prometheus metrics

```bash
uv pip install prometheus-fastapi-instrumentator
```

```python
from prometheus_fastapi_instrumentator import Instrumentator

Instrumentator().instrument(app).expose(app, endpoint="/metrics", include_in_schema=False)
```

You get, for free:

- `http_requests_total{method, handler, status}`
- `http_request_duration_seconds{method, handler}` (histogram → p50/p95/p99)
- `http_request_size_bytes`, `http_response_size_bytes`

Then in Grafana:

```promql
# request rate
sum by (handler) (rate(http_requests_total[5m]))

# p95 latency
histogram_quantile(0.95, sum by (le, handler) (rate(http_request_duration_seconds_bucket[5m])))

# error rate
sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m]))
```

Custom business metrics:

```python
from prometheus_client import Counter, Histogram

ORDERS_CREATED = Counter("orders_created_total", "Orders created", ["currency"])
ORDER_PROCESS_TIME = Histogram(
    "order_process_seconds", "Time to process an order",
    buckets=(0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30),
)


@app.post("/orders")
async def create_order(...) -> ...:
    with ORDER_PROCESS_TIME.time():
        # ... do work ...
        ORDERS_CREATED.labels(currency="USD").inc()
```

Alerting golden signals: **rate, errors, duration, saturation (queue depth, pool usage)**.

### OpenTelemetry tracing

```bash
uv pip install opentelemetry-distro opentelemetry-instrumentation-fastapi \
              opentelemetry-instrumentation-sqlalchemy opentelemetry-instrumentation-httpx \
              opentelemetry-exporter-otlp
```

```python
# app/core/tracing.py
from opentelemetry import trace
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.instrumentation.sqlalchemy import SQLAlchemyInstrumentor
from opentelemetry.instrumentation.httpx import HTTPXClientInstrumentor


def configure_tracing(app, service_name: str, otlp_endpoint: str) -> None:
    resource = Resource(attributes={"service.name": service_name})
    provider = TracerProvider(resource=resource)
    provider.add_span_processor(
        BatchSpanProcessor(OTLPSpanExporter(endpoint=otlp_endpoint, insecure=True))
    )
    trace.set_tracer_provider(provider)

    FastAPIInstrumentor.instrument_app(app)
    HTTPXClientInstrumentor().instrument()
    # Call after engine is created:
    # SQLAlchemyInstrumentor().instrument(engine=engine)
```

Run an OTel collector locally (Docker), point it at Jaeger/Tempo/Honeycomb. Every request becomes a trace with spans for FastAPI, every DB query, every outbound httpx call. Click into a slow request, see exactly where time went.

To add custom spans:

```python
from opentelemetry import trace
tracer = trace.get_tracer(__name__)


async def fetch_user_data(user_id: int) -> dict:
    with tracer.start_as_current_span("fetch_user_data") as span:
        span.set_attribute("user.id", user_id)
        # ... work ...
        return result
```

### Profiling

When metrics say "this endpoint is slow but I don't know why":

- **`py-spy`** — sampling profiler, no code changes, attaches to a running process:
  ```bash
  py-spy record -o profile.svg --pid <pid> --duration 60
  ```
- **`scalene`** — better attribution for Python vs. C / CPU vs. memory.
- **`cProfile`** in tests — for isolating a function:
  ```python
  import cProfile
  cProfile.run("expensive_function()", sort="cumulative")
  ```

For async code: use `py-spy --native --threads`. The async machinery adds frames; learn to read past them.

### Common bottlenecks (in approximate order)

1. **Synchronous DB driver inside `async def`** (module 09).
2. **N+1 queries** (module 07).
3. **Missing DB indexes** — `EXPLAIN ANALYZE` your slow queries.
4. **Cold connection pool** under load spikes.
5. **JSON serialization for huge payloads** — pagination is your friend.
6. **Pydantic validation on enormous request bodies** — limit body size at the proxy and in endpoints.
7. **CPU-bound work (regex, image processing, ML inference)** on the API process — push to workers.
8. **Calling external services without parallelism** — `asyncio.gather`.
9. **No HTTP keep-alive between services** — short-lived `httpx.AsyncClient` per request.
10. **Wrong worker count** — too few = under-utilized CPU; too many = thrashing. Start with `(cpu_count * 2) + 1`, measure.

---

## 4. Practical Application — Wiring all three pillars into one app

```python
# app/main.py (excerpt — combines with module 10 setup)
import structlog
from prometheus_fastapi_instrumentator import Instrumentator

from app.core.config import settings
from app.core.logging import configure_logging
from app.core.tracing import configure_tracing

configure_logging(settings.log_level)
logger = structlog.get_logger("app")


app = FastAPI(...)

# Prometheus
Instrumentator().instrument(app).expose(
    app, endpoint="/metrics", include_in_schema=False
)

# OpenTelemetry
if settings.otel_enabled:
    configure_tracing(app, settings.service_name, settings.otel_endpoint)

# ... CORS, error handlers, routers from module 10 ...
```

```python
# app/services/orders.py — instrumented business logic
import structlog
from opentelemetry import trace
from prometheus_client import Counter

from app.models.orders import Order

logger = structlog.get_logger("orders")
tracer = trace.get_tracer(__name__)

ORDERS_CREATED = Counter("orders_created_total", "Orders created", ["currency"])


async def create_order(db, user_id: int, items: list[dict], currency: str) -> Order:
    with tracer.start_as_current_span("orders.create") as span:
        span.set_attribute("user.id", user_id)
        span.set_attribute("orders.item_count", len(items))

        order = Order(user_id=user_id, currency=currency)
        db.add(order)
        await db.flush()

        ORDERS_CREATED.labels(currency=currency).inc()
        logger.info(
            "order_created",
            order_id=order.id,
            user_id=user_id,
            currency=currency,
            item_count=len(items),
        )
        return order
```

Now: a single request produces a JSON log line (searchable), a counter increment (graphable, alertable), and a trace span (drillable). Three views of the same event.

**Verify locally**

```bash
# logs — stdout in dev
curl http://localhost:8000/api/v1/orders -X POST ...
# {"event":"order_created","order_id":1,...}

# metrics
curl http://localhost:8000/metrics | grep orders_created_total
# orders_created_total{currency="USD"} 1.0

# traces — visit your Jaeger UI at localhost:16686
```

### Health checks

Two flavors, both needed:

```python
@app.get("/health/live", tags=["meta"], include_in_schema=False)
def liveness() -> dict:
    return {"status": "ok"}


@app.get("/health/ready", tags=["meta"], include_in_schema=False)
async def readiness(db: DBSession) -> dict:
    # check critical deps
    await db.execute(text("SELECT 1"))
    # ping redis, etc.
    return {"status": "ok"}
```

- **Liveness**: "am I alive at all?" — used by k8s to restart. Should *not* fail just because DB is down (restarting won't help).
- **Readiness**: "should I receive traffic?" — used by k8s to gate routing. Should fail if any required dep is down.

---

## 5. Common Mistakes & Gotchas

- **Logging an entire request body or response.** PII, secrets, GDPR. Log structured event names + safe fields. If you must capture bodies in dev, redact in prod via a logging filter.
- **`print()` instead of structured logging.** Stdout in a container is OK for the *format*, but `print` doesn't add timestamps, levels, or context. Use a logger.
- **One Prometheus histogram per user/order ID** — explodes the label cardinality. Stick to low-cardinality labels (method, status, endpoint).
- **`/metrics` exposed publicly.** Tells attackers your stack, your endpoints, your error rates. Restrict by IP or auth.
- **Sampling traces at 100% in production.** Expensive. Sample 1–10% of traffic, 100% of errors (using a tail-based sampler).
- **Profiling on a dev box and assuming prod behaves the same.** Cache hit rates, network latency, DB indexes — all different. Profile in staging at minimum.
- **No SLOs.** Without targets, every dashboard is decoration. Define p95 < 300ms for the critical path. Alert when error budget burns.
- **Catch-all `except Exception: logger.error(...)` without re-raising.** Hides the bug, returns a 200 OK, ruins your day six months later. Either re-raise or convert deliberately.
- **Confusing readiness and liveness.** Restart loops because liveness fails on a DB outage. Or no traffic gating because readiness is wired to liveness.
- **Trace context not propagating through async task queues.** Out of the box, Celery/Arq workers start fresh contexts. Use OpenTelemetry's distributed propagation explicitly.

---

## 🎯 Key Takeaways

- **Structured logs, Prometheus metrics, OpenTelemetry traces — pick all three.** They answer different questions; you'll need each in different incidents.
- **`request_id` in every log line, every span, every error response.** It's the join key that turns three disconnected pillars into one searchable story.
- **Define SLOs before dashboards.** "p95 latency < 300ms, error rate < 0.1%" — measure against numbers, not vibes.
- **Most FastAPI perf problems are I/O, not CPU.** Profile-led optimization in async code starts with "what am I awaiting and how long does it take?"
- **Observability is a Day-1 task, not a Day-90 one.** Adding it after an incident is double the work and the wrong learning curve.

*← [prev](./13_openapi_customization_and_client_generation.md) | [next →](./15_production.md)*
