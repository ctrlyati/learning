# 11 — Packages & Modules

> **Goal:** Understand Go's code organization system — packages for code structure, modules for dependency management.

---

## 1. Packages

Every Go file belongs to a package:

```go
// File: mathutil/math.go
package mathutil   // package declaration — must match folder name

// Exported (uppercase) — visible outside the package
func Add(a, b int) int {
    return a + b
}

// Unexported (lowercase) — package-private
func helper(n int) int {
    return n * 2
}
```

---

## 2. Importing Packages

```go
package main

import (
    "fmt"                          // standard library
    "math/rand"                    // standard library subfolder
    "github.com/user/project/util" // external package

    // Aliasing
    myfmt "fmt"
    
    // Blank import — runs init(), used for side effects
    _ "github.com/lib/pq"  // registers postgres driver
)

func main() {
    fmt.Println(rand.Intn(100))
}
```

---

## 3. Go Modules (go mod)

Modules manage your project's dependencies:

```bash
# Initialize a new module
go mod init github.com/yourname/myproject

# This creates go.mod
```

### go.mod

```
module github.com/yourname/myproject

go 1.21

require (
    github.com/gorilla/mux v1.8.0
    github.com/lib/pq v1.10.7
)
```

### go.sum

Auto-generated file with cryptographic hashes to verify downloads.

---

## 4. Common go Commands

```bash
# Add a dependency
go get github.com/gorilla/mux@latest
go get github.com/gorilla/mux@v1.8.0

# Remove unused dependencies
go mod tidy

# Download all dependencies
go mod download

# List all dependencies
go list -m all

# Show dependency graph
go mod graph

# Verify dependencies haven't been tampered with
go mod verify

# Vendor dependencies (copy to ./vendor/)
go mod vendor
```

---

## 5. Project Structure

```
myproject/
├── go.mod
├── go.sum
├── main.go              # package main
├── cmd/
│   ├── server/
│   │   └── main.go      # entry point for server
│   └── cli/
│       └── main.go      # entry point for CLI
├── internal/            # private packages (cannot be imported externally)
│   ├── auth/
│   │   └── auth.go      # package auth
│   └── db/
│       └── db.go        # package db
├── pkg/                 # public packages (can be imported externally)
│   └── mathutil/
│       ├── math.go
│       └── math_test.go
└── api/
    └── handler.go
```

### `internal` package rule

```go
// Any code inside /internal/ can ONLY be imported by code in the parent of /internal/
// This is enforced by the Go compiler.

// OK: myproject/cmd/server/main.go importing myproject/internal/auth
import "myproject/internal/auth"

// ERROR: another-project importing myproject/internal/auth
import "myproject/internal/auth"  // compile error!
```

---

## 6. Package Visibility Rules

| Name | Visibility |
|------|-----------|
| `Uppercase` | Exported — visible from other packages |
| `lowercase` | Unexported — package-private |

```go
package user

type User struct {          // Exported type
    ID    int               // Exported field
    Name  string            // Exported field
    token string            // Unexported field
}

func NewUser(name string) *User {   // Exported constructor
    return &User{
        Name:  name,
        token: generateToken(),
    }
}

func generateToken() string {      // Unexported helper
    return "secret"
}
```

---

## 7. Multiple Files in One Package

All files in the same directory share the same package:

```go
// File: shapes/circle.go
package shapes

type Circle struct{ Radius float64 }
func (c Circle) Area() float64 { return math.Pi * c.Radius * c.Radius }

// File: shapes/rectangle.go
package shapes

type Rectangle struct{ Width, Height float64 }
func (r Rectangle) Area() float64 { return r.Width * r.Height }

// Both belong to package "shapes" — can access each other's unexported symbols
```

---

## 8. init() in Packages

```go
package db

import "database/sql"

var DB *sql.DB

func init() {
    var err error
    DB, err = sql.Open("postgres", "...")
    if err != nil {
        panic(err)
    }
}
```

Init execution order:
1. Package-level variable declarations (in dependency order)
2. `init()` functions (in file order, then declaration order within file)
3. `main()` in the main package

---

## 9. Build Tags

Conditionally include files:

```go
//go:build linux
// +build linux  (older syntax, still needed for Go < 1.17)

package main

func platformSpecific() {
    fmt.Println("Linux!")
}
```

```bash
go build -tags integration ./...
```

---

## 🎯 Interview Tips

- **Q: What is the difference between a package and a module?** → A package is a directory of Go source files sharing the same `package` name. A module is a collection of packages versioned together, defined by `go.mod`.
- **Q: What does the `internal` directory enforce?** → The Go compiler enforces that packages inside `internal/` can only be imported by code in the parent directory tree. Used to hide implementation details.
- **Q: What is a blank import (`_ "pkg"`)** → Imports a package only for its side effects (running its `init()` function). Commonly used to register database drivers or image codecs.
- **Q: What does `go mod tidy` do?** → Removes unused dependencies from `go.mod`/`go.sum` and adds any missing ones.
- **Q: Can two files in the same directory have different package names?** → No (except `_test` suffix is allowed for test files).
- **Q: How do you import a local package?** → By its module path, e.g., `import "github.com/you/project/mypkg"` where the module is `github.com/you/project`.

---

*Next: [12 — Testing →](./12_testing.md)*
