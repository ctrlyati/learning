# 16 — Messaging & Integration

> **Goal:** Integrate asynchronously with the outside world — produce/consume Kafka topics, work with RabbitMQ queues, and understand where Spring Integration fits.

---

## 1. Messaging — mental model + working code

Synchronous HTTP couples services tightly: caller waits for callee, an outage cascades. **Messaging decouples**: producer writes to a broker, consumers read independently. Slower consumers don't slow producers; outages buffer instead of cascading.

### When to reach for messaging

- **Async work** (sending email, generating PDF, processing video).
- **Cross-service events** (`OrderPlaced`, `InventoryReserved`).
- **Stream processing** (clicks, telemetry, logs).
- **Decoupled scaling** (10 consumers behind one topic).

### Kafka vs RabbitMQ — the short version

| Aspect              | Kafka                                | RabbitMQ                              |
| ------------------- | ------------------------------------ | ------------------------------------- |
| Model               | Distributed log (topic = ordered append) | Broker with queues and exchanges  |
| Strength            | High throughput, replay, partitioning | Routing, low-latency tasks            |
| Retention           | Days/weeks/forever                    | Until consumed                        |
| Consumer model      | Pull, offset-based                    | Push, ack-based                       |
| Typical use         | Event streaming, analytics, sourcing  | Task queues, RPC, routing             |

If you're not sure: Kafka for event-driven architectures and analytics, RabbitMQ for task queues and microservice RPC.

---

## 2. Kafka with Spring — what Spring does

### Add the starter

```xml
<dependency>
    <groupId>org.springframework.kafka</groupId>
    <artifactId>spring-kafka</artifactId>
</dependency>
```

### Configure

```yaml
spring:
  kafka:
    bootstrap-servers: localhost:9092
    producer:
      key-serializer: org.apache.kafka.common.serialization.StringSerializer
      value-serializer: org.springframework.kafka.support.serializer.JsonSerializer
      acks: all
      properties:
        enable.idempotence: true
    consumer:
      group-id: bookstore-api
      key-deserializer: org.apache.kafka.common.serialization.StringDeserializer
      value-deserializer: org.springframework.kafka.support.serializer.JsonDeserializer
      auto-offset-reset: earliest
      properties:
        spring.json.trusted.packages: "com.example.bookstore.*"
```

### Spring auto-config wires up:

- `KafkaTemplate<K, V>` — for producing
- `ConcurrentKafkaListenerContainerFactory` — for `@KafkaListener` methods
- Health indicators for the broker
- Micrometer metrics for sent/received counts and lag

### Produce

```java
@Service
public class OrderEventPublisher {

    private final KafkaTemplate<String, OrderPlacedEvent> template;

    public OrderEventPublisher(KafkaTemplate<String, OrderPlacedEvent> template) {
        this.template = template;
    }

    public void publish(OrderPlacedEvent event) {
        template.send("orders.placed", event.orderId().toString(), event);
    }
}
```

`KafkaTemplate.send` returns a `CompletableFuture<SendResult>` if you need confirmation.

### Consume

```java
@Component
public class InventoryListener {

    @KafkaListener(topics = "orders.placed", groupId = "inventory-service")
    public void onOrderPlaced(OrderPlacedEvent event) {
        // reserve inventory
    }
}
```

### Acknowledgement modes

```yaml
spring:
  kafka:
    listener:
      ack-mode: manual_immediate
```

```java
@KafkaListener(topics = "orders.placed")
public void onOrderPlaced(OrderPlacedEvent event, Acknowledgment ack) {
    try {
        processOrder(event);
        ack.acknowledge();              // explicit ack after successful processing
    } catch (Exception e) {
        // do NOT ack — message will be redelivered (or dead-lettered after retries)
        throw e;
    }
}
```

### Error handling & DLT

```java
@Bean
public DefaultErrorHandler errorHandler(KafkaTemplate<String, Object> template) {
    DeadLetterPublishingRecoverer recoverer = new DeadLetterPublishingRecoverer(template);
    FixedBackOff backOff = new FixedBackOff(1000L, 3);   // retry 3 times, 1s apart
    return new DefaultErrorHandler(recoverer, backOff);
}
```

Failed messages after retries go to `<topic>.DLT`. Monitor this topic; non-empty = on-call wakes up.

---

## 3. RabbitMQ with Spring AMQP

### Add the starter

```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-amqp</artifactId>
</dependency>
```

### Configure

```yaml
spring:
  rabbitmq:
    host: localhost
    port: 5672
    username: guest
    password: guest
```

### Declarative topology

```java
@Configuration
public class AmqpTopology {

    public static final String ORDERS_EXCHANGE = "orders";
    public static final String ORDERS_QUEUE = "orders.email-confirmations";

    @Bean public TopicExchange ordersExchange() { return new TopicExchange(ORDERS_EXCHANGE); }
    @Bean public Queue emailQueue() { return new Queue(ORDERS_QUEUE, true); }

    @Bean
    public Binding emailBinding(Queue emailQueue, TopicExchange ordersExchange) {
        return BindingBuilder.bind(emailQueue).to(ordersExchange).with("orders.placed");
    }

    @Bean
    public Jackson2JsonMessageConverter jsonConverter() {
        return new Jackson2JsonMessageConverter();
    }
}
```

### Produce

```java
@Service
public class OrderEventPublisher {

    private final RabbitTemplate rabbit;

    public OrderEventPublisher(RabbitTemplate rabbit) { this.rabbit = rabbit; }

    public void publish(OrderPlacedEvent event) {
        rabbit.convertAndSend(AmqpTopology.ORDERS_EXCHANGE, "orders.placed", event);
    }
}
```

### Consume

```java
@Component
public class EmailListener {

    @RabbitListener(queues = AmqpTopology.ORDERS_QUEUE)
    public void onOrderPlaced(OrderPlacedEvent event) {
        // send confirmation
    }
}
```

### Failure modes

- **No ack, no requeue:** message vanishes — bug.
- **Requeue forever:** poison message blocks the queue.
- **Dead-letter exchange** is the right pattern. Configure on the queue, route failures to a DLQ, alert when non-empty.

---

## 4. Practical application — order pipeline with Kafka + DLT

The flow:

```
[Order API]──@TransactionalEventListener(AFTER_COMMIT)──>[publish Kafka]──>[orders.placed]
                                                                              │
                                       ┌──────────────────────────────────────┴───┐
                                       ▼                                          ▼
                              [Inventory consumer]                       [Email consumer]
                                       │                                          │
                                       ▼                                          ▼
                              [orders.reserved]                          [orders.email-sent]
```

### Domain event

```java
public record OrderPlacedEvent(
    Long orderId,
    String userId,
    Long bookId,
    int quantity,
    BigDecimal total,
    Instant occurredAt
) {}
```

### Publish reliably (transactional outbox pattern — sketch)

The risk: order is saved, but Kafka publish fails after commit. Now state diverges. The "transactional outbox" pattern writes the event to an `outbox` table in the same transaction; a separate poller publishes to Kafka and marks rows sent.

For most apps, `@TransactionalEventListener(AFTER_COMMIT)` is enough. For high-stakes systems, use a proper outbox (libraries: Debezium CDC + Kafka Connect, or Eventuate).

```java
@Service
public class OrderService {

    private final OrderRepository repo;
    private final ApplicationEventPublisher events;

    public OrderService(OrderRepository repo, ApplicationEventPublisher events) {
        this.repo = repo;
        this.events = events;
    }

    @Transactional
    public Order placeOrder(...) {
        Order order = repo.save(...);
        events.publishEvent(new OrderPlacedEvent(...));
        return order;
    }
}

@Component
class OrderEventBridge {

    private final KafkaTemplate<String, OrderPlacedEvent> kafka;
    public OrderEventBridge(KafkaTemplate<String, OrderPlacedEvent> kafka) { this.kafka = kafka; }

    @TransactionalEventListener(phase = TransactionPhase.AFTER_COMMIT)
    public void bridge(OrderPlacedEvent event) {
        kafka.send("orders.placed", event.orderId().toString(), event);
    }
}
```

### Consumer with retry + DLT

```java
@Component
public class InventoryConsumer {

    private final InventoryService inventory;
    public InventoryConsumer(InventoryService inventory) { this.inventory = inventory; }

    @KafkaListener(topics = "orders.placed", groupId = "inventory")
    @RetryableTopic(
        attempts = "4",
        backoff = @Backoff(delay = 1000, multiplier = 2.0),
        dltStrategy = DltStrategy.FAIL_ON_ERROR,
        autoCreateTopics = "true"
    )
    public void onOrderPlaced(OrderPlacedEvent event) {
        inventory.reserve(event.bookId(), event.quantity());
    }

    @DltHandler
    public void handleDlt(OrderPlacedEvent event) {
        // alert, persist for manual review, etc.
    }
}
```

---

## Spring Integration — overview

[Spring Integration](https://spring.io/projects/spring-integration) is the older, swiss-army-knife abstraction for messaging-style flows. It implements [Enterprise Integration Patterns](https://www.enterpriseintegrationpatterns.com/): channels, splitters, aggregators, transformers, routers.

When useful:
- File polling pipelines (read folder → parse → enrich → write to DB).
- Adapters to legacy systems (FTP, JMS, mail, JMX).
- Complex routing not satisfied by raw Kafka/Rabbit.

Minimal example — file inbound adapter to DB:

```java
@Configuration
@EnableIntegration
public class FileImportFlow {

    @Bean
    public IntegrationFlow flow(BookRepository repo) {
        return IntegrationFlow.from(Files.inboundAdapter(new File("/tmp/imports"))
                            .patternFilter("*.csv"),
                        e -> e.poller(Pollers.fixedDelay(5000)))
            .transform(new FileToStringTransformer())
            .split(s -> s.delimiters("\n"))
            .filter((String line) -> !line.isBlank())
            .handle(line -> {
                String[] parts = ((String) line.getPayload()).split(",");
                repo.save(new Book(parts[0], parts[1], new BigDecimal(parts[2])));
            })
            .get();
    }
}
```

Most modern stacks reach for Kafka + Spring Cloud Stream or direct Spring Kafka instead. Know Spring Integration exists; don't default to it.

---

## 5. Common Mistakes & Gotchas

- **Publishing inside a transaction without `AFTER_COMMIT`.** Order rolls back, but Kafka message went out. Downstream services act on a phantom order.

- **No idempotency in consumers.** Kafka delivers at-least-once. Re-processing the same `OrderPlacedEvent` reserves inventory twice. Store processed event IDs or use idempotent operations.

- **Hard-coded consumer group ID across environments.** Dev consumer reads prod-style topics. Always template the group ID with profile/env.

- **Ignoring DLT/DLQ.** Failed messages pile up unnoticed. Set up alerts. Treat DLT non-empty as a P1.

- **Long-running processing in the listener thread.** Listener thread is blocked; consumer lag grows; rebalances thrash. Hand off to `@Async` or a worker pool.

- **Serialization mismatches.** Producer writes JSON with field X; consumer expects field Y. Breaking changes are silent until a real message arrives. Use a schema registry (Confluent, Apicurio) for production.

- **Auto-create topics in production.** Hides config drift. Topics in prod should be Terraform/Kafka-CLI-created with intentional partition counts and retention.

- **One topic per consumer.** Anti-pattern. Topics model **events**; consumer groups model **subscribers**. Multiple groups can read the same topic.

- **Using Kafka as a queue.** It's a log. If you need work-distribution semantics with ack/nack and TTL, RabbitMQ or a real queue is a better fit.

- **No backpressure.** Producer outpaces consumers, Kafka retention is exceeded, messages drop silently. Monitor lag (`kafka.consumer.lag` metric).

---

## 🎯 Key Takeaways

- **Async messaging decouples failures.** A down consumer doesn't stop the producer.
- **Kafka for streams + replay; RabbitMQ for routing + tasks.** Pick by use case.
- **`@TransactionalEventListener(AFTER_COMMIT)` is the simplest bridge** from DB write to broker publish. Outbox pattern is the bulletproof one.
- **At-least-once delivery means idempotent consumers.** Always.
- **DLT/DLQ are not optional.** Configure, alert, and triage.

*[← prev](./15_building_apis.md) | [next →](./17_production.md)*
