# 12 — Async, Scheduling & Events

> **Goal:** Run work off the request thread with `@Async`, schedule recurring tasks with `@Scheduled`, and decouple components with `ApplicationEvents` — including the under-used `@TransactionalEventListener`.

---

## 1. `@Async` — mental model + working code

`@Async` runs a method on a different thread, returning immediately (or returning a `CompletableFuture` you can compose).

### Enable it

```java
@SpringBootApplication
@EnableAsync
public class BookstoreApiApplication { ... }
```

### Use it

```java
@Service
public class EmailService {

    @Async
    public void sendOrderConfirmation(Order order) {
        // takes a few seconds — runs on a thread-pool thread
        // caller returns immediately
    }

    @Async
    public CompletableFuture<EnrichedOrder> enrich(Order order) {
        // async with a return value the caller can await
        return CompletableFuture.completedFuture(...);
    }
}
```

### Configure the executor

The default `SimpleAsyncTaskExecutor` makes one thread per call — disaster under load. **Always** define your own:

```java
@Configuration
@EnableAsync
public class AsyncConfig implements AsyncConfigurer {

    @Override
    public Executor getAsyncExecutor() {
        var executor = new ThreadPoolTaskExecutor();
        executor.setCorePoolSize(8);
        executor.setMaxPoolSize(32);
        executor.setQueueCapacity(200);
        executor.setThreadNamePrefix("bookstore-async-");
        executor.setRejectedExecutionHandler(new ThreadPoolExecutor.CallerRunsPolicy());
        executor.initialize();
        return executor;
    }

    @Override
    public AsyncUncaughtExceptionHandler getAsyncUncaughtExceptionHandler() {
        return (ex, method, params) ->
            log.error("Async failure in {}", method.getName(), ex);
    }
}
```

### Multiple named executors

```java
@Bean("emailExecutor")
public Executor emailExecutor() { ... }

@Bean("reportingExecutor")
public Executor reportingExecutor() { ... }

@Async("emailExecutor")
public void sendEmail(...) { ... }

@Async("reportingExecutor")
public void generateReport(...) { ... }
```

Different queues, different SLAs, isolated failures.

---

## 2. What Spring does — proxies again

`@Async` is, like `@Transactional`, **AOP via proxy**. The proxy submits a `Callable`/`Runnable` to the executor and returns immediately.

### Consequences

- **Self-call doesn't async.** `this.sendEmail()` inside the same bean → synchronous. Same as `@Transactional`.
- **Must be a public method.** Private methods aren't proxied.
- **Return type must be `void`, `Future`, `CompletableFuture`, or `ListenableFuture`.** Spring complains otherwise.
- **`Future`/`CompletableFuture` is the *only* way to surface async exceptions to the caller.** `void` returns drop them; only the `AsyncUncaughtExceptionHandler` sees them.

---

## 3. `@Scheduled` — cron, fixed-rate, fixed-delay

Enable:

```java
@SpringBootApplication
@EnableScheduling
public class BookstoreApiApplication { ... }
```

### Three timing styles

```java
@Component
public class Schedules {

    // Run every 5 minutes, regardless of previous run duration
    @Scheduled(fixedRate = 5 * 60 * 1000)
    public void heartbeat() { ... }

    // Wait 60s AFTER previous run completes before next start
    @Scheduled(fixedDelay = 60_000)
    public void poll() { ... }

    // Cron: every day at 3:00 AM in Europe/Berlin
    @Scheduled(cron = "0 0 3 * * *", zone = "Europe/Berlin")
    public void nightlyReport() { ... }

    // Wait until ready, then start
    @Scheduled(initialDelay = 30_000, fixedRate = 60_000)
    public void delayedHeartbeat() { ... }
}
```

### The default scheduler is single-threaded

`TaskSchedulingAutoConfiguration` gives you **one** thread by default. If `heartbeat` runs every 5 min and takes 6 min, `poll` is starved. Configure a thread pool:

```yaml
spring:
  task:
    scheduling:
      pool:
        size: 4
      thread-name-prefix: bookstore-sched-
```

### Cron expressions in Spring (6 fields)

```
second  minute  hour  day-of-month  month  day-of-week
   *      *      *        *           *         *
```

Special: `0 */15 * * * *` = every 15 min. `0 0 9 * * MON-FRI` = 9 AM on weekdays.

### Scheduled tasks in a multi-instance deployment

If three replicas all have `@Scheduled`, the job runs three times. Solutions:

- **Distributed lock** (Redis, JDBC) via [ShedLock](https://github.com/lukas-krecan/ShedLock):
  ```java
  @Scheduled(cron = "0 0 3 * * *")
  @SchedulerLock(name = "nightlyReport", lockAtLeastFor = "PT1M", lockAtMostFor = "PT10M")
  public void nightlyReport() { ... }
  ```
- **Run on a dedicated single-replica job pod** (Kubernetes `CronJob` or a separate Spring Boot service).

---

## 4. Application events — decoupling with publish/subscribe

A bean publishes; zero or more beans listen. No direct dependency.

```java
// 1. The event
public record OrderPlacedEvent(Long orderId, String userId, BigDecimal total) {}

// 2. Publisher
@Service
public class OrderService {

    private final ApplicationEventPublisher events;

    public OrderService(ApplicationEventPublisher events) {
        this.events = events;
    }

    @Transactional
    public Order placeOrder(...) {
        // ... persist order
        events.publishEvent(new OrderPlacedEvent(order.getId(), userId, order.getTotal()));
        return order;
    }
}

// 3. Listener — anywhere in the app
@Component
public class EmailListener {

    @EventListener
    public void onOrderPlaced(OrderPlacedEvent e) {
        // send confirmation email — synchronous, in the publisher's thread
    }
}

// 4. Async listener
@Component
public class AnalyticsListener {

    @Async
    @EventListener
    public void onOrderPlaced(OrderPlacedEvent e) {
        // analytics push — separate thread
    }
}
```

### Transactional event listeners

The deadly bug: publish event inside transaction → listener runs **inside** the same transaction → if listener fails, transaction rolls back; if transaction rolls back **after** listener succeeded, listener's side effects (email sent) can't be undone.

The fix:

```java
@TransactionalEventListener(phase = TransactionPhase.AFTER_COMMIT)
public void onOrderPlaced(OrderPlacedEvent e) {
    // only runs if the publishing transaction committed successfully
}
```

Phases: `BEFORE_COMMIT`, `AFTER_COMMIT` (default), `AFTER_ROLLBACK`, `AFTER_COMPLETION`.

This is the **right** pattern for "do this after the order is durably saved" — emails, analytics, downstream calls.

### Practical application — order placement with all three

```java
@Service
public class OrderService {

    private final OrderRepository repo;
    private final ApplicationEventPublisher events;

    public OrderService(OrderRepository repo, ApplicationEventPublisher events) {
        this.repo = repo;
        this.events = events;
    }

    @Transactional
    public Order placeOrder(Long userId, Long bookId, int qty) {
        Order order = repo.save(new Order(userId, bookId, qty));
        events.publishEvent(new OrderPlacedEvent(order.getId(), userId, order.getTotal()));
        return order;
    }
}

@Component
class OrderListeners {

    @Async
    @TransactionalEventListener(phase = TransactionPhase.AFTER_COMMIT)
    public void sendEmail(OrderPlacedEvent e) {
        // off the request thread + only after commit
    }

    @Async
    @TransactionalEventListener(phase = TransactionPhase.AFTER_COMMIT)
    public void pushAnalytics(OrderPlacedEvent e) { ... }

    @TransactionalEventListener(phase = TransactionPhase.AFTER_ROLLBACK)
    public void recordFailure(OrderPlacedEvent e) {
        // capture the abort
    }
}

@Component
class NightlyReports {

    @Scheduled(cron = "0 0 3 * * *", zone = "UTC")
    @SchedulerLock(name = "dailyOrderSummary", lockAtMostFor = "PT30M")
    public void emailDailySummary() { ... }
}
```

That's a real pattern for the order pipeline: synchronous DB write, asynchronous post-commit side effects, scheduled batch on top.

---

## 5. Common Mistakes & Gotchas

- **`@Async` self-call.** Same proxy trap as `@Transactional`. Call from outside the bean.

- **Using the default `SimpleAsyncTaskExecutor`.** Unbounded threads. Production load = OOM. Always supply a configured `ThreadPoolTaskExecutor`.

- **Thread-pool exhaustion blocking the request thread.** With `CallerRunsPolicy`, when the queue is full, the calling thread runs the task — which on a request thread means the user waits. Decide which is worse for your case: drop, queue, or block.

- **No timeout on async work.** `CompletableFuture.get()` without a timeout = hung threads. Always `get(5, TimeUnit.SECONDS)` or use the timeout-aware combinators.

- **Side effects in `@EventListener` without `@TransactionalEventListener`.** Email goes out, transaction rolls back, customer gets a confirmation for an order that doesn't exist.

- **`@Scheduled` on all replicas in production.** Three pods = three runs = three duplicates. Use ShedLock or a separate scheduler service.

- **Mixing scheduling and request-scoped beans.** Scheduled methods run outside any HTTP request. Injecting a request-scoped bean throws.

- **Long-running `@Scheduled` blocking the scheduler.** With pool size = 1, all schedules queue behind it. Either increase pool size or move long work to `@Async`.

- **`@Async` with `@Transactional` on the same method.** The transaction interceptor and async interceptor are both proxy advice. Subtle ordering issues. Best practice: split — caller is `@Transactional`, async method is plain.

- **Forgetting that `@Async void` swallows exceptions.** Use `CompletableFuture<...>` return type to propagate, or register a custom `AsyncUncaughtExceptionHandler` to log.

---

## 🎯 Key Takeaways

- **`@Async` requires a configured executor.** The default is a footgun.
- **`@Scheduled` defaults to one thread.** Tune the pool size to your job count.
- **`@TransactionalEventListener(AFTER_COMMIT)` is the right place for post-write side effects.** Memorize this pattern.
- **Both async and scheduling are AOP/proxy.** Self-calls and private methods don't work.
- **Multi-instance deployments need leader election or distributed locks** for any "run once" scheduled job.

*[← prev](./11_testing.md) | [next →](./13_caching.md)*
