# 02 — Control Flow

> **Goal:** Master Go's control structures — loops, conditionals, and switch statements.

---

## 1. if / else

```go
x := 10

if x > 5 {
    fmt.Println("greater")
} else if x == 5 {
    fmt.Println("equal")
} else {
    fmt.Println("less")
}
```

### if with initialization statement

```go
// Variable declared in the if scope only
if n := 10; n%2 == 0 {
    fmt.Println("even")
} else {
    fmt.Println("odd")
}
// n is NOT accessible here

// Very common with error handling
if err := doSomething(); err != nil {
    fmt.Println("Error:", err)
    return
}
```

---

## 2. for Loop (Go's only loop)

Go has **no while, do-while, or foreach** — `for` does everything.

### Classic C-style for

```go
for i := 0; i < 5; i++ {
    fmt.Println(i)
}
```

### While-style (condition only)

```go
n := 1
for n < 100 {
    n *= 2
}
fmt.Println(n) // 128
```

### Infinite loop

```go
for {
    // runs forever until break or return
    if done {
        break
    }
}
```

### range — iterate over collections

```go
// Slice
nums := []int{10, 20, 30}
for i, v := range nums {
    fmt.Printf("index=%d, value=%d\n", i, v)
}

// Skip index
for _, v := range nums {
    fmt.Println(v)
}

// Skip value
for i := range nums {
    fmt.Println(i)
}

// Map
m := map[string]int{"a": 1, "b": 2}
for key, val := range m {
    fmt.Printf("%s: %d\n", key, val)
}

// String (iterates over runes)
for i, ch := range "hello" {
    fmt.Printf("%d: %c\n", i, ch)
}

// Channel
ch := make(chan int)
go func() { ch <- 42; close(ch) }()
for v := range ch {
    fmt.Println(v)
}
```

---

## 3. break, continue, goto

```go
// break — exit the loop
for i := 0; i < 10; i++ {
    if i == 5 {
        break
    }
    fmt.Println(i)
}

// continue — skip current iteration
for i := 0; i < 10; i++ {
    if i%2 == 0 {
        continue
    }
    fmt.Println(i) // prints odd numbers
}

// Labeled break (for nested loops)
outer:
for i := 0; i < 3; i++ {
    for j := 0; j < 3; j++ {
        if i == 1 && j == 1 {
            break outer  // breaks both loops
        }
        fmt.Printf("%d,%d\n", i, j)
    }
}
```

---

## 4. switch

```go
day := "Monday"

switch day {
case "Monday", "Tuesday", "Wednesday", "Thursday", "Friday":
    fmt.Println("Weekday")
case "Saturday", "Sunday":
    fmt.Println("Weekend")
default:
    fmt.Println("Unknown")
}
```

### switch with no condition (like if-else chain)

```go
score := 85

switch {
case score >= 90:
    fmt.Println("A")
case score >= 80:
    fmt.Println("B")
case score >= 70:
    fmt.Println("C")
default:
    fmt.Println("F")
}
```

### switch with initialization

```go
switch x := getValue(); {
case x > 0:
    fmt.Println("positive")
case x < 0:
    fmt.Println("negative")
default:
    fmt.Println("zero")
}
```

### fallthrough

In Go, cases do NOT fall through by default (unlike C). Use `fallthrough` explicitly:

```go
switch 2 {
case 1:
    fmt.Println("one")
    fallthrough
case 2:
    fmt.Println("two")    // prints
    fallthrough
case 3:
    fmt.Println("three")  // also prints (because of fallthrough above)
case 4:
    fmt.Println("four")   // does NOT print
}
```

### Type switch

```go
func describe(i interface{}) {
    switch v := i.(type) {
    case int:
        fmt.Printf("int: %d\n", v)
    case string:
        fmt.Printf("string: %s\n", v)
    case bool:
        fmt.Printf("bool: %t\n", v)
    default:
        fmt.Printf("unknown type: %T\n", v)
    }
}

describe(42)      // int: 42
describe("hello") // string: hello
describe(true)    // bool: true
```

---

## 5. defer

`defer` delays a function call until the surrounding function returns. Used for cleanup.

```go
func readFile(name string) {
    f, err := os.Open(name)
    if err != nil {
        return
    }
    defer f.Close()  // runs when readFile() returns, no matter what

    // ... read file
}
```

### Multiple defers — LIFO order

```go
func main() {
    defer fmt.Println("first deferred")   // runs last
    defer fmt.Println("second deferred")  // runs second
    defer fmt.Println("third deferred")   // runs first
    fmt.Println("main body")
}
// Output:
// main body
// third deferred
// second deferred
// first deferred
```

### defer with loop (common gotcha)

```go
// WRONG — only defers last value of i
for i := 0; i < 3; i++ {
    defer fmt.Println(i) // captures i by reference
}

// CORRECT — capture by value using a wrapper
for i := 0; i < 3; i++ {
    i := i  // shadow the variable
    defer fmt.Println(i)
}
```

---

## 🎯 Interview Tips

- **Q: Does Go have a while loop?** → No. `for` with only a condition acts as a while loop.
- **Q: Does switch fall through by default?** → No. Unlike C/Java, each case breaks automatically. Use `fallthrough` to opt in.
- **Q: What is the order of deferred calls?** → LIFO (last in, first out) — like a stack.
- **Q: Can defer modify return values?** → Yes, if the return values are named. This is a powerful (and tricky) pattern:
  ```go
  func double(n int) (result int) {
      defer func() { result *= 2 }()
      result = n
      return  // returns n*2, not n
  }
  ```
- **Q: What does `for range` return for a string?** → The index (byte position) and the rune (Unicode code point), not the byte.
- **Q: Can you use `break` with a label?** → Yes, labeled break exits a specific outer loop.

---

*Next: [03 — Functions →](./03_functions.md)*
