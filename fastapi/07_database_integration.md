# 07 — Database Integration

> **Goal:** Connect FastAPI to a real SQL database with SQLAlchemy 2.0 async, manage sessions per request, run schema migrations with Alembic, and know when SQLModel beats raw SQLAlchemy.

---

## 1. Concept — One engine app-wide, one session per request

The mental model:

- **Engine** = connection pool. Created once at app startup. Lives for app lifetime.
- **Session** = a unit-of-work. Created per request. Tracks changes, batched into one transaction. Closed on response.
- **ORM models** = Python classes mirroring DB tables.
- **Pydantic schemas** = request/response shapes. NOT the same as ORM models.

These four things are distinct. Conflating them is the most common source of FastAPI+DB pain.

```python
from sqlalchemy.ext.asyncio import create_async_engine, async_sessionmaker

engine = create_async_engine("postgresql+asyncpg://user:pw@localhost/app", pool_size=10)
AsyncSessionLocal = async_sessionmaker(engine, expire_on_commit=False)
```

That's the engine. The session is created per request via a FastAPI dependency (module 05).

---

## 2. Mechanism — SQLAlchemy 2.0 is async-aware and type-friendly

SQLAlchemy 2.0 introduced a new style API: `select(...)`, `session.execute(...)`, typed `Mapped[T]` annotations. The legacy `Query` object still works but is discouraged. Async is first-class via `AsyncSession`.

```python
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column
from sqlalchemy import String, DateTime, func
from datetime import datetime


class Base(DeclarativeBase):
    pass


class User(Base):
    __tablename__ = "users"

    id: Mapped[int] = mapped_column(primary_key=True)
    email: Mapped[str] = mapped_column(String(255), unique=True, index=True)
    full_name: Mapped[str] = mapped_column(String(120))
    hashed_password: Mapped[str] = mapped_column(String(255))
    is_active: Mapped[bool] = mapped_column(default=True)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now()
    )
```

The `Mapped[T]` annotations give you real types — `User(...).id` is `int` to mypy, `User.id` (the column) is a SQLAlchemy `InstrumentedAttribute[int]`. This is a *huge* DX win over 1.x.

To query:

```python
from sqlalchemy import select


async def get_user_by_email(db: AsyncSession, email: str) -> User | None:
    result = await db.execute(select(User).where(User.email == email))
    return result.scalar_one_or_none()
```

`.scalar_one_or_none()` returns the model or None. `.scalars().all()` for lists. `.scalar_one()` raises if zero or many.

---

## 3. Variations & Depth

### Session per request — the dependency

```python
# app/db/session.py
from collections.abc import AsyncGenerator
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine

engine = create_async_engine(
    "postgresql+asyncpg://user:pw@localhost/app",
    pool_size=10,
    max_overflow=20,
    pool_pre_ping=True,
)
AsyncSessionLocal = async_sessionmaker(engine, expire_on_commit=False, autoflush=False)


async def get_db() -> AsyncGenerator[AsyncSession, None]:
    async with AsyncSessionLocal() as session:
        try:
            yield session
            await session.commit()
        except Exception:
            await session.rollback()
            raise
```

Two design choices in that dependency:

- **Commit at end on success** — caller doesn't have to remember. Some teams prefer explicit `await db.commit()` in services. Either works; pick one and standardize.
- **`expire_on_commit=False`** — without it, accessing attributes on a committed object triggers a refresh query. Annoying with async (no implicit IO). Set False, refresh manually if needed.

### CRUD service layer (no FastAPI imports)

```python
# app/services/users.py
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.users import User
from app.core.security import hash_password


async def create_user(db: AsyncSession, *, email: str, full_name: str, password: str) -> User:
    user = User(email=email, full_name=full_name, hashed_password=hash_password(password))
    db.add(user)
    await db.flush()           # populates user.id without committing
    await db.refresh(user)     # ensures server defaults are loaded
    return user


async def get_user(db: AsyncSession, user_id: int) -> User | None:
    return await db.get(User, user_id)


async def list_users(db: AsyncSession, limit: int = 20, offset: int = 0) -> list[User]:
    result = await db.execute(select(User).limit(limit).offset(offset))
    return list(result.scalars())
```

The service is pure: takes a session, returns models. No HTTP, no FastAPI. Testable without spinning up the app.

### The endpoint stitches schemas, deps, and services

```python
# app/api/v1/users.py
from fastapi import APIRouter, HTTPException, status

from app.api.deps import DBSession
from app.schemas.users import UserCreate, UserOut
from app.services import users as users_svc

router = APIRouter(prefix="/users", tags=["users"])


@router.post("", response_model=UserOut, status_code=status.HTTP_201_CREATED)
async def create_user(payload: UserCreate, db: DBSession) -> UserOut:
    existing = await users_svc.get_user_by_email(db, payload.email)
    if existing:
        raise HTTPException(409, "email already registered")
    user = await users_svc.create_user(db, **payload.model_dump())
    return user  # UserOut has from_attributes=True


@router.get("/{user_id}", response_model=UserOut)
async def get_user(user_id: int, db: DBSession) -> UserOut:
    user = await users_svc.get_user(db, user_id)
    if not user:
        raise HTTPException(404, "user not found")
    return user
```

Note the layering:

- **Endpoint:** HTTP concerns (status codes, error mapping).
- **Service:** Business logic, no HTTP.
- **Model:** Schema-bound, no logic.

This is "ports and adapters" in miniature. Read the *Architecture Patterns with Python* book for the full treatment.

### Alembic migrations

Schema changes need version control. Alembic is the migration tool for SQLAlchemy.

```bash
uv pip install alembic
alembic init -t async migrations
```

Edit `migrations/env.py` to point at your `Base.metadata`:

```python
# migrations/env.py (excerpt)
from app.models.base import Base
target_metadata = Base.metadata
```

Configure DB URL in `alembic.ini` (or override via env var in `env.py`).

Generate and apply:

```bash
alembic revision --autogenerate -m "add users table"
alembic upgrade head
```

Common workflow:

1. Change the SQLAlchemy model.
2. `alembic revision --autogenerate -m "..."`.
3. Hand-edit the generated migration if it's wrong (autogen misses renames, enum changes).
4. `alembic upgrade head` locally.
5. Commit the migration file.
6. CI/CD runs `alembic upgrade head` on deploy.

Never delete migration files in git. Never edit one that has been applied to a shared environment.

### SQLModel option

[SQLModel](https://sqlmodel.tiangolo.com/) (by the FastAPI author) unifies SQLAlchemy + Pydantic. One class is both your ORM model and your schema:

```python
from sqlmodel import SQLModel, Field


class User(SQLModel, table=True):
    id: int | None = Field(default=None, primary_key=True)
    email: str = Field(index=True, unique=True)
    full_name: str
```

Pros: less duplication, faster prototyping.
Cons: tighter coupling (your DB schema and your API schema become the same thing — sometimes you want them different), thinner ecosystem than SQLAlchemy proper, occasional Pydantic v2 friction.

**My recommendation:** Use SQLAlchemy + Pydantic for serious apps. Use SQLModel for prototypes and small services where the schema split is overhead. They share enough vocabulary that switching later is feasible.

### Connection pool sizing

Defaults: `pool_size=5`, `max_overflow=10`. Production rule of thumb:

```
pool_size ≈ (workers × per-worker concurrency) / 2
```

Postgres default `max_connections` is 100. If you have 4 workers × pool_size=10 × overflow=20 = 120 possible connections — you'll hit the cap. Either lower pool sizes or use PgBouncer in front.

`pool_pre_ping=True` adds a `SELECT 1` before each checkout — catches stale connections after DB restarts. Tiny perf cost, large robustness win. Use it.

---

## 4. Practical Application — Full async CRUD with tests

**`app/models/users.py`** (already shown above)

**`app/schemas/users.py`**

```python
from datetime import datetime
from pydantic import BaseModel, ConfigDict, EmailStr, Field


class UserBase(BaseModel):
    email: EmailStr
    full_name: str = Field(min_length=1, max_length=120)


class UserCreate(UserBase):
    password: str = Field(min_length=8, max_length=72)


class UserOut(UserBase):
    model_config = ConfigDict(from_attributes=True)
    id: int
    is_active: bool
    created_at: datetime
```

**`app/services/users.py`** (already shown)

**`app/api/v1/users.py`** (already shown)

**`tests/conftest.py`**

```python
import pytest
import pytest_asyncio
from httpx import AsyncClient, ASGITransport
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine

from app.main import app
from app.models.base import Base
from app.api.deps import get_db


@pytest_asyncio.fixture
async def db_session():
    engine = create_async_engine("sqlite+aiosqlite:///:memory:")
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    Session = async_sessionmaker(engine, expire_on_commit=False)
    async with Session() as s:
        yield s
    await engine.dispose()


@pytest_asyncio.fixture
async def client(db_session: AsyncSession):
    async def override_db():
        yield db_session
    app.dependency_overrides[get_db] = override_db
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as c:
        yield c
    app.dependency_overrides.clear()
```

**`tests/test_users.py`**

```python
import pytest


@pytest.mark.asyncio
async def test_create_and_read_user(client) -> None:
    r = await client.post(
        "/api/v1/users",
        json={"email": "ada@example.com", "full_name": "Ada", "password": "S3cretPass"},
    )
    assert r.status_code == 201
    uid = r.json()["id"]

    r = await client.get(f"/api/v1/users/{uid}")
    assert r.status_code == 200
    assert r.json()["email"] == "ada@example.com"


@pytest.mark.asyncio
async def test_duplicate_email_rejected(client) -> None:
    payload = {"email": "x@y.com", "full_name": "X", "password": "Sup3rPass"}
    await client.post("/api/v1/users", json=payload)
    r = await client.post("/api/v1/users", json=payload)
    assert r.status_code == 409
```

---

## 5. Common Mistakes & Gotchas

- **Async dependency + sync DB driver.** `psycopg2` blocks the event loop. Use `asyncpg` (Postgres) or `aiomysql` / `aiosqlite`. Or use `def` endpoints with sync SQLAlchemy.
- **Session lifetime longer than a request.** Sessions cache identity-mapped objects. Holding one across requests = stale data + memory growth + race conditions. Always one-per-request.
- **`SELECT N+1`.** Accessing a relationship on each item in a list triggers a query per item. Use `selectinload(User.orders)` or `joinedload(User.orders)` to eager-load.
- **`response_model` mismatch with ORM model** — without `from_attributes=True`, Pydantic raises "Input should be a valid dictionary." Set it on every output schema.
- **Migrations diverge from models.** Someone edits a model but forgets `alembic revision`. Run `alembic check` (1.13+) in CI to catch drift.
- **Connection pool exhausted under load.** Symptoms: `TimeoutError: QueuePool limit ... overflow ... reached`. Usually a session leak (forgot `await session.close()` or a `yield`-based dep), not a pool size problem.
- **Editing applied migrations.** Don't. Add a new migration that fixes it. Editing breaks any environment that's already past that revision.
- **SQLite in production.** SQLite + async + write contention = `database is locked`. Fine for tests, dev, embedded. Not for a real API.
- **Storing money as `float`.** Use `Numeric(precision, scale)` / `Decimal`. `0.1 + 0.2 != 0.3` is not a meme; it's bug-causing reality.
- **`expire_on_commit=True` (default)** + async session — accessing attributes after commit triggers a sync refresh. Set to `False`.

---

## 🎯 Key Takeaways

- **Engine = singleton, session = per-request, ORM model ≠ schema.** Internalize the split; everything else falls out of it.
- **SQLAlchemy 2.0 async with `Mapped[T]` annotations is the modern style.** No more `Query`, no more loose typing. Treat 1.x patterns as legacy.
- **Service layer with no FastAPI imports** keeps your business logic testable and reusable (CLI, worker, second API surface). Worth the discipline from day one.
- **Alembic is non-negotiable** for any DB you'll redeploy. Generate, hand-check, commit, deploy. `alembic check` in CI catches drift.
- **SQLModel is great for prototypes, SQLAlchemy for serious work.** Pick deliberately — the cost of switching later is real but not catastrophic if your service layer is clean.

*← [prev](./06_authentication_and_authorization.md) | [next →](./08_background_tasks_and_long_running_work.md)*
