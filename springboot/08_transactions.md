# 08 — Transactions

> **Goal:** Understand `@Transactional` at the proxy/AOP level, control propagation and isolation deliberately, and avoid the self-call trap that breaks transactions silently.

---

## 1. `@Transactional` — mental model + working code

A transaction is a unit of work that either fully succeeds or fully rolls back. In Spring you mark **a method or class** with `@Transactional` and the framework opens a transaction before the method, commits on normal return, rolls back on `RuntimeException`.

```java
package com.example.bookstore.order;

import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
public class OrderService {

    private final BookRepository bookRepo;
    private final OrderRepository orderRepo;

    public OrderService(BookRepository bookRepo, OrderRepository orderRepo) {
        this.bookRepo = bookRepo;
        this.orderRepo = orderRepo;
    }

    @Transactional
    public Order placeOrder(Long bookId, int quantity) {
        Book book = bookRepo.findById(bookId).orElseThrow();
        if (book.getStock() < quantity) {
            throw new InsufficientStockException();   // → rollback
        }
        book.setStock(book.getStock() - quantity);
        bookRepo.save(book);

        Order order = new Order(bookId, quantity, book.getPrice().multiply(BigDecimal.valueOf(quantity)));
        return orderRepo.save(order);
    }
}
```

Either both rows change (decremented stock + new order) or neither. That's atomicity.

---

## 2. The proxy mechanism — what Spring actually does

Spring `@Transactional` is **not magic**. It's **AOP via proxies**:

1. At startup, Spring sees `@Transactional` on `OrderService.placeOrder`.
2. It wraps the bean in a **CGLIB proxy** (for classes) or **JDK dynamic proxy** (for interfaces).
3. When another bean injects `OrderService`, it actually gets the **proxy**.
4. Calling `proxy.placeOrder(...)` runs the interceptor: `PlatformTransactionManager.getTransaction()` → invoke real method → `commit()` or `rollback()`.

### The fatal consequence: self-calls bypass the proxy

```java
@Service
public class OrderService {

    public void publicEntry(Long bookId, int qty) {
        this.placeOrder(bookId, qty);   // ← bypasses the proxy. NO TRANSACTION.
    }

    @Transactional
    public Order placeOrder(Long bookId, int qty) { ... }
}
```

`this.placeOrder` is a normal Java method call. The transactional interceptor lives on the proxy, not on `this`. **No transaction starts.** Silent. Production-breaking. Untestable without an integration test that asserts rollback.

### Fixes

1. **Move `placeOrder` to a different bean** that gets injected, so the call goes through the proxy.
2. **Self-inject** (anti-pattern but works):
   ```java
   @Autowired private OrderService self;
   public void publicEntry(...) { self.placeOrder(...); }
   ```
3. **Programmatic transactions** with `TransactionTemplate`.

---

## 3. Propagation, isolation, rollback rules

### Propagation — how nested calls interact

| Propagation     | Behavior                                                                  |
| --------------- | ------------------------------------------------------------------------- |
| `REQUIRED` (default) | Join existing tx, or create one if none exists                       |
| `REQUIRES_NEW`  | Suspend existing, always start a new (and independent) tx                 |
| `MANDATORY`     | Must join existing; throw if no tx                                        |
| `SUPPORTS`      | Join if exists; otherwise run non-transactionally                         |
| `NOT_SUPPORTED` | Suspend existing; run non-transactionally                                 |
| `NEVER`         | Throw if a tx exists                                                      |
| `NESTED`        | Savepoint within outer tx (requires JDBC nested tx support)               |

Example — audit log shouldn't rollback with the business tx:

```java
@Service
public class OrderService {
    @Autowired private AuditService audit;

    @Transactional
    public Order placeOrder(...) {
        // business work...
        audit.logOrderPlaced(...);   // own REQUIRES_NEW tx
        // if business work fails after this, audit row still persists
        return order;
    }
}

@Service
public class AuditService {
    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void logOrderPlaced(...) { ... }
}
```

### Isolation — concurrent access guarantees

| Isolation         | Prevents                                                                |
| ----------------- | ----------------------------------------------------------------------- |
| `READ_UNCOMMITTED`| Nothing (don't use)                                                     |
| `READ_COMMITTED`  | Dirty reads (Postgres default)                                          |
| `REPEATABLE_READ` | Dirty + non-repeatable reads (MySQL default)                            |
| `SERIALIZABLE`    | All anomalies, lowest concurrency                                       |

```java
@Transactional(isolation = Isolation.REPEATABLE_READ)
public void transferFunds(...) { ... }
```

99% of business code can use the database default. Reach for higher isolation only for specific consistency problems and **measure the contention cost**.

### Rollback rules

By default Spring rolls back on:
- Any **unchecked exception** (`RuntimeException` and subclasses)
- `Error`

It **commits** on **checked exceptions** (`Exception` subclasses). This trips people up:

```java
@Transactional
public void doWork() throws BusinessException {  // checked
    throw new BusinessException("nope");          // → COMMITS!
}
```

Fix:

```java
@Transactional(rollbackFor = BusinessException.class)
public void doWork() throws BusinessException { ... }

// Or alternately
@Transactional(rollbackFor = Exception.class)
public void doWork() throws Exception { ... }
```

### Read-only optimization

```java
@Transactional(readOnly = true)
public List<BookDto> search(...) { ... }
```

Hibernate disables dirty-checking and flushing. Faster, and a useful intent marker.

---

## 4. Practical application — a transfer between accounts

```java
package com.example.bookstore.account;

import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.*;
import java.math.BigDecimal;

@Service
public class AccountTransferService {

    private final AccountRepository accountRepo;
    private final LedgerService ledger;
    private final NotificationService notifications;

    public AccountTransferService(AccountRepository accountRepo,
                                  LedgerService ledger,
                                  NotificationService notifications) {
        this.accountRepo = accountRepo;
        this.ledger = ledger;
        this.notifications = notifications;
    }

    @Transactional(
        propagation = Propagation.REQUIRED,
        isolation = Isolation.REPEATABLE_READ,
        rollbackFor = TransferException.class,
        timeout = 5    // seconds
    )
    public TransferResult transfer(Long fromId, Long toId, BigDecimal amount)
            throws TransferException {

        Account from = accountRepo.findByIdForUpdate(fromId)
                .orElseThrow(() -> new TransferException("source missing"));
        Account to = accountRepo.findByIdForUpdate(toId)
                .orElseThrow(() -> new TransferException("destination missing"));

        if (from.getBalance().compareTo(amount) < 0) {
            throw new TransferException("insufficient funds");
        }

        from.debit(amount);
        to.credit(amount);
        accountRepo.save(from);
        accountRepo.save(to);

        // Ledger runs in its own tx — audit must survive even on rollback elsewhere
        ledger.record(fromId, toId, amount);

        // Notification is fire-and-forget — must not affect the tx
        notifications.queueTransferNotification(fromId, toId, amount);

        return new TransferResult(from.getId(), to.getId(), amount);
    }
}

@Service
class LedgerService {
    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void record(Long from, Long to, BigDecimal amount) { ... }
}

@Service
class NotificationService {
    // No @Transactional — sends to a queue
    public void queueTransferNotification(Long from, Long to, BigDecimal amount) { ... }
}
```

### The pessimistic lock

```java
public interface AccountRepository extends JpaRepository<Account, Long> {

    @Lock(LockModeType.PESSIMISTIC_WRITE)
    @Query("select a from Account a where a.id = :id")
    Optional<Account> findByIdForUpdate(@Param("id") Long id);
}
```

Generates `SELECT ... FOR UPDATE`. Other transactions trying to lock the same row wait or fail (depending on isolation).

---

## 5. Common Mistakes & Gotchas

- **Self-call ignoring `@Transactional`.** The #1 production bug. Cover with an integration test that asserts rollback on a method called from outside the bean.

- **`@Transactional` on a private method.** Proxies can only intercept public methods. The annotation is silently ignored. No warning.

- **`@Transactional` on a `final` class or `final` method.** CGLIB can't subclass to proxy. Spring may throw at startup; older versions may silently skip.

- **Checked exceptions don't trigger rollback by default.** If you throw a checked exception expecting rollback, you'll commit instead. Always specify `rollbackFor`.

- **Catching exceptions inside a `@Transactional` method.** The tx interceptor only sees thrown exceptions. Catch-and-swallow → commit. If you must catch, set `TransactionAspectSupport.currentTransactionStatus().setRollbackOnly()`.

- **Long-running transactions.** A 30-second transaction holds locks, fills the undo log, starves other writers. Keep transactions short; do I/O (HTTP, file) **outside**.

- **Mixing JPA and JDBC without configuring shared tx.** Spring can manage both with `JpaTransactionManager`, but if you use `JdbcTemplate` directly and forget the manager binding, you get two separate transactions. Use the same `DataSource` and let Spring's auto-config wire it.

- **Calling external services from inside a transaction.** HTTP latency = long transaction = held locks. Send domain events post-commit (Spring's `@TransactionalEventListener(phase = AFTER_COMMIT)`).

- **`REQUIRES_NEW` without understanding cost.** Suspending and starting a new transaction means a second DB connection. Connection pool exhaustion under load.

- **Assuming `readOnly = true` is enforced.** It's a *hint* for Hibernate. The DB still gets normal SQL; if you accidentally call `save`, it'll still try to write.

---

## 🎯 Key Takeaways

- **`@Transactional` is a proxy.** Self-calls bypass it. Internalize this or be bitten by it.
- **Default rollback is on `RuntimeException` only.** Checked exceptions need `rollbackFor`.
- **Keep transactions short, push I/O out.** Long transactions are the silent killer of throughput.
- **Use `REQUIRES_NEW` deliberately for audit/log writes** that must survive business rollbacks.
- **`@Transactional(readOnly = true)` on read paths** — faster and clearer intent.

*[← prev](./07_spring_data_jpa.md) | [next →](./09_database_migrations.md)*
