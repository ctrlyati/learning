# 12 — Testing

> **Goal:** Write fast, reliable tests for FastAPI apps — sync `TestClient`, async `httpx.AsyncClient`, pytest fixtures, dependency overrides, and factory patterns.

---

## 1. Concept — Test the app, not the framework

You don't need to start Uvicorn to test FastAPI. The app is just an ASGI application — `TestClient` (sync) and `httpx.AsyncClient` with `ASGITransport` (async) speak to it in-process. No network, no port collisions, no flakiness.

```python
# tests/test_health.py
from fastapi.testclient import TestClient
from app.main import app


def test_health_returns_ok() -> None:
    client = TestClient(app)
    r = client.get("/health")
    assert r.status_code == 200
    assert r.json() == {"status": "ok"}
```

That's a real test. Sub-second. No mocks needed for HTTP.

---

## 2. Mechanism — Two clients, two worlds

| Client | Used in | Drives |
| ------ | ------- | ------ |
| `fastapi.testclient.TestClient` | sync test functions | runs an event loop internally, calls your app |
| `httpx.AsyncClient(transport=ASGITransport(app=app))` | `async def` tests with `pytest-asyncio` | uses the test's loop |

Why both? Some libraries (like `pytest-asyncio` + async DB fixtures) require async tests. Some don't. `TestClient` is dead simple. `AsyncClient` is the right tool for async tests.

```python
# Async style
import pytest
from httpx import AsyncClient, ASGITransport
from app.main import app


@pytest.mark.asyncio
async def test_health_async() -> None:
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as c:
        r = await c.get("/health")
        assert r.status_code == 200
```

---

## 3. Variations & Depth

### `pytest-asyncio` config

```toml
# pyproject.toml
[tool.pytest.ini_options]
asyncio_mode = "auto"        # all async test functions just work
testpaths = ["tests"]
```

With `asyncio_mode = "auto"` you don't need `@pytest.mark.asyncio` on every test.

### Fixtures — the bread and butter

```python
# tests/conftest.py
import pytest
import pytest_asyncio
from httpx import AsyncClient, ASGITransport
from sqlalchemy.ext.asyncio import create_async_engine, async_sessionmaker

from app.main import app
from app.models.base import Base
from app.api.deps import get_db


@pytest_asyncio.fixture
async def db_engine():
    engine = create_async_engine("sqlite+aiosqlite:///:memory:")
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    yield engine
    await engine.dispose()


@pytest_asyncio.fixture
async def db_session(db_engine):
    Session = async_sessionmaker(db_engine, expire_on_commit=False)
    async with Session() as session:
        yield session


@pytest_asyncio.fixture
async def client(db_session):
    async def override_db():
        yield db_session

    app.dependency_overrides[get_db] = override_db
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as c:
        yield c
    app.dependency_overrides.clear()
```

Now every test gets a fresh in-memory DB, a wired-up client, and clean overrides.

### Dependency overrides — the testing superpower

This is *why* DI matters. To swap auth, DB, external services in tests:

```python
from app.api.deps import get_current_user, get_db
from app.main import app


def fake_user():
    return {"id": 42, "is_admin": True}


async def test_admin_only_endpoint(client):
    app.dependency_overrides[get_current_user] = fake_user
    try:
        r = await client.get("/api/v1/admin/stats")
        assert r.status_code == 200
    finally:
        app.dependency_overrides.pop(get_current_user, None)
```

Better: wrap in a fixture.

```python
@pytest_asyncio.fixture
async def authed_client(client):
    app.dependency_overrides[get_current_user] = fake_user
    yield client
    app.dependency_overrides.pop(get_current_user, None)


async def test_admin_works(authed_client):
    r = await authed_client.get("/api/v1/admin/stats")
    assert r.status_code == 200
```

### Factories — building test data

Avoid hand-writing `User(email="a@x.com", full_name="A", ...)` in every test. Use [`factory_boy`](https://factoryboy.readthedocs.io/) or [`polyfactory`](https://polyfactory.litestar.dev/) (preferred for Pydantic):

```python
from polyfactory.factories.pydantic_factory import ModelFactory
from app.schemas.users import UserCreate


class UserCreateFactory(ModelFactory[UserCreate]):
    __model__ = UserCreate


def test_create_user(client):
    payload = UserCreateFactory.build()
    r = client.post("/api/v1/users", json=payload.model_dump())
    assert r.status_code == 201
```

`polyfactory` reads the Pydantic schema and generates valid-by-construction data. You override specific fields when the test needs them: `UserCreateFactory.build(email="duplicate@x.com")`.

### Mocking outbound HTTP with `respx`

When your app calls another service, mock at the transport layer:

```python
import respx
from httpx import Response


@respx.mock
async def test_dashboard_aggregates(client):
    respx.get("http://users-svc/users/1").mock(return_value=Response(200, json={"id": 1, "name": "Ada"}))
    respx.get("http://orders-svc/users/1/orders").mock(return_value=Response(200, json=[]))

    r = await client.get("/api/v1/dashboard", headers={"Authorization": "Bearer t"})
    assert r.status_code == 200
    assert r.json()["profile"]["name"] == "Ada"
```

No real network. Deterministic. Fast.

### Parameterized tests

```python
import pytest


@pytest.mark.parametrize(
    "payload,expected_status",
    [
        ({"email": "ok@x.com", "full_name": "Ok", "password": "Sup3rPass"}, 201),
        ({"email": "bad", "full_name": "Ok", "password": "Sup3rPass"}, 422),
        ({"email": "x@y.com", "full_name": "", "password": "Sup3rPass"}, 422),
        ({"email": "x@y.com", "full_name": "Ok", "password": "short"}, 422),
    ],
)
async def test_user_validation(client, payload, expected_status):
    r = await client.post("/api/v1/users", json=payload)
    assert r.status_code == expected_status
```

One test covers four scenarios. Pytest reports each separately on failure.

### Testing WebSockets

```python
def test_ws_echo() -> None:
    with TestClient(app) as client:
        with client.websocket_connect("/ws/echo") as ws:
            ws.send_text("hello")
            assert ws.receive_text() == "echo: hello"
```

`TestClient` supports WebSockets even though it's "sync" — under the hood it runs an event loop.

### Testing background tasks

`BackgroundTasks` execute synchronously after the response *in `TestClient`* (Starlette guarantees this). So:

```python
def test_signup_sends_email(client, monkeypatch):
    calls = []
    monkeypatch.setattr("app.api.v1.auth.send_welcome_email", lambda e: calls.append(e))
    r = client.post("/auth/signup", json={"email": "a@x.com"})
    assert r.status_code == 201
    assert calls == ["a@x.com"]
```

For Celery / Arq: use the library's eager/sync test mode, or unit-test the task function directly.

---

## 4. Practical Application — Full test file for a `/users` resource

```python
# tests/test_users.py
import pytest
from polyfactory.factories.pydantic_factory import ModelFactory

from app.schemas.users import UserCreate


class UserCreateFactory(ModelFactory[UserCreate]):
    __model__ = UserCreate


@pytest.fixture
def valid_payload() -> dict:
    return UserCreateFactory.build().model_dump()


async def test_create_user_happy(client, valid_payload):
    r = await client.post("/api/v1/users", json=valid_payload)
    assert r.status_code == 201
    data = r.json()
    assert data["email"] == valid_payload["email"]
    assert "id" in data
    assert "password" not in data         # response_model filters it


async def test_get_user_after_create(client, valid_payload):
    created = (await client.post("/api/v1/users", json=valid_payload)).json()
    r = await client.get(f"/api/v1/users/{created['id']}")
    assert r.status_code == 200
    assert r.json()["email"] == valid_payload["email"]


async def test_get_unknown_user_404(client):
    r = await client.get("/api/v1/users/999999")
    assert r.status_code == 404


async def test_duplicate_email_409(client, valid_payload):
    await client.post("/api/v1/users", json=valid_payload)
    r = await client.post("/api/v1/users", json=valid_payload)
    assert r.status_code == 409


@pytest.mark.parametrize(
    "field,value",
    [
        ("email", "not-an-email"),
        ("full_name", ""),
        ("full_name", "x" * 200),
        ("password", "short"),
    ],
)
async def test_validation_errors(client, valid_payload, field, value):
    payload = {**valid_payload, field: value}
    r = await client.post("/api/v1/users", json=payload)
    assert r.status_code == 422
    assert r.json()["error"]["code"] == "validation_error"
```

Run with:

```bash
pytest tests/ -v --cov=app --cov-report=term-missing
```

A test file like this — fixtures, factories, parameterization, override-based isolation — is what production-grade FastAPI testing looks like.

---

## 5. Common Mistakes & Gotchas

- **Not clearing `dependency_overrides` between tests.** Override leaks: test 1 fakes auth, test 2 unexpectedly inherits it. Always use a fixture with teardown, or `try/finally`.
- **Shared DB state across tests.** Tests that depend on order are a recipe for misery. Either use per-test transactions that rollback, or an in-memory DB recreated per test (slower but safer).
- **`TestClient` in an `async def` test.** Don't. Use `AsyncClient` with `ASGITransport`. Mixing causes "event loop is closed" errors.
- **Mocking `requests` when your code uses `httpx`.** Mismatch. Use `respx` for httpx.
- **Hitting real third-party APIs.** Tests become flaky, slow, and rate-limited. Always mock outbound HTTP.
- **Using production-shaped fixtures for unit tests.** A unit test of a service function doesn't need an HTTP client. Test the function directly with a session fixture only.
- **`monkeypatch` instead of dependency overrides.** Works, but bypasses the DI graph and breaks when paths change. Overrides are the idiomatic way.
- **Skipping migrations in test DB setup.** Sometimes models drift from migrations; using `Base.metadata.create_all` hides the drift. Run `alembic upgrade head` against your test DB if you want to catch this.
- **Not testing the error envelope.** If your global exception handler reshapes errors, write tests that assert `{"error": {"code": ...}}` not just status codes.
- **Tests that pass for the wrong reason.** Always make a test fail first (delete a line in the code), confirm it fails, then restore. Otherwise you're shipping placebo coverage.

---

## 🎯 Key Takeaways

- **In-process testing via ASGI transport** — never start a real server in your test suite.
- **Dependency overrides are the cleanest way to swap auth, DB, external services.** No mocking libraries needed for FastAPI itself.
- **Fixtures + factories + parameterization** is the trio that scales test code. Hand-rolled fixtures past 50 tests become unmaintainable.
- **Mock at the transport layer (`respx`), not the function layer.** Closer to reality, less coupled to implementation.
- **A test that doesn't fail when the code is wrong is worse than no test.** Verify by breaking; the green bar only matters after you've seen the red.

*← [prev](./11_websockets_and_server_sent_events.md) | [next →](./13_openapi_customization_and_client_generation.md)*
