# 07 — Child Processes & Worker Threads

> **Goal:** Run CPU-bound computation without blocking the main event loop, understand when to spawn processes vs threads, and utilize SharedArrayBuffer.

---

## 1. Concept: Multi-Processing vs Multi-Threading

Because your main JavaScript code runs on a single thread, heavy CPU computation (like image processing, data encryption, or machine learning calculations) will block the event loop. To bypass this, Node.js offers two scaling mechanisms:

| Strategy | Module | Description | Isolation | Overhead |
|----------|--------|-------------|-----------|----------|
| **Child Process** | `child_process` | Spawns a completely new OS process (runs any CLI tool or JS script). | Isolated memory. Communicates via IPC/Pipes. | High (requires full OS process setup). |
| **Worker Thread** | `worker_threads` | Spawns a new thread in the same OS process (runs JS code). | Shared memory available. Communicates via MessagePort. | Low (fast startup, light memory footprint). |

---

## 2. Mechanism: Spawning & Message Ports

### Child Process: `spawn` vs `exec`
- **`exec()`**: Runs a command in a subshell and buffers the entire stdout/stderr output. **If the output exceeds `maxBuffer` (default: 1MB), the process crashes.**
- **`spawn()`**: Streams output chunks via stdout/stderr readable streams. Ideal for large outputs or long-running tasks.
- **`fork()`**: A special case of `spawn()` specifically for Node modules, opening a dedicated Inter-Process Communication (IPC) channel.

### Worker Threads: Message Passing & Shared Memory
Workers run in separate threads, each with its own V8 instance and event loop.
- By default, communication uses `parentPort.postMessage(data)` which **serializes** data, creating a copy on the target thread.
- For maximum performance, you can use `SharedArrayBuffer` to share memory directly between threads, modifying raw byte arrays concurrently using atomic operations (`Atomics`).

---

## 3. Variations & Depth: Memory Synchronization

When sharing memory using `SharedArrayBuffer`, multiple threads can modify the same memory cells simultaneously, creating race conditions. The global `Atomics` object provides thread-safe operations (like adding or loading values) and thread waiting/waking controls.

---

## 4. Practical Application: Offloading CPU Work to a Thread

Let's build a main script that spawns a Worker thread to compute a CPU-bound Fibonacci sequence.

**`worker.js` (The Worker Script)**
```javascript
import { parentPort } from 'node:worker_threads';

function fibonacci(n) {
  if (n <= 1) return n;
  return fibonacci(n - 1) + fibonacci(n - 2);
}

// Listen for messages from the main thread
parentPort.on('message', (n) => {
  console.log(`[Worker] Starting computation for: ${n}`);
  const result = fibonacci(n);
  // Send result back
  parentPort.postMessage({ n, result });
});
```

**`main.js` (The Orchestrator)**
```javascript
import { Worker } from 'node:worker_threads';
import { fileURLToPath } from 'url';
import path from 'path';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function computeFibonacciAsync(n) {
  return new Promise((resolve, reject) => {
    // Spawn the worker thread
    const worker = new Worker(path.join(__dirname, 'worker.js'));
    
    worker.postMessage(n);

    worker.on('message', (data) => {
      resolve(data.result);
      worker.terminate(); // Shut down worker thread
    });

    worker.on('error', (err) => {
      reject(err);
      worker.terminate();
    });

    worker.on('exit', (code) => {
      if (code !== 0) {
        reject(new Error(`Worker stopped with exit code ${code}`));
      }
    });
  });
}

// Main execution
async function run() {
  console.log('[Main] Event Loop is active and responsive.');
  
  // Schedule a timer to prove the main event loop isn't blocked
  const timer = setInterval(() => {
    console.log('[Main] Tick...');
  }, 100);

  const target = 40; // Heavy CPU computation
  const result = await computeFibonacciAsync(target);
  
  console.log(`[Main] Computation finished: Fib(${target}) = ${result}`);
  clearInterval(timer);
}

run().catch(console.error);
```

---

## 5. Common Mistakes & Gotchas

- **Shell Injection:** Passing unescaped user inputs directly to `exec()` (e.g. `exec("ping " + userInput)`) allows commands like `127.0.0.1; rm -rf /`. Always use `spawn()` with an argument array instead: `spawn("ping", [userInput])`.
- **Worker Thread Spawning Overhead:** Booting a new worker thread takes ~10–50ms. Spawning a new worker for every single HTTP request will degrade performance. Instead, use a **worker pool** (like `piscina`) to reuse threads.
- **Buffer overflows in `exec`:** Calling `exec("cat huge_file.txt")` will exhaust the default 1MB memory buffer. Always use `spawn` and stream the output.

---

## 🎯 Key Takeaways

- **Use Child Processes** to execute native OS commands or separate scripts (e.g., Python scripts).
- **Use Worker Threads** for running intensive JavaScript tasks in parallel.
- **Never spawn threads dynamically on a per-request basis.** Keep a persistent thread pool to balance startup overhead.

---

*← [asynchronous patterns](./06_asynchronous_patterns.md) | [next → 08 Performance & Debugging](./08_performance_and_debugging.md)*
