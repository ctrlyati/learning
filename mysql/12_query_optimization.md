# 12 — Query Optimization: EXPLAIN, EXPLAIN ANALYZE, hints, rewrites

> **Goal:** Read query plans, identify the bottleneck, and rewrite or hint your way to a fast query.

---

## 1. EXPLAIN — analogy + runnable SQL

EXPLAIN is the optimizer narrating its plan to you. "I will scan table X this way, then join to Y like that, using these indexes, expecting this many rows." If you're tuning a slow query without EXPLAIN, you're guessing.

```sql
EXPLAIN
SELECT c.first_name, c.last_name, COUNT(*) AS rentals
FROM customer c
JOIN rental r ON r.customer_id = c.customer_id
GROUP BY c.customer_id
ORDER BY rentals DESC
LIMIT 10;
```

Output (tabular form):
```
id | select_type | table | type | possible_keys     | key                 | rows | Extra
1  | SIMPLE      | c     | ALL  | PRIMARY           | NULL                | 599  | Using temporary; Using filesort
1  | SIMPLE      | r     | ref  | idx_fk_customer_id| idx_fk_customer_id  | 26   | Using index
```

You read it bottom-up logically: for each row of `c`, fetch matching `r` rows via the FK index.

---

## 2. EXPLAIN columns — what each field means

| Column        | Meaning                                                                       |
|---------------|-------------------------------------------------------------------------------|
| id            | query block id; same id = joined together; higher id = inner subquery         |
| select_type   | SIMPLE / PRIMARY / SUBQUERY / DERIVED / UNION                                 |
| table         | which table this row is about                                                 |
| partitions    | which partitions hit (if partitioned)                                         |
| type          | join/access type — `const` < `eq_ref` < `ref` < `range` < `index` < `ALL`     |
| possible_keys | indexes the optimizer considered                                              |
| key           | index actually chosen (or NULL)                                               |
| key_len       | bytes of the key used (helpful for composite indexes)                         |
| ref           | what's compared against the index (constant, column)                          |
| rows          | optimizer's estimate of rows examined                                         |
| filtered      | percentage of `rows` that satisfy WHERE after access                          |
| Extra         | the goldmine column — see below                                               |

### `type` cheat sheet

- **`const`** — single-row PK/UNIQUE lookup. Best.
- **`eq_ref`** — for joins, one row per outer row via PK/UNIQUE. Excellent.
- **`ref`** — non-unique index lookup, multiple rows possible. Good.
- **`range`** — index range scan (BETWEEN, >, <). Fine if rows is small.
- **`index`** — full *index* scan (faster than table scan but still all rows).
- **`ALL`** — full table scan. Often bad — investigate.

### `Extra` flags (the diagnostic gold)

- **`Using index`** — covering index, no row lookup. Excellent.
- **`Using where`** — applied a filter after access. Normal.
- **`Using filesort`** — MySQL must sort; no usable index for ORDER BY. Costly on large sets.
- **`Using temporary`** — built a temp table for GROUP BY/DISTINCT/sort. Also costly.
- **`Using join buffer (hash join)`** — 8.0.18+ hash join. Acceptable, but indexes would be better.
- **`Impossible WHERE`** — query trivially returns nothing. Often a sign of a typo or NULL trap.

---

## 3. EXPLAIN ANALYZE and JSON output — depth (MySQL 8.0.18+)

### EXPLAIN ANALYZE
Actually executes the query and reports timings:

```sql
EXPLAIN ANALYZE
SELECT c.first_name, COUNT(*)
FROM customer c
JOIN rental r ON r.customer_id = c.customer_id
GROUP BY c.customer_id
ORDER BY COUNT(*) DESC LIMIT 10;
```

Output (tree format, abbreviated):
```
-> Limit: 10 row(s)  (cost=2010 rows=10) (actual time=42..42 rows=10 loops=1)
  -> Sort: COUNT(*) DESC, limit input to 10 rows
    -> Group aggregate: count(0)
      -> Nested loop inner join  (cost=1810 rows=8000)
        -> Index scan on c using PRIMARY  (rows=599)
        -> Index lookup on r using idx_fk_customer_id (customer_id=c.customer_id)
```

The `actual time=` and `actual rows=` numbers compared to estimates reveal where the optimizer guessed wrong (the #1 cause of bad plans).

### EXPLAIN FORMAT=JSON
Detailed cost breakdown for each step:
```sql
EXPLAIN FORMAT=JSON SELECT ...;
```
Shows `query_cost`, `read_cost`, `eval_cost`, `prefix_cost`, etc. Useful for comparing two query rewrites.

### Visual EXPLAIN (Workbench)
MySQL Workbench renders EXPLAIN as a tree with colored nodes (green = good, red = full scan). Great for quick scanning.

---

## 4. Practical application — six common rewrites

### A. Replace `OR` across columns with `UNION ALL`
```sql
-- Slow: OR can prevent index merge
SELECT * FROM payment WHERE customer_id = 5 OR staff_id = 1;

-- Fast: each branch uses its own index
SELECT * FROM payment WHERE customer_id = 5
UNION ALL
SELECT * FROM payment WHERE staff_id = 1 AND customer_id <> 5;
```

### B. Move function off the indexed column
```sql
-- BAD
WHERE YEAR(payment_date) = 2005

-- GOOD
WHERE payment_date >= '2005-01-01' AND payment_date < '2006-01-01'
```

### C. Eliminate `SELECT *` to enable a covering index
```sql
-- Cannot be covered (selects all columns)
SELECT * FROM payment WHERE customer_id = 5;

-- Can be covered if (customer_id, amount, payment_date) index exists
SELECT amount, payment_date FROM payment WHERE customer_id = 5;
```

### D. Replace correlated subquery with JOIN
```sql
-- Per-row correlated subquery
SELECT c.customer_id,
  (SELECT COUNT(*) FROM rental r WHERE r.customer_id = c.customer_id) AS n
FROM customer c;

-- Single GROUP BY pass
SELECT c.customer_id, COUNT(r.rental_id) AS n
FROM customer c LEFT JOIN rental r ON r.customer_id = c.customer_id
GROUP BY c.customer_id;
```

### E. Use keyset pagination instead of OFFSET
```sql
-- BAD at depth
SELECT * FROM rental ORDER BY rental_id LIMIT 100000, 20;

-- GOOD
SELECT * FROM rental WHERE rental_id > :last_seen ORDER BY rental_id LIMIT 20;
```

### F. Batch deletes/updates
```sql
-- Bad: one giant transaction, long-held locks
DELETE FROM event_log WHERE created_at < '2024-01-01';

-- Good: chunked
DELETE FROM event_log WHERE created_at < '2024-01-01' LIMIT 10000;
-- repeat
```

### Optimizer hints (8.0+)

```sql
SELECT /*+ INDEX(payment idx_pay_cust_date) */ *
FROM payment WHERE customer_id = 5;

SELECT /*+ JOIN_ORDER(c, r) */ *
FROM customer c JOIN rental r ON r.customer_id = c.customer_id;

SELECT /*+ NO_HASH_JOIN(r) */ ...;
```

Use sparingly. Hints become wrong as data shifts. Prefer fixing the index.

### `ANALYZE TABLE` and statistics
```sql
ANALYZE TABLE payment;
```
Refreshes the optimizer's row-count estimates. Run after bulk loads or major data changes.

---

## 5. Common Mistakes & Gotchas

- **EXPLAIN doesn't run the query** (except `EXPLAIN ANALYZE`). It only plans. Trust `actual rows`/`actual time` from ANALYZE for ground truth.
- **`rows` is an estimate.** Can be wildly off (10x, 100x). When estimate << actual, the plan is often wrong.
- **`type: index` is NOT good.** It means full *index* scan. People confuse it with `type: ref` (good).
- **Filesort isn't always bad** — small in-memory sort is fine. Bad when row count is huge.
- **Hint addiction.** Every hint becomes tech debt. Schema/index changes outlast hints.
- **Forgetting to ANALYZE after bulk loads.** Stats go stale → bad plans.
- **Optimizer trace** (`SET optimizer_trace='enabled=on'; ... SELECT * FROM information_schema.optimizer_trace;`) is the next-level deep dive when EXPLAIN isn't enough.
- **Premature optimization.** Profile first (slow query log, perf schema — Module 17). Don't tune queries no user is running.
- **vs. Postgres:** Postgres's `EXPLAIN (ANALYZE, BUFFERS)` shows actual page reads. MySQL's equivalent is in `performance_schema` (Module 17). Don't expect parity.

---

## 🎯 Key Takeaways

- **EXPLAIN before you tune.** Reading a query plan is the most valuable SQL skill, period.
- **`type: ALL` and `Using filesort`/`Using temporary` are your usual targets.** Index your way out.
- **EXPLAIN ANALYZE shows estimate vs reality.** Big mismatches = optimizer working with bad info.
- **Six core rewrites** (drop function, kill SELECT *, replace correlated, OR → UNION, keyset, chunk) cover 80% of fixes.
- **Hints are a last resort.** They're a contract you have to maintain forever.

*← [11 indexes](./11_indexes.md) | [next → Transactions & Isolation](./13_transactions_isolation.md)*
