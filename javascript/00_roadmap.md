# 00 — JavaScript Deep Dive: Roadmap

> **Goal:** Take a working developer from "I can write JS" to "I understand JS deeply enough to debug, design, and ship production systems with confidence" — and prepare them for a smooth jump into TypeScript.

This is a 17-module deep dive. Each module is self-contained, runnable, and written for **professional upskilling** — not your first programming course. We assume you already write code in *something*; we will sharpen your JavaScript until it is a precision tool.

---

## 📚 Module Table

| #  | File | Title | What you walk away with |
|----|------|-------|--------------------------|
| 01 | [01_setup_and_runtime.md](./01_setup_and_runtime.md) | Setup & Runtimes | Know how JS runs in browser, Node, Bun, Deno; pick the right one |
| 02 | [02_values_types_coercion.md](./02_values_types_coercion.md) | Values, Types, Coercion, Equality | Predict every `==` / `===` / `+` quirk |
| 03 | [03_variables_scope_hoisting.md](./03_variables_scope_hoisting.md) | Variables, Scope, Hoisting, TDZ | Read any JS scope correctly at a glance |
| 04 | [04_functions_closures_this.md](./04_functions_closures_this.md) | Functions, Closures, `this`, Arrows | Master first-class functions and binding |
| 05 | [05_objects_prototypes_classes.md](./05_objects_prototypes_classes.md) | Objects, Prototypes, Classes | Understand the prototype chain, not just `class` syntax |
| 06 | [06_arrays_iteration.md](./06_arrays_iteration.md) | Arrays & Iteration | Use map/filter/reduce/iterators idiomatically |
| 07 | [07_control_flow_destructuring.md](./07_control_flow_destructuring.md) | Control Flow, Destructuring, Spread/Rest, Optional Chaining | Write tight modern JS |
| 08 | [08_modules_esm_cjs.md](./08_modules_esm_cjs.md) | Modules: ESM vs CJS | Navigate the dual-module ecosystem without breaking things |
| 09 | [09_error_handling.md](./09_error_handling.md) | Errors, try/catch, Custom Errors | Build error hierarchies that survive contact with prod |
| 10 | [10_async_promises_await.md](./10_async_promises_await.md) | Async: callbacks → promises → async/await | Reason about async control flow precisely |
| 11 | [11_event_loop.md](./11_event_loop.md) | The Event Loop (Browser & Node) | Predict execution order of any async snippet |
| 12 | [12_dom_browser_apis.md](./12_dom_browser_apis.md) | DOM & Browser APIs | fetch, storage, events, modern web platform |
| 13 | [13_node_essentials.md](./13_node_essentials.md) | Node.js Essentials | fs, path, streams, http, process |
| 14 | [14_npm_package_json_semver.md](./14_npm_package_json_semver.md) | npm, package.json, semver, monorepos | Manage real-world dependency graphs |
| 15 | [15_testing.md](./15_testing.md) | Testing: Vitest/Jest, mocking, e2e | Ship code with a safety net |
| 16 | [16_tooling.md](./16_tooling.md) | Tooling: bundlers, transpilers, linters, formatters | Configure Vite, ESBuild, ESLint, Prettier |
| 17 | [17_modern_patterns_production.md](./17_modern_patterns_production.md) | Production: error tracking, perf, security, TS migration prep | Production-ready patterns |

---

## 🗓 Suggested Timeline

**Pace:** 1 module/day → ~3 weeks. Pad weekends for the heavier ones (10, 11, 13, 16).

| Week | Modules | Focus |
|------|---------|-------|
| 1 | 01–06 | Language fundamentals (clean foundation) |
| 2 | 07–12 | Async, modules, error model, browser/DOM |
| 3 | 13–17 | Node, ecosystem, tooling, production |

If you only have a weekend: do 02, 04, 10, 11, 17 — those are the highest leverage for a working dev.

---

## ✅ Prerequisites

- You can read code in *some* language (Python, Go, Java, C#, anything).
- You have Node.js 20+ installed (`node -v`).
- You have a code editor with a JS-aware extension (VS Code is the path of least resistance).
- A modern browser with devtools (Chrome/Firefox/Edge).
- Optional but useful: a terminal you don't hate.

You do **not** need prior JS — but if you have some, you'll move faster through 01–07.

---

## 🧠 Mental Models to Carry Through

These are the "cheat codes" — internalize them and 90% of JS becomes obvious.

1. **Almost everything is an object-ish thing with a prototype.** Functions, arrays, errors — all objects. Even primitives auto-box. The "weird" inheritance you've heard about is just: every object has a hidden link (`[[Prototype]]`) to another object, and lookups walk that chain.

2. **JS is single-threaded + cooperative + event-loop-driven.** There is one call stack. Anything async (timers, I/O, promises) is queued and runs *after* current synchronous code finishes. If you block the thread, the world freezes.

3. **The prototype chain *is* inheritance.** `class` is sugar over `Function.prototype` machinery. If you understand `Object.create` and `__proto__`, you understand classes for free.

4. **Functions are values.** They can be passed, returned, stored, and they capture their lexical scope (closures). Closures are not magic — they are "the function remembers where it was born."

5. **Async is cooperative, not preemptive.** A promise doesn't run "in the background" — it represents a future value, scheduled onto the microtask queue. `await` is just a suspension point that yields to the event loop.

6. **`this` is determined at call site, not definition.** Except for arrow functions, which capture `this` lexically like any other variable. This one rule resolves 80% of `this` confusion.

---

## 🔗 Canonical References

Bookmark these. They are better than any tutorial including this one.

- **MDN Web Docs** — https://developer.mozilla.org/en-US/docs/Web/JavaScript — the reference. When in doubt, MDN.
- **Node.js Docs** — https://nodejs.org/docs/latest/api/ — for everything server-side.
- **You Don't Know JS (Yet)** by Kyle Simpson — https://github.com/getify/You-Dont-Know-JS — deep, free, opinionated; read at least the *Scope & Closures* and *this & Object Prototypes* books.
- **javascript.info** — https://javascript.info/ — the single best free tutorial; complementary to this course.
- **Node.js Best Practices** — https://github.com/goldbergyoni/nodebestpractices — production wisdom, distilled.
- **TC39 Proposals** — https://github.com/tc39/proposals — see what's coming next in the language.

---

## 🎯 What "Done" Looks Like

After 17 modules you should be able to:
- Read any JS codebase and predict what happens, including async order.
- Debug `this`, `==`, hoisting, and prototype issues without Googling.
- Set up a Vite or Node project from scratch with linting, formatting, tests.
- Diagnose a production bug from a stack trace and a Sentry breadcrumb.
- Decide intelligently when (and when not) to reach for TypeScript.

---

## ➡️ Natural Follow-up: TypeScript

Once you finish module 17, the **TypeScript course** (a planned companion) is the natural next step. You'll find it dramatically easier *because* you understood JS first — TS is a type system layered over the JS runtime, and confused JS makes for confused TS.

---

*Course start → [01 — Setup & Runtimes](./01_setup_and_runtime.md)*
