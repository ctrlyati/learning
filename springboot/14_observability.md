# 14 — Observability

> **Goal:** Make your app legible in production via Actuator endpoints, Micrometer metrics, health checks, and distributed tracing — the three pillars of modern observability.

---

## 1. Spring Boot Actuator — mental model + working code

Observability has three pillars: **logs**, **metrics**, **traces**. Boot's **Actuator** is the framework that exposes them (and more) over HTTP/JMX.

### Add the starter

```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-actuator</artifactId>
</dependency>
```

### Default exposure

By default, only `/actuator/health` is exposed over HTTP. Open more:

```yaml
management:
  endpoints:
    web:
      exposure:
        include: health, info, metrics, prometheus, mappings, env, conditions, beans, loggers
  endpoint:
    health:
      show-details: when-authorized
      probes:
        enabled: true     # /actuator/health/liveness and /readiness for Kubernetes
  info:
    env:
      enabled: true       # expose info.* properties
    git:
      mode: full
```

### Hit them

```bash
curl localhost:8080/actuator
curl localhost:8080/actuator/health
curl localhost:8080/actuator/health/liveness
curl localhost:8080/actuator/health/readiness
curl localhost:8080/actuator/metrics
curl localhost:8080/actuator/metrics/jvm.memory.used
curl localhost:8080/actuator/loggers
curl localhost:8080/actuator/mappings
```

### Custom info

```yaml
info:
  app:
    name: bookstore-api
    version: 1.4.2
```

```bash
curl localhost:8080/actuator/info
# {"app":{"name":"bookstore-api","version":"1.4.2"}}
```

---

## 2. Health checks — what Spring does

Spring assembles health from multiple `HealthIndicator` beans:

- `DataSourceHealthIndicator` — checks DB connection
- `DiskSpaceHealthIndicator` — checks free disk
- `RedisHealthIndicator` — checks Redis
- `PingHealthIndicator` — always UP

Aggregate status = worst child status. One DOWN → /health is DOWN.

### Custom health indicator

```java
package com.example.bookstore.health;

import org.springframework.boot.actuate.health.*;
import org.springframework.stereotype.Component;

@Component
public class CatalogServiceHealthIndicator implements HealthIndicator {

    private final CatalogClient client;
    public CatalogServiceHealthIndicator(CatalogClient client) { this.client = client; }

    @Override
    public Health health() {
        try {
            var status = client.ping();
            return Health.up().withDetail("upstream", status).build();
        } catch (Exception e) {
            return Health.down(e).build();
        }
    }
}
```

Hit `/actuator/health` and your indicator is automatically included under `components.catalogService`.

### Liveness vs readiness (Kubernetes-friendly)

- **Liveness**: "Is the JVM healthy?" Failure → kill the pod.
- **Readiness**: "Should I receive traffic right now?" Failure → remove from load balancer, but keep running (e.g., during DB migration startup).

Boot exposes them at `/actuator/health/liveness` and `/actuator/health/readiness` when `probes.enabled: true`. You influence readiness with `ApplicationAvailability`:

```java
@Component
public class StartupReadiness {

    private final ApplicationEventPublisher events;
    public StartupReadiness(ApplicationEventPublisher events) { this.events = events; }

    @EventListener
    public void onMigrationsDone(MigrationsCompletedEvent e) {
        AvailabilityChangeEvent.publish(events, this, ReadinessState.ACCEPTING_TRAFFIC);
    }
}
```

---

## 3. Micrometer & metrics — depth

**Micrometer** is the abstraction; you pick the backend (Prometheus, Datadog, New Relic, etc.).

### Built-in metrics (out of the box)

- JVM: `jvm.memory.used`, `jvm.gc.*`, `jvm.threads.live`
- HTTP server: `http.server.requests` (with tags: `uri`, `method`, `status`, `exception`)
- HTTP client: `http.client.requests`
- DataSource: `hikaricp.connections.*`
- Logback: `logback.events`
- Tomcat: `tomcat.sessions.*`, `tomcat.threads.*`

### Prometheus

```xml
<dependency>
    <groupId>io.micrometer</groupId>
    <artifactId>micrometer-registry-prometheus</artifactId>
</dependency>
```

```yaml
management:
  endpoints:
    web:
      exposure:
        include: health, prometheus, metrics
  metrics:
    tags:
      application: ${spring.application.name}
      env: ${spring.profiles.active:default}
```

Scrape:
```bash
curl localhost:8080/actuator/prometheus
# # HELP jvm_memory_used_bytes The amount of used memory
# # TYPE jvm_memory_used_bytes gauge
# jvm_memory_used_bytes{application="bookstore-api",area="heap",id="..."} 1.234E8
```

### Custom counters and timers

```java
@Service
public class OrderService {

    private final Counter ordersCreated;
    private final Timer placeOrderTimer;

    public OrderService(MeterRegistry registry) {
        this.ordersCreated = Counter.builder("bookstore.orders.created")
            .description("Total orders created")
            .tag("source", "api")
            .register(registry);
        this.placeOrderTimer = Timer.builder("bookstore.orders.place.duration")
            .publishPercentiles(0.5, 0.95, 0.99)
            .register(registry);
    }

    public Order placeOrder(...) {
        return placeOrderTimer.record(() -> {
            // expensive work
            Order o = doPlaceOrder(...);
            ordersCreated.increment();
            return o;
        });
    }
}
```

### `@Timed` annotation (with Spring AOP)

```java
@Timed(value = "bookstore.book.get", percentiles = {0.5, 0.95})
public BookDto getOne(Long id) { ... }
```

### Common tag mistake

Avoid high-cardinality tags. `tag("userId", id)` with 10M users = 10M time series = your metrics backend dies. Tags should have **bounded** cardinality (env, region, http_method, status_code).

---

## 4. Distributed tracing — Micrometer Tracing + OpenTelemetry

In Boot 3, **Micrometer Tracing** replaces Spring Cloud Sleuth. It auto-instruments HTTP, JDBC, Kafka, etc., and exports spans to Zipkin, Jaeger, OTLP, etc.

### Add the deps

```xml
<dependency>
    <groupId>io.micrometer</groupId>
    <artifactId>micrometer-tracing-bridge-otel</artifactId>
</dependency>
<dependency>
    <groupId>io.opentelemetry</groupId>
    <artifactId>opentelemetry-exporter-otlp</artifactId>
</dependency>
```

### Configure

```yaml
management:
  tracing:
    sampling:
      probability: 1.0   # 100% in dev; 0.1 in prod
  otlp:
    tracing:
      endpoint: http://otel-collector:4318/v1/traces
```

Now every HTTP request has a trace ID propagated via `traceparent` header, and your logs include it (with the right logback pattern):

```xml
<!-- logback-spring.xml -->
<pattern>%d{HH:mm:ss.SSS} [%X{traceId:-} %X{spanId:-}] %-5level %logger{36} - %msg%n</pattern>
```

### Manual spans for important business operations

```java
@Service
public class OrderService {

    private final Tracer tracer;
    public OrderService(Tracer tracer) { this.tracer = tracer; }

    public Order placeOrder(...) {
        var span = tracer.nextSpan().name("place-order").start();
        try (var ws = tracer.withSpan(span)) {
            span.tag("user.id", userId.toString());
            // work...
            return order;
        } finally {
            span.end();
        }
    }
}
```

Or via the `@Observed` annotation (cleanest):

```java
@Observed(name = "order.place", contextualName = "place-order")
public Order placeOrder(...) { ... }
```

### Practical application — fully observable order endpoint

```java
@RestController
@RequestMapping("/api/v1/orders")
public class OrderController {

    private final OrderService service;
    private final Counter ordersCreated;
    private final Timer placeTimer;

    public OrderController(OrderService service, MeterRegistry registry) {
        this.service = service;
        this.ordersCreated = registry.counter("bookstore.orders.created");
        this.placeTimer = registry.timer("bookstore.orders.place.duration");
    }

    @PostMapping
    @Observed(name = "order.create")
    public ResponseEntity<OrderDto> create(@Valid @RequestBody PlaceOrderRequest req) {
        return placeTimer.record(() -> {
            OrderDto created = service.placeOrder(req);
            ordersCreated.increment();
            return ResponseEntity.status(201).body(created);
        });
    }
}
```

Result in production:
- Every request shows in `http.server.requests` with status/path tags.
- `bookstore.orders.created` counts business-meaningful events.
- `bookstore.orders.place.duration` shows p50/p95/p99 latencies.
- A trace ID ties together the API call, DB calls, downstream HTTP, and the listener that emails confirmation.

---

## 5. Common Mistakes & Gotchas

- **Exposing `/actuator/env`, `/heapdump`, `/threaddump` publicly.** They leak secrets, memory snapshots, internal state. Either put Actuator on a separate port, behind auth, or behind a private network.

- **High-cardinality metric tags.** `path`, `userId`, `traceId` as tags = explosion. Stick to bounded enums.

- **Health checks that hit slow upstream services.** Your `/health` calls a slow vendor API → load balancer thinks you're DOWN → cascading outage. Health checks should be **cheap and local**. Use circuit breakers for upstream calls.

- **Treating `/actuator/health` as a deep system check.** It tells you "this instance is up" — not "the whole platform is healthy." Don't make architecture decisions from a single health endpoint.

- **Forgetting to set sampling in production.** 100% trace sampling = huge volume in your tracing backend. Sample 1-10% and use tail-based sampling at the collector.

- **No application.name or env tags on metrics.** Mixed dashboards for staging+prod look identical and you'll diagnose the wrong thing.

- **Custom `HealthIndicator` that returns DOWN on a transient blip.** Spring asks every few seconds; a single blip = pod restart. Add hysteresis or convert to a metric instead.

- **Logback `MDC` without trace propagation.** You log to file but the trace IDs aren't in your log lines, so logs and traces can't be correlated. Configure the pattern with `%X{traceId}` and `%X{spanId}`.

- **Counting metrics that mean nothing.** `methodCalled` counters that everyone ignores. Counters should map to **business questions**: "how many orders placed?", "how many DB retries?", not "how often was this method called?"

---

## 🎯 Key Takeaways

- **Actuator is non-negotiable in production.** `/health`, `/metrics`, `/prometheus` minimum.
- **Liveness ≠ readiness.** Wire them correctly for your orchestrator.
- **Micrometer abstracts the metrics backend.** Write to `MeterRegistry`; swap Prometheus for Datadog without code changes.
- **Tag cardinality is the silent killer.** Bound tags or pay the bill.
- **Trace IDs in logs** make log search 10x more useful. Wire MDC.

*[← prev](./13_caching.md) | [next →](./15_building_apis.md)*
