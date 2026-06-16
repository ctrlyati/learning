# 00 — Nest.js Deep-Dive Roadmap

> **Goal:** Take a developer who understands TypeScript/JavaScript and Node.js fundamentals, and turn them into a backend engineer who can design, build, test, and operate production-grade, enterprise-scale APIs and microservices with Nest.js.

---

## Why Nest.js?

Nest.js is a **progressive Node.js framework** for building efficient, reliable, and scalable server-side applications. While frameworks like Express, Fastify, and Koa provide minimal HTTP routing abstractions, they leave architecture entirely up to the developer. For small projects, this is fine. For large teams and enterprise apps, it often leads to spaghetti code.

Nest.js solves this by providing:

- **A strong architecture out-of-the-box.** Heavily inspired by Angular, Nest relies on a modular, controller-provider architecture that scales cleanly.
- **TypeScript by default.** Complete type safety, interface enforcement, and decorator-based configuration.
- **Dependency Injection (DI) Container.** Managed lifecycle, testing utilities, and decouple-able component wiring.
- **A unified ecosystem.** Direct first-party wrappers for databases, WebSockets, microservices, task queues, and configuration.
- **Underlying HTTP server abstraction.** By default, Nest sits on top of **Express**, but can be easily configured to run on top of **Fastify** for higher performance.

---

## Module Map

| #  | File | Topic | What you walk away with |
|----|------|-------|-------------------------|
| 00 | `00_roadmap.md` | This file | Course curriculum, prerequisites, and setup instructions. |
| 01 | `01_setup_and_first_server.md` | Setup & First Server | Nest CLI usage, project layout, hot-reloads, and bootstrap files. |
| 02 | `02_controllers_and_routing.md` | Controllers & Routing | Route handlers, request/response decorators, param extraction, status codes. |
| 03 | `03_providers_and_di.md` | Providers & DI | Dependency injection mechanics, constructor injection, custom providers, token matching. |
| 04 | `04_modules_and_namespaces.md` | Modules & Namespaces | Feature encapsulation, shared/global modules, dynamic modules (`forRoot`/`register`). |
| 05 | `05_middleware_and_request_lifecycle.md` | Middleware & Request Lifecycle | Request pipelines, Express compatibility, and the complete execution order. |
| 06 | `06_pipes_and_validation.md` | Pipes & Validation | Transform pipes, DTO validation using `class-validator`, custom validators. |
| 07 | `07_exception_filters.md` | Exception Filters | Centralized error layer, built-in HTTP exceptions, and custom catch-all filters. |
| 08 | `08_guards_and_authorization.md` | Guards & Authorization | Route protection, metadata reflection, Role-Based Access Control (RBAC). |
| 09 | `09_interceptors.md` | Interceptors & AOP | Aspect-Oriented programming, response manipulation, RxJS pipes, logging, execution time profiling. |
| 10 | `10_database_integration.md` | Database & Prisma ORM | Prisma Client configuration, seed scripts, model queries, transactions. |
| 11 | `11_authentication_and_jwt.md` | Auth & Passport | Passport strategies, JWT issuance, route guards, custom `@CurrentUser()` decorator. |
| 12 | `12_configuration_and_env.md` | Configuration & Env | Safe environment loading, ConfigModule, validation with Joi/class-validator. |
| 13 | `13_testing_unit_and_e2e.md` | Unit & E2E Testing | Jest test configurations, testing modules, mock dependencies, Supertest E2E runs. |
| 14 | `14_websockets_and_microservices.md` | WebSockets & Microservices | Gateway communication, transport mechanisms (TCP/Redis/RMQ), message patterns. |
| 15 | `15_queues_and_task_scheduling.md` | Queues & Task Scheduling | Background processing with BullMQ, queue processors, and scheduled cron jobs. |
| 16 | `16_production_and_docker.md` | Production & Dockerization | Multi-stage Docker files, PM2 clusters, memory optimization, Terminus health checks. |
| 17 | `17_interview_questions.md` | Interview Prep | 25+ essential Junior, Mid, and Senior Nest.js interview questions with detailed answers. |

Total: **18 files**.

---

## Timeline (one module per day ≈ 3 weeks)

| Week | Days | Focus |
|------|------|-------|
| 1    | 1–3  | Modules 01–03: CLI setup, Controllers, and the Dependency Injection container. |
| 1    | 4–5  | Modules 04–05: Modules design, Dynamic configurations, and Middleware. |
| 1    | 6–7  | Modules 06–07: Pipes validation, DTOs, and Exception Filters. |
| 2    | 8–9  | Modules 08–09: Guards, Authorization, and Interceptors. |
| 2    | 10–12| Modules 10–12: Database setup (Prisma/Postgres), Auth (Passport/JWT), Config Module. |
| 2    | 13–14| Module 13: Unit and E2E Testing with Jest. |
| 3    | 15–16| Modules 14–16: Real-time WebSockets, Microservices, Task Queues, and Production Docker. |
| 3    | 17    | Module 17: Interview questions & final code cleanup. |

---

## Prerequisites

To get the most out of this course, you should have a firm grasp of the following:

- **Modern JavaScript/TypeScript** — async/await, closures, promises, classes, interfaces, generic types.
- **Node.js Basics** — npm scripts, node event loop, Express middleware concepts, package.json management.
- **SQL / Relational DBs** — tables, primary/foreign keys, joins, queries.
- **Docker basics (Optional but helpful)** — running containers, understanding images.

---

## Core Mental Models

Understanding these five concepts is crucial to mastering Nest.js.

### 1. Inversion of Control (IoC) & Dependency Injection (DI)
Instead of a class instantiating its dependencies manually (e.g. `const service = new UserService()`), the Nest.js container instantiates them and injects them via the constructor. You register your classes as `@Injectable()`, declare them in module `providers`, and let Nest handle their lifetimes.

### 2. Modular Architecture
Every application has at least one module (the root `AppModule`). Features are encapsulated into self-contained feature modules (e.g. `UserModule`, `AuthModule`). If `Module A` needs a provider from `Module B`, `Module B` must explicitly `export` it, and `Module A` must `import` `Module B`. This strict boundary enforcement makes the codebase clean and maintainable.

### 3. Decorator-Driven Design
Nest relies heavily on ES2016 decorators (`@Controller()`, `@Get()`, `@Injectable()`). Under the hood, decorators register metadata about your classes and methods using `reflect-metadata`. At bootstrap time, the framework reads this metadata to construct routing tables, wire dependencies, and apply lifecycle hooks.

### 4. Express/Fastify Abstraction
Nest is HTTP-platform agnostic. It wraps Express by default but allows swapping to Fastify with minimal changes. To maintain this layer of abstraction, you should avoid interacting with the raw Express request/response object (`@Req()` and `@Res()`) directly, preferring Nest's built-in decorators and wrappers instead.

### 5. Execution Pipeline (Order of Execution)
When an HTTP request enters a Nest.js application, it flows through a highly structured pipeline. Knowing this sequence by heart is the key to debugging lifecycle issues:
```
Request ──> Middleware ──> Guards ──> Interceptors (Pre-handler) ──> Pipes ──> Controller (Route Handler) ──> Interceptors (Post-handler) ──> Exception Filters ──> Response
```

---

## External Resources

- **Official Nest.js Documentation** — <https://docs.nestjs.com/> — the gold standard reference. Keep it open.
- **TypeScript Deep Dive** — <https://basarat.gitbook.io/typescript/> — excellent resource if Nest's type mechanics feel overwhelming.
- **Prisma Docs** — <https://www.prisma.io/docs> — for database models and client syntax.
- **BullMQ Docs** — <https://docs.bullmq.io/> — for advanced queue setups.
- **RxJS Documentation** — <https://rxjs.dev/> — essential for writing custom interceptors.

---

## How to Study This Course

1. **Write it out.** Type the code, set up the CLI, and run the servers locally. Avoid copy-pasting.
2. **Interact with endpoints.** Use `curl` or Postman to trigger route validations, interceptors, and error handlers.
3. **Trace the request lifecycle.** Put `console.log` statements inside Middleware, Guards, Interceptors, Pipes, and Controllers to see the exact execution sequence.
4. **Solve the exercises.** Complete the practice tasks and review the interview Q&A at the end of each module.

Let's begin!

*next → [01 — Setup and First Server](./01_setup_and_first_server.md)*
