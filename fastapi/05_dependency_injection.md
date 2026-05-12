# 05 — Dependency Injection

> **Goal:** Understand `Depends` deeply — sub-dependencies, `yield` for setup/teardown, request-scoped caching, and how the DI tree gives FastAPI apps their composability.

---

## 1. Concept — A dependency is "something else FastAPI runs first"

A dependency is a callable. You declare it with `Depends(...)` in a path operation, and FastAPI:

1. Resolves it before calling your endpoint.
2. Passes the return value as that parameter.
3. Caches it for the rest of this request (by default).

```python
from fastapi import FastAPI, Depends
from typing import Annotated

app = FastAPI()


def common_params(q: str | None = None, limit: int = 10) -> dict[str, object]:
    return {"q": q, "limit": limit}


@app.get("/items")
def list_items(params: Annotated[dict, Depends(common_params)]) -> dict:
    return {"params": params}


@app.get("/users")
def list_users(params: Annotated[dict, Depends(common_params)]) -> dict:
    return {"params": params}
```

The dependency itself has the same parameter machinery as an endpoint — it can take query params, headers, sub-dependencies, anything. The two endpoints now share query-param parsing without copy-paste.

---

## 2. Mechanism — Per-request DI tree resolution

Think of dependencies as a **tree** rooted at your endpoint. Each `Depends(X)` is a child. FastAPI:

1. Walks the tree depth-first.
2. For each node, resolves its own dependencies first.
3. Calls the node.
4. **Caches the result by callable identity** within this request — if `Depends(get_db)` appears five times anywhere in the tree, `get_db()` runs once.
5. After your endpoint returns, walks back up, running teardown for any `yield`-based deps in reverse order.

This is why dependencies feel "magical" — they're really plain function calls that FastAPI orchestrates, with caching and lifecycle. There's no DI container, no annotations, no XML. Just Python callables.

You can disable caching per-use: `Depends(get_db, use_cache=False)`. Rare, but it's the escape hatch if your dependency *has* to run multiple times (e.g., generating a new UUID per call).

---

## 3. Variations & Depth

### Sub-dependencies

```python
from fastapi import Depends, Header, HTTPException
from typing import Annotated


def get_token(authorization: Annotated[str | None, Header()] = None) -> str:
    if not authorization or not authorization.startswith("Bearer "):
        raise HTTPException(401, "missing bearer token")
    return authorization[len("Bearer "):]


def get_current_user(token: Annotated[str, Depends(get_token)]) -> dict:
    if token != "valid-token":  # in real life, decode JWT
        raise HTTPException(401, "invalid token")
    return {"id": 1, "email": "ada@example.com"}


@app.get("/me")
def me(user: Annotated[dict, Depends(get_current_user)]) -> dict:
    return user
```

The tree: `me → get_current_user → get_token → (Header)`. FastAPI resolves bottom-up. If `get_token` raises, the chain stops, the endpoint never runs, the response is 401.

### `yield` dependencies — setup + teardown

```python
from collections.abc import Generator


def get_db_session() -> Generator:
    session = SessionLocal()
    try:
        yield session
    finally:
        session.close()


@app.get("/users/{uid}")
def get_user(uid: int, db: Annotated[Session, Depends(get_db_session)]) -> dict:
    return db.get(User, uid)
```

Everything before `yield` runs before the endpoint. Everything after (including in `finally`) runs after — even if the endpoint raised. This is your bread-and-butter for DB sessions, file handles, transactions, distributed locks.

Async version:

```python
from collections.abc import AsyncGenerator


async def get_db_session() -> AsyncGenerator:
    async with AsyncSessionLocal() as session:
        yield session
```

You can even raise `HTTPException` *after* the `yield` in newer FastAPI versions — useful for "commit failed at teardown" scenarios.

### Class-based dependencies

A class is a callable. Its `__init__` parameters are what FastAPI wires up.

```python
class Pagination:
    def __init__(self, limit: int = 10, offset: int = 0) -> None:
        self.limit = limit
        self.offset = offset


@app.get("/items")
def list_items(p: Annotated[Pagination, Depends()]) -> dict:
    return {"limit": p.limit, "offset": p.offset}
```

Note `Depends()` with no arg — FastAPI uses the type as the dependency.

Useful when you want IDE autocomplete on the dep's attributes, or when the dep holds state across methods.

### Dependencies that don't return anything useful — `dependencies=[]`

Sometimes you only want a side effect: enforce auth, check rate limit, set a header. Use the `dependencies=[...]` argument:

```python
def require_admin(user: Annotated[dict, Depends(get_current_user)]) -> None:
    if not user.get("is_admin"):
        raise HTTPException(403, "admins only")


@app.get("/admin/stats", dependencies=[Depends(require_admin)])
def admin_stats() -> dict:
    return {"users": 1234}
```

The dep runs (and can raise) but isn't injected as a parameter. Cleaner than adding an unused `_=Depends(...)`.

You can attach `dependencies=[...]` at the `APIRouter` or even `FastAPI()` level — every route inherits it. That's how `dependencies=[Depends(require_admin)]` on an `/admin` router protects all admin endpoints with one line.

### Caching nuances

```python
async def get_random_id() -> str:
    return uuid.uuid4().hex


@app.get("/x")
def x(
    a: Annotated[str, Depends(get_random_id)],
    b: Annotated[str, Depends(get_random_id)],
    c: Annotated[str, Depends(get_random_id, use_cache=False)],
) -> dict:
    return {"a": a, "b": b, "c": c}
    # a == b (cached), c != a (cache bypassed)
```

The cache key is the **callable identity**, not the call signature. `Depends(get_db, use_cache=False)` and `Depends(get_db)` are *different* cache keys.

### Scoping beyond request

FastAPI's DI is **request-scoped**. For app-scoped resources (DB engine, HTTP client, ML model), use a **lifespan** context manager:

```python
from contextlib import asynccontextmanager
from fastapi import FastAPI
import httpx


@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.http = httpx.AsyncClient(timeout=10)
    yield
    await app.state.http.aclose()


app = FastAPI(lifespan=lifespan)


@app.get("/proxy")
async def proxy(request: Request) -> dict:
    r = await request.app.state.http.get("https://httpbin.org/get")
    return r.json()
```

Or access via `Depends` reading from `app.state`:

```python
def get_http(request: Request) -> httpx.AsyncClient:
    return request.app.state.http
```

The pattern: **lifespan creates, dependency exposes, request uses.**

---

## 4. Practical Application — `/orders` with DB session + auth dependencies

This is what production FastAPI code actually looks like.

**`app/db/session.py`**

```python
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine

engine = create_async_engine("postgresql+asyncpg://user:pw@localhost/app")
AsyncSessionLocal = async_sessionmaker(engine, expire_on_commit=False)
```

**`app/api/deps.py`**

```python
from collections.abc import AsyncGenerator
from typing import Annotated

from fastapi import Depends, Header, HTTPException, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.db.session import AsyncSessionLocal


async def get_db() -> AsyncGenerator[AsyncSession, None]:
    async with AsyncSessionLocal() as session:
        try:
            yield session
        except Exception:
            await session.rollback()
            raise
        # commit happens in services, not here


DBSession = Annotated[AsyncSession, Depends(get_db)]


async def get_current_user(
    authorization: Annotated[str | None, Header()] = None,
    db: DBSession = ...,
) -> dict:
    if not authorization or not authorization.startswith("Bearer "):
        raise HTTPException(status.HTTP_401_UNAUTHORIZED, "missing bearer token")
    token = authorization[len("Bearer "):]
    # in real life: decode JWT, look up user
    if token != "valid":
        raise HTTPException(status.HTTP_401_UNAUTHORIZED, "invalid token")
    return {"id": 1, "is_admin": False}


CurrentUser = Annotated[dict, Depends(get_current_user)]


def require_admin(user: CurrentUser) -> dict:
    if not user.get("is_admin"):
        raise HTTPException(status.HTTP_403_FORBIDDEN, "admins only")
    return user
```

Notice the **`Annotated` aliases** (`DBSession`, `CurrentUser`). They cut noise in every endpoint signature.

**`app/api/v1/orders.py`**

```python
from fastapi import APIRouter, HTTPException, status
from pydantic import BaseModel

from app.api.deps import CurrentUser, DBSession

router = APIRouter(prefix="/orders", tags=["orders"])


class OrderIn(BaseModel):
    item_id: int
    quantity: int


class OrderOut(OrderIn):
    id: int
    user_id: int


@router.post("", response_model=OrderOut, status_code=status.HTTP_201_CREATED)
async def create_order(
    payload: OrderIn,
    user: CurrentUser,
    db: DBSession,
) -> OrderOut:
    # services/orders.py owns the SQL; this endpoint orchestrates
    # order = await orders_service.create(db, user_id=user["id"], **payload.model_dump())
    # await db.commit()
    return OrderOut(id=99, user_id=user["id"], **payload.model_dump())
```

One endpoint, no auth boilerplate, no session management, fully typed.

**Test with dependency override** (preview of module 12)

```python
from app.api.deps import get_current_user
from app.main import app
from fastapi.testclient import TestClient


def fake_user() -> dict:
    return {"id": 7, "is_admin": True}


def test_create_order_authenticated() -> None:
    app.dependency_overrides[get_current_user] = fake_user
    try:
        client = TestClient(app)
        r = client.post("/api/v1/orders", json={"item_id": 1, "quantity": 2})
        assert r.status_code == 201
        assert r.json()["user_id"] == 7
    finally:
        app.dependency_overrides.clear()
```

`dependency_overrides` is the whole reason DI exists. Tests don't mock; they swap.

---

## 5. Common Mistakes & Gotchas

- **DB session leaks** — using `SessionLocal()` directly in an endpoint instead of `Depends(get_db)` means `.close()` may never run if you raise. Always use `yield`-based deps for resources.
- **Cache surprises with `use_cache`** — if a dep has side effects (logging the request, incrementing a counter), it runs *once per request* by default. To run every time, `use_cache=False`. To run once per app, that's a lifespan singleton, not a dep.
- **Sync dependency calling async code** — your dep returns a coroutine, FastAPI doesn't await it. Make the dep `async def` whenever it needs to await.
- **Async dependency for cheap work** — if it doesn't await, leave it `def`. FastAPI runs sync deps in a threadpool but skips the overhead for trivial work — both are fine. Don't reach for `async` reflexively.
- **`yield` dependency that raises after `yield`** — older FastAPI couldn't translate this into a response. Modern (0.100+) can, but make sure your CI runs on a recent version.
- **Circular dep imports** — `deps.py` imports from `services.py` which imports from `deps.py`. Symptom: `ImportError` on startup. Move shared types to a `types.py` or use string forward refs.
- **Class-based dep with `__init__` doing real work** — runs every request. If it builds a regex or compiles a template, lift that to module scope.
- **Forgetting to clear `dependency_overrides` between tests** — leaks across the suite. Always `try/finally` or use a fixture with teardown.
- **`Depends` outside FastAPI** — `Depends` only resolves when FastAPI is driving. Calling a function with `Depends` defaults from your own code passes the `Depends` object itself. Don't do that.

---

## 🎯 Key Takeaways

- **DI in FastAPI is just functions, plus a resolver.** No container, no annotations, no XML. Read it as plain Python.
- **The tree is per-request, results are cached by callable identity.** This is the single behavior that demystifies "why is this running twice / not running."
- **`yield` deps are how you do resource lifecycle.** DB sessions, file handles, locks, transactions. Setup before yield, cleanup after, exceptions handled.
- **`Annotated` aliases like `DBSession = Annotated[AsyncSession, Depends(get_db)]`** are the production-grade pattern. Define once, reuse everywhere, types still flow through.
- **`dependency_overrides` makes tests trivial.** If you're mocking `get_current_user` with monkeypatch instead of overrides, you're fighting the framework.

*← [prev](./04_request_bodies.md) | [next →](./06_authentication_and_authorization.md)*
