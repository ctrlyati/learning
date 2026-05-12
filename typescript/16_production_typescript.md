# 16 — Production: Monorepo Configs, Project References, Build Performance, Migrating JS

> **Goal:** Operate TypeScript at scale — multiple packages, fast builds, and confident migration of legacy JS.

---

## 1. Monorepo TS configs — `extends` and base configs

A monorepo has many `tsconfig.json` files. They should all `extends` a base:

```
repo/
  tsconfig.base.json         ← shared compilerOptions
  packages/
    api/
      tsconfig.json          ← extends base, package-specific
      src/
    web/
      tsconfig.json
      src/
    shared/
      tsconfig.json
      src/
```

`tsconfig.base.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "exactOptionalPropertyTypes": true,
    "verbatimModuleSyntax": true,
    "declaration": true,
    "declarationMap": true,
    "sourceMap": true
  }
}
```

`packages/api/tsconfig.json`:

```json
{
  "extends": "../../tsconfig.base.json",
  "compilerOptions": {
    "outDir": "./dist",
    "rootDir": "./src",
    "lib": ["ES2022"],
    "types": ["node"]
  },
  "include": ["src/**/*"],
  "exclude": ["dist", "node_modules"]
}
```

Centralizing options means upgrading the whole repo is one diff.

---

## 2. Project references — incremental builds across packages

Without project references, TS recompiles everything every time. With them, it builds a dependency graph and only rebuilds what changed.

Mark each package as composable:

```json
// packages/shared/tsconfig.json
{
  "extends": "../../tsconfig.base.json",
  "compilerOptions": {
    "composite": true,
    "outDir": "./dist",
    "rootDir": "./src"
  },
  "include": ["src/**/*"]
}
```

Add references where one package depends on another:

```json
// packages/api/tsconfig.json
{
  "extends": "../../tsconfig.base.json",
  "compilerOptions": { "composite": true, "outDir": "./dist", "rootDir": "./src" },
  "include": ["src/**/*"],
  "references": [
    { "path": "../shared" }
  ]
}
```

A root `tsconfig.json` orchestrates:

```json
{
  "files": [],
  "references": [
    { "path": "./packages/shared" },
    { "path": "./packages/api" },
    { "path": "./packages/web" }
  ]
}
```

Build with:

```bash
tsc --build              # build everything in topological order
tsc --build --watch      # watch mode for the whole graph
tsc --build --clean      # remove all .tsbuildinfo
tsc --build --force      # full rebuild
```

Each package gets a `.tsbuildinfo` file with incremental state. Don't commit them; do gitignore.

**Caveat:** project references and bundlers don't always cooperate. For app code consumed by Vite/esbuild, project references are often overkill — use them mainly for libraries, codegen, or when build time is genuinely a problem.

---

## 3. Build performance — diagnosing slowness

When `tsc` is slow, measure:

```bash
tsc --extendedDiagnostics
# Files:           1234
# Lines:           234567
# Identifiers:     1024000
# Symbols:         512000
# Types:           48000
# Memory used:     412 MB
# I/O time:        0.42s
# Parse time:      1.21s
# Bind time:       0.45s
# Check time:      8.34s         ← usually the culprit
# Emit time:       0.92s
# Total time:      11.34s
```

Long check time → expensive types. Common culprits:

- **Recursive conditional types** with high fan-out
- **Huge unions** (string literal types from auto-generated code)
- **Deep mapped types** over wide objects
- **Library `.d.ts`** with elaborate types (zod, Prisma)

Tactics:

1. `skipLibCheck: true` — non-negotiable for app code.
2. **Project references** — split a slow package into pieces.
3. **Type widening at boundaries** — `Prettify<T>` or explicit annotations on exported APIs prevent TS from re-deriving the same intersection at every call site.
4. **`isolatedModules: true`** — required by transpilers like esbuild; surfaces patterns that bundlers can't handle (e.g., `const enum` re-exports), letting you fix them.
5. **Type-check separately from build** — use esbuild/swc for `dist/`, run `tsc --noEmit` in CI only.

Profile the type-check itself with:

```bash
tsc --generateTrace ./trace
# open trace/trace.json in chrome://tracing or speedscope
```

---

## 4. Migrating a JS codebase

Pragmatic playbook for an existing JS project:

**Phase 1 — `allowJs`, no rename.** Add TS without forcing renames.

```json
{
  "compilerOptions": {
    "allowJs": true,
    "checkJs": false,
    "outDir": "./dist",
    "noEmit": true,
    "strict": false
  },
  "include": ["src/**/*"]
}
```

Now you can write *new* code in `.ts` while old `.js` keeps working.

**Phase 2 — `checkJs` with JSDoc.** Add `// @ts-check` at the top of `.js` files (or set `checkJs: true` globally) and use JSDoc to type things without renaming:

```js
// @ts-check
/** @param {string} name */
function greet(name) { return `hi ${name}`; }
```

JSDoc covers most of TS's vocabulary. You can lift entire files this way before any rename.

**Phase 3 — rename file by file.** `.js` → `.ts`. Resolve type errors. Commit per file. PRs stay reviewable.

**Phase 4 — turn on strict flags incrementally.** See Module 13's migration order.

**Phase 5 — remove `allowJs` once everything is TS.** Now you have a fully typed codebase.

Key tools:

- **`// @ts-expect-error` with explanation** — for known-broken code you'll fix later. Errors when the underlying issue is fixed (forces cleanup).
- **`// @ts-ignore`** — only for genuinely unfixable upstream issues.
- **`ts-migrate`** (Airbnb) — bulk converter; produces messy code but a real starting point.
- **`type-coverage`** — measures what % of your code is typed (excluding `any`). Useful CI metric.

---

## 5. Practical application — a turborepo-style layout

```
my-app/
  package.json                  ← workspace root
  pnpm-workspace.yaml
  tsconfig.base.json
  tsconfig.json                 ← project references root
  packages/
    types/
      tsconfig.json
      src/index.ts              ← shared types
    api/
      tsconfig.json
      src/index.ts
      src/routes/users.ts
    web/
      tsconfig.json
      src/main.tsx
    shared/
      tsconfig.json
      src/result.ts
      src/branded.ts
```

Build pipeline:

```json
// package.json
{
  "scripts": {
    "build":     "tsc --build",
    "watch":     "tsc --build --watch",
    "typecheck": "tsc --build --dry",
    "clean":     "tsc --build --clean"
  }
}
```

CI runs `pnpm typecheck` for fast feedback, `pnpm build` for artifacts, ESLint for style, Vitest for tests. Each package can be deployed independently because each has its own `dist/` and `package.json`.

For things bundled by Vite (the web package), often you skip `composite` and `tsc` entirely for that package — let Vite handle it and just add `tsc --noEmit` to CI for type checking.

---

## 6. Things that go wrong at scale

**`paths` and bundlers diverge.** A `paths` alias works in `tsc` and your editor but produces broken `import` strings in dist if not rewritten. Use `tsc-alias`, or prefer subpath imports (`#alias/*`) — they're handled natively by Node and bundlers.

**Duplicated `@types/*` versions.** A monorepo with two versions of `@types/react` in different packages will produce confusing "two `React` types" errors. Use a workspace dependency hoist or pin in the root.

**Circular project references.** TS will refuse and tell you. Restructure — usually means a "shared" package both A and B import from, never A → B → A.

**`tsbuildinfo` corruption.** If incremental builds get weird, `tsc --build --clean` fixes it.

**Editor lag.** Often the TS server is exhausting CPU on a complex type. Restart the TS server (`Cmd+Shift+P → TypeScript: Restart TS Server`). If it persists, profile with `--generateTrace` and simplify the offending types.

---

## 7. Common mistakes & gotchas

**Mistake 1: Putting `outDir` and `rootDir` wrong in monorepos.** `rootDir` must contain every input file. Reaching outside (e.g., importing `../other-package/src`) will error or emit weird paths. Use proper package boundaries.

**Mistake 2: Adopting project references everywhere reflexively.** They have setup cost. Use only when build time hurts or you need clean library boundaries.

**Mistake 3: Migrating in one PR.** A "convert everything to TS" PR is unreviewable. File by file, behind feature flags if needed.

**Mistake 4: Skipping the JSDoc phase.** Lifting JS with JSDoc is faster than rewriting. Many teams stay there permanently and ship just fine.

**Mistake 5: Letting `any` count as "typed."** `type-coverage --strict` excludes `any`. Use it as the real metric.

**Mistake 6: Treating tsc as a bundler.** It transpiles single files. For browser bundles, use Vite/esbuild/webpack. For Node libraries, `tsc` is fine but consider `tsup` for ESM+CJS dual builds.

---

## 🎯 Key Takeaways

- One `tsconfig.base.json`, every package extends — your future self thanks you.
- Project references unlock incremental builds across packages; reach for them when build time becomes a real cost.
- Diagnose slowness with `--extendedDiagnostics` and `--generateTrace`; the bottleneck is almost always type-checking, not emit.
- Migrate JS in phases: `allowJs` → JSDoc + `checkJs` → file-by-file rename → strict flag rollout. Never big-bang.
- Production TS is mostly about boundaries: package boundaries, build boundaries (typecheck vs bundle), and migration boundaries (one file at a time).

You're done. You now have a working mental map of TypeScript from compiler internals to production rollout. Keep [Type Challenges](https://github.com/type-challenges/type-challenges) bookmarked — the gym never closes.

*← [15 — Real-world patterns](./15_real_world_patterns.md) | [00 — Roadmap](./00_roadmap.md)*
