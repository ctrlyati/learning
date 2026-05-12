# 07 — Collections: `Vec`, `String`, `HashMap`, Slices

> **Goal:** Master the three collection types you'll use daily, and understand how slices unify them.

## 1. `Vec<T>` — the Workhorse

`Vec<T>` is a growable, heap-allocated, contiguous array. Roughly: C++'s `std::vector`, Java's `ArrayList`, Python's `list`.

```rust
let v: Vec<i32> = Vec::new();
let mut v = vec![1, 2, 3];        // macro literal
v.push(4);
v.extend([5, 6]);
v.pop();                          // returns Option<T>
let first = v[0];                 // panics on out-of-bounds
let maybe = v.get(99);            // returns Option<&T>
```

A `Vec` is three words: pointer, length, capacity.

- **length** — number of valid items.
- **capacity** — slots allocated; growing past it triggers reallocation.

When the buffer needs to grow, `Vec` allocates a larger one (typically 2×) and moves elements over. References *into* the old buffer become invalid — which is why the borrow checker forbids holding a reference while pushing:

```rust
let mut v = vec![1, 2, 3];
let r = &v[0];
v.push(4);          // error: cannot borrow `v` as mutable because it is also borrowed as immutable
println!("{r}");
```

Iteration takes three forms:

```rust
for x in &v       { /* x: &T   — read */ }
for x in &mut v   { /* x: &mut T — write through *x */ }
for x in v        { /* x: T    — consume; v moved */ }
```

Common methods you'll use constantly:

```rust
v.len(); v.is_empty(); v.clear();
v.contains(&x); v.iter().position(|x| *x == 5);
v.sort(); v.sort_by(|a, b| b.cmp(a));
v.dedup();
v.retain(|x| *x > 0);            // in-place filter
let mut v2 = v.clone();
v.append(&mut v2);                // moves all of v2 into v
```

For specific element types:

```rust
let nums = vec![1, 2, 3, 4];
let sum: i32 = nums.iter().sum();
let max = nums.iter().max().unwrap();
```

Iterators get a full module (12). For now: any time you'd write a `for` loop to transform/filter/aggregate, there's likely a one-liner.

---

## 2. `String` and `&str` — Two Halves of the Same Story

This is the source of more confusion than any other Rust topic. There are two string types because there are two roles:

- **`String`** — owned, growable, heap-allocated, UTF-8.
- **`&str`** — a borrowed view: pointer + length, into UTF-8 bytes owned by someone else.

A string literal `"hello"` has type `&'static str` — it lives in the program's read-only data forever.

### Constructing

```rust
let s1 = String::new();
let s2 = String::from("hello");
let s3 = "hello".to_string();
let s4: String = "hello".into();
let s5 = format!("{} {}", "hello", 42);
```

### Mutating

```rust
let mut s = String::from("hello");
s.push(' ');
s.push_str("world");
s += "!";                         // shorthand for push_str on a &str
let combined = format!("{s}!!!"); // doesn't move s
```

### Borrowing

```rust
let s = String::from("hello world");
let view: &str = &s;             // deref coercion: String -> &str
let hello: &str = &s[..5];        // slice
let world: &str = &s[6..];
```

### Iterating

UTF-8 is *not* fixed-width. A `char` is a Unicode scalar (4 bytes max). Iterate with intent:

```rust
let s = "héllo";
for b in s.bytes()       { /* u8, raw bytes */ }
for c in s.chars()       { /* char, Unicode scalars */ }
for (i, c) in s.char_indices() { /* (byte index, char) */ }
```

You **cannot** index a string by integer:

```rust
let c = s[0];   // error: `String` cannot be indexed by `{integer}`
```

Why? Because `s[0]` would be ambiguous — bytes? chars? graphemes? Rust forces you to choose. For grapheme clusters (`👨‍👩‍👧` is one "user-perceived character" but several scalars), use the `unicode-segmentation` crate.

### Function Parameters — Always Prefer `&str`

```rust
fn greet(name: &str) {
    println!("hi, {name}");
}

greet("literal");
greet(&String::from("owned"));
```

`&str` accepts both literal and owned strings via deref coercion. Use `String` in parameters only when you specifically need ownership (e.g., to store it).

---

## 3. `HashMap<K, V>` — Keyed Lookup

```rust
use std::collections::HashMap;

let mut scores: HashMap<String, i32> = HashMap::new();
scores.insert(String::from("alice"), 90);
scores.insert(String::from("bob"), 75);

if let Some(&s) = scores.get("alice") {
    println!("alice: {s}");
}

for (name, score) in &scores {
    println!("{name}: {score}");
}
```

### Ownership Behavior

`insert` moves the key and value (unless they're `Copy`). `get(&key)` borrows. `remove(&key)` returns `Option<V>` and moves out.

### The `entry` API — the killer feature

For "insert if absent, modify if present," use `entry`:

```rust
let text = "the quick brown fox jumps over the lazy dog";
let mut counts: HashMap<&str, u32> = HashMap::new();
for word in text.split_whitespace() {
    *counts.entry(word).or_insert(0) += 1;
}
```

Variants:

```rust
counts.entry("k").or_insert_with(|| expensive_default());
counts.entry("k").and_modify(|v| *v += 1).or_insert(1);
```

### Choosing a Hasher

Rust's default `HashMap` uses a DOS-resistant hasher (SipHash). For trusted, performance-sensitive workloads, switch to `FxHashMap` (from `rustc-hash`) or `AHashMap` (from `ahash`) — significantly faster, same API.

### Other Useful Maps

- **`BTreeMap<K, V>`** — keys sorted, slower lookup, supports range queries.
- **`HashSet<T>` / `BTreeSet<T>`** — keys without values.

---

## 4. Slices — One Concept, Many Uses

A slice `&[T]` is a fat pointer: pointer + length. It views into something contiguous without owning it. Almost every read-only API in the standard library takes a slice rather than a `Vec`:

```rust
fn sum(xs: &[i32]) -> i32 {
    xs.iter().sum()
}

fn main() {
    let v = vec![1, 2, 3, 4, 5];
    let arr = [10, 20, 30];
    println!("{}", sum(&v));         // &Vec<i32> coerces to &[i32]
    println!("{}", sum(&arr));       // &[i32; 3] coerces to &[i32]
    println!("{}", sum(&v[1..4]));   // explicit slice
}
```

`&str` is exactly the slice type for strings. `&mut [T]` lets you write through:

```rust
fn double_in_place(xs: &mut [i32]) {
    for x in xs { *x *= 2; }
}
```

### Real Mini-Example — A Word Frequency Top-N

Combining everything in this module:

```rust
use std::collections::HashMap;

fn top_words(text: &str, n: usize) -> Vec<(String, u32)> {
    let mut counts: HashMap<String, u32> = HashMap::new();
    for word in text.split_whitespace() {
        let key = word.trim_matches(|c: char| !c.is_alphanumeric())
                      .to_lowercase();
        if key.is_empty() { continue; }
        *counts.entry(key).or_insert(0) += 1;
    }
    let mut pairs: Vec<(String, u32)> = counts.into_iter().collect();
    pairs.sort_by(|a, b| b.1.cmp(&a.1).then_with(|| a.0.cmp(&b.0)));
    pairs.truncate(n);
    pairs
}

fn main() {
    let text = "The fox jumps. The fox runs! The lazy dog watches.";
    for (word, count) in top_words(text, 3) {
        println!("{word:>10}: {count}");
    }
}
```

What's happening:
- Take a `&str` (cheap, doesn't take ownership of caller's text).
- Build an owned `HashMap<String, u32>` because keys must be owned (the slices into `text` would tie us to the caller's lifetime; sometimes useful, sometimes not).
- Convert into a `Vec` for sorting (HashMap is unordered).
- Stable sort by descending count, then ascending alphabetical.

---

## 5. Common Mistakes & Gotchas

- **`cannot borrow as mutable because also borrowed as immutable`** while iterating + mutating a `Vec`. Collect indices first, or use `Vec::retain`/`drain`/`iter_mut`.
- **Indexing a `String` with `s[0]`**. Doesn't compile. Use `s.chars().next()`, `s.as_bytes()[0]`, or `s.get(0..1)`.
- **Slicing strings on byte boundaries inside a multibyte char.** `&"é"[..1]` panics. Use `s.char_indices()` to find safe boundaries.
- **`HashMap<&str, _>` outliving the source.** Keys borrow from somewhere; that source must outlive the map. Use `HashMap<String, _>` if in doubt.
- **Cloning a whole `Vec` when you only need to read it.** Pass `&[T]` instead.
- **`vec![Vec::new(); n]` to make a 2D grid.** `vec![T; n]` requires `T: Clone` and clones n times — fine for `Vec::new()` (cheap). But `vec![Mutex::new(0); n]` won't compile because `Mutex` isn't `Clone`.
- **Forgetting `use std::collections::HashMap;`** — `HashMap` is not in the prelude.
- **Treating `Vec::with_capacity(n)` as `vec![T::default(); n]`.** The first allocates capacity but length is 0; you still need to push. The second creates a length-`n` vector.
- **`vec.contains(&item)` is O(n).** For frequent lookups, use a `HashSet`.

---

## 🎯 Key Takeaways

- `Vec<T>` is your default growable container; `[T; N]` for compile-time-known sizes; `&[T]` for borrowed views.
- `String` is owned and mutable; `&str` is a borrowed UTF-8 slice. **Function parameters should almost always take `&str`.**
- UTF-8 means no integer indexing into strings. Use `chars()`, `bytes()`, or `char_indices()` deliberately.
- `HashMap`'s `entry` API is the idiomatic way to do "increment if exists, insert otherwise."
- Slices unify arrays, vectors, and string views under one borrowing API — design around them.

*[← prev](./06_control_flow_functions_expressions.md) | [next →](./08_error_handling.md)*
