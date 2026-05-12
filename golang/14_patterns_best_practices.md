# 14 — Patterns & Best Practices

> **Goal:** Learn the idiomatic Go patterns that separate good Go developers from great ones — exactly what senior interviewers look for.

---

## 1. Functional Options Pattern

Clean way to handle optional configuration:

```go
type Server struct {
    host    string
    port    int
    timeout time.Duration
    maxConn int
}

type Option func(*Server)

func WithHost(host string) Option {
    return func(s *Server) { s.host = host }
}

func WithPort(port int) Option {
    return func(s *Server) { s.port = port }
}

func WithTimeout(t time.Duration) Option {
    return func(s *Server) { s.timeout = t }
}

func NewServer(opts ...Option) *Server {
    s := &Server{
        host:    "localhost",  // defaults
        port:    8080,
        timeout: 30 * time.Second,
    }
    for _, opt := range opts {
        opt(s)
    }
    return s
}

// Usage — only specify what you want to override
s := NewServer(
    WithHost("0.0.0.0"),
    WithPort(9090),
)
```

---

## 2. Options Struct Pattern

Alternative to functional options — simpler for many fields:

```go
type ServerConfig struct {
    Host    string
    Port    int
    Timeout time.Duration
}

func (c *ServerConfig) applyDefaults() {
    if c.Host == "" { c.Host = "localhost" }
    if c.Port == 0  { c.Port = 8080 }
    if c.Timeout == 0 { c.Timeout = 30 * time.Second }
}

func NewServer(cfg ServerConfig) *Server {
    cfg.applyDefaults()
    return &Server{/* ... */}
}
```

---

## 3. Constructor Functions (New*)

```go
type Queue struct {
    items []interface{}
    mu    sync.Mutex
}

// Always provide a constructor to enforce invariants
func NewQueue() *Queue {
    return &Queue{
        items: make([]interface{}, 0),
    }
}

func (q *Queue) Enqueue(item interface{}) {
    q.mu.Lock()
    defer q.mu.Unlock()
    q.items = append(q.items, item)
}

func (q *Queue) Dequeue() (interface{}, bool) {
    q.mu.Lock()
    defer q.mu.Unlock()
    if len(q.items) == 0 {
        return nil, false
    }
    item := q.items[0]
    q.items = q.items[1:]
    return item, true
}
```

---

## 4. Repository Pattern

```go
// Domain model
type User struct {
    ID    int
    Name  string
    Email string
}

// Repository interface
type UserRepository interface {
    FindByID(ctx context.Context, id int) (*User, error)
    FindAll(ctx context.Context) ([]*User, error)
    Save(ctx context.Context, u *User) error
    Delete(ctx context.Context, id int) error
}

// Implementation
type PostgresUserRepository struct {
    db *sql.DB
}

func (r *PostgresUserRepository) FindByID(ctx context.Context, id int) (*User, error) {
    u := &User{}
    err := r.db.QueryRowContext(ctx,
        "SELECT id, name, email FROM users WHERE id = $1", id,
    ).Scan(&u.ID, &u.Name, &u.Email)
    if err == sql.ErrNoRows {
        return nil, ErrNotFound
    }
    return u, err
}

// Service layer
type UserService struct {
    repo UserRepository
}

func NewUserService(repo UserRepository) *UserService {
    return &UserService{repo: repo}
}
```

---

## 5. Context Propagation

Always pass context as the first argument through call chains:

```go
func (s *UserService) GetUser(ctx context.Context, id int) (*User, error) {
    // Pass context to all downstream calls
    user, err := s.repo.FindByID(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("GetUser: %w", err)
    }

    // Check for cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    return user, nil
}

// HTTP handler — extract context from request
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()  // carries deadline, cancellation, values
    user, err := h.service.GetUser(ctx, userID)
    // ...
}
```

---

## 6. Generics (Go 1.18+)

```go
// Generic function
func Map[T, U any](slice []T, f func(T) U) []U {
    result := make([]U, len(slice))
    for i, v := range slice {
        result[i] = f(v)
    }
    return result
}

// Generic with constraints
type Number interface {
    ~int | ~int32 | ~int64 | ~float32 | ~float64
}

func Sum[T Number](nums []T) T {
    var total T
    for _, n := range nums {
        total += n
    }
    return total
}

// Generic data structure
type Stack[T any] struct {
    items []T
}

func (s *Stack[T]) Push(item T) { s.items = append(s.items, item) }

func (s *Stack[T]) Pop() (T, bool) {
    var zero T
    if len(s.items) == 0 {
        return zero, false
    }
    item := s.items[len(s.items)-1]
    s.items = s.items[:len(s.items)-1]
    return item, true
}

// Usage
nums := []int{1, 2, 3, 4, 5}
doubled := Map(nums, func(n int) int { return n * 2 })
fmt.Println(Sum(nums))       // 15
fmt.Println(Sum([]float64{1.1, 2.2}))  // 3.3
```

---

## 7. Error Handling — Wrap Don't Lose

```go
// Good — context preserved, original inspectable
func processUser(id int) error {
    user, err := db.FindUser(id)
    if err != nil {
        return fmt.Errorf("processUser(id=%d): %w", id, err)
    }
    return nil
}

// Bad — original error lost
return fmt.Errorf("processUser: %s", err)  // %s not %w — can't unwrap!

// Bad — no context
return err  // caller doesn't know where it came from
```

---

## 8. Graceful Shutdown

```go
func main() {
    srv := &http.Server{Addr: ":8080", Handler: router}

    go func() {
        if err := srv.ListenAndServe(); err != http.ErrServerClosed {
            log.Fatal(err)
        }
    }()

    // Wait for interrupt signal
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    
    log.Println("Shutting down server...")
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := srv.Shutdown(ctx); err != nil {
        log.Fatal("Server forced to shutdown:", err)
    }
    log.Println("Server exiting")
}
```

---

## 9. Worker Pool Pattern

```go
func workerPool(jobs <-chan int, results chan<- int, numWorkers int) {
    var wg sync.WaitGroup
    
    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for job := range jobs {
                results <- processJob(job)
            }
        }()
    }
    
    go func() {
        wg.Wait()
        close(results)
    }()
}

func main() {
    jobs := make(chan int, 100)
    results := make(chan int, 100)
    
    workerPool(jobs, results, 5)
    
    for i := 0; i < 50; i++ {
        jobs <- i
    }
    close(jobs)
    
    for result := range results {
        fmt.Println(result)
    }
}
```

---

## 10. Common Anti-Patterns to Avoid

```go
// ❌ Ignoring errors
result, _ := someFunction()  // never do this in production

// ❌ Naked return in long functions
func longFunction() (result string, err error) {
    // ... 50 lines of code ...
    return  // unclear what's being returned
}

// ❌ Using interface{} when you can be specific
func process(data interface{}) {}  // prefer a specific interface or generics

// ❌ Not closing resources
f, _ := os.Open("file.txt")
// missing: defer f.Close()

// ❌ Writing to a nil map
var m map[string]int
m["key"] = 1  // panic!

// ❌ Mutex copied by value
type MyStruct struct{ mu sync.Mutex }
s := MyStruct{}
s2 := s  // WRONG — copies the mutex, s2.mu is in wrong state

// ✅ Use pointer receiver for types with mutexes
func (s *MyStruct) DoSomething() { s.mu.Lock(); defer s.mu.Unlock() }
```

---

## 🎯 Interview Tips

- **Q: What is the functional options pattern?** → A way to provide optional configuration using variadic functions, allowing clean defaults and extensibility without breaking the API.
- **Q: How do you implement graceful shutdown?** → Use `os.Signal` with `signal.Notify`, then call `server.Shutdown(ctx)` with a timeout context.
- **Q: What are Go generics and when were they introduced?** → Go 1.18 (2022). They allow writing type-parametric functions and data structures using `[T any]` or constrained type parameters.
- **Q: What is a worker pool?** → A pattern where a fixed number of goroutines (workers) read from a shared jobs channel. Limits concurrency and resource usage.
- **Q: Why avoid copying a struct that contains a `sync.Mutex`?** → The mutex's internal state (locked/unlocked) is part of the struct's memory. Copying it copies the state, leading to undefined behavior.
- **Q: What is the "accept interfaces, return structs" principle?** → Functions should accept interface parameters (for flexibility and testability) but return concrete types (so callers get the full API without extra assertions).

---

*Next: [15 — Interview Q&A →](./15_interview_qa.md)*
