# 04 — Objects: Optional, Readonly, Index Signatures

> **Goal:** Model real-world data shapes precisely — and learn the subtle semantics of object types.

---

## 1. Mental model — an object type is a *contract about which keys exist*

```ts
type User = {
  id: string;
  name: string;
  email: string;
};

const u: User = { id: "1", name: "Ada", email: "ada@x.com" };
```

TypeScript is **structural** — anything with the right shape is assignable:

```ts
const u2 = { id: "2", name: "Grace", email: "g@x.com", role: "admin" };
const x: User = u2; // ✓ — extra properties are fine via aliasing
```

**Excess property checking** — but a *fresh object literal* is checked stricter:

```ts
const x: User = { id: "1", name: "Ada", email: "ada@x.com", role: "admin" };
// ✗ Object literal may only specify known properties
```

This catches typos in inline objects but not in pre-bound variables. Surprising, but practical.

---

## 2. Optional, readonly, and the difference from `undefined`

```ts
type User = {
  id: string;
  name: string;
  readonly createdAt: Date;   // can't reassign
  email?: string;             // may be missing entirely
};

const u: User = { id: "1", name: "Ada", createdAt: new Date() };
u.createdAt = new Date(); // ✗ readonly
u.email;                  // string | undefined
```

There's a subtle difference between `email?: string` and `email: string | undefined`:

```ts
type A = { x?: number };
type B = { x: number | undefined };

const a: A = {};            // ✓ — key may be absent
const b: B = {};            // ✗ — key must be present (even if undefined)
const b2: B = { x: undefined }; // ✓
```

Under `exactOptionalPropertyTypes` (covered in Module 13), `a.x = undefined` becomes an error too — `?` will then mean *only* "may be absent."

`readonly` is shallow:

```ts
type Config = { readonly servers: string[] };
const c: Config = { servers: ["a"] };
c.servers = [];        // ✗
c.servers.push("b");   // ✓ (array itself isn't readonly)
```

For deep immutability use `ReadonlyArray<T>` and `readonly` recursively, or write a `DeepReadonly<T>` utility (Module 09).

---

## 3. Index signatures — keys you don't know in advance

```ts
type Headers = { [key: string]: string };

const h: Headers = {};
h["Content-Type"] = "json";
h.foo = "bar";
```

Two valid key types: `string`, `number`, `symbol`, or a *finite union* of literal keys (the last via mapped types — Module 08).

You can mix index signatures with known keys, but every known key must be **assignable to the index type**:

```ts
type StringDict = {
  [key: string]: string;
  count: number;     // ✗ — number not assignable to index type string
};

type StringDict2 = {
  [key: string]: string | number;
  count: number;     // ✓
  name: string;      // ✓
};
```

**Better alternatives in modern TS:**

- `Record<K, V>` for dictionaries with known key sets
- `Map<K, V>` when keys are dynamic and you want real iteration order, deletion, etc.

```ts
type Status = "pending" | "ok" | "fail";
type Counts = Record<Status, number>;
// equivalent to: { pending: number; ok: number; fail: number }
```

---

## 4. `noUncheckedIndexedAccess` — the flag that changes index signatures

By default:

```ts
const h: Headers = {};
const x = h["nope"]; // string — TS lies, x might be undefined at runtime
```

With `noUncheckedIndexedAccess: true`:

```ts
const x = h["nope"]; // string | undefined — truthful
```

Same for arrays:

```ts
const xs = [1, 2, 3];
const first = xs[0]; // number | undefined  (with the flag)
if (first !== undefined) {
  first.toFixed();   // ✓
}
```

Turn this on in new projects. The friction is real but the bug-prevention is enormous.

---

## 5. Practical application — modeling an API response

```ts
type Money = {
  readonly amount: number;
  readonly currency: "USD" | "EUR" | "GBP";
};

type Address = {
  readonly street: string;
  readonly city: string;
  readonly postalCode: string;
  readonly country: string;
  readonly unit?: string;          // optional, may be absent
};

type Customer = {
  readonly id: string;
  readonly name: string;
  readonly email: string;
  readonly billingAddress: Address;
  readonly shippingAddress?: Address;        // entire field optional
  readonly metadata: Record<string, string>; // arbitrary tags
};

type LineItem = {
  readonly sku: string;
  readonly quantity: number;
  readonly unitPrice: Money;
};

type Order = {
  readonly id: string;
  readonly customer: Customer;
  readonly items: ReadonlyArray<LineItem>;
  readonly placedAt: Date;
  readonly notes?: string;
};

function totalCents(order: Order): number {
  return order.items.reduce(
    (sum, it) => sum + it.unitPrice.amount * it.quantity,
    0,
  );
}

const o: Order = {
  id: "o1",
  customer: {
    id: "c1",
    name: "Ada",
    email: "ada@x.com",
    billingAddress: { street: "1", city: "X", postalCode: "0", country: "US" },
    metadata: { source: "web" },
  },
  items: [{ sku: "A", quantity: 2, unitPrice: { amount: 1500, currency: "USD" } }],
  placedAt: new Date(),
};

// o.items.push(...) ✗ ReadonlyArray
// o.id = "o2"        ✗ readonly
```

Every illegal mutation is a compile error. The model documents itself.

---

## 6. `keyof`, `typeof`, and indexed access — preview

```ts
type User = { id: string; name: string; age: number };

type UserKey = keyof User;          // "id" | "name" | "age"
type AgeType = User["age"];          // number
type IdOrName = User["id" | "name"]; // string

const defaultUser = { id: "0", name: "anon", age: 0 };
type DefaultUser = typeof defaultUser; // { id: string; name: string; age: number }
```

`keyof` and `typeof` (the *type-level* one) are the bedrock of advanced types. We'll use them constantly from Module 06 onward.

---

## 7. Common mistakes & gotchas

**Mistake 1: Trusting array/index access.** Without `noUncheckedIndexedAccess`, `obj[k]` and `arr[i]` lie about possibly being `undefined`. Turn the flag on.

**Mistake 2: Excess property checking confusion.**

```ts
type Opts = { width: number };
function fn(o: Opts) {}
fn({ width: 1, height: 2 });    // ✗ excess property
const o = { width: 1, height: 2 };
fn(o);                          // ✓ — same data, different rule
```

If you genuinely want to allow extras, use an index signature or `& Record<string, unknown>`.

**Mistake 3: `readonly` doesn't deep-freeze.** It blocks reassignment, not nested mutation. Use deep utility types or runtime `Object.freeze` if you need both.

**Mistake 4: Optional vs `| undefined` mixed up.** Under default settings they're nearly identical; under `exactOptionalPropertyTypes` they diverge meaningfully.

**Mistake 5: `Record<string, T>` doesn't actually restrict keys.** `Record<string, T>` ≡ `{ [k: string]: T }`. Use a literal-union key (`Record<"a" | "b", T>`) when you want exhaustiveness.

**Mistake 6: Using `object` thinking it means "plain object."** `object` means "non-primitive." Functions and arrays are `object` too. Use `Record<string, unknown>` for "shaped object."

---

## 🎯 Key Takeaways

- Object types are structural contracts — same shape ≡ same type.
- `readonly` and `?` carry meaningful semantics; default to readonly for value objects.
- Prefer `Record<K, V>` over loose index signatures, and `Map` when keys are truly dynamic.
- Turn on `noUncheckedIndexedAccess` in new projects — it prevents a giant class of runtime bugs.
- Excess property checking applies to fresh literals only; this is by design and rarely a problem once you know the rule.

*← [03 — Functions](./03_functions.md) | [next →](./05_unions_and_narrowing.md)*
