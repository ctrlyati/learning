# 16 — Production

> **Goal:** Ship Django with confidence — gunicorn/uvicorn behind nginx, in Docker, env-driven settings, Sentry, and the deployment pitfalls every senior has been bitten by.

---

## 1. The production stack — a senior-default reference

```
┌─────────────────────────────────────────────────────┐
│  CDN (CloudFront / CloudFlare)                      │  static + edge cache
├─────────────────────────────────────────────────────┤
│  nginx                                              │  TLS, static files, gzip, rate limit
├─────────────────────────────────────────────────────┤
│  gunicorn (sync) or uvicorn (async/ASGI)            │  Python app server
│   └─ Django (WSGI or ASGI)                          │
├─────────────────────────────────────────────────────┤
│  PostgreSQL                Redis                    │  data, cache, sessions, queues
├─────────────────────────────────────────────────────┤
│  Celery workers (optional, background tasks)        │
└─────────────────────────────────────────────────────┘
```

Pieces and roles:

- **nginx** — TLS terminator, static-file server, reverse proxy, rate limiter.
- **gunicorn / uvicorn** — Python application servers. gunicorn for WSGI (sync Django); uvicorn for ASGI (async / Channels).
- **PostgreSQL** — your default RDBMS. SQLite is for dev only.
- **Redis** — cache + sessions + DRF throttle counters + Celery broker.
- **Celery (or RQ/Dramatiq)** — long-running tasks (emails, image processing, webhooks).
- **Sentry** — error tracking and performance monitoring.

---

## 2. gunicorn — the WSGI workhorse

```bash
pip install gunicorn
```

Run:

```bash
gunicorn mysite.wsgi:application \
    --bind 0.0.0.0:8000 \
    --workers 3 \
    --threads 2 \
    --timeout 30 \
    --access-logfile - \
    --error-logfile -
```

Worker model:

- `--workers N` — processes (each a full Python interpreter). Rule of thumb: `2 * cores + 1`.
- `--threads N` — threads per worker (sync mode).
- `--worker-class gthread` — threaded (default for many configs).
- `--worker-class gevent` — async-ish for I/O-heavy sync code.
- `--max-requests 1000 --max-requests-jitter 100` — graceful worker recycling to dodge memory leaks.

A typical production gunicorn config (`gunicorn.conf.py`):

```python
import multiprocessing
bind = "0.0.0.0:8000"
workers = multiprocessing.cpu_count() * 2 + 1
threads = 2
timeout = 30
keepalive = 2
max_requests = 1000
max_requests_jitter = 100
graceful_timeout = 30
accesslog = "-"
errorlog = "-"
loglevel = "info"
```

For ASGI (Django 5 async features, Channels): replace gunicorn with **uvicorn** or run `gunicorn -k uvicorn.workers.UvicornWorker`.

```bash
pip install uvicorn[standard]
uvicorn mysite.asgi:application --workers 4 --host 0.0.0.0 --port 8000
```

---

## 3. nginx — front everything

```nginx
upstream django {
    server localhost:8000;
}

server {
    listen 443 ssl http2;
    server_name myapp.com;

    ssl_certificate     /etc/letsencrypt/live/myapp.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/myapp.com/privkey.pem;

    client_max_body_size 25M;

    location /static/ {
        alias /app/staticfiles/;
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    location /media/ {
        alias /app/media/;
    }

    location / {
        proxy_pass http://django;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 30s;
    }
}

server {
    listen 80;
    server_name myapp.com;
    return 301 https://$host$request_uri;
}
```

In Django:

```python
SECURE_PROXY_SSL_HEADER = ("HTTP_X_FORWARDED_PROTO", "https")
USE_X_FORWARDED_HOST = True
```

Otherwise Django sees HTTP requests (from nginx) and `request.is_secure()` returns False.

If you don't want nginx, **Whitenoise** can serve static from gunicorn itself — simpler for small deployments. See module 9.

---

## 4. Environment-based settings

The senior pattern:

```
mysite/settings/
├── __init__.py
├── base.py
├── dev.py
├── test.py
└── prod.py
```

```python
# base.py
import environ
from pathlib import Path

BASE_DIR = Path(__file__).resolve().parent.parent.parent
env = environ.Env()
environ.Env.read_env(BASE_DIR / ".env")

SECRET_KEY = env("SECRET_KEY")
DEBUG = env.bool("DEBUG", default=False)
ALLOWED_HOSTS = env.list("ALLOWED_HOSTS", default=[])

DATABASES = {"default": env.db("DATABASE_URL", default="sqlite:///db.sqlite3")}
CACHES = {"default": env.cache("REDIS_URL", default="locmemcache://")}

INSTALLED_APPS = [...]
MIDDLEWARE = [...]
# ... etc
```

```python
# prod.py
from .base import *

DEBUG = False
SECURE_SSL_REDIRECT = True
SECURE_HSTS_SECONDS = 31536000
SESSION_COOKIE_SECURE = True
CSRF_COOKIE_SECURE = True
# ... full SECURE_* set from module 15

# Logging
LOGGING = {
    "version": 1,
    "disable_existing_loggers": False,
    "formatters": {"json": {"format": '{"time":"%(asctime)s","lvl":"%(levelname)s","msg":"%(message)s"}'}},
    "handlers": {"console": {"class": "logging.StreamHandler", "formatter": "json"}},
    "root": {"handlers": ["console"], "level": "INFO"},
    "loggers": {
        "django.request": {"level": "WARNING"},
        "django.db.backends": {"level": "WARNING"},   # turn UP to DEBUG to see queries
    },
}
```

Run with `DJANGO_SETTINGS_MODULE=mysite.settings.prod`.

`.env` files for local dev only; for prod, inject env vars from your orchestrator (Docker secrets, Kubernetes ConfigMap/Secret, ECS task definition, etc.).

---

## 5. Dockerfile — multi-stage, slim, deterministic

```dockerfile
# syntax=docker/dockerfile:1.7
FROM python:3.12-slim AS builder

ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PIP_NO_CACHE_DIR=1
WORKDIR /build

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential libpq-dev && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --prefix=/install -r requirements.txt


FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 \
    DJANGO_SETTINGS_MODULE=mysite.settings.prod

RUN apt-get update && apt-get install -y --no-install-recommends \
    libpq5 && rm -rf /var/lib/apt/lists/*

COPY --from=builder /install /usr/local
WORKDIR /app
COPY . .

RUN useradd --create-home --shell /bin/bash app
RUN python manage.py collectstatic --noinput
RUN chown -R app:app /app
USER app

EXPOSE 8000
CMD ["gunicorn", "mysite.wsgi:application", "-c", "gunicorn.conf.py"]
```

Key practices:

- **Multi-stage** — compile wheels in builder, copy to slim runtime. Smaller, faster, more secure image.
- **Non-root user** — `USER app` so the container doesn't run as root.
- **`collectstatic` at build time** — the image ships ready to serve.
- **`PYTHONUNBUFFERED=1`** — logs stream immediately instead of buffering.
- **No `.env` in the image** — inject at runtime.

Compose for local development:

```yaml
# docker-compose.yml
services:
  web:
    build: .
    ports: ["8000:8000"]
    env_file: .env
    depends_on: [db, redis]
    volumes: [".:/app"]
  db:
    image: postgres:16
    environment:
      POSTGRES_DB: mysite
      POSTGRES_USER: mysite
      POSTGRES_PASSWORD: mysite
    volumes: [postgres_data:/var/lib/postgresql/data]
  redis:
    image: redis:7-alpine
volumes:
  postgres_data:
```

---

## 6. Sentry — error tracking + performance

```bash
pip install sentry-sdk
```

```python
# prod.py
import sentry_sdk
from sentry_sdk.integrations.django import DjangoIntegration

sentry_sdk.init(
    dsn=env("SENTRY_DSN"),
    integrations=[DjangoIntegration()],
    environment=env("ENV", default="production"),
    release=env("GIT_SHA", default="unknown"),
    traces_sample_rate=0.1,        # 10% performance traces
    send_default_pii=False,        # don't send user data
    profiles_sample_rate=0.1,
)
```

You now get:
- Every unhandled exception, with traceback, request context, user (if logged in).
- Slow-query and slow-view alerts (with `traces_sample_rate`).
- Release tracking (which deploy introduced this bug).
- Issue grouping, ownership routing, Slack alerts.

Tag releases:

```bash
sentry-cli releases new $GIT_SHA
sentry-cli releases set-commits $GIT_SHA --auto
sentry-cli releases finalize $GIT_SHA
```

---

## 7. Background tasks — Celery (or alternatives)

Long-running work (email, image resize, third-party API calls) doesn't belong in the request cycle. Celery is the standard.

```bash
pip install celery redis
```

```python
# mysite/celery.py
import os
from celery import Celery

os.environ.setdefault("DJANGO_SETTINGS_MODULE", "mysite.settings.prod")
app = Celery("mysite")
app.config_from_object("django.conf:settings", namespace="CELERY")
app.autodiscover_tasks()
```

```python
# mysite/__init__.py
from .celery import app as celery_app
__all__ = ("celery_app",)
```

```python
# settings/base.py
CELERY_BROKER_URL = env("REDIS_URL")
CELERY_RESULT_BACKEND = env("REDIS_URL")
CELERY_TASK_ALWAYS_EAGER = env.bool("CELERY_EAGER", default=False)   # for tests
```

```python
# blog/tasks.py
from celery import shared_task

@shared_task
def send_welcome_email(user_id):
    from django.contrib.auth import get_user_model
    User = get_user_model()
    user = User.objects.get(pk=user_id)
    # send email
```

```python
# In a view:
from django.db import transaction
def signup(request):
    user = form.save()
    transaction.on_commit(lambda: send_welcome_email.delay(user.id))
```

Run a worker alongside your web container:

```bash
celery -A mysite worker --loglevel=info
celery -A mysite beat --loglevel=info        # for scheduled tasks
```

Alternatives: **RQ** (simpler), **Dramatiq** (more modern), **django-q2**. Celery is heavyweight but battle-tested.

---

## 8. Database in production

- **Use PostgreSQL.** MySQL works; SQLite doesn't.
- **Connection pooling.** Django opens a fresh DB connection per request by default. Set `CONN_MAX_AGE = 60` to persist connections per process. Beyond that, use **pgbouncer** in front.
- **Indexes.** `django-debug-toolbar` in staging surfaces missing indexes. Add them in migrations with `Meta.indexes`.
- **Migrations.** Run them as a deploy step, not inside the web container's startup. CI/CD: `migrate` → restart workers (zero downtime via blue/green).
- **Backups.** `pg_dump` nightly to S3, plus point-in-time recovery (WAL archiving). Test restores quarterly — backup you haven't restored is a hope, not a backup.

---

## 9. Performance tuning checklist

1. **Profile first.** Use `django-debug-toolbar` (dev), `silk` (more detailed), or Sentry Performance (prod). Never guess.
2. **Kill N+1 queries.** Module 5. The #1 cause of "Django is slow." `select_related` / `prefetch_related`.
3. **Add indexes.** Slow `WHERE col = ?` on a 1M-row table? Index `col`. `EXPLAIN ANALYZE` is your friend.
4. **Cache the hot path.** Trending posts, dashboard widgets, computed aggregates — module 12.
5. **`CONN_MAX_AGE`.** Stop opening a new DB connection per request.
6. **`only()` / `defer()`** on huge `TextField`/`JSONField` columns when listing.
7. **Pagination, always.** Never return unbounded querysets — pages of 25/50/100 max.
8. **gunicorn workers tuned.** `2*cores+1` is a starting point; load-test and adjust.
9. **CDN for static.** Even Whitenoise is faster behind CloudFront/CloudFlare.
10. **Async only where it helps.** Module 13. Not a silver bullet.

---

## 10. Zero-downtime deploys

The pattern:

1. New code rolls out behind a load balancer that drains old workers.
2. Migrations are **always backward-compatible** with the running old code — at least for one deploy.
3. Two-phase risky migrations:
   - **Deploy A:** add new column, dual-write to old + new from app code.
   - **Backfill** the new column.
   - **Deploy B:** read from new column; old code still works.
   - **Deploy C:** drop old column.
4. New deploys run `migrate` as a one-shot job/hook before swapping traffic.
5. Drain old workers (gunicorn `--graceful-timeout`).

Tools: **django-tenant-schemas**, **django-zero-downtime-migrations** for migration safety checks.

---

## 11. Health checks and observability

A `/healthz/` endpoint that checks DB and cache:

```python
from django.db import connection
from django.core.cache import cache
from django.http import JsonResponse

def healthz(request):
    try:
        connection.ensure_connection()
        cache.set("health", "ok", timeout=10)
        assert cache.get("health") == "ok"
    except Exception as e:
        return JsonResponse({"status": "fail", "error": str(e)}, status=503)
    return JsonResponse({"status": "ok"})
```

For metrics: `django-prometheus` exposes a `/metrics/` endpoint for Prometheus scraping. Pair with Grafana.

For tracing: OpenTelemetry's `opentelemetry-instrumentation-django` instruments requests/queries.

---

## 12. Practical application — deployment-ready settings module

```python
# mysite/settings/prod.py
from .base import *
import sentry_sdk
from sentry_sdk.integrations.django import DjangoIntegration

DEBUG = False
ALLOWED_HOSTS = env.list("ALLOWED_HOSTS")
SECRET_KEY = env("SECRET_KEY")

DATABASES = {"default": env.db("DATABASE_URL")}
DATABASES["default"]["CONN_MAX_AGE"] = 60

CACHES = {
    "default": {
        "BACKEND": "django.core.cache.backends.redis.RedisCache",
        "LOCATION": env("REDIS_URL"),
    }
}

SESSION_ENGINE = "django.contrib.sessions.backends.cached_db"

STORAGES = {
    "default": {
        "BACKEND": "storages.backends.s3.S3Storage",
        "OPTIONS": {"bucket_name": env("S3_BUCKET"), "region_name": env("AWS_REGION"), "location": "media"},
    },
    "staticfiles": {"BACKEND": "whitenoise.storage.CompressedManifestStaticFilesStorage"},
}

# Security headers (module 15)
SECURE_SSL_REDIRECT = True
SECURE_HSTS_SECONDS = 31536000
SECURE_HSTS_INCLUDE_SUBDOMAINS = True
SECURE_HSTS_PRELOAD = True
SECURE_PROXY_SSL_HEADER = ("HTTP_X_FORWARDED_PROTO", "https")
SESSION_COOKIE_SECURE = True
CSRF_COOKIE_SECURE = True
SECURE_CONTENT_TYPE_NOSNIFF = True
X_FRAME_OPTIONS = "DENY"

# Sentry
sentry_sdk.init(
    dsn=env("SENTRY_DSN"),
    integrations=[DjangoIntegration()],
    environment=env("ENV", default="production"),
    release=env("GIT_SHA", default="dev"),
    traces_sample_rate=0.1,
    send_default_pii=False,
)

# Celery
CELERY_BROKER_URL = env("REDIS_URL")
CELERY_RESULT_BACKEND = env("REDIS_URL")

# Logging
LOGGING = {
    "version": 1,
    "disable_existing_loggers": False,
    "handlers": {"console": {"class": "logging.StreamHandler"}},
    "root": {"handlers": ["console"], "level": "INFO"},
    "loggers": {
        "django.request": {"level": "WARNING"},
        "django.security": {"level": "WARNING"},
    },
}
```

```bash
# Deploy script (simplified)
docker build -t myapp:$GIT_SHA .
docker push registry/myapp:$GIT_SHA

# In the cluster:
kubectl set image deployment/web web=registry/myapp:$GIT_SHA
kubectl rollout status deployment/web

# Or in a simpler stack:
docker compose pull
docker compose up -d --no-deps web
docker compose exec web python manage.py migrate --noinput
```

---

## 13. Common deployment pitfalls

1. **`DEBUG=True` in production.** Every Django senior has seen this in the wild. Set it via env var and *verify* with `manage.py check --deploy`.
2. **`ALLOWED_HOSTS = []` / `["*"]`.** First request 400s ("Invalid HTTP_HOST header"). Setting it to `["*"]` defeats Host header validation. Use the actual domains.
3. **Static files 404.** `collectstatic` wasn't run, or `STATIC_ROOT` doesn't match nginx `alias`, or you forgot Whitenoise. Module 9.
4. **Database migrations not run.** Container starts, requests fail with `relation "auth_user" does not exist`. Always run `migrate` before traffic flips.
5. **SECRET_KEY mismatch across instances.** Multiple gunicorn pods with different keys → sessions invalidate at random. Inject the same key everywhere.
6. **Time zone confusion.** `USE_TZ=True` everywhere, store UTC, convert at display. Naïve datetimes from one server, aware from another → off-by-hours.
7. **Worker count too high.** Each gunicorn worker is a full Python process. 32 workers on a 4GB pod → OOM. Profile.
8. **No `CONN_MAX_AGE`.** Each request opens + closes a Postgres connection. On a busy app, the DB spends more time accepting connections than running queries.
9. **Logging at DEBUG level.** Floods log aggregation, costs money. INFO or WARNING in prod.
10. **No Sentry / no observability.** You find out about errors when users tweet about them.
11. **No backups (or untested backups).** "We have nightly snapshots" until you find out at 3am they've been failing for 2 months.
12. **Running `python manage.py shell` in production for "quick fixes".** No audit trail, no review, regrettable. Use a management command and a PR.
13. **Skipping the staging environment.** Test deploys in staging that mirrors prod — same Postgres, same Redis, same env shape.
14. **Trusting client-supplied `X-Forwarded-For`.** Without configuring trusted proxies, attackers spoof their IP. Use `django-ipware` or explicit proxy lists.
15. **Hot-reloading `runserver` in production.** Don't. Ever. `runserver` is single-threaded, has no graceful shutdown, no worker model. gunicorn/uvicorn or nothing.

---

## 🎯 Key Takeaways

- **The production stack is non-negotiable: nginx + gunicorn/uvicorn + Postgres + Redis + Sentry.** Each piece has a clear job. Skipping nginx or Sentry "to keep it simple" costs more later.
- **Settings split per environment, secrets via env vars.** `mysite.settings.prod` reads `DATABASE_URL`, `SECRET_KEY`, `REDIS_URL` from the environment. Never commit secrets, never use `.env` files in prod images.
- **`manage.py check --deploy` is the security baseline.** Run it in CI and treat warnings as failures. `DEBUG=True` slipping to prod is the most common Django incident — make it impossible by construction.
- **Migrations are a deploy step, not a runtime concern.** Run them backward-compatibly so old + new workers coexist during rollout. Two-phase migrations for column drops/renames.
- **Profile, don't guess.** Sentry Performance + `django-debug-toolbar` reveal what's actually slow. The fix is almost always: missing index, N+1 query, or unbounded queryset — not "we need to rewrite in Go."

---

You finished. You should now be able to: start a Django project from zero, model and migrate a non-trivial schema, build views and templates idiomatically, wire up an authenticated admin, ship a DRF API, layer caching and async where it pays, write a test suite, harden security, and deploy behind nginx with Sentry watching.

The framework rewards depth — every senior Django dev I know is still discovering things about it years in. Bookmark `docs.djangoproject.com`, lurk on the Django forum, and ship. Welcome to the club.

*← [prev](./15_security.md) | [home](./00_roadmap.md)*
