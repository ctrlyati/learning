# 06 — Generics Fundamentals

> **Goal:** Write functions, classes, and types that work over any type — without losing type information.

---

## 1. Mental model — generics are *type-level parameters*

A normal function takes values:

```ts
function identity(x: number): number { return x; }
```

A generic function takes values **and** types:

```ts
function identity<T>(x: T): T { return x; }

const a = identity<string>("hi"); // a: string
const b = identity(42);            // b: number — T inferred
```

`T` is a **type parameter**. The caller (or inference) supplies it. The body has no idea what `T` is — only that input and output share it.

The convention is single uppercase letters: `T`, `U`, `K`, `V`. Use longer names for clarity in big signatures: `<TInput, TOutput>`.

---

## 2. Why generics — the alternative is bad

Without generics:

```ts
function firstAny(xs: any[]): any { return xs[0]; }
const x = firstAny([1, 2, 3]); // x: any — types lost
x.toUpperCase();               // ✓ compiles, ✗ explodes
```

With generics:

```ts
function first<T>(xs: T[]): T | undefined { return xs[0]; }
const x = first([1, 2, 3]); // x: number | undefined
x?.toUpperCase();           // ✗ — toUpperCase doesn't exist on number — caught
```

Type information **flows through** the function instead of being thrown away.

---

## 3. Generic functions, types, classes, and methods

```ts
// Generic function
function pair<T, U>(a: T, b: U): [T, U] { return [a, b]; }

// Generic type
type Box<T> = { value: T };
const n: Box<number> = { value: 1 };

// Generic interface
interface Repository<T> {
  get(id: string): Promise<T | null>;
  save(item: T): Promise<void>;
}

// Generic class
class Stack<T> {
  private items: T[] = [];
  push(x: T) { this.items.push(x); }
  pop(): T | undefined { return this.items.pop(); }
  peek(): T | undefined { return this.items.at(-1); }
}

const s = new Stack<string>();
s.push("a");
const top = s.peek(); // top: string | undefined

// Method-level generic (different from class-level)
class Mapper<T> {
  constructor(private value: T) {}
  map<U>(f: (x: T) => U): Mapper<U> { return new Mapper(f(this.value)); }
}
```

Class-level `T` is fixed at construction. Method-level `U` is fresh per call.

---

## 4. Type inference — the magic

TS infers type parameters from arguments:

```ts
function map<T, U>(xs: T[], f: (x: T) => U): U[] {
  return xs.map(f);
}

const result = map([1, 2, 3], (n) => n.toString()); // result: string[]
//                  T = number,  U inferred as string
```

When inference can't pin a type, you must annotate:

```ts
function make<T>(): T[] { return []; }
const a = make();          // a: unknown[]
const b = make<number>();  // b: number[] — explicit
```

**Default type parameters** give a fallback:

```ts
type State<T = string> = { value: T };
const s1: State = { value: "ok" };       // T defaults to string
const s2: State<number> = { value: 42 };
```

---

## 5. Practical application — a typed `pluck` and a generic cache

```ts
// pluck — keyof keeps it honest
function pluck<T, K extends keyof T>(items: T[], key: K): T[K][] {
  return items.map((item) => item[key]);
}

const users = [
  { id: 1, name: "Ada" },
  { id: 2, name: "Grace" },
];
const names = pluck(users, "name"); // string[]
const ids   = pluck(users, "id");   // number[]
// pluck(users, "email"); ✗ "email" is not keyof T
```

```ts
// LRU-ish cache parameterized by key and value
class Cache<K, V> {
  private map = new Map<K, V>();
  constructor(private maxSize: number) {}

  get(key: K): V | undefined { return this.map.get(key); }

  set(key: K, value: V): void {
    if (this.map.size >= this.maxSize) {
      const firstKey = this.map.keys().next().value as K;
      this.map.delete(firstKey);
    }
    this.map.set(key, value);
  }
}

const userCache = new Cache<string, { name: string }>(100);
userCache.set("u1", { name: "Ada" });
const u = userCache.get("u1"); // { name: string } | undefined
```

The `K extends keyof T` constraint and the `Map<K, V>` re-export are previews of constraints (Module 07) and built-in generic types.

---

## 6. Common gotchas — keeping inference happy

**Gotcha 1: Single-use generics that should just be `unknown`.**

```ts
// This is a generic for no reason — T is used once
function log<T>(x: T): void { console.log(x); }
// Equivalent to:
function log(x: unknown): void { console.log(x); }
```

A generic only earns its keep when the type **flows from one position to another**.

**Gotcha 2: Generic constraints widening the result.**

```ts
function box<T extends string>(x: T): T { return x; }
const a = box("hi");          // a: "hi" — literal preserved
const b = box<string>("hi");  // b: string — widened by explicit annotation
```

Be careful with explicit type arguments — they often widen.

**Gotcha 3: Inference into callbacks.** Callback parameter types flow from the generic — don't annotate them or you break it:

```ts
[1,2,3].map(x => x.toFixed());           // x: number ✓
[1,2,3].map((x: number) => x.toFixed()); // also ✓ but redundant
```

**Gotcha 4: Generic classes don't enforce variance.** A `Box<Cat>` is assignable to `Box<Animal>` if `Cat extends Animal`, but TS won't always catch unsafe writes through aliases. Be cautious with mutable generic containers.

**Gotcha 5: Forgetting that `T[]` and `Array<T>` are identical.** Pick one style for consistency.

**Gotcha 6: Trying to do runtime reflection on `T`.** You can't. `T` is erased. Pass a runtime tag (constructor, schema, key) if you need to act on the type at runtime:

```ts
function instantiate<T>(Ctor: new () => T): T {
  return new Ctor();
}
```

---

## 7. The `<T>` parsing pitfall in `.tsx` files

In `.tsx`, `<T>(x: T) => x` is parsed as JSX. Workarounds:

```tsx
const id1 = <T,>(x: T) => x;          // trailing comma
const id2 = <T extends unknown>(x: T) => x; // constraint disambiguates
function id3<T>(x: T): T { return x; } // function declarations are fine
```

---

## 🎯 Key Takeaways

- A generic earns its keep only when the type parameter is used in **two or more positions** (input and output, or two inputs).
- Lean on inference; explicit type arguments often widen and lose information.
- `keyof T` constraints are the gateway drug to safe property access — Module 07 takes them further.
- Class-level vs method-level type parameters serve different purposes; understand both.
- Generics are erased at runtime — pass a constructor, schema, or discriminant if you need runtime type info.

*← [05 — Unions & narrowing](./05_unions_and_narrowing.md) | [next →](./07_generics_advanced.md)*
