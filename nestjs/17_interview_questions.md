# 17 — Interview Questions

> **Goal:** Review 25+ common Nest.js interview questions spanning Junior, Mid-Level, and Senior competencies to validate your understanding and excel in technical evaluations.

---

## 1. Junior Level Questions (Fundamentals)

- **Q: What is Nest.js, and what problem does it solve in the Node.js ecosystem?**
  → Nest.js is a progressive TypeScript backend framework. It solves the lack of architecture in Node.js backends. While libraries like Express offer routing but leave file organization and dependency management entirely to the developer, Nest provides a rigid modular architecture out-of-the-box, ensuring teams build scalable, decoupled services.

- **Q: What are the three core decorators that define a standard Nest.js application?**
  → 
  1. `@Module()`: Group related controllers and providers into modular namespaces.
  2. `@Controller()`: Define routing entry points and handle incoming HTTP requests.
  3. `@Injectable()`: Declare that a class can be managed by the Nest Dependency Injection container and injected as a dependency.

- **Q: What is Dependency Injection (DI) and how do you use it in Nest.js?**
  → Dependency Injection is an Inversion of Control (IoC) design pattern where classes receive their dependencies from an external source (the Nest DI container) rather than instantiating them manually. You declare class parameters in your constructor, and Nest resolves and passes them in at runtime:
  ```typescript
  constructor(private readonly usersService: UsersService) {}
  ```

- **Q: What is a DTO and why should you use it?**
  → A Data Transfer Object (DTO) is a class defining the exact schema of data sent over the network. In Nest, DTOs are written as classes (not interfaces) so they exist at runtime, allowing decorators from `class-validator` to inspect and validate request payloads.

- **Q: What is the difference between a Module and a Provider?**
  → A **Module** is a boundary wrapper class decorated with `@Module()` that orchestrates imports, exports, controllers, and providers. A **Provider** is any class decorated with `@Injectable()` (like services, repositories, or factories) that contains business logic and can be injected into modules.

- **Q: What does the `exports` array in `@Module()` do?**
  → Because providers are private to their modules by default, listing a provider in `exports` makes it visible to any external module that imports this module.

- **Q: What is the difference between `@Get(':id')` and `@Get('id')`?**
  → `@Get(':id')` maps a dynamic path parameter (e.g. `/users/42`), allowing you to extract `42` using `@Param('id')`. `@Get('id')` matches only the literal path `/users/id`.

- **Q: What is the default HTTP status code returned for POST requests in Nest.js?**
  → `201 Created`. All other request verbs (GET, PUT, DELETE) default to `200 OK`.

---

## 2. Mid-Level Questions (Intermediate Patterns)

- **Q: What is the exact execution order of the Nest.js request pipeline?**
  → Middleware ──> Guards ──> Interceptors (pre-handler) ──> Pipes ──> Controller Route Handler ──> Interceptors (post-handler) ──> Exception Filters.

- **Q: Explain the difference between Middleware and Interceptors.**
  → **Middleware** is Express-compatible, executes first, operates on raw `req`/`res` objects, and has no reference to the route handler method. **Interceptors** are based on Aspect-Oriented Programming (AOP), execute both before *and* after the route handler runs, use RxJS streams, and have access to `ExecutionContext` reflection metadata.

- **Q: How do you handle circular dependencies between two services in Nest.js?**
  → Use `forwardRef()` on both sides of constructor injection:
  ```typescript
  constructor(@Inject(forwardRef(() => AuthService)) private authService: AuthService) {}
  ```
  And also apply `forwardRef()` inside module imports if the modules reference each other.

- **Q: How does Nest.js handle environment configurations securely?**
  → Nest uses `@nestjs/config` which loads environmental `.env` files into a unified `ConfigModule`. You inject `ConfigService` to read variables, and can validate schema presence at startup using `Joi` validation schemas.

- **Q: What is a Dynamic Module, and when should you use one?**
  → A Dynamic Module is a module whose providers and configurations are customized at runtime when it is imported (e.g., configuring database connections or API URLs). Dynamic modules expose static methods like `register()`, `forRoot()`, or `forRootAsync()`.

- **Q: How do you configure a Global Exception Filter, and why should you use `APP_FILTER`?**
  → Registering exception filters via `app.useGlobalFilters()` in `main.ts` disconnects the filter from the DI container. To support service injection inside global filters, register the filter as a provider in `AppModule` using the `APP_FILTER` token.

- **Q: What is the role of `class-transformer` inside the global `ValidationPipe`?**
  → `class-transformer` parses incoming plain JSON objects into actual class instances matching your DTO type definitions. This is required if you want your DTO methods or custom validation logics to execute.

- **Q: How do you execute multiple SQL queries inside a single transaction using Prisma in Nest.js?**
  → Wrap queries inside the `prisma.$transaction()` method. For sequential actions, pass an array of database operations. For complex logic that depends on previous query results, pass an async callback function carrying the transaction client `tx` parameter.

---

## 3. Senior Level Questions (Architecture, Scaling & Internals)

- **Q: What are the three provider injection scopes in Nest.js, and what is the performance risk of request-scoped providers?**
  → 
  1. **DEFAULT (Singleton):** A single instance is shared globally (Best performance).
  2. **REQUEST:** A new instance is created for every HTTP request.
  3. **TRANSIENT:** A new instance is created for every injecting consumer.
  *Performance Risk:* Request-scoped providers bubble up. If a singleton controller injects a request-scoped service, the controller automatically becomes request-scoped. Spawning thousands of class instantiations per second increases garbage collection load and degrades performance under high traffic.

- **Q: How would you scale a Nest.js application horizontally on a virtual machine (VM) vs. inside Docker?**
  → On a VM, scale using **PM2 in cluster mode** to spawn one application process per CPU core, sharing the port via internal load balancing. In Docker, deploy on **Kubernetes/ECS** and scale by increasing the task container replica count, letting orchestrator load balancers route traffic.

- **Q: How can you create a custom parameter decorator, and how does it retrieve request data?**
  → Use `createParamDecorator()`, which receives a data parameter and the `ExecutionContext`. You switch the context to HTTP and extract properties directly from the request object:
  ```typescript
  export const CurrentUser = createParamDecorator((data, ctx: ExecutionContext) => {
    return ctx.switchToHttp().getRequest().user;
  });
  ```

- **Q: What is Aspect-Oriented Programming (AOP), and how is it implemented in Nest.js?**
  → AOP is a paradigm focusing on separating cross-cutting concerns (logging, caching, security validation) from business logic. Nest implements AOP using **Guards, Interceptors, Pipes, and Exception Filters**, which intercept execution contexts without requiring code modifications inside service methods.

- **Q: Why does `next.handle()` in an interceptor return an RxJS Observable, and how does this affect controller execution?**
  → `next.handle()` represents the route handler execution stream. Because RxJS Observables are cold, the controller method **will not execute** until you return or subscribe to the stream. This allows interceptors to inspect authorization, inject caching, or abort execution before the controller is ever triggered.

- **Q: How do you configure a Nest.js application to run as a microservice using TCP, and how does the client communicate with it?**
  → In `main.ts`, bootstrap the application using `NestFactory.createMicroservice` passing `Transport.TCP` and port configurations. To communicate, the gateway client registers `ClientsModule` in its module imports and injects a `ClientProxy` using `@Inject('SERVICE_TOKEN')`.

- **Q: How does the compiler option `metadata` in `nest-cli.json` affect TypeScript reflection performance?**
  → Nest relies on decorators generating metadata. Generating metadata manually for complex DTO classes can slow build cycles. Enabling CLI plugins (like the Swagger or CRUD plugins) dynamically generates class annotations during compilation, saving performance.

- **Q: What is the purpose of `@nestjs/terminus`, and how does it prevent traffic routing to unhealthy nodes?**
  → Terminus configures health endpoints (liveness/readiness checks) that monitor CPU, memory, and database connections. Orchestrators (like Kubernetes) query this route periodically; if a check fails (e.g. database goes down), Terminus returns `503 Service Unavailable`, signaling the load balancer to remove the node from active traffic rotation.

- **Q: How does BullMQ manage job persistence, and how do you configure retries for failed jobs in Nest.js?**
  → BullMQ persists jobs, payloads, and states inside a Redis database. To configure retries, inject the queue using `@InjectQueue()` and pass options like `attempts: 3` and `backoff: { type: 'exponential', delay: 5000 }` to `queue.add()`.

---

## 🎯 Review & Practice Exercises

1. **Life Cycle Test:** Set up an interceptor and a guard on the same route. Log timestamps inside both to visually verify that the Guard executes before the Interceptor pre-handler.
2. **Mock testing:** Write a unit test for a service, mock out database queries, and assert that service throws a custom HttpException when database unique constraints fail.
3. **Multi-stage Docker run:** Build your Nest.js app locally using the multi-stage Dockerfile from Module 16, run the container, and check the image size reduction.

Congratulations on completing the Nest.js Deep-Dive Course!

*← [16 — Production and Docker](./16_production_and_docker.md)*
