# 16 — Web & APIs: requests, httpx, FastAPI
> **Goal:** Call HTTP APIs idiomatically (sync and async) and build a small typed FastAPI service with pydantic models.

## 1. The HTTP client landscape

Three names you'll see:

- **`urllib`** — stdlib, low-level, ugly. Only use it when you can't pip install.
- **`requests`** — the iconic sync HTTP library. Battle-tested; still the right default for sync code.
- **`httpx`** — modern, supports both sync *and* async, near-identical API to `requests`. The 2026 default for new projects.

Mental model: HTTP is request → response. The client handles connection pooling, redirects, decoding, retries (sometimes), and timeouts. Everything else (auth, JSON, headers) is just structured input/output.

```python
# requests — sync
import requests
r = requests.get("https://api.github.com/users/python", timeout=10)
r.raise_for_status()
print(r.json()["public_repos"])

# httpx — sync, almost identical
import httpx
r = httpx.get("https://api.github.com/users/python", timeout=10)
r.raise_for_status()
print(r.json()["public_repos"])

# httpx — async (module 12)
import asyncio
async def main():
    async with httpx.AsyncClient(timeout=10) as client:
        r = await client.get("https://api.github.com/users/python")
        r.raise_for_status()
        print(r.json()["public_repos"])
asyncio.run(main())
```

## 2. Mechanism — a robust HTTP client

Naive `requests.get(url)` works in tutorials and fails in production. The shape you actually want:

```python
import httpx

def make_client() -> httpx.Client:
    return httpx.Client(
        base_url="https://api.example.com",
        timeout=httpx.Timeout(connect=5, read=10, write=10, pool=5),
        headers={"User-Agent": "myapp/1.0"},
        follow_redirects=True,
        limits=httpx.Limits(max_keepalive_connections=20, max_connections=100),
    )

with make_client() as client:
    r = client.get("/users/42")
    r.raise_for_status()
    user = r.json()
```

Key decisions:

- **One client per app, reused.** Connection pooling matters. `requests.get(...)` creates a fresh connection every call.
- **Timeouts on every call.** Default is *no timeout* in `requests`; in `httpx` it's 5 s. Either way, set it explicitly.
- **`raise_for_status()`** to convert 4xx/5xx into exceptions.
- **`base_url`** so callers pass relative paths.

### POST/PUT with JSON

```python
r = client.post("/users", json={"name": "Yati", "email": "y@example.com"})
r.raise_for_status()
created = r.json()
```

`json=` automatically serializes and sets `Content-Type: application/json`. Don't manually `json.dumps` and pass `data=` — that's the old way and easy to get wrong.

### Auth

```python
# Bearer token — most APIs
client = httpx.Client(headers={"Authorization": f"Bearer {token}"})

# Basic auth
client = httpx.Client(auth=("user", "password"))

# OAuth, AWS sig — use a library (httpx-auth, requests-aws4auth)
```

### Retries and backoff

`httpx`/`requests` don't retry by default. For real services, use `tenacity`:

```python
from tenacity import retry, stop_after_attempt, wait_exponential, retry_if_exception_type

@retry(
    stop=stop_after_attempt(5),
    wait=wait_exponential(multiplier=1, min=1, max=30),
    retry=retry_if_exception_type((httpx.HTTPError,)),
)
def fetch(url: str) -> dict:
    r = client.get(url)
    r.raise_for_status()
    return r.json()
```

Don't roll your own retry loop — it's harder than it looks (jitter, distinguishing transient vs permanent errors, total budget).

### Pagination

```python
def iter_users(client):
    url = "/users?page=1"
    while url:
        r = client.get(url)
        r.raise_for_status()
        data = r.json()
        yield from data["items"]
        url = data.get("next")    # API returns next-page URL
```

Generator + `yield from` (module 7) makes pagination one-liner-friendly downstream.

## 3. Variations — building APIs with FastAPI

`FastAPI` (built on `Starlette` + `pydantic`) is the modern Python web framework: async-first, type-driven, auto-generated OpenAPI docs.

```bash
pip install "fastapi[standard]"
```

Minimal app:

```python
# app.py
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, EmailStr, Field

app = FastAPI(title="Users API", version="0.1.0")

class UserIn(BaseModel):
    name: str = Field(min_length=1, max_length=100)
    email: EmailStr

class UserOut(UserIn):
    id: int

_db: dict[int, UserOut] = {}
_next_id = 1

@app.post("/users", response_model=UserOut, status_code=201)
def create_user(user: UserIn) -> UserOut:
    global _next_id
    out = UserOut(id=_next_id, **user.model_dump())
    _db[_next_id] = out
    _next_id += 1
    return out

@app.get("/users/{user_id}", response_model=UserOut)
def get_user(user_id: int) -> UserOut:
    if user_id not in _db:
        raise HTTPException(404, "user not found")
    return _db[user_id]

@app.get("/users", response_model=list[UserOut])
def list_users(limit: int = 10) -> list[UserOut]:
    return list(_db.values())[:limit]
```

Run it:

```bash
fastapi dev app.py
# or:
uvicorn app:app --reload
```

What you get for free:

- **Type-driven validation.** A POST with `{"name": "", "email": "not-an-email"}` returns a 422 with structured error messages — pydantic does the work.
- **Auto-generated OpenAPI docs** at `/docs` (Swagger) and `/redoc`.
- **Dependency injection** via the `Depends()` pattern.
- **Async or sync handlers** — write either; FastAPI runs them appropriately.

### Pydantic models — the heart of FastAPI

Pydantic is the modern data-validation library. Models look like dataclasses but parse, validate, and serialize:

```python
from pydantic import BaseModel, Field, EmailStr, field_validator
from datetime import datetime

class Order(BaseModel):
    id: int
    customer_email: EmailStr
    total_cents: int = Field(ge=0)
    placed_at: datetime
    tags: list[str] = []

    @field_validator("tags")
    @classmethod
    def lowercase_tags(cls, v: list[str]) -> list[str]:
        return [t.lower() for t in v]

# Validates and parses
o = Order.model_validate({
    "id": 1,
    "customer_email": "y@example.com",
    "total_cents": 9900,
    "placed_at": "2026-05-11T12:00:00Z",
    "tags": ["VIP", "RUSH"],
})
o.tags                       # ['vip', 'rush']
o.model_dump_json()          # JSON string
o.model_dump()               # dict
```

Use pydantic for everything that crosses a trust boundary: HTTP requests, config files, environment variables (`pydantic-settings`), message queues. It's the modern Python answer to "validate this dict before I trust it."

### Dependency injection in FastAPI

```python
from fastapi import Depends
from typing import Annotated

def get_db():
    db = connect()
    try:
        yield db
    finally:
        db.close()

DB = Annotated[Connection, Depends(get_db)]

@app.get("/users/{id}")
def get_user(id: int, db: DB) -> UserOut:
    return query_user(db, id)
```

The `Annotated[..., Depends(...)]` pattern is the FastAPI idiom — clean type hint, automatic injection, easy to override in tests.

### Async handlers

```python
@app.get("/external")
async def proxy() -> dict:
    async with httpx.AsyncClient() as client:
        r = await client.get("https://api.example.com")
        return r.json()
```

Use `async def` when the handler awaits I/O. Use plain `def` for CPU-bound or sync DB calls — FastAPI runs them in a threadpool automatically.

## 4. Practical Application — typed client + small service together

A FastAPI mini-service plus a typed httpx client that consumes it. Realistic enough to be a starter for any internal API:

```python
# server.py
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, EmailStr, Field

app = FastAPI()

class UserIn(BaseModel):
    name: str = Field(min_length=1)
    email: EmailStr

class UserOut(UserIn):
    id: int

_db: dict[int, UserOut] = {}
_next = 1

@app.post("/users", response_model=UserOut, status_code=201)
def create(u: UserIn) -> UserOut:
    global _next
    out = UserOut(id=_next, **u.model_dump())
    _db[_next] = out
    _next += 1
    return out

@app.get("/users/{uid}", response_model=UserOut)
def get(uid: int) -> UserOut:
    if uid not in _db:
        raise HTTPException(404, "not found")
    return _db[uid]
```

```python
# client.py
import httpx
from pydantic import BaseModel, EmailStr

class UserIn(BaseModel):
    name: str
    email: EmailStr

class UserOut(UserIn):
    id: int

class UsersClient:
    def __init__(self, base_url: str, timeout: float = 10.0) -> None:
        self._client = httpx.Client(base_url=base_url, timeout=timeout)

    def __enter__(self): return self
    def __exit__(self, *exc): self._client.close()

    def create(self, name: str, email: str) -> UserOut:
        r = self._client.post("/users", json={"name": name, "email": email})
        r.raise_for_status()
        return UserOut.model_validate(r.json())

    def get(self, uid: int) -> UserOut | None:
        r = self._client.get(f"/users/{uid}")
        if r.status_code == 404:
            return None
        r.raise_for_status()
        return UserOut.model_validate(r.json())


if __name__ == "__main__":
    with UsersClient("http://localhost:8000") as users:
        u = users.create("Yati", "yati@example.com")
        print(u)
        print(users.get(u.id))
        print(users.get(9999))    # None
```

Why this is good:

- **Both sides share pydantic models** — symmetric validation, no string-typed dicts.
- **Client is a context manager** — connection pool is cleaned up.
- **Errors are typed:** 4xx via `raise_for_status`, 404 returns `None` (semantic difference). The caller distinguishes "missing" from "broken."
- **No global state in the client** — easy to stub in tests.

For the server side, you'd add: a real DB (SQLAlchemy/SQLModel), proper auth, structured logging (module 17), and metrics. The skeleton above grows into all of that without changing shape.

## 5. Common Mistakes & Gotchas

- **No timeout.** Default `requests.get(url)` waits forever. A single dead upstream can lock up your service. Always set timeouts.
- **One `requests.get(url)` per call** in a hot loop. No connection pooling — TLS handshake each time, ~10× slower. Use `httpx.Client` / `requests.Session`.
- **Manually building query strings.** Use `params={"limit": 10, "q": query}` — handles encoding and types.
- **`json.dumps()` then passing as `data=`.** Wrong content type, wrong encoding. Use `json=` instead.
- **Catching `Exception` from HTTP calls.** Catch `httpx.HTTPError` (or `requests.RequestException`) — be specific.
- **Mixing sync HTTP into async code.** `requests.get()` inside `async def` blocks the loop (module 12). Use `httpx.AsyncClient`.
- **Not raising for 4xx/5xx.** A `200` check isn't enough — check `r.raise_for_status()` or you'll silently process error pages as data.
- **Logging full responses including secrets.** Tokens, cookies, PII. Log carefully or not at all.
- **FastAPI handlers returning untyped dicts.** Add `response_model=` so the response is filtered, validated, and documented in OpenAPI.
- **Pydantic `dict()` (deprecated) instead of `model_dump()`.** Pydantic v2 renamed everything. Use the v2 method names.
- **Storing pydantic models as ORM rows.** Pydantic is for validation/IO, not persistence. Use SQLAlchemy/SQLModel; pydantic at the boundary.

## 🎯 Key Takeaways

- **`httpx` is the modern default** — sync and async share an API, you only learn one. Use a long-lived `Client`/`AsyncClient` and always set timeouts.
- **Pydantic at every trust boundary.** HTTP request bodies, config, env vars, message payloads. Untrusted data should never propagate as a raw `dict`.
- **FastAPI = type hints + pydantic + async + auto OpenAPI.** Once you've used it, every other Python framework feels manual. Learn `Depends` + `Annotated` and you've learned 80% of the framework.
- **Retries belong to a library.** `tenacity` for sync, `tenacity` for async. Don't write your own.
- **Symmetric models on client and server** turn HTTP from "stringly-typed JSON soup" into typed function calls. The reuse pays for itself in days.

*← [prev](./15_performance.md) | [next →](./17_production_patterns.md)*
