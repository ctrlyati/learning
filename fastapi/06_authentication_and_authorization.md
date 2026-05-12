# 06 — Authentication & Authorization

> **Goal:** Implement production-ready auth in FastAPI — OAuth2 password flow with JWT, API keys, OIDC integration, and RBAC patterns — knowing exactly what each piece does and where it sits in the stack.

---

## 1. Concept — Auth is a dependency that either returns a user or raises 401/403

In FastAPI, authentication is just a `Depends(get_current_user)`. There's no special `@login_required` decorator — there's the DI system, and you write dependencies that:

- Look at the request (header, cookie, query).
- Verify credentials.
- Return a user object on success.
- Raise `HTTPException(401)` on failure.

Authorization is the same pattern: a dependency that takes the current user, checks roles/scopes, and raises `HTTPException(403)` on failure.

```python
from fastapi import FastAPI, Depends, HTTPException
from fastapi.security import OAuth2PasswordBearer
from typing import Annotated

app = FastAPI()
oauth2 = OAuth2PasswordBearer(tokenUrl="/auth/token")


async def get_current_user(token: Annotated[str, Depends(oauth2)]) -> dict:
    if token != "valid-token":
        raise HTTPException(401, "invalid token")
    return {"id": 1, "email": "ada@example.com"}


@app.get("/me")
async def me(user: Annotated[dict, Depends(get_current_user)]) -> dict:
    return user
```

That's the whole shape. Everything below is filling in the credential-verification middle.

---

## 2. Mechanism — `fastapi.security` gives you OpenAPI-aware schemes

FastAPI ships standard auth schemes that double as OpenAPI security definitions:

- `OAuth2PasswordBearer` — token comes from `Authorization: Bearer <token>` header.
- `OAuth2AuthorizationCodeBearer` — for OIDC / third-party providers.
- `HTTPBasic` — Basic auth (rarely used in 2026 except for internal tools).
- `APIKeyHeader`, `APIKeyQuery`, `APIKeyCookie` — for service-to-service API keys.

Each is a callable dependency. When used, it shows up correctly in Swagger UI as an "Authorize" button so you (and your users) can test protected endpoints from the browser.

The key insight: **FastAPI doesn't authenticate; it extracts credentials.** Verification is your code. That's good — auth is the one place you do not want a magic black box.

---

## 3. Variations & Depth

### OAuth2 password flow + JWT (the canonical pattern)

This is the standard. User POSTs username/password to `/auth/token`, gets a JWT, sends it as `Authorization: Bearer <jwt>` on subsequent requests.

```python
# app/core/security.py
from datetime import datetime, timedelta, UTC
import jwt  # PyJWT
from passlib.context import CryptContext

SECRET = "change-me-via-env"  # use pydantic-settings (module 15)
ALGO = "HS256"
ACCESS_TTL_MIN = 30

pwd = CryptContext(schemes=["bcrypt"], deprecated="auto")


def hash_password(plain: str) -> str:
    return pwd.hash(plain)


def verify_password(plain: str, hashed: str) -> bool:
    return pwd.verify(plain, hashed)


def create_access_token(sub: str, scopes: list[str] | None = None) -> str:
    now = datetime.now(UTC)
    payload = {
        "sub": sub,
        "iat": now,
        "exp": now + timedelta(minutes=ACCESS_TTL_MIN),
        "scopes": scopes or [],
    }
    return jwt.encode(payload, SECRET, algorithm=ALGO)


def decode_access_token(token: str) -> dict:
    return jwt.decode(token, SECRET, algorithms=[ALGO])
```

```python
# app/api/auth.py
from fastapi import APIRouter, Depends, HTTPException, status
from fastapi.security import OAuth2PasswordRequestForm
from typing import Annotated
from pydantic import BaseModel

from app.core.security import create_access_token, verify_password
# from app.services.users import get_user_by_email

router = APIRouter(prefix="/auth", tags=["auth"])

# Replace with real DB lookup
_FAKE_USERS = {
    "ada@example.com": {"id": 1, "hashed": "$2b$12$..."},  # bcrypt of "secret"
}


class TokenOut(BaseModel):
    access_token: str
    token_type: str = "bearer"


@router.post("/token", response_model=TokenOut)
async def login(
    form: Annotated[OAuth2PasswordRequestForm, Depends()],
) -> TokenOut:
    user = _FAKE_USERS.get(form.username)
    if not user or not verify_password(form.password, user["hashed"]):
        raise HTTPException(
            status.HTTP_401_UNAUTHORIZED,
            "incorrect username or password",
            headers={"WWW-Authenticate": "Bearer"},
        )
    return TokenOut(access_token=create_access_token(sub=form.username))
```

```python
# app/api/deps.py
from fastapi import Depends, HTTPException, status
from fastapi.security import OAuth2PasswordBearer
from typing import Annotated
import jwt

from app.core.security import decode_access_token

oauth2_scheme = OAuth2PasswordBearer(tokenUrl="/auth/token")


async def get_current_user(
    token: Annotated[str, Depends(oauth2_scheme)],
) -> dict:
    creds_exc = HTTPException(
        status.HTTP_401_UNAUTHORIZED,
        "could not validate credentials",
        headers={"WWW-Authenticate": "Bearer"},
    )
    try:
        payload = decode_access_token(token)
    except jwt.PyJWTError:
        raise creds_exc
    sub = payload.get("sub")
    if not sub:
        raise creds_exc
    # In real life: load user from DB by sub
    return {"email": sub, "scopes": payload.get("scopes", [])}


CurrentUser = Annotated[dict, Depends(get_current_user)]
```

**Test it**

```bash
# Get a token
curl -X POST http://127.0.0.1:8000/auth/token \
     -d "username=ada@example.com&password=secret"
# {"access_token":"eyJ...","token_type":"bearer"}

# Use it
curl http://127.0.0.1:8000/me \
     -H "Authorization: Bearer eyJ..."
# {"email":"ada@example.com",...}
```

### API keys

For service-to-service or admin tools:

```python
from fastapi.security import APIKeyHeader

api_key_scheme = APIKeyHeader(name="X-API-Key", auto_error=False)


async def get_api_caller(
    key: Annotated[str | None, Depends(api_key_scheme)],
) -> dict:
    if key not in {"key-svc-a", "key-svc-b"}:  # in real life: DB / hash compare
        raise HTTPException(401, "invalid api key")
    return {"caller": "svc-a" if key == "key-svc-a" else "svc-b"}
```

Compare keys with `secrets.compare_digest()` to avoid timing attacks — not `==`.

### OIDC / third-party login

Use a library like `authlib` or `fastapi-users`. The flow:

1. Redirect user to `https://accounts.google.com/...?client_id=...`.
2. Google redirects back to your `/auth/callback?code=...`.
3. Your server exchanges the code for an ID token + access token.
4. You decode the ID token (JWT signed by Google), get the user identity, mint your own session token.

Keep your auth-server logic separate from your API logic — typically a dedicated `app/auth/` module or even a separate microservice.

### RBAC patterns

Three common approaches, increasing in flexibility:

**A. Simple role check**

```python
def require_role(role: str):
    def checker(user: CurrentUser) -> dict:
        if role not in user.get("roles", []):
            raise HTTPException(403, "forbidden")
        return user
    return checker


@app.get("/admin/users", dependencies=[Depends(require_role("admin"))])
def list_users() -> list:
    return []
```

**B. Scopes (OAuth2 standard)**

`OAuth2PasswordBearer` supports a `scopes` dict. Endpoints declare required scopes:

```python
from fastapi.security import SecurityScopes

oauth2_scheme = OAuth2PasswordBearer(
    tokenUrl="/auth/token",
    scopes={"me": "Read own info", "items:write": "Create items"},
)


async def get_current_user(
    security_scopes: SecurityScopes,
    token: Annotated[str, Depends(oauth2_scheme)],
) -> dict:
    user = decode_and_load(token)  # your code
    for required in security_scopes.scopes:
        if required not in user["scopes"]:
            raise HTTPException(
                403,
                f"missing scope: {required}",
                headers={"WWW-Authenticate": f'Bearer scope="{required}"'},
            )
    return user


from fastapi import Security

@app.post("/items", dependencies=[Security(get_current_user, scopes=["items:write"])])
def create_item() -> dict:
    return {"ok": True}
```

`Security(...)` is `Depends` with scope metadata — that's the only difference.

**C. Policy / casbin / OPA**

For complex orgs with hundreds of permissions, push policy decisions to a library like [casbin](https://casbin.org/) or an external service like [OPA](https://www.openpolicyagent.org/). The endpoint dep becomes:

```python
async def check_policy(
    request: Request,
    user: CurrentUser,
    action: str = "read",
    resource: str = "item",
) -> None:
    if not await policy_engine.allows(user, action, resource):
        raise HTTPException(403)
```

Don't reach for this on day one. Roles + scopes cover 90% of apps.

---

## 4. Practical Application — JWT-protected `/notes` with owner check

A user can CRUD their own notes; admins can see all.

**`app/schemas/notes.py`**

```python
from pydantic import BaseModel


class NoteIn(BaseModel):
    title: str
    body: str


class NoteOut(NoteIn):
    id: int
    owner_id: int
```

**`app/api/v1/notes.py`**

```python
from fastapi import APIRouter, HTTPException, status

from app.api.deps import CurrentUser
from app.schemas.notes import NoteIn, NoteOut

router = APIRouter(prefix="/notes", tags=["notes"])

_DB: dict[int, NoteOut] = {}
_NEXT_ID = 1


def _own_or_admin(note: NoteOut, user: dict) -> None:
    if note.owner_id != user["id"] and "admin" not in user.get("roles", []):
        raise HTTPException(status.HTTP_403_FORBIDDEN, "not your note")


@router.post("", response_model=NoteOut, status_code=status.HTTP_201_CREATED)
async def create_note(payload: NoteIn, user: CurrentUser) -> NoteOut:
    global _NEXT_ID
    note = NoteOut(id=_NEXT_ID, owner_id=user["id"], **payload.model_dump())
    _DB[_NEXT_ID] = note
    _NEXT_ID += 1
    return note


@router.get("/{note_id}", response_model=NoteOut)
async def get_note(note_id: int, user: CurrentUser) -> NoteOut:
    note = _DB.get(note_id)
    if note is None:
        raise HTTPException(404, "not found")
    _own_or_admin(note, user)
    return note


@router.delete("/{note_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_note(note_id: int, user: CurrentUser) -> None:
    note = _DB.get(note_id)
    if note is None:
        raise HTTPException(404, "not found")
    _own_or_admin(note, user)
    del _DB[note_id]
```

**Auth-aware test**

```python
import jwt
from datetime import datetime, timedelta, UTC
from fastapi.testclient import TestClient
from app.core.security import SECRET, ALGO
from app.main import app


def token_for(user_id: int, roles: list[str] | None = None) -> str:
    payload = {
        "sub": str(user_id),
        "iat": datetime.now(UTC),
        "exp": datetime.now(UTC) + timedelta(minutes=5),
        "roles": roles or [],
    }
    return jwt.encode(payload, SECRET, algorithm=ALGO)


def test_user_cannot_read_others_note() -> None:
    client = TestClient(app)
    # User 1 creates
    r = client.post(
        "/api/v1/notes",
        json={"title": "x", "body": "y"},
        headers={"Authorization": f"Bearer {token_for(1)}"},
    )
    nid = r.json()["id"]

    # User 2 tries to read
    r = client.get(
        f"/api/v1/notes/{nid}",
        headers={"Authorization": f"Bearer {token_for(2)}"},
    )
    assert r.status_code == 403
```

---

## 5. Common Mistakes & Gotchas

- **Storing JWT secrets in code.** Use `pydantic-settings` (module 15) and load from env. Rotate the secret with care — old tokens become invalid.
- **No token expiry.** A JWT without `exp` is a permanent backdoor. Always set `exp`. Short TTLs (15–60 min) + refresh tokens is the safe pattern.
- **Putting sensitive data in the JWT payload.** JWTs are signed, not encrypted. Anyone can decode the body. Keep it to `sub`, `roles`, `exp`. Real data lives in your DB.
- **Using `algorithm="none"`** — vulnerability source. Pin to `HS256` or `RS256`. PyJWT defaults to safe behavior but never let an `alg` from an attacker-controlled token decide.
- **bcrypt with input > 72 bytes** silently truncates — don't allow passwords longer than 72 chars or use argon2 instead.
- **Comparing API keys with `==`** — vulnerable to timing attacks. Use `secrets.compare_digest()`.
- **CORS + cookies + CSRF.** If you use cookie-based sessions for browser apps, CORS alone is not enough — you need CSRF tokens. JWTs in `Authorization` headers avoid this entirely (which is one reason they're popular).
- **Refresh tokens stored in localStorage.** XSS pulls them. Either use httpOnly cookies (with CSRF protection) or rotate aggressively. The "best" pattern depends on threat model — pick one consciously.
- **Treating "401" and "403" the same.** 401 = "I don't know who you are." 403 = "I know who you are and you can't." Mixing these confuses clients and humans.

---

## 🎯 Key Takeaways

- **Auth in FastAPI is a dependency, not a decorator.** That's the entire design — fully composable, fully testable via overrides.
- **`OAuth2PasswordBearer` + JWT is the production default** for self-hosted user auth. Know it cold; everything else is a variation.
- **Scopes are the standard authorization vocabulary.** `Security(dep, scopes=[...])` plays well with OpenAPI and clients. Roles map to scopes; don't reinvent.
- **Encrypt at rest (passwords), sign for transport (tokens), keep secrets in env.** Three rules, no exceptions.
- **For OIDC / third-party, lean on `authlib` or `fastapi-users`.** Hand-rolling OIDC is a multi-week mistake.

*← [prev](./05_dependency_injection.md) | [next →](./07_database_integration.md)*
