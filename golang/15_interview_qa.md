# 15 — Interview Q&A

> **Goal:** 50+ most common Go interview questions with concise, complete answers. Review these before your interview!

---

## 🟢 Basics

**Q1: What is Go and who created it?**  
Go (Golang) was created by Google engineers Robert Griesemer, Rob Pike, and Ken Thompson. Released in 2009, it's a statically typed, compiled language designed for simplicity, performance, and built-in concurrency.

**Q2: What are Go's main advantages?**  
Fast compilation, simple syntax, built-in concurrency (goroutines + channels), garbage collection, strong standard library, static typing with type inference, cross-platform builds, and excellent tooling.

**Q3: What is the zero value in Go?**  
Every type has a zero value when declared without initialization:
- `int`, `float64` → `0`
- `bool` → `false`
- `string` → `""`
- pointer, slice, map, channel, func, interface → `nil`

**Q4: What is the difference between `var x int` and `x := 0`?**  
Both create an `int` variable with value 0. `:=` is shorthand (short variable declaration) that infers type and can only be used inside functions. `var` can be used at package level.

**Q5: Can you have unused imports or variables in Go?**  
No. The compiler rejects unused local variables and unused imports. Unused package-level variables are allowed.

**Q6: What is `iota`?**  
A special constant auto-incrementer used in `const` blocks, starting at 0 and incrementing by 1 for each const spec.

---

## 🔵 Types & Interfaces

**Q7: What is the difference between an array and a slice?**  
- Array: fixed size, value type, `[N]T` — copying makes an independent duplicate.
- Slice: dynamic, reference type, `[]T` — a view (pointer, length, capacity) into an underlying array. Appending may create a new backing array.

**Q8: How does `append` work when capacity is exceeded?**  
Go allocates a new underlying array (typically doubling capacity), copies all elements, and returns a new slice header pointing to the new array.

**Q9: What is the difference between `make` and `new`?**  
- `new(T)` — allocates a zero-value T and returns a `*T`.
- `make(T, ...)` — creates and initializes slices, maps, and channels. Returns `T` (not a pointer).

**Q10: How does Go implement interfaces?**  
Implicitly — any type that has all the methods of an interface satisfies it, with no explicit `implements` declaration. This is called structural typing (duck typing).

**Q11: What is an interface value composed of?**  
An interface value is a tuple of `(type, value)`. A nil interface has `(nil, nil)`. An interface holding a nil pointer is `(*T, nil)` and is NOT equal to a nil interface.

**Q12: What is a type assertion vs type switch?**  
- Type assertion: `v := i.(T)` — extracts concrete type from interface (panics if wrong; use `v, ok := i.(T)` for safety).
- Type switch: `switch v := i.(type) { case T: ... }` — handles multiple possible types.

**Q13: What is the empty interface (`interface{}`)?**  
Has no methods, so every type satisfies it. Used to represent any value. In Go 1.18+, `any` is an alias. Avoid overusing — prefer specific interfaces.

---

## 🟡 Functions & Methods

**Q14: Can Go functions return multiple values?**  
Yes. The idiomatic pattern is `(result, error)`. Example: `func divide(a, b float64) (float64, error)`.

**Q15: What is a closure?**  
A function that captures and remembers variables from its enclosing scope. The captured variables remain alive as long as the closure does.

**Q16: What is the difference between value and pointer receivers?**  
Value receiver gets a copy — changes don't affect the original. Pointer receiver works on the original — can mutate it. Use pointer receivers for mutating methods or large structs. Be consistent within a type.

**Q17: What is `defer` and when does it run?**  
`defer` schedules a function call to run when the surrounding function returns (via return, panic, or fall-off). Multiple defers execute in LIFO order.

**Q18: Can `defer` modify return values?**  
Yes, if the return values are named. The deferred function can access and modify them by name.

**Q19: What is `init()`?**  
A special function that runs automatically before `main()`, after all package-level variable declarations. Can't be called manually. Multiple `init()` functions per file are allowed (run in order).

---

## 🟠 Memory & Pointers

**Q20: Is Go pass by value or reference?**  
Always pass by value. But you can pass a pointer (which is itself a value) to achieve reference semantics. Maps, slices, and channels internally contain pointers, so mutations to their contents are visible to callers.

**Q21: What is a memory leak in Go?**  
Despite garbage collection, Go can leak: goroutines that never terminate, items in maps/slices that hold references to large objects, `time.Ticker` not stopped, unclosed HTTP response bodies, etc.

**Q22: What does the garbage collector do in Go?**  
Go uses a concurrent, tri-color mark-and-sweep GC. It runs concurrently with the program to minimize pause times (typically sub-millisecond).

**Q23: What is `unsafe` package?**  
Provides low-level memory operations that bypass Go's type safety — pointer arithmetic, converting between types. Rarely needed; usage signals a code smell unless doing very specific low-level work.

---

## 🔴 Concurrency

**Q24: What is a goroutine?**  
A lightweight, cooperatively scheduled execution unit managed by the Go runtime. Starts with ~2KB stack (vs ~1MB for OS threads). You can run millions simultaneously.

**Q25: What is the difference between concurrency and parallelism?**  
Concurrency: structuring a program to handle multiple things at once (Go's goroutines). Parallelism: literally executing multiple things at the same time (requires multiple CPU cores). `GOMAXPROCS` controls the degree of parallelism.

**Q26: What is a race condition and how do you detect it?**  
When two goroutines access shared memory concurrently and at least one is a write, without synchronization. Detect with `go run -race` or `go test -race`.

**Q27: What is a deadlock?**  
When all goroutines are blocked waiting for each other, making no progress. Go detects this at runtime and panics with "all goroutines are asleep - deadlock!"

**Q28: What is the difference between buffered and unbuffered channels?**  
- Unbuffered: sender blocks until receiver is ready (synchronous). Provides a rendezvous/synchronization point.
- Buffered: sender blocks only when buffer is full (asynchronous up to capacity).

**Q29: What happens when you send to a closed channel?**  
Panic. Only the sender should close channels. Closing signals "no more values."

**Q30: What happens when you receive from a closed channel?**  
Returns the zero value immediately, and the second return value (`ok`) is `false`. `for range` exits cleanly when the channel is closed.

**Q31: When to use channels vs mutexes?**  
- Channels: transferring ownership of data, coordinating goroutines, signaling events ("communicate by sharing memory").
- Mutex: protecting shared state that multiple goroutines need to read/write.

**Q32: What is `sync.WaitGroup`?**  
A counter-based synchronization primitive. Call `Add(n)` before starting goroutines, `Done()` when each finishes, and `Wait()` to block until all are done.

**Q33: What is `sync.Once`?**  
Ensures a function runs exactly once, even from multiple goroutines. Used for lazy initialization and singletons.

**Q34: What is a goroutine leak?**  
A goroutine that never terminates, holding resources. Common causes: blocked channel send/receive with no partner, infinite loop without cancellation. Always provide a way to stop goroutines (context, done channel).

**Q35: What is `context.Context` used for?**  
Carries deadlines, cancellation signals, and request-scoped values across API boundaries. Should be the first parameter of any function that initiates long operations or calls external services.

---

## 🟣 Error Handling

**Q36: Why does Go use error values instead of exceptions?**  
Errors are explicit, making control flow clear. Callers must handle errors at the point of occurrence. No hidden exceptional paths; easier to reason about.

**Q37: What is the difference between `errors.Is` and `errors.As`?**  
- `errors.Is(err, target)`: checks if any error in the chain equals `target` (for sentinel errors).
- `errors.As(err, &target)`: checks if any error in the chain can be assigned to `target`'s type (for typed errors).

**Q38: What is error wrapping?**  
Using `fmt.Errorf("context: %w", err)` to add context while preserving the original error for inspection via `errors.Is`/`errors.As`.

**Q39: When should you use `panic`?**  
Only for truly unrecoverable programmer errors (not for business logic). Also used in `Must*` functions (e.g., `regexp.MustCompile`). Never for expected error conditions.

---

## 🔷 Packages & Modules

**Q40: What is the difference between a package and a module?**  
Package: a directory of Go files sharing one package name — the basic unit of code organization. Module: a collection of packages versioned together, defined by `go.mod` — the unit of distribution and dependency management.

**Q41: What does `go mod tidy` do?**  
Adds missing and removes unused module dependencies from `go.mod` and `go.sum`.

**Q42: What is the `internal` package?**  
A special directory whose packages can only be imported by code within the parent directory tree. Enforced by the Go compiler to hide implementation details.

**Q43: What is a blank import (`_ "pkg"`)?**  
Imports a package only for its side effects (runs its `init()` function). Commonly used to register database drivers or image codecs.

---

## ⚙️ Testing

**Q44: How do you write a test in Go?**  
Create a file ending in `_test.go`. Write functions with signature `func TestXxx(t *testing.T)`. Run with `go test ./...`.

**Q45: What is table-driven testing?**  
Defining test cases in a slice of structs, then ranging over them with `t.Run()`. Reduces duplication and makes adding new cases trivial. The idiomatic Go testing style.

**Q46: What is the difference between `t.Error` and `t.Fatal`?**  
`t.Error`: marks test as failed, continues running. `t.Fatal`: marks as failed, stops immediately (like `t.Error` + `return`).

**Q47: How do you mock in Go without a framework?**  
Define an interface for the dependency. Create a mock struct implementing that interface. Inject in tests. No external frameworks needed, though testify/mock and gomock are popular.

---

## 🏆 Advanced

**Q48: What are Go generics?**  
Introduced in Go 1.18. Allow writing type-parametric functions and types: `func Map[T, U any](s []T, f func(T) U) []U`. Use type constraints to restrict allowed types.

**Q49: What is `GOMAXPROCS`?**  
Controls how many OS threads can execute Go code simultaneously. Defaults to the number of CPU cores. `runtime.GOMAXPROCS(n)` to change at runtime.

**Q50: What is the Go scheduler?**  
Go uses an M:N scheduler — M goroutines run on N OS threads (where N = GOMAXPROCS). The scheduler uses work-stealing to balance goroutines across threads. It's cooperative + preemptive (goroutines are preempted at safe points since Go 1.14).

**Q51: What is escape analysis?**  
The compiler determines at compile time whether a variable can live on the stack or must be allocated on the heap (escapes to heap). Stack allocation is cheaper (no GC pressure). Variables escape when they outlive the function or are passed by pointer to unknown code.

**Q52: What is `reflect` package?**  
Provides runtime reflection — inspect types and values dynamically. Used by `encoding/json`, `fmt`, `testing`. Expensive; avoid in hot paths. Use when you truly need to be generic without generics.

**Q53: What is a struct tag?**  
A string literal following a struct field that provides metadata for reflection-based libraries: `json:"name,omitempty"`, `db:"user_id"`, `validate:"required,min=1"`.

**Q54: How does Go handle circular imports?**  
The Go compiler disallows circular imports — package A cannot import B if B imports A. Solution: extract the shared dependency into a third package, or redesign with interfaces.

**Q55: What is the difference between `T` and `*T` in an interface implementation?**  
If a method is defined with a pointer receiver `*T`, only `*T` satisfies the interface. If defined with value receiver `T`, both `T` and `*T` satisfy the interface (Go automatically takes the address).

---

## 💡 Quick Code Snippets for Review

```go
// Goroutine-safe counter
type Counter struct {
    mu    sync.Mutex
    value int
}
func (c *Counter) Inc() { c.mu.Lock(); c.value++; c.mu.Unlock() }
func (c *Counter) Val() int { c.mu.Lock(); defer c.mu.Unlock(); return c.value }

// Context with timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// Channel pipeline
gen := func(nums ...int) <-chan int {
    out := make(chan int)
    go func() { for _, n := range nums { out <- n }; close(out) }()
    return out
}

// Table test skeleton
tests := []struct{ name string; in int; want int }{
    {"zero", 0, 0}, {"positive", 5, 25},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        if got := square(tt.in); got != tt.want {
            t.Errorf("square(%d) = %d; want %d", tt.in, got, tt.want)
        }
    })
}

// Functional options
type Option func(*Config)
func WithTimeout(d time.Duration) Option { return func(c *Config) { c.timeout = d } }
func New(opts ...Option) *Config {
    c := &Config{timeout: 30 * time.Second}
    for _, o := range opts { o(c) }
    return c
}
```

---

## 🚀 Final Interview Checklist

- [ ] Can explain goroutines vs OS threads
- [ ] Know the difference between channels and mutexes  
- [ ] Can write a table-driven test
- [ ] Understand interface implicit implementation
- [ ] Know when to use pointer vs value receivers
- [ ] Can explain `defer` execution order
- [ ] Understand error wrapping and `errors.Is`/`errors.As`
- [ ] Know the zero values for all types
- [ ] Can implement a worker pool
- [ ] Understand context cancellation
- [ ] Know Go's memory model basics
- [ ] Can explain slice backing arrays and append behavior

---

*Good luck with your interviews, Yati! You've got this! 🚀*

---

*← [14 — Patterns & Best Practices](./14_patterns_best_practices.md) | [00 — Roadmap →](./00_roadmap.md)*
