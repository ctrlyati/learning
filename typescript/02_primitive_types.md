# 02 — Primitive Types, Arrays, Tuples, Literals, `type` vs `interface`

> **Goal:** Build the vocabulary you'll use in every `.ts` file — and understand the two ways to name a type.

---

## 1. Primitives — the seven you actually use

```ts
const name: string = "Ada";
const age: number = 36;
const isActive: boolean = true;
const id: bigint = 123n;
const sym: symbol = Symbol("k");
const nothing: null = null;
const missing: undefined = undefined;
```

Three more pseudo-types you must know:

```ts
let any_: any;        // opt out of type checking — avoid
let unknown_: unknown; // opt out of *using* — must narrow first
function fail(): never { throw new Error(); } // returns nothing, ever
```

`any` vs `unknown` — the most important distinction:

```ts
const a: any = JSON.parse("{}");
a.foo.bar.baz();         // compiles, may explode

const u: unknown = JSON.parse("{}");
u.foo;                   // ✗ Object is of type 'unknown'
if (typeof u === "object" && u !== null && "foo" in u) {
  u.foo;                 // ✓ narrowed
}
```

**Rule:** `unknown` is `any` with seatbelts. Use it everywhere you'd be tempted to use `any`.

---

## 2. Arrays and tuples

```ts
const xs: number[] = [1, 2, 3];
const ys: Array<number> = [1, 2, 3];   // identical
const zs: ReadonlyArray<number> = [1]; // can't push, pop, sort, etc.
```

Tuples are fixed-length, position-typed arrays:

```ts
const point: [number, number] = [10, 20];
const named: [name: string, age: number] = ["Ada", 36]; // labels, runtime-erased

// Variadic + rest tuples
type HttpCall = [method: "GET" | "POST", url: string, ...headers: string[]];
const call: HttpCall = ["GET", "/api", "Accept: json", "Auth: token"];
```

The `as const` assertion freezes a tuple's literal types:

```ts
const a = [1, 2, 3];          // number[]
const b = [1, 2, 3] as const; // readonly [1, 2, 3]
```

This single trick unlocks half of advanced TS. Module 08 leans on it heavily.

---

## 3. Literal types and unions of literals

A literal type is a type with exactly one value:

```ts
type Yes = "yes";
const a: Yes = "yes";   // ✓
const b: Yes = "no";    // ✗
```

Combine with `|` to form a finite enum-like type without using `enum`:

```ts
type LogLevel = "debug" | "info" | "warn" | "error";

function log(level: LogLevel, msg: string) { /* ... */ }

log("info", "ok");     // ✓
log("INFO", "ok");     // ✗ autocomplete saves you
```

This pattern — **string union over enum** — is the modern idiom. `enum` has runtime cost, awkward semantics, and doesn't tree-shake well. Reach for `as const` objects instead when you need runtime values:

```ts
const LogLevel = {
  Debug: "debug",
  Info:  "info",
  Warn:  "warn",
  Error: "error",
} as const;

type LogLevel = typeof LogLevel[keyof typeof LogLevel];
// → "debug" | "info" | "warn" | "error"
```

`typeof` and `keyof` are introduced more carefully in Module 08, but get used to seeing them now.

---

## 4. `type` vs `interface` — pick a default and move on

Both name a shape:

```ts
interface User {
  id: string;
  name: string;
}

type User = {
  id: string;
  name: string;
};
```

They are **almost** interchangeable. Real differences:

| Capability | `interface` | `type` |
|---|---|---|
| Object shape | ✓ | ✓ |
| Extension | `extends` | `&` (intersection) |
| Declaration merging | ✓ (multiple decls combine) | ✗ |
| Unions | ✗ | ✓ |
| Tuples | ✗ | ✓ |
| Mapped/conditional types | ✗ | ✓ |
| Primitive aliasing | ✗ | ✓ (`type ID = string`) |

Practical guidance:

- **Default to `type`.** It does everything.
- **Use `interface` for public APIs of a library** when consumers might want declaration merging (e.g., extending `Express.Request`).
- Don't mix idiosyncratically across one codebase. Pick one, document it.

Declaration merging in action — this is why libraries use `interface`:

```ts
// in your code
declare global {
  namespace Express {
    interface Request {
      user?: { id: string };  // merged into the existing interface
    }
  }
}
```

You cannot do this with `type`.

---

## 5. Practical application — modeling a typed config

```ts
type Env = "dev" | "staging" | "prod";

type DatabaseConfig = {
  readonly host: string;
  readonly port: number;
  readonly ssl: boolean;
};

type AppConfig = {
  readonly env: Env;
  readonly db: DatabaseConfig;
  readonly featureFlags: ReadonlyArray<string>;
  readonly version: `${number}.${number}.${number}`; // template literal type
};

const config: AppConfig = {
  env: "prod",
  db: { host: "db.example.com", port: 5432, ssl: true },
  featureFlags: ["new-checkout", "dark-mode"] as const,
  version: "1.4.2",
};

// Compile-time errors:
// config.env = "dev";              ✗ readonly
// config.featureFlags.push("x");   ✗ ReadonlyArray
// const bad: AppConfig = { ..., version: "1.4" }; ✗ template literal mismatch
```

Every illegal mutation is caught at compile time. The runtime cost is zero.

---

## 6. Common mistakes & gotchas

**Mistake 1: Type widening.** TS infers the *widest* useful type by default.

```ts
const obj = { kind: "circle", radius: 5 }; // { kind: string; radius: number }
type Shape = { kind: "circle"; radius: number } | { kind: "square"; size: number };
const s: Shape = obj; // ✗ "string" not assignable to "circle"
```

Fix with `as const`:

```ts
const obj = { kind: "circle", radius: 5 } as const;
const s: Shape = obj; // ✓
```

**Mistake 2: `const` ≠ literal type for objects.** `const` only freezes the binding. The object's properties are still inferred wide. `as const` is what you actually want.

**Mistake 3: Reaching for `enum`.** Modern TS prefers string unions or `as const` objects. They're erasable, tree-shakable, and play better with JSON.

**Mistake 4: `any` slips in via `JSON.parse`.** Always type the result: `JSON.parse(s) as unknown` then narrow, or use a schema validator (Module 15).

**Mistake 5: Empty object type `{}`.** It means "any non-nullish value," not "an empty object." Use `Record<string, never>` or `object` for stricter intent.

```ts
const x: {} = "hello"; // ✓ (!) — strings are non-nullish
```

**Mistake 6: `Object` (capital O) and `Function`.** Banned by most lint rules. They mean almost nothing useful. Use specific shapes or `unknown`.

---

## 🎯 Key Takeaways

- Master `unknown` — it's the safe `any`. Use it whenever you'd be tempted by `any`.
- String literal unions beat `enum` for almost every use case. Pair with `as const` for runtime values.
- `type` is the better default; reach for `interface` when you need declaration merging or are publishing a public API.
- `as const` is the single most underused keyword combo — it preserves literal types through inference.
- `readonly` and `ReadonlyArray` are nearly free safety; default to them and add mutation only where intentional.

*← [00 — Roadmap](./00_roadmap.md) | [next →](./03_functions.md)*
