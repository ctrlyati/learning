# 03 — Providers and DI

> **Goal:** Master the Dependency Injection (DI) system in Nest.js, understand providers, configure custom providers, control injection scopes, and resolve circular dependencies.

---

## 1. The Core Concept — Inversion of Control and DI

In traditional development, if `UserController` needs `UserService`, it instantiates it directly:

```typescript
// Anti-pattern in modular applications
export class UserController {
  private userService = new UserService(); // Hard coupling
}
```

This makes testing difficult (you can't easily mock `UserService`) and creates tight coupling.

Nest.js resolves this using **Inversion of Control (IoC)**. Instead of your classes creating their dependencies, they declare what they need, and the **Nest DI Container** instantiates and injects those dependencies at runtime.

The mental model is:

```
1. Write class + `@Injectable()`
2. Register in `@Module({ providers: [MyService] })`
3. Declare in constructor: `constructor(private myService: MyService) {}`
4. Nest DI Container instantiates it once (Singleton) and passes it in.
```

---

## 2. Standard and Custom Providers

A provider is a class or value that can be injected into other classes. When you write `providers: [UserService]`, it is actually a shorthand for:

```typescript
providers: [
  {
    provide: UserService, // The Injection Token (can be a class, string, or symbol)
    useClass: UserService,  // The Class to instantiate
  }
]
```

Nest supports several types of custom providers.

### Value Providers (`useValue`)

Value providers are useful for injecting constant values, configuration objects, or external libraries.

```typescript
// src/app.module.ts
import { Module } from '@nestjs/common';

const API_CONFIG = {
  endpoint: 'https://api.github.com',
  timeout: 5000,
};

@Module({
  providers: [
    {
      provide: 'API_CONFIG', // Injection token is a string
      useValue: API_CONFIG,
    },
  ],
})
export class AppModule {}
```

To inject a provider that uses a string token, you must use the `@Inject()` decorator:

```typescript
// src/app.service.ts
import { Injectable, Inject } from '@nestjs/common';

@Injectable()
export class AppService {
  constructor(
    @Inject('API_CONFIG') private readonly config: any
  ) {}

  getEndpoint(): string {
    return this.config.endpoint;
  }
}
```

### Factory Providers (`useFactory`)

Factory providers are useful when dependencies must be created dynamically based on conditions, environment variables, or other injected services.

```typescript
// src/database.module.ts
import { Module } from '@nestjs/common';
import { LoggerService } from './logger.service';

@Module({
  providers: [
    LoggerService,
    {
      provide: 'CONNECTION_POOL',
      // useFactory accepts parameters matching items in the inject array
      useFactory: (logger: LoggerService) => {
        logger.log('Initializing DB connection pool...');
        const isProd = process.env.NODE_ENV === 'production';
        return isProd ? { poolSize: 50 } : { poolSize: 5 };
      },
      inject: [LoggerService], // Declare dependencies required by the factory function
    },
  ],
})
export class DatabaseModule {}
```

---

## 3. Provider Scopes

By default, every provider in Nest is a **Singleton**. A single instance is created at startup and shared across all request handlers. However, Nest allows you to customize the provider lifetime:

| Scope | Description | When to use |
|-------|-------------|-------------|
| **DEFAULT** (Singleton) | One instance shared across the entire application. | Default option. Best for performance and memory. |
| **REQUEST** | A new instance is created for *every* incoming HTTP request. | Storing request-scoped state (e.g. tenant IDs, auth tokens). |
| **TRANSIENT** | A new instance is created for *every* consumer class that injects it. | Lightweight, stateless utilities like loggers or counters. |

### Defining Scopes

```typescript
import { Injectable, Scope } from '@nestjs/common';

@Injectable({ scope: Scope.REQUEST })
export class RequestScopedService {
  // This class will be instantiated on every HTTP request
}
```

*Warning: If Controller A (Singleton) injects Service B (Request-scoped), Controller A automatically becomes request-scoped as well. Request-scope is bubble-up. This can negatively impact performance under high traffic.*

---

## 4. Circular Dependencies and `forwardRef`

A circular dependency occurs when `Class A` depends on `Class B`, and `Class B` depends on `Class A` at the same time.

```typescript
// users.service.ts
@Injectable()
export class UsersService {
  constructor(private authService: AuthService) {} // Needs Auth
}

// auth.service.ts
@Injectable()
export class AuthService {
  constructor(private usersService: UsersService) {} // Needs Users
}
```

Nest will fail to compile this dependency graph because both classes wait for the other to be initialized. To resolve this, use `forwardRef()` on both sides:

```typescript
// users.service.ts
import { Injectable, Inject, forwardRef } from '@nestjs/common';
import { AuthService } from './auth.service';

@Injectable()
export class UsersService {
  constructor(
    @Inject(forwardRef(() => AuthService))
    private authService: AuthService,
  ) {}
}
```

```typescript
// auth.service.ts
import { Injectable, Inject, forwardRef } from '@nestjs/common';
import { UsersService } from './users.service';

@Injectable()
export class AuthService {
  constructor(
    @Inject(forwardRef(() => UsersService))
    private usersService: UsersService,
  ) {}
}
```

---

## 5. Practical Application — Dynamic ApiClient Factory

Let's build a practical ApiClient provider using `useFactory`. It switches HTTP configs based on configuration status.

```typescript
// src/api/api-client.ts
export class ApiClient {
  constructor(private readonly baseUrl: string, private readonly timeout: number) {}
  
  async getStatus() {
    return { baseUrl: this.baseUrl, status: 'healthy', timeout: this.timeout };
  }
}

// src/api/api.module.ts
import { Module } from '@nestjs/common';
import { ApiClient } from './api-client';

@Module({
  providers: [
    {
      provide: 'CONFIG_ENV',
      useValue: { env: process.env.NODE_ENV || 'development' },
    },
    {
      provide: ApiClient,
      useFactory: (config: { env: string }) => {
        if (config.env === 'production') {
          return new ApiClient('https://api.production.com', 2000);
        }
        return new ApiClient('https://api.staging.com', 10000);
      },
      inject: ['CONFIG_ENV'],
    },
  ],
  exports: [ApiClient], // Export ApiClient so other modules can use it
})
export class ApiModule {}
```

Usage in a controller:

```typescript
// src/app.controller.ts
import { Controller, Get } from '@nestjs/common';
import { ApiClient } from './api/api-client';

@Controller('api')
export class AppController {
  constructor(private readonly apiClient: ApiClient) {} // Automatically resolved

  @Get('status')
  async getStatus() {
    return this.apiClient.getStatus();
  }
}
```

---

## 6. Common mistakes & gotchas

- **Injecting request-scoped services into singleton wrappers.** If your database transaction service is request-scoped, and you inject it into a singleton router listener, the singleton will automatically be downgraded to request-scoped. This increases resource allocations.
- **Forgetting to export providers.** If `Module A` declares `UserService` inside its `providers` array, and `Module B` imports `Module A`, `Module B` still cannot inject `UserService` unless `Module A` explicitly lists it in its `exports` array.
- **String token name typos.** When using `@Inject('MY_TOKEN')`, a simple case-sensitive mismatch like `@Inject('My_Token')` will cause Nest to throw a runtime error. It is best practice to define tokens in a central `constants.ts` file.

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **Singleton Default.** Keep providers singleton unless request state dictates otherwise.
2. **Factory Dynamic.** Use `useFactory` to wrap third-party database clients or configurations.
3. **Decoupled Tokens.** Providers can be matched by Class, string name, or JavaScript Symbols.
4. **Export Requirements.** To share a provider, a module must register it in `providers`, and list it in `exports`.

### Interview Q&A

- **Q: Explain the difference between Singleton, Request, and Transient provider scopes.**
  → *Singleton* creates one instance shared globally. *Request* creates an instance per HTTP request. *Transient* creates a new instance for every injecting class.

- **Q: How does `forwardRef()` solve circular dependencies in Nest.js?**
  → `forwardRef()` returns a deferred wrapper. Instead of compiling class definitions immediately, Nest resolves the references lazily at instantiation time.

- **Q: Why would you use `useFactory` over `useClass`?**
  → `useClass` initiates a static class. `useFactory` runs a dynamic function that can fetch configurations, accept parameter overrides, execute logic, and conditionally return differing implementations.

---

*← [02 — Controllers and Routing](./02_controllers_and_routing.md) | [04 — Modules and Namespaces →](./04_modules_and_namespaces.md)*
