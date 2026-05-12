# 13 — Transactions and Isolation Levels

> **Goal:** Understand ACID, the four standard isolation levels, how InnoDB implements them with MVCC and locks, and what deadlocks look like in practice.

---

## 1. Transactions — analogy + runnable SQL

A transaction is a sequence of statements that either all succeed or all fail — atomically. Transferring money between accounts: deduct from A, add to B. If the second step fails, the first must un-happen.

```sql
START TRANSACTION;

UPDATE account SET balance = balance - 100 WHERE id = 1;
UPDATE account SET balance = balance + 100 WHERE id = 2;

COMMIT;   -- both stick
-- ROLLBACK;  -- neither sticks
```

Without transactions, partial failures leave inconsistent data. With them, the database is a finite-state machine moving between consistent states.

---

## 2. ACID and how InnoDB implements it — mechanism

**A**tomic — all or nothing.
**C**onsistent — invariants (constraints, FKs) hold before and after.
**I**solated — concurrent transactions don't observe each other's intermediate state.
**D**urable — committed changes survive crashes.

InnoDB delivers these via:
- **Undo logs** — used for rollback and MVCC reads.
- **Redo logs** (the WAL) — used for crash recovery; flushed on commit.
- **Row-level locks** — taken on writes (and some reads).
- **MVCC** — multi-version concurrency control: readers see a consistent snapshot without blocking writers.
- **Doublewrite buffer** — protects against partial-page writes during crash.

`autocommit` is ON by default. Each statement is its own transaction unless you `START TRANSACTION` (or `BEGIN`).

```sql
-- Check
SELECT @@autocommit;
SET autocommit = 0;  -- now you must COMMIT manually
```

---

## 3. Isolation levels — depth

The SQL standard defines four levels, each preventing progressively more anomalies:

| Level             | Dirty Read | Non-Repeatable Read | Phantom Read |
|-------------------|------------|---------------------|--------------|
| READ UNCOMMITTED  | possible   | possible            | possible     |
| READ COMMITTED    | prevented  | possible            | possible     |
| REPEATABLE READ   | prevented  | prevented           | possible¹    |
| SERIALIZABLE      | prevented  | prevented           | prevented    |

¹ InnoDB's REPEATABLE READ uses **gap locks** + MVCC to prevent phantoms in most cases — stronger than the standard requires.

**InnoDB default: REPEATABLE READ.** (Postgres default: READ COMMITTED.) This matters when porting code.

```sql
SET TRANSACTION ISOLATION LEVEL READ COMMITTED;
-- or per-session:
SET SESSION TRANSACTION ISOLATION LEVEL READ COMMITTED;
```

### Anomalies — concrete examples

**Dirty read (READ UNCOMMITTED):**
```
T1: UPDATE account SET balance=0 WHERE id=1;
T2: SELECT balance FROM account WHERE id=1;  -- sees 0
T1: ROLLBACK;
-- T2 acted on data that never existed
```

**Non-repeatable read (READ COMMITTED):**
```
T1: SELECT balance FROM account WHERE id=1;  -- 100
T2: UPDATE account SET balance=200 WHERE id=1; COMMIT;
T1: SELECT balance FROM account WHERE id=1;  -- 200, different!
```

**Phantom read (REPEATABLE READ in standard SQL):**
```
T1: SELECT COUNT(*) FROM order WHERE status='new';   -- 5
T2: INSERT INTO order(status) VALUES ('new'); COMMIT;
T1: SELECT COUNT(*) FROM order WHERE status='new';   -- 6, a phantom appeared
```
InnoDB's REPEATABLE READ uses gap locks on read-with-write or `SELECT ... FOR UPDATE` to prevent this. Plain SELECTs see a consistent snapshot via MVCC.

### MVCC — how readers don't block writers

Every row in InnoDB has hidden columns: `DB_TRX_ID` (transaction that wrote it), `DB_ROLL_PTR` (pointer to undo log entry).

When a transaction starts (REPEATABLE READ), InnoDB snapshots the system. Reads find row versions visible to that snapshot (using undo logs to reconstruct older versions).

Writers don't block readers; readers don't block writers. Only writer-vs-writer on the same row blocks (via row locks).

### Locks

InnoDB takes various locks:
- **Record lock** — locks the index row.
- **Gap lock** — locks the *space between* index records (prevents inserts).
- **Next-key lock** — record + gap before it (REPEATABLE READ default).
- **Insert intention lock** — a special gap lock for inserts.
- **Table lock** — DDL, LOCK TABLES.

Explicit row locks:
```sql
SELECT * FROM account WHERE id = 1 FOR UPDATE;     -- exclusive lock
SELECT * FROM account WHERE id = 1 FOR SHARE;      -- shared (read) lock
```

### Deadlocks

Two transactions, each waiting on a lock the other holds. InnoDB detects deadlocks and **rolls back the smaller transaction**, returning error `1213 (40001) Deadlock found`.

Classic example:
```
T1: UPDATE account SET balance = balance-100 WHERE id=1;  -- locks row 1
T2: UPDATE account SET balance = balance-50  WHERE id=2;  -- locks row 2
T1: UPDATE account SET balance = balance+100 WHERE id=2;  -- waits on T2
T2: UPDATE account SET balance = balance+50  WHERE id=1;  -- waits on T1 → DEADLOCK
```

Mitigations:
- **Acquire locks in a consistent global order** (e.g., always lower id first).
- **Keep transactions short.**
- **Retry on deadlock** — application-level retry loop is the standard pattern.

Inspect deadlocks:
```sql
SHOW ENGINE INNODB STATUS;
-- look for LATEST DETECTED DEADLOCK section
```

---

## 4. Practical application — money transfer with retry

```sql
-- Schema
CREATE TABLE account (
  id      INT PRIMARY KEY,
  balance DECIMAL(12,2) NOT NULL,
  CHECK (balance >= 0)
);
INSERT INTO account VALUES (1, 1000), (2, 500);

-- Transfer (pseudocode wrapping a transaction)
START TRANSACTION;

-- Lock both rows in id order to avoid deadlocks
SELECT balance FROM account WHERE id = 1 FOR UPDATE;
SELECT balance FROM account WHERE id = 2 FOR UPDATE;

UPDATE account SET balance = balance - 100 WHERE id = 1;
UPDATE account SET balance = balance + 100 WHERE id = 2;

COMMIT;
```

Application code wraps this in a retry loop on error 1213.

### Inventory deduction with optimistic concurrency

For high-contention spots, **optimistic locking** beats pessimistic:
```sql
-- Read current version
SELECT qty, version FROM inventory WHERE sku='A1';

-- Update only if version unchanged
UPDATE inventory
SET qty = qty - 1, version = version + 1
WHERE sku='A1' AND version = :v_old;

-- If 0 rows affected → someone else won; retry from read.
```

### Row visibility experiment

In two CLI sessions:
```sql
-- Session A:
SET SESSION TRANSACTION ISOLATION LEVEL REPEATABLE READ;
START TRANSACTION;
SELECT balance FROM account WHERE id=1;  -- 1000

-- Session B:
UPDATE account SET balance=2000 WHERE id=1; COMMIT;

-- Session A again (same transaction):
SELECT balance FROM account WHERE id=1;  -- still 1000! MVCC snapshot.
COMMIT;
SELECT balance FROM account WHERE id=1;  -- now 2000
```

Switch session A to `READ COMMITTED` and the second query returns 2000 immediately. This is the level you want when freshness matters more than internal consistency.

---

## 5. Common Mistakes & Gotchas

- **Forgetting to COMMIT.** Some clients leave transactions open silently. Idle transactions hold locks and undo segments — terrible for concurrency.
- **Long-running transactions.** They prevent purge of old row versions (undo grows unboundedly), and hold locks for ages.
- **Not retrying on deadlock.** Error 1213 is *expected*. Production code must catch and retry.
- **`SELECT FOR UPDATE` outside a transaction.** With autocommit on, the lock is released immediately — useless.
- **Mixing isolation levels in one transaction.** Set at the start; don't fiddle midway.
- **DDL inside a transaction.** MySQL auto-commits before DDL (no transactional DDL). Surprise commits = surprise visibility.
- **Assuming defaults.** MySQL = REPEATABLE READ; Postgres = READ COMMITTED. Driver/library may set its own default.
- **Using `LOCK TABLES`.** A relic from MyISAM days. With InnoDB you almost never want it.
- **Phantom-read confusion.** InnoDB's REPEATABLE READ usually prevents them via gap locks, contrary to the SQL spec. Don't assume cross-DB behavior.
- **vs. Postgres:** Postgres has SERIALIZABLE that's truly serializable (SSI). InnoDB's SERIALIZABLE is just REPEATABLE READ + auto-promotion of SELECTs to FOR SHARE. Different mechanisms, similar guarantees in most cases.

---

## 🎯 Key Takeaways

- **Atomicity isn't free.** Wrap multi-step operations in `START TRANSACTION`/`COMMIT` deliberately.
- **InnoDB default is REPEATABLE READ.** Different from Postgres. Know what your code assumes.
- **MVCC means readers don't block writers.** Snapshot isolation at the row-version level is what makes InnoDB scale.
- **Deadlocks are expected.** Order your lock acquisitions, keep transactions short, retry on 1213.
- **Long-running transactions are silent killers.** Idle-in-transaction connections are a top-3 cause of MySQL outages.

*← [12 optimization](./12_query_optimization.md) | [next → Procedures, Triggers, Events](./14_procedures_triggers.md)*
