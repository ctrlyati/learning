# 12 — Testing

> **Goal:** Write robust Go tests using the built-in testing package — a skill every Go interviewer will probe.

---

## 1. Basic Test

```go
// File: mathutil/math.go
package mathutil

func Add(a, b int) int { return a + b }
func Subtract(a, b int) int { return a - b }
```

```go
// File: mathutil/math_test.go (must end with _test.go)
package mathutil

import "testing"

func TestAdd(t *testing.T) {
    result := Add(2, 3)
    if result != 5 {
        t.Errorf("Add(2, 3) = %d; want 5", result)
    }
}
```

```bash
go test ./...              # run all tests
go test ./mathutil/        # run tests in specific package
go test -v ./...           # verbose (shows test names)
go test -run TestAdd ./... # run tests matching pattern
```

---

## 2. Table-Driven Tests (Idiomatic Go)

The most common and recommended pattern:

```go
func TestAdd(t *testing.T) {
    tests := []struct {
        name     string
        a, b     int
        expected int
    }{
        {"positive numbers", 2, 3, 5},
        {"negative numbers", -1, -2, -3},
        {"zero", 0, 0, 0},
        {"mixed", -5, 5, 0},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := Add(tt.a, tt.b)
            if result != tt.expected {
                t.Errorf("Add(%d, %d) = %d; want %d", tt.a, tt.b, result, tt.expected)
            }
        })
    }
}
```

```bash
go test -run TestAdd/zero  # run specific sub-test
```

---

## 3. Testing Functions: t.Error vs t.Fatal

```go
func TestSomething(t *testing.T) {
    // t.Error — marks as failed, continues running
    t.Error("this failed but test continues")

    // t.Errorf — formatted error, continues
    t.Errorf("expected %d, got %d", 5, result)

    // t.Fatal — marks as failed, STOPS the test immediately
    t.Fatal("fatal error — test stops here")

    // t.Fatalf — formatted fatal
    t.Fatalf("cannot continue: %v", err)

    // t.Log — only visible with -v flag
    t.Log("debug info")

    // t.Skip — skip the test
    t.Skip("skipping until feature is ready")
}
```

---

## 4. Setup and Teardown

```go
func TestMain(m *testing.M) {
    // Global setup
    fmt.Println("Setting up test suite")
    
    code := m.Run()  // run all tests
    
    // Global teardown
    fmt.Println("Tearing down test suite")
    os.Exit(code)
}

// Per-test setup with t.Cleanup
func TestDatabase(t *testing.T) {
    db := setupTestDB(t)
    t.Cleanup(func() {
        db.Close()
    })
    
    // test using db...
}
```

---

## 5. Benchmark Tests

```go
func BenchmarkAdd(b *testing.B) {
    for i := 0; i < b.N; i++ {  // b.N is chosen by the framework
        Add(100, 200)
    }
}

// Benchmark with setup
func BenchmarkSort(b *testing.B) {
    data := generateData(10000)
    b.ResetTimer()  // don't count setup time
    
    for i := 0; i < b.N; i++ {
        sort.Ints(data)
    }
}
```

```bash
go test -bench=.           # run benchmarks
go test -bench=BenchmarkAdd -benchmem  # with memory stats
go test -bench=. -benchtime=5s         # run for 5 seconds
```

---

## 6. Test Coverage

```bash
go test -cover ./...                    # show coverage %
go test -coverprofile=coverage.out ./..
go tool cover -html=coverage.out        # open HTML report in browser
```

---

## 7. Mocking with Interfaces

```go
// Define interface for testability
type UserRepository interface {
    GetUser(id int) (*User, error)
    SaveUser(u *User) error
}

// Real implementation
type PostgresUserRepo struct{ db *sql.DB }
func (r *PostgresUserRepo) GetUser(id int) (*User, error) { ... }
func (r *PostgresUserRepo) SaveUser(u *User) error { ... }

// Mock for testing
type MockUserRepo struct {
    users map[int]*User
    err   error
}

func (m *MockUserRepo) GetUser(id int) (*User, error) {
    if m.err != nil {
        return nil, m.err
    }
    return m.users[id], nil
}

func (m *MockUserRepo) SaveUser(u *User) error { return m.err }

// Service using interface
type UserService struct {
    repo UserRepository
}

func (s *UserService) GetUser(id int) (*User, error) {
    return s.repo.GetUser(id)
}

// Test
func TestGetUser(t *testing.T) {
    mock := &MockUserRepo{
        users: map[int]*User{1: {ID: 1, Name: "Yati"}},
    }
    svc := &UserService{repo: mock}

    u, err := svc.GetUser(1)
    if err != nil {
        t.Fatal(err)
    }
    if u.Name != "Yati" {
        t.Errorf("expected Yati, got %s", u.Name)
    }
}
```

---

## 8. Subtests and Parallel Tests

```go
func TestConcurrent(t *testing.T) {
    tests := []struct {
        name  string
        input int
    }{
        {"small", 10},
        {"medium", 100},
        {"large", 1000},
    }

    for _, tt := range tests {
        tt := tt  // capture range variable
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()  // run subtests in parallel
            result := expensiveOperation(tt.input)
            // assert...
        })
    }
}
```

---

## 9. HTTP Handler Testing

```go
import (
    "net/http"
    "net/http/httptest"
    "testing"
)

func HelloHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("Hello, World!"))
}

func TestHelloHandler(t *testing.T) {
    req := httptest.NewRequest("GET", "/hello", nil)
    rr := httptest.NewRecorder()

    HelloHandler(rr, req)

    if rr.Code != http.StatusOK {
        t.Errorf("expected status 200, got %d", rr.Code)
    }
    if rr.Body.String() != "Hello, World!" {
        t.Errorf("unexpected body: %s", rr.Body.String())
    }
}
```

---

## 10. Testify (popular library)

```bash
go get github.com/stretchr/testify
```

```go
import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestWithTestify(t *testing.T) {
    result := Add(2, 3)
    
    assert.Equal(t, 5, result, "Add should return 5")
    assert.NotNil(t, result)
    assert.True(t, result > 0)

    // require stops the test on failure (like t.Fatal)
    require.NoError(t, someErr)
    require.NotNil(t, someValue)
}
```

---

## 🎯 Interview Tips

- **Q: How do you write tests in Go?** → Files ending in `_test.go`, functions starting with `Test`, accepting `*testing.T`.
- **Q: What is table-driven testing?** → A pattern where test cases are defined in a slice of structs, then iterated with `t.Run()`. Reduces code duplication and makes adding new cases trivial.
- **Q: What is the difference between `t.Error` and `t.Fatal`?** → `t.Error` marks the test as failed but continues. `t.Fatal` marks as failed and stops execution immediately (useful after nil checks).
- **Q: How do you mock in Go?** → Define an interface, implement a mock struct that satisfies it, inject it in tests. No frameworks required (though testify/mock and gomock are popular).
- **Q: How do you run a specific test?** → `go test -run TestName ./...`
- **Q: What is `t.Parallel()`?** → Marks the test to run in parallel with other parallel tests. Good for slow I/O-bound tests.
- **Q: How do you test HTTP handlers?** → Use `net/http/httptest` package — `httptest.NewRequest` and `httptest.NewRecorder`.

---

*Next: [13 — Standard Library →](./13_standard_library.md)*
