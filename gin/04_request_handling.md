# 04 — Request Handling: Binding, Validation, Uploads

> **Goal:** Master every Gin binding API — JSON, form, query, URI, header — wire up `validator/v10`, write custom validators, and handle multipart file uploads correctly.

---

## 1. Binding — mental model + working code

**Binding** = "populate this Go struct from the incoming request, validating along the way." Gin uses struct tags as the contract:

```go
type CreateUserRequest struct {
    Email    string `json:"email"    form:"email"    binding:"required,email"`
    Username string `json:"username" form:"username" binding:"required,min=3,max=32"`
    Age      int    `json:"age"      form:"age"      binding:"required,gte=13,lte=120"`
    Role     string `json:"role"     form:"role"     binding:"omitempty,oneof=admin user guest"`
}
```

Three things happen when you call a Bind method:

1. Gin picks a decoder based on either the `Content-Type` header (`Bind`) or what you asked for explicitly (`BindJSON`, `BindQuery`, etc.).
2. The decoder populates fields by matching struct tags.
3. The validator (`go-playground/validator/v10`) walks the populated struct and checks each `binding:"..."` rule.

If any step fails, you get a non-nil `error`.

### `Bind*` vs `ShouldBind*`

| Family | On error | Use when |
|--------|----------|----------|
| `c.Bind*(...)` | Writes 400 and aborts the chain for you | You want default behavior and a simple handler |
| `c.ShouldBind*(...)` | Returns the error; you decide what to do | Almost always — you want control of the error format |

**Use `ShouldBind*` in real code.** `Bind` produces Gin's default error JSON, which usually doesn't match your API's error envelope. Module 07 wires this into a central error handler.

```go
func createUser(c *gin.Context) {
    var req CreateUserRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": "invalid request", "details": err.Error()})
        return
    }
    // ... use req
    c.JSON(201, gin.H{"id": 42, "email": req.Email})
}
```

```bash
curl -i -X POST -H 'Content-Type: application/json' \
  -d '{"email":"a@b.com","username":"yati","age":30}' \
  http://localhost:8080/users
# 201
curl -i -X POST -H 'Content-Type: application/json' \
  -d '{"email":"not-an-email","username":"y","age":5}' \
  http://localhost:8080/users
# 400, validation errors
```

---

## 2. How binding works under the hood

### The dispatch table

`c.ShouldBind(&v)` looks at the request's `Content-Type` header and picks one of:

| Content-Type | Binder |
|-------------|--------|
| `application/json` | `binding.JSON` (uses `encoding/json`) |
| `application/xml`, `text/xml` | `binding.XML` |
| `application/x-yaml`, `application/yaml` | `binding.YAML` |
| `application/x-www-form-urlencoded` | `binding.Form` |
| `multipart/form-data` | `binding.FormMultipart` |
| `application/x-protobuf` | `binding.ProtoBuf` |

For GET requests it defaults to `binding.Form` (which reads from query). If you want guarantees, use the specific method (`ShouldBindJSON`) — it ignores `Content-Type` and always decodes as JSON.

### The decoders

- `ShouldBindJSON` → `json.NewDecoder(c.Request.Body).Decode(&v)`
- `ShouldBindXML` → `xml.NewDecoder(...).Decode(&v)`
- `ShouldBindQuery` → reads from `c.Request.URL.Query()`, populates via `form` tags
- `ShouldBindUri` → reads from path params (`c.Params`), populates via `uri` tags
- `ShouldBindHeader` → reads request headers, populates via `header` tags

### The validator

After decoding, Gin invokes `binding.Validator.ValidateStruct(v)`. By default this is a wrapper around `go-playground/validator/v10`. You can swap it for any validator implementing the `binding.StructValidator` interface — sometimes used to install custom error translators.

### Required vs zero values

`binding:"required"` rejects the zero value (`""`, `0`, `false`, nil). To accept zero values, drop `required`:

```go
type Filter struct {
    Active *bool `json:"active" binding:"omitempty"`
}
```

`*bool` lets you distinguish "absent" (`nil`), `false`, and `true`. Pointer types are the standard Go way to express "optional" in a JSON contract.

---

## 3. The full binding surface

### JSON body

```go
type LoginReq struct {
    Email    string `json:"email"    binding:"required,email"`
    Password string `json:"password" binding:"required,min=8"`
}
var req LoginReq
if err := c.ShouldBindJSON(&req); err != nil { /* 400 */ }
```

### Form (`application/x-www-form-urlencoded`)

```go
type LoginForm struct {
    Email    string `form:"email"    binding:"required,email"`
    Password string `form:"password" binding:"required,min=8"`
}
var f LoginForm
if err := c.ShouldBind(&f); err != nil { /* 400 */ }
```

```bash
curl -i -d 'email=a@b.com&password=correcthorse' http://localhost:8080/login
```

### Query

```go
type ListParams struct {
    Page  int    `form:"page,default=1"      binding:"gte=1"`
    Limit int    `form:"limit,default=20"    binding:"gte=1,lte=100"`
    Sort  string `form:"sort,default=-created_at"`
    Tags  []string `form:"tag"`               // ?tag=a&tag=b → ["a","b"]
}
var p ListParams
if err := c.ShouldBindQuery(&p); err != nil { /* 400 */ }
```

`form:"page,default=1"` is a Gin-specific convenience — if the param is absent, the field is set to `1` before validation.

### URI (path) params

```go
type IDParam struct {
    ID int64 `uri:"id" binding:"required,gt=0"`
}
r.GET("/users/:id", func(c *gin.Context) {
    var p IDParam
    if err := c.ShouldBindUri(&p); err != nil {
        c.JSON(400, gin.H{"error": "bad id"})
        return
    }
    // use p.ID
})
```

Much cleaner than the `strconv.Atoi(c.Param("id"))` dance, and it validates in the same step.

### Headers

```go
type HdrReq struct {
    Token         string `header:"X-Api-Key"        binding:"required,len=32"`
    AcceptVersion string `header:"Accept-Version"   binding:"omitempty,oneof=v1 v2"`
}
var h HdrReq
if err := c.ShouldBindHeader(&h); err != nil { /* 400 */ }
```

### Combined — `should` binding chain

A request can have all of them at once: URI + query + header + body. Bind each separately:

```go
func updateUser(c *gin.Context) {
    var uri IDParam
    if err := c.ShouldBindUri(&uri); err != nil { c.JSON(400, errJSON(err)); return }

    var body UpdateUserReq
    if err := c.ShouldBindJSON(&body); err != nil { c.JSON(400, errJSON(err)); return }

    // use uri.ID and body
}
```

### Common validator tags

| Tag | Effect |
|-----|--------|
| `required` | Reject zero value |
| `omitempty` | Skip other rules if value is zero |
| `email` | Valid email per RFC 5322 |
| `url` | Valid URL |
| `uuid` / `uuid4` | UUID format |
| `min=N` / `max=N` | Length (strings, slices) or numeric bound |
| `gte=N` / `lte=N` / `gt=N` / `lt=N` | Numeric comparisons |
| `len=N` | Exact length |
| `oneof=a b c` | Value must be one of the listed |
| `eqfield=Other` / `nefield=Other` | Cross-field comparison |
| `dive` | Apply nested rules to each element of a slice/map |
| `alphanum` / `numeric` / `ascii` | Character class |
| `e164` | E.164 phone number |
| `datetime=2006-01-02` | Parseable with Go's reference time layout |

Full reference: <https://pkg.go.dev/github.com/go-playground/validator/v10>.

### Custom validators

Register at startup, then use in tags:

```go
import (
    "github.com/gin-gonic/gin/binding"
    "github.com/go-playground/validator/v10"
)

func init() {
    if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
        _ = v.RegisterValidation("not_admin", func(fl validator.FieldLevel) bool {
            return fl.Field().String() != "admin"
        })
    }
}

type CreateUserReq struct {
    Username string `json:"username" binding:"required,not_admin"`
}
```

### Translating validation errors

The errors from `validator/v10` are a slice (`validator.ValidationErrors`). Each element has the field name, the tag that failed, the bad value. Translate them into your error envelope:

```go
func bindErr(err error) gin.H {
    var ves validator.ValidationErrors
    if errors.As(err, &ves) {
        out := make([]gin.H, 0, len(ves))
        for _, fe := range ves {
            out = append(out, gin.H{
                "field":  fe.Field(),
                "rule":   fe.Tag(),
                "param":  fe.Param(),
                "got":    fe.Value(),
            })
        }
        return gin.H{"error": "validation failed", "fields": out}
    }
    return gin.H{"error": err.Error()}
}
```

### File uploads

Single file:

```go
r.POST("/upload", func(c *gin.Context) {
    file, err := c.FormFile("file")
    if err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    // file is *multipart.FileHeader; save it:
    if err := c.SaveUploadedFile(file, "./uploads/"+file.Filename); err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{
        "filename": file.Filename,
        "size":     file.Size,
        "header":   file.Header.Get("Content-Type"),
    })
})
```

Multiple files:

```go
r.POST("/upload-many", func(c *gin.Context) {
    form, err := c.MultipartForm()
    if err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    files := form.File["files"]
    for _, f := range files {
        _ = c.SaveUploadedFile(f, "./uploads/"+f.Filename)
    }
    c.JSON(200, gin.H{"count": len(files)})
})
```

Configure max upload size:

```go
r.MaxMultipartMemory = 8 << 20 // 8 MiB in memory; larger goes to disk
```

This is **only** the in-memory threshold. Total upload size is bounded by `c.Request.Body` — if you want an absolute cap, wrap the body:

```go
c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 50<<20) // 50 MiB hard cap
```

---

## 4. Practical application — typed create-user endpoint with full validation

```go
// internal/http/handlers/users.go
package handlers

import (
    "errors"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/gin-gonic/gin/binding"
    "github.com/go-playground/validator/v10"
)

type CreateUserReq struct {
    Email    string   `json:"email"    binding:"required,email"`
    Username string   `json:"username" binding:"required,min=3,max=32,alphanum,not_admin"`
    Password string   `json:"password" binding:"required,min=8,max=72"`
    Confirm  string   `json:"confirm"  binding:"required,eqfield=Password"`
    Age      int      `json:"age"      binding:"required,gte=13,lte=120"`
    Role     string   `json:"role"     binding:"omitempty,oneof=admin user guest"`
    Tags     []string `json:"tags"     binding:"omitempty,dive,min=1,max=20"`
}

type CreateUserResp struct {
    ID       int64  `json:"id"`
    Email    string `json:"email"`
    Username string `json:"username"`
}

func init() {
    if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
        _ = v.RegisterValidation("not_admin", func(fl validator.FieldLevel) bool {
            return fl.Field().String() != "admin"
        })
    }
}

func CreateUser(c *gin.Context) {
    var req CreateUserReq
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, bindErr(err))
        return
    }

    // ... save to DB ...
    resp := CreateUserResp{ID: 42, Email: req.Email, Username: req.Username}
    c.JSON(http.StatusCreated, resp)
}

func bindErr(err error) gin.H {
    var ves validator.ValidationErrors
    if errors.As(err, &ves) {
        out := make([]gin.H, 0, len(ves))
        for _, fe := range ves {
            out = append(out, gin.H{
                "field": fe.Field(),
                "rule":  fe.Tag(),
                "param": fe.Param(),
            })
        }
        return gin.H{"error": "validation failed", "fields": out}
    }
    return gin.H{"error": err.Error()}
}
```

Wire it:

```go
// cmd/api/main.go
r := gin.Default()
r.POST("/users", handlers.CreateUser)
r.Run(":8080")
```

Test cases:

```bash
# Happy path
curl -i -X POST -H 'Content-Type: application/json' \
  -d '{"email":"a@b.com","username":"yati","password":"correcthorse","confirm":"correcthorse","age":30,"role":"user","tags":["go","gin"]}' \
  http://localhost:8080/users
# 201

# Validation: mismatched confirm
curl -i -X POST -H 'Content-Type: application/json' \
  -d '{"email":"a@b.com","username":"yati","password":"correcthorse","confirm":"wrong","age":30}' \
  http://localhost:8080/users
# 400, fields: [{field: Confirm, rule: eqfield}]

# Validation: reserved username
curl -i -X POST -H 'Content-Type: application/json' \
  -d '{"email":"a@b.com","username":"admin","password":"correcthorse","confirm":"correcthorse","age":30}' \
  http://localhost:8080/users
# 400, fields: [{field: Username, rule: not_admin}]
```

---

## 5. Common mistakes & gotchas

- **Using `c.Bind*` and then writing your own error response.** `Bind` already wrote one. The second `c.JSON` is a no-op (and logs a warning). Use `ShouldBind*` when you want to control the response.
- **Forgetting `Content-Type: application/json` on `curl`.** Gin's `Bind` picks the form binder and your `email` field shows up empty. Either always use explicit `ShouldBindJSON`, or always set the header.
- **`binding:"required"` on a `bool`** — `false` is the zero value, so `required` rejects it. Use `*bool` if you need to distinguish "false" from "absent."
- **Multiple body reads.** `c.Request.Body` is a one-shot stream. If a middleware logs the body and a handler tries to bind, the handler sees EOF. Buffer the body in the middleware and restore it.
- **Forgetting `dive` on slices of structs.** `binding:"required"` on `[]Foo` only checks the slice itself, not the elements. Use `binding:"required,dive"` to apply rules to each element.
- **`form:"name,default=X"` doesn't work in JSON.** That syntax is for the form binder only. For JSON, set defaults in a separate step after `ShouldBindJSON`.
- **Validation tags on unexported fields.** Validator can't see lower-case fields. All bound fields must be exported.
- **Not capping upload size.** `r.MaxMultipartMemory` controls memory vs disk spill, not total size. Use `http.MaxBytesReader` for a real cap, or you've shipped a DoS.
- **Reusing a struct for both create and update with one validation tag set.** Updates often allow partial fields; creates require them. Use two structs (`CreateFooReq`, `UpdateFooReq`) — duplicating five lines is cheaper than the bug where `required` fires on a PATCH.

---

## 🎯 Key Takeaways

1. **The struct definition is the API contract.** Tags govern decoding, validation, and (with `swaggo`) OpenAPI generation. Treat changes to tags like API-breaking changes.
2. **Prefer `ShouldBind*` over `Bind*`** so you control error responses. Wrap `validator.ValidationErrors` into a structured field-level error format consumers can parse.
3. **`ShouldBindUri` and `ShouldBindHeader` exist** — use them. They replace ad-hoc `strconv.Atoi(c.Param("id"))` and `c.GetHeader` parsing with a single typed, validated struct.
4. **Pointers for optional JSON fields.** `*bool`, `*int`, `*string` distinguish "absent" from "zero" — this is the idiomatic Go way to model optional inputs.
5. **Cap multipart uploads at the body level**, not just the in-memory threshold. `http.MaxBytesReader` is the missing line in nearly every Gin upload tutorial — and it is the difference between a feature and a DoS.

*← [03 — Context Deep Dive](./03_context_deep_dive.md) | [05 — Response Handling →](./05_response_handling.md)*
