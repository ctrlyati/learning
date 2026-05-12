# 09 — Utility Types (and Writing Your Own)

> **Goal:** Know every built-in utility type cold, and be able to write your own for any shape you need.

---

## 1. The built-in catalog

### Object transformations

```ts
type User = { id: string; name: string; age?: number };

Partial<User>;              // { id?: string; name?: string; age?: number }
Required<User>;             // { id: string; name: string; age: number }
Readonly<User>;             // { readonly id: string; ... }
Pick<User, "id" | "name">;  // { id: string; name: string }
Omit<User, "age">;          // { id: string; name: string }
Record<"a" | "b", number>;  // { a: number; b: number }
```

### Union manipulation

```ts
type T = "a" | "b" | "c";

Exclude<T, "a">;        // "b" | "c"
Extract<T, "a" | "x">;  // "a"
NonNullable<string | null | undefined>; // string
```

### Function introspection

```ts
type F = (x: number, s: string) => boolean;

Parameters<F>;        // [number, string]
ReturnType<F>;        // boolean
ConstructorParameters<typeof Date>; // [] | [string | number | Date] | ...
InstanceType<typeof Date>;          // Date
ThisParameterType<F>;
OmitThisParameter<F>;
```

### Async unwrap

```ts
Awaited<Promise<Promise<number>>>; // number — recursively unwraps
```

### String

```ts
Uppercase<"hi">;     // "HI"
Lowercase<"HI">;     // "hi"
Capitalize<"hi">;    // "Hi"
Uncapitalize<"Hi">;  // "hi"
```

Memorize this list — they are the working vocabulary of every TS codebase.

---

## 2. How they're implemented (read-along)

```ts
type Partial<T>  = { [K in keyof T]?: T[K] };
type Required<T> = { [K in keyof T]-?: T[K] };
type Readonly<T> = { readonly [K in keyof T]: T[K] };

type Pick<T, K extends keyof T> = { [P in K]: T[P] };

type Omit<T, K extends PropertyKey> = {
  [P in keyof T as P extends K ? never : P]: T[P];
};

type Record<K extends PropertyKey, V> = { [P in K]: V };

type Exclude<T, U> = T extends U ? never : T;
type Extract<T, U> = T extends U ? T : never;
type NonNullable<T> = T extends null | undefined ? never : T;

type Parameters<F> = F extends (...a: infer A) => any ? A : never;
type ReturnType<F> = F extends (...a: any[]) => infer R ? R : never;
```

If those read like normal code now, Modules 06–08 did their job.

---

## 3. Writing your own — common patterns

### `DeepPartial`

```ts
type DeepPartial<T> = T extends object
  ? { [K in keyof T]?: DeepPartial<T[K]> }
  : T;

type Config = { db: { host: string; port: number }; flags: string[] };
type CfgPatch = DeepPartial<Config>;
// { db?: { host?: string; port?: number }; flags?: string[] | undefined }
```

Edge case: the `T extends object` branch matches arrays and functions too. Refine if needed:

```ts
type DeepPartial<T> = T extends (...args: any[]) => any
  ? T
  : T extends Array<infer U>
    ? Array<DeepPartial<U>>
    : T extends object
      ? { [K in keyof T]?: DeepPartial<T[K]> }
      : T;
```

### `Prettify` — flatten intersections in editor hovers

```ts
type Prettify<T> = { [K in keyof T]: T[K] } & {};

type Ugly = { a: string } & { b: number };
type Nice = Prettify<Ugly>; // { a: string; b: number } in tooltips
```

Pure ergonomics. Costs nothing.

### `RequireAtLeastOne` / `RequireOnlyOne`

```ts
type RequireAtLeastOne<T, K extends keyof T = keyof T> =
  Omit<T, K> & { [P in K]-?: Required<Pick<T, P>> & Partial<Pick<T, Exclude<K, P>>> }[K];

type Input = { email?: string; phone?: string; name: string };
type ContactRequired = RequireAtLeastOne<Input, "email" | "phone">;

const c1: ContactRequired = { name: "A", email: "a@b" };           // ✓
const c2: ContactRequired = { name: "A", phone: "555" };           // ✓
const c3: ContactRequired = { name: "A", email: "a@b", phone: "5" }; // ✓
// const c4: ContactRequired = { name: "A" };                      ✗ — neither
```

### `ValueOf`

```ts
type ValueOf<T> = T[keyof T];

type User = { id: string; age: number };
type V = ValueOf<User>; // string | number
```

### `UnionToIntersection` (the famous one)

```ts
type UnionToIntersection<U> =
  (U extends any ? (x: U) => void : never) extends (x: infer I) => void ? I : never;

type X = UnionToIntersection<{ a: 1 } | { b: 2 }>; // { a: 1 } & { b: 2 }
```

Uses contravariance of function parameters. Used in plugin systems and shape merging.

### `Brand`

A tiny utility that creates nominal types out of structural ones:

```ts
type Brand<T, B> = T & { readonly __brand: B };

type UserId  = Brand<string, "UserId">;
type OrderId = Brand<string, "OrderId">;

const u = "u1" as UserId;
const o = "o1" as OrderId;

function getUser(id: UserId) {}
getUser(u);     // ✓
// getUser(o);  ✗ OrderId not assignable to UserId
// getUser("x"); ✗ raw string not assignable
```

Module 15 expands on branded types as a pattern.

---

## 4. Practical application — a typed update API

You want a function that updates an entity with a partial patch, but only certain keys are mutable:

```ts
type Mutable<T, K extends keyof T> = T & { [P in K]: T[P] };

type Entity = {
  readonly id: string;
  readonly createdAt: Date;
  name: string;
  email: string;
  status: "active" | "suspended";
};

type UpdatableKeys = "name" | "email" | "status";
type UpdatePatch = Partial<Pick<Entity, UpdatableKeys>>;

function update(entity: Entity, patch: UpdatePatch): Entity {
  return { ...entity, ...patch };
}

const e: Entity = { id: "1", createdAt: new Date(), name: "Ada", email: "a@b", status: "active" };
update(e, { name: "Ada Lovelace" });               // ✓
update(e, { status: "suspended" });                // ✓
// update(e, { id: "2" });                         ✗ id not in UpdatableKeys
// update(e, { createdAt: new Date() });           ✗ same
```

The shape of the update function is **inferred from the entity** plus a single `UpdatableKeys` declaration. Add a new mutable field by adding it to the union. The compiler propagates everywhere.

---

## 5. Composing utilities for real specs

```ts
type ApiResponse<T> = {
  data: T;
  meta: { requestedAt: string; took: number };
};

type WithoutMeta<T> = T extends ApiResponse<infer D> ? D : T;
type ApiPatch<T> = DeepPartial<WithoutMeta<T>>;

type UserResp = ApiResponse<User>;
type UserPatch = ApiPatch<UserResp>;
```

You'll find yourself stacking 3–5 utilities deep in mature codebases. The `Prettify` trick is invaluable for keeping editor hovers readable.

---

## 6. Common mistakes & gotchas

**Mistake 1: Confusing `Pick` and `Omit`.** `Pick<T, K>` keeps `K`. `Omit<T, K>` removes `K`. They're complements.

**Mistake 2: `Omit` doesn't distribute over unions.**

```ts
type U = { kind: "a"; x: 1 } | { kind: "b"; y: 2 };
type O = Omit<U, "kind">; // { x: 1 } | { y: 2 }   ❌ wait, this actually works in modern TS

type DistributiveOmit<T, K extends PropertyKey> = T extends any ? Omit<T, K> : never;
```

The built-in `Omit` *is* now distributive in recent TS versions, but if you want to be explicit, write `DistributiveOmit`.

**Mistake 3: Not using `Prettify` for hover legibility.** A wall of `& X & Y & Z` in tooltips is exhausting. `Prettify<T>` collapses it.

**Mistake 4: Hand-rolling things that exist.** Check the catalog before writing your own. `NonNullable`, `Awaited`, `ConstructorParameters` are easy to forget.

**Mistake 5: Recursive utilities without guards.** `DeepPartial` over a self-referencing type can hit recursion limits. Use class type guards or memoization patterns where applicable.

**Mistake 6: Branded types without runtime construction.** Just casting `as UserId` is brittle. Wrap construction in a function: `function userId(s: string): UserId { /* validate */ return s as UserId; }`.

---

## 🎯 Key Takeaways

- Memorize `Partial`, `Required`, `Readonly`, `Pick`, `Omit`, `Record`, `Exclude`, `Extract`, `NonNullable`, `Parameters`, `ReturnType`, `Awaited` — they appear in every codebase.
- Reading their implementations in this module reinforces mapped + conditional + `infer` from prior modules.
- Custom utilities like `DeepPartial`, `Prettify`, `RequireAtLeastOne`, `Brand` are reusable across projects — keep a personal `types/utility.ts`.
- Always check whether a built-in utility solves your problem before writing one.
- `Prettify<T> = { [K in keyof T]: T[K] } & {}` is the cheapest ergonomic win in the language.

*← [08 — Mapped & template literal types](./08_mapped_and_template_literal.md) | [next →](./10_classes_and_decorators.md)*
