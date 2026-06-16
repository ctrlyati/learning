# 11 — Authentication and JWT

> **Goal:** Build a secure user authentication system using Passport.js and JWT (JSON Web Tokens), hash passwords using bcrypt, protect routes, and create a custom parameter decorator to extract the current user.

---

## 1. Authentication Architecture in Nest.js

Nest.js leverages the industry-standard **Passport.js** ecosystem to handle authentication. The authentication flow is divided into strategies and guards:

```
1. Login Request ──> Passport LocalStrategy ──(Checks email/password)──> AuthService issues JWT
2. API Request ──> JwtAuthGuard ──> Passport JwtStrategy ──(Verifies Token)──> Attaches user to req.user ──> Controller
```

- **Local Strategy:** Verifies user credentials (email/password) and returns the user object.
- **JWT Strategy:** Verifies that the client sent a valid, signed Bearer Token in request headers and extracts user payload details.

---

## 2. Setting Up Authentication Dependencies

Install Passport, JWT wrappers, and bcrypt helper libraries:

```bash
npm install --save @nestjs/passport passport passport-local @nestjs/jwt passport-jwt bcrypt
npm install --save-dev @types/passport-local @types/passport-jwt @types/bcrypt
```

---

## 3. Implementing the Strategies

### Local Strategy (Validating Credentials)

The local strategy intercepts requests to verify username and password:

```typescript
// src/auth/strategies/local.strategy.ts
import { Injectable, UnauthorizedException } from '@nestjs/common';
import { PassportStrategy } from '@nestjs/passport';
import { Strategy } from 'passport-local';
import { AuthService } from '../auth.service';

@Injectable()
export class LocalStrategy extends PassportStrategy(Strategy) {
  constructor(private authService: AuthService) {
    // Customize credentials field names if they differ from "username"/"password"
    super({ usernameField: 'email' });
  }

  async validate(email: string, passport: string): Promise<any> {
    const user = await this.authService.validateUser(email, passport);
    if (!user) {
      throw new UnauthorizedException('Invalid login credentials');
    }
    return user; // Attached automatically to req.user
  }
}
```

### JWT Strategy (Validating Token)

The JWT strategy extracts the token from the request header and verifies its signature:

```typescript
// src/auth/strategies/jwt.strategy.ts
import { Injectable } from '@nestjs/common';
import { PassportStrategy } from '@nestjs/passport';
import { ExtractJwt, Strategy } from 'passport-jwt';

@Injectable()
export class JwtStrategy extends PassportStrategy(Strategy) {
  constructor() {
    super({
      jwtFromRequest: ExtractJwt.fromAuthHeaderAsBearerToken(),
      ignoreExpiration: false,
      secretOrKey: 'JWT_SECRET_KEY_CHANGE_ME', // Retrieve from ConfigService in production
    });
  }

  async validate(payload: any) {
    // The payload is the decoded JWT object.
    // What we return here will be bound directly to req.user.
    return { userId: payload.sub, email: payload.email, role: payload.role };
  }
}
```

---

## 4. Issuing JWTs inside AuthService

The `AuthService` handles credential hashing checks and issues signed JWT tokens.

```typescript
// src/auth/auth.service.ts
import { Injectable } from '@nestjs/common';
import { JwtService } from '@nestjs/jwt';
import * as bcrypt from 'bcrypt';
import { UsersService } from '../users/users.service';

@Injectable()
export class AuthService {
  constructor(
    private usersService: UsersService,
    private jwtService: JwtService
  ) {}

  async validateUser(email: string, pass: string): Promise<any> {
    const user = await this.usersService.findOneByEmail(email);
    if (user && (await bcrypt.compare(pass, user.password))) {
      const { password, ...result } = user;
      return result;
    }
    return null;
  }

  async login(user: any) {
    const payload = { email: user.email, sub: user.id, role: user.role };
    return {
      access_token: this.jwtService.sign(payload),
    };
  }
}
```

Now configure the authentication module:

```typescript
// src/auth/auth.module.ts
import { Module } from '@nestjs/common';
import { JwtModule } from '@nestjs/jwt';
import { PassportModule } from '@nestjs/passport';
import { AuthService } from './auth.service';
import { UsersModule } from '../users/users.module';
import { LocalStrategy } from './strategies/local.strategy';
import { JwtStrategy } from './strategies/jwt.strategy';
import { AuthController } from './auth.controller';

@Module({
  imports: [
    UsersModule,
    PassportModule,
    JwtModule.register({
      secret: 'JWT_SECRET_KEY_CHANGE_ME',
      signOptions: { expiresIn: '1h' }, // Expiration
    }),
  ],
  providers: [AuthService, LocalStrategy, JwtStrategy],
  controllers: [AuthController],
})
export class AuthModule {}
```

---

## 5. Protecting Routes and Custom Decorators

To protect endpoints, we write a guard. While we can use `@UseGuards(AuthGuard('jwt'))` directly, creating a custom guard keeps code clean and types sound.

### The Custom `JwtAuthGuard`

```typescript
// src/auth/guards/jwt-auth.guard.ts
import { Injectable } from '@nestjs/common';
import { AuthGuard } from '@nestjs/passport';

@Injectable()
export class JwtAuthGuard extends AuthGuard('jwt') {}
```

### Custom Parameter Decorator `@CurrentUser()`

Instead of extracting the user by writing `req.user` in every controller, let's create a custom decorator to keep parameters type-safe.

```typescript
// src/common/decorators/current-user.decorator.ts
import { createParamDecorator, ExecutionContext } from '@nestjs/common';

export const CurrentUser = createParamDecorator(
  (data: unknown, ctx: ExecutionContext) => {
    const request = ctx.switchToHttp().getRequest();
    return request.user; // Extract user object from Express request
  },
);
```

### Usage in Controller

```typescript
// src/users/users.controller.ts
import { Controller, Get, UseGuards } from '@nestjs/common';
import { JwtAuthGuard } from '../auth/guards/jwt-auth.guard';
import { CurrentUser } from '../common/decorators/current-user.decorator';

@Controller('users')
export class UsersController {
  
  @Get('me')
  @UseGuards(JwtAuthGuard) // Protect endpoint with JWT check
  getProfile(@CurrentUser() user: any) {
    // Inject user directly using parameter decorator
    return user;
  }
}
```

---

## 6. Common mistakes & gotchas

- **Sharing the secret key in codebase repositories.** Storing the JWT secret key as a plain string inside `JwtModule.register` config files is a high-risk security hazard. Always load secret keys dynamically from system environment variables using the `ConfigService` (Module 12).
- **Typos in strategy names.** By default, `AuthGuard('jwt')` binds to the strategy called `'jwt'`. If you call `super('jwt-secret')` inside your `JwtStrategy` constructor but call `@UseGuards(AuthGuard('jwt'))` on controllers, Nest won't match them, throwing a `500 Server Error`.
- **Forgetting that LocalStrategy overrides request content.** The local strategy consumes `req.body.email` and `req.body.password`. If you send credentials nested inside `{ user: { email, password } }` layout, Passport will fail to find them, rejecting login requests immediately with a `400 Bad Request` code.

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **Passport Abstraction.** Strategies write validation checks; Guards apply them to routes.
2. **Strategy Validate.** What you return from `validate()` in your strategy class is bound directly to `req.user`.
3. **Decouple decorators.** Custom decorators keep controllers clean and decouple route handlers from raw Express request structures.
4. **Token signing.** Generate tokens inside AuthService using `@nestjs/jwt` helper client.

### Interview Q&A

- **Q: How does Nest.js authentication link guards and passport strategies?**
  → The `AuthGuard('name')` decorator runs first. It triggers Passport execution, maps parameters to the registered `PassportStrategy` matching `'name'`, runs the strategy's `validate()` method, and binds the returned value to the Express/Fastify request object as `req.user`.

- **Q: Why does JwtStrategy contain a `validate()` method, and who calls it?**
  → Passport's JWT library verifies token signature and expiration automatically. Once verification succeeds, it calls `validate(payload)` passing the decoded JWT payload. The returned value is then bound to the request user object.

- **Q: How do you extract JWT tokens from cookies instead of bearer headers?**
  → Customize the strategy's `jwtFromRequest` option. Provide a custom extraction function, for example: `jwtFromRequest: ExtractJwt.fromExtractors([(req) => req?.cookies?.jwtToken])`.

---

*← [10 — Database Integration](./10_database_integration.md) | [12 — Configuration and Env →](./12_configuration_and_env.md)*
