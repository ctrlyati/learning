# 08 — Background Tasks & Long-Running Work

> **Goal:** Pick the right tool for asynchronous work that shouldn't block an HTTP response — from `BackgroundTasks` for tiny stuff to Celery/RQ/Arq for real queues.

---

## 1. Concept — A request must return quickly; everything else is "later"

If your endpoint takes more than ~1 second, you're already on borrowed time. Beyond ~10s, clients time out, browsers spin, mobile apps retry, and your worker pool clogs. The cure: do the slow work *after* responding, in a different execution context.

```python
from fastapi import FastAPI, BackgroundTasks

app = FastAPI()


def send_welcome_email(email: str) -> None:
    # imagine SMTP call that takes 3 seconds
    print(f"sent welcome to {email}")


@app.post("/signup", status_code=201)
def signup(email: str, bg: BackgroundTasks) -> dict:
    # ... create user in DB ...
    bg.add_task(send_welcome_email, email)
    return {"status": "ok"}
```

The response goes out immediately; `send_welcome_email` runs after. This is the simplest possible offloading — and it's fine for a narrow band of cases.

---

## 2. Mechanism — `BackgroundTasks` runs in the same process, after response

`BackgroundTasks` is part of Starlette. After your endpoint returns:

1. FastAPI sends the response to the client.
2. Then, in the same Uvicorn worker, runs each `bg.add_task(...)` in order.
3. Sync tasks run in a threadpool; async tasks run on the event loop.

Implications:

- **Same process.** If the worker crashes or you redeploy, the task is lost. No retries, no persistence.
- **Same machine.** Can't scale workers independently from the API.
- **Blocking the worker for slow work.** A 30-second task ties up that worker — fewer concurrent requests served.

That's why it's only safe for tasks that are:

- **Quick** (a few seconds max).
- **Idempotent** or **loseable** if the worker dies (analytics, cache warm-up, fire-and-forget emails to a queue).
- **Non-critical** for the user's next action.

For everything else, you need a real queue.

---

## 3. Variations & Depth — Celery, RQ, Arq, and beyond

| Tool       | Broker           | Sync/Async   | When to use                                                        |
| ---------- | ---------------- | ------------ | ------------------------------------------------------------------ |
| `BackgroundTasks` | n/a (in-process) | both    | < 5s, non-critical, can be lost                                    |
| **RQ**     | Redis            | sync only    | Small/medium apps, simple Python jobs, you already have Redis      |
| **Arq**    | Redis            | **async**    | FastAPI-native feel, async-everything, modern stack                |
| **Celery** | Redis / RabbitMQ | sync (mostly)| Large scale, complex workflows, mature ecosystem, multi-language   |
| **Dramatiq** | Redis / RabbitMQ | sync       | Cleaner Celery alternative, opinionated, less battle-tested        |
| Cloud queues | SQS / Pub/Sub  | both         | Already on AWS/GCP, ops simplicity over feature richness           |

### When to pick each (decision tree)

1. **Task < 5s, can be lost on crash?** → `BackgroundTasks`.
2. **Async ecosystem (httpx, asyncpg)?** → **Arq**.
3. **Sync codebase or need huge scale & maturity?** → **Celery**.
4. **Just want jobs.run() and you already use Redis?** → **RQ**.
5. **All-in on a cloud provider?** → SQS + workers, or Cloud Run jobs.

### Arq example (recommended for new FastAPI projects)

`arq` is by the httpx/uvicorn author. Native asyncio. Tiny API.

**`worker.py`**

```python
from arq.connections import RedisSettings


async def send_email(ctx: dict, to: str, body: str) -> None:
    # await an SMTP client, an SES call, whatever
    print(f"sending to {to}: {body[:40]}...")


class WorkerSettings:
    functions = [send_email]
    redis_settings = RedisSettings(host="localhost")
    max_jobs = 10
```

Run: `arq worker.WorkerSettings`.

**Enqueue from FastAPI**

```python
from contextlib import asynccontextmanager
from arq import create_pool
from arq.connections import RedisSettings
from fastapi import FastAPI


@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.arq = await create_pool(RedisSettings(host="localhost"))
    yield
    await app.state.arq.close()


app = FastAPI(lifespan=lifespan)


@app.post("/signup")
async def signup(email: str) -> dict:
    await app.state.arq.enqueue_job("send_email", email, "Welcome!")
    return {"status": "queued"}
```

Jobs persist in Redis. Worker can be a separate container, separate machine. Crashes are retried.

### Celery example (for established ecosystems)

```python
# celery_app.py
from celery import Celery

celery_app = Celery(
    "tasks",
    broker="redis://localhost:6379/0",
    backend="redis://localhost:6379/1",
)


@celery_app.task(bind=True, max_retries=3)
def send_email(self, to: str, body: str) -> None:
    try:
        # ... SMTP ...
        pass
    except Exception as exc:
        raise self.retry(exc=exc, countdown=2 ** self.request.retries)
```

Run worker: `celery -A celery_app worker --loglevel=info`.

From FastAPI:

```python
@app.post("/signup")
def signup(email: str) -> dict:
    send_email.delay(email, "Welcome!")
    return {"status": "queued"}
```

`celery.delay()` is sync — fine in a `def` endpoint. In `async def`, wrap with `run_in_threadpool` if it ever blocks (rare with Redis broker).

Celery's power: chains, groups, chords, periodic tasks via `beat`, dead-letter queues, multi-broker support. Its pain: configuration sprawl, sync-first design, version pin gymnastics.

### Scheduling periodic work

Don't put cron in your FastAPI process. Use:

- **Celery Beat** (alongside Celery workers).
- **Arq cron jobs** (built into `WorkerSettings`).
- **System cron / Kubernetes CronJob** invoking a CLI script.
- **APScheduler** in a separate process — never in a Uvicorn worker (it'd run N times for N workers).

### Streaming responses (when "background" isn't the answer)

If the user *wants* to wait — e.g., for an LLM streaming tokens — use `StreamingResponse`, not background tasks:

```python
from fastapi.responses import StreamingResponse


async def token_stream():
    for tok in llm.stream("..."):
        yield tok


@app.post("/chat")
async def chat() -> StreamingResponse:
    return StreamingResponse(token_stream(), media_type="text/event-stream")
```

Module 11 (WebSockets/SSE) covers this in depth.

---

## 4. Practical Application — Image processing pipeline with Arq

User uploads an image, gets a job ID instantly. A worker resizes it. The user polls or subscribes for the result.

**`app/api/v1/images.py`**

```python
from fastapi import APIRouter, File, HTTPException, Request, UploadFile, status
from typing import Annotated
from uuid import uuid4
import aiofiles

router = APIRouter(prefix="/images", tags=["images"])

UPLOAD_DIR = "/tmp/uploads"


@router.post("/process", status_code=status.HTTP_202_ACCEPTED)
async def process_image(
    request: Request,
    file: Annotated[UploadFile, File()],
) -> dict[str, str]:
    if file.content_type not in {"image/jpeg", "image/png"}:
        raise HTTPException(415)

    job_id = uuid4().hex
    path = f"{UPLOAD_DIR}/{job_id}.bin"
    async with aiofiles.open(path, "wb") as f:
        while chunk := await file.read(64 * 1024):
            await f.write(chunk)

    await request.app.state.arq.enqueue_job("resize_image", path, job_id, _job_id=job_id)
    return {"job_id": job_id, "status_url": f"/api/v1/images/{job_id}"}


@router.get("/{job_id}")
async def get_status(job_id: str, request: Request) -> dict:
    from arq.jobs import Job
    job = Job(job_id, request.app.state.arq)
    info = await job.info()
    if info is None:
        raise HTTPException(404, "unknown job")
    return {"status": info.status, "result": getattr(info, "result", None)}
```

**`worker.py`**

```python
from PIL import Image
from arq.connections import RedisSettings


async def resize_image(ctx: dict, source_path: str, job_id: str) -> dict:
    out = f"/tmp/uploads/{job_id}.thumb.jpg"
    # PIL is sync — fine here, worker is a different process
    img = Image.open(source_path)
    img.thumbnail((256, 256))
    img.save(out, "JPEG", quality=85)
    return {"thumbnail": out}


class WorkerSettings:
    functions = [resize_image]
    redis_settings = RedisSettings(host="localhost")
```

**Smoke test**

```bash
# upload
curl -X POST http://127.0.0.1:8000/api/v1/images/process \
     -F "file=@./photo.jpg"
# 202 {"job_id":"...","status_url":"/api/v1/images/..."}

# poll
curl http://127.0.0.1:8000/api/v1/images/<job_id>
# {"status":"complete","result":{"thumbnail":"/tmp/uploads/.thumb.jpg"}}
```

The API responds in <100ms. The worker takes whatever time it needs without blocking other requests.

---

## 5. Common Mistakes & Gotchas

- **Treating `BackgroundTasks` as a real queue.** A redeploy or crash drops them. No retries. Use only for "fire and forget" work where loss is OK.
- **Blocking I/O inside `BackgroundTasks` running async.** `bg.add_task(requests.get, url)` blocks the event loop after the response. Use `httpx.AsyncClient` or wrap in `run_in_threadpool`.
- **Sharing DB sessions across the request and the background task.** The request's session closes when the response is sent. The task sees stale or closed connections. Open a fresh session inside the task.
- **No idempotency.** Jobs can be retried. If `send_email` runs twice, the user gets two emails. Either make tasks idempotent (de-dupe by job ID) or accept the duplication and document it.
- **Cron in Uvicorn workers.** N workers = N runs of "the" cron job. Run cron in *one* place: Beat, CronJob, or a dedicated singleton.
- **Worker poisoning** — one bad task crashes the worker, taking down the whole queue's progress on that machine. Wrap task bodies in `try/except` + log; let supervisord/Kubernetes restart.
- **Celery + asyncio mishap.** Celery's `apply_async` is sync. From `async def`, calling it is fine (it doesn't await). But your worker functions are sync — don't try to use async libraries inside without `asyncio.run`. For an async-first stack, Arq is simpler.
- **Heavy Celery `result_backend`** with millions of small jobs — Redis fills up. Set `result_expires` or use `ignore_result=True` for fire-and-forget.
- **Not monitoring queue depth.** When workers fall behind, you have minutes-to-hours before users complain. Prometheus + Grafana + an alert on `queue_depth > N` are mandatory in production.

---

## 🎯 Key Takeaways

- **`BackgroundTasks` is for ≤5-second, non-critical work.** Anything bigger needs a real queue.
- **Arq for async-first FastAPI, Celery for scale and ecosystem, RQ for simplicity.** Decide upfront and commit — switching queue libraries mid-project is painful.
- **Persist jobs in a broker (Redis / RabbitMQ / SQS).** In-memory queues are a developer convenience, not production architecture.
- **Idempotency is the only retry strategy worth deploying.** If a job can't safely run twice, you're going to debug a 3 AM incident.
- **Monitor queue depth like you monitor CPU.** Backlogs are the canary for half your production problems.

*← [prev](./07_database_integration.md) | [next →](./09_async_and_concurrency.md)*
