# 12 — Working with Libraries: `@types`, Module Resolution, ESM/CJS Interop

> **Goal:** Be the person on the team who can debug "why isn't this import working?" in 30 seconds.

---

## 1. The `@types/*` ecosystem — DefinitelyTyped

If a JS library doesn't ship its own `.d.ts`, types may live at `@types/<name>`:

```bash
npm install lodash
npm install --save-dev @types/lodash
```

Now `import _ from "lodash"` is fully typed.

How TS finds them: `tsconfig`'s `typeRoots` (default `node_modules/@types`) is auto-scanned. Every `@types/*` package whose name matches an installed package gets included.

To restrict (avoid pulling in `@types/jest` globals into your app code):

```json
{
  "compilerOptions": {
    "types": ["node"]   // only these are included; others ignored
  }
}
```

Use `types` cautiously — it's an allowlist, not an exclude list.

---

## 2. Library type sources — the hierarchy TS checks

When you `import "foo"`, TS finds types in this order:

1. The package's `package.json` `"types"` or `"typings"` field
2. The package's `package.json` `"exports"` field with conditions (`"types"`, `"import"`, `"require"`)
3. An `index.d.ts` adjacent to `index.js`
4. An installed `@types/foo` package
5. A wildcard ambient module declaration in your project

If all of these miss, you get `Could not find a declaration file for module 'foo'`. The fix is one of (4) or (5) — covered in Module 11.

Modern packages with proper `exports` look like:

```json
{
  "name": "lib",
  "type": "module",
  "exports": {
    ".": {
      "types": "./dist/index.d.ts",
      "import": "./dist/index.js",
      "require": "./dist/index.cjs"
    },
    "./sub": {
      "types": "./dist/sub.d.ts",
      "import": "./dist/sub.js"
    }
  }
}
```

Subpath exports (`"./sub"`) restrict what's importable — `import x from "lib/internal"` will fail unless declared.

---

## 3. Module resolution strategies

`tsconfig`'s `moduleResolution` controls how TS turns `"./foo"` and `"foo"` into actual files. The choices that matter:

- **`NodeNext`** — modern Node, honors `package.json` `exports`, `type`, requires `.js` extensions on relative imports. Pair with `module: NodeNext`.
- **`Bundler`** — for Vite, esbuild, webpack, Bun. Doesn't require extensions. Honors `exports`. Pair with `module: ESNext` and a bundler that does the actual resolution at build time.
- **`Node10`** (formerly `Node`) — pre-`exports` Node. Avoid in new projects.
- **`Classic`** — never use this. Legacy artifact.

The two that matter for new code: **`NodeNext`** if you target Node directly, **`Bundler`** if a bundler handles output.

---

## 4. ESM / CJS interop — the source of half your bugs

A pure-ESM package can be `import`ed from ESM but not `require`d from CJS without dynamic `import()`. A CJS package can be `require`d, and *also* `import`ed from ESM with default-import interop:

```ts
// CJS package "old-lib" with `module.exports = function () {}`
import oldLib from "old-lib"; // ✓ with esModuleInterop
oldLib();
```

The `esModuleInterop` flag synthesizes a `default` export at the type level for CJS modules. Without it:

```ts
import * as oldLib from "old-lib";
(oldLib as any)(); // ugly
```

Common pain points:

**Pain 1: "ERR_REQUIRE_ESM."** A CJS package tried to `require()` an ESM-only package. Convert your package to ESM, or use dynamic `import()`:

```ts
const lib = await import("esm-only-lib");
```

**Pain 2: Default export confusion.**

```ts
// CJS: module.exports = { foo, bar }
import * as lib from "x";  // lib.foo, lib.bar
import lib from "x";       // requires esModuleInterop; lib === { foo, bar }

// ESM: export default function foo() {}
import foo from "x";       // foo === function
```

When in doubt, look at the package's `dist/index.d.ts`. It's authoritative.

**Pain 3: `__esModule` flag.** Old transpilers added `module.exports.__esModule = true` to mark "this looks like ESM." `esModuleInterop` knows about this. Modern packages just ship dual builds (CJS + ESM via `exports` conditions).

---

## 5. Practical application — debugging a broken import

Symptom: `import { foo } from "lib"` shows `foo` as `any` even though `lib` is installed.

Step-by-step diagnosis:

```bash
# 1. Does lib have its own types?
ls node_modules/lib/dist/index.d.ts

# 2. Or does its package.json point somewhere?
cat node_modules/lib/package.json
# Look for: "types", "typings", "exports"."."."types"

# 3. Is there a community types package?
npm view @types/lib

# 4. What does TS itself say?
npx tsc --traceResolution 2>&1 | grep -A 5 "lib"
```

`--traceResolution` is the magic flag — it dumps every step TS took looking for the module. Read the output and you'll see exactly which file got picked (or didn't).

Other diagnostics:

```bash
npx tsc --showConfig          # final merged tsconfig — extends, paths, all of it
npx tsc --listFiles           # every file the compiler is loading
```

---

## 6. Path aliases — `paths` and `baseUrl`

Avoid `../../../utils/foo`:

```json
{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@/*": ["src/*"],
      "~/utils/*": ["src/utils/*"]
    }
  }
}
```

```ts
import { foo } from "@/utils/foo";
```

**Critical caveat:** `tsc` understands `paths`, but **the runtime does not**. A bundler (Vite, esbuild, webpack) must resolve them, **or** you must add a runtime resolver (`tsconfig-paths`, `tsx` reads `paths` automatically). Otherwise compiled output ships with broken imports.

A safer modern alternative — **subpath imports** in `package.json`:

```json
{
  "imports": {
    "#utils/*": "./src/utils/*.ts"
  }
}
```

```ts
import { foo } from "#utils/foo";
```

These work in both Node and bundlers without extra config. Prefer them for new projects.

---

## 7. Common mistakes & gotchas

**Mistake 1: Setting `types: []` and losing all global typings silently.** Empty array means "no global types." Often unintended.

**Mistake 2: Mixing `paths` with `tsc` builds without a path-rewriter.** Output won't run. Either use a bundler or `tsc-alias` to rewrite imports post-build.

**Mistake 3: Trusting the editor over the build.** VS Code uses TS with auto-include settings; CI uses your real `tsconfig`. Run `tsc --noEmit` locally before pushing.

**Mistake 4: Pinning `@types/*` to wrong major.** `@types/react@18` for React 18, `@types/react@19` for React 19. Mismatches give weird errors.

**Mistake 5: Installing `@types/*` for a package that already ships its own types.** You'll get duplicate or stale type info. Check first.

**Mistake 6: Forgetting `skipLibCheck: false` consequences.** Turning it off forces TS to type-check every `.d.ts` in `node_modules` — usually not your problem, often broken upstream, slows builds. Leave `skipLibCheck: true` unless you're a library author.

---

## 🎯 Key Takeaways

- TS finds types via the package's `exports.types`, then root `index.d.ts`, then `@types/*`, then your ambient declarations.
- `--traceResolution` is the debugger for "why doesn't this import resolve?"
- Pick `NodeNext` for native Node, `Bundler` for everything bundler-driven; avoid `Node10`/`Classic`.
- `esModuleInterop` is mandatory for sane CJS-from-ESM interop; pair with `verbatimModuleSyntax` for explicit type-only imports.
- Prefer `package.json` subpath imports (`#alias/*`) over `tsconfig` `paths` — they work everywhere with zero config.

*← [11 — Modules & declarations](./11_modules_and_declarations.md) | [next →](./13_strict_mode_deep_dive.md)*
