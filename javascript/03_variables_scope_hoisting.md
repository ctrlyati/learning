# 03 — Variables, Scope, Hoisting, TDZ

> **Goal:** Read any JS file and know which variables are visible where, why, and what happens if you reference one too early.

---

## 1. The Three Declarations — Mental Model

JavaScript has three ways to bind a name to a value: `var`, `let`, `const`. They differ in **scope**, **hoisting**, and **reassignability**.

| | `var` | `let` | `const` |
|---|-------|-------|---------|
| Scope | function | block | block |
| Hoisted | yes — initialized to `undefined` | yes — but in TDZ | yes — but in TDZ |
| Reassign | yes | yes | no |
| Redeclare in same scope | yes | no | no |
| Global → `window` property | yes (browser) | no | no |

```js
function demo() {
  if (true) {
    var a = 1;   // function-scoped → leaks out
    let b = 2;   // block-scoped
    const c = 3; // block-scoped
  }
  console.log(a); // 1
  console.log(b); // ReferenceError
  console.log(c); // ReferenceError
}
demo();
```

**Rule of thumb:** `const` by default. `let` when you must reassign. `var` only in legacy code or when you specifically want function scope (almost never).

`const` makes the **binding** immutable, not the value. Objects are still mutable:
```js
const user = { name: "Ada" };
user.name = "Grace";  // OK — mutating the object
user = {};            // TypeError — rebinding `user`
```
For deep immutability, use `Object.freeze` (shallow) or libraries like `immer`.

---

## 2. Scope — Under the Hood

JS uses **lexical scope**: a variable's scope is determined by where it is *written* in the source, not where it is called.

There are three scope kinds:
1. **Global scope** — top of a script.
2. **Function scope** — inside a `function` (created by `var` or `function` decls).
3. **Block scope** — inside `{ }` (created by `let`/`const`/`class`/`function` in strict mode).

Scopes nest. Inner scopes can read outer ones; outer cannot read inner.

```js
const outer = "I'm global";

function f() {
  const middle = "I'm in f";
  function g() {
    const inner = "I'm in g";
    console.log(outer, middle, inner); // all visible
  }
  g();
  // console.log(inner); // ReferenceError
}
f();
```

This is what powers **closures** (module 04).

### Hoisting — the engine's two-pass model
Before executing a scope, the engine scans for declarations and "hoists" them to the top.

- `function fn() {...}` declarations → hoisted *with their definition*. Callable before written.
- `var x` → hoisted, initialized to `undefined`.
- `let x`, `const x`, `class X` → hoisted but **NOT** initialized. Touching them before the declaration line throws `ReferenceError`. This zone is the **Temporal Dead Zone (TDZ)**.

```js
console.log(varX);  // undefined (hoisted, no error)
console.log(letX);  // ReferenceError — TDZ
console.log(constX);// ReferenceError — TDZ
fn();               // works — function declarations fully hoisted

var varX = 1;
let letX = 2;
const constX = 3;
function fn() { console.log("hi"); }
```

### Function declaration vs function expression
```js
foo(); // works
function foo() { console.log("decl"); }

bar(); // TypeError: bar is not a function — `var bar` is undefined here
var bar = function() { console.log("expr"); };

baz(); // ReferenceError — TDZ
const baz = function() {};
```

---

## 3. Variations & Depth

### Block scope subtleties
```js
{
  let x = 1;
}
console.log(x); // ReferenceError

// `for (let i ...)` creates a NEW binding per iteration. This matters:
for (let i = 0; i < 3; i++) {
  setTimeout(() => console.log(i), 0); // 0, 1, 2
}
for (var i = 0; i < 3; i++) {
  setTimeout(() => console.log(i), 0); // 3, 3, 3 — all see the same `i`
}
```
This single difference is why `let` exists.

### Globals: don't accidentally make them
In **non-strict** mode, assigning to an undeclared name creates a global:
```js
function bug() {
  leak = 42; // implicit global!
}
bug();
console.log(leak); // 42 — congrats, you polluted the global
```
ES Modules and `"use strict"` make this a `ReferenceError`. **Always be in strict mode.** Modules are strict by default.

### `var` and the global object (browser)
```js
// In a classic browser script (not a module):
var foo = 1;
console.log(window.foo); // 1
let bar = 2;
console.log(window.bar); // undefined  ← let/const don't attach to window
```

### Shadowing
Inner scope can re-use a name. The inner one wins for that scope.
```js
const x = "outer";
{
  const x = "inner";
  console.log(x); // "inner"
}
console.log(x); // "outer"
```
ESLint can warn on shadowing if you want.

### `class` declarations are TDZ-ed too
```js
new Foo(); // ReferenceError
class Foo {}
```

---

## 4. Practical Application — Refactoring a `var`-era Snippet

You'll inherit code like this. Here's a realistic before/after.

**Before (legacy):**
```js
function processItems(items) {
  for (var i = 0; i < items.length; i++) {
    var item = items[i];
    setTimeout(function () {
      console.log("processing", item, "index", i);
    }, 100 * i);
  }
}
processItems(["a", "b", "c"]);
// Logs: "processing c index 3" three times — closure over the same i and item.
```

**After (modern):**
```js
function processItems(items) {
  for (const [i, item] of items.entries()) {
    setTimeout(() => {
      console.log("processing", item, "index", i);
    }, 100 * i);
  }
}
processItems(["a", "b", "c"]);
// "processing a index 0", "processing b index 1", "processing c index 2"
```

Three things changed:
1. `var` → `const` (no reassignment of `i` / `item` per iteration; each iteration is its own binding).
2. `function () { ... }` → arrow. Lexical `this`, less typing.
3. Manual `items[i]` → `entries()` destructuring.

A small TDZ-safe utility you'll write often:
```js
function loadConfig(env) {
  const required = ["DATABASE_URL", "API_KEY"];
  const missing = required.filter((k) => !env[k]);
  if (missing.length) {
    throw new Error(`Missing required env vars: ${missing.join(", ")}`);
  }
  // Frozen so callers can't mutate it
  return Object.freeze({
    databaseUrl: env.DATABASE_URL,
    apiKey: env.API_KEY,
    port: Number(env.PORT) || 3000,
  });
}

const config = loadConfig(process.env);
```

---

## 5. Common Mistakes & Gotchas

- **`const` does not freeze the object.** Common interview gotcha. `const a = []; a.push(1);` is fine.
- **Loop variable in async callbacks.** Classic `var i` bug above. Use `let` or `for...of`.
- **Forgetting `let`/`const` entirely:** `x = 5` in non-strict mode creates a global. Always be in modules or strict mode.
- **TDZ surprises with destructuring:**
  ```js
  const { x = 1 } = { x: undefined }; // x = 1 — undefined triggers default
  const { y = 1 } = { y: null };      // y = null — null does NOT
  ```
- **Mixing `var` and `let` for the same name in nested scopes:**
  ```js
  let x = 1;
  { var x = 2; } // SyntaxError — var hoisting collides with let in outer scope
  ```
- **`typeof` is the *only* operator that doesn't throw on undeclared names:**
  ```js
  typeof neverDeclared; // "undefined" — safe
  neverDeclared;        // ReferenceError
  ```
  This is occasionally used for feature-detection: `typeof window !== "undefined"`.
- **Function declarations inside blocks** behave differently in strict vs sloppy mode and can vary between engines. Don't do it; assign to a `const` instead.

```js
// "Wat"
let a;
console.log(a);     // undefined — declared but unassigned ≠ TDZ
console.log(typeof undeclared); // "undefined" — no error
console.log(undeclared);        // ReferenceError
```

---

## 🎯 Key Takeaways

- **`const` by default, `let` when you must reassign, `var` essentially never.** This single rule eliminates a class of bugs.
- **Scope is lexical** — determined by where code is written. Memorize the three scope kinds: global, function, block.
- **TDZ is a feature.** It exists so you can't read a `let`/`const` before its declaration. Embrace the early errors.
- **`for (let i ...)` creates a fresh binding per iteration.** `var` does not. This makes `let` correct for async loops by default.
- **Always run in strict mode** (modules are strict automatically). Implicit globals are a vulnerability.

---

*← [02 Values, Types, Coercion](./02_values_types_coercion.md) | [next → 04 Functions, Closures, `this`, Arrows](./04_functions_closures_this.md)*
