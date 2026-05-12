# 14 — API Design with Gin

> **Goal:** Design REST APIs that scale — sensible resource modeling, versioning, pagination, rate limiting, idempotency, and OpenAPI generation with `swaggo` or `huma`.

---

## 1. REST shape — mental model + working code

A REST API names *resources* (nouns) and uses HTTP methods (verbs) to act on them. The standard mapping:

| Method | URL | Meaning | Idempotent? | Body |
|--------|-----|---------|-------------|------|
| GET    | `/orders`         | List orders | yes | no |
| GET    | `/orders/:id`     | Read one    | yes | no |
| POST   | `/orders`         | Create      | no  | yes |
| PUT    | `/orders/:id`     | Replace     | yes | yes (full resource) |
| PATCH  | `/orders/:id`     | Partial update | no | yes (changes only) |
| DELETE | `/orders/:id`     | Delete      | yes | no |

Sub-resources:

```
POST   /orders/:id/refunds       create a refund
GET    /orders/:id/items         list line items
PUT    /orders/:id/items/:lid    replace one line item
```

This is **not law** — it is convention that lowers the cognitive cost for API consumers. Stick to it unless you have a specific reason.

### Canonical envelope

Pick one response envelope and use it everywhere. Two common patterns:

**Flat** (resource at the top level):

```json
{ "id": 1, "email": "y@x.com", "name": "yati" }
```

**Wrapped** (resource under `data`, metadata as siblings):

```json
{
  "data": { "id": 1, "email": "y@x.com", "name": "yati" },
  "meta": { "request_id": "abc" }
}
```

Both are fine. Pick one *before* writing handlers, document it, enforce it. Mixing them across endpoints is the rookie tell.

### Error envelope

Already designed in module 07:

```json
{
  "error":      "NOT_FOUND",
  "message":    "user not found",
  "request_id": "abc123",
  "fields":     [ { "field": "email", "rule": "required" } ]
}
```

The `error` code is **stable across versions** — clients switch on it. The `message` is human-readable and can evolve.

---

## 2. Versioning — path, header, or both

Three approaches:

| Approach | Example | Trade-off |
|----------|---------|-----------|
| **Path** | `/api/v1/users` | Easy to route, easy to deploy, ugly for "RESTful purists" |
| **Header** | `Accept: application/vnd.example.v2+json` | Clean URLs, but invisible in browser tooling and harder to debug |
| **Subdomain** | `v1.api.example.com` | Works for DNS-level routing; rare in Go shops |

**Path versioning is the Go ecosystem default.** It plays nicely with routing groups, with API gateways, and with `curl` debugging. Use it.

```go
v1 := r.Group("/api/v1")
v1.GET("/users", listUsers)

v2 := r.Group("/api/v2")
v2.GET("/users", listUsersV2)   // new pagination semantics, say
```

Inside a major version, additive changes (new fields, new endpoints) are non-breaking and ship freely. Removing or renaming fields → new major version.

---

## 3. Pagination, filtering, sorting, idempotency, rate limiting

### Pagination

Three styles:

- **Offset/limit** — `?page=3&limit=20`. Easiest. Breaks on large offsets (DB scans deep into the table) and on inserts (rows shift).
- **Cursor (keyset)** — `?after=eyJpZCI6MTIz`. Stable under inserts, fast at depth, harder to implement.
- **Page tokens** — opaque server-issued tokens (`?page_token=xyz`). Hides implementation details from clients. Default for Google APIs.

For most APIs, **cursor pagination** is the right call by the time you're at ~10k rows:

```sql
SELECT id, email, name FROM users
WHERE id > $1            -- the cursor
ORDER BY id
LIMIT $2;
```

```json
{
  "data": [ {"id":124,...}, ... ],
  "next_cursor": "eyJpZCI6MTQzfQ=="
}
```

Encode the cursor as base64(JSON) so you can change its shape without changing clients.

### Filtering and sorting

Light filters via query params:

```
GET /orders?status=paid&customer_id=42
GET /orders?created_after=2026-01-01
GET /orders?sort=-created_at,id
```

A `-` prefix on sort means descending. Validate the sort-by field against an allow-list — never plug user input into `ORDER BY` directly (SQL injection on identifiers).

For complex querying, don't reinvent SQL — define a small DSL, or accept structured filters in a POST body (`POST /orders/search`).

### Idempotency

POSTs aren't naturally idempotent. If a client retries after a flaky network, you might duplicate the order. Industry pattern (Stripe-style): **`Idempotency-Key` header**.

```go
func Idempotency(store IdempotencyStore) gin.HandlerFunc {
    return func(c *gin.Context) {
        key := c.GetHeader("Idempotency-Key")
        if key == "" || c.Request.Method != "POST" {
            c.Next()
            return
        }
        if cached, ok := store.Lookup(c.Request.Context(), key); ok {
            c.Data(cached.Status, cached.ContentType, cached.Body)
            c.Abort()
            return
        }

        // Capture the response so we can cache it.
        recorder := &responseRecorder{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
        c.Writer = recorder
        c.Next()
        if recorder.Status() >= 200 && recorder.Status() < 300 {
            _ = store.Store(c.Request.Context(), key, recorder.Status(),
                recorder.Header().Get("Content-Type"), recorder.body.Bytes(), 24*time.Hour)
        }
    }
}
```

Cache responses by key; on a duplicate POST within the TTL, return the cached response. Module 12 covered the `c.Writer` interface; this is one of the places that surface earns its keep.

### Rate limiting

Per-IP, per-user, per-API-key. The token-bucket pattern from module 06 is the building block; in production add:

- **Redis-backed limiters** for multi-instance services. Local `golang.org/x/time/rate` is per-process; behind a load balancer each pod has its own limit.
- **Standard headers**: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, plus `Retry-After` on 429.
- **Different tiers** by API key or user role.

Libraries: `github.com/ulule/limiter/v3` or `github.com/sethvargo/go-limiter` (both Redis-aware).

### `OPTIONS` and CORS

For browser SPAs hitting a different origin, you need CORS. Use `gin-contrib/cors`:

```go
import "github.com/gin-contrib/cors"

r.Use(cors.New(cors.Config{
    AllowOrigins:     []string{"https://app.example.com"},
    AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
    AllowHeaders:     []string{"Authorization", "Content-Type", "Idempotency-Key"},
    ExposeHeaders:    []string{"X-Request-ID", "X-RateLimit-Remaining"},
    AllowCredentials: true,
    MaxAge:           12 * time.Hour,
}))
```

**Never `AllowAllOrigins: true` with `AllowCredentials: true`.** The browser rejects it, and even if it didn't, this is a credentials-leak misconfiguration.

---

## 4. OpenAPI — `swaggo` or `huma`

OpenAPI is the spec your consumers want. Two production paths in Go:

### Path A: `swaggo` (annotation-driven)

You annotate Gin handlers in comments; `swag init` produces an `openapi.json` and a Swagger UI route.

```bash
go install github.com/swaggo/swag/cmd/swag@latest
go get github.com/swaggo/gin-swagger
go get github.com/swaggo/files
```

```go
// @title           Hello Gin API
// @version         1.0
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

// GetUser godoc
// @Summary      Get user by ID
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "User ID"
// @Success      200  {object}  service.User
// @Failure      404  {object}  apperr.Error
// @Router       /users/{id} [get]
// @Security     BearerAuth
func GetUser(c *gin.Context) { /* ... */ }
```

```bash
swag init -g cmd/api/main.go
# generates docs/docs.go, docs/swagger.json, docs/swagger.yaml

# wire up:
import (
    swaggerfiles "github.com/swaggo/files"
    ginSwagger "github.com/swaggo/gin-swagger"
    _ "github.com/you/hello-gin/docs"
)
r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
```

Pros: works with existing Gin code, lots of teams already use it.
Cons: annotations drift from code, no compile-time check that the spec matches the handler.

### Path B: `huma` (code-first, type-safe)

`huma` is a newer framework that runs *on top of* Gin (or Chi, Fiber) and generates OpenAPI **from your Go types**. The handler signature is typed:

```bash
go get github.com/danielgtaylor/huma/v2
```

```go
import (
    "github.com/danielgtaylor/huma/v2"
    "github.com/danielgtaylor/huma/v2/adapters/humagin"
)

type GetUserInput struct {
    ID int64 `path:"id" doc:"User ID" example:"42"`
}
type GetUserOutput struct {
    Body service.User
}

func main() {
    r := gin.New()
    api := humagin.New(r, huma.DefaultConfig("Hello Gin", "1.0.0"))

    huma.Register(api, huma.Operation{
        OperationID: "get-user",
        Method:      http.MethodGet,
        Path:        "/api/v1/users/{id}",
        Summary:     "Get user by ID",
        Tags:        []string{"users"},
    }, func(ctx context.Context, in *GetUserInput) (*GetUserOutput, error) {
        u, err := userSvc.Get(ctx, in.ID)
        if err != nil { return nil, huma.Error404NotFound("user not found") }
        return &GetUserOutput{Body: *u}, nil
    })

    r.Run(":8080")
}
```

OpenAPI is **derived from the types**, served at `/openapi.yaml` automatically, and clients can be generated from it. Validation, content negotiation, default OpenAPI metadata — all free.

Pros: contract and code can't drift; OpenAPI is always accurate; fewer hand-written validation/binding lines.
Cons: handler signature changes; not pure Gin idiom.

**My recommendation:** new services that care about OpenAPI fidelity → `huma`. Existing Gin services or teams that want minimum disruption → `swaggo`.

---

## 5. Practical application — `GET /orders` with cursor pagination, filtering, sort, OpenAPI

```go
// internal/http/handlers/orders.go
package handlers

import (
    "encoding/base64"
    "encoding/json"
    "strconv"
    "strings"

    "github.com/gin-gonic/gin"
    "github.com/you/hello-gin/internal/apperr"
)

type listOrdersQuery struct {
    Limit  int    `form:"limit,default=20"  binding:"gte=1,lte=100"`
    Cursor string `form:"cursor"`
    Status string `form:"status"            binding:"omitempty,oneof=pending paid shipped delivered cancelled"`
    Sort   string `form:"sort,default=-id"`
}

type cursor struct {
    LastID int64 `json:"lid"`
}

var allowedSort = map[string]string{
    "id":          "id ASC",
    "-id":         "id DESC",
    "created_at":  "created_at ASC",
    "-created_at": "created_at DESC",
}

func (h *OrderHandler) List(c *gin.Context) error {
    var q listOrdersQuery
    if err := c.ShouldBindQuery(&q); err != nil {
        return apperr.BadRequest("invalid query", err)
    }
    sortClause, ok := allowedSort[strings.ToLower(q.Sort)]
    if !ok {
        return apperr.BadRequest("invalid sort field", nil)
    }

    var afterID int64
    if q.Cursor != "" {
        raw, err := base64.StdEncoding.DecodeString(q.Cursor)
        if err != nil {
            return apperr.BadRequest("invalid cursor", err)
        }
        var cur cursor
        if err := json.Unmarshal(raw, &cur); err != nil {
            return apperr.BadRequest("invalid cursor", err)
        }
        afterID = cur.LastID
    }

    orders, err := h.Orders.List(c.Request.Context(), service.OrderFilter{
        AfterID:    afterID,
        Limit:      q.Limit,
        Status:     q.Status,
        SortClause: sortClause,
    })
    if err != nil {
        return apperr.Internal(err)
    }

    resp := gin.H{"data": orders}
    if len(orders) == q.Limit {
        next, _ := json.Marshal(cursor{LastID: orders[len(orders)-1].ID})
        resp["next_cursor"] = base64.StdEncoding.EncodeToString(next)
    }
    c.JSON(200, resp)
    _ = strconv.Itoa(0) // (placeholder; remove)
    return nil
}
```

Test:

```bash
# First page
curl 'http://localhost:8080/api/v1/orders?limit=2&status=paid'
# {"data":[{"id":7,...},{"id":5,...}],"next_cursor":"eyJsaWQiOjV9"}

# Next page using the cursor
curl 'http://localhost:8080/api/v1/orders?limit=2&status=paid&cursor=eyJsaWQiOjV9'
```

Why this is the senior shape:

- **Bounded `limit`** prevents `?limit=1000000` DoS.
- **`oneof` validation on `status`** — clients can't probe internal states.
- **Sort field allow-list** — no injection via `ORDER BY`.
- **Opaque base64 cursor** — server can evolve cursor shape without breaking clients (versioned later via a `v` field if needed).
- **Cursor returned only when there might be more** — clients don't need to count.

---

## 5. Common mistakes & gotchas

- **Offset pagination at scale.** `OFFSET 100000` makes the DB scan 100k rows. Move to cursor pagination as soon as a list endpoint serves real volume.
- **Sort fields from user input plugged straight into `ORDER BY`.** Identifiers aren't parameterizable — this is SQL injection. Validate against an allow-list.
- **PATCH that requires every field.** Defeats the purpose. PATCH should accept partial bodies; use pointer fields (`*string`) or a `*Updates` struct that says "if nil, don't change."
- **No `Idempotency-Key` on payment-like endpoints.** A double-charged customer is forever. Add idempotency keys to any operation with side effects you can't trivially reverse.
- **Returning the same `id` for different resources across endpoints.** Use prefixed IDs (`usr_abc`, `ord_xyz`) so a leaked ID is identifiable in support and logs. Stripe-style.
- **Inconsistent envelope.** One endpoint returns the resource at top level, another wraps in `{data: ...}`. Pick one in module 14 and live with it.
- **`AllowAllOrigins: true` + `AllowCredentials: true`.** Browser blocks it; even if it didn't, it's a credentials leak. List explicit origins.
- **Annotating with `swaggo` and never running `swag init` in CI.** The generated `docs/` drifts. Pin the spec generation to CI; fail the build if the diff is non-empty.
- **Versioning every minor change.** v1, v2, v3 within a month tells consumers your API is unstable. Add fields freely (non-breaking); only bump majors for removals/renames/semantic changes.
- **Returning Go zero-values for "not present."** If `OrderHandler.List` returns `[]Order(nil)`, the JSON is `null`, not `[]`. Initialize: `orders := make([]Order, 0)`. Clients should not have to special-case `null`-vs-`[]`.

---

## 🎯 Key Takeaways

1. **Pick one envelope, one versioning strategy, one error format — before writing handlers.** Consistency is the single biggest signal of a thought-through API. Path versioning (`/api/v1`) is the Go default; don't get cute.
2. **Cursor pagination scales; offset pagination doesn't.** Adopt cursors early — opaque, base64-encoded, with a `next_cursor` only when more rows exist.
3. **`Idempotency-Key` for any state-changing endpoint with side effects.** Stripe popularized it for a reason: every payment, every email, every order. Network retries are real.
4. **OpenAPI is non-optional for any public/internal API.** `swaggo` for retrofitting existing Gin code; `huma` for new code where you want types ↔ spec to be the same artifact. Either way, CI must regenerate or validate the spec on every PR.
5. **Validate sort fields against an allow-list, bound `limit`, validate enums with `oneof`.** Three small lines that close most "I sent a weird query and got a 500" reports — and the kind of detail interviewers ask about when they're trying to tell senior from mid-level.

*← [13 — Observability](./13_observability.md) | [15 — Production →](./15_production.md)*
