# 14 — Stored Procedures, Functions, Triggers, Events

> **Goal:** Use server-side code for the right reasons (and avoid it for the wrong ones). Know the syntax, the lifecycle, and the operational tradeoffs.

---

## 1. Server-side code — analogy + runnable SQL

The database can run code. Stored procedures are like saved scripts inside the DB; functions return a value; triggers fire automatically on row changes; events are scheduled jobs.

```sql
DELIMITER $$
CREATE PROCEDURE give_raise(IN dept INT, IN pct DECIMAL(5,2))
BEGIN
  UPDATE employee SET salary = salary * (1 + pct/100) WHERE dept_id = dept;
END$$
DELIMITER ;

CALL give_raise(3, 5.0);
```

Two things to notice: the `DELIMITER` dance (so MySQL doesn't terminate the procedure body on the first `;`) and the `CALL` to invoke.

---

## 2. The four primitives — mechanism

### Stored procedure
- Multi-statement; can take IN/OUT/INOUT parameters.
- Can have variables, control flow (IF, CASE, LOOP, WHILE, REPEAT), cursors, exception handlers.
- Invoked with `CALL`.
- Doesn't return a single value — can produce result sets.

```sql
DELIMITER $$
CREATE PROCEDURE find_or_create_customer(IN p_email VARCHAR(255), OUT p_id INT)
BEGIN
  SELECT customer_id INTO p_id FROM customer WHERE email = p_email;
  IF p_id IS NULL THEN
    INSERT INTO customer(email) VALUES (p_email);
    SET p_id = LAST_INSERT_ID();
  END IF;
END$$
DELIMITER ;

CALL find_or_create_customer('x@y.com', @id);
SELECT @id;
```

### Stored function
- Returns one scalar value.
- Usable in expressions (SELECT, WHERE).
- Subject to `DETERMINISTIC` / `READS SQL DATA` / `MODIFIES SQL DATA` declarations.

```sql
DELIMITER $$
CREATE FUNCTION full_name(p_first VARCHAR(50), p_last VARCHAR(50))
RETURNS VARCHAR(101)
DETERMINISTIC
BEGIN
  RETURN CONCAT(p_first, ' ', p_last);
END$$
DELIMITER ;

SELECT full_name(first_name, last_name) FROM customer LIMIT 5;
```

### Trigger
- Fires automatically on `BEFORE`/`AFTER` `INSERT`/`UPDATE`/`DELETE` for each row.
- Refers to old/new values via `OLD.col`/`NEW.col`.
- Cannot be invoked directly.

```sql
CREATE TABLE customer_audit (
  audit_id   INT AUTO_INCREMENT PRIMARY KEY,
  customer_id INT,
  changed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  old_email  VARCHAR(255),
  new_email  VARCHAR(255)
);

DELIMITER $$
CREATE TRIGGER trg_customer_email_audit
AFTER UPDATE ON customer
FOR EACH ROW
BEGIN
  IF OLD.email <> NEW.email THEN
    INSERT INTO customer_audit(customer_id, old_email, new_email)
    VALUES (NEW.customer_id, OLD.email, NEW.email);
  END IF;
END$$
DELIMITER ;
```

### Event (scheduled job)
- Runs on a schedule via the event scheduler.
- Useful for periodic cleanup, materialized-view refresh.

```sql
SET GLOBAL event_scheduler = ON;

CREATE EVENT ev_purge_old_logs
ON SCHEDULE EVERY 1 DAY STARTS '2026-05-12 02:00:00'
DO
  DELETE FROM event_log WHERE created_at < NOW() - INTERVAL 90 DAY;

SHOW EVENTS;
DROP EVENT ev_purge_old_logs;
```

---

## 3. When to use them — depth and tradeoffs

### Reasons to use server-side code

- **Atomicity for multi-statement operations** that don't fit cleanly into one statement.
- **Reducing round-trips** — one CALL instead of N queries from app.
- **Auditing/historical change tracking** — triggers are the canonical way.
- **Enforcing complex invariants** that can't be expressed as constraints.
- **Periodic maintenance** without a separate cron infrastructure (events).

### Reasons NOT to

- **Logic lives outside version control by default.** Schema migration tools mitigate this.
- **Hard to test** vs. application code.
- **Hidden side-effects** — a trigger firing on every UPDATE can confuse new team members for weeks.
- **Performance opaque** — EXPLAIN doesn't dive into procedures.
- **Vendor lock-in** — porting MySQL procedures to Postgres or back is painful.
- **Debug ergonomics are dire** — `SIGNAL SQLSTATE` and printf are basically your tools.

Modern consensus (Planetscale, GitHub, Stripe-style shops): keep DB logic minimal. Use views, functions for trivial helpers, triggers only for audit tables. Put business logic in the app.

### Error handling — handlers and SIGNAL

```sql
DELIMITER $$
CREATE PROCEDURE safe_transfer(IN p_from INT, IN p_to INT, IN p_amt DECIMAL(10,2))
BEGIN
  DECLARE EXIT HANDLER FOR SQLEXCEPTION
  BEGIN
    ROLLBACK;
    RESIGNAL;
  END;

  START TRANSACTION;
  UPDATE account SET balance = balance - p_amt WHERE id = p_from;
  IF (SELECT balance FROM account WHERE id = p_from) < 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'Insufficient funds';
  END IF;
  UPDATE account SET balance = balance + p_amt WHERE id = p_to;
  COMMIT;
END$$
DELIMITER ;
```

### Cursors (for row-by-row work — usually a smell)

```sql
DELIMITER $$
CREATE PROCEDURE walk_films()
BEGIN
  DECLARE done INT DEFAULT 0;
  DECLARE v_title VARCHAR(200);
  DECLARE cur CURSOR FOR SELECT title FROM film LIMIT 5;
  DECLARE CONTINUE HANDLER FOR NOT FOUND SET done = 1;

  OPEN cur;
  read_loop: LOOP
    FETCH cur INTO v_title;
    IF done THEN LEAVE read_loop; END IF;
    -- do something with v_title
  END LOOP;
  CLOSE cur;
END$$
DELIMITER ;
```
Cursors are O(N) round-trips inside the engine. Almost always a set-based query is better.

### Inspecting and managing
```sql
SHOW PROCEDURE STATUS WHERE Db = 'sakila';
SHOW CREATE PROCEDURE give_raise;
DROP PROCEDURE give_raise;
SHOW TRIGGERS FROM sakila;
SHOW EVENTS;
```

---

## 4. Practical application — Sakila audit and aggregation

```sql
-- 1. Trigger: maintain a denormalized rental_count on customer
ALTER TABLE customer ADD COLUMN rental_count INT NOT NULL DEFAULT 0;

UPDATE customer c SET rental_count = (
  SELECT COUNT(*) FROM rental r WHERE r.customer_id = c.customer_id
);

DELIMITER $$
CREATE TRIGGER trg_rental_ai
AFTER INSERT ON rental
FOR EACH ROW
  UPDATE customer SET rental_count = rental_count + 1 WHERE customer_id = NEW.customer_id;

CREATE TRIGGER trg_rental_ad
AFTER DELETE ON rental
FOR EACH ROW
  UPDATE customer SET rental_count = rental_count - 1 WHERE customer_id = OLD.customer_id;
$$
DELIMITER ;

-- 2. Function: compute "active" status from rental recency
DELIMITER $$
CREATE FUNCTION is_recently_active(p_customer_id INT)
RETURNS TINYINT(1)
READS SQL DATA
BEGIN
  RETURN (SELECT IF(MAX(rental_date) >= NOW() - INTERVAL 30 DAY, 1, 0)
          FROM rental WHERE customer_id = p_customer_id);
END$$
DELIMITER ;

SELECT customer_id, is_recently_active(customer_id) AS active_30d
FROM customer LIMIT 10;

-- 3. Event: nightly rebuild of a daily revenue summary
CREATE TABLE daily_revenue (
  day DATE PRIMARY KEY,
  revenue DECIMAL(12,2)
);

CREATE EVENT ev_refresh_daily_revenue
ON SCHEDULE EVERY 1 DAY STARTS '2026-05-12 02:00:00'
DO
  REPLACE INTO daily_revenue
  SELECT DATE(payment_date), SUM(amount)
  FROM payment
  GROUP BY DATE(payment_date);
```

---

## 5. Common Mistakes & Gotchas

- **Triggers that do too much.** A simple INSERT becomes a transactional cascade firing 5 triggers. Performance and debuggability suffer.
- **Trigger order is undefined** between multiple triggers on the same event (in old MySQL). 5.7.2+ allows `FOLLOWS`/`PRECEDES`.
- **Recursive triggers.** Trigger A updates table B, which has trigger A' that updates A — infinite loop or unintended cascade. Default `max_sp_recursion_depth = 0` blocks recursion, but cross-trigger chains slip through.
- **Functions called per-row in WHERE/SELECT** kill performance — they're not inlined, and each call is a separate context switch.
- **Not declaring DETERMINISTIC.** Without it, replication may refuse to log statements using the function (binlog format issue).
- **DDL inside procedures auto-commits.** A procedure that mixes DML and DDL has weird transaction boundaries.
- **Forgetting `DELIMITER`.** The CREATE PROCEDURE statement gets cut at the first `;` inside the body.
- **Not version-controlling procedures.** Use migration tools (Flyway, Liquibase, dbmate) so procedures live in your repo.
- **Event scheduler turned off.** `event_scheduler = ON` must be set for events to run.
- **vs. Postgres:** PL/pgSQL is much more capable than MySQL's procedural SQL. If you're doing serious server-side logic, Postgres is the friendlier engine.

---

## 🎯 Key Takeaways

- **Use sparingly.** Server-side code is opaque, hard to test, and hides side effects from new engineers.
- **Triggers for auditing** is the one near-universal good fit.
- **Events replace cron** for DB-local maintenance (purges, summary refresh).
- **Always declare DETERMINISTIC / READS SQL DATA** on functions — affects replication and the optimizer.
- **Keep procedures in version control** via migrations, not just in the database.

*← [13 transactions](./13_transactions_isolation.md) | [next → Backup, Restore, Replication](./15_backup_replication.md)*
