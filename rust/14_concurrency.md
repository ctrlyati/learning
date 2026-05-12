# 14 — Concurrency: Threads, Channels, `Send`/`Sync`, `Mutex`

> **Goal:** Write correct multi-threaded code, leveraging the type system to prevent data races at compile time.

## 1. "Fearless Concurrency" — What That Actually Means

In C/C++, multi-threaded code is the source of the worst bugs in the industry: data races, torn reads, ABA, use-after-free across threads. Most are detected only in production, often years later, sometimes never.

Rust's pitch is simple: **the same ownership and borrowing rules that prevent use-after-free in single-threaded code prevent data races in multi-threaded code.** The mechanism is two marker traits — `Send` and `Sync` — that the compiler uses to verify, at compile time, what's safe to share or send.

You will still write deadlocks. The compiler can't catch those. But data races (two threads, one piece of memory, at least one writer, no synchronization) — those are caught.

---

## 2. Spawning Threads

```rust
use std::thread;
use std::time::Duration;

fn main() {
    let h = thread::spawn(|| {
        for i in 1..5 {
            println!("worker {i}");
            thread::sleep(Duration::from_millis(10));
        }
    });

    for i in 1..3 {
        println!("main {i}");
        thread::sleep(Duration::from_millis(15));
    }

    h.join().unwrap();    // wait; unwrap propagates a panic from the worker
}
```

`thread::spawn` returns a `JoinHandle<T>` whose `T` is whatever the closure returns. `.join()` blocks the caller until the thread finishes, returning `Result<T, _>` (the `Err` carries any panic).

### Moving Data In

The closure must own everything it captures, because the thread might outlive the spawning scope:

```rust
let v = vec![1, 2, 3];
let h = thread::spawn(move || {
    println!("{v:?}");
});
h.join().unwrap();
```

Without `move`, the compiler complains that `v` might be dropped before the thread reads it.

### Scoped Threads (since Rust 1.63) — Borrow Across Threads

When threads are guaranteed to finish within a scope, you can borrow:

```rust
use std::thread;

fn main() {
    let v = vec![1, 2, 3];
    thread::scope(|s| {
        s.spawn(|| println!("{v:?}"));     // borrow &v — OK, scoped
        s.spawn(|| println!("len {}", v.len()));
    });
    // both threads joined automatically here
    println!("still have v: {v:?}");
}
```

Use scoped threads whenever your parallelism is fork-join — much less painful than `Arc`/`clone` ceremony.

---

## 3. Sharing State — `Mutex`, `RwLock`, Atomics

Threads usually need to share data. Rust gives you three primary tools.

### `Mutex<T>` — Mutual Exclusion

```rust
use std::sync::{Arc, Mutex};
use std::thread;

fn main() {
    let counter = Arc::new(Mutex::new(0_u64));
    let mut handles = vec![];

    for _ in 0..10 {
        let c = Arc::clone(&counter);
        handles.push(thread::spawn(move || {
            for _ in 0..1000 {
                let mut n = c.lock().unwrap();   // blocks until acquired
                *n += 1;
            }
        }));
    }

    for h in handles { h.join().unwrap(); }
    println!("{}", *counter.lock().unwrap());    // 10000
}
```

`Mutex::lock()` returns `Result<MutexGuard<T>, PoisonError<...>>`. The guard is a smart pointer; when dropped (end of scope), the lock is released. **There is no separate `unlock` call.** This is critical for correctness — you can't forget to release.

A lock is "poisoned" if a thread panicked while holding it. `unwrap()` propagates that; in production, decide whether you trust the data after a panic.

### `RwLock<T>` — Many Readers OR One Writer

```rust
use std::sync::RwLock;

let lock = RwLock::new(5);
{
    let r1 = lock.read().unwrap();
    let r2 = lock.read().unwrap();        // multiple readers OK
    println!("{r1} {r2}");
}
{
    let mut w = lock.write().unwrap();    // exclusive
    *w += 1;
}
```

Use `RwLock` when reads vastly outnumber writes; for balanced or write-heavy workloads, `Mutex` is often faster (less bookkeeping).

### Atomics — Lock-Free for Primitives

For single integers and booleans, atomics are faster than `Mutex`:

```rust
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::thread;

let counter = Arc::new(AtomicU64::new(0));
let mut handles = vec![];
for _ in 0..10 {
    let c = Arc::clone(&counter);
    handles.push(thread::spawn(move || {
        for _ in 0..1000 {
            c.fetch_add(1, Ordering::Relaxed);
        }
    }));
}
for h in handles { h.join().unwrap(); }
println!("{}", counter.load(Ordering::Relaxed));   // 10000
```

`Ordering::Relaxed` is fine for a simple counter (no other memory depends on its value). For data-dependent ordering, you need `Acquire`/`Release` or `SeqCst` — read about memory ordering before using anything beyond `Relaxed`.

---

## 4. Channels — Message Passing

Often easier to reason about than shared state: have threads communicate by *sending values*. Standard library:

```rust
use std::sync::mpsc;        // multi-producer, single-consumer
use std::thread;

fn main() {
    let (tx, rx) = mpsc::channel();

    for i in 0..3 {
        let tx = tx.clone();
        thread::spawn(move || {
            tx.send(format!("from {i}")).unwrap();
        });
    }
    drop(tx);   // close the original sender so rx loop ends

    for msg in rx {     // iterates until all senders dropped
        println!("{msg}");
    }
}
```

Sending **moves** the value into the channel. The receiver gets ownership. There's no shared mutable state — therefore no synchronization headache.

For higher-throughput, multi-consumer, or async channels, use `crossbeam-channel` (sync) or `tokio::sync::mpsc` (async — module 15).

---

## 5. `Send` and `Sync` — How the Compiler Knows It's Safe

These are **marker traits** (no methods). They classify types:

- **`Send`** — safe to *transfer ownership* to another thread.
- **`Sync`** — safe to *share a reference* (`&T`) across threads. Equivalently: `T: Sync` iff `&T: Send`.

Most types are auto-`Send` and auto-`Sync` — the compiler derives them based on fields. Notable exceptions:

| Type            | Send? | Sync? | Why                                     |
| --------------- | ----- | ----- | --------------------------------------- |
| `Rc<T>`         | No    | No    | refcount is not atomic                  |
| `RefCell<T>`    | Yes¹  | No    | runtime borrow check is not atomic      |
| `Cell<T>`       | Yes¹  | No    | same                                    |
| `*const T`, `*mut T` | No | No   | raw pointers — opt in if you know       |
| `MutexGuard<T>` | No    | Sync (if T:Sync) | guards must be released on the same thread that took them |

¹ if `T: Send`.

When you write a generic function that spawns a thread:

```rust
fn spawn<F>(f: F) where F: FnOnce() + Send + 'static {
    std::thread::spawn(f);
}
```

The `Send + 'static` bound is what forces callers to pass owning, thread-safe data. Try to pass an `Rc` and the compiler stops you with a clear message.

### Practical Example — Parallel Word Counter

```rust
use std::sync::{Arc, Mutex};
use std::thread;
use std::collections::HashMap;

fn count_words(texts: Vec<String>) -> HashMap<String, u64> {
    let result = Arc::new(Mutex::new(HashMap::<String, u64>::new()));
    let mut handles = vec![];

    for text in texts {
        let result = Arc::clone(&result);
        handles.push(thread::spawn(move || {
            // do the work locally — minimize lock contention
            let mut local = HashMap::<String, u64>::new();
            for word in text.split_whitespace() {
                *local.entry(word.to_string()).or_insert(0) += 1;
            }
            // merge into shared state once
            let mut guard = result.lock().unwrap();
            for (k, v) in local {
                *guard.entry(k).or_insert(0) += v;
            }
        }));
    }

    for h in handles { h.join().unwrap(); }
    Arc::try_unwrap(result).unwrap().into_inner().unwrap()
}

fn main() {
    let texts = vec![
        "the quick brown fox".into(),
        "the lazy dog jumps".into(),
        "the fox is quick".into(),
    ];
    let counts = count_words(texts);
    let mut pairs: Vec<_> = counts.into_iter().collect();
    pairs.sort_by(|a, b| b.1.cmp(&a.1));
    for (w, c) in pairs.iter().take(5) {
        println!("{w:>10}: {c}");
    }
}
```

Pattern to remember: **work locally, merge once.** Per-word locking would be a contention disaster. Each thread builds its own `HashMap`, then takes the lock exactly once to merge.

For compute-heavy parallelism without writing thread plumbing yourself, use **`rayon`**:

```rust
use rayon::prelude::*;

let total: u64 = (0..1_000_000_u64).into_par_iter().filter(|n| n % 7 == 0).sum();
```

`into_par_iter()` parallelizes. `rayon` handles thread pool, work-stealing, and ordering.

---

## 5. Common Mistakes & Gotchas

- **Deadlock from acquiring two `Mutex`es in different orders.** Always lock in the same global order across the codebase. If you can't, use `try_lock` and back off.
- **Holding a `MutexGuard` across an `await` (in async code).** The guard is `!Send`, so the future becomes `!Send` and can't run on a multi-thread runtime. Drop the guard before awaiting, or use `tokio::sync::Mutex`.
- **`Arc<Mutex<T>>::lock().unwrap()` panicking with `PoisonError`.** Some thread panicked while holding the lock. Decide your recovery strategy; consider `parking_lot::Mutex` which doesn't poison.
- **Sharing `Rc<T>` across threads.** Compiler stops you. Switch to `Arc<T>`.
- **`RefCell` inside `Arc`.** Compiles, but `RefCell` isn't `Sync`, so the compound `Arc<RefCell<T>>` won't be shared between threads. Use `Arc<Mutex<T>>`.
- **Lock granularity too coarse** — every operation contends. Split into per-shard locks (`DashMap` is a ready-made example).
- **Lock granularity too fine** — overhead exceeds the work done. Profile.
- **Spawning too many OS threads.** Each costs ~1-2 MB stack. For 10k+ tasks, use async (module 15) or a thread pool (`rayon`).
- **`unwrap`ing `JoinHandle::join()` and silently losing panics.** In production, log or surface them.
- **Using `Mutex<Vec<T>>` when `crossbeam-channel` would do.** Channels often express the intent better and reduce contention.

---

## 🎯 Key Takeaways

- `thread::spawn` for unscoped threads (need `move`, owned data); `thread::scope` for fork-join (can borrow).
- Share state with `Arc<Mutex<T>>`, `Arc<RwLock<T>>`, or atomics; communicate with channels (`mpsc`, `crossbeam`).
- `Send` = transferable to another thread; `Sync` = shareable by reference. The compiler enforces both — data races are caught at compile time.
- Hold locks for the shortest possible time; do work locally, merge under the lock once.
- For data-parallel workloads, reach for `rayon` before hand-rolling threads — it's almost always faster and shorter.

*[← prev](./13_smart_pointers.md) | [next →](./15_async_rust.md)*
