# 15 — Production

> **Goal:** Ship FastAPI to production with confidence — gunicorn+uvicorn workers, Docker images, env-driven config via pydantic-settings, reverse proxy headers, health checks, and deployment patterns.

---

## 1. Concept — A FastAPI app in production is a process, in a container, behind a proxy

The stack, from outside in:

```
Client → Load Balancer (TLS, DDoS, WAF)
       → Reverse Proxy / Ingress (nginx, Envoy, AWS ALB)
       → Container Runtime (k8s pod, ECS task, Cloud Run)
       → Process Manager (gunicorn / uvicorn supervisor)
       → Worker Processes (uvicorn workers running your FastAPI app)
       → Your code
```

Each layer has a job. Your job, as the FastAPI developer, is the bottom three: code, workers, config. The rest is platform — but you must understand the interfaces.

```dockerfile
# Minimal production Dockerfile
FROM python:3.12-slim
ENV PYTHONUNBUFFERED=1 PIP_DISABLE_PIP_VERSION_CHECK=1
WORKDIR /app

COPY pyproject.toml uv.lock ./
RUN pip install --no-cache-dir uv && uv pip install --system --no-deps -r <(uv pip compile pyproject.toml)

COPY app/ ./app/
EXPOSE 8000
CMD ["gunicorn", "app.main:app", "-w", "4", "-k", "uvicorn.workers.UvicornWorker", "-b", "0.0.0.0:8000"]
```

That image runs anywhere — k8s, ECS, Cloud Run, Fly.io. Behind a proxy. With logs to stdout. The way prod is supposed to work.

---

## 2. Mechanism — Why gunicorn + uvicorn (and when uvicorn alone)

A single Uvicorn process: one event loop, one CPU core, one point of failure. Restart = downtime.

`gunicorn` is a battle-tested process manager. With the `UvicornWorker` class, gunicorn:

- Spawns N uvicorn worker processes (one per core typical).
- Restarts a worker if it crashes.
- Rotates workers periodically (memory hygiene).
- Drains gracefully on SIGTERM.

```
gunicorn app.main:app \
    -w 4 \
    -k uvicorn.workers.UvicornWorker \
    -b 0.0.0.0:8000 \
    --timeout 30 \
    --graceful-timeout 30 \
    --max-requests 1000 \
    --max-requests-jitter 100 \
    --access-logfile -
```

Key flags:

- `-w N`: worker count. Rule of thumb `(CPU * 2) + 1` for sync-heavy apps; `CPU + 1` for async-heavy.
- `-k uvicorn.workers.UvicornWorker`: ASGI worker class.
- `--timeout 30`: kill workers if a request takes longer than 30s (catches deadlocks).
- `--max-requests 1000` (+ `--max-requests-jitter`): periodic worker recycle — defends against slow memory leaks.

**When uvicorn alone is fine:**
- Behind a platform that handles supervision (k8s with `replicas: N`, Cloud Run with autoscaling, AWS Lambda).
- In those cases, each container runs one `uvicorn` process, and the platform spawns/restarts containers.

For k8s specifically, the modern preference is **one container = one uvicorn worker**, and let `replicas` provide concurrency. It plays nicer with horizontal pod autoscaling and isolated failures.

---

## 3. Variations & Depth

### Configuration with `pydantic-settings`

Never hardcode secrets. Never read `os.environ` scattered across modules. One place:

```bash
uv pip install pydantic-settings
```

```python
# app/core/config.py
from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        case_sensitive=False,
        extra="ignore",
    )

    # app
    app_name: str = "MyApp"
    app_version: str = "0.1.0"
    env: str = Field("local", description="local|staging|prod")
    log_level: str = "INFO"

    # security
    secret_key: str
    access_token_expire_minutes: int = 30

    # db
    database_url: str
    db_pool_size: int = 10

    # cors / hosts
    cors_origins: list[str] = []
    allowed_hosts: list[str] = ["*"]

    # otel
    otel_enabled: bool = False
    otel_endpoint: str = "http://localhost:4317"
    service_name: str = "myapp-api"


settings = Settings()  # type: ignore[call-arg]
```

`.env` (gitignored):

```
SECRET_KEY=dev-only-not-for-prod
DATABASE_URL=postgresql+asyncpg://user:pw@localhost/app
CORS_ORIGINS=["http://localhost:3000"]
```

In prod: inject via env vars, k8s Secrets, or AWS Secrets Manager (read at startup). The `Settings` class validates types — missing `SECRET_KEY` raises at startup, not at first request. Fail-fast.

### Reverse proxy headers — trust but verify

When behind a proxy:

```
Client (1.2.3.4) → LB (10.0.0.5) → Pod
```

`request.client.host` shows `10.0.0.5`. The real IP is in `X-Forwarded-For`. Same for scheme (`X-Forwarded-Proto: https`) and host.

Uvicorn handles this via `--forwarded-allow-ips`:

```bash
uvicorn app.main:app --proxy-headers --forwarded-allow-ips="*"
```

Or in gunicorn: `--forwarded-allow-ips="*"`.

**Security note:** Only set `*` if you're certain only your proxy can reach the container's port. Otherwise, a direct attacker can inject `X-Forwarded-For` and spoof an IP.

In code:

```python
@app.get("/whoami")
def whoami(request: Request) -> dict:
    return {
        "ip": request.client.host,                     # real client after forward
        "scheme": request.url.scheme,                  # http or https
        "host": request.headers.get("host"),
    }
```

### Health checks — wire to your platform

```python
from fastapi import APIRouter, Depends
from sqlalchemy import text
from app.api.deps import DBSession

router = APIRouter(tags=["meta"], include_in_schema=False)


@router.get("/health/live")
def liveness() -> dict:
    return {"status": "ok"}


@router.get("/health/ready")
async def readiness(db: DBSession) -> dict:
    await db.execute(text("SELECT 1"))
    return {"status": "ok"}
```

Kubernetes:

```yaml
livenessProbe:
  httpGet: { path: /health/live, port: 8000 }
  periodSeconds: 10
  failureThreshold: 3
readinessProbe:
  httpGet: { path: /health/ready, port: 8000 }
  periodSeconds: 5
  failureThreshold: 2
```

### Graceful shutdown

Uvicorn handles SIGTERM correctly: stops accepting new connections, finishes in-flight ones, then exits. In `lifespan`, your cleanup runs.

```python
@asynccontextmanager
async def lifespan(app: FastAPI):
    # startup
    app.state.http = httpx.AsyncClient()
    yield
    # shutdown: runs on SIGTERM / clean exit
    await app.state.http.aclose()
    await engine.dispose()
```

For long-running endpoints (LLM streaming), set gunicorn's `--graceful-timeout` higher than your worst-case response time.

### A production-grade Dockerfile

```dockerfile
# syntax=docker/dockerfile:1.7
FROM python:3.12-slim AS builder

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    PYTHONDONTWRITEBYTECODE=1

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential && rm -rf /var/lib/apt/lists/*

WORKDIR /app
RUN pip install --no-cache-dir uv

COPY pyproject.toml uv.lock ./
RUN uv venv /opt/venv && \
    VIRTUAL_ENV=/opt/venv uv pip install --no-cache .

# ---

FROM python:3.12-slim AS runtime

ENV PATH="/opt/venv/bin:$PATH" \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1

RUN apt-get update && apt-get install -y --no-install-recommends \
    libpq5 ca-certificates && rm -rf /var/lib/apt/lists/* && \
    useradd --create-home --shell /bin/bash app

COPY --from=builder /opt/venv /opt/venv
WORKDIR /app
COPY --chown=app:app app/ ./app/

USER app
EXPOSE 8000

CMD ["gunicorn", "app.main:app", \
     "-w", "4", \
     "-k", "uvicorn.workers.UvicornWorker", \
     "-b", "0.0.0.0:8000", \
     "--timeout", "30", \
     "--graceful-timeout", "30", \
     "--max-requests", "1000", \
     "--max-requests-jitter", "100", \
     "--access-logfile", "-", \
     "--forwarded-allow-ips=*"]
```

Properties:

- **Multi-stage build**: only runtime deps in final image. Result: ~150 MB instead of 800 MB.
- **Non-root user**: `app` user, not root. Compliance + defense in depth.
- **No `pip cache`, no `__pycache__`** in image.
- **Pinned base image** (slim, specific Python).

### Logging in containers

- Log to **stdout**. Don't write to files. The container runtime captures stdout/stderr and ships to your log platform.
- JSON format (structlog from module 14).
- No log rotation, no log files. That's the platform's job.

### Secret management

- `.env` for **local dev only**. Never commit.
- Prod: platform-native secrets (k8s Secrets, AWS Secrets Manager, Doppler, 1Password Secrets Automation).
- `pydantic-settings` reads env vars, so just inject them as env vars in the container spec.
- Rotate `SECRET_KEY` carefully — invalidates existing JWTs. Use key rotation patterns (`SECRET_KEYS=[current,previous]`) if you need overlap.

### Deployment patterns

| Pattern              | Where it shines                                            |
| -------------------- | ---------------------------------------------------------- |
| **Kubernetes**       | Multiple services, autoscaling, you have an ops team       |
| **Cloud Run / Fargate** | Stateless services, scale-to-zero, no cluster to manage |
| **VPS + systemd**    | Tiny apps, side projects, predictable cost                 |
| **PaaS** (Fly, Render, Railway) | Solo dev, ship in a day                         |
| **Lambda** + Mangum  | Spiky traffic, latency-insensitive, no WebSockets          |

For a service expecting steady traffic at production scale, k8s or Cloud Run. For everything else, pick the simplest thing.

### CI/CD outline

```yaml
# .github/workflows/ci.yml (sketch)
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: astral-sh/setup-uv@v3
      - run: uv pip install --system -e ".[dev]"
      - run: ruff check .
      - run: mypy app/
      - run: pytest --cov=app --cov-report=xml
      - run: alembic check  # detect model/migration drift

  build:
    needs: test
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/build-push-action@v6
        with:
          push: true
          tags: registry/app:${{ github.sha }}
```

Then a separate deploy step pushes the image tag into k8s/etc.

---

## 4. Practical Application — Production-ready `main.py` checklist

```python
# app/main.py — production skeleton (synthesized from modules 10, 14, 15)
import structlog
from contextlib import asynccontextmanager
from fastapi import FastAPI
from prometheus_fastapi_instrumentator import Instrumentator

from app.api.v1 import users, items
from app.api.deps import internal_router  # health checks
from app.core.config import settings
from app.core.logging import configure_logging
from app.core.tracing import configure_tracing
from app.core.errors import register_exception_handlers
from app.middleware import register_middleware

configure_logging(settings.log_level)
logger = structlog.get_logger("app")


@asynccontextmanager
async def lifespan(app: FastAPI):
    logger.info("startup", env=settings.env, version=settings.app_version)
    # init shared clients here
    yield
    logger.info("shutdown")


app = FastAPI(
    title=settings.app_name,
    version=settings.app_version,
    lifespan=lifespan,
    docs_url="/docs" if settings.env != "prod" else None,
    redoc_url="/redoc" if settings.env != "prod" else None,
    openapi_url="/openapi.json" if settings.env != "prod" else None,
)

register_middleware(app, settings)
register_exception_handlers(app)

if settings.otel_enabled:
    configure_tracing(app, settings.service_name, settings.otel_endpoint)

Instrumentator().instrument(app).expose(
    app, endpoint="/metrics", include_in_schema=False
)

app.include_router(internal_router)
app.include_router(users.router, prefix="/api/v1")
app.include_router(items.router, prefix="/api/v1")
```

Notes on this skeleton:

- **Docs hidden in prod** — many shops do this for internal services. Make the call deliberately.
- **Middleware registration** centralized — easier to audit ordering.
- **Lifespan owns startup/shutdown** — single place to wire and unwire shared resources.

### A "go-live" checklist

- [ ] All secrets in env vars, none in code or `.env` committed.
- [ ] `pydantic-settings` validates required vars at startup.
- [ ] DB migrations applied (`alembic upgrade head`) as part of release.
- [ ] Health checks: `/health/live`, `/health/ready` wired to probes.
- [ ] Reverse proxy headers configured (`--forwarded-allow-ips`).
- [ ] CORS allow-list explicit, not wildcard.
- [ ] Trusted hosts middleware on (specific hostnames).
- [ ] HTTPS enforced at the proxy/LB; HSTS header set.
- [ ] Structured JSON logging to stdout.
- [ ] Prometheus `/metrics` exposed, scraped, dashboards built.
- [ ] OpenTelemetry traces shipping to backend.
- [ ] Error tracking (Sentry / similar) configured.
- [ ] Alerts: error rate, p95 latency, queue depth (if applicable).
- [ ] Worker count tuned (`(cpu_count * 2) + 1` baseline, then load-test).
- [ ] `--max-requests` set (defense against slow leaks).
- [ ] Graceful shutdown verified (kill the pod, watch logs).
- [ ] Dockerfile uses non-root user, multi-stage, pinned base.
- [ ] CI runs ruff, mypy, pytest, alembic check on every PR.
- [ ] Rollback plan tested (previous image tag still works).

---

## 5. Common Mistakes & Gotchas

- **`uvicorn --reload` in production.** Watches files, leaks memory, hangs on imports. Dev-only.
- **`--workers` on uvicorn AND replica count > 1.** Now you have replicas × workers processes — easy to exceed DB connection limit. Pick one knob.
- **Single worker on a multi-core machine.** You're using 1/N of the box. Either scale workers or scale replicas.
- **Storing state in process memory.** "Active user count," in-memory caches — broken the moment you have >1 worker/replica. Use Redis or accept per-instance state and document it.
- **Forgetting `--forwarded-allow-ips`** behind a proxy. `request.client.host` is the load balancer; your rate limiter banishes itself.
- **HTTPS-only at the proxy but `/auth` cookies without `Secure`.** Set the `Secure` and `SameSite` cookie attrs. Otherwise a downgraded HTTP request leaks the cookie.
- **Long-running endpoints + small `--timeout`.** Gunicorn kills the worker mid-stream. Raise timeout *and* prefer streaming/SSE for genuinely long work.
- **Hardcoded `localhost` DB URL in `pyproject.toml` or fallback in code.** Now CI passes but prod fails. Fail-fast: require `DATABASE_URL` with no default.
- **Skipping migrations during deploy** — schema drift in prod. Run `alembic upgrade head` as part of the release pipeline, before the new image starts serving.
- **No rollback plan.** "Deploy and pray" works until it doesn't. Keep the previous image tag, know how to flip back in <2 minutes.
- **`requirements.txt` and `pyproject.toml` both present**, diverging. Pick one source of truth.
- **`/docs` exposed publicly with no auth.** Reveals internal endpoints, sometimes leaks intent. Gate with auth, or disable in prod.

---

## 🎯 Key Takeaways

- **One worker per core *or* one container per worker — pick a model and stick with it.** Hybrid (k8s + many gunicorn workers) is unnecessary complexity for most apps.
- **`pydantic-settings` is the only sane way to configure a Python service.** Type-checked, validated at startup, env-aware. Adopt it on day one.
- **Reverse proxy headers, health checks, graceful shutdown** are not extras — they're the contract with your platform. Get them wrong and Kubernetes lies about your service's health.
- **Logs to stdout, metrics on `/metrics`, traces over OTLP** is the platform-agnostic observability triad. Works on every runtime; portable across clouds.
- **Production isn't a different framework, it's the same framework with the knobs set to "adult."** Every choice in this module is a 30-line config change that prevents a 3 AM page.

---

You've finished the deep-dive. From here, the next loop is *practice*: build a real CRUD service end-to-end (auth, DB, tests, Docker, metrics) and put it in front of users — or interviewers. The frameworks change every few years. The skills you've built here — typed contracts, dependency graphs, async I/O reasoning, schema-driven APIs, production hygiene — don't.

*← [prev](./14_observability_and_performance.md) | [back to roadmap](./00_roadmap.md)*
