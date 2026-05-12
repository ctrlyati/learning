# 17 — Production

> **Goal:** Take a Spring Boot 3 app from "runs on my machine" to "runs reliably in production" — packaging, containerization, native images, graceful shutdown, and the operational patterns that keep you on-call peaceful.

---

## 1. Packaging — mental model + working code

Spring Boot's standard deployment unit is the **fat (uber) jar**: your code + all dependencies + an embedded server, in a single file.

### Build it

```bash
./mvnw clean package
java -jar target/bookstore-api-0.0.1-SNAPSHOT.jar
```

Or with Gradle:

```bash
./gradlew clean bootJar
java -jar build/libs/bookstore-api-0.0.1-SNAPSHOT.jar
```

### The layered jar — better Docker caching

Boot 3 supports layered jars: dependencies, snapshot dependencies, Spring Boot loader, and your code are separate layers. Your code changes daily; dependencies rarely. Layering lets Docker cache the heavy layers and only rebuild the thin top layer.

Build layered:

```bash
./mvnw spring-boot:build-image
# or extract layers from an existing jar
java -Djarmode=layertools -jar app.jar extract
```

The Dockerfile pattern:

```dockerfile
# Stage 1: extract layers
FROM eclipse-temurin:21-jre AS builder
WORKDIR /build
COPY target/bookstore-api-0.0.1-SNAPSHOT.jar app.jar
RUN java -Djarmode=layertools -jar app.jar extract

# Stage 2: runtime
FROM eclipse-temurin:21-jre
WORKDIR /app
COPY --from=builder /build/dependencies/         ./
COPY --from=builder /build/spring-boot-loader/   ./
COPY --from=builder /build/snapshot-dependencies/ ./
COPY --from=builder /build/application/          ./

EXPOSE 8080
USER 1000:1000
ENTRYPOINT ["java", "org.springframework.boot.loader.launch.JarLauncher"]
```

Build:
```bash
docker build -t bookstore-api:1.0.0 .
docker run --rm -p 8080:8080 -e SPRING_PROFILES_ACTIVE=prod bookstore-api:1.0.0
```

### Or use the Spring Boot image builder (Cloud Native Buildpacks)

```bash
./mvnw spring-boot:build-image \
  -Dspring-boot.build-image.imageName=bookstore-api:1.0.0
```

Produces an optimized OCI image with no Dockerfile. Includes appropriate base layer, layered jar, and security defaults. Highly recommended starting point.

---

## 2. Native image with GraalVM — what & when

Native image compiles your app **ahead-of-time** to a self-contained executable. Tradeoffs:

| Aspect          | JVM                          | Native                                 |
| --------------- | ---------------------------- | -------------------------------------- |
| Startup         | 2–10s                        | 30–100ms                               |
| Memory          | 200MB+                       | 50–100MB                               |
| Build time      | 30s                          | 3–10 minutes                           |
| Peak throughput | Higher (JIT optimizes hot paths) | Lower (no JIT)                     |
| Reflection      | Works                        | Needs config hints                     |
| Dynamic class loading | Works                  | Limited                                |

**When native shines:** serverless (Lambda, Cloud Run), CLI tools, low-replica systems where startup time matters.

**When to stick with JVM:** long-running services, heavy reflection, dynamic class loading, max throughput. Most apps.

### Build native

Requires GraalVM 21+ and the `native` profile (set up by Initializr):

```bash
./mvnw -Pnative native:compile
./target/bookstore-api
```

Gradle:
```bash
./gradlew nativeCompile
./build/native/nativeCompile/bookstore-api
```

### Compatibility notes

- **Use only Boot-blessed libraries** for native — Spring tracks compatibility. Random third-party libs may need reflection hints.
- **Test in JVM and native** — most logic works identically, but reflection-heavy code (some serialization libs, AOP edge cases) can diverge.
- **Add `@RegisterReflectionForBinding`** for classes Jackson serializes that aren't directly referenced in your code.

---

## 3. Externalized configuration & secrets

Reprise of module 04 with production focus:

### 12-factor in practice

```yaml
# application.yml — defaults, in code
spring:
  application:
    name: bookstore-api
  datasource:
    url: ${DATABASE_URL:jdbc:postgresql://localhost:5432/bookstore}
    username: ${DATABASE_USER:app}
    password: ${DATABASE_PASSWORD}

server:
  port: ${PORT:8080}

logging:
  level:
    root: ${LOG_LEVEL:INFO}
```

In Kubernetes:

```yaml
env:
  - name: SPRING_PROFILES_ACTIVE
    value: prod
  - name: DATABASE_URL
    value: jdbc:postgresql://pg.production:5432/bookstore
  - name: DATABASE_PASSWORD
    valueFrom:
      secretKeyRef:
        name: bookstore-db
        key: password
```

### Secret rotation

Reading secrets at startup means rotation requires restart. For long-running apps with frequent rotation, watch the secret source (Vault Spring integration, Reloader for ConfigMaps) and refresh affected beans. Don't roll your own — use Spring Cloud Config or Vault integration.

### Config validation at startup

Module 04 covered `@ConfigurationProperties` validation. In production, missing or invalid required env vars must crash startup loudly. Better than running on bad config.

---

## 4. Practical application — production readiness checklist

### Graceful shutdown

```yaml
server:
  shutdown: graceful
spring:
  lifecycle:
    timeout-per-shutdown-phase: 30s
```

On SIGTERM, Tomcat stops accepting new requests and waits up to 30 seconds for in-flight ones to complete. Kubernetes sends SIGTERM, then waits `terminationGracePeriodSeconds` (default 30). Align these:

```yaml
# Kubernetes
spec:
  terminationGracePeriodSeconds: 60
  containers:
    - lifecycle:
        preStop:
          exec:
            command: ["sleep", "10"]   # let load balancer remove pod first
```

### Health probes wired to Kubernetes

From module 14:

```yaml
management:
  endpoint:
    health:
      probes:
        enabled: true
      show-details: never
  endpoints:
    web:
      exposure:
        include: health, prometheus, info
```

Kubernetes:
```yaml
livenessProbe:
  httpGet:
    path: /actuator/health/liveness
    port: 8080
  initialDelaySeconds: 30
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /actuator/health/readiness
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

### Resource limits & JVM tuning

```yaml
resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 2
    memory: 1Gi
```

Container-aware JVM (Java 17+ does this by default):

```bash
java -XX:MaxRAMPercentage=75.0 -jar app.jar
```

Don't set `-Xmx` to a fixed value in containers — use a percentage so memory limits drive the heap.

### Logging in JSON for log aggregation

```xml
<dependency>
    <groupId>net.logstash.logback</groupId>
    <artifactId>logstash-logback-encoder</artifactId>
    <version>7.4</version>
</dependency>
```

`logback-spring.xml`:
```xml
<configuration>
    <springProfile name="prod">
        <appender name="JSON" class="ch.qos.logback.core.ConsoleAppender">
            <encoder class="net.logstash.logback.encoder.LogstashEncoder">
                <includeMdcKeyName>traceId</includeMdcKeyName>
                <includeMdcKeyName>spanId</includeMdcKeyName>
            </encoder>
        </appender>
        <root level="INFO">
            <appender-ref ref="JSON" />
        </root>
    </springProfile>
    <springProfile name="!prod">
        <include resource="org/springframework/boot/logging/logback/base.xml" />
    </springProfile>
</configuration>
```

### Connection pool tuning (HikariCP — default)

```yaml
spring:
  datasource:
    hikari:
      maximum-pool-size: 20
      minimum-idle: 5
      connection-timeout: 5000
      idle-timeout: 600000
      max-lifetime: 1800000
      leak-detection-threshold: 60000
```

Rule of thumb: pool size = `(2 × cpu_cores) + effective_spindle_count`. For a typical 4-core container against Postgres: 8–12 connections.

### Timeouts everywhere

Default to **no timeout** is the most common production cause of cascading failures. Always set timeouts on HTTP clients, DB queries, async tasks.

```java
@Bean
public RestClient catalogClient() {
    return RestClient.builder()
        .baseUrl("https://catalog.example.com")
        .requestFactory(new JdkClientHttpRequestFactory(
            HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(2)).build()))
        .build();
}
```

### Circuit breakers (Resilience4j)

```xml
<dependency>
    <groupId>io.github.resilience4j</groupId>
    <artifactId>resilience4j-spring-boot3</artifactId>
</dependency>
```

```java
@Service
public class CatalogService {

    @CircuitBreaker(name = "catalog", fallbackMethod = "fallback")
    @Retry(name = "catalog")
    @TimeLimiter(name = "catalog")
    public CompletableFuture<CatalogResponse> fetch(Long id) { ... }

    private CompletableFuture<CatalogResponse> fallback(Long id, Throwable t) {
        return CompletableFuture.completedFuture(CatalogResponse.unknown());
    }
}
```

```yaml
resilience4j:
  circuitbreaker:
    instances:
      catalog:
        failure-rate-threshold: 50
        wait-duration-in-open-state: 10s
        sliding-window-size: 20
```

### Production launch checklist

```
[ ] Profiles: SPRING_PROFILES_ACTIVE=prod set explicitly
[ ] No secrets in YAML — all in env vars / secret manager
[ ] ddl-auto: validate (not update/create)
[ ] Flyway/Liquibase migration job runs before app rollout
[ ] /actuator/health configured with proper probes
[ ] /actuator endpoints not all exposed publicly (env, heapdump, threaddump in particular)
[ ] Logging in JSON with trace/span IDs
[ ] Metrics scraped by Prometheus / Datadog / etc.
[ ] HTTP client timeouts set
[ ] DB connection pool tuned
[ ] Graceful shutdown enabled
[ ] Kubernetes resource requests/limits set
[ ] preStop hook + terminationGracePeriodSeconds align with server.shutdown
[ ] Image scanned for CVEs (Trivy, Snyk)
[ ] CI runs unit + slice + integration tests + native image build (if used)
[ ] Rollback plan: previous image tag pinned, migration is forward-compatible
[ ] Runbook for top 3 alerts on-call will see
```

---

## 5. Common Mistakes & Gotchas

- **`SPRING_PROFILES_ACTIVE` not set in prod.** App boots with default config (often dev DB URLs). Catastrophic. Make it required.

- **No graceful shutdown.** Pod terminated mid-request → client sees 502s. Always enable + align with K8s terminationGracePeriodSeconds.

- **Heap fills the container memory limit, OOMKilled.** JVM heap + metaspace + native + thread stacks must fit in the container limit. Use `-XX:MaxRAMPercentage=75` and leave headroom.

- **Migrations run by every replica.** Race conditions, partial migrations. Run as a pre-deploy job; replicas come up after migration is done.

- **Connection pool too small.** Threads wait for connections, latency spikes, no obvious error. Watch `hikaricp.connections.pending` metric.

- **Connection pool too big.** DB CPU saturates from connection overhead. The right size is usually smaller than you think.

- **Logs as plain text in JSON pipeline.** Your log aggregator misparses. Always JSON in production environments.

- **Exposing all Actuator endpoints publicly.** `/actuator/env` leaks config including hostnames and secret keys. Restrict via `include` whitelist + auth + separate management port.

- **No fallbacks for external dependencies.** Upstream slow → your service slow → upstream of you slow. Set timeouts, add circuit breakers.

- **Manual SSH-and-fix in production.** No reproducibility. Every change goes through CI/CD or it's a bug waiting to happen.

- **Skipping native image testing.** Native works on dev box, breaks in container due to missing reflection metadata. Add a native build to CI even if you ship JVM.

- **Single replica in production.** No tolerance for restarts, deploys, or node failures. Minimum 2 replicas + readiness probe + load balancer.

---

## 🎯 Key Takeaways

- **Layered jars + Cloud Native Buildpacks** are the modern Spring Boot deployment. No hand-written Dockerfile needed.
- **Native image is a tool, not a default.** Use when startup/memory beats throughput.
- **Graceful shutdown + readiness probes + termination grace** is the trifecta for zero-downtime deploys.
- **Timeouts + connection pool + circuit breakers** prevent cascading failures.
- **Observability + structured logs + runbooks** are what separate hobby-grade from production-grade.

You've finished the deep-dive. From here, the work is *applying* it — build the bookstore-api, ship it, observe it under load, fix what breaks. Each module's gotcha section is a real bug from somebody's postmortem. Read them again when you're tired and shipping at 11pm; they'll save you.

*[← prev](./16_messaging_integration.md) | [back to roadmap →](./00_roadmap.md)*
