# 12 — Closures, Iterators, Functional Patterns

> **Goal:** Use Rust's iterator and closure machinery for concise, fast, allocation-free data processing.

## 1. Closures — Functions That Capture Their Environment

A closure is an anonymous function that can capture variables from the surrounding scope:

```rust
fn main() {
    let factor = 3;
    let triple = |x| x * factor;
    println!("{}", triple(7));    // 21
}
```

Syntax:

```rust
let add = |a, b| a + b;             // type-inferred params, expression body
let add: fn(i32, i32) -> i32 = |a, b| a + b;   // typed
let block = |x| {                    // multi-statement body
    let y = x + 1;
    y * 2
};
```

The compiler synthesizes a unique anonymous struct for each closure, holding the captured variables. There's no runtime closure object — it's all monomorphized.

### The Three Closure Traits

How a closure captures depends on what it does with the captures:

| Trait    | What it does                                                    | When chosen                                |
| -------- | --------------------------------------------------------------- | ------------------------------------------ |
| `Fn`     | borrows captures immutably (`&T`); callable many times          | closure only reads captures                |
| `FnMut`  | borrows captures mutably (`&mut T`); callable many times        | closure mutates captures                   |
| `FnOnce` | takes captures by ownership; callable once                      | closure consumes (moves) captures          |

Every closure implements at least `FnOnce`; if it doesn't move, also `FnMut`; if it doesn't mutate, also `Fn`. The compiler picks the most permissive automatically.

To accept a closure as a parameter, bound by what you need:

```rust
fn apply<F: Fn(i32) -> i32>(f: F, x: i32) -> i32 { f(x) }

fn run_n_times<F: FnMut()>(mut f: F, n: u32) {
    for _ in 0..n { f(); }
}

fn run_once<F: FnOnce() -> String>(f: F) -> String { f() }
```

### `move` — Force Ownership Capture

```rust
let s = String::from("hello");
let greet = move || println!("{s}");    // s moved into closure
greet();
// println!("{s}");                     // ERROR — s is gone
```

`move` is essential when sending closures to threads (the captures must outlive the spawning function).

### Returning Closures

```rust
fn make_adder(x: i32) -> impl Fn(i32) -> i32 {
    move |y| x + y
}

fn main() {
    let add5 = make_adder(5);
    println!("{}", add5(3));    // 8
}
```

Use `Box<dyn Fn(...) -> ...>` if you need to return one of several closures (different anonymous types):

```rust
fn pick(neg: bool) -> Box<dyn Fn(i32) -> i32> {
    if neg { Box::new(|x| -x) } else { Box::new(|x| x) }
}
```

---

## 2. Iterators — Lazy, Composable, Zero-Cost

An iterator is anything that implements `Iterator`:

```rust
trait Iterator {
    type Item;
    fn next(&mut self) -> Option<Self::Item>;
    // 70+ provided methods built on top of next
}
```

`for` is sugar over this. Manual use:

```rust
let v = vec![1, 2, 3];
let mut it = v.iter();
while let Some(x) = it.next() {
    println!("{x}");
}
```

### Three Iteration Forms

```rust
let v = vec![1, 2, 3];
v.iter()          // -> Iterator<Item = &T>
v.iter_mut()      // -> Iterator<Item = &mut T>
v.into_iter()     // -> Iterator<Item = T>      consumes v
```

Same for slices, hash maps, etc. Pick by ownership intent.

### Adapters and Consumers

**Adapters** transform an iterator into another iterator. They're lazy — nothing runs until consumed.

```rust
let doubled = v.iter().map(|x| x * 2);                // Map<...>
let evens   = v.iter().filter(|x| **x % 2 == 0);
let chunks  = v.iter().enumerate();                    // (index, value)
let pairs   = v.iter().zip(["a","b","c"].iter());
let chained = v.iter().chain([4, 5, 6].iter());
let unique  = v.iter().copied().take(5).skip(2).rev();
```

**Consumers** drive the chain to completion:

```rust
let total: i32 = v.iter().sum();
let max  = v.iter().max();                             // Option<&T>
let any_neg = v.iter().any(|x| *x < 0);
let all_pos = v.iter().all(|x| *x > 0);
let count = v.iter().count();
let collected: Vec<i32> = v.iter().map(|x| x * 2).collect();
let joined: String = ["a","b","c"].iter().copied().collect::<Vec<_>>().join(", ");

// fold — left reduce
let sum2 = v.iter().fold(0, |acc, x| acc + x);

// reduce — fold with first element as initial
let max_or_none: Option<i32> = v.iter().copied().reduce(i32::max);
```

`collect` is generic in the target type — a turbofish or annotation tells it where to land:

```rust
let s: HashSet<i32> = v.iter().copied().collect();
let v2 = v.iter().map(|x| x * 2).collect::<Vec<_>>();
```

### `Result` in Iterator Chains

`collect` can flip `Vec<Result<T, E>>` to `Result<Vec<T>, E>` — an early-exit short-circuit:

```rust
let raw = ["1", "2", "3"];
let nums: Result<Vec<i32>, _> = raw.iter().map(|s| s.parse::<i32>()).collect();
// Ok(vec![1, 2, 3])

let raw = ["1", "x", "3"];
let nums: Result<Vec<i32>, _> = raw.iter().map(|s| s.parse::<i32>()).collect();
// Err(...)
```

### Zero-Cost — What It Actually Means

This:

```rust
let sum: i32 = (1..=1_000).filter(|x| x % 2 == 0).map(|x| x * x).sum();
```

Compiles to assembly equivalent to a hand-rolled `for` loop with an `if` and an accumulator. No allocations, no virtual dispatch, no per-element overhead. The compiler inlines the closures, fuses the iterator stages, and optimizes the result. This is the "zero-cost abstractions" promise made tangible.

---

## 3. Implementing Your Own Iterator

```rust
struct Counter { current: u32, max: u32 }

impl Counter {
    fn new(max: u32) -> Self { Self { current: 0, max } }
}

impl Iterator for Counter {
    type Item = u32;
    fn next(&mut self) -> Option<u32> {
        if self.current < self.max {
            self.current += 1;
            Some(self.current)
        } else {
            None
        }
    }
}

fn main() {
    let total: u32 = Counter::new(5).sum();   // 15
    let zipped: Vec<(u32, u32)> = Counter::new(5).zip(Counter::new(5).skip(1)).collect();
    println!("{zipped:?}");
}
```

Implementing `Iterator` is one method (`next`); you immediately get `map`, `filter`, `sum`, `zip`, etc., for free. This is composition over inheritance, lived.

---

## 4. Practical Example — Log Analyzer

Take a log file, extract HTTP error responses, group by path, count, and sort:

```rust
use std::collections::HashMap;

fn analyze<'a>(lines: impl Iterator<Item = &'a str>) -> Vec<(String, u32)> {
    let counts = lines
        .filter_map(|line| {
            // expected format: "<ip> <method> <path> <status>"
            let parts: Vec<&str> = line.split_whitespace().collect();
            let [_ip, _method, path, status] = parts.as_slice() else { return None; };
            let code: u16 = status.parse().ok()?;
            if (400..600).contains(&code) {
                Some(path.to_string())
            } else {
                None
            }
        })
        .fold(HashMap::<String, u32>::new(), |mut m, p| {
            *m.entry(p).or_insert(0) += 1;
            m
        });

    let mut out: Vec<(String, u32)> = counts.into_iter().collect();
    out.sort_by(|a, b| b.1.cmp(&a.1).then_with(|| a.0.cmp(&b.0)));
    out
}

fn main() {
    let log = "\
1.2.3.4 GET /home 200
5.6.7.8 GET /api/users 500
9.0.1.2 POST /api/login 401
1.2.3.4 GET /api/users 500
9.0.1.2 GET /missing 404
";
    for (path, count) in analyze(log.lines()) {
        println!("{count:>3}  {path}");
    }
}
```

What this shows:
- `impl Iterator<Item = &'a str>` accepts any iterator of string slices — works on `lines()`, on a `Vec`'s `iter()`, on anything.
- `filter_map` combines `filter` and `map` — return `None` to skip, `Some(x)` to include.
- `let else` for early-out destructuring.
- `fold` builds the map without an intermediate `Vec`.
- The whole pipeline is one allocation (`out`'s `Vec`); the rest fuses into a single pass.

---

## 5. Common Mistakes & Gotchas

- **"closure may outlive the current function" when spawning a thread.** Add `move` to the closure: `thread::spawn(move || { ... })`.
- **`expected closure that implements Fn, found one that implements FnMut`.** You're mutating a capture inside a closure passed to something expecting `Fn`. Restructure or use `RefCell` (module 13) if you must.
- **Iterating with indices: `for i in 0..v.len() { let x = v[i]; ... }`.** Idiomatic: `for x in &v` or `for (i, x) in v.iter().enumerate()`.
- **`collect::<Vec<_>>().iter()`** — collecting then re-iterating is wasted allocation. Most of the time, just chain.
- **`.iter().count()` instead of `.len()`** on a Vec — count() iterates; len() is O(1).
- **Lazy iterators that "do nothing."** `v.iter().for_each(|_| ())` runs; `v.iter().map(|x| println!("{x}"));` does NOT — `map` is lazy and you discarded it. Use `.for_each` for side effects.
- **Forgetting that `iter()` yields references.** `v.iter().map(|x| x * 2)` may need `*x * 2` or `.copied()` first if `T` is `Copy`. Otherwise you're trying to multiply `&i32 * i32`.
- **Using `into_iter()` and then trying to use the original `Vec`.** `into_iter` consumes. If the source is still needed, use `iter()` or `iter().cloned()`.
- **Closures with too-permissive captures.** A closure that only reads `x` will still keep `x`'s ownership/borrow alive for the closure's lifetime. Drop the closure when done, or use `move` deliberately.

---

## 🎯 Key Takeaways

- Closures are anonymous functions with captured environment; `Fn`/`FnMut`/`FnOnce` describe how they use their captures.
- Iterators are lazy — adapters transform, consumers drive. The chain doesn't run until something pulls (or you call `.collect`/`.for_each`/etc.).
- Iterator chains compile to the same machine code as a hand-written `for` loop — zero-cost is real and measurable.
- `collect` is shape-shifting — it can produce `Vec`, `HashMap`, `Result<Vec<_>, _>`, `String`, and more, depending on the target type.
- Implementing `Iterator` for your own type takes one method (`next`) and unlocks the entire combinator library.

*[← prev](./11_modules_crates_workspaces.md) | [next →](./13_smart_pointers.md)*
