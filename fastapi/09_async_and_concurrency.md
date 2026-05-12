# 09 — Async & Concurrency

> **Goal:** Pick `async def` or `def` for every endpoint with confidence, avoid blocking the event loop, and use `httpx` and `run_in_threadpool` correctly when worlds collide.

---

## 1. Concept — One event loop, one worker, many requests

A Uvicorn worker runs a single Python process with a single event loop. Concurrency comes from `await`: while one request waits on I/O, the loop services another. There is **no parallelism inside a worker** for CPU work — that's what `--workers N` and a process manager are for.

```python
from fastapi import FastAPI
import asyncio
import httpx

app = FastAPI()


@app.get("/slow-async")
async def slow_async() -> dict:
    async with httpx.AsyncClient() as client:
        r = await client.get("https://httpbin.org/delay/2")
    return r.json()


@app.get("/slow-sync")
def slow_sync() -> dict:
    import requests
    r = requests.get("https://httpbin.org/delay/2")
    return r.json()
```

Both endpoints take ~2 seconds. The difference: 100 concurrent calls to `/slow-async` are handled by one worker (they all await in parallel on the I/O). 100 concurrent calls to `/slow-sync` get queued — limited by the threadpool size (default 40).

The headline rule:

> **Use `async def` when you `await`. Use `def` when you don't.**

FastAPI runs `def` endpoints in a threadpool automatically. So a sync endpoint *won't* block your loop. What *does* block: a sync call inside an `async def`.

---

## 2. Mechanism — How FastAPI dispatches sync vs async

Pseudocode of FastAPI's per-request flow:

```python
if iscoroutinefunction(endpoint):
    result = await endpoint(...)            # runs on the event loop
else:
    result = await run_in_threadpool(endpoint, ...)   # runs in threadpool
```

`run_in_threadpool` is Starlette's wrapper around `anyio.to_thread.run_sync`. Default threadpool size: **40 threads** (Starlette/AnyIO default). You can raise it but every thread is real memory; usually the right move is to *fix the blocking call*, not grow the pool.

So:

- **Pure sync `def` endpoint** → threadpool. Safe but bounded.
- **Pure async `async def` endpoint with all `await`s on async libs** → event loop. Best.
- **`async def` endpoint that calls a blocking sync function** → DISASTER. The blocking call freezes the event loop for everyone.

The third case is the bug that kills production performance. It's silent — no error, no warning. Just throughput collapse under load.

---

## 3. Variations & Depth

### How do you know if a library is blocking?

- **Async libraries** name themselves: `httpx`, `asyncpg`, `aioredis`, `aiofiles`, `aiomysql`, `SQLAlchemy AsyncSession`. They expose `await` on their methods.
- **Sync libraries**: `requests`, `psycopg2`, `redis`, `pymysql`, `boto3`, regular `open()`. Calling them inside `async def` is the blocking trap.
- When unsure: try `await lib.method()`. If `TypeError: object is not awaitable`, it's sync.

### `run_in_threadpool` — the safety hatch

When you must call a sync library from `async def`:

```python
from fastapi.concurrency import run_in_threadpool


@app.post("/process")
async def process(data: bytes) -> dict:
    # PIL is sync, image processing is CPU-bound
    result = await run_in_threadpool(do_heavy_pil_work, data)
    return result
```

This shoves the call into the threadpool, leaving the loop free. Works for moderate volume — but if 100 requests all need `run_in_threadpool`, you're back to the 40-thread bottleneck. At that point: switch to a real worker queue (module 08) or scale workers.

### `httpx` for outbound HTTP

`httpx` is `requests`'s API with both sync and async modes. The async client supports connection pooling — use it via a long-lived instance:

```python
from contextlib import asynccontextmanager
import httpx
from fastapi import FastAPI, Depends, Request


@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.http = httpx.AsyncClient(
        timeout=httpx.Timeout(10.0, connect=3.0),
        limits=httpx.Limits(max_connections=100, max_keepalive_connections=20),
    )
    yield
    await app.state.http.aclose()


app = FastAPI(lifespan=lifespan)


def get_http(request: Request) -> httpx.AsyncClient:
    return request.app.state.http


@app.get("/weather/{city}")
async def weather(city: str, http: httpx.AsyncClient = Depends(get_http)) -> dict:
    r = await http.get(f"https://wttr.in/{city}?format=j1", timeout=5)
    r.raise_for_status()
    return r.json()
```

**Never** create `AsyncClient()` per request — connection pools take ~100ms to warm up, defeating the point.

### Parallel awaits — `asyncio.gather`

If you call three slow services and they're independent, fan out:

```python
import asyncio


@app.get("/dashboard")
async def dashboard(http: httpx.AsyncClient = Depends(get_http)) -> dict:
    weather, news, stocks = await asyncio.gather(
        http.get("https://api.weather/...").json(),  # wrong — see below
        http.get("https://api.news/...").json(),
        http.get("https://api.stocks/...").json(),
    )
    return {"weather": weather, "news": news, "stocks": stocks}
```

Wait — that's wrong. `httpx.get` returns a `Response`, not a coroutine; you need `.json()` after awaiting. Corrected:

```python
async def fetch(client, url):
    r = await client.get(url)
    r.raise_for_status()
    return r.json()


@app.get("/dashboard")
async def dashboard(http: httpx.AsyncClient = Depends(get_http)) -> dict:
    weather, news, stocks = await asyncio.gather(
        fetch(http, "https://api.weather/..."),
        fetch(http, "https://api.news/..."),
        fetch(http, "https://api.stocks/..."),
    )
    return {"weather": weather, "news": news, "stocks": stocks}
```

3 sequential calls × 200ms = 600ms. With `gather`, ≈ 200ms. This is async's whole point.

### CPU-bound work — `async` won't save you

A `await heavy_math()` doesn't run on another core; it runs on the loop, blocking everything until it returns. For CPU-bound work:

- **Move to a worker queue** (Celery/Arq) on dedicated hosts.
- Or `run_in_threadpool` with caution (helps if there's GIL-releasing C code, like numpy).
- Or `ProcessPoolExecutor` via `loop.run_in_executor(pool, fn, ...)` — bypasses the GIL but copies data across processes.

`async` is for I/O. Read that ten times.

### Timeouts everywhere

Every outbound call needs a timeout. Defaults are usually "forever":

```python
async with httpx.AsyncClient(timeout=httpx.Timeout(10.0, connect=3.0)) as client:
    ...
```

For finer control, wrap with `asyncio.timeout`:

```python
async with asyncio.timeout(5):
    result = await long_call()
```

If a downstream hangs and you have no timeout, your worker accumulates hung requests until it dies of memory or connection exhaustion.

---

## 4. Practical Application — Async aggregator endpoint

Build an endpoint that returns user info + their last 3 orders + their billing status, fan-out to three downstream services in parallel.

```python
# app/api/v1/dashboard.py
import asyncio
import httpx
from fastapi import APIRouter, Depends, HTTPException, Request

from app.api.deps import CurrentUser

router = APIRouter(prefix="/dashboard", tags=["dashboard"])


def get_http(request: Request) -> httpx.AsyncClient:
    return request.app.state.http


async def _fetch(client: httpx.AsyncClient, url: str) -> dict:
    r = await client.get(url, timeout=3.0)
    r.raise_for_status()
    return r.json()


@router.get("")
async def my_dashboard(
    user: CurrentUser,
    http: httpx.AsyncClient = Depends(get_http),
) -> dict:
    uid = user["id"]

    try:
        profile, orders, billing = await asyncio.gather(
            _fetch(http, f"http://users-svc/users/{uid}"),
            _fetch(http, f"http://orders-svc/users/{uid}/orders?limit=3"),
            _fetch(http, f"http://billing-svc/users/{uid}/status"),
        )
    except httpx.HTTPError as e:
        raise HTTPException(502, f"downstream failure: {e}") from e

    return {
        "profile": profile,
        "recent_orders": orders,
        "billing_status": billing,
    }
```

**Test for timing**

```python
import pytest
import httpx
from fastapi.testclient import TestClient
from unittest.mock import AsyncMock, patch


def test_dashboard_fans_out_in_parallel():
    # All three "downstream" calls take 200ms; if serial, total = 600ms.
    # We expect ≈ 200ms.
    ...
```

In real tests, you'd mock with `respx` (an httpx mock library) and assert the calls happened concurrently. The shape:

```python
import respx
import asyncio
from httpx import Response


@respx.mock
async def test_dashboard_parallel():
    respx.get("http://users-svc/users/1").mock(return_value=Response(200, json={"id": 1}))
    respx.get("http://orders-svc/users/1/orders").mock(return_value=Response(200, json=[]))
    respx.get("http://billing-svc/users/1/status").mock(return_value=Response(200, json={"ok": True}))
    # assert response and that wall clock is close to one round trip
```

---

## 5. Common Mistakes & Gotchas

- **`async def` + `requests.get(...)`.** The classic. Blocks the loop for the duration. Use `httpx.AsyncClient`.
- **`async def` + `time.sleep(5)`.** Same problem. Use `await asyncio.sleep(5)`.
- **`async def` + sync ORM (`psycopg2`, `Session.execute(...)`).** Blocks. Use `asyncpg` + `AsyncSession`, or move the endpoint to `def`.
- **No timeout on `httpx`.** Hangs the worker, then the pod, then a region.
- **`AsyncClient()` per request.** Throws away the pool. Use `lifespan` + DI.
- **Mixing `await` and `gather` incorrectly.** `await gather(a, b)` is correct. `gather(await a, await b)` is sequential and worse than serial (creates and immediately awaits).
- **Long CPU work in `async def`.** Tokenizing 10 MB of text takes 5 seconds — your loop is frozen for 5 seconds. Push to a threadpool or worker.
- **Forgetting `async` propagates.** If `get_user_data` is `async`, every caller must `await` it. One missing `await` and you `print(<coroutine object>)` instead of the data — confusing for hours.
- **Threadpool size = 40 is small.** If 50 sync endpoints land at once, the 50th waits. Visible as latency spikes. Either go async or raise it: `anyio.to_thread.current_default_thread_limiter().total_tokens = 100`. Both have tradeoffs.
- **Misreading benchmarks.** A blog says "FastAPI handles 50K req/s." That's a hello-world. With a sync DB call, you'll see 200–500 req/s per worker. Benchmark *your* code.
- **Cancelled tasks.** If a client disconnects mid-request, FastAPI cancels the coroutine. Resources held in `try/finally` clean up correctly — but custom cleanup in `except Exception` won't catch `CancelledError`. Use `try/finally` for cleanup.

---

## 🎯 Key Takeaways

- **`async def` is for `await`-ing I/O. `def` is for everything else.** FastAPI handles both safely; you just have to be consistent within each function.
- **The blocking-call-inside-`async def` trap is the #1 production performance bug** in FastAPI apps. Audit every `async def` for sync I/O.
- **One long-lived `httpx.AsyncClient`, wired via lifespan + dependency.** Per-request clients waste pool warmup.
- **`asyncio.gather` is free speedup** for independent calls. Use it whenever you can fan out.
- **Timeouts on every outbound call.** Always. There is no scenario where "no timeout" is the right answer.

*← [prev](./08_background_tasks_and_long_running_work.md) | [next →](./10_middleware_cors_and_exception_handlers.md)*
