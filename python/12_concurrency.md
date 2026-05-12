# 12 — Concurrency: Threads, Processes, asyncio
> **Goal:** Pick the right concurrency model — threads, processes, or asyncio — based on whether you're I/O-bound or CPU-bound.

## 1. The GIL — what it actually means

CPython has a **Global Interpreter Lock**: only one thread can execute Python bytecode at a time. Practical implications:

- **CPU-bound code in threads doesn't go faster.** Two threads doing math share one core's worth of Python.
- **I/O-bound code in threads *does* go faster.** When a thread is blocked on a socket read, the GIL is released, and another thread runs.
- **C extensions (numpy, requests' SSL, sqlite3) release the GIL** during heavy work.
- **`multiprocessing` sidesteps the GIL** by using OS processes — true parallelism, but with serialization overhead.
- **`asyncio` is single-threaded** but never blocks: cooperative multitasking via `await`.

The decision matrix:

| Workload | Best fit |
|----------|----------|
| Many slow network calls in parallel | **asyncio** (or threads if libraries don't support async) |
| Few blocking I/O calls, mixed with sync code | **threads** (`ThreadPoolExecutor`) |
| Heavy CPU work (parsing, hashing, math without numpy) | **multiprocessing** |
| Heavy CPU work with numpy/pandas | usually fine in threads (releases GIL) |
| 10,000+ concurrent network connections | **asyncio** — the only realistic option |

Note: Python 3.13 introduced an experimental "no-GIL" build (PEP 703). It's the future, but in 2026 you should still architect for the GIL — pure-Python threads remain ~single-core for CPU work.

## 2. Threads — `ThreadPoolExecutor` is your default

Forget `threading.Thread` for app code. Use `concurrent.futures.ThreadPoolExecutor`:

```python
from concurrent.futures import ThreadPoolExecutor, as_completed
import requests

urls = ["https://example.com", "https://python.org", "https://docs.python.org"]

def fetch(url: str) -> tuple[str, int]:
    r = requests.get(url, timeout=5)
    return url, r.status_code

with ThreadPoolExecutor(max_workers=8) as pool:
    # map preserves order
    for url, status in pool.map(fetch, urls):
        print(url, status)

# Or, for finer control + handling completions as they arrive:
with ThreadPoolExecutor(max_workers=8) as pool:
    futures = {pool.submit(fetch, u): u for u in urls}
    for fut in as_completed(futures):
        url = futures[fut]
        try:
            _, status = fut.result()
            print(url, status)
        except Exception as e:
            print(f"{url} failed: {e}")
```

When threads make sense:

- HTTP requests with a sync library (`requests`).
- Reading/writing many files.
- DB calls with sync drivers (`psycopg2`, `pymysql`).

`max_workers` is empirical — start with `min(32, (os.cpu_count() or 1) + 4)` for I/O. Too many threads = context-switch thrash and connection exhaustion.

### Locks for shared state

If multiple threads mutate the same data, you need a lock:

```python
from threading import Lock
counter = 0
lock = Lock()

def bump():
    global counter
    with lock:
        counter += 1     # read-modify-write must be atomic
```

Better: avoid shared state. Each worker returns a result; the main thread aggregates. Functional > shared-mutable.

## 3. Multiprocessing — sidestep the GIL

`ProcessPoolExecutor` parallelizes CPU work. Each task runs in a separate Python process, so the GIL is per-process and you can use all cores:

```python
from concurrent.futures import ProcessPoolExecutor
import hashlib

def heavy(n: int) -> str:
    h = hashlib.sha256()
    for i in range(n):
        h.update(str(i).encode())
    return h.hexdigest()[:8]

if __name__ == "__main__":
    with ProcessPoolExecutor() as pool:
        results = list(pool.map(heavy, [10**6] * 8))
    print(results)
```

The `if __name__ == "__main__":` guard is *required* on Windows and macOS (which use `spawn`) — otherwise the child processes re-import the module and fork-bomb you.

**Costs:**

- Arguments and return values are pickled to send across processes. Big objects = slow.
- Process startup is much heavier than thread startup.
- Globals are not shared (each process is a separate Python).

Use multiprocessing when each task is large enough that pickling overhead is negligible (think "100ms+ of work").

## 4. asyncio — cooperative, non-blocking I/O

asyncio is a single-threaded event loop. Functions marked `async def` are *coroutines* — they suspend at `await` points and let the loop run other coroutines. Perfect for "thousands of concurrent network calls."

```python
import asyncio
import httpx

async def fetch(client: httpx.AsyncClient, url: str) -> tuple[str, int]:
    r = await client.get(url, timeout=5)
    return url, r.status_code

async def main() -> None:
    urls = [f"https://httpbin.org/delay/{i}" for i in range(1, 6)]
    async with httpx.AsyncClient() as client:
        # gather runs them concurrently — one event loop, one thread
        results = await asyncio.gather(*(fetch(client, u) for u in urls))
    for url, status in results:
        print(url, status)

if __name__ == "__main__":
    asyncio.run(main())
```

5 sequential 1–5 second calls would take 15 seconds. With `gather`, ~5 seconds (the slowest one), all from one thread.

### TaskGroup (3.11+) — structured concurrency

```python
async def worker(name: str) -> int:
    await asyncio.sleep(1)
    return len(name)

async def main():
    async with asyncio.TaskGroup() as tg:
        t1 = tg.create_task(worker("alice"))
        t2 = tg.create_task(worker("bob"))
    print(t1.result(), t2.result())
```

If any task raises, all siblings are cancelled and the exceptions are bundled into an `ExceptionGroup` (module 8). Much safer than naked `gather` for production code.

### Async essentials

- **You can only `await` inside an `async def`.**
- **`asyncio.run(coro)` is the entry point** — don't manage event loops by hand.
- **One blocking call ruins everything.** A `time.sleep(5)` inside an async function freezes the entire loop. Use `await asyncio.sleep(5)`.
- **For accidentally-sync calls, use `asyncio.to_thread(fn, *args)`** — runs the function in a thread without blocking the loop:
  ```python
  result = await asyncio.to_thread(slow_blocking_db_call, query)
  ```
- **Never call sync HTTP libraries (`requests`, `urllib`) inside async code** — use `httpx` or `aiohttp`.

### When asyncio shines vs threads

asyncio scales better — a single process can hold tens of thousands of open sockets. Threads top out at hundreds. But asyncio requires the entire library ecosystem to support it (`asyncpg` not `psycopg2`, `httpx` not `requests`, etc.). If your dependencies are sync-only, threads are simpler.

## 5. Practical Application — a concurrent URL checker, three ways

Same task, three implementations. Pick based on workload:

```python
# Shared
URLS = [f"https://httpbin.org/status/{c}" for c in (200, 404, 500, 200, 200)]
TIMEOUT = 5

# --- Threads (sync libraries) ---
from concurrent.futures import ThreadPoolExecutor
import requests

def check_threaded(urls: list[str]) -> dict[str, int]:
    def one(url):
        try:
            return url, requests.get(url, timeout=TIMEOUT).status_code
        except requests.RequestException:
            return url, 0
    with ThreadPoolExecutor(max_workers=16) as pool:
        return dict(pool.map(one, urls))

# --- asyncio (high concurrency) ---
import asyncio
import httpx

async def check_async(urls: list[str]) -> dict[str, int]:
    async def one(client, url):
        try:
            r = await client.get(url, timeout=TIMEOUT)
            return url, r.status_code
        except httpx.HTTPError:
            return url, 0
    async with httpx.AsyncClient() as client:
        results = await asyncio.gather(*(one(client, u) for u in urls))
    return dict(results)

# --- multiprocessing (CPU-bound — overkill here, included for contrast) ---
from concurrent.futures import ProcessPoolExecutor

def check_process(urls: list[str]) -> dict[str, int]:
    def one(url):
        try:
            return url, requests.get(url, timeout=TIMEOUT).status_code
        except requests.RequestException:
            return url, 0
    with ProcessPoolExecutor() as pool:
        return dict(pool.map(one, urls))

if __name__ == "__main__":
    print("threads :", check_threaded(URLS))
    print("asyncio :", asyncio.run(check_async(URLS)))
    # processes also works but pays large overhead for I/O
```

Rule of thumb in production:

- **<100 concurrent I/O calls, sync ecosystem** → threads.
- **>1000 concurrent I/O calls** → asyncio (and accept the rewrite to async libraries).
- **CPU-bound with numpy** → just use threads; numpy releases the GIL.
- **Pure-Python CPU-bound** → multiprocessing or rewrite the hot path in C/Rust.

## Common Mistakes & Gotchas

- **Threads for CPU work in pure Python.** GIL says no. Use processes or numpy.
- **`requests` inside `async def`.** Blocks the entire event loop. Use `httpx` or wrap with `asyncio.to_thread`.
- **`time.sleep` in async code.** Same — freezes the loop. Use `await asyncio.sleep`.
- **No `if __name__ == "__main__":` guard with multiprocessing.** Fork-bomb on Windows/macOS.
- **Sharing complex objects across processes.** Pickling overhead can dominate. Pass small inputs, return small outputs.
- **Forgetting to `await` a coroutine.** `fetch(url)` returns a coroutine object that does nothing until awaited. mypy/pyright catches this; warnings appear at runtime.
- **Catching a coroutine's exception by inspecting `result()` without checking `exception()`.** With raw `gather`, exceptions can be silently aggregated. Prefer `TaskGroup` (3.11+).
- **Spawning threads/tasks without bounds.** "One per request" works until 10K requests blows your memory. Use a pool with `max_workers` or a semaphore.
- **Mixing sync locks with async code.** Use `asyncio.Lock` inside async; `threading.Lock` inside threads. They don't interoperate.
- **Assuming asyncio = parallelism.** It's *concurrency* on a single thread. CPU work still blocks the loop. For real parallelism with async, combine with `loop.run_in_executor` or `asyncio.to_thread`.

## 🎯 Key Takeaways

- **The choice is workload-driven, not preference-driven.** I/O-bound → threads or asyncio; CPU-bound pure-Python → processes; CPU-bound vectorized → threads. Get this right and concurrency becomes easy.
- **`ThreadPoolExecutor` and `asyncio.run` + `TaskGroup` cover ~95% of real-world concurrency.** Skip `threading.Thread` and raw `loop.run_forever()` — too low-level for app code.
- **The GIL only matters for pure-Python CPU work.** For HTTP, files, DB queries, sleeps — the GIL is released and threads work fine.
- **Async is opinionated all the way down.** One sync call (`requests`, `time.sleep`, `psycopg2`) and your event loop stalls. Audit the whole dependency graph before going async.
- **Limit concurrency.** Pools, semaphores, queues — never "spawn one task per item" unbounded. The fastest way to crash a service is to outpace your downstream.

*← [prev](./11_file_io_and_serialization.md) | [next →](./13_testing.md)*
