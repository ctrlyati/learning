# 10 — Asynchronous JavaScript: Callbacks, Promises, async/await, Microtasks

> **Goal:** Reason precisely about asynchronous control flow — when a callback runs, when a promise resolves, when `await` resumes — and avoid the classic concurrency bugs.

---

## 1. The Three Eras of Async — Mental Model

JS is single-threaded. Long-running I/O can't block. So the language has evolved three solutions:

1. **Callbacks** — pass a function to be called later.
2. **Promises** — an object that represents a future value.
3. **async/await** — syntax sugar over promises that reads like sync code.

```js
// 1. Callbacks (Node "errback" style)
fs.readFile("a.txt", "utf8", (err, data) => {
  if (err) return done(err);
  fs.readFile("b.txt", "utf8", (err2, data2) => {
    if (err2) return done(err2);
    done(null, data + data2);
  });
});

// 2. Promises
fs.promises.readFile("a.txt", "utf8")
  .then(a => fs.promises.readFile("b.txt", "utf8").then(b => a + b))
  .then(combined => console.log(combined))
  .catch(console.error);

// 3. async/await
const a = await fs.promises.readFile("a.txt", "utf8");
const b = await fs.promises.readFile("b.txt", "utf8");
console.log(a + b);
```

Same logic, three styles. New code = async/await with the occasional `.then` for chains.

---

## 2. Promises — Under the Hood

A Promise is in one of three states:
- **pending** — not settled
- **fulfilled** — resolved with a value
- **rejected** — failed with a reason

Once settled, it stays settled forever. Settling is irreversible.

### Construction
```js
const p = new Promise((resolve, reject) => {
  setTimeout(() => resolve(42), 100);
});

p.then(
  (value) => console.log("ok", value),
  (err) => console.log("err", err),
);
```

You almost never `new Promise` in app code. Use the API that returns one (`fetch`, `fs.promises.*`, etc.). Manual construction is needed only when adapting old callback APIs.

### Promisification
```js
import { promisify } from "node:util";
const readFile = promisify(require("fs").readFile);
await readFile("a.txt", "utf8");
```

### Chaining and value transformation
`.then` returns a new Promise. The callback's return value becomes the new promise's value:

```js
fetch("/user")
  .then(res => res.json())          // returns a promise; .then unwraps it
  .then(user => user.name)
  .then(name => console.log(name));
```

Throw inside `.then` → next `.catch` handles it.

### `async` functions
- An `async` function always returns a Promise.
- `return x` inside it = `Promise.resolve(x)`.
- `throw e` = `Promise.reject(e)`.
- `await p` pauses the function; resumes when `p` settles.

```js
async function getName() {
  const res = await fetch("/user");
  if (!res.ok) throw new Error("HTTP " + res.status);
  return (await res.json()).name;
}
```

### Combinators
```js
Promise.all([p1, p2, p3])         // all fulfill → array; first reject → reject
Promise.allSettled([p1, p2, p3])  // wait all; array of {status, value/reason}
Promise.race([p1, p2, p3])        // first to settle (fulfill or reject) wins
Promise.any([p1, p2, p3])         // first FULFILLED wins; all reject → AggregateError
```

```js
// Common: "fetch with timeout"
function fetchTimeout(url, ms) {
  return Promise.race([
    fetch(url),
    new Promise((_, rej) => setTimeout(() => rej(new Error("timeout")), ms)),
  ]);
}
// Better in modern code: AbortController (below).
```

### `AbortController` — proper cancellation
```js
const ctrl = new AbortController();
const timer = setTimeout(() => ctrl.abort(), 5000);

try {
  const res = await fetch(url, { signal: ctrl.signal });
  // ...
} finally {
  clearTimeout(timer);
}
```

Modern Node `fs.promises`, timers, and many libraries accept `signal`. ES2024 has `AbortSignal.timeout(ms)` which is cleaner:
```js
const res = await fetch(url, { signal: AbortSignal.timeout(5000) });
```

---

## 3. Microtasks vs Macrotasks (preview of module 11)

Two queues of pending work feed the event loop:

- **Microtask queue:** promise callbacks (`.then`, `await` continuations), `queueMicrotask`, `MutationObserver`. Drained **completely** between every macrotask.
- **Macrotask queue:** `setTimeout`, `setInterval`, `setImmediate` (Node), I/O callbacks, UI events.

Result: promise callbacks always run before the next timer fires.

```js
console.log("1");
setTimeout(() => console.log("2"), 0);
Promise.resolve().then(() => console.log("3"));
console.log("4");
// Output: 1, 4, 3, 2
```

Walkthrough: synchronous "1" and "4" run; the promise callback "3" is a microtask, drained immediately after the current sync code; only then does the timer "2" fire.

We dive deeper in module 11.

---

## 4. Practical Application — Concurrent, Cancellable, Limited Workers

A realistic helper: process N items concurrently with a limit, with cancellation.

```js
async function mapWithConcurrency(items, mapper, { concurrency = 5, signal } = {}) {
  const results = new Array(items.length);
  let next = 0;
  async function worker() {
    while (next < items.length) {
      if (signal?.aborted) throw new DOMException("aborted", "AbortError");
      const i = next++;
      results[i] = await mapper(items[i], i);
    }
  }
  const workers = Array.from({ length: Math.min(concurrency, items.length) }, worker);
  await Promise.all(workers);
  return results;
}

// Use it
const urls = ["/a", "/b", "/c", "/d", "/e", "/f"];
const ctrl = new AbortController();

const data = await mapWithConcurrency(
  urls,
  async (u) => (await fetch(u, { signal: ctrl.signal })).json(),
  { concurrency: 3, signal: ctrl.signal },
);
```

You'll want this exact pattern any time you have to call an API for many items.

### Sequential vs parallel — pick consciously
```js
// Sequential (slow but ordered, lower load)
for (const u of urls) await fetch(u);

// Parallel (fast, but unbounded — can overload upstream)
await Promise.all(urls.map((u) => fetch(u)));

// Limited parallel (Goldilocks zone — what real systems use)
await mapWithConcurrency(urls, fetch, { concurrency: 5 });
```

### `for await...of` — async iteration
```js
async function* lines(stream) {
  let buf = "";
  for await (const chunk of stream) {
    buf += chunk;
    let idx;
    while ((idx = buf.indexOf("\n")) >= 0) {
      yield buf.slice(0, idx);
      buf = buf.slice(idx + 1);
    }
  }
  if (buf) yield buf;
}

import { createReadStream } from "node:fs";
for await (const line of lines(createReadStream("./big.txt", { encoding: "utf8" }))) {
  process(line);
}
```

---

## 5. Common Mistakes & Gotchas

- **Forgetting `await`:**
  ```js
  async function f() { /* ... */ }
  f(); // returns a Promise; if it rejects, you get an unhandledRejection
  ```
  Lint rule: `no-floating-promises`.
- **Awaiting in a loop when you meant parallel:**
  ```js
  for (const u of urls) await fetch(u); // serial
  await Promise.all(urls.map((u) => fetch(u))); // parallel
  ```
- **`forEach(async ...)` not awaited.** `forEach` doesn't await. Use `for...of` or `Promise.all(map)`.
- **Mixing `.then` and `await` confusingly:**
  ```js
  await fetch("/x").then(handle); // works but reads weird; pick one style
  ```
- **Returning vs awaiting in catch:**
  ```js
  async function f() {
    try { return await risky(); }   // ✓ catches risky's rejection here
    catch (e) { handle(e); }
  }
  async function g() {
    try { return risky(); }         // ✗ promise leaks past catch
    catch (e) { /* never runs */ }
  }
  ```
  **Always `return await` inside try if you want the catch to fire.**
- **`new Promise` anti-pattern:** wrapping an existing promise.
  ```js
  // Don't:
  return new Promise((resolve, reject) => {
    fetch(url).then(resolve, reject);
  });
  // Just:
  return fetch(url);
  ```
- **Unhandled rejection because promise was created but never `.then`-ed/`await`-ed.** Fire-and-forget needs `.catch(handle)`.
- **`Promise.all` short-circuits.** Other promises continue running but their values are discarded. If those promises hold resources, leak risk.
- **`async` constructors don't exist.** Use a static factory: `class X { static async create() { ... } }`.
- **Microtask starvation:** an infinite chain of `.then(() => Promise.resolve().then(...))` will block timers and I/O forever. Rare but possible.

```js
// "Wat"
async function f() { return 1; }
f() instanceof Promise; // true

await Promise.resolve(Promise.resolve(42)); // 42 — promises auto-flatten
await Promise.resolve({ then: r => r(1) }); // 1 — "thenables" are accepted

// Returning vs returning-await — error propagation difference
async function a() { try { return doFail(); } catch (e) { return "handled"; } }       // doesn't catch
async function b() { try { return await doFail(); } catch (e) { return "handled"; } } // does catch
```

---

## 🎯 Key Takeaways

- **`async/await` is the default** for new code. Drop `.then` chains except for tight transformations.
- **Pick concurrency consciously** — sequential, fully parallel, or limited (use a helper). Default `Promise.all` with no limit is how you DDoS your own backend.
- **Use `AbortController` for cancellation** — `fetch`, `setTimeout`, `fs.promises`, and most modern APIs accept `signal`.
- **Inside `try`, prefer `return await` over `return`** if you want the surrounding catch to fire.
- **Never silently fire-and-forget a promise.** Every async call either gets `await`ed, returned, or `.catch`-handled. Lint enforces this in real codebases.

---

*← [09 Error Handling](./09_error_handling.md) | [next → 11 The Event Loop in Depth](./11_event_loop.md)*
