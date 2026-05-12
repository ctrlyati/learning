# 13 — OpenAPI Customization & Client Generation

> **Goal:** Treat your OpenAPI schema as the API's source of truth — customize it with examples, tags, security schemes, and generate typed clients (TypeScript, Python) from it.

---

## 1. Concept — The OpenAPI schema is your public contract

FastAPI generates an OpenAPI 3.1 schema automatically from your code. It's exposed at `/openapi.json` and rendered in Swagger UI (`/docs`) and ReDoc (`/redoc`). Anyone integrating with your API — frontend devs, partner teams, gateway proxies, client SDKs — reads this schema. Treat it as production output, not a debugging convenience.

```python
from fastapi import FastAPI

app = FastAPI(
    title="Notes API",
    version="2.4.1",
    description="Internal API for the notes feature.",
    contact={"name": "Platform Team", "email": "platform@example.com"},
    license_info={"name": "Proprietary"},
    openapi_tags=[
        {"name": "users", "description": "User CRUD."},
        {"name": "notes", "description": "Note CRUD and sharing."},
        {"name": "admin", "description": "Administrative endpoints."},
    ],
)
```

The above already gives you a polished docs page. Beyond that, you customize per-route and per-field.

---

## 2. Mechanism — Schema generated from types + decorator metadata

For each route, FastAPI builds the OpenAPI operation by reading:

- The **path** and **method** from the decorator.
- The **parameters** from the function signature (path/query/body/header).
- The **request body** schema from the body's Pydantic model.
- The **response schema** from `response_model=` or the return annotation.
- The **status codes** from `status_code=` and `responses=`.
- The **summary** from the first line of the docstring; **description** from the rest.
- The **tags** from the decorator or router.

Anything you don't write, FastAPI infers. Anything you write, FastAPI uses. There's no separate schema file — your code *is* the spec.

To inspect: `curl http://127.0.0.1:8000/openapi.json | jq '.paths'`.

---

## 3. Variations & Depth

### Examples — make Swagger usable

```python
from pydantic import BaseModel, Field


class ItemIn(BaseModel):
    name: str = Field(examples=["widget"])
    price: float = Field(examples=[9.99])

    model_config = {
        "json_schema_extra": {
            "examples": [
                {"name": "widget", "price": 9.99},
                {"name": "gadget", "price": 19.99},
            ]
        }
    }
```

Or richer multi-example via `Body(openapi_examples=...)`:

```python
from fastapi import Body
from typing import Annotated


@app.post("/items")
def create_item(
    item: Annotated[
        ItemIn,
        Body(openapi_examples={
            "normal": {
                "summary": "Standard item",
                "value": {"name": "widget", "price": 9.99},
            },
            "premium": {
                "summary": "Premium item",
                "value": {"name": "deluxe widget", "price": 99.99},
            },
            "invalid": {
                "summary": "Will be rejected",
                "value": {"name": "", "price": -1.0},
            },
        }),
    ],
) -> ItemIn:
    return item
```

Each example becomes a clickable preset in Swagger UI. Worth doing for any endpoint clients will hand-test.

### Response schemas & multiple status codes

```python
from fastapi import HTTPException, status


@app.get(
    "/users/{user_id}",
    response_model=UserOut,
    responses={
        404: {"model": ErrorOut, "description": "User not found"},
        403: {"model": ErrorOut, "description": "Cannot read this user"},
    },
)
def get_user(user_id: int) -> UserOut:
    ...
```

Clients now know that 404 returns an `ErrorOut` body, not a `UserOut`. Type-safe client generators use this.

### Tags, summary, description, deprecation

```python
@app.post(
    "/items",
    tags=["items"],
    summary="Create an item",
    description="Creates a new item. Requires `items:write` scope.",
    response_description="The created item, with its assigned ID.",
    deprecated=False,
)
def create_item(item: ItemIn) -> ItemOut:
    """Create an item.

    The first line of the docstring overrides `summary=` if you don't pass it.
    The rest of the docstring becomes the `description`.
    Markdown is rendered in Swagger UI.
    """
    ...
```

Use `deprecated=True` for endpoints you're sunsetting — Swagger renders them with a strikethrough.

### `response_model` flags

```python
@app.get(
    "/users/{user_id}",
    response_model=UserOut,
    response_model_exclude_unset=True,    # omit unset fields (PATCH responses)
    response_model_exclude_none=True,     # omit None fields
    response_model_by_alias=True,         # serialize with aliases (camelCase)
)
```

`response_model_exclude_unset` is gold for PATCH: only the fields the client actually changed appear in the response.

### Security schemes in OpenAPI

When you use `OAuth2PasswordBearer`, `HTTPBearer`, or `APIKeyHeader`, FastAPI automatically registers a security scheme in `/openapi.json`. Swagger UI renders an "Authorize" button. Clients pick this up.

For custom schemes or richer descriptions, override `app.openapi`:

```python
from fastapi.openapi.utils import get_openapi


def custom_openapi():
    if app.openapi_schema:
        return app.openapi_schema
    schema = get_openapi(
        title=app.title,
        version=app.version,
        routes=app.routes,
        description=app.description,
    )
    schema["components"]["securitySchemes"]["bearerAuth"] = {
        "type": "http",
        "scheme": "bearer",
        "bearerFormat": "JWT",
        "description": "Paste your JWT token (without 'Bearer ' prefix).",
    }
    schema["security"] = [{"bearerAuth": []}]
    app.openapi_schema = schema
    return schema


app.openapi = custom_openapi
```

### Hide endpoints from docs

```python
@app.post("/internal/notify", include_in_schema=False)
def internal_notify(): ...
```

Or hide an entire router. Useful for internal-only endpoints exposed only inside the cluster.

### Splitting public vs internal docs

Run two FastAPI apps, or use `app.openapi()` filtering. Some teams expose `/docs` (public, filtered) and `/internal/docs` (everything).

---

## 4. Practical Application — Generate a typed TypeScript client

You ship a FastAPI service. A React frontend consumes it. Don't hand-write API types.

### Option A — `openapi-typescript` (just types, you bring your own fetch)

```bash
npx openapi-typescript http://localhost:8000/openapi.json -o ./src/api/schema.d.ts
```

```ts
import type { paths } from "./schema";

type CreateUserBody =
    paths["/api/v1/users"]["post"]["requestBody"]["content"]["application/json"];
type UserOut =
    paths["/api/v1/users/{user_id}"]["get"]["responses"]["200"]["content"]["application/json"];

async function createUser(body: CreateUserBody): Promise<UserOut> {
    const r = await fetch("/api/v1/users", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
    });
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    return r.json();
}
```

Pros: types only, no runtime, framework-agnostic. Pairs beautifully with `openapi-fetch` for a thin typed client.

### Option B — `openapi-typescript-codegen` or `orval` (full client)

Generates classes/functions per endpoint. More opinionated, easier for teams who want one obvious way.

```bash
npx orval --input http://localhost:8000/openapi.json --output ./src/api/client.ts
```

### Option C — Python client (`openapi-python-client`)

For an internal service calling your service:

```bash
openapi-python-client generate --url http://localhost:8000/openapi.json
```

This produces a Pydantic-backed client package. Type-safe, async/sync, ready to publish.

### CI integration

Bake client regeneration into CI:

1. PR opens with API change.
2. CI starts the API, runs `openapi-typescript` against it.
3. If `schema.d.ts` changed: bot opens a PR against the frontend repo. (Or fails the API PR until the schema commit is updated.)

This keeps clients and servers in lock-step. The OpenAPI schema becomes a versioned interface, not an afterthought.

### A worked example endpoint with deluxe docs

```python
# app/api/v1/items.py
from fastapi import APIRouter, Body, HTTPException, Path, status
from typing import Annotated
from pydantic import BaseModel, Field

router = APIRouter(prefix="/items", tags=["items"])


class ItemIn(BaseModel):
    name: str = Field(min_length=1, max_length=100, examples=["widget"])
    price: float = Field(gt=0, examples=[9.99])


class ItemOut(ItemIn):
    id: int = Field(examples=[1])


class ErrorOut(BaseModel):
    code: str = Field(examples=["item_not_found"])
    message: str


@router.post(
    "",
    response_model=ItemOut,
    status_code=status.HTTP_201_CREATED,
    summary="Create an item",
    description="Creates a new item in the catalog. Names must be unique within the seller's namespace.",
    response_description="The created item, including its generated ID.",
    responses={
        409: {"model": ErrorOut, "description": "Item with this name already exists"},
        422: {"description": "Validation error"},
    },
)
def create_item(
    item: Annotated[
        ItemIn,
        Body(openapi_examples={
            "standard": {
                "summary": "Standard widget",
                "value": {"name": "widget", "price": 9.99},
            },
            "premium": {
                "summary": "Premium widget",
                "value": {"name": "deluxe widget", "price": 99.99},
            },
        }),
    ],
) -> ItemOut:
    """Create an item.

    The endpoint:

    - Validates `name` (1–100 chars) and `price` (> 0).
    - Generates an ID.
    - Returns `201 Created` with the full item.
    """
    return ItemOut(id=1, **item.model_dump())


@router.get(
    "/{item_id}",
    response_model=ItemOut,
    responses={404: {"model": ErrorOut, "description": "Item not found"}},
)
def get_item(
    item_id: Annotated[int, Path(ge=1, le=1_000_000, description="Item ID")],
) -> ItemOut:
    """Fetch an item by ID."""
    raise HTTPException(404, "not implemented")
```

Visit `/docs`. The page now has examples, descriptions, error schemas, parameter constraints — exactly what a frontend dev needs.

---

## 5. Common Mistakes & Gotchas

- **`response_model` not narrow enough.** You return a SQLAlchemy `User` with a `hashed_password` field; without `response_model=UserOut`, it's serialized into the response. Leak.
- **Drift between OpenAPI and reality.** Endpoint returns `{"items": [...]}` but `response_model=list[Item]`. Tests catch it; type-check the response in tests.
- **No examples** → Swagger fills with `string`/`0` defaults. Clients can't try things. 60 seconds of `examples=[...]` per field pays off forever.
- **Custom `openapi.json` route returning stale schema.** If you cache `app.openapi_schema` and then mutate routes, you'll serve old data. Don't cache during dev.
- **Bumping FastAPI between OpenAPI 3.0 and 3.1.** Some generators only support 3.0. Pin your tooling or pass `openapi_version="3.0.3"` to `FastAPI(...)`.
- **`responses=` documenting codes you never actually return.** Clients build dead-code branches. Be honest.
- **Documenting non-success status codes via `HTTPException(detail=...)` strings.** Strings are not schemas. Define `ErrorOut` and use `responses={...: {"model": ErrorOut}}`.
- **Versioning by changing path AND OpenAPI title.** Pick one strategy: URL versioning (`/api/v1`), or content-negotiation (`Accept: application/vnd.app.v1+json`). Document the strategy.
- **Auto-generating clients on every PR without a lockfile** → noisy diffs, broken builds when someone forgets to regenerate. Either generate in CI and commit, or generate in client's CI from a pinned API version.
- **Forgetting `include_in_schema=False`** for `/health`, `/metrics`, `/internal/*`. Clutters the docs; sometimes leaks intent.

---

## 🎯 Key Takeaways

- **The OpenAPI schema is the contract.** Audit it as carefully as your DB schema. Diff it in PR reviews.
- **`response_model`, `responses=`, `examples=`, `tags=`, docstrings** — these are not decoration. They make your API usable by humans and machines.
- **Generate clients automatically.** Hand-writing types from API docs is busywork that goes stale.
- **Hide internal endpoints with `include_in_schema=False`.** Public docs should show only what consumers should call.
- **The schema is the *single* source of truth shared between frontend, backend, and partners.** Drift between them is a class of bugs you can eliminate at the tooling layer — do it.

*← [prev](./12_testing.md) | [next →](./14_observability_and_performance.md)*
