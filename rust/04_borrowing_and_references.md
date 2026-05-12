# 04 — Borrowing & References

> **Goal:** Use references to share data without transferring ownership, and learn the borrow checker's two ironclad rules.

## 1. Borrowing — Lending Without Giving Away

In module 03, every function call moved ownership. That's verbose and prevents the caller from reusing the value. Borrowing fixes both.

A **reference** is created with `&` and read like a pointer that's guaranteed to be valid:

```rust
fn len(s: &String) -> usize {
    s.len()
}                     // s goes out of scope, but it didn't own anything, so nothing dropped

fn main() {
    let s = String::from("hello");
    let n = len(&s);      // pass an immutable reference — no move
    println!("{s} has length {n}");   // s is still valid
}
```

The analogy: instead of giving the book to a friend, you let them read it over your shoulder. The book never leaves your hands, so when they're done you still have it.

To borrow mutably, use `&mut`:

```rust
fn push_excitement(s: &mut String) {
    s.push('!');
}

fn main() {
    let mut s = String::from("hi");
    push_excitement(&mut s);
    println!("{s}");   // hi!
}
```

Two requirements for `&mut s` to compile:
1. `s` itself must be declared `mut`.
2. The function must declare it accepts `&mut String`.

---

## 2. The Two Borrowing Rules

These are the rules the borrow checker enforces. Memorize them:

**Rule 1: At any given time, you can have either**
- **any number of immutable references (`&T`)**, OR
- **exactly one mutable reference (`&mut T`)**.

Not both. Never both.

**Rule 2: References must always be valid** (never dangling, never pointing at freed memory).

That's it. Every borrow-checker error you ever see comes from one of these two rules.

### Rule 1 in Action

Multiple immutable readers — fine:

```rust
let s = String::from("hello");
let r1 = &s;
let r2 = &s;
println!("{r1} {r2} {s}");
```

One mutable writer alone — fine:

```rust
let mut s = String::from("hello");
let r = &mut s;
r.push('!');
println!("{r}");
```

Mixing — error:

```rust
let mut s = String::from("hello");
let r1 = &s;
let r2 = &mut s;        // error[E0502]: cannot borrow `s` as mutable because it is also borrowed as immutable
println!("{r1} {r2}");
```

Why? If a writer mutates `s` (perhaps reallocating its buffer), any reader holding a pointer to the old buffer would now be looking at freed memory. Banning concurrent reader+writer at compile time is what eliminates an entire class of bugs.

### Non-Lexical Lifetimes (NLL) — the rule is smarter than it sounds

The borrow checker knows when a reference is *last used*, not just when it goes out of scope. So this works:

```rust
let mut s = String::from("hello");
let r1 = &s;
let r2 = &s;
println!("{r1} {r2}");      // last use of r1, r2 here
let r3 = &mut s;            // OK — immutable borrows are no longer in use
r3.push('!');
println!("{r3}");
```

This used to be an error in Rust 2015. Modern Rust scopes borrows to their actual usage span.

### Rule 2 — No Dangling References

```rust
fn dangle() -> &String {
    let s = String::from("hello");
    &s
}                  // s dropped here — the returned reference would point to garbage
// error[E0106]: missing lifetime specifier
//   = help: this function's return type contains a borrowed value, but there is no value
//           for it to be borrowed from
```

Fix: return the owned `String`, not a reference. We'll see *legitimate* reference-returning functions in module 10 (lifetimes).

---

## 3. Variations — `&T`, `&mut T`, Slices, Method Receivers

References show up in several flavors.

### `&T` and `&mut T` on Function Signatures

A method on `Vec<i32>`:

```rust
fn sum(v: &Vec<i32>) -> i32 { v.iter().sum() }
fn double(v: &mut Vec<i32>) { for x in v { *x *= 2; } }
```

Reading a reference uses **automatic dereferencing** for method calls (`v.iter()` works on `&Vec`), but for plain value access you sometimes need `*r`:

```rust
let x = 5;
let r = &x;
let y = *r + 1;     // explicit deref to read the value
```

In `for x in v` where `v: &mut Vec<i32>`, each `x` is `&mut i32`, so `*x *= 2` writes through the reference.

### Slices — `&[T]` and `&str`

A **slice** is a reference to a contiguous range of elements without owning them. It's two words: a pointer + a length.

```rust
let v = vec![1, 2, 3, 4, 5];
let s: &[i32] = &v[1..4];   // [2, 3, 4]
println!("{}", s.len());     // 3
```

Strings have their own slice type, `&str`:

```rust
let owned: String = String::from("hello world");
let hello: &str = &owned[..5];      // "hello"
let world: &str = &owned[6..];      // "world"
```

String literals like `"hello"` are *already* `&'static str` — slices into the program's read-only data. This is why you see `&str` more often than `&String` in function signatures: `&str` accepts both.

```rust
fn greet(name: &str) {
    println!("Hi, {name}");
}

fn main() {
    greet("literal");                     // &'static str
    greet(&String::from("owned"));        // &str via deref coercion
}
```

**Always prefer `&str` over `&String` in function parameters.** Same for `&[T]` over `&Vec<T>`. Both are strictly more general.

### Method Receivers — `&self`, `&mut self`, `self`

These are syntactic sugar for the same three patterns:

```rust
struct Counter { n: u32 }

impl Counter {
    fn read(&self) -> u32 { self.n }            // borrow immutably
    fn bump(&mut self)    { self.n += 1; }       // borrow mutably
    fn into_n(self) -> u32 { self.n }            // consume self
}
```

`into_*` and `from_*` are common naming conventions for ownership-taking conversions.

---

## 4. Practical Example — A Caching Word Counter

Let's build a function that takes a borrowed string slice, counts words, and returns owned data. This shows the most common Rust function shape: borrow inputs, return owned outputs.

```rust
use std::collections::HashMap;

fn word_counts(text: &str) -> HashMap<String, u32> {
    let mut counts = HashMap::new();
    for word in text.split_whitespace() {
        // word is &str; we need an owned String to use as a HashMap key
        *counts.entry(word.to_string()).or_insert(0) += 1;
    }
    counts
}

fn print_top(counts: &HashMap<String, u32>, n: usize) {
    let mut pairs: Vec<(&String, &u32)> = counts.iter().collect();
    pairs.sort_by(|a, b| b.1.cmp(a.1));
    for (word, count) in pairs.iter().take(n) {
        println!("{word:>15}: {count}");
    }
}

fn main() {
    let text = "the quick brown fox jumps over the lazy dog the the fox";
    let counts = word_counts(text);     // text borrowed, counts owned
    print_top(&counts, 3);              // borrow counts, don't consume
    println!("vocab size: {}", counts.len());   // counts still usable
}
```

What to notice:
- `word_counts(text: &str)` takes a borrow — caller keeps `text`.
- `word_counts` returns an owned `HashMap` — ownership transferred to `main`.
- `print_top(&counts, ...)` borrows `counts`, so we can use it again on the next line.
- `pairs` holds `(&String, &u32)` — references back into `counts`. They're valid as long as `counts` lives.

This shape — **borrow inputs, return owned outputs** — is the default. Reach for `&mut` only when in-place mutation is the clearer design.

---

## 5. Common Mistakes & Gotchas

- **`cannot borrow as mutable because also borrowed as immutable`** — somewhere above, you took an `&` reference that's still in use when you tried `&mut`. Find the `let r = &x` and either drop it earlier or restructure.
- **`cannot borrow as mutable, as it is not declared as mutable`** — you forgot `let mut` on the variable itself. Both the binding *and* the reference type need to opt in.
- **Iterator invalidation:** mutating a `Vec` while you hold an iterator into it is a compile error in Rust (it's UB in C++). Fix: collect the indices first, then mutate; or use `Vec::retain`, `Vec::drain`, etc.
- **`&String` in function signatures.** Works, but unnecessarily restrictive. Use `&str`. Same for `&Vec<T>` → `&[T]`.
- **Returning a reference from a function without explaining its lifetime.** The compiler will demand a lifetime annotation (module 10). Often the right fix is to return owned data instead.
- **Passing `&mut self` everywhere "just in case."** It locks the caller out of any concurrent immutable access. Default to `&self`; only widen when needed.
- **Reborrowing confusion:** `let r2: &mut T = &mut *r1;` is a *reborrow* — `r1` is temporarily inert until `r2` is no longer used. The compiler does this automatically when you pass `r1` to a function expecting `&mut T`.
- **Holding references across an `await`** (in async code, module 15) — sometimes forbidden because the future may move between threads. We'll deal with this when we get there.

---

## 🎯 Key Takeaways

- `&T` is shared/read-only, `&mut T` is exclusive/read-write. The two are mutually exclusive at any point in time.
- The borrow checker uses non-lexical lifetimes — references are "live" only between their first and last use, not their full scope.
- Prefer `&str` over `&String` and `&[T]` over `&Vec<T>` in function signatures — strictly more general.
- The default function shape is: **borrow inputs, return owned outputs.** Use `&mut` only when in-place mutation is clearer.
- Every borrow-checker error reduces to one of two rules. When stuck, ask: "where's the conflicting borrow?" or "where's the dangling reference?"

*[← prev](./03_ownership.md) | [next →](./05_structs_enums_pattern_matching.md)*
