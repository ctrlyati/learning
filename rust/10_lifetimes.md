# 10 — Lifetimes Deep Dive

> **Goal:** Understand lifetime annotations as the compiler's notation for "how long must this reference be valid?" — and stop being scared of them.

## 1. The Mental Model — Lifetimes Are Just Labels

A lifetime is a **named scope** in which a reference is valid. They look like `'a`, `'static`, `'short`. They don't create or destroy anything — they're labels the borrow checker uses to verify that no reference outlives the data it points to.

You've already been writing lifetimes. The compiler infers them most of the time. Lifetimes appear in your source only when inference can't figure them out.

Here's the simplest case where you must annotate:

```rust
fn longest(x: &str, y: &str) -> &str {     // error[E0106]: missing lifetime specifier
    if x.len() > y.len() { x } else { y }
}
```

The compiler asks: the returned `&str` borrows from one of the inputs — but *which*? Both are possible at runtime. We need to tell the type system that the result lives at least as long as both inputs:

```rust
fn longest<'a>(x: &'a str, y: &'a str) -> &'a str {
    if x.len() > y.len() { x } else { y }
}
```

Read it: "for some lifetime `'a`, both `x` and `y` are borrowed for at least `'a`, and the returned `&str` is also valid for at least `'a`." The caller picks `'a` to be the **shorter** of the actual two input lifetimes.

```rust
fn main() {
    let s1 = String::from("long string");
    let result;
    {
        let s2 = String::from("short");
        result = longest(&s1, &s2);     // 'a = lifetime of s2 (shorter)
        println!("{result}");           // OK — used inside s2's scope
    }
    // println!("{result}");            // ERROR — s2 is gone
}
```

The annotation lets the compiler check this at compile time. No runtime check.

---

## 2. Lifetime Elision — Why You Rarely Have to Write Them

The compiler applies three rules to *elide* (omit) lifetimes in function signatures:

1. Each input reference parameter gets its own lifetime: `fn f(a: &T, b: &U)` is treated as `fn f<'a, 'b>(a: &'a T, b: &'b U)`.
2. If there's exactly one input lifetime, it's assigned to all output references.
3. If one of the inputs is `&self` or `&mut self`, its lifetime is assigned to all outputs.

Examples of elision in action:

```rust
fn first_word(s: &str) -> &str { /* ... */ }
// expands to: fn first_word<'a>(s: &'a str) -> &'a str

impl Foo {
    fn name(&self) -> &str { &self.name }
}
// expands to: fn name<'a>(&'a self) -> &'a str
```

That's why you almost never write lifetime annotations on methods. They only show up when:
- A function takes multiple references and returns one (rule fails) — must annotate.
- A struct holds a reference (always must annotate).

---

## 3. Lifetimes in Structs, Methods, and Bounds

### Structs Holding References

A struct with a reference field must declare the lifetime, because the struct's validity depends on that reference still being valid:

```rust
struct Excerpt<'a> {
    part: &'a str,
}

fn main() {
    let novel = String::from("Call me Ishmael. Some years ago...");
    let first_sentence = novel.split('.').next().unwrap();
    let e = Excerpt { part: first_sentence };
    println!("{}", e.part);
}
```

`Excerpt<'a>` says: "an `Excerpt` cannot outlive the data its `part` field references." If you tried to drop `novel` while `e` is alive, the compiler stops you.

### Methods on Lifetime-Generic Structs

```rust
impl<'a> Excerpt<'a> {
    fn level(&self) -> i32 { 3 }                     // no refs; trivial

    fn announce(&self, announcement: &str) -> &str { // elision: returns &self.part lifetime
        println!("Attention: {announcement}");
        self.part
    }
}
```

### `'static` — the Forever Lifetime

`'static` means "valid for the entire program." Three common occurrences:

- String literals: `"hello"` is `&'static str` because it lives in the binary.
- Constants: `const X: &str = "x";`
- A bound saying "the type contains no references that aren't `'static`":

```rust
fn spawn<F: FnOnce() + Send + 'static>(f: F) {
    /* tokio::spawn / std::thread::spawn want this */
}
```

Don't sprinkle `'static` to silence errors — it usually means "I want to leak this," which is rarely what you want.

### Lifetime Bounds on Generics

```rust
fn longest_with_announcement<'a, T>(
    x: &'a str,
    y: &'a str,
    ann: T,
) -> &'a str
where
    T: std::fmt::Display,
{
    println!("Announcement: {ann}");
    if x.len() > y.len() { x } else { y }
}
```

You can also bound a generic type by a lifetime: `T: 'a` means "the type `T` does not contain any references shorter than `'a`."

---

## 4. Practical Example — A Tokenizer With Borrowed Slices

Real-world: a parser whose tokens borrow into the original input. Cheaper than copying every substring out.

```rust
#[derive(Debug)]
enum Token<'a> {
    Word(&'a str),
    Number(&'a str),
    Punct(char),
}

struct Tokenizer<'a> {
    input: &'a str,
    pos: usize,
}

impl<'a> Tokenizer<'a> {
    fn new(input: &'a str) -> Self {
        Self { input, pos: 0 }
    }

    fn next_token(&mut self) -> Option<Token<'a>> {
        let bytes = self.input.as_bytes();
        // skip whitespace
        while self.pos < bytes.len() && bytes[self.pos].is_ascii_whitespace() {
            self.pos += 1;
        }
        if self.pos >= bytes.len() { return None; }

        let start = self.pos;
        let c = bytes[self.pos] as char;

        if c.is_ascii_alphabetic() {
            while self.pos < bytes.len() && (bytes[self.pos] as char).is_ascii_alphanumeric() {
                self.pos += 1;
            }
            Some(Token::Word(&self.input[start..self.pos]))
        } else if c.is_ascii_digit() {
            while self.pos < bytes.len() && (bytes[self.pos] as char).is_ascii_digit() {
                self.pos += 1;
            }
            Some(Token::Number(&self.input[start..self.pos]))
        } else {
            self.pos += 1;
            Some(Token::Punct(c))
        }
    }
}

fn main() {
    let src = String::from("hello 42 world!");
    let mut tk = Tokenizer::new(&src);
    while let Some(t) = tk.next_token() {
        println!("{t:?}");
    }
}
```

Lifetime story:
- `Tokenizer<'a>` borrows `input` for `'a`.
- Every `Token<'a>` it produces borrows into the same `'a`.
- The compiler ensures `src` outlives both `tk` and any `Token` we hold.

This is how `serde`, `nom`, `regex`, and most performance-critical parsers in the Rust ecosystem work — they avoid allocating per-token by borrowing into the input string.

---

## 5. Common Mistakes & Gotchas

- **`missing lifetime specifier`** — elision rules failed. The signature has multiple input refs and a returned ref. Either annotate (`<'a>`) or return owned data.
- **`borrowed value does not live long enough`** — the value goes out of scope before the reference does. Move the value's declaration up, or restructure so you don't hold the reference past its source's lifetime.
- **`cannot return reference to local variable`** — same problem in reverse. The local dies at function end. Return `String` not `&str`.
- **Storing a reference in a struct accidentally.** If you find yourself adding `<'a>` to "just make it compile," stop and ask whether the struct should *own* the data instead. Storing references is for hot loops, not normal app code.
- **`'static` everywhere.** People paste `'static` to silence errors. It works only when the data really does live forever. Often the right fix is structural: own the data, or pass it through.
- **Confusing trait-object lifetimes.** `Box<dyn Trait>` is sugar for `Box<dyn Trait + 'static>`. To allow non-`'static`, write `Box<dyn Trait + 'a>`.
- **Two function params, two refs, return one — trying to give them different lifetimes.** It's possible but usually wrong. The standard signature `fn f<'a>(x: &'a T, y: &'a T) -> &'a T` says "use the shorter of the two" — and that's almost always what you want.
- **Trying to mutate through a reference of the wrong lifetime.** Lifetimes are independent of mutability. `&'a mut T` says "exclusive access for `'a`." Both must check out.

---

## 🎯 Key Takeaways

- Lifetimes are compile-time labels for "how long is this reference valid?" — never runtime cost.
- The compiler elides lifetimes in most function signatures via three simple rules; you only annotate when those rules can't pick one.
- Structs holding references must be annotated, and the struct cannot outlive what it references.
- `'static` means "lives forever" (literals, constants, leaked allocations) — use it deliberately, never as a silencer.
- When lifetimes get knotted, the right fix is often structural: own the data, return owned types, or restructure so the reference doesn't need to escape.

*[← prev](./09_traits_and_generics.md) | [next →](./11_modules_crates_workspaces.md)*
