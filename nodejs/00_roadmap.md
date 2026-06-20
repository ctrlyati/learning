# 00 — Node.js Deep-Dive Roadmap

> **Goal:** Take a competent JavaScript developer from basic script-running to production-grade Node.js backend engineering and runtime fluency.

This course focuses on Node.js internals, asynchronous patterns, system APIs, performance profiling, and production operations. We assume you already know standard JavaScript syntax. By the end of this course, you will understand how libuv schedules tasks, how to prevent event loop blockages, how to architect high-throughput stream pipelines, and how to debug memory leaks in production.

---

## Prerequisites

Before starting, you should be comfortable with:
- **JavaScript Fundamentals** — closures, promises, async/await, scope. See [`../javascript/00_roadmap.md`](../javascript/00_roadmap.md).
- **TypeScript Basics** — basic types, interfaces. See [`../typescript/00_roadmap.md`](../typescript/00_roadmap.md).
- **Command Line Basics** — navigating directories, running processes, env variables.
- A local install of **Node.js LTS (v20 or v22)**.

---

## Module Table

| #  | File                                                           | Topic                                                         | Est. Time |
|----|----------------------------------------------------------------|---------------------------------------------------------------|-----------|
| 00 | `00_roadmap.md`                                                | This file                                                     | 30 min    |
| 01 | `01_architecture_and_v8.md`                                    | Event loop (libuv), V8, single thread vs thread pool          | 2.5 h     |
| 02 | `02_modules_esm_vs_cjs.md`                                     | CommonJS vs ESM, resolution algorithms, interop               | 2 h       |
| 03 | `03_buffers_and_streams.md`                                    | TypedArrays, Buffer memory, Stream APIs, pipelines, backpressure| 3 h     |
| 04 | `04_file_system_and_path.md`                                   | Fs/promises, file descriptors, paths, symbolic links           | 2 h       |
| 05 | `05_networking_http_net.md`                                    | TCP sockets, HTTP agents, streaming responses, keep-alive     | 3 h       |
| 06 | `06_asynchronous_patterns.md`                                  | Callback patterns, EventEmitters, AsyncLocalStorage           | 2.5 h     |
| 07 | `07_child_processes_and_worker_threads.md`                     | Spawning CLI tools, fork, worker_threads, shared memory       | 3 h       |
| 08 | `08_performance_and_debugging.md`                              | Memory leaks, flamegraphs, heap snapshots, Chrome DevTools   | 3 h       |
| 09 | `09_security_and_production.md`                                | Security practices, clustering, Dockerization, process manager| 2.5 h     |

**Total**: ~24 hours of study. At 1 module per day, it takes about **1.5 to 2 weeks**.

---

## Core Mental Models

### 1. The Event Loop is run by libuv, not V8
V8 compiles and executes JavaScript code. libuv handles the asynchronous I/O (network, file system, database connections) using the operating system's non-blocking primitives (like epoll, kqueue, or IOCP). When the OS completes the I/O, libuv places the callback on the event loop queue for V8 to execute.

### 2. Node.js is single-threaded, but multi-threaded under the hood
Your JavaScript code runs on a single main thread. However, libuv maintains a background thread pool (default size is 4) for CPU-bound tasks or APIs that do not support non-blocking OS system calls (e.g., DNS lookups, crypto, compression, and some file system operations on certain OSes).

### 3. Streams prevent memory exhaustion
Instead of reading a 5GB file entirely into RAM before serving it, streams read and write the data in chunks (usually 64KB). By linking readable streams to writable streams using pipelines with built-in backpressure handling, you can process massive datasets with a constant, tiny memory footprint.

### 4. CommonJS and ES Modules are fundamentally different
CommonJS loads modules synchronously and evaluates them at runtime (`require`). ES Modules are parsed and resolved statically before execution (`import`). Combining them requires careful configuration and understanding of how Node.js resolves imports.

---

## External Resources

- **[Node.js Docs](https://nodejs.org/docs/)** — The official API documentation.
- **[libuv docs](http://docs.libuv.org/)** — For understanding the underlying C library.
- **[Node.js Diagnostics Guide](https://nodejs.org/en/learn/diagnostics)** — Essential for performance debugging.

---

*next →* [`01_architecture_and_v8.md`](./01_architecture_and_v8.md)
