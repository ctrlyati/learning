# 07 — Exception Filters

> **Goal:** Leverage the Nest.js exception handling layer, use built-in HTTP exceptions, and build custom Exception Filters to format standard error response schemas.

---

## 1. The Core Concept — The Exception Zone

Nest.js ships with a built-in **exceptions layer** that catches all unhandled exceptions across the application. When an exception is not caught by your code, Nest intercepts it, formats it, and returns a clean JSON response.

By default, any unhandled error will output:

```json
{
  "statusCode": 500,
  "message": "Internal server error"
}
```

This prevents database connection details or stack traces from leaking to clients.

The mental model is:

```
Controller / Service ──(Throws Exception)──> [ Exception Layer (Filters) ] ──> Formatted JSON Response
```

---

## 2. Built-in HTTP Exceptions

Nest.js provides a large set of standard exceptions inheriting from `HttpException`. You should throw these directly inside your services:

```typescript
import { Injectable, NotFoundException, BadRequestException } from '@nestjs/common';

@Injectable()
export class UsersService {
  findOne(id: number) {
    const user = this.database.find(id);
    if (!user) {
      // Automatically triggers a 404 response
      throw new NotFoundException(`User with ID ${id} not found`);
    }
    return user;
  }
}
```

### Common Built-in Exceptions

- `BadRequestException` (400)
- `UnauthorizedException` (401)
- `ForbiddenException` (403)
- `NotFoundException` (404)
- `ConflictException` (409)
- `InternalServerErrorException` (500)

---

## 3. Creating Custom Exception Filters

Sometimes you want to format error messages in a custom style, add logging to specific errors, or translate database constraints directly into HTTP status codes. For this, you write an **Exception Filter**.

An exception filter must implement the `ExceptionFilter` interface and be decorated with `@Catch()`.

Let's build a global exception filter that intercepts all `HttpException` occurrences and appends a timestamp and the request path.

```typescript
// src/common/filters/http-exception.filter.ts
import { ExceptionFilter, Catch, ArgumentsHost, HttpException } from '@nestjs/common';
import { Request, Response } from 'express';

@Catch(HttpException) // Catch only HTTP exceptions
export class HttpExceptionFilter implements ExceptionFilter {
  catch(exception: HttpException, host: ArgumentsHost) {
    // 1. Switch context to HTTP
    const ctx = host.switchToHttp();
    const response = ctx.getResponse<Response>();
    const request = ctx.getRequest<Request>();
    
    // 2. Extract exception parameters
    const status = exception.getStatus();
    const exceptionResponse = exception.getResponse();

    // 3. Format error message
    const message = typeof exceptionResponse === 'object' 
      ? (exceptionResponse as any).message || exception.message
      : exceptionResponse;

    // 4. Send formatted response
    response.status(status).json({
      statusCode: status,
      timestamp: new Date().toISOString(),
      path: request.url,
      message: message,
    });
  }
}
```

---

## 4. Binding Exception Filters

Exception filters can be applied at different scopes:

### Method Scope
Applies to a single route:

```typescript
@Post()
@UseFilters(new HttpExceptionFilter())
async create(@Body() createUserDto: CreateUserDto) {
  throw new ForbiddenException();
}
```

### Controller Scope
Applies to all routes in a controller:

```typescript
@Controller('users')
@UseFilters(HttpExceptionFilter) // Can pass class definition; Nest resolves DI
export class UsersController {}
```

### Global Scope
Applies to the entire application.

Option A: Register inside `main.ts` (Does **not** support Dependency Injection):

```typescript
// src/main.ts
app.useGlobalFilters(new HttpExceptionFilter());
```

Option B: Register as a provider in `AppModule` (Supports full Dependency Injection):

```typescript
// src/app.module.ts
import { Module } from '@nestjs/common';
import { APP_FILTER } from '@nestjs/core';
import { HttpExceptionFilter } from './common/filters/http-exception.filter';

@Module({
  providers: [
    {
      provide: APP_FILTER, // Token to register a global filter
      useClass: HttpExceptionFilter,
    },
  ],
})
export class AppModule {}
```

---

## 5. Practical Application — Mapping Database Errors to HTTP

When database operations fail (e.g. Prisma throws a unique constraint error like `P2002`), it throws a raw database exception. We don't want to write try-catch loops in every controller. We can write an exception filter to catch these database errors globally and translate them into a `409 Conflict` status automatically.

```typescript
// src/common/filters/prisma-client-exception.filter.ts
import { ExceptionFilter, Catch, ArgumentsHost, HttpStatus } from '@nestjs/common';
import { Response } from 'express';

// Define a structural error type representing database unique constraints
interface PrismaError {
  code: string;
  meta?: { target?: string[] };
  message: string;
}

@Catch() // Empty @Catch() catches ALL unhandled exceptions (HTTP + raw system errors)
export class PrismaClientExceptionFilter implements ExceptionFilter {
  catch(exception: any, host: ArgumentsHost) {
    const ctx = host.switchToHttp();
    const response = ctx.getResponse<Response>();

    // Check if error is a Prisma unique constraint violation (Code P2002)
    if (exception.code === 'P2002') {
      const status = HttpStatus.CONFLICT;
      const target = exception.meta?.target ? exception.meta.target.join(', ') : 'field';

      response.status(status).json({
        statusCode: status,
        message: `Database conflict: Record with this ${target} already exists.`,
        error: 'Conflict',
      });
      return;
    }

    // Default fallback for other unexpected errors (e.g. 500)
    response.status(HttpStatus.INTERNAL_SERVER_ERROR).json({
      statusCode: HttpStatus.INTERNAL_SERVER_ERROR,
      message: 'Internal Database Query Error',
    });
  }
}
```

---

## 6. Common mistakes & gotchas

- **Catching global errors and losing custom validation structures.** When using the global `ValidationPipe`, validation errors are thrown as a specialized `BadRequestException` carrying an array of validation validation constraints. If your custom exception filter overrides this output using a plain string conversion, the client will lose the validation array detail. Always inspect if `exception.getResponse()` returns an object carrying sub-messages.
- **Using a raw database error filter and exposing details.** If a database server goes down, database clients throw connection errors. A catch-all filter must be careful to never print the raw database error strings to the JSON response; otherwise, database connection string variables, usernames, or table schemas could leak.
- **Forgetting that `main.ts` global filters cannot use DI.** If you register a filter using `app.useGlobalFilters(new HttpExceptionFilter(myService))`, you must resolve and pass `myService` manually. Always use the `APP_FILTER` injection token approach in `AppModule` instead if your exception filter requires configuration parameters or logging services.

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **Uncaught Shield.** Nest's exception layer catches all unhandled errors, preventing security leaks.
2. **HttpException Hierarchy.** Use built-in exceptions like `NotFoundException` directly in services.
3. **Catch Scope.** Specify target exception types inside `@Catch(Type)` to build specific mapping handlers.
4. **DI Injection.** Use the `APP_FILTER` token to inject services into global exception filters.

### Interview Q&A

- **Q: What is the benefit of registering a global exception filter in `AppModule` using the `APP_FILTER` token over registering it in `main.ts` using `app.useGlobalFilters()`?**
  → Registering via the `APP_FILTER` token runs the filter inside the dependency injection container, allowing you to inject services (e.g. loggers, configuration providers) into the filter's constructor. `main.ts` registration does not support DI resolving.

- **Q: How can you write a catch-all exception filter that catches both HTTP exceptions and system-level errors?**
  → Leave the `@Catch()` decorator empty. Inside the filter, check if the error is an instance of `HttpException` to extract HTTP status codes; otherwise, fall back to a generic `500 Internal Server Error`.

- **Q: What happens to validation error details if you write an exception filter that does not inspect `exception.getResponse()`?**
  → The detailed validation constraints array (e.g., `["email must be an email"]` generated by the global `ValidationPipe`) will be hidden, and the client will only receive a generic `BadRequestException` message string.

---

*← [06 — Pipes and Validation](./06_pipes_and_validation.md) | [08 — Guards and Authorization →](./08_guards_and_authorization.md)*
