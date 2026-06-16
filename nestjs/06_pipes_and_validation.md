# 06 — Pipes and Validation

> **Goal:** Leverage Nest.js pipes to validate incoming request bodies (DTOs), transform input data types, create custom validation rules, and configure global input validation.

---

## 1. Core Concepts — Validation and Transformation

In Nest.js, **Pipes** are classes implementing the `PipeTransform` interface. Pipes have two primary use cases:

- **Transformation:** Convert input data to the desired format or type (e.g. converting a string `'42'` to an integer `42`).
- **Validation:** Check incoming data against validation constraints and throw an exception if the data is invalid.

The mental model is:

```
Request Data ──> [ Pipe (Validate / Transform) ] ──> Controller Handler
                      │
                      └── (Invalid) ──> Throws HTTP BadRequestException (400)
```

Nest provides several built-in pipes, including `ParseIntPipe`, `ParseFloatPipe`, `ParseBoolPipe`, `ParseArrayPipe`, `ParseUUIDPipe`, and `ValidationPipe`.

---

## 2. Using Built-in Pipes for Transformation

You can bind pipes directly to parameter decorators inside your controllers.

```typescript
// src/users/users.controller.ts
import { Controller, Get, Param, ParseIntPipe, ParseUUIDPipe } from '@nestjs/common';

@Controller('users')
export class UsersController {
  
  // GET /users/42 (Converts id to a number, throws 400 Bad Request if not a numeric string)
  @Get(':id')
  findOne(@Param('id', ParseIntPipe) id: number) {
    console.log(typeof id); // "number"
    return { id };
  }

  // GET /users/uuid/abc-123 (Throws 400 if ID is not a valid UUID)
  @Get('uuid/:id')
  findUUID(@Param('id', ParseUUIDPipe) id: string) {
    return { uuid: id };
  }
}
```

---

## 3. DTO Validation with `class-validator`

For request bodies (`@Body()`), Nest integrates with two libraries to provide decorator-based validation: `class-validator` and `class-transformer`.

### Install Dependencies

```bash
npm i --save class-validator class-transformer
```

### Create a DTO (Data Transfer Object)

We decorate DTO properties with validator rules:

```typescript
// src/users/dto/create-user.dto.ts
import { IsString, IsEmail, MinLength, MaxLength, IsInt, Min, IsOptional } from 'class-validator';

export class CreateUserDto {
  @IsString()
  @MinLength(3)
  @MaxLength(20)
  readonly username: string;

  @IsEmail()
  readonly email: string;

  @IsInt()
  @Min(18)
  readonly age: number;

  @IsString()
  @IsOptional()
  readonly bio?: string;
}
```

### Wire up the Validation Pipe

To trigger validation, you must apply the `ValidationPipe`. You can apply it globally in `main.ts`:

```typescript
// src/main.ts
import { NestFactory } from '@nestjs/core';
import { ValidationPipe } from '@nestjs/common';
import { AppModule } from './app.module';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);

  // Apply ValidationPipe globally to ALL endpoints
  app.useGlobalPipes(new ValidationPipe({
    whitelist: true,               // Strips out any properties that do not have validation decorators
    forbidNonWhitelisted: true,    // Throws a 400 Bad Request if extra non-whitelisted properties are sent
    transform: true,               // Automatically transforms payloads to match DTO class instances
  }));

  await app.listen(3000);
}
bootstrap();
```

Inside the controller:

```typescript
// src/users/users.controller.ts
import { Controller, Post, Body } from '@nestjs/common';
import { CreateUserDto } from './dto/create-user.dto';

@Controller('users')
export class UsersController {
  @Post()
  create(@Body() createUserDto: CreateUserDto) {
    // Because transform: true is enabled globally,
    // createUserDto is an actual instance of the CreateUserDto class
    return createUserDto;
  }
}
```

---

## 4. Writing Custom Pipes

You can write custom pipes to handle specific validation or transformation logic. A pipe must implement `value` and `metadata` handling.

Let's build a pipe that sanitizes input string fields by trimming whitespace.

```typescript
// src/common/pipes/trim-strings.pipe.ts
import { PipeTransform, ArgumentMetadata, Injectable, BadRequestException } from '@nestjs/common';

@Injectable()
export class TrimStringsPipe implements PipeTransform {
  // value: the input value
  // metadata: type, metatype, data
  transform(value: any, metadata: ArgumentMetadata) {
    if (typeof value === 'string') {
      return value.trim();
    }

    if (value && typeof value === 'object') {
      this.trimObject(value);
    }

    return value;
  }

  private trimObject(obj: any) {
    Object.keys(obj).forEach((key) => {
      if (typeof obj[key] === 'string') {
        obj[key] = obj[key].trim();
      } else if (typeof obj[key] === 'object') {
        this.trimObject(obj[key]);
      }
    });
  }
}
```

Usage in a controller:

```typescript
// src/users/users.controller.ts
import { Controller, Post, Body, UsePipes } from '@nestjs/common';
import { TrimStringsPipe } from '../common/pipes/trim-strings.pipe';

@Controller('users')
export class UsersController {
  @Post('comments')
  @UsePipes(new TrimStringsPipe()) // Binds this custom pipe to the handler
  postComment(@Body() payload: { comment: string }) {
    return payload; // The comment string will be trimmed automatically
  }
}
```

---

## 5. Common mistakes & gotchas

- **Forgetting to install validation packages.** If you run Nest without `class-validator` or `class-transformer` installed, `@Body()` validation will simply do nothing.
- **DTO validation not working because type is `any` or `interface`.** Nest parses types at runtime. Because TypeScript interfaces compile down to nothing in JavaScript, Nest cannot read interface validation rules. **Always use classes** for DTOs.
- **Not enabling `transform: true` in ValidationPipe options.** If you omit `transform: true`, Nest won't compile query or route parameter types to their target types. A param route like `@Param('id', ParseIntPipe)` works because of the explicit pipe, but a query DTO like `@Query() query: PaginationQueryDto` will have all its properties left as string types.

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **Pipes run late.** Pipes execute *after* guards and interceptors, but *before* route handlers.
2. **DTO Classes.** Always define DTO contracts using TypeScript classes, not interfaces.
3. **Whitelist Stripping.** Set `whitelist: true` to prevent clients from posting random payloads to your database models.
4. **Transform Types.** Enable `transform: true` to cast request objects into class DTO instances automatically.

### Interview Q&A

- **Q: Why must DTOs in Nest.js be declared as classes and not interfaces?**
  → TypeScript interfaces disappear during JavaScript compilation (type erasure). Classes remain as constructor objects in JS. Nest requires runtime access to classes to perform DTO metadata validation using `class-validator`.

- **Q: What do `whitelist` and `forbidNonWhitelisted` do in the `ValidationPipe` configuration?**
  → `whitelist: true` filters out any properties in the request payload that do not have associated validation decorators. `forbidNonWhitelisted: true` raises a `400 Bad Request` exception if any non-decorated properties are present, preventing SQL injection or unexpected payload inputs.

- **Q: What is the execution order of standard parameters validation pipes?**
  → Nest resolves pipes from left to right. Inside `@Param('id', ValidationPipe, ParseIntPipe) id`, validation check runs first, followed by the integer parser.

---

*← [05 — Middleware and Request Lifecycle](./05_middleware_and_request_lifecycle.md) | [07 — Exception Filters →](./07_exception_filters.md)*
