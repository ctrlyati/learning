# 08 — Guards and Authorization

> **Goal:** Create Nest.js guards, leverage the execution context, set up custom route metadata using the Reflector, and implement Role-Based Access Control (RBAC).

---

## 1. The Core Concept — Route Guards

In Nest.js, **Guards** are classes that implement the `CanActivate` interface and are decorated with `@Injectable()`. While middleware handles request preparation, guards have a single responsibility: **determine whether a request is allowed to proceed** based on authentication, roles, or permissions.

Guards execute **after** all middleware, but **before** any interceptors or pipes.

The mental model is:

```
Request ──> Middleware ──> [ Guard ] ──( returns true )──> Interceptor ──> Pipe ──> Handler
                              │
                        ( returns false )
                              │
                              └──> Throws HTTP ForbiddenException (403)
```

If a guard method returns `false`, Nest automatically aborts the request pipeline and throws a `403 ForbiddenException` to the client.

---

## 2. The Execution Context (`ExecutionContext`)

Guards (along with Interceptors and Exception Filters) have access to the `ExecutionContext` argument.

`ExecutionContext` inherits from `ArgumentsHost` and provides metadata about the code currently being executed. It allows you to write highly generic guards that inspect both the current HTTP request and the controller class or method definition.

### Inspecting Handler Details

```typescript
import { CanActivate, ExecutionContext, Injectable } from '@nestjs/common';
import { Observable } from 'rxjs';

@Injectable()
export class DebugGuard implements CanActivate {
  canActivate(context: ExecutionContext): boolean | Promise<boolean> | Observable<boolean> {
    // 1. Get the controller class metadata
    const controllerClass = context.getClass(); 
    console.log(`Executing within controller: ${controllerClass.name}`);

    // 2. Get the handler function metadata
    const routeHandler = context.getHandler(); 
    console.log(`Routing to handler: ${routeHandler.name}`);

    // 3. Switch context to HTTP to extract request object
    const request = context.switchToHttp().getRequest();
    console.log(`Requesting URL: ${request.url}`);

    return true; // Allow route execution
  }
}
```

---

## 3. Metadata Reflection and the `Reflector`

To implement authorization (e.g. check if a user is an `"admin"`), we need to attach access rules directly to our routes. We do this by attaching custom **metadata** to controller classes or individual methods.

Nest provides the `SetMetadata()` helper, but we should wrap it in a custom decorator for type-safety.

### Create a Custom `@Roles()` Decorator

```typescript
// src/common/decorators/roles.decorator.ts
import { SetMetadata } from '@nestjs/common';

// Define a key to identify roles metadata in reflection tables
export const ROLES_KEY = 'roles';

// The roles decorator takes a list of strings
export const Roles = (...roles: string[]) => SetMetadata(ROLES_KEY, roles);
```

### Apply `@Roles()` to routes

```typescript
// src/users/users.controller.ts
import { Controller, Get, UseGuards } from '@nestjs/common';
import { Roles } from '../common/decorators/roles.decorator';
import { RolesGuard } from '../common/guards/roles.guard';

@Controller('users')
@UseGuards(RolesGuard) // Apply the guard to the entire controller
export class UsersController {
  
  @Get('admin-panel')
  @Roles('admin') // Attach metadata: only allow "admin" role to access this route
  getAdminData() {
    return { data: 'highly-secured-system-info' };
  }

  @Get('profile')
  @Roles('user', 'admin') // Allow both roles
  getProfile() {
    return { data: 'user-profile-info' };
  }
}
```

---

## 4. Practical Application — Writing the `RolesGuard`

The guard uses Nest's built-in `Reflector` service to extract the metadata set by the `@Roles()` decorator and compare it against the user attached to the request (which is typically populated by an upstream authentication middleware or guard).

```typescript
// src/common/guards/roles.guard.ts
import { CanActivate, ExecutionContext, Injectable } from '@nestjs/common';
import { Reflector } from '@nestjs/core';
import { ROLES_KEY } from '../decorators/roles.decorator';

@Injectable()
export class RolesGuard implements CanActivate {
  // Inject Reflector to read decorators metadata
  constructor(private reflector: Reflector) {}

  canActivate(context: ExecutionContext): boolean {
    // 1. Read metadata from the route handler function
    // If no roles are defined on the handler, check the parent controller class
    const requiredRoles = this.reflector.getAllAndOverride<string[]>(ROLES_KEY, [
      context.getHandler(),
      context.getClass(),
    ]);

    // 2. If no roles are required, allow the request to proceed
    if (!requiredRoles) {
      return true;
    }

    // 3. Extract the request object and the user
    const { user } = context.switchToHttp().getRequest();

    // 4. Return false if no user exists or they lack the required roles
    if (!user || !user.role) {
      return false; 
    }

    // 5. Check authorization match
    return requiredRoles.some((role) => user.role === role);
  }
}
```

To test this guard, you would send a mock user inside request headers (using a test middleware that sets `req.user = { role: 'admin' }` or similar authentication logic).

---

## 5. Common mistakes & gotchas

- **Forgetting that Guards run *after* middleware.** If you attempt to access `req.body` inside a guard, but you haven't loaded the `json` parsing middleware (which Nest does automatically, but can be disabled), the request body will be `undefined`.
- **Forgetting to inject Reflector in global guards.** If you register a guard globally using `app.useGlobalGuards(new RolesGuard())` in `main.ts`, the guard won't have access to the `Reflector` instance initialized by Nest's DI container, causing an instantiation crash. Use the `APP_GUARD` token inside `AppModule` instead.
- **Using `getAllAndMerge` vs `getAllAndOverride`.** When reading metadata, `getAllAndMerge` combines settings from both the controller level and method level. `getAllAndOverride` overrides controller settings with method settings (which is usually what you want for security).

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **Guards return Boolean.** True allows execution; False cancels the call and raises a `403 Forbidden` response.
2. **Context Agnostic.** ExecutionContext exposes both the HTTP requests and class/method reflection data.
3. **Reflector Power.** Use `Reflector` to retrieve custom metadata from routes during guards checking.
4. **Binding Scopes.** Guards can be scoped to routes, controllers, or globally via `APP_GUARD`.

### Interview Q&A

- **Q: How does `ExecutionContext` differ from `ArgumentsHost`?**
  → `ArgumentsHost` provides access to raw parameters passed to handler pipelines (Express/Fastify requests, response arrays). `ExecutionContext` inherits from `ArgumentsHost` and adds reflection data about the executing class and route handler method.

- **Q: Why should you avoid registering a global guard using `app.useGlobalGuards()` if the guard requires dependencies?**
  → Using `app.useGlobalGuards()` instantiates the guard manually, disconnecting it from Nest's Dependency Injection system. To support DI inside global guards, register the guard as a provider inside `AppModule` using the `APP_GUARD` token.

- **Q: What happens if a route does not have the custom `@Roles()` decorator applied but the controller is bound to `RolesGuard`?**
  → Inside the guard, `reflector.getAllAndOverride()` will return `undefined`. Because there are no required roles, the guard should return `true` to allow normal public access.

---

*← [07 — Exception Filters](./07_exception_filters.md) | [09 — Interceptors →](./09_interceptors.md)*
