# 09 — Interceptors

> **Goal:** Create Nest.js interceptors, understand Aspect-Oriented Programming (AOP) principles, utilize RxJS operators to manipulate response streams, handle request timeouts, and build a unified API response envelope.

---

## 1. The Core Concept — Aspect-Oriented Programming

**Interceptors** are classes that implement the `NestInterceptor` interface and are decorated with `@Injectable()`. Interceptors are inspired by Aspect-Oriented Programming (AOP) techniques. They allow you to:

- Bind extra logic **before** and **after** method execution.
- Transform the result returned from a handler.
- Transform the exception thrown from a handler.
- Extend basic function behavior (e.g. add caching or request timeouts).

The mental model is:

```
Request ──> Middleware ──> Guards ──> [ Interceptor (Pre-handler) ] ──> Pipes ──> Controller
                                                                                    │
Response <── [ Interceptor (Post-handler / RxJS) ] <────────────────────────────────┘
```

Unlike guards, interceptors wrap the execution flow, allowing you to intercept both the request going in *and* the response coming out.

---

## 2. Using `CallHandler` and RxJS Observables

The `intercept()` method takes two parameters: the `ExecutionContext` (which we saw in Module 08) and a `CallHandler`.

```typescript
export interface NestInterceptor {
  intercept(context: ExecutionContext, next: CallHandler): Observable<any>;
}
```

The `CallHandler` represents the route handler execution stream. If you do not call `next.handle()`, the route handler method will **never** be executed.

`next.handle()` returns an RxJS `Observable`. We use RxJS operators (like `map`, `tap`, `catchError`, `timeout`) to modify the stream before it is returned as an HTTP response.

### Logging Execution Time with `tap()`

The `tap()` operator executes side effects without modifying the stream values:

```typescript
// src/common/interceptors/logging.interceptor.ts
import { Injectable, NestInterceptor, ExecutionContext, CallHandler } from '@nestjs/common';
import { Observable } from 'rxjs';
import { tap } from 'rxjs/operators';

@Injectable()
export class LoggingInterceptor implements NestInterceptor {
  intercept(context: ExecutionContext, next: CallHandler): Observable<any> {
    console.log('Before route handler execution...');
    const now = Date.now();

    return next
      .handle()
      .pipe(
        tap(() => console.log(`After route execution... took ${Date.now() - now}ms`))
      );
  }
}
```

---

## 3. Practical Application — API Response Envelope

By default, controllers return raw database payloads or plain JSON strings. Let's write an interceptor that intercepts all outgoing responses and nests them inside a standardized envelope wrapper:

```json
{
  "success": true,
  "data": <handler return value>,
  "timestamp": "2026-06-16T14:52:17Z"
}
```

### Implementing `TransformInterceptor`

The `map()` operator transforms the data emitted by the controller:

```typescript
// src/common/interceptors/transform.interceptor.ts
import { Injectable, NestInterceptor, ExecutionContext, CallHandler } from '@nestjs/common';
import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';

export interface ResponseEnvelope<T> {
  success: boolean;
  data: T;
  timestamp: string;
}

@Injectable()
export class TransformInterceptor<T> implements NestInterceptor<T, ResponseEnvelope<T>> {
  intercept(context: ExecutionContext, next: CallHandler): Observable<ResponseEnvelope<T>> {
    return next.handle().pipe(
      map((data) => ({
        success: true,
        data: data,
        timestamp: new Date().toISOString(),
      })),
    );
  }
}
```

Usage in a controller:

```typescript
// src/users/users.controller.ts
import { Controller, Get, UseInterceptors } from '@nestjs/common';
import { TransformInterceptor } from '../common/interceptors/transform.interceptor';

@Controller('users')
@UseInterceptors(TransformInterceptor) // Bind the envelope transformer
export class UsersController {
  @Get()
  findAll() {
    return ['User A', 'User B']; // Returns: { success: true, data: [...], timestamp: ... }
  }
}
```

---

## 4. Timeout Interceptor

We can write an interceptor to guarantee that the server terminates requests that take too long to resolve (e.g. database deadlocks or slow external API dependencies).

```typescript
// src/common/interceptors/timeout.interceptor.ts
import { 
  Injectable, 
  NestInterceptor, 
  ExecutionContext, 
  CallHandler, 
  RequestTimeoutException 
} from '@nestjs/common';
import { Observable, throwError, TimeoutError } from 'rxjs';
import { catchError, timeout } from 'rxjs/operators';

@Injectable()
export class TimeoutInterceptor implements NestInterceptor {
  intercept(context: ExecutionContext, next: CallHandler): Observable<any> {
    return next.handle().pipe(
      timeout(5000), // Wait for a maximum of 5 seconds
      catchError((err) => {
        if (err instanceof TimeoutError) {
          // Translate RxJS TimeoutError into a standard Nest HTTP Exception
          return throwError(() => new RequestTimeoutException('Request timed out'));
        }
        return throwError(() => err);
      }),
    );
  }
}
```

---

## 5. Common mistakes & gotchas

- **Forgetting to return `next.handle()`.** If your interceptor does not return the result of `next.handle().pipe(...)`, the request pipeline is broken. The controller method will run, but the client will receive an empty `200 OK` response without any payload.
- **Applying interceptors globally and breaking file streaming.** If you apply a global response transformer interceptor (like `TransformInterceptor` above) to all endpoints, it will attempt to wrap files, Excel sheets, or HTML views in your `{ success: true, data: ... }` JSON structure. This breaks raw downloads. Always exclude file export routes or inspect headers inside your interceptor:
  ```typescript
  const request = context.switchToHttp().getRequest();
  if (request.url.includes('/download')) return next.handle();
  ```
- **Confusing Interceptors with Middleware.** Middleware runs first, handles raw request/response objects, and has no reference to the route handler method. Interceptors run later, have access to `ExecutionContext` reflection fields, and wrap the execution context using RxJS.

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **AOP Wrapping.** Interceptors execute logic before the handler runs *and* after the handler returns.
2. **RxJS Streams.** Use standard RxJS operators to edit, timeout, cache, or rescue streams.
3. **Response Enveloping.** Cleanly format return payloads globally using `map()`.
4. **Execution context.** Gain class name and method execution context details.

### Interview Q&A

- **Q: What is Aspect-Oriented Programming (AOP), and how do Nest.js Interceptors fit into this?**
  → AOP is a programming paradigm that separates cross-cutting concerns (like logging, caching, execution profiling) from core business logic. Nest Interceptors implement this by intercepting and wrapping method execution, inserting functionality before or after without modifying the source method.

- **Q: What happens if your interceptor does not return `next.handle()`?**
  → The route handler (controller method) will not be executed. The request will finish immediately, bypassing the normal route routing, resulting in an empty response payload.

- **Q: How would you implement a simple API caching system using interceptors?**
  → Maintain an in-memory cache map inside the interceptor. Before calling `next.handle()`, inspect the request path. If cache contains the path, return an RxJS `of(cachedResponse)`. Otherwise, call `next.handle()` and use the `tap()` operator to store the returning response.

---

*← [08 — Guards and Authorization](./08_guards_and_authorization.md) | [10 — Database Integration →](./10_database_integration.md)*
