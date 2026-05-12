# 03 — Pydantic v2 Deep Dive

> **Goal:** Understand Pydantic v2 as the runtime type system underlying FastAPI — `BaseModel`, validators, `model_config`, serialization, and the migration surprises coming from v1.

---

## 1. Concept — `BaseModel` is a runtime, validating type

Pydantic is a data-validation library. A `BaseModel` subclass declares fields with type hints; constructing an instance triggers validation and coercion. It's the *runtime* counterpart to `mypy`'s static checks.

```python
from pydantic import BaseModel
from datetime import datetime


class User(BaseModel):
    id: int
    email: str
    is_active: bool = True
    created_at: datetime


u = User(id="42", email="ada@x.com", created_at="2026-01-01T10:00:00")
# {'id': 42, 'email': 'ada@x.com', 'is_active': True,
#  'created_at': datetime.datetime(2026, 1, 1, 10, 0)}
print(u.id)             # 42 (int — coerced from string)
print(u.created_at)     # datetime (parsed from ISO 8601)

u.model_dump()          # → dict
u.model_dump_json()     # → JSON string
```

The headline facts:

- v2 is written in **Rust** (`pydantic-core`). Validation is 5–50× faster than v1.
- Fields support **type coercion by default** (loose mode). For strictness, configure it.
- Errors are rich `ValidationError` objects with a list of issue dicts — FastAPI surfaces them as 422 responses.

---

## 2. Mechanism — How Pydantic v2 plugs into FastAPI

When FastAPI sees a parameter typed as a `BaseModel`, it:

1. Reads the model's **JSON Schema** (`Model.model_json_schema()`) to populate OpenAPI.
2. At request time, parses the JSON body and calls `Model.model_validate(payload)`.
3. On success: hands the instance to your function.
4. On `ValidationError`: returns 422 with the error list, no exception bubbles up.

For response serialization with `response_model`:

1. Your function returns a value.
2. FastAPI calls `Model.model_validate(value)` (or accepts the model if already correct).
3. Calls `Model.model_dump(mode="json")` to produce the response JSON.

This means **every BaseModel field is a runtime gate.** Use that — don't think of them as decorative.

---

## 3. Variations & Depth

### Fields and constraints

```python
from pydantic import BaseModel, Field, EmailStr
from typing import Annotated


class UserCreate(BaseModel):
    email: EmailStr
    name: Annotated[str, Field(min_length=1, max_length=100)]
    age: Annotated[int, Field(ge=13, le=120)]
    bio: str | None = Field(default=None, max_length=500)
    tags: list[str] = Field(default_factory=list)
```

`EmailStr` requires `pip install "pydantic[email]"` (already pulled in by `fastapi[standard]`). There's also `HttpUrl`, `IPvAnyAddress`, `UUID4`, and others.

### Validators

Two kinds:

```python
from pydantic import BaseModel, field_validator, model_validator


class SignUp(BaseModel):
    username: str
    password: str
    password_confirm: str

    @field_validator("username")
    @classmethod
    def username_lowercase(cls, v: str) -> str:
        v = v.strip().lower()
        if not v.isalnum():
            raise ValueError("username must be alphanumeric")
        return v

    @model_validator(mode="after")
    def passwords_match(self) -> "SignUp":
        if self.password != self.password_confirm:
            raise ValueError("passwords do not match")
        return self
```

`@field_validator` runs per-field. `@model_validator(mode="after")` runs after all fields are populated (`mode="before"` runs on the raw dict).

> **v1 → v2 migration:** `@validator` → `@field_validator`. `@root_validator` → `@model_validator`. The signature changed: v2 uses `@classmethod` and the value is the second arg.

### `model_config`

Per-model behavior is set with `model_config: ConfigDict` (not the v1 inner `class Config:`):

```python
from pydantic import BaseModel, ConfigDict


class Item(BaseModel):
    model_config = ConfigDict(
        str_strip_whitespace=True,    # auto strip on str fields
        frozen=True,                  # immutable after construction
        extra="forbid",               # reject unknown fields (defaults to "ignore")
        populate_by_name=True,        # allow aliases AND field names
        from_attributes=True,         # for ORM objects (was orm_mode in v1)
    )

    id: int
    name: str
```

The `from_attributes=True` config is essential when returning SQLAlchemy ORM objects through `response_model` — it tells Pydantic to read attributes off the object instead of expecting a dict.

### Aliases

```python
from pydantic import BaseModel, Field


class UserOut(BaseModel):
    user_id: int = Field(alias="userId")
    display_name: str = Field(alias="displayName")

    model_config = {"populate_by_name": True}


# Accepts either {"userId": 1, ...} or {"user_id": 1, ...}
# Serializes with alias by default if you call model_dump(by_alias=True)
```

Useful when bridging Python's `snake_case` to JavaScript's `camelCase`.

### Serialization control

```python
from pydantic import BaseModel, field_serializer
from datetime import datetime


class Event(BaseModel):
    id: int
    happened_at: datetime
    secret_token: str

    @field_serializer("happened_at")
    def serialize_dt(self, dt: datetime) -> str:
        return dt.isoformat()

    def model_dump_safe(self) -> dict:
        return self.model_dump(exclude={"secret_token"})
```

For one-off response shaping, `response_model_exclude` / `response_model_include` on the decorator also work.

### Discriminated (tagged) unions

A v2 strength — fast, correct union parsing:

```python
from typing import Literal, Union
from pydantic import BaseModel, Field


class CatEvent(BaseModel):
    kind: Literal["cat"] = "cat"
    purr_volume: int


class DogEvent(BaseModel):
    kind: Literal["dog"] = "dog"
    bark_volume: int


Event = Annotated[Union[CatEvent, DogEvent], Field(discriminator="kind")]


class Payload(BaseModel):
    event: Event
```

Pydantic picks the right variant in O(1) based on the `kind` tag — no try-each-variant fallback.

### Performance notes

- v2 is faster — but you can lose it by doing dumb things. The biggest wins:
  - Reuse model *classes* (compiled validators are cached).
  - Avoid building models inside hot loops where a plain `dict` would do.
  - For huge payloads, `model_validate_json(bytes)` is faster than `model_validate(json.loads(bytes))` — it skips one full parse.
- `model_construct(...)` skips validation entirely. Useful when you've already verified data (e.g., loaded from your own DB) and want to avoid the cost. Dangerous as user-input shortcut.

---

## 4. Practical Application — A typed `/users` endpoint with validation

```python
# app/schemas/users.py
from datetime import datetime
from pydantic import BaseModel, ConfigDict, EmailStr, Field, field_validator


class UserBase(BaseModel):
    email: EmailStr
    full_name: str = Field(min_length=1, max_length=120)


class UserCreate(UserBase):
    password: str = Field(min_length=8, max_length=72)

    @field_validator("password")
    @classmethod
    def password_strong_enough(cls, v: str) -> str:
        if v.lower() == v or v.upper() == v:
            raise ValueError("password must contain mixed case")
        if not any(c.isdigit() for c in v):
            raise ValueError("password must contain a digit")
        return v


class UserOut(UserBase):
    model_config = ConfigDict(from_attributes=True)
    id: int
    created_at: datetime
```

```python
# app/api/v1/users.py
from fastapi import APIRouter, HTTPException, status
from datetime import datetime, UTC

from app.schemas.users import UserCreate, UserOut

router = APIRouter(prefix="/users", tags=["users"])
_DB: dict[int, UserOut] = {}
_NEXT_ID = 1


@router.post("", response_model=UserOut, status_code=status.HTTP_201_CREATED)
def create_user(payload: UserCreate) -> UserOut:
    global _NEXT_ID
    if any(u.email == payload.email for u in _DB.values()):
        raise HTTPException(409, "email already registered")
    user = UserOut(
        id=_NEXT_ID,
        email=payload.email,
        full_name=payload.full_name,
        created_at=datetime.now(UTC),
    )
    _DB[_NEXT_ID] = user
    _NEXT_ID += 1
    return user
```

**Test it**

```bash
# happy path
http POST :8000/api/v1/users \
    email=ada@example.com full_name="Ada Lovelace" password=Sup3rPass
# 201 with UserOut JSON

# weak password
http POST :8000/api/v1/users \
    email=x@y.com full_name=Y password=weak
# 422 — "password must contain mixed case"

# invalid email
http POST :8000/api/v1/users \
    email=not-an-email full_name=Y password=Sup3rPass
# 422 — "value is not a valid email address"
```

Notice we never wrote a single `if not email_regex.match(...)` line. The schema is the spec.

---

## 5. Common Mistakes & Gotchas

- **v1 → v2 surface area, in approximate order of pain:**
  - `Config` class → `model_config = ConfigDict(...)`
  - `orm_mode = True` → `from_attributes = True`
  - `.dict()` → `.model_dump()`. `.json()` → `.model_dump_json()`. `.parse_obj()` → `.model_validate()`.
  - `@validator` → `@field_validator` (now needs `@classmethod`).
  - `@root_validator` → `@model_validator(mode="before"|"after")`.
  - `Field(regex=...)` → `Field(pattern=...)`.
  - `Optional[X] = None` is **no longer implicit** if you only annotate `X = None`. Either `X | None = None` or import `Optional`.
- **Strict mode confusion.** Default v2 is *lax* (coerces). If you need strict, use `model_config = ConfigDict(strict=True)` or per-field `Field(strict=True)`. Strict means `"1"` won't become `1`.
- **Returning a plain dict where a `response_model` is declared.** Works, but you lose attribute access in middleware/services. Build the model.
- **`from_attributes` forgotten** when returning a SQLAlchemy row → `ValidationError: Input should be a valid dictionary`. Set it on every output schema you use with the ORM.
- **Recursive models** need `model_rebuild()` when forward refs are involved across modules. v2 mostly handles it, but watch for `PydanticUndefinedAnnotation`.
- **Heavy validators that hit a DB** — `@field_validator` runs at parse time, before your endpoint. Don't put DB queries in there; put them in the endpoint or service.
- **`extra="forbid"` on input models is a security win** — clients can't smuggle fields. Worth defaulting on if your API is internal.

---

## 🎯 Key Takeaways

- **Pydantic models are the runtime contract.** A `BaseModel` instance is a *validated* thing — treat it as a stronger type than the raw dict.
- **v2's speed comes from `pydantic-core` in Rust.** You get it for free unless you defeat it with `model_construct` or per-instance class creation.
- **Validators are for shape & invariants, not business rules.** Don't query a DB in `@field_validator`. Do enforce "passwords must be mixed case."
- **The v1→v2 migration list is long but mechanical.** Run `bump-pydantic` once, then read the diff carefully — especially `Config` → `model_config` and `regex` → `pattern`.
- **`from_attributes=True` is the single most-forgotten setting** in FastAPI + ORM apps. Bake it into every response schema base class.

*← [prev](./02_path_operations_and_routing.md) | [next →](./04_request_bodies.md)*
