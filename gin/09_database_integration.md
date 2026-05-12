# 09 — Database Integration

> **Goal:** Wire a real database into a Gin service — `database/sql` with `pgx`, type-safe queries with `sqlc`, an ORM option (GORM), and the request-scoped-transaction pattern senior engineers use.

---

## 1. `database/sql` + `pgx` — mental model + working code

`database/sql` is Go's stdlib database interface. It defines `*sql.DB`, `*sql.Tx`, `*sql.Rows`, `Exec`, `Query`, etc. It does **not** speak any particular SQL dialect — you add a *driver*. For PostgreSQL the modern choice is `jackc/pgx/v5`. You have two ways to use it:

- **`pgx` via `database/sql`** — `_ "github.com/jackc/pgx/v5/stdlib"` and use `sql.Open("pgx", dsn)`. Standard interface; works with any stdlib-compatible library.
- **`pgx` native** — `pgxpool.New(ctx, dsn)` — slightly faster, richer types (UUID, JSONB, numeric), but you give up stdlib interop.

Pick `pgx` native if you're PostgreSQL-only and want the speed/types. Pick `database/sql` + pgx-stdlib for portability or if you use `sqlc` (which targets `database/sql`).

This module uses `database/sql` + pgx-stdlib because that's the path most teams take.

### Setup

```bash
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/stdlib
```

```go
// internal/store/db.go
package store

import (
    "context"
    "database/sql"
    "fmt"
    "time"

    _ "github.com/jackc/pgx/v5/stdlib"
)

func Open(ctx context.Context, dsn string) (*sql.DB, error) {
    db, err := sql.Open("pgx", dsn)
    if err != nil {
        return nil, fmt.Errorf("open: %w", err)
    }
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(25)
    db.SetConnMaxLifetime(30 * time.Minute)
    db.SetConnMaxIdleTime(5 * time.Minute)

    pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    if err := db.PingContext(pingCtx); err != nil {
        return nil, fmt.Errorf("ping: %w", err)
    }
    return db, nil
}
```

### A query

```go
// internal/store/users.go
package store

import (
    "context"
    "database/sql"
    "errors"
    "fmt"

    "github.com/you/hello-gin/internal/service"
)

type UserStore struct{ DB *sql.DB }

func (s *UserStore) GetByID(ctx context.Context, id int64) (*service.User, error) {
    const q = `SELECT id, email, name FROM users WHERE id = $1`
    var u service.User
    err := s.DB.QueryRowContext(ctx, q, id).Scan(&u.ID, &u.Email, &u.Name)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, service.ErrUserNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("getByID(%d): %w", id, err)
    }
    return &u, nil
}

func (s *UserStore) Create(ctx context.Context, u *service.User) error {
    const q = `INSERT INTO users (email, name) VALUES ($1, $2) RETURNING id`
    err := s.DB.QueryRowContext(ctx, q, u.Email, u.Name).Scan(&u.ID)
    if err != nil {
        return fmt.Errorf("create: %w", err)
    }
    return nil
}
```

### Wiring into Gin

```go
// cmd/api/main.go
package main

import (
    "context"
    "log"
    "os"

    "github.com/gin-gonic/gin"
    "github.com/you/hello-gin/internal/http/handlers"
    "github.com/you/hello-gin/internal/service"
    "github.com/you/hello-gin/internal/store"
)

func main() {
    db, err := store.Open(context.Background(), os.Getenv("DATABASE_URL"))
    if err != nil {
        log.Fatalf("db: %v", err)
    }
    defer db.Close()

    users := &store.UserStore{DB: db}
    svc := &service.UserService{Store: users}
    h := &handlers.UserHandler{Users: svc}

    r := gin.Default()
    r.GET("/users/:id", h.Get)
    r.Run(":8080")
}
```

Three rules that already differentiate professional code:

1. **Always pass `ctx` from `c.Request.Context()` to `QueryContext`/`ExecContext`/`QueryRowContext`.** When the client disconnects or the request times out, the DB query is cancelled. Without `ctx`, your DB keeps running queries no one waits for.
2. **Tune the connection pool.** Defaults (`MaxOpenConns` is unlimited!) will saturate your database under load. 25 is a defensible starting point for most APIs; tune from there.
3. **Map `sql.ErrNoRows` to a domain error.** Don't let `sql.ErrNoRows` leak into handlers — they have no business knowing about SQL.

---

## 2. How `*sql.DB` works — pooling and contexts

`*sql.DB` is not a single connection — it's a **pool of connections**. Calls to `QueryContext` take a connection out, run the statement, and return it. Pool semantics:

- **`MaxOpenConns`** — hard cap on simultaneous connections to the DB. Default: 0 = unlimited. Always set this.
- **`MaxIdleConns`** — how many idle connections to keep around. Default: 2. Bump it up if you don't want pool churn.
- **`ConnMaxLifetime`** — how long a connection can live before being recycled. Useful behind a load-balanced DB or with serverless Postgres that closes idle connections.
- **`ConnMaxIdleTime`** — how long an idle connection stays before being closed.

`QueryRowContext` returns a `*sql.Row` whose `.Scan` actually runs the query (lazy). `QueryContext` returns `*sql.Rows` which holds a connection open until `Close` is called — **always `defer rows.Close()`** or you leak.

### Transactions

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil { return err }
defer tx.Rollback()        // safe no-op if Commit succeeded

if _, err := tx.ExecContext(ctx, "UPDATE ..."); err != nil {
    return err
}
if _, err := tx.ExecContext(ctx, "INSERT ..."); err != nil {
    return err
}
return tx.Commit()
```

A `*sql.Tx` holds one connection from the pool until commit/rollback. Don't hand `*sql.Tx` to long-running goroutines or you'll pin pool slots.

---

## 3. Variations — `sqlc`, GORM, and request-scoped transactions

### `sqlc` — type-safe SQL without an ORM

You write SQL with named parameters in a `.sql` file; `sqlc generate` produces Go code with typed methods. Best of both worlds: hand-written SQL + compile-time safety.

```sql
-- query.sql
-- name: GetUser :one
SELECT id, email, name FROM users WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (email, name) VALUES ($1, $2) RETURNING id, email, name;

-- name: ListUsers :many
SELECT id, email, name FROM users ORDER BY id LIMIT $1 OFFSET $2;
```

```yaml
# sqlc.yaml
version: "2"
sql:
  - engine: postgresql
    queries: query.sql
    schema: schema.sql
    gen:
      go:
        package: dbgen
        out: internal/store/dbgen
        sql_package: database/sql
```

```bash
sqlc generate
# produces internal/store/dbgen/*.go with typed methods
```

Usage:

```go
import "github.com/you/hello-gin/internal/store/dbgen"

q := dbgen.New(db)
u, err := q.GetUser(ctx, 42)
```

`sqlc` is the modern idiomatic choice. You keep SQL visible (queryable, indexable, EXPLAIN-able), you keep type safety, and you avoid ORM magic.

### GORM — when an ORM is the right call

```go
import (
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
)

type User struct {
    ID    int64
    Email string
    Name  string
}

db, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{})
db.AutoMigrate(&User{})

var u User
db.WithContext(ctx).First(&u, 42)
```

When to pick GORM:

- Your team is more comfortable with ORMs (Rails/Django/Hibernate backgrounds).
- Schema is fluid and migrations + struct evolution matter more than query control.
- CRUD-heavy app where SQL is mostly boilerplate.

When **not** to pick GORM:

- Hot paths with complex queries — GORM hides the SQL, and that hiding is exactly what bites you when the EXPLAIN plan goes sideways.
- Teams that already know SQL well. `sqlc` is simply better for that audience.

There's no wrong answer; just be deliberate. Most production Gin services I see at senior shops use `sqlc` (or `database/sql` directly), not GORM.

### Request-scoped transactions

Sometimes a request must do several writes atomically. The clean pattern: a `TxManager` that runs a function inside a transaction and exposes the same store interface.

```go
// internal/store/txmgr.go
package store

import (
    "context"
    "database/sql"
)

type DBTX interface {
    QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
    ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
}

// UserStore is generalized to take DBTX so it works with *sql.DB or *sql.Tx.
type UserStoreTx struct{ DB DBTX }

type TxManager struct{ DB *sql.DB }

func (m *TxManager) Run(ctx context.Context, fn func(*UserStoreTx) error) (err error) {
    tx, err := m.DB.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer func() {
        if p := recover(); p != nil {
            _ = tx.Rollback()
            panic(p)
        }
        if err != nil {
            _ = tx.Rollback()
            return
        }
        err = tx.Commit()
    }()

    return fn(&UserStoreTx{DB: tx})
}
```

Service-layer usage:

```go
func (s *UserService) Transfer(ctx context.Context, fromID, toID int64, amount int64) error {
    return s.Tx.Run(ctx, func(st *store.UserStoreTx) error {
        if err := st.Debit(ctx, fromID, amount); err != nil { return err }
        if err := st.Credit(ctx, toID,   amount); err != nil { return err }
        return st.RecordTransfer(ctx, fromID, toID, amount)
    })
}
```

The transaction is scoped to one function call. If any step returns an error, everything rolls back. No hidden global state, no transaction-in-context-key hack — the dependency is explicit.

---

## 4. Practical application — REST CRUD on `users` with pgx + connection pool tuning

A full slice. Schema, store, service, handler, and a few `curl`s.

### Schema

```sql
-- migrations/0001_users.sql
CREATE TABLE users (
    id          BIGSERIAL PRIMARY KEY,
    email       TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Use a migration tool (`golang-migrate`, `goose`, `atlas`). For learning, run the SQL directly: `psql ... -f migrations/0001_users.sql`.

### Store

```go
// internal/store/users.go
package store

import (
    "context"
    "database/sql"
    "errors"
    "fmt"

    "github.com/jackc/pgx/v5/pgconn"
    "github.com/you/hello-gin/internal/service"
)

type UserStore struct{ DB *sql.DB }

func (s *UserStore) GetByID(ctx context.Context, id int64) (*service.User, error) {
    const q = `SELECT id, email, name FROM users WHERE id = $1`
    var u service.User
    err := s.DB.QueryRowContext(ctx, q, id).Scan(&u.ID, &u.Email, &u.Name)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, service.ErrUserNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("getByID: %w", err)
    }
    return &u, nil
}

func (s *UserStore) Create(ctx context.Context, u *service.User) error {
    const q = `INSERT INTO users (email, name) VALUES ($1, $2) RETURNING id`
    err := s.DB.QueryRowContext(ctx, q, u.Email, u.Name).Scan(&u.ID)
    if err != nil {
        var pg *pgconn.PgError
        if errors.As(err, &pg) && pg.Code == "23505" {       // unique violation
            return service.ErrEmailTaken
        }
        return fmt.Errorf("create: %w", err)
    }
    return nil
}

func (s *UserStore) List(ctx context.Context, limit, offset int) ([]service.User, error) {
    const q = `SELECT id, email, name FROM users ORDER BY id LIMIT $1 OFFSET $2`
    rows, err := s.DB.QueryContext(ctx, q, limit, offset)
    if err != nil {
        return nil, fmt.Errorf("list: %w", err)
    }
    defer rows.Close()
    out := make([]service.User, 0, limit)
    for rows.Next() {
        var u service.User
        if err := rows.Scan(&u.ID, &u.Email, &u.Name); err != nil {
            return nil, err
        }
        out = append(out, u)
    }
    return out, rows.Err()
}
```

Notice the **Postgres error code check (`23505`)** — that's how you turn a unique-constraint violation into a domain "email taken" error.

### Service + handler

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
    List(ctx context.Context, limit, offset int) ([]User, error)
}

type UserService struct{ Store UserStore }

func (s *UserService) Get(ctx context.Context, id int64) (*User, error) {
    return s.Store.GetByID(ctx, id)
}

func (s *UserService) Create(ctx context.Context, u *User) error {
    return s.Store.Create(ctx, u)
}

func (s *UserService) List(ctx context.Context, page, limit int) ([]User, error) {
    return s.Store.List(ctx, limit, (page-1)*limit)
}
```

```go
// internal/http/handlers/users.go
package handlers

import (
    "errors"
    "strconv"

    "github.com/gin-gonic/gin"
    "github.com/you/hello-gin/internal/apperr"
    "github.com/you/hello-gin/internal/service"
)

type UserHandler struct{ Users *service.UserService }

type createUserReq struct {
    Email string `json:"email" binding:"required,email"`
    Name  string `json:"name"  binding:"required,min=1,max=100"`
}

func (h *UserHandler) Create(c *gin.Context) error {
    var req createUserReq
    if err := c.ShouldBindJSON(&req); err != nil {
        return apperr.BadRequest("invalid body", err)
    }
    u := &service.User{Email: req.Email, Name: req.Name}
    if err := h.Users.Create(c.Request.Context(), u); err != nil {
        if errors.Is(err, service.ErrEmailTaken) {
            return apperr.Conflict("email taken", err)
        }
        return apperr.Internal(err)
    }
    c.JSON(201, u)
    return nil
}

func (h *UserHandler) Get(c *gin.Context) error {
    id, err := strconv.ParseInt(c.Param("id"), 10, 64)
    if err != nil {
        return apperr.BadRequest("id must be an integer", err)
    }
    u, err := h.Users.Get(c.Request.Context(), id)
    if err != nil {
        if errors.Is(err, service.ErrUserNotFound) {
            return apperr.NotFound("user not found")
        }
        return apperr.Internal(err)
    }
    c.JSON(200, u)
    return nil
}

func (h *UserHandler) List(c *gin.Context) error {
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
    if limit > 100 {
        limit = 100
    }
    users, err := h.Users.List(c.Request.Context(), page, limit)
    if err != nil {
        return apperr.Internal(err)
    }
    c.JSON(200, gin.H{"data": users, "page": page, "limit": limit})
    return nil
}
```

### Wiring + curl

```go
// cmd/api/main.go
db, _ := store.Open(ctx, os.Getenv("DATABASE_URL"))
users := &store.UserStore{DB: db}
svc := &service.UserService{Store: users}
h := &handlers.UserHandler{Users: svc}

r := gin.Default()
api := r.Group("/api/v1")
api.POST("/users",     ginx.Wrap(log, h.Create))
api.GET("/users/:id",  ginx.Wrap(log, h.Get))
api.GET("/users",      ginx.Wrap(log, h.List))
r.Run(":8080")
```

```bash
curl -X POST -H 'Content-Type: application/json' \
  -d '{"email":"y@x.com","name":"yati"}' \
  http://localhost:8080/api/v1/users
# 201 {"id":1,"email":"y@x.com","name":"yati"}

# Duplicate
curl -i -X POST -H 'Content-Type: application/json' \
  -d '{"email":"y@x.com","name":"yati"}' \
  http://localhost:8080/api/v1/users
# 409 {"error":"CONFLICT","message":"email taken",...}

curl http://localhost:8080/api/v1/users/1
curl 'http://localhost:8080/api/v1/users?page=1&limit=10'
```

---

## 5. Common mistakes & gotchas

- **Not passing `ctx` to DB calls.** A client disconnects, the request returns, your query still runs to completion. The connection isn't returned to the pool until the query finishes. Under load this is how Gin services tip over.
- **No `MaxOpenConns`.** The default is unlimited. A spike of slow queries opens 500 connections to Postgres, Postgres falls over, your service falls over with it. Set it. 25 is a reasonable start.
- **Forgetting `defer rows.Close()`.** Leaks a connection. Tools like `golangci-lint` with `rowserrcheck` catch this; turn them on.
- **`sql.ErrNoRows` leaking into handlers.** A 500 because Postgres said "no rows" is a contract violation. The store layer maps it to a domain `ErrNotFound`; the handler maps that to 404.
- **`SELECT * FROM users`.** Schema changes silently add a column, your `Scan` blows up at runtime. Always list columns explicitly. Same for `INSERT`.
- **Holding a `*sql.Tx` open across an HTTP round-trip.** Transaction lifetime = function lifetime, full stop. If you find yourself thinking "I'll commit later in another handler," you don't want a transaction — you want a saga or an outbox.
- **String-concatenating user input into SQL.** `"WHERE name = '" + name + "'"`. SQL injection. Always use parameterized queries (`$1`, `?`).
- **Using GORM "because it's easier" then drowning in N+1.** GORM's defaults aren't N+1-aware. Use `Preload` correctly, log SQL in dev mode, and benchmark with real data — or pick `sqlc`.
- **No retry/backoff on transient errors.** A flapping DB connection during a leader failover yields one 500 instead of a 200. Wrap critical paths with a tiny retry helper for serialization errors / connection drops.

---

## 🎯 Key Takeaways

1. **`pgx` via `database/sql`** is the modern default for Postgres in Go: stdlib interface, pgx's parser and types, plays nicely with `sqlc`.
2. **Always tune the pool** (`MaxOpenConns`, `MaxIdleConns`, `ConnMaxLifetime`) and always pass `c.Request.Context()` to DB calls. These two habits eliminate 80% of DB-related Gin incidents.
3. **`sqlc` is the senior-Go default for new services** — hand-written SQL with compile-time type safety. GORM has a place (CRUD-heavy, fluid schema, team preference), but go in eyes-open.
4. **Don't let `sql.ErrNoRows` or driver-specific error codes (`23505`) escape the store layer.** Translate them to domain errors right at the SQL boundary. Handlers should know about your domain, not Postgres.
5. **Request-scoped transactions live in a `TxManager.Run(ctx, fn)`** function — explicit, scoped, no hidden state in `context.Context`. If a function needs the tx, it takes a parameter that is a tx-aware store.

*← [08 — Templates and Static Files](./08_templates_and_static.md) | [10 — Authentication and Authorization →](./10_auth.md)*
