# 07 — Generics Advanced: Constraints, Conditional Types, `infer`

> **Goal:** Move from "I can write `<T>`" to "I can build library-grade typed APIs."

---

## 1. Constraints — restricting what `T` can be

`T extends X` means "T must be assignable to X."

```ts
function longest<T extends { length: number }>(a: T, b: T): T {
  return a.length >= b.length ? a : b;
}

longest("hello", "hi");           // ✓ strings have length
longest([1, 2, 3], [4]);          // ✓ arrays have length
// longest(1, 2);                 ✗ numbers have no length
```

The constraint also enables property access on `T` inside the function — without it, you can't touch `.length`.

A constraint **does not widen** the inferred type:

```ts
function head<T extends readonly unknown[]>(xs: T): T[0] {
  return xs[0];
}

const x = head([1, "a", true] as const); // x: 1 — literal preserved
```

`keyof` constraints are the most common pattern:

```ts
function get<T, K extends keyof T>(obj: T, key: K): T[K] {
  return obj[key];
}

const u = { id: 1, name: "Ada" };
const id = get(u, "id");     // number
const nm = get(u, "name");   // string
// get(u, "email");          ✗ not a key
```

---

## 2. Conditional types — `T extends U ? X : Y`

A type-level ternary:

```ts
type IsString<T> = T extends string ? true : false;

type A = IsString<"hi">;   // true
type B = IsString<42>;     // false
```

When the `T` being tested is a **naked type parameter** and you pass a **union**, conditional types **distribute** over the union:

```ts
type ToArray<T> = T extends any ? T[] : never;

type X = ToArray<string | number>;
//   ≡ ToArray<string> | ToArray<number>
//   ≡ string[] | number[]
```

To **disable distribution**, wrap both sides in tuples:

```ts
type ToArrayNoDist<T> = [T] extends [any] ? T[] : never;

type Y = ToArrayNoDist<string | number>; // (string | number)[]
```

This is critical when you want to operate on the union *as a whole* (e.g., `IsUnion<T>` checks).

---

## 3. `infer` — pattern-matching inside conditionals

`infer X` lets you capture a type inside a conditional pattern:

```ts
type ReturnTypeOf<F> = F extends (...args: any[]) => infer R ? R : never;

type A = ReturnTypeOf<() => string>;            // string
type B = ReturnTypeOf<(n: number) => boolean>;  // boolean
type C = ReturnTypeOf<"not a function">;        // never
```

This is how the built-in `ReturnType<T>` works.

More patterns:

```ts
type FirstArg<F> = F extends (first: infer A, ...rest: any[]) => any ? A : never;
type ArrayElement<T> = T extends (infer E)[] ? E : never;
type PromiseValue<T> = T extends Promise<infer V> ? V : T; // unwrap
type Awaited2<T> = T extends Promise<infer V> ? Awaited2<V> : T; // recursive

type X = ArrayElement<number[]>;    // number
type Y = PromiseValue<Promise<string>>; // string
type Z = Awaited2<Promise<Promise<number>>>; // number
```

`infer` can appear multiple times and even with constraints (`infer R extends string`).

---

## 4. Recursive conditional types — bounded recursion

TS allows conditional types to recurse, with an internal limit (~50 levels). Used for things like deep types and tuple manipulation:

```ts
type DeepReadonly<T> = T extends (...args: any[]) => any
  ? T
  : T extends object
    ? { readonly [K in keyof T]: DeepReadonly<T[K]> }
    : T;

type Config = { servers: { url: string; ports: number[] }[] };
type FrozenConfig = DeepReadonly<Config>;
// every nested property becomes readonly
```

```ts
// Reverse a tuple
type Reverse<T extends readonly unknown[]> =
  T extends readonly [infer Head, ...infer Tail]
    ? [...Reverse<Tail>, Head]
    : [];

type R = Reverse<[1, 2, 3]>; // [3, 2, 1]
```

The `[infer Head, ...infer Tail]` pattern is the type-level equivalent of `[head, ...tail] = arr`.

---

## 5. Practical application — a typed `Promise.allSettled` wrapper

We want a function that takes a tuple of promises and returns a tuple of results, *preserving each element type*:

```ts
type Awaited2<T> = T extends Promise<infer V> ? Awaited2<V> : T;

type SettledTuple<T extends readonly unknown[]> = {
  -readonly [K in keyof T]:
    | { status: "ok";  value: Awaited2<T[K]> }
    | { status: "err"; error: unknown };
};

async function settleAll<T extends readonly Promise<unknown>[]>(
  promises: readonly [...T],
): Promise<SettledTuple<T>> {
  const results = await Promise.allSettled(promises);
  return results.map((r) =>
    r.status === "fulfilled"
      ? { status: "ok",  value: r.value }
      : { status: "err", error: r.reason },
  ) as SettledTuple<T>;
}

const out = await settleAll([
  Promise.resolve(1),
  Promise.resolve("hi"),
  Promise.resolve(true),
] as const);

// out is typed as:
// [
//   { status: "ok"; value: number  } | { status: "err"; error: unknown },
//   { status: "ok"; value: string  } | { status: "err"; error: unknown },
//   { status: "ok"; value: boolean } | { status: "err"; error: unknown },
// ]

if (out[0].status === "ok") {
  out[0].value.toFixed(2); // ✓ number
}
```

Each tuple slot keeps its own value type. This is a real-world payoff of mapped + conditional + `infer`.

---

## 6. Variance, bivariance, and `extends` direction

`A extends B` means "A is a subtype of B" (A is **more specific**). Useful intuition:

- Function **parameters** are *contravariant* — fewer accepted = more specific.
- Function **return types** are *covariant* — more specific = more specific.
- Mutable containers are *invariant* — `Box<Cat>` and `Box<Animal>` are unrelated.

Real consequence:

```ts
type Animal = { name: string };
type Cat = { name: string; meow: () => void };

let a: (x: Animal) => void = (x) => {};
let c: (x: Cat) => void = (x) => x.meow();

a = c; // ✗ — c expects more (Cat), so it can't stand in where Animal is allowed
c = a; // ✓ — a accepts any Animal, so it works where Cat is expected
```

Memorize the direction once, never confuse yourself again.

---

## 7. Common mistakes & gotchas

**Mistake 1: Distributive conditional types when you didn't want them.**

```ts
type NonNull<T> = T extends null | undefined ? never : T;
type X = NonNull<string | null>; // string ✓ — distribution worked for us

type IsUnion<T> = [T] extends [string] ? false : true; // wrap to *prevent* distribution
```

Know when you want each.

**Mistake 2: `any` in conditional positions.** `any` matches everything *and* its negation. Avoid it inside `extends` — use `unknown` or specific types.

**Mistake 3: Recursion blowing up.** Deep recursive types hit the recursion limit. Use tail-recursion-friendly patterns (accumulator tuples) and avoid quadratic growth.

**Mistake 4: `infer` in the wrong slot.** `infer` only works inside the *extends* clause of a conditional, not anywhere. `T extends Promise<infer R>` ✓; `Promise<infer R>` alone ✗.

**Mistake 5: Constraining with `extends object` thinking it means "plain object."** Same problem as everywhere else — use `Record<string, unknown>` or a more precise shape.

**Mistake 6: Reaching for advanced types when a function overload would do.** Type-level programming is powerful but harder to read. Prefer the simpler tool when behavior is straightforward.

---

## 🎯 Key Takeaways

- Constraints (`T extends X`) restrict and unlock — both at once.
- Conditional types are a ternary at the type level; learn the distribution rule and how to opt out.
- `infer` is type-level pattern matching — the same shape lets you extract return types, array elements, promise values, and tuple parts.
- Recursive conditional types power deep utilities (`DeepReadonly`, `Reverse`) but watch the recursion limit.
- These tools are how libraries like tRPC, Prisma, and zod give you end-to-end inference — knowing them lets you read and contribute to that level of code.

*← [06 — Generics fundamentals](./06_generics_fundamentals.md) | [next →](./08_mapped_and_template_literal.md)*
