# 08 — Error Handling

> **Goal:** Understand Go's explicit, idiomatic approach to error handling — no exceptions, just values.

---

## 1. The error Interface

```go
// Built into Go — just an interface
type error interface {
    Error() string
}
```

---

## 2. Basic Error Handling Pattern

```go
// Functions return (result, error)
func divide(a, b float64) (float64, error) {
    if b == 0 {
        return 0, errors.New("cannot divide by zero")
    }
    return a / b, nil
}

result, err := divide(10, 0)
if err != nil {
    fmt.Println("Error:", err)
    return
}
fmt.Println("Result:", result)
```

**Rule:** Always handle errors. Ignoring them with `_` is a code smell.

---

## 3. Creating Errors

```go
import (
    "errors"
    "fmt"
)

// Simple error
err1 := errors.New("something went wrong")

// Formatted error
err2 := fmt.Errorf("failed to process item %d: %s", 42, "not found")

// Both implement the error interface
```

---

## 4. Custom Error Types

```go
// Custom error with extra fields
type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("validation failed for field %s: %s", e.Field, e.Message)
}

func validateAge(age int) error {
    if age < 0 {
        return &ValidationError{Field: "age", Message: "must be non-negative"}
    }
    if age > 150 {
        return &ValidationError{Field: "age", Message: "unrealistically large"}
    }
    return nil
}

err := validateAge(-5)
if err != nil {
    fmt.Println(err)
    // cast to access fields:
    var ve *ValidationError
    if errors.As(err, &ve) {
        fmt.Println("Field:", ve.Field)
    }
}
```

---

## 5. Error Wrapping (Go 1.13+)

Wrap errors to add context while preserving the original:

```go
// Wrap with %w
func readConfig(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return fmt.Errorf("readConfig: %w", err)
    }
    _ = data
    return nil
}

// Unwrap chain
err := readConfig("/nonexistent/path")
fmt.Println(err)
// readConfig: open /nonexistent/path: no such file or directory

// errors.Is — checks error chain for target
if errors.Is(err, os.ErrNotExist) {
    fmt.Println("File does not exist")
}

// errors.As — finds first matching type in chain
var pathErr *os.PathError
if errors.As(err, &pathErr) {
    fmt.Println("Path:", pathErr.Path)
}
```

### Wrapping chain example

```go
var ErrNotFound = errors.New("not found")

func getUser(id int) (*User, error) {
    if id == 0 {
        return nil, fmt.Errorf("getUser(id=%d): %w", id, ErrNotFound)
    }
    return &User{}, nil
}

func handleRequest(id int) error {
    _, err := getUser(id)
    if err != nil {
        return fmt.Errorf("handleRequest: %w", err)
    }
    return nil
}

err := handleRequest(0)
// handleRequest: getUser(id=0): not found

fmt.Println(errors.Is(err, ErrNotFound))  // true!
```

---

## 6. errors.Is vs errors.As

```go
var ErrPermission = errors.New("permission denied")

// errors.Is — value comparison, walks the error chain
errors.Is(err, ErrPermission)     // true if err or any wrapped error == ErrPermission

// errors.As — type comparison, walks the error chain
var netErr *net.OpError
errors.As(err, &netErr)           // true if err or any wrapped error is *net.OpError
```

---

## 7. Sentinel Errors

Pre-defined error values in a package:

```go
// Define sentinel errors
var (
    ErrNotFound      = errors.New("not found")
    ErrUnauthorized  = errors.New("unauthorized")
    ErrInvalidInput  = errors.New("invalid input")
)

func findUser(id int) (*User, error) {
    if id <= 0 {
        return nil, ErrInvalidInput
    }
    // ... lookup
    return nil, ErrNotFound
}

u, err := findUser(-1)
switch {
case errors.Is(err, ErrInvalidInput):
    fmt.Println("bad request")
case errors.Is(err, ErrNotFound):
    fmt.Println("user not found")
case err != nil:
    fmt.Println("internal error:", err)
default:
    fmt.Println("user:", u)
}
```

---

## 8. panic and recover

Use sparingly — only for unrecoverable programming errors:

```go
// panic — terminates the goroutine, prints stack trace
func mustPositive(n int) int {
    if n <= 0 {
        panic(fmt.Sprintf("expected positive, got %d", n))
    }
    return n
}

// recover — catches a panic in the same goroutine
func safeDiv(a, b int) (result int, err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("recovered from panic: %v", r)
        }
    }()
    result = a / b  // panics if b == 0
    return
}

r, err := safeDiv(10, 0)
fmt.Println(r, err)  // 0, recovered from panic: runtime error: integer divide by zero
```

### When to use panic

✅ Programmer errors that should never happen (nil pointer from bad initialization)  
✅ `Must*` wrapper functions (e.g., `template.Must`, `regexp.MustCompile`)  
❌ Normal error conditions (use `error` return instead)  
❌ Network, file, or user input errors

---

## 9. Error Handling Patterns

### Sentinel + typed errors

```go
// errors.go in your package
var (
    ErrNotFound = errors.New("not found")
    ErrConflict = errors.New("conflict")
)

type DBError struct {
    Code    int
    Message string
    Err     error
}

func (e *DBError) Error() string {
    return fmt.Sprintf("db error %d: %s", e.Code, e.Message)
}

func (e *DBError) Unwrap() error { return e.Err }
```

### errgroup for concurrent errors

```go
import "golang.org/x/sync/errgroup"

g, ctx := errgroup.WithContext(context.Background())

g.Go(func() error {
    return fetchData(ctx, "endpoint1")
})
g.Go(func() error {
    return fetchData(ctx, "endpoint2")
})

if err := g.Wait(); err != nil {
    log.Fatal(err)
}
```

---

## 🎯 Interview Tips

- **Q: Why does Go use error values instead of exceptions?** → Errors are explicit, predictable, and force the caller to handle them. Exceptions can be hard to trace and create implicit control flow.
- **Q: What is the difference between `errors.Is` and `errors.As`?** → `errors.Is` checks if any error in the chain matches a specific value (good for sentinel errors). `errors.As` checks if any error in the chain matches a specific type (good for extracting typed error info).
- **Q: What is error wrapping and why use it?** → Adding context to an error with `fmt.Errorf("...: %w", err)`. The `%w` verb wraps the original so it can still be inspected with `errors.Is`/`errors.As`.
- **Q: When should you use panic?** → Only for unrecoverable programming errors or in `Must*` initialization functions. Never for normal business logic errors.
- **Q: What is a sentinel error?** → A package-level error variable like `io.EOF` or `sql.ErrNoRows` that callers can compare against with `errors.Is`.
- **Q: How do you implement the Unwrap() method?** → Return the wrapped error: `func (e *MyError) Unwrap() error { return e.cause }`. This makes `errors.Is` and `errors.As` traverse the chain.

---

*Next: [09 — Goroutines & Concurrency →](./09_goroutines_concurrency.md)*
