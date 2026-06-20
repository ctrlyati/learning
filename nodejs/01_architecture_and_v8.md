# 01 — Node.js Architecture & V8 Internals

> **Goal:** Understand how Node.js leverages V8 and libuv to achieve high-performance asynchronous execution, and how to measure event loop delays.

---

## 1. Concept: Single-Threaded but Multi-Threaded

A common misconception is that Node.js is completely single-threaded. In reality, **your JavaScript code runs on a single thread**, but the underlying runtime (C++ bindings, V8, and libuv) is multi-threaded.

```
+-----------------------------------------------------------+
|                        Your JS Code                       |
+-----------------------------------------------------------+
                              |
                              v
+-----------------------------------------------------------+
|                    Node.js C++ Bindings                   |
+-----------------------------------------------------------+
          |                                       |
          v                                       v
+------------------+                    +-------------------+
|    V8 Engine     |                    |    libuv Loop     |
| (JS Compilation) |                    | (Async I/O Event) |
+------------------+                    +-------------------+
                                                  |
                                                  v
                                        +-------------------+
                                        | libuv Thread Pool |
                                        |  (Default size: 4)|
                                        +-------------------+
```

- **V8 Engine:** Compiles and executes JavaScript to machine code.
- **libuv:** A C library that abstracts non-blocking I/O operations (network sockets) using OS primitives (`epoll` on Linux, `kqueue` on macOS, `IOCP` on Windows) and manages a thread pool for operations that cannot be done asynchronously by the OS (DNS, File system calls, Crypto, Zlib).

---

## 2. Mechanism: The Event Loop & Microtask Queues

The Event Loop runs in a continuous loop. In each tick, it executes callbacks across several phases:

1. **Timers:** Executes callbacks scheduled by `setTimeout()` and `setInterval()`.
2. **Pending Callbacks:** Executes I/O callbacks deferred from the previous loop iteration.
3. **Idle, Prepare:** Used internally by the runtime.
4. **Poll:** Retrieves new I/O events; executes I/O-related callbacks. Node will block here if nothing else is scheduled.
5. **Check:** Executes callbacks scheduled by `setImmediate()`.
6. **Close Callbacks:** Executes close events like `socket.on('close')`.

### Microtask Queues (Tick Queues)
Between every phase of the event loop, Node.js checks and drains two microtask queues:
1. **`process.nextTick()` Queue:** Run first.
2. **Promise microtask Queue:** Run second.

```javascript
// Execution Order Demonstration
setTimeout(() => console.log("Timer phase"), 0);
setImmediate(() => console.log("Check phase"));
process.nextTick(() => console.log("NextTick microtask"));
Promise.resolve().then(() => console.log("Promise microtask"));
console.log("Synchronous code");

// Output:
// Synchronous code
// NextTick microtask
// Promise microtask
// Timer phase (or Check phase, depending on initialization timing)
```

---

## 3. Variations & Depth: Thread Pool Tuning

You can increase the size of the libuv thread pool if your app does heavy cryptographic operations or filesystem access:

```bash
# On Linux/macOS
export UV_THREADPOOL_SIZE=8

# On Windows (PowerShell)
$env:UV_THREADPOOL_SIZE="8"
```
The maximum value is `1024`. If set too high, thread context-switching overhead will degrade performance.

---

## 4. Practical Application: Event Loop Lag Monitor

You can monitor event loop health by measuring the delay between scheduled timer execution.

**`loop_monitor.js`**
```javascript
function monitorLoop() {
  let lastTime = Date.now();
  
  setInterval(() => {
    const now = Date.now();
    // In a perfect world, this interval runs exactly every 1000ms.
    // Any extra time is event loop delay (lag).
    const lag = now - lastTime - 1000;
    
    if (lag > 10) {
      console.warn(`[WARNING] Event Loop Lag detected: ${lag}ms`);
    } else {
      console.log(`Event Loop Lag: ${lag}ms`);
    }
    lastTime = now;
  }, 1000).unref(); // .unref() lets Node exit even if this timer is active
}

monitorLoop();

// Simulate blocking execution for 500ms
setTimeout(() => {
  console.log("Blocking event loop...");
  const start = Date.now();
  while (Date.now() - start < 500) {
    // Sync block
  }
  console.log("Unblocked.");
}, 2000);
```

---

## 5. Common Mistakes & Gotchas

- **Blocking the main thread:** Executing CPU-bound tasks (like sorting 10 million items, synchronous hashing, or parsing extremely large JSON arrays) blocks the loop. No other I/O callbacks will be serviced.
- **Using Sync methods in web servers:** Calling `fs.readFileSync()` on every request blocks the loop for all users. Always use `fs.promises` or callback equivalents in production.
- **Confusing `process.nextTick()` with `setImmediate()`:** `process.nextTick()` fires immediately after the current operation finishes (before the event loop moves to the next phase), meaning recursive `process.nextTick()` calls will starve all I/O!

---

## 🎯 Key Takeaways

- **JavaScript is single-threaded; Node.js is not.** Leverage background workers or external queues for heavy computation.
- **Never block the event loop.** A blocked event loop means your server cannot accept incoming requests or handle database responses.
- **Use `process.nextTick()` sparingly.** It executes before the event loop continues, which can cause starvation. Use `setImmediate()` for deferring work safely.

---

*← [roadmap](./00_roadmap.md) | [next → 02 Modules ESM vs CommonJS](./02_modules_esm_vs_cjs.md)*
