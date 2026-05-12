# 13 — Smart Pointers: `Box`, `Rc`, `Arc`, `RefCell`, Interior Mutability

> **Goal:** Reach for the right smart pointer when ordinary references can't express your design — and understand the cost.

## 1. `Box<T>` — Heap Allocation, Single Owner

`Box<T>` puts a value on the heap and gives you a uniquely owning pointer to it. It's the simplest smart pointer.

```rust
fn main() {
    let b = Box::new(42);          // i32 on the heap
    println!("{}", *b);             // deref to read
}
// b dropped here → heap freed
```

You reach for `Box` in three situations:

**1. Recursive types** — Rust must know struct sizes at compile time, but a node containing itself has infinite size. The fix: wrap the recursive field in a `Box`, which has fixed size (one pointer).

```rust
enum List {
    Cons(i32, Box<List>),
    Nil,
}
let l = List::Cons(1, Box::new(List::Cons(2, Box::new(List::Nil))));
```

**2. Trait objects** — `Box<dyn Trait>` for heap-allocated polymorphism.

```rust
trait Animal { fn speak(&self); }
struct Dog; impl Animal for Dog { fn speak(&self) { println!("woof"); } }
struct Cat; impl Animal for Cat { fn speak(&self) { println!("meow"); } }

let zoo: Vec<Box<dyn Animal>> = vec![Box::new(Dog), Box::new(Cat)];
for a in &zoo { a.speak(); }
```

**3. Large values you want off the stack** — moving a `Box` is cheap (one pointer copy) regardless of the inner data's size.

`Box` follows ownership rules normally: one owner, drops when out of scope. There's no reference counting; it's just "the value lives on the heap."

---

## 2. `Rc<T>` and `Arc<T>` — Shared Ownership

Sometimes a single owner doesn't fit your model — multiple parts of the program need to keep a value alive. **Reference counting** is the answer.

### `Rc<T>` — single-threaded shared ownership

```rust
use std::rc::Rc;

fn main() {
    let a = Rc::new(String::from("shared"));
    let b = Rc::clone(&a);            // count = 2 — does NOT clone the String
    let c = Rc::clone(&a);            // count = 3
    println!("{a} {b} {c}");
    println!("count: {}", Rc::strong_count(&a));   // 3
}
// Each Rc dropped decrements; when count hits 0, the String is dropped.
```

`Rc::clone(&x)` is the idiomatic call; calling `x.clone()` works but is visually identical to a deep clone, which it isn't.

`Rc<T>` is **not** thread-safe — its counters are not atomic. The compiler enforces this: `Rc<T>` does not implement `Send`. Try to send it to another thread and you'll see a clear error.

### `Arc<T>` — multi-threaded shared ownership

`Arc` ("atomic Rc") has the same API but uses atomic counters, making it `Send + Sync`. Use it whenever you share data across threads.

```rust
use std::sync::Arc;
use std::thread;

fn main() {
    let data = Arc::new(vec![1, 2, 3]);
    let mut handles = vec![];
    for i in 0..3 {
        let d = Arc::clone(&data);
        handles.push(thread::spawn(move || {
            println!("thread {i}: {:?}", *d);
        }));
    }
    for h in handles { h.join().unwrap(); }
}
```

**Cost:** atomic ops are slightly more expensive than non-atomic. Use `Rc` when you know it's single-threaded; `Arc` when you don't.

### Cycles and `Weak`

Reference counting **does not collect cycles.** If `A` holds an `Rc<B>` and `B` holds an `Rc<A>`, neither will ever drop. Memory leak.

The fix: `Weak<T>`. A `Weak` doesn't keep the value alive; you upgrade to `Rc` to use it (returning `Option<Rc<T>>`):

```rust
use std::rc::{Rc, Weak};
use std::cell::RefCell;

struct Node {
    value: i32,
    parent: RefCell<Weak<Node>>,
    children: RefCell<Vec<Rc<Node>>>,
}
```

Parent-child trees are the canonical example: parents hold strong refs to children, children hold weak refs to parent.

---

## 3. `RefCell<T>` — Interior Mutability

The borrow checker normally enforces "exclusive write XOR shared read" at *compile time*. Sometimes you need to defer that check to *runtime* — common when you have an `Rc<T>` (shared, immutable) but need to mutate the inside.

`RefCell<T>` lets you call `.borrow()` (returns a guard with `Deref` to `&T`) or `.borrow_mut()` (`Deref` to `&mut T`). The same rules apply, but violations panic at runtime instead of failing to compile.

```rust
use std::cell::RefCell;

fn main() {
    let c = RefCell::new(5);
    *c.borrow_mut() += 1;          // mutate through immutable binding
    println!("{}", c.borrow());     // 6
}
```

The `Rc<RefCell<T>>` combo unlocks "shared, mutable" data:

```rust
use std::rc::Rc;
use std::cell::RefCell;

fn main() {
    let shared = Rc::new(RefCell::new(vec![1, 2, 3]));
    let alt = Rc::clone(&shared);
    alt.borrow_mut().push(4);
    println!("{:?}", shared.borrow());   // [1, 2, 3, 4]
}
```

For the multi-threaded analogue, swap `Rc<RefCell<T>>` for `Arc<Mutex<T>>` (module 14).

`Cell<T>` is the simpler cousin — for `Copy` types, it lets you `.get()` and `.set()` without borrowing semantics:

```rust
use std::cell::Cell;
let c = Cell::new(5);
c.set(c.get() + 1);
```

### When to Use What

| Need                                            | Reach for                              |
| ----------------------------------------------- | -------------------------------------- |
| Heap allocation, single owner                   | `Box<T>`                               |
| Multiple owners, single thread                  | `Rc<T>`                                |
| Multiple owners, multi-thread                   | `Arc<T>`                               |
| Mutate through shared reference, single thread  | `Rc<RefCell<T>>` or `Cell<T>`          |
| Mutate through shared reference, multi-thread   | `Arc<Mutex<T>>` or `Arc<RwLock<T>>`    |
| Avoid reference cycles                          | `Weak<T>` for backlinks                |
| Recursive types                                 | `Box<T>` for the recursive field       |

---

## 4. Practical Example — A Tree With Parent Backlinks

A common case: a tree where each node knows its children and (weakly) its parent.

```rust
use std::cell::RefCell;
use std::rc::{Rc, Weak};

struct Node {
    value: i32,
    parent: RefCell<Weak<Node>>,
    children: RefCell<Vec<Rc<Node>>>,
}

impl Node {
    fn new(value: i32) -> Rc<Self> {
        Rc::new(Self {
            value,
            parent: RefCell::new(Weak::new()),
            children: RefCell::new(vec![]),
        })
    }

    fn add_child(self: &Rc<Self>, child: Rc<Node>) {
        *child.parent.borrow_mut() = Rc::downgrade(self);
        self.children.borrow_mut().push(child);
    }

    fn parent_value(&self) -> Option<i32> {
        self.parent.borrow().upgrade().map(|p| p.value)
    }
}

fn main() {
    let root = Node::new(1);
    let child = Node::new(2);
    root.add_child(Rc::clone(&child));

    println!("child's parent value: {:?}", child.parent_value());   // Some(1)
    println!("root strong count: {}", Rc::strong_count(&root));     // 1
    println!("child strong count: {}", Rc::strong_count(&child));   // 2 (root + local)
}
```

Why this works:
- `Rc<Node>` lets the same child be referenced from multiple places (here just root, but in real trees you might query it from elsewhere).
- `RefCell<Weak<Node>>` for `parent` — interior mutability so `add_child` can set it without `&mut`, weak so it doesn't create a cycle that leaks.
- `Rc::downgrade` makes a `Weak` from an `Rc`; `weak.upgrade()` returns `Option<Rc<T>>`.

If you replaced `Weak` with `Rc` for `parent`, the tree would never drop. Try it — Rust won't catch the leak; you have to design around it.

---

## 5. Common Mistakes & Gotchas

- **Calling `x.clone()` on an `Rc<String>` to "copy the string."** It only bumps the refcount. The `String` itself is not duplicated. Use `(*x).clone()` to actually deep-clone.
- **Putting `Rc<T>` in a struct sent to another thread.** Compiler error: `Rc<T> cannot be sent between threads safely`. Use `Arc<T>`.
- **`already borrowed: BorrowMutError` panic from `RefCell`.** You called `borrow_mut()` while a `borrow()` (or another `borrow_mut`) is still alive. Often: a method takes `&self`, calls `self.field.borrow()`, and inside the borrow's scope calls another method that also borrows `self.field`. Reduce the borrow scope or restructure.
- **Reference cycles with `Rc<RefCell<T>>`.** Memory leak with no compiler warning. Use `Weak` for back-references.
- **`Box::leak(b)` everywhere.** It returns `&'static mut T` and never frees. Useful for tests and FFI; almost always wrong in app code.
- **Overusing `Rc<RefCell<T>>` because the borrow checker is annoying.** It's an escape hatch; if every field is `Rc<RefCell<...>>`, you've recreated the bug-prone shared-mutable model Rust is designed to prevent. Take a step back and rethink ownership.
- **Forgetting `Arc<Mutex<T>>` is two allocations.** For hot-path data, consider `parking_lot::Mutex` (faster) or atomics directly (`AtomicUsize`, etc.).
- **Treating `RefCell` as thread-safe.** It's not. Use `Mutex` or `RwLock` across threads.
- **Boxing trivially-sized values.** `Box<i32>` is almost always wrong — heap allocation for 4 bytes is wasteful. Box only when you need it (recursion, trait objects, large data).

---

## 🎯 Key Takeaways

- `Box<T>` is heap allocation with single ownership — for recursion, trait objects, and large data.
- `Rc<T>` and `Arc<T>` are reference-counted shared ownership — `Rc` single-threaded, `Arc` thread-safe; both are non-mutating views.
- `RefCell<T>` shifts borrow checking to runtime — combine with `Rc` for "shared mutable" single-threaded data.
- Reference cycles with `Rc`/`Arc` *leak silently* — break them with `Weak<T>` for back-links.
- Reach for these when ownership trees can't express your design; don't reach for them to escape the borrow checker.

*[← prev](./12_closures_iterators.md) | [next →](./14_concurrency.md)*
