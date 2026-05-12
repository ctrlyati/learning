# 17 — Testing, Benchmarking, Documentation

> **Goal:** Write the unit tests, integration tests, benchmarks, and docs that ship with every professional Rust crate.

## 1. Unit Tests — Inline With the Code

Rust's testing story is batteries-included. The convention: tests live in the same file as the code they test, in a `#[cfg(test)] mod tests` block at the bottom.

```rust
pub fn add(a: i32, b: i32) -> i32 { a + b }

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn adds_positive() {
        assert_eq!(add(2, 3), 5);
    }

    #[test]
    fn adds_negative() {
        assert_eq!(add(-2, -3), -5);
    }

    #[test]
    #[should_panic(expected = "overflow")]
    fn overflow_panics() {
        add(i32::MAX, 1);    // panics in debug
    }

    #[test]
    fn ignored_test() {
        // skipped unless `cargo test -- --ignored`
    }
}
```

Run with `cargo test`. Useful flags:

```bash
cargo test                       # all tests
cargo test adds                  # name filter (substring match)
cargo test -- --nocapture        # print stdout from passing tests
cargo test -- --test-threads=1   # serialize (for tests touching shared resources)
cargo test --release             # optimized — useful for slow tests
cargo test -p mycrate            # one workspace member
```

`#[cfg(test)]` ensures the test module only compiles for `cargo test`, keeping it out of release builds.

### Assertions

```rust
assert!(condition);
assert_eq!(a, b);
assert_ne!(a, b);
assert!(a > 0, "expected positive, got {a}");      // custom message
```

`assert_eq!` requires both operands to implement `PartialEq + Debug`.

### Returning `Result` From Tests

You can use `?` in tests by returning `Result<(), E>`:

```rust
#[test]
fn parses() -> Result<(), Box<dyn std::error::Error>> {
    let n: i32 = "42".parse()?;
    assert_eq!(n, 42);
    Ok(())
}
```

---

## 2. Integration Tests — `tests/` Directory

Integration tests live in `tests/` at the crate root, each file is a separate compiled binary that uses the crate from the *outside* — exactly as a library consumer would.

```
my_crate/
├── Cargo.toml
├── src/
│   └── lib.rs
└── tests/
    ├── api.rs
    └── common/
        └── mod.rs       # shared test helpers (subdirs aren't auto-compiled)
```

```rust
// tests/api.rs
use my_crate::add;

#[test]
fn integration_add_works() {
    assert_eq!(add(2, 2), 4);
}
```

Integration tests:
- Can only access the **public** API — the `pub` interface of your crate.
- Each file is a separate binary, so each gets its own `target/debug/deps/...` artifact.
- Don't share state with each other (or with unit tests).

For binary crates (`src/main.rs`), integration tests are awkward — you usually factor logic into a `lib.rs` and have `main.rs` be a thin wrapper.

---

## 3. Documentation Tests — Examples That Run

Doc comments use `///` (item docs) or `//!` (module/crate docs). Code blocks inside are *compiled and run* by `cargo test`. This means every example in your docs is guaranteed to be correct.

```rust
/// Adds two numbers.
///
/// # Examples
///
/// ```
/// use my_crate::add;
///
/// assert_eq!(add(2, 3), 5);
/// assert_eq!(add(-1, 1), 0);
/// ```
///
/// # Panics
///
/// Panics on overflow in debug builds.
pub fn add(a: i32, b: i32) -> i32 { a + b }
```

Common section headers Rustaceans expect:

- `# Examples` — runnable code samples.
- `# Panics` — when this function may panic.
- `# Errors` — for `Result`-returning functions, what errors mean.
- `# Safety` — for `unsafe fn`, what the caller must guarantee.

Generate the HTML site:

```bash
cargo doc --open                  # builds and opens in browser
cargo doc --no-deps               # skip dependencies
```

Hide setup code from the rendered doc but include it in the test by prefixing lines with `#`:

```rust
/// ```
/// # use my_crate::Config;
/// let cfg = Config::new();
/// assert_eq!(cfg.port(), 8080);
/// ```
```

Mark a code block as `ignore`, `no_run`, or `compile_fail` if appropriate:

````
/// ```ignore
/// // not even compiled
/// ```
///
/// ```no_run
/// // compiled but not executed (network calls, infinite loops)
/// ```
///
/// ```compile_fail
/// let x: u32 = -1;   // demonstrates an error; test passes if this fails to compile
/// ```
````

`cargo test` runs doc tests by default. They show up in the output under "Doc-tests crate_name."

---

## 4. Practical Example — A Tested, Documented Library

A small `parser` crate showing the full picture.

```rust
// src/lib.rs

//! A tiny CSV-line parser.
//!
//! # Example
//! ```
//! use parser::parse_line;
//! let row = parse_line("a, b ,c").unwrap();
//! assert_eq!(row, vec!["a", "b", "c"]);
//! ```

use thiserror::Error;

#[derive(Debug, Error, PartialEq)]
pub enum ParseError {
    #[error("empty input")]
    Empty,
    #[error("unbalanced quote at column {0}")]
    UnbalancedQuote(usize),
}

/// Parses a single CSV line into trimmed fields.
///
/// Whitespace around commas is stripped. Quotes are not yet supported (returns Err).
///
/// # Examples
/// ```
/// use parser::parse_line;
/// assert_eq!(parse_line("hello, world").unwrap(), vec!["hello", "world"]);
/// ```
///
/// # Errors
/// Returns [`ParseError::Empty`] for an empty input.
pub fn parse_line(input: &str) -> Result<Vec<String>, ParseError> {
    if input.trim().is_empty() {
        return Err(ParseError::Empty);
    }
    if input.contains('"') {
        let col = input.find('"').unwrap();
        return Err(ParseError::UnbalancedQuote(col));
    }
    Ok(input.split(',').map(|f| f.trim().to_string()).collect())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_simple_row() {
        assert_eq!(parse_line("a,b,c").unwrap(), vec!["a", "b", "c"]);
    }

    #[test]
    fn trims_whitespace() {
        assert_eq!(parse_line(" a , b ,c ").unwrap(), vec!["a", "b", "c"]);
    }

    #[test]
    fn empty_is_error() {
        assert_eq!(parse_line("   "), Err(ParseError::Empty));
    }

    #[test]
    fn quotes_not_supported_yet() {
        let r = parse_line(r#"a,"b",c"#);
        assert!(matches!(r, Err(ParseError::UnbalancedQuote(_))));
    }
}
```

Plus an integration test:

```rust
// tests/api.rs
use parser::parse_line;

#[test]
fn round_trips_a_realistic_line() {
    let line = "id, name, email";
    let parsed = parse_line(line).unwrap();
    assert_eq!(parsed.len(), 3);
}
```

Run everything:

```bash
cargo test       # unit + integration + doc-tests
cargo doc --open
```

Add CI: a typical GitHub Actions workflow runs `cargo fmt -- --check`, `cargo clippy -- -D warnings`, and `cargo test --all-features` on push.

### Benchmarking

Stable Rust's built-in `#[bench]` is nightly-only. The community standard is **`criterion`**:

```toml
[dev-dependencies]
criterion = { version = "0.5", features = ["html_reports"] }

[[bench]]
name = "parse_bench"
harness = false
```

```rust
// benches/parse_bench.rs
use criterion::{black_box, criterion_group, criterion_main, Criterion};
use parser::parse_line;

fn bench_parse(c: &mut Criterion) {
    let input = "alpha, beta, gamma, delta, epsilon";
    c.bench_function("parse_5_fields", |b| {
        b.iter(|| parse_line(black_box(input)).unwrap())
    });
}

criterion_group!(benches, bench_parse);
criterion_main!(benches);
```

```bash
cargo bench
```

`criterion` runs many iterations, reports mean/std/outliers, and writes HTML reports under `target/criterion/`. For perf comparisons across commits, it remembers prior baselines.

For lower-level profiling: `cargo flamegraph`, `perf` (Linux), Instruments (macOS), and `cargo-pgo` for profile-guided optimization.

---

## 5. Common Mistakes & Gotchas

- **Tests sharing state across the file system or env vars.** Cargo runs tests in parallel by default — two tests writing the same temp path race. Use `tempfile::TempDir` per test, or `--test-threads=1` for tests that genuinely can't parallelize.
- **`cargo test` "passes" but you didn't write any tests.** Look for "0 passed" in the output. CI green doesn't mean tests exist.
- **Doc tests failing because of missing `use`.** Doc tests run as if they're independent programs. You need every `use` they need (or hide setup with `# use ...;`).
- **Hidden import lines (`# ...`) shown in error messages but invisible in the rendered docs.** Surprising the first time.
- **`assert_eq!` printing huge structs unreadably.** Use `pretty_assertions` for diff-style output: replace `use std::assert_eq;` with `use pretty_assertions::assert_eq;` in tests.
- **`#[bench]` syntax errors on stable.** Use `criterion` or `divan`.
- **Forgetting `harness = false` for criterion benches.** Without it, Cargo tries to use the default test harness and `criterion_main!` can't take over.
- **Not running `cargo test` in CI.** This includes doc tests — they're free to run and catch bit-rot in examples.
- **Treating `#[cfg(test)]` modules as documentation.** They aren't — they don't render in `cargo doc`. Put real examples in `///` doc comments.
- **Letting unit tests reach into private internals when an integration test would be cleaner.** White-box tests are fine for invariants; for behavior, prefer the public API.

---

## 🎯 Key Takeaways

- Unit tests live in `#[cfg(test)] mod tests` blocks beside the code; integration tests live in `tests/` and use the public API.
- Doc tests in `///` comments are compiled and run — your examples can never silently rot.
- `cargo test` runs unit, integration, and doc tests in one command; combine with `clippy` and `fmt --check` in CI.
- For benchmarks, use `criterion` on stable; it gives statistical rigor and HTML reports out of the box.
- Treat tests, docs, and `clippy` as part of the deliverable — a Rust crate without them looks unprofessional.

*[← prev](./16_unsafe_and_ffi.md) | [next →](./18_production_rust.md)*
