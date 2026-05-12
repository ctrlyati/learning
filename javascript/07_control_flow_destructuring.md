# 07 — Control Flow, Destructuring, Spread/Rest, Optional Chaining

> **Goal:** Write expressive, modern JavaScript using the syntactic features that make 2020s code dramatically tighter than 2015 code.

---

## 1. Control Flow — Mental Model

JS has the usual suspects: `if`/`else`, `switch`, `for`, `while`, `do...while`, plus three special loops:

- `for...in` — iterates **enumerable string keys** of an object (incl. inherited). Avoid for arrays.
- `for...of` — iterates **values of any iterable**. Use this for arrays, strings, maps, sets.
- `for await...of` — iterates async iterables (module 10).

```js
// Classic
for (let i = 0; i < 3; i++) console.log(i);

// Iterable values
for (const ch of "hi") console.log(ch);

// Object keys
for (const key of Object.keys(obj)) console.log(key, obj[key]);
for (const [k, v] of Object.entries(obj)) console.log(k, v);
```

### `switch` and the fall-through trap
```js
switch (status) {
  case "ok":
  case "success":         // intentional fall-through (no break)
    handleSuccess();
    break;
  case "error":
    handleError();
    break;
  default:
    log("unknown", status);
}
```
Modern alternative: object/Map dispatch.
```js
const handlers = {
  ok: handleSuccess,
  success: handleSuccess,
  error: handleError,
};
(handlers[status] ?? (() => log("unknown", status)))();
```

### Ternary and short-circuit
```js
const label = isAdmin ? "Admin" : "User";
const name = user?.name || "anonymous";
const port = process.env.PORT ?? 3000;        // ?? only triggers on null/undefined
```

`||` vs `??`:
- `||` returns the right side if left is **falsy** (0, "", false, null, undefined, NaN).
- `??` returns the right side **only** if left is `null`/`undefined`.

```js
const port1 = 0 || 3000;  // 3000  ← bug: 0 is a valid port
const port2 = 0 ?? 3000;  // 0     ← correct: 0 is not nullish
```
**Rule:** use `??` for "default if missing", `||` for "default if not truthy".

### Labels (rare but useful)
```js
outer: for (const row of rows) {
  for (const cell of row) {
    if (cell === target) break outer;
  }
}
```

---

## 2. Destructuring — Under the Hood

Destructuring lets you bind multiple names from an array or object in one expression.

### Array destructuring
```js
const [a, b, c] = [1, 2, 3];
const [, , third] = [1, 2, 3];           // skip with commas
const [head, ...tail] = [1, 2, 3, 4];    // head=1, tail=[2,3,4]
const [x = 10, y = 20] = [5];            // x=5, y=20

// Swap
let p = 1, q = 2;
[p, q] = [q, p]; // p=2, q=1

// Works on any iterable
const [r, g, b] = "RGB"; // "R","G","B"
```

### Object destructuring
```js
const { name, age } = { name: "Ada", age: 36 };

// Rename
const { name: who } = { name: "Ada" }; // who = "Ada"

// Defaults
const { port = 3000 } = config;

// Rename + default
const { name: who = "anon" } = {};

// Rest
const { id, ...rest } = { id: 1, a: 1, b: 2 }; // rest = { a:1, b:2 }

// Nested
const { user: { name, address: { city } } } = response;

// Computed key
const key = "name";
const { [key]: value } = { name: "Ada" }; // value = "Ada"
```

### Function parameters — the killer use case
```js
function createUser({ name, email, age = 18, role = "user" } = {}) {
  // ...
}
createUser({ name: "Ada", email: "a@b.c" });
```
The trailing `= {}` lets you call `createUser()` with no args without a TypeError.

### Destructuring with default + null gotcha
```js
const { x = 10 } = { x: undefined }; // x = 10  (undefined triggers default)
const { y = 10 } = { y: null };      // y = null (null does NOT)
```

---

## 3. Spread, Rest, Optional Chaining, Nullish Coalescing

### Spread `...`
Expands an iterable (in arrays/calls) or object (in objects) into individual elements/properties.

```js
const arr = [1, 2, 3];
const more = [0, ...arr, 4];           // [0,1,2,3,4]
Math.max(...arr);                       // 3
const copy = [...arr];                  // shallow copy

const obj = { a: 1, b: 2 };
const merged = { ...obj, c: 3, b: 99 }; // {a:1, b:99, c:3}  ← later wins
const oCopy = { ...obj };               // shallow copy
```

### Rest `...` (looks the same, opposite role)
Collects remaining items into an array (in destructuring or function params).
```js
function sum(...nums) { return nums.reduce((a, b) => a + b, 0); }
sum(1, 2, 3); // 6

const [first, ...others] = [1, 2, 3, 4]; // first=1, others=[2,3,4]
const { x, ...rest } = obj;
```

Mnemonic: spread is on the right of `=`, rest is on the left.

### Optional chaining `?.`
Short-circuits to `undefined` if the left side is `null` or `undefined`.

```js
const street = user?.address?.street;            // undefined-safe
const first = arr?.[0];                          // optional indexing
const result = maybeFn?.();                      // optional call
const x = api?.user?.profile?.image?.url ?? "fallback.png";
```

Don't overuse — `a?.b?.c?.d?.e` often hides a missing-data bug. Validate at boundaries instead.

### Nullish coalescing `??` (recap)
```js
const value = input ?? defaultValue; // only falls back on null/undefined
```

### Logical assignment operators (ES2021)
```js
a ||= b;   // a = a || b
a &&= b;   // a = a && b
a ??= b;   // a = a ?? b   ← assign only if a is null/undefined

// Common pattern: lazy init
config.cache ??= new Map();
```

### Tagged templates & `String.raw` (handy)
```js
function html(strings, ...values) {
  return strings.reduce(
    (out, s, i) => out + s + (values[i] !== undefined ? escape(values[i]) : ""),
    ""
  );
}
const escape = (s) => String(s).replaceAll("<", "&lt;");
const safe = html`<p>Hi ${userInput}</p>`; // user content auto-escaped
```

---

## 4. Practical Application — Cleaning HTTP Request Handling

A typical Express/Hono-style handler, written cleanly:

```js
async function handleCreatePost(req, res) {
  const {
    body: { title, content, tags = [], publish = false } = {},
    user: { id: authorId } = {},
    headers: { "x-request-id": requestId = crypto.randomUUID() } = {},
  } = req;

  if (!title?.trim() || !content?.trim()) {
    return res.status(400).json({ error: "title and content required", requestId });
  }

  const post = await db.posts.create({
    data: {
      title: title.trim(),
      content,
      tags: [...new Set(tags)],          // dedupe via Set + spread
      authorId,
      publishedAt: publish ? new Date() : null,
    },
  });

  res.status(201).json({ ...post, requestId });
}
```

Notice every modern feature in play:
- Nested destructuring with defaults pulled apart `req`.
- `?.trim()` for null-safe normalization.
- `??` and `||=` would also be reasonable here.
- Spread for object construction and array dedupe.

---

## 5. Common Mistakes & Gotchas

- **`||` instead of `??`** for defaults — bites you with `0`, `""`, `false`.
- **Object destructuring without parens at statement start:**
  ```js
  { a, b } = obj; // SyntaxError — interpreted as block
  ({ a, b } = obj); // OK
  ```
  Doesn't apply if you're declaring (`const { a } = obj` is fine).
- **Default values vs `null`:** defaults only kick in for `undefined`. Pass `null` and you get `null`.
- **Shallow spread:**
  ```js
  const a = { nested: { x: 1 } };
  const b = { ...a };
  b.nested.x = 99;
  console.log(a.nested.x); // 99 — same nested object
  ```
  Use `structuredClone(a)` for a deep copy (built-in, fast, browser+Node).
- **Spreading non-iterables in arrays throws:**
  ```js
  [...123];        // TypeError
  [...{a:1}];      // TypeError — plain objects aren't iterable
  ```
- **`for...in` on arrays** picks up custom prototype properties and goes string-keyed. Use `for...of` or `entries()`.
- **Optional chain short-circuit cascade:**
  ```js
  obj?.a.b.c; // if obj is null/undef, returns undefined; otherwise crashes if `a` is undefined
  obj?.a?.b?.c; // safer
  ```
- **Switch + lexical declarations:**
  ```js
  switch (x) {
    case 1:
      const y = 1; // lives for the whole switch — collisions in other cases
      break;
    case 2:
      const y = 2; // SyntaxError
  }
  ```
  Wrap each case in `{ }` to give it a block.

```js
// "Wat"
const a = [1, 2];
const b = a;
const c = [...a];
b === a;       // true   (same reference)
c === a;       // false  (new array)

const x = { a: 1 };
const { a, b = 2 } = x;   // a=1, b=2
const { a: a, b: b2 = 3 } = x; // a=1, b2=3
```

---

## 🎯 Key Takeaways

- **`??` is for `null`/`undefined`; `||` is for falsy.** Picking the wrong one is a real-world bug source.
- **Destructure function params with defaults + `= {}` fallback.** Single most-used pattern in modern JS APIs.
- **Spread is shallow.** Use `structuredClone` for deep copies; `JSON.parse(JSON.stringify(...))` is a hack with caveats (loses Dates, undefined, etc.).
- **Optional chaining is for unknown shapes** (e.g. external API responses), not for your own data — fix your data model instead.
- **Modern syntax compounds:** `({ user: { id: authorId } = {} } = req)` reads tersely once you know the rules. Practice until it's automatic.

---

*← [06 Arrays & Iteration](./06_arrays_iteration.md) | [next → 08 Modules: ESM vs CJS](./08_modules_esm_cjs.md)*
