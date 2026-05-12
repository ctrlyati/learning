# 11 — Modules, Declaration Files, Ambient Types, Triple-Slash

> **Goal:** Understand how TypeScript sees your imports — and how to type the things you didn't write.

---

## 1. Mental model — `.ts` files are modules; `.d.ts` files are types-only

Every `.ts`, `.tsx`, `.mts`, `.cts` file with a top-level `import` or `export` is a **module**. Files without any imports/exports are **scripts** — their declarations leak into a global scope. Avoid scripts in app code.

A `.d.ts` file contains *only types* — no runtime code. It either:
- Describes the shape of a JS module, **or**
- Declares ambient types (globals).

`tsc` emits `.d.ts` files automatically with `"declaration": true` so consumers of your library get types.

---

## 2. ESM vs CJS — the module system source of pain

```ts
// ESM (modern)
import fs from "node:fs";
export const helper = () => {};

// CJS (legacy)
const fs = require("node:fs");
module.exports = { helper: () => {} };
```

Set `"type": "module"` in `package.json` for ESM. With `module: NodeNext` in `tsconfig`, TS enforces ESM rules:

- `import` paths must include the `.js` extension (yes, even for `.ts` source — TS resolves `./foo.js` to `./foo.ts`):
  ```ts
  import { helper } from "./foo.js"; // ✓
  import { helper } from "./foo";    // ✗ in NodeNext
  ```
- No `__dirname` / `__filename` — use `import.meta.url`.
- Top-level `await` is allowed.

`esModuleInterop: true` patches the import interop:

```ts
// Without esModuleInterop:
import * as fs from "fs";
// With esModuleInterop:
import fs from "fs";
```

Always on in modern projects.

---

## 3. Re-exports, type-only imports, side-effect imports

```ts
// Re-exports
export { foo, bar } from "./helpers.js";
export * from "./types.js";
export { default as helpers } from "./helpers.js";

// Type-only imports — erased completely from JS output
import type { User } from "./types.js";
import { type User, getUser } from "./api.js"; // mixed

// Type-only re-export
export type { User } from "./types.js";

// Side-effect-only import (no bindings, just runs the file)
import "./polyfills.js";
```

`verbatimModuleSyntax: true` in tsconfig forces you to mark type-only imports explicitly. Use it — it removes a class of bundler/tree-shake bugs.

---

## 4. Declaration files — `.d.ts`

A library you wrote in TS produces a `.d.ts` file like:

```ts
// dist/index.d.ts
export declare function add(a: number, b: number): number;
export interface Options { verbose?: boolean }
```

If you use a JS library *without* types, you write your own:

```ts
// types/legacy-lib.d.ts
declare module "legacy-lib" {
  export function doThing(input: string): number;
  export const VERSION: string;
}
```

Now `import { doThing } from "legacy-lib"` is typed.

A wildcard module declaration handles whole categories:

```ts
declare module "*.svg" {
  const content: string;
  export default content;
}

declare module "*.json" {
  const value: unknown;
  export default value;
}
```

Tells TS that `import logo from "./logo.svg"` produces a `string`.

---

## 5. Ambient declarations — globals and `declare global`

For globals that exist at runtime but TS doesn't know about:

```ts
// types/globals.d.ts
declare const __APP_VERSION__: string;
declare function debug(...args: unknown[]): void;

declare global {
  interface Window {
    myAnalytics: { track(event: string): void };
  }
  namespace NodeJS {
    interface ProcessEnv {
      DATABASE_URL: string;
      PORT: string;
    }
  }
}

export {}; // makes this file a module so `declare global` is valid
```

That last `export {}` is the trick — without it, the file is a script and `declare global` is illegal.

Now `process.env.DATABASE_URL` is `string` instead of `string | undefined`.

**Be careful:** lying about `process.env` types means callers won't check for missing values. Pair with runtime validation (zod) for real safety.

---

## 6. Triple-slash directives — the legacy hatch

```ts
/// <reference types="node" />
/// <reference path="./other.d.ts" />
/// <reference lib="dom" />
```

These were the pre-`tsconfig` way to bring in types. You'll see them in old `.d.ts` files and library bundles. Modern code prefers `tsconfig` `types`, `lib`, and `include`.

Cases where they still appear:

- A library author wants its consumer to inherit type references (`/// <reference types="node" />` at the top of an exported `.d.ts`).
- Hand-rolled type packages.

Don't add new ones to app code.

---

## 7. Practical application — typing an untyped npm package

You install `funky-helper` and find no types:

```bash
npm install funky-helper
```

Step 1 — check for community types:

```bash
npm install --save-dev @types/funky-helper
```

If they exist, you're done.

Step 2 — write your own ambient module file:

```ts
// types/funky-helper.d.ts
declare module "funky-helper" {
  export interface FunkyOptions {
    verbose?: boolean;
    timeout?: number;
  }

  export function funkify<T>(input: T, opts?: FunkyOptions): Promise<T>;
  export const version: string;

  export default function defaultFunky(s: string): string;
}
```

Step 3 — register the directory in tsconfig if needed:

```json
{
  "compilerOptions": {
    "typeRoots": ["./node_modules/@types", "./types"]
  },
  "include": ["src/**/*", "types/**/*"]
}
```

Now `import funkify from "funky-helper"` is typed. Verify with `tsc --noEmit`.

Step 4 (optional) — open a PR to DefinitelyTyped to publish your types as `@types/funky-helper`. Real career win.

---

## 8. Common mistakes & gotchas

**Mistake 1: Forgetting `.js` extensions in `NodeNext` mode.** Source is `.ts`, but the import path is `.js`. Get used to it — it's correct.

**Mistake 2: Mixing CJS and ESM.** A package with `"type": "module"` cannot `require()` and a CJS package can't be `import`ed cleanly without interop. Pick a lane per package.

**Mistake 3: `declare global` in a script file.** It must be in a module — add `export {}` if there's nothing else to export.

**Mistake 4: Ambient declarations that lie.** Saying `process.env.X: string` doesn't *make* it defined. Pair with runtime checks.

**Mistake 5: Forgetting `import type`.** With `verbatimModuleSyntax`, TS errors on `import { Type, fn } from "x"` if `Type` is type-only — bundlers can't always tree-shake it. Use `import { type Type, fn }`.

**Mistake 6: Editing `.d.ts` files in `node_modules`.** They get blown away on reinstall. Use `patch-package` or augmentation in your own files.

---

## 🎯 Key Takeaways

- Modules are files with `import`/`export`; without them you're polluting global scope.
- `.d.ts` files describe shape only — no runtime code, no emit.
- Use `import type` and `verbatimModuleSyntax` to make type-only imports explicit and bundler-safe.
- Ambient module/global declarations let you type any JS library, even without `@types`.
- Triple-slash references are mostly legacy; configure types via `tsconfig` instead.

*← [10 — Classes & decorators](./10_classes_and_decorators.md) | [next →](./12_libraries_and_resolution.md)*
