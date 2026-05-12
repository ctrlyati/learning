# 08 — Templates and Static Files

> **Goal:** Render server-side HTML with `html/template`, manage multiple template sets, serve static assets, and ship the whole thing inside a single binary using `embed.FS`.

---

## 1. Templates — mental model + working code

Server-rendered HTML in Gin is just `html/template` from the Go standard library with a thin layer to load templates and render them by name. The mental model:

```
LoadHTMLGlob/Files  →  parses .tmpl files into one *html/template.Template
c.HTML(status, name, data)  →  executes the template named "name" against data
```

### A "hello" template

```text
project/
├── main.go
└── templates/
    └── index.tmpl
```

```html
<!-- templates/index.tmpl -->
<!doctype html>
<html>
  <head><title>{{ .Title }}</title></head>
  <body>
    <h1>Hello, {{ .Name | upper }}</h1>
    <ul>
      {{ range .Items }}
        <li>{{ . }}</li>
      {{ end }}
    </ul>
  </body>
</html>
```

```go
// main.go
package main

import (
    "net/http"
    "strings"

    "github.com/gin-gonic/gin"
)

func main() {
    r := gin.Default()
    r.SetFuncMap(map[string]any{
        "upper": strings.ToUpper,
    })
    r.LoadHTMLGlob("templates/*.tmpl")

    r.GET("/", func(c *gin.Context) {
        c.HTML(http.StatusOK, "index.tmpl", gin.H{
            "Title": "Welcome",
            "Name":  "yati",
            "Items": []string{"go", "gin", "html/template"},
        })
    })

    r.Run(":8080")
}
```

```bash
curl http://localhost:8080/
# <!doctype html> ... <h1>Hello, YATI</h1> ...
```

Two important details:

- **`r.SetFuncMap` before `LoadHTML*`.** Funcs must be in the template's function map at parse time. Set them first.
- **`html/template`, not `text/template`.** `html/template` auto-escapes — `{{ .UserInput }}` is HTML-safe by default. Never use `text/template` for HTML; it's an XSS factory.

---

## 2. How Gin handles templates

`gin.Engine` has an `HTMLRender` interface. The two built-in implementations:

- **`render.HTMLProduction`** — parses all templates once at startup. Fast, but a template change requires a restart.
- **`render.HTMLDebug`** — re-parses on every request. Used automatically in `GIN_MODE=debug`.

`LoadHTMLGlob` / `LoadHTMLFiles` install whichever mode is active. In production mode you get the production renderer; in debug mode you get the live-reload renderer. You don't need to think about this most of the time — it Just Works.

### Template names

By default the template's name is its filename (without directories). So `templates/users/list.tmpl` is rendered as `c.HTML(200, "list.tmpl", ...)`. If two files have the same basename, the second overwrites the first — **silently**. Use `LoadHTMLFiles` with explicit paths and explicit names if this matters:

Alternative: use `template.New("...").Funcs(...).ParseFiles(...)` yourself and assign with `r.SetHTMLTemplate(t)`. Module covers this below.

### Layouts and partials

`html/template` supports `{{ define "name" }} ... {{ end }}` and `{{ template "name" . }}`. The convention is one template file per "page" that defines a `content` block, and a layout that uses it:

```html
<!-- templates/layout.tmpl -->
{{ define "layout" }}
<!doctype html>
<html>
  <head><title>{{ .Title }}</title></head>
  <body>
    <header>My Site</header>
    {{ template "content" . }}
    <footer>{{ .Year }}</footer>
  </body>
</html>
{{ end }}
```

```html
<!-- templates/index.tmpl -->
{{ define "content" }}
  <h1>Hello, {{ .Name }}</h1>
{{ end }}
{{ template "layout" . }}
```

```go
c.HTML(200, "index.tmpl", gin.H{"Title": "Home", "Name": "yati", "Year": 2026})
```

The page template `define`s `content` and then invokes `layout`. The layout pulls `content` back in.

---

## 3. Multi-template, custom rendering, and static files

### Multiple template "sets" via `multitemplate`

When you have many pages and want each composed of layout + page + partials, `LoadHTMLGlob` becomes awkward because all templates share one global function map and namespace. The community renderer `github.com/gin-contrib/multitemplate` lets you register named template sets:

```go
import "github.com/gin-contrib/multitemplate"

func loadTemplates() multitemplate.Renderer {
    r := multitemplate.NewRenderer()
    r.AddFromFiles("index",     "templates/layout.tmpl", "templates/index.tmpl")
    r.AddFromFiles("users/list","templates/layout.tmpl", "templates/users/list.tmpl")
    r.AddFromFiles("users/show","templates/layout.tmpl", "templates/users/show.tmpl")
    return r
}

func main() {
    r := gin.Default()
    r.HTMLRender = loadTemplates()
    r.GET("/", func(c *gin.Context) {
        c.HTML(200, "index", gin.H{"Title": "Home"})
    })
}
```

This is the idiomatic pattern for non-trivial HTML apps.

### Static files

```go
r.Static("/static", "./static")                       // serve ./static/ at /static/*
r.StaticFile("/favicon.ico", "./static/favicon.ico")  // single file
r.StaticFS("/assets", http.Dir("./assets"))           // any http.FileSystem
```

Behind the scenes, `Static` registers `GET /static/*filepath` and calls `http.FileServer(http.Dir(...))`. You can swap the filesystem for anything that implements `http.FileSystem`, including an embedded one.

### `embed.FS` — ship templates + assets inside the binary

The killer feature in Go 1.16+ for web servers: embed your templates and static files into the binary so deployment is a single file with no sibling directories.

```go
package main

import (
    "embed"
    "html/template"
    "io/fs"
    "net/http"

    "github.com/gin-gonic/gin"
)

//go:embed templates/*.tmpl
var tmplFS embed.FS

//go:embed static
var staticFS embed.FS

func main() {
    r := gin.Default()

    // Templates from embed.FS
    tmpl := template.Must(template.New("").ParseFS(tmplFS, "templates/*.tmpl"))
    r.SetHTMLTemplate(tmpl)

    // Static files from embed.FS — strip the "static/" prefix
    staticSub, _ := fs.Sub(staticFS, "static")
    r.StaticFS("/static", http.FS(staticSub))

    r.GET("/", func(c *gin.Context) {
        c.HTML(200, "index.tmpl", gin.H{"Title": "Embedded", "Name": "yati"})
    })

    r.Run(":8080")
}
```

Build and ship:

```bash
go build -o app ./cmd/api
./app
# templates and static files are inside `app` — no other files needed
```

**This is how almost every modern Go web service should be packaged.** No "did you remember to copy the templates dir into the Docker image?" stories.

### Subtleties with `embed.FS`

- The `//go:embed` directive must be on a line immediately above the variable. Comments between them break it.
- Patterns are relative to the **package directory**, not the working directory.
- Hidden files (`.foo`) are excluded by default; use `all:templates` to include them.
- `embed.FS` is read-only and zero-allocation per request after startup.

### Gin's HTML escaping

`{{ .Comment }}` is escaped. To inject pre-trusted HTML (a CMS preview, sanitized markdown), use the `template.HTML` type:

```go
c.HTML(200, "post.tmpl", gin.H{
    "Body": template.HTML(sanitizedHTMLString),
})
```

Use `bluemonday` or similar to sanitize before wrapping in `template.HTML`. Wrapping unsanitized user input is a textbook XSS.

---

## 4. Practical application — a small "users" web app with layout, partials, and embedded assets

A real, runnable mini-app. Layout + index + user list, served from `embed.FS`, with a CSS file.

```text
project/
├── cmd/web/main.go
└── web/
    ├── templates/
    │   ├── layout.tmpl
    │   ├── index.tmpl
    │   └── users_list.tmpl
    └── static/
        └── app.css
```

```html
<!-- web/templates/layout.tmpl -->
{{ define "layout" }}
<!doctype html>
<html>
  <head>
    <title>{{ .Title }}</title>
    <link rel="stylesheet" href="/static/app.css">
  </head>
  <body>
    <nav><a href="/">home</a> | <a href="/users">users</a></nav>
    <main>{{ template "content" . }}</main>
  </body>
</html>
{{ end }}
```

```html
<!-- web/templates/index.tmpl -->
{{ define "content" }}
  <h1>Welcome</h1>
  <p>Visit <a href="/users">/users</a>.</p>
{{ end }}
{{ template "layout" . }}
```

```html
<!-- web/templates/users_list.tmpl -->
{{ define "content" }}
  <h1>Users</h1>
  <table>
    <thead><tr><th>ID</th><th>Name</th><th>Email</th></tr></thead>
    <tbody>
      {{ range .Users }}
        <tr><td>{{ .ID }}</td><td>{{ .Name }}</td><td>{{ .Email }}</td></tr>
      {{ end }}
    </tbody>
  </table>
{{ end }}
{{ template "layout" . }}
```

```css
/* web/static/app.css */
body { font-family: -apple-system, sans-serif; max-width: 720px; margin: 2rem auto; }
nav a { margin-right: 0.5rem; }
table { border-collapse: collapse; }
th, td { border: 1px solid #ddd; padding: 0.4rem 0.8rem; }
```

```go
// cmd/web/main.go
package main

import (
    "embed"
    "html/template"
    "io/fs"
    "net/http"

    "github.com/gin-gonic/gin"
)

//go:embed web/templates/*.tmpl
var tmplFS embed.FS

//go:embed web/static
var staticFS embed.FS

type User struct {
    ID    int64
    Name  string
    Email string
}

func main() {
    r := gin.Default()

    tmpl := template.Must(template.New("").ParseFS(tmplFS, "web/templates/*.tmpl"))
    r.SetHTMLTemplate(tmpl)

    sub, _ := fs.Sub(staticFS, "web/static")
    r.StaticFS("/static", http.FS(sub))

    r.GET("/", func(c *gin.Context) {
        c.HTML(200, "index.tmpl", gin.H{"Title": "Home"})
    })

    r.GET("/users", func(c *gin.Context) {
        c.HTML(200, "users_list.tmpl", gin.H{
            "Title": "Users",
            "Users": []User{
                {1, "yati", "y@example.com"},
                {2, "alex", "a@example.com"},
            },
        })
    })

    r.Run(":8080")
}
```

Build and run:

```bash
go build -o web ./cmd/web
./web
# open http://localhost:8080
```

The binary is self-contained. Drop it into a `FROM scratch` Docker image; nothing else to copy.

---

## 5. Common mistakes & gotchas

- **`text/template` for HTML.** Imports `text/template` instead of `html/template`. Result: no auto-escaping; XSS the moment any user input renders. Always import `html/template`.
- **Wrapping raw HTML strings in `template.HTML` without sanitization.** Same XSS bug, just opt-in. Use a sanitizer (`bluemonday`) at the boundary, never trust user-supplied HTML.
- **`SetFuncMap` *after* `LoadHTMLGlob`.** The funcs aren't in the parsed templates. You get "function 'upper' not defined" at execute time. Set funcs first.
- **Duplicate template basenames.** `users/list.tmpl` and `orders/list.tmpl` collide under `LoadHTMLGlob` — the second silently overwrites the first. Use `multitemplate` or explicit `ParseFiles` calls with renamed templates.
- **Forgetting `fs.Sub` with `embed.FS` for static.** `r.StaticFS("/static", http.FS(staticFS))` serves at `/static/web/static/app.css`. Use `fs.Sub` to strip the prefix.
- **Editing a template and seeing no change** — you're in `release` mode. Templates are parsed once at startup. Set `GIN_MODE=debug` (or restart) during development; module 01's Air covers the production-mode case via restart.
- **Mixing layout invocation patterns.** Either pages define `content` and call `template "layout"`, or pages extend layout via partials. Pick one across the codebase; switching mid-project is painful.
- **Putting business logic in templates.** Templates are dumb — if you find yourself reaching for `{{ if and (eq .Role "admin") (gt .Age 18) }}`, move the decision into Go and pass a single boolean.

---

## 🎯 Key Takeaways

1. **Use `embed.FS` for templates and static files** in any new Gin web app. A single deployable binary eliminates a whole class of "missing asset" deploy bugs.
2. **`html/template`, never `text/template`, for HTML.** Auto-escaping is the line between a safe app and an XSS demo.
3. **Set funcs *before* parsing**, and prefer `multitemplate` for any app with more than a handful of pages or shared partials.
4. **`r.StaticFS` accepts any `http.FileSystem`** — embedded, network-backed, even in-memory. Reach for `fs.Sub` to strip embed prefixes.
5. **In production, templates are parsed once.** That's a feature, not a bug — it's why server-rendered Go is fast. During development, `gin.DebugMode` re-parses per request; in production you restart.

*← [07 — Error Handling](./07_error_handling.md) | [09 — Database Integration →](./09_database_integration.md)*
