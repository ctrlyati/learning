# 07 — Interfaces

> **Goal:** Master Go's most powerful abstraction — implicit interfaces enable flexible, decoupled design.

---

## 1. What is an Interface?

An interface defines a set of method signatures. Any type that implements all the methods **automatically satisfies** the interface — no `implements` keyword needed.

```go
type Animal interface {
    Sound() string
    Name() string
}

type Dog struct{ name string }
func (d Dog) Sound() string { return "Woof" }
func (d Dog) Name() string  { return d.name }

type Cat struct{ name string }
func (c Cat) Sound() string { return "Meow" }
func (c Cat) Name() string  { return c.name }

// Both Dog and Cat satisfy Animal implicitly
func makeSound(a Animal) {
    fmt.Printf("%s says %s\n", a.Name(), a.Sound())
}

makeSound(Dog{"Rex"})  // Rex says Woof
makeSound(Cat{"Mia"})  // Mia says Meow
```

---

## 2. Defining Interfaces

```go
// Single method (most common pattern — Go favors small interfaces)
type Reader interface {
    Read(p []byte) (n int, err error)
}

type Writer interface {
    Write(p []byte) (n int, err error)
}

// Composed interface
type ReadWriter interface {
    Reader
    Writer
}

// Multiple methods
type Shape interface {
    Area() float64
    Perimeter() float64
}
```

---

## 3. The empty interface — `interface{}` / `any`

Accepts any type (like `Object` in Java):

```go
// interface{} is the pre-Go 1.18 way
func printAny(v interface{}) {
    fmt.Printf("type: %T, value: %v\n", v, v)
}

// any is an alias (Go 1.18+)
func printAny(v any) {
    fmt.Printf("type: %T, value: %v\n", v, v)
}

printAny(42)
printAny("hello")
printAny([]int{1, 2, 3})
printAny(nil)
```

---

## 4. Type Assertion

Extract the concrete value from an interface:

```go
var i interface{} = "hello"

// Unsafe — panics if wrong type
s := i.(string)
fmt.Println(s)   // hello

// Safe — no panic
s, ok := i.(string)
if ok {
    fmt.Println("string:", s)
}

n, ok := i.(int)
fmt.Println(ok)  // false
fmt.Println(n)   // 0 (zero value)
```

---

## 5. Type Switch

Check an interface value's type against multiple types:

```go
func describe(i interface{}) string {
    switch v := i.(type) {
    case int:
        return fmt.Sprintf("int: %d", v)
    case float64:
        return fmt.Sprintf("float64: %.2f", v)
    case string:
        return fmt.Sprintf("string: %q", v)
    case bool:
        return fmt.Sprintf("bool: %t", v)
    case []int:
        return fmt.Sprintf("[]int with %d elements", len(v))
    case nil:
        return "nil"
    default:
        return fmt.Sprintf("unknown type: %T", v)
    }
}
```

---

## 6. Interface Values

An interface value has two components: `(type, value)`:

```go
type MyError struct{ Msg string }
func (e *MyError) Error() string { return e.Msg }

var err error
fmt.Println(err == nil)   // true  — (nil, nil)

err = &MyError{"something broke"}
fmt.Println(err == nil)   // false — (*MyError, &MyError{...})
```

### The nil interface gotcha

```go
func getError() error {
    var p *MyError = nil
    return p  // returns (*MyError, nil) — NOT a nil interface!
}

err := getError()
fmt.Println(err == nil)  // FALSE! (common bug)

// Fix: return nil directly
func getError() error {
    return nil
}
```

---

## 7. Common Standard Library Interfaces

```go
// fmt.Stringer — controls fmt.Println output
type Stringer interface {
    String() string
}

// error — the built-in error interface
type error interface {
    Error() string
}

// io.Reader
type Reader interface {
    Read(p []byte) (n int, err error)
}

// io.Writer
type Writer interface {
    Write(p []byte) (n int, err error)
}

// io.Closer
type Closer interface {
    Close() error
}

// sort.Interface
type Interface interface {
    Len() int
    Less(i, j int) bool
    Swap(i, j int)
}
```

### Implementing sort.Interface

```go
type ByLength []string

func (b ByLength) Len() int           { return len(b) }
func (b ByLength) Less(i, j int) bool { return len(b[i]) < len(b[j]) }
func (b ByLength) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

fruits := []string{"banana", "apple", "kiwi"}
sort.Sort(ByLength(fruits))
fmt.Println(fruits)  // [kiwi apple banana]
```

---

## 8. Interface Design Principles

```go
// Prefer small, focused interfaces (Unix philosophy)
type Reader interface { Read(...) }      // 1 method
type Writer interface { Write(...) }     // 1 method
type Closer interface { Close() }        // 1 method

// Compose when needed
type ReadWriteCloser interface {
    Reader
    Writer
    Closer
}

// Accept interfaces, return concrete types (Postel's law)
func ProcessData(r io.Reader) *Result { ... }  // good
func ProcessData(f *os.File) *Result { ... }   // too restrictive
```

---

## 9. Polymorphism Example

```go
type Shape interface {
    Area() float64
}

type Circle struct{ Radius float64 }
type Rectangle struct{ Width, Height float64 }
type Triangle struct{ Base, Height float64 }

func (c Circle) Area() float64    { return math.Pi * c.Radius * c.Radius }
func (r Rectangle) Area() float64 { return r.Width * r.Height }
func (t Triangle) Area() float64  { return 0.5 * t.Base * t.Height }

func totalArea(shapes []Shape) float64 {
    total := 0.0
    for _, s := range shapes {
        total += s.Area()
    }
    return total
}

shapes := []Shape{
    Circle{5},
    Rectangle{4, 6},
    Triangle{3, 8},
}
fmt.Printf("Total: %.2f\n", totalArea(shapes))
```

---

## 🎯 Interview Tips

- **Q: How is Go's interface different from Java/C#?** → Go uses **implicit (structural) typing** — no `implements` keyword. If a type has the required methods, it satisfies the interface automatically.
- **Q: What is the empty interface?** → `interface{}` (or `any` in Go 1.18+) has no methods, so every type satisfies it. Used for generic containers before generics were added.
- **Q: What is the difference between type assertion and type switch?** → Type assertion `i.(T)` checks/extracts a single type. Type switch handles multiple types.
- **Q: Can a nil interface cause problems?** → Yes. A nil interface `(nil, nil)` is different from an interface holding a nil pointer `(*T, nil)`. Returning a typed nil pointer as an error interface causes `err != nil` to be true.
- **Q: What is the "accept interfaces, return structs" principle?** → Functions should accept interface types (flexible, testable) and return concrete types (predictable, no hidden behavior).
- **Q: Why does Go prefer small interfaces?** → Small interfaces (1–3 methods) are easy to satisfy, compose well, and are more reusable. Large interfaces are harder to mock in tests.

---

*Next: [08 — Error Handling →](./08_error_handling.md)*
