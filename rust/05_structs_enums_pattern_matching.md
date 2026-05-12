# 05 — Structs, Enums, Pattern Matching, Option/Result

> **Goal:** Use Rust's algebraic data types and pattern matching — and understand why `Option`/`Result` make `null` and exceptions obsolete.

## 1. Structs — Bundling Named Data

A `struct` is a record with named fields:

```rust
struct User {
    username: String,
    email: String,
    active: bool,
    sign_in_count: u32,
}

fn main() {
    let u = User {
        username: String::from("yati"),
        email: String::from("y@example.com"),
        active: true,
        sign_in_count: 1,
    };
    println!("{}", u.username);
}
```

Mutation requires the whole binding to be `mut`:

```rust
let mut u = User { /* ... */ };
u.email = String::from("new@example.com");
```

Rust does not have field-level mutability — the entire struct is mutable or not.

**Struct update syntax** (`..base`) lets you build a new struct from an old one:

```rust
let u2 = User { email: String::from("y2@example.com"), ..u };
// u is now partially moved (username moved into u2); use cautiously
```

**Tuple structs** name the type but not the fields — useful for newtype wrappers:

```rust
struct Wrapper(Vec<String>);
struct Color(u8, u8, u8);

let red = Color(255, 0, 0);
println!("{}", red.0);
```

**Unit structs** carry no data — useful for marker types and trait implementations:

```rust
struct AlwaysEqual;
```

Methods live in `impl` blocks:

```rust
impl User {
    fn new(username: String, email: String) -> Self {
        Self { username, email, active: true, sign_in_count: 0 }
    }

    fn deactivate(&mut self) {
        self.active = false;
    }

    fn is_active(&self) -> bool { self.active }
}
```

`Self` (capital S) is a type alias for the impl target. `self` (lowercase) is the receiver.

`new` is a convention, not a keyword. Constructors are just associated functions.

---

## 2. Enums — A Value That Is One Of Several Things

In most languages, enums are integer constants. In Rust, an enum is a **tagged union**: each variant can carry its own payload.

```rust
enum IpAddr {
    V4(u8, u8, u8, u8),
    V6(String),
}

let home = IpAddr::V4(127, 0, 0, 1);
let loop6 = IpAddr::V6(String::from("::1"));
```

Variants can carry tuples, named fields, or nothing at all:

```rust
enum Message {
    Quit,                          // no data
    Move { x: i32, y: i32 },       // named fields
    Write(String),                  // tuple variant
    ChangeColor(i32, i32, i32),     // tuple variant
}
```

Methods on enums use the same `impl` syntax:

```rust
impl Message {
    fn is_quit(&self) -> bool {
        matches!(self, Message::Quit)
    }
}
```

This single feature — variants with payloads — unlocks the two most important enums in the standard library.

### `Option<T>` — replaces `null`

```rust
enum Option<T> {
    None,
    Some(T),
}
```

There is no `null` in Rust. If a value might be missing, its type *must* say so. The compiler then forces you to handle the missing case:

```rust
fn first_word(s: &str) -> Option<&str> {
    s.split_whitespace().next()
}

fn main() {
    match first_word("hello world") {
        Some(w) => println!("first: {w}"),
        None    => println!("empty"),
    }
}
```

You cannot accidentally dereference a `None`. The type system makes that impossible.

### `Result<T, E>` — replaces exceptions

```rust
enum Result<T, E> {
    Ok(T),
    Err(E),
}
```

Fallible operations return `Result`. The caller is forced to acknowledge that failure exists:

```rust
fn parse_age(s: &str) -> Result<u32, std::num::ParseIntError> {
    s.parse::<u32>()
}
```

We cover error handling in detail in module 08.

---

## 3. Pattern Matching — `match`, `if let`, `while let`

`match` is exhaustive — the compiler refuses to compile if you forget a variant. This is a feature, not a bug; it means adding a new enum variant is a search-able code change.

```rust
fn describe(msg: Message) -> String {
    match msg {
        Message::Quit => String::from("quit"),
        Message::Move { x, y } => format!("move to ({x}, {y})"),
        Message::Write(text) => format!("write: {text}"),
        Message::ChangeColor(r, g, b) => format!("rgb({r},{g},{b})"),
    }
}
```

Patterns are very expressive. You can match literals, ranges, structs, tuples, and bind names along the way:

```rust
let pair = (3, 7);
match pair {
    (0, 0) => println!("origin"),
    (x, 0) => println!("on x-axis at {x}"),
    (0, y) => println!("on y-axis at {y}"),
    (x, y) if x == y => println!("on diagonal at {x}"),    // match guard
    (x, y) => println!("at ({x}, {y})"),
}
```

Numeric ranges in match arms:

```rust
match age {
    0..=12   => "child",
    13..=19  => "teen",
    20..=64  => "adult",
    _        => "senior",
};
```

`_` is the wildcard catch-all. Use it last.

### `if let` — shorthand for one-arm match

When you only care about one variant, `if let` is concise:

```rust
let some_value: Option<i32> = Some(7);
if let Some(n) = some_value {
    println!("got {n}");
} else {
    println!("nothing");
}
```

### `while let` — loop while a pattern matches

```rust
let mut stack = vec![1, 2, 3];
while let Some(top) = stack.pop() {
    println!("{top}");
}
```

### `let else` — bail out if the pattern doesn't match

```rust
fn process(input: &str) -> Result<u32, String> {
    let Ok(n) = input.parse::<u32>() else {
        return Err(format!("not a number: {input}"));
    };
    Ok(n * 2)
}
```

This pattern keeps the happy path un-indented.

### Destructuring Structs and Tuples

```rust
struct Point { x: i32, y: i32 }
let p = Point { x: 3, y: 7 };
let Point { x, y } = p;
println!("{x} {y}");
```

You can destructure in function parameters too:

```rust
fn print_point(Point { x, y }: &Point) {
    println!("({x}, {y})");
}
```

---

## 4. Practical Example — A Tiny State Machine

A traffic light, modeled as an enum, with transitions enforced by pattern matching:

```rust
#[derive(Debug, Clone, Copy, PartialEq)]
enum Light {
    Red,
    Green,
    Yellow,
}

impl Light {
    fn next(self) -> Self {
        match self {
            Light::Red    => Light::Green,
            Light::Green  => Light::Yellow,
            Light::Yellow => Light::Red,
        }
    }
}

#[derive(Debug)]
enum Action {
    Stop,
    Go,
    Slow,
}

fn action_for(light: Light) -> Action {
    match light {
        Light::Red    => Action::Stop,
        Light::Green  => Action::Go,
        Light::Yellow => Action::Slow,
    }
}

fn main() {
    let mut light = Light::Red;
    for _ in 0..6 {
        println!("light={light:?} action={:?}", action_for(light));
        light = light.next();
    }
}
```

Add a fourth variant `Light::Off` and the compiler will instantly highlight every `match` you forgot to update. That's the safety net Rust gives you on a real codebase.

A more realistic example — a simple parser result:

```rust
#[derive(Debug)]
enum Token {
    Number(i64),
    Word(String),
    Punct(char),
}

fn tokenize(input: &str) -> Vec<Token> {
    let mut out = Vec::new();
    let mut chars = input.chars().peekable();
    while let Some(&c) = chars.peek() {
        if c.is_ascii_digit() {
            let mut n = 0_i64;
            while let Some(&d) = chars.peek() {
                if let Some(digit) = d.to_digit(10) {
                    n = n * 10 + digit as i64;
                    chars.next();
                } else { break; }
            }
            out.push(Token::Number(n));
        } else if c.is_alphabetic() {
            let mut w = String::new();
            while let Some(&ch) = chars.peek() {
                if ch.is_alphanumeric() { w.push(ch); chars.next(); }
                else { break; }
            }
            out.push(Token::Word(w));
        } else if c.is_whitespace() {
            chars.next();
        } else {
            out.push(Token::Punct(c));
            chars.next();
        }
    }
    out
}

fn main() {
    let tokens = tokenize("hello 42 world!");
    for t in tokens { println!("{t:?}"); }
}
```

Enums + `match` are the workhorse of every Rust parser, state machine, and protocol implementation you'll write.

---

## 5. Common Mistakes & Gotchas

- **Non-exhaustive match:** `match` requires every variant be handled. Add `_ => ...` for a catch-all, but think twice — exhaustiveness is what makes adding variants safe.
- **`unwrap()` on `Option`/`Result`** — works, but panics on `None`/`Err`. Fine in tests/prototypes; in production code, prefer `match`, `if let`, `?`, `unwrap_or`, `unwrap_or_else`, or `expect("a meaningful message")`.
- **Forgetting `derive`s.** A struct that doesn't `#[derive(Debug)]` can't be `{:?}` printed; one without `Clone` can't be `.clone()`d; one without `PartialEq` can't be `==`. Add the derives you need at the top: `#[derive(Debug, Clone, PartialEq)]`.
- **Moving out of a `match` on `&T`.** `match &my_string { s => ... }` binds `s: &String`. To move, match on the owned value.
- **Shadowing inside patterns:** `let x = 5; match Some(10) { Some(x) => println!("{x}"), None => () }` prints `10`, not `5`. The inner `x` shadows.
- **Forgetting `Self` vs `self`.** `Self` is the type; `self` is the value. `Self::new()` is a constructor call; `self.foo()` is a method call.
- **Using `if let` when `match` would catch a future variant.** `if let` silently ignores the other variants; `match` doesn't. For protocol-critical code, prefer `match`.
- **Constructing tuple-variant enums with `{}` syntax.** `Message::Write { 0: s }` works but is confusing; use `Message::Write(s)`.

---

## 🎯 Key Takeaways

- Structs bundle named data; enums let a value be one of several shapes, each with its own payload.
- `Option<T>` replaces `null`; `Result<T, E>` replaces exceptions. Both force the caller to acknowledge the missing/error case.
- `match` is exhaustive — adding an enum variant becomes a guided refactor, not a bug hunt.
- `if let`, `while let`, and `let else` are convenient single-pattern forms for the common cases.
- `#[derive(Debug, Clone, PartialEq, Eq, Hash)]` is the boilerplate you'll add to most data types — learn the common derives by heart.

*[← prev](./04_borrowing_and_references.md) | [next →](./06_control_flow_functions_expressions.md)*
