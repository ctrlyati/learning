# 05 — DML Basics: SELECT, WHERE, ORDER BY, LIMIT

> **Goal:** Master the four most-used clauses in SQL — they're the 80% of every query you'll ever write.

---

## 1. SELECT — analogy + runnable SQL

If a table is a spreadsheet, `SELECT` is "show me these rows and columns." The full shape of a basic query:

```
SELECT  <columns>
FROM    <table>
WHERE   <row filter>
ORDER BY <sort>
LIMIT   <how many>;
```

```sql
USE sakila;

SELECT first_name, last_name, email
FROM customer
WHERE active = 1
ORDER BY last_name
LIMIT 10;
```

Read it as: "from `customer`, take rows where `active=1`, sort by `last_name`, give me the first 10, but only the three columns I named."

---

## 2. The logical execution order — the mechanism

Despite SQL being written `SELECT ... FROM ... WHERE ... ORDER BY ... LIMIT`, the engine processes clauses in a different order:

```
1. FROM        -- pick the source table(s)
2. WHERE       -- filter rows
3. GROUP BY    -- bucket rows (Module 08)
4. HAVING      -- filter buckets
5. SELECT      -- evaluate column expressions
6. DISTINCT    -- deduplicate
7. ORDER BY    -- sort
8. LIMIT       -- truncate
```

This explains several quirks:
- You **can't reference a SELECT alias in WHERE**, because WHERE runs before SELECT.
  ```sql
  SELECT price * 1.2 AS gross FROM product WHERE gross > 100;  -- ERROR
  SELECT price * 1.2 AS gross FROM product WHERE price * 1.2 > 100;  -- works
  ```
  (You *can* reference aliases in `ORDER BY` and `GROUP BY` because they come after SELECT.)
- `LIMIT` happens last, so a query without `ORDER BY` + `LIMIT 10` returns 10 *arbitrary* rows — not "the first ten." There's no inherent row order in a table.

---

## 3. Variations and depth

### SELECT column lists

```sql
SELECT *               FROM film;           -- everything (avoid in code)
SELECT title, length   FROM film;           -- specific columns
SELECT title AS movie  FROM film;           -- alias
SELECT DISTINCT rating FROM film;           -- unique values
SELECT 1 + 1, NOW(), USER();                -- no FROM needed for expressions
```

`SELECT *` is fine in the CLI for exploration; never in application code (breaks when columns change, pulls TEXT/BLOB unnecessarily, breaks covering-index optimizations — Module 11).

### WHERE — filtering

```sql
SELECT * FROM film WHERE rental_rate = 0.99;
SELECT * FROM film WHERE length BETWEEN 60 AND 90;
SELECT * FROM film WHERE rating IN ('G','PG');
SELECT * FROM film WHERE title LIKE 'A%';        -- starts with A
SELECT * FROM film WHERE description IS NULL;
SELECT * FROM film WHERE release_year >= 2005 AND length < 120;
```

WHERE evaluates a **boolean expression per row**. Anything that returns truthy passes. Three-valued logic (TRUE/FALSE/NULL) — Module 06 covers NULL pitfalls.

### ORDER BY — sorting

```sql
SELECT title, length FROM film ORDER BY length DESC;
SELECT title, length FROM film ORDER BY length DESC, title ASC;  -- tiebreaker
SELECT title, length FROM film ORDER BY 2 DESC;                  -- by column position (avoid)
SELECT title, length AS L FROM film ORDER BY L DESC;             -- by alias (fine)
```

Sorting is **expensive** if there's no index supporting the order. EXPLAIN shows `Using filesort` when the engine sorts in memory or on disk. With an appropriate index, sorting is free (the index is already sorted).

### LIMIT — pagination

```sql
SELECT * FROM film ORDER BY film_id LIMIT 10;        -- first 10
SELECT * FROM film ORDER BY film_id LIMIT 10 OFFSET 20;  -- 11th-30th... wait
SELECT * FROM film ORDER BY film_id LIMIT 20, 10;    -- MySQL shorthand: OFFSET 20, fetch 10
```

**Pagination warning:** `LIMIT 1000000, 10` reads and discards 1,000,000 rows. Brutal at scale. Use **keyset pagination** (Module 11):
```sql
-- "next page" given the last seen film_id
SELECT * FROM film WHERE film_id > 12345 ORDER BY film_id LIMIT 10;
```

### INSERT, UPDATE, DELETE — the other DML

```sql
INSERT INTO category (name) VALUES ('Documentary');
INSERT INTO category (name) VALUES ('Drama'), ('Comedy'), ('Thriller');  -- multi-row

UPDATE film SET rental_rate = rental_rate * 1.1 WHERE rating = 'G';

DELETE FROM rental WHERE return_date IS NULL AND rental_date < '2005-01-01';
```

**Always run UPDATE/DELETE first as a SELECT** to confirm the WHERE clause matches what you expect. Better, set safe-update mode in your CLI:
```sql
SET SQL_SAFE_UPDATES = 1;  -- forbids UPDATE/DELETE without WHERE on a key
```

`UPDATE ... LIMIT N` and `DELETE ... LIMIT N` exist in MySQL (not SQL standard). Useful for batch deletes:
```sql
DELETE FROM event_log WHERE created_at < '2024-01-01' LIMIT 10000;
-- repeat until 0 rows affected; avoids one giant transaction
```

`INSERT ... ON DUPLICATE KEY UPDATE` (MySQL-specific upsert):
```sql
INSERT INTO inventory (sku, qty) VALUES ('A1', 5)
ON DUPLICATE KEY UPDATE qty = qty + VALUES(qty);
```

`REPLACE INTO` exists but **deletes + reinserts** — fires DELETE triggers, breaks FK chains. Prefer `INSERT ... ON DUPLICATE KEY UPDATE`.

---

## 4. Practical application — exploring Sakila

A handful of queries that exercise everything above:

```sql
-- Top 5 longest films
SELECT title, length
FROM film
ORDER BY length DESC
LIMIT 5;

-- Active customers in store 1
SELECT customer_id, first_name, last_name, email
FROM customer
WHERE store_id = 1 AND active = 1
ORDER BY last_name, first_name;

-- Rentals from a specific week
SELECT rental_id, rental_date, customer_id
FROM rental
WHERE rental_date >= '2005-05-24' AND rental_date < '2005-05-31'
ORDER BY rental_date;

-- Films with the word 'epic' in description, R-rated, sorted by length desc
SELECT title, length
FROM film
WHERE description LIKE '%epic%'
  AND rating = 'R'
ORDER BY length DESC;

-- Update: bump rental rate for short films
UPDATE film
SET rental_rate = rental_rate + 0.50
WHERE length < 60
  AND rental_rate < 5.00;

-- Delete: clean out test addresses (hypothetical)
DELETE FROM address WHERE address LIKE 'TEST%' LIMIT 1000;
```

---

## 5. Common Mistakes & Gotchas

- **`SELECT *` in production code.** Pulls every column (incl. TEXT/BLOB), breaks when schema changes, prevents covering indexes.
- **No `ORDER BY` with `LIMIT`.** Returns "10 random rows" — the storage engine's natural order, which can change. Always pair them.
- **Referencing a SELECT alias in WHERE.** Doesn't work — WHERE runs first. Repeat the expression or wrap in a subquery.
- **`OFFSET` for deep pagination.** `LIMIT 100000, 10` is O(100010). Use keyset pagination for scrolling lists.
- **Forgetting WHERE in UPDATE/DELETE.** A bare `DELETE FROM customer;` deletes everything. Use safe-update mode and always preview as SELECT.
- **`= NULL` doesn't work.** `WHERE col = NULL` returns no rows. Use `IS NULL` (Module 06).
- **Unquoted strings.** `WHERE name = john` is a column reference, not a literal — error. Quote: `'john'`.
- **`!=` vs `<>`.** Both work; `<>` is SQL standard. MySQL accepts both.
- **vs. Postgres:** MySQL's `LIMIT 20, 10` (offset, count) is non-standard. Standard SQL is `OFFSET 20 ROWS FETCH NEXT 10 ROWS ONLY`. MySQL also accepts `LIMIT 10 OFFSET 20` — prefer that for portability.

---

## 🎯 Key Takeaways

- **Logical execution order ≠ written order.** WHERE before SELECT before ORDER BY before LIMIT. Internalize this.
- **`SELECT *` is for exploration, never for code.** Always list columns.
- **`ORDER BY` is mandatory** any time `LIMIT` matters. Tables have no inherent order.
- **OFFSET is O(N) — keyset pagination scales.** This will save you from a P0 outage someday.
- **Safe-update mode + preview-as-SELECT** is the discipline that keeps you from running `DELETE FROM customer` at 11pm.

*← [04 ddl](./04_ddl_constraints.md) | [next → Filtering Deeper](./06_filtering_deeper.md)*
