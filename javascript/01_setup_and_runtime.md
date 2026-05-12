# 01 — Setup & Runtimes

> **Goal:** Understand the four major places JavaScript runs (browser, Node.js, Bun, Deno), get them installed, and run your first scripts in each.

---

## 1. The JavaScript Runtime — Mental Model

A *runtime* is the environment that takes your `.js` file and actually executes it. Crucially, **the JavaScript language and the JavaScript runtime are different things.**

- **The language** is defined by ECMAScript (ECMA-262). It gives you syntax, types, prototypes, promises, etc.
- **The runtime** adds the bridge to the outside world: file I/O, network, the DOM, timers, environment variables.

So `for`, `Array.prototype.map`, `async`/`await` — language. `fetch`, `document`, `fs.readFile`, `Bun.serve` — runtime.

```js
// Pure language — runs anywhere
const square = (n) => n * n;
console.log(square(7)); // 49
```

```js
// Runtime-specific — only works in browser
document.body.innerHTML = "Hello";

// Runtime-specific — only works in Node/Bun/Deno
import fs from "node:fs";
fs.writeFileSync("hello.txt", "hi");
```

The four runtimes you should know in 2026:

| Runtime | Engine | Where it shines | Default module system |
|---------|--------|------------------|-----------------------|
| **Browser** (V8/SpiderMonkey/JSC) | varies | UI, web apps | ESM (via `<script type="module">`) |
| **Node.js** | V8 | Servers, CLIs, build tools | CommonJS (legacy default), ESM (modern) |
| **Bun** | JavaScriptCore | Fast all-in-one for new projects | ESM-first, both supported |
| **Deno** | V8 | Secure-by-default scripting, edge | ESM only, TS native |

---

## 2. Under the Hood — How the Engine Runs Your Code

When you hand a `.js` file to V8 (the engine in Chrome and Node), roughly:

1. **Parse** → produces an AST (abstract syntax tree).
2. **Compile to bytecode** (Ignition interpreter).
3. **Execute** bytecode; hot functions get **JIT-compiled** to optimized machine code (TurboFan).
4. **Garbage collect** unreachable objects (generational GC).

You don't manage memory manually. You don't pick threads. The engine + runtime handle it.

A runtime wraps the engine and adds:
- An **event loop** (we cover this in module 11)
- **APIs** for I/O (`fetch`, `fs`, etc.)
- A **module loader**
- A **process model** (for Node/Bun/Deno: `process.argv`, `process.env`, exit codes)

```js
// Same code, three runtimes, three behaviors
console.log(typeof window);  // browser: "object" | node/bun/deno: "undefined"
console.log(typeof process); // node/bun: "object" | browser: "undefined" | deno: "object" (with Node compat)
```

---

## 3. Installing Each Runtime

### Browser
You already have one. Open devtools (F12) and click the **Console** tab. That's a full REPL.

```js
// Paste into the browser console
const t = performance.now();
for (let i = 0; i < 1e6; i++) {}
console.log(`Took ${performance.now() - t}ms`);
```

### Node.js (the one you must install)
Use a version manager — `nvm` on macOS/Linux, `nvm-windows` or `fnm` on Windows. Pin to an LTS:

```bash
fnm install 22
fnm use 22
node -v   # v22.x.x
```

Run a script:
```bash
node hello.js
```

REPL:
```bash
node
> 2 + 2
4
> .exit
```

### Bun
A JS runtime + package manager + bundler + test runner, written in Zig. Drop-in faster Node for many use cases.

```bash
# macOS / Linux / WSL
curl -fsSL https://bun.sh/install | bash
# Windows (PowerShell)
irm bun.sh/install.ps1 | iex

bun -v
bun run hello.js
```

### Deno
Secure by default — must opt into file/network/env access via flags. Great for scripts.

```bash
# macOS / Linux
curl -fsSL https://deno.land/install.sh | sh
# Windows (PowerShell)
irm https://deno.land/install.ps1 | iex

deno --version
deno run hello.js               # no I/O perms
deno run --allow-net hello.js   # grant network
```

---

## 4. Practical Application — Same Script, Three Runtimes

A tiny script that fetches a URL and prints the byte length. Notice how little changes.

**`fetch_size.js`**
```js
const url = "https://example.com";

async function main() {
  const t0 = performance.now();
  const res = await fetch(url);
  const buf = await res.arrayBuffer();
  const ms = (performance.now() - t0).toFixed(1);
  console.log(`${url} → ${buf.byteLength} bytes in ${ms}ms`);
}

main().catch((err) => {
  console.error("failed:", err);
  // Browser has no `process`; guard it.
  if (typeof process !== "undefined") process.exit(1);
});
```

Run it:

```bash
node fetch_size.js                   # works on Node 18+ (global fetch)
bun fetch_size.js                    # works
deno run --allow-net fetch_size.js   # explicit network permission
```

In the browser, paste the body of `main()` straight into devtools — `fetch` is the same API everywhere. (It originated in the browser; Node copied it.)

A minimal HTML harness for browser:

**`index.html`**
```html
<!doctype html>
<html>
<body>
  <script type="module" src="./fetch_size.js"></script>
</body>
</html>
```

Serve it (don't open via `file://` — modules need HTTP):
```bash
npx serve .
# or
bunx serve .
```

---

## 5. Common Mistakes & Gotchas

- **Opening `index.html` with `file://`.** ES modules and `fetch` won't work. Always serve over HTTP, even locally.
- **Mixing CommonJS and ESM in Node without thinking.** A `.js` file in a project with `"type": "module"` is ESM; otherwise CJS. We unpack this in module 08.
- **Assuming `window` or `document` exists in Node.** They don't. Same for `process` in the browser. Code that runs in both ("isomorphic") must guard.
- **Old "Node doesn't have fetch" advice.** Outdated. Node 18+ ships `fetch` globally. Don't `npm install node-fetch` in new projects.
- **`console.log` lying about objects.** It often shows the object's *current* state, not the state at log time. Use `console.log(JSON.parse(JSON.stringify(obj)))` or breakpoints when debugging mutation.
- **Top-level `await` only works in ESM.** In a CommonJS file, `await` outside an `async` function is a syntax error.
- **Node version drift.** Features land in V8 long before they appear in your installed Node. Always know your `node -v`.

```js
// Wat moment
console.log(typeof null);     // "object"  ← bug from 1995, never fixed
console.log(typeof NaN);      // "number"  ← yes, NaN is a number
console.log(typeof function(){}); // "function" ← but functions are objects
```

We'll meet many more "wat" moments in module 02.

---

## 🎯 Key Takeaways

- **Language ≠ runtime.** ECMAScript gives you syntax; the runtime gives you the world.
- **Pin your Node version** with `fnm`/`nvm`. Production drift starts here.
- **Bun and Deno are real options in 2026** — Bun for performance, Deno for security/scripting. Node remains the default for jobs.
- **`fetch` is universal now.** Stop reaching for `axios` reflexively in new code.
- **Always serve HTML over HTTP** when you use modules — `file://` will betray you.

---

*← [roadmap](./00_roadmap.md) | [next → 02 Values, Types, Coercion, Equality](./02_values_types_coercion.md)*
