# 03 — Ownership

> **Goal:** Internalize Rust's ownership model — the single mental model that makes everything else click.

This is the chapter. If you take nothing else from this course, take this. Ownership is what makes Rust *Rust*. Spend two days here if you need to.

## 1. The Three Rules — and an Analogy

The entire ownership system reduces to three rules:

1. Every value in Rust has an **owner** (a variable).
2. There can only be **one owner at a time**.
3. When the owner goes out of scope, the value is **dropped** (its memory is freed).

Analogy: think of a value as a physical book. The owner is the person holding it. You can only have one person holding the book at a time. When that person leaves the room, the book gets recycled. If you want someone else to have it, you either **give** it (move) or let them **borrow** it (we'll cover borrowing in module 04).

The minimal example:

```rust
fn main() {
    let s = String::from("hello");   // s owns the heap allocation
    println!("{s}");
}                                     // s goes out of scope -> memory freed
```

No garbage collector ran. No `free()` was called by you. The compiler inserted the cleanup at the end of the scope. This is **deterministic destruction**, the same model as C++ RAII, but enforced by the type system.

---

## 2. Move Semantics — How Assignment Actually Works

Here's the moment most newcomers stub their toe:

```rust
fn main() {
    let s1 = String::from("hello");
    let s2 = s1;
    println!("{s1}");   // error[E0382]: borrow of moved value: `s1`
}
```

Read the error literally: `s1` was *moved* into `s2`. After the move, `s1` is no longer a valid binding. The book changed hands.

Why? `String` owns a heap allocation. If both `s1` and `s2` referred to the same allocation, when both went out of scope, the allocator would be told to free the same memory twice — a **double-free**, a classic C bug. Rust avoids this by invalidating `s1`.

To duplicate the data, **clone** explicitly:

```rust
let s1 = String::from("hello");
let s2 = s1.clone();           // deep copy; both own independent allocations
println!("{s1} {s2}");
```

Cloning is visible. You can see, in the source, where you pay for memory. This is the "explicit > implicit" mental model in action.

### Copy Types — the exception

Stack-only types like integers, floats, bools, chars, and tuples of those implement the `Copy` trait. They're cheap to duplicate and have no owned heap memory, so assignment **copies** instead of moves:

```rust
let x = 5;
let y = x;
println!("{x} {y}");   // both work — i32 is Copy
```

Rule of thumb: if a type owns a heap allocation (`String`, `Vec<T>`, `Box<T>`), it is **not** `Copy`. If it's a fixed-size stack value with no destructor, it usually is.

### Function Calls Move Too

Passing a value to a function moves it — same rules:

```rust
fn take_it(s: String) {
    println!("got {s}");
}

fn main() {
    let s = String::from("hi");
    take_it(s);
    println!("{s}");   // error: borrow of moved value `s`
}
```

To use it after, either return it back, clone, or — much more commonly — pass a reference (`&s`). That's module 04.

---

## 3. Drop, Scope, and Returning Ownership

The `Drop` trait runs custom code when a value is dropped. You'll rarely implement it, but seeing it makes the model concrete:

```rust
struct Loud(&'static str);

impl Drop for Loud {
    fn drop(&mut self) {
        println!("dropping {}", self.0);
    }
}

fn main() {
    let _a = Loud("first");
    {
        let _b = Loud("second");
    }   // prints "dropping second"
    println!("end of main");
}   // prints "dropping first"
```

Output:
```
dropping second
end of main
dropping first
```

Drops run in reverse order of declaration, at the end of the enclosing scope. Always.

### Returning to Move Ownership Out

Functions can return values to transfer ownership back to the caller:

```rust
fn make_greeting() -> String {
    let s = String::from("hello");
    s   // return moves ownership to the caller
}

fn main() {
    let g = make_greeting();
    println!("{g}");
}
```

This is how a constructor-like function works. The caller now owns the `String`.

### Partial Moves

You can move *out* of a struct field, leaving the rest intact — but then you can't use the whole struct anymore:

```rust
struct Person { name: String, age: u32 }

fn main() {
    let p = Person { name: String::from("Yati"), age: 30 };
    let n = p.name;        // moves p.name out
    println!("{}", p.age); // OK — age is Copy
    // println!("{}", p.name); // ERROR — moved
    // println!("{:?}", p);    // ERROR — partial move
}
```

---

## 4. A Practical Example — Building a String Pipeline

Let's process some text in a way that exercises ownership transfer between functions.

```rust
fn main() {
    let raw = String::from("  Hello, World!  ");
    let cleaned = clean(raw);          // move raw -> clean -> return -> cleaned
    let louder = shout(cleaned);       // move cleaned -> shout -> return -> louder
    println!("{louder}");              // "HELLO, WORLD!"
}

fn clean(mut s: String) -> String {
    s = s.trim().to_string();
    s
}

fn shout(s: String) -> String {
    s.to_uppercase()
}
```

Each function takes ownership, does its work, and hands ownership back. This works, but it's **not** how real Rust is written — passing ownership in/out of every function is verbose and prevents the caller from reusing the value.

The idiomatic version uses references (next module):

```rust
fn shout(s: &str) -> String { s.to_uppercase() }
```

But seeing the move-based version first makes clear what borrowing is *avoiding*.

### A Subtler Example — Vec and Iteration

```rust
fn main() {
    let v = vec![String::from("a"), String::from("b")];
    for s in v {            // moves v's elements into s, one at a time
        println!("{s}");
    }
    // println!("{:?}", v); // ERROR — v was moved by the for loop
}
```

To iterate without consuming, borrow:

```rust
for s in &v {
    println!("{s}");
}
println!("still have {} items", v.len());
```

This single distinction — `for x in v` (consume) vs `for x in &v` (borrow) — comes up daily.

---

## 5. Common Mistakes & Gotchas

- **`borrow of moved value`** — you used a value after handing it off. Fix: pass a reference (`&v`), clone (`v.clone()`), or restructure so the original owner doesn't need it back.
- **`use of moved value` inside a loop** — moving something into a function inside a loop's body, when the loop expected to use it again next iteration. Hoist the move out, or borrow.
- **Cloning to silence the compiler.** `.clone()` is fine while learning, but every clone is a heap allocation. Once comfortable, refactor to references. *Don't* delete clones at the cost of making the code unreadable — measure first.
- **Returning a reference to a local.** `fn f() -> &String { let s = String::from("x"); &s }` — error: `s` is dropped at end of `f`. Return the owned `String` instead.
- **Confusing `Copy` and `Clone`.** All `Copy` types are `Clone`, but not vice versa. `Copy` is implicit (assignment); `Clone` is explicit (`.clone()`). `String` is `Clone` but not `Copy`.
- **Thinking ownership is about performance.** Performance is a side effect. Ownership is about **correctness**: no double-frees, no use-after-free, no aliased mutation.
- **Worrying about "fighting the borrow checker" forever.** Everyone fights it for the first ~2 weeks. Then it becomes background noise, then it becomes a friend that catches your bugs before they ship.

---

## 🎯 Key Takeaways

- Every value has exactly one owner; values are dropped (memory freed) when the owner goes out of scope.
- Assignment and function calls **move** ownership for non-`Copy` types, invalidating the original binding.
- `Copy` types (integers, floats, bools, chars, fixed-size tuples of those) are duplicated cheaply on assignment.
- `.clone()` is your explicit "yes, deep-copy this" — visible, intentional, sometimes the right answer.
- This model is enforced at compile time with **zero runtime cost** — no GC, no reference counting (unless you opt in via `Rc`/`Arc`).

*[← prev](./02_variables_and_primitives.md) | [next →](./04_borrowing_and_references.md)*
