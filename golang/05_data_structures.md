# 05 — Data Structures: Arrays, Slices & Maps

> **Goal:** Master Go's built-in collection types — the workhorses of every Go program.

---

## 1. Arrays

Fixed size, rarely used directly (slices are preferred):

```go
var a [5]int                    // [0 0 0 0 0]
b := [3]string{"a", "b", "c"}  // ["a" "b" "c"]
c := [...]int{1, 2, 3, 4}      // size inferred: [1 2 3 4]

fmt.Println(len(b))             // 3
fmt.Println(b[0])               // "a"

// Arrays are values — copying creates a full independent copy
d := b
d[0] = "z"
fmt.Println(b[0])  // "a" (unchanged)
fmt.Println(d[0])  // "z"

// 2D array
matrix := [2][3]int{{1, 2, 3}, {4, 5, 6}}
fmt.Println(matrix[1][2])  // 6
```

---

## 2. Slices

Slices are **dynamic, reference-backed views** into an array — the most commonly used collection:

```go
// Declare and initialize
s1 := []int{1, 2, 3}           // slice literal
s2 := make([]int, 5)            // length 5, cap 5
s3 := make([]int, 3, 10)        // length 3, cap 10
var s4 []int                    // nil slice (len=0, cap=0)

fmt.Println(len(s1))   // 3
fmt.Println(cap(s2))   // 5
fmt.Println(s4 == nil) // true
```

### Slice from array

```go
arr := [5]int{10, 20, 30, 40, 50}
s := arr[1:4]          // [20 30 40]  (low:high, high exclusive)
s2 := arr[:3]          // [10 20 30]
s3 := arr[2:]          // [30 40 50]
s4 := arr[:]           // full copy reference

// Slices share the underlying array!
s[0] = 99
fmt.Println(arr)  // [10 99 30 40 50] — original modified!
```

### append

```go
s := []int{1, 2, 3}
s = append(s, 4)          // [1 2 3 4]
s = append(s, 5, 6, 7)    // [1 2 3 4 5 6 7]

// Append one slice to another
a := []int{1, 2}
b := []int{3, 4, 5}
a = append(a, b...)        // [1 2 3 4 5]
```

### How append works (capacity doubling)

```go
s := make([]int, 0)
for i := 0; i < 8; i++ {
    s = append(s, i)
    fmt.Printf("len=%d cap=%d\n", len(s), cap(s))
}
// len=1 cap=1
// len=2 cap=2
// len=3 cap=4   ← doubled
// len=5 cap=8   ← doubled
```

### copy

```go
src := []int{1, 2, 3}
dst := make([]int, len(src))
n := copy(dst, src)       // returns number of elements copied
fmt.Println(dst)          // [1 2 3]
fmt.Println(n)            // 3

// Partial copy
dst2 := make([]int, 2)
copy(dst2, src)
fmt.Println(dst2)         // [1 2]
```

### Slice tricks

```go
s := []int{1, 2, 3, 4, 5}

// Delete element at index i
i := 2
s = append(s[:i], s[i+1:]...)   // [1 2 4 5]

// Insert at index i
s = []int{1, 2, 4, 5}
s = append(s[:2], append([]int{3}, s[2:]...)...)  // [1 2 3 4 5]

// Reverse
for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
    s[i], s[j] = s[j], s[i]
}

// Filter (keep even numbers)
result := s[:0]   // reuse backing array
for _, v := range s {
    if v%2 == 0 {
        result = append(result, v)
    }
}
```

### 2D slice

```go
rows, cols := 3, 4
grid := make([][]int, rows)
for i := range grid {
    grid[i] = make([]int, cols)
}
grid[1][2] = 42
```

---

## 3. Maps

Key-value pairs, unordered, O(1) average lookup:

```go
// Create
m1 := map[string]int{"a": 1, "b": 2}
m2 := make(map[string]int)
var m3 map[string]int  // nil map — reads are safe, writes PANIC!

// Read
val := m1["a"]         // 1
val2 := m1["z"]        // 0 (zero value, no panic)

// Safe read with existence check
val, ok := m1["x"]
if ok {
    fmt.Println("found:", val)
} else {
    fmt.Println("not found")
}

// Write
m2["hello"] = 42
m2["world"] = 100

// Delete
delete(m2, "hello")

// Length
fmt.Println(len(m2))   // 1

// Iteration (order is random!)
for k, v := range m2 {
    fmt.Printf("%s: %d\n", k, v)
}
```

### Map with struct values

```go
type User struct {
    Name string
    Age  int
}

users := map[string]User{
    "u1": {Name: "Alice", Age: 30},
    "u2": {Name: "Bob", Age: 25},
}

// Note: cannot modify a struct field directly through a map
// users["u1"].Age = 31  // COMPILE ERROR

// Must reassign the whole value
u := users["u1"]
u.Age = 31
users["u1"] = u

// OR use pointer values
usersPtr := map[string]*User{
    "u1": {Name: "Alice", Age: 30},
}
usersPtr["u1"].Age = 31  // OK with pointers
```

### Sorted map iteration

```go
import "sort"

m := map[string]int{"banana": 3, "apple": 1, "cherry": 2}

keys := make([]string, 0, len(m))
for k := range m {
    keys = append(keys, k)
}
sort.Strings(keys)

for _, k := range keys {
    fmt.Printf("%s: %d\n", k, m[k])
}
```

### Map as a set

```go
set := map[string]struct{}{} // struct{} uses zero bytes

set["apple"] = struct{}{}
set["banana"] = struct{}{}

_, exists := set["apple"]
fmt.Println(exists)  // true
```

---

## 4. Comparison

| Feature | Array | Slice | Map |
|---------|-------|-------|-----|
| Size | Fixed | Dynamic | Dynamic |
| Type | `[N]T` | `[]T` | `map[K]V` |
| Zero value | `[N]T{}` all zeros | `nil` | `nil` |
| Comparable | ✅ Yes (`==`) | ❌ No | ❌ No |
| Ordered | ✅ Yes | ✅ Yes | ❌ No |
| Backed by | itself | underlying array | hash table |

---

## 🎯 Interview Tips

- **Q: What is the difference between a slice and an array?** → Arrays have fixed size and are value types. Slices are dynamic, reference-backed windows into an underlying array.
- **Q: What happens when you append beyond a slice's capacity?** → Go allocates a new, larger array (usually doubles capacity), copies elements, and returns a new slice header. The original backing array is abandoned.
- **Q: Is it safe to read from a nil map?** → Yes, returns zero value. But writing to a nil map **panics**.
- **Q: Why is map iteration order random?** → Intentionally randomized by the Go runtime to prevent developers from depending on insertion order.
- **Q: How do you check if a key exists in a map?** → `val, ok := m[key]` — if `ok` is false, the key doesn't exist.
- **Q: Are slices passed by value or reference?** → A slice header (pointer to array, length, capacity) is passed by value. But the pointer inside refers to the same backing array, so mutations to elements are visible to the caller.
- **Q: How do you efficiently delete from the middle of a slice?** → `s = append(s[:i], s[i+1:]...)` — note this modifies the original slice's backing array.

---

*Next: [06 — Structs & Methods →](./06_structs_methods.md)*
