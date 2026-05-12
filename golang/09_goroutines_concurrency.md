# 09 — Goroutines & Concurrency

> **Goal:** Understand Go's lightweight concurrency model — one of its most celebrated features.

---

## 1. Goroutines

A goroutine is a lightweight thread managed by the Go runtime (not the OS):

```go
func sayHello(name string) {
    fmt.Println("Hello,", name)
}

func main() {
    go sayHello("Yati")  // start a goroutine with 'go' keyword
    go sayHello("Alice")
    go sayHello("Bob")

    time.Sleep(time.Millisecond * 100)  // wait for goroutines (naive approach)
    fmt.Println("main done")
}
```

- Goroutines start with ~2KB stack (vs ~1MB for OS threads)
- Can run millions concurrently
- Scheduled by Go runtime (M:N scheduling — M goroutines on N OS threads)

---

## 2. sync.WaitGroup — wait for goroutines to finish

```go
import "sync"

func worker(id int, wg *sync.WaitGroup) {
    defer wg.Done()  // signal completion when function returns
    fmt.Printf("Worker %d starting\n", id)
    time.Sleep(time.Millisecond * 100)
    fmt.Printf("Worker %d done\n", id)
}

func main() {
    var wg sync.WaitGroup

    for i := 1; i <= 5; i++ {
        wg.Add(1)          // increment counter before starting goroutine
        go worker(i, &wg)  // pass pointer to wg
    }

    wg.Wait()  // block until counter reaches 0
    fmt.Println("All workers completed")
}
```

---

## 3. Race Conditions

When multiple goroutines access shared data concurrently without synchronization:

```go
// UNSAFE — race condition
counter := 0
var wg sync.WaitGroup

for i := 0; i < 1000; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        counter++  // NOT atomic — read-modify-write is three operations!
    }()
}
wg.Wait()
fmt.Println(counter)  // Likely NOT 1000!
```

Detect races: `go run -race main.go`

---

## 4. sync.Mutex — mutual exclusion

```go
type SafeCounter struct {
    mu    sync.Mutex
    count int
}

func (c *SafeCounter) Increment() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count++
}

func (c *SafeCounter) Value() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.count
}

counter := &SafeCounter{}
var wg sync.WaitGroup

for i := 0; i < 1000; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        counter.Increment()
    }()
}
wg.Wait()
fmt.Println(counter.Value())  // Always 1000
```

---

## 5. sync.RWMutex — multiple readers, single writer

```go
type Cache struct {
    mu    sync.RWMutex
    store map[string]string
}

func (c *Cache) Get(key string) (string, bool) {
    c.mu.RLock()           // multiple readers can hold this simultaneously
    defer c.mu.RUnlock()
    val, ok := c.store[key]
    return val, ok
}

func (c *Cache) Set(key, value string) {
    c.mu.Lock()            // exclusive write lock
    defer c.mu.Unlock()
    c.store[key] = value
}
```

---

## 6. sync.Once — run something exactly once

```go
type Singleton struct {
    data string
}

var (
    instance *Singleton
    once     sync.Once
)

func GetInstance() *Singleton {
    once.Do(func() {
        instance = &Singleton{data: "initialized"}
    })
    return instance
}
```

---

## 7. sync.Map — concurrent-safe map

```go
var m sync.Map

// Store
m.Store("key1", "value1")
m.Store("key2", 42)

// Load
val, ok := m.Load("key1")
if ok {
    fmt.Println(val)
}

// LoadOrStore — atomic check-and-set
actual, loaded := m.LoadOrStore("key3", "default")
fmt.Println(actual, loaded)

// Delete
m.Delete("key1")

// Range
m.Range(func(key, value interface{}) bool {
    fmt.Printf("%v: %v\n", key, value)
    return true  // return false to stop
})
```

---

## 8. Atomic Operations

```go
import "sync/atomic"

var counter int64

// Atomic increment (no mutex needed)
atomic.AddInt64(&counter, 1)

// Atomic load/store
val := atomic.LoadInt64(&counter)
atomic.StoreInt64(&counter, 100)

// Compare and swap
swapped := atomic.CompareAndSwapInt64(&counter, 100, 200)
fmt.Println(swapped)  // true if counter was 100
```

---

## 9. Context — cancellation & timeouts

```go
import "context"

// Cancel context
ctx, cancel := context.WithCancel(context.Background())
defer cancel()  // always defer cancel to avoid goroutine leaks

go func(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            fmt.Println("goroutine cancelled:", ctx.Err())
            return
        default:
            fmt.Println("working...")
            time.Sleep(100 * time.Millisecond)
        }
    }
}(ctx)

time.Sleep(300 * time.Millisecond)
cancel()  // signal cancellation
time.Sleep(50 * time.Millisecond)
```

### Timeout context

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.example.com", nil)
resp, err := http.DefaultClient.Do(req)
if errors.Is(err, context.DeadlineExceeded) {
    fmt.Println("request timed out")
}
```

### Context values (use sparingly)

```go
type ctxKey string

const userIDKey ctxKey = "userID"

// Store
ctx := context.WithValue(context.Background(), userIDKey, 42)

// Retrieve
if uid, ok := ctx.Value(userIDKey).(int); ok {
    fmt.Println("User ID:", uid)
}
```

---

## 10. Goroutine Leaks

A goroutine leak occurs when goroutines are created but never terminate:

```go
// LEAK — no way to signal this goroutine to stop
func leaky() {
    go func() {
        for {
            time.Sleep(time.Second)
            doWork()
        }
    }()
}

// FIXED — use context for cancellation
func fixed(ctx context.Context) {
    go func() {
        for {
            select {
            case <-ctx.Done():
                return  // clean exit
            case <-time.After(time.Second):
                doWork()
            }
        }
    }()
}
```

---

## 🎯 Interview Tips

- **Q: What is a goroutine?** → A lightweight, cooperatively scheduled thread managed by the Go runtime. Starts with ~2KB stack. You can run millions simultaneously.
- **Q: What is the difference between concurrency and parallelism?** → Concurrency is about structuring code to handle multiple things at once. Parallelism is running them literally simultaneously. Go provides concurrency; parallelism depends on `GOMAXPROCS` and available cores.
- **Q: What is a race condition?** → When two or more goroutines access shared memory without synchronization, and at least one is a write. Use `-race` flag to detect.
- **Q: When to use channels vs mutexes?** → Channels: pass data between goroutines, signal events. Mutex: protect shared state that multiple goroutines read/write. Go's motto: "Share memory by communicating, not communicate by sharing memory."
- **Q: What causes goroutine leaks?** → Goroutines blocked on channels with no sender, or infinite loops without a cancellation mechanism. Always provide a way to stop goroutines.
- **Q: What is GOMAXPROCS?** → Controls how many OS threads run Go code simultaneously. Default is the number of CPU cores. `runtime.GOMAXPROCS(n)` to change.
- **Q: What does `sync.Once` do?** → Ensures a function runs exactly once, even when called from multiple goroutines. Ideal for lazy initialization and singletons.

---

*Next: [10 — Channels →](./10_channels.md)*
