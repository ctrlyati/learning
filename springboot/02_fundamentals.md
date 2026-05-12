# 02 — Spring Boot Fundamentals

> **Goal:** Understand what `@SpringBootApplication` actually does, how auto-configuration works, what a "starter" really is, and why the embedded server matters.

---

## 1. `@SpringBootApplication` — mental model + working code

This single annotation is a **meta-annotation**:

```java
@Target(ElementType.TYPE)
@Retention(RetentionPolicy.RUNTIME)
@SpringBootConfiguration
@EnableAutoConfiguration
@ComponentScan(...)
public @interface SpringBootApplication { ... }
```

It expands to three things:

| Sub-annotation              | Effect                                                                |
| --------------------------- | --------------------------------------------------------------------- |
| `@SpringBootConfiguration`  | Marks the class as a source of bean definitions (a `@Configuration`) |
| `@EnableAutoConfiguration`  | Triggers conditional auto-config scanning                             |
| `@ComponentScan`            | Scans this package + subpackages for `@Component`-annotated classes   |

### Equivalent, expanded form

```java
package com.example.bookstore;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.SpringBootConfiguration;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.context.annotation.ComponentScan;

@SpringBootConfiguration
@EnableAutoConfiguration
@ComponentScan
public class BookstoreApiApplication {
    public static void main(String[] args) {
        SpringApplication.run(BookstoreApiApplication.class, args);
    }
}
```

Identical behavior. Don't write it like this; the point is to know what's *inside* the umbrella.

---

## 2. Auto-configuration — what Spring does behind the annotation

When you add `spring-boot-starter-web`, you get **Tomcat + Spring MVC + Jackson** without writing a single config line. How?

### The mechanism

1. **`spring-boot-autoconfigure.jar`** ships hundreds of `@AutoConfiguration` classes.
2. Each is **conditional**:
   - `@ConditionalOnClass(DispatcherServlet.class)` — only if Spring MVC is on the classpath
   - `@ConditionalOnMissingBean(DataSource.class)` — only if you haven't defined your own
   - `@ConditionalOnProperty("spring.datasource.url")` — only if a property exists
3. Spring registers each auto-config class' beans **only when conditions pass**.
4. Your own beans always **win** over auto-configured ones (`@ConditionalOnMissingBean` is the polite default).

### Inspecting it

Add this to `application.properties`:

```properties
logging.level.org.springframework.boot.autoconfigure=DEBUG
```

Or hit Actuator (after enabling it):

```bash
curl http://localhost:8080/actuator/conditions | jq
```

You get a **positive matches** report (auto-configs that activated) and **negative matches** (auto-configs that didn't, with the reason).

### A handwritten auto-config class

```java
package com.example.bookstore.config;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnClass;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.context.annotation.Bean;

@AutoConfiguration
@ConditionalOnClass(name = "com.example.bookstore.book.BookService")
public class BookAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public BookGreeter bookGreeter() {
        return new BookGreeter("Welcome to the bookstore");
    }
}
```

Register it in `META-INF/spring/org.springframework.boot.autoconfigure.AutoConfiguration.imports` (Spring Boot 3 style):

```
com.example.bookstore.config.BookAutoConfiguration
```

This is **how every starter works internally**.

---

## 3. Starters — depth

A "starter" is just a Maven/Gradle dependency that pulls a curated set of transitive dependencies. **The starter jar itself often contains no code** — only a `pom.xml`.

### Examples

| Starter                              | What you get                                       |
| ------------------------------------ | -------------------------------------------------- |
| `spring-boot-starter-web`            | Spring MVC, Tomcat, Jackson, validation            |
| `spring-boot-starter-data-jpa`       | Hibernate, JPA, transactions, JDBC                 |
| `spring-boot-starter-security`       | Spring Security core + config                      |
| `spring-boot-starter-actuator`       | Production endpoints (health, metrics, info)       |
| `spring-boot-starter-test`           | JUnit 5, Mockito, AssertJ, Spring Test, JsonPath   |
| `spring-boot-starter-webflux`        | Reactive web (Netty + Reactor) — alternative to MVC |

### Maven snippet

```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-data-jpa</artifactId>
</dependency>
```

### Gradle snippet

```kotlin
implementation("org.springframework.boot:spring-boot-starter-data-jpa")
```

No version. The `spring-boot-starter-parent` BOM resolves it.

### The reactive alternative

Spring Boot supports two web stacks:

- **Spring MVC** (Servlet-based, blocking, Tomcat) — `spring-boot-starter-web`
- **Spring WebFlux** (reactive, non-blocking, Netty) — `spring-boot-starter-webflux`

**Don't mix them in the same module.** Pick one. This course uses MVC throughout because it's still 90% of the market.

---

## 4. Practical application — full startup trace + custom banner

Let's watch what happens during startup. Run the app with `--debug`:

```bash
./mvnw spring-boot:run -Dspring-boot.run.arguments="--debug"
```

You'll see ~3000 lines of `CONDITIONS EVALUATION REPORT`. Search for `WebMvcAutoConfiguration` — it should be in **Positive matches**. Search for `WebFluxAutoConfiguration` — **Negative matches** (`@ConditionalOnClass` not met).

### Customizing the startup

```java
package com.example.bookstore;

import org.springframework.boot.Banner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class BookstoreApiApplication {
    public static void main(String[] args) {
        SpringApplication app = new SpringApplication(BookstoreApiApplication.class);
        app.setBannerMode(Banner.Mode.OFF);     // quiet start
        app.setAdditionalProfiles("dev");        // force dev profile
        app.run(args);
    }
}
```

### A `CommandLineRunner` for one-off startup work

```java
package com.example.bookstore;

import org.springframework.boot.CommandLineRunner;
import org.springframework.stereotype.Component;

@Component
public class StartupSeeder implements CommandLineRunner {

    @Override
    public void run(String... args) {
        System.out.println("Bookstore is alive. Seeding initial data...");
        // call services, prime caches, etc.
    }
}
```

Spring runs every `CommandLineRunner` after the context is fully started, **before** accepting HTTP traffic. Useful for data seeding, warm-up, validation that infrastructure is reachable.

### The embedded server

```java
package com.example.bookstore.config;

import org.apache.catalina.connector.Connector;
import org.springframework.boot.web.embedded.tomcat.TomcatServletWebServerFactory;
import org.springframework.boot.web.server.WebServerFactoryCustomizer;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class TomcatConfig {

    @Bean
    public WebServerFactoryCustomizer<TomcatServletWebServerFactory> tomcatCustomizer() {
        return factory -> {
            factory.setPort(8080);
            factory.addConnectorCustomizers(connector ->
                connector.setAttribute("maxThreads", 200)
            );
        };
    }
}
```

Tomcat is bundled. You can swap to Jetty:

```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-web</artifactId>
    <exclusions>
        <exclusion>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-tomcat</artifactId>
        </exclusion>
    </exclusions>
</dependency>
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-jetty</artifactId>
</dependency>
```

---

## 5. Common Mistakes & Gotchas

- **Defining a `DataSource` bean and wondering why your `spring.datasource.url` is ignored.** Your bean wins, the auto-config doesn't run, properties from `application.yml` aren't bound. Either accept the auto-configured bean *or* fully configure yours.

- **Placing `@SpringBootApplication` in a non-root package.** Component scanning starts *here* and goes *down*. `com.example.bookstore.app.AppMain` won't find `@Service` beans in `com.example.bookstore.book`.

- **Mixing `spring-boot-starter-web` and `spring-boot-starter-webflux` in one module.** Spring tries to start both stacks; you get bizarre errors. Pick one.

- **Assuming `@SpringBootApplication` auto-scans every jar.** It only scans the **base package**. Library beans need either `@ComponentScan(basePackages=...)` or an auto-configuration entry in `META-INF/spring/...`.

- **Disabling all auto-configs to "speed up startup".** Almost always premature. Use Actuator `/conditions` to see what's running; exclude individually with `@SpringBootApplication(exclude = JpaAutoConfiguration.class)`.

- **Forgetting Boot 3 → Jakarta migration.** Old tutorials show `javax.servlet.http.HttpServletRequest`. Spring Boot 3 is `jakarta.servlet.http.HttpServletRequest`. Copy-pasted code from 2020 will fail to compile.

---

## 🎯 Key Takeaways

- **`@SpringBootApplication` = `@SpringBootConfiguration` + `@EnableAutoConfiguration` + `@ComponentScan`.** Know the expansion cold.
- **Auto-configuration is just conditional bean wiring.** It's not magic — it's `@ConditionalOnClass`, `@ConditionalOnMissingBean`, and a registry file.
- **Starters are dependency bundles, not code.** They exist to manage transitive deps and pin compatible versions.
- **Embedded server in a fat jar** is *the* deployment unit. One artifact, one command, one port.
- **`spring.factories` is gone in Boot 3** — replaced by `META-INF/spring/org.springframework.boot.autoconfigure.AutoConfiguration.imports`. Old library code may need updating.

*[← prev](./01_setup_and_starter.md) | [next →](./03_dependency_injection.md)*
