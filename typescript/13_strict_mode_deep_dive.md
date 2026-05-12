# 13 — Strict Mode Deep Dive

> **Goal:** Know exactly what each strict flag catches — and why every new project should turn them all on.

---

## 1. The strict bundle

`"strict": true` enables eight flags as a group:

```json
{
  "compilerOptions": {
    "strict": true
    // ↓ equivalent to:
    // "noImplicitAny": true,
    // "strictNullChecks": true,
    // "strictFunctionTypes": true,
    // "strictBindCallApply": true,
    // "strictPropertyInitialization": true,
    // "noImplicitThis": true,
    // "useUnknownInCatchVariables": true,
    // "alwaysStrict": true
  }
}
```

You can opt out of any one individually (`"strictNullChecks": false`) without disabling the rest. In practice: turn on `strict` and don't disable anything.

There are several **additional** strictness flags that aren't in the bundle but should be in any new project. We'll cover those at the end.

---

## 2. Each strict flag, explained

### `noImplicitAny`

Forbids parameters/variables that the compiler can't infer. Forces you to type or let inference work.

```ts
function f(x) { return x; }     // ✗ Parameter 'x' implicitly has an 'any' type
function f(x: number) { return x; } // ✓
```

### `strictNullChecks`

Treats `null` and `undefined` as their own types — they no longer slip into any value type.

```ts
const x: string = null; // ✗ with strictNullChecks
const y: string | null = null; // ✓ explicit
```

This single flag is the most valuable in the language. It eliminates 90% of "x is undefined" runtime errors.

### `strictFunctionTypes`

Function parameters are checked **contravariantly** (Module 03). Without it, TS uses bivariance for compat with old code.

```ts
type Handler<T> = (x: T) => void;

let animal: Handler<{ name: string }> = (a) => {};
let cat: Handler<{ name: string; meow(): void }> = (c) => c.meow();

cat = animal;     // ✓ — animal handles less, can stand in
animal = cat;     // ✗ with strictFunctionTypes — cat needs more than animal provides
```

Catches real bugs in callback APIs.

### `strictBindCallApply`

`bind`, `call`, `apply` get checked against the function's actual signature instead of being `any`-typed.

```ts
function add(a: number, b: number) { return a + b; }
add.call(null, 1, "2"); // ✗ — string not assignable to number
```

### `strictPropertyInitialization`

Class fields without an initializer or definite-assignment assertion (`!`) error out:

```ts
class User {
  name: string;       // ✗ — no initializer, no assignment in constructor
}

class User2 {
  name: string;
  constructor(name: string) { this.name = name; } // ✓
}

class User3 {
  name!: string;      // ✓ — definite assignment assertion (you promise it's set somewhere)
}

class User4 {
  name: string = "anon"; // ✓
}
```

### `noImplicitThis`

`this` of unknown type in functions errors instead of silently being `any`.

```ts
function f() { return this.x; } // ✗ — implicit any 'this'
function f(this: { x: number }) { return this.x; } // ✓
```

### `useUnknownInCatchVariables`

Caught errors are `unknown` instead of `any`. Forces you to narrow before use:

```ts
try { /* … */ }
catch (e) {
  // e: unknown (with the flag)
  if (e instanceof Error) console.log(e.message);
}
```

`Error` is the conventional throwable but JS can throw anything (`throw "oops"`), so `unknown` is correct.

### `alwaysStrict`

Emits `"use strict"` at the top of every file and parses in strict mode. With ES modules this is implicit — the flag mostly matters for older targets.

---

## 3. The flags **outside** `strict` you should also enable

These aren't in the bundle but pull serious weight:

### `noUncheckedIndexedAccess`

`obj[k]` and `arr[i]` become `T | undefined`. Painful at first, prevents a huge class of bugs.

```ts
const xs = [1, 2, 3];
const x = xs[10]; // number | undefined (with the flag)
```

### `noImplicitOverride`

Overriding a parent method requires `override` keyword:

```ts
class A { greet() {} }
class B extends A {
  override greet() {} // required, makes refactors safe
}
```

### `noFallthroughCasesInSwitch`

Catches missing `break`/`return`:

```ts
switch (x) {
  case "a": doA();
  case "b": doB(); // ✗ fallthrough from "a"
}
```

### `noUnusedLocals` / `noUnusedParameters`

Errors on unused symbols. Prefer ESLint's `no-unused-vars` for finer control (it can ignore `_prefixed` names).

### `exactOptionalPropertyTypes`

Distinguishes "property absent" from "property present with value `undefined`":

```ts
type T = { x?: number };
const a: T = {};               // ✓
const b: T = { x: undefined }; // ✗ with the flag — `?` means absent only
```

Stricter, more precise. Some libraries are not yet compatible.

### `noPropertyAccessFromIndexSignature`

For index-signature types, requires bracket access:

```ts
type Headers = { [k: string]: string };
const h: Headers = {};
h.foo;     // ✗ with the flag
h["foo"];  // ✓
```

Distinguishes "known property" from "lookup."

### `noImplicitReturns`

Every code path must return:

```ts
function f(x: boolean): number {
  if (x) return 1;
  // ✗ — falls off the end
}
```

---

## 4. Practical application — a "max strict" tsconfig

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "lib": ["ES2022"],
    "outDir": "./dist",
    "rootDir": "./src",
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true,
    "declaration": true,
    "sourceMap": true,
    "verbatimModuleSyntax": true,

    "strict": true,
    "noUncheckedIndexedAccess": true,
    "noImplicitOverride": true,
    "noFallthroughCasesInSwitch": true,
    "noImplicitReturns": true,
    "exactOptionalPropertyTypes": true,
    "noPropertyAccessFromIndexSignature": true
  },
  "include": ["src/**/*"]
}
```

Start every new project with this. Migrating an existing JS project? Module 16 walks through how.

---

## 5. Migrating to strict — the realistic path

You inherit a codebase with `strict: false`. Don't flip everything at once — you'll get thousands of errors and revert.

The progression that actually works:

1. Turn on **`noImplicitAny`** first. Add explicit types to function parameters until clean.
2. Turn on **`strictNullChecks`**. Most painful step. Add `?.`, `??`, narrowing, and `| null` / `| undefined` annotations as you go.
3. Turn on **`strictFunctionTypes`** and **`strictBindCallApply`** — usually low-impact.
4. Turn on **`strictPropertyInitialization`** — touches every class.
5. Turn on **`noImplicitThis`**, **`useUnknownInCatchVariables`** — usually small.
6. Turn on the extra flags one by one.

Use `// @ts-expect-error explanation` for known issues you'll fix later — better than `// @ts-ignore` because it errors when the underlying code is fixed (forcing cleanup).

---

## 6. Common mistakes & gotchas

**Mistake 1: Disabling strict per file with `// @ts-nocheck`.** Effective short-term, hides debt. Use only for generated files.

**Mistake 2: Sprinkling `as any` to silence errors.** That's not migration, that's surrender. Prefer `as unknown as T` if you must, with a comment.

**Mistake 3: Treating `strictPropertyInitialization` errors with `!`.** The definite-assignment assertion is a promise you can't keep blindly. Used right (DI containers, framework lifecycle), it's fine; used to silence the compiler, it's a bomb.

**Mistake 4: Catching `Error` directly.** With `useUnknownInCatchVariables`, you must narrow. Use a helper:

```ts
function toError(x: unknown): Error {
  return x instanceof Error ? x : new Error(String(x));
}
```

**Mistake 5: Disabling a strict flag because "the library doesn't work with it."** Often you can fix the library typings locally (Module 11 ambient declarations) instead.

**Mistake 6: Conflating ESLint and tsc strictness.** ESLint covers style and best practices; tsc covers types. Both, in CI.

---

## 🎯 Key Takeaways

- `"strict": true` is non-negotiable for new projects — it's the bedrock of TypeScript's value.
- Beyond the bundle, also turn on `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, `noImplicitOverride`, and `verbatimModuleSyntax`.
- Migrating an existing codebase is a per-flag journey, not a single switch — `// @ts-expect-error` is your friend.
- Catch blocks should use `unknown` and narrow — `useUnknownInCatchVariables` enforces it.
- Flags work together — `strictNullChecks` is what makes narrowing valuable, `strictFunctionTypes` is what makes callback APIs safe.

*← [12 — Libraries & resolution](./12_libraries_and_resolution.md) | [next →](./14_errors_and_exhaustiveness.md)*
