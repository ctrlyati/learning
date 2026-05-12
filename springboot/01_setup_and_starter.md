# 01 — Setup & First Starter

> **Goal:** Install the toolchain, generate a Spring Boot 3.x project, run it, and understand every file the Initializr created.

---

## 1. The Toolchain — mental model + immediate working code

A Spring Boot project needs **three** things on your machine: a **JDK**, a **build tool** (Maven or Gradle), and a way to **scaffold** the project (Spring Initializr — web UI or CLI). Everything else is a library on the classpath.

### Mental model

```
[Your code]  ──┐
[Spring libs] ─┼──► Build tool ──► fat jar ──► java -jar app.jar ──► embedded Tomcat on :8080
[Auto-config] ─┘
```

The "server" is not separately installed. The build packages an **embedded Tomcat** (or Jetty/Undertow/Netty) into your jar.

### Install (Windows, macOS, Linux — pick one path)

**JDK 17+** (LTS — pick 17 or 21):
- Windows: `winget install EclipseAdoptium.Temurin.21.JDK`
- macOS: `brew install --cask temurin`
- Linux (Debian/Ubuntu): `sudo apt install openjdk-21-jdk`

Verify:
```bash
java -version
# openjdk version "21.0.x" 2024-xx-xx LTS
```

**Maven** (bundled wrapper is fine — see below), or **Gradle** (also bundled).

You almost never install Maven/Gradle globally — the Initializr ships a wrapper (`./mvnw`, `./gradlew`) that downloads the correct version on first run. Use the wrapper. Always.

---

## 2. Generating the project — what each Initializr knob does

Open [start.spring.io](https://start.spring.io) or use the CLI:

```bash
# CLI alternative (curl works on every OS)
curl https://start.spring.io/starter.zip \
  -d type=maven-project \
  -d language=java \
  -d bootVersion=3.3.0 \
  -d groupId=com.example \
  -d artifactId=bookstore-api \
  -d name=bookstore-api \
  -d packageName=com.example.bookstore \
  -d javaVersion=21 \
  -d dependencies=web,actuator \
  -o bookstore-api.zip

unzip bookstore-api.zip -d bookstore-api
cd bookstore-api
```

### Knob-by-knob

| Knob              | What it controls                                                   |
| ----------------- | ------------------------------------------------------------------ |
| **Project**       | Maven or Gradle. Pick Maven if your team uses it; Gradle is faster |
| **Language**      | Java / Kotlin / Groovy. This course uses Java                      |
| **Spring Boot**   | Pin to a current GA release (3.3.x as of writing)                  |
| **Group**         | Reverse-domain package root, e.g. `com.example`                    |
| **Artifact**      | Project / jar name                                                 |
| **Packaging**     | Jar (almost always) vs War (legacy app servers only)               |
| **Java**          | Match your installed JDK. 17 or 21                                 |
| **Dependencies**  | These become starters in `pom.xml`/`build.gradle`                  |

---

## 3. Project layout — every file explained

```
bookstore-api/
├── .mvn/wrapper/              # Maven wrapper jar + properties
├── mvnw, mvnw.cmd             # Maven wrapper scripts (use these, not system mvn)
├── pom.xml                    # Build file (or build.gradle.kts for Gradle)
├── src/
│   ├── main/
│   │   ├── java/com/example/bookstore/
│   │   │   └── BookstoreApiApplication.java   # @SpringBootApplication entry point
│   │   └── resources/
│   │       ├── application.properties         # Configuration
│   │       ├── static/                        # Served as /** (CSS, JS, images)
│   │       └── templates/                     # Server-side templates (Thymeleaf, etc.)
│   └── test/
│       └── java/com/example/bookstore/
│           └── BookstoreApiApplicationTests.java
└── HELP.md
```

### `pom.xml` — Maven dependency snippet

```xml
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <parent>
        <groupId>org.springframework.boot</groupId>
        <artifactId>spring-boot-starter-parent</artifactId>
        <version>3.3.0</version>
        <relativePath/>
    </parent>

    <groupId>com.example</groupId>
    <artifactId>bookstore-api</artifactId>
    <version>0.0.1-SNAPSHOT</version>

    <properties>
        <java.version>21</java.version>
    </properties>

    <dependencies>
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-web</artifactId>
        </dependency>
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-actuator</artifactId>
        </dependency>
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-test</artifactId>
            <scope>test</scope>
        </dependency>
    </dependencies>

    <build>
        <plugins>
            <plugin>
                <groupId>org.springframework.boot</groupId>
                <artifactId>spring-boot-maven-plugin</artifactId>
            </plugin>
        </plugins>
    </build>
</project>
```

### `build.gradle.kts` — same project, Gradle Kotlin DSL

```kotlin
plugins {
    java
    id("org.springframework.boot") version "3.3.0"
    id("io.spring.dependency-management") version "1.1.5"
}

group = "com.example"
version = "0.0.1-SNAPSHOT"
java.toolchain.languageVersion = JavaLanguageVersion.of(21)

repositories { mavenCentral() }

dependencies {
    implementation("org.springframework.boot:spring-boot-starter-web")
    implementation("org.springframework.boot:spring-boot-starter-actuator")
    testImplementation("org.springframework.boot:spring-boot-starter-test")
}

tasks.withType<Test> { useJUnitPlatform() }
```

### The entry point

```java
package com.example.bookstore;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class BookstoreApiApplication {
    public static void main(String[] args) {
        SpringApplication.run(BookstoreApiApplication.class, args);
    }
}
```

That's it. Eight lines of code start an HTTP server, register thousands of beans, expose `/actuator/health`, and bind to port 8080.

---

## 4. Practical application — your first endpoint

Add a controller next to `BookstoreApiApplication.java`:

```java
package com.example.bookstore;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

import java.time.Instant;
import java.util.Map;

@RestController
public class HelloController {

    @GetMapping("/hello")
    public Map<String, Object> hello() {
        return Map.of(
            "message", "Hello, Spring Boot 3!",
            "timestamp", Instant.now().toString()
        );
    }
}
```

Run it:

```bash
# Maven
./mvnw spring-boot:run

# Gradle
./gradlew bootRun
```

Hit it:

```bash
curl http://localhost:8080/hello
# {"message":"Hello, Spring Boot 3!","timestamp":"2026-05-13T10:00:00Z"}

curl http://localhost:8080/actuator/health
# {"status":"UP"}
```

### Package layout for a real project

Don't dump everything in the root package. Use **feature packages**, not layer packages:

```
com.example.bookstore/
├── BookstoreApiApplication.java
├── book/
│   ├── Book.java
│   ├── BookRepository.java
│   ├── BookService.java
│   └── BookController.java
├── author/
│   ├── Author.java
│   └── ...
└── config/
    └── WebConfig.java
```

Why? Features change together. Layers don't. Feature packages also keep package-private classes properly scoped — `BookRepository` shouldn't be visible to `author/`.

---

## 5. Common Mistakes & Gotchas

- **Using system `mvn` instead of `./mvnw`.** The wrapper pins the Maven version. Skip it and you get "works on my machine" bugs the moment a teammate has Maven 3.6 vs your 3.9.

- **Putting `@SpringBootApplication` in the wrong package.** Component scanning starts from the package of the `@SpringBootApplication` class and goes *down*. Beans in sibling packages aren't found. Always keep the entry class at the root of your code package.

- **Mixing Boot starter versions manually.** The `spring-boot-starter-parent` BOM manages every Spring/Jackson/Hibernate version transitively. Override one and you get classpath conflicts. If you must override, do it in `<properties>` (e.g. `<jackson.version>`) so the BOM picks it up.

- **Java version mismatch.** `pom.xml` says Java 21 but `JAVA_HOME` points to 17. Result: `UnsupportedClassVersionError` at startup or, worse, compile-time success and weird runtime errors. Run `./mvnw -v` to see which JDK the build is using.

- **Forgetting that `application.properties` lives in `src/main/resources`.** Putting it in `src/main/java` means it's not on the classpath and Spring silently uses defaults.

- **Building a war when you wanted a jar.** Spring Boot 3 strongly prefers jar packaging with embedded server. Only use war if you're deploying into an existing Tomcat/WildFly (which you almost certainly shouldn't be in 2026).

---

## 🎯 Key Takeaways

- **Use the wrapper (`./mvnw`, `./gradlew`).** Reproducible builds across machines and CI is non-negotiable in professional work.
- **Initializr is the only sane way to start a new project.** Manual `pom.xml` editing comes later when you understand the BOM.
- **`@SpringBootApplication` is a meta-annotation** combining `@Configuration`, `@EnableAutoConfiguration`, and `@ComponentScan`. Memorize that — it's the most asked Boot interview question.
- **Embedded server, fat jar, single `java -jar` command.** This is Spring Boot's whole value prop over classic Spring + Tomcat deployments.
- **Feature-packaged code, not layer-packaged.** It's how real teams ship.

*[← prev](./00_roadmap.md) | [next →](./02_fundamentals.md)*
