# 09 — Error Handling, try/catch, Custom Errors

> **Goal:** Build a robust error model — typed custom errors, correct propagation, async-safe handling — so production stack traces tell you what's wrong, not just where.

---

## 1. Errors Are Values — Mental Model

In JS, an error is just an object — almost always an instance of `Error` or one of its subclasses. You **throw** an error and **catch** it elsewhere on the call stack.

```js
function divide(a, b) {
  if (b === 0) throw new Error("Cannot divide by zero");
  return a / b;
}

try {
  divide(10, 0);
} catch (err) {
  console.error(err.message);   // "Cannot divide by zero"
  console.error(err.name);      // "Error"
  console.error(err.stack);     // multi-line stack trace
}
```

You can throw *any* value — `throw 42`, `throw "oops"`, `throw { code: 500 }` — but you really shouldn't. **Always throw `Error` instances.** Tools, frameworks, and humans expect `.message`, `.stack`, `.name`.

### Built-in error subclasses
- `TypeError` — wrong type (calling non-function, reading prop of `null`)
- `RangeError` — value out of range (`new Array(-1)`)
- `SyntaxError` — invalid code (caught by `JSON.parse`, `eval`)
- `ReferenceError` — undeclared identifier
- `URIError` — bad URI in `decodeURI`/`encodeURI`
- `AggregateError` — multiple errors (from `Promise.any`)

```js
JSON.parse("{ broken");        // throws SyntaxError
null.foo;                      // throws TypeError
new Array(-1);                 // throws RangeError
```

---

## 2. try/catch/finally — Under the Hood

```js
try {
  doRiskyThing();
} catch (err) {
  // runs only if try block throws
  log(err);
} finally {
  // runs ALWAYS — success, throw, or even return inside try
  cleanup();
}
```

`finally` runs even if `try` or `catch` `return`s or `throw`s. Use it for cleanup (closing files, releasing locks).

```js
function readConfig() {
  const handle = openFile("config.json");
  try {
    return parse(handle.read());
  } finally {
    handle.close(); // always
  }
}
```

### Bare catch (ES2019)
You can omit the parameter if you don't need it:
```js
try { JSON.parse(input); } catch { return null; }
```

### Re-throwing and wrapping
```js
try {
  await db.query(sql);
} catch (err) {
  // Add context, preserve original via `cause` (ES2022)
  throw new DatabaseError("Failed to fetch user", { cause: err });
}
```

`Error` accepts `{ cause }` since ES2022 and Node 16.9+. Inspecting:
```js
catch (err) {
  console.error(err.message);
  if (err.cause) console.error("Caused by:", err.cause);
}
```

### Custom error classes
```js
class AppError extends Error {
  constructor(message, { cause, code, statusCode = 500 } = {}) {
    super(message, { cause });
    this.name = this.constructor.name;
    this.code = code;
    this.statusCode = statusCode;
    // V8-specific: trim the constructor frame from the stack
    if (Error.captureStackTrace) Error.captureStackTrace(this, this.constructor);
  }
}

class ValidationError extends AppError {
  constructor(message, opts = {}) {
    super(message, { ...opts, statusCode: 400, code: "VALIDATION_ERROR" });
  }
}

class NotFoundError extends AppError {
  constructor(resource, id) {
    super(`${resource} ${id} not found`, { statusCode: 404, code: "NOT_FOUND" });
  }
}
```

Discriminate cleanly:
```js
try { await getUser(id); }
catch (err) {
  if (err instanceof NotFoundError) return res.status(404).json({ error: err.message });
  if (err instanceof ValidationError) return res.status(400).json({ error: err.message });
  throw err; // unknown → propagate
}
```

---

## 3. Async Error Handling

### Promises
```js
fetchUser(id)
  .then(handleUser)
  .catch(handleError);
```
`.catch` catches *any* rejection earlier in the chain.

### `async/await`
```js
try {
  const user = await fetchUser(id);
  handleUser(user);
} catch (err) {
  handleError(err);
}
```

Same try/catch syntax — much nicer.

### Throwing inside async functions
```js
async function f() { throw new Error("nope"); }
// Equivalent to:
async function f2() { return Promise.reject(new Error("nope")); }
```
Always becomes a rejected promise.

### Unhandled rejections
A promise that rejects with no `.catch` triggers `unhandledrejection` (browser) or `process.on('unhandledRejection')` (Node). In modern Node it crashes the process by default — that is **the right behavior**. Log it, then let the supervisor restart you.

```js
// Top of your entry point — last-resort handlers
process.on("unhandledRejection", (reason) => {
  console.error("UNHANDLED REJECTION:", reason);
  process.exit(1);
});
process.on("uncaughtException", (err) => {
  console.error("UNCAUGHT EXCEPTION:", err);
  process.exit(1);
});
```

### `Promise.allSettled` and `AggregateError`
```js
const results = await Promise.allSettled([job1(), job2(), job3()]);
const errors = results.filter((r) => r.status === "rejected").map((r) => r.reason);
if (errors.length) throw new AggregateError(errors, "Some jobs failed");
```

### Timers and event handlers — error sinkholes
```js
setTimeout(() => { throw new Error("lost"); }, 0);
// In Node, becomes uncaughtException. In browser, hits window.onerror.
// CANNOT be caught by surrounding try/catch.
```

Wrap risky callbacks:
```js
function safe(fn) {
  return (...args) => {
    try { return fn(...args); }
    catch (err) { reportError(err); }
  };
}
button.addEventListener("click", safe(handleClick));
```

---

## 4. Practical Application — Production-Grade Error Layer

A small but realistic Node-style server-error toolkit:

```js
// errors.js
export class AppError extends Error {
  constructor(message, { cause, code, statusCode = 500, expose = false } = {}) {
    super(message, { cause });
    this.name = this.constructor.name;
    this.code = code ?? "INTERNAL";
    this.statusCode = statusCode;
    this.expose = expose; // safe to send back to client?
    if (Error.captureStackTrace) Error.captureStackTrace(this, this.constructor);
  }
}

export class ValidationError extends AppError {
  constructor(message, fields) {
    super(message, { code: "VALIDATION_ERROR", statusCode: 400, expose: true });
    this.fields = fields;
  }
}

export class NotFoundError extends AppError {
  constructor(resource, id) {
    super(`${resource} ${id} not found`, { code: "NOT_FOUND", statusCode: 404, expose: true });
  }
}

export class UpstreamError extends AppError {
  constructor(service, cause) {
    super(`Upstream service '${service}' failed`, {
      cause,
      code: "UPSTREAM_ERROR",
      statusCode: 502,
      expose: false,
    });
    this.service = service;
  }
}
```

```js
// middleware/errorHandler.js
import { AppError } from "./errors.js";

export function errorHandler(err, req, res, _next) {
  // Always log
  req.log.error({ err, requestId: req.id }, err.message);

  // Known operational error
  if (err instanceof AppError) {
    return res.status(err.statusCode).json({
      error: {
        code: err.code,
        message: err.expose ? err.message : "Internal server error",
        ...(err.fields ? { fields: err.fields } : {}),
        requestId: req.id,
      },
    });
  }

  // Unknown / programmer error: do NOT leak internals
  return res.status(500).json({
    error: { code: "INTERNAL", message: "Internal server error", requestId: req.id },
  });
}
```

```js
// usage
import { NotFoundError, ValidationError, UpstreamError } from "./errors.js";

async function getUserHandler(req, res) {
  if (!req.params.id) throw new ValidationError("Missing id", { id: "required" });

  let user;
  try { user = await db.users.findById(req.params.id); }
  catch (err) { throw new UpstreamError("postgres", err); }

  if (!user) throw new NotFoundError("User", req.params.id);

  res.json(user);
}
```

Patterns demonstrated:
- Distinguish **operational** (expected) errors from **programmer** errors.
- `expose` flag controls leakage — never echo unknown internals to clients.
- `cause` preserves the original error for logs.
- Stable `code` string for clients to switch on (`if (err.code === "NOT_FOUND")`).
- Stack trace cleaned of constructor frame.

---

## 5. Common Mistakes & Gotchas

- **Throwing strings:** `throw "something"`. No stack, no `.message`. Always `throw new Error(...)`.
- **`catch` swallowing without logging:**
  ```js
  try { doIt(); } catch {} // silent failure — debugging nightmare
  ```
  At minimum log it; ideally re-throw or convert to a known type.
- **Catching too broadly:**
  ```js
  try { JSON.parse(x); } catch (e) { /* ignore */ } // also catches bugs unrelated to JSON
  ```
  Catch only what you can handle; rethrow the rest.
- **Async errors lost in `forEach`:**
  ```js
  arr.forEach(async (x) => { throw new Error("nope"); }); // never caught
  ```
- **`try/catch` doesn't catch async callbacks:**
  ```js
  try { setTimeout(() => { throw new Error("lost"); }, 0); }
  catch (e) { /* never runs — handler is on a separate tick */ }
  ```
- **Using `instanceof` across realms / module copies:** can be false even for the "same" class in iframes or duplicate module instances. Prefer a `code` property check for cross-context discrimination.
- **Resource leaks without `finally`:** opened a DB connection, threw, never closed. Always pair acquire with finally-release.
- **Throwing in a `finally` block** silently overrides the original error. Don't.
- **`Promise.all` short-circuits on first rejection.** Other in-flight promises keep running but their results are discarded. Use `Promise.allSettled` if you need every result.

```js
// "Wat"
try { throw 42; } catch (e) { console.log(typeof e); } // "number" — yes, you can throw anything

const p = new Promise((_, reject) => reject(new Error("x")));
// No await, no .catch — Node will warn AND eventually crash (modern default).
```

---

## 🎯 Key Takeaways

- **Always throw `Error` instances** with descriptive messages. Subclass `Error` for your domain so callers can `instanceof`-check.
- **Use `cause`** (ES2022) to wrap errors with context without losing the original stack.
- **`async/await` + `try/catch` is the cleanest async error model.** Avoid mixing `.then().catch()` and `await` in the same function.
- **Distinguish operational vs programmer errors.** Operational ones get user-friendly handling; programmer errors should crash so a supervisor restarts a clean process.
- **Never let errors leak unfiltered to clients.** Use an `expose` flag or whitelist; prevent stack traces and internal codes from reaching production responses.

---

*← [08 Modules: ESM vs CJS](./08_modules_esm_cjs.md) | [next → 10 Async: callbacks, promises, async/await](./10_async_promises_await.md)*
