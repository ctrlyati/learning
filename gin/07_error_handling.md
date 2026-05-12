# 07 — Error Handling

> **Goal:** Build a centralized, structured error-handling pipeline with `c.Error`, a final error middleware, domain-to-HTTP mapping, and panic recovery — the way senior Go engineers do it.

---

## 1. Errors in Gin — mental model + working code

Go's idiom is "return an error." Gin's idiom is "handler writes a response or aborts." The bridge between them is `c.Error(err)` — it appends an error to `c.Errors` (a slice on the context) so that something downstream can inspect, log, and convert it to a response.

```go
func getUser(c *gin.Context) {
    id, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        _ = c.Error(err)                            // record
        c.JSON(http.StatusBadRequest, gin.H{"error": "id must be an integer"})
        return
    }
    u, err := userService.Get(c.Request.Context(), int64(id))
    if err != nil {
        _ = c.Error(err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
        return
    }
    c.JSON(200, u)
}
```

You record the error *and* still write a response. The reason to record: a logging middleware later can read `c.Errors` and emit the *cause*, not just the status. That's the whole point — the response is for the user, the recorded error is for you.

But there's a cleaner pattern: **handlers return errors; a wrapper renders them.** Let's get there.

---

## 2. How `c.Error` works

```go
type Error struct {
    Err  error
    Type ErrorType
    Meta any
}

func (c *Context) Error(err error) *Error {
    parsed := &Error{Err: err, Type: ErrorTypePrivate}
    c.Errors = append(c.Errors, parsed)
    return parsed
}
```

`c.Errors` is a slice on the Context (`Errors []*Error`). Each entry carries the error, a type tag (`ErrorTypeBind`, `ErrorTypePublic`, `ErrorTypePrivate`, `ErrorTypeAny`), and an optional `Meta` payload.

You can read them in middleware:

```go
func ErrorLogger(log *slog.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Next()
        for _, e := range c.Errors {
            log.Error("handler error",
                "err", e.Err.Error(),
                "type", e.Type,
                "path", c.Request.URL.Path,
                "status", c.Writer.Status(),
            )
        }
    }
}
```

This is the **observer** pattern: handlers record errors, middleware reports them. `c.Errors` itself doesn't change the HTTP response.

### Tagged errors

```go
_ = c.Error(err).SetType(gin.ErrorTypePublic)        // safe to show to user
_ = c.Error(err).SetMeta(gin.H{"field": "email"})    // attach context

// Later, filter by type:
publics := c.Errors.ByType(gin.ErrorTypePublic)
```

In practice most teams skip the `ErrorType` machinery and use their own typed errors. Gin's is too coarse.

---

## 3. The senior pattern — handlers return errors, middleware renders them

Define a typed error envelope and a handler wrapper. This pattern scales much better than spraying `c.JSON(...)` across handlers.

### `apperr` — the typed error

```go
// internal/apperr/apperr.go
package apperr

import (
    "errors"
    "fmt"
    "net/http"
)

type Code string

const (
    CodeBadRequest    Code = "BAD_REQUEST"
    CodeUnauthorized  Code = "UNAUTHORIZED"
    CodeForbidden     Code = "FORBIDDEN"
    CodeNotFound      Code = "NOT_FOUND"
    CodeConflict      Code = "CONFLICT"
    CodeRateLimit     Code = "RATE_LIMIT"
    CodeInternal      Code = "INTERNAL"
)

type Error struct {
    Code     Code              // semantic error code; stable in API contract
    HTTP     int               // HTTP status
    Message  string            // user-safe message
    Fields   map[string]string // optional per-field details
    Cause    error             // wrapped underlying error
}

func (e *Error) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
    }
    return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

// Constructors

func BadRequest(msg string, cause error) *Error {
    return &Error{Code: CodeBadRequest, HTTP: http.StatusBadRequest, Message: msg, Cause: cause}
}
func Unauthorized(msg string) *Error {
    return &Error{Code: CodeUnauthorized, HTTP: http.StatusUnauthorized, Message: msg}
}
func Forbidden(msg string) *Error {
    return &Error{Code: CodeForbidden, HTTP: http.StatusForbidden, Message: msg}
}
func NotFound(msg string) *Error {
    return &Error{Code: CodeNotFound, HTTP: http.StatusNotFound, Message: msg}
}
func Conflict(msg string, cause error) *Error {
    return &Error{Code: CodeConflict, HTTP: http.StatusConflict, Message: msg, Cause: cause}
}
func Internal(cause error) *Error {
    return &Error{Code: CodeInternal, HTTP: http.StatusInternalServerError,
        Message: "internal server error", Cause: cause}
}

// As unwraps to *Error.
func As(err error) (*Error, bool) {
    var e *Error
    if errors.As(err, &e) {
        return e, true
    }
    return nil, false
}
```

### The handler wrapper

```go
// internal/http/ginx/wrap.go
package ginx

import (
    "log/slog"

    "github.com/gin-gonic/gin"
    "github.com/you/hello-gin/internal/apperr"
)

// HandlerWithErr is a handler that returns an error.
type HandlerWithErr func(c *gin.Context) error

// Wrap converts a HandlerWithErr into a gin.HandlerFunc that
// records the error and renders a normalized JSON response.
func Wrap(log *slog.Logger, h HandlerWithErr) gin.HandlerFunc {
    return func(c *gin.Context) {
        if err := h(c); err != nil {
            render(c, log, err)
        }
    }
}

func render(c *gin.Context, log *slog.Logger, err error) {
    _ = c.Error(err)                                 // record for middleware
    if ae, ok := apperr.As(err); ok {
        if ae.HTTP >= 500 {
            log.Error("server error", "code", ae.Code, "err", ae.Error(),
                "request_id", c.GetString("request_id"))
        }
        c.AbortWithStatusJSON(ae.HTTP, gin.H{
            "error":     string(ae.Code),
            "message":   ae.Message,
            "fields":    ae.Fields,
            "request_id": c.GetString("request_id"),
        })
        return
    }
    // Unknown error → 500
    log.Error("unexpected error", "err", err.Error(),
        "request_id", c.GetString("request_id"))
    c.AbortWithStatusJSON(500, gin.H{
        "error":      "INTERNAL",
        "message":    "internal server error",
        "request_id": c.GetString("request_id"),
    })
}
```

### Handlers become clean

```go
// internal/http/handlers/users.go
package handlers

import (
    "strconv"

    "github.com/gin-gonic/gin"
    "github.com/you/hello-gin/internal/apperr"
    "github.com/you/hello-gin/internal/service"
)

type UserHandler struct {
    Users *service.UserService
}

func (h *UserHandler) Get(c *gin.Context) error {
    id, err := strconv.ParseInt(c.Param("id"), 10, 64)
    if err != nil {
        return apperr.BadRequest("id must be an integer", err)
    }
    u, err := h.Users.Get(c.Request.Context(), id)
    if err != nil {
        return mapServiceErr(err)
    }
    c.JSON(200, u)
    return nil
}

func mapServiceErr(err error) error {
    switch {
    case errors.Is(err, service.ErrUserNotFound):
        return apperr.NotFound("user not found")
    case errors.Is(err, service.ErrEmailTaken):
        return apperr.Conflict("email already taken", err)
    default:
        return apperr.Internal(err)
    }
}
```

### Wiring

```go
// internal/http/router.go
api := r.Group("/api/v1")
api.GET("/users/:id", ginx.Wrap(log, h.Get))
api.POST("/users",    ginx.Wrap(log, h.Create))
```

Now every handler is one liner per route, errors are structured, and adding a new endpoint is mechanical.

---

## 4. Practical application — panic recovery + domain error mapping

A complete example combining recovery, structured errors, and domain mapping. We model a `UserService` that returns domain errors; the handler maps them to `apperr` and the wrapper renders them.

### Domain layer

```go
// internal/service/user.go
package service

import (
    "context"
    "errors"
)

var (
    ErrUserNotFound = errors.New("user not found")
    ErrEmailTaken   = errors.New("email taken")
)

type User struct {
    ID    int64  `json:"id"`
    Email string `json:"email"`
    Name  string `json:"name"`
}

type UserStore interface {
    GetByID(ctx context.Context, id int64) (*User, error)
    Create(ctx context.Context, u *User) error
}

type UserService struct{ Store UserStore }

func (s *UserService) Get(ctx context.Context, id int64) (*User, error) {
    u, err := s.Store.GetByID(ctx, id)
    if err != nil {
        return nil, err
    }
    return u, nil
}
```

### Fake store

```go
// internal/store/memstore.go
package store

import (
    "context"

    "github.com/you/hello-gin/internal/service"
)

type Mem struct {
    Users map[int64]*service.User
}

func (m *Mem) GetByID(_ context.Context, id int64) (*service.User, error) {
    u, ok := m.Users[id]
    if !ok {
        return nil, service.ErrUserNotFound
    }
    return u, nil
}

func (m *Mem) Create(_ context.Context, u *service.User) error {
    for _, x := range m.Users {
        if x.Email == u.Email {
            return service.ErrEmailTaken
        }
    }
    u.ID = int64(len(m.Users) + 1)
    m.Users[u.ID] = u
    return nil
}
```

### Recovery middleware that logs structured

```go
// internal/http/middleware/recovery.go
package middleware

import (
    "log/slog"
    "net/http"
    "runtime/debug"

    "github.com/gin-gonic/gin"
)

func Recovery(log *slog.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        defer func() {
            if rec := recover(); rec != nil {
                log.Error("panic recovered",
                    "panic", rec,
                    "stack", string(debug.Stack()),
                    "request_id", c.GetString("request_id"),
                    "path", c.Request.URL.Path,
                )
                c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
                    "error":      "INTERNAL",
                    "message":    "internal server error",
                    "request_id": c.GetString("request_id"),
                })
            }
        }()
        c.Next()
    }
}
```

### Main

```go
// cmd/api/main.go
package main

import (
    "log/slog"
    "os"

    "github.com/gin-gonic/gin"
    "github.com/you/hello-gin/internal/http/ginx"
    "github.com/you/hello-gin/internal/http/handlers"
    "github.com/you/hello-gin/internal/http/middleware"
    "github.com/you/hello-gin/internal/service"
    "github.com/you/hello-gin/internal/store"
)

func main() {
    log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

    mem := &store.Mem{Users: map[int64]*service.User{
        1: {ID: 1, Email: "y@x.com", Name: "yati"},
    }}
    svc := &service.UserService{Store: mem}
    h := &handlers.UserHandler{Users: svc}

    r := gin.New()
    r.Use(middleware.RequestID(), middleware.Logger(log), middleware.Recovery(log))

    api := r.Group("/api/v1")
    api.GET("/users/:id", ginx.Wrap(log, h.Get))
    api.GET("/panic", ginx.Wrap(log, func(c *gin.Context) error {
        panic("intentional")
    }))

    r.Run(":8080")
}
```

Test:

```bash
# Happy path
curl -i http://localhost:8080/api/v1/users/1
# 200 {"id":1,"email":"y@x.com","name":"yati"}

# Not found — mapped from service.ErrUserNotFound
curl -i http://localhost:8080/api/v1/users/999
# 404 {"error":"NOT_FOUND","message":"user not found","request_id":"..."}

# Bad ID — mapped from strconv error
curl -i http://localhost:8080/api/v1/users/abc
# 400 {"error":"BAD_REQUEST","message":"id must be an integer","request_id":"..."}

# Panic — caught by Recovery
curl -i http://localhost:8080/api/v1/panic
# 500 {"error":"INTERNAL","message":"internal server error","request_id":"..."}
# server logs include the stack trace
```

Every response has a consistent shape; every error has a stable `error` code; every server-side error has a stack trace in logs correlated by `request_id`. That is the standard.

---

## 5. Common mistakes & gotchas

- **Returning raw stdlib errors to the client.** `c.JSON(500, gin.H{"error": err.Error()})` leaks internal details — SQL fragments, file paths, panics with line numbers. Always map to a sanitized message.
- **`fmt.Errorf` without `%w`.** Use `fmt.Errorf("could not load user %d: %w", id, err)` — the `%w` lets `errors.Is`/`errors.As` unwrap. `%v` flattens the chain and you lose the cause.
- **`c.Error(err)` without a response.** Recording an error doesn't write anything to the client. You must still `c.JSON` / `c.Abort*` / `return`. Many beginners think `c.Error` ends the handler — it doesn't.
- **Panic in a spawned goroutine.** `gin.Recovery` only catches the current goroutine. `go func() { panic(...) }()` from a handler will crash the process. Always wrap goroutines with `defer recover()` of your own (module 12).
- **Catching errors with `if err != nil { return err }` but never wrapping with context.** When you see "sql: no rows in result set" in a log with no other info, you'll wish you had wrapped at each layer.
- **Different error envelopes per endpoint.** One endpoint returns `{"error": "..."}`, another `{"errors": [...]}`, another `{"message": "..."}`. Clients have to handle every shape. Pick one envelope and enforce it via the wrapper.
- **Stack traces in production responses.** A 500 page that shows stack frames is a security and operational sin. Stack traces go to **logs**, not bodies.
- **Forgetting to log a 4xx that indicates a bug.** A 500 should be logged at ERROR; a 401 you expect should not. But a sudden spike of 400s from one client is signal — log at INFO or WARN so you can see it.

---

## 🎯 Key Takeaways

1. **Define a typed error envelope (`apperr.Error`) and map domain errors to it.** This single decision keeps your handlers tiny and your API responses consistent.
2. **Use a handler wrapper (`ginx.Wrap`) that converts `error` returns into responses.** It eliminates the four-line `if err != nil { c.JSON(...); return }` pattern from every handler and is the single biggest readability win in a Gin codebase.
3. **Wrap errors with `%w`, not `%v`.** It is the foundation of `errors.Is`/`errors.As`, which is the foundation of clean domain → transport error mapping.
4. **Recover panics in middleware *and* in every goroutine you spawn.** `gin.Recovery` is not a process-wide safety net; it is per-goroutine.
5. **Stack traces and underlying causes go to structured logs with `request_id`; the response body shows a clean code + message.** That correlation is what makes 3 a.m. debugging possible.

*← [06 — Middleware](./06_middleware.md) | [08 — Templates and Static Files →](./08_templates_and_static.md)*
