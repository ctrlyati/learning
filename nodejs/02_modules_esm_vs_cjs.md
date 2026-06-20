# 02 — Modules: ESM vs CommonJS

> **Goal:** Master the differences between CommonJS (CJS) and ES Modules (ESM), understand module resolution rules, and configure package.json exports.

---

## 1. Concept: Two Module Systems

Node.js supports two different module formats:
1. **CommonJS (CJS):** The original Node.js module format. Uses `require()` and `module.exports`.
2. **ES Modules (ESM):** The official ECMAScript standard. Uses `import` and `export`.

```javascript
// CommonJS (legacy / default)
const math = require('./math');
module.exports = { add: (a, b) => a + b };

// ES Modules (modern / standard)
import { add } from './math.js';
export const add = (a, b) => a + b;
```

---

## 2. Mechanism: How They Load

The core difference is *when* and *how* modules are loaded and executed.

| Feature | CommonJS (`require`) | ES Modules (`import`) |
|---------|-----------------------|-----------------------|
| **Evaluation** | Synchronous, at runtime. | Asynchronous, in 3 distinct phases (Parse, Link, Evaluate). |
| **Static Analysis** | No. Paths can be dynamic (e.g. `require(path)`). | Yes. Imports are analyzed before execution. |
| **Scope** | Wraps code in a function wrapper at runtime. | Top-level scope is modular. |
| **Globals** | Has `__dirname`, `__filename`, `require`. | None of those. Uses `import.meta.url`. |

When Node parses ES Modules, it builds a module graph *before* running any JavaScript. This allows ESM to support circular dependencies better than CJS and supports static tooling optimizations (tree-shaking).

---

## 3. Variations & Depth: The ESM / CJS Boundary

### Emulating `__dirname` and `__filename` in ESM
In ES Modules, `__dirname` is not defined. You must derive it using `import.meta.url`:

```javascript
import { fileURLToPath } from 'url';
import { dirname } from 'path';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
```

### Loading CJS from ESM (Allowed)
```javascript
import cjsModule from './legacy-cjs-file.cjs'; // works!
```

### Loading ESM from CJS (Requires Dynamic Import)
Because CJS is synchronous, you cannot use static `import` or standard `require` to load ESM. You must use a dynamic asynchronous `import()`:

```javascript
// In a CJS file
async function loadESM() {
  const { modernFunction } = await import('./esm-file.mjs');
  modernFunction();
}
```

### Configuring `package.json`
- `"type": "commonjs"` (default): `.js` files are treated as CommonJS.
- `"type": "module"`: `.js` files are treated as ES Modules.
- Regardless of `"type"`, `.cjs` is always CommonJS, and `.mjs` is always ESM.

---

## 4. Practical Application: Building a Dual-Format Package

Let's write a library that supports both module formats using `package.json` conditional exports.

**`package.json`**
```json
{
  "name": "my-dual-library",
  "version": "1.0.0",
  "type": "module",
  "exports": {
    ".": {
      "import": "./dist/index.js",
      "require": "./dist/index.cjs"
    }
  }
}
```

**`dist/index.js` (ESM target)**
```javascript
export function greet(name) {
  return `Hello ESM, ${name}!`;
}
```

**`dist/index.cjs` (CJS target)**
```javascript
function greet(name) {
  return `Hello CJS, ${name}!`;
}
module.exports = { greet };
```

---

## 5. Common Mistakes & Gotchas

- **Forgetting `.js` extensions in ESM:** In Node.js ESM, you must specify the file extension (e.g. `import './math.js'`), unlike frontend bundlers or TypeScript defaults which allow `import './math'`.
- **Top-level await only in ESM:** You can `await` at the top level of an ES module. In CommonJS, doing so throws a `SyntaxError: await is only valid in async functions`.
- **Dual-package hazard:** If a consumer imports both the ESM version and require-s the CJS version of a library, Node loads the library *twice* into memory. If the library holds singleton states, they will drift apart.

---

## 🎯 Key Takeaways

- **Default to ESM** for modern projects by setting `"type": "module"` in `package.json`.
- **Remember the boundary rules:** ESM can synchronously import CJS; CJS must asynchronously import ESM.
- **Provide explicit paths:** ESM resolution is strict; file extensions are mandatory.

---

*← [architecture](./01_architecture_and_v8.md) | [next → 03 Buffers & Streams](./03_buffers_and_streams.md)*
