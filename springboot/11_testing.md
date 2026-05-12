# 11 — Testing

> **Goal:** Test fast at the unit level with Mockito, narrow at the slice level with `@WebMvcTest`/`@DataJpaTest`, and full-stack with `@SpringBootTest` + Testcontainers — the pro testing pyramid for Spring Boot.

---

## 1. Unit tests with Mockito — mental model + working code

The cheapest test: instantiate the class under test, inject mocks for its collaborators, exercise the method, assert.

`spring-boot-starter-test` already pulls in JUnit 5, Mockito, AssertJ, JsonPath, Hamcrest, Spring Test.

```java
package com.example.bookstore.book;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.*;
import org.mockito.junit.jupiter.MockitoExtension;

import java.math.BigDecimal;
import java.util.Optional;

import static org.assertj.core.api.Assertions.*;
import static org.mockito.Mockito.*;

@ExtendWith(MockitoExtension.class)
class BookServiceTest {

    @Mock BookRepository repo;
    @Mock BookNotifier notifier;
    @InjectMocks BookService service;

    @Test
    void create_persistsAndNotifies() {
        when(repo.save(any())).thenAnswer(inv -> {
            Book b = inv.getArgument(0);
            return new Book(1L, b.getTitle(), b.getAuthor(), b.getPrice());
        });

        BookDto result = service.create(new CreateBookRequest("Effective Java", "Bloch", new BigDecimal("49.99")));

        assertThat(result.id()).isEqualTo(1L);
        verify(notifier).notifyCreated(any(Book.class));
    }

    @Test
    void getOne_throwsWhenMissing() {
        when(repo.findById(99L)).thenReturn(Optional.empty());

        assertThatThrownBy(() -> service.getOne(99L))
            .isInstanceOf(BookNotFoundException.class);
    }
}
```

No Spring context. Milliseconds per test. Run hundreds of these.

---

## 2. Slice tests — what Spring loads vs ignores

Slice tests load *just enough* Spring to test one layer.

| Slice annotation       | Loads                                              |
| ---------------------- | -------------------------------------------------- |
| `@WebMvcTest`          | Controllers + `MockMvc` + filters + `ControllerAdvice` |
| `@DataJpaTest`         | JPA repositories + entity manager + in-memory DB   |
| `@JsonTest`            | Jackson serializers/deserializers                  |
| `@RestClientTest`      | `RestTemplate` / `RestClient` with `MockRestServiceServer` |
| `@WebFluxTest`         | Reactive web controllers                           |
| `@JdbcTest`            | `JdbcTemplate` and embedded DB                     |

### `@WebMvcTest`

```java
package com.example.bookstore.book;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.mock.mockito.MockBean;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;

import static org.mockito.Mockito.*;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.*;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.*;

@WebMvcTest(BookController.class)
class BookControllerTest {

    @Autowired MockMvc mvc;
    @MockBean BookService service;

    @Test
    void getOne_returnsBook() throws Exception {
        when(service.getOne(1L)).thenReturn(new BookDto(1L, "Effective Java", "Bloch", new java.math.BigDecimal("49.99")));

        mvc.perform(get("/api/v1/books/1"))
            .andExpect(status().isOk())
            .andExpect(content().contentTypeCompatibleWith(MediaType.APPLICATION_JSON))
            .andExpect(jsonPath("$.title").value("Effective Java"))
            .andExpect(jsonPath("$.author").value("Bloch"));
    }

    @Test
    void create_rejectsInvalidBody() throws Exception {
        mvc.perform(post("/api/v1/books")
                .contentType(MediaType.APPLICATION_JSON)
                .content("""
                    {"title": "", "author": "", "price": -1}
                """))
            .andExpect(status().isBadRequest());
    }
}
```

No real service, no real DB. Just the web layer.

### `@DataJpaTest`

```java
package com.example.bookstore.book;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.orm.jpa.DataJpaTest;
import org.springframework.test.context.TestPropertySource;

import static org.assertj.core.api.Assertions.*;

@DataJpaTest
class BookRepositoryTest {

    @Autowired BookRepository repo;

    @Test
    void savesAndFindsByAuthor() {
        repo.save(new Book("Effective Java", "Joshua Bloch", new java.math.BigDecimal("49.99")));
        repo.save(new Book("Clean Code", "Robert Martin", new java.math.BigDecimal("39.99")));

        var page = repo.findByAuthorContainingIgnoreCase("bloch",
            org.springframework.data.domain.PageRequest.of(0, 10));

        assertThat(page.getContent()).hasSize(1);
        assertThat(page.getContent().get(0).getTitle()).isEqualTo("Effective Java");
    }
}
```

`@DataJpaTest` defaults to an in-memory H2 (great for quick checks) and rolls back after each test.

---

## 3. Full-stack tests with `@SpringBootTest` + Testcontainers

For real confidence: start the full app, real DB (in a container), make actual HTTP calls.

### Add Testcontainers

```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-testcontainers</artifactId>
    <scope>test</scope>
</dependency>
<dependency>
    <groupId>org.testcontainers</groupId>
    <artifactId>postgresql</artifactId>
    <scope>test</scope>
</dependency>
<dependency>
    <groupId>org.testcontainers</groupId>
    <artifactId>junit-jupiter</artifactId>
    <scope>test</scope>
</dependency>
```

### A real-DB integration test

```java
package com.example.bookstore;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.boot.testcontainers.service.connection.ServiceConnection;
import org.springframework.http.*;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.*;

import static org.assertj.core.api.Assertions.*;

@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
@Testcontainers
class BookstoreIntegrationTest {

    @Container
    @ServiceConnection
    static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>("postgres:16-alpine");

    @Autowired TestRestTemplate rest;

    @Test
    void createAndFetchBook() {
        var create = """
            { "title": "Effective Java", "author": "Bloch", "price": 49.99 }
            """;
        var headers = new HttpHeaders();
        headers.setContentType(MediaType.APPLICATION_JSON);

        var response = rest.exchange("/api/v1/books",
            HttpMethod.POST, new HttpEntity<>(create, headers), String.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.CREATED);
        String location = response.getHeaders().getLocation().toString();

        var fetched = rest.getForObject(location, String.class);
        assertThat(fetched).contains("Effective Java");
    }
}
```

`@ServiceConnection` (Boot 3.1+) is the killer feature: Spring Boot auto-wires the container's connection details into `spring.datasource.*`. No `@DynamicPropertySource` boilerplate.

### Reusing containers across tests

```java
abstract class IntegrationTestBase {
    @Container @ServiceConnection
    static final PostgreSQLContainer<?> postgres =
        new PostgreSQLContainer<>("postgres:16-alpine");

    static { postgres.start(); }   // started once for the JVM
}

@SpringBootTest
@Testcontainers
class BookFlowTest extends IntegrationTestBase { ... }

@SpringBootTest
@Testcontainers
class OrderFlowTest extends IntegrationTestBase { ... }
```

Massive speedup vs. a new container per test class.

---

## 4. Practical application — the testing pyramid for one feature

For `BookService.create`:

```
                   ┌───────────────────────────┐
   (1 slow test)   │  @SpringBootTest          │  Testcontainers + real PG
                   │  end-to-end POST /books   │
                   └───────────────────────────┘
                 ┌───────────────────────────────┐
   (3 slice)     │  @WebMvcTest BookController   │  routing, JSON, validation
                 │  @DataJpaTest BookRepository  │  JPQL, derived queries
                 └───────────────────────────────┘
        ┌─────────────────────────────────────────────┐
  (many)│ Mockito unit tests of BookService methods   │  pure logic, edge cases
        │ Validation tests on CreateBookRequest       │
        └─────────────────────────────────────────────┘
```

### Validating a DTO without Spring

```java
class CreateBookRequestTest {
    static final Validator validator =
        Validation.buildDefaultValidatorFactory().getValidator();

    @Test
    void rejectsBlankTitle() {
        var req = new CreateBookRequest("", "Bloch", new BigDecimal("10"));
        assertThat(validator.validate(req)).extracting(v -> v.getPropertyPath().toString())
            .contains("title");
    }
}
```

### Slicing with security disabled

`@WebMvcTest` doesn't load `SecurityFilterChain` unless you import it. If your controller is protected, either:

```java
@WebMvcTest(controllers = BookController.class)
@Import(SecurityConfig.class)
class BookControllerSecuredTest {

    @Test
    @WithMockUser(roles = "USER")
    void getOne_okAsUser() throws Exception { ... }

    @Test
    void getOne_401WhenUnauthenticated() throws Exception { ... }
}
```

`@WithMockUser` plants a fake authenticated user in the `SecurityContext`.

### `MockMvc` vs `WebTestClient` vs `TestRestTemplate`

| Tool                 | When                                         |
| -------------------- | -------------------------------------------- |
| `MockMvc`            | `@WebMvcTest` — no real server               |
| `TestRestTemplate`   | `@SpringBootTest(RANDOM_PORT)` — real HTTP   |
| `WebTestClient`      | WebFlux, also works for MVC; fluent API      |

---

## 5. Common Mistakes & Gotchas

- **`@SpringBootTest` for everything.** It starts the whole context — slow. Use unit tests + slices, reserve full context tests for the few flows that actually need them.

- **Reusing the same Spring context across mixed slice tests.** Spring caches contexts; if your test changes config, you get a new context (slow). Keep `@MockBean` usage consistent or accept the cost.

- **Not asserting JSON content with `jsonPath`.** Assertions on serialized strings are brittle. `jsonPath("$.data.id")` survives field reordering and adjacent additions.

- **`@MockBean` overload.** Every `@MockBean` triggers a context restart. For unit-style logic, prefer plain `@Mock` (no context) in unit tests.

- **Forgetting test transactions.** `@DataJpaTest` rolls back by default. `@SpringBootTest` does **not**. State leaks between tests. Either `@Transactional` on the test class or clean up explicitly.

- **Time-dependent tests.** `Instant.now()` in code, "expect exactly now" in test. Use a `Clock` bean and inject a `Clock.fixed(...)` in tests.

- **Testcontainers without a Docker daemon in CI.** Make sure your CI runner has Docker (or uses a service container). On macOS arm64, pin a multi-arch image.

- **Asserting on exception messages.** Refactor → message changes → test fails. Assert on exception **type** + key state, not English strings.

- **Mocking the class under test.** Subtle but common: `@MockBean BookService` while testing `BookController` is fine. `@MockBean BookController` while testing it isn't testing anything.

- **Network calls in unit tests.** Wrap HTTP clients behind an interface, mock the interface. Production-bound URLs in unit tests are a sin.

---

## 🎯 Key Takeaways

- **Pyramid: many unit, some slice, few full-stack.** Speed → confidence in that order.
- **Slices > full context** for layer-specific tests. `@WebMvcTest`, `@DataJpaTest`, `@JsonTest`.
- **Testcontainers + `@ServiceConnection`** is the modern standard for integration tests against real DBs.
- **Mock at boundaries**, not internals. The further inside your service you mock, the more your test mirrors implementation and resists refactoring.
- **Tests document behavior.** A failing test should explain what was expected — name them `verb_outcome_givenInput`.

*[← prev](./10_security.md) | [next →](./12_async_scheduling_events.md)*
