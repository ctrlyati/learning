# 06 — Control Flow, Functions, Expressions vs Statements

> **Goal:** Understand Rust's expression-oriented design, where `if`, `match`, `loop`, and blocks all return values.

## 1. Expressions vs Statements — the Core Distinction

In most languages, `if` is a statement. You can't do `x = if cond { 1 } else { 2 }` in C or Java. In Rust, you can — because `if` is an **expression** that produces a value.

The rule: a **statement** performs an action and produces no value (`let x = 5;`). An **expression** evaluates to a value (`5 + 3`, `if cond {1} else {2}`, a function call, a block).

The trailing `;` turns an expression into a statement (by discarding its value):

```rust
let x = 5;        // statement (the `let` binding)
let y = {
    let z = 3;
    z + 1         // expression — no semicolon — value of the block is 4
};                // entire block is an expression, assigned to y
println!("{y}");  // 4
```

This is why Rust functions don't usually need `return`:

```rust
fn double(x: i32) -> i32 {
    x * 2          // last expression, no semicolon → returned
}
```

Adding a semicolon would change the type to `()` and produce a compiler error:

```rust
fn double(x: i32) -> i32 {
    x * 2;         // statement; block returns ()
    // error[E0308]: mismatched types — expected `i32`, found `()`
}
```

Use explicit `return` for early exits:

```rust
fn classify(n: i32) -> &'static str {
    if n < 0 { return "negative"; }
    if n == 0 { return "zero"; }
    "positive"
}
```

---

## 2. `if`, `loop`, `while`, `for` — Each as an Expression

### `if` returns a value

```rust
let n = 7;
let parity = if n % 2 == 0 { "even" } else { "odd" };
println!("{parity}");
```

Both arms must be the same type, or the compiler complains:

```rust
let x = if cond { 5 } else { "five" };  // error: incompatible arm types
```

`else if` chains work as expected, but for many branches `match` is usually cleaner.

### `loop` is the simplest, and it returns too

```rust
let mut counter = 0;
let result = loop {
    counter += 1;
    if counter == 10 { break counter * 2; }
};
println!("{result}");   // 20
```

`break` with a value is unique to `loop` (not `while`/`for`).

**Loop labels** let you break/continue outer loops:

```rust
'outer: for i in 0..10 {
    for j in 0..10 {
        if i * j > 20 { break 'outer; }
    }
}
```

### `while` is conditional looping

```rust
let mut n = 3;
while n > 0 {
    println!("{n}");
    n -= 1;
}
```

`while` always returns `()`. For "loop until I have a value," use `loop` + `break value` or `while let` (module 05).

### `for` iterates anything implementing `IntoIterator`

```rust
for i in 0..5 { println!("{i}"); }            // 0 1 2 3 4
for i in 0..=5 { println!("{i}"); }           // inclusive: 0..5

let v = vec![10, 20, 30];
for x in &v { println!("{x}"); }              // borrow
for x in &mut v.clone() { *x += 1; }          // mutable borrow
for x in v { println!("{x}"); }               // consume

for (i, x) in v.iter().enumerate() {
    println!("{i}: {x}");
}
```

`for` is sugar over the iterator protocol — covered in module 12.

---

## 3. Functions — Signatures, Expressions, Diverging

A function:

```rust
fn add(a: i32, b: i32) -> i32 {
    a + b
}
```

Every parameter needs a type. The return type follows `->`. The body is a block expression.

**No return type** means the function returns `()` (unit):

```rust
fn greet(name: &str) {
    println!("Hi, {name}");
}
```

**The `!` ("never") type** marks functions that don't return at all (panics, infinite loops, `process::exit`):

```rust
fn die(msg: &str) -> ! {
    eprintln!("{msg}");
    std::process::exit(1);
}
```

`!` coerces to any type, which is why this works:

```rust
let x: i32 = if cond { 5 } else { panic!("bad") };
// the panic arm has type !, which fits as i32
```

### Functions Are First-Class Values

Function pointers have type `fn(args) -> ret`:

```rust
fn double(n: i32) -> i32 { n * 2 }

fn apply(f: fn(i32) -> i32, x: i32) -> i32 { f(x) }

fn main() {
    println!("{}", apply(double, 5));   // 10
}
```

For closures (which capture environment), you'll use trait bounds like `Fn`, `FnMut`, `FnOnce` — module 12.

### Generics — Brief Preview

A function can be generic over types:

```rust
fn largest<T: PartialOrd>(list: &[T]) -> &T {
    let mut largest = &list[0];
    for item in list {
        if item > largest { largest = item; }
    }
    largest
}
```

Full coverage in module 09.

---

## 4. Practical Example — A Mini Calculator REPL

Putting expressions, control flow, and functions together:

```rust
use std::io::{self, BufRead, Write};

#[derive(Debug)]
enum Op { Add, Sub, Mul, Div }

fn parse_op(s: &str) -> Option<Op> {
    match s {
        "+" => Some(Op::Add),
        "-" => Some(Op::Sub),
        "*" => Some(Op::Mul),
        "/" => Some(Op::Div),
        _   => None,
    }
}

fn apply(a: f64, op: Op, b: f64) -> Result<f64, &'static str> {
    let r = match op {
        Op::Add => a + b,
        Op::Sub => a - b,
        Op::Mul => a * b,
        Op::Div => {
            if b == 0.0 { return Err("division by zero"); }
            a / b
        }
    };
    Ok(r)
}

fn evaluate(line: &str) -> Result<f64, String> {
    let parts: Vec<&str> = line.split_whitespace().collect();
    let [a_s, op_s, b_s] = parts.as_slice() else {
        return Err(format!("expected '<num> <op> <num>', got {line:?}"));
    };
    let a: f64 = a_s.parse().map_err(|e| format!("lhs: {e}"))?;
    let b: f64 = b_s.parse().map_err(|e| format!("rhs: {e}"))?;
    let op = parse_op(op_s).ok_or_else(|| format!("unknown op {op_s:?}"))?;
    apply(a, op, b).map_err(String::from)
}

fn main() {
    let stdin = io::stdin();
    let mut out = io::stdout().lock();
    loop {
        write!(out, "> ").unwrap();
        out.flush().unwrap();
        let mut line = String::new();
        if stdin.lock().read_line(&mut line).unwrap() == 0 { break; }
        let line = line.trim();
        if line.is_empty() { continue; }
        if line == "quit" { break; }
        match evaluate(line) {
            Ok(v)  => println!("= {v}"),
            Err(e) => println!("error: {e}"),
        }
    }
}
```

What this demonstrates:
- `parse_op` returns `Option`, idiomatic for "lookup that may miss."
- `apply` returns `Result` because division can fail.
- `evaluate` uses `let else` to bail out when input shape is wrong, and `?` to short-circuit on parse errors (formal coverage in module 08).
- The main `loop` returns `()` but uses `break` to exit.
- `match` makes the success/failure handling exhaustive.

---

## 5. Common Mistakes & Gotchas

- **`expected i32, found ()`** at end of a function — you put a semicolon after the last expression, turning it into a statement. Remove the `;`.
- **Forgetting that `if`/`match` return values.** Beginners write five lines of `let mut x; if cond { x = 1 } else { x = 2 }`. Idiomatic: `let x = if cond { 1 } else { 2 };`.
- **`while true { ... }` instead of `loop { ... }`.** Use `loop` — Clippy will tell you, and `loop` is clearer about your intent.
- **Mismatched arm types in `if`.** Both branches must produce the same type. To return early in only one branch, use `return ...;` or `panic!()` — both have type `!` and fit anywhere.
- **Trying to return from inside a closure with `return`.** `return` inside a closure exits the *closure*, not the enclosing function.
- **Using `for i in 0..v.len()` when iterating a `Vec`.** Idiomatic: `for x in &v` or `for (i, x) in v.iter().enumerate()`. Indexing `v[i]` does bounds-checking on every access.
- **`continue` and `break` in the wrong loop.** When nested, label the outer loop (`'outer: for ...`) and `break 'outer`.
- **Forgetting that blocks introduce scope.** A `let` inside `{ }` is dropped at the closing brace — handy for limiting borrow lifetimes.

---

## 🎯 Key Takeaways

- Almost everything is an expression. `if`, `match`, `loop`, blocks — all produce values you can assign or return.
- The trailing `;` is meaningful: it turns an expression into a statement and discards its value.
- Functions return their last expression; explicit `return` is for early exits.
- `loop` + `break value` is the only loop form that yields a value; use it for "search until found."
- `!` (never type) lets diverging expressions like `panic!` fit into any context.

*[← prev](./05_structs_enums_pattern_matching.md) | [next →](./07_collections.md)*
