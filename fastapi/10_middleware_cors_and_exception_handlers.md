# 10 — Middleware, CORS & Exception Handlers

> **Goal:** Use middleware for cross-cutting concerns (request IDs, timing, logging), configure CORS correctly, and map exceptions to clean responses without `try/except` in every endpoint.

---

## 1. Concept — Middleware wraps every request/response

A middleware is a function or class that runs before your endpoint, after your endpoint, or both. It can:

- Read/modify the request.
- Read/modify the response.
- Short-circuit and return its own response.
- Time the call, log it, attach a request ID, enforce a header.

Middleware in FastAPI (which is Starlette) is layered: each one wraps the next. The outermost runs first (request) and last (response).

```python
from fastapi import FastAPI, Request
from starlette.middleware.base import BaseHTTPMiddleware
import time
import uuid

app = FastAPI()


@app.middleware("http")
async def add_request_id_and_timing(request: Request, call_next):
    request_id = request.headers.get("X-Request-ID") or uuid.uuid4().hex
    request.state.request_id = request_id

    start = time.perf_counter()
    response = await call_next(request)
    elapsed_ms = (time.perf_counter() - start) * 1000

    response.headers["X-Request-ID"] = request_id
    response.headers["X-Process-Time-MS"] = f"{elapsed_ms:.1f}"
    return response
```

That single middleware gives you traceable, timed requests everywhere. `request.state.request_id` is now readable inside any endpoint or dependency.

---

## 2. Mechanism — Two middleware APIs, one execution model

Two equivalent ways to write middleware:

**A. The decorator (simple, function-style)**

```python
@app.middleware("http")
async def my_middleware(request: Request, call_next):
    # before
    response = await call_next(request)
    # after
    return response
```

**B. The class (composable, reusable)**

```python
from starlette.middleware.base import BaseHTTPMiddleware


class TimingMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request, call_next):
        start = time.perf_counter()
        response = await call_next(request)
        response.headers["X-Process-Time-MS"] = f"{(time.perf_counter() - start) * 1000:.1f}"
        return response


app.add_middleware(TimingMiddleware)
```

There's also a third, lower-level ASGI middleware that operates on the raw `(scope, receive, send)` protocol — used by libraries like Sentry, OpenTelemetry. You rarely need to write one.

### Order matters

Middleware added **last** runs **first** for the request (outermost wrapper). For:

```python
app.add_middleware(A)  # added 1st
app.add_middleware(B)  # added 2nd
```

Request flow: B → A → endpoint → A → B. Response flow: A → B → client.

This is the opposite of intuition — and the source of "why isn't my CORS header on the error response?" bugs.

**Add CORS *last* in your code so it runs first** — it must wrap everything for preflights to work on error paths too.

---

## 3. Variations & Depth

### CORS — Cross-Origin Resource Sharing

When a browser at `https://app.example.com` calls `https://api.example.com`, it sends a CORS preflight (`OPTIONS`) first. Your API must respond with the right `Access-Control-Allow-*` headers, or the browser blocks the actual request.

```python
from fastapi.middleware.cors import CORSMiddleware

app.add_middleware(
    CORSMiddleware,
    allow_origins=["https://app.example.com", "https://admin.example.com"],
    allow_credentials=True,
    allow_methods=["GET", "POST", "PUT", "DELETE", "PATCH"],
    allow_headers=["Authorization", "Content-Type", "X-Request-ID"],
    expose_headers=["X-Request-ID", "X-Process-Time-MS"],
    max_age=3600,
)
```

Rules of survival:

- **`allow_origins=["*"]` and `allow_credentials=True` is invalid** — browsers reject it. Pick specific origins or set credentials False.
- **List origins explicitly in production.** No wildcards. Use `allow_origin_regex=` for "any subdomain of mine."
- **`max_age=3600`** caches preflight responses for 1h — saves a round trip per request.
- **CORS doesn't protect you.** It's a browser-enforced thing. A server-to-server call ignores it entirely. Auth + CSRF do the actual protection.

### Trusted hosts

Reject requests with weird `Host` headers (DNS rebinding defense):

```python
from starlette.middleware.trustedhost import TrustedHostMiddleware

app.add_middleware(
    TrustedHostMiddleware,
    allowed_hosts=["api.example.com", "*.example.com"],
)
```

### GZip

Free 60–80% reduction on JSON responses:

```python
from starlette.middleware.gzip import GZipMiddleware

app.add_middleware(GZipMiddleware, minimum_size=1000)
```

Skips small payloads where compression overhead exceeds savings. Most reverse proxies (nginx, Cloudflare) do gzip too — only one layer needs to.

### Exception handlers

Instead of `try/except` in every endpoint, register handlers globally:

```python
from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse


class DomainError(Exception):
    def __init__(self, code: str, message: str, status: int = 400) -> None:
        self.code = code
        self.message = message
        self.status = status


@app.exception_handler(DomainError)
async def domain_error_handler(request: Request, exc: DomainError) -> JSONResponse:
    return JSONResponse(
        status_code=exc.status,
        content={
            "error": {
                "code": exc.code,
                "message": exc.message,
                "request_id": getattr(request.state, "request_id", None),
            }
        },
    )


@app.exception_handler(RequestValidationError)
async def validation_handler(request: Request, exc: RequestValidationError) -> JSONResponse:
    return JSONResponse(
        status_code=422,
        content={
            "error": {
                "code": "validation_error",
                "details": exc.errors(),
                "request_id": getattr(request.state, "request_id", None),
            }
        },
    )
```

Now any endpoint can `raise DomainError("user_not_found", "no user with id 7", 404)` and get a consistent envelope. Your code stays clean; your API stays uniform.

### Logging middleware (structured)

```python
import logging
import structlog

logger = structlog.get_logger("api.request")


@app.middleware("http")
async def access_log(request: Request, call_next):
    start = time.perf_counter()
    response = await call_next(request)
    elapsed_ms = (time.perf_counter() - start) * 1000
    logger.info(
        "request",
        method=request.method,
        path=request.url.path,
        status=response.status_code,
        duration_ms=round(elapsed_ms, 1),
        request_id=getattr(request.state, "request_id", None),
        client=request.client.host if request.client else None,
    )
    return response
```

Module 14 covers structlog and observability in depth.

### Don't put expensive work in middleware

Middleware runs *for every request*. Reading the request body in middleware is especially expensive — you may consume it before the endpoint sees it (Starlette lets you replay, but with caveats). Keep middleware cheap and side-effect-light.

---

## 4. Practical Application — Production-grade request setup

A `main.py` that puts everything together: request ID, timing, structured logging, CORS, gzip, trusted hosts, error handlers.

```python
# app/main.py
import logging
import time
import uuid
from contextlib import asynccontextmanager

import structlog
from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from fastapi.middleware.cors import CORSMiddleware
from starlette.middleware.gzip import GZipMiddleware
from starlette.middleware.trustedhost import TrustedHostMiddleware

from app.core.config import settings  # pydantic-settings (module 15)
from app.core.errors import DomainError
from app.api.v1 import users, items

logger = structlog.get_logger("app")


@asynccontextmanager
async def lifespan(app: FastAPI):
    logger.info("startup", env=settings.env)
    yield
    logger.info("shutdown")


app = FastAPI(
    title=settings.app_name,
    version=settings.app_version,
    lifespan=lifespan,
)


# --- middleware (order: bottom = outermost) ---

@app.middleware("http")
async def request_context(request: Request, call_next):
    rid = request.headers.get("X-Request-ID") or uuid.uuid4().hex
    request.state.request_id = rid
    structlog.contextvars.bind_contextvars(request_id=rid)

    start = time.perf_counter()
    try:
        response = await call_next(request)
    except Exception:
        logger.exception("unhandled")
        raise
    elapsed_ms = (time.perf_counter() - start) * 1000

    response.headers["X-Request-ID"] = rid
    response.headers["X-Process-Time-MS"] = f"{elapsed_ms:.1f}"
    logger.info(
        "request",
        method=request.method,
        path=request.url.path,
        status=response.status_code,
        duration_ms=round(elapsed_ms, 1),
    )
    structlog.contextvars.clear_contextvars()
    return response


app.add_middleware(GZipMiddleware, minimum_size=1024)

app.add_middleware(
    TrustedHostMiddleware,
    allowed_hosts=settings.allowed_hosts,
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.cors_origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
    expose_headers=["X-Request-ID"],
)


# --- exception handlers ---

@app.exception_handler(DomainError)
async def domain_handler(request: Request, exc: DomainError) -> JSONResponse:
    return JSONResponse(
        status_code=exc.status,
        content={
            "error": {
                "code": exc.code,
                "message": exc.message,
                "request_id": request.state.request_id,
            }
        },
    )


@app.exception_handler(RequestValidationError)
async def validation_handler(request: Request, exc: RequestValidationError) -> JSONResponse:
    return JSONResponse(
        status_code=422,
        content={
            "error": {
                "code": "validation_error",
                "details": exc.errors(),
                "request_id": request.state.request_id,
            }
        },
    )


# --- routers ---

app.include_router(users.router, prefix="/api/v1")
app.include_router(items.router, prefix="/api/v1")


@app.get("/health")
def health() -> dict:
    return {"status": "ok"}
```

This is roughly the skeleton I'd ship to production on day one. Add Prometheus + OpenTelemetry (module 14) and you're 80% of the way.

**Verify CORS preflight**

```bash
curl -i -X OPTIONS http://127.0.0.1:8000/api/v1/users \
     -H "Origin: https://app.example.com" \
     -H "Access-Control-Request-Method: POST"
# HTTP/1.1 200 OK
# access-control-allow-origin: https://app.example.com
# access-control-allow-credentials: true
# access-control-allow-methods: GET, POST, ...
```

---

## 5. Common Mistakes & Gotchas

- **CORS doesn't apply on errors.** If your middleware order is wrong, error responses (raised inside the endpoint) skip CORS, and the browser shows "No 'Access-Control-Allow-Origin'." Solution: add CORS middleware last so it wraps everything.
- **`allow_origins=["*"]` with credentials.** Browsers reject. Either list origins or drop credentials.
- **Middleware consuming the body**, then the endpoint sees nothing. To inspect the body, read it (`await request.body()`), then rewrap. Easier: do the inspection in a dependency where you have a typed model.
- **Adding middleware after startup.** `app.add_middleware()` must be called before the server starts. Otherwise it's silently ignored.
- **Exception handler that itself raises.** Starlette will fall back to a 500 — you'll lose your nice error envelope. Wrap handlers defensively.
- **Logging the full request body** — fine in dev, a GDPR/SOC2 nightmare in prod (passwords, PII). Log path, method, status, duration. Body only with explicit allow-list of fields.
- **Catching `Exception` in middleware** and swallowing — masks bugs. Re-raise after logging.
- **Slow middleware on every request** — 10ms of overhead × millions of requests = wasted infra. Profile middleware specifically.
- **Putting auth in middleware.** It works, but you lose per-route control and OpenAPI doesn't know about it. Use dependencies (module 06).

---

## 🎯 Key Takeaways

- **Middleware is for cross-cutting concerns** — request IDs, timing, logging, CORS, compression. Per-endpoint policy belongs in dependencies.
- **Order: added last = runs first.** CORS goes last in your code so it wraps everything.
- **Exception handlers replace `try/except` boilerplate.** Define a `DomainError` taxonomy, raise from anywhere, handle in one place.
- **CORS is a browser thing, not security.** Configure it correctly so legitimate clients work; don't rely on it for protection.
- **Request IDs in every log line, every response header** — the single highest-ROI observability practice. Adopt before you have an outage, not after.

*← [prev](./09_async_and_concurrency.md) | [next →](./11_websockets_and_server_sent_events.md)*
