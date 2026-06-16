# 02 — Controllers and Routing

> **Goal:** Understand the role of Nest.js controllers in the HTTP layer, learn routing decorators, extract request params, queries, headers, and bodies, and manage HTTP responses.

---

## 1. Controllers — The Entry Point for HTTP

In Nest.js, a controller is a TypeScript class decorated with `@Controller()`. Its sole responsibility is to handle incoming HTTP requests, route them to appropriate business logic services, and return responses back to the client.

The mental model is:

```
Client Request ──> Nest Router ──> Controller `@Get('/users')` ──> Service Logic ──> HTTP Response
```

Here is a basic controller:

```typescript
// src/users/users.controller.ts
import { Controller, Get } from '@nestjs/common';

@Controller('users') // Namespace: Prefixes all routes in this class with '/users'
export class UsersController {
  @Get() // Maps GET /users
  findAll(): string[] {
    return ['User A', 'User B'];
  }

  @Get('active') // Maps GET /users/active
  findActive(): string {
    return 'Active users list';
  }
}
```

---

## 2. Request Payloads & Parameter Extraction

Nest provides request-scoped decorators to extract values directly from incoming HTTP requests without needing to interact with the raw Node/Express request object.

### The Cheat Sheet of Request Decorators

| Decorator | Equivalent Express Property | Purpose |
|-----------|-----------------------------|---------|
| `@Req()` | `req` | Accesses the entire request object (avoid in normal usage). |
| `@Res()` | `res` | Accesses the Express response object (disables Nest's auto-response lifecycle). |
| `@Param(key?)` | `req.params[key]` | Extracts path parameters (e.g. `/users/:id`). |
| `@Query(key?)` | `req.query[key]` | Extracts query parameters (e.g. `/users?role=admin`). |
| `@Body(key?)` | `req.body[key]` | Extracts request body payload (for POST, PUT, PATCH). |
| `@Headers(name?)` | `req.headers[name]` | Extracts request HTTP headers. |
| `@Ip()` | `req.ip` | Extracts client IP address. |

### Accessing Path & Query Parameters

Path variables are declared using colons in routing paths, and queries are queried dynamically:

```typescript
// src/users/users.controller.ts
import { Controller, Get, Param, Query } from '@nestjs/common';

@Controller('users')
export class UsersController {
  // GET /users/42
  @Get(':id')
  findOne(@Param('id') id: string): string {
    return `Looking up user with ID: ${id}`;
  }

  // GET /users/search?role=admin&limit=10
  @Get('search')
  search(
    @Query('role') role: string,
    @Query('limit') limit: string,
  ): string {
    return `Searching users with role ${role} and limit ${limit}`;
  }
}
```

### Accessing request bodies

Post requests require DTOs (Data Transfer Objects). A DTO defines the shape of the data sent in the request body.

```typescript
// src/users/dto/create-user.dto.ts
export class CreateUserDto {
  username: string;
  email: string;
}

// src/users/users.controller.ts
import { Controller, Post, Body } from '@nestjs/common';
import { CreateUserDto } from './dto/create-user.dto';

@Controller('users')
export class UsersController {
  @Post()
  create(@Body() createUserDto: CreateUserDto) {
    return {
      message: 'User created successfully',
      data: createUserDto,
    };
  }
}
```

---

## 3. Formatting HTTP Responses

Nest.js maps handler return values to HTTP responses automatically:
- Return a JavaScript object or array → Nest marshals it to JSON and sets `Content-Type: application/json`.
- Return a string → Nest sends it as text and sets `Content-Type: text/html`.

### Status Codes

By default, Nest returns `200 OK` for all requests, except for `POST` requests which return `201 Created`. Use `@HttpCode()` to customize this behavior.

```typescript
// src/users/users.controller.ts
import { Controller, Post, Delete, HttpCode, HttpStatus } from '@nestjs/common';

@Controller('users')
export class UsersController {
  @Post('reset')
  @HttpCode(HttpStatus.NO_CONTENT) // 204 No Content
  resetPassword() {
    // Logic to reset password
  }
}
```

### Custom Headers & Redirects

You can send custom headers using `@Header()` and handle redirections with `@Redirect()`.

```typescript
// src/users/users.controller.ts
import { Controller, Get, Header, Redirect } from '@nestjs/common';

@Controller('users')
export class UsersController {
  @Get('export')
  @Header('Cache-Control', 'none')
  @Header('X-Custom-Header', 'custom-value')
  exportData() {
    return { data: 'csv-formatted-info' };
  }

  @Get('old-docs')
  @Redirect('https://docs.nestjs.com/v10', 301)
  redirectToDocs() {}
}
```

---

## 4. Practical Application — A CRUD Controller

Let's build a clean, real-world controller for a resource called `products`.

```typescript
// src/products/dto/create-product.dto.ts
export class CreateProductDto {
  readonly name: string;
  readonly price: number;
  readonly category: string;
}

// src/products/dto/update-product.dto.ts
export class UpdateProductDto {
  readonly name?: string;
  readonly price?: number;
  readonly category?: string;
}

// src/products/products.controller.ts
import { 
  Controller, 
  Get, 
  Post, 
  Put, 
  Delete, 
  Param, 
  Body, 
  Query, 
  HttpCode, 
  HttpStatus 
} from '@nestjs/common';
import { CreateProductDto } from './dto/create-product.dto';
import { UpdateProductDto } from './dto/update-product.dto';

@Controller('products')
export class ProductsController {
  
  @Get()
  findAll(
    @Query('category') category?: string,
    @Query('limit') limit: number = 10
  ) {
    return {
      action: 'Fetch all products',
      filters: { category, limit }
    };
  }

  @Get(':id')
  findOne(@Param('id') id: string) {
    return {
      action: `Fetch product details for product ${id}`
    };
  }

  @Post()
  @HttpCode(HttpStatus.CREATED)
  create(@Body() createProductDto: CreateProductDto) {
    return {
      action: 'Create a new product',
      payload: createProductDto
    };
  }

  @Put(':id')
  update(
    @Param('id') id: string, 
    @Body() updateProductDto: UpdateProductDto
  ) {
    return {
      action: `Update product ${id}`,
      payload: updateProductDto
    };
  }

  @Delete(':id')
  @HttpCode(HttpStatus.NO_CONTENT)
  remove(@Param('id') id: string) {
    // Delete logic goes here
    console.log(`Product ${id} deleted`);
  }
}
```

To test these endpoints:

```bash
# Get products
curl http://localhost:3000/products?category=electronics&limit=5

# Create a product
curl -X POST http://localhost:3000/products \
  -H "Content-Type: application/json" \
  -d '{"name": "Laptop", "price": 999, "category": "electronics"}'

# Delete a product
curl -X DELETE http://localhost:3000/products/42 -i
```

---

## 5. Common mistakes & gotchas

- **Using `@Res()` without understanding its implications.** If you inject `@Res() res: Response` from Express to set headers or cookies manually, **Nest's automatic response handling is disabled.** If you don't call `res.send(...)` or `res.json(...)`, the request will hang indefinitely. To inject the response *but* keep Nest's auto-rendering active, use `@Res({ passthrough: true })`.
- **Forgetting route parameters colons.** Writing `@Get('users/id')` instead of `@Get('users/:id')` will make Nest map the literal route `/users/id` rather than treating `id` as a dynamic route path variable.
- **Route ordering conflicts.** Nest matches routes in the order they are defined. If you define `@Get(':id')` before `@Get('search')`, hit `/users/search`, Nest will capture `search` as the `:id` parameter and call the `findOne` controller handler instead. Always put static paths before dynamic parameter paths.

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **Name-spacing.** Use `@Controller('prefix')` to cleanly namespace routes.
2. **Auto-serialization.** Nest formats JavaScript arrays and objects to JSON automatically.
3. **Decouple from Express.** Use request decorators like `@Body()`, `@Query()` instead of inspecting `req` properties to ease Fastify compatibility.
4. **Order of definition matters.** Put static paths above dynamic wildcard parameters inside controllers.

### Interview Q&A

- **Q: What happens if you inject `@Res() res` in a controller method?**
  → Injecting `@Res()` bypasses Nest's normal serialization lifecycle, placing Express/Fastify response control completely in the developer's hands. The client will hang unless `res.json()` or `res.send()` is manually invoked, unless `{ passthrough: true }` is enabled in the decorator options.

- **Q: How does route precedence work in Nest.js controllers?**
  → Nest matches incoming paths top-to-bottom as declared in the controller class. If a dynamic parameter route like `@Get(':id')` is placed above a static route like `@Get('profile')`, hitting `/profile` will be captured by `:id` handler with `id = 'profile'`.

- **Q: How can we define dynamic redirects in Nest.js?**
  → Return an object matching `{ url: string, statusCode: number }` from a method decorated with `@Redirect()`. Nest will use the returned object properties to override the decorator's static redirect target dynamically.

---

*← [01 — Setup and First Server](./01_setup_and_first_server.md) | [03 — Providers & DI →](./03_providers_and_di.md)*
