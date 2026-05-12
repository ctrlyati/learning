# 01 — Setup & Cargo

> **Goal:** Install the Rust toolchain, understand what each tool does, and ship a working "hello world" via Cargo.

## 1. The Toolchain — your Swiss Army knife, installed in one shot

In most languages, the compiler, package manager, formatter, linter, doc generator, and test runner are separate things you wire together. In Rust, they ship as one coordinated toolchain installed via **rustup**.

Install once on macOS / Linux / WSL:

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

On Windows: download `rustup-init.exe` from https://rustup.rs.

After install, restart your shell and verify:

```bash
rustc --version    # the compiler
cargo --version    # the build tool / package manager
rustup --version   # the toolchain manager itself
```

Then immediately ship a hello world:

```bash
cargo new hello
cd hello
cargo run
```

You should see `Hello, world!`. That single command compiled, linked, and ran your binary. Welcome to Rust.

---

## 2. What Each Tool Actually Does

`rustup` manages **toolchains** (compiler versions). You can have stable, beta, and nightly all installed:

```bash
rustup toolchain install stable
rustup toolchain install nightly
rustup default stable          # set the default
rustup update                  # update all toolchains
rustup component add clippy rustfmt rust-src rust-analyzer
```

`rustc` is the compiler. You almost never call it directly — Cargo does it for you. But you can:

```bash
rustc main.rs    # produces ./main (or main.exe)
```

`cargo` is the workhorse. Memorize these:

| Command          | What it does                                                            |
| ---------------- | ----------------------------------------------------------------------- |
| `cargo new foo`  | Create a new binary project at `./foo`                                  |
| `cargo new --lib foo` | Create a new library project                                       |
| `cargo init`     | Initialize Cargo in the current directory                               |
| `cargo build`    | Compile (debug mode, in `./target/debug/`)                              |
| `cargo build --release` | Compile with optimizations (in `./target/release/`)              |
| `cargo run`      | Build + run                                                             |
| `cargo check`    | Type-check without producing a binary — **5–10× faster than build**     |
| `cargo test`     | Run all tests                                                           |
| `cargo fmt`      | Format code (uses `rustfmt`)                                            |
| `cargo clippy`   | Lint (catches anti-patterns; treat warnings as errors)                  |
| `cargo doc --open` | Build + open HTML docs for your crate and its dependencies            |
| `cargo add serde` | Add a dependency to `Cargo.toml`                                       |
| `cargo update`   | Update `Cargo.lock` to latest compatible versions                       |
| `cargo tree`     | Show the dependency graph                                               |

Under the hood, when you run `cargo build`, Cargo:
1. Reads `Cargo.toml` to find dependencies and metadata.
2. Resolves versions, downloads from crates.io into `~/.cargo/registry/`.
3. Compiles each dependency once (cached in `target/`).
4. Compiles your crate, links the binary.
5. Writes `Cargo.lock` to pin exact versions for reproducibility.

---

## 3. The Layout of a Cargo Project

`cargo new hello` creates this:

```
hello/
├── Cargo.toml      # manifest: name, version, dependencies
├── .gitignore      # ignores /target by default
└── src/
    └── main.rs     # entry point for binaries
```

A library project has `src/lib.rs` instead. A project can have both — `lib.rs` for the library, `main.rs` for a binary that uses it.

`Cargo.toml` looks like:

```toml
[package]
name = "hello"
version = "0.1.0"
edition = "2024"   # or 2021, 2018; pick the latest your toolchain supports

[dependencies]
# add things like: serde = { version = "1", features = ["derive"] }
```

Editions are Rust's way of making non-breaking syntactic changes. `edition = "2024"` opts into the latest idioms. They are **not** language versions — all editions can interoperate.

After `cargo build`, you also have:

```
target/
├── debug/         # unoptimized builds + incremental cache
└── release/       # optimized builds (when you use --release)
Cargo.lock         # exact version pins; commit for binaries, optional for libs
```

Add `target/` to `.gitignore` (Cargo does it for you on `cargo new`).

---

## 4. A Realistic First Project — a CLI that greets

Let's build slightly more than hello world. From scratch:

```bash
cargo new greeter
cd greeter
```

Edit `Cargo.toml` to add a dependency:

```toml
[package]
name = "greeter"
version = "0.1.0"
edition = "2024"

[dependencies]
clap = { version = "4", features = ["derive"] }
```

Or run `cargo add clap --features derive`.

Replace `src/main.rs`:

```rust
use clap::Parser;

/// A friendly greeter.
#[derive(Parser)]
#[command(version, about)]
struct Args {
    /// Name to greet
    #[arg(short, long, default_value = "world")]
    name: String,

    /// Number of times to greet
    #[arg(short, long, default_value_t = 1)]
    count: u8,
}

fn main() {
    let args = Args::parse();
    for _ in 0..args.count {
        println!("Hello, {}!", args.name);
    }
}
```

Run it:

```bash
cargo run -- --name Yati --count 3
# Hello, Yati!
# Hello, Yati!
# Hello, Yati!
```

The `--` separates Cargo's arguments from your program's. You just used a real crate, derived a CLI parser via macros, and shipped a binary — in 20 lines.

Format and lint before committing:

```bash
cargo fmt
cargo clippy -- -D warnings   # treat warnings as errors
```

---

## 5. Common Mistakes & Gotchas

- **Forgetting `cargo check`.** New Rustaceans run `cargo build` for every typo. `cargo check` skips codegen and runs in a fraction of the time. Use it constantly.
- **Editing files outside `src/`.** Cargo only compiles what's reachable from `main.rs`/`lib.rs`. A stray file in the project root won't be compiled.
- **`error: linker 'cc' not found`** on Linux/WSL: install build-essentials (`sudo apt install build-essential`). On macOS: `xcode-select --install`. On Windows: install the MSVC build tools (rustup-init prompts for this).
- **`error: failed to download` from crates.io** — almost always a network/proxy issue. Check `~/.cargo/config.toml` for stale proxy settings.
- **Committing `target/`.** It's huge (gigabytes after a while) and machine-specific. Make sure it's in `.gitignore`.
- **Using `rustc` directly.** Beyond a one-file demo, always use Cargo. Manual `rustc` invocations don't see your `Cargo.toml` and won't link your dependencies.
- **Mixing toolchains accidentally.** A `rust-toolchain.toml` file in a project root pins the toolchain for that project. Add one if you need a specific version (e.g. `1.78`) for reproducibility.

---

## 🎯 Key Takeaways

- `rustup` manages toolchains, `cargo` manages projects, `rustc` is the compiler — you'll mostly only type `cargo`.
- `cargo check` is your fastest feedback loop; use it instead of `cargo build` while iterating.
- Every project is a **crate**; binaries live in `src/main.rs`, libraries in `src/lib.rs`. Both can coexist.
- `Cargo.toml` is hand-written; `Cargo.lock` is generated. Commit the lock for binaries, generally not for libraries.
- `cargo fmt` + `cargo clippy` are non-negotiable in any professional Rust codebase. Run them in CI.

*[← prev](./00_roadmap.md) | [next →](./02_variables_and_primitives.md)*
