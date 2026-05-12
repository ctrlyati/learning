# 13 — Caching

> **Goal:** Speed up hot reads with `@Cacheable`, evict on writes with `@CacheEvict`, swap between Caffeine (in-process) and Redis (shared) without touching annotations, and avoid the classic cache traps.

---

## 1. `@Cacheable` — mental model + working code

A cache is a key→value store between your method and its expensive backing call (DB, HTTP, computation). Spring's caching abstraction sits **above** any specific cache.

### Enable caching

```java
@SpringBootApplication
@EnableCaching
public class BookstoreApiApplication { ... }
```

### Use it

```java
@Service
public class BookService {

    private final BookRepository repo;

    public BookService(BookRepository repo) { this.repo = repo; }

    @Cacheable(value = "books", key = "#id")
    public BookDto getOne(Long id) {
        return repo.findById(id).map(BookDto::from)
            .orElseThrow(() -> new BookNotFoundException(id));
    }

    @CachePut(value = "books", key = "#result.id")
    public BookDto update(Long id, UpdateBookRequest req) {
        // update DB, return new DTO. Cache replaced.
    }

    @CacheEvict(value = "books", key = "#id")
    public void delete(Long id) {
        repo.deleteById(id);
    }

    @CacheEvict(value = "books", allEntries = true)
    public void clearCache() {}
}
```

### Default cache backend

With just `spring-boot-starter-cache`, the default is a `ConcurrentMapCacheManager` — useful for dev but unbounded (memory leak) and not shared across instances.

Add Caffeine for a real local cache:

```xml
<dependency>
    <groupId>com.github.ben-manes.caffeine</groupId>
    <artifactId>caffeine</artifactId>
</dependency>
```

```yaml
spring:
  cache:
    type: caffeine
    caffeine:
      spec: maximumSize=1000,expireAfterWrite=10m,recordStats
    cache-names: books,authors
```

---

## 2. What Spring does — yet another proxy

`@Cacheable` is, predictably, an AOP advice:

1. Caller invokes `getOne(42)`.
2. Proxy intercepts, computes the key (`SpEL` expression `#id` → `42`).
3. Asks the `CacheManager` for cache `books` → check if key `42` exists.
4. **Hit:** return cached value, don't call the real method.
5. **Miss:** call the real method, store the result under `42`, return it.

### Same proxy trap

`this.getOne(42)` bypasses the proxy → no cache. Always call from outside the bean.

### Conditional caching

```java
@Cacheable(value = "books", key = "#id", condition = "#id > 0", unless = "#result == null")
public BookDto getOne(Long id) { ... }
```

- `condition` — evaluated **before** method call; skips caching entirely if false.
- `unless` — evaluated **after** method call; skips storing if result matches.

Common pattern: `unless = "#result == null"` to avoid caching nulls.

### Composite keys

```java
@Cacheable(value = "booksByAuthor", key = "#author + ':' + #page + ':' + #size")
public Page<BookDto> findByAuthor(String author, int page, int size) { ... }
```

Or use a `KeyGenerator`:

```java
@Bean
public KeyGenerator authorPageKey() {
    return (target, method, params) -> params[0] + ":" + params[1] + ":" + params[2];
}

@Cacheable(value = "booksByAuthor", keyGenerator = "authorPageKey")
public Page<BookDto> findByAuthor(String author, int page, int size) { ... }
```

---

## 3. Caffeine vs Redis — backend depth

### Caffeine — in-process, fast, per-instance

- High-quality LRU/W-TinyLFU algorithm.
- Sub-microsecond reads.
- **Not shared across instances** — each replica has its own cache.
- Good for hot reads that don't need cross-instance consistency.

```yaml
spring:
  cache:
    type: caffeine
    caffeine:
      spec: maximumSize=10000,expireAfterWrite=5m,recordStats
```

Per-cache config:
```java
@Bean
public CacheManager cacheManager() {
    var mgr = new CaffeineCacheManager();
    mgr.registerCustomCache("books", Caffeine.newBuilder()
        .maximumSize(10_000).expireAfterWrite(Duration.ofMinutes(10)).build());
    mgr.registerCustomCache("authors", Caffeine.newBuilder()
        .maximumSize(500).expireAfterAccess(Duration.ofHours(1)).build());
    return mgr;
}
```

### Redis — shared, networked, persistent option

- Shared across all instances → consistent.
- Survives JVM restart (with persistence on).
- Higher latency than Caffeine (network hop).
- Adds operational complexity (Redis cluster, eviction tuning).

Add:
```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-data-redis</artifactId>
</dependency>
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-cache</artifactId>
</dependency>
```

Config:
```yaml
spring:
  cache:
    type: redis
    redis:
      time-to-live: 10m
      cache-null-values: false
      key-prefix: bookstore:
      use-key-prefix: true
  data:
    redis:
      host: localhost
      port: 6379
```

Per-cache TTL:
```java
@Bean
public RedisCacheManagerBuilderCustomizer redisCacheCustomizer() {
    return builder -> builder
        .withCacheConfiguration("books",
            RedisCacheConfiguration.defaultCacheConfig().entryTtl(Duration.ofMinutes(10)))
        .withCacheConfiguration("authors",
            RedisCacheConfiguration.defaultCacheConfig().entryTtl(Duration.ofHours(1)));
}
```

### Two-level cache (L1 + L2)

Caffeine for sub-ms hot path, Redis for shared fallback. Use a library like [redisson-spring-data](https://redisson.org/) or roll your own `CacheManager` composite. Often premature optimization; start with one tier.

---

## 4. Practical application — book service with cache and invalidation

```java
@Service
public class BookService {

    private final BookRepository repo;

    public BookService(BookRepository repo) { this.repo = repo; }

    @Cacheable(value = "books", key = "#id", unless = "#result == null")
    @Transactional(readOnly = true)
    public BookDto getOne(Long id) {
        return repo.findById(id).map(BookDto::from).orElse(null);
    }

    @Cacheable(value = "booksByAuthor",
               key = "#author + ':' + #pageable.pageNumber + ':' + #pageable.pageSize")
    @Transactional(readOnly = true)
    public Page<BookDto> findByAuthor(String author, Pageable pageable) {
        return repo.findByAuthorContainingIgnoreCase(author, pageable).map(BookDto::from);
    }

    @CachePut(value = "books", key = "#result.id")
    @CacheEvict(value = "booksByAuthor", allEntries = true)  // any author cache may be stale
    @Transactional
    public BookDto update(Long id, UpdateBookRequest req) {
        Book book = repo.findById(id).orElseThrow(() -> new BookNotFoundException(id));
        book.setTitle(req.title());
        book.setAuthor(req.author());
        book.setPrice(req.price());
        return BookDto.from(repo.save(book));
    }

    @Caching(evict = {
        @CacheEvict(value = "books", key = "#id"),
        @CacheEvict(value = "booksByAuthor", allEntries = true)
    })
    @Transactional
    public void delete(Long id) { repo.deleteById(id); }
}
```

### Monitoring cache hit/miss rates

Caffeine with `recordStats`:

```java
@Component
public class CacheMetrics {
    public CacheMetrics(CacheManager cm, MeterRegistry registry) {
        cm.getCacheNames().forEach(name -> {
            Cache cache = cm.getCache(name);
            if (cache != null && cache.getNativeCache() instanceof
                    com.github.benmanes.caffeine.cache.Cache<?, ?> caffeine) {
                CaffeineCacheMetrics.monitor(registry, caffeine, name);
            }
        });
    }
}
```

Now Micrometer exposes hit/miss/eviction counters on `/actuator/metrics`.

---

## 5. Common Mistakes & Gotchas

- **Caching mutable objects without serializing them.** Two threads grab the same object reference, mutate it, the cache holds the mutated state. Cache DTOs (immutable records) — never entities — and don't share references.

- **`@Cacheable` self-call.** Proxy. Bypass. No cache. You've heard this song.

- **No TTL.** Cache fills indefinitely. OOM with Caffeine; high memory cost with Redis. **Always** set a max size + expiration.

- **Caching the result of a method with side effects.** First call writes, returns. Cached. Subsequent calls return cached value, side effect doesn't happen. Use `@Cacheable` only on pure read methods.

- **Inconsistent eviction.** `update(id)` evicts the single book but you also have `booksByAuthor` cache with that book in a page. Either invalidate broadly (`allEntries = true`) or don't cache paged results.

- **Caching `null`.** Default behavior varies by backend. Always use `unless = "#result == null"` unless you specifically want negative caching.

- **Distributed caches with serialization mismatches.** Redis stores bytes; your DTO needs to deserialize cleanly. Add a class version, never break serialization compatibility, or use a versioned key prefix.

- **Cache stampede.** TTL expires → 1000 concurrent requests all miss → 1000 DB queries. Mitigations: `sync = true` on `@Cacheable` (single in-flight per key), pre-warming, request coalescing.

- **Caching across replicas without invalidation.** Replica A updates row 42 → replica A evicts → replicas B, C still serve stale. Either use a shared cache (Redis) or publish invalidation events.

- **Counting `@Cacheable` as a correctness mechanism.** It's a performance optimization. If your app **requires** the cached value to be consistent, design for that with explicit invalidation or use the DB as truth.

---

## 🎯 Key Takeaways

- **`@Cacheable` is a proxy** — same self-call rules as `@Transactional`/`@Async`.
- **TTL + max size, always.** Unbounded cache = memory leak.
- **Cache immutable DTOs**, not entities. Mutation in one place corrupts the cache.
- **Caffeine for per-instance speed; Redis for cross-instance consistency.** Pick one based on your replication model.
- **Cache invalidation is the hard part.** Be explicit. `@CacheEvict` on every write, including failed writes if needed.

*[← prev](./12_async_scheduling_events.md) | [next →](./14_observability.md)*
