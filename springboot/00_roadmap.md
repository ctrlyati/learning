# 00 — Spring Boot Deep-Dive Roadmap

> **Goal:** Take a working Java developer from "I can write a class" to "I can design, build, secure, test, observe, and ship a production-grade Spring Boot service" — through 17 hands-on modules.

---

## Who this is for

You write Java. You know what a class, interface, generic, and stream is. You've used HTTP. You may have touched Spring once and walked away confused by the magic. This course turns the magic into mechanics.

**Prerequisites:**
- **Java 17+** working knowledge (records, sealed types, `var`, lambdas, streams)
- Basic OOP (classes, interfaces, generics)
- HTTP fundamentals (verbs, status codes, headers, JSON)
- Optional but recommended: **Docker** → see [../docker/00_roadmap.md](../docker/00_roadmap.md)
- Optional but recommended: **MySQL/SQL** → see [../mysql/00_roadmap.md](../mysql/00_roadmap.md)

If any of the above is shaky, fix that first. Spring Boot multiplies your Java knowledge; it doesn't replace it.

---

## The module table

| #  | Module                                | Focus                                             |
| -- | ------------------------------------- | ------------------------------------------------- |
| 01 | Setup & first starter                 | JDK, Maven/Gradle, Initializr, project layout     |
| 02 | Spring Boot fundamentals              | Auto-config, starters, `@SpringBootApplication`   |
| 03 | Dependency Injection & IoC            | Beans, scopes, constructor injection              |
| 04 | Configuration                         | `application.yml`, profiles, `@ConfigurationProperties` |
| 05 | Web layer                             | `@RestController`, mapping, content negotiation   |
| 06 | Validation & error handling           | `@Valid`, `@ControllerAdvice`, ProblemDetail      |
| 07 | Spring Data JPA                       | Entities, repos, JPQL, pagination                 |
| 08 | Transactions                          | `@Transactional`, propagation, isolation          |
| 09 | Database migrations                   | Flyway, Liquibase, environment-aware migrations   |
| 10 | Spring Security                       | Filter chain, JWT, OAuth2 resource server         |
| 11 | Testing                               | Slices, Testcontainers, Mockito                   |
| 12 | Async, scheduling, events             | `@Async`, `@Scheduled`, `ApplicationEvents`       |
| 13 | Caching                               | `@Cacheable`, Redis, Caffeine, TTL                |
| 14 | Observability                         | Actuator, Micrometer, tracing                     |
| 15 | Building APIs                         | REST best practices, OpenAPI, HATEOAS, GraphQL    |
| 16 | Messaging & integration               | Kafka, RabbitMQ, Spring Integration               |
| 17 | Production                            | Native image, Docker, externalized config         |

---

## Suggested timeline

**One module per day = ~2.5 weeks** to ship-ready. Stretch to one module every two days if you also want to build the running side-project (a small `bookstore-api` referenced throughout).

- **Week 1 (modules 1–5):** "I can build and serve JSON"
- **Week 2 (modules 6–11):** "I can persist, secure, and test"
- **Week 3 (modules 12–17):** "I can run this in production"

---

## Core mental models

These are the levers. Internalize these six and the rest of Spring stops feeling like magic.

1. **Convention over configuration.** Spring Boot has opinions. Follow them and 80% of config writes itself. Fight them and you'll re-implement what `spring-boot-starter-web` already gave you.

2. **The application context is a graph of beans.** At startup, Spring scans, instantiates, and wires a dependency graph in memory. Every `@Autowired` is a node lookup. Every circular dependency is a graph cycle.

3. **Spring is mostly an annotation processor at startup.** `@RestController`, `@Service`, `@Transactional`, `@Cacheable` — none of these are runtime magic. They produce bean definitions, proxies, and aspect interceptors *before* your first request.

4. **Proxies are how AOP works.** `@Transactional` and `@Cacheable` wrap your bean in a proxy. The proxy intercepts external calls and adds the cross-cutting behavior. **Self-calls (`this.foo()`) bypass the proxy** — this is the source of countless production bugs.

5. **Profiles are the right knob for environment differences.** Not `if (env.equals("prod"))` scattered through code. Activate `application-prod.yml` with `SPRING_PROFILES_ACTIVE=prod` and let Spring assemble the right context.

6. **Auto-configuration is conditional bean wiring.** `spring-boot-autoconfigure` ships hundreds of `@Configuration` classes guarded by `@ConditionalOnClass`, `@ConditionalOnMissingBean`, etc. Add a starter → those conditions flip → beans appear. You can always override by declaring your own bean.

---

## External references

- **[spring.io/guides](https://spring.io/guides)** — short, focused, official walkthroughs
- **[Spring Boot reference documentation](https://docs.spring.io/spring-boot/docs/current/reference/htmlsingle/)** — the source of truth; read sections, not the whole thing
- **[Baeldung Spring tutorials](https://www.baeldung.com/spring-boot)** — pragmatic recipes, kept current
- **["Spring in Action" by Craig Walls](https://www.manning.com/books/spring-in-action-sixth-edition)** — the canonical book; 6th edition covers Boot 3.x
- **[Spring Academy](https://spring.academy/)** — free official courses with hands-on labs
- **[Spring Boot 3 Release Notes](https://github.com/spring-projects/spring-boot/wiki)** — track breaking changes (Jakarta EE namespace, native image, etc.)

---

## Spring Boot 3.x specifics you'll meet

- **`jakarta.*` namespace** replaces `javax.*` (Servlet API, Persistence API, Validation)
- **Java 17 baseline** — no more Java 8 patterns
- **GraalVM native image** is a first-class citizen via `spring-boot-starter-parent` 3.x
- **Observability** built on Micrometer 1.10+ and Micrometer Tracing (replaces Spring Cloud Sleuth)
- **ProblemDetail (RFC 7807)** is the default error response shape

---

## Closing line

Professional Spring Boot work isn't memorizing annotations — it's knowing which lever to pull when. Each module gives you one lever and the failure modes that ship with it. Finish all 17 and you can walk into any Java backend team and contribute on day one.

*[next → 01 — Setup & First Starter](./01_setup_and_starter.md)*
