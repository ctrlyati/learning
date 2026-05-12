# 18 — Production Rust: Workflows, Cross-Compilation, Performance, Ecosystem

> **Goal:** Operate Rust in production — release builds, cross-compilation, performance tuning, and the crates you'll reach for on every real project.

## 1. The Daily Cargo Workflow

The toolchain you'll actually run, in approximately the order you'll run it:

```bash
cargo check                      # fast type-check, your inner loop
cargo clippy --all-targets -- -D warnings    # treat all lints as errors
cargo fmt --all -- --check       # CI-style formatting check
cargo test --all-features
cargo doc --no-deps --open       # browse docs
cargo build --release            # ship build
cargo install --path .           # install binary into ~/.cargo/bin
```

A `Makefile.toml` (`cargo install cargo-make`) or a `justfile` (`cargo install just`) is how teams document and standardize these incantations.

### Key Files in a Production Project

```
.
├── Cargo.toml              # manifest
├── Cargo.lock              # lockfile (commit for binaries; usually for libs too now)
├── rust-toolchain.toml     # pin Rust version: channel = "1.79.0"
├── .cargo/config.toml      # local cargo configuration (build flags, mirrors)
├── deny.toml               # cargo-deny: license/security/dup checks
├── clippy.toml             # clippy customization
├── rustfmt.toml            # rustfmt customization
└── .github/workflows/ci.yml  # CI pipeline
```

`rust-toolchain.toml` pinning is non-negotiable for reproducible builds:

```toml
[toolchain]
channel = "1.79.0"
components = ["rustfmt", "clippy"]
```

CI just runs `rustup show` and gets the right version automatically.

### Release Profiles

Tune `Cargo.toml`:

```toml
[profile.release]
opt-level = 3
lto = "fat"            # link-time optimization; big perf gain, slow link
codegen-units = 1      # one codegen unit -> better optimization, slow build
strip = "symbols"       # strip debug symbols from binaries
panic = "abort"         # smaller, faster, no unwinding (be sure no one expects panic-recovery)

[profile.dev]
opt-level = 1           # debug builds with some optimization — much faster runtime
```

These choices trade build speed for runtime speed. For a binary, full LTO + 1 codegen unit + abort can shave 20-40% off, at the cost of much slower release builds.

---

## 2. Cross-Compilation

Rust cross-compiles well, but you need three things: a target installed, a linker for that target, and any sysroot the target needs.

### Add a Target

```bash
rustup target add x86_64-unknown-linux-musl
rustup target add aarch64-apple-darwin
rustup target add wasm32-unknown-unknown
rustup target list --installed
```

### Build for It

```bash
cargo build --release --target=x86_64-unknown-linux-musl
```

For musl on Linux (statically linked, no glibc dependency), you usually just need `musl-tools` installed. For Apple Silicon from Linux, or Windows from macOS, you'll need a cross linker — easiest path: **`cross`**, which uses Docker images:

```bash
cargo install cross
cross build --release --target=aarch64-unknown-linux-gnu
```

### WebAssembly

```bash
rustup target add wasm32-unknown-unknown
cargo build --release --target=wasm32-unknown-unknown
```

For browser-facing wasm with JS interop, use `wasm-bindgen` and `wasm-pack`. For server-side wasm (WASI), `wasm32-wasi`.

### Static Linking on Linux

Pair `musl` with this in `.cargo/config.toml`:

```toml
[target.x86_64-unknown-linux-musl]
linker = "x86_64-linux-musl-gcc"
rustflags = ["-C", "target-feature=+crt-static"]
```

The result: a single static binary you can `scp` anywhere with no library dependencies — the deployment story Go popularized, available in Rust too.

---

## 3. Performance Tuning

Rust gives you C-like performance by default, but you still need to measure.

### Profile Before Optimizing

- **`cargo flamegraph`** — sampling profiler, produces a flamegraph SVG. Best starting point.
- **`perf`** (Linux) — low-level CPU profiling.
- **`samply`** — cross-platform alternative to perf.
- **`criterion`** — micro-benchmarks (module 17).
- **`heaptrack`** / **`dhat`** — heap profiling.

### Common Wins

- **Switch to a faster hash:** `HashMap<_, _, ahash::RandomState>` or `rustc_hash::FxHashMap` are 1.5-3× faster for trusted workloads.
- **Use `&str` not `String`** in hot paths — avoid the allocation.
- **Pre-allocate:** `Vec::with_capacity(n)` if you know the size; same for `String`.
- **Use `iter()` chains** — they fuse and inline; `for` over indices doesn't always.
- **`SmallVec` / `arrayvec`** for collections that are usually small — keep them on the stack.
- **`Cow<'_, str>`** when you sometimes own and sometimes borrow.
- **Reduce cloning:** every `.clone()` on a `String` or `Vec` is a heap allocation. `Arc::clone` is just a refcount bump — much cheaper.
- **Parallelize with `rayon`:** `into_par_iter()` is often a 1-line change for embarrassingly parallel work.

### Compiler Tricks

- **`#[inline]`** — hint to inline; rarely needed, the compiler is smart, but useful at FFI boundaries.
- **`#[cold]`** — mark unlikely paths (error returns) for better branch prediction.
- **`unsafe { unreachable_unchecked() }`** in branches you've proven impossible — eliminates the panic path. Use only with measurement and `// SAFETY:` notes.

### Watch Out For

- **Debug builds are 10-100× slower than release.** Always benchmark with `cargo build --release`.
- **`println!` in a hot loop** locks stdout per call. Use `BufWriter` over a locked stdout.
- **Allocator choice matters.** On Linux with many allocations, switch to `jemallocator` or `mimalloc` for 10-30% gains:
  ```rust
  #[global_allocator]
  static ALLOC: jemallocator::Jemalloc = jemallocator::Jemalloc;
  ```

---

## 4. The Ecosystem — Crates You'll Use Almost Daily

A non-exhaustive but realistic kit, grouped by purpose.

### Foundational

- **`serde`** + **`serde_json`** / **`serde_yaml`** / **`toml`** — serialization. Universal.
- **`anyhow`** — application errors (module 8).
- **`thiserror`** — library errors (module 8).
- **`tracing`** + **`tracing-subscriber`** — structured, async-aware logging. Replaces `log` for new projects.
- **`clap`** — CLI parsing. The de facto standard.
- **`chrono`** or **`time`** — date/time.
- **`uuid`** — UUIDs.

### Async & Networking

- **`tokio`** — runtime; the default choice (module 15).
- **`reqwest`** — HTTP client.
- **`axum`** or **`actix-web`** — web frameworks.
- **`tonic`** — gRPC.
- **`hyper`** — low-level HTTP.

### Data & Persistence

- **`sqlx`** — async SQL with compile-time query checking.
- **`sea-orm`** / **`diesel`** — ORMs.
- **`redis`** — Redis client.
- **`rusqlite`** — SQLite.
- **`mongodb`** — MongoDB driver.

### Concurrency / Performance

- **`rayon`** — data parallelism (module 14).
- **`crossbeam`** — channels, atomics, lock-free queues.
- **`parking_lot`** — faster, non-poisoning `Mutex`/`RwLock`.
- **`dashmap`** — concurrent `HashMap`.
- **`ahash`** / **`rustc-hash`** — fast hashers.

### Testing & Tooling

- **`pretty_assertions`** — diff-style `assert_eq!` failures.
- **`insta`** — snapshot testing.
- **`proptest`** / **`quickcheck`** — property-based testing.
- **`mockall`** — mocking.
- **`criterion`** — benchmarks.
- **`cargo-watch`** — re-run on file change.
- **`cargo-edit`** — `cargo add`/`upgrade` (built into Cargo since 1.62).
- **`cargo-deny`** — license/security/dup audits in CI.
- **`cargo-audit`** — RUSTSEC vulnerability scan.
- **`cargo-machete`** — find unused dependencies.
- **`cargo-llvm-cov`** — coverage reports.

### Misc

- **`once_cell`** / **`std::sync::OnceLock`** — lazy statics.
- **`regex`** — regular expressions.
- **`itertools`** — extra iterator combinators.
- **`bytes`** — efficient byte-buffer handling.
- **`mimalloc`** / **`jemallocator`** — alternative allocators.

`cargo search` and crates.io's "most downloaded" listings are how to discover more. The Rust ecosystem moves fast — `lib.rs` is a curated alternative to crates.io for browsing.

---

## 5. Common Mistakes & Gotchas

- **Shipping debug builds.** Always `--release` for production. Easy to forget if you build with bare `cargo build`.
- **Not pinning the toolchain.** A `rust-toolchain.toml` saves a CI nightmare three months from now.
- **Forgetting `Cargo.lock` in version control for binaries.** Your dependency tree drifts otherwise. (For libraries, the conventional answer used to be "don't commit"; the modern guidance is to commit it for reproducible CI.)
- **`opt-level = 3 + lto = "fat"` everywhere.** Slows your build pipeline brutally for tiny benefit on cold paths. Profile before fully tuning.
- **Cross-compiling without Docker (`cross`).** It works once you wrestle linker setups, but `cross` saves you days.
- **Not running `cargo audit` in CI.** Supply-chain vulnerabilities slip in. Make it a required check.
- **Pulling in heavyweight dependencies for trivial functionality.** `chrono` for "format a date" is fine, but `reqwest` for a one-off HTTP call adds tokio + native-tls + ~50 transitive crates. Sometimes a tiny crate (`ureq`) is the right call.
- **Ignoring binary size.** Default Rust binaries are large (debug info, multiple codegen units). `strip = "symbols"`, `panic = "abort"`, and `cargo bloat` help.
- **Using `println!` for production logging.** Use `tracing`. Structured, leveled, async-aware, plays well with OpenTelemetry.
- **Single-threaded tokio runtime by accident.** `#[tokio::main(flavor = "current_thread")]` exists for tests; for prod, use the multi-thread default.
- **Reaching for `unsafe` for perf without measuring.** The borrow checker isn't slow. Measure first; almost always the win is algorithmic, not from removing safety.

---

## 🎯 Key Takeaways

- A production Rust project ships with `rust-toolchain.toml`, `Cargo.lock`, `clippy` clean, `fmt` clean, and a CI that runs `test`, `clippy`, `fmt --check`, `audit`, and `deny`.
- Tune release profiles (`lto`, `codegen-units`, `strip`, `panic = "abort"`) only after profiling — defaults are conservative for a reason.
- Cross-compilation works well; reach for `cross` to skip the linker dance, and prefer `musl` on Linux for static binaries.
- Performance gains usually come from algorithms, fewer allocations, faster hashers, and `rayon` — not from `unsafe`.
- Master a small ecosystem kit (`serde`, `tokio`, `tracing`, `anyhow`/`thiserror`, `clap`, `sqlx`/`reqwest`/`axum`) — these turn up on almost every real Rust codebase.

---

You've reached the end of the course. From here:

1. Build something real — a CLI, a web service, a parser, a small game. Pick something you'd otherwise do in a familiar language and force yourself to use Rust.
2. Read other people's Rust code. The standard library, `tokio`, `serde`, `axum` — all open source, all idiomatic, all teach more than any tutorial.
3. Subscribe to **This Week in Rust** and skim weekly. The ecosystem is your edge.
4. Contribute to a crate you depend on. The Rust community is unusually welcoming to first PRs, and a single merged contribution teaches more than a month of solo coding.

Welcome to the language. Now ship something.

*[← prev](./17_testing_and_docs.md) | [back to roadmap →](./00_roadmap.md)*
