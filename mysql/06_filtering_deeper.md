# 06 — Filtering Deeper: Operators, LIKE, IN, BETWEEN, NULL

> **Goal:** Wield WHERE clauses with precision. Understand three-valued logic, pattern matching costs, and which operators kill (or use) indexes.

---

## 1. Three-valued logic — analogy + runnable SQL

In normal programming, booleans are TRUE or FALSE. SQL adds a third: **UNKNOWN**, produced whenever NULL touches a comparison. That's because NULL means "I don't know" — and you can't compare an unknown.

```sql
SELECT NULL = NULL;     -- NULL (not TRUE!)
SELECT NULL = 1;        -- NULL
SELECT NULL <> 1;       -- NULL
SELECT NULL IS NULL;    -- 1 (TRUE)
SELECT NULL IS NOT NULL; -- 0 (FALSE)
```

WHERE only keeps rows where the expression is **TRUE**. NULL/UNKNOWN rows are dropped — silently — both for `WHERE col = x` AND for `WHERE col <> x`.

```sql
-- Suppose 100 rows; 30 have description IS NULL.
SELECT COUNT(*) FROM film WHERE description = 'foo';   -- 0
SELECT COUNT(*) FROM film WHERE description <> 'foo';  -- 70 — NOT 100!
-- The 30 NULL rows are excluded from BOTH.
```

To include NULL handling explicitly:
```sql
SELECT COUNT(*) FROM film
WHERE description <> 'foo' OR description IS NULL;     -- now 100
```

Or use the null-safe operator `<=>`:
```sql
SELECT * FROM film WHERE description <=> NULL;  -- equivalent to IS NULL
SELECT * FROM film WHERE description <=> 'foo'; -- like = but NULL-safe
```

---

## 2. The operator catalog — mechanism + index behavior

### Comparison
`=`, `<>` (or `!=`), `<`, `<=`, `>`, `>=`, `<=>` (NULL-safe equal).

All can use a B-tree index on the column when applied directly: `WHERE col = ?` or `WHERE col > ?`.

### IN — set membership
```sql
SELECT * FROM film WHERE rating IN ('G', 'PG', 'PG-13');
-- equivalent to: rating='G' OR rating='PG' OR rating='PG-13'
```
Uses an index when the column is indexed and the list is reasonably small. Large IN lists (thousands) can degrade — consider a temp table or JOIN.

`NOT IN` has a NULL trap: if any value in the list is NULL, the whole expression becomes NULL (false), excluding everything.
```sql
SELECT * FROM film WHERE rating NOT IN ('G', NULL);  -- returns 0 rows!
```

### BETWEEN — inclusive range
```sql
SELECT * FROM film WHERE length BETWEEN 60 AND 90;
-- equivalent to: length >= 60 AND length <= 90
```
Both bounds inclusive. Index-friendly for ranges.

### LIKE — pattern matching
- `%` matches any number of characters.
- `_` matches exactly one.

```sql
WHERE title LIKE 'The %'         -- starts with "The "  — uses index
WHERE title LIKE '%epic%'        -- contains "epic"     — full table scan
WHERE title LIKE 'A_pen%'        -- second char any, then "pen"
```

**Critical:** A LIKE pattern starting with `%` (or `_`) **cannot use a B-tree index**, because B-trees are sorted prefix lookups. Leading wildcard = full scan. For substring search at scale, use **FULLTEXT** indexes:

```sql
CREATE FULLTEXT INDEX ft_film_desc ON film(description);
SELECT * FROM film
WHERE MATCH(description) AGAINST('epic' IN NATURAL LANGUAGE MODE);
```

`LIKE` is case-sensitivity depends on the **column collation**. `_ci` (case-insensitive) collations make `'A%' = 'a%'`. Use `LIKE BINARY` to force case-sensitive.

### REGEXP / RLIKE
Full regex matching. Powerful, slow (no index use):
```sql
WHERE email REGEXP '^[a-z]+@example\\.(com|org)$'
```

### Boolean logic
`AND`, `OR`, `NOT`, parentheses. AND binds tighter than OR — when in doubt, parenthesize.
```sql
WHERE rating = 'G' AND length > 90 OR length > 180
-- means: (rating='G' AND length>90) OR length>180  -- probably not what you want
WHERE rating = 'G' AND (length > 90 OR length > 180)
```

---

## 3. NULL handling — depth

### Where NULL hides

- **Comparisons** return NULL → row dropped from WHERE.
- **Aggregates ignore NULL** (except `COUNT(*)`). `AVG(col)` averages only non-NULL rows.
- **`COUNT(col)` vs `COUNT(*)`:** `COUNT(col)` counts non-NULL values; `COUNT(*)` counts all rows.
- **GROUP BY treats all NULLs as one group.**
- **ORDER BY:** NULLs sort *first* in ASC, *last* in DESC in MySQL. (Postgres lets you specify `NULLS FIRST/LAST`; MySQL does not — workaround: `ORDER BY col IS NULL, col`.)
- **UNIQUE allows multiple NULLs.** A `UNIQUE(email)` constraint will accept two rows where email is NULL.

### Useful NULL functions

```sql
COALESCE(a, b, c)   -- first non-NULL of args
IFNULL(a, b)        -- MySQL shortcut: a if not null else b
NULLIF(a, b)        -- NULL if a=b, else a (e.g., divide-by-zero guard: x/NULLIF(y,0))
ISNULL(x)           -- 1 if NULL else 0
```

```sql
SELECT COALESCE(middle_name, '') AS m FROM customer;
SELECT salary / NULLIF(hours_worked, 0) AS hourly FROM payroll;
```

---

## 4. Practical application — Sakila edge cases

```sql
-- Customers with no recorded last rental — note IS NULL
SELECT c.first_name, c.last_name
FROM customer c
LEFT JOIN rental r ON r.customer_id = c.customer_id
WHERE r.rental_id IS NULL;

-- Films with NULL original_language vs. specific value
SELECT title FROM film WHERE original_language_id IS NULL;
SELECT title FROM film WHERE original_language_id = 1;

-- Find titles starting with A or B (index-friendly)
SELECT title FROM film WHERE title LIKE 'A%' OR title LIKE 'B%';
-- equivalent and sometimes faster:
SELECT title FROM film WHERE title >= 'A' AND title < 'C';

-- Find films of length not in a set, NULL-safe
SELECT title FROM film
WHERE length NOT IN (60, 90, 120) OR length IS NULL;

-- Ranges
SELECT * FROM payment
WHERE payment_date >= '2005-05-01' AND payment_date < '2005-06-01';
-- Prefer half-open >= AND < over BETWEEN for date ranges; avoids end-of-day issues.

-- Combining safely
SELECT * FROM customer
WHERE active = 1
  AND (email LIKE '%@gmail.com' OR email LIKE '%@yahoo.com');
```

### Index-aware WHERE patterns

```sql
-- BAD: function on indexed column kills index
WHERE YEAR(payment_date) = 2005

-- GOOD: range that lets the index work
WHERE payment_date >= '2005-01-01' AND payment_date < '2006-01-01'

-- BAD: type mismatch causes implicit conversion, no index
WHERE customer_id = '123'   -- if customer_id is INT, MySQL casts every row's int to string

-- GOOD: same type
WHERE customer_id = 123
```

We'll see why in Modules 11 & 12 — for now: **don't apply functions to indexed columns in WHERE**.

---

## 5. Common Mistakes & Gotchas

- **`= NULL` doesn't work.** Always `IS NULL` / `IS NOT NULL` (or `<=>`).
- **`NOT IN (..., NULL)` returns nothing.** Strip NULLs from your list, or restructure with `NOT EXISTS`.
- **Leading wildcard `LIKE '%foo%'` is a full scan.** Use FULLTEXT for substring search, or a trigram extension if available.
- **Using `OR` heavily on different columns** can prevent index use. Consider rewriting as `UNION ALL` of two indexed queries.
- **Date functions in WHERE.** `WHERE DATE(created_at) = '2025-01-01'` ignores any index on `created_at`. Use range form.
- **`COUNT(col)` when you meant `COUNT(*)`** — silently undercounts NULLs.
- **Operator precedence forgotten.** AND > OR. Parenthesize when mixing.
- **Implicit type conversion** silently disables indexes. Match types in comparisons.
- **`BETWEEN` on dates.** `BETWEEN '2025-01-01' AND '2025-01-31'` excludes anything on Jan 31 after midnight — DATETIME values like `'2025-01-31 14:00'` are excluded if the right bound is interpreted as `2025-01-31 00:00:00`. Use `>=` and `<` with the next-day boundary.
- **vs. Postgres:** Postgres has `IS DISTINCT FROM` which is null-safe equality and standard SQL. MySQL uses `<=>` instead.

---

## 🎯 Key Takeaways

- **NULL is unknown, not zero.** Three-valued logic poisons WHERE silently — train your eye to spot it.
- **Leading-wildcard LIKE is unindexable.** This single fact justifies FULLTEXT or external search (Elasticsearch, Meilisearch) at scale.
- **Functions on indexed columns kill the index.** Rewrite as ranges.
- **`NOT IN` + NULL = empty result.** A classic interview gotcha and a real production bug source.
- **Half-open date ranges (`>=` AND `<`) beat BETWEEN** for DATETIME columns — and read more honestly across types.

*← [05 dml basics](./05_dml_basics.md) | [next → JOINs](./07_joins.md)*
