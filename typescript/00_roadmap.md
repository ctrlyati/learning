# 00 — TypeScript Roadmap

> **Goal:** Take you from "I know JavaScript" to "I can architect type-safe, production-grade TypeScript systems" in roughly 2.5 weeks of focused study.

TypeScript is not a different language — it is JavaScript with a compile-time type system that *erases* before it runs. Treat the type layer as a separate sub-language you are also learning.

---

## Module Table

| #  | File | Topic | Why it matters |
|----|------|-------|----------------|
| 01 | [Setup & compiler](./01_setup_and_compiler.md) | tsc, tsconfig, runners | The dev loop is everything |
| 02 | [Primitive types & literals](./02_primitive_types.md) | type vs interface | The vocabulary |
| 03 | [Functions](./03_functions.md) | Overloads, `this` | Where most code lives |
| 04 | [Objects](./04_objects.md) | Optional, readonly, index signatures | Modeling data |
| 05 | [Unions, intersections, narrowing](./05_unions_and_narrowing.md) | Discriminated unions | The TS superpower |
| 06 | [Generics fundamentals](./06_generics_fundamentals.md) | Type parameters | Reusable typed code |
| 07 | [Generics advanced](./07_generics_advanced.md) | Constraints, conditional, infer | Library-grade types |
| 08 | [Mapped & template literal types](./08_mapped_and_template_literal.md) | Key remapping | Transform types |
| 09 | [Utility types](./09_utility_types.md) | Partial, Pick, ReturnType, custom | Daily tools |
| 10 | [Classes & decorators](./10_classes_and_decorators.md) | Modifiers, abstract, decorators | OOP in TS |
| 11 | [Modules & .d.ts](./11_modules_and_declarations.md) | Ambient, triple-slash | Interop layer |
| 12 | [Working with libraries](./12_libraries_and_resolution.md) | @types, ESM/CJS | Real ecosystem |
| 13 | [Strict mode deep dive](./13_strict_mode_deep_dive.md) | Every strict flag | Catch bugs early |
| 14 | [Errors & exhaustiveness](./14_errors_and_exhaustiveness.md) | Result types | Reliable code |
| 15 | [Real-world patterns](./15_real_world_patterns.md) | Branded, builder, zod | Production toolkit |
| 16 | [Production TypeScript](./16_production_typescript.md) | Project refs, migration | Scaling |

---

## Suggested Timeline (1 module/day)

| Week | Focus | Modules |
|------|-------|---------|
| Week 1 | Foundations | 01–05 |
| Week 2 | Type system mastery | 06–11 |
| Week 3 (half) | Production craft | 12–16 |

If you have only 1 hour/day, this is realistic. With 2–3 hours/day you can compress to ~10 days.

---

## Prerequisites

You **must** be comfortable with modern JavaScript: closures, `this`, prototypes, ES modules, promises, async/await, destructuring, spread, and array methods. If any of those feel shaky, work through the JS course first:

- [JavaScript roadmap](../javascript/00_roadmap.md)

You also need Node.js 20+ installed and a working editor (VS Code is the de-facto choice — its TS server *is* the same one `tsc` uses).

---

## Mental Models (internalize these — everything else follows)

1. **Types are erased at runtime.** `tsc` deletes every annotation. At runtime you have plain JS. If you need a runtime check, you must write it (or use a schema library like zod). This is why `instanceof MyInterface` is impossible — interfaces don't exist after compilation.

2. **TypeScript is structurally typed, not nominally.** Two types with the same shape are compatible, even if declared separately. `class Cat { name: string }` and `class Dog { name: string }` are interchangeable to the type system. Branded types (Module 15) are how you opt into nominal typing when you need it.

3. **Narrowing is control-flow analysis.** Inside an `if (typeof x === 'string')` branch, TS knows `x` is `string`. The compiler reads your code like a human and refines types as it goes. Master this and 80% of "fighting the compiler" disappears.

4. **The type system is a sub-language.** Conditional types, mapped types, `infer`, and template literals form a pure functional language *over types*. You can compute, recurse, and pattern-match — at compile time. This is what powers libraries like tRPC, zod, Prisma, and Drizzle.

5. **Make illegal states unrepresentable.** The whole point of the type system is to design data such that bugs cannot be expressed. Prefer discriminated unions over boolean flags. Prefer branded IDs over raw strings. Prefer `readonly` by default.

6. **The compiler is a collaborator, not an adversary.** When you "fight" TS, the type usually reflects a real bug or unclear data model. Lean into the error before reaching for `any` or `as`.

---

## Reference Library (bookmark all)

- [TypeScript Handbook](https://www.typescriptlang.org/docs/handbook/intro.html) — official, surprisingly readable
- [Type Challenges](https://github.com/type-challenges/type-challenges) — gym for type-level programming
- [Total TypeScript](https://www.totaltypescript.com/) — Matt Pocock's free + paid material; the best modern resource
- [Matt Pocock's YouTube & Twitter](https://www.youtube.com/@mattpocockuk) — short, dense, idiomatic tips
- [DefinitelyTyped](https://github.com/DefinitelyTyped/DefinitelyTyped) — read real `.d.ts` files for libraries you use
- [tsconfig reference](https://www.typescriptlang.org/tsconfig) — every flag explained

---

By the end of Module 16, you should be able to: design a typed API client from scratch, migrate a JS codebase incrementally, configure a TS monorepo with project references, write your own utility types, and read library `.d.ts` files fluently.

Open [Module 01](./01_setup_and_compiler.md) and let's go.

*next →* [01 — Setup & Compiler](./01_setup_and_compiler.md)
