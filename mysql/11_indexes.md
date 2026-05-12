# 11 — Indexes

> **Goal:** Understand the B-tree, design composite and covering indexes, recognize when an index hurts, and reason about why a query is fast or slow.

---

## 1. Indexes — analogy + runnable SQL

An index is a sorted reference structure. The classic analogy: the index at the back of a book. Without it, finding "MVCC" requires reading every page (a **full table scan**, O(N)). With it, you flip to "M," scan a few entries, and jump straight to the page (O(log N)).

```sql
-- Without an index, this scans every customer
SELECT * FROM customer WHERE email = 'MARY.SMITH@sakilacustomer.org';

CREATE INDEX idx_customer_email ON customer(email);

-- Now: B-tree lookup, microseconds even with millions of rows
SELECT * FROM customer WHERE email = 'MARY.SMITH@sakilacustomer.org';
```

Verify with EXPLAIN:
```sql
EXPLAIN SELECT * FROM customer WHERE email = 'MARY.SMITH@sakilacustomer.org';
-- Look at `key` column: it should now show idx_customer_email
-- And `type`: ref or const, instead of ALL
```

---

## 2. The B+ tree — how InnoDB indexes work

InnoDB uses **B+ trees** for all indexes (and the table itself, more on that below).

```
                  [50]
                /      \
           [20,35]    [70,85]
          /  |   \    /  |  \
       leaf leaf leaf leaf leaf leaf  (sorted, linked list)
```

Properties:
- **Balanced** — every leaf at the same depth (typically 3–4 levels even for billion-row tables).
- **Sorted** — leaves contain key values in sorted order, with horizontal pointers between siblings (great for range scans).
- **High fan-out** — each internal node holds many keys, so depth grows logarithmically.
- **Updates rebalance** — splits and merges keep the tree balanced.

A lookup of "find email = X" descends 3–4 nodes from root to leaf — usually 3–4 page reads (~16KB each in InnoDB). That's why indexes are so fast.

### Clustered index — the table IS the PK B-tree

In InnoDB, the table's **primary key is itself a B-tree, and the leaf nodes contain the full row**. This is called a **clustered index**.

Consequences:
- Lookup by PK is one B-tree traversal — extremely fast.
- Rows are physically stored in PK order. Range scans by PK are sequential reads.
- **Secondary indexes** (anything other than the PK) store the indexed column(s) + the **PK as the row pointer**.
  - To fetch full row from a secondary index hit: traverse secondary index → get PK → traverse clustered index = two B-tree traversals.
  - This is why a slim PK matters (Module 02) — it appears in every secondary index.

```sql
-- Suppose payment table:
PRIMARY KEY (payment_id)
INDEX idx_customer (customer_id)

-- WHERE customer_id = 5:
-- 1. idx_customer B-tree: find leaf entries with customer_id=5, get payment_ids
-- 2. For each payment_id: clustered B-tree lookup to get full row
-- This second step is the 'lookup' / 'bookmark lookup'
```

### Covering index — skip the second lookup

If all columns the query needs are in the index, MySQL skips the clustered-index visit.

```sql
CREATE INDEX idx_pay_cust_amt ON payment(customer_id, amount);

-- Covering: idx contains both columns referenced
SELECT customer_id, amount FROM payment WHERE customer_id = 5;
-- EXPLAIN Extra: 'Using index'  <- this means covered
```

Covering indexes are the single biggest hidden lever in MySQL performance.

---

## 3. Composite, prefix, and covering indexes — depth

### Composite index — leftmost prefix rule

```sql
CREATE INDEX idx_a_b_c ON t(a, b, c);
```

This index is sorted by `a`, then by `b` within `a`, then by `c` within `(a,b)`. Usable for:
- `WHERE a = ?`  — yes
- `WHERE a = ? AND b = ?` — yes
- `WHERE a = ? AND b = ? AND c = ?` — yes
- `WHERE a = ? AND c = ?` — partially (uses `a`, then filters `c` per-row)
- `WHERE b = ?` — **no** (can't skip the leading column)
- `WHERE b = ? AND c = ?` — **no**

This is the **leftmost prefix rule**. Pick column order to match your most common query patterns.

### Range stops the index

```sql
CREATE INDEX idx_x_y_z ON t(x, y, z);
SELECT * FROM t WHERE x = 1 AND y > 5 AND z = 7;
```
Index used for `x = 1 AND y > 5`, but `z = 7` is filtered per-row, not by the index — because `y` is a range and the rest of the index is no longer sorted by `z` once `y` ranges.

**Rule:** put equality columns first, range columns last. Move highly-selective columns earlier.

### Prefix indexes (for long strings)

```sql
CREATE INDEX idx_title_pre ON film(title(20));
```
Indexes only the first 20 characters. Saves space; usable for `WHERE title = ?` and `WHERE title LIKE 'foo%'`. Cannot cover queries that select the column. Useful for VARCHAR(255) emails where the prefix is highly unique.

### Functional and JSON indexes (8.0.13+)

```sql
CREATE INDEX idx_lower_email ON customer ((LOWER(email)));
SELECT * FROM customer WHERE LOWER(email) = 'mary@x.com';

-- Or for JSON:
ALTER TABLE event_log ADD COLUMN user_id INT
  GENERATED ALWAYS AS (payload->>'$.user_id') STORED,
  ADD INDEX idx_user (user_id);
```

### When indexes hurt

- **Every index slows writes.** INSERT/UPDATE/DELETE must update every index that touches changed columns.
- **Indexes consume RAM** (the buffer pool caches index pages alongside data).
- **Bad indexes mislead the optimizer.** Stale stats + a tempting-but-wrong index = bad plan.
- **Low-cardinality indexes are usually useless.** An index on `is_active TINYINT(1)` (50/50) won't be used — a full scan is cheaper than 50% index lookups + bookmark lookups. Indexes shine when selectivity is high.

Rule of thumb: an index helps if it filters down to <~10% of rows. Below that, the optimizer will (rightly) ignore it.

---

## 4. Practical application — diagnosing a slow query

```sql
-- Slow: searching by last_name without an index
EXPLAIN SELECT * FROM customer WHERE last_name = 'SMITH';
-- type: ALL (full scan), rows: ~599

CREATE INDEX idx_customer_last_name ON customer(last_name);

EXPLAIN SELECT * FROM customer WHERE last_name = 'SMITH';
-- type: ref, key: idx_customer_last_name, rows: 1-3
```

### A real-world composite-index design exercise

Your most common query:
```sql
SELECT payment_id, amount, payment_date
FROM payment
WHERE customer_id = ? AND payment_date >= ?
ORDER BY payment_date DESC
LIMIT 20;
```

Best index:
```sql
CREATE INDEX idx_pay_cust_date ON payment(customer_id, payment_date);
```

Why:
- Equality on `customer_id` first.
- Range/sort on `payment_date` second — the index is already sorted in the order we want.
- The index covers WHERE + ORDER BY. EXPLAIN should show no `Using filesort` and no `Using temporary`.

To make it covering for this query:
```sql
CREATE INDEX idx_pay_cust_date_amt ON payment(customer_id, payment_date, amount);
```

Now `SELECT customer_id, payment_date, amount` is fully covered.

### Keyset pagination using the index

```sql
-- Page N: keyset on (payment_date, payment_id)
SELECT payment_id, amount, payment_date
FROM payment
WHERE customer_id = 5
  AND (payment_date, payment_id) < ('2005-08-15 00:00:00', 12345)
ORDER BY payment_date DESC, payment_id DESC
LIMIT 20;
```

This walks the index forward without scanning skipped pages. Compare with `OFFSET 100000 LIMIT 20`, which reads and discards 100k rows.

---

## 5. Common Mistakes & Gotchas

- **Function on indexed column kills the index.** `WHERE YEAR(d) = 2025` doesn't use an index on `d`. Use range. (Or use a functional index, MySQL 8.)
- **Implicit type conversion.** `WHERE varchar_col = 5` casts every row's column to a number — full scan.
- **Leading wildcard `LIKE '%foo'`.** Cannot use B-tree.
- **Indexing every column "just in case."** Each index taxes writes and space. Prune aggressively.
- **Wrong column order in composite.** `(date, customer_id)` for `WHERE customer_id = ?` is useless.
- **Indexing low-cardinality columns alone.** `is_active`, `gender`, `status` rarely benefit from a standalone index. Combine with another column.
- **Forgetting that secondary indexes carry the PK.** Wide PKs bloat every secondary index.
- **`SELECT *` defeats covering indexes.** Always list the columns you need.
- **Stale statistics.** After bulk loads, run `ANALYZE TABLE` to refresh.
- **Adding an index = blocking ALTER on old MySQL.** Use online DDL or gh-ost on big tables.
- **vs. PostgreSQL:** Postgres has many index types (B-tree, hash, GIN, GiST, BRIN). MySQL/InnoDB has B-tree, FULLTEXT, and SPATIAL — that's basically it. Less choice, simpler reasoning.

---

## 🎯 Key Takeaways

- **The PK is the table.** In InnoDB, choose your PK like you mean it — it appears in every secondary index.
- **Composite-index column order = (equality, equality, ..., range/sort).** Leftmost-prefix rule governs everything.
- **Covering indexes are the secret weapon.** Add the columns the query selects, skip the bookmark lookup.
- **Indexes slow writes.** Every one is a tradeoff; audit and prune.
- **Don't fight the optimizer with bad query shape.** Functions on columns, leading wildcards, and type mismatches all silently disable indexes.

*← [10 windows](./10_window_functions.md) | [next → Query Optimization](./12_query_optimization.md)*
