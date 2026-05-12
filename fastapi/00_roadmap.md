# 00 — FastAPI Deep-Dive Roadmap

> **Goal:** Take a working Python developer from "I can write a function" to "I ship, operate, and reason about FastAPI services in production" — covering FastAPI 0.110+ with Pydantic v2, async patterns, SQLAlchemy 2.0, auth, testing, observability, and deployment.

This is a professional upskilling course. It assumes you already know Python and can read type-annotated code. It does not waste time on basics you can Google — it focuses on the *mental models* that separate "tutorial follower" from "engineer who can debug a 2 AM production incident."

---

## Module Table

| #   | Module                                | Focus                                                                 |
| --- | ------------------------------------- | --------------------------------------------------------------------- |
| 01  | Setup & First App                     | uv/pip, venv, uvicorn, project layout, Swagger/ReDoc                  |
| 02  | Path Operations & Routing             | Methods, params, response_model, status codes, APIRouter              |
| 03  | Pydantic v2 Deep Dive                 | BaseModel, validators, model_config, serialization, v1→v2 diffs       |
| 04  | Request Bodies                        | JSON, form, file uploads, `Annotated` patterns                        |
| 05  | Dependency Injection                  | `Depends`, sub-deps, `yield` deps, caching, scoping                   |
| 06  | Authentication & Authorization        | OAuth2 password, JWT, API keys, OIDC, RBAC                            |
| 07  | Database Integration                  | SQLAlchemy 2.0 async, sessions, Alembic, SQLModel option              |
| 08  | Background Tasks & Long-Running Work  | `BackgroundTasks`, Celery, RQ, Arq — when to pick each                |
| 09  | Async & Concurrency                   | async vs sync endpoints, blocking traps, `run_in_threadpool`, httpx   |
| 10  | Middleware, CORS, Exception Handlers  | Custom middleware, ordering, global exceptions, request IDs           |
| 11  | WebSockets & Server-Sent Events       | Real-time patterns, connection mgmt, scaling                          |
| 12  | Testing                               | `TestClient`, `AsyncClient`, fixtures, dep overrides, factories       |
| 13  | OpenAPI Customization & Client Gen    | response_model, examples, tags, security, typed clients               |
| 14  | Observability & Performance           | Structured logging, Prometheus, OpenTelemetry, profiling              |
| 15  | Production                            | gunicorn+uvicorn, Docker, `pydantic-settings`, proxy headers, health  |

---

## Timeline

Roughly **one module per day = ~2 weeks** of focused study (1.5–2 hrs/day). Heavier modules (05 DI, 06 auth, 07 DB, 14 observability) deserve a second pass and a side-project commit each. If you cram, you'll forget — the goal is durable skill, not speed.

A reasonable schedule:

- **Week 1:** Modules 01–07 (foundations + data + auth)
- **Week 2:** Modules 08–15 (concurrency + ops + production)

After finishing: build one real CRUD service end-to-end (auth, DB, tests, Docker, metrics) as your portfolio piece. That synthesis is where the learning sticks.

---

## Prerequisites

- **Python fluency**, especially **type hints** — FastAPI *is* a type-hint-driven framework. If `list[dict[str, int]]` or `Annotated[str, Depends(...)]` feels foreign, start with [`../python/00_roadmap.md`](../python/00_roadmap.md) and especially the typing module.
- **HTTP basics** — methods, status codes, headers, cookies, MIME types, what JSON is.
- **Async familiarity** — `async`/`await`, the event loop, why blocking calls hurt. See [`../python/12_concurrency.md`](../python/12_concurrency.md) if rusty. You can survive without it for a few modules, but module 09 will be painful.
- A working **terminal + editor** (VS Code or PyCharm both fine).
- **Git** for committing exercise code.

You do **not** need prior FastAPI, Flask, Django, or even web framework experience. You do need to be comfortable reading a stack trace.

---

## Core Mental Models

Five ideas that, once internalized, make the entire framework click:

1. **Type hints ARE the contract.** A parameter's annotation generates validation, the OpenAPI schema, the docs UI, and (when wrapped in `Depends`) a node in the DI graph. You don't write schemas separately from your function signatures — the signatures *are* the schemas. This is the single biggest mindset shift coming from Flask.

2. **Dependency injection is a tree, resolved per request.** Every `Depends(...)` adds a node. FastAPI walks the tree, caches by callable identity within the request (unless `use_cache=False`), runs `yield`-style teardown in reverse order, and only then calls your endpoint. Auth, DB sessions, feature flags, and rate-limit checks all compose through this one mechanism.

3. **Async endpoints need async all the way down.** An `async def` route that calls a synchronous DB driver blocks the event loop, freezing every other request on that worker. Either go fully async (httpx + asyncpg + SQLAlchemy async) or use `def` endpoints (which FastAPI runs in a threadpool). Mixing carelessly is the #1 production performance bug.

4. **Pydantic models are runtime types, not just hints.** Unlike `dataclass` or `TypedDict`, a `BaseModel` validates, coerces, and serializes at runtime. `User(id="42")` succeeds because `"42"` coerces to `int` — that's a feature, but it bites if you assume strictness. In v2, everything is rebuilt in Rust; mental model: "validators run on construction, serializers run on `.model_dump()`."

5. **The OpenAPI schema is the source of truth.** It's auto-generated from your code, but downstream — TypeScript clients, contract tests, API gateways, partner integrations — treats it as canonical. Wrong response_model means wrong client types and broken consumers. Audit `/openapi.json` like you audit your DB schema.

A sixth, optional but powerful:

6. **FastAPI is Starlette + Pydantic + magic.** When debugging deep — middleware, WebSockets, streaming responses — you're really debugging Starlette. Read its source; it's short and clarifying.

---

## External Links

- **[fastapi.tiangolo.com](https://fastapi.tiangolo.com/)** — Official docs. Sebastián Ramírez's tutorial style is excellent; use it as a reference alongside this course.
- **[Pydantic v2 docs](https://docs.pydantic.dev/latest/)** — Especially the migration guide and the "Validators" + "Serialization" pages.
- **[SQLAlchemy 2.0 ORM docs](https://docs.sqlalchemy.org/en/20/orm/)** — The 2.0 style is significantly different from 1.x. Read the "ORM Quick Start" and "Asyncio" pages.
- **["Architecture Patterns with Python" by Percival & Gregory](https://www.cosmicpython.com/)** — Free online. Ports & adapters, repository pattern, unit-of-work. Read this *while* doing module 07. It will reshape how you structure FastAPI apps.
- **[Starlette docs](https://www.starlette.io/)** — What's underneath. Middleware, routing internals, background tasks, WebSockets — all Starlette.
- **[encode/httpx](https://www.python-httpx.org/)** — The async HTTP client you'll use for outbound calls and in tests.

---

## Closing

Web frameworks come and go. The skills here — typed contracts, dependency graphs, async I/O reasoning, schema-driven APIs, production hygiene — transfer to whatever ships next. FastAPI happens to be the best vehicle for learning them right now, *and* it's what's getting hired for. Treat each module as both a feature deep-dive and a chance to sharpen those underlying instincts.

Ship it.

*[next → 01_setup_and_first_app.md](./01_setup_and_first_app.md)*
