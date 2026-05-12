# 07 — Spring Data JPA

> **Goal:** Map Java records/classes to relational tables, use repository abstractions, write JPQL and derived queries, and paginate — without falling into N+1.

---

## 1. JPA basics — mental model + working code

JPA (Jakarta Persistence API) is the spec; **Hibernate** is the default implementation Spring Boot ships. **Spring Data JPA** adds repository abstractions on top.

```
[Your Entity]──@Entity──>[JPA/Hibernate]──SQL──>[Database]
       ▲                        │
       └──Spring Data Repository┘
```

### Add the starter

Maven:
```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-data-jpa</artifactId>
</dependency>
<dependency>
    <groupId>org.postgresql</groupId>
    <artifactId>postgresql</artifactId>
    <scope>runtime</scope>
</dependency>
```

Gradle:
```kotlin
implementation("org.springframework.boot:spring-boot-starter-data-jpa")
runtimeOnly("org.postgresql:postgresql")
```

### Configure the datasource

```yaml
spring:
  datasource:
    url: jdbc:postgresql://localhost:5432/bookstore
    username: app
    password: secret
  jpa:
    hibernate:
      ddl-auto: validate     # NEVER use 'create' or 'update' in production
    show-sql: false
    properties:
      hibernate:
        format_sql: true
        jdbc.batch_size: 50
```

For dev / quick prototyping use H2:

```xml
<dependency>
    <groupId>com.h2database</groupId>
    <artifactId>h2</artifactId>
    <scope>runtime</scope>
</dependency>
```

```yaml
spring:
  datasource:
    url: jdbc:h2:mem:dev
  jpa:
    hibernate:
      ddl-auto: create-drop  # OK for dev only
```

---

## 2. Entities — what JPA does behind the annotations

```java
package com.example.bookstore.book;

import jakarta.persistence.*;
import java.math.BigDecimal;
import java.time.Instant;

@Entity
@Table(name = "books")
public class Book {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(nullable = false, length = 200)
    private String title;

    @Column(nullable = false, length = 100)
    private String author;

    @Column(nullable = false, precision = 10, scale = 2)
    private BigDecimal price;

    @Column(name = "created_at", nullable = false, updatable = false)
    private Instant createdAt = Instant.now();

    @Version
    private Long version;  // optimistic locking

    protected Book() {}    // JPA requires a no-arg constructor

    public Book(String title, String author, BigDecimal price) {
        this.title = title; this.author = author; this.price = price;
    }

    // getters and setters omitted for brevity
}
```

### Why `protected Book()` (no-arg constructor)?

Hibernate instantiates entities via reflection and then sets fields. It needs a no-arg constructor. `protected` keeps it semi-private while satisfying JPA.

### Mechanism

At startup:
1. Hibernate scans `@Entity` classes.
2. Builds a metadata model: tables, columns, relationships.
3. With `ddl-auto: validate`, it queries the DB and confirms the schema matches. With `update`, it tries to alter the schema (risky). With `none`, it skips entirely.
4. The `EntityManager` proxy is created per transaction; persistent objects are tracked in its **persistence context** (the first-level cache).

### Relationships

```java
@Entity
public class Book {
    @Id @GeneratedValue Long id;

    @ManyToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "author_id")
    private Author author;
}

@Entity
public class Author {
    @Id @GeneratedValue Long id;
    private String name;

    @OneToMany(mappedBy = "author", cascade = CascadeType.ALL, orphanRemoval = true)
    private List<Book> books = new ArrayList<>();
}
```

**Always set `fetch = FetchType.LAZY` on `@ManyToOne` and `@OneToOne`.** The default is EAGER, which silently loads parent rows on every query.

---

## 3. Repositories — derived queries, JPQL, native

```java
package com.example.bookstore.book;

import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.*;
import org.springframework.data.repository.query.Param;
import java.util.*;

public interface BookRepository extends JpaRepository<Book, Long> {

    // Derived query — name parsed by Spring Data
    List<Book> findByAuthorOrderByTitleAsc(String author);

    // With pagination
    Page<Book> findByAuthor(String author, Pageable pageable);

    // Counting / existence
    long countByAuthor(String author);
    boolean existsByTitleIgnoreCase(String title);

    // JPQL
    @Query("select b from Book b where b.price > :min and b.author = :author")
    List<Book> expensiveBy(@Param("min") BigDecimal min, @Param("author") String author);

    // Native SQL
    @Query(value = "select * from books where lower(title) like %:q%", nativeQuery = true)
    List<Book> searchByTitle(@Param("q") String q);

    // Modifying query (update/delete)
    @Modifying
    @Query("update Book b set b.price = b.price * :factor where b.author = :author")
    int adjustPriceForAuthor(@Param("factor") BigDecimal factor, @Param("author") String author);
}
```

### Derived query keywords

| Keyword            | Example                                   |
| ------------------ | ----------------------------------------- |
| `findBy` / `getBy` | `findByTitle`                             |
| `And` / `Or`       | `findByTitleAndAuthor`                    |
| `Between`          | `findByPriceBetween`                      |
| `LessThan`         | `findByPriceLessThan`                     |
| `Like`             | `findByTitleLike`                         |
| `In`               | `findByAuthorIn`                          |
| `OrderBy`          | `findByAuthorOrderByPriceDesc`            |
| `Top` / `First`    | `findTop10ByOrderByPriceDesc`             |
| `Distinct`         | `findDistinctByAuthor`                    |

Useful for simple queries. For anything complex, switch to `@Query` — names get unreadable fast.

### Projections — avoid loading whole entities

```java
public interface BookTitleView {
    Long getId();
    String getTitle();
}

public interface BookRepository extends JpaRepository<Book, Long> {
    List<BookTitleView> findAllProjectedBy();
}
```

Hibernate selects only `id, title` — much faster when you don't need the full row.

---

## 4. Practical application — full slice

```java
// 1. Entity
@Entity
@Table(name = "books")
public class Book {
    @Id @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;
    @Column(nullable = false) private String title;
    @Column(nullable = false) private String author;
    @Column(nullable = false, precision = 10, scale = 2) private BigDecimal price;
    @Column(name = "created_at", nullable = false, updatable = false)
    private Instant createdAt = Instant.now();

    protected Book() {}
    public Book(String title, String author, BigDecimal price) {
        this.title = title; this.author = author; this.price = price;
    }
    // getters/setters
}
```

```java
// 2. Repository
public interface BookRepository extends JpaRepository<Book, Long> {
    Page<Book> findByAuthorContainingIgnoreCase(String author, Pageable pageable);
}
```

```java
// 3. DTO + mapper
public record BookDto(Long id, String title, String author, BigDecimal price) {
    public static BookDto from(Book b) {
        return new BookDto(b.getId(), b.getTitle(), b.getAuthor(), b.getPrice());
    }
}
```

```java
// 4. Service
@Service
public class BookService {
    private final BookRepository repo;
    public BookService(BookRepository repo) { this.repo = repo; }

    @Transactional(readOnly = true)
    public Page<BookDto> search(String author, Pageable pageable) {
        Page<Book> page = (author == null)
                ? repo.findAll(pageable)
                : repo.findByAuthorContainingIgnoreCase(author, pageable);
        return page.map(BookDto::from);
    }

    @Transactional
    public BookDto create(CreateBookRequest req) {
        Book saved = repo.save(new Book(req.title(), req.author(), req.price()));
        return BookDto.from(saved);
    }

    @Transactional(readOnly = true)
    public BookDto getOne(Long id) {
        return repo.findById(id).map(BookDto::from)
                .orElseThrow(() -> new BookNotFoundException(id));
    }
}
```

```java
// 5. Controller — unchanged from module 05
```

### Pagination request → response

```bash
curl "http://localhost:8080/api/v1/books?page=0&size=5&sort=price,desc&author=bloch"
```

```json
{
  "content": [
    {"id": 1, "title": "Effective Java", "author": "Joshua Bloch", "price": 49.99}
  ],
  "pageable": { "pageNumber": 0, "pageSize": 5, ... },
  "totalElements": 1,
  "totalPages": 1
}
```

---

## 5. Common Mistakes & Gotchas

- **N+1 query problem.** Iterating `List<Author>` and accessing `author.getBooks()` lazily fires one query per author. Solution: `@EntityGraph(attributePaths = "books")` on the repo method, or a `JOIN FETCH` in JPQL, or DTO projection.

  ```java
  @Query("select a from Author a left join fetch a.books where a.id in :ids")
  List<Author> findWithBooks(@Param("ids") List<Long> ids);
  ```

- **EAGER fetching everywhere.** `@ManyToOne` default is EAGER. Loading an `Order` silently loads its `Customer`, their `Address`, etc. Make everything LAZY and explicitly fetch what you need.

- **`ddl-auto: update` in production.** Hibernate's schema diff is a guess. It can drop columns, fail mid-migration, leave the schema in weird states. **Use Flyway/Liquibase** (module 09). `validate` is the only safe production setting.

- **No `@Version` field → silent lost-updates.** Two requests read the same row, both update, the second overwrites the first with stale data. `@Version` enables optimistic locking; Hibernate detects the conflict and throws `OptimisticLockException`.

- **Calling `repo.save(entity)` inside a loop without `flush()` and clearing the persistence context.** OOM on large batches. Either use Spring Batch, JDBC batch inserts, or periodically `entityManager.flush(); entityManager.clear();`.

- **Returning entities outside a transaction.** Accessing lazy associations throws `LazyInitializationException`. Either map to DTO inside the transaction or use `@Transactional(readOnly = true)` on the service method and let the open-session-in-view filter help in dev (don't rely on it in prod).

- **Using `findAll()` on tables with millions of rows.** Streams everything to the JVM. Always paginate or stream with cursor.

- **Mixing `Optional<T>` semantics.** `findById` returns `Optional<T>`. `findByXyz` returns `T` (or `null`). Or `List<T>`. Read the signature — don't assume.

- **`@Transactional` on a private or `final` method.** Spring proxies can't intercept it. The annotation is silently ignored.

---

## 🎯 Key Takeaways

- **Entity ≠ DTO.** Persistence concerns and API concerns are separate; map at the service boundary.
- **LAZY everywhere, fetch explicitly.** EAGER is a footgun.
- **`@Version` for any updatable row** that two requests might touch concurrently.
- **Pagination is mandatory** for any "list" endpoint. There's no such thing as "small enough to skip it" once your table grows.
- **`ddl-auto: validate` + Flyway/Liquibase** in production. Hibernate's automatic schema management is a learning aid, not a production tool.

*[← prev](./06_validation_and_errors.md) | [next →](./08_transactions.md)*
