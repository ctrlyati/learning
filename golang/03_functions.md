# 03 — Functions

> **Goal:** Master Go functions — the core unit of reusability, including closures, defer, variadic functions, and first-class functions.

---

## 1. Basic Function

```go
// Syntax: func name(params) returnType { body }

func greet(name string) string {
    return "Hello, " + name + "!"
}

func main() {
    msg := greet("Yati")
    fmt.Println(msg)
}
```

---

## 2. Multiple Return Values

One of Go's most distinctive features:

```go
func divide(a, b float64) (float64, error) {
    if b == 0 {
        return 0, errors.New("division by zero")
    }
    return a / b, nil
}

result, err := divide(10, 2)
if err != nil {
    log.Fatal(err)
}
fmt.Println(result) // 5
```

### Multiple params of same type — shorthand

```go
func add(a, b int) int {      // instead of (a int, b int)
    return a + b
}

func minMax(a, b, c int) (int, int) {  // min, max
    // ...
}
```

---

## 3. Named Return Values

```go
func split(sum int) (x, y int) {
    x = sum * 4 / 9
    y = sum - x
    return  // "naked return" — returns x and y
}

// Named returns are useful for documentation and defer
func readConfig(path string) (config Config, err error) {
    defer func() {
        if err != nil {
            err = fmt.Errorf("readConfig: %w", err)
        }
    }()
    // ...
    return
}
```

---

## 4. Variadic Functions

```go
func sum(nums ...int) int {
    total := 0
    for _, n := range nums {
        total += n
    }
    return total
}

fmt.Println(sum(1, 2, 3))       // 6
fmt.Println(sum(1, 2, 3, 4, 5)) // 15

// Spread a slice into variadic
nums := []int{1, 2, 3, 4}
fmt.Println(sum(nums...))       // 10
```

---

## 5. Functions as First-Class Values

```go
// Assign function to variable
add := func(a, b int) int { return a + b }
fmt.Println(add(3, 4)) // 7

// Pass function as argument
func apply(f func(int, int) int, a, b int) int {
    return f(a, b)
}

result := apply(func(a, b int) int { return a * b }, 3, 4)
fmt.Println(result) // 12

// Return a function
func multiplier(factor int) func(int) int {
    return func(n int) int {
        return n * factor
    }
}

double := multiplier(2)
triple := multiplier(3)
fmt.Println(double(5)) // 10
fmt.Println(triple(5)) // 15
```

---

## 6. Closures

A closure is a function that captures variables from its outer scope:

```go
func counter() func() int {
    count := 0
    return func() int {
        count++
        return count
    }
}

c1 := counter()
c2 := counter()

fmt.Println(c1()) // 1
fmt.Println(c1()) // 2
fmt.Println(c1()) // 3
fmt.Println(c2()) // 1 (independent state)
```

### Closure gotcha — loop variable capture

```go
// WRONG: all goroutines capture the same 'i'
funcs := make([]func(), 5)
for i := 0; i < 5; i++ {
    funcs[i] = func() { fmt.Println(i) }
}
// All print 5 (the final value of i)

// CORRECT: capture by value
for i := 0; i < 5; i++ {
    i := i  // new variable per iteration
    funcs[i] = func() { fmt.Println(i) }
}

// OR pass as argument
for i := 0; i < 5; i++ {
    funcs[i] = func(n int) func() {
        return func() { fmt.Println(n) }
    }(i)
}
```

---

## 7. Anonymous Functions (immediately invoked)

```go
result := func(a, b int) int {
    return a + b
}(3, 4)
fmt.Println(result) // 7
```

---

## 8. init() Function

`init()` runs automatically before `main()`, used for package-level setup:

```go
package main

var config map[string]string

func init() {
    config = map[string]string{
        "host": "localhost",
        "port": "8080",
    }
}

func main() {
    fmt.Println(config["host"]) // localhost
}
```

Rules:
- Cannot be called manually
- Can have multiple `init()` in one file (run in order)
- Runs after all variable initializations

---

## 9. Function Type Declarations

```go
// Define a custom function type
type MathFunc func(int, int) int

func operate(a, b int, f MathFunc) int {
    return f(a, b)
}

add := MathFunc(func(a, b int) int { return a + b })
sub := MathFunc(func(a, b int) int { return a - b })

fmt.Println(operate(10, 5, add)) // 15
fmt.Println(operate(10, 5, sub)) // 5
```

---

## 10. Recursive Functions

```go
func factorial(n int) int {
    if n <= 1 {
        return 1
    }
    return n * factorial(n-1)
}

func fibonacci(n int) int {
    if n <= 1 {
        return n
    }
    return fibonacci(n-1) + fibonacci(n-2)
}
```

---

## 🎯 Interview Tips

- **Q: Can Go functions return multiple values?** → Yes. This is idiomatic for returning `(result, error)` pairs.
- **Q: What is a closure?** → A function that references variables from its enclosing scope. The captured variables remain alive as long as the closure does.
- **Q: What is a variadic function?** → A function that accepts a variable number of arguments using `...`. The `fmt.Println` is variadic.
- **Q: What is the difference between `func f()` and `var f = func()`?** → The first is a declared function (callable by name anywhere in the package). The second is a function value stored in a variable.
- **Q: Can you call `init()` manually?** → No, it's called automatically by the Go runtime.
- **Q: What is a naked return?** → A `return` with no arguments in a function with named return values. Considered bad practice in long functions — reduces readability.
- **Q: How do you pass a slice to a variadic function?** → Use the spread operator: `f(slice...)`.

---

*Next: [04 — Pointers →](./04_pointers.md)*
