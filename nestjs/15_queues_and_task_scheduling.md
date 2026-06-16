# 15 — Queues and Task Scheduling

> **Goal:** Run scheduled tasks (cron jobs), configure background worker queues using BullMQ, publish background jobs from producers, and run async processors.

---

## 1. Task Scheduling with `@nestjs/schedule`

Task scheduling allows you to run functions at specific times (e.g. database cleanups, generating daily email reports, caching remote queries). Nest provides the `@nestjs/schedule` wrapper, which integrates standard Node-cron libraries.

### Setup

```bash
npm install --save @nestjs/schedule
npm install --save-dev @types/cron
```

### Basic Cron & Interval Configuration

Register the `ScheduleModule` in the root module:

```typescript
// src/app.module.ts
import { Module } from '@nestjs/common';
import { ScheduleModule } from '@nestjs/schedule';

@Module({
  imports: [
    ScheduleModule.forRoot(), // Setup scheduling context
  ],
})
export class AppModule {}
```

Now define jobs inside any `@Injectable()` service provider:

```typescript
// src/tasks/tasks.service.ts
import { Injectable, Logger } from '@nestjs/common';
import { Cron, CronExpression, Interval, Timeout } from '@nestjs/schedule';

@Injectable()
export class TasksService {
  private readonly logger = new Logger(TasksService.name);

  // 1. Cron Job: Runs every day at midnight
  @Cron(CronExpression.EVERY_DAY_AT_MIDNIGHT)
  handleMidnightCleanup() {
    this.logger.log('Executing database log cleanup...');
    // Log cleanup SQL query...
  }

  // 2. Dynamic Cron: Runs every 45 seconds
  @Cron('*/45 * * * * *')
  handleCustomCron() {
    this.logger.log('Executing 45s heartbeat check...');
  }

  // 3. Interval: Runs every 10 seconds (10000ms) after boot
  @Interval(10000)
  handleHeartbeat() {
    this.logger.debug('System healthy check');
  }

  // 4. Timeout: Runs once 5 seconds (5000ms) after application startup
  @Timeout(5000)
  handleStartupInit() {
    this.logger.log('Running delayed database seeding confirmation...');
  }
}
```

---

## 2. Background Processing with BullMQ

For resource-heavy tasks (e.g. video processing, PDF exports, bulk emails), executing them inside HTTP requests is an anti-pattern. If a task takes 10 seconds, the client connection will time out.

**BullMQ** solves this. It is a robust, Redis-backed queue system. A service **produces** (adds) a job to a queue, and a background worker **consumes** (processes) the job asynchronously.

The mental model is:

```
HTTP Post ──> [ UsersService ] ──( Adds Job )──> [ Redis Queue ]
                                                       │
                                                (Worker picks up)
                                                       ▼
                                             [ EmailProcessor ]
```

---

## 3. Implementing Worker Queues in Nest.js

### Install Dependencies

Make sure you have a running **Redis** server locally (e.g., via Docker: `docker run -d -p 6379:6379 redis:latest`).

```bash
npm install --save @nestjs/bull bullmq
```

### Import BullModule

```typescript
// src/app.module.ts
import { Module } from '@nestjs/common';
import { BullModule } from '@nestjs/bull';

@Module({
  imports: [
    // Configure connection to Redis
    BullModule.forRoot({
      redis: {
        host: 'localhost',
        port: 6379,
      },
    }),
  ],
})
export class AppModule {}
```

### Register a Queue

Register a specific queue (e.g. `'email-sending'`) in the target feature module:

```typescript
// src/email/email.module.ts
import { Module } from '@nestjs/common';
import { BullModule } from '@nestjs/bull';
import { EmailProcessor } from './email.processor';
import { EmailService } from './email.service';

@Module({
  imports: [
    // Register the queue name in DI container
    BullModule.registerQueue({
      name: 'email-sending',
    }),
  ],
  providers: [EmailService, EmailProcessor],
  exports: [EmailService],
})
export class EmailModule {}
```

---

## 4. Practical Application — Producer and Processor

### The Producer (Adding Jobs)

Inject the queue using the `@InjectQueue()` decorator and publish a job:

```typescript
// src/email/email.service.ts
import { Injectable } from '@nestjs/common';
import { InjectQueue } from '@nestjs/bull';
import { Queue } from 'bull';

@Injectable()
export class EmailService {
  // Inject the queue instance
  constructor(
    @InjectQueue('email-sending') private emailQueue: Queue
  ) {}

  async sendWelcomeEmail(email: string, username: string) {
    // Publish a job to the Redis queue
    await this.emailQueue.add(
      'welcome', // Job name
      { email, username }, // Payload passed to consumer
      { attempts: 3, backoff: 5000 } // Auto-retry 3 times with 5s delay on failure
    );
    console.log(`Job queued for user: ${email}`);
  }
}
```

### The Processor (Worker Consuming Jobs)

Create a class decorated with `@Processor()` to consume jobs from the Redis queue.

```typescript
// src/email/email.processor.ts
import { Processor, Process } from '@nestjs/bull';
import { Job } from 'bull';

@Processor('email-sending') // Bind to the queue name
export class EmailProcessor {
  
  // Bind to the job name 'welcome'
  @Process('welcome')
  async handleWelcomeEmail(job: Job<{ email: string; username: string }>) {
    console.log(`[Worker] Processing welcome email job #${job.id}...`);
    const { email, username } = job.data;

    // Simulate sending email (slow network request)
    await new Promise((resolve) => setTimeout(resolve, 3000));

    console.log(`[Worker] Welcome email successfully sent to: ${email}`);
    return { success: true };
  }
}
```

---

## 5. Common mistakes & gotchas

- **Blocking the main event loop in processors.** BullMQ processors run in the main Node.js event loop by default. If your processor runs CPU-bound operations (e.g. heavy image resizing), it will freeze the entire HTTP server. Use **separate process sandboxes** (worker threads) for CPU-bound tasks.
- **Forgetting that Redis must be running.** If your local Redis server is down, Nest will fail to boot or throw continuous Redis connection errors, locking application execution.
- **Not handling job failures.** If an email processor crashes because of an external API timeout, the job is marked as failed. Always configure auto-retries (`attempts: 3`) and set up event listeners (`@OnQueueFailed()`) to log failures to monitoring pipelines.

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **Schedule hooks.** `@Cron()` and `@Interval()` require a root `ScheduleModule` registration to execute.
2. **Redis backed.** BullMQ saves all queue jobs in Redis, allowing persistence across restarts.
3. **Decouple writes.** Push slow processes to background worker queues to keep HTTP response times short (< 100ms).
4. **Retry configs.** Always configure attempts and backoff rates on critical jobs like payment processing.

### Interview Q&A

- **Q: Why should you use a queue system like BullMQ instead of executing async tasks using `setTimeout()` inside controllers?**
  → `setTimeout` runs in memory. If the application server crashes or restarts, all pending tasks are lost forever. BullMQ stores jobs in Redis, ensuring durability, progress tracking, automatic retries on failure, and concurrency limits across distributed servers.

- **Q: What happens if a worker processor blocks the CPU?**
  → Since Node.js is single-threaded, a CPU-intensive worker will block the event loop, preventing the HTTP server from accepting connections or resolving other requests, leading to server downtime. CPU-intensive operations should run in child processes/worker threads.

- **Q: How does the `@InjectQueue()` decorator work in Nest.js?**
  → It retrieves the BullMQ `Queue` instance registered under the specified string name from the DI container, allowing services to add new jobs (payloads and configuration options) to Redis.

---

*← [14 — WebSockets and Microservices](./14_websockets_and_microservices.md) | [16 — Production and Docker →](./16_production_and_docker.md)*
