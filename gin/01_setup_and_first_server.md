# 01 — Setup and First Server

> **Goal:** Install Go 1.22+, scaffold a Gin module, run a first server, set up Air for hot reload, and lay out a project structure you won't outgrow in week two.

---

## 1. From `go mod init` to a running server — mental model + working code

A Go web service is just a program with a `main` function. Gin doesn't change that. The mental model is:

```
go.mod  →  declares module name + dependencies
main.go →  creates *gin.Engine, registers routes, starts http.Server
```

Nothing else is required.

### Install Go 1.22+

Go 1.22 introduced the new `for-range` loop variable semantics and shipped routing-pattern improvements in `net/http`. Gin v1.10 requires Go 1.20 at minimum, but use 1.22+ — every modern dependency assumes it.

```bash
# Verify
go version
# go version go1.22.x ...  (or later)
```

### Initialize the module

```bash
mkdir hello-gin && cd hello-gin
go mod init github.com/you/hello-gin
go get -u github.com/gin-gonic/gin@latest
```

`go mod init` writes a `go.mod`. `go get` adds Gin to it and creates `go.sum` (lockfile of cryptographic hashes — commit it).

```text
hello-gin/
├── go.mod
├── go.sum
└── main.go
```

### First server

```go
// main.go
package main

import (
    "log"
    "net/http"

    "github.com/gin-gonic/gin"
)

func main() {
    r := gin.Default() // Engine with Logger + Recovery middleware

    r.GET("/ping", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"message": "pong"})
    })

    if err := r.Run(":8080"); err != nil {
        log.Fatalf("server failed: %v", err)
    }
}
```

Run and test:

```bash
go run .
# in another terminal:
curl -i http://localhost:8080/ping
# HTTP/1.1 200 OK
# Content-Type: application/json; charset=utf-8
# {"message":"pong"}
```

That's the whole "hello world." Three Gin APIs: `gin.Default()`, `r.GET(...)`, `c.JSON(...)`.

---

## 2. How Gin gets here — `gin.Engine`, `gin.Default`, and `r.Run`

### `gin.Engine`

`gin.Engine` is the top-level struct. It owns:

- A **router tree per HTTP method** (`trees [9]methodTree`).
- A pool of `*gin.Context` (`sync.Pool`) — reused across requests.
- The chain of middleware registered at engine level (`Handlers []HandlerFunc`).
- Configuration knobs (`RedirectTrailingSlash`, `HandleMethodNotAllowed`, `MaxMultipartMemory`, etc.).

**It implements `http.Handler`.** That single fact is the most important thing in this module. Everything Gin does happens inside its `ServeHTTP(w, r)` method.

### `gin.Default()` vs `gin.New()`

```go
gin.New()       // Engine with NO middleware
gin.Default()   // Engine with Logger() + Recovery() pre-attached
```

`Default()` is fine for getting started. In production code I almost always use `gin.New()` and attach my own logger (zap or slog) and a recovery middleware that emits structured logs — module 13 covers it.

### `r.Run` is just `http.ListenAndServe`

```go
// Roughly:
func (engine *Engine) Run(addr ...string) error {
    address := resolveAddress(addr)
    return http.ListenAndServe(address, engine)
}
```

That's it. For graceful shutdown, TLS, custom timeouts — bypass `r.Run` and use `http.Server` directly. Module 15 does this. For now `r.Run(":8080")` is fine.

### `GIN_MODE`

Gin has three modes that affect logging verbosity and panic output:

| Mode | Env var | What it changes |
|------|---------|-----------------|
| `debug` | unset or `GIN_MODE=debug` | Verbose route logging, colorized output, prints warnings |
| `release` | `GIN_MODE=release` | Quiet startup, no debug warnings |
| `test` | `GIN_MODE=test` | Used inside tests; suppresses startup noise |

Set `GIN_MODE=release` in production, or call `gin.SetMode(gin.ReleaseMode)` in `main`. Otherwise Gin prints a startle-warning to stderr on every boot.

---

## 3. Project layout that scales

The "right" layout depends on the project, but here are three sensible defaults.

### Tiny (single file)

```text
main.go
go.mod
```

Use this for prototypes, internal tools, anything < ~200 lines.

### Small service (flat package)

```text
.
├── main.go
├── handlers.go
├── models.go
├── go.mod
└── go.sum
```

Fine up to a few thousand lines. Don't over-architect.

### Production service (typical layout)

```text
.
├── cmd/
│   └── api/
│       └── main.go              // entry point — wires everything
├── internal/
│   ├── http/
│   │   ├── router.go            // gin.Engine setup, route registration
│   │   ├── middleware/
│   │   │   ├── auth.go
│   │   │   └── requestid.go
│   │   └── handlers/
│   │       ├── users.go
│   │       └── orders.go
│   ├── service/                 // business logic, no HTTP types
│   │   ├── user.go
│   │   └── order.go
│   ├── store/                   // data access
│   │   ├── postgres.go
│   │   └── queries.sql.go       // sqlc output
│   └── config/
│       └── config.go
├── migrations/
├── api/
│   └── openapi.yaml
├── Dockerfile
├── go.mod
└── go.sum
```

Key choices:

- **`cmd/<bin>/main.go`** — the entrypoint. If you ship multiple binaries (API server, worker, CLI), each lives under `cmd/`.
- **`internal/`** — Go's compiler enforces that nothing outside this module can import from `internal/`. Use it to wall off implementation details.
- **`internal/service/` knows nothing about HTTP.** This is the discipline that lets you swap Gin for gRPC later. Handlers translate HTTP ↔ service; services translate service ↔ store.
- **`api/` for contracts** — OpenAPI specs, protobuf, anything a consumer reads.

This is the layout module 09 onward assumes.

---

## 4. Practical application — scaffold a real starting point

Let's build the skeleton you'll evolve over the rest of the course.

### Files

```text
hello-gin/
├── cmd/api/main.go
├── internal/http/router.go
├── internal/http/handlers/ping.go
├── go.mod
└── .air.toml
```

### `cmd/api/main.go`

```go
package main

import (
    "log"
    "os"

    "github.com/gin-gonic/gin"
    "github.com/you/hello-gin/internal/http/router"
)

func main() {
    if os.Getenv("GIN_MODE") == "" {
        gin.SetMode(gin.DebugMode) // explicit; silences the startup warning
    }

    r := router.New()

    addr := ":" + envDefault("PORT", "8080")
    log.Printf("listening on %s", addr)
    if err := r.Run(addr); err != nil {
        log.Fatalf("server failed: %v", err)
    }
}

func envDefault(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}
```

### `internal/http/router.go`

```go
package router

import (
    "github.com/gin-gonic/gin"
    "github.com/you/hello-gin/internal/http/handlers"
)

func New() *gin.Engine {
    r := gin.New()
    r.Use(gin.Logger(), gin.Recovery())

    r.GET("/healthz", handlers.Health)
    r.GET("/ping", handlers.Ping)

    return r
}
```

### `internal/http/handlers/ping.go`

```go
package handlers

import (
    "net/http"

    "github.com/gin-gonic/gin"
)

func Ping(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"message": "pong"})
}

func Health(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```

Test:

```bash
go run ./cmd/api
curl http://localhost:8080/ping
# {"message":"pong"}
curl http://localhost:8080/healthz
# {"status":"ok"}
```

### Hot reload with Air

Manually re-running `go run` after every save gets old. Install [Air](https://github.com/air-verse/air):

```bash
go install github.com/air-verse/air@latest
# Make sure $GOPATH/bin (or $HOME/go/bin) is on your PATH.
```

Create `.air.toml` in the project root:

```toml
# .air.toml
root = "."
testdata_dir = "testdata"
tmp_dir = "tmp"

[build]
  args_bin = []
  bin = "./tmp/main.exe"
  cmd = "go build -o ./tmp/main.exe ./cmd/api"
  delay = 1000
  exclude_dir = ["assets", "tmp", "vendor", "testdata"]
  exclude_file = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  full_bin = ""
  include_dir = []
  include_ext = ["go", "tpl", "tmpl", "html"]
  kill_delay = "0s"
  log = "build-errors.log"
  send_interrupt = false
  stop_on_error = true

[log]
  time = false

[misc]
  clean_on_exit = false
```

On Linux/macOS, change `bin` and the `-o` target from `./tmp/main.exe` to `./tmp/main`. Then:

```bash
air
# [INFO] hot reload running. edit any .go file and save to rebuild.
```

Save any file under `internal/` or `cmd/` — Air rebuilds and restarts in ~1 second.

---

## 5. Common mistakes & gotchas

- **Forgetting `go mod tidy`** after deleting an import. Your `go.sum` carries dead entries. Run `go mod tidy` before every commit; CI should fail if it produces a diff.
- **Running `gin.Default()` in production without setting `GIN_MODE=release`** — Gin prints `[GIN-debug] [WARNING] Running in "debug" mode.` to stderr on every boot, and the per-request logger is unstructured. Switch to release and your own logger.
- **Putting code in `pkg/`** when nothing outside the module will ever import it. Prefer `internal/`. `pkg/` is for libraries other repos consume.
- **Calling `r.Run` and trying to do graceful shutdown** — `r.Run` doesn't let you intercept signals. For real shutdown handling use `http.Server` directly (module 15).
- **Using `GOPATH` instead of modules.** Modules have been the default since Go 1.16. If a tutorial says "put your code in `~/go/src/...`", it is outdated.
- **Treating `gin.H` as if it were special.** `gin.H` is just `type H map[string]any`. You can return any value that JSON-marshals: a struct, a slice, anything. Use real structs for any response that has more than two fields — they document the API.
- **Running `go install github.com/gin-gonic/gin`.** Gin is a library, not a binary. Use `go get`. `go install` is for tools (Air, sqlc, golangci-lint, etc.).

---

## 🎯 Key Takeaways

1. **`gin.Engine` is an `http.Handler`** — Gin doesn't replace `net/http`; it sits on top. That single fact unlocks `httptest`, OTel middleware, graceful shutdown via `http.Server`, and any future framework swap.
2. **Use `gin.New()` + your own logger in production.** `gin.Default()` is for getting started. Senior engineers ship structured logs to stdout for the platform to collect.
3. **`internal/` is non-negotiable for production services.** It is the only Go-level boundary enforcement you get. Use it to keep `service/` ignorant of HTTP types.
4. **Air is worth the five-minute setup.** Hot reload doubles the speed of the learning loop; manual `go run` cycles destroy flow.
5. **Set `GIN_MODE=release` in production, always.** Forgetting this is a tell — it shows up in interview takehomes and in production logs and signals "first Gin project."

*← [00 — Roadmap](./00_roadmap.md) | [02 — Routing →](./02_routing.md)*
