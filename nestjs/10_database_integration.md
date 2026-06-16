# 10 — Database Integration

> **Goal:** Choose the right database strategy in Nest.js, configure Prisma Client with PostgreSQL, write service-level database queries, and handle database transactions safely.

---

## 1. Database Strategies: TypeORM vs. Prisma vs. Mongoose

Nest.js is database agnostic. It provides official wrappers for the most popular Node.js database libraries:

- **TypeORM (`@nestjs/typeorm`):** A classic Active Record / Data Mapper ORM. Heavily OOP-based, relies on decorators inside database entities. Matches Spring Boot / Hibernate styles.
- **Prisma (Custom integration):** A modern, type-safe schema-first client. It generates a strongly typed query builder from a central schema file. No entity decorators needed.
- **Mongoose (`@nestjs/mongoose`):** The default choice for MongoDB databases.

For this course, we use **Prisma with PostgreSQL**, which has become the industry standard for new TypeScript applications due to its type-safety and query profiling advantages.

---

## 2. Setting Up Prisma in Nest.js

Let's configure Prisma from scratch.

### Install Dependencies

```bash
npm install prisma --save-dev
npm install @prisma/client
```

### Initialize Prisma

```bash
npx prisma init
```

This creates a new folder `prisma/` containing a `schema.prisma` file, and appends a `DATABASE_URL` environment variable to your `.env` file.

### Define Database Models

Open `prisma/schema.prisma` and add a PostgreSQL provider and a simple model:

```prisma
// prisma/schema.prisma
datasource db {
  provider = "postgresql"
  url      = env("DATABASE_URL")
}

generator client {
  provider = "prisma-client-js"
}

model User {
  id        Int      @id @default(autoincrement())
  email     String   @unique
  name      String?
  createdAt DateTime @default(now())
}
```

Run database migrations to generate database tables and the Prisma client files:

```bash
npx prisma migrate dev --name init
```

---

## 3. Creating the PrismaService

Next, build a dedicated service to wrap the Prisma client and manage database connection lifecycles.

```typescript
// src/prisma/prisma.service.ts
import { Injectable, OnModuleInit, OnModuleDestroy } from '@nestjs/common';
import { PrismaClient } from '@prisma/client';

@Injectable()
export class PrismaService extends PrismaClient implements OnModuleInit, OnModuleDestroy {
  // OnModuleInit is a Nest lifecycle hook.
  // We open the database connection when the module loads.
  async onModuleInit() {
    await this.$connect();
  }

  // OnModuleDestroy is a Nest lifecycle hook.
  // We close the database connection when the application shuts down.
  async onModuleDestroy() {
    await this.$disconnect();
  }
}
```

Now wrap this service inside a global module:

```typescript
// src/prisma/prisma.module.ts
import { Module, Global } from '@nestjs/common';
import { PrismaService } from './prisma.service';

@Global() // Global so we don't have to import PrismaModule everywhere
@Module({
  providers: [PrismaService],
  exports: [PrismaService], // Export so other modules can inject PrismaService
})
export class PrismaModule {}
```

---

## 4. Querying Databases in Services

Inject `PrismaService` into feature services to read and write database records.

```typescript
// src/users/users.service.ts
import { Injectable, ConflictException, NotFoundException } from '@nestjs/common';
import { PrismaService } from '../prisma/prisma.service';
import { CreateUserDto } from './dto/create-user.dto';

@Injectable()
export class UsersService {
  constructor(private prisma: PrismaService) {}

  async create(createUserDto: CreateUserDto) {
    const existing = await this.prisma.user.findUnique({
      where: { email: createUserDto.email },
    });

    if (existing) {
      throw new ConflictException('Email already registered');
    }

    return this.prisma.user.create({
      data: createUserDto,
    });
  }

  async findAll() {
    return this.prisma.user.findMany();
  }

  async findOne(id: number) {
    const user = await this.prisma.user.findUnique({ where: { id } });
    if (!user) {
      throw new NotFoundException(`User with ID ${id} not found`);
    }
    return user;
  }
}
```

---

## 5. Transaction Management

A database transaction ensures that multiple queries execute successfully as a single unit of work. If any query fails, the database rolls back all changes to keep data consistent.

Prisma provides the `$transaction` API.

### Sequential Transactions

Run a batch of writes sequentially:

```typescript
async deleteUserAndBackup(userId: number) {
  // Runs both queries inside a single database transaction
  const [deletedUser, backupLog] = await this.prisma.$transaction([
    this.prisma.user.delete({ where: { id: userId } }),
    this.prisma.auditLog.create({
      data: { action: 'DELETE_USER', details: `Deleted user ID: ${userId}` },
    }),
  ]);
  return deletedUser;
}
```

### Interactive Transactions

When queries depend on other query results, pass an async callback function to `$transaction`:

```typescript
async transferFunds(senderId: number, receiverId: number, amount: number) {
  return this.prisma.$transaction(async (tx) => {
    // 1. Fetch sender balance inside transaction scope (tx)
    const sender = await tx.user.findUnique({ where: { id: senderId } });
    if (!sender || sender.balance < amount) {
      throw new Error('Insufficient balance or sender not found');
    }

    // 2. Deduct funds from sender
    await tx.user.update({
      where: { id: senderId },
      data: { balance: sender.balance - amount },
    });

    // 3. Add funds to receiver
    const receiver = await tx.user.findUnique({ where: { id: receiverId } });
    if (!receiver) {
      throw new Error('Receiver not found');
    }

    await tx.user.update({
      where: { id: receiverId },
      data: { balance: receiver.balance + amount },
    });

    return { senderId, receiverId, amount, status: 'SUCCESS' };
  });
}
```

---

## 6. Common mistakes & gotchas

- **Exceeding database connection limits.** Each instance of `PrismaClient` keeps its own database connection pool. If you instantiate `new PrismaClient()` manually inside multiple services instead of injecting `PrismaService` as a singleton provider, your application will quickly exhaust the PostgreSQL connection limit.
- **Forgetting that transaction queries must use `tx`, not `this.prisma`.** Inside an interactive transaction `this.prisma.$transaction(async (tx) => { ... })`, any query that uses `this.prisma.user.update(...)` instead of `tx.user.update(...)` will run **outside** the transaction context, causing thread locks or missing rollback configurations.
- **N+1 query issues in relation mappings.** Calling `users.map(async u => await prisma.posts.findMany({ where: { userId: u.id } }))` creates one database trip per user. Use Prisma's `include` option instead to fetch relations in a single optimized JOIN statement:
  ```typescript
  this.prisma.user.findMany({ include: { posts: true } });
  ```

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **Lifecycle hooks.** Connect on `onModuleInit` and disconnect on `onModuleDestroy`.
2. **Global Client.** Use a single global `PrismaService` provider to share the connection pool.
3. **Interactive `tx`.** Always execute database calls using the scoped `tx` client inside transactions.
4. **Relations JOIN.** Utilize `include` options to request SQL JOIN actions, avoiding N+1 query loops.

### Interview Q&A

- **Q: How does Prisma differ from TypeORM, and why would you choose one over the other?**
  → TypeORM is an OOP-based data mapper ORM that relies on class entities with decorators and manages state in-memory. Prisma is schema-first and generates a type-safe TypeScript query builder client from a schema file, offering faster compilation validation and simpler relation operations.

- **Q: Why is it important to bind `$connect` and `$disconnect` to Nest.js lifecycle hooks?**
  → Database connections are expensive. Connecting on `onModuleInit` ensures the database is ready when Nest starts handling requests. Disconnecting on `onModuleDestroy` prevents database connection leaks and socket crashes during hot reloads or server restarts.

- **Q: What is an interactive transaction in Prisma, and how is it coded?**
  → Interactive transactions accept an asynchronous callback function passing a transaction client `tx` parameter. All database queries must be called on `tx` so that Nest can safely roll back all operations if any intermediate statement throws an error.

---

*← [09 — Interceptors](./09_interceptors.md) | [11 — Authentication and JWT →](./11_authentication_and_jwt.md)*
