# 08 — Mapped Types, Template Literal Types, Key Remapping

> **Goal:** Compute new types from existing ones — programmatically.

---

## 1. Mapped types — `for...in` for the type system

```ts
type User = { id: string; name: string; age: number };

type Stringify<T> = { [K in keyof T]: string };

type S = Stringify<User>; // { id: string; name: string; age: string }
```

The `[K in keyof T]` syntax iterates over each key of `T` and produces a new property. The value type can be anything — it can reference `T[K]` to use the original:

```ts
type Optional<T>  = { [K in keyof T]?: T[K] };
type Required2<T> = { [K in keyof T]-?: T[K] };       // strip optionality
type Mutable<T>   = { -readonly [K in keyof T]: T[K] }; // strip readonly
type Frozen<T>    = { readonly [K in keyof T]: T[K] };
```

The `-?` and `-readonly` syntax **removes** modifiers; `+?` / `+readonly` adds them (the `+` is implicit).

These are the engine behind `Partial`, `Required`, `Readonly` — Module 09 covers the named utilities.

---

## 2. Key remapping with `as`

You can rename keys in a mapped type:

```ts
type Getters<T> = {
  [K in keyof T as `get${Capitalize<string & K>}`]: () => T[K];
};

type User = { id: string; age: number };
type UserGetters = Getters<User>;
// { getId: () => string; getAge: () => number }
```

You can also **filter** keys by mapping to `never`:

```ts
type RemoveField<T, Field extends keyof T> = {
  [K in keyof T as K extends Field ? never : K]: T[K];
};

type WithoutAge = RemoveField<User, "age">;
// { id: string }
```

This is how `Omit` is implemented under the hood.

---

## 3. Template literal types — string types that compute

A template literal type does at the type level what `${}` does at runtime:

```ts
type Greeting = `Hello, ${string}`;
const g1: Greeting = "Hello, Ada";    // ✓
const g2: Greeting = "Hi";            // ✗

type CssVar = `--${string}`;
const v: CssVar = "--primary-color";  // ✓
```

They distribute over unions:

```ts
type Lang = "en" | "fr";
type Region = "US" | "GB";
type Locale = `${Lang}-${Region}`;
// "en-US" | "en-GB" | "fr-US" | "fr-GB"
```

Built-in string manipulation types:

```ts
type A = Uppercase<"hello">;   // "HELLO"
type B = Lowercase<"HI">;      // "hi"
type C = Capitalize<"world">;  // "World"
type D = Uncapitalize<"World">; // "world"
```

Combined with `infer` they parse strings:

```ts
type ExtractRouteParams<Path extends string> =
  Path extends `${string}/:${infer Param}/${infer Rest}`
    ? Param | ExtractRouteParams<`/${Rest}`>
    : Path extends `${string}/:${infer Param}`
      ? Param
      : never;

type P = ExtractRouteParams<"/users/:userId/posts/:postId">;
// "userId" | "postId"
```

This is how typed router libraries give you autocomplete on `params.userId`.

---

## 4. Putting it all together — a typed event bus from event names

```ts
type EventMap = {
  "user:login":  { id: string };
  "user:logout": { id: string; reason: string };
  "post:create": { title: string };
};

type Listeners<T> = {
  [K in keyof T as `on${Capitalize<string & K>}`]: (payload: T[K]) => void;
};

type BusListeners = Listeners<EventMap>;
/*
{
  "onUser:login":  (payload: { id: string }) => void;
  "onUser:logout": (payload: { id: string; reason: string }) => void;
  "onPost:create": (payload: { title: string }) => void;
}
*/
```

(Colons in keys make this a contrived example — but illustrates the mechanic.)

A more realistic reusable utility — flip an object's keys and values:

```ts
type Flip<T extends Record<string, string>> = {
  [K in keyof T as T[K]]: K;
};

type In  = { a: "x"; b: "y" };
type Out = Flip<In>; // { x: "a"; y: "b" }
```

---

## 5. Practical application — typed environment variables

```ts
type EnvSchema = {
  DATABASE_URL: string;
  PORT: number;
  NODE_ENV: "development" | "production" | "test";
  ENABLE_DEBUG: boolean;
};

type EnvAccessor<T> = {
  [K in keyof T as `get${Capitalize<string & Lowercase<string & K>>}`]: () => T[K];
};

declare const env: EnvAccessor<EnvSchema>;

env.getDatabase_url();   // string
env.getPort();           // number
env.getNode_env();       // "development" | "production" | "test"
```

A more polished version with snake_case → camelCase:

```ts
type SnakeToCamel<S extends string> =
  S extends `${infer Head}_${infer Tail}`
    ? `${Lowercase<Head>}${Capitalize<SnakeToCamel<Tail>>}`
    : Lowercase<S>;

type X = SnakeToCamel<"DATABASE_URL">; // "databaseUrl"
type Y = SnakeToCamel<"NODE_ENV">;     // "nodeEnv"

type CamelEnv<T> = {
  [K in keyof T as SnakeToCamel<string & K>]: T[K];
};

type C = CamelEnv<EnvSchema>;
// { databaseUrl: string; port: number; nodeEnv: ...; enableDebug: boolean }
```

This pattern is real — many ORMs, GraphQL codegens, and config libraries use it to produce idiomatic JS APIs from upstream snake_case.

---

## 6. `keyof` on unions, intersections, and indexes

```ts
type A = { a: 1; b: 2 };
type B = { b: 3; c: 4 };

type K1 = keyof (A | B); // "b"           — only shared keys
type K2 = keyof (A & B); // "a" | "b" | "c" — all keys

type V = (A & B)["b"];   // 2 & 3 = never (incompatible)
```

These are the kind of corner cases you'll meet when stitching together advanced types in libraries.

---

## 7. Common mistakes & gotchas

**Mistake 1: Forgetting `& string` when remapping keys.** `keyof T` can include `number | symbol`, but template literals require `string`. Use `K extends string` constraint or `${K & string}`.

**Mistake 2: Distribution surprises in mapped types over unions.**

```ts
type Map1<T> = { [K in keyof T]: T[K] };
type X = Map1<{ a: 1 } | { b: 2 }>;
// { a: 1 } | { b: 2 } — distributes
```

If you need to merge, you have to compute differently.

**Mistake 3: Mapped types that try to add new keys.** You can only iterate existing keys — you can't synthesize new ones from thin air. Use intersections (`& { extra: T }`) for that.

**Mistake 4: Template literal explosion.** `${A}${B}${C}` with three 100-element unions = 1,000,000 strings. The compiler will balk. Keep cardinality reasonable.

**Mistake 5: Treating mapped types as cheap.** Deeply nested mapped + conditional types slow `tsc` down. Profile (`tsc --extendedDiagnostics`) if your editor lags.

**Mistake 6: Over-engineering.** If a simple union works, use it. Type-level programming is read more often than written; clever is liability.

---

## 🎯 Key Takeaways

- Mapped types iterate keys — `Partial`, `Readonly`, `Pick`, `Omit` are all written this way.
- Key remapping (`as`) lets you rename or filter properties — and it's how `Omit` works internally.
- Template literal types operate on string types just like regular template literals operate on values.
- Combining `infer` + template literals + recursion lets you parse strings at compile time — the basis of typed routers and SQL builders.
- Power costs perf and readability — reach for these when the abstraction *pays* for itself across many call sites.

*← [07 — Generics advanced](./07_generics_advanced.md) | [next →](./09_utility_types.md)*
