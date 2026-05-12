# 01 — Setup & First App

> **Goal:** Get a FastAPI dev environment running with `uv` (or `pip`), write a first endpoint, understand the project layout, and read the auto-generated Swagger/ReDoc docs.

---

## 1. Concept — A FastAPI "app" is a Python object you hand to an ASGI server

FastAPI is a Python web framework built on Starlette (ASGI) and Pydantic. You define an `app` object, decorate functions to register routes, and run it under an ASGI server like **Uvicorn**. There is no "FastAPI runtime" — your app is plain Python objects until Uvicorn drives them.

The smallest possible app:

```python
# main.py
from fastapi import FastAPI

app = FastAPI(title="My First API", version="0.1.0")


@app.get("/")
def read_root() -> dict[str, str]:
    return {"message": "hello, fastapi"}
```

Run it:

```bash
uvicorn main:app --reload
```

Then in another terminal:

```bash
curl http://127.0.0.1:8000/
# {"message":"hello, fastapi"}
```

You now have a working server. Open <http://127.0.0.1:8000/docs> in a browser — Swagger UI is already there. Open <http://127.0.0.1:8000/redoc> — ReDoc too. You wrote zero lines of doc code; it came from your type hints.

---

## 2. Mechanism — How `uv` / `pip` / venv / uvicorn fit together

There are four roles, and beginners conflate them:

| Tool         | Role                                                              |
| ------------ | ----------------------------------------------------------------- |
| `python`     | The interpreter itself                                            |
| `venv` / `uv venv` | An *isolated* Python environment (so projects don't share deps) |
| `pip` / `uv pip` | Installs packages *into* that environment                     |
| `uvicorn`    | An ASGI server that loads and runs your `app` object              |

**Why `uv`?** It's a Rust-based replacement for `pip` and `virtualenv` from Astral (the Ruff people). 10–100× faster, single binary, drop-in compatible. In 2026 it's the default for new projects at most shops. `pip` still works fine — pick one and stick with it.

### Setup with uv (recommended)

```bash
# Install uv once, globally
curl -LsSf https://astral.sh/uv/install.sh | sh    # macOS/Linux
# or: powershell -c "irm https://astral.sh/uv/install.ps1 | iex"  (Windows)

# In your project
mkdir hello-fastapi && cd hello-fastapi
uv venv                          # creates .venv/
source .venv/bin/activate        # Linux/Mac
# .venv\Scripts\activate         # Windows PowerShell

uv pip install "fastapi[standard]"
```

`fastapi[standard]` pulls in uvicorn, httpx (for testing), Jinja2, python-multipart, and email-validator — the "batteries" you almost always want.

### Setup with vanilla pip

```bash
python -m venv .venv
source .venv/bin/activate
pip install "fastapi[standard]"
```

### Running

The modern way (FastAPI 0.110+):

```bash
fastapi dev main.py     # dev: reload on save, prints URLs
fastapi run main.py     # prod-ish: no reload, multiple workers wrapper
```

The classic way (still works, more explicit):

```bash
uvicorn main:app --reload --host 0.0.0.0 --port 8000
```

`main:app` means "import the module `main`, get the attribute `app`." That string is how every ASGI server finds your app.

---

## 3. Project Layout

A one-file `main.py` is fine for a demo. Past ~200 lines, split. The conventional layout:

```
hello-fastapi/
├── .venv/                    # virtualenv, gitignored
├── pyproject.toml            # deps, build config
├── .env                      # local secrets, gitignored
├── README.md
└── app/
    ├── __init__.py
    ├── main.py               # creates FastAPI() and includes routers
    ├── core/
    │   ├── config.py         # pydantic-settings
    │   └── security.py       # JWT, password hashing
    ├── api/
    │   ├── deps.py           # shared dependencies (get_db, get_current_user)
    │   └── v1/
    │       ├── users.py      # APIRouter for /users
    │       └── items.py      # APIRouter for /items
    ├── models/               # SQLAlchemy ORM models
    ├── schemas/              # Pydantic request/response models
    ├── services/             # business logic, no FastAPI imports here
    └── db/
        └── session.py        # engine, SessionLocal
```

The rule that matters: **`services/` must not import from `fastapi`.** That keeps your business logic testable without spinning up an app. We'll enforce this throughout the course.

A starter `pyproject.toml`:

```toml
[project]
name = "hello-fastapi"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = [
    "fastapi[standard]>=0.110",
    "pydantic>=2.6",
    "pydantic-settings>=2.2",
    "sqlalchemy>=2.0",
    "asyncpg>=0.29",
]

[project.optional-dependencies]
dev = [
    "pytest>=8",
    "pytest-asyncio>=0.23",
    "httpx>=0.27",
    "ruff>=0.4",
    "mypy>=1.10",
]
```

Install with `uv pip install -e ".[dev]"`.

---

## 4. Practical Application — A two-endpoint app with proper layout

Let's build the same idea but properly modularized. Two endpoints: `GET /health` and `GET /items/{item_id}`.

**`app/main.py`**

```python
from fastapi import FastAPI

from app.api.v1 import items

app = FastAPI(
    title="Hello FastAPI",
    version="0.1.0",
    description="My first properly-structured FastAPI service.",
)


@app.get("/health", tags=["meta"])
def health() -> dict[str, str]:
    return {"status": "ok"}


app.include_router(items.router, prefix="/api/v1")
```

**`app/api/v1/items.py`**

```python
from fastapi import APIRouter, HTTPException

router = APIRouter(prefix="/items", tags=["items"])

_FAKE_DB: dict[int, dict[str, str]] = {
    1: {"name": "widget"},
    2: {"name": "gadget"},
}


@router.get("/{item_id}")
def get_item(item_id: int) -> dict[str, str]:
    if item_id not in _FAKE_DB:
        raise HTTPException(status_code=404, detail="item not found")
    return _FAKE_DB[item_id]
```

Run:

```bash
fastapi dev app/main.py
```

Test:

```bash
curl http://127.0.0.1:8000/health
# {"status":"ok"}

curl http://127.0.0.1:8000/api/v1/items/1
# {"name":"widget"}

curl -i http://127.0.0.1:8000/api/v1/items/999
# HTTP/1.1 404 Not Found
# {"detail":"item not found"}

curl -i http://127.0.0.1:8000/api/v1/items/abc
# HTTP/1.1 422 Unprocessable Entity
# {"detail":[{"type":"int_parsing", ... }]}
```

That last one is the magic: `item_id: int` automatically rejected `"abc"` with a 422 before your function ran. No `try/except`, no `int(...)` cast. The type hint is enforced.

### Reading the docs

- <http://127.0.0.1:8000/docs> — Swagger UI. Click a route, click "Try it out", fill values, hit Execute. The `tags=` argument groups them.
- <http://127.0.0.1:8000/redoc> — ReDoc. Same data, prettier for sharing.
- <http://127.0.0.1:8000/openapi.json> — Raw OpenAPI 3.1 schema. This is what's pulled into clients, gateways, and Postman.

---

## 5. Common Mistakes & Gotchas

- **Installing FastAPI globally with `sudo pip install`** — pollutes system Python, breaks on next OS update. Always venv.
- **Running `python main.py`** instead of `uvicorn main:app` — works if you add `if __name__ == "__main__": uvicorn.run(app)`, but breaks `--reload` and worker counts. Use the server CLI.
- **`from app import main; main.app`** vs **`main:app`** in uvicorn — the colon is required. It's not a typo.
- **Hardcoding `host="0.0.0.0"` in code** — fine for Docker, bad on laptop (exposes to LAN). Pass it via CLI / env.
- **Editing `.venv/` files** to "fix" something. Never. Reinstall or upgrade the package.
- **`from fastapi import *`** — there's no `__all__`. Be explicit.
- **Confusing `fastapi run` with production-grade serving.** `fastapi run` is a convenience wrapper. For real production use `gunicorn -k uvicorn.workers.UvicornWorker` or a managed runtime — covered in module 15.
- **`--reload` in production.** It uses `watchfiles` and re-imports your module on every change, leaking memory, racing on startup. Dev only.

---

## 🎯 Key Takeaways

- **`fastapi[standard]` + `uv` is the 2026 default setup.** One install, batteries included, fast.
- **`uvicorn main:app` is the universal incantation.** Anything that runs ASGI (Uvicorn, Hypercorn, Daphne, AWS Lambda Web Adapter) speaks this dialect.
- **A folder layout that separates `api/`, `services/`, `models/`, `schemas/` pays compounding interest** as the app grows — start with it, not after.
- **Swagger UI and ReDoc are free.** If you're writing API docs in Confluence by hand, stop.
- **`pyproject.toml` is the single source of truth** for dependencies, Python version, and tool config. No more `requirements.txt` + `setup.py` + `Pipfile` zoo.

*← [prev](./00_roadmap.md) | [next →](./02_path_operations_and_routing.md)*
