# 03 — Functions: Parameters, Overloads, `this`

> **Goal:** Type every function shape you'll meet in real code — including the awkward ones.

---

## 1. Mental model — a function type has parameters *and* a return

```ts
function add(a: number, b: number): number {
  return a + b;
}

// Same thing as a type alias:
type Add = (a: number, b: number) => number;

const add2: Add = (a, b) => a + b; // params inferred from the type
```

Three forms you'll write daily:

```ts
// Function declaration (hoisted)
function f(x: number): number { return x; }

// Function expression
const g = function (x: number): number { return x; };

// Arrow function
const h = (x: number): number => x;
```

**Return type inference** is excellent — usually omit return types on internal functions and let TS infer. **Always annotate returns on exported/public functions** so signature changes are caught at the boundary, not deep in callers.

---

## 2. Parameter types — optional, default, rest

```ts
// Optional (must be trailing)
function greet(name: string, greeting?: string): string {
  return `${greeting ?? "Hello"}, ${name}`;
}

// Default (also makes it optional from the caller's view)
function greet2(name: string, greeting: string = "Hello"): string {
  return `${greeting}, ${name}`;
}

// Rest
function sum(...ns: number[]): number {
  return ns.reduce((a, b) => a + b, 0);
}

// Tuple-typed rest (precise positional types)
function tag(strings: TemplateStringsArray, ...values: [string, number]) { /* ... */ }
```

Optional `param?: T` has type `T | undefined`. You must check before using it.

---

## 3. Overloads — multiple call signatures, one implementation

When a function returns different types based on input shape:

```ts
// Overload signatures (visible to callers)
function parse(input: string): object;
function parse(input: number): string;
function parse(input: boolean): null;

// Implementation signature (must be compatible with all overloads — not visible to callers)
function parse(input: string | number | boolean): object | string | null {
  if (typeof input === "string")  return JSON.parse(input);
  if (typeof input === "number")  return input.toString();
  return null;
}

const a = parse("[]");   // object
const b = parse(42);     // string
const c = parse(true);   // null
```

The implementation signature is **not** a callable overload. Only the listed overloads are.

**Modern alternative** — most overloads can be replaced by generics + conditional types (Module 07). Reach for overloads when:

- Behavior genuinely diverges by input type
- Callers need distinct doc comments per signature

Otherwise generics are usually cleaner.

---

## 4. The `this` parameter — TS's solution to JS's worst feature

In JS, `this` is dynamic. TS lets you type it explicitly as a *fake first parameter* (erased at runtime):

```ts
interface Button {
  text: string;
  onClick(this: Button, event: Event): void;
}

const btn: Button = {
  text: "OK",
  onClick(event) {
    console.log(this.text); // this: Button — typed
  },
};

// Compile error if you detach the method:
const handler = btn.onClick;
// handler(new Event("click")); ✗ The 'this' context of type 'void' is not assignable
```

Two related types worth knowing:

- **`ThisType<T>`** — used inside object types to set the type of `this` for methods.
- **`OmitThisParameter<T>`** — strips the `this` parameter from a function type.

You'll mostly see explicit `this` parameters in callback-heavy APIs (jQuery-era), event emitters, and `.call/.apply/.bind` typings.

---

## 5. Practical application — a typed event emitter

```ts
type EventMap = {
  login:  { userId: string };
  logout: { userId: string; reason: "manual" | "timeout" };
  error:  Error;
};

class TypedEmitter<TEvents extends Record<string, unknown>> {
  private listeners: { [K in keyof TEvents]?: Array<(payload: TEvents[K]) => void> } = {};

  on<K extends keyof TEvents>(event: K, fn: (payload: TEvents[K]) => void): void {
    (this.listeners[event] ??= []).push(fn);
  }

  emit<K extends keyof TEvents>(event: K, payload: TEvents[K]): void {
    this.listeners[event]?.forEach((fn) => fn(payload));
  }
}

const bus = new TypedEmitter<EventMap>();

bus.on("login", ({ userId }) => console.log(userId));         // ✓ payload typed
bus.emit("logout", { userId: "u1", reason: "timeout" });      // ✓
bus.emit("logout", { userId: "u1", reason: "wat" });          // ✗ literal mismatch
bus.on("nope", () => {});                                     // ✗ unknown event
```

Every callback knows its payload shape. Every `emit` is checked. Zero runtime overhead beyond the JS you'd write anyway.

---

## 6. Function type compatibility — bivariance and the strict callback gotcha

TS checks function assignability with **parameter contravariance** (under `strictFunctionTypes`). This trips people up:

```ts
type Handler = (e: Event) => void;
type MouseHandler = (e: MouseEvent) => void;

const h: Handler = (e) => {};
const mh: MouseHandler = h;     // ✓ a handler that accepts any Event accepts MouseEvent
const h2: Handler = (e: MouseEvent) => {}; // ✗ a MouseHandler cannot stand in for a Handler
```

Methods (`m(x: T): void`) are checked **bivariantly** by default for legacy compatibility. Function-typed properties (`m: (x: T) => void`) are checked **strictly**. Prefer the property form in new code.

---

## 7. Common mistakes & gotchas

**Mistake 1: Using `Function` as a type.** `Function` accepts anything callable but provides no parameter info. Use `(...args: never[]) => unknown` or, better, name the actual signature.

**Mistake 2: Forgetting return type on async functions.** An async function returning `User` actually has type `Promise<User>`. Be explicit:

```ts
async function getUser(id: string): Promise<User> { ... }
```

**Mistake 3: Overloads that lie.** The implementation must accept the union of all overloaded params. TS doesn't deeply verify each overload against the body.

**Mistake 4: `void` vs `undefined` in callback returns.**

```ts
type Cb = () => void;
const cb: Cb = () => 42;        // ✓ — `void` means "I don't care what you return"
const f: () => undefined = () => 42; // ✗ — must literally return undefined
```

This is *intentional* — it lets `[1,2,3].forEach(x => arr.push(x))` typecheck even though `push` returns `number`.

**Mistake 5: Losing `this` by destructuring or detaching.** `const { onClick } = btn; onClick(e)` will fail or misbehave. Use `.bind`, arrow methods, or accept the loss with explicit typing.

**Mistake 6: Annotating callback parameters when context would infer them.**

```ts
// Wasteful:
[1,2,3].map((x: number) => x * 2);
// Better — let it flow:
[1,2,3].map(x => x * 2);
```

Contextual typing is one of TS's best features. Use it.

---

## 🎯 Key Takeaways

- Annotate **parameters** and **public/exported return types**; let inference handle the rest.
- Prefer string literal unions + generics over function overloads; reach for overloads only when truly needed.
- `unknown` parameters force callers to narrow — safer than `any` for boundary functions.
- Typed `this` parameters are erased at runtime and catch detached-method bugs.
- A typed event emitter / message bus pattern shows how generics + mapped types eliminate stringly-typed APIs entirely.

*← [02 — Primitive types](./02_primitive_types.md) | [next →](./04_objects.md)*
