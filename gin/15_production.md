# 15 — Production

> **Goal:** Ship a Gin service the way professionals do — static binary, distroless Docker image, TLS, config via env/viper, behind a reverse proxy, with graceful shutdown and a checklist for the pitfalls that bite real services.

---

## 1. Building a static binary — mental model + working code

Go compiles to a single statically-linked binary by default. That property is the entire reason Go is so popular for ops. For Gin, no special build is needed:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
go build -trimpath -ldflags="-s -w" -o ./bin/api ./cmd/api
```

What each flag does:

- **`CGO_ENABLED=0`** — disables cgo. Without cgo your binary is *truly* static (no glibc dependency); the result runs on `scratch` or `distroless:static`.
- **`GOOS=linux GOARCH=amd64`** — cross-compile from your laptop (macOS/Windows) for the target. Switch `arm64` if your platform is ARM.
- **`-trimpath`** — strip filesystem paths from the binary. Smaller, less leaky, reproducible.
- **`-ldflags="-s -w"`** — drop the symbol table and DWARF debug info. Roughly halves the binary size. Stack traces in panics still show function names; you only lose debugger-quality symbols.

For a typical Gin service the result is a 15–25 MB binary you copy into a Docker image.

### Version stamping

You usually want `--version` to print git SHA + build time:

```go
// internal/version/version.go
package version

var (
    Commit = "unknown"   // set via -ldflags
    Time   = "unknown"
    Tag    = "dev"
)
```

```bash
COMMIT=$(git rev-parse --short HEAD)
TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
go build -ldflags="-s -w \
  -X 'github.com/you/hello-gin/internal/version.Commit=$COMMIT' \
  -X 'github.com/you/hello-gin/internal/version.Time=$TIME' \
  -X 'github.com/you/hello-gin/internal/version.Tag=$(git describe --tags --always)'" \
  -o ./bin/api ./cmd/api
```

Expose at `/version`:

```go
r.GET("/version", func(c *gin.Context) {
    c.JSON(200, gin.H{"commit": version.Commit, "time": version.Time, "tag": version.Tag})
})
```

Pays for itself the first time you ask "what's actually deployed in staging?"

---

## 2. Docker — distroless multi-stage build

The canonical Dockerfile for a Gin service:

```dockerfile
# Dockerfile
# syntax=docker/dockerfile:1.7

# ---- build stage ----
FROM golang:1.22-alpine AS build
WORKDIR /src

# Cache dependencies first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG COMMIT=unknown
ARG TIME=unknown
ARG TAG=dev

ENV CGO_ENABLED=0 GOOS=linux

RUN go build -trimpath \
    -ldflags="-s -w \
      -X 'github.com/you/hello-gin/internal/version.Commit=${COMMIT}' \
      -X 'github.com/you/hello-gin/internal/version.Time=${TIME}' \
      -X 'github.com/you/hello-gin/internal/version.Tag=${TAG}'" \
    -o /out/api ./cmd/api

# ---- runtime stage ----
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/api /api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
```

Key choices:

- **`distroless/static:nonroot`** — no shell, no `ls`, no `cat`, no package manager, no userspace at all except your binary, CA certs, and timezone files. Tiny attack surface; ~2 MB base. The `nonroot` variant runs as UID 65532 by default.
- **Multi-stage build** — the build image (`golang:1.22-alpine`) is heavy but discarded. The runtime image only carries the binary.
- **No shell means `docker exec -it ... sh` doesn't work.** That's the trade-off. In return, an RCE in your service has nowhere to pivot.
- **`USER nonroot`** — never run as root inside a container.

Build and run:

```bash
docker build -t hello-gin:dev \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  --build-arg TAG=$(git describe --tags --always) .

docker run --rm -p 8080:8080 -e GIN_MODE=release hello-gin:dev
```

Image size will be in the 10–30 MB range. That's the bar.

### Health-check in Kubernetes

```yaml
livenessProbe:
  httpGet: { path: /healthz, port: 8080 }
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet: { path: /readyz, port: 8080 }
  initialDelaySeconds: 2
  periodSeconds: 5
```

With `terminationGracePeriodSeconds: 30` and the graceful shutdown from module 12, you get clean rolling deploys.

---

## 3. Config, TLS, reverse proxies, graceful shutdown

### Config — start simple

For most Gin services, **env vars + a tiny loader** is enough. Don't reach for `viper` unless you actually need its features (file watching, multi-source).

```go
// internal/config/config.go
package config

import (
    "fmt"
    "os"
    "strconv"
    "time"
)

type Config struct {
    Env             string
    Port            string
    DatabaseURL     string
    JWTSecret       string
    JWTTTL          time.Duration
    ShutdownTimeout time.Duration
    LogLevel        string
}

func Load() (*Config, error) {
    cfg := &Config{
        Env:             env("ENV", "dev"),
        Port:            env("PORT", "8080"),
        DatabaseURL:     env("DATABASE_URL", ""),
        JWTSecret:       env("JWT_SECRET", ""),
        JWTTTL:          mustDuration("JWT_TTL", "15m"),
        ShutdownTimeout: mustDuration("SHUTDOWN_TIMEOUT", "25s"),
        LogLevel:        env("LOG_LEVEL", "info"),
    }
    if cfg.DatabaseURL == "" {
        return nil, fmt.Errorf("DATABASE_URL required")
    }
    if cfg.JWTSecret == "" {
        return nil, fmt.Errorf("JWT_SECRET required")
    }
    return cfg, nil
}

func env(k, def string) string {
    if v := os.Getenv(k); v != "" {
        return v
    }
    return def
}
func mustDuration(k, def string) time.Duration {
    s := env(k, def)
    d, err := time.ParseDuration(s)
    if err != nil {
        panic(fmt.Sprintf("invalid duration %s=%s", k, s))
    }
    return d
}
func mustInt(k, def string) int {
    s := env(k, def)
    n, err := strconv.Atoi(s)
    if err != nil {
        panic(fmt.Sprintf("invalid int %s=%s", k, s))
    }
    return n
}
```

This loader is 30 lines, has no dependencies, and fails fast on missing required values. That's the senior bar.

### When to reach for `viper`

```bash
go get github.com/spf13/viper
```

`viper` shines when you genuinely need:

- Multiple config sources (env + file + Consul/etcd).
- Live config reload.
- Hierarchical config with overrides.

For 90% of Gin services, the 30-line env loader above is enough. Code that reads like "we use viper because everyone does" is a code smell.

### TLS — terminate at the LB or in-app?

**Production answer: terminate TLS at your load balancer / ingress.** Cloud LBs (ALB, GCLB), Cloudflare, nginx — all of them terminate TLS, manage certs, do OCSP stapling. Your Go service speaks plain HTTP on the pod network.

When you do need in-app TLS (single-binary deploys, internal mTLS):

```go
srv := &http.Server{
    Addr:    ":8443",
    Handler: r,
    TLSConfig: &tls.Config{
        MinVersion: tls.VersionTLS12,    // baseline 2026
        // For mTLS:
        // ClientAuth: tls.RequireAndVerifyClientCert,
        // ClientCAs:  clientCAs,
    },
    // ... timeouts as before
}
if err := srv.ListenAndServeTLS("server.crt", "server.key"); err != nil { ... }
```

For dev with TLS, `mkcert` is the cleanest local CA tool. For production cert automation, use Let's Encrypt via `cert-manager` (Kubernetes) or `acme.sh`.

### Reverse proxy — what to configure

Behind nginx, Envoy, Caddy, or a cloud LB, set up:

- **`X-Forwarded-For` / `X-Real-IP`** — forwarded by the LB; Gin reads via `c.ClientIP()`. Configure `r.SetTrustedProxies([]string{"10.0.0.0/8"})` to the LB CIDR. Without this, attackers spoof their IP.
- **`X-Forwarded-Proto`** — tells your app whether the original request was HTTPS. Necessary for setting `Secure` cookies correctly behind an HTTP-terminating LB.
- **Buffering off for SSE/streams** — `X-Accel-Buffering: no` (nginx) or set the LB to streaming mode. Your CSV export will otherwise buffer the whole 200 MB before sending.
- **Timeouts coordinated.** If your LB has a 60s idle timeout and your Go `IdleTimeout: 120s`, the LB will drop kept-alive connections out from under you. Set Go's `IdleTimeout` slightly *higher* than the LB's so the LB always closes first.

### Production `http.Server` settings

Already shown in modules 12 and 13, but repeating because this is **the** difference between "tutorial" and "production":

```go
srv := &http.Server{
    Addr:              ":" + cfg.Port,
    Handler:           r,
    ReadHeaderTimeout: 5 * time.Second,    // anti-slowloris: header must arrive in 5s
    ReadTimeout:       15 * time.Second,   // full request must arrive in 15s
    WriteTimeout:      30 * time.Second,   // response must be written in 30s
    IdleTimeout:       120 * time.Second,  // keep-alive idle
    MaxHeaderBytes:    1 << 20,            // 1 MiB
}
```

Without `ReadHeaderTimeout`, a slowloris (one byte per 30 seconds) ties up your goroutines.

### Graceful shutdown (recap from module 12)

```go
rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()
// start server in goroutine
<-rootCtx.Done()
shutCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
defer cancel()
_ = srv.Shutdown(shutCtx)
```

Kubernetes terminationGracePeriod must exceed your `ShutdownTimeout`, or pods get SIGKILL'd mid-request.

---

## 4. Practical application — end-to-end production wiring

A complete `main.go` you could reasonably deploy.

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
    "github.com/prometheus/client_golang/prometheus/promhttp"

    "github.com/you/hello-gin/internal/config"
    "github.com/you/hello-gin/internal/http/middleware"
    "github.com/you/hello-gin/internal/http/router"
    "github.com/you/hello-gin/internal/obs"
    "github.com/you/hello-gin/internal/store"
    "github.com/you/hello-gin/internal/version"
)

func main() {
    cfg, err := config.Load()
    if err != nil {
        // Use plain log here — slog not yet initialized.
        os.Stderr.WriteString("config error: " + err.Error() + "\n")
        os.Exit(2)
    }

    level := slog.LevelInfo
    if cfg.LogLevel == "debug" {
        level = slog.LevelDebug
    }
    log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
    slog.SetDefault(log)

    log.Info("starting", "env", cfg.Env, "commit", version.Commit, "tag", version.Tag)

    if cfg.Env == "prod" {
        gin.SetMode(gin.ReleaseMode)
    }

    rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    // Tracing (optional; init best-effort)
    shutdownTrace, err := obs.InitTracer(rootCtx, "hello-gin", version.Tag)
    if err != nil {
        log.Warn("tracer init failed", "err", err)
    } else {
        defer shutdownTrace(context.Background())
    }

    db, err := store.Open(rootCtx, cfg.DatabaseURL)
    if err != nil {
        log.Error("db open", "err", err); os.Exit(1)
    }
    defer db.Close()

    r := router.New(router.Deps{
        Log:       log,
        DB:        db,
        JWTSecret: []byte(cfg.JWTSecret),
        JWTTTL:    cfg.JWTTTL,
    })

    // Admin endpoints (metrics, version, health)
    r.GET("/version", func(c *gin.Context) {
        c.JSON(200, gin.H{"commit": version.Commit, "time": version.Time, "tag": version.Tag})
    })
    r.GET("/metrics", gin.WrapH(promhttp.Handler()))
    r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
    r.GET("/readyz", func(c *gin.Context) {
        ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
        defer cancel()
        if err := db.PingContext(ctx); err != nil {
            c.JSON(503, gin.H{"status": "degraded", "db": err.Error()})
            return
        }
        c.JSON(200, gin.H{"status": "ready"})
    })

    if err := r.SetTrustedProxies([]string{"10.0.0.0/8", "172.16.0.0/12"}); err != nil {
        log.Warn("trusted proxies", "err", err)
    }

    srv := &http.Server{
        Addr:              ":" + cfg.Port,
        Handler:           r,
        ReadHeaderTimeout: 5 * time.Second,
        ReadTimeout:       15 * time.Second,
        WriteTimeout:      30 * time.Second,
        IdleTimeout:       120 * time.Second,
        MaxHeaderBytes:    1 << 20,
    }

    serverErr := make(chan error, 1)
    go func() {
        log.Info("listening", "addr", srv.Addr)
        if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            serverErr <- err
        }
        close(serverErr)
    }()

    select {
    case <-rootCtx.Done():
        log.Info("shutdown signal received")
    case err := <-serverErr:
        if err != nil {
            log.Error("server crashed", "err", err); os.Exit(1)
        }
    }

    log.Info("graceful shutdown starting", "timeout", cfg.ShutdownTimeout)
    shutCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
    defer cancel()
    if err := srv.Shutdown(shutCtx); err != nil {
        log.Error("graceful shutdown failed", "err", err)
    }
    _ = middleware.RequestIDKey // ensure import; remove in real code
    log.Info("bye")
}
```

Run it:

```bash
DATABASE_URL=postgres://... \
JWT_SECRET=super-secret-from-vault \
GIN_MODE=release \
LOG_LEVEL=info \
./bin/api
```

Deploy: build the Docker image, push it, run a Helm chart / Kustomize / whatever your platform uses. Liveness, readiness, metrics, version, request IDs, traces, structured logs, graceful shutdown — all there.

### A short production-readiness checklist

Before any service goes to prod, walk through:

- [ ] `GIN_MODE=release` in prod, `gin.SetMode(gin.ReleaseMode)` early in `main`.
- [ ] Explicit `http.Server` with all four timeouts set.
- [ ] Graceful shutdown wired to SIGTERM/SIGINT.
- [ ] `/healthz`, `/readyz`, `/metrics`, `/version` endpoints.
- [ ] `slog`/`zap` JSON logs to stdout, no `fmt.Println` anywhere on the hot path.
- [ ] `request_id` middleware first; logger reads it.
- [ ] `Recovery` middleware with structured log + stack trace.
- [ ] `SetTrustedProxies` configured to the actual LB CIDR.
- [ ] CORS configured with explicit origins, no `AllowAllOrigins` with credentials.
- [ ] Cookies are `Secure`, `HttpOnly`, with a `SameSite` policy.
- [ ] DB connection pool tuned (`MaxOpenConns`, lifetimes).
- [ ] Every DB and HTTP client call uses `c.Request.Context()`.
- [ ] Every spawned goroutine has `c.Copy()` (if it needs context state) + `defer recover()`.
- [ ] `http.MaxBytesReader` on file-upload routes.
- [ ] Rate limiting on auth + signup endpoints at minimum.
- [ ] No secrets in code; all from env/secret manager.
- [ ] Image is distroless `nonroot`.
- [ ] Tests with `-race` and a non-empty coverage floor in CI.
- [ ] `golangci-lint`, `go vet`, `staticcheck` clean.
- [ ] Build is reproducible (`-trimpath`, pinned Go version, locked deps via `go.sum`).
- [ ] `terminationGracePeriodSeconds` > `ShutdownTimeout`.
- [ ] Liveness and readiness probes pointed at the right endpoints with sane timings.

Print this. Hand it to a teammate when they're about to ship their first Gin service.

---

## 5. Common mistakes & gotchas

- **Deploying with `gin.DebugMode`.** Verbose logs to stderr on every request, plus a startup warning. Trivial to forget; immediately obvious to a reviewer.
- **`r.Run(":8080")` instead of an explicit `http.Server`.** No timeouts, no graceful shutdown — slowloris, leaked goroutines on disconnect, half-served requests on restart.
- **Forgetting `CGO_ENABLED=0` for distroless.** Image builds, container fails to start with `no such file: ld-linux-x86-64.so.2`. Set `CGO_ENABLED=0` or use a libc-bearing base.
- **Running as root in the container.** Easy to do, easy to miss. Always `USER nonroot:nonroot` (or your own non-root UID). Many corporate clusters refuse to run root pods.
- **Secrets in `Dockerfile` ARG/ENV.** They're baked into image layers visible to anyone with pull access. Inject at runtime via env from a secret manager.
- **No `SetTrustedProxies` behind an LB.** `c.ClientIP()` returns the LB's IP, or worse, anything the attacker sets in `X-Forwarded-For`. Configure the trusted CIDR.
- **`/metrics` exposed without auth on a public port.** Leaks internal metrics, version strings, sometimes labels with usernames. Either move to a separate port not exposed via the public LB, or basic-auth it.
- **Coordinated shutdown timeouts wrong.** `ShutdownTimeout=30s` but `terminationGracePeriodSeconds=10s` → K8s SIGKILLs mid-drain. Or the reverse: K8s waits forever for a pod that's wedged. Pick numbers and document them.
- **Logging to a file inside the container.** 12-factor: logs to stdout. The platform (K8s, ECS, Nomad) collects them. Files inside containers get lost.
- **`panic` in `init()`.** `init()` runs before `main`; a panic there crashes with a less-useful trace and zero observability. Push validation to `main`'s startup.
- **Building with whatever Go version is on the dev machine.** Pin the version in `go.mod`'s `toolchain` directive, in Dockerfile, in CI. Reproducibility starts here.

---

## 🎯 Key Takeaways

1. **Static binary + distroless + multi-stage Dockerfile + `nonroot` user** is the canonical Go web deploy. A 15 MB image, no shell, no surface area, no surprises. Anything more elaborate is justified by a specific need.
2. **`http.Server` with timeouts and `Shutdown(ctx)` is non-negotiable.** `r.Run` is tutorial code. Production Gin always wraps `gin.Engine` in an explicit `http.Server` and orchestrates shutdown around it.
3. **TLS terminates at the LB; the app speaks plain HTTP on the pod network.** Configure `SetTrustedProxies` to the LB's CIDR or `c.ClientIP()` is untrustworthy.
4. **Start with env vars + a 30-line loader.** Reach for `viper` only when you have a concrete need it solves. Most Gin services don't.
5. **The production checklist is the senior delta.** Anyone can write `r.GET("/ping", ...)`. Knowing — and consistently shipping — `GIN_MODE=release`, distroless `nonroot`, four `http.Server` timeouts, graceful shutdown coordinated with `terminationGracePeriod`, structured logs with request IDs, trusted-proxy config, and rate-limited auth endpoints is what makes the service operable at 3 a.m. That's the bar this whole course was building toward.

---

## Closing — what to do next

You've finished the course. Now:

1. **Build the project from the roadmap.** A small JSON API for a domain you understand, with auth, tests, structured logs, and a Dockerfile. Don't skip the Dockerfile.
2. **Read Alex Edwards' "Let's Go" once cover to cover.** It's stdlib `net/http`, not Gin, but the architectural patterns are the same and it reinforces every concept here.
3. **Read the Gin source.** Specifically `context.go`, `tree.go`, and `gin.go`. ~3,000 lines total. Knowing what's under the hood removes all remaining mystery.
4. **Take a real production incident and trace it back to a concept in this course.** Almost every Go HTTP incident maps to one of: missing `c.Copy()`, missing `ctx`, missing timeout, missing `SetTrustedProxies`, panic in a goroutine. You've now seen all five.

Type the code, ship a real project, read the source, debug a real outage. After those four steps, you are operating at a senior Go backend level on HTTP services. That is real, durable, marketable skill — and worth the two weeks.

*← [14 — API Design with Gin](./14_api_design.md) | [00 — Roadmap](./00_roadmap.md)*
