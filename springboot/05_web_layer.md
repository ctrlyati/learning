# 05 — Web Layer

> **Goal:** Build production-quality REST endpoints — request mapping, binding rules, response shaping, content negotiation, and the `DispatcherServlet` flow that runs underneath it all.

---

## 1. `@RestController` — mental model + working code

A `@RestController` is `@Controller` + `@ResponseBody`. Return values are serialized straight to the response body (typically as JSON via Jackson).

```java
package com.example.bookstore.book;

import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/books")
public class BookController {

    @GetMapping
    public List<Book> list() { return List.of(); }

    @GetMapping("/{id}")
    public Book getOne(@PathVariable Long id) { return new Book(id, "title", "author"); }
}
```

### The HTTP-verb shortcuts

| Annotation        | HTTP method |
| ----------------- | ----------- |
| `@GetMapping`     | GET         |
| `@PostMapping`    | POST        |
| `@PutMapping`     | PUT         |
| `@PatchMapping`   | PATCH       |
| `@DeleteMapping`  | DELETE      |
| `@RequestMapping` | any (also lets you set produces/consumes) |

---

## 2. The DispatcherServlet — what Spring does behind the scenes

Every HTTP request hits `DispatcherServlet`, which:

1. Asks **`HandlerMapping`** "which controller method handles this URL + method?"
2. Asks **`HandlerAdapter`** to invoke it, resolving parameters via `HandlerMethodArgumentResolver`s:
   - `@PathVariable` → from URI template
   - `@RequestParam` → from query string / form
   - `@RequestBody` → deserialized via Jackson from request body
   - `@RequestHeader` → from headers
   - `HttpServletRequest`, `Authentication`, `Principal`, `Locale` → from request context
3. Captures the return value, asks `HandlerMethodReturnValueHandler`s how to write it:
   - `String` → view name (in `@Controller`) or body (in `@RestController`)
   - Object → JSON via `MappingJackson2HttpMessageConverter`
   - `ResponseEntity<T>` → body + status + headers
4. Runs **`HandlerInterceptor`** chain (auth, logging, etc.) before & after.
5. If anything throws, **`HandlerExceptionResolver`** chain converts to a response.

### Inspect the routes

```bash
curl http://localhost:8080/actuator/mappings | jq
```

You'll see every URL → method binding Spring discovered.

---

## 3. Binding — path, query, header, body

```java
@RestController
@RequestMapping("/books")
public class BookController {

    // Path variable
    @GetMapping("/{id}")
    public Book one(@PathVariable Long id) { ... }

    // Query parameter with default
    @GetMapping
    public List<Book> list(
            @RequestParam(defaultValue = "0") int page,
            @RequestParam(defaultValue = "20") int size,
            @RequestParam Optional<String> author
    ) { ... }

    // Header
    @GetMapping("/{id}/audit")
    public Audit audit(@PathVariable Long id,
                       @RequestHeader("X-User-Id") String userId) { ... }

    // Body
    @PostMapping
    public Book create(@RequestBody CreateBookRequest req) { ... }

    // Form-encoded
    @PostMapping("/login")
    public LoginResult login(@RequestParam String username,
                              @RequestParam String password) { ... }
}

record CreateBookRequest(String title, String author, BigDecimal price) {}
```

### `Pageable` shortcut

```java
import org.springframework.data.domain.Pageable;

@GetMapping
public Page<Book> list(Pageable pageable) {
    return repo.findAll(pageable);
}
```

Hits `/books?page=2&size=10&sort=title,desc` and Spring builds the `Pageable` for you.

---

## 4. Response shaping — `ResponseEntity`, status codes, headers

### Return type = body, status = 200

```java
@GetMapping("/{id}")
public Book one(@PathVariable Long id) { return ...; }
```

### `ResponseEntity` for full control

```java
@PostMapping
public ResponseEntity<Book> create(@RequestBody CreateBookRequest req) {
    Book saved = service.create(req);
    URI location = URI.create("/books/" + saved.id());
    return ResponseEntity.created(location).body(saved);
}

@DeleteMapping("/{id}")
public ResponseEntity<Void> delete(@PathVariable Long id) {
    service.delete(id);
    return ResponseEntity.noContent().build();
}
```

### `@ResponseStatus` for simple cases

```java
@PostMapping
@ResponseStatus(HttpStatus.CREATED)
public Book create(@RequestBody CreateBookRequest req) { return service.create(req); }
```

### Content negotiation — JSON, XML, etc.

By default `spring-boot-starter-web` registers a Jackson JSON converter. To also support XML:

```xml
<dependency>
    <groupId>com.fasterxml.jackson.dataformat</groupId>
    <artifactId>jackson-dataformat-xml</artifactId>
</dependency>
```

Now:

```bash
curl -H "Accept: application/xml" http://localhost:8080/books/1
# returns XML

curl -H "Accept: application/json" http://localhost:8080/books/1
# returns JSON
```

Restrict per endpoint:

```java
@GetMapping(value = "/{id}", produces = MediaType.APPLICATION_JSON_VALUE)
public Book json(@PathVariable Long id) { ... }
```

### Practical application — full CRUD controller

```java
package com.example.bookstore.book;

import jakarta.validation.Valid;
import java.net.URI;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/v1/books")
public class BookController {

    private final BookService service;

    public BookController(BookService service) { this.service = service; }

    @GetMapping
    public Page<BookDto> list(@RequestParam(required = false) String author,
                               Pageable pageable) {
        return service.search(author, pageable);
    }

    @GetMapping("/{id}")
    public BookDto one(@PathVariable Long id) {
        return service.getOne(id);
    }

    @PostMapping
    public ResponseEntity<BookDto> create(@Valid @RequestBody CreateBookRequest req) {
        BookDto created = service.create(req);
        return ResponseEntity
                .created(URI.create("/api/v1/books/" + created.id()))
                .body(created);
    }

    @PutMapping("/{id}")
    public BookDto replace(@PathVariable Long id, @Valid @RequestBody UpdateBookRequest req) {
        return service.replace(id, req);
    }

    @PatchMapping("/{id}")
    public BookDto patch(@PathVariable Long id, @RequestBody Map<String, Object> updates) {
        return service.patch(id, updates);
    }

    @DeleteMapping("/{id}")
    public ResponseEntity<Void> delete(@PathVariable Long id) {
        service.delete(id);
        return ResponseEntity.noContent().build();
    }
}

record BookDto(Long id, String title, String author, BigDecimal price) {}
record CreateBookRequest(String title, String author, BigDecimal price) {}
record UpdateBookRequest(String title, String author, BigDecimal price) {}
```

### Cross-cutting: `CORS`

```java
@CrossOrigin(origins = "https://frontend.example.com")
@RestController
@RequestMapping("/api/v1/books")
public class BookController { ... }
```

Or globally:

```java
@Configuration
public class WebConfig implements WebMvcConfigurer {
    @Override
    public void addCorsMappings(CorsRegistry registry) {
        registry.addMapping("/api/**")
                .allowedOrigins("https://frontend.example.com")
                .allowedMethods("GET", "POST", "PUT", "DELETE");
    }
}
```

### Interceptors — request-scoped cross-cutting

```java
@Component
public class RequestLogger implements HandlerInterceptor {
    @Override
    public boolean preHandle(HttpServletRequest req, HttpServletResponse res, Object handler) {
        log.info("{} {}", req.getMethod(), req.getRequestURI());
        return true;
    }
}

@Configuration
public class WebConfig implements WebMvcConfigurer {
    private final RequestLogger logger;
    public WebConfig(RequestLogger logger) { this.logger = logger; }

    @Override
    public void addInterceptors(InterceptorRegistry registry) {
        registry.addInterceptor(logger).addPathPatterns("/api/**");
    }
}
```

---

## 5. Common Mistakes & Gotchas

- **Returning entities directly from controllers.** Couples your API to your DB schema. Add or change a column → API breaks. Always map to a DTO (record).

- **Using `Map<String, Object>` as return type "for flexibility".** You lose type safety, OpenAPI generation breaks, and clients have no contract. Records are five lines of code; write them.

- **Mismatched `@RequestBody` content types.** If the client sends `Content-Type: application/x-www-form-urlencoded` and you have `@RequestBody`, Spring returns 415. Use `@RequestParam` for form data.

- **Wildcard `@CrossOrigin(origins = "*")` with credentials.** Browsers reject it. Be specific about origins and never combine `*` with `allowCredentials = true`.

- **Returning `null` from a controller.** Becomes a 200 with empty body — confusing. Throw a `404`-mapped exception instead (see module 06).

- **Thinking `@GetMapping("/books/")` and `@GetMapping("/books")` are the same.** They were in Boot 2; in Boot 3 with the new path-matching they aren't, unless you opt in. Pick one form per project and stick to it.

- **Using `HttpServletRequest` to dig out path variables manually.** That's what `@PathVariable` is for. The argument resolvers exist precisely so you don't write that code.

- **Long parameter lists in a single method.** Eight `@RequestParam` arguments → wrap them in a record and bind via `@ModelAttribute`.

---

## 🎯 Key Takeaways

- **`@RestController` returns serialized bodies.** Use DTOs (records) — never expose entities.
- **`DispatcherServlet` is the front door.** Every request goes through HandlerMapping → HandlerAdapter → argument resolvers → return-value handlers.
- **`ResponseEntity` for status + headers + body**; bare return type for the simple 200-with-body case.
- **Content negotiation is config-driven**, not method-driven. Add Jackson XML, request `Accept: application/xml`, done.
- **Boundary mapping (DTO ↔ entity) is non-negotiable** in any service you'll maintain past month 3.

*[← prev](./04_configuration.md) | [next →](./06_validation_and_errors.md)*
