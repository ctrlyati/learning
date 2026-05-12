# 04 — Pointers

> **Goal:** Understand how Go handles memory references — a concept crucial for performance and mutability.

---

## 1. What is a Pointer?

A pointer stores the **memory address** of a value rather than the value itself.

```go
x := 42
p := &x          // p is a pointer to x (&x = address of x)

fmt.Println(x)   // 42          (value)
fmt.Println(p)   // 0xc000014090 (memory address)
fmt.Println(*p)  // 42          (dereferenced value)

*p = 100         // modify through pointer
fmt.Println(x)   // 100
```

Key operators:
- `&x` — get the address of `x` (reference)
- `*p` — dereference the pointer (get the value at the address)

---

## 2. Pointer Types

```go
var p *int           // pointer to int, zero value is nil
fmt.Println(p)       // <nil>
fmt.Println(p == nil) // true

// Safe: check before dereferencing
if p != nil {
    fmt.Println(*p)
}
```

---

## 3. new() vs &

```go
// new() allocates memory, returns a pointer to zero value
p1 := new(int)         // *int pointing to 0
fmt.Println(*p1)       // 0
*p1 = 42
fmt.Println(*p1)       // 42

// & with composite literal (more common)
type Point struct{ X, Y int }
p2 := &Point{1, 2}    // *Point
fmt.Println(p2.X)     // 1 (no need to write (*p2).X)
```

---

## 4. Pass by Value vs Pass by Reference

Go is **pass by value** — functions receive copies. Pointers let you pass references:

```go
// Pass by value — original NOT modified
func doubleValue(n int) {
    n *= 2  // only modifies the copy
}

// Pass by pointer — original IS modified
func doublePointer(n *int) {
    *n *= 2
}

x := 10
doubleValue(x)
fmt.Println(x)  // 10 (unchanged)

doublePointer(&x)
fmt.Println(x)  // 20 (changed!)
```

### Practical example — swap

```go
func swap(a, b *int) {
    *a, *b = *b, *a
}

x, y := 1, 2
swap(&x, &y)
fmt.Println(x, y)  // 2 1
```

---

## 5. Pointers to Structs

```go
type User struct {
    Name string
    Age  int
}

// Both access syntaxes work
u := &User{Name: "Yati", Age: 28}
fmt.Println((*u).Name)  // explicit dereference
fmt.Println(u.Name)     // shorthand — Go auto-dereferences
u.Age = 29              // modifies the original struct
```

---

## 6. When to Use Pointers

| Situation | Use Pointer? |
|-----------|-------------|
| Large struct that's expensive to copy | ✅ Yes |
| Need to mutate the value in a function | ✅ Yes |
| Nullable/optional value | ✅ Yes (`*T` can be `nil`) |
| Small struct/primitive, no mutation needed | ❌ No — value is cheaper |
| Method needs to mutate the receiver | ✅ Yes (pointer receiver) |

```go
// When NOT to use pointer (small, immutable)
func area(w, h float64) float64 {
    return w * h
}

// When to use pointer (large struct, mutation)
func updateUser(u *User) {
    u.Name = "Updated"
}
```

---

## 7. Pointer to Pointer (rare)

```go
x := 42
p := &x
pp := &p

fmt.Println(**pp)  // 42
**pp = 100
fmt.Println(x)     // 100
```

---

## 8. Nil Pointer Dereference (Common Bug)

```go
var p *int
fmt.Println(*p)  // PANIC: nil pointer dereference

// Always guard:
func safeRead(p *int) int {
    if p == nil {
        return 0
    }
    return *p
}
```

---

## 9. Pointers and Interfaces

```go
type Stringer interface {
    String() string
}

type Name struct{ value string }

// Value receiver — both *Name and Name satisfy the interface
func (n Name) String() string { return n.value }

// Pointer receiver — ONLY *Name satisfies the interface
func (n *Name) SetValue(v string) { n.value = v }
```

---

## 🎯 Interview Tips

- **Q: Is Go pass by value or reference?** → Always **pass by value**. But you can pass a pointer (which is a value containing an address) to achieve reference-like behavior.
- **Q: What is the zero value of a pointer?** → `nil`
- **Q: What happens when you dereference a nil pointer?** → Panic at runtime.
- **Q: What's the difference between `new(T)` and `&T{}`?** → Both allocate and return a pointer. `new(T)` gives you a zero-value `T`. `&T{}` lets you initialize fields.
- **Q: When should you use a pointer receiver vs value receiver on a method?** → Use pointer receiver when the method needs to modify the struct, or when the struct is large. Value receivers are fine for small, read-only methods.
- **Q: Can you have a pointer to an interface?** → Technically yes, but it's almost always a mistake. Pass the interface value directly.

---

*Next: [05 — Data Structures →](./05_data_structures.md)*
