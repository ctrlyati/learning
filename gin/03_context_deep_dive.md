# 03 — `gin.Context` Deep Dive

> **Goal:** Understand what `*gin.Context` actually is, how it is pooled, what every field/method does, and the rules for safely using it across goroutines and middleware.

---

## 1. What `gin.Context` is — mental model + working code

`*gin.Context` is **the one parameter every Gin handler and middleware receives**. Conceptually it carries:

- The incoming `*http.Request`
- The outgoing `http.ResponseWriter` (wrapped, so Gin can record status and bytes written)
- The list of handlers for this request (`handlers []HandlerFunc`) and a current index
- A `map[string]any` of request-scoped values (`Keys`)
- A slice of errors collected with `c.Error(...)`
- A reference back to `*gin.Engine`

```go
func handler(c *gin.Context) {
    // request side
    method := c.Request.Method
    path := c.Request.URL.Path
    auth := c.GetHeader("Authorization")
    id := c.Param("id")
    q := c.Query("q")

    // request-scoped values
    c.Set("trace_id", "abc-123")
    v, ok := c.Get("trace_id")           // v: any, ok: bool

    // response side
    c.Header("X-Custom", "yes")
    c.Status(202)
    c.JSON(202, gin.H{"id": id, "trace": v, "method": method, "path": path, "auth": auth, "q": q})
}
```

You will use these eight or nine methods 90% of the time:

| Method | Returns | Use |
|--------|---------|-----|
| `c.Param(key)` | string | Path param value |
| `c.Query(key)` | string | Query param value |
| `c.GetHeader(key)` | string | Request header |
| `c.ShouldBindJSON(&v)` | error | Decode JSON body into struct |
| `c.Set(key, val)` | — | Stash value for downstream handlers |
| `c.Get(key)` | (any, bool) | Read stashed value |
| `c.Header(k, v)` | — | Set response header |
| `c.JSON(status, obj)` | — | Write JSON response |
| `c.AbortWithStatusJSON(s, obj)` | — | Stop chain + write JSON |

---

## 2. How Gin implements `Context` — pooling and lifecycle

### The pool

`gin.Engine` holds a `sync.Pool` of `*Context`. On every request:

```go
// Simplified
func (engine *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
    c := engine.pool.Get().(*Context)
    c.writermem.reset(w)
    c.Request = req
    c.reset()

    engine.handleHTTPRequest(c)

    engine.pool.Put(c)
}
```

`c.reset()` zeros the params, handler index, errors, and `Keys` map. The `*Context` value itself is reused. **This is why you must not retain a reference to `c` past the handler.** When the response is written and `ServeHTTP` returns, the same pointer might be servicing a different request a microsecond later.

### Why pooling?

Allocating a fresh `Context` per request was a measurable overhead in Go's GC at Gin's scale. Pooling drops allocations to near-zero on the hot path. The cost is the discipline above.

### The handler chain index

`c.index` starts at `-1`. The dispatcher calls `c.Next()`, which increments the index and invokes `c.handlers[c.index]`. Inside a middleware, calling `c.Next()` again drains the rest of the chain before returning. Calling `c.Abort()` sets `c.index = abortIndex` (a large sentinel), so subsequent `Next` calls do nothing.

```go
// Approximate Next implementation:
func (c *Context) Next() {
    c.index++
    for c.index < int8(len(c.handlers)) {
        c.handlers[c.index](c)
        c.index++
    }
}
```

This is the entire middleware engine. There is no goroutine, no channel, no event loop — just a slice and an index.

---

## 3. Variations and depth — every Context surface you should know

### Request side

```go
c.Request           // *http.Request — the unmodified stdlib request
c.Param("id")       // path param
c.Query("q")        // query param
c.DefaultQuery("page", "1")
c.QueryArray("tag")
c.QueryMap("filter")
c.GetHeader("Authorization")
c.ContentType()     // "application/json" etc.
c.ClientIP()        // respects ForwardedByClientIP + TrustedProxies (configure!)
c.RemoteIP()        // raw RemoteAddr
c.GetRawData()      // []byte of the body — consumes it; can't bind after
c.Cookie("session") // (string, error)
```

**`ClientIP` is the one to be careful about.** In a real deployment you are usually behind a load balancer, so `RemoteAddr` is the LB's IP. Gin can read `X-Forwarded-For` / `X-Real-IP`, but you must tell it which upstream proxies to trust:

```go
r.SetTrustedProxies([]string{"10.0.0.0/8"}) // or nil to trust all (dangerous)
```

Without this, an attacker can spoof `X-Forwarded-For` and forge their IP in logs.

### Response side

```go
c.Status(202)
c.Header("X-Foo", "bar")
c.SetCookie(name, value, maxAge, path, domain, secure, httpOnly)
c.String(200, "hello %s", name)
c.JSON(200, obj)
c.IndentedJSON(200, obj)
c.SecureJSON(200, slice)        // prepends "while(1);" — XSS protection for legacy clients
c.AsciiJSON(200, obj)           // escapes non-ASCII chars
c.PureJSON(200, obj)            // doesn't escape HTML chars
c.XML(200, obj)
c.YAML(200, obj)
c.ProtoBuf(200, msg)
c.Data(200, "application/pdf", b)
c.File("/path/to/file.pdf")
c.FileAttachment("/path/to/file.pdf", "report.pdf")
c.Redirect(http.StatusFound, "/new")
```

Module 05 covers response forms in depth.

### Request-scoped storage — `Set` / `Get`

```go
// In auth middleware:
c.Set("user_id", 42)
c.Set("user", &User{ID: 42, Name: "yati"})

// In a downstream handler:
uid := c.GetInt("user_id")             // typed helper, returns zero if missing
if u, ok := c.Get("user"); ok {
    user := u.(*User)
    _ = user
}
```

Typed helpers: `GetString`, `GetInt`, `GetInt64`, `GetBool`, `GetFloat64`, `GetDuration`, `GetTime`, `GetStringSlice`, `GetStringMap`, `GetStringMapString`. All return the zero value if the key is missing — they do not panic.

**Do not use `c.Set` as a substitute for function parameters.** If a function needs the user ID, take it as a parameter. `c.Set` is for cross-cutting concerns set by middleware (request ID, authenticated user, span context).

### `context.Context` and request-scoped deadlines

`c.Request.Context()` returns the stdlib `context.Context` carrying any cancellation/deadline propagated by the HTTP server. Pass it to anything that hits the network or DB:

```go
ctx := c.Request.Context()
rows, err := db.QueryContext(ctx, "SELECT ...")
```

You can also derive your own deadline:

```go
ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
defer cancel()
```

**Never pass `c` itself to anything that expects `context.Context`.** Yes, `*gin.Context` happens to implement the `context.Context` interface (`Deadline`, `Done`, `Err`, `Value`) — but its `Done()` is **not** wired to the request's cancellation in older Gin versions, and crucially, holding a reference outside the handler is unsafe due to pooling. Always use `c.Request.Context()`.

### `Abort` family

```go
c.Abort()                                    // stop the chain after the current handler
c.AbortWithStatus(401)
c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
c.AbortWithError(500, err)                   // also calls c.Error(err)
```

`Abort` doesn't return from your function. You still need `return`:

```go
if token == "" {
    c.AbortWithStatusJSON(401, gin.H{"error": "missing token"})
    return                                    // <-- still required
}
```

Forgetting this `return` is one of the most common Gin bugs. The current handler keeps running; only subsequent handlers are skipped.

---

## 4. Practical application — auth middleware + handler using the full Context surface

A realistic example: middleware verifies a bearer token, stashes the user, sets a request ID, and the handler uses both.

```go
// internal/http/middleware/auth.go
package middleware

import (
    "crypto/rand"
    "encoding/hex"
    "net/http"
    "strings"

    "github.com/gin-gonic/gin"
)

type AuthUser struct {
    ID   int64
    Name string
    Role string
}

func RequestID() gin.HandlerFunc {
    return func(c *gin.Context) {
        rid := c.GetHeader("X-Request-ID")
        if rid == "" {
            b := make([]byte, 8)
            _, _ = rand.Read(b)
            rid = hex.EncodeToString(b)
        }
        c.Set("request_id", rid)
        c.Header("X-Request-ID", rid)
        c.Next()
    }
}

func Auth(verify func(token string) (*AuthUser, error)) gin.HandlerFunc {
    return func(c *gin.Context) {
        h := c.GetHeader("Authorization")
        if !strings.HasPrefix(h, "Bearer ") {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error":     "missing or malformed Authorization header",
                "requestID": c.GetString("request_id"),
            })
            return
        }
        token := strings.TrimPrefix(h, "Bearer ")
        u, err := verify(token)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error":     "invalid token",
                "requestID": c.GetString("request_id"),
            })
            return
        }
        c.Set("user", u)
        c.Next()
    }
}

// Helper for handlers
func CurrentUser(c *gin.Context) *AuthUser {
    v, ok := c.Get("user")
    if !ok {
        return nil
    }
    u, _ := v.(*AuthUser)
    return u
}
```

```go
// cmd/api/main.go
package main

import (
    "errors"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/you/hello-gin/internal/http/middleware"
)

func main() {
    r := gin.Default()
    r.Use(middleware.RequestID())

    verify := func(token string) (*middleware.AuthUser, error) {
        if token == "dev-token" {
            return &middleware.AuthUser{ID: 1, Name: "yati", Role: "admin"}, nil
        }
        return nil, errors.New("nope")
    }

    api := r.Group("/api", middleware.Auth(verify))
    api.GET("/me", me)

    r.Run(":8080")
}

func me(c *gin.Context) {
    u := middleware.CurrentUser(c)
    c.JSON(http.StatusOK, gin.H{
        "user":      u,
        "requestID": c.GetString("request_id"),
    })
}
```

Test:

```bash
curl -i http://localhost:8080/api/me
# 401, requestID present in body + header

curl -i -H "Authorization: Bearer dev-token" http://localhost:8080/api/me
# 200, user + requestID
```

Watch the response headers — `X-Request-ID` is set by middleware, reused or generated.

---

## 5. Common mistakes & gotchas

- **Holding `c` past the handler.** The pool reclaims it. Common forms: stashing `c` in a struct field, sending `c` over a channel, capturing `c` in a goroutine closure. Always copy: `cc := c.Copy()` (returns a safe, read-only-ish copy with only `Request` and `Keys` snapshotted). Module 12 deep-dives this.
- **Forgetting `return` after `c.AbortWithStatusJSON`.** Abort sets a flag — it does not exit your function. The current handler keeps executing. Reviewers should flag any abort not followed by `return` on the very next line.
- **Using `c.Set` as parameter-passing for ordinary handler code.** It's untyped, unchecked at compile time, and turns refactors into a graph search. Reserve it for cross-cutting state set by middleware.
- **Type-asserting from `c.Get` without `ok`.** If the value is missing, `c.Get` returns `(nil, false)`. A naked `.(*User)` will panic. Use the typed `GetString`/`GetInt` helpers or always check `ok`.
- **Reading the body twice.** `c.ShouldBindJSON` consumes `c.Request.Body`. A second call returns `EOF`. If you must bind twice (e.g., to a generic envelope, then to a typed payload), buffer first:
  ```go
  body, _ := io.ReadAll(c.Request.Body)
  c.Request.Body = io.NopCloser(bytes.NewReader(body))   // restore
  ```
- **Calling `c.JSON` after `c.AbortWithStatusJSON`.** The first one writes the response; the second produces a "headers already written" warning in logs and the client sees the first body. Whichever comes first wins; pick one.
- **Trusting `c.ClientIP()` without `SetTrustedProxies`.** Without it, an attacker can spoof `X-Forwarded-For`. Lock it down to your LB CIDR.
- **Treating `c` as a `context.Context` and passing it to goroutines or DB calls.** Use `c.Request.Context()`. Always. This is the rule that prevents the most production bugs in Gin services.

---

## 🎯 Key Takeaways

1. **`*gin.Context` is pooled and recycled.** It is safe inside a handler, unsafe outside. Treat it like a borrowed object — `c.Copy()` is your friend for goroutines (module 12).
2. **`c.Request.Context()` is the real `context.Context`** for cancellation, deadlines, and DB/HTTP calls. `c` itself is for handler-local concerns.
3. **`Abort` does not `return`.** Every `c.AbortWithStatusJSON(...)` should be followed by `return` on the next line. Make this a code-review reflex.
4. **`c.Set`/`c.Get` is for cross-cutting middleware-injected values** (request ID, user, span). Not a substitute for explicit function parameters; not a runtime grab-bag.
5. **Always configure `SetTrustedProxies`** in production. Otherwise `ClientIP` is a security liability — attackers can forge audit-log IPs trivially.

*← [02 — Routing](./02_routing.md) | [04 — Request Handling →](./04_request_handling.md)*
