# 06 — Structs & Methods

> **Goal:** Learn how Go uses structs and methods to model real-world entities — the foundation of OOP in Go without classes.

---

## 1. Defining a Struct

```go
type Person struct {
    Name    string
    Age     int
    Email   string
    private string  // unexported (lowercase)
}
```

---

## 2. Creating Struct Instances

```go
// Named fields (recommended)
p1 := Person{Name: "Yati", Age: 28, Email: "yati@example.com"}

// Positional (fragile — avoid for structs with many fields)
p2 := Person{"Bob", 30, "bob@example.com", ""}

// Zero value
var p3 Person
fmt.Println(p3)  // { 0  }

// Pointer to struct
p4 := &Person{Name: "Alice", Age: 25}
fmt.Println(p4.Name)  // Alice (auto-dereferenced)

// Using new()
p5 := new(Person)
p5.Name = "Carol"
```

---

## 3. Anonymous Structs

```go
// One-off struct without a type name
point := struct {
    X, Y int
}{X: 10, Y: 20}

fmt.Println(point.X)

// Often used in test tables
tests := []struct {
    input    string
    expected int
}{
    {"hello", 5},
    {"go", 2},
}
```

---

## 4. Methods

Methods are functions attached to a type:

```go
type Rectangle struct {
    Width  float64
    Height float64
}

// Value receiver — works on a copy
func (r Rectangle) Area() float64 {
    return r.Width * r.Height
}

// Value receiver
func (r Rectangle) Perimeter() float64 {
    return 2 * (r.Width + r.Height)
}

// Pointer receiver — can modify the struct
func (r *Rectangle) Scale(factor float64) {
    r.Width *= factor
    r.Height *= factor
}

rect := Rectangle{Width: 10, Height: 5}
fmt.Println(rect.Area())       // 50
fmt.Println(rect.Perimeter())  // 30
rect.Scale(2)
fmt.Println(rect.Width)        // 20 (modified!)
```

---

## 5. Value vs Pointer Receiver

| Scenario | Receiver |
|----------|----------|
| Method needs to mutate the struct | `*T` (pointer) |
| Struct is large (avoid copying) | `*T` (pointer) |
| Read-only, small struct | `T` (value) |
| Implementing interfaces | Consistent — choose one and stick with it |

**Key rule:** If any method uses a pointer receiver, all methods should use pointer receivers for consistency.

```go
// Bad — mixed receivers (avoid)
func (r Rectangle) Area() float64 { ... }   // value
func (r *Rectangle) Scale(f float64) { ... } // pointer

// Good — use pointer receiver for all when any method mutates
func (r *Rectangle) Area() float64 { ... }
func (r *Rectangle) Scale(f float64) { ... }
```

---

## 6. Methods on Non-Struct Types

You can define methods on any named type defined in the same package:

```go
type Celsius float64
type Fahrenheit float64

func (c Celsius) ToFahrenheit() Fahrenheit {
    return Fahrenheit(c*9/5 + 32)
}

func (f Fahrenheit) ToCelsius() Celsius {
    return Celsius((f - 32) * 5 / 9)
}

boiling := Celsius(100)
fmt.Println(boiling.ToFahrenheit())  // 212
```

---

## 7. Struct Embedding (Composition)

Go uses embedding instead of inheritance:

```go
type Animal struct {
    Name string
}

func (a Animal) Speak() string {
    return a.Name + " makes a sound"
}

type Dog struct {
    Animal          // embedded (anonymous field)
    Breed string
}

func (d Dog) Speak() string {  // override
    return d.Name + " barks!"  // access embedded fields directly
}

d := Dog{
    Animal: Animal{Name: "Rex"},
    Breed:  "Labrador",
}

fmt.Println(d.Name)      // "Rex" — promoted field
fmt.Println(d.Speak())   // "Rex barks!" — overridden method
fmt.Println(d.Animal.Speak())  // "Rex makes a sound" — explicit access
```

### Embedding with pointer

```go
type Logger struct {
    prefix string
}
func (l *Logger) Log(msg string) {
    fmt.Printf("[%s] %s\n", l.prefix, msg)
}

type Service struct {
    *Logger       // embedded pointer
    Name string
}

svc := Service{
    Logger: &Logger{prefix: "INFO"},
    Name:   "UserService",
}
svc.Log("started")  // [INFO] started
```

---

## 8. Struct Tags

Tags add metadata for serialization/validation:

```go
import "encoding/json"

type User struct {
    ID       int    `json:"id"`
    Name     string `json:"name"`
    Email    string `json:"email,omitempty"`  // omit if empty
    Password string `json:"-"`                // never include
    Age      int    `json:"age,string"`       // encode as string
}

u := User{ID: 1, Name: "Yati", Email: "yati@example.com", Password: "secret"}
data, _ := json.Marshal(u)
fmt.Println(string(data))
// {"id":1,"name":"Yati","email":"yati@example.com"}
```

---

## 9. Comparing Structs

```go
type Point struct{ X, Y int }

p1 := Point{1, 2}
p2 := Point{1, 2}
p3 := Point{3, 4}

fmt.Println(p1 == p2)  // true
fmt.Println(p1 == p3)  // false

// Structs with uncomparable fields (slice, map, func) cannot be compared with ==
type Bad struct{ Items []int }
// Bad{} == Bad{}  // COMPILE ERROR
```

---

## 10. Stringer Interface (fmt.Stringer)

```go
type Color int

const (
    Red Color = iota
    Green
    Blue
)

func (c Color) String() string {
    switch c {
    case Red:
        return "Red"
    case Green:
        return "Green"
    case Blue:
        return "Blue"
    }
    return "Unknown"
}

fmt.Println(Red)    // Red (calls String() automatically with fmt)
fmt.Println(Green)  // Green
```

---

## 🎯 Interview Tips

- **Q: Does Go support inheritance?** → No. Go uses **composition via embedding** instead. This is intentional — "composition over inheritance."
- **Q: What is the difference between embedding and a regular field?** → Embedded type's fields and methods are promoted and accessible directly on the outer struct. A regular named field requires explicit access.
- **Q: When should I use pointer vs value receiver?** → Use pointer receivers when the method mutates the struct or the struct is large. Be consistent within a type.
- **Q: What are struct tags used for?** → Metadata for reflection-based libraries like `encoding/json`, `encoding/xml`, `gorm`, `validate`, etc.
- **Q: Can structs implement interfaces?** → Yes, implicitly — if a struct has all the methods of an interface, it satisfies it with no explicit declaration needed.
- **Q: Are structs comparable?** → Yes, if all fields are comparable (no slices, maps, or funcs). Two struct values are equal if all their fields are equal.

---

*Next: [07 — Interfaces →](./07_interfaces.md)*
