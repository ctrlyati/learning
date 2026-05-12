# 11 — Testing

> **Goal:** Test Gin handlers, middleware, and full HTTP flows the idiomatic Go way — `httptest`, table-driven tests, dependency injection for mocks, and `testcontainers-go` for real-DB integration tests.

---

## 1. Testing handlers — mental model + working code

`gin.Engine` is an `http.Handler`. That single fact means you test it the same way you test any stdlib HTTP handler: with `httptest.NewRecorder` and `http.NewRequest`, or with `httptest.NewServer` for round-trip integration tests.

### A minimal handler test

```go
// internal/http/handlers/ping_test.go
package handlers_test

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
)

func TestPing(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.GET("/ping", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"message": "pong"})
    })

    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/ping", nil)
    r.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)
    assert.JSONEq(t, `{"message":"pong"}`, w.Body.String())
}
```

Three primitives:

- `gin.SetMode(gin.TestMode)` — silences `[GIN-debug]` log spam during tests.
- `httptest.NewRecorder()` — an in-memory `http.ResponseWriter` you can inspect afterward (`w.Code`, `w.Body`, `w.Header()`).
- `r.ServeHTTP(w, req)` — runs the whole Gin stack in-process, no socket.

No `r.Run`, no `:8080`, no timing flakes. Tests run in microseconds.

### Why `testify`?

Stdlib testing works; `github.com/stretchr/testify/assert` and `/require` make assertions readable. `require.NoError(t, err)` halts the test on failure; `assert.Equal(...)` continues. Industry-standard, low controversy.

---

## 2. How the test harness works

`gin.Engine` doesn't care if it's running inside `http.Server` or inside a unit test. `ServeHTTP(w, r)` is the entry point either way. The same router walk, the same middleware chain, the same `*gin.Context` pool. Your test:

1. Builds the engine the same way `main` does (route registration is the only code under test that touches Gin).
2. Constructs a `*http.Request` with method, URL, headers, body.
3. Constructs a `*httptest.ResponseRecorder`.
4. Calls `engine.ServeHTTP(w, r)`.
5. Inspects `w.Code`, `w.Body`, `w.Header()` — and any side effects (DB writes, mock calls).

`httptest.NewRequest` is the convenience constructor (panics on impossible inputs — fine for tests). `http.NewRequest` is the one you'd use in production code.

### `httptest.NewServer` — when you need a real socket

Some libraries (HTTP clients with their own transports, retry-aware clients) need a real network endpoint:

```go
srv := httptest.NewServer(r)
defer srv.Close()

resp, err := http.Get(srv.URL + "/ping")
```

Uses an ephemeral port. Tests must be careful not to leak servers between subtests.

---

## 3. Variations — table tests, mocking, integration, and request bodies

### Table-driven tests

The Go idiom. Every test function becomes a table of cases.

```go
func TestGetUser(t *testing.T) {
    gin.SetMode(gin.TestMode)

    cases := []struct {
        name       string
        id         string
        wantStatus int
        wantBody   string
    }{
        {"happy",       "1",   http.StatusOK,              `{"id":1,"email":"y@x.com","name":"yati"}`},
        {"not found",   "999", http.StatusNotFound,        `{"error":"NOT_FOUND","message":"user not found"}`},
        {"bad id",      "abc", http.StatusBadRequest,      `{"error":"BAD_REQUEST","message":"id must be an integer"}`},
    }
    for _, tc := range cases {
        tc := tc
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            r := newTestRouter()        // builds engine with in-memory store seeded with id=1
            w := httptest.NewRecorder()
            req := httptest.NewRequest("GET", "/api/v1/users/"+tc.id, nil)
            r.ServeHTTP(w, req)

            assert.Equal(t, tc.wantStatus, w.Code)
            assert.JSONEq(t, tc.wantBody, w.Body.String())
        })
    }
}
```

Notes:

- `tc := tc` shadowing was required for parallel tests pre-Go 1.22. With Go 1.22's new loop-variable semantics it's no longer needed — but it's harmless and many codebases still write it for clarity.
- `t.Parallel()` runs subtests concurrently. Make sure your `newTestRouter` returns an *independent* engine + store per call.

### Posting a JSON body

```go
body := bytes.NewBufferString(`{"email":"y@x.com","name":"yati"}`)
req := httptest.NewRequest("POST", "/api/v1/users", body)
req.Header.Set("Content-Type", "application/json")
```

For typed bodies, marshal in the test:

```go
b, _ := json.Marshal(map[string]any{"email": "y@x.com", "name": "yati"})
req := httptest.NewRequest("POST", "/api/v1/users", bytes.NewReader(b))
req.Header.Set("Content-Type", "application/json")
```

### Mocking dependencies

The reason your service takes an interface (`UserStore`) is that you can pass a mock in tests:

```go
type fakeStore struct {
    users map[int64]*service.User
}

func (f *fakeStore) GetByID(_ context.Context, id int64) (*service.User, error) {
    u, ok := f.users[id]
    if !ok {
        return nil, service.ErrUserNotFound
    }
    return u, nil
}

func (f *fakeStore) Create(_ context.Context, u *service.User) error {
    for _, x := range f.users {
        if x.Email == u.Email {
            return service.ErrEmailTaken
        }
    }
    u.ID = int64(len(f.users) + 1)
    f.users[u.ID] = u
    return nil
}

func newTestRouter() *gin.Engine {
    fs := &fakeStore{users: map[int64]*service.User{
        1: {ID: 1, Email: "y@x.com", Name: "yati"},
    }}
    svc := &service.UserService{Store: fs}
    h := &handlers.UserHandler{Users: svc}

    r := gin.New()
    log := slog.New(slog.NewTextHandler(io.Discard, nil))
    r.Use(middleware.RequestID())
    api := r.Group("/api/v1")
    api.GET("/users/:id", ginx.Wrap(log, h.Get))
    api.POST("/users",    ginx.Wrap(log, h.Create))
    return r
}
```

Hand-rolled fakes are usually faster to maintain than a generated mock (`gomock`/`mockery`). For interfaces with 10+ methods, a generator is worth it.

### Testing middleware in isolation

Middleware is also a `gin.HandlerFunc`. Test it on a minimal engine:

```go
func TestRequireJWT(t *testing.T) {
    gin.SetMode(gin.TestMode)
    secret := []byte("test")
    r := gin.New()
    r.Use(middleware.RequireJWT(secret))
    r.GET("/whoami", func(c *gin.Context) {
        c.JSON(200, gin.H{"id": c.GetInt64("user_id")})
    })

    t.Run("missing header", func(t *testing.T) {
        w := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/whoami", nil)
        r.ServeHTTP(w, req)
        assert.Equal(t, 401, w.Code)
    })

    t.Run("valid token", func(t *testing.T) {
        tok, _ := middleware.IssueToken(secret, 42, "user", time.Minute)
        w := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/whoami", nil)
        req.Header.Set("Authorization", "Bearer "+tok)
        r.ServeHTTP(w, req)
        assert.Equal(t, 200, w.Code)
        assert.JSONEq(t, `{"id":42}`, w.Body.String())
    })
}
```

### Integration tests with `testcontainers-go`

For tests that need a real Postgres (testing the actual SQL, the actual migrations), spin one up per test run:

```bash
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres
```

```go
// internal/store/users_integration_test.go
//go:build integration

package store_test

import (
    "context"
    "database/sql"
    "path/filepath"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
    tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"

    "github.com/you/hello-gin/internal/service"
    "github.com/you/hello-gin/internal/store"
)

func TestUserStore_Create_Get(t *testing.T) {
    ctx := context.Background()
    pg, err := tcpg.Run(ctx, "postgres:16-alpine",
        tcpg.WithInitScripts(filepath.Join("..", "..", "migrations", "0001_users.sql")),
        tcpg.WithDatabase("test"),
        tcpg.WithUsername("test"),
        tcpg.WithPassword("test"),
        tcpg.BasicWaitStrategies(),
    )
    require.NoError(t, err)
    t.Cleanup(func() { _ = pg.Terminate(ctx) })

    dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
    require.NoError(t, err)

    db, err := sql.Open("pgx", dsn)
    require.NoError(t, err)
    require.NoError(t, db.PingContext(ctx))
    db.SetMaxOpenConns(5)

    s := &store.UserStore{DB: db}
    u := &service.User{Email: "t@x.com", Name: "test"}
    require.NoError(t, s.Create(ctx, u))
    require.NotZero(t, u.ID)

    got, err := s.GetByID(ctx, u.ID)
    require.NoError(t, err)
    require.Equal(t, "t@x.com", got.Email)
}

func init() {
    _ = time.Local // suppress unused-import in some setups
}
```

The build tag (`//go:build integration`) keeps it out of the default `go test ./...` run:

```bash
go test ./...                          # unit tests, fast
go test -tags=integration ./...        # full suite, slower
```

Tests like this catch real SQL bugs (typos, missing indexes, type mismatches) that `fakeStore` cannot.

### Coverage and the race detector

Always run tests with `-race` in CI:

```bash
go test -race -cover ./...
```

`-race` instruments your code with the race detector. It will catch concurrent map writes, unprotected shared state, and many of the goroutine bugs that module 12 will warn you about.

---

## 4. Practical application — full handler test suite with mocks

A complete test file for the user handler from module 09. Demonstrates the full pattern.

```go
// internal/http/handlers/users_test.go
package handlers_test

import (
    "bytes"
    "context"
    "encoding/json"
    "io"
    "log/slog"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/you/hello-gin/internal/http/ginx"
    "github.com/you/hello-gin/internal/http/handlers"
    "github.com/you/hello-gin/internal/service"
)

// ---------- Fake store ----------

type fakeStore struct {
    users  map[int64]*service.User
    nextID int64
}

func newFakeStore() *fakeStore {
    return &fakeStore{users: map[int64]*service.User{}, nextID: 1}
}

func (f *fakeStore) GetByID(_ context.Context, id int64) (*service.User, error) {
    u, ok := f.users[id]
    if !ok {
        return nil, service.ErrUserNotFound
    }
    return u, nil
}
func (f *fakeStore) Create(_ context.Context, u *service.User) error {
    for _, x := range f.users {
        if x.Email == u.Email {
            return service.ErrEmailTaken
        }
    }
    u.ID = f.nextID
    f.nextID++
    f.users[u.ID] = u
    return nil
}
func (f *fakeStore) List(_ context.Context, limit, offset int) ([]service.User, error) {
    out := make([]service.User, 0)
    for _, u := range f.users {
        out = append(out, *u)
    }
    return out, nil
}

// ---------- Harness ----------

func newRouter(t *testing.T) (*gin.Engine, *fakeStore) {
    t.Helper()
    gin.SetMode(gin.TestMode)
    fs := newFakeStore()
    fs.users[1] = &service.User{ID: 1, Email: "y@x.com", Name: "yati"}
    fs.nextID = 2

    svc := &service.UserService{Store: fs}
    h := &handlers.UserHandler{Users: svc}

    log := slog.New(slog.NewTextHandler(io.Discard, nil))
    r := gin.New()
    api := r.Group("/api/v1")
    api.GET("/users/:id", ginx.Wrap(log, h.Get))
    api.POST("/users",    ginx.Wrap(log, h.Create))
    return r, fs
}

// ---------- Tests ----------

func TestGetUser(t *testing.T) {
    cases := []struct {
        name        string
        id          string
        wantStatus  int
        wantSubstr  string
    }{
        {"ok",      "1",   http.StatusOK,         `"id":1`},
        {"missing", "999", http.StatusNotFound,   `"NOT_FOUND"`},
        {"badid",   "abc", http.StatusBadRequest, `"BAD_REQUEST"`},
    }
    for _, tc := range cases {
        tc := tc
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            r, _ := newRouter(t)
            w := httptest.NewRecorder()
            req := httptest.NewRequest("GET", "/api/v1/users/"+tc.id, nil)
            r.ServeHTTP(w, req)

            assert.Equal(t, tc.wantStatus, w.Code)
            assert.Contains(t, w.Body.String(), tc.wantSubstr)
        })
    }
}

func TestCreateUser_HappyAndDuplicate(t *testing.T) {
    r, fs := newRouter(t)

    body, _ := json.Marshal(map[string]any{"email": "new@x.com", "name": "new"})
    w := httptest.NewRecorder()
    req := httptest.NewRequest("POST", "/api/v1/users", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    r.ServeHTTP(w, req)
    require.Equal(t, http.StatusCreated, w.Code)

    var created service.User
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
    require.Equal(t, "new@x.com", created.Email)
    require.NotZero(t, created.ID)
    require.Contains(t, fs.users, created.ID) // side effect check

    // Second time → 409
    w2 := httptest.NewRecorder()
    req2 := httptest.NewRequest("POST", "/api/v1/users", bytes.NewReader(body))
    req2.Header.Set("Content-Type", "application/json")
    r.ServeHTTP(w2, req2)
    require.Equal(t, http.StatusConflict, w2.Code)
}

func TestCreateUser_Validation(t *testing.T) {
    r, _ := newRouter(t)
    body := bytes.NewBufferString(`{"email":"bad","name":""}`)
    w := httptest.NewRecorder()
    req := httptest.NewRequest("POST", "/api/v1/users", body)
    req.Header.Set("Content-Type", "application/json")
    r.ServeHTTP(w, req)
    require.Equal(t, http.StatusBadRequest, w.Code)
    require.Contains(t, w.Body.String(), `"BAD_REQUEST"`)
}
```

Run:

```bash
go test -race -v ./internal/http/handlers/
# === RUN   TestGetUser
# === RUN   TestGetUser/ok
# === RUN   TestGetUser/missing
# === RUN   TestGetUser/badid
# --- PASS: TestGetUser (0.00s)
# ...
```

---

## 5. Common mistakes & gotchas

- **Forgetting `gin.SetMode(gin.TestMode)`.** Without it your test output is buried in `[GIN-debug]` log lines and the `Recovery` middleware prints stack traces to stdout. Always set it.
- **Sharing the engine across parallel subtests.** If `newRouter` builds one engine and tests mutate the underlying store, parallel tests race. Build a fresh engine + store per subtest.
- **Forgetting `req.Header.Set("Content-Type", "application/json")` on POST tests.** `c.ShouldBindJSON` is fine without it (it ignores the header), but `c.ShouldBind` reads the header to pick a binder. Setting it removes ambiguity.
- **Not running with `-race`.** The race detector catches real concurrency bugs. CI should always use `-race`. It's ~2× slower; that's a perfectly good trade.
- **Asserting on full JSON strings with field-order sensitivity.** Go's JSON output ordering is deterministic (struct field order, sorted map keys since Go 1.12), but it's still fragile. Use `assert.JSONEq` (which parses both sides) or assert on specific fields after `json.Unmarshal`-ing the response.
- **Side-effect-only tests with no assertion.** Calling the handler and checking only `w.Code == 200` misses regressions in the body. Assert on the response body shape too.
- **Coupling tests to internal implementation.** Test the HTTP behavior, not internal types. If your `fakeStore` exposes "did Create get called?" — fine. If your test reaches into a private field of `UserService`, you've created a brittle test.
- **Integration tests in the default `go test` run.** They're slow, depend on Docker, and break local-dev flow. Tag them (`//go:build integration`) and run them in CI's slower job.
- **Not closing `testcontainers` containers.** `t.Cleanup(func() { _ = pg.Terminate(ctx) })`. Otherwise CI runners pile up zombie Postgres containers and OOM.

---

## 🎯 Key Takeaways

1. **`r.ServeHTTP(w, req)` is the entire test harness.** No socket, no goroutine, no `:8080`. Tests run in microseconds and don't flake.
2. **Define your stores as interfaces and inject them.** That's the *whole reason* — it lets you slot a `fakeStore` into tests and get a fast, isolated suite. No DI framework required.
3. **Table-driven + `t.Run` + `t.Parallel`** is the Go idiom. Internalize it. Reviewers will expect every handler to have one of these test functions per public route.
4. **Tag integration tests** with `//go:build integration` and use `testcontainers-go` for real DBs. Unit tests run on every save; integration tests run in CI. Don't conflate them.
5. **Always run `-race` in CI.** It is the single most valuable Go testing flag and the only way to catch the kind of concurrent-map bug you'll otherwise debug at 2 a.m. in production.

*← [10 — Authentication and Authorization](./10_auth.md) | [12 — Concurrency Patterns →](./12_concurrency.md)*
