# 15 — Async Rust: Futures, `tokio`, `async/await`

> **Goal:** Write efficient I/O-bound concurrent code with `async`/`await`, `tokio`, and the right mental model for what the compiler is doing under the hood.

This is the hardest chapter for many Rust learners. Async in Rust is not async in Go or Node.js. The model is different — and the compiler errors can be cryptic until the model clicks.

## 1. The Mental Model — Async = Polling, Not Threads

In Go, `go f()` spawns a goroutine, scheduled by the runtime onto OS threads. In Node.js, `await fetch()` registers a callback with the event loop. In Rust, `async fn` is a **state machine** that the compiler generates from your code, and a **runtime** (which you provide!) drives it.

Three layers:

1. **`async fn` and `.await`** — language syntax that the compiler desugars into a state machine implementing the `Future` trait.
2. **`Future` trait** — `poll(self, cx) -> Poll<Output>`. The runtime calls `poll`; the future either returns `Ready(value)` or `Pending` (and registers itself to be polled again when something changes).
3. **Runtime** (`tokio`, `async-std`, `smol`) — owns the executor that polls futures, plus reactor/timers/IO. Not in the standard library.

Key differences from threaded code:
- Each task is *cheap* — kilobytes, not megabytes. You can have a million.
- Tasks are scheduled cooperatively at `.await` points. A task that doesn't `.await` blocks its thread.
- Most async APIs are zero-cost when not actually pending: a `read` that has data ready returns immediately.

```rust
// Cargo.toml: tokio = { version = "1", features = ["full"] }
use std::time::Duration;

#[tokio::main]
async fn main() {
    let h = tokio::spawn(async {
        for i in 1..4 {
            println!("worker {i}");
            tokio::time::sleep(Duration::from_millis(50)).await;
        }
    });

    for i in 1..3 {
        println!("main {i}");
        tokio::time::sleep(Duration::from_millis(70)).await;
    }
    h.await.unwrap();
}
```

`#[tokio::main]` is sugar that turns your `async fn main` into a synchronous `main` that builds a tokio runtime and `block_on`s the future. Without it, `main` cannot be `async`.

---

## 2. `async`, `.await`, and What They Compile To

`async fn` returns a value implementing `Future`. The body doesn't run until you `.await` (or otherwise drive) the resulting future:

```rust
async fn fetch_user(id: u64) -> String {
    format!("user {id}")
}

#[tokio::main]
async fn main() {
    let f = fetch_user(7);   // does NOT print or run anything yet — just builds a future
    let s = f.await;          // now it runs to completion
    println!("{s}");
}
```

`.await` says: "park this task until the inner future is `Ready`; let the runtime run something else in the meantime." It can only be used inside `async fn` or an `async {}` block.

### Concurrency Within a Task — `join!` and `select!`

`.await` is sequential — one then the next. To do things concurrently:

```rust
use tokio::join;

#[tokio::main]
async fn main() {
    let (a, b) = join!(fetch_user(1), fetch_user(2));   // both run concurrently
    println!("{a} / {b}");
}
```

`join!` polls all futures concurrently, completes when all finish.

`select!` waits for the *first* to finish:

```rust
use tokio::select;

select! {
    res = fetch_user(1) => println!("user1: {res}"),
    _ = tokio::time::sleep(Duration::from_secs(1)) => println!("timeout"),
}
```

### `tokio::spawn` — True Parallel Tasks

`join!` is concurrency *within* one task. `tokio::spawn` hands a future to the runtime to run on its thread pool:

```rust
let h = tokio::spawn(async { /* runs in parallel */ });
let v = h.await.unwrap();    // .await yields the JoinHandle's result
```

The future passed to `spawn` must be `Send + 'static` (it may run on any worker thread, and live arbitrarily long).

---

## 3. Async I/O — the Whole Point

Sync I/O blocks the OS thread. Async I/O lets one thread juggle thousands of in-flight operations.

### TCP Echo Server

```rust
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;

#[tokio::main]
async fn main() -> std::io::Result<()> {
    let listener = TcpListener::bind("127.0.0.1:8080").await?;
    println!("listening on 127.0.0.1:8080");

    loop {
        let (mut socket, addr) = listener.accept().await?;
        println!("connection from {addr}");

        tokio::spawn(async move {
            let mut buf = [0u8; 1024];
            loop {
                let n = match socket.read(&mut buf).await {
                    Ok(0) => return,                     // EOF
                    Ok(n) => n,
                    Err(e) => { eprintln!("read: {e}"); return; }
                };
                if let Err(e) = socket.write_all(&buf[..n]).await {
                    eprintln!("write: {e}"); return;
                }
            }
        });
    }
}
```

Per-connection task. Tens of thousands of connections from one process — possible because each is a small future, not a 2 MB OS thread.

### HTTP Client (with `reqwest`)

```rust
// Cargo.toml: reqwest = { version = "0.12", features = ["json"] }
//             tokio = { version = "1", features = ["full"] }

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let body = reqwest::get("https://httpbin.org/json")
        .await?
        .text()
        .await?;
    println!("{body}");
    Ok(())
}
```

Two `.await`s — one for the response headers, one for the body. The connection holds open through both, but the task yields the runtime in between.

### Concurrent Requests — `FuturesUnordered`

```rust
use futures::stream::{FuturesUnordered, StreamExt};

#[tokio::main]
async fn main() {
    let urls = ["https://example.com", "https://rust-lang.org", "https://crates.io"];
    let mut futs: FuturesUnordered<_> = urls
        .iter()
        .map(|u| async move {
            let body = reqwest::get(*u).await?.text().await?;
            Ok::<(_, _), reqwest::Error>((u, body.len()))
        })
        .collect();

    while let Some(res) = futs.next().await {
        match res {
            Ok((u, len)) => println!("{u}: {len} bytes"),
            Err(e)       => eprintln!("error: {e}"),
        }
    }
}
```

`FuturesUnordered` runs many futures concurrently and yields results in completion order — perfect for "fan out, collect as they finish."

---

## 4. Practical Example — Concurrent Web Scraper With Bounded Parallelism

A common shape: process a queue of URLs, but limit how many run at once (don't get banned, don't OOM).

```rust
use std::sync::Arc;
use tokio::sync::Semaphore;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let urls: Vec<String> = (1..=20)
        .map(|i| format!("https://httpbin.org/delay/1?id={i}"))
        .collect();

    let sem = Arc::new(Semaphore::new(5));     // at most 5 in flight
    let client = reqwest::Client::new();

    let mut handles = vec![];
    for url in urls {
        let sem = Arc::clone(&sem);
        let client = client.clone();             // reqwest::Client is cheap to clone (Arc inside)
        handles.push(tokio::spawn(async move {
            let _permit = sem.acquire_owned().await.unwrap();
            let body = client.get(&url).send().await?.text().await?;
            Ok::<(_, _), reqwest::Error>((url, body.len()))
        }));
    }

    for h in handles {
        match h.await.unwrap() {
            Ok((u, n)) => println!("{u} -> {n} bytes"),
            Err(e)     => eprintln!("err: {e}"),
        }
    }
    Ok(())
}
```

What this exercises:
- `Arc<Semaphore>` — a shared concurrency limiter; permits return automatically when the guard drops.
- `reqwest::Client::clone()` — internally `Arc`'d, so it's cheap to clone per task and pools connections.
- Each URL becomes its own spawned task; the runtime schedules them across its worker threads.
- `tokio::spawn` returns a `JoinHandle<T>`; awaiting it yields `Result<T, JoinError>`.

This pattern — spawn N, gate by semaphore, collect — covers a huge fraction of real async Rust.

---

## 5. Common Mistakes & Gotchas

- **Forgetting `.await`.** `let f = fetch();` doesn't run anything — `fetch` returns an unstarted future. Compiler often warns ("unused implementer of `Future`"). Listen.
- **Calling sync, blocking code inside async.** `std::thread::sleep`, blocking file I/O, CPU-heavy loops — they freeze the entire worker thread, starving every other task on it. Use `tokio::time::sleep`, `tokio::fs`, or `tokio::task::spawn_blocking` for legitimately blocking code.
- **`MutexGuard` held across `.await`.** The guard is `!Send`, so the future becomes `!Send`, and tokio's multi-thread runtime rejects it. Either drop the guard before `.await` (`let n = { *guard }; drop(guard); something().await;`) or use `tokio::sync::Mutex`.
- **Awaiting in a loop instead of concurrently.** `for url in urls { fetch(url).await; }` is sequential. Use `join_all`, `FuturesUnordered`, or spawn tasks.
- **Using `Rc` in async tasks** that run on a multi-thread runtime. `Rc` is `!Send`. Use `Arc`.
- **Recursive `async fn`.** Compiler error: a future contains itself, infinite size. Wrap the recursive call: `Box::pin(my_async_fn(...))`.
- **Mixing runtimes** (e.g., calling an `async-std` API inside a tokio task). Some types are runtime-specific (timers, I/O). Pick one runtime per project — almost always `tokio`.
- **`.await` inside a `match` guard or in places it's not allowed.** Restructure to `let result = future.await; match result { ... }`.
- **Ignoring `JoinHandle` results.** Spawning and dropping the handle is fine if you don't need the result, but errors and panics in the spawned task get silently lost. Log them or `.await` the handle.
- **Cancellation surprises.** Dropping a future cancels it at the next `.await` boundary. `select!` cancels the losers — make sure that's safe (no half-applied state).

---

## 🎯 Key Takeaways

- Async in Rust is a *zero-cost state machine* generated by the compiler; a runtime (almost always `tokio`) polls those state machines on a thread pool.
- `async fn`s return futures that don't do anything until awaited — there's no implicit task spawning.
- For real parallelism within a runtime use `tokio::spawn`; for in-task concurrency use `join!`, `select!`, or `FuturesUnordered`.
- Never block the runtime. Use async I/O, async sleeps, and `spawn_blocking` for unavoidable sync work.
- `Send` matters even more here than in plain threads — guards held across `.await` are the most common stumbling block.

*[← prev](./14_concurrency.md) | [next →](./16_unsafe_and_ffi.md)*
