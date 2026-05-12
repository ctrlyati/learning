# 15 — Real-World Patterns: Branded Types, Builder, Type-Safe API Clients, Zod

> **Goal:** Tie everything together into the patterns you'll reach for daily in production code.

---

## 1. Branded types — opt-in nominal typing

Structural typing is great, but sometimes two types share a shape and *shouldn't* be interchangeable. The classic case:

```ts
function getUser(id: string) {}
function getOrder(id: string) {}

const orderId = "o123";
getUser(orderId); // ✓ — but a serious bug
```

Brand them:

```ts
declare const brand: unique symbol;
type Brand<T, B> = T & { readonly [brand]: B };

type UserId  = Brand<string, "UserId">;
type OrderId = Brand<string, "OrderId">;

function userId(s: string): UserId {
  // optional runtime validation
  if (!/^u_/.test(s)) throw new Error("invalid UserId");
  return s as UserId;
}

function getUser(id: UserId) {}

const u = userId("u_123");
const o = "o_456" as OrderId;

getUser(u);  // ✓
// getUser(o);    ✗ OrderId not assignable to UserId
// getUser("x");  ✗ string not assignable to UserId
```

The brand is a phantom type — zero runtime cost. Used heavily for: IDs, currency amounts (cents vs dollars), validated email/URL, sanitized HTML, file paths.

**Pattern: smart constructor.** The only way to make a `UserId` is through the `userId()` function. Encapsulates validation + branding.

---

## 2. The builder pattern — fluent APIs with progressive types

A builder accumulates state, and *the type system tracks what's been set*:

```ts
class QueryBuilder<TFields = {}> {
  constructor(private state: TFields) {}

  where<K extends string, V>(key: K, value: V):
    QueryBuilder<TFields & { [P in K]: V }> {
    return new QueryBuilder({ ...this.state, [key]: value } as TFields & { [P in K]: V });
  }

  build(): TFields { return this.state; }
}

const q = new QueryBuilder({})
  .where("status", "active" as const)
  .where("age", 36)
  .build();

// q: { status: "active"; age: number }
```

A more useful version requires *certain fields* before `.build()` is callable — you encode required fields in the type and only expose `build()` when they're present:

```ts
type Required<T, K extends string> = T extends { [P in K]: any } ? T : never;

class HttpRequestBuilder<T = {}> {
  constructor(private state: T) {}

  url(u: string): HttpRequestBuilder<T & { url: string }> {
    return new HttpRequestBuilder({ ...this.state, url: u });
  }

  method(m: "GET" | "POST"): HttpRequestBuilder<T & { method: "GET" | "POST" }> {
    return new HttpRequestBuilder({ ...this.state, method: m });
  }

  // build() only valid when both url and method are set
  build(this: HttpRequestBuilder<{ url: string; method: "GET" | "POST" }>): { url: string; method: string } {
    return this.state;
  }
}

new HttpRequestBuilder()
  .url("/api")
  .method("GET")
  .build();   // ✓

new HttpRequestBuilder()
  .url("/api")
  .build();   // ✗ — `this` constraint not met
```

The `this` parameter constraint is the trick. Module 03's `this` typing pays off here.

---

## 3. Schema validation with Zod

Types are erased at runtime. The moment data enters your program (HTTP body, database row, env var, file), TS *thinks* it's typed but actually has no idea. **Validate at the boundary.**

[Zod](https://zod.dev) is the most popular choice. It defines a schema and gives you both runtime validation **and** a TS type inferred from the schema:

```ts
import { z } from "zod";

const UserSchema = z.object({
  id: z.string().uuid(),
  name: z.string().min(1),
  email: z.string().email(),
  age: z.number().int().nonnegative(),
  role: z.enum(["user", "admin"]),
});

type User = z.infer<typeof UserSchema>;
// { id: string; name: string; email: string; age: number; role: "user" | "admin" }

function handleRequest(body: unknown): User {
  const parsed = UserSchema.parse(body);  // throws on invalid
  // or: const parsed = UserSchema.safeParse(body); → { success, data | error }
  return parsed; // typed as User
}
```

One source of truth — schema. Type and validator stay in lockstep.

For env variables:

```ts
const Env = z.object({
  DATABASE_URL: z.string().url(),
  PORT: z.coerce.number().int().positive(),
  NODE_ENV: z.enum(["development", "test", "production"]),
});

export const env = Env.parse(process.env);
// env.PORT is a real validated number, not a possibly-undefined string
```

Alternatives in the same space: **valibot** (smaller bundle), **arktype** (more powerful types), **typebox** (JSON Schema-compatible). Same idea, different trade-offs.

---

## 4. Type-safe API clients — end-to-end inference

The state of the art:

- **REST**: a contract package both server and client import; a wrapper validates response with zod and returns typed results.
- **tRPC**: write server procedures, get a typed client automatically. Zero codegen.
- **GraphQL**: codegen turns schema → typed client (`graphql-codegen`).
- **OpenAPI**: codegen turns spec → typed client (`openapi-typescript`).

A minimal hand-rolled REST client:

```ts
type Endpoint<TReq, TRes> = {
  path: string;
  method: "GET" | "POST" | "PUT" | "DELETE";
  reqSchema: z.ZodType<TReq>;
  resSchema: z.ZodType<TRes>;
};

function defineEndpoint<TReq, TRes>(e: Endpoint<TReq, TRes>) { return e; }

const getUser = defineEndpoint({
  path: "/users/:id",
  method: "GET",
  reqSchema: z.object({ id: z.string() }),
  resSchema: z.object({ id: z.string(), name: z.string() }),
});

async function call<TReq, TRes>(
  endpoint: Endpoint<TReq, TRes>,
  req: TReq,
): Promise<TRes> {
  endpoint.reqSchema.parse(req);             // validate input
  const res = await fetch(/* … */);
  const json = await res.json();
  return endpoint.resSchema.parse(json);     // validate output → typed
}

const user = await call(getUser, { id: "u1" });
// user: { id: string; name: string }
```

In a real codebase, this grows into a registry of endpoints, generated SDKs, or one of the libraries above. The principle stays: **schema is the source of truth**, types are inferred.

---

## 5. Practical application — a small typed feature

Let's build a typed in-memory cache with TTL, validation, and branded keys:

```ts
import { z } from "zod";

declare const brand: unique symbol;
type Brand<T, B> = T & { readonly [brand]: B };
type CacheKey = Brand<string, "CacheKey">;

function key(parts: string[]): CacheKey {
  return parts.join(":") as CacheKey;
}

class TypedCache {
  private store = new Map<CacheKey, { value: unknown; expires: number }>();

  set<T>(k: CacheKey, value: T, ttlMs: number): void {
    this.store.set(k, { value, expires: Date.now() + ttlMs });
  }

  get<T>(k: CacheKey, schema: z.ZodType<T>): T | null {
    const entry = this.store.get(k);
    if (!entry || entry.expires < Date.now()) {
      this.store.delete(k);
      return null;
    }
    return schema.parse(entry.value);
  }
}

const cache = new TypedCache();
const UserSchema = z.object({ id: z.string(), name: z.string() });
type User = z.infer<typeof UserSchema>;

cache.set<User>(key(["user", "u1"]), { id: "u1", name: "Ada" }, 60_000);
const u = cache.get(key(["user", "u1"]), UserSchema); // User | null
```

What this demonstrates:

- **Branded keys** prevent accidental string concatenation bugs at call sites.
- **Schema-validated reads** mean if data was corrupted (different process wrote it, format changed), you find out instantly instead of hours later.
- **Generics** preserve type information from `set` to `get`.

---

## 6. Common mistakes & gotchas

**Mistake 1: Branded types without smart constructors.** If everyone can do `"raw" as UserId`, the brand is meaningless. Always validate in a single function and export only that.

**Mistake 2: Builders that don't constrain `build()`.** A fluent API that lets you call `.build()` before all required fields are set is no safer than a plain object literal. Use `this`-parameter constraints.

**Mistake 3: Trusting `JSON.parse`.** It returns `any`. Validate immediately or type as `unknown` and narrow.

**Mistake 4: Hand-maintaining server and client types.** They will drift. Generate from a schema (zod, OpenAPI, GraphQL, tRPC).

**Mistake 5: Validating data in the middle of the program instead of at the boundary.** Validate **once at entry**, then trust the type inside. Defense-in-depth in the type layer is fine; defense-in-depth in runtime checks is wasteful.

**Mistake 6: Choosing a "perfect" library without measuring.** Zod's bundle size matters in browsers. Valibot or typebox may be better. tRPC requires both ends to be TypeScript. Match tool to constraints.

---

## 🎯 Key Takeaways

- Branded types + smart constructors give you nominal typing where the structural default is dangerous (IDs, currencies, validated strings).
- Builders with `this`-parameter constraints make required-field omission a compile error.
- Schema validators (zod and friends) are the bridge between unknown runtime data and trusted typed code — *one source of truth* for both.
- Type-safe API clients eliminate an entire category of bugs; pick the approach (tRPC, OpenAPI, GraphQL codegen) that matches your stack.
- Validate at boundaries; trust types inside; never validate in the middle.

*← [14 — Errors & exhaustiveness](./14_errors_and_exhaustiveness.md) | [next →](./16_production_typescript.md)*
