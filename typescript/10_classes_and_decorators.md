# 10 — Classes, Access Modifiers, Abstract Classes, Decorators

> **Goal:** Use classes idiomatically in TS — including the modern (Stage 3) decorators that finally landed.

---

## 1. Class basics — TS adds visibility, parameter properties, and types

```ts
class User {
  public  readonly id: string;       // public is default; explicit is fine
  private          email: string;    // private to TS
  protected        role: "user" | "admin";

  constructor(id: string, email: string, role: User["role"] = "user") {
    this.id = id;
    this.email = email;
    this.role = role;
  }

  greet(): string {
    return `${this.email} (${this.role})`;
  }
}

const u = new User("u1", "ada@x.com");
u.id;       // ✓
// u.email; // ✗ private
```

**Parameter properties** — shorthand for declaring + assigning fields:

```ts
class User {
  constructor(
    public readonly id: string,
    private email: string,
    protected role: "user" | "admin" = "user",
  ) {}
}
// equivalent to the verbose version above
```

`#name` is a *true* JS private field (runtime-enforced). `private` is TS-only (erased — accessible via `(u as any).email` at runtime).

```ts
class Account {
  #password: string; // truly private at runtime

  constructor(pw: string) { this.#password = pw; }
  matches(p: string): boolean { return this.#password === p; }
}
```

Use `#` for actual secrecy; use `private` for typing convenience.

---

## 2. Inheritance, abstract classes, and `implements`

```ts
abstract class Shape {
  abstract area(): number;          // subclasses must implement
  describe(): string { return `area=${this.area()}`; } // shared
}

class Circle extends Shape {
  constructor(public radius: number) { super(); }
  area(): number { return Math.PI * this.radius ** 2; }
}

// new Shape();           ✗ abstract
new Circle(5).describe(); // ✓
```

`implements` checks shape conformance without inheritance:

```ts
interface Printable { print(): string }

class Doc implements Printable {
  print(): string { return "doc"; }
}
```

`implements` is a **check**, not a contract that flows. The class still has to declare every member itself; `implements` won't add things for you.

---

## 3. `static`, `static blocks`, and abstract `static`

```ts
class Counter {
  static count = 0;
  static {
    // initialization block — runs once
    Counter.count = 0;
  }
  static increment() { return ++Counter.count; }
}
```

You **cannot** mark `static` members `abstract`. If you need a "static interface," use a plain function or a separate type.

---

## 4. Generic classes and `this` types

```ts
class Builder<T extends object = {}> {
  private state: T;
  constructor(state: T = {} as T) { this.state = state; }

  set<K extends string, V>(key: K, value: V): Builder<T & { [P in K]: V }> {
    return new Builder({ ...this.state, [key]: value } as T & { [P in K]: V });
  }

  build(): T { return this.state; }
}

const b = new Builder()
  .set("name", "Ada")
  .set("age", 36)
  .build();
// b: { name: string; age: number }
```

Each `.set` returns a new generic with the key added — fluent API with full inference.

`this` types let methods return *the subclass type*:

```ts
class Chainable {
  log(): this { console.log("…"); return this; }
}

class MyChain extends Chainable {
  more(): this { return this; }
}

new MyChain().log().more(); // ✓ — log() returned MyChain, not Chainable
```

---

## 5. Decorators — the (finally stable) Stage 3 form

TS 5.0 added the standard ECMAScript decorators. They're **different** from the legacy `experimentalDecorators` flag — don't enable that one in new projects.

A decorator is a function called with the thing being decorated and a context object. It can wrap, replace, or augment.

```ts
function logged<T extends (...a: any[]) => any>(
  original: T,
  context: ClassMethodDecoratorContext,
): T {
  return function (this: unknown, ...args: any[]) {
    console.log(`→ ${String(context.name)}`, args);
    const result = original.apply(this, args);
    console.log(`← ${String(context.name)}`, result);
    return result;
  } as T;
}

class Calc {
  @logged
  add(a: number, b: number): number { return a + b; }
}

new Calc().add(1, 2);
// → add [1, 2]
// ← add 3
```

Other decorator targets (each with its own context type):

- `ClassDecoratorContext` — `@dec class Foo {}`
- `ClassMethodDecoratorContext` — `@dec method() {}`
- `ClassFieldDecoratorContext` — `@dec field`
- `ClassGetterDecoratorContext` / `ClassSetterDecoratorContext`
- `ClassAccessorDecoratorContext` — for the new `accessor` keyword

```ts
class Foo {
  @validate accessor count: number = 0;
}
```

The new `accessor` keyword desugars to a private field + getter/setter, and decorators can wrap that pair.

**Caveat:** many libraries (TypeORM, NestJS, older Angular) still rely on the legacy decorators. If you're using those frameworks, you keep `experimentalDecorators: true` and live in the old world for now. The migration to Stage 3 is gradual across the ecosystem.

---

## 6. Practical application — a typed repository base class

```ts
interface Entity { id: string }

abstract class Repository<T extends Entity> {
  protected items = new Map<string, T>();

  abstract validate(data: Omit<T, "id">): void;

  create(data: Omit<T, "id">): T {
    this.validate(data);
    const id = crypto.randomUUID();
    const entity = { ...data, id } as T;
    this.items.set(id, entity);
    return entity;
  }

  get(id: string): T | undefined { return this.items.get(id); }

  update(id: string, patch: Partial<Omit<T, "id">>): T {
    const existing = this.items.get(id);
    if (!existing) throw new Error(`not found: ${id}`);
    const updated = { ...existing, ...patch };
    this.items.set(id, updated);
    return updated;
  }
}

interface User extends Entity { name: string; email: string }

class UserRepo extends Repository<User> {
  validate(data: Omit<User, "id">): void {
    if (!data.email.includes("@")) throw new Error("bad email");
  }
}

const repo = new UserRepo();
const u = repo.create({ name: "Ada", email: "ada@x.com" });
repo.update(u.id, { name: "Ada Lovelace" });
// repo.create({ name: "x", email: "noat" }); throws at runtime
```

Notice the use of `Omit<T, "id">` — the type system enforces that callers don't pass an `id`, only the repo can mint one.

---

## 7. Common mistakes & gotchas

**Mistake 1: `private` ≠ runtime privacy.** Use `#` fields when you need real isolation. `private` is for editor/typecheck enforcement only.

**Mistake 2: `implements` doesn't infer.**

```ts
interface Greeter { greet(name: string): string }

class G implements Greeter {
  greet(name) { return name; } // ✗ implicit any — still need to type yourself
}
```

`implements` is a check, not a source of types.

**Mistake 3: Calling `super()` after using `this`.** TS catches this in derived constructors — fix the order.

**Mistake 4: Using legacy decorators by accident.** If you see `experimentalDecorators: true` in a new project without a framework that needs it, remove it. The Stage 3 form is the future.

**Mistake 5: Treating classes as the default unit.** In modern TS, plain functions + types are usually more idiomatic than classes for app code. Reach for classes when you genuinely need: identity, lifecycle, polymorphism, or framework integration.

**Mistake 6: Returning anything but `this` from chainable methods.** Chains break the moment a method returns a sibling type. Use `this` types or generic builders.

---

## 🎯 Key Takeaways

- TS classes add visibility (`private`/`protected`/`public`), parameter properties, and abstract members on top of JS — but real privacy comes from `#fields`.
- `implements` *checks* shape; it doesn't *give* you types.
- Stage 3 decorators are stable in TS 5+; legacy decorators are only for framework compatibility.
- Generic classes + `this` types power fluent builder APIs with end-to-end inference.
- In modern TS app code, prefer functions + types; reach for classes when identity, lifecycle, or framework integration justifies them.

*← [09 — Utility types](./09_utility_types.md) | [next →](./11_modules_and_declarations.md)*
