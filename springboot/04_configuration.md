# 04 — Configuration

> **Goal:** Master the property hierarchy, profiles, `@ConfigurationProperties` type-safe binding, and how to override config per environment without touching code.

---

## 1. `application.properties` / `application.yml` — mental model + code

Spring Boot has one canonical config file: `src/main/resources/application.properties` or `application.yml`. Pick one (most teams pick YAML for nesting).

### Properties form

```properties
server.port=8080
spring.application.name=bookstore-api
spring.datasource.url=jdbc:postgresql://localhost:5432/bookstore
spring.datasource.username=app
spring.datasource.password=secret

bookstore.feature.recommendations.enabled=true
bookstore.feature.recommendations.max-results=10
```

### YAML form (same thing)

```yaml
server:
  port: 8080
spring:
  application:
    name: bookstore-api
  datasource:
    url: jdbc:postgresql://localhost:5432/bookstore
    username: app
    password: secret

bookstore:
  feature:
    recommendations:
      enabled: true
      max-results: 10
```

Spring binds `bookstore.feature.recommendations.max-results` → Java property `maxResults` via **relaxed binding**:

| YAML / Properties        | Java property name |
| ------------------------ | ------------------ |
| `max-results` (kebab)    | `maxResults`       |
| `max_results` (snake)    | `maxResults`       |
| `MAX_RESULTS` (env var)  | `maxResults`       |
| `maxResults` (camel)     | `maxResults`       |

---

## 2. The property hierarchy — what Spring does

Spring layers property sources in **priority order** (highest wins):

1. Command-line args (`--server.port=9090`)
2. JVM system properties (`-Dserver.port=9090`)
3. OS environment variables (`SERVER_PORT=9090`)
4. `application-{profile}.yml` (profile-specific)
5. `application.yml` (default)
6. `@PropertySource` annotations
7. Defaults (e.g. `server.port=8080`)

**This is the entire mental model of 12-factor config in Spring.** You write defaults in `application.yml`, override per env via env vars in production. No code changes.

### Inspecting

```bash
curl http://localhost:8080/actuator/env | jq
```

Shows every property source and the resolved values (with secrets masked).

### `@Value` — single property injection

```java
package com.example.bookstore.book;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

@Service
public class RecommendationService {

    private final int maxResults;

    public RecommendationService(
            @Value("${bookstore.feature.recommendations.max-results:5}") int maxResults) {
        this.maxResults = maxResults;
    }
}
```

The `:5` after the colon is the **default** if the property is missing.

`@Value` is fine for one or two values. For groups of related config, use `@ConfigurationProperties` (next).

---

## 3. `@ConfigurationProperties` — type-safe binding

The professional way to handle config: a Java record/class binds to a tree of properties.

```java
package com.example.bookstore.config;

import org.springframework.boot.context.properties.ConfigurationProperties;

@ConfigurationProperties(prefix = "bookstore.feature.recommendations")
public record RecommendationProperties(
        boolean enabled,
        int maxResults,
        Algorithm algorithm
) {
    public enum Algorithm { COLLABORATIVE, CONTENT, HYBRID }
}
```

Enable it (in any `@Configuration` class, or on the main class):

```java
@SpringBootApplication
@ConfigurationPropertiesScan
public class BookstoreApiApplication { ... }
```

Now inject it like any bean:

```java
@Service
public class RecommendationService {

    private final RecommendationProperties props;

    public RecommendationService(RecommendationProperties props) {
        this.props = props;
    }
}
```

YAML:

```yaml
bookstore:
  feature:
    recommendations:
      enabled: true
      max-results: 20
      algorithm: HYBRID
```

### Validation on config

```java
import jakarta.validation.constraints.*;
import org.springframework.validation.annotation.Validated;

@Validated
@ConfigurationProperties(prefix = "bookstore.feature.recommendations")
public record RecommendationProperties(
        boolean enabled,
        @Min(1) @Max(100) int maxResults,
        @NotNull Algorithm algorithm
) { ... }
```

App fails to start if config is invalid. **Use this aggressively** — typos in YAML are otherwise silent.

---

## 4. Profiles — environment-specific configuration

Profiles let you ship one jar that behaves differently in dev/staging/prod.

### File structure

```
src/main/resources/
├── application.yml          # defaults, common to all envs
├── application-dev.yml      # local development overrides
├── application-staging.yml
├── application-prod.yml
```

### `application.yml` (defaults)

```yaml
spring:
  application:
    name: bookstore-api

server:
  port: 8080

bookstore:
  feature:
    recommendations:
      enabled: false
```

### `application-dev.yml`

```yaml
spring:
  datasource:
    url: jdbc:h2:mem:dev
    username: sa
    password:

bookstore:
  feature:
    recommendations:
      enabled: true
      max-results: 5

logging:
  level:
    com.example.bookstore: DEBUG
```

### `application-prod.yml`

```yaml
spring:
  datasource:
    url: ${DATABASE_URL}
    username: ${DATABASE_USER}
    password: ${DATABASE_PASSWORD}

bookstore:
  feature:
    recommendations:
      enabled: true
      max-results: 25
```

### Activating a profile

- Env var: `SPRING_PROFILES_ACTIVE=prod`
- Command line: `--spring.profiles.active=prod`
- In code (rare): `app.setAdditionalProfiles("prod")`
- Multiple: `SPRING_PROFILES_ACTIVE=prod,eu-west`

### Profile-scoped beans

```java
@Configuration
@Profile("dev")
public class DevDataConfig {
    @Bean
    public DataSeeder devSeeder() { return new InMemorySeeder(); }
}

@Configuration
@Profile("prod")
public class ProdDataConfig {
    @Bean
    public DataSeeder prodSeeder() { return new NoopSeeder(); }
}
```

Only the active profile's beans get registered.

### Practical application — the bookstore config layer

```java
// 1. The config record
package com.example.bookstore.config;

import jakarta.validation.constraints.*;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.validation.annotation.Validated;

@Validated
@ConfigurationProperties(prefix = "bookstore")
public record BookstoreProperties(
        @NotBlank String tenantId,
        Feature feature,
        Upstream upstream
) {
    public record Feature(
            boolean recommendations,
            @Min(1) int defaultPageSize
    ) {}
    public record Upstream(
            @NotBlank String catalogUrl,
            @NotNull java.time.Duration timeout
    ) {}
}
```

```java
// 2. Enable scanning
@SpringBootApplication
@ConfigurationPropertiesScan
public class BookstoreApiApplication { ... }
```

```yaml
# 3. application.yml
bookstore:
  tenant-id: acme-corp
  feature:
    recommendations: true
    default-page-size: 20
  upstream:
    catalog-url: https://catalog.example.com
    timeout: 5s
```

```java
// 4. Use it
@RestController
public class HealthCheckController {
    private final BookstoreProperties props;

    public HealthCheckController(BookstoreProperties props) {
        this.props = props;
    }

    @GetMapping("/info")
    public Map<String, Object> info() {
        return Map.of(
            "tenant", props.tenantId(),
            "upstream", props.upstream().catalogUrl()
        );
    }
}
```

### Externalized config in production

In production, **never bake secrets into YAML**. Pass them via env vars:

```bash
export SPRING_PROFILES_ACTIVE=prod
export DATABASE_URL="jdbc:postgresql://..."
export DATABASE_PASSWORD="$(vault read -field=password secret/bookstore/db)"
java -jar bookstore-api.jar
```

The `${DATABASE_URL}` placeholder in `application-prod.yml` resolves from the env var via the property hierarchy.

---

## 5. Common Mistakes & Gotchas

- **Putting secrets in `application.yml` and committing them.** Use env vars (12-factor) or a secret manager (Vault, AWS Secrets Manager, Spring Cloud Config). Don't lecture yourself with `.gitignore` — extract them entirely.

- **Using `@Value` for groups of related properties.** It scales poorly. Five `@Value` annotations on one bean = refactor to `@ConfigurationProperties`.

- **`@ConfigurationProperties` without `@ConfigurationPropertiesScan` or `@EnableConfigurationProperties`.** Binding silently doesn't happen; you get `null`/defaults. Always enable one or the other.

- **Profile-specific files but wrong activation.** `application-dev.yml` only loads when `dev` profile is active. Set `SPRING_PROFILES_ACTIVE=dev` (env) or add to `application.yml`: `spring.profiles.active: dev` for local dev.

- **Relying on profile defaults at deploy time.** Production must explicitly set `SPRING_PROFILES_ACTIVE=prod`. Otherwise a dev DB URL ships to prod. Make this part of the deployment checklist.

- **Forgetting relaxed binding rules.** `BOOKSTORE_FEATURE_RECOMMENDATIONS_ENABLED=true` as an env var maps to `bookstore.feature.recommendations.enabled`. Underscores ↔ dots, uppercase ↔ kebab-case. Don't fight it.

- **Mutating `@ConfigurationProperties` at runtime.** Records are immutable (good). If you use a class with setters, do not change values after startup — beans hold the old reference.

---

## 🎯 Key Takeaways

- **`@ConfigurationProperties` over scattered `@Value`** for any non-trivial config. Type-safe, validated, IDE-completable.
- **Profiles are environment levers.** One jar, many configurations, swap by env var.
- **Property hierarchy is the 12-factor backbone.** Defaults in code, overrides in env. Memorize the priority order.
- **Validate config with `@Validated`.** Startup is the right place for a typo to fail loudly.
- **Secrets never live in YAML.** Externalize via env vars or a secret backend.

*[← prev](./03_dependency_injection.md) | [next →](./05_web_layer.md)*
