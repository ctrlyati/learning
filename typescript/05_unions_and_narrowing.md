# 05 — Unions, Intersections, Narrowing, Discriminated Unions

> **Goal:** Master TypeScript's killer feature — control-flow-based type narrowing — and the discriminated union pattern that powers it.

---

## 1. Unions (`|`) and intersections (`&`) — set theory for types

A union `A | B` is the **set union**: a value is one of A or B.
An intersection `A & B` is the **set intersection**: a value is both A and B at once (i.e., has all keys of both).

```ts
type Id = string | number;       // either
type WithName = { name: string };
type WithAge  = { age: number };
type Person   = WithName & WithAge;
const p: Person = { name: "Ada", age: 36 };
```

Counterintuitive moment: a **union of object types** has access only to the **shared** keys.

```ts
type Cat = { name: string; meow: () => void };
type Dog = { name: string; bark: () => void };

function speak(animal: Cat | Dog) {
  console.log(animal.name);  // ✓ both have name
  animal.meow();             // ✗ — not on Dog
}
```

To call `.meow()` you must **narrow** to `Cat` first.

---

## 2. Narrowing — TypeScript reads your code

The compiler tracks types through control flow. The standard narrowing tools:

```ts
// typeof — for primitives
function len(x: string | number): number {
  if (typeof x === "string") return x.length; // x: string here
  return x.toString().length;                 // x: number here
}

// instanceof — for classes
function format(x: Date | string): string {
  if (x instanceof Date) return x.toISOString();
  return x;
}

// in operator — for object shape
function hasName(o: { name: string } | { id: string }): string {
  return "name" in o ? o.name : o.id;
}

// equality narrows literals out
function f(x: "a" | "b" | "c") {
  if (x === "a") return; // x is "a"
  // x: "b" | "c"
}

// Truthiness narrows out null/undefined/0/""
function g(s: string | null) {
  if (s) s.toUpperCase(); // s: string
}

// Type predicates — custom narrowing
function isString(x: unknown): x is string {
  return typeof x === "string";
}
```

**`x is T` is a user-defined type guard.** TS *trusts* you — if your runtime check is wrong, narrowing will lie. Prefer `assert` functions for invariants:

```ts
function assertString(x: unknown): asserts x is string {
  if (typeof x !== "string") throw new Error("expected string");
}

function f(x: unknown) {
  assertString(x);
  x.toUpperCase(); // ✓ x: string
}
```

---

## 3. Discriminated unions — the pattern that makes TypeScript click

Add a **literal-typed tag** ("discriminant") to each variant:

```ts
type Shape =
  | { kind: "circle"; radius: number }
  | { kind: "square"; size: number }
  | { kind: "rect";   width: number; height: number };

function area(s: Shape): number {
  switch (s.kind) {
    case "circle": return Math.PI * s.radius ** 2;     // s: { kind: "circle"; radius: number }
    case "square": return s.size ** 2;
    case "rect":   return s.width * s.height;
  }
}
```

Inside each `case`, TS narrows `s` to the matching variant. No `instanceof`, no manual property checks, no casts.

This is the workhorse pattern for: API response types, Redux/Zustand actions, parser ASTs, state machines, error variants — anywhere you have "one of N shapes."

---

## 4. Exhaustiveness checking with `never`

Add a `default` branch that asserts `never`. When you add a new variant, TS forces you to handle it:

```ts
function area(s: Shape): number {
  switch (s.kind) {
    case "circle": return Math.PI * s.radius ** 2;
    case "square": return s.size ** 2;
    case "rect":   return s.width * s.height;
    default: {
      const _exhaustive: never = s;
      throw new Error(`Unhandled: ${JSON.stringify(s)}`);
    }
  }
}
```

If you add `| { kind: "triangle"; base: number; height: number }` to `Shape` and forget the case, the `_exhaustive: never = s` line breaks compilation. **Free refactor safety.** Module 14 generalizes this.

---

## 5. Practical application — a typed async result

```ts
type Loadable<T, E = Error> =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "success"; data: T }
  | { status: "error";   error: E };

function render<T>(state: Loadable<T>): string {
  switch (state.status) {
    case "idle":    return "—";
    case "loading": return "Loading…";
    case "success": return `OK: ${JSON.stringify(state.data)}`;
    case "error":   return `Error: ${state.error.message}`;
  }
}

const s1: Loadable<number> = { status: "loading" };
const s2: Loadable<number> = { status: "success", data: 42 };
// const s3: Loadable<number> = { status: "success" }; ✗ data missing
// const s4: Loadable<number> = { status: "error",   data: 1 }; ✗ wrong shape
```

Compare to the typical "loose" version with three booleans (`isLoading`, `isError`, `isSuccess`) — there are 8 boolean combinations but only 4 valid states. The discriminated union makes the other 4 unrepresentable.

---

## 6. Narrowing edge cases — control flow lies sometimes

**Re-widening on closures:**

```ts
function f(x: string | null) {
  if (x === null) return;
  // x: string
  setTimeout(() => {
    x.length; // ✗ — 'x' could be null again — closure captures original type
  });
}
```

Workaround — assign to a `const`:

```ts
function f(x: string | null) {
  if (x === null) return;
  const safe = x;
  setTimeout(() => safe.length); // ✓
}
```

**Re-widening on mutable bindings (`let`)** — mutating a `let` re-widens:

```ts
let v: string | number = "a";
v.toUpperCase(); // ✓ narrowed
v = 1;
v.toUpperCase(); // ✗ widened back
```

**Boolean filters lose narrowing without help:**

```ts
const xs = ["a", null, "b"].filter((x) => x !== null);
// xs: (string | null)[]  — TS doesn't narrow through filter callbacks by default

const ys = ["a", null, "b"].filter((x): x is string => x !== null);
// ys: string[] — explicit type predicate
```

---

## 7. Common mistakes & gotchas

**Mistake 1: Forgetting the discriminant.** Without a tag, you can't narrow:

```ts
type Bad = { radius: number } | { size: number };
function area(s: Bad) {
  s.radius; // ✗ may not exist
  if ("radius" in s) s.radius; // ✓ works but verbose
}
```

Always add `kind` / `type` / `tag`.

**Mistake 2: Using `as` to "narrow."** A cast bypasses checking. `s as Circle` lies if `s` isn't a circle. Use type guards or discriminants.

**Mistake 3: Type predicates without runtime check correctness.**

```ts
function isUser(x: unknown): x is User {
  return true; // compiles, completely wrong
}
```

TS trusts you. Get the runtime logic right or use a schema validator.

**Mistake 4: Intersecting incompatible primitives.**

```ts
type X = string & number; // never
```

TS reduces impossible intersections to `never`. If you see `never`, you intersected incompatible things.

**Mistake 5: Ignoring `void` returns in switch arms.** With `noFallthroughCasesInSwitch` off, missing `break`/`return` silently flows through. Turn the flag on.

**Mistake 6: Hand-rolling exhaustiveness checks inconsistently.** Pick one helper:

```ts
function assertNever(x: never): never {
  throw new Error(`Unexpected: ${JSON.stringify(x)}`);
}
```

Use it everywhere.

---

## 🎯 Key Takeaways

- Unions express alternatives; intersections combine shapes — both follow set semantics.
- Narrowing is control-flow-aware: master `typeof`, `instanceof`, `in`, equality, truthiness, and type predicates.
- Discriminated unions + exhaustive `switch` are the single most powerful pattern in TS — use them everywhere you'd reach for a class hierarchy or boolean flags.
- `never` is your refactor safety net — it catches every unhandled variant the moment you add one.
- Closures and `let` reassignments can re-widen narrowed types; assign to `const` or extract a function when needed.

*← [04 — Objects](./04_objects.md) | [next →](./06_generics_fundamentals.md)*
