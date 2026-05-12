# 11 — The Event Loop in Depth (Browser & Node)

> **Goal:** Predict the execution order of *any* mix of sync code, promises, timers, and I/O — in both the browser and Node — by understanding the event loop's queues and phases.

---

## 1. The Event Loop — Mental Model

JS engines run on a single thread with one **call stack**. The event loop is the orchestrator that decides what runs next when the stack is empty:

```
┌────────────────────────────────────────────────┐
│              Call Stack (one)                  │
│   [ currently executing function frames ]      │
└──────────────────────┬─────────────────────────┘
                       │ when empty…
                       ▼
            drain ALL microtasks
                       │
                       ▼
            run ONE macrotask
                       │
                       ▼
            (browser only) render frame
                       │
                       ▼
                  back to top
```

**Two queue tiers**:
- **Microtasks** — promise callbacks (`.then`, `await` continuation), `queueMicrotask`, `MutationObserver`. Drained completely after each macrotask AND after the initial sync script.
- **Macrotasks (a.k.a. tasks)** — `setTimeout`, `setInterval`, `setImmediate` (Node), I/O callbacks, UI events, message events. One per loop iteration.

Crucially: **microtasks always run before the next macrotask.**

```js
console.log("A");
setTimeout(() => console.log("B"), 0);
Promise.resolve().then(() => console.log("C"));
queueMicrotask(() => console.log("D"));
console.log("E");
// A E C D B
```
- Sync: `A`, `E`.
- Microtasks drain: `C`, `D` (in order queued).
- One macrotask: `B`.

---

## 2. Browser Event Loop — Under the Hood

The browser model adds **rendering** to the loop. Each iteration:

1. Pick one task from the task queue, run it to completion.
2. Drain all microtasks.
3. If it's time to render a frame: run requestAnimationFrame callbacks, style/layout/paint.
4. Loop.

```js
// Visualizing the order
console.log("script start");

setTimeout(() => console.log("setTimeout"), 0);

Promise.resolve()
  .then(() => console.log("promise 1"))
  .then(() => console.log("promise 2"));

requestAnimationFrame(() => console.log("rAF"));

console.log("script end");

// script start
// script end
// promise 1
// promise 2
// rAF                ← before paint
// setTimeout
```

`requestAnimationFrame` runs *before* the next paint, so use it for visual updates (not for "I want this fast").

### The 4ms minimum & deferred timers
Browsers clamp `setTimeout(fn, 0)` to ~4ms after several nestings, and to 1000ms when the tab is backgrounded. Don't use `setTimeout` for high-frequency work — use `requestAnimationFrame` for visual or `MessageChannel` for fast yields.

```js
// Fast yield trick used by React, etc.
const channel = new MessageChannel();
function nextTask(cb) {
  channel.port1.onmessage = cb;
  channel.port2.postMessage(0);
}
```

---

## 3. Node Event Loop — Phases

Node uses libuv, which runs the loop in **phases**, each with its own callback queue:

```
   ┌───────────────────────────┐
┌─►│           timers          │  setTimeout / setInterval expirations
│  ├───────────────────────────┤
│  │     pending callbacks     │  some I/O errors deferred from prev iter
│  ├───────────────────────────┤
│  │       idle, prepare       │  internal
│  ├───────────────────────────┤
│  │           poll            │  I/O: read/write completions; may block
│  ├───────────────────────────┤
│  │           check           │  setImmediate callbacks
│  ├───────────────────────────┤
│  │      close callbacks      │  socket.on('close') etc
│  └───────────────────────────┘
└───────────── loop ─────────────
```

**Microtasks (and `process.nextTick`) drain between every phase and even between individual callbacks** — not just at the end.

### `process.nextTick` vs `queueMicrotask` vs Promises
- `process.nextTick` queue → drained **before** the promise microtask queue.
- Both drain before continuing to the next phase.

```js
setImmediate(() => console.log("immediate"));
setTimeout(() => console.log("timeout"), 0);
process.nextTick(() => console.log("nextTick"));
Promise.resolve().then(() => console.log("promise"));
console.log("sync");

// sync
// nextTick
// promise
// timeout      ← order between timeout/immediate is NOT guaranteed when at top level
// immediate
```

Inside an I/O callback, `setImmediate` is guaranteed to fire before any `setTimeout(..., 0)`:
```js
fs.readFile(__filename, () => {
  setTimeout(() => console.log("t"), 0);
  setImmediate(() => console.log("i"));
});
// i, then t — every time
```

This is one of the few places `setImmediate` matters: yielding back to the loop after a chunk of work without waiting on the timer phase.

### `setTimeout(fn, 0)` is not zero
Minimum is 1ms in Node (and 4ms in browsers after nesting). For "yield to the loop and continue ASAP," prefer `setImmediate` (Node) or `queueMicrotask`/`MessageChannel` (browser/cross-platform).

---

## 4. Practical Application — Predicting Order

### Example 1 — sync vs micro vs macro (cross-platform)
```js
console.log("1");
setTimeout(() => {
  console.log("2");
  Promise.resolve().then(() => console.log("3"));
}, 0);
Promise.resolve().then(() => {
  console.log("4");
  setTimeout(() => console.log("5"), 0);
});
console.log("6");
```
Walk it:
- Sync: `1`, `6`.
- Microtasks drain: `4`. Inside `4`, schedules timer `5`.
- Macrotask: timer fires → `2`. Schedules micro for `3`.
- Microtasks drain: `3`.
- Macrotask: timer for `5` fires → `5`.

Output: `1 6 4 2 3 5`.

### Example 2 — `async`/`await` is just promises
```js
async function f() {
  console.log("a");
  await null;       // yields here
  console.log("b");
}
f();
console.log("c");
// a, c, b
```

`await null` is equivalent to `await Promise.resolve(null)`. The continuation (`console.log("b")`) is scheduled as a microtask. So `c` (sync) runs before `b`.

### Example 3 — long-running sync blocks everything
```js
console.log("start");
setTimeout(() => console.log("timer"), 100);
const start = Date.now();
while (Date.now() - start < 2000) {} // blocks for 2s
console.log("end");
// start, end (after 2s), timer (immediately after, NOT 100ms later)
```
The thread was blocked. Real apps use Web Workers (browser) or `worker_threads` (Node) for CPU-bound work.

### Example 4 — `Promise.all` doesn't change ordering rules
```js
console.log("1");
Promise.all([Promise.resolve("a"), Promise.resolve("b")])
  .then(([a, b]) => console.log(a, b));
console.log("2");
// 1, 2, a b
```

### Example 5 — microtask after every callback in Node
```js
setTimeout(() => {
  console.log("timer");
  Promise.resolve().then(() => console.log("micro inside timer"));
}, 0);
setTimeout(() => console.log("timer 2"), 0);
// timer, micro inside timer, timer 2
```
The microtask scheduled inside the first timer drains before the second timer runs.

### Web Workers / worker_threads — the escape hatch
For CPU work without blocking:
```js
// browser
const worker = new Worker("./crunch.js", { type: "module" });
worker.postMessage({ data: bigArray });
worker.onmessage = (e) => console.log(e.data);
```
```js
// node
import { Worker } from "node:worker_threads";
const worker = new Worker("./crunch.js");
worker.postMessage(bigArray);
worker.on("message", console.log);
```

---

## 5. Common Mistakes & Gotchas

- **Assuming `setTimeout(fn, 0)` runs immediately.** It runs after current sync + all microtasks + any pending I/O of higher priority (in Node).
- **Microtask starvation:**
  ```js
  function spin() { Promise.resolve().then(spin); }
  spin();  // I/O and timers will never fire — microtasks dominate
  ```
  Avoid recursive promise chains with no awaiting of macrotasks.
- **CPU-bound work on the main thread.** Even 50ms blocks user input. Move to a worker.
- **`async` doesn't make code run "in parallel"** — only I/O happens off-thread; your JS still runs on the main thread.
- **Order of `setTimeout(0)` vs `setImmediate` at top level is unspecified** in Node. Inside an I/O callback, `setImmediate` wins.
- **Browsers throttle background tabs** to 1Hz. Long polling via `setInterval` isn't reliable when the tab is hidden.
- **Promise constructor body runs SYNCHRONOUSLY:**
  ```js
  console.log("a");
  new Promise((res) => { console.log("b"); res(); }).then(() => console.log("c"));
  console.log("d");
  // a, b, d, c
  ```
- **Awaiting a non-promise** still costs a microtask:
  ```js
  async function f() {
    console.log(1);
    await 0;        // micro yield
    console.log(2);
  }
  ```

```js
// "Wat"
async function f() {
  console.log("a");
  await Promise.resolve();
  console.log("b");
  await Promise.resolve();
  console.log("c");
}
console.log("0");
f();
console.log("1");
Promise.resolve().then(() => console.log("p"));
console.log("2");
// 0, a, 1, 2, p, b, c
```
Each `await` is a microtask boundary.

---

## 🎯 Key Takeaways

- **One stack, two queues (micro + macro), with microtasks always draining first.** This single rule predicts most "why does this run before that?" puzzles.
- **`await` is a microtask boundary.** Two `await`s = two yields = potentially other code interleaves.
- **Don't block the main thread.** CPU-bound work belongs in `Worker`/`worker_threads`. UI freezes happen at 50ms+; perceptible at 100ms.
- **Browser vs Node differences are real but small:** browser adds rendering between phases; Node has phases and `process.nextTick`/`setImmediate`. The microtask rule is identical.
- **Use the right scheduler for the job:** `requestAnimationFrame` for visuals, `setImmediate`/`MessageChannel` for fast yields, timers for actual delays.

---

*← [10 Async: Promises & async/await](./10_async_promises_await.md) | [next → 12 DOM & Browser APIs](./12_dom_browser_apis.md)*
