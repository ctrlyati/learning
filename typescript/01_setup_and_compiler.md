# 01 — Setup & the TypeScript Compiler

> **Goal:** Get a fast, reliable TypeScript dev loop and understand exactly what `tsc` does to your code.

---

## 1. The mental model — what TypeScript actually is

TypeScript is two things bolted together:

1. A **type checker** (`tsc --noEmit`) that reads your code and reports errors.
2. A **transpiler** that strips types and downlevels modern JS to whatever target you choose.

These are independent. `tsc` will happily emit JavaScript even when there are type errors (unless you set `noEmitOnError`). Many modern toolchains (esbuild, swc, Bun, Vite) only do step 2 — they transpile but don't type-check. Type-checking is then done separately, often in CI, by running `tsc --noEmit`.

Install once:

```bash
npm install -g typescript
tsc --version    # 5.x at time of writing
```

Or per-project (preferred — pin the version):

```bash
mkdir my-app && cd my-app
npm init -y
npm install --save-dev typescript @types/node
npx tsc --init
```

That last command generates a `tsconfig.json` — your project's contract with the compiler.

---

## 2. tsconfig essentials — the flags that actually matter

A minimal, modern, sane `tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "lib": ["ES2022"],
    "outDir": "./dist",
    "rootDir": "./src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true,
    "declaration": true,
    "sourceMap": true,
    "noUncheckedIndexedAccess": true
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist"]
}
```

What each flag does:

- **`target`** — the JS version `tsc` emits. `ES2022` is fine for Node 18+. For browsers via a bundler, this often doesn't matter (the bundler controls output).
- **`module`** — output module system. `NodeNext` honors your `package.json`'s `"type": "module"` field. Use `ESNext` if a bundler handles modules.
- **`moduleResolution`** — how `import 'foo'` is resolved. Pair with `module`: `NodeNext` ↔ `NodeNext`, `Bundler` ↔ `ESNext`.
- **`lib`** — which built-in type definitions are available. Add `"DOM"` for browser code.
- **`strict`** — enables eight strictness flags as a bundle. Module 13 covers each.
- **`esModuleInterop`** — lets you `import fs from 'fs'` instead of `import * as fs from 'fs'`. Always on.
- **`skipLibCheck`** — don't type-check `node_modules/.../*.d.ts`. Massive speedup, almost no downside.
- **`noUncheckedIndexedAccess`** — `arr[0]` becomes `T | undefined`. Catches a huge class of bugs. Worth the friction.
- **`declaration`** — emit `.d.ts` files alongside `.js`. Required if you publish a library.

---

## 3. Running TypeScript — the runner zoo

You almost never want to manually `tsc` then `node dist/index.js` in development. Pick a runner:

### `tsx` (recommended for Node scripts)

```bash
npm install --save-dev tsx
npx tsx src/index.ts
npx tsx watch src/index.ts   # auto-reload
```

Built on esbuild. Fast. ESM-first. **Does not type-check** — pair with `tsc --noEmit` in a `npm run check` script.

### `ts-node` (older, more configurable)

```bash
npm install --save-dev ts-node
npx ts-node src/index.ts
```

Slower than `tsx` but supports the full TS language service. Most new projects pick `tsx`.

### `bun` (all-in-one)

```bash
bun run src/index.ts
```

Bun runs `.ts` natively with no setup. Excellent for scripts and prototypes. Type-checking is still your job (`tsc --noEmit`).

### Plain `tsc --watch`

```bash
npx tsc --watch
node dist/index.js
```

Two terminals. Old-school. Useful when you want exactly what production will run.

---

## 4. Practical application — a real starter

```bash
mkdir hello-ts && cd hello-ts
npm init -y
npm pkg set type=module
npm install --save-dev typescript tsx @types/node
npx tsc --init
mkdir src
```

`src/index.ts`:

```ts
type Greeting = {
  readonly name: string;
  readonly enthusiasm: 1 | 2 | 3;
};

function greet({ name, enthusiasm }: Greeting): string {
  return `Hello, ${name}${"!".repeat(enthusiasm)}`;
}

console.log(greet({ name: "TypeScript", enthusiasm: 3 }));
```

`package.json` scripts:

```json
{
  "scripts": {
    "dev":   "tsx watch src/index.ts",
    "build": "tsc",
    "start": "node dist/index.js",
    "check": "tsc --noEmit"
  }
}
```

Now `npm run dev` gives you a hot-reloading dev loop. `npm run check` is your fast type-check (run it in CI). `npm run build && npm start` mirrors production.

Open `dist/index.js` after `npm run build` and look — types are gone. Just plain JS. That is *the* central insight.

---

## 5. Common mistakes & gotchas

**Mistake 1: Not setting `strict: true`.** The default `tsconfig.json` from `tsc --init` has it on, but tutorials often turn it off. Don't. You will write better code with it on from day one.

**Mistake 2: Confusing `module` and `target`.** `target` is the JS *syntax* version (arrow functions, `??`, etc.). `module` is the *module system* (`require` vs `import`). They're orthogonal.

**Mistake 3: Editing `.js` output instead of `.ts`.** Always edit source. Add `dist/` to `.gitignore`.

**Mistake 4: Ignoring `tsc` errors because "the code runs."** It runs because `tsc` emitted JS anyway. Use `noEmitOnError: true` if you want hard failures.

**Mistake 5: Mixing `module: CommonJS` with `"type": "module"` in `package.json`.** This produces ESM-flavored imports compiled to CJS that Node refuses to load. Pick a lane: full ESM (`"type": "module"` + `module: NodeNext`) or full CJS.

**Mistake 6: Running `tsx` and assuming you have type safety.** `tsx` skips type-checking for speed. You **must** run `tsc --noEmit` separately, ideally as a pre-commit hook and in CI.

---

## 🎯 Key Takeaways

- TypeScript = type checker + transpiler; modern toolchains often split these for speed.
- A good `tsconfig.json` with `strict`, `noUncheckedIndexedAccess`, and `skipLibCheck` is table stakes.
- Use `tsx` for the inner dev loop, `tsc --noEmit` for verification, and `tsc` only for builds (or skip it entirely with a bundler).
- Types are erased — never rely on them at runtime.
- Pin TypeScript per-project so upgrades are deliberate; library authors must ship `.d.ts` files via `declaration: true`.

*next →* [02 — Primitive types & literals](./02_primitive_types.md)
