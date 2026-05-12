# 05 — Response Handling

> **Goal:** Send any response Gin supports — JSON, XML, YAML, HTML, files, streams — set headers and cookies correctly, handle content negotiation, and stream large payloads without OOMing.

---

## 1. Responses — mental model + working code

A response in Gin is **status code + headers + body**, written to `c.Writer`. The shortcuts (`c.JSON`, `c.String`, `c.File`, etc.) are conveniences that set `Content-Type`, write the status, marshal/serialize the body, and flush. Underneath, they all use `c.Writer.Write(...)`.

```go
func handler(c *gin.Context) {
    c.Header("X-Foo", "bar")            // set a response header
    c.JSON(http.StatusOK, gin.H{"hello": "world"})
}
```

The first call to `c.Writer.Write` (or `c.Status`) commits the status code and headers. After that point, header changes are silently dropped — that's stdlib `net/http` behavior, not Gin. Set headers first, then write.

---

## 2. How Gin writes responses — `responseWriter`

`c.Writer` is a `gin.ResponseWriter` interface, implemented by `*responseWriter`. It wraps the stdlib `http.ResponseWriter` and adds:

```go
type ResponseWriter interface {
    http.ResponseWriter
    http.Hijacker          // for WebSockets
    http.Flusher           // for streaming
    http.CloseNotifier     // deprecated, use Request.Context

    Status() int           // current status code
    Size() int             // bytes written
    Written() bool         // has WriteHeader been called?
    WriteHeaderNow()       // force header commit
    Pusher() http.Pusher   // HTTP/2 server push
}
```

The wrapper is **why Gin's logger middleware can record status + bytes** after a handler runs — it kept track. It's also why middleware that runs after `c.Next()` can inspect what the handler did.

### The rendering layer

Each `c.JSON`, `c.XML`, `c.YAML`, etc., calls `c.Render(status, renderer)`, which:

1. Sets `Content-Type` from the renderer's `WriteContentType`.
2. Calls `c.Writer.WriteHeader(status)`.
3. Calls `renderer.Render(c.Writer)` which marshals and writes the body.

The `render` package (`github.com/gin-gonic/gin/render`) holds all the implementations. They're small — `render.JSON` is essentially `json.NewEncoder(w).Encode(obj)`. You can write your own.

---

## 3. The full response surface

### JSON variants

```go
c.JSON(200, obj)            // standard, fastest; escapes HTML chars
c.IndentedJSON(200, obj)    // pretty-printed; slower, for debug
c.SecureJSON(200, []int{1}) // prepends "while(1);" — protects legacy clients from JSON array XSS
c.AsciiJSON(200, obj)       // escapes non-ASCII; useful for systems that mangle UTF-8
c.PureJSON(200, obj)        // does NOT escape <, >, & — use when the consumer is not a browser
c.JSONP(200, obj)           // wraps in callback if ?callback=foo is set
```

By default Gin uses `encoding/json` (stdlib). The build tag `jsoniter` swaps in `json-iterator/go`. There's also `go-json` via `gin/json/sonic` etc. For most services the stdlib is fast enough; only swap if you've profiled.

### XML, YAML, ProtoBuf, MsgPack

```go
c.XML(200, obj)             // marshals with encoding/xml
c.YAML(200, obj)             // requires "gopkg.in/yaml.v3" indirectly via render
c.ProtoBuf(200, msg)
```

Useful for legacy SOAP integrations and config endpoints. JSON dominates new APIs.

### Strings and raw data

```go
c.String(200, "hello %s, %d items", name, count)   // text/plain
c.Data(200, "application/octet-stream", []byte{...})
c.DataFromReader(200, contentLength, contentType, reader, extraHeaders)
```

### Files

```go
c.File("/var/data/report.pdf")
c.FileAttachment("/var/data/report.pdf", "Q3-report.pdf") // sets Content-Disposition
c.FileFromFS("nested/path.png", http.FS(embeddedFS))      // serve from embed.FS
```

Module 08 covers static file serving and `embed.FS` in depth.

### HTML templates

```go
r.LoadHTMLGlob("templates/*.tmpl")
r.GET("/", func(c *gin.Context) {
    c.HTML(200, "index.tmpl", gin.H{"title": "Hello"})
})
```

Module 08 again. The 30-second version: Gin wraps Go's `html/template` and lets you render by template name.

### Redirects

```go
c.Redirect(http.StatusMovedPermanently, "/new")        // 301
c.Redirect(http.StatusFound, "/login")                 // 302
c.Redirect(http.StatusSeeOther, "/result")             // 303 — POST → GET safe
c.Redirect(http.StatusTemporaryRedirect, "/maint")     // 307 — preserves method+body
c.Redirect(http.StatusPermanentRedirect, "/v2")        // 308
```

**307 vs 302 matters.** A 302 may cause a POST to be retried as a GET (browsers do this); 307 preserves the original method. For API redirects, use 307 or 308.

### Headers and cookies

```go
c.Header("Cache-Control", "no-store")
c.Header("X-Total-Count", strconv.Itoa(n))

// Cookies
c.SetCookie(
    "session",     // name
    "encrypted",   // value
    3600,          // maxAge seconds (0 = session cookie, <0 = delete)
    "/",           // path
    "example.com", // domain ("" = current host)
    true,          // secure (HTTPS only)
    true,          // httpOnly (no JS access)
)

// To set SameSite, use a sync.Once or initialization to set it engine-wide:
r.Use(func(c *gin.Context) {
    c.SetSameSite(http.SameSiteLaxMode)
    c.Next()
})
```

Set `Secure: true` and `HttpOnly: true` for session cookies in production. Forgetting either is a real CVE pattern.

### Streaming

`c.Stream(fn)` flushes after each chunk and stops when `fn` returns `false`:

```go
r.GET("/stream", func(c *gin.Context) {
    c.Stream(func(w io.Writer) bool {
        select {
        case <-c.Request.Context().Done():
            return false                       // client gone, stop
        case msg := <-someChan:
            _, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
            return true                        // continue
        case <-time.After(30 * time.Second):
            return false                       // timeout
        }
    })
})
```

This is the building block for Server-Sent Events (SSE):

```go
c.Header("Content-Type", "text/event-stream")
c.Header("Cache-Control", "no-cache")
c.Header("Connection", "keep-alive")
c.Stream(func(w io.Writer) bool { /* ... */ })
```

`c.Writer` also implements `http.Flusher`, so you can write and flush manually:

```go
flusher, _ := c.Writer.(http.Flusher)
fmt.Fprintf(c.Writer, "chunk %d\n", i)
flusher.Flush()
```

### Content negotiation

```go
func handler(c *gin.Context) {
    obj := gin.H{"name": "yati"}
    c.Negotiate(200, gin.Negotiate{
        Offered: []string{gin.MIMEJSON, gin.MIMEXML, gin.MIMEYAML},
        Data:    obj,
    })
}
```

Gin picks based on the request's `Accept` header. If none of the offered types match, it writes a 406. In practice most JSON APIs don't bother — they hardcode JSON and document it. Negotiation is useful for public APIs that genuinely support multiple formats.

### Manual status codes

```go
c.Status(204)               // sets status, doesn't write body
c.AbortWithStatus(503)
```

For a 204 No Content response, set the status and return. Don't write a body — clients are entitled to ignore it, and some break on empty JSON.

### Status codes you should know cold

| Code | Name | Use |
|------|------|-----|
| 200 | OK | Successful GET, default success |
| 201 | Created | Successful POST/PUT that created a resource — include `Location` header |
| 202 | Accepted | Async work queued |
| 204 | No Content | Successful DELETE, or PUT with no body to return |
| 301/308 | Permanent redirect | Resource moved permanently |
| 302/307 | Temporary redirect | Temporary; 307 preserves method |
| 400 | Bad Request | Malformed input, validation failure |
| 401 | Unauthorized | Missing/invalid auth |
| 403 | Forbidden | Authenticated but not allowed |
| 404 | Not Found | Resource doesn't exist |
| 405 | Method Not Allowed | Wrong verb — include `Allow` header |
| 409 | Conflict | Optimistic-concurrency failure, unique-key violation |
| 410 | Gone | Permanently removed |
| 415 | Unsupported Media Type | Wrong `Content-Type` |
| 422 | Unprocessable Entity | Syntactically valid but semantically wrong (validation) — 400 also accepted |
| 429 | Too Many Requests | Rate limited — include `Retry-After` |
| 500 | Internal Server Error | Unhandled error |
| 503 | Service Unavailable | Downstream down, shutting down |

Interview-grade detail. Pick a convention (400 vs 422 for validation) and stick with it across the codebase.

---

## 4. Practical application — a download endpoint with streaming, plus SSE

### Endpoint A: stream a generated CSV

```go
package main

import (
    "encoding/csv"
    "net/http"
    "strconv"

    "github.com/gin-gonic/gin"
)

func main() {
    r := gin.Default()
    r.GET("/users.csv", exportUsersCSV)
    r.GET("/events", streamEvents)
    r.Run(":8080")
}

type user struct {
    ID    int64
    Name  string
    Email string
}

// Pretend this comes from a DB cursor.
func userIter() func() (user, bool) {
    rows := []user{
        {1, "yati", "yati@example.com"},
        {2, "alex", "alex@example.com"},
        {3, "kai", "kai@example.com"},
    }
    i := 0
    return func() (user, bool) {
        if i >= len(rows) {
            return user{}, false
        }
        u := rows[i]
        i++
        return u, true
    }
}

func exportUsersCSV(c *gin.Context) {
    c.Header("Content-Type", "text/csv; charset=utf-8")
    c.Header("Content-Disposition", `attachment; filename="users.csv"`)
    c.Status(http.StatusOK)

    w := csv.NewWriter(c.Writer)
    _ = w.Write([]string{"id", "name", "email"})

    next := userIter()
    for {
        u, ok := next()
        if !ok {
            break
        }
        _ = w.Write([]string{strconv.FormatInt(u.ID, 10), u.Name, u.Email})
    }
    w.Flush()
}
```

Test:

```bash
curl -i http://localhost:8080/users.csv
# Content-Type: text/csv; charset=utf-8
# Content-Disposition: attachment; filename="users.csv"
# id,name,email
# 1,yati,yati@example.com
# ...
```

The CSV is written incrementally to `c.Writer`. For a million-row export, only one row at a time lives in memory.

### Endpoint B: Server-Sent Events

```go
import (
    "fmt"
    "io"
    "time"
)

func streamEvents(c *gin.Context) {
    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    c.Header("Connection", "keep-alive")
    c.Header("X-Accel-Buffering", "no")        // disable nginx buffering

    tick := time.NewTicker(1 * time.Second)
    defer tick.Stop()

    c.Stream(func(w io.Writer) bool {
        select {
        case <-c.Request.Context().Done():
            return false
        case t := <-tick.C:
            fmt.Fprintf(w, "event: tick\ndata: {\"time\":\"%s\"}\n\n", t.Format(time.RFC3339))
            return true
        }
    })
}
```

Test:

```bash
curl -N http://localhost:8080/events
# event: tick
# data: {"time":"2026-05-13T10:00:00Z"}
# 
# event: tick
# data: {"time":"2026-05-13T10:00:01Z"}
# ...
```

`-N` disables curl's output buffering so you actually see chunks as they arrive. Browsers consume this via the `EventSource` API.

---

## 5. Common mistakes & gotchas

- **Setting headers after `c.JSON`.** Headers must be set before the first body byte. After `c.JSON` writes, `c.Header(...)` is a no-op. Gin doesn't log a warning either — it just silently fails.
- **Writing the body twice.** `c.JSON(200, x); c.JSON(400, y)` — the second one writes garbage onto the first response (or fails). The client sees the first body with mangled trailing bytes. One write per request; use `Abort*` if you want to skip remaining handlers.
- **302 instead of 307 for POST redirects.** A 302 lets the browser change the method to GET; the request body is lost. Use 307 or 308 unless you specifically want method-rewriting behavior.
- **Forgetting `Secure` + `HttpOnly` on session cookies.** Without `Secure`, the cookie is sent over HTTP and can be sniffed. Without `HttpOnly`, page JS can read it (XSS → token theft). Both are non-negotiable in production.
- **Setting `Cache-Control: no-cache` when you mean `no-store`.** `no-cache` allows caches to store but requires revalidation. `no-store` is the actual "do not cache." Bank statements: `no-store`. Static assets with hashed URLs: `public, max-age=31536000, immutable`.
- **Streaming without honoring `c.Request.Context().Done()`.** When the client disconnects, your loop keeps running, generating data nobody reads. The connection eventually breaks, but you've burned CPU and possibly hit a DB. Always select on `Done()`.
- **Returning a Go zero-value struct as a 200.** `c.JSON(200, user{})` returns `{"id":0,"name":""}` if the user wasn't found. Check first, return 404.
- **`c.IndentedJSON` in production.** Doubles the bytes on the wire and isn't faster to debug — you have JSON pretty-printers in your tools. Use `c.JSON`.
- **Sending sensitive data in URL query strings.** They land in proxy logs, browser history, `Referer` headers. Use body or headers for tokens, even on GET-shaped APIs.

---

## 🎯 Key Takeaways

1. **`c.Writer` is the stdlib response writer with bookkeeping.** It implements `http.Flusher`, `http.Hijacker`, `http.Pusher` — anything you can do in plain `net/http`, you can do here.
2. **Order matters: status + headers, then body.** First byte commits the headers. Set everything before `c.JSON`/`c.String`.
3. **Stream big responses.** CSV exports, log tails, SSE — keep memory bounded by writing as you produce, and always select on `Request.Context().Done()` so disconnects free your goroutine.
4. **Cookies need `Secure` + `HttpOnly` + a `SameSite` policy.** Forgetting any of these is the most common audit finding in Go web services.
5. **Pick a validation-error status code (400 or 422) and use it everywhere.** Consistency lets API consumers write one error handler instead of branching per endpoint — a small thing that signals professionalism in any code review.

*← [04 — Request Handling](./04_request_handling.md) | [06 — Middleware →](./06_middleware.md)*
