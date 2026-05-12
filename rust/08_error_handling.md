# 08 — Error Handling: `Result`, `?`, Custom Errors, `anyhow`/`thiserror`

> **Goal:** Handle fallible operations idiomatically, propagate errors with `?`, and use `anyhow`/`thiserror` like a professional.

## 1. The Two Failure Modes — `panic!` vs `Result`

Rust draws a hard line between two kinds of failure:

- **`panic!`** — unrecoverable. Bug, invariant violation, "this should never happen." Unwinds the stack and (by default) prints a backtrace. The program is in an undefined logical state.
- **`Result<T, E>`** — recoverable. Expected failure modes: file not found, parse error, network down. The caller decides what to do.

```rust
fn divide(a: f64, b: f64) -> f64 {
    if b == 0.0 { panic!("division by zero"); }    // wrong: caller can't recover
    a / b
}

fn divide_ok(a: f64, b: f64) -> Result<f64, &'static str> {
    if b == 0.0 { Err("division by zero") } else { Ok(a / b) }
}
```

Use `panic!` (or `unwrap`, `expect`, `assert!`, `unreachable!`) only for true bugs. For anything the caller might want to retry, log, or surface to a user, return `Result`.

---

## 2. `Result`, `?`, and Propagation

`Result<T, E>` is the enum from module 05. Idiomatic handling:

```rust
use std::fs::File;
use std::io::{self, Read};

fn read_username() -> Result<String, io::Error> {
    let mut f = File::open("user.txt")?;     // ? returns Err(e) if open failed
    let mut s = String::new();
    f.read_to_string(&mut s)?;                // ? again
    Ok(s)
}
```

The `?` operator does this:

```rust
let mut f = match File::open("user.txt") {
    Ok(file) => file,
    Err(e)   => return Err(e.into()),         // note the .into()
};
```

Two things to know:

1. `?` only works in functions returning `Result<_, _>` or `Option<_>` (or a few other types implementing `FromResidual`).
2. `?` calls `.into()` on the error, converting it to your function's error type via the `From` trait. This is what makes mixing error sources possible.

### Combinators You'll Use Daily

```rust
let n: i32 = "42".parse().unwrap();             // panic on failure
let n: i32 = "42".parse().expect("not a number"); // panic with msg
let n: i32 = "42".parse().unwrap_or(0);          // default on failure
let n: i32 = "42".parse().unwrap_or_else(|_| 0); // default from closure

let r: Result<i32, _> = "42".parse();
let doubled = r.map(|n| n * 2);                  // transform Ok side
let logged  = r.map_err(|e| format!("parse: {e}")); // transform Err side
let combined = r.and_then(|n| if n > 0 { Ok(n) } else { Err("nope") });
```

### `Option` Has the Same Toolkit

```rust
let v = vec![1, 2, 3];
let first = v.first().copied().unwrap_or(0);
let parsed = "42".parse::<i32>().ok();           // Result -> Option (drops error)
let result = some_option.ok_or("was None");       // Option -> Result
```

`?` works on `Option` too:

```rust
fn second_char(s: &str) -> Option<char> {
    s.chars().nth(1)?.to_lowercase().next()
}
```

---

## 3. Custom Error Types — Three Levels of Effort

### Level 1: A `String` (prototyping only)

```rust
fn load() -> Result<String, String> {
    std::fs::read_to_string("file").map_err(|e| e.to_string())
}
```

Easy, but loses type information. Fine for scripts; unacceptable for libraries.

### Level 2: A Hand-Rolled Enum

```rust
use std::fmt;

#[derive(Debug)]
enum LoadError {
    Io(std::io::Error),
    Parse(std::num::ParseIntError),
    Empty,
}

impl fmt::Display for LoadError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            Self::Io(e)    => write!(f, "io error: {e}"),
            Self::Parse(e) => write!(f, "parse error: {e}"),
            Self::Empty    => write!(f, "input was empty"),
        }
    }
}

impl std::error::Error for LoadError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Self::Io(e)    => Some(e),
            Self::Parse(e) => Some(e),
            Self::Empty    => None,
        }
    }
}

impl From<std::io::Error> for LoadError {
    fn from(e: std::io::Error) -> Self { Self::Io(e) }
}
impl From<std::num::ParseIntError> for LoadError {
    fn from(e: std::num::ParseIntError) -> Self { Self::Parse(e) }
}
```

A lot of boilerplate. With these `From` impls, `?` automatically converts:

```rust
fn load_number(path: &str) -> Result<i32, LoadError> {
    let text = std::fs::read_to_string(path)?;     // io::Error -> LoadError
    if text.trim().is_empty() { return Err(LoadError::Empty); }
    let n: i32 = text.trim().parse()?;             // ParseIntError -> LoadError
    Ok(n)
}
```

### Level 3: `thiserror` (libraries) and `anyhow` (applications)

**`thiserror`** generates everything in level 2 from `#[derive]`:

```toml
[dependencies]
thiserror = "1"
```

```rust
use thiserror::Error;

#[derive(Debug, Error)]
pub enum LoadError {
    #[error("io error: {0}")]
    Io(#[from] std::io::Error),

    #[error("parse error: {0}")]
    Parse(#[from] std::num::ParseIntError),

    #[error("input was empty")]
    Empty,
}
```

That's it. `Display`, `Error`, and `From` are all derived. Use `thiserror` whenever your code is a *library* — callers want typed errors they can match on.

**`anyhow`** is the opposite: a single boxed error type for applications where you don't care about the variant, you just want to bubble errors up with context.

```toml
[dependencies]
anyhow = "1"
```

```rust
use anyhow::{Context, Result};

fn load_config(path: &str) -> Result<String> {        // Result = Result<_, anyhow::Error>
    let text = std::fs::read_to_string(path)
        .with_context(|| format!("reading {path}"))?;
    Ok(text)
}

fn main() -> Result<()> {
    let c = load_config("config.toml")?;
    println!("{c}");
    Ok(())
}
```

`with_context` adds a layer of explanation; when the error prints, you see the full chain:

```
Error: reading config.toml

Caused by:
    No such file or directory (os error 2)
```

**Rule of thumb:** `thiserror` in libraries (so callers can match on variants), `anyhow` in binaries / app code (so you don't fight types).

---

## 4. Practical Example — A Robust File Processor

```rust
use anyhow::{Context, Result, bail};
use std::fs;
use std::path::Path;

fn process_file(path: &Path) -> Result<u64> {
    let text = fs::read_to_string(path)
        .with_context(|| format!("reading {}", path.display()))?;

    let lines: Vec<&str> = text.lines().collect();
    if lines.is_empty() {
        bail!("file {} is empty", path.display());
    }

    let mut total: u64 = 0;
    for (i, line) in lines.iter().enumerate() {
        let n: u64 = line.trim().parse()
            .with_context(|| format!("line {} ({line:?})", i + 1))?;
        total = total.checked_add(n)
            .with_context(|| format!("overflow at line {}", i + 1))?;
    }
    Ok(total)
}

fn main() -> Result<()> {
    let total = process_file(Path::new("numbers.txt"))?;
    println!("total = {total}");
    Ok(())
}
```

What this shows:
- `anyhow::Result` as the universal return type.
- `with_context` to add a frame each time we descend.
- `bail!` for early-exit with a formatted error.
- `checked_add` returning `Option`, converted via `with_context` (yes, it works on `Option` too).
- `main` returns `Result<()>` so `?` works at the top level — Rust prints `Error: ...` and exits non-zero.

---

## 5. Common Mistakes & Gotchas

- **`?` outside a `Result`-returning function.** Error: `the `?` operator can only be used in a function that returns Result`. Either change the return type or handle the error explicitly.
- **`.unwrap()` everywhere.** It panics on `Err`/`None`. Fine in `main` of a quick script, in tests, and in code that genuinely cannot fail (with a comment explaining why). Replace with `?`, `expect("…")`, `unwrap_or`, etc., otherwise.
- **`panic!` for expected failures.** A user typing bad input is not a bug — return `Err`, don't panic. Reserve `panic!` for "the laws of the universe were violated."
- **Stringly-typed errors in libraries.** Hard to match on, hard to filter, easy to break with formatting changes. Use `thiserror`.
- **`anyhow` in libraries.** Callers can't match variants — they get an opaque blob. Fine for binaries; bad for libs.
- **Forgetting `Ok(())` at the end of a `Result<()>` function.** Common, the compiler's error message is clear ("expected `Result`, found `()`").
- **`?` on an `Option` inside a `Result` function (or vice versa).** Convert with `.ok_or(err)` / `.ok()`.
- **Catching panics.** `std::panic::catch_unwind` exists but is for FFI boundaries — don't use it as exception handling. Panics indicate bugs; fix the bug.
- **Not using `#[from]` and writing `From` impls manually with `thiserror`.** Just add `#[from]` to the variant field and skip the impl.

---

## 🎯 Key Takeaways

- `Result<T, E>` is the canonical fallible type; `?` propagates errors and converts via `From`.
- `panic!` is for bugs only; expected failures must return `Result`.
- For libraries, derive errors with **`thiserror`** so callers can match on variants.
- For applications, use **`anyhow`** with `.context(...)` to build readable error chains; `main() -> Result<()>` enables top-level `?`.
- Combinator methods (`map`, `map_err`, `and_then`, `unwrap_or_else`, `ok_or`) let you handle errors fluently without exploding into nested `match` blocks.

*[← prev](./07_collections.md) | [next →](./09_traits_and_generics.md)*
