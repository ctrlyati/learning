# 03 — Dependency Injection & IoC

> **Goal:** Internalize Spring's bean graph, the four stereotype annotations, why constructor injection is non-negotiable, and how bean scopes shape your design.

---

## 1. IoC & DI — mental model + working code

**Inversion of Control (IoC):** instead of *you* calling `new BookRepository()`, you declare a dependency and **Spring hands you** a fully-wired instance.

**Dependency Injection (DI):** the mechanism. Spring inspects your class's constructor (or fields, or setters) and supplies the required beans.

### Without DI

```java
public class BookService {
    private final BookRepository repo = new InMemoryBookRepository(); // hardcoded
    private final EmailSender email = new SmtpEmailSender("smtp.example.com");
    // ...
}
```

Coupled to concrete classes. Untestable without rewriting. Can't swap implementations per environment.

### With DI (constructor injection — the right way)

```java
package com.example.bookstore.book;

import org.springframework.stereotype.Service;

@Service
public class BookService {

    private final BookRepository repo;
    private final EmailSender email;

    public BookService(BookRepository repo, EmailSender email) {
        this.repo = repo;
        this.email = email;
    }

    public Book findById(Long id) {
        return repo.findById(id).orElseThrow();
    }
}
```

Spring sees the constructor, looks up a `BookRepository` bean and an `EmailSender` bean in the context, instantiates `BookService`, and registers it. You never touch `new`.

---

## 2. The bean lifecycle — what Spring does behind the annotations

When Spring boots:

1. **Scan** the base package (from `@SpringBootApplication`'s package). Find every `@Component`, `@Service`, `@Repository`, `@Controller`, `@Configuration`.
2. **Build bean definitions** — metadata about each class (scope, dependencies, init/destroy methods). Nothing is instantiated yet.
3. **Resolve the dependency graph.** Detect cycles. Order topologically.
4. **Instantiate** singleton beans in order. For each: call constructor with resolved dependencies → apply `BeanPostProcessor`s (this is where AOP proxies wrap your bean) → call `@PostConstruct` methods.
5. **Application is ready.** Start the embedded server.
6. **On shutdown:** call `@PreDestroy` methods in reverse order.

### Visualizing the lifecycle

```java
package com.example.bookstore.book;

import jakarta.annotation.PostConstruct;
import jakarta.annotation.PreDestroy;
import org.springframework.stereotype.Service;

@Service
public class BookService {

    public BookService() {
        System.out.println("1. Constructor");
    }

    @PostConstruct
    void init() {
        System.out.println("2. @PostConstruct — context is fully wired");
    }

    @PreDestroy
    void cleanup() {
        System.out.println("3. @PreDestroy — context is shutting down");
    }
}
```

### Stereotype annotations — semantic only

```java
@Component   // generic
@Service     // business logic
@Repository  // data access (also translates persistence exceptions)
@Controller  // web controller (returns view names)
@RestController  // @Controller + @ResponseBody — returns JSON
```

To Spring, they're all `@Component`. To you and your team, they document **intent**. Use the most specific one.

### `@Repository` is special

It adds a `PersistenceExceptionTranslationPostProcessor` that converts JDBC/JPA exceptions into Spring's `DataAccessException` hierarchy. Without it, you handle `SQLException`. With it, you handle the cleaner `DataAccessException`.

---

## 3. Variations & depth — injection types, scopes, qualifiers

### Three injection styles

```java
@Service
public class BookService {

    // 1) Constructor (PREFERRED — supports final fields, easy to test, fails fast)
    private final BookRepository repo;
    public BookService(BookRepository repo) { this.repo = repo; }

    // 2) Setter (rare — only for truly optional deps)
    private EmailSender email;
    @Autowired(required = false)
    public void setEmail(EmailSender email) { this.email = email; }

    // 3) Field (DON'T — untestable without reflection, hides dependencies)
    @Autowired private AuditLog audit;
}
```

**Use constructor injection.** Always. Spring 4.3+ doesn't even require `@Autowired` if there's exactly one constructor.

### Bean scopes

| Scope         | Lifetime                                                |
| ------------- | ------------------------------------------------------- |
| `singleton`   | One per Spring context (the default)                    |
| `prototype`   | New instance per injection / `getBean()` call           |
| `request`     | One per HTTP request (web only)                         |
| `session`     | One per HTTP session (web only)                         |
| `application` | One per ServletContext                                  |

```java
@Service
@Scope(ConfigurableBeanFactory.SCOPE_PROTOTYPE)
public class ShoppingCart { ... }
```

**Default is singleton.** That means your service is shared across all threads. Make services **stateless**.

### Qualifiers — disambiguating multiple beans of the same type

```java
public interface PaymentProcessor { void charge(BigDecimal amount); }

@Component("stripe") class StripeProcessor implements PaymentProcessor { ... }
@Component("paypal") class PaypalProcessor implements PaymentProcessor { ... }

@Service
public class CheckoutService {
    private final PaymentProcessor processor;

    public CheckoutService(@Qualifier("stripe") PaymentProcessor processor) {
        this.processor = processor;
    }
}
```

Or inject all of them:

```java
public CheckoutService(List<PaymentProcessor> all,
                       Map<String, PaymentProcessor> byName) {
    // all = [stripe, paypal]
    // byName = {"stripe": ..., "paypal": ...}
}
```

This is hugely useful for **strategy patterns**.

### `@Primary` — the default among many

```java
@Component
@Primary
public class StripeProcessor implements PaymentProcessor { ... }
```

Injected without a qualifier → you get the Primary one.

### Java config (`@Configuration` + `@Bean`)

When you can't put `@Component` on a class (third-party library), declare beans manually:

```java
@Configuration
public class ClientsConfig {

    @Bean
    public RestClient bookstoreClient(
            @Value("${bookstore.upstream.url}") String url) {
        return RestClient.builder().baseUrl(url).build();
    }
}
```

---

## 4. Practical application — a fully wired feature slice

A miniature feature: list and create books, with a notification side-effect.

```java
// domain
package com.example.bookstore.book;

public record Book(Long id, String title, String author) {}
```

```java
// repository (in-memory for now)
package com.example.bookstore.book;

import java.util.*;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicLong;
import org.springframework.stereotype.Repository;

@Repository
public class BookRepository {
    private final Map<Long, Book> store = new ConcurrentHashMap<>();
    private final AtomicLong ids = new AtomicLong();

    public Book save(Book b) {
        long id = b.id() == null ? ids.incrementAndGet() : b.id();
        Book saved = new Book(id, b.title(), b.author());
        store.put(id, saved);
        return saved;
    }

    public Optional<Book> findById(Long id) { return Optional.ofNullable(store.get(id)); }
    public Collection<Book> findAll() { return store.values(); }
}
```

```java
// notifier (an optional dependency)
package com.example.bookstore.book;

import org.springframework.stereotype.Component;

@Component
public class ConsoleNotifier implements BookNotifier {
    public void notifyCreated(Book b) {
        System.out.println("Book created: " + b.title());
    }
}

interface BookNotifier { void notifyCreated(Book b); }
```

```java
// service
package com.example.bookstore.book;

import java.util.Collection;
import org.springframework.stereotype.Service;

@Service
public class BookService {

    private final BookRepository repo;
    private final BookNotifier notifier;

    public BookService(BookRepository repo, BookNotifier notifier) {
        this.repo = repo;
        this.notifier = notifier;
    }

    public Book create(String title, String author) {
        Book saved = repo.save(new Book(null, title, author));
        notifier.notifyCreated(saved);
        return saved;
    }

    public Collection<Book> all() { return repo.findAll(); }
}
```

```java
// controller
package com.example.bookstore.book;

import java.util.Collection;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/books")
public class BookController {

    private final BookService service;

    public BookController(BookService service) {
        this.service = service;
    }

    @GetMapping
    public Collection<Book> list() { return service.all(); }

    @PostMapping
    public ResponseEntity<Book> create(@RequestBody CreateBookRequest req) {
        Book b = service.create(req.title(), req.author());
        return ResponseEntity.status(201).body(b);
    }

    record CreateBookRequest(String title, String author) {}
}
```

Run it:

```bash
curl -X POST http://localhost:8080/books \
  -H "Content-Type: application/json" \
  -d '{"title":"Effective Java","author":"Joshua Bloch"}'
# {"id":1,"title":"Effective Java","author":"Joshua Bloch"}

curl http://localhost:8080/books
# [{"id":1,...}]
```

Four classes, no `new`, every wire managed by Spring.

---

## 5. Common Mistakes & Gotchas

- **Field injection (`@Autowired private Foo foo;`).** Makes unit tests harder (you need reflection or Spring to inject). Hides cyclic deps. Permits non-final fields. Never use it in code you'll ship.

- **Circular dependencies.** `A` needs `B`, `B` needs `A`. Spring 2.6+ refuses to start by default. Symptom: `BeanCurrentlyInCreationException`. Fix: redesign — usually one of them should be an event publisher or split into a third bean. **Don't** "fix" it with `@Lazy`; you're hiding a design smell.

- **Mutating singleton state.** A `@Service` is shared across threads. Adding a `private List<...>` that grows per request is a memory leak *and* a race condition. Keep services stateless; state lives in the database or in request-scoped beans.

- **Forgetting that `@Configuration` classes are themselves singletons.** `@Bean` methods are intercepted via a CGLIB proxy, so calling `this.someBean()` from another `@Bean` method returns the cached bean — except in `@Configuration(proxyBeanMethods = false)`, where it returns a fresh instance each time. Know what you've opted into.

- **Multiple beans of the same type with no `@Primary` or `@Qualifier`.** Startup fails with `NoUniqueBeanDefinitionException`. Either qualify, mark primary, or inject `List<Foo>`.

- **Component-scanning library code.** Adding `@ComponentScan("com.thirdparty")` to grab beans from a non-Spring library — fragile and slow. Use a proper `@Configuration` class that declares the beans you need.

---

## 🎯 Key Takeaways

- **Constructor injection or it didn't happen.** Final fields, fail-fast, framework-agnostic tests.
- **The application context is a graph; cycles are bugs.** Redesign before you reach for `@Lazy`.
- **Stereotype annotations document intent.** `@Service`/`@Repository`/`@Controller` aren't decorative — they communicate to the next developer.
- **Singletons are shared, so services must be stateless.** All mutable state belongs in DB, cache, or request scope.
- **`@Bean` in `@Configuration` is the escape hatch** for wiring code you don't control (third-party libs, conditional logic, builders).

*[← prev](./02_fundamentals.md) | [next →](./04_configuration.md)*
