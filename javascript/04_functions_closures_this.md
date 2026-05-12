# 04 — Functions, Closures, `this`, Arrows

> **Goal:** Master the most important value type in JavaScript — the function — including closures, the four binding rules for `this`, and when to reach for arrow functions.

---

## 1. Functions Are Values — Mental Model

In JS, functions are first-class objects. You can:
- Pass them as arguments
- Return them from other functions
- Store them in variables, arrays, object properties
- Attach properties to them (`fn.foo = 1`)

This is the foundation of callbacks, higher-order functions, and event-driven code.

```js
// Five equivalent ways to make a function
function f1(x) { return x * 2; }                 // function declaration
const f2 = function(x) { return x * 2; };        // function expression
const f3 = function named(x) { return x * 2; };  // named function expression
const f4 = (x) => x * 2;                         // arrow
const f5 = new Function("x", "return x * 2");    // Function constructor (avoid)

[f1, f2, f3, f4, f5].forEach((fn) => console.log(fn(21))); // 42 x5
```

### Higher-order functions
```js
function repeat(n, fn) {
  for (let i = 0; i < n; i++) fn(i);
}
repeat(3, (i) => console.log("hi", i));

// Returns a function (closure!)
function multiplier(by) {
  return (x) => x * by;
}
const triple = multiplier(3);
console.log(triple(10)); // 30
```

---

## 2. Closures — Under the Hood

**A closure is a function bundled with the lexical environment in which it was defined.** When the function executes, it can read (and write) variables from that environment, even after the outer function has returned.

```js
function makeCounter() {
  let count = 0;
  return {
    inc: () => ++count,
    get: () => count,
  };
}
const c = makeCounter();
c.inc(); c.inc(); c.inc();
console.log(c.get()); // 3 — `count` is "alive" because the inner functions still reference it
```

`count` would normally die when `makeCounter` returns, but the returned functions hold references to its environment. The garbage collector keeps that environment alive as long as those functions are reachable.

### Why closures matter in production
- **Encapsulation / private state** (above)
- **Memoization**
- **Currying / partial application**
- **Event handlers that need to remember setup data**
- **Module pattern** (pre-ESM)

```js
// Memoization
function memoize(fn) {
  const cache = new Map();
  return (arg) => {
    if (!cache.has(arg)) cache.set(arg, fn(arg));
    return cache.get(arg);
  };
}
const slowSquare = (n) => { for (let i = 0; i < 1e7; i++); return n * n; };
const fastSquare = memoize(slowSquare);
console.time("first");  fastSquare(7); console.timeEnd("first");
console.time("second"); fastSquare(7); console.timeEnd("second"); // ~0ms
```

```js
// Currying
const curry = (fn) => (a) => (b) => fn(a, b);
const add = curry((a, b) => a + b);
const add10 = add(10);
console.log(add10(5)); // 15
```

### Closures keep references, not copies
```js
let x = 1;
const read = () => x;
x = 99;
console.log(read()); // 99 — closure sees current value
```

---

## 3. `this` — The Four Binding Rules

`this` is a **dynamic** binding determined at *call time*, not definition time. Four rules, in priority order:

### 3.1 `new` binding (highest priority)
```js
function Person(name) { this.name = name; }
const p = new Person("Ada");
// `new` creates a fresh object, sets it as `this`, returns it.
console.log(p.name); // "Ada"
```

### 3.2 Explicit binding — `call`, `apply`, `bind`
```js
function greet(greeting) { return `${greeting}, ${this.name}`; }
const ada = { name: "Ada" };
greet.call(ada, "Hi");        // "Hi, Ada"
greet.apply(ada, ["Hello"]);  // "Hello, Ada"
const greetAda = greet.bind(ada);
greetAda("Hey");              // "Hey, Ada"
```
`call` takes args individually; `apply` takes them as an array; `bind` returns a new function with `this` permanently set.

### 3.3 Implicit (method) binding
```js
const obj = {
  name: "Grace",
  greet() { return `Hi, ${this.name}`; },
};
obj.greet(); // "Hi, Grace" — `this` is the object before the dot
```

### 3.4 Default binding
```js
function loose() { return this; }
loose();
// In strict mode (and modules): undefined
// In sloppy mode: the global object (window/globalThis)
```

### Losing `this` — the classic bug
```js
const ada = { name: "Ada", greet() { return `Hi ${this.name}`; } };
const fn = ada.greet;
fn(); // "Hi undefined" — calling without the dot loses `this`

// Same bug in callbacks:
setTimeout(ada.greet, 100); // "Hi undefined"
// Fix:
setTimeout(() => ada.greet(), 100);     // arrow keeps the call expression
setTimeout(ada.greet.bind(ada), 100);   // explicit bind
```

---

## 4. Arrow Functions — The Big Difference

Arrows are NOT just shorter `function`s. They differ in:

1. **No own `this`** — they capture `this` lexically (from the enclosing scope).
2. **No own `arguments`** object.
3. **Cannot be used with `new`.**
4. **No `prototype` property.**
5. Concise body: `(x) => expr` returns `expr` automatically.

### When to use arrows
- Callbacks where you want to inherit `this` from outside.
- Anywhere terseness wins (`map`, `filter`, `reduce`).

### When NOT to use arrows
- **Object methods** that need `this` to refer to the object:
  ```js
  const obj = {
    name: "Ada",
    greet: () => `Hi ${this.name}`,    // `this` is the OUTER this, not obj
    greet2() { return `Hi ${this.name}`; }, // ← do this
  };
  ```
- **Constructors:** `new ArrowFn()` throws.
- **Prototype methods on classes** are usually fine to use the regular method shorthand.

### A subtle but practical example
```js
class Counter {
  constructor() {
    this.count = 0;
  }
  // Method shorthand — `this` works, but loses binding when detached
  startBroken() { setInterval(this.tick, 1000); }
  // Arrow class field — `this` permanently bound to instance
  startWorking() { setInterval(this.tickArrow, 1000); }

  tick() { console.log("count", ++this.count); }     // breaks when detached
  tickArrow = () => { console.log("count", ++this.count); }; // safe
}
```

This pattern (arrow class fields for callbacks) is very common in React and other frameworks.

---

## 4b. Practical Application — Building an Event Emitter

Closures + first-class functions = a tiny event emitter, no classes needed.

```js
function createEmitter() {
  const listeners = new Map(); // event → Set of handlers

  return {
    on(event, handler) {
      if (!listeners.has(event)) listeners.set(event, new Set());
      listeners.get(event).add(handler);
      return () => this.off(event, handler); // returns unsubscribe fn
    },
    off(event, handler) {
      listeners.get(event)?.delete(handler);
    },
    emit(event, ...args) {
      const set = listeners.get(event);
      if (!set) return;
      for (const h of set) {
        try { h(...args); }
        catch (err) { console.error(`Handler for "${event}" threw:`, err); }
      }
    },
  };
}

const bus = createEmitter();
const off = bus.on("user:login", (user) => console.log("Welcome", user.name));
bus.emit("user:login", { name: "Ada" }); // Welcome Ada
off();
bus.emit("user:login", { name: "Ada" }); // (silent)
```

Notice:
- `listeners` is private — only accessible via the returned methods. That's a closure giving you encapsulation without `class` syntax.
- `on` returns an unsubscribe closure. Idiomatic in modern JS (RxJS, Redux, React `useEffect`).

---

## 5. Common Mistakes & Gotchas

- **Arrow methods on object literals** lose `this`. Use method shorthand instead.
- **Detached methods.** `const fn = obj.method; fn();` — `this` becomes undefined.
- **Forgetting `new`.** Calling a constructor without `new` makes `this` the global (or throws in strict mode), and you'll mutate the wrong thing.
- **Closures retaining memory.** A closure keeps its entire scope alive. Capturing a huge object you don't need can prevent GC. Be intentional about what you close over.
  ```js
  function leaky() {
    const huge = new Array(1e6).fill("data");
    return () => "hi";  // closure unnecessarily keeps `huge` alive
  }
  ```
- **Returning from `forEach` doesn't break the loop.** Use `for...of`, `.some()`, or `.find()`.
- **`bind` creates a new function every call.** In React render paths, this can cause needless re-renders. Bind once or use class fields.
- **Implicit return + braces:** `(x) => { x }` returns `undefined`. The `{}` is a body, not an object literal. Wrap object literals in parens: `(x) => ({ x })`.
- **`arguments` doesn't exist in arrows.** Use rest params: `(...args) => args.reduce(...)`.

```js
// "Wat"
const fn = () => {};
fn.prototype;          // undefined — arrows have none
new fn();              // TypeError

const obj = {
  arr: [1, 2, 3],
  doubleAll: function () {
    return this.arr.map(function (x) { return x * this.arr.length; }); // `this` is undefined inside the inner function!
  },
  doubleAllFixed: function () {
    return this.arr.map((x) => x * this.arr.length); // arrow inherits `this`
  },
};
```

---

## 🎯 Key Takeaways

- **Functions are values.** Pass, return, store, attach. This unlocks callbacks, HOFs, and the entire async API surface.
- **Closures = function + its birth scope.** Use them for private state, memoization, and unsubscribe handles.
- **`this` is decided at the call site,** in priority order: `new` > explicit (`call/apply/bind`) > implicit (method) > default. Detaching a method drops `this`.
- **Arrows capture `this` lexically** and have no `arguments`, no `prototype`, no `new`. They are not just "shorter functions."
- **Bind once, not every render.** In hot paths and React components, repeatedly creating bound or arrow functions can hurt perf and break memoization.

---

*← [03 Variables, Scope, Hoisting](./03_variables_scope_hoisting.md) | [next → 05 Objects, Prototypes, Classes](./05_objects_prototypes_classes.md)*
