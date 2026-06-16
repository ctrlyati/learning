# 05 — Middleware and Request Lifecycle

> **Goal:** Build class-based and functional middleware, wire them to routes, and trace the complete request-response lifecycle of a Nest.js application.

---

## 1. Middleware in Nest.js

Middleware is a function called **before** the route handler is executed. Nest middleware is Express-compatible by default, meaning it has access to the standard `request`, `response`, and `next()` function of Express/Fastify.

The mental model is:

```
Request ──> [ Middleware 1 ] ──> [ Middleware 2 ] ──> [ Guards / Router ] ──> Controller
```

Tasks suited for middleware:
- Request logging
- Parsing headers/cookies
- IP whitelist filtering
- Rate limiting (though Nest has specific Guards for this)

---

## 2. Implementing Middleware

Nest supports both **class-based** and **functional** middleware.

### Class-based Middleware

Class-based middleware implements the `NestMiddleware` interface and must be decorated with `@Injectable()`. This allows you to inject providers (like ConfigService or database clients) into the middleware.

```typescript
// src/common/middleware/logger.middleware.ts
import { Injectable, NestMiddleware } from '@nestjs/common';
import { Request, Response, NextFunction } from 'express';

@Injectable()
export class LoggerMiddleware implements NestMiddleware {
  use(req: Request, res: Response, next: NextFunction) {
    const start = Date.now();
    const { method, originalUrl } = req;

    res.on('finish', () => {
      const duration = Date.now() - start;
      const { statusCode } = res;
      console.log(`[HTTP] ${method} ${originalUrl} ${statusCode} - ${duration}ms`);
    });

    next(); // Pass control to the next middleware
  }
}
```

### Functional Middleware

If your middleware doesn't need to inject any DI services, you can write it as a simple function:

```typescript
// src/common/middleware/functional-logger.middleware.ts
import { Request, Response, NextFunction } from 'express';

export function functionalLogger(req: Request, res: Response, next: NextFunction) {
  console.log(`[Functional Logger] Requesting URL: ${req.url}`);
  next();
}
```

---

## 3. Registering Middleware

Unlike controllers or providers, middleware is **not** registered in the `@Module()` decorator properties. Instead, you configure it inside the module class using the `configure()` method, which implements the `NestModule` interface.

### Binding to Specific Controllers/Routes

```typescript
// src/app.module.ts
import { Module, NestModule, MiddlewareConsumer, RequestMethod } from '@nestjs/common';
import { LoggerMiddleware } from './common/middleware/logger.middleware';
import { UsersController } from './users/users.controller';
import { UsersModule } from './users/users.module';

@Module({
  imports: [UsersModule],
})
export class AppModule implements NestModule {
  configure(consumer: MiddlewareConsumer) {
    consumer
      .apply(LoggerMiddleware)
      .exclude(
        { path: 'users/active', method: RequestMethod.GET } // Exclude specific routes
      )
      .forRoutes(UsersController); // Bind to all routes in UsersController
  }
}
```

### Global Middleware

To register middleware globally for every single incoming request, register it inside `src/main.ts`:

```typescript
// src/main.ts
import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';
import { functionalLogger } from './common/middleware/functional-logger.middleware';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);
  
  // Register functional middleware globally
  app.use(functionalLogger);

  await app.listen(3000);
}
bootstrap();
```

---

## 4. The Complete Request-Response Lifecycle

Understanding the exact sequence in which Nest executes components is critical. When a request hits the port, Nest handles it in this precise order:

```
1. Global Middleware
2. Module Middleware
3. Global Guards
4. Controller Guards
5. Route Guards
6. Global Interceptors (Pre-handler)
7. Controller Interceptors (Pre-handler)
8. Route Interceptors (Pre-handler)
9. Global Pipes
10. Controller Pipes
11. Route Pipes
12. Parameter Pipes
13. Controller Route Handler (Business Logic)
14. Route Interceptors (Post-handler / RxJS stream)
15. Controller Interceptors (Post-handler)
16. Global Interceptors (Post-handler)
17. Exception Filters (Runs only if an error is thrown)
18. Send HTTP Response
```

### Trace Details
- **Middleware:** Executes first. Returns immediately if `next()` is not called.
- **Guards:** Determine whether the request is authorized. Returns boolean.
- **Interceptors (Pre):** Aspect-Oriented handlers. Binds logic before the controller executes.
- **Pipes:** Transform incoming types and validate the shape of inputs.
- **Interceptors (Post):** Process returning controller streams (e.g. mapping formatting, timeout handling).
- **Filters:** Handle any uncaught exceptions thrown during the execution flow.

---

## 5. Practical Application — Rate Limiter Middleware

Let's build a simple middleware that rate limits requests to `/api/sensitive` paths based on client IP.

```typescript
// src/common/middleware/ip-limiter.middleware.ts
import { Injectable, NestMiddleware, HttpStatus } from '@nestjs/common';
import { Request, Response, NextFunction } from 'express';

@Injectable()
export class IpLimiterMiddleware implements NestMiddleware {
  // Simple in-memory tracker (for demonstration only; use Redis in production)
  private ipRequestCounts = new Map<string, { count: number; resetTime: number }>();
  private readonly LIMIT = 10; // Max requests
  private readonly WINDOW = 60000; // 1 minute in ms

  use(req: Request, res: Response, next: NextFunction) {
    const ip = req.ip || req.socket.remoteAddress || 'unknown';
    const now = Date.now();

    let rateData = this.ipRequestCounts.get(ip);

    if (!rateData || now > rateData.resetTime) {
      rateData = { count: 0, resetTime: now + this.WINDOW };
    }

    rateData.count++;
    this.ipRequestCounts.set(ip, rateData);

    if (rateData.count > this.LIMIT) {
      res.status(HttpStatus.TOO_MANY_REQUESTS).json({
        statusCode: HttpStatus.TOO_MANY_REQUESTS,
        error: 'Too Many Requests',
        message: 'Rate limit exceeded. Try again in a minute.',
      });
      return; // Stop execution, do not call next()
    }

    next();
  }
}
```

Registering this inside `AppModule`:

```typescript
// src/app.module.ts
import { Module, NestModule, MiddlewareConsumer } from '@nestjs/common';
import { IpLimiterMiddleware } from './common/middleware/ip-limiter.middleware';

@Module({})
export class AppModule implements NestModule {
  configure(consumer: MiddlewareConsumer) {
    consumer
      .apply(IpLimiterMiddleware)
      .forRoutes('sensitive-data'); // Protects /sensitive-data endpoint
  }
}
```

---

## 6. Common mistakes & gotchas

- **Forgetting `next()`.** If you omit calling `next()` in your middleware handler, Nest will freeze. The request will spin indefinitely, and the server will never send a response to the client.
- **Trying to inject providers into Functional Middleware.** Functional middleware is just a function. It does not run in the DI container context, so you cannot inject services into it. Use class-based middleware for any tasks requiring ConfigService, Loggers, or database configurations.
- **Registering Global Class Middleware incorrectly.** You cannot use `app.use(MyClassMiddleware)` in `main.ts` because `app.use()` expects a raw Express middleware function. To use class-based middleware globally with full DI capabilities, apply it to all routes `*` inside the root `AppModule` configure method.

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **Express Compatibility.** Middleware acts exactly like standard Express middleware.
2. **Class vs. Function.** Write class-based middleware for DI support; write functions for stateless logic.
3. **Module Binding.** Map middleware to routes inside module `configure()` hooks.
4. **Execution Order.** Middleware runs first in the Nest.js HTTP lifecycle.

### Interview Q&A

- **Q: What is the exact sequence of execution for Middleware, Guards, Interceptors, and Pipes?**
  → The incoming request runs through **Middleware** first, then validation **Guards**, then **Interceptors** (pre-handler hook), then validation **Pipes**, then the **Controller handler**.

- **Q: Why can't you use Dependency Injection inside functional middleware?**
  → Functional middleware is a plain JavaScript function executed outside the Nest Dependency Injection container context. It has no access to Nest providers or injector graphs.

- **Q: How do you register a class-based middleware to run globally while retaining DI capabilities?**
  → Implement `NestModule` in the root `AppModule` and register the middleware with `consumer.apply(MyMiddleware).forRoutes('*')`.

---

*← [04 — Modules and Namespaces](./04_modules_and_namespaces.md) | [06 — Pipes and Validation →](./06_pipes_and_validation.md)*
