# 01 — Go Basics

> **Goal:** Understand Go's fundamental building blocks — types, variables, constants, and the package system.

---

## 1. Hello, World

```go
package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
}
```

- Every Go file belongs to a **package**
- The `main` package + `main()` function = entry point
- `import` brings in packages; unused imports are a **compile error**

---

## 2. Variables

### Declaration styles

```go
// Explicit type
var name string = "Yati"

// Type inferred
var age = 28

// Short declaration (most common, inside functions only)
city := "San Francisco"

// Multiple variables
var x, y int = 10, 20
a, b := "hello", true

// Block declaration
var (
    firstName string = "Yati"
    lastName  string = "Doe"
    score     int    = 100
)
```

### Zero values (default when declared but not initialized)

| Type | Zero Value |
|------|-----------|
| `int`, `float64` | `0` |
| `bool` | `false` |
| `string` | `""` |
| pointer, slice, map, channel, func | `nil` |

```go
var count int    // count = 0
var flag bool    // flag = false
var label string // label = ""
```

---

## 3. Data Types

### Numeric types

```go
// Integers
var i8  int8   = 127          // -128 to 127
var i16 int16  = 32767
var i32 int32  = 2147483647
var i64 int64  = 9223372036854775807
var i   int    = 42           // platform-dependent (32 or 64 bit)

// Unsigned integers
var u uint   = 42
var u8 uint8 = 255            // also: byte

// Floats
var f32 float32 = 3.14
var f64 float64 = 3.14159265358979  // preferred

// Complex
var c complex64  = 1 + 2i
var c2 complex128 = 3 + 4i
```

### String

```go
greeting := "Hello, Go!"
fmt.Println(len(greeting))         // length in bytes
fmt.Println(greeting[0])           // byte value: 72 (H)
fmt.Println(string(greeting[0]))   // "H"

// Multi-line (raw string literal)
text := `This is
a multi-line
string`

// String formatting
name := "Yati"
msg := fmt.Sprintf("Hello, %s! You are %d years old.", name, 28)
```

### Boolean

```go
t := true
f := false
fmt.Println(t && f)  // false
fmt.Println(t || f)  // true
fmt.Println(!t)      // false
```

### Rune (Unicode code point)

```go
var r rune = 'A'    // rune is alias for int32
fmt.Println(r)      // 65
```

---

## 4. Constants

```go
const Pi = 3.14159
const MaxRetries = 3
const Greeting = "Hello"

// Typed constant
const TypedPi float64 = 3.14159

// Block of constants
const (
    StatusOK    = 200
    StatusNotFound = 404
    StatusError = 500
)
```

### iota — auto-incrementing constants

```go
type Direction int

const (
    North Direction = iota  // 0
    East                     // 1
    South                    // 2
    West                     // 3
)

// Custom iota patterns
const (
    _  = iota             // skip 0
    KB = 1 << (10 * iota) // 1024
    MB                    // 1048576
    GB                    // 1073741824
)
```

---

## 5. Type Conversions

Go does **not** do implicit type conversion — always explicit:

```go
var i int = 42
var f float64 = float64(i)
var u uint = uint(f)

// String conversions
s := strconv.Itoa(42)          // int → string: "42"
n, err := strconv.Atoi("42")   // string → int: 42, nil

// String ↔ bytes
b := []byte("hello")           // string → []byte
s2 := string(b)                // []byte → string
```

---

## 6. fmt Package Essentials

```go
// Print functions
fmt.Print("no newline")
fmt.Println("with newline")
fmt.Printf("formatted: %s %d %f\n", "text", 42, 3.14)

// Format to string (no print)
s := fmt.Sprintf("Hello, %s!", "Yati")

// Common format verbs
// %v  — default format
// %+v — struct with field names
// %#v — Go syntax representation
// %T  — type of the value
// %d  — decimal integer
// %f  — float
// %s  — string
// %q  — quoted string
// %t  — bool
// %p  — pointer address
// %b  — binary
// %x  — hexadecimal

type Person struct{ Name string; Age int }
p := Person{"Yati", 28}
fmt.Printf("%v\n", p)   // {Yati 28}
fmt.Printf("%+v\n", p)  // {Name:Yati Age:28}
fmt.Printf("%#v\n", p)  // main.Person{Name:"Yati", Age:28}
fmt.Printf("%T\n", p)   // main.Person
```

---

## 7. Scope

```go
package main

var globalVar = "I'm global"  // package-level scope

func main() {
    localVar := "I'm local"   // function scope
    
    {
        blockVar := "I'm block-scoped"  // block scope
        fmt.Println(blockVar)
    }
    // blockVar is NOT accessible here
    
    fmt.Println(localVar)
    fmt.Println(globalVar)
}
```

---

## 🎯 Interview Tips

- **Q: What are the zero values in Go?** → Numbers=0, bool=false, string="", pointers/slices/maps/channels=nil
- **Q: Difference between `var x = 5` and `x := 5`?** → `:=` is shorthand, only works inside functions. `var` works anywhere.
- **Q: Can you have unused variables in Go?** → No — unused local variables are a compile error. (But unused package-level variables are okay.)
- **Q: What is `iota`?** → A special constant that auto-increments within a `const` block, starting at 0.
- **Q: Is Go strongly typed?** → Yes. No implicit conversions between types.
- **Q: What is `byte` and `rune`?** → `byte` is an alias for `uint8`. `rune` is an alias for `int32` (Unicode code point).

---

*Next: [02 — Control Flow →](./02_control_flow.md)*
