# 11 — Modules, Crates, Workspaces, Visibility

> **Goal:** Organize a real project: split code into modules, publish or consume crates, and structure a workspace of multiple crates.

## 1. The Hierarchy — Package > Crate > Module > Item

These four words are precise in Rust:

- **Package** — what a `Cargo.toml` describes. May contain at most one library crate and any number of binary crates.
- **Crate** — a tree of modules compiled into a single library (`.rlib`) or binary. The compilation unit.
- **Module** — a namespace within a crate. Modules form a tree rooted at `lib.rs` (for libs) or `main.rs` (for bins).
- **Item** — anything you can name: a function, struct, enum, trait, const, module, etc.

A bare `cargo new my_app` gives you:
- one package `my_app`
- one binary crate `my_app` (entry: `src/main.rs`)
- one module: the crate root

`cargo new --lib my_lib` gives:
- one package `my_lib`
- one library crate `my_lib` (entry: `src/lib.rs`)

You can have both `src/main.rs` and `src/lib.rs` in the same package — common when shipping a library plus a CLI that uses it.

---

## 2. Modules — Inline, File, and Directory

Three ways to declare a module.

### Inline (good for small modules)

```rust
// src/lib.rs
mod math {
    pub fn double(x: i32) -> i32 { x * 2 }
}

pub fn hello() {
    println!("{}", math::double(21));
}
```

### One File per Module

```
src/
├── lib.rs
└── math.rs
```

```rust
// src/lib.rs
mod math;
pub use math::double;
```

```rust
// src/math.rs
pub fn double(x: i32) -> i32 { x * 2 }
```

### Directory per Module (with submodules)

```
src/
├── lib.rs
└── math/
    ├── mod.rs        # OR math.rs at the same level (newer style)
    ├── algebra.rs
    └── geometry.rs
```

Modern Rust prefers `math.rs` *next to* a `math/` directory, instead of `math/mod.rs`:

```
src/
├── lib.rs
├── math.rs           # declares submodules
└── math/
    ├── algebra.rs
    └── geometry.rs
```

```rust
// src/math.rs
pub mod algebra;
pub mod geometry;
```

Both styles work; pick one and stay consistent within a project.

### Paths and `use`

Refer to items by their full path or `use` them in:

```rust
// crate-rooted (absolute):
use crate::math::algebra::add;
// relative:
use self::math::geometry::area;
use super::sibling_module::thing;

// rename to avoid clashes:
use std::io::Result as IoResult;

// glob import (use sparingly, mostly for preludes):
use crate::prelude::*;
```

`use` brings names into scope; it doesn't move them. Multiple `use`s of the same path are fine.

---

## 3. Visibility — `pub`, `pub(crate)`, `pub(super)`

By default, everything is **private to its module**. You opt into wider visibility:

| Modifier            | Visible to                                          |
| ------------------- | --------------------------------------------------- |
| (none)              | only the defining module                            |
| `pub`               | everyone (entire world if your crate is published)  |
| `pub(crate)`        | only within this crate                              |
| `pub(super)`        | only within the parent module                       |
| `pub(in path)`      | within the specified module path                    |

Field visibility is independent of struct visibility:

```rust
pub struct User {
    pub name: String,        // public field
    age: u32,                // private field — accessible only inside this module
}
```

This is how invariants are enforced — make fields private and expose constructors/methods that maintain them.

For library APIs, `pub(crate)` is the workhorse. It lets you split implementation across modules without exposing internals to your library's consumers.

### Re-exports

`pub use` re-exports a name through your module, flattening the public API:

```rust
// src/lib.rs
mod math;
pub use math::algebra::Matrix;   // consumers write `my_lib::Matrix`, not `my_lib::math::algebra::Matrix`
```

This is how you keep an internal directory structure clean while presenting a tidy facade to users.

---

## 4. Workspaces — Multi-Crate Projects

A **workspace** is a set of crates that share a single `Cargo.lock`, `target/` directory, and (optionally) dependency versions. The standard layout:

```
my_project/
├── Cargo.toml          # workspace root, no [package]
├── crates/
│   ├── core/
│   │   ├── Cargo.toml
│   │   └── src/lib.rs
│   ├── cli/
│   │   ├── Cargo.toml
│   │   └── src/main.rs
│   └── api/
│       ├── Cargo.toml
│       └── src/main.rs
```

Root `Cargo.toml`:

```toml
[workspace]
resolver = "2"
members = ["crates/*"]

[workspace.dependencies]
serde = { version = "1", features = ["derive"] }
tokio = { version = "1", features = ["full"] }
```

Member crates inherit:

```toml
# crates/core/Cargo.toml
[package]
name = "myproj-core"
version = "0.1.0"
edition = "2024"

[dependencies]
serde = { workspace = true }
```

Cross-crate references inside a workspace go by **path**:

```toml
# crates/cli/Cargo.toml
[dependencies]
myproj-core = { path = "../core" }
```

Build all crates with one `cargo build`. Test one with `cargo test -p myproj-core`. Workspaces are how every nontrivial Rust project organizes its code.

### A Realistic Example

Imagine a SaaS backend:
- `crates/core` — domain types, business logic (no I/O).
- `crates/db` — repository implementations against PostgreSQL.
- `crates/api` — Axum HTTP server.
- `crates/worker` — background job runner.
- `crates/cli` — admin CLI.

Each is its own crate, each tested independently, all sharing one `Cargo.lock`. Add a new feature: usually one or two crates change, the rest don't recompile.

### Crate Names vs Module Names

A crate name is what you put in `[dependencies]` and what `use` paths root at. Hyphens in crate names become underscores in code:

```toml
[dependencies]
my-thing = "1"
```

```rust
use my_thing::Foo;
```

---

## 5. Common Mistakes & Gotchas

- **`mod foo;` not finding `foo.rs`.** Either the file isn't where Cargo expects (`src/foo.rs` or `src/foo/mod.rs`), or you forgot to declare it in the parent (`mod foo;` in `lib.rs` or the parent's file).
- **Cyclic module dependencies.** Modules are a tree; you can't have `a` and `b` `use` each other in a way that creates a cycle. Refactor: extract shared parts to a third module both depend on.
- **Forgetting `pub` on a function and getting "private item" errors.** Make it `pub` (full public) or `pub(crate)` (visible inside this crate only).
- **`use` inside a function only brings the name into that function.** Most `use` statements live at module scope, near the top of the file.
- **`cargo build` building the whole workspace from a subcrate directory.** It's intentional. Use `cargo build -p crate_name` to scope.
- **Two crates in the workspace pulling different versions of the same dependency.** Cargo allows it (binaries get separate copies), but it bloats and can cause type mismatches if both expose the same type. Use `[workspace.dependencies]` to align.
- **Forgetting to `pub use` important items from submodules.** Users see your crate as "where do I find `Matrix`?" Re-export at the crate root.
- **`extern crate foo;`** — pre-2018 syntax. You don't need it anymore in Rust 2018+. Just add to `Cargo.toml` and `use foo::...`.
- **Declaring `mod` twice.** Each module is declared exactly once in its parent. Importing it elsewhere uses `use`, not `mod`.

---

## 🎯 Key Takeaways

- Crates are the compilation unit; modules are namespaces inside a crate; packages are what `Cargo.toml` describes.
- Modern layout: `foo.rs` next to a `foo/` directory for submodules — avoid `mod.rs` in new projects.
- Default visibility is **private to the module**; opt into `pub`, `pub(crate)`, or `pub(super)` deliberately.
- `pub use` is the primary tool for crafting a clean public API on top of a sensibly structured internal tree.
- Workspaces are how serious Rust projects scale — split by domain, share `Cargo.lock`, build/test independently.

*[← prev](./10_lifetimes.md) | [next →](./12_closures_iterators.md)*
