# 15 — Building APIs

> **Goal:** Design REST APIs that age well — proper resource modeling, versioning, OpenAPI documentation via Springdoc, HATEOAS principles, and when to reach for GraphQL.

---

## 1. REST best practices — mental model + working code

A good REST API is a contract: predictable URLs, predictable verbs, predictable status codes, predictable error shape.

### The non-negotiables

- **Nouns for resources, verbs for HTTP methods.** `/orders`, not `/getOrders`. `POST /orders`, not `POST /createOrder`.
- **Plural collections, item by ID.** `/books`, `/books/42`.
- **Hierarchies map to URLs.** `/authors/1/books`, `/orders/42/items`.
- **Status codes mean what they say.** 200, 201, 204, 400, 401, 403, 404, 409, 422, 429, 500. Don't return 200 for an error.
- **Idempotency.** `GET`, `PUT`, `DELETE` are idempotent. `POST` is not (without an idempotency key).
- **Filtering, sorting, pagination via query params.** `/books?author=bloch&page=0&size=20&sort=title,asc`.
- **Consistent error shape** (use `ProblemDetail` from module 06).

### A reference controller

```java
@RestController
@RequestMapping("/api/v1/books")
@Tag(name = "Books", description = "Operations on the books catalog")
public class BookController {

    private final BookService service;
    public BookController(BookService service) { this.service = service; }

    @GetMapping
    @Operation(summary = "List books, paginated, optionally filtered by author")
    public Page<BookDto> list(@RequestParam(required = false) String author, Pageable pageable) {
        return service.search(author, pageable);
    }

    @GetMapping("/{id}")
    @Operation(summary = "Get a single book by ID")
    @ApiResponses({
        @ApiResponse(responseCode = "200", description = "Found"),
        @ApiResponse(responseCode = "404", description = "Not found", content = @Content)
    })
    public BookDto one(@PathVariable Long id) { return service.getOne(id); }

    @PostMapping
    @ResponseStatus(HttpStatus.CREATED)
    @Operation(summary = "Create a new book")
    public BookDto create(@Valid @RequestBody CreateBookRequest req) {
        return service.create(req);
    }

    @PutMapping("/{id}")
    public BookDto replace(@PathVariable Long id, @Valid @RequestBody UpdateBookRequest req) {
        return service.replace(id, req);
    }

    @DeleteMapping("/{id}")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    public void delete(@PathVariable Long id) { service.delete(id); }
}
```

---

## 2. OpenAPI with Springdoc — what it generates

[Springdoc-openapi](https://springdoc.org/) scans your controllers + Bean Validation + Jackson annotations and produces an OpenAPI 3 spec **and** a Swagger UI.

### Add the dep

```xml
<dependency>
    <groupId>org.springdoc</groupId>
    <artifactId>springdoc-openapi-starter-webmvc-ui</artifactId>
    <version>2.5.0</version>
</dependency>
```

### Configure

```yaml
springdoc:
  api-docs:
    path: /v3/api-docs
  swagger-ui:
    path: /swagger-ui.html
    operations-sorter: method
  show-actuator: false
```

Visit:
- `http://localhost:8080/v3/api-docs` — JSON spec
- `http://localhost:8080/swagger-ui.html` — interactive UI

### Customize the doc

```java
@Configuration
public class OpenApiConfig {

    @Bean
    public OpenAPI bookstoreOpenAPI() {
        return new OpenAPI()
            .info(new Info()
                .title("Bookstore API")
                .description("A small bookstore service")
                .version("v1.0")
                .contact(new Contact().name("API team").email("api@example.com")))
            .components(new Components().addSecuritySchemes("bearer-jwt",
                new SecurityScheme().type(SecurityScheme.Type.HTTP).scheme("bearer").bearerFormat("JWT")))
            .addSecurityItem(new SecurityRequirement().addList("bearer-jwt"));
    }
}
```

### Annotate DTOs

```java
public record CreateBookRequest(
    @Schema(example = "Effective Java") @NotBlank String title,
    @Schema(example = "Joshua Bloch") @NotBlank String author,
    @Schema(example = "49.99") @NotNull @DecimalMin("0") BigDecimal price
) {}
```

OpenAPI gets the type, the example, the required flag, the min — all from one source.

---

## 3. Versioning, content negotiation, HATEOAS

### Versioning strategies

| Strategy             | URL                              | Pros                        | Cons                       |
| -------------------- | -------------------------------- | --------------------------- | -------------------------- |
| **URI versioning**   | `/api/v1/books`                  | Simple, visible             | Couples version to URL     |
| **Header versioning**| `X-API-Version: 1`               | Clean URLs                  | Harder to test in browser  |
| **Media-type**       | `Accept: application/vnd.acme.v1+json` | RESTful purist        | Complex, often overkill    |

Most teams use URI versioning. Bump the major version when you make breaking changes; deprecate the old one with a sunset header for 6+ months.

### Additive evolution

Within a version, only add. Adding optional fields and new endpoints doesn't break clients.

```java
public record BookDto(
    Long id,
    String title,
    String author,
    BigDecimal price,
    @JsonInclude(Include.NON_NULL) String isbn   // added later, null-safe for old clients
) {}
```

### HATEOAS overview

Hypermedia-driven APIs return links alongside data, so clients discover URLs instead of hardcoding them.

```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-hateoas</artifactId>
</dependency>
```

```java
@GetMapping("/{id}")
public EntityModel<BookDto> one(@PathVariable Long id) {
    BookDto book = service.getOne(id);
    return EntityModel.of(book,
        linkTo(methodOn(BookController.class).one(id)).withSelfRel(),
        linkTo(methodOn(BookController.class).list(null, null)).withRel("all-books"),
        linkTo(methodOn(OrderController.class).createOrder(id)).withRel("place-order"));
}
```

Response:

```json
{
  "id": 1,
  "title": "Effective Java",
  "author": "Joshua Bloch",
  "_links": {
    "self": { "href": "http://localhost:8080/api/v1/books/1" },
    "all-books": { "href": "http://localhost:8080/api/v1/books" },
    "place-order": { "href": "http://localhost:8080/api/v1/orders" }
  }
}
```

Honest assessment: HATEOAS is more popular in books than in practice. Most internal services don't bother. Public APIs sometimes benefit. **Know what it is**, decide deliberately.

---

## 4. Practical application — production-quality endpoint design

### Pagination + filtering + HATEOAS-lite

```java
public record BookListResponse(
    List<BookDto> items,
    PageInfo page,
    Links links
) {
    public record PageInfo(int number, int size, long totalElements, int totalPages) {}
    public record Links(String self, String next, String prev) {}
}

@GetMapping
public BookListResponse list(@RequestParam(required = false) String author, Pageable pageable) {
    Page<BookDto> page = service.search(author, pageable);
    String self = buildUri(author, pageable.getPageNumber(), pageable.getPageSize());
    String next = page.hasNext() ? buildUri(author, pageable.getPageNumber() + 1, pageable.getPageSize()) : null;
    String prev = page.hasPrevious() ? buildUri(author, pageable.getPageNumber() - 1, pageable.getPageSize()) : null;
    return new BookListResponse(
        page.getContent(),
        new BookListResponse.PageInfo(page.getNumber(), page.getSize(), page.getTotalElements(), page.getTotalPages()),
        new BookListResponse.Links(self, next, prev));
}
```

### Idempotency keys for unsafe operations

```java
@PostMapping
public ResponseEntity<OrderDto> placeOrder(
        @RequestHeader(value = "Idempotency-Key", required = false) String idempotencyKey,
        @Valid @RequestBody PlaceOrderRequest req) {
    OrderDto created = service.placeOrder(req, idempotencyKey);
    return ResponseEntity.status(201).body(created);
}
```

The service stores (idempotencyKey → order ID) for some TTL. Repeated POST with the same key returns the previous result. Standard pattern for payment-like APIs.

### Rate limiting (sketch)

Use Bucket4j or a gateway (Spring Cloud Gateway, Envoy) — not roll-your-own.

```java
@Bean
public Bucket bucket() {
    Bandwidth limit = Bandwidth.classic(100, Refill.greedy(100, Duration.ofMinutes(1)));
    return Bucket.builder().addLimit(limit).build();
}
```

### GraphQL — when and how

GraphQL shines when:
- Clients vary widely in what they need (mobile vs web vs admin).
- Multiple resources composed in one request reduces round-trips.
- Schema-driven typing is a strong tooling win.

Add the starter:

```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-graphql</artifactId>
</dependency>
```

`src/main/resources/graphql/schema.graphqls`:
```graphql
type Book { id: ID!, title: String!, author: String!, price: Float! }
type Query {
    book(id: ID!): Book
    books(author: String): [Book!]!
}
type Mutation {
    createBook(input: CreateBookInput!): Book!
}
input CreateBookInput { title: String!, author: String!, price: Float! }
```

```java
@Controller
public class BookGraphQLController {

    private final BookService service;
    public BookGraphQLController(BookService service) { this.service = service; }

    @QueryMapping
    public BookDto book(@Argument Long id) { return service.getOne(id); }

    @QueryMapping
    public List<BookDto> books(@Argument String author) { ... }

    @MutationMapping
    public BookDto createBook(@Argument CreateBookInput input) { ... }
}
```

Don't add GraphQL "because it's cool." It adds caching complexity, security challenges (depth limits, query cost analysis), and operational overhead. Use it when the cost is justified.

---

## 5. Common Mistakes & Gotchas

- **Verbs in URLs.** `POST /api/createBook` — wrong. Use `POST /api/books`.

- **Returning 200 with an error in the body.** "200 with `{"error": "not found"}`" forces every client to parse bodies before checking status. Use 4xx/5xx and `ProblemDetail`.

- **Breaking changes inside the same version.** Adding a required field, renaming a field, changing the type. Add new fields optional. Deprecate, don't rename.

- **`PATCH` confusion.** JSON Merge Patch vs JSON Patch (RFC 6902). Pick one and document. Don't accept arbitrary `Map<String, Object>` and YOLO it — you'll lose null-vs-missing semantics.

- **No pagination on list endpoints.** Works for 50 rows in dev, hangs for 50k rows in prod. Default page size, hard cap.

- **Returning entities (not DTOs).** Coupling DB schema to API. See module 05.

- **Inconsistent date formats.** Some endpoints `yyyy-MM-dd`, others `MM/dd/yyyy`. Use ISO 8601 (`yyyy-MM-ddTHH:mm:ssZ`) everywhere via Jackson config.

- **Swagger UI exposed in production.** Useful in dev, but exposes the entire API surface to anyone who finds it. Disable or auth-gate in prod.

- **No deprecation policy.** Bumping to v2 and immediately killing v1 breaks clients. Document, communicate, sunset over months.

- **CORS open to `*` with credentials.** Browsers reject; security review fails. Whitelist origins.

- **GraphQL N+1.** Resolving each `author` in a list of `books` separately. Use DataLoader for batching.

---

## 🎯 Key Takeaways

- **REST is a contract.** Predictability beats cleverness — every time.
- **OpenAPI via Springdoc is free documentation.** Annotate DTOs once, get spec + Swagger UI.
- **Version in the URL** for most use cases. Additive evolution within a version.
- **HATEOAS is a tool, not a religion.** Reach for it when discovery actually matters.
- **GraphQL is a different stack with different operational concerns.** Adopt deliberately, not by fashion.

*[← prev](./14_observability.md) | [next →](./16_messaging_integration.md)*
