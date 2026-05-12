# 08 — Modules: ESM vs CommonJS, Dynamic Imports

> **Goal:** Understand the two competing module systems in JavaScript, when each is used, how they interop, and how dynamic imports unlock code splitting.

---

## 1. Modules — Mental Model

A *module* is a file that explicitly states what it exports (provides) and what it imports (depends on). In JS, two systems coexist:

- **CommonJS (CJS)** — Node's original; synchronous; `require` / `module.exports`. Files are wrapped in a function at runtime.
- **ECMAScript Modules (ESM)** — the official standard; static; `import` / `export`. Strict mode by default; top-level `await` allowed.

Browsers only support ESM. Node supports both. Bun and Deno are ESM-first.

```js
// CommonJS — older Node
const fs = require("node:fs");
function read(p) { return fs.readFileSync(p, "utf8"); }
module.exports = { read };
module.exports.read = read;
// or: exports.read = read;
```

```js
// ESM — modern everywhere
import fs from "node:fs";
export function read(p) { return fs.readFileSync(p, "utf8"); }
export default function () {}
```

---

## 2. ESM Mechanics — Under the Hood

### Telling Node which mode you're in
- `package.json` with `"type": "module"` → all `.js` files are ESM.
- `package.json` without it (or `"type": "commonjs"`) → all `.js` are CJS.
- File extensions override: `.mjs` is always ESM; `.cjs` is always CJS.

```json
// package.json
{
  "type": "module",
  "main": "./src/index.js"
}
```

### Export forms
```js
// Named exports
export const PI = 3.14;
export function hello() {}
export class Box {}

// Default export (one per module)
export default function main() {}

// Aggregate / re-export
export { foo, bar } from "./other.js";
export * from "./other.js";
```

### Import forms
```js
import main from "./main.js";                 // default
import { foo, bar } from "./util.js";          // named
import { foo as f } from "./util.js";          // rename
import * as util from "./util.js";             // namespace
import "./side-effects.js";                    // pure side-effect import
```

### Static analysis
ESM imports are **statically analyzed** at parse time. You cannot conditionally `import` at the top level:

```js
// SYNTAX ERROR
if (cond) import "./mod.js";

// Use dynamic import instead
if (cond) await import("./mod.js");
```

This staticness enables:
- **Tree-shaking** — bundlers drop unused exports.
- **Top-level `await`** that pauses the module graph correctly.
- **Cyclic imports** that work because bindings are *live*.

### Live bindings
ESM imports are *bindings*, not value copies. If the exporter changes the value, importers see the new value.

```js
// counter.js
export let count = 0;
export function inc() { count++; }
```
```js
// main.js
import { count, inc } from "./counter.js";
console.log(count); // 0
inc();
console.log(count); // 1  ← live!
// count = 5; // TypeError — you cannot reassign an import
```
CommonJS does not work this way (you get a snapshot).

---

## 3. Dynamic Imports & Interop

### `import()` — the function form
Returns a Promise. Works in both ESM and CJS.

```js
// Code-splitting in the browser (Vite, webpack, Rollup all do this)
button.addEventListener("click", async () => {
  const { renderChart } = await import("./chart.js"); // loaded on demand
  renderChart(data);
});
```

```js
// Conditional load
const adapter = process.env.DB === "postgres"
  ? await import("./db/postgres.js")
  : await import("./db/sqlite.js");
```

### Top-level `await` (ESM only)
```js
// main.js (ESM)
const config = await fetch("/config.json").then(r => r.json());
export default config;
```

Importers wait for this module's promise to settle before they get bindings.

### Interop in Node

| Importer | Imports... | Result |
|----------|-----------|--------|
| ESM | ESM | Native, works |
| ESM | CJS | Works. `module.exports` shows up as the **default** export. Named exports of CJS may be detected too. |
| CJS | ESM | Use dynamic `await import()` (Node 20+ has `require()` for ESM gated behind a flag, becoming default in 24). |
| CJS | CJS | Native |

```js
// ESM importing CJS
import lodash from "lodash";              // CJS module — default = its exports
import { kebabCase } from "lodash";       // named (works because Node detects)
```

```js
// CJS importing ESM (older approach)
async function main() {
  const { fetchData } = await import("./esmModule.js");
  await fetchData();
}
main();
```

### `package.json` exports map
Modern packages expose subpaths and conditional exports:

```json
{
  "name": "my-lib",
  "type": "module",
  "exports": {
    ".": {
      "import": "./dist/index.mjs",
      "require": "./dist/index.cjs",
      "types": "./dist/index.d.ts"
    },
    "./utils": "./dist/utils.mjs"
  }
}
```
This lets a single package serve ESM, CJS, and types correctly.

### The full import specifier rules in Node ESM
- Bare specifiers (`"lodash"`) → resolved via `node_modules` and `package.json` `exports`.
- Relative (`"./util.js"`) → must include the **extension**.
- Absolute (`"/abs/path.js"` or `"file:///..."`) → as-is.

```js
// ESM in Node REQUIRES the file extension on relative imports
import { x } from "./util";      // ERR_MODULE_NOT_FOUND
import { x } from "./util.js";   // OK
```
Bundlers (Vite, webpack) hide this for you in browser builds.

---

## 4. Practical Application — A Mixed Repo

Layout:
```
my-app/
  package.json        // "type": "module"
  src/
    index.js          // ESM
    util.js           // ESM
    legacy/
      old-thing.cjs   // forced CJS
```

`src/util.js`:
```js
export function greet(name) { return `Hi, ${name}`; }
```

`src/legacy/old-thing.cjs`:
```js
module.exports = { compute: (x) => x * 2 };
```

`src/index.js`:
```js
import { greet } from "./util.js";
import oldThing from "./legacy/old-thing.cjs"; // default = module.exports
import("./util.js").then((m) => console.log(m.greet("dynamic")));

console.log(greet("Ada"));
console.log(oldThing.compute(21)); // 42

// Top-level await works because we're ESM
const data = await fetch("https://api.example.com/health").then(r => r.json());
console.log("health:", data);
```

Run: `node src/index.js`.

### Code-splitting in the browser
```js
// router.js
const routes = {
  "/": () => import("./pages/home.js"),
  "/about": () => import("./pages/about.js"),
  "/admin": () => import("./pages/admin.js"), // big — only loads on /admin
};

async function navigate(path) {
  const mod = await routes[path]();
  document.getElementById("app").innerHTML = mod.render();
}
```

Vite/webpack will automatically split each `import()` into its own chunk file.

---

## 5. Common Mistakes & Gotchas

- **Forgetting `.js` extensions in Node ESM.** Most painful new-developer error. Browsers and Vite are forgiving; Node's native loader is not.
- **`__dirname` and `__filename` don't exist in ESM.** Use:
  ```js
  import { fileURLToPath } from "node:url";
  const __filename = fileURLToPath(import.meta.url);
  const __dirname = new URL(".", import.meta.url).pathname;
  ```
- **`require` doesn't exist in ESM** (without the new compat shim). Use dynamic `import()`.
- **`module.exports = ...` overwrites previous `exports.x = ...`** assignments. Pick one form per file.
- **Circular imports:**
  - ESM: works with live bindings — at the moment the first module reads from the cycle, the value may still be `undefined`.
  - CJS: returns a partial snapshot — importer may see an incomplete object.
  - Either way, prefer to break cycles by extracting a third module.
- **Top-level await in ESM blocks the module graph.** A slow `await fetch(...)` at module top means everything depending on you waits. Move expensive work into a function exported for callers.
- **Dual-package hazard:** If your library ships both ESM and CJS, two copies might exist in memory (one for each consumer). State (singletons, instanceof checks) won't work across them. Single-format is simpler.
- **`import` is hoisted.** All `import` statements run before any other code in the file. You can't put `console.log("loading X")` before `import` to "see when it loads" — the import already happened.

```js
// "Wat"
import("./mod.js");           // returns a Promise — async load
require("./mod.cjs");         // returns the exports — sync load (only in CJS)

// In ESM, "this" at module top is undefined; in CJS it's module.exports.
console.log(this); // {} in CJS; undefined in ESM
```

---

## 🎯 Key Takeaways

- **ESM is the standard;** new code in 2026 should default to ESM unless a specific dependency forces CJS.
- **Set `"type": "module"`** in `package.json` for all-ESM Node projects. Use `.cjs` for the rare CommonJS holdouts.
- **Always include `.js` extensions** on relative imports for native Node ESM.
- **Dynamic `import()` powers code-splitting** and conditional loads. Use it for heavy or rarely-used features.
- **Library authors:** prefer ESM-only and use `package.json` `exports` field to target consumers correctly. Avoid the dual-package hazard.

---

*← [07 Control Flow, Destructuring](./07_control_flow_destructuring.md) | [next → 09 Error Handling](./09_error_handling.md)*
