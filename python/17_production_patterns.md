# 17 — Production Patterns
> **Goal:** Ship Python that survives contact with reality — structured logging, typed config, secret handling, project layout, and observability.

This module is the gap between "works on my laptop" and "runs reliably in production." None of it is fancy; all of it is what seniors do reflexively.

## 1. Structured logging — JSON, levels, context

Plain `logging` (module 10) gets you `INFO: user logged in`. Production wants structured, machine-readable lines with correlation IDs:

```json
{"ts":"2026-05-11T12:01:02Z","level":"INFO","msg":"user_login","user_id":42,"trace_id":"abc123"}
```

The 2026 stack: `structlog` for code ergonomics, JSON renderer for output, then ship to a log aggregator (Loki, Datadog, CloudWatch, etc.).

```bash
pip install structlog
```

```python
import logging
import structlog

def setup_logging(level: str = "INFO") -> None:
    logging.basicConfig(level=level, format="%(message)s")
    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,    # auto-include context
            structlog.processors.add_log_level,
            structlog.processors.TimeStamper(fmt="iso", utc=True),
            structlog.processors.StackInfoRenderer(),
            structlog.processors.format_exc_info,
            structlog.processors.JSONRenderer(),         # machine-readable
        ],
        wrapper_class=structlog.make_filtering_bound_logger(
            getattr(logging, level)
        ),
    )

log = structlog.get_logger()

setup_logging("INFO")
log.info("user_login", user_id=42)
# {"event": "user_login", "user_id": 42, "level": "info", "timestamp": "2026-05-11T12:01:02Z"}
```

### Context propagation

`structlog.contextvars` adds fields that show up on every log call inside the context — perfect for trace IDs in HTTP handlers:

```python
from structlog.contextvars import bind_contextvars, clear_contextvars
import uuid

@app.middleware("http")
async def add_trace_id(request, call_next):
    bind_contextvars(trace_id=str(uuid.uuid4()))
    try:
        return await call_next(request)
    finally:
        clear_contextvars()

# Anywhere downstream:
log.info("processing", endpoint=request.url.path)
# trace_id appears automatically
```

For local development, swap `JSONRenderer` for `ConsoleRenderer` (pretty colored output). Same code, different processor.

## 2. Configuration — typed, layered, env-driven

Twelve-factor: **config in environment variables**. But raw `os.environ.get(...)` everywhere is brittle. Use `pydantic-settings`:

```bash
pip install pydantic-settings
```

```python
# config.py
from pydantic import Field, SecretStr
from pydantic_settings import BaseSettings, SettingsConfigDict
from functools import lru_cache

class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        env_prefix="MYAPP_",
        case_sensitive=False,
        extra="ignore",
    )

    # Required
    database_url: str
    api_key: SecretStr

    # Optional with defaults
    log_level: str = "INFO"
    request_timeout_s: float = 10.0
    max_workers: int = Field(default=8, ge=1, le=64)

@lru_cache
def settings() -> Settings:
    return Settings()       # validated on first access
```

Now anywhere:

```python
from .config import settings
log.info("starting", db=settings().database_url)
```

Why this is good:

- **Validated at startup.** A missing `MYAPP_DATABASE_URL` fails immediately with a clear message — not 5 minutes into a request when it's first used.
- **Typed.** `settings().request_timeout_s` is a `float`. Not a string you `float()`-cast at the call site.
- **`SecretStr`** masks secrets in logs and `repr()` — fewer accidental leaks.
- **`.env` for local dev**, real env vars in production.

### Layered config

`.env` for development, real env vars in CI/prod. Never commit `.env` — add to `.gitignore`. Commit a `.env.example` instead, with placeholder values, as documentation.

## 3. Secrets — never in code, never in logs

Rules in priority order:

1. **No secrets in source.** Ever. Use env vars, vaults, or KMS.
2. **No secrets in logs.** Wrap in `SecretStr`; sanitize before logging.
3. **No secrets in error messages or tracebacks** that leave the process. Sanitize at the boundary.
4. **No secrets in docker images / build artifacts.** Pass at runtime via env or mounts.
5. **No secrets in version control history.** If you slip up, rotate immediately — `git filter-branch` doesn't undo a leak.

For real secret management:

- **Local dev:** `.env` file (gitignored).
- **CI:** GitHub Actions / GitLab CI secrets.
- **Cloud:** AWS Secrets Manager, Google Secret Manager, HashiCorp Vault. Pull at startup, not per-request, and cache.

For pre-commit safety, use `detect-secrets` or `gitleaks` in your hooks. For dependency vulnerability scanning, `pip-audit` or `safety`.

## 4. Project layout for production

A real-world layout, building on modules 5 and 14:

```
myservice/
├── pyproject.toml
├── README.md
├── .env.example
├── .gitignore
├── .pre-commit-config.yaml
├── Dockerfile
├── docker-compose.yml          # dev DB, etc.
├── src/
│   └── myservice/
│       ├── __init__.py
│       ├── __main__.py
│       ├── config.py
│       ├── logging.py
│       ├── observability.py    # metrics, tracing setup
│       ├── api/
│       │   ├── __init__.py
│       │   ├── app.py          # FastAPI app factory
│       │   ├── deps.py         # Depends() functions
│       │   └── routes/
│       │       └── users.py
│       ├── domain/             # business logic, no I/O
│       │   ├── __init__.py
│       │   └── models.py
│       ├── adapters/           # I/O: db, http, queue
│       │   ├── __init__.py
│       │   ├── db.py
│       │   └── email.py
│       └── services/           # orchestration: domain + adapters
│           ├── __init__.py
│           └── users.py
├── tests/
│   ├── conftest.py
│   ├── unit/
│   └── integration/
└── ops/
    ├── alembic/                # DB migrations
    └── k8s/                    # deployment manifests
```

Layering rules (pure architecture, language-agnostic):

- **`domain/`** depends on nothing else. Pure types and rules.
- **`adapters/`** depends on `domain/` and external libraries. Wraps the world.
- **`services/`** depends on `domain/` and `adapters/`. Orchestrates use cases.
- **`api/`** depends on `services/`. Translates HTTP ↔ services.

This shape (a.k.a. hexagonal/ports-and-adapters) keeps tests easy: swap an adapter for a fake (module 13), services unchanged.

### Dockerfile (multi-stage, slim)

```dockerfile
FROM python:3.13-slim AS builder
WORKDIR /app
RUN pip install --no-cache-dir build
COPY pyproject.toml README.md ./
COPY src ./src
RUN python -m build --wheel

FROM python:3.13-slim
WORKDIR /app
COPY --from=builder /app/dist/*.whl /tmp/
RUN pip install --no-cache-dir /tmp/*.whl && rm /tmp/*.whl
ENV PYTHONUNBUFFERED=1
USER 1000
CMD ["python", "-m", "myservice"]
```

Highlights: separate build stage so build deps don't ship; `--no-cache-dir` keeps the image small; `PYTHONUNBUFFERED=1` so logs flush immediately; non-root `USER` for safety.

### Pre-commit hooks

```yaml
# .pre-commit-config.yaml
repos:
  - repo: https://github.com/astral-sh/ruff-pre-commit
    rev: v0.5.0
    hooks:
      - id: ruff
      - id: ruff-format
  - repo: https://github.com/pre-commit/mirrors-mypy
    rev: v1.10.0
    hooks:
      - id: mypy
        additional_dependencies: [pydantic, types-requests]
  - repo: https://github.com/Yelp/detect-secrets
    rev: v1.5.0
    hooks:
      - id: detect-secrets
```

```bash
pip install pre-commit && pre-commit install
```

Now lint, format, type-check, and secret-scan run on every commit. Catches issues before CI does.

## 5. Observability — logs, metrics, traces

The "three pillars." All cheap to add at the start, painful to retrofit.

### Metrics

`prometheus-client` is the de facto for Prometheus / Grafana stacks:

```python
from prometheus_client import Counter, Histogram, start_http_server

REQUESTS = Counter("http_requests_total", "HTTP requests", ["method", "path", "status"])
LATENCY  = Histogram("http_request_duration_seconds", "request latency", ["path"])

@app.middleware("http")
async def metrics_middleware(request, call_next):
    with LATENCY.labels(request.url.path).time():
        response = await call_next(request)
    REQUESTS.labels(request.method, request.url.path, response.status_code).inc()
    return response

start_http_server(9090)    # /metrics endpoint for scraping
```

For SaaS metrics (Datadog, NewRelic), use their SDKs — same idea, different transport.

### Tracing — OpenTelemetry

OpenTelemetry is the vendor-neutral standard for distributed tracing:

```bash
pip install opentelemetry-distro opentelemetry-exporter-otlp
opentelemetry-bootstrap -a install     # auto-installs instrumentations
```

```bash
opentelemetry-instrument --service_name=myservice --traces_exporter=otlp \
    python -m myservice
```

Auto-instruments FastAPI, httpx, SQLAlchemy, requests, and many more. You get end-to-end traces (HTTP request → DB query → outbound call) for free, then ship them to Jaeger, Tempo, Honeycomb, etc.

### Health checks and readiness

```python
@app.get("/healthz")
def liveness():
    return {"status": "ok"}

@app.get("/readyz")
def readiness(db: DB):
    try:
        db.execute("SELECT 1")
    except Exception:
        return {"status": "not ready"}, 503
    return {"status": "ready"}
```

Liveness: "process is alive." Readiness: "process can serve traffic." Kubernetes and most orchestrators want both.

## Practical Application — a production-shaped service skeleton

Putting the whole module together — startup wiring for a real FastAPI service:

```python
# src/myservice/__main__.py
import uvicorn
from .config import settings
from .logging import setup_logging
from .api.app import create_app

def main() -> None:
    setup_logging(settings().log_level)
    app = create_app()
    uvicorn.run(
        app,
        host="0.0.0.0",
        port=8000,
        log_config=None,        # we configured logging ourselves
        access_log=False,       # we'll log in middleware with structure
    )

if __name__ == "__main__":
    main()
```

```python
# src/myservice/api/app.py
import uuid
import structlog
from fastapi import FastAPI, Request
from structlog.contextvars import bind_contextvars, clear_contextvars
from ..config import settings
from .routes import users

log = structlog.get_logger()

def create_app() -> FastAPI:
    app = FastAPI(title="myservice", version="0.1.0")

    @app.middleware("http")
    async def trace_and_log(request: Request, call_next):
        trace_id = request.headers.get("x-trace-id", str(uuid.uuid4()))
        bind_contextvars(trace_id=trace_id, path=request.url.path)
        log.info("request_start", method=request.method)
        try:
            response = await call_next(request)
            log.info("request_done", status=response.status_code)
            response.headers["x-trace-id"] = trace_id
            return response
        except Exception:
            log.exception("request_failed")
            raise
        finally:
            clear_contextvars()

    @app.get("/healthz")
    def liveness(): return {"status": "ok"}

    app.include_router(users.router, prefix="/users", tags=["users"])

    log.info("app_initialized", env=settings().log_level)
    return app
```

What this gives you:

- Trace IDs flow through every log line in a request.
- Startup config is validated; failures are loud and immediate.
- Logs are JSON, ready to ship to any aggregator.
- Code is layered (API → services → adapters → domain) — testable, swappable.
- `python -m myservice` is the single entry point. Same in dev, container, and k8s.

Add metrics and OpenTelemetry as shown above and you have an operationally complete starting point.

## Common Mistakes & Gotchas

- **`print()` for diagnostics in production.** Goes to stdout, no levels, no structure, no metadata. Use `structlog`.
- **`logging.basicConfig` from a library.** Steals user config. Apps configure; libraries get a logger and emit.
- **f-string log messages with PII.** `log.info(f"user {user}")` always interpolates and may dump secrets. Use structured fields: `log.info("event", user_id=user.id)`.
- **Reading config from env at use site.** Brittle, untyped, no validation. Centralize in `pydantic-settings`.
- **Secrets in `.env` committed to git.** Add `.env` to `.gitignore` from day one. Commit `.env.example`.
- **Hardcoded `localhost`/dev URLs in code.** Always config-driven.
- **No health endpoints.** Orchestrator can't tell when to restart you. Add `/healthz` and `/readyz`.
- **Single global state and singletons everywhere.** Hard to test. Inject dependencies (FastAPI `Depends`, plain constructor injection elsewhere).
- **Catching exceptions and silently logging.** A logged-and-swallowed error is invisible in metrics. Either handle properly or re-raise.
- **No timeouts on outbound HTTP / DB / cache calls.** One slow upstream lights your service on fire.
- **Skipping observability "until it matters."** It always matters before you add it. Logs, metrics, traces from day one is cheap; bolting them on after a 3am page is not.
- **Long-running blocking calls in async handlers.** Wrap with `asyncio.to_thread` or move to a sync handler — FastAPI handles both.

## 🎯 Key Takeaways

- **Structured logging from day one.** `structlog` + JSON + context vars is a 30-minute setup that pays back the first time you debug a production incident.
- **Typed, validated config (`pydantic-settings`).** Move every `os.environ.get` to a single `Settings` model. Bad configs fail loudly at startup, not silently at request 1000.
- **Layered architecture (`domain` / `adapters` / `services` / `api`).** Doesn't add complexity for small projects; saves you from rewrites in big ones. Testing becomes trivial.
- **Observability is `logs + metrics + traces`.** Prometheus + OpenTelemetry + structured logs is the 2026 default stack — all open standards, all free, all easy to add early and painful to add late.
- **Production discipline is the difference between "knows Python" and "ships Python."** Linters, type checkers, secret scanners, pre-commit hooks, health endpoints, timeouts — none of it is glamorous, all of it is what keeps you off the on-call rotation.

---

You've reached the end. From running `python hello.py` to shipping a typed, tested, observable service — every piece is here. The next step isn't another module; it's building something with all of it together. Pick a small real problem, scaffold a project the way module 14 and 17 describe, and write it. Re-read whichever module you fight with as you go.

Welcome to professional Python.

*← [prev](./16_web_and_apis.md) | [back to roadmap](./00_roadmap.md)*
