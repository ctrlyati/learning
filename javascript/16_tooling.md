# 16 — Tooling: Bundlers, Transpilers, Linters, Formatters

> **Goal:** Configure a modern JS project with Vite (or ESBuild), ESLint, and Prettier — and understand what each tool actually does so you can debug when they fight.

---

## 1. The Tooling Landscape — Mental Model

Modern JS projects involve a handful of tools, each with one job:

| Tool kind | Job | Examples |
|-----------|-----|----------|
| **Bundler** | Combine modules into output files | Vite, webpack, Rollup, Parcel, esbuild, rspack |
| **Transpiler** | Translate newer/other syntax to runnable JS | Babel, SWC, esbuild, TSC, OXC |
| **Linter** | Find bugs/anti-patterns in source | ESLint, Biome, Oxlint |
| **Formatter** | Enforce consistent style | Prettier, Biome, dprint |
| **Type checker** | Verify types | TypeScript (`tsc`), Flow (rare) |

Most modern stacks pick **Vite + ESLint + Prettier** (or **Biome** which combines lint+format). Vite uses esbuild internally for dev and Rollup for production.

```bash
# Scaffold a new app
npm create vite@latest my-app
cd my-app
npm install
npm run dev    # esbuild-powered dev server
npm run build  # rollup-powered production bundle
```

---

## 2. Bundlers & Transpilers — Under the Hood

### Why bundle?
Browsers load every `<script type="module">` import as a separate HTTP request. Without bundling, 500-module apps make 500 requests. Bundlers:
- Concatenate modules into fewer files
- Tree-shake unused exports
- Code-split by `import()` boundaries
- Transform syntax (TS → JS, JSX → JS, modern → older targets)
- Optimize: minify, hash filenames for caching, source maps

### Vite — dev vs build
- **Dev:** serves source files directly over native ESM, transforming on demand with esbuild. No bundling step → instant cold start.
- **Build:** uses Rollup for production output (better tree-shaking, smaller bundles than esbuild's bundler).

```js
// vite.config.js
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: { port: 5173 },
  build: {
    target: "es2020",
    sourcemap: true,
    rollupOptions: {
      output: { manualChunks: { vendor: ["react", "react-dom"] } },
    },
  },
});
```

### esbuild — when you don't need a framework
Plain bundling/transpiling, blazing fast. Good for libraries:

```bash
npx esbuild src/index.ts \
  --bundle --minify --sourcemap \
  --target=es2020 --format=esm \
  --outfile=dist/index.mjs
```

### SWC and OXC — the Rust newcomers
- **SWC** — Rust transpiler used by Next.js. Drop-in for Babel.
- **OXC** — newer Rust-based linter/transformer. Very fast.

You'll mostly meet these as the engine *inside* your framework's build, not directly.

### Babel — still around
Used when:
- You need a Babel-only plugin (e.g. some experimental TC39 proposals).
- A framework specifically requires it.
- Otherwise: skip. esbuild/SWC do 95% of what Babel did, much faster.

### Tree-shaking — what makes it work
Tree-shaking can drop unused exports only when:
1. You use **ESM** (`import`/`export`), not CJS.
2. The library marks itself **side-effect-free**: `"sideEffects": false` in `package.json`.
3. Your imports are static (`import { foo } from "lib"`, not `require("lib").foo`).

A package's `"sideEffects": false` declaration is high-leverage — it lets bundlers drop modules that have only re-exports.

### Source maps
A `.map` file mapping minified output back to source. Always generate them. In production, host them privately and reference via `Sourcemap` HTTP header for tools like Sentry — don't expose raw source to the public.

---

## 3. Linters & Formatters

### ESLint — finds bugs
Rules range from "prevent real bugs" (`no-unused-vars`, `eqeqeq`) to "stylistic" (handle in Prettier instead).

`eslint.config.js` (flat config — modern):
```js
import js from "@eslint/js";
import tseslint from "typescript-eslint";

export default [
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    rules: {
      "no-unused-vars": "warn",
      "eqeqeq": ["error", "always"],
      "no-console": ["warn", { allow: ["warn", "error"] }],
    },
  },
  { ignores: ["dist/**", "node_modules/**"] },
];
```

Run:
```bash
npx eslint .
npx eslint . --fix          # auto-fix safe issues
```

Useful plugin packs:
- `eslint-plugin-import` — order/correctness of imports
- `eslint-plugin-react-hooks` — catches React hook bugs (huge value)
- `eslint-plugin-n` — Node-specific rules
- `eslint-plugin-promise` — promise patterns
- `@typescript-eslint/...` — TS rules (covers many things `tsc` doesn't)

### Prettier — opinionated formatter
```bash
npx prettier --write .
npx prettier --check .   # CI mode
```

`.prettierrc.json`:
```json
{
  "singleQuote": false,
  "trailingComma": "all",
  "printWidth": 100,
  "tabWidth": 2
}
```

The point of Prettier is to stop arguing about style. Pick the defaults, move on.

### Biome — the all-in-one challenger
Biome is a single Rust tool that does linting and formatting in one binary, much faster than ESLint + Prettier together. In 2026, Biome is a serious choice for greenfield projects:

```bash
npm i -D --save-exact @biomejs/biome
npx biome init
npx biome check --write .
```

Trade-off: smaller rule set than ESLint, less plugin ecosystem. For React/TS apps with many specialized lint needs, ESLint still wins. For lean libraries: Biome is great.

### Editor integration
- VS Code: install ESLint and Prettier extensions; enable "format on save."
- Lint-staged + Husky to format/lint only changed files on commit:

```json
// package.json
{
  "scripts": { "prepare": "husky" },
  "lint-staged": {
    "*.{js,ts,jsx,tsx}": ["prettier --write", "eslint --fix"]
  }
}
```

```bash
npx husky add .husky/pre-commit "npx lint-staged"
```

This stops malformed code from being committed in the first place.

---

## 4. Practical Application — Setting Up a Real Project

End-to-end: a fresh TS+React project with Vite, ESLint, Prettier, Vitest, and a CI script.

```bash
npm create vite@latest my-app -- --template react-ts
cd my-app
npm install
npm install -D \
  eslint @eslint/js typescript-eslint \
  eslint-plugin-react-hooks eslint-plugin-import \
  prettier eslint-config-prettier \
  vitest @vitest/coverage-v8 \
  husky lint-staged
```

`eslint.config.js`:
```js
import js from "@eslint/js";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import importPlugin from "eslint-plugin-import";
import prettier from "eslint-config-prettier";

export default [
  { ignores: ["dist", "coverage", "node_modules"] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    plugins: { "react-hooks": reactHooks, import: importPlugin },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "import/order": ["error", { "newlines-between": "always" }],
      "eqeqeq": ["error", "always"],
      "no-console": ["warn", { allow: ["warn", "error"] }],
    },
  },
  prettier, // disable lint rules that conflict with Prettier
];
```

`.prettierrc.json`:
```json
{ "singleQuote": false, "trailingComma": "all", "printWidth": 100 }
```

`package.json` scripts:
```json
{
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview",
    "test": "vitest",
    "test:ci": "vitest run --coverage",
    "lint": "eslint . --max-warnings=0",
    "format": "prettier --write .",
    "typecheck": "tsc --noEmit",
    "ci": "npm run lint && npm run typecheck && npm run test:ci && npm run build",
    "prepare": "husky"
  },
  "lint-staged": {
    "*.{js,ts,jsx,tsx}": ["prettier --write", "eslint --fix"]
  }
}
```

A minimal CI workflow (`.github/workflows/ci.yml`):
```yaml
name: ci
on: [push, pull_request]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: 22, cache: npm }
      - run: npm ci
      - run: npm run ci
```

You now have: dev server, prod build, unit tests with coverage, lint, type-check, format-on-commit, and CI. That's a complete baseline.

---

## 5. Common Mistakes & Gotchas

- **Mixing ESLint stylistic rules with Prettier.** They will fight forever. Use `eslint-config-prettier` to disable conflicting rules.
- **No `ignores` block.** ESLint will lint `node_modules`/`dist` and take forever.
- **Targeting too-old browsers.** Massive bundle bloat. In 2026, target `es2020`+ unless you know you need older.
- **Source maps in production with no protection.** Useful for error tracking, but if hosted publicly any user can read your source. Configure your error-tracking tool to upload them privately.
- **Bundling Node-only packages for the browser.** `fs`, `path`, etc. don't exist in the browser. Vite will warn; webpack used to silently fail (now also warns).
- **Forgetting `sideEffects: false`** when publishing a library — kills tree-shaking for consumers.
- **Auto-formatting on save without commit hooks.** Some editors lag; CI catches but PRs become noisy. Use both: editor + lint-staged.
- **TypeScript checks in `vite build` only.** Vite uses esbuild which only **strips** types, doesn't check them. You must run `tsc --noEmit` separately. Hence `"build": "tsc -b && vite build"`.
- **Multiple lockfiles** (`package-lock.json` + `yarn.lock`) → tools get confused. Pick one.
- **Disabling lint rules everywhere instead of fixing them.** `// eslint-disable-next-line` accumulates and hides real issues. Triage and fix.
- **"Bundling" Node code with esbuild** to ship to production — unnecessary for backends; use a process manager and run source directly.

```js
// "Wat"
import * as React from "react"; // tree-shakeable in modern bundlers
import React from "react";       // also fine; default-export interop
// Both work in Vite + Rollup; some older webpack setups don't tree-shake the namespace import
```

### When to NOT use Vite
- Pure backend Node apps — no bundler needed; just run source.
- Library publishing — use `tsup` (esbuild wrapper), `unbuild`, or `rollup` directly. Vite is opinionated for apps.
- Browser extensions / electron — different toolchains.

---

## 🎯 Key Takeaways

- **Vite is the modern default for apps.** esbuild for dev (instant), Rollup for build (small).
- **ESLint catches bugs; Prettier formats.** Don't ask one tool to do the other's job. Use `eslint-config-prettier` to keep peace.
- **`tsc --noEmit` separately from your bundler** — Vite/esbuild only strip types, not check them. CI must run both.
- **Tree-shaking requires ESM + `sideEffects: false`.** Publishing a library? Get this right — your consumers' bundle size depends on it.
- **Lint-staged + Husky + format-on-save** is the trifecta that keeps CI green and PRs readable. Set it up once, save endless review nitpicks.

---

*← [15 Testing](./15_testing.md) | [next → 17 Modern Patterns & Production](./17_modern_patterns_production.md)*
