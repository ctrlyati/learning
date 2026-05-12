# 02 — Path Operations & Routing

> **Goal:** Master the building blocks of every FastAPI app — HTTP methods, path & query params, response models, status codes, and how `APIRouter` keeps things modular.

---

## 1. Concept — A "path operation" is the (method, path) pair bound to a function

In FastAPI, a *path operation* is a function decorated with `@app.get("/x")`, `@app.post("/x")`, etc. The decorator says: "when an HTTP `GET` comes in for path `/x`, call this function." That's it. Everything else — validation, docs, DI — is metadata layered on top.

```python
from fastapi import FastAPI

app = FastAPI()


@app.get("/")
def root() -> dict[str, str]:
    return {"hello": "world"}


@app.post("/items")
def create_item(name: str) -> dict[str, str]:
    return {"created": name}
```

The decorators are `@app.get`, `@app.post`, `@app.put`, `@app.patch`, `@app.delete`, `@app.head`, `@app.options`, `@app.trace`. There's also `@app.api_route("/x", methods=["GET", "HEAD"])` for multi-method handlers.

---

## 2. Mechanism — Type hints drive validation, OpenAPI, and DI

When FastAPI inspects a route function, it classifies each parameter:

1. **In the path?** (e.g. `item_id` matches `/items/{item_id}`) → **path param**.
2. **A `Pydantic BaseModel`?** → **request body**.
3. **A `File`, `Form`, `Header`, `Cookie`, `Body`, `Query` marker?** → that location.
4. **Wrapped in `Depends(...)`?** → **dependency**.
5. **Everything else** → **query param**.

Then it applies the type annotation as a validator. `int` parses & range-checks. `Literal["a", "b"]` enforces enum. A `BaseModel` parses JSON. A `datetime` parses ISO 8601. Failures become 422 responses automatically.

The function signature is simultaneously:

- The validation schema.
- The OpenAPI input description.
- The dependency-injection graph.

This is the design — when you fight it, you lose. When you lean into it, the framework does enormous work for free.

---

## 3. Variations & Depth

### Path parameters

```python
from fastapi import FastAPI, Path
from typing import Annotated

app = FastAPI()


@app.get("/users/{user_id}")
def get_user(user_id: int) -> dict[str, int]:
    return {"user_id": user_id}


@app.get("/files/{file_path:path}")  # converter — matches slashes
def get_file(file_path: str) -> dict[str, str]:
    return {"path": file_path}


@app.get("/items/{item_id}")
def get_item(
    item_id: Annotated[int, Path(ge=1, le=10_000, description="Item ID")],
) -> dict[str, int]:
    return {"item_id": item_id}
```

The `:path` converter is the only built-in one — everything else (regex, etc.) goes through `Path(...)`.

### Query parameters

Any non-path scalar param is a query param:

```python
@app.get("/search")
def search(
    q: str,                      # required
    limit: int = 10,             # optional with default
    offset: int = 0,
    include_archived: bool = False,
) -> dict[str, object]:
    return {"q": q, "limit": limit, "offset": offset}
```

```bash
curl "http://127.0.0.1:8000/search?q=widget&limit=5&include_archived=true"
```

For richer validation, use `Annotated[..., Query(...)]`:

```python
from fastapi import Query
from typing import Annotated

@app.get("/search")
def search(
    q: Annotated[str, Query(min_length=2, max_length=64, pattern=r"^[\w\s]+$")],
    limit: Annotated[int, Query(ge=1, le=100)] = 10,
    tags: Annotated[list[str] | None, Query()] = None,
) -> dict[str, object]:
    return {"q": q, "limit": limit, "tags": tags or []}
```

Note `tags: list[str]` — FastAPI knows to accept `?tags=a&tags=b&tags=c`.

> **Pydantic v1 → v2 note:** `regex=` was renamed to `pattern=` in v2-aligned FastAPI. If you're porting old code, this trips often.

### Response models & status codes

```python
from pydantic import BaseModel
from fastapi import status


class ItemOut(BaseModel):
    id: int
    name: str
    price: float


class ItemIn(BaseModel):
    name: str
    price: float


@app.post(
    "/items",
    response_model=ItemOut,
    status_code=status.HTTP_201_CREATED,
    responses={
        409: {"description": "Item already exists"},
    },
)
def create_item(item: ItemIn) -> ItemOut:
    return ItemOut(id=42, **item.model_dump())
```

`response_model=ItemOut` does two things:

1. Filters the return value — any extra fields you accidentally leak (a password hash, an internal flag) are stripped.
2. Defines the schema in OpenAPI for clients.

If your annotation already matches what you return, you can skip `response_model=` and just rely on the return type. The argument exists for cases where the runtime return type is wider than the public contract.

### APIRouter & modularization

```python
# app/api/v1/users.py
from fastapi import APIRouter, HTTPException
from pydantic import BaseModel

router = APIRouter(prefix="/users", tags=["users"])


class UserOut(BaseModel):
    id: int
    email: str


@router.get("/{user_id}", response_model=UserOut)
def get_user(user_id: int) -> UserOut:
    if user_id != 1:
        raise HTTPException(404, "user not found")
    return UserOut(id=1, email="ada@example.com")
```

```python
# app/main.py
from fastapi import FastAPI
from app.api.v1 import users, items

app = FastAPI()
app.include_router(users.router, prefix="/api/v1")
app.include_router(items.router, prefix="/api/v1")
```

Routers can be nested:

```python
admin = APIRouter(prefix="/admin", tags=["admin"])
admin.include_router(users.router)
app.include_router(admin)
# resulting paths: /admin/users/{user_id}
```

You can attach dependencies, tags, and responses *at the router level* so every route inherits them — useful for auth:

```python
from fastapi import Depends
from app.api.deps import require_admin

router = APIRouter(
    prefix="/admin",
    tags=["admin"],
    dependencies=[Depends(require_admin)],
    responses={403: {"description": "Forbidden"}},
)
```

---

## 4. Practical Application — A small `/items` API with router + schemas

**`app/schemas/items.py`**

```python
from pydantic import BaseModel, Field


class ItemIn(BaseModel):
    name: str = Field(min_length=1, max_length=100)
    price: float = Field(gt=0)
    tags: list[str] = Field(default_factory=list)


class ItemOut(ItemIn):
    id: int
```

**`app/api/v1/items.py`**

```python
from fastapi import APIRouter, HTTPException, Query, status
from typing import Annotated

from app.schemas.items import ItemIn, ItemOut

router = APIRouter(prefix="/items", tags=["items"])

_DB: dict[int, ItemOut] = {}
_NEXT_ID = 1


@router.get("", response_model=list[ItemOut])
def list_items(
    limit: Annotated[int, Query(ge=1, le=100)] = 20,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> list[ItemOut]:
    return list(_DB.values())[offset : offset + limit]


@router.post("", response_model=ItemOut, status_code=status.HTTP_201_CREATED)
def create_item(payload: ItemIn) -> ItemOut:
    global _NEXT_ID
    item = ItemOut(id=_NEXT_ID, **payload.model_dump())
    _DB[_NEXT_ID] = item
    _NEXT_ID += 1
    return item


@router.get("/{item_id}", response_model=ItemOut)
def get_item(item_id: int) -> ItemOut:
    item = _DB.get(item_id)
    if item is None:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "item not found")
    return item


@router.delete("/{item_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_item(item_id: int) -> None:
    if item_id not in _DB:
        raise HTTPException(404, "item not found")
    del _DB[item_id]
```

**Smoke test with curl**

```bash
curl -X POST http://127.0.0.1:8000/api/v1/items \
     -H "Content-Type: application/json" \
     -d '{"name":"widget","price":9.99,"tags":["red"]}'
# {"name":"widget","price":9.99,"tags":["red"],"id":1}

curl "http://127.0.0.1:8000/api/v1/items?limit=10"
curl http://127.0.0.1:8000/api/v1/items/1
curl -X DELETE http://127.0.0.1:8000/api/v1/items/1 -i
# HTTP/1.1 204 No Content
```

Same with httpie (often nicer):

```bash
http POST :8000/api/v1/items name=widget price:=9.99 tags:='["red"]'
```

**Minimal test (we'll go deep in module 12)**

```python
# tests/test_items.py
from fastapi.testclient import TestClient
from app.main import app


def test_create_and_get_item() -> None:
    client = TestClient(app)
    r = client.post("/api/v1/items", json={"name": "x", "price": 1.0})
    assert r.status_code == 201
    item_id = r.json()["id"]

    r = client.get(f"/api/v1/items/{item_id}")
    assert r.status_code == 200
    assert r.json()["name"] == "x"


def test_invalid_price_rejected() -> None:
    client = TestClient(app)
    r = client.post("/api/v1/items", json={"name": "x", "price": -1.0})
    assert r.status_code == 422  # gt=0 violated
```

---

## 5. Common Mistakes & Gotchas

- **Order of route declarations matters.** `/users/me` must be declared *before* `/users/{user_id}` or the latter will catch `"me"` and try `int("me")` → 422.
- **Returning a `dict` when `response_model` is set.** Works (Pydantic re-validates), but you lose type checking. Prefer constructing the model.
- **`response_model_exclude_unset=True`** to omit defaults — useful for PATCH responses. Forget it and clients see `null` fields they think are real.
- **Confusing `status_code=` (on the decorator) with `HTTPException`.** The decorator sets the default success code. `HTTPException` sets an error code at raise time.
- **Mutable defaults in a Pydantic field**: `tags: list[str] = []` — wrong, all instances share the list. Use `Field(default_factory=list)`. (Pydantic v2 catches this with an error; v1 silently broke.)
- **Forgetting `Annotated`.** The older `tags: list[str] = Query(default=None)` style still works but FastAPI's docs now use `Annotated` exclusively, and so does this course. It's clearer and plays nice with `mypy`.
- **Routers without a `prefix`** then including with a prefix — the final path is `prefix + router.prefix + route.path`. Easy to double-prefix.
- **Setting `tags=` per route AND on the router** — they merge, you get duplicates in Swagger.

---

## 🎯 Key Takeaways

- **Method + path + function = one path operation.** Everything else is metadata.
- **Path params, query params, bodies, and dependencies are distinguished by type and decoration**, not by position or name. Learn the rules once, never look them up again.
- **`response_model` is your public contract.** Use it to *narrow* what leaves your server, not just to document.
- **`APIRouter` is how real apps scale** — by feature, by version, by auth boundary. Use it from day one.
- **Validation errors are 422, not 400.** This is a FastAPI convention (RFC 4918 "Unprocessable Entity"). Document it for your API consumers; they'll otherwise wonder.

*← [prev](./01_setup_and_first_app.md) | [next →](./03_pydantic_v2_deep_dive.md)*
