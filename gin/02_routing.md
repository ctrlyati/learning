# 02 — Routing

> **Goal:** Master every Gin routing primitive — methods, params, groups, wildcards — and understand the radix-tree internals well enough to predict and debug conflicts.

---

## 1. Routes — mental model + working code

A route in Gin is the tuple `(HTTP method, path pattern) → []HandlerFunc`. There is one chain of handlers per route; the last is the "endpoint handler" and the earlier ones are middleware that has been merged in.

### HTTP methods

```go
r.GET("/users", listUsers)
r.POST("/users", createUser)
r.PUT("/users/:id", replaceUser)
r.PATCH("/users/:id", updateUser)
r.DELETE("/users/:id", deleteUser)
r.HEAD("/users/:id", headUser)
r.OPTIONS("/users", optionsUsers)

// Any combination:
r.Match([]string{"GET", "POST"}, "/multi", handler)

// Wildcard for any method (rare — usually wrong):
r.Any("/echo", echo)
```

### Path parameters — `:name`

```go
r.GET("/users/:id", func(c *gin.Context) {
    id := c.Param("id")              // always a string
    c.JSON(200, gin.H{"id": id})
})
```

```bash
curl http://localhost:8080/users/42
# {"id":"42"}
```

`c.Param` always returns a string. Parse it yourself:

```go
id, err := strconv.ParseInt(c.Param("id"), 10, 64)
if err != nil {
    c.AbortWithStatusJSON(400, gin.H{"error": "id must be an integer"})
    return
}
```

### Catch-all parameters — `*name`

```go
r.GET("/files/*filepath", func(c *gin.Context) {
    fp := c.Param("filepath")        // includes the leading slash
    c.String(200, "serving %s", fp)
})
```

```bash
curl http://localhost:8080/files/a/b/c.txt
# serving /a/b/c.txt
```

A `*name` segment must be **the last segment**. You can have one per route.

### Query parameters

```go
r.GET("/search", func(c *gin.Context) {
    q := c.Query("q")                          // "" if absent
    page := c.DefaultQuery("page", "1")
    tags := c.QueryArray("tag")                // ?tag=a&tag=b → ["a","b"]
    filters := c.QueryMap("filter")            // ?filter[type]=book → {"type":"book"}
    c.JSON(200, gin.H{
        "q": q, "page": page, "tags": tags, "filters": filters,
    })
})
```

```bash
curl 'http://localhost:8080/search?q=go&tag=lang&tag=fast&filter[type]=book&page=2'
# {"filters":{"type":"book"},"page":"2","q":"go","tags":["lang","fast"]}
```

---

## 2. How Gin implements routing — the radix tree

Gin's router is a fork of `httprouter`. For each HTTP method it maintains a separate **radix tree** (compressed prefix tree). When a request arrives, Gin:

1. Picks the tree for the request method (`engine.trees[methodIndex(method)]`).
2. Walks the tree node by node, matching path prefixes.
3. When it hits a `:name` or `*name` segment, captures the value into the params slice on `gin.Context`.
4. At the leaf, retrieves the merged `[]HandlerFunc` and assigns it to `c.handlers`.
5. Calls `c.Next()` to start running them.

### Why a radix tree?

Three reasons:

- **Lookup cost is `O(L)`** where L is the request path length, independent of how many routes you've registered.
- **Memory is compact** — common prefixes collapse to a single node.
- **Conflict detection at registration time** — Gin rejects ambiguous patterns when you call `r.GET(...)`, not at request time.

### The conflict rules that will bite you

```go
r.GET("/users/:id", h1)
r.GET("/users/me", h2)
// panic: '/users/me' in new path '/users/me' conflicts with existing wildcard ':id'
```

In a node that has a wildcard child (`:id`), you cannot also have a literal child (`me`) at the same level. Gin sees `me` and `:id` as siblings at the same tree depth and refuses to register both. Workarounds:

```go
// Option A: separate paths
r.GET("/users/me", h2)
r.GET("/users/by-id/:id", h1)

// Option B: dispatch inside one handler
r.GET("/users/:id", func(c *gin.Context) {
    if c.Param("id") == "me" {
        currentUser(c)
        return
    }
    userByID(c)
})
```

Most teams pick A — it's clearer and aligns with REST conventions.

### `RedirectTrailingSlash`

By default, requests to `/users/` when only `/users` is registered get a `301`/`307` redirect to `/users`. Some HTTP clients (curl, browsers) follow this transparently; some (mobile SDKs) do not. Turn it off if redirects cause grief:

```go
r := gin.New()
r.RedirectTrailingSlash = false
```

### `HandleMethodNotAllowed`

If you `POST /users` but only `GET /users` is registered, Gin returns `404 Not Found` by default. To get `405 Method Not Allowed` (more correct):

```go
r.HandleMethodNotAllowed = true
```

---

## 3. Route groups and composition

### `Group` — shared prefix + shared middleware

```go
r := gin.Default()

// /api/v1
v1 := r.Group("/api/v1")
v1.Use(RequestID(), Auth())              // every route in v1 gets these
{
    v1.GET("/users", listUsers)
    v1.POST("/users", createUser)
    v1.GET("/users/:id", getUser)

    // Nested group
    admin := v1.Group("/admin", RequireRole("admin"))
    {
        admin.DELETE("/users/:id", deleteUser)
        admin.GET("/audit-log", auditLog)
    }
}
```

The curly braces `{ ... }` are a Go syntax trick — a bare block — used purely for visual indentation. They have no semantic effect. Many Gin codebases use them; others don't. Personal taste.

### Middleware ordering on groups

When a request hits `/api/v1/admin/users/42`:

```
[Logger, Recovery]              ← engine-level (gin.Default)
[RequestID, Auth]               ← v1 group
[RequireRole("admin")]          ← admin group
[deleteUser]                    ← route handler
```

All concatenated into one `[]HandlerFunc`, executed in order.

### `RouterGroup` is composable

`*gin.RouterGroup` and `*gin.Engine` share an interface (`gin.IRoutes`) — both have `GET`, `POST`, `Use`, etc. You can pass a `gin.IRoutes` to a function that registers routes, decoupling registration from where it happens:

```go
func RegisterUserRoutes(rg gin.IRoutes, h *UserHandler) {
    rg.GET("/users", h.List)
    rg.POST("/users", h.Create)
    rg.GET("/users/:id", h.Get)
}

func main() {
    r := gin.Default()
    v1 := r.Group("/api/v1")
    RegisterUserRoutes(v1, userHandler)
    RegisterOrderRoutes(v1, orderHandler)
    r.Run(":8080")
}
```

This is the pattern module 09 onward uses — keeps `cmd/api/main.go` short.

---

## 4. Practical application — versioned REST API with mixed param styles

A small but realistic slice: a versioned API that serves blog posts, with path params, query filters, a catch-all for static assets, and method-not-allowed handling.

```go
// cmd/api/main.go
package main

import (
    "net/http"
    "strconv"

    "github.com/gin-gonic/gin"
)

type Post struct {
    ID     int    `json:"id"`
    Title  string `json:"title"`
    Author string `json:"author"`
    Tags   []string `json:"tags"`
}

// In-memory store for the example.
var posts = []Post{
    {1, "Hello Gin", "yati", []string{"go", "intro"}},
    {2, "Radix Trees", "yati", []string{"go", "internals"}},
    {3, "Production Tips", "alex", []string{"ops", "go"}},
}

func main() {
    r := gin.Default()
    r.HandleMethodNotAllowed = true
    r.RedirectTrailingSlash = false

    v1 := r.Group("/api/v1")
    {
        v1.GET("/posts", listPosts)
        v1.GET("/posts/:id", getPost)
        v1.GET("/authors/:author/posts", postsByAuthor)
    }

    r.NoRoute(func(c *gin.Context) {
        c.JSON(http.StatusNotFound, gin.H{"error": "route not found", "path": c.Request.URL.Path})
    })
    r.NoMethod(func(c *gin.Context) {
        c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
    })

    r.Run(":8080")
}

func listPosts(c *gin.Context) {
    author := c.Query("author")
    tag := c.Query("tag")
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

    out := make([]Post, 0, limit)
    for _, p := range posts {
        if author != "" && p.Author != author {
            continue
        }
        if tag != "" && !contains(p.Tags, tag) {
            continue
        }
        out = append(out, p)
        if len(out) >= limit {
            break
        }
    }
    c.JSON(http.StatusOK, gin.H{"data": out, "count": len(out)})
}

func getPost(c *gin.Context) {
    id, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "id must be an integer"})
        return
    }
    for _, p := range posts {
        if p.ID == id {
            c.JSON(http.StatusOK, p)
            return
        }
    }
    c.JSON(http.StatusNotFound, gin.H{"error": "post not found", "id": id})
}

func postsByAuthor(c *gin.Context) {
    author := c.Param("author")
    out := make([]Post, 0)
    for _, p := range posts {
        if p.Author == author {
            out = append(out, p)
        }
    }
    c.JSON(http.StatusOK, gin.H{"author": author, "posts": out})
}

func contains(haystack []string, needle string) bool {
    for _, s := range haystack {
        if s == needle {
            return true
        }
    }
    return false
}
```

Try it:

```bash
# List all
curl 'http://localhost:8080/api/v1/posts'

# Filter by author and tag
curl 'http://localhost:8080/api/v1/posts?author=yati&tag=go'

# Single post
curl -i 'http://localhost:8080/api/v1/posts/2'

# Posts by author (path param)
curl 'http://localhost:8080/api/v1/authors/alex/posts'

# 404
curl -i 'http://localhost:8080/api/v1/posts/9999'

# 405 (because HandleMethodNotAllowed is on)
curl -i -X PUT 'http://localhost:8080/api/v1/posts'

# Bad ID
curl -i 'http://localhost:8080/api/v1/posts/abc'
```

### Inspecting registered routes

Useful while debugging:

```go
for _, ri := range r.Routes() {
    log.Printf("%-7s %s -> %s", ri.Method, ri.Path, ri.Handler)
}
```

In debug mode Gin prints this at startup automatically. Quick way to spot a typo: read the table.

---

## 5. Common mistakes & gotchas

- **Wildcard / static conflict** — `r.GET("/users/:id", ...)` + `r.GET("/users/new", ...)` panics at registration. Solution: separate paths or dispatch in one handler (see section 2).
- **`*filepath` not last** — `r.GET("/static/*path/raw", ...)` is invalid. Catch-all must be the final segment.
- **Treating `c.Param("id")` as an int** — it is always `string`. Parse and validate it, with a 400 response on failure. Forgetting this means panics or zero values silently flowing into your DB.
- **Defining routes inside a handler** — `r.GET` inside a request handler races and grows the route tree unboundedly. Register routes once, in `main` or in a setup function called from `main`.
- **Relying on `RedirectTrailingSlash`** — works in browsers, often surprises programmatic clients with `307` body-preserving redirects that they don't follow. Be explicit; register both or turn it off.
- **`r.Any` for everything** — `r.Any("/users", h)` registers the same handler for GET, POST, PUT, DELETE, etc. It's almost never what you want; the right answer is usually two or three explicit methods.
- **Putting versioning in the host header instead of the path** — `Accept: application/vnd.api.v2+json` is technically more REST-y but operationally a nightmare. Path versioning (`/api/v1`, `/api/v2`) is the norm in Go shops. Module 14 covers this.
- **Forgetting `NoRoute` / `NoMethod`** — without them you get Gin's default `404 page not found` plain-text response, which is inconsistent with your JSON error format and breaks API consumers expecting JSON.

---

## 🎯 Key Takeaways

1. **Routing cost is `O(path length)`, not `O(routes)`.** A radix tree is per-method; you can register tens of thousands of routes without slowdown.
2. **Wildcard conflicts panic at startup, by design.** Read the panic message — it tells you exactly which two routes collide. Don't fight the router; redesign the paths.
3. **`RouterGroup` is composable via `gin.IRoutes`.** Pass groups into per-domain `Register...Routes(rg, deps...)` functions. Keeps `main` short and tests easy.
4. **`c.Param` is always a string** — parse and validate every numeric/UUID param. The first three lines of every `GET /things/:id` handler are usually a `strconv.Atoi` and a 400.
5. **Always register `NoRoute` and `NoMethod`** in any service that returns JSON, so your error format is consistent end-to-end. API consumers will notice; reviewers and SREs will notice.

*← [01 — Setup and First Server](./01_setup_and_first_server.md) | [03 — Context Deep Dive →](./03_context_deep_dive.md)*
