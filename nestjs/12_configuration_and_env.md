# 12 — Configuration and Env

> **Goal:** Manage application configurations dynamically using `@nestjs/config`, load and validate environment variables using Joi, and implement type-safe configurations.

---

## 1. Core Concept — Configuration Separation

A production-grade application must follow the **Twelve-Factor App** methodology, which states that all configurations (database credentials, API keys, ports, secrets) must be stored in the **environment** and never hardcoded in source code.

Nest.js provides the `@nestjs/config` module, which wraps the `dotenv` library and manages configurations inside a unified `ConfigModule` and `ConfigService`.

The mental model is:

```
.env File / Env Variables ──> [ ConfigModule ] ──(Validation check)──> ConfigService ──> Inject to App
```

---

## 2. Setting Up `@nestjs/config`

### Install Dependencies

```bash
npm install --save @nestjs/config joi
```

*Note: We install `joi` to validate the shape and existence of environment variables at startup.*

### Basic Setup

Import `ConfigModule` inside the root `AppModule` imports array:

```typescript
// src/app.module.ts
import { Module } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';

@Module({
  imports: [
    ConfigModule.forRoot({
      isGlobal: true, // Makes ConfigService available everywhere without re-importing ConfigModule
      envFilePath: `.env.${process.env.NODE_ENV || 'development'}`, // Load different file based on environment
    }),
  ],
})
export class AppModule {}
```

---

## 3. Environment Variables Validation

If a critical variable (like `DATABASE_URL`) is missing in production, the application should fail to start immediately rather than failing later at runtime. We use `Joi` to define a strict validation schema.

```typescript
// src/app.module.ts
import { Module } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';
import * as Joi from 'joi';

@Module({
  imports: [
    ConfigModule.forRoot({
      isGlobal: true,
      validationSchema: Joi.object({
        NODE_ENV: Joi.string()
          .valid('development', 'production', 'test', 'provision')
          .default('development'),
        PORT: Joi.number().default(3000),
        DATABASE_URL: Joi.string().required(), // App will fail to boot if this is missing
        JWT_SECRET: Joi.string().required(),
        JWT_EXPIRATION: Joi.string().default('3600s'),
      }),
      validationOptions: {
        allowUnknown: true, // Allow extraneous variables in process.env
        abortEarly: true,   // Stop validation on first error
      },
    }),
  ],
})
export class AppModule {}
```

---

## 4. Injecting and Using the `ConfigService`

Inject `ConfigService` into providers or controllers to read environment variables:

```typescript
// src/auth/strategies/jwt.strategy.ts
import { Injectable } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { PassportStrategy } from '@nestjs/passport';
import { ExtractJwt, Strategy } from 'passport-jwt';

@Injectable()
export class JwtStrategy extends PassportStrategy(Strategy) {
  // Inject ConfigService in constructor
  constructor(private configService: ConfigService) {
    super({
      jwtFromRequest: ExtractJwt.fromAuthHeaderAsBearerToken(),
      ignoreExpiration: false,
      // Read variables using configService.get()
      secretOrKey: configService.get<string>('JWT_SECRET'),
    });
  }

  async validate(payload: any) {
    return { userId: payload.sub, email: payload.email };
  }
}
```

---

## 5. Type-Safe Configuration Namespaces

Using plain string lookups like `configService.get('DATABASE_URL')` works, but lacks type safety and auto-complete support in IDEs. Nest lets you write **Configuration Namespaces** to group variables into typed nested objects.

### Create a Namespace

```typescript
// src/config/database.config.ts
import { registerAs } from '@nestjs/config';

// Group DB variables under a "database" namespace
export default registerAs('database', () => ({
  host: process.env.DATABASE_HOST || 'localhost',
  port: parseInt(process.env.DATABASE_PORT || '5432', 10),
  url: process.env.DATABASE_URL,
}));
```

Register this config loader in your module:

```typescript
// src/app.module.ts
import { Module } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';
import databaseConfig from './config/database.config';

@Module({
  imports: [
    ConfigModule.forRoot({
      load: [databaseConfig], // Register namespace loaders
    }),
  ],
})
export class AppModule {}
```

### Injecting Namespaced Configs

Use the `@Inject` decorator passing the config reference:

```typescript
// src/database/database-client.ts
import { Injectable, Inject } from '@nestjs/common';
import { ConfigType } from '@nestjs/config';
import databaseConfig from '../config/database.config';

@Injectable()
export class DatabaseClient {
  constructor(
    // Inject the config namespace with complete TypeScript type protection
    @Inject(databaseConfig.KEY)
    private dbConfig: ConfigType<typeof databaseConfig>
  ) {
    // TypeScript knows dbConfig.url and dbConfig.port exists with correct types!
    console.log(`Connecting to DB: ${this.dbConfig.host}:${this.dbConfig.port}`);
  }
}
```

---

## 6. Common mistakes & gotchas

- **Assuming environment variables are typed.** By default, `process.env` treats all variables as string types. Reading `process.env.PORT` will return `"3000"` (a string). If you pass it directly to `app.listen()`, it will work, but passing string ports to other libraries might cause crashes. Always parse numbers explicitly, or use `Joi` validation schema which handles casting automatically.
- **Forgetting that `main.ts` executes before dependency injection.** You cannot inject `ConfigService` in `main.ts` before the app is instantiated. To read a port at bootstrap time, call `app.get(ConfigService)` after creating the app:
  ```typescript
  const app = await NestFactory.create(AppModule);
  const configService = app.get(ConfigService);
  const port = configService.get('PORT');
  await app.listen(port);
  ```
- **Not setting `isGlobal: true` and getting resolution errors.** If you forget to configure `isGlobal: true` inside `ConfigModule.forRoot`, trying to inject `ConfigService` in nested modules without importing `ConfigModule` directly will cause DI compilation failures.

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **Config Validation.** Validate variable presence at boot using Joi schemas to prevent runtime failures.
2. **Type Safety.** Leverage `registerAs` namespace configurations to enforce compile-time safety.
3. **App Resolving.** Retrieve services manually in `main.ts` using `app.get(ConfigService)` to bind boot ports.
4. **Environment files.** Set dynamic paths like `.env.${process.env.NODE_ENV}` to toggle testing vs. development files.

### Interview Q&A

- **Q: Why should you use `ConfigModule` validation schemas in Nest.js?**
  → Validation schemas (e.g., using Joi) check that all required environment configurations are set and have correct types at startup. This causes the process to fail immediately on bootstrap if keys are missing, preventing runtime outages.

- **Q: How does namespaced configuration via `registerAs` improve code quality?**
  → It groups related settings into nested objects (e.g. `databaseConfig`, `mailConfig`) and exposes them with full TypeScript autocomplete support via `ConfigType`, avoiding typo-prone string lookups like `config.get('DB_URL')`.

- **Q: How can you access `ConfigService` inside the root `main.ts` file?**
  → You cannot use constructor injection in `main.ts`. You must retrieve it manually from the compiled application instance by running `const configService = app.get(ConfigService)` after `NestFactory.create` finishes.

---

*← [11 — Authentication and JWT](./11_authentication_and_jwt.md) | [13 — Testing Unit and E2E →](./13_testing_unit_and_e2e.md)*
