# 09 — Traits & Generics

> **Goal:** Build polymorphic, reusable code without inheritance — using traits, generic types, and trait bounds.

## 1. Traits — Behavior, Defined as a Contract

A **trait** is Rust's interface. It declares a set of methods a type must provide. Other languages might call this an interface, protocol, or type class.

```rust
trait Greet {
    fn hello(&self) -> String;
}

struct English;
struct Hindi;

impl Greet for English {
    fn hello(&self) -> String { String::from("Hello!") }
}

impl Greet for Hindi {
    fn hello(&self) -> String { String::from("Namaste!") }
}

fn main() {
    let langs: Vec<Box<dyn Greet>> = vec![Box::new(English), Box::new(Hindi)];
    for l in &langs { println!("{}", l.hello()); }
}
```

The analogy: a trait is a job description. A type "applies" by implementing the trait, and the rest of the codebase can hire any type that meets the description.

### Default Methods

A trait method can have a default body, which implementors override only if they need to:

```rust
trait Greet {
    fn name(&self) -> &str;
    fn hello(&self) -> String { format!("Hello, {}!", self.name()) }
}
```

This is how `Iterator` provides 70+ methods (`map`, `filter`, `sum`, `collect`, ...) on top of the single required `next` method.

### Common Standard Traits (memorize these)

| Trait                 | What it enables                                   |
| --------------------- | ------------------------------------------------- |
| `Debug`               | `{:?}` formatting; usually derived                |
| `Display`             | `{}` formatting; manually implemented             |
| `Clone`               | `.clone()` deep copy                              |
| `Copy`                | implicit copy on assignment (must also be `Clone`)|
| `PartialEq`, `Eq`     | `==`, `!=`                                        |
| `PartialOrd`, `Ord`   | `<`, `<=`, `.cmp()`                               |
| `Hash`                | usable as a `HashMap` / `HashSet` key             |
| `Default`             | `T::default()`                                    |
| `From<T>`, `Into<T>`  | infallible conversions                            |
| `TryFrom<T>`, `TryInto<T>` | fallible conversions (return `Result`)       |
| `Iterator`            | iteration                                         |
| `Drop`                | custom destructor                                 |

Most of these are derivable:

```rust
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct UserId(u64);
```

---

## 2. Generics — One Implementation, Many Types

Generics let one function or struct work over many types. Type parameters go in `<T>`:

```rust
fn largest<T: PartialOrd>(xs: &[T]) -> &T {
    let mut largest = &xs[0];
    for x in xs {
        if x > largest { largest = x; }
    }
    largest
}

fn main() {
    println!("{}", largest(&[3, 1, 4, 1, 5, 9, 2, 6]));
    println!("{}", largest(&['a', 'z', 'm']));
}
```

The `T: PartialOrd` is a **trait bound**: `T` can be any type that implements `PartialOrd`. Without the bound, the compiler refuses `x > largest` because not all types are comparable.

### Generic Structs and Methods

```rust
struct Pair<A, B> { first: A, second: B }

impl<A, B> Pair<A, B> {
    fn new(a: A, b: B) -> Self { Self { first: a, second: b } }
    fn swap(self) -> Pair<B, A> { Pair { first: self.second, second: self.first } }
}

// methods only when bounds are met
impl<A: std::fmt::Display, B: std::fmt::Display> Pair<A, B> {
    fn print(&self) { println!("({}, {})", self.first, self.second); }
}
```

### Multiple Bounds and `where` Clauses

For readability, complex bounds go in a `where` clause:

```rust
fn process<T, U>(t: T, u: U) -> String
where
    T: std::fmt::Debug + Clone,
    U: std::fmt::Display,
{
    format!("{:?} / {}", t.clone(), u)
}
```

### Monomorphization — Zero Cost

Rust generics are compiled by **monomorphization**: at compile time, each unique type combination produces its own specialized version of the function. The runtime cost is zero — calls dispatch directly, just like hand-written code. The trade-off is binary size (more code generated for more type combinations).

This is fundamentally different from Java's type erasure.

---

## 3. Trait Objects vs Generics — Static vs Dynamic Dispatch

Two ways to write polymorphic code:

### Static Dispatch (`impl Trait` / generics)

```rust
fn greet<T: Greet>(g: &T) {
    println!("{}", g.hello());
}
// or equivalently:
fn greet2(g: &impl Greet) {
    println!("{}", g.hello());
}
```

The compiler generates one `greet` per concrete type used. Each call is a direct (inlinable) function call. Zero runtime overhead.

### Dynamic Dispatch (`dyn Trait`)

```rust
fn greet(g: &dyn Greet) {
    println!("{}", g.hello());
}

let langs: Vec<Box<dyn Greet>> = vec![Box::new(English), Box::new(Hindi)];
```

`&dyn Greet` is a **trait object**: two pointers, one to the data, one to a vtable. Method calls go through the vtable (one indirection), enabling heterogeneous collections.

When to use which:

- **Generics / `impl Trait`** — one type per call site, max performance, larger binary. The default.
- **`dyn Trait`** — heterogeneous collections (`Vec<Box<dyn Trait>>`), plugin-style extensibility, smaller binary, slight perf cost.

Not all traits can be made into objects ("object-safe"): no generic methods, no `Self` in return position (with caveats), no associated constants.

### `impl Trait` in Return Position

Returning a complex type from a function — say, a closure or an iterator chain — without naming it:

```rust
fn counter() -> impl Iterator<Item = u32> {
    (0..).step_by(2).take(5)
}
```

The caller knows it's *some* `Iterator<Item = u32>`, but not which one. Useful for hiding implementation details and combinator-heavy code.

For multiple possible types, use `Box<dyn Trait>`:

```rust
fn make_iter(reverse: bool) -> Box<dyn Iterator<Item = i32>> {
    if reverse { Box::new((0..10).rev()) } else { Box::new(0..10) }
}
```

---

## 4. Practical Example — A Generic `Repository`

A common shape in real apps: storage that works for any type with an `id`.

```rust
use std::collections::HashMap;
use std::hash::Hash;

trait HasId {
    type Id: Eq + Hash + Clone;
    fn id(&self) -> Self::Id;
}

struct Repo<T: HasId> {
    items: HashMap<T::Id, T>,
}

impl<T: HasId> Repo<T> {
    fn new() -> Self { Self { items: HashMap::new() } }

    fn insert(&mut self, item: T) {
        self.items.insert(item.id(), item);
    }

    fn get(&self, id: &T::Id) -> Option<&T> {
        self.items.get(id)
    }

    fn len(&self) -> usize { self.items.len() }
}

#[derive(Debug, Clone)]
struct User { id: u64, name: String }

impl HasId for User {
    type Id = u64;
    fn id(&self) -> u64 { self.id }
}

fn main() {
    let mut repo = Repo::<User>::new();
    repo.insert(User { id: 1, name: "Alice".into() });
    repo.insert(User { id: 2, name: "Bob".into() });
    println!("{:?}", repo.get(&1));   // Some(User { id: 1, name: "Alice" })
    println!("{}", repo.len());        // 2
}
```

Notice:
- **Associated type** `type Id` — each implementor picks its key type.
- The `Repo` is generic in `T`, but its key type is *derived from* `T` via `T::Id`.
- One implementation, infinite type combinations, zero runtime cost.

This is the generic-programming superpower. Compare to Java generics (which would erase `Id`) or Go (which until recently didn't have it at all).

---

## 5. Common Mistakes & Gotchas

- **"the trait `Foo` cannot be made into an object"** — your trait isn't object-safe. Either remove `Self` from return positions, remove generic methods, or use generics (`impl Trait`) instead of `dyn Trait`.
- **Forgetting bounds.** `fn f<T>(x: T) { x.clone(); }` — error: `T` doesn't implement `Clone`. Add `T: Clone`.
- **Trying to write `impl<T> Trait for Vec<T>` in another crate.** The **orphan rule**: you may implement `Trait for Type` only if either `Trait` or `Type` is local to your crate. Workaround: a newtype wrapper.
- **Stacking too many bounds inline.** Move them to a `where` clause for readability.
- **`Box<dyn Trait>` everywhere when `impl Trait` would do.** Indirection has a cost; reach for trait objects when you actually need the heterogeneity.
- **`#[derive(Copy)]` without `Clone`.** `Copy: Clone` — if you derive `Copy`, you must also derive (or implement) `Clone`.
- **Hash + Eq inconsistency.** If you implement them manually, equal values *must* hash to the same value. Otherwise `HashMap` corrupts silently. Derive both together if possible.
- **`PartialOrd` vs `Ord`.** Floats implement `PartialOrd` but not `Ord` (because of NaN). To sort `Vec<f64>`, use `sort_by(|a,b| a.partial_cmp(b).unwrap())` or `sort_by(f64::total_cmp)`.
- **Trait bounds on struct definitions.** Idiomatic Rust pushes bounds onto `impl` blocks instead of struct definitions: `struct Foo<T> { ... }` and `impl<T: Bound> Foo<T> { ... }`. Keeps construction unconstrained.

---

## 🎯 Key Takeaways

- Traits define *behavior contracts* — implement them on types to enable polymorphism without inheritance.
- Generics + trait bounds give you static dispatch, monomorphized to zero-cost specialized code.
- `dyn Trait` opts into runtime dispatch for heterogeneous collections and plugin patterns; pay one indirection.
- `#[derive(...)]` covers the common traits — learn the standard set (`Debug`, `Clone`, `PartialEq`, `Hash`, `Default`).
- Associated types let traits be *parameterized by output type* — the cleanest way to express "this trait determines a related type."

*[← prev](./08_error_handling.md) | [next →](./10_lifetimes.md)*
