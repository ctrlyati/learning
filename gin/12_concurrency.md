# 12 — Concurrency Patterns

> **Goal:** Spawn goroutines from Gin handlers correctly (including `c.Copy()`), propagate `context.Context`, build background workers, and shut the whole thing down gracefully.

---

## 1. Goroutines from handlers — the `c.Copy()` rule

Goroutines from inside a Gin handler are **the single biggest footgun in the framework.** Three reasons:

1. `*gin.Context` is pooled. The pointer is recycled the instant the response is written.
2. `*gin.Context`'s fields (`Keys` map, params, errors) are not goroutine-safe.
3. A panic in a spawned goroutine bypasses `gin.Recovery` and crashes the process.

### The rule

If you spawn a goroutine and want anything `gin.Context`-like inside it, **call `c.Copy()` first** and pass the copy. Never the original `c`. Always wrap the goroutine in your own `defer recover()`.

### What `c.Copy()` returns

```go
// Approximate:
func (c *Context) Copy() *Context {
    cp := *c
    cp.writermem.ResponseWriter = nil
    cp.Writer = &cp.writermem
    cp.index = abortIndex
    cp.handlers = nil
    cp.Keys = map[string]any{}
    for k, v := range c.Keys {
        cp.Keys[k] = v
    }
    return &cp
}
```

A new `*Context` with:

- A snapshotted **copy of `c.Keys`** (so you can read `c.Get("user_id")` etc.).
- The same `Request` (so `c.Request.Context()` still works).
- No usable `Writer` (the response is for the handler, not the goroutine).
- `index = abortIndex`, so `Next` does nothing.

In other words: a read-only-ish snapshot of request metadata that's safe to retain.

### The correct pattern

```go
func enqueueWelcomeEmail(c *gin.Context) {
    user := middleware.CurrentUser(c)

    cc := c.Copy()                                   // 1. Copy

    go func(cc *gin.Context, u *service.User) {
        defer func() {                               // 2. Recover
            if rec := recover(); rec != nil {
                log.Error("background panic",
                    "panic", rec,
                    "request_id", cc.GetString("request_id"))
            }
        }()

        ctx, cancel := context.WithTimeout(           // 3. Use Request.Context() for cancellation,
            context.Background(), 30*time.Second)     //    or a derived background context with its own timeout.
        defer cancel()

        if err := emailer.SendWelcome(ctx, u.Email); err != nil {
            log.Error("welcome email failed",
                "err", err,
                "request_id", cc.GetString("request_id"))
        }
    }(cc, user)

    c.JSON(202, gin.H{"queued": true})
}
```

Three habits in one block:

1. **`c.Copy()` first**, pass the copy into the goroutine.
2. **`defer recover()`** in every goroutine.
3. **A `context.Context` that does NOT die when the HTTP request returns.** `c.Request.Context()` cancels as soon as the response is written — that's wrong for background work. Use `context.Background()` (or a long-lived parent) with its own timeout.

The third point is subtle and important. `c.Request.Context().Done()` fires when the *response* is written or the *client* disconnects. If your goroutine is supposed to keep running after the response, don't tie it to that context.

---

## 2. How goroutines, `Context`, and Gin interact

### `c.Request.Context()` vs `context.Background()`

| Use case | Context to pass |
|----------|-----------------|
| DB query *during* the request | `c.Request.Context()` — cancels if client disconnects |
| HTTP call *during* the request | `c.Request.Context()` — same reason |
| Background work spawned from handler | `context.Background()` derived (with timeout) — must survive the request |
| Worker goroutine in `main` | a context owned by `main`, cancellable on shutdown |

The most common bug: using `c.Request.Context()` in a background goroutine and then wondering why the email send always errors with "context canceled" right after the response is sent.

### `context.Context` propagation

Anywhere your service code touches the network or a DB, take `ctx context.Context` as the first parameter. Pass `c.Request.Context()` from the handler. This is the universal Go idiom — Gin doesn't change it.

```go
func (s *UserService) Get(ctx context.Context, id int64) (*User, error) {
    return s.Store.GetByID(ctx, id)
}

// handler
u, err := svc.Get(c.Request.Context(), id)
```

Never call `context.TODO()` outside of skeleton code. Never call `context.Background()` inside a handler for an in-request call. The one place `Background()` is correct in a handler is exactly the background-work case above.

### Panic in a goroutine kills the process

```go
go func() {
    panic("oh no")    // bypasses gin.Recovery
}()
```

`gin.Recovery` runs `defer recover()` on the request goroutine only. Anything you `go func()` is a new goroutine with no recover frame. **Always defer recover in every goroutine you spawn.**

A common pattern is a helper:

```go
func safeGo(name string, log *slog.Logger, fn func()) {
    go func() {
        defer func() {
            if rec := recover(); rec != nil {
                log.Error("goroutine panic", "name", name, "panic", rec,
                    "stack", string(debug.Stack()))
            }
        }()
        fn()
    }()
}
```

Force everyone to use `safeGo(...)`. Code review enforces it.

---

## 3. Background workers and graceful shutdown

### A worker started from `main`

```go
// internal/worker/welcome.go
package worker

import (
    "context"
    "log/slog"
    "time"
)

type WelcomeEmailer interface {
    Send(ctx context.Context, email string) error
}

type WelcomeWorker struct {
    Emailer WelcomeEmailer
    Queue   <-chan string
    Log     *slog.Logger
}

func (w *WelcomeWorker) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            w.Log.Info("welcome worker stopping")
            return
        case email := <-w.Queue:
            sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
            if err := w.Emailer.Send(sendCtx, email); err != nil {
                w.Log.Error("send failed", "err", err, "email", email)
            }
            cancel()
        }
    }
}
```

### Wiring + graceful shutdown

```go
// cmd/api/main.go
package main

import (
    "context"
    "errors"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/you/hello-gin/internal/worker"
)

func main() {
    log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

    // Root context for the whole process, cancelled on SIGINT/SIGTERM.
    rootCtx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    // Worker
    queue := make(chan string, 100)
    w := &worker.WelcomeWorker{Emailer: realEmailer, Queue: queue, Log: log}
    workerDone := make(chan struct{})
    go func() {
        defer close(workerDone)
        w.Run(rootCtx)
    }()

    // Gin server
    r := gin.New()
    r.POST("/signup", func(c *gin.Context) {
        // ... after persisting the user:
        select {
        case queue <- "user@x.com":
        default:
            log.Warn("welcome queue full, dropping")
        }
        c.JSON(202, gin.H{"queued": true})
    })

    srv := &http.Server{
        Addr:              ":8080",
        Handler:           r,
        ReadHeaderTimeout: 5 * time.Second,
        ReadTimeout:       15 * time.Second,
        WriteTimeout:      30 * time.Second,
        IdleTimeout:       60 * time.Second,
    }

    serverErr := make(chan error, 1)
    go func() {
        log.Info("listening", "addr", srv.Addr)
        if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            serverErr <- err
        }
        close(serverErr)
    }()

    // Wait for shutdown signal or server crash.
    select {
    case <-rootCtx.Done():
        log.Info("shutdown signal received")
    case err := <-serverErr:
        if err != nil {
            log.Error("server crashed", "err", err)
        }
    }

    // Stop accepting new connections; wait up to 25 seconds.
    shutCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
    defer cancel()
    if err := srv.Shutdown(shutCtx); err != nil {
        log.Error("graceful shutdown failed", "err", err)
    }

    // Cancel root context → worker stops.
    stop()
    <-workerDone
    log.Info("clean shutdown complete")
}

var realEmailer worker.WelcomeEmailer // wire your real impl
```

The shutdown choreography:

1. `signal.NotifyContext` cancels `rootCtx` on SIGINT/SIGTERM (Ctrl-C or Kubernetes pod termination).
2. `srv.Shutdown(ctx)` stops accepting new connections, waits for in-flight requests, returns when they're all done or `ctx` expires.
3. `stop()` cancels `rootCtx`, signaling the worker to drain.
4. `<-workerDone` waits for the worker to exit.

This is the production-grade pattern. Every Gin service should look like this.

### Fan-out within a request

Sometimes you want to call three downstream services in parallel inside one handler:

```go
func (h *Handler) Dashboard(c *gin.Context) error {
    ctx := c.Request.Context()

    type result struct {
        users  int
        orders int
        rev    int64
        err    error
    }
    var r result

    var wg sync.WaitGroup
    wg.Add(3)
    var mu sync.Mutex

    go func() {
        defer wg.Done()
        n, err := h.UserService.Count(ctx)
        mu.Lock()
        defer mu.Unlock()
        if err != nil { r.err = err; return }
        r.users = n
    }()

    go func() {
        defer wg.Done()
        n, err := h.OrderService.Count(ctx)
        mu.Lock()
        defer mu.Unlock()
        if err != nil && r.err == nil { r.err = err; return }
        r.orders = n
    }()

    go func() {
        defer wg.Done()
        v, err := h.OrderService.Revenue(ctx)
        mu.Lock()
        defer mu.Unlock()
        if err != nil && r.err == nil { r.err = err; return }
        r.rev = v
    }()

    wg.Wait()
    if r.err != nil {
        return apperr.Internal(r.err)
    }
    c.JSON(200, gin.H{"users": r.users, "orders": r.orders, "revenue": r.rev})
    return nil
}
```

Use `golang.org/x/sync/errgroup` to clean this up:

```go
import "golang.org/x/sync/errgroup"

func (h *Handler) Dashboard(c *gin.Context) error {
    ctx := c.Request.Context()
    g, gctx := errgroup.WithContext(ctx)

    var users, orders int
    var revenue int64

    g.Go(func() error {
        n, err := h.UserService.Count(gctx)
        if err != nil { return err }
        users = n; return nil
    })
    g.Go(func() error {
        n, err := h.OrderService.Count(gctx)
        if err != nil { return err }
        orders = n; return nil
    })
    g.Go(func() error {
        v, err := h.OrderService.Revenue(gctx)
        if err != nil { return err }
        revenue = v; return nil
    })

    if err := g.Wait(); err != nil {
        return apperr.Internal(err)
    }
    c.JSON(200, gin.H{"users": users, "orders": orders, "revenue": revenue})
    return nil
}
```

`errgroup` cancels `gctx` on the first error, so the other in-flight goroutines abort early. This is the senior Go idiom for fan-out — and notice: **no `c.Copy()` needed here because the goroutines complete before the handler returns**. The `c.Copy()` rule applies when the goroutine outlives the handler.

---

## 4. Practical application — handler that fires-and-forgets an audit log

A realistic slice: every request to a sensitive endpoint enqueues an audit-log entry on a buffered channel; a worker drains it to a database. The handler must not block on the DB.

```go
// internal/audit/audit.go
package audit

import (
    "context"
    "log/slog"
    "runtime/debug"
    "time"
)

type Entry struct {
    UserID    int64
    Action    string
    Resource  string
    RequestID string
    At        time.Time
}

type Sink interface {
    Insert(ctx context.Context, e Entry) error
}

type Logger struct {
    Sink Sink
    ch   chan Entry
    log  *slog.Logger
}

func New(sink Sink, log *slog.Logger, bufSize int) *Logger {
    return &Logger{
        Sink: sink,
        ch:   make(chan Entry, bufSize),
        log:  log,
    }
}

// Record is called from handlers. Never blocks; drops on full buffer.
func (l *Logger) Record(e Entry) {
    select {
    case l.ch <- e:
    default:
        l.log.Warn("audit buffer full, dropping", "action", e.Action)
    }
}

// Run drains the buffer until ctx is cancelled.
func (l *Logger) Run(ctx context.Context) {
    defer func() {
        if rec := recover(); rec != nil {
            l.log.Error("audit worker panic", "panic", rec, "stack", string(debug.Stack()))
        }
    }()
    for {
        select {
        case <-ctx.Done():
            l.drain()
            return
        case e := <-l.ch:
            sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            if err := l.Sink.Insert(sctx, e); err != nil {
                l.log.Error("audit insert failed", "err", err)
            }
            cancel()
        }
    }
}

func (l *Logger) drain() {
    for {
        select {
        case e := <-l.ch:
            sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
            _ = l.Sink.Insert(sctx, e)
            cancel()
        default:
            return
        }
    }
}
```

```go
// handlers
func (h *UserHandler) Delete(c *gin.Context) error {
    id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
    if err := h.Users.Delete(c.Request.Context(), id); err != nil {
        return apperr.Internal(err)
    }
    h.Audit.Record(audit.Entry{
        UserID:    c.GetInt64("user_id"),
        Action:    "user.delete",
        Resource:  fmt.Sprintf("user:%d", id),
        RequestID: c.GetString("request_id"),
        At:        time.Now().UTC(),
    })
    c.Status(http.StatusNoContent)
    return nil
}
```

Key design choices:

- **`Record` never blocks the handler.** If the channel is full, we drop and log a warning. The alternative (block) makes the audit subsystem a single point of failure for the API.
- **The worker uses `context.Background()`** for the DB insert, not the handler's context. The handler returned long ago; the audit write must survive.
- **On shutdown, drain the buffer** so we don't lose in-flight entries.

---

## 5. Common mistakes & gotchas

- **Passing `c` to a goroutine.** Classic. The pool recycles `c`; your goroutine sees garbage. `cc := c.Copy()` and pass `cc`. Code-review rule: any `go func()` from a handler captures **zero** identifiers named `c` directly.
- **Using `c.Request.Context()` for background work.** It cancels when the response is written. Your DB call dies with "context canceled" and you blame the network. Use `context.Background()` + a fresh timeout for genuine background work.
- **Panic in a goroutine.** `gin.Recovery` is per-goroutine. Always `defer recover()` in every goroutine you spawn. Build a `safeGo(name, log, fn)` helper and ban naked `go`.
- **Blocking on a full channel in the request path.** Channel buffers are not infinite. Either drop (and log/metric the drop) or use a pool of workers. Don't block the request thread on backpressure.
- **Not shutting down workers on SIGTERM.** Kubernetes sends SIGTERM, waits 30s, then SIGKILL. If your worker is mid-DB-write at SIGKILL, you may corrupt state. Use `signal.NotifyContext` + drain.
- **`http.Server` defaults are unsafe.** `r.Run` doesn't set timeouts. `ReadTimeout: 0` is "wait forever" — a slowloris attack pins all your connections. Always set `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout` on an explicit `http.Server`.
- **Goroutine leaks.** A goroutine that selects on a channel that no one closes lives forever. Always provide a cancellation path. `context.WithCancel` is your friend; use `pprof` to spot leaks.
- **`sync.Mutex` copied by value.** `mu sync.Mutex` inside a struct used through a value receiver copies the mutex — each copy is a separate lock. Use pointer receivers for any type with a mutex. `go vet` catches this; CI must run it.
- **Calling `wg.Done()` outside a `defer`.** A panic between `wg.Add(1)` and the bare `wg.Done()` deadlocks `wg.Wait()`. Always `defer wg.Done()` at the top of the goroutine.

---

## 🎯 Key Takeaways

1. **`c.Copy()` before spawning, `defer recover()` inside the spawn, `context.Background()`-derived for the work.** Memorize this triple. It is the difference between "ships to prod" and "page at 3 a.m."
2. **`errgroup` for in-request fan-out, channels for background workers.** Don't conflate the two. In-request goroutines complete before the response; background goroutines outlive it.
3. **Always run an explicit `http.Server` with timeouts.** `r.Run(":8080")` is tutorial code. Production code sets `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, and uses `srv.Shutdown(ctx)`.
4. **`signal.NotifyContext` + `srv.Shutdown` + worker drain** is the canonical graceful shutdown. Kubernetes, ECS, Nomad — every modern platform sends SIGTERM and waits before SIGKILL. Honor it.
5. **Goroutine bugs are the #1 production incident class in Go HTTP services.** Concurrent map writes, dropped panics, leaked goroutines, races on Gin's Context. The race detector (`-race`) catches most; discipline catches the rest.

*← [11 — Testing](./11_testing.md) | [13 — Observability →](./13_observability.md)*
