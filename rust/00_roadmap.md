# 00 — Rust Roadmap (Beginner to Pro)

> **Goal:** Take a working developer from "I've never written Rust" to "I can ship and review production Rust" in 4–5 focused weeks.

Rust is dense. Unlike Python or Go, you cannot skim the early chapters and start being productive — the borrow checker will reject your programs until you internalize ownership. This course is built around that reality: the first half forges the mental models, the second half compounds on them.

---

## 1. Module Map

| #  | Module                                  | Why it matters                                                                |
| -- | --------------------------------------- | ----------------------------------------------------------------------------- |
| 01 | Setup & Cargo                           | The toolchain *is* the language ergonomics. Get this wrong and nothing works. |
| 02 | Variables, Mutability, Primitives       | Immutable-by-default + shadowing change how you write loops & state.          |
| 03 | Ownership                               | The single most important chapter. Everything later builds on this.           |
| 04 | Borrowing & References                  | How you actually pass data around without moving it.                          |
| 05 | Structs, Enums, Pattern Matching        | Rust's type system shines here — `Option`/`Result` replace `null` and `try`.  |
| 06 | Control Flow, Functions, Expressions    | Almost everything is an expression. Returns are just the last expression.     |
| 07 | Collections                             | `Vec`, `String`, `HashMap`, slices — and the ownership rules that govern them. |
| 08 | Error Handling                          | `?`, custom errors, `anyhow` for apps, `thiserror` for libs.                  |
| 09 | Traits & Generics                       | Polymorphism without inheritance.                                             |
| 10 | Lifetimes Deep Dive                     | The compiler's notation for "how long does this reference live?"              |
| 11 | Modules, Crates, Workspaces             | How real projects are organized.                                              |
| 12 | Closures & Iterators                    | Functional programming, zero-cost.                                            |
| 13 | Smart Pointers & Interior Mutability    | When the borrow checker isn't enough at compile time.                         |
| 14 | Concurrency (threads, channels, Mutex)  | "Fearless concurrency" — the type system catches data races.                  |
| 15 | Async Rust                              | `async/await`, futures, `tokio`. The hardest chapter for many.                |
| 16 | Unsafe Rust & FFI                       | When and *only* when to drop the guardrails.                                  |
| 17 | Testing, Benchmarking, Documentation    | Rust's batteries-included approach.                                           |
| 18 | Production Rust                         | Workflows, cross-compile, perf, the crates you'll actually use.               |

---

## 2. Timeline (4–5 Weeks)

Pace yourself: **1 module per 1–2 days**. Spend the long end on 03 (Ownership), 10 (Lifetimes), 14 (Concurrency), 15 (Async).

| Week  | Modules     | Focus                                          |
| ----- | ----------- | ---------------------------------------------- |
| 1     | 01–05       | Toolchain, syntax, ownership, types            |
| 2     | 06–10       | Collections, errors, traits, lifetimes         |
| 3     | 11–14       | Project organization, iterators, concurrency   |
| 4     | 15–18       | Async, unsafe, testing, production             |
| 5 (opt.) | review + a real project | Build something end-to-end (CLI tool, web service, parser) |

If you skip the project, you will forget half of this in a month. Do the project.

---

## 3. Prerequisites

You should already be comfortable with:

- Some systems-y or statically-typed language (C, C++, Go, Java, Kotlin, TypeScript with strict mode all count).
- The terminal: `cd`, env vars, `PATH`.
- Git basics.
- The idea of a stack vs. heap, even if fuzzy.

You do **not** need to know C++ templates, monads, or category theory. Rust borrows ideas from those worlds but explains them on its own terms.

---

## 4. Core Mental Models

Memorize these. Every module reinforces at least one.

1. **Ownership.** Every value has exactly one owner. When the owner goes out of scope, the value is dropped. Move, don't copy (unless `Copy`).
2. **Zero-cost abstractions.** Iterators, generics, async — they compile down to what you'd write by hand in C. You don't pay for what you don't use, and what you do use is as fast as hand-rolled.
3. **Fearless concurrency.** The same rules that prevent use-after-free at single-threaded compile time prevent data races at multi-threaded compile time. `Send` and `Sync` are the type-system encoding of this.
4. **If it compiles, it usually works.** This is folklore but largely true. The compiler is doing the work that test suites do in dynamic languages. Embrace the friction up front.
5. **Explicit > implicit.** Allocations, clones, conversions, error propagation — Rust makes them visible. `.clone()`, `.to_owned()`, `?`, `as`. No hidden costs.
6. **Composition via traits, not inheritance.** No class hierarchies. You add behavior to types by implementing traits. Polymorphism is generic + trait bounds, or `dyn Trait` for runtime dispatch.

---

## 5. External Resources (bookmark all of these)

- **The Rust Book** — https://doc.rust-lang.org/book/ — the canonical reference. Read in parallel.
- **Rustlings** — https://github.com/rust-lang/rustlings — small exercises that compile-check your understanding.
- **Rust by Example** — https://doc.rust-lang.org/rust-by-example/ — concept-by-concept code samples.
- **std docs** — https://doc.rust-lang.org/std/ — bookmark and use `rustup doc --std` offline.
- **This Week in Rust** — https://this-week-in-rust.org/ — weekly newsletter, the way to stay current.
- **The Rustonomicon** — https://doc.rust-lang.org/nomicon/ — for when you reach Module 16 (Unsafe).

---

## 6. How to Use This Course

- **Type the code.** Don't copy-paste. Muscle memory matters in Rust because the compiler errors are how you learn.
- **Read every error message.** Rust's compiler is the best teacher in the industry. If it tells you to add `&mut`, ask *why*.
- **Use `cargo check` constantly.** It's faster than `cargo build` and catches the same errors.
- **`cargo clippy` and `cargo fmt` are not optional.** Run them before you commit.
- **Don't fight the borrow checker.** When stuck, the answer is almost always: clone for now, refactor later, or change ownership direction (pass `&T`, return `T`).

---

## 7. Closing

You're learning Rust because the industry is moving toward it for systems work, infrastructure, embedded, WebAssembly, and increasingly backend services. Companies pay a premium for engineers who can navigate the borrow checker without flinching. The investment in the next 4–5 weeks compounds for the rest of your career — Rust changes how you think about ownership and concurrency in *every* language you touch afterward.

Start with module 01.

*[next →](./01_setup_and_cargo.md)*
