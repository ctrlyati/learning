# 06 — Middleware

> **Goal:** Internalize middleware as "a function in a chain," write production-grade middleware, understand `Next` vs `Abort`, and pick the right scope (global, group, per-route) every time.

---

## 1. Middleware — mental model + working code

A Gin middleware is **a `gin.HandlerFunc`** — same type as a handler:

```go
type HandlerFunc func(*Context)
```

There is no separate "Middleware" type. Anything you can do in a middleware, you can do in a handler, and vice versa. The only difference is convention: middleware tends to do something *before* and/or *after* `c.Next()`, while a handler is usually the leaf that writes the response.

### A minimal timing middleware

```go
package middleware

import (
    "log"
    "time"

    "github.com/gin-gonic/gin"
)

func Timing() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        c.Next()                                       // run the rest of the chain
        log.Printf("%s %s -> %d in %v",
            c.Request.Method, c.Request.URL.Path,
            c.Writer.Status(), time.Since(start))
    }
}
```

Wire it:

```go
r := gin.New()
r.Use(middleware.Timing(), gin.Recovery())
r.GET("/ping", func(c *gin.Context) { c.String(200, "pong") })
```

```bash
curl http://localhost:8080/ping
# server logs: GET /ping -> 200 in 91µs
```

The middleware pattern: pre-work → `c.Next()` → post-work. Either side can be empty.

---

## 2. How Gin runs the chain — `Next`, `Abort`, `index`

### The slice + index machine

For every route, Gin merges global, group, and per-route middleware into one `[]HandlerFunc`. The leaf handler is the last element. On request:

```go
// Inside ServeHTTP, simplified:
c.handlers = mergedChain
c.index = -1
c.Next()
```

`Next` walks the slice:

```go
func (c *Context) Next() {
    c.index++
    for c.index < int8(len(c.handlers)) {
        c.handlers[c.index](c)
        c.index++
    }
}
```

When a middleware calls `c.Next()` from the middle of its body, it recursively drains the rest of the chain before control returns. That's how "post-work" works.

### `Abort` is a flag flip

```go
func (c *Context) Abort() {
    c.index = abortIndex                              // 63, a sentinel
}
```

Set the index past anything reachable. Subsequent loop iterations exit. **Abort does not return from the current function.** You still write `return` on the next line:

```go
func RequireAuth(c *gin.Context) {
    if !ok {
        c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
        return                                        // <-- mandatory
    }
    c.Next()
}
```

`c.IsAborted()` lets a later piece check whether to short-circuit; mostly used inside Recovery.

### Implicit `Next`

If a middleware doesn't call `c.Next()` explicitly, Gin calls it for you when the function returns — by virtue of the `for` loop in `Next`. So:

```go
// These are equivalent:
func A(c *gin.Context) {
    log.Println("before")
    c.Next()
    log.Println("after")
}

func B(c *gin.Context) {
    log.Println("before")
    // no c.Next() — but the surrounding loop still calls handlers[i+1] next
}
```

In B, you can't run code *after* downstream handlers. Call `c.Next()` explicitly whenever you need post-work.

---

## 3. Variations — scope, order, and composition

### Scopes

```go
r := gin.New()

// 1. Engine-level: every request
r.Use(Logger(), Recovery())

// 2. Group-level: every route in the group
api := r.Group("/api", RequestID(), Auth())

// 3. Per-route: only this route
api.GET("/admin", RequireRole("admin"), adminHandler)
```

The final chain for `GET /api/admin` is:

```
[Logger, Recovery, RequestID, Auth, RequireRole("admin"), adminHandler]
```

Engine → group → per-route → handler. Within each scope, registration order is preserved.

### Order matters

```go
r.Use(Logger())            // logs duration; needs to wrap everything
r.Use(Recovery())          // catches panics; needs to be outside the handler
r.Use(RequestID())         // sets ID into context; later middleware reads it
r.Use(Auth())              // can use the request ID for the unauthorized log
```

If `Recovery` runs before `Logger`, panics aren't logged. If `Auth` runs before `RequestID`, the auth-failure log has no correlation ID. Pick an order and document it.

### Per-request global state — `c.Set` and helpers

Middleware that sets values for downstream handlers should expose **typed accessors** to avoid scattering `c.Get`/type-assertion across the codebase:

```go
package middleware

const userKey = "user"

func Auth(verify func(string) (*User, error)) gin.HandlerFunc {
    return func(c *gin.Context) {
        u, err := verify(c.GetHeader("Authorization"))
        if err != nil {
            c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
            return
        }
        c.Set(userKey, u)
        c.Next()
    }
}

func CurrentUser(c *gin.Context) (*User, bool) {
    v, ok := c.Get(userKey)
    if !ok {
        return nil, false
    }
    u, ok := v.(*User)
    return u, ok
}
```

Handlers call `middleware.CurrentUser(c)`. The key is private, the type assertion is in one place, refactors are easy.

### Built-in middleware worth knowing

- **`gin.Logger()`** — per-request log line to stdout. Unstructured. Replace with your own in production.
- **`gin.Recovery()`** — recovers from panics, writes 500. Always include.
- **`gin.LoggerWithConfig(cfg)`** — custom format string.
- **`gin.BasicAuth(map[string]string{...})`** — HTTP Basic Auth from a static map. Useful for `/metrics`, `/debug`.

### Common third-party middleware

- **`github.com/gin-contrib/cors`** — CORS handling. Configure allowed origins explicitly; never `AllowAllOrigins: true` with credentials.
- **`github.com/gin-contrib/gzip`** — gzip response compression.
- **`github.com/gin-contrib/sessions`** — server-side sessions.
- **`github.com/gin-contrib/secure`** — security headers (HSTS, X-Frame-Options, CSP).
- **`github.com/gin-contrib/requestid`** — drop-in request ID middleware.
- **`github.com/gin-contrib/timeout`** — wraps handlers with a context deadline + canned 504 response.

### Idiomatic Go question: when does this stop being middleware?

When you find yourself writing complex business logic inside `func() gin.HandlerFunc { ... }`, you've conflated transport with domain. Push the logic into a service package and let the middleware do only:

- Auth/authz
- Request ID / tracing
- Rate limiting
- Logging
- Recovery
- Body size limits
- CORS / security headers
- Metrics

If a middleware needs to know about "users" or "orders," it's probably a handler in disguise.

---

## 4. Practical application — request ID, structured logging, rate limit, and timeout

A realistic stack a senior engineer would assemble in a Gin service. Five middlewares, ordered carefully.

```go
// internal/http/middleware/middleware.go
package middleware

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "errors"
    "log/slog"
    "net/http"
    "sync"
    "time"

    "github.com/gin-gonic/gin"
    "golang.org/x/time/rate"
)

// ---------- Request ID ----------

const RequestIDKey = "request_id"

func RequestID() gin.HandlerFunc {
    return func(c *gin.Context) {
        rid := c.GetHeader("X-Request-ID")
        if rid == "" {
            b := make([]byte, 8)
            _, _ = rand.Read(b)
            rid = hex.EncodeToString(b)
        }
        c.Set(RequestIDKey, rid)
        c.Header("X-Request-ID", rid)
        c.Next()
    }
}

// ---------- slog logger ----------

func Logger(log *slog.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path
        c.Next()
        log.Info("http",
            "method", c.Request.Method,
            "path", path,
            "status", c.Writer.Status(),
            "bytes", c.Writer.Size(),
            "duration_ms", time.Since(start).Milliseconds(),
            "request_id", c.GetString(RequestIDKey),
            "client_ip", c.ClientIP(),
        )
    }
}

// ---------- Recovery with structured log ----------

func Recovery(log *slog.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        defer func() {
            if rec := recover(); rec != nil {
                log.Error("panic",
                    "request_id", c.GetString(RequestIDKey),
                    "panic", rec,
                )
                c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
                    "error":      "internal server error",
                    "request_id": c.GetString(RequestIDKey),
                })
            }
        }()
        c.Next()
    }
}

// ---------- Rate limit (per-IP, token bucket) ----------

type ipLimiter struct {
    mu       sync.Mutex
    limiters map[string]*rate.Limiter
    r        rate.Limit
    burst    int
}

func newIPLimiter(rps int, burst int) *ipLimiter {
    return &ipLimiter{
        limiters: make(map[string]*rate.Limiter),
        r:        rate.Limit(rps),
        burst:    burst,
    }
}

func (l *ipLimiter) get(ip string) *rate.Limiter {
    l.mu.Lock()
    defer l.mu.Unlock()
    if lim, ok := l.limiters[ip]; ok {
        return lim
    }
    lim := rate.NewLimiter(l.r, l.burst)
    l.limiters[ip] = lim
    return lim
}

func RateLimit(rps, burst int) gin.HandlerFunc {
    lim := newIPLimiter(rps, burst)
    return func(c *gin.Context) {
        if !lim.get(c.ClientIP()).Allow() {
            c.Header("Retry-After", "1")
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limited"})
            return
        }
        c.Next()
    }
}

// ---------- Timeout ----------

func Timeout(d time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        ctx, cancel := context.WithTimeout(c.Request.Context(), d)
        defer cancel()
        c.Request = c.Request.WithContext(ctx)

        done := make(chan struct{})
        go func() {
            c.Next()
            close(done)
        }()

        select {
        case <-done:
            return
        case <-ctx.Done():
            if errors.Is(ctx.Err(), context.DeadlineExceeded) {
                c.AbortWithStatusJSON(http.StatusGatewayTimeout,
                    gin.H{"error": "request timed out"})
            }
        }
    }
}
```

> **Note on `Timeout`:** the goroutine + select pattern above is a sketch — real production timeout middleware is harder than it looks because Gin's `Context` and `Writer` are not goroutine-safe. Use `github.com/gin-contrib/timeout` in real code; this example illustrates the pattern. The full story lives in module 12.

### Wiring

```go
// cmd/api/main.go
package main

import (
    "log/slog"
    "os"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/you/hello-gin/internal/http/middleware"
)

func main() {
    log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

    r := gin.New()
    r.Use(
        middleware.RequestID(),
        middleware.Logger(log),
        middleware.Recovery(log),
        middleware.RateLimit(50, 100),
        middleware.Timeout(2*time.Second),
    )

    r.GET("/ping", func(c *gin.Context) {
        c.JSON(200, gin.H{"message": "pong"})
    })

    r.GET("/boom", func(c *gin.Context) {
        panic("boom")
    })

    r.Run(":8080")
}
```

Test:

```bash
curl -i http://localhost:8080/ping
# X-Request-ID present; structured log line in stdout

curl -i http://localhost:8080/boom
# 500 with request_id in body; structured error log

# Hammer until 429
for i in {1..200}; do curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/ping; done | tail
```

---

## 5. Common mistakes & gotchas

- **Forgetting `return` after `c.Abort*`.** Abort flips a flag; the current function keeps running. Code-review reflex: every `Abort*` line must be followed by `return`.
- **Wrong order of `Recovery` and `Logger`.** Put `Recovery` such that it can write the 500 response *and* `Logger` records that 500. Usual order: RequestID → Logger → Recovery → everything else, so Logger captures the final status that Recovery set.
- **Panic in a goroutine bypasses `Recovery`.** `gin.Recovery` recovers from the current goroutine only. A `go func() { panic(...) }()` from inside a handler will crash the process. Module 12 covers the fix: wrap every spawn with its own `defer recover()`.
- **Mutable shared state in middleware closure.** Middleware factories return `gin.HandlerFunc`. If you capture a mutable map in the closure and write to it from the handler without a mutex, you have a race. Use `sync.Mutex` or `sync.Map`.
- **Reading the body in middleware, then trying to bind later.** `c.Request.Body` is a one-shot reader. If a logging middleware reads it, the handler's `ShouldBindJSON` returns EOF. Buffer and restore with `io.NopCloser(bytes.NewReader(buf))`.
- **Using `c.Next()` inside a non-middleware handler.** `c.Next()` from the leaf handler is a no-op (nothing left in the chain), but it can confuse readers. Reserve it for middleware.
- **Engine-level middleware for endpoint-specific concerns.** If only one route needs CSRF protection, attach it to that route, not `r.Use`. Engine-level adds latency to every request including `/healthz` and `/metrics`.
- **Treating `gin.BasicAuth` as production-grade.** It compares passwords against a hardcoded map with `subtle.ConstantTimeCompare`. Fine for `/metrics`, not fine as your only auth layer.

---

## 🎯 Key Takeaways

1. **Middleware is just a `HandlerFunc` in a slice.** No magic. `c.Next()` walks forward; `c.Abort()` flips a sentinel. Internalize this and the framework loses all its mystery.
2. **`Abort` does not return.** The single most common Gin bug. Make it a code-review rule: every `c.Abort*` line is immediately followed by `return`.
3. **Order the chain deliberately.** RequestID → Logger → Recovery → Auth → Rate limit → Timeout → Handler is a defensible default. Document it in `router.go`.
4. **Push business logic out of middleware.** Middleware handles transport concerns only — auth, IDs, metrics, recovery, limits. Anything that talks about your domain should be a service called from a handler.
5. **Panic in a spawned goroutine kills the process.** `Recovery` is per-goroutine. If you `go func()` from a handler, that function needs its own `defer recover()` or you've shipped a crash bug.

*← [05 — Response Handling](./05_response_handling.md) | [07 — Error Handling →](./07_error_handling.md)*
