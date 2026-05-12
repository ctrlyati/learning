# 06 — Validation & Error Handling

> **Goal:** Validate input declaratively, handle exceptions in one place, and return RFC 7807 `ProblemDetail` responses your clients can rely on.

---

## 1. Bean Validation — mental model + working code

Bean Validation (JSR 380) is the standard. Spring Boot includes Hibernate Validator via `spring-boot-starter-validation` (pulled transitively by `spring-boot-starter-web` in Boot 3).

```java
package com.example.bookstore.book;

import jakarta.validation.constraints.*;
import java.math.BigDecimal;

public record CreateBookRequest(
        @NotBlank @Size(max = 200) String title,
        @NotBlank @Size(max = 100) String author,
        @NotNull @DecimalMin("0.00") @DecimalMax("9999.99") BigDecimal price,
        @Email String contactEmail,
        @Pattern(regexp = "^97[89]-\\d{10}$") String isbn
) {}
```

### Triggering validation in a controller

```java
import jakarta.validation.Valid;

@PostMapping
public ResponseEntity<BookDto> create(@Valid @RequestBody CreateBookRequest req) {
    BookDto created = service.create(req);
    return ResponseEntity.status(201).body(created);
}
```

Without `@Valid`, the constraints are ignored. **`@Valid` is the trigger; the annotations are the rules.**

### What happens on failure

Spring throws `MethodArgumentNotValidException` *before your method runs*. By default Boot 3 returns a `ProblemDetail` (RFC 7807):

```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "status": 400,
  "detail": "Invalid request content.",
  "instance": "/api/v1/books"
}
```

Without further customization the field-level details are buried. We fix that in section 4.

---

## 2. Validation under the hood

The mechanism:

1. `RequestResponseBodyMethodProcessor` deserializes the JSON to the record.
2. Because of `@Valid`, it calls `Validator.validate(record)` — Hibernate Validator's implementation.
3. Each annotation has a `ConstraintValidator` registered. They run and produce `ConstraintViolation`s.
4. If any violations exist → `MethodArgumentNotValidException` is thrown.
5. The exception is caught by `ResponseEntityExceptionHandler` (built-in) which produces the `ProblemDetail`.

### Constraint annotations cheat sheet

| Annotation              | Applies to                                  |
| ----------------------- | ------------------------------------------- |
| `@NotNull`              | any object — must be non-null               |
| `@NotEmpty`             | string/collection/array/map — non-null, size > 0 |
| `@NotBlank`             | string — non-null and contains non-whitespace |
| `@Size(min, max)`       | string/collection/array/map                  |
| `@Min` / `@Max`         | numeric                                      |
| `@DecimalMin`/`@DecimalMax` | numeric, supports decimals               |
| `@Positive`/`@Negative` | numeric                                      |
| `@Email`                | string                                       |
| `@Pattern(regexp = ...)`| string                                       |
| `@Past` / `@Future`     | temporal                                     |
| `@Valid` (on a field)   | cascade validation to nested object         |

### Custom constraint

```java
package com.example.bookstore.validation;

import jakarta.validation.*;
import java.lang.annotation.*;

@Target({ElementType.FIELD, ElementType.PARAMETER, ElementType.RECORD_COMPONENT})
@Retention(RetentionPolicy.RUNTIME)
@Constraint(validatedBy = Isbn13Validator.class)
public @interface Isbn13 {
    String message() default "must be a valid ISBN-13";
    Class<?>[] groups() default {};
    Class<? extends Payload>[] payload() default {};
}
```

```java
public class Isbn13Validator implements ConstraintValidator<Isbn13, String> {
    private static final java.util.regex.Pattern P =
        java.util.regex.Pattern.compile("^97[89]-?\\d{10}$");

    @Override
    public boolean isValid(String value, ConstraintValidatorContext ctx) {
        return value == null || P.matcher(value).matches();
    }
}
```

Use it:

```java
public record CreateBookRequest(..., @Isbn13 String isbn) {}
```

---

## 3. Path / query validation

`@Valid` only works on request bodies. For path/query params, annotate the controller with `@Validated` (Spring's class-level annotation):

```java
import org.springframework.validation.annotation.Validated;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.NotBlank;

@RestController
@RequestMapping("/api/v1/books")
@Validated
public class BookController {

    @GetMapping("/{id}")
    public BookDto one(@PathVariable @Min(1) Long id) { ... }

    @GetMapping
    public List<BookDto> search(@RequestParam @NotBlank String author) { ... }
}
```

These throw `ConstraintViolationException`, **different** from `MethodArgumentNotValidException`. You'll handle both.

---

## 4. Practical application — global error handling with `@ControllerAdvice`

This is **the** professional pattern. One class. Every exception. Consistent shape.

```java
package com.example.bookstore.web;

import jakarta.validation.ConstraintViolationException;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.http.*;
import org.springframework.web.bind.MethodArgumentNotValidException;
import org.springframework.web.bind.annotation.*;

import java.net.URI;
import java.time.Instant;
import java.util.*;
import java.util.stream.Collectors;

@RestControllerAdvice
public class GlobalExceptionHandler {

    // 404 — domain object missing
    @ExceptionHandler(BookNotFoundException.class)
    public ProblemDetail handleNotFound(BookNotFoundException ex) {
        ProblemDetail pd = ProblemDetail.forStatusAndDetail(HttpStatus.NOT_FOUND, ex.getMessage());
        pd.setType(URI.create("https://api.bookstore.example.com/errors/not-found"));
        pd.setTitle("Book not found");
        pd.setProperty("timestamp", Instant.now());
        return pd;
    }

    // 400 — invalid request body
    @ExceptionHandler(MethodArgumentNotValidException.class)
    public ProblemDetail handleBodyValidation(MethodArgumentNotValidException ex) {
        Map<String, String> fieldErrors = ex.getBindingResult().getFieldErrors().stream()
                .collect(Collectors.toMap(
                    err -> err.getField(),
                    err -> err.getDefaultMessage() == null ? "invalid" : err.getDefaultMessage(),
                    (a, b) -> a));
        ProblemDetail pd = ProblemDetail.forStatusAndDetail(HttpStatus.BAD_REQUEST, "Validation failed");
        pd.setTitle("Bad request");
        pd.setProperty("errors", fieldErrors);
        return pd;
    }

    // 400 — invalid path/query parameter
    @ExceptionHandler(ConstraintViolationException.class)
    public ProblemDetail handleParamValidation(ConstraintViolationException ex) {
        List<String> messages = ex.getConstraintViolations().stream()
                .map(v -> v.getPropertyPath() + " " + v.getMessage())
                .toList();
        ProblemDetail pd = ProblemDetail.forStatusAndDetail(HttpStatus.BAD_REQUEST, "Invalid parameters");
        pd.setProperty("errors", messages);
        return pd;
    }

    // 409 — DB constraint
    @ExceptionHandler(DataIntegrityViolationException.class)
    public ProblemDetail handleDbConflict(DataIntegrityViolationException ex) {
        ProblemDetail pd = ProblemDetail.forStatusAndDetail(HttpStatus.CONFLICT, "Resource conflict");
        pd.setTitle("Conflict");
        return pd;
    }

    // 500 — fallback
    @ExceptionHandler(Exception.class)
    public ProblemDetail handleUnexpected(Exception ex) {
        ProblemDetail pd = ProblemDetail.forStatusAndDetail(
            HttpStatus.INTERNAL_SERVER_ERROR, "Unexpected error");
        // do NOT leak ex.getMessage() in production
        return pd;
    }
}
```

### The domain exception

```java
package com.example.bookstore.book;

public class BookNotFoundException extends RuntimeException {
    public BookNotFoundException(Long id) {
        super("Book " + id + " not found");
    }
}
```

### Service throws it

```java
public BookDto getOne(Long id) {
    return repo.findById(id)
            .map(BookMapper::toDto)
            .orElseThrow(() -> new BookNotFoundException(id));
}
```

### Result — clean responses

`GET /api/v1/books/9999`:

```http
HTTP/1.1 404 Not Found
Content-Type: application/problem+json

{
  "type": "https://api.bookstore.example.com/errors/not-found",
  "title": "Book not found",
  "status": 404,
  "detail": "Book 9999 not found",
  "instance": "/api/v1/books/9999",
  "timestamp": "2026-05-13T10:00:00Z"
}
```

`POST /api/v1/books` with bad body:

```http
HTTP/1.1 400 Bad Request
Content-Type: application/problem+json

{
  "type": "about:blank",
  "title": "Bad request",
  "status": 400,
  "detail": "Validation failed",
  "instance": "/api/v1/books",
  "errors": {
    "title": "must not be blank",
    "price": "must be greater than or equal to 0.00"
  }
}
```

### `@ResponseStatus` shortcut

For exceptions you control and don't need extra detail on:

```java
@ResponseStatus(HttpStatus.NOT_FOUND)
public class BookNotFoundException extends RuntimeException { ... }
```

Spring's default handler picks up the status. Skip the `@ExceptionHandler` for the simple case.

---

## 5. Common Mistakes & Gotchas

- **Forgetting `@Valid` on `@RequestBody`.** Constraints exist on the record but aren't checked. Easy to miss in code review — write a test that submits invalid input and asserts 400.

- **Validating an entity instead of a DTO.** The validation rules belong on the input contract. Entities should be valid by construction (validated upstream). Mixing both creates duplication and contradictions.

- **`@Validated` vs `@Valid` confusion.** `@Valid` is JSR-380, used on bodies and cascades into fields. `@Validated` is Spring-specific, used at the class level to enable method-level validation (path/query/method args). You often need both.

- **Returning the exception's full stack trace in production.** Information leak. Catch in `@ControllerAdvice`, log the full stack server-side, return a sanitized `ProblemDetail`.

- **Mapping `IllegalArgumentException` to 400 globally.** Too broad. Library code throws it for completely different reasons. Define your own domain exceptions; map those.

- **Using `@ControllerAdvice` and not `@RestControllerAdvice` for JSON APIs.** Without `@ResponseBody` semantics, return values become view names. `@RestControllerAdvice` is the correct one for REST.

- **Ignoring `BindException` for `@ModelAttribute` form binding.** It's a separate exception path from JSON body validation. If you accept forms, handle it.

- **Swallowing exceptions in services.** `try { ... } catch (Exception e) { return null; }` is the source of "the API returns 200 but the data is wrong" tickets. Let exceptions flow; handle them at the boundary.

---

## 🎯 Key Takeaways

- **`@Valid` + Bean Validation + DTOs** is the single right input-validation pattern.
- **One `@RestControllerAdvice`** for the whole app. Consistency is the contract.
- **`ProblemDetail` (RFC 7807)** is the default in Boot 3. Clients can parse it generically.
- **Custom domain exceptions, not framework exceptions.** `BookNotFoundException` reads better than `EntityNotFoundException` 18 months in.
- **Never leak internals.** Log the exception; return a sanitized response. Two different audiences.

*[← prev](./05_web_layer.md) | [next →](./07_spring_data_jpa.md)*
