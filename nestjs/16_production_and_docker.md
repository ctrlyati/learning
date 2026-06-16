# 16 — Production and Docker

> **Goal:** Compile Nest.js applications for production, write a multi-stage Dockerfile optimized for image size and security, and implement health checks using `@nestjs/terminus`.

---

## 1. Production Build Lifecycle

Before deployment, you must compile Nest.js TypeScript code into plain JavaScript. Nest handles this via the CLI:

```bash
npm run build
```

This compiles your source files and outputs them into a root `/dist` directory. The entrypoint for running the compiled code is:

```bash
node dist/main.js
```

During build:
1. `tsconfig.build.json` is used (excludes test files like `*.spec.ts`).
2. Nest CLI runs `tsc` (or `swc` if configured) for compilation.
3. Assets declared in `nest-cli.json` are copied.

---

## 2. Multi-stage Dockerfile for Nest.js

A standard Dockerfile often includes development dependencies and source files, resulting in bloated image sizes (> 1GB). We write a **multi-stage Dockerfile** to build the application in a temporary container and output a minimal runtime container containing only compiled code and production dependencies (< 200MB).

### Production Dockerfile (`Dockerfile`)

```dockerfile
# Multi-stage build for Nest.js (using Node 20 alpine)

# STAGE 1: Build the application
FROM node:20-alpine AS builder
WORKDIR /app

# Copy package files for dependency caching
COPY package*.json ./
COPY prisma ./prisma/

# Install ALL dependencies (including devDependencies for compilation)
RUN npm ci

# Copy source code
COPY . .

# Generate Prisma Client and compile TypeScript
RUN npx prisma generate
RUN npm run build

# Prune devDependencies to keep image size small
RUN npm prune --production

# STAGE 2: Run the application
FROM node:20-alpine AS runner
WORKDIR /app

ENV NODE_ENV=production

# Copy only necessary files from builder stage
COPY --from=builder /app/package*.json ./
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/prisma ./prisma

# Use a non-root user for security
USER node

EXPOSE 3000

# Start Nest.js application
CMD ["node", "dist/main.js"]
```

---

## 3. Implementing Health Checks with Terminus

Kubernetes or AWS ECS requires health endpoints (liveness/readiness probes) to check if the application is running and able to process traffic. Nest provides `@nestjs/terminus` to configure structured health endpoints.

### Install Dependencies

```bash
npm install --save @nestjs/terminus
```

### Implementing Health Controller

We configure checks for database connection and memory allocation:

```typescript
// src/health/health.controller.ts
import { Controller, Get } from '@nestjs/common';
import { 
  HealthCheck, 
  HealthCheckService, 
  PrismaHealthIndicator, 
  MemoryHealthIndicator 
} from '@nestjs/terminus';
import { PrismaService } from '../prisma/prisma.service';

@Controller('health')
export class HealthController {
  constructor(
    private health: HealthCheckService,
    private prismaIndicator: PrismaHealthIndicator,
    private prisma: PrismaService,
    private memory: MemoryHealthIndicator,
  ) {}

  @Get()
  @HealthCheck() // Decodes HTTP status (returns 200 if OK, 503 if any check fails)
  check() {
    return this.health.check([
      // 1. Check database connection health
      () => this.prismaIndicator.pingCheck('database', this.prisma),
      
      // 2. Check heap memory usage (fail if heap exceeds 150MB)
      () => this.memory.checkHeap('memory_heap', 150 * 1024 * 1024),
    ]);
  }
}
```

Register this controller in a `HealthModule` and add it to `AppModule`.

---

## 4. Clustering and Scaling Strategies

Because Node.js runs on a single thread, a single Nest instance cannot utilize multi-core servers (like AWS instances with 4+ vCPUs). To scale, you must run multiple processes:

- **Inside Containers (Kubernetes/ECS):** Scale by increasing the container replica count. Let Docker and Kubernetes handle load balancing across pods. This is the preferred modern pattern.
- **On Virtual Machines (EC2/VPS):** Scale using **PM2** in cluster mode to spawn one process per CPU core.

### PM2 Configuration (`ecosystem.config.js`)

```javascript
module.exports = {
  apps: [
    {
      name: 'hello-nest',
      script: 'dist/main.js',
      instances: 'max', // Spawns one process per CPU core
      exec_mode: 'cluster', // Enables clustering
      env: {
        NODE_ENV: 'production',
      },
    },
  ],
};
```

Start the application with PM2:

```bash
npm install -g pm2
pm2 start ecosystem.config.js
```

---

## 5. Common mistakes & gotchas

- **Including `devDependencies` in production containers.** Leaving packages like TypeScript, ESLint, or Jest inside your production runner container increases vulnerability surface areas and wastes server storage. Always use `npm prune --production` or double-copy builder patterns in Dockerfiles.
- **Forgetting Prisma Client generation inside Docker.** If you copy the builder files but forget to run `npx prisma generate` in your Dockerfile build stage, the application will crash with `PrismaClientInitializationError: Prisma Client has not been generated yet`.
- **Using `npm install` inside Docker build scripts.** Using `npm install` installs floating versions based on semantic versioning rules. Always use `npm ci` (clean install), which locks dependencies exactly to your `package-lock.json` file.

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **Multi-stage caching.** Structure Docker COPY instructions so dependencies are cached, avoiding installing node modules on every code change.
2. **Prune packages.** Run `npm prune --production` to clear compiler packages.
3. **Terminus probing.** Implement liveness probes assessing DB ping states to catch query lockups.
4. **Node non-root.** Set `USER node` inside Dockerfiles to protect your container from hosting root shell exploits.

### Interview Q&A

- **Q: Why should you use a multi-stage Dockerfile for Nest.js applications?**
  → It separates compilation steps from runtime. DevDependencies (like TS compiler, linters, testing frameworks) and raw source code are left in the temporary build image, resulting in a lightweight, secure production image containing only compiled JS and production dependencies.

- **Q: How does `@nestjs/terminus` assist in deployment orchestration?**
  → It exposes standard endpoints checking database connection viability or memory limits. In failure states, it returns a `503 Service Unavailable` status, signaling to load balancers (like Kubernetes) to stop routing traffic to this container.

- **Q: How does PM2 cluster mode utilize multi-core server processors?**
  → It leverages Node's built-in `cluster` module to spawn multiple instances of the application that share the same port. PM2 acts as a master process, distributing incoming TCP connections across child threads.

---

*← [15 — Queues and Task Scheduling](./15_queues_and_task_scheduling.md) | [17 — Interview Questions →](./17_interview_questions.md)*
