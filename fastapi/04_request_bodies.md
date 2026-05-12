# 04 — Request Bodies

> **Goal:** Handle every kind of request payload FastAPI cares about — JSON, form data, multipart file uploads — and use the `Annotated` pattern to disambiguate and validate them.

---

## 1. Concept — The body is whatever isn't a path/query/header/cookie

FastAPI infers "where does this parameter come from" by inspecting types. For a `BaseModel`, that's the JSON body. For `File`, multipart. For `Form`, form-encoded. You can also mix, but FastAPI then needs hints.

```python
from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI()


class ItemIn(BaseModel):
    name: str
    price: float


@app.post("/items")
def create_item(item: ItemIn) -> dict[str, object]:
    return {"received": item.model_dump()}
```

```bash
curl -X POST http://127.0.0.1:8000/items \
     -H "Content-Type: application/json" \
     -d '{"name":"widget","price":9.99}'
# {"received":{"name":"widget","price":9.99}}
```

The function signature says "I take an `ItemIn` from the body." FastAPI parses, validates, hands you the model. JSON content-type required.

---

## 2. Mechanism — `Annotated` is the disambiguator

When you have multiple body-like params, or when you want a *single* scalar in the body, FastAPI can't read your mind. Use `Body`, `Form`, `File`, etc., often via `Annotated[...]`:

```python
from fastapi import FastAPI, Body, Form, File, UploadFile
from typing import Annotated
from pydantic import BaseModel

app = FastAPI()


class ItemIn(BaseModel):
    name: str
    price: float


# Two body params
@app.post("/orders")
def create_order(
    item: ItemIn,
    quantity: Annotated[int, Body(ge=1)],
) -> dict[str, object]:
    return {"item": item.model_dump(), "quantity": quantity}
```

Now FastAPI expects `{"item": {...}, "quantity": N}` in JSON — it nests automatically because there are multiple body params.

`Annotated[T, X]` keeps `T` as the type (for mypy, IDEs) and attaches `X` as metadata for FastAPI. It's the recommended style since FastAPI 0.95+.

---

## 3. Variations & Depth

### Form data (HTML forms)

```python
from fastapi import FastAPI, Form
from typing import Annotated

app = FastAPI()


@app.post("/login")
def login(
    username: Annotated[str, Form()],
    password: Annotated[str, Form()],
) -> dict[str, str]:
    return {"user": username}
```

Requires `python-multipart` (in `fastapi[standard]`). `Content-Type: application/x-www-form-urlencoded`.

```bash
curl -X POST http://127.0.0.1:8000/login \
     -d "username=ada&password=secret"
```

In FastAPI 0.106+, you can use a Pydantic model for the whole form:

```python
from fastapi import Form
from pydantic import BaseModel


class LoginForm(BaseModel):
    username: str
    password: str


@app.post("/login")
def login(form: Annotated[LoginForm, Form()]) -> dict[str, str]:
    return {"user": form.username}
```

### File uploads

`UploadFile` is the right type. It wraps `SpooledTemporaryFile` — small files in memory, big files on disk. Don't use `bytes` for uploads unless you're sure they're tiny.

```python
from fastapi import FastAPI, File, UploadFile, HTTPException
from typing import Annotated

app = FastAPI()

MAX_SIZE = 5 * 1024 * 1024  # 5 MB


@app.post("/upload")
async def upload(
    file: Annotated[UploadFile, File(description="An image")],
) -> dict[str, object]:
    if file.content_type not in {"image/png", "image/jpeg"}:
        raise HTTPException(415, "unsupported media type")

    contents = await file.read()
    if len(contents) > MAX_SIZE:
        raise HTTPException(413, "file too large")

    # save / process
    return {"filename": file.filename, "size": len(contents)}
```

Multiple files:

```python
@app.post("/upload-many")
async def upload_many(
    files: Annotated[list[UploadFile], File()],
) -> list[dict]:
    return [{"name": f.filename, "type": f.content_type} for f in files]
```

Mixed form + file:

```python
@app.post("/submit")
async def submit(
    title: Annotated[str, Form()],
    file: Annotated[UploadFile, File()],
) -> dict[str, str]:
    return {"title": title, "filename": file.filename or ""}
```

```bash
curl -X POST http://127.0.0.1:8000/submit \
     -F "title=My Report" \
     -F "file=@./report.pdf"
```

> **Heads-up:** Once you use `Form()` or `File()`, the body becomes multipart, and you *cannot* also receive JSON in the same request. HTTP allows only one body encoding.

### Singular body values with `Body`

```python
@app.put("/items/{item_id}/price")
def update_price(
    item_id: int,
    new_price: Annotated[float, Body(embed=True, gt=0)],
) -> dict[str, float]:
    return {"item_id": item_id, "price": new_price}
```

`embed=True` makes the request `{"new_price": 19.99}` instead of just `19.99`. Embed when you'll ever want to add more fields later.

### Body + examples

```python
class ItemIn(BaseModel):
    name: str
    price: float
    model_config = {
        "json_schema_extra": {
            "examples": [
                {"name": "widget", "price": 9.99},
                {"name": "gadget", "price": 19.99},
            ]
        }
    }
```

Or via `Body(openapi_examples={...})` for richer multi-example docs.

### Headers, cookies, dependencies on request data

```python
from fastapi import Header, Cookie
from typing import Annotated


@app.get("/me")
def whoami(
    user_agent: Annotated[str | None, Header()] = None,
    session_id: Annotated[str | None, Cookie()] = None,
) -> dict[str, str | None]:
    return {"ua": user_agent, "session": session_id}
```

Headers are case-insensitive; FastAPI converts `User-Agent` ↔ `user_agent`. To disable: `Header(convert_underscores=False)`.

For dependencies on raw `Request`:

```python
from fastapi import Request


@app.get("/ip")
def get_ip(request: Request) -> dict[str, str]:
    return {"ip": request.client.host if request.client else "unknown"}
```

Useful for escape hatches, but reach for it rarely — most things have a typed shortcut.

---

## 4. Practical Application — Avatar upload with metadata

Realistic example: a user uploads an avatar with a caption and visibility flag.

**`app/schemas/uploads.py`**

```python
from pydantic import BaseModel, Field


class AvatarOut(BaseModel):
    id: int
    filename: str
    caption: str
    is_public: bool
    size_bytes: int
    content_type: str
```

**`app/api/v1/uploads.py`**

```python
from fastapi import APIRouter, File, Form, HTTPException, UploadFile, status
from typing import Annotated

from app.schemas.uploads import AvatarOut

router = APIRouter(prefix="/uploads", tags=["uploads"])

ALLOWED = {"image/png", "image/jpeg", "image/webp"}
MAX_BYTES = 2 * 1024 * 1024  # 2 MB
_DB: dict[int, AvatarOut] = {}
_NEXT_ID = 1


@router.post(
    "/avatar",
    response_model=AvatarOut,
    status_code=status.HTTP_201_CREATED,
)
async def upload_avatar(
    file: Annotated[UploadFile, File(description="PNG / JPEG / WebP, ≤ 2 MB")],
    caption: Annotated[str, Form(min_length=1, max_length=200)],
    is_public: Annotated[bool, Form()] = False,
) -> AvatarOut:
    global _NEXT_ID

    if file.content_type not in ALLOWED:
        raise HTTPException(
            status.HTTP_415_UNSUPPORTED_MEDIA_TYPE,
            f"only {sorted(ALLOWED)} allowed",
        )

    data = await file.read()
    if len(data) > MAX_BYTES:
        raise HTTPException(
            status.HTTP_413_REQUEST_ENTITY_TOO_LARGE,
            "file exceeds 2 MB",
        )

    # In real life: persist to S3 / disk, store metadata in DB.
    avatar = AvatarOut(
        id=_NEXT_ID,
        filename=file.filename or "unnamed",
        caption=caption,
        is_public=is_public,
        size_bytes=len(data),
        content_type=file.content_type,
    )
    _DB[_NEXT_ID] = avatar
    _NEXT_ID += 1
    return avatar
```

**Test with curl**

```bash
curl -X POST http://127.0.0.1:8000/api/v1/uploads/avatar \
     -F "file=@./me.png" \
     -F "caption=My Avatar" \
     -F "is_public=true"
# 201
# {"id":1,"filename":"me.png","caption":"My Avatar","is_public":true,
#  "size_bytes":12345,"content_type":"image/png"}
```

Test with the rejection paths:

```bash
# wrong type
curl -X POST http://127.0.0.1:8000/api/v1/uploads/avatar \
     -F "file=@./resume.pdf" -F "caption=test"
# 415

# too big — assume big.png > 2MB
curl -X POST http://127.0.0.1:8000/api/v1/uploads/avatar \
     -F "file=@./big.png" -F "caption=test"
# 413

# missing caption
curl -X POST http://127.0.0.1:8000/api/v1/uploads/avatar \
     -F "file=@./me.png"
# 422
```

**Test in pytest**

```python
# tests/test_uploads.py
import io
from fastapi.testclient import TestClient
from app.main import app


def test_upload_avatar() -> None:
    client = TestClient(app)
    png_bytes = b"\x89PNG\r\n\x1a\n" + b"0" * 100  # fake but plausible
    r = client.post(
        "/api/v1/uploads/avatar",
        files={"file": ("me.png", io.BytesIO(png_bytes), "image/png")},
        data={"caption": "hello", "is_public": "true"},
    )
    assert r.status_code == 201
    assert r.json()["filename"] == "me.png"
```

---

## 5. Common Mistakes & Gotchas

- **Reading huge files with `await file.read()` into memory.** Stream them: iterate `await file.read(chunk_size)` in a loop, or pass `file.file` (the underlying `SpooledTemporaryFile`) to a streaming consumer.
- **Forgetting `python-multipart`.** Without it, `Form()` and `File()` raise `RuntimeError: Form data requires "python-multipart"`. Bundled with `fastapi[standard]`; absent in barebones installs.
- **Trying to combine JSON body and `File()` in the same request.** Won't work — HTTP doesn't allow it. If you want metadata + file, use `Form()` for the metadata fields.
- **Trusting `file.content_type`.** It's set by the client. For real validation, sniff the magic bytes (e.g., `python-magic`) or process with a library that fails on malformed input.
- **No upload size limit at the proxy.** FastAPI's per-endpoint check is fine, but a malicious client can still flood with 10 GB before your code runs. Configure your reverse proxy (`client_max_body_size` in nginx, `--limit-request-line` in gunicorn) too.
- **`UploadFile` after the request closes** — FastAPI cleans up the temp file once your function returns. Don't return references to it; copy the bytes you need first.
- **Async `def` endpoint + synchronous file I/O.** `await file.read()` is async; saving with `open(path, "wb").write(data)` blocks. Use `aiofiles` or `await run_in_threadpool(...)`.
- **`Annotated[X, Body()]` vs `Annotated[X, Form()]` mismatch with client.** If client sends JSON but you ask for `Form()`, you get 422. Pick one and document it.

---

## 🎯 Key Takeaways

- **`Annotated[T, Body|Form|File|Header|Cookie|Query|Path]` is the canonical syntax.** Learn it; everything in modern FastAPI uses it.
- **JSON and multipart are mutually exclusive per request.** Design accordingly — wrap metadata as `Form()` fields when files are involved.
- **`UploadFile` over `bytes`** for any non-trivial file. Memory is cheaper to lose than to debug.
- **Validate content type AND size in the endpoint, AND set proxy limits.** Defense in depth — clients are not your friends.
- **Pydantic models can be used for form bodies too** (FastAPI 0.106+) — treat forms the same way you treat JSON.

*← [prev](./03_pydantic_v2_deep_dive.md) | [next →](./05_dependency_injection.md)*
