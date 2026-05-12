# 14 — Error Handling Patterns, `Result` Types, Exhaustiveness Checks

> **Goal:** Stop catching strings, start treating errors as data, and let the compiler enforce that you handle every case.

---

## 1. The problem with `throw`

JavaScript's `throw` has three issues TypeScript inherits:

1. **The thrown value has no type.** You can throw anything (`throw "oops"`, `throw 42`, `throw undefined`). With `useUnknownInCatchVariables`, `catch (e)` gives you `unknown`.
2. **Function signatures lie.** `function getUser(id: string): User` doesn't say it might throw. There's no `throws` clause.
3. **Exceptions break the type-flow narrative.** They jump out of the call stack. Exhaustiveness checks at the call site can't reason about them.

For some errors (truly exceptional, can't recover) `throw` is appropriate. For predictable failures (invalid input, missing record, network failure) — model them as **values**.

---

## 2. The `Result` type — explicit success/failure

```ts
type Ok<T>  = { ok: true;  value: T };
type Err<E> = { ok: false; error: E };
type Result<T, E = Error> = Ok<T> | Err<E>;

const ok  = <T>(value: T): Ok<T>   => ({ ok: true, value });
const err = <E>(error: E): Err<E>  => ({ ok: false, error });
```

A function that may fail returns `Result`:

```ts
type ParseError = { kind: "ParseError"; message: string };

function parseAge(input: string): Result<number, ParseError> {
  const n = Number(input);
  if (Number.isNaN(n))     return err({ kind: "ParseError", message: "not a number" });
  if (n < 0 || n > 150)    return err({ kind: "ParseError", message: "out of range" });
  return ok(n);
}

const r = parseAge("42");
if (r.ok) {
  console.log(r.value.toFixed());  // ✓ value: number
} else {
  console.log(r.error.message);    // ✓ error: ParseError
}
```

The compiler now **forces you to check** before using either side. No silent crashes. No hidden control flow.

---

## 3. Discriminated error types

For richer error handling, give each failure mode its own variant:

```ts
type FetchError =
  | { kind: "Network";  cause: unknown }
  | { kind: "NotFound"; id: string }
  | { kind: "Unauthorized" }
  | { kind: "ServerError"; status: number };

async function fetchUser(id: string): Promise<Result<User, FetchError>> {
  let res: Response;
  try {
    res = await fetch(`/users/${id}`);
  } catch (cause) {
    return err({ kind: "Network", cause });
  }
  if (res.status === 404) return err({ kind: "NotFound", id });
  if (res.status === 401) return err({ kind: "Unauthorized" });
  if (!res.ok)            return err({ kind: "ServerError", status: res.status });
  return ok((await res.json()) as User);
}
```

The caller now handles each case discriminated:

```ts
const r = await fetchUser("u1");
if (!r.ok) {
  switch (r.error.kind) {
    case "Network":      retry(); break;
    case "NotFound":     showEmpty(); break;
    case "Unauthorized": redirectLogin(); break;
    case "ServerError":  toast(`Server error ${r.error.status}`); break;
  }
} else {
  render(r.value);
}
```

The `switch` is checked for exhaustiveness (next section). Add `| { kind: "RateLimited" }` to `FetchError` and the compiler instantly tells you about every site that needs to handle it.

---

## 4. The `assertNever` pattern

```ts
export function assertNever(value: never, msg = "Unhandled variant"): never {
  throw new Error(`${msg}: ${JSON.stringify(value)}`);
}

function describe(s: Shape): string {
  switch (s.kind) {
    case "circle": return "circle";
    case "square": return "square";
    default:       return assertNever(s);
  }
}
```

If a new variant is added, `s` is no longer `never` in the default branch and you get a compile error pointing right at the missing case.

This is the single most valuable refactor-safety tool the type system provides. Use it religiously.

A common variant for production code that doesn't crash on invalid input:

```ts
function describe(s: Shape): string {
  switch (s.kind) {
    case "circle": return "circle";
    case "square": return "square";
    default: {
      const _exhaustive: never = s;
      return "unknown";  // graceful runtime fallback
    }
  }
}
```

You still get the compile error, but production keeps running.

---

## 5. Practical application — a real `Result` toolkit

```ts
// result.ts
export type Result<T, E = Error> =
  | { ok: true;  value: T }
  | { ok: false; error: E };

export const ok  = <T>(value: T): Result<T, never>    => ({ ok: true,  value });
export const err = <E>(error: E): Result<never, E>    => ({ ok: false, error });

export function map<T, U, E>(r: Result<T, E>, f: (v: T) => U): Result<U, E> {
  return r.ok ? ok(f(r.value)) : r;
}

export function flatMap<T, U, E>(r: Result<T, E>, f: (v: T) => Result<U, E>): Result<U, E> {
  return r.ok ? f(r.value) : r;
}

export function unwrap<T, E>(r: Result<T, E>): T {
  if (r.ok) return r.value;
  throw r.error instanceof Error ? r.error : new Error(String(r.error));
}

export function fromThrowing<T>(fn: () => T): Result<T, unknown> {
  try { return ok(fn()); }
  catch (e) { return err(e); }
}

export async function fromPromise<T>(p: Promise<T>): Promise<Result<T, unknown>> {
  try { return ok(await p); }
  catch (e) { return err(e); }
}
```

Usage:

```ts
const r = await fromPromise(fetch("/api"));
const parsed = flatMap(r, (res) => fromThrowing(() => res.json()));

if (parsed.ok) console.log(parsed.value);
else           console.error(parsed.error);
```

Many teams adopt a library — `neverthrow`, `ts-results`, or `effect` — for the same shape with more combinators.

---

## 6. When `throw` is still right

`Result` is the right default for *expected* failures. `throw` is appropriate when:

- The error is a **bug** (invariant violation, unreachable state, programming error).
- The error is **unrecoverable** (out of memory, configuration missing at startup).
- You're at a **boundary** that translates: HTTP middleware that turns thrown errors into 500s, top-level process error handlers.

```ts
function assert(cond: boolean, msg: string): asserts cond {
  if (!cond) throw new Error(msg);
}

function divide(a: number, b: number): number {
  assert(b !== 0, "division by zero is a programmer bug here");
  return a / b;
}
```

The `asserts cond` return type narrows `cond` to `true` after the call. This is the right way to encode invariants.

---

## 7. Common mistakes & gotchas

**Mistake 1: `Result` everywhere, including for impossible cases.** If a function genuinely cannot fail, don't wrap it. `Result` adds noise.

**Mistake 2: Single `Error` type for everything.** Loses information. Use discriminated error variants for anything you'd actually handle differently.

**Mistake 3: Ignoring the `error` branch.** `if (r.ok) {…}` with no `else` silently drops failures. Use ESLint's `no-unused-vars` and `consistent-return` to catch.

**Mistake 4: Catching too broadly.** `try { … } catch { return err(…) }` swallows bugs. Catch only what you intend; rethrow the rest.

**Mistake 5: Forgetting exhaustiveness on async.** `Promise<Result>` chains are easy to misroute. Pull each `await` onto its own line and check `ok` before continuing.

**Mistake 6: Designing errors as strings.** `err("user not found")` loses structure. Use objects with a `kind` discriminant.

---

## 🎯 Key Takeaways

- Treat predictable failures as values (`Result<T, E>`), not exceptions.
- Discriminated error variants + `switch` + `assertNever` give you compile-time exhaustiveness across the entire codebase.
- `throw` is for bugs and unrecoverable situations; `Result` is for *handleable* failures.
- A small `Result` toolkit (`ok`, `err`, `map`, `flatMap`, `fromPromise`) covers most needs without a library — adopt one (neverthrow, effect) when you want richer combinators.
- The `asserts cond` pattern is the right way to encode invariants in code that should never fail.

*← [13 — Strict mode deep dive](./13_strict_mode_deep_dive.md) | [next →](./15_real_world_patterns.md)*
