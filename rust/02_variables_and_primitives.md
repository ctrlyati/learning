# 02 — Variables, Mutability, Primitives, Shadowing

> **Goal:** Internalize Rust's immutable-by-default stance and the primitive type set you'll use every day.

## 1. `let`, `mut`, and Why Defaults Matter

In Rust, `let x = 5;` declares `x` as **immutable**. Try to reassign it and the compiler refuses:

```rust
fn main() {
    let x = 5;
    x = 6;  // error[E0384]: cannot assign twice to immutable variable `x`
}
```

To make it mutable, you opt in:

```rust
fn main() {
    let mut x = 5;
    x = 6;
    println!("{x}");
}
```

The analogy: in most languages, every variable is a sticky note you can scribble over. In Rust, every variable is engraved in stone unless you specifically use a whiteboard (`mut`). This sounds annoying for five minutes; then you realize half the bugs in your previous codebase came from values mutating when nobody expected them to.

Constants are stricter — typed, capitalized, evaluated at compile time:

```rust
const MAX_USERS: u32 = 10_000;
```

`static` declares a global with a fixed memory address (rare in app code, common in embedded):

```rust
static GREETING: &str = "Hello";
```

Underscore-prefixed names (`let _x = 5;`) silence the "unused variable" warning. A bare `_` discards the value entirely.

---

## 2. Scalar Primitives

Rust's primitives are deliberately precise. No "int" — you pick the width.

**Integers:**

| Signed   | Unsigned | Width                        |
| -------- | -------- | ---------------------------- |
| `i8`     | `u8`     | 8-bit                        |
| `i16`    | `u16`    | 16-bit                       |
| `i32`    | `u32`    | 32-bit (default for integers)|
| `i64`    | `u64`    | 64-bit                       |
| `i128`   | `u128`   | 128-bit                      |
| `isize`  | `usize`  | pointer-width (32 or 64)     |

`usize` is the type of array indices, lengths, and capacities. Use it when you mean "a count of bytes/elements."

```rust
let a: i32 = -42;
let b: u64 = 1_000_000;     // underscores are visual separators
let c = 0xff;               // hex
let d = 0b1010_0001;        // binary
let e = b'A';               // u8 byte literal
```

Integer overflow in debug mode **panics**; in release mode it **wraps**. To make intent explicit:

```rust
let x: u8 = 255;
let y = x.wrapping_add(1);     // 0
let z = x.checked_add(1);      // None (returns Option<u8>)
let s = x.saturating_add(1);   // 255
let (r, overflowed) = x.overflowing_add(1);  // (0, true)
```

**Floats:** `f32` and `f64`. Default is `f64`.

```rust
let pi: f64 = 3.14159;
let half = 0.5_f32;
```

**Bool:** `true` / `false`, one byte.

**Char:** four bytes, holds a single Unicode scalar (not a byte!):

```rust
let c: char = 'z';
let heart: char = '❤';
let emoji: char = '🦀';
```

**Tuple:** fixed-size heterogeneous group.

```rust
let t: (i32, f64, &str) = (1, 2.0, "three");
let (a, b, c) = t;          // destructuring
let first = t.0;            // index access
let unit: () = ();          // the "unit" type — empty tuple, returned by void-ish functions
```

**Array:** fixed-size, same-type, stack-allocated.

```rust
let arr: [i32; 5] = [1, 2, 3, 4, 5];
let zeros = [0u8; 1024];   // [0, 0, 0, ... 1024 times]
let first = arr[0];
let len = arr.len();       // 5
```

Out-of-bounds access panics at runtime (no buffer overruns into adjacent memory — that's the safety guarantee).

For dynamic-size lists, use `Vec<T>` (chapter 07).

---

## 3. Shadowing — Reassignment's Better Cousin

Shadowing is **redeclaring** a name with `let`. It's not mutation; it creates a new binding that hides the old one:

```rust
fn main() {
    let x = 5;
    let x = x + 1;       // new x, value 6
    let x = x * 2;       // new x, value 12
    println!("{x}");     // 12
}
```

Why it matters: shadowing lets you change the **type**, which `mut` cannot:

```rust
let spaces = "   ";          // &str
let spaces = spaces.len();   // usize — different type, same name
```

Compare to `mut`:

```rust
let mut spaces = "   ";
spaces = spaces.len();  // error: expected `&str`, found `usize`
```

Use `mut` when a value evolves but stays the same type (a counter, an accumulator). Use shadowing for transformation pipelines and conversions.

Shadowing also resets immutability:

```rust
let mut x = 5;
x += 1;
let x = x;   // re-shadow as immutable; x is now frozen at 6
```

This is a common pattern: build something with `mut`, then freeze it before passing it on.

---

## 4. Practical Mini-Example — Parsing a Config

Tying it together: read a number from a string, validate it, and use it.

```rust
use std::io::{self, Write};

fn main() {
    print!("Enter a port number: ");
    io::stdout().flush().unwrap();

    let mut input = String::new();
    io::stdin().read_line(&mut input).expect("failed to read");

    // shadow `input` from String -> &str -> u16
    let input = input.trim();
    let port: u16 = match input.parse() {
        Ok(n) if (1024..=65535).contains(&n) => n,
        Ok(n) => {
            eprintln!("port {n} is reserved or invalid");
            return;
        }
        Err(_) => {
            eprintln!("not a number: {input:?}");
            return;
        }
    };

    println!("Will bind to port {port}");
}
```

Notice:
- `input` is shadowed twice: `String` → `&str` (via `.trim()`) → `u16` (via `.parse()`).
- `port` is annotated `: u16`, which tells `.parse()` what to parse into.
- `match` handles the `Result` and validates the range — we'll formalize this in modules 05 and 08.
- `1024..=65535` is an inclusive range; `.contains(&n)` checks membership.

---

## 5. Common Mistakes & Gotchas

- **Forgetting `mut` and getting `cannot assign twice to immutable variable`.** Add `mut`. If you find yourself sprinkling `mut` everywhere, ask whether you should be returning new values instead.
- **Using `mut` when shadowing is clearer.** Type changes are a strong signal to shadow.
- **Integer type defaults bite you.** `let x = 5;` is `i32`. If you then pass it to a function expecting `u64`, you get a type error. Either annotate (`let x: u64 = 5;`) or cast (`x as u64`).
- **`as` casts truncate silently.** `let x: u8 = 300_i32 as u8;` gives you `44`, not an error. For checked conversion, use `TryFrom`/`try_into()`.
- **Comparing different numeric types.** `1u32 == 1i32` is a type error. Convert first.
- **Float equality.** `0.1 + 0.2 == 0.3` is `false` in Rust just like everywhere else. Use `(a - b).abs() < f64::EPSILON` or a tolerance.
- **`char` is 4 bytes, not 1.** Iterating `"é"` as bytes gives different results than as chars. We'll revisit in module 07 (strings).
- **Treating `()` as `null`.** It's the unit type, returned by functions that "don't return anything." It is *not* a missing value — for that, use `Option<T>` (module 05).

---

## 🎯 Key Takeaways

- Immutable-by-default eliminates a class of bugs; `mut` is an opt-in marker that this value will change.
- The integer type set is precise and explicit. `usize` for indexing and lengths; `i32`/`u32` are sensible defaults otherwise.
- Overflow panics in debug, wraps in release — use `checked_*` / `saturating_*` / `wrapping_*` to be unambiguous in production code.
- Shadowing redeclares; it's the right tool for transformation pipelines and type changes within a scope.
- Casting with `as` is allowed but lossy — prefer `try_into()` for any cast that could fail.

*[← prev](./01_setup_and_cargo.md) | [next →](./03_ownership.md)*
