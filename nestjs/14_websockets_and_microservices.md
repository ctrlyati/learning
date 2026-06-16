# 14 — WebSockets and Microservices

> **Goal:** Create real-time WebSockets Gateways, understand microservice topologies, configure message and event-driven patterns, and establish communication using TCP transport.

---

## 1. WebSockets with Gateways

WebSockets allow two-way, persistent connection channels between the client browser and the server. In Nest.js, WebSocket entry points are called **Gateways**. Gateways are decorated with `@WebSocketGateway()` and are treated as providers (supporting Dependency Injection).

### Setup Dependencies

```bash
npm install --save @nestjs/websockets @nestjs/platform-socket.io
```

### Implementing a Chat Gateway

Here is a gateway that listens for `'message'` events and broadcasts them to all connected clients:

```typescript
// src/chat/chat.gateway.ts
import { 
  SubscribeMessage, 
  WebSocketGateway, 
  WebSocketServer, 
  OnGatewayConnection, 
  OnGatewayDisconnect,
  MessageBody,
  ConnectedSocket
} from '@nestjs/websockets';
import { Server, Socket } from 'socket.io';

@WebSocketGateway({ cors: { origin: '*' } }) // Enable CORS
export class ChatGateway implements OnGatewayConnection, OnGatewayDisconnect {
  // Access the underlying Socket.io Server instance
  @WebSocketServer()
  server: Server;

  handleConnection(client: Socket) {
    console.log(`Client connected: ${client.id}`);
  }

  handleDisconnect(client: Socket) {
    console.log(`Client disconnected: ${client.id}`);
  }

  // Listens for: socket.emit('msgToServer', data)
  @SubscribeMessage('msgToServer')
  handleMessage(
    @MessageBody() payload: { sender: string; text: string },
    @ConnectedSocket() client: Socket
  ): void {
    console.log(`Received message from ${payload.sender}: ${payload.text}`);
    
    // Broadcast message to all connected clients
    this.server.emit('msgToClient', {
      sender: payload.sender,
      text: payload.text,
      timestamp: new Date().toISOString(),
    });
  }
}
```

---

## 2. Microservice Architectures in Nest.js

A Nest.js microservice is an application that uses a **different transport layer** than HTTP. Nest provides built-in transporters for:
- TCP (Default)
- Redis
- RabbitMQ
- NATS
- gRPC
- Kafka

The mental model is:

```
[ HTTP Gateway API ] ──( ClientProxy Client )──> [ TCP Microservice ]
                                                       │
                                            (Resolves query/event)
```

---

## 3. Communication Patterns: Request-Response vs. Event-Driven

Nest microservices distinguish between two types of messages:

- **Request-Response (`@MessagePattern`):** The sender publishes a message and expects a response (comparable to RPC or HTTP).
- **Event-Driven (`@EventPattern`):** The sender publishes an event and returns immediately (comparable to Pub/Sub).

---

## 4. Practical Application — Creating a TCP Microservice

Let's build a microservice that handles user registration audit logs over TCP.

### Step 1: Create the Microservice Application (`src/microservice.ts`)

Instead of standard HTTP servers, we bootstrap a microservice:

```typescript
// src/microservice.ts
import { NestFactory } from '@nestjs/core';
import { Transport, MicroserviceOptions } from '@nestjs/microservices';
import { AppModule } from './app.module';

async function bootstrap() {
  const app = await NestFactory.createMicroservice<MicroserviceOptions>(
    AppModule,
    {
      transport: Transport.TCP,
      options: {
        host: '127.0.0.1',
        port: 8888, // TCP port
      },
    },
  );
  await app.listen();
  console.log('TCP Microservice is listening...');
}
bootstrap();
```

### Step 2: Implement the Controller inside the Microservice

```typescript
// src/audit/audit.controller.ts
import { Controller } from '@nestjs/common';
import { MessagePattern, EventPattern, Payload } from '@nestjs/microservices';

@Controller()
export class AuditController {
  
  // Request-Response pattern
  @MessagePattern({ cmd: 'get_status' })
  getStatus(data: any): string {
    return 'Audit service is healthy';
  }

  // Event-Driven pattern (No response expected)
  @EventPattern('user_created')
  handleUserCreated(@Payload() data: { email: string }) {
    console.log(`[Audit Event] Logging user creation: ${data.email}`);
    // Save to audit logs database...
  }
}
```

### Step 3: Communicating from the HTTP Gateway

To send messages to our TCP microservice, the HTTP server registers a `ClientProxy` provider.

```typescript
// src/users/users.module.ts
import { Module } from '@nestjs/common';
import { ClientsModule, Transport } from '@nestjs/microservices';
import { UsersController } from './users.controller';
import { UsersService } from './users.service';

@Module({
  imports: [
    // Register the client proxy
    ClientsModule.register([
      {
        name: 'AUDIT_SERVICE', // DI Token
        transport: Transport.TCP,
        options: {
          host: '127.0.0.1',
          port: 8888,
        },
      },
    ]),
  ],
  controllers: [UsersController],
  providers: [UsersService],
})
export class UsersModule {}
```

Usage in the service:

```typescript
// src/users/users.service.ts
import { Injectable, Inject } from '@nestjs/common';
import { ClientProxy } from '@nestjs/microservices';

@Injectable()
export class UsersService {
  constructor(
    @Inject('AUDIT_SERVICE') private client: ClientProxy
  ) {}

  async registerUser(email: string) {
    // 1. Emit event asynchronously (No response wait)
    this.client.emit('user_created', { email });

    // 2. Request-Response query (Awaits response)
    const status = await this.client.send({ cmd: 'get_status' }, {}).toPromise();
    console.log(`Audit service status: ${status}`);

    return { success: true };
  }
}
```

---

## 5. Common mistakes & gotchas

- **Exposing WebSocket gateways without CORS enabled.** By default, socket connections from different domains will be rejected by Socket.io. Always set options like `{ cors: { origin: '*' } }` to allow connections from frontend test suites.
- **Forgetting that `ClientProxy.send()` returns an RxJS Observable.** Running `this.client.send(...)` will **not** send the request. In RxJS, Observables are cold: they do nothing until you subscribe to them. You must use `.subscribe()` or convert it using `.toPromise()` / `firstValueFrom()` for it to execute:
  ```typescript
  // Send is triggered
  const response = await firstValueFrom(this.client.send({ cmd: 'get' }, {}));
  ```
- **Confusing event streams with message queries.** Using `client.send` for a method decorated with `@EventPattern` or `client.emit` for a method decorated with `@MessagePattern` will lead to silent failures or stuck client connections.

---

## 🎯 Key Takeaways & Interview Q&A

### Key Takeaways
1. **Gateways are Providers.** WebSocket gateways support the same dependency injection lifecycle as regular services.
2. **Cold Observables.** Always subscribe to or await microservice `send()` calls.
3. **Transport flex.** Swap transports (TCP, Redis, RabbitMQ) via options configuration; handlers code remains identical.
4. **Pattern Matching.** Use `@MessagePattern` for queries; `@EventPattern` for background commands.

### Interview Q&A

- **Q: What is the difference between `@MessagePattern` and `@EventPattern` decorators in Nest.js microservices?**
  → `@MessagePattern` is used for Request-Response patterns. The sender expects a returning payload (behaves like an RPC call). `@EventPattern` is used for Event-Driven patterns. The publisher sends a message and returns immediately without waiting for execution results (behaves like a Pub/Sub event).

- **Q: Why will running `this.client.send('pattern', data)` fail to trigger a request if you don't await or subscribe to it?**
  → Nest's `ClientProxy.send()` returns an RxJS Observable. Observables are cold: they do not trigger network requests until there is a subscriber listening. Converting it to a promise via `firstValueFrom` or calling `.subscribe()` is required.

- **Q: How does Nest.js handle WebSocket connections, and what is a Gateway?**
  → A Gateway is a wrapper class decorated with `@WebSocketGateway()` which manages socket configurations, connection lifecycles, and routes incoming message events using `@SubscribeMessage()`.

---

*← [13 — Testing: Unit and E2E](./13_testing_unit_and_e2e.md) | [15 — Queues and Task Scheduling →](./15_queues_and_task_scheduling.md)*
