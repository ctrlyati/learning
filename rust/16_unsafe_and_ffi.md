# 16 — Unsafe Rust, FFI, and When to Reach for Them

> **Goal:** Understand what `unsafe` actually means, when it's necessary, and how to call C from Rust (and vice versa) safely.

## 1. What `unsafe` Actually Buys You

`unsafe` is *not* a license to break the rules. It's a license to perform five specific operations the borrow checker can't verify:

1. **Dereference a raw pointer** (`*const T`, `*mut T`).
2. **Call an `unsafe fn` or `unsafe` method.**
3. **Access or modify a mutable static variable.**
4. **Implement an `unsafe` trait** (e.g., `Send`, `Sync` manually).
5. **Access fields of a `union`.**

Everything else — ownership rules, borrow rules, lifetimes — *still applies* inside `unsafe`. You don't get to leak memory, alias `&mut`, or read uninitialized memory just because you opened an `unsafe` block. You're just promising the compiler you've checked invariants it can't.

```rust
fn main() {
    let mut x = 5;
    let r1 = &x as *const i32;
    let r2 = &mut x as *mut i32;

    unsafe {
        println!("r1 = {}", *r1);
        *r2 = 10;
        println!("r2 = {}", *r2);
    }
}
```

The analogy: safe Rust is the well-lit gym. `unsafe` is the heavy-machinery room — labeled, gated, requires training. You go in deliberately, do one specific thing, and get out.

### When You Actually Need It

In application code: almost never. In ten years of writing Rust apps, you might write `unsafe` zero times.

In low-level libraries:

- **FFI** — calling C, embedded peripherals.
- **Custom data structures** with internal aliasing the borrow checker can't model (intrusive linked lists, lock-free queues).
- **SIMD intrinsics** before stable wrappers exist.
- **Performance hotpaths** where bounds checks must be elided (`get_unchecked`).
- **Low-level allocator code.**

If you find yourself in `unsafe` to "make the borrow checker shut up," stop. There's a safer way; you just haven't found it yet. Ask on the forums or `#help` channel.

---

## 2. Raw Pointers and Their Discipline

`*const T` and `*mut T` look like C pointers, behave like C pointers, and have *none* of the guarantees of `&T`/`&mut T`:

- They can be null (no `Option` wrapper required).
- They can be unaligned.
- They can dangle (point to freed or wrong-typed memory).
- They can alias freely (`*mut` overlapping `*mut` is allowed).
- They have no lifetime.

You create them safely:

```rust
let x = 42;
let p1: *const i32 = &x;
let mut y = 99;
let p2: *mut i32 = &mut y;
let null: *const i32 = std::ptr::null();
```

You dereference them only inside `unsafe`:

```rust
unsafe {
    println!("{}", *p1);
    *p2 += 1;
}
```

The standard library exposes pointer methods that *require* invariants:

```rust
unsafe {
    let v = vec![1, 2, 3];
    let ptr = v.as_ptr();
    println!("{}", *ptr.add(1));   // safe iff index < v.len()
}
```

`ptr.add(n)` is `unsafe` because the caller must ensure the resulting pointer is in bounds *and* in the same allocation.

---

## 3. FFI — Calling C From Rust

Foreign Function Interface. Declare the C function in an `extern "C"` block, link against the library, call it from `unsafe`.

### Calling a C Standard Library Function

```rust
extern "C" {
    fn abs(input: i32) -> i32;
    fn strlen(s: *const std::os::raw::c_char) -> usize;
}

fn main() {
    unsafe {
        println!("{}", abs(-42));     // 42
    }
    let cstr = std::ffi::CString::new("hello").unwrap();
    unsafe {
        println!("{}", strlen(cstr.as_ptr()));   // 5
    }
}
```

Most C interop in production uses `bindgen` (auto-generates Rust declarations from a C header) and a wrapper crate. Don't hand-write hundreds of `extern` declarations.

### Strings Across the Boundary

Rust strings are UTF-8 with no terminator; C strings are null-terminated bytes. The bridge types live in `std::ffi`:

- **`CString`** — owned, null-terminated; for passing *to* C.
- **`CStr`** — borrowed view of a null-terminated C string; for receiving *from* C.

```rust
use std::ffi::{CStr, CString};

fn from_c(c: *const i8) -> String {
    unsafe { CStr::from_ptr(c).to_string_lossy().into_owned() }
}

fn to_c(s: &str) -> CString {
    CString::new(s).expect("nul byte in string")
}
```

### Exposing Rust to C

Mark `extern "C"` and `#[no_mangle]`:

```rust
#[no_mangle]
pub extern "C" fn rust_double(x: i32) -> i32 { x * 2 }
```

Build as a `cdylib` or `staticlib` in `Cargo.toml`:

```toml
[lib]
name = "myrust"
crate-type = ["cdylib"]
```

C side:

```c
#include <stdint.h>
extern int32_t rust_double(int32_t);
int main() { printf("%d\n", rust_double(21)); }
```

### Linking a C Library

Two routes:

**1. System library** (already on the linker path):

```rust
#[link(name = "z")]
extern "C" { fn compress(...); }
```

**2. Build script** (`build.rs`) that compiles C code as part of your crate:

```toml
[build-dependencies]
cc = "1"
```

```rust
// build.rs
fn main() {
    cc::Build::new().file("src/native.c").compile("native");
}
```

For complex bindings, use the `bindgen` + `cc` combination. For a high-level wrapper, study how crates like `openssl-sys` are organized.

---

## 4. Practical Example — Wrapping a C Function Safely

The typical pattern: `unsafe` C call wrapped in a *safe*, idiomatic Rust API. Callers shouldn't need to know C exists.

```rust
use std::ffi::{CStr, CString};
use std::os::raw::c_char;

extern "C" {
    fn getenv(name: *const c_char) -> *const c_char;
}

/// Safe wrapper around the libc `getenv`.
pub fn env(name: &str) -> Option<String> {
    let c_name = CString::new(name).ok()?;
    let ptr = unsafe { getenv(c_name.as_ptr()) };
    if ptr.is_null() {
        None
    } else {
        // SAFETY: getenv returns a pointer to a null-terminated string in the env block,
        // which lives as long as the process (modulo concurrent setenv calls — see notes).
        let s = unsafe { CStr::from_ptr(ptr) };
        Some(s.to_string_lossy().into_owned())
    }
}

fn main() {
    println!("PATH = {:?}", env("PATH"));
}
```

The critical pattern:

1. The `unsafe` is **localized** — one block, well-commented.
2. The wrapper is **safe** — callers can't misuse it. No raw pointers leak out.
3. A `// SAFETY:` comment explains what invariant is being upheld. **Always** write these. They're the documentation that future-you (or a reviewer) needs to trust the code.

This is the central pattern of safe FFI: *encapsulate `unsafe` behind a safe API, and document the invariants.*

### When to Skip All This

For well-known C libraries, a wrapper crate already exists. Search before you write:

- `libsqlite3-sys` + `rusqlite`
- `openssl-sys` + `openssl`
- `libgit2-sys` + `git2`

Use the wrapper. Only write your own bindings for genuinely unwrapped libraries.

---

## 5. Common Mistakes & Gotchas

- **`unsafe` doesn't disable the borrow checker.** Aliasing `&mut`, reading uninitialized memory, racing on `*mut` — still UB even inside `unsafe`. Soundness violations in `unsafe` blocks are *your* responsibility, not the compiler's.
- **Returning a raw pointer to a stack local from a function.** Caller dereferences it, reads garbage. C makes the same mistake easy; Rust's borrow checker normally prevents it — *until* you opt into raw pointers.
- **Forgetting `extern "C"` calling convention.** Rust's default ABI is unstable; calling Rust from C without `extern "C"` can crash on some platforms.
- **Forgetting `#[no_mangle]`.** Without it, your function's symbol is mangled and C can't find it.
- **Passing a Rust `&str` to a function expecting C string.** `&str` is *not* null-terminated. Convert via `CString::new`.
- **Holding a `*const c_char` from `getenv` across a call to `setenv`.** Libc may invalidate it. Read documentation of every C function you call about lifetimes.
- **Using `mem::transmute` for "type punning."** Almost always wrong. Look for `as`, `From`/`Into`, `bytemuck`, or `pointer::cast` first. `transmute` is for when literally nothing else works.
- **`unsafe impl Send for MyType {}` casually.** You're promising the compiler this type is safe to send between threads. If it isn't, you've created a data race that the next reviewer will be cleaning up.
- **No `// SAFETY:` comments.** In a serious codebase, every `unsafe` block should be accompanied by a comment stating which invariants make it safe. Reviewers depend on this.
- **Linking failures on the wrong platform.** Native libs must exist for whichever OS you're building for. Cross-compiling FFI is its own can of worms.

---

## 🎯 Key Takeaways

- `unsafe` enables five specific operations; it does **not** disable any other compiler check.
- In application code you almost never need `unsafe`; in low-level libraries (FFI, allocators, lock-free) it's a tool of last resort.
- The pattern is always: tiny, well-commented `unsafe` block wrapped in a safe public API.
- For C interop, use `CString`/`CStr`, `extern "C"`, `#[no_mangle]`, and `bindgen`/`cc` for non-trivial libraries.
- Write `// SAFETY:` comments justifying every `unsafe` block — they're the price of admission for serious Rust.

*[← prev](./15_async_rust.md) | [next →](./17_testing_and_docs.md)*
