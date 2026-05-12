# 00 — Gin Deep-Dive Roadmap

> **Goal:** Take a developer who knows Go fundamentals and turn them into an engineer who can design, build, test, and operate production HTTP services with Gin v1.10+ on Go 1.22+.

---

## Why Gin?

Gin is a **thin, opinionated layer over `net/http`** that gives you a fast radix-tree router, a pooled request context, a middleware chain, and a handful of response helpers. It does **not** ship an ORM, a DI container, or a config system — that is by design. You compose those pieces yourself, which is exactly the skillset a senior Go backend engineer is paid for.

Gin's selling points:

- **Performance.** The router is a radix tree; routing cost is `O(path length)`, not `O(routes)`.
- **Familiarity.** Handlers are `func(*gin.Context)` — one parameter, no return value, no generics gymnastics.
- **Ecosystem.** The largest Go web-framework ecosystem (validators, JWT, OpenAPI, OTel middleware).
- **Stays close to stdlib.** A `gin.Engine` is an `http.Handler`. You can always drop down.

---

## Module Map

| #  | File | Topic | What you walk away with |
|----|------|-------|-------------------------|
| 00 | `00_roadmap.md` | This file | Mental models + study plan |
| 01 | `01_setup_and_first_server.md` | Setup & first server | Go modules, `go.mod`, Air hot reload, project layout |
| 02 | `02_routing.md` | Routing | Path/query params, groups, route tree internals |
| 03 | `03_context_deep_dive.md` | `gin.Context` | Request/response helpers, `Set`/`Get`, `Abort`, pooling |
| 04 | `04_request_handling.md` | Request binding | JSON/form/query/URI/header binding, validator/v10, uploads |
| 05 | `05_response_handling.md` | Responses | JSON/XML/YAML/HTML/Stream, headers, redirects, negotiation |
| 06 | `06_middleware.md` | Middleware | Writing it, ordering, `Next` vs `Abort`, scopes |
| 07 | `07_error_handling.md` | Errors | `c.Error`, central error middleware, panic recovery |
| 08 | `08_templates_and_static.md` | Templates & static | `html/template`, multi-template, `embed.FS` |
| 09 | `09_database_integration.md` | Databases | `database/sql` + `pgx`, `sqlc`, GORM, request-scoped tx |
| 10 | `10_auth.md` | Auth | JWT, OAuth2, sessions, RBAC, cookies |
| 11 | `11_testing.md` | Testing | `httptest`, table tests, mocks, Testcontainers-go |
| 12 | `12_concurrency.md` | Concurrency | `c.Copy()`, workers, `context.Context`, graceful shutdown |
| 13 | `13_observability.md` | Observability | `slog`/`zap`, Prometheus, OTel, request IDs, health |
| 14 | `14_api_design.md` | API design | REST, OpenAPI (swaggo/huma), versioning, pagination, rate limiting |
| 15 | `15_production.md` | Production | Static binaries, distroless Docker, TLS, viper, pitfalls |

Total: **16 files**.

---

## Timeline (one module per day ≈ two weeks)

| Week | Days | Focus |
|------|------|-------|
| 1    | 1–2  | Modules 01–02: get a server running and routing solid |
| 1    | 3–5  | Modules 03–05: master the request/response cycle |
| 1    | 6–7  | Modules 06–07: middleware and errors — the load-bearing pieces |
| 2    | 8–10 | Modules 08–10: templates, DB, auth — the practical stack |
| 2    | 11–13 | Modules 11–13: tests, concurrency, observability — the senior-engineer stuff |
| 2    | 14    | Modules 14–15: API design polish and production hardening |

Total: **~14 days at 1.5–2 hr/day**, ~28 hours of focused work. Realistic for a working developer.

---

## Prerequisites

You need solid Go fundamentals. Specifically:

- **Functions, methods, structs, interfaces, embedding** — see [`../golang/00_roadmap.md`](../golang/00_roadmap.md). Gin handlers are functions; middleware composition leans on closures.
- **Goroutines and channels** — module 12 will burn you otherwise.
- **`error` and error wrapping (`errors.Is/As/Unwrap`)** — module 07 builds on this.
- **HTTP basics** — methods, status codes, headers, cookies, content negotiation. If "415 Unsupported Media Type" doesn't ring a bell, read MDN's HTTP overview first.
- **SQL basics** — joins, transactions, indexes. Helpful for module 09. See [`../mysql/00_roadmap.md`](../mysql/00_roadmap.md).
- **Optional: Docker** — module 15 packages a service. See [`../docker/00_roadmap.md`](../docker/00_roadmap.md).

If any of those feel shaky, stop and shore them up. Gin will not paper over weak Go fundamentals — it will expose them.

---

## Core Mental Models

Internalize these six and the rest of the course is mechanical.

### 1. Gin is a thin layer over `net/http`

`gin.Engine` implements `http.Handler`. You can plug it into `http.Server`, `httptest.NewServer`, or any HTTP-aware library (rate-limiters, proxies, OTel). When Gin's helpers don't fit, drop down to `http.ResponseWriter` and `*http.Request` — both live on `c.Writer` and `c.Request`. Nothing is hidden.

### 2. `gin.Context` is request-scoped state + helpers

Each request gets a `*gin.Context`. It carries the request, the response writer, a key/value map (`c.Set`/`c.Get`), the handler chain, and any errors collected via `c.Error`. **It is pooled** (`sync.Pool`) and recycled after the response is written. The two consequences that bite people:

- Don't keep references to `*gin.Context` past the handler. If you need it in a goroutine, **call `c.Copy()` first**.
- Don't stash large objects in `c.Keys` — they survive in the pool and bloat memory.

### 3. Middleware is just a function in a chain

A middleware is a `gin.HandlerFunc` — same signature as a handler. Gin stores handlers in a slice per route. `c.Next()` walks forward; `c.Abort()` flips a flag so the loop stops calling subsequent handlers. There is no magic — it is a for-loop over a `[]HandlerFunc`.

```go
type HandlerFunc func(*Context)

// Conceptually:
for i := c.index; i < len(c.handlers) && !c.aborted; i++ {
    c.handlers[i](c)
}
```

This means "return early" in middleware is **`c.Abort()` followed by `return`**, not just `return`. Forgetting `Abort` is the #1 middleware bug.

### 4. Binding uses struct tags as the contract

`c.ShouldBindJSON(&req)`, `c.ShouldBindQuery(&req)`, `c.ShouldBindUri(&req)` — each looks at struct tags (`json:"..."`, `form:"..."`, `uri:"..."`, `binding:"..."`) and the `Content-Type` header to decide how to populate the struct. Validation rules live in the `binding:"..."` tag and are evaluated by `go-playground/validator/v10`. **The struct definition is the API contract.** Generate OpenAPI from it; never let it drift.

### 5. The radix-tree router is why routing is O(path length)

Gin's router (forked from `httprouter`) stores routes as a prefix tree (radix tree), one tree per HTTP method. Matching `/api/v1/users/42/posts/7` is a walk down the tree — its cost depends on the path length, not on how many routes are registered. This is why you can register thousands of routes without slowdown, and why path-conflict errors at startup (`'/users/:id' conflicts with '/users/me'`) come from tree-construction rules.

### 6. `c.Next` / `c.Abort` control flow, they are not `return`

A middleware that wants logic *after* the handler runs (timing, response logging, panic catching) calls `c.Next()` in the middle of itself:

```go
func Timing() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        c.Next()                    // run the rest of the chain
        log.Printf("%s took %v", c.FullPath(), time.Since(start))
    }
}
```

This is the most powerful pattern Gin offers and the one beginners miss.

---

## Alternative Frameworks — When NOT to use Gin

You should know the alternatives because in interviews and design reviews this comes up:

| Framework | When it makes sense |
|-----------|---------------------|
| **stdlib `net/http` + `chi`** | Small services, libraries, or when you want zero non-stdlib dependencies on the request path. Go 1.22+ added pattern routing (`mux.HandleFunc("GET /users/{id}", ...)`) which closes much of the gap. |
| **Echo** | Very similar to Gin, slightly different API. Pick based on team preference — they are interchangeable. |
| **Fiber** | Built on `fasthttp`, not `net/http`. Faster microbenchmarks but **incompatible with the standard `http.Handler` ecosystem** (most middleware, OTel, `httptest`, etc., don't work). Pick only if you've measured a bottleneck and accept the ecosystem cost. |
| **Huma** | Code-first OpenAPI framework that can run on top of Gin/Chi/Fiber. Great choice if OpenAPI fidelity matters more than raw speed. |
| **gRPC / Connect** | Internal service-to-service traffic. Not a Gin alternative — they solve a different problem. |

**Gin remains the safe default** for an HTTP/JSON service today: large ecosystem, stdlib-compatible, well-known. This course is on Gin, but the concepts (radix routers, middleware chains, binding via tags) port directly to Echo and Chi.

---

## External Resources

- **Official Gin docs** — <https://gin-gonic.com/docs/> — keep this open while you work through modules.
- **validator/v10 docs** — <https://pkg.go.dev/github.com/go-playground/validator/v10> — the validation tag reference you will use daily.
- **Go by Example** — <https://gobyexample.com/> — quick refresher for any stdlib API.
- **"Let's Go" and "Let's Go Further" by Alex Edwards** — <https://lets-go.alexedwards.net/> — uses stdlib, not Gin, but the architectural patterns (middleware composition, error handling, request flow) are directly applicable and arguably the best Go web book series available.
- **Go blog: HTTP/2 and net/http** — <https://go.dev/blog/> — search for "net/http" — understanding the server internals demystifies Gin.
- **`go-playground/validator` issues** — when you hit a weird validation edge case, the issue tracker has answers.
- **OpenTelemetry Go contrib** — <https://github.com/open-telemetry/opentelemetry-go-contrib> — for the Gin OTel middleware used in module 13.

---

## How to study this course

1. **Type the code.** Do not copy-paste. Muscle memory matters.
2. **Run `curl` against every endpoint.** Each module has the commands.
3. **Break things on purpose.** Send malformed JSON, omit required fields, hit `/users/abc` when it expects an int. See how Gin responds; that's the only way you'll know what to fix in production.
4. **At the end of each module, write one paragraph in your own words** explaining the headline concept. If you can't, re-read the module.
5. **Don't skip module 12.** Concurrency bugs in Gin handlers are the most common production incident in Go services and they almost all trace back to misusing `c` from a goroutine.

---

## Closing line

If you finish all 15 modules, type every example, and write one realistic project of your own (suggested: a small JSON API for a domain you understand — bookmarks, task tracker, expense log — with auth, tests, structured logs, and a Dockerfile), you will be operating at **mid-to-senior Go backend** level on HTTP services. That is a real, durable, market-rate skill. Treat this fortnight as an investment, not a checkbox.

*next → [01 — Setup and First Server](./01_setup_and_first_server.md)*
