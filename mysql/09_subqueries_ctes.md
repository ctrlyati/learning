# 09 — Subqueries, CTEs, and Recursive CTEs

> **Goal:** Compose complex queries by nesting and naming intermediate result sets — using subqueries, derived tables, common table expressions (CTEs), and recursive CTEs.

---

## 1. Subqueries — analogy + runnable SQL

A subquery is a query inside another query. Like nesting function calls in code: the inner expression resolves, then feeds the outer.

```sql
-- Films longer than the average
SELECT title, length
FROM film
WHERE length > (SELECT AVG(length) FROM film);
```

Three places subqueries live:
- **In WHERE / HAVING** (scalar, IN, EXISTS)
- **In FROM** (derived table — must have an alias)
- **In SELECT** (scalar — returns one value per outer row)

---

## 2. Subquery flavors — the mechanism

### Scalar subquery — returns one row, one column
```sql
SELECT title, length, (SELECT AVG(length) FROM film) AS avg_length
FROM film;
```
Cached per query. If the subquery returns more than one row, MySQL errors.

### IN subquery — set membership
```sql
SELECT title FROM film
WHERE film_id IN (
  SELECT film_id FROM film_category WHERE category_id = 1
);
```
The optimizer often rewrites this to a semi-join (like an INNER JOIN that doesn't duplicate rows). Modern MySQL is good at this; ancient MySQL was infamous for being bad.

### EXISTS / NOT EXISTS — correlated subquery
```sql
-- Customers who have rented something
SELECT c.first_name, c.last_name
FROM customer c
WHERE EXISTS (
  SELECT 1 FROM rental r WHERE r.customer_id = c.customer_id
);
```
EXISTS short-circuits at the first matching row. For "does any match exist?" semantics, EXISTS is usually faster than IN with a big subquery.

### NOT EXISTS — anti-join
```sql
-- Films never in any inventory
SELECT f.title
FROM film f
WHERE NOT EXISTS (
  SELECT 1 FROM inventory i WHERE i.film_id = f.film_id
);
```
Cleaner and NULL-safer than `NOT IN (subquery)` (which has the NULL pitfall from Module 06).

### Derived table — subquery in FROM
```sql
SELECT segment, COUNT(*) AS customers
FROM (
  SELECT customer_id,
         CASE WHEN COUNT(*) >= 30 THEN 'power' ELSE 'casual' END AS segment
  FROM rental
  GROUP BY customer_id
) AS t
GROUP BY segment;
```
Must have an alias (`AS t`). Behaves like a temporary table for the duration of the outer query.

### Correlated subquery
References a column from the outer query — re-evaluated for each outer row.
```sql
SELECT c.customer_id,
  (SELECT COUNT(*) FROM rental r WHERE r.customer_id = c.customer_id) AS rental_count
FROM customer c;
```
Easy to read, often slow at scale. Frequently rewritable as a JOIN + GROUP BY.

---

## 3. CTEs (WITH clause) — depth (MySQL 8+)

CTEs are *named* subqueries that appear before the main query. They make complex queries readable, and they let you reference the same intermediate result multiple times.

```sql
WITH active_customers AS (
  SELECT customer_id FROM customer WHERE active = 1
),
recent_rentals AS (
  SELECT customer_id, rental_date
  FROM rental
  WHERE rental_date >= '2005-08-01'
)
SELECT c.customer_id, COUNT(rr.rental_date) AS recent_count
FROM active_customers c
LEFT JOIN recent_rentals rr ON rr.customer_id = c.customer_id
GROUP BY c.customer_id;
```

Equivalent to nested derived tables, but vastly more readable. **MySQL CTEs are NOT materialized by default** (unlike older Postgres) — the optimizer can inline them. So use CTEs purely for clarity; performance is roughly the same as the equivalent subquery.

### Recursive CTEs — for trees and graphs

Two parts: an **anchor** (base case) and a **recursive** member (referencing the CTE itself), connected by `UNION ALL`.

```sql
-- Employee hierarchy
CREATE TABLE employee (
  emp_id INT PRIMARY KEY,
  name   VARCHAR(50),
  manager_id INT
);
INSERT INTO employee VALUES
  (1, 'Alice', NULL),
  (2, 'Bob',   1),
  (3, 'Carol', 1),
  (4, 'Dave',  2),
  (5, 'Eve',   4);

WITH RECURSIVE org_chart AS (
  -- Anchor: top-level (no manager)
  SELECT emp_id, name, manager_id, 0 AS level, CAST(name AS CHAR(200)) AS path
  FROM employee
  WHERE manager_id IS NULL

  UNION ALL

  -- Recursive: anyone whose manager is in the CTE so far
  SELECT e.emp_id, e.name, e.manager_id, oc.level + 1,
         CONCAT(oc.path, ' > ', e.name)
  FROM employee e
  JOIN org_chart oc ON oc.emp_id = e.manager_id
)
SELECT * FROM org_chart ORDER BY path;
```

Result:
```
emp_id | name  | manager_id | level | path
1      | Alice | NULL       | 0     | Alice
2      | Bob   | 1          | 1     | Alice > Bob
4      | Dave  | 2          | 2     | Alice > Bob > Dave
5      | Eve   | 4          | 3     | Alice > Bob > Dave > Eve
3      | Carol | 1          | 1     | Alice > Carol
```

Other recursive use cases:
- Bill of materials (parts → subparts)
- Friend graphs / "shortest path" up to N hops
- Generating sequences:

```sql
-- Generate 1..10
WITH RECURSIVE seq AS (
  SELECT 1 AS n
  UNION ALL
  SELECT n + 1 FROM seq WHERE n < 10
)
SELECT * FROM seq;
```

**Safety:** MySQL caps recursion depth at `cte_max_recursion_depth` (default 1000). Always design a terminating condition.

---

## 4. Practical application — Sakila with subqueries and CTEs

```sql
-- Films above average length per category
WITH cat_avg AS (
  SELECT fc.category_id, AVG(f.length) AS avg_len
  FROM film_category fc
  JOIN film f ON f.film_id = fc.film_id
  GROUP BY fc.category_id
)
SELECT c.name, f.title, f.length, ca.avg_len
FROM film f
JOIN film_category fc ON fc.film_id = f.film_id
JOIN category c       ON c.category_id = fc.category_id
JOIN cat_avg ca       ON ca.category_id = fc.category_id
WHERE f.length > ca.avg_len
ORDER BY c.name, f.length DESC;

-- Customers who rented every film in a specific category (relational division)
SELECT c.customer_id, c.first_name, c.last_name
FROM customer c
WHERE NOT EXISTS (
  SELECT 1
  FROM film_category fc
  WHERE fc.category_id = 6
    AND NOT EXISTS (
      SELECT 1
      FROM rental r
      JOIN inventory i ON i.inventory_id = r.inventory_id
      WHERE r.customer_id = c.customer_id
        AND i.film_id = fc.film_id
    )
);

-- Rental funnel report via multiple CTEs
WITH base AS (
  SELECT DATE(rental_date) AS day, COUNT(*) AS rentals
  FROM rental
  GROUP BY day
),
returns AS (
  SELECT DATE(return_date) AS day, COUNT(*) AS returned
  FROM rental
  WHERE return_date IS NOT NULL
  GROUP BY day
)
SELECT b.day, b.rentals, COALESCE(r.returned, 0) AS returned
FROM base b
LEFT JOIN returns r ON r.day = b.day
ORDER BY b.day;

-- Recursive: generate a date series for the last 30 days
WITH RECURSIVE days AS (
  SELECT CURDATE() AS d
  UNION ALL
  SELECT d - INTERVAL 1 DAY FROM days WHERE d > CURDATE() - INTERVAL 29 DAY
)
SELECT * FROM days;
```

---

## 5. Common Mistakes & Gotchas

- **Correlated subquery in SELECT used like a JOIN.** It's per-row, can be O(N²). Rewrite as JOIN + GROUP BY when N is large.
- **`NOT IN (subquery)` with NULLs** — the classic trap. Always prefer `NOT EXISTS`.
- **Forgetting the alias on a derived table.** `SELECT * FROM (SELECT ...);` is a syntax error. Add `AS t`.
- **Recursive CTE infinite loop.** Anchor + recursive must converge. Add a `WHERE level < N` guard or rely on `cte_max_recursion_depth` to error.
- **Expecting CTEs to be materialized.** They aren't, in MySQL 8. If you need materialization (so a complex CTE isn't recomputed), use a temp table.
- **Subquery returning >1 row** where 1 expected. `SELECT (SELECT id FROM x)` errors if the subquery returns multiple rows. Add `LIMIT 1` (with an `ORDER BY`!).
- **CTE name collision** with a real table. The CTE wins for the duration of the query, which can be confusing.
- **`UNION` vs `UNION ALL` in recursive CTE.** `UNION` deduplicates per recursion step, slow. `UNION ALL` is the standard form.
- **vs. Postgres:** Postgres CTEs were historically *always* materialized — an "optimization fence." MySQL CTEs are inlined by default. If you migrate from old Postgres, performance characteristics differ.

---

## 🎯 Key Takeaways

- **Use CTEs for readability.** A 200-line nested query becomes five named, comprehensible blocks.
- **`NOT EXISTS` over `NOT IN`** for anti-join semantics — NULL-safe and usually faster.
- **Recursive CTEs** unlock trees, graphs, and synthetic series — features that used to require app-side loops.
- **Correlated subqueries are convenient and dangerous** — fine on small data, deadly on large unless the inner query is well-indexed.
- **MySQL CTEs aren't materialized** — they're a syntactic convenience. Use temp tables when you need actual materialization.

*← [08 aggregations](./08_aggregations.md) | [next → Window Functions](./10_window_functions.md)*
