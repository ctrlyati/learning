# 04 — Modules and Namespaces

> **Goal:** Structure Nest.js applications using Modules, master component encapsulation, implement shared and global modules, and configure custom Dynamic Modules.

---

## 1. Modules — The Architectural Foundation

Every Nest.js application is a tree of modules. A module is a class decorated with `@Module()`, which defines the boundary, dependencies, and visibility of components.

The mental model is:

```
                  [ AppModule (Root) ]
                 /                    \
     [ UsersModule ]               [ AuthModule ]
        /        \                    /       \
 [Controller] [Service]        [Controller] [Service]
```

### The `@Module()` Decorator Properties

| Option | Type | Description |
|--------|------|-------------|
| `imports` | `Array` | Other modules that export the providers this module needs. |
| `controllers` | `Array` | The set of controllers defined in this module that handle HTTP routing. |
| `providers` | `Array` | The services, factories, or helper classes instantiated by the DI container in this module. |
| `exports` | `Array` | The subset of providers defined in this module that should be visible to other modules importing this one. |

### Module Encapsulation (Crucial)

By default, providers are **encapsulated**. They are private to the module they are declared in.

If `UsersService` is declared in `UsersModule`'s `providers`, and `AuthService` inside `AuthModule` needs to inject it, you must follow these two steps:

1. **Export** `UsersService` from `UsersModule`:
```typescript
// src/users/users.module.ts
@Module({
  providers: [UsersService],
  exports: [UsersService], // Export UserService to make it public
})
export class UsersModule {}
```

2. **Import** `UsersModule` into `AuthModule`:
```typescript
// src/auth/auth.module.ts
@Module({
  imports: [UsersModule], // Import module to gain access to exported providers
  providers: [AuthService],
})
export class AuthModule {}
```

---

## 2. Global Modules & Reusability

If you have utility providers that are used everywhere (e.g. database client, logger, configuration), importing their modules into every single feature module becomes tedious.

Nest allows you to register modules as **Global** using the `@Global()` decorator.

```typescript
// src/common/database.module.ts
import { Module, Global } from '@nestjs/common';
import { DatabaseService } from './database.service';

@Global() // Makes DatabaseModule global
@Module({
  providers: [DatabaseService],
  exports: [DatabaseService],
})
export class DatabaseModule {}
```

Once `DatabaseModule` is loaded in the root `AppModule`, its exported providers are automatically available for injection in **any** module in the application.

*Warning: Global modules should be used sparingly. Making everything global pollutes the injection space and obscures architectural boundaries.*

---

## 3. Dynamic Modules

Standard modules are static: they wire up hardcoded providers and classes. **Dynamic modules** allow modules to be customized dynamically with configurations when they are imported.

The classic example is a configuration module that requires a filepath option, or a database module that requires database connection credentials.

### Writing a Dynamic Module

Dynamic modules must return an object implementing `DynamicModule` (which inherits the properties of standard modules + adds a dynamic `module` field).

By convention, Nest modules use these static method names:
- `register()`: Configure a module for a single use case (unique options).
- `forRoot()`: Configure a module once globally (e.g. database setup).
- `forFeature()`: Configure a module subclassing from root configurations (e.g. mapping schema models).

Here is a configurable custom Logger module:

```typescript
// src/logger/logger-options.interface.ts
export interface LoggerOptions {
  level: 'info' | 'warn' | 'error' | 'debug';
  prefix?: string;
}

// src/logger/logger.module.ts
import { Module, DynamicModule } from '@nestjs/common';
import { LoggerService } from './logger.service';

@Module({})
export class LoggerModule {
  static register(options: LoggerOptions): DynamicModule {
    return {
      module: LoggerModule,
      providers: [
        {
          provide: 'LOGGER_OPTIONS',
          useValue: options,
        },
        LoggerService,
      ],
      exports: [LoggerService],
    };
  }
}
```

```typescript
// src/logger/logger.service.ts
import { Injectable, Inject } from '@nestjs/common';
import { LoggerOptions } from './logger-options.interface';

@Injectable()
export class LoggerService {
  constructor(
    @Inject('LOGGER_OPTIONS') private readonly options: LoggerOptions
  ) {}

  log(msg: string) {
    const prefix = this.options.prefix ? `[${this.options.prefix}] ` : '';
    console.log(`${prefix}${this.options.level.toUpperCase()}: ${msg}`);
  }
}
```

### Importing a Dynamic Module

Import it in your feature modules using the static configuration method:

```typescript
// src/app.module.ts
import { Module } from '@nestjs/common';
import { LoggerModule } from './logger/logger.module';

@Module({
  imports: [
    LoggerModule.register({ level: 'debug', prefix: 'APP_LAYER' })
  ],
})
export class AppModule {}
```

---

## 4. Practical Application — Configurable HttpClient Module

Let's build an HTTP fetch dynamic module that can load remote configs asynchronously.

```typescript
// src/http/http-options.ts
export interface HttpOptions {
  baseUrl: string;
}

// src/http/http.service.ts
import { Injectable, Inject } from '@nestjs/common';
import { HttpOptions } from './http-options';

@Injectable()
export class HttpService {
  constructor(@Inject('HTTP_OPTIONS') private options: HttpOptions) {}

  async fetch(path: string) {
    const url = `${this.options.baseUrl}${path}`;
    console.log(`Sending GET request to ${url}...`);
    return { url, data: {} };
  }
}

// src/http/http.module.ts
import { Module, DynamicModule, Provider } from '@nestjs/common';
import { HttpService } from './http.service';
import { HttpOptions } from './http-options';

@Module({})
export class HttpModule {
  // Static configuration
  static register(options: HttpOptions): DynamicModule {
    return {
      module: HttpModule,
      providers: [
        { provide: 'HTTP_OPTIONS', useValue: options },
        HttpService,
      ],
      exports: [HttpService],
    };
  }

  // Asynchronous configuration pattern (Common in Nest.js libraries)
  static registerAsync(options: {
    useFactory: (...args: any[]) => Promise<HttpOptions> | HttpOptions;
    inject?: any[];
  }): DynamicModule {
    
    const optionsProvider: Provider = {
      provide: 'HTTP_OPTIONS',
      useFactory: options.useFactory,
      inject: options.inject || [],
    };

    return {
      module: HttpModule,
      imports: [],
      providers: [optionsProvider, HttpService],
      exports: [HttpService],
    };
  }
}
```

---

## 5. Common mistakes & gotchas

- **Double declaration in `providers`.** Declaring `UsersService` in both `UsersModule` and `AppModule` creates two independent instances in the DI container. This defeats the singleton model and causes bugs if the service stores in-memory state. Declare it once in `UsersModule` and export it.
- **Circular module dependencies.** If `Module A` imports `Module B` and `Module B` imports `Module A`, Nest will throw a runtime error. Resolve this by wrapping imports on both sides in `forwardRef()`:
  ```typescript
  // module-a.ts
  imports: [forwardRef(() => ModuleB)]
  ```
- **Forgetting exports in dynamic modules.** When defining `register()`, if you return `providers: [MyService]` but forget to write `exports: [MyService]`, the importing modules won't be able to resolve `MyService`.

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **Module Scope.** Providers are private to their modules by default.
2. **Global Caution.** Use `@Global()` rarely; clear architectural lines are cleaner.
3. **Dynamic Customization.** Dynamic modules allow setting configurations on imports via helper methods.
4. **Circular Resolution.** Solve circular module boundaries using `forwardRef` in the module imports.

### Interview Q&A

- **Q: What is the purpose of the `exports` array in a Nest.js module?**
  → It exposes private module providers to other modules. Any module importing this module gets access only to providers listed in `exports`.

- **Q: When and why would you make a module Dynamic?**
  → When a module needs to be customized dynamically before instantiation (like reading environment configurations, DB credentials, API endpoints, or third-party SDK parameters).

- **Q: How does `forRootAsync` differ from `forRoot`?**
  → `forRoot` configures modules synchronously at startup. `forRootAsync` accepts factory providers and dynamic dependency arrays, allowing Nest to resolve configuration settings asynchronously (e.g. fetching credentials from ConfigService or AWS Secrets Manager).

---

*← [03 — Providers and DI](./03_providers_and_di.md) | [05 — Middleware and Request Lifecycle →](./05_middleware_and_request_lifecycle.md)*
