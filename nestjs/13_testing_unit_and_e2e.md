# 13 — Testing: Unit and E2E

> **Goal:** Write high-coverage unit tests for Nest.js services and controllers using Jest, mock dependencies inside the DI container, and build isolated End-to-End (E2E) tests using Supertest.

---

## 1. Testing Philosophy in Nest.js

Nest.js is designed with testability in mind. Because it uses Dependency Injection (DI) to construct its graph, mocking database connections or HTTP clients is simple: you override the provider tokens inside Jest testing modules.

Nest uses **Jest** as its default test runner. Testing is split into:

- **Unit Testing:** Tests a single class (e.g., service or controller) in isolation. All database services or external dependencies are mocked out completely.
- **End-to-End (E2E) Testing:** Boots the entire Nest application context (modules, pipelines, middleware, validation pipes) and hits endpoints using `supertest` to verify HTTP inputs and database side-effects.

---

## 2. Unit Testing Services with Mocking

To test a service in isolation, we mock its injected providers. We use the `@nestjs/testing` utility `Test.createTestingModule()` to build a temporary DI container.

Let's test `UsersService` which depends on `PrismaService`.

```typescript
// src/users/users.service.spec.ts
import { Test, TestingModule } from '@nestjs/testing';
import { UsersService } from './users.service';
import { PrismaService } from '../prisma/prisma.service';

describe('UsersService', () => {
  let service: UsersService;
  let prisma: PrismaService;

  // Create mock database client functions using jest.fn()
  const mockPrismaService = {
    user: {
      findUnique: jest.fn(),
      create: jest.fn(),
    },
  };

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      providers: [
        UsersService,
        {
          provide: PrismaService, // Override the token
          useValue: mockPrismaService, // Provide mock implementation
        },
      ],
    }).compile();

    service = module.get<UsersService>(UsersService);
    prisma = module.get<PrismaService>(PrismaService);
  });

  it('should be defined', () => {
    expect(service).toBeDefined();
  });

  describe('create', () => {
    it('should create a user if email is unique', async () => {
      const dto = { email: 'test@test.com', username: 'tester', age: 25 };
      
      // 1. Mock DB call: User does not exist
      mockPrismaService.user.findUnique.mockResolvedValue(null);
      // 2. Mock DB call: Create returns user payload
      mockPrismaService.user.create.mockResolvedValue({ id: 1, ...dto });

      const result = await service.create(dto);

      expect(result.id).toEqual(1);
      expect(prisma.user.create).toHaveBeenCalledWith({ data: dto });
    });

    it('should throw ConflictException if email already exists', async () => {
      const dto = { email: 'exists@test.com', username: 'tester', age: 25 };
      
      // Mock DB: User email is already taken
      mockPrismaService.user.findUnique.mockResolvedValue({ id: 99, email: dto.email });

      await expect(service.create(dto)).rejects.toThrow('Email already registered');
    });
  });
});
```

To run this unit test:

```bash
npm run test
```

---

## 3. End-to-End (E2E) Testing with Supertest

E2E tests verify that the entire request lifecycle (Middleware → Guards → Interceptors → Pipes → Handlers) runs correctly. E2E tests are stored in the root `test/` directory.

Let's test the `GET /users/me` endpoint. We will mock the `JwtStrategy` so we can test the protected controller without needing to hit a database during auth verification.

```typescript
// test/users.e2e-spec.ts
import { Test, TestingModule } from '@nestjs/testing';
import { INestApplication, ValidationPipe } from '@nestjs/common';
import * as request from 'supertest';
import { AppModule } from '../src/app.module';
import { JwtAuthGuard } from '../src/auth/guards/jwt-auth.guard';

describe('UsersController (E2E)', () => {
  let app: INestApplication;

  // Mock guard that bypasses authentication
  const mockJwtGuard = {
    canActivate: (context) => {
      const req = context.switchToHttp().getRequest();
      req.user = { userId: 1, email: 'mocked@test.com', role: 'admin' };
      return true;
    },
  };

  beforeAll(async () => {
    const moduleFixture: TestingModule = await Test.createTestingModule({
      imports: [AppModule],
    })
      // Override the JwtAuthGuard globally for E2E isolation
      .overrideGuard(JwtAuthGuard)
      .useValue(mockJwtGuard)
      .compile();

    app = moduleFixture.createNestApplication();
    
    // IMPORTANT: E2E tests boot an isolated environment.
    // We must re-bind any global pipes/validation manually!
    app.useGlobalPipes(new ValidationPipe({ transform: true }));
    
    await app.init(); // Compile Nest pipelines
  });

  afterAll(async () => {
    await app.close(); // Clean up HTTP connections
  });

  it('GET /users/me (Authorized)', () => {
    return request(app.getHttpServer())
      .get('/users/me')
      .expect(200)
      .expect((res) => {
        expect(res.body.email).toEqual('mocked@test.com');
        expect(res.body.role).toEqual('admin');
      });
  });

  it('POST /users (Validation check)', () => {
    // Post empty object to trigger ValidationPipe
    return request(app.getHttpServer())
      .post('/users')
      .send({})
      .expect(400) // Expect validation validation failure
      .expect((res) => {
        expect(res.body.error).toEqual('Bad Request');
        expect(res.body.message).toContain('email must be an email');
      });
  });
});
```

To run this E2E test suite:

```bash
npm run test:e2e
```

---

## 4. Overriding Providers for E2E Tests

Often you want to override database services, email processors, or billing systems in E2E files to mock side-effects. Use Nest's `overrideProvider()` API:

```typescript
const moduleFixture = await Test.createTestingModule({
  imports: [AppModule],
})
  .overrideProvider(EmailService)
  .useValue({ sendWelcomeEmail: jest.fn().mockResolvedValue(true) })
  .compile();
```

---

## 5. Common mistakes & gotchas

- **Forgetting to copy global settings to the test app.** If you apply global pipes (`ValidationPipe`) or filters (`HttpExceptionFilter`) in `main.ts` but forget to bind them to your test app inside E2E tests (`app.useGlobalPipes(...)`), your E2E validation tests will fail because validation checks won't run.
- **Memory leaks by not closing the app.** Failing to call `await app.close()` in Jest's `afterAll()` hook leaves HTTP listener handles open, causing Jest to hang or leak memory across test files.
- **Sharing mock service state across tests.** If your mock service object holds state variables (e.g. `mockValue`), and you don't reset it in `beforeEach` using `jest.clearAllMocks()`, tests will impact each other and fail depending on execution order.

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **Testing Mock Container.** `Test.createTestingModule` replaces standard modular application graphs dynamically.
2. **E2E Pipeline.** Ensure to re-apply global guards, pipes, and interceptors to the test app inside E2E specs.
3. **Override Utility.** Use `overrideGuard()` and `overrideProvider()` to disconnect external API dependencies.
4. **Mock Cleanups.** Reset Jest mocks in `beforeEach` loops to maintain absolute isolation.

### Interview Q&A

- **Q: How does `overrideProvider` work during Nest.js E2E tests?**
  → It tells the Nest compilation builder to replace the token reference of a service with a mock implementation (e.g. `useValue` or `useFactory`), avoiding hitting actual third-party services like payment gateways or email servers.

- **Q: Why must you call `app.init()` manually in E2E tests, but not in unit tests?**
  → Unit tests just construct classes. E2E tests boot the HTTP routing layer, compiling all modules, middleware configurations, guards, interceptors, and pipes. Calling `app.init()` is required to build this routing pipeline before passing requests through.

- **Q: What is the role of `jest.clearAllMocks()` in testing services?**
  → It resets mock counters and parameters tracked by `jest.fn()` mocks between individual tests, ensuring that call count assertions (e.g., `toHaveBeenCalledTimes(1)`) are isolated and do not bleed.

---

*← [12 — Configuration and Env](./12_configuration_and_env.md) | [14 — WebSockets and Microservices →](./14_websockets_and_microservices.md)*
