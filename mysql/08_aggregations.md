# 08 — Aggregations: GROUP BY, HAVING, ROLLUP

> **Goal:** Compute counts, sums, averages, and grouped summaries — and know the difference between WHERE and HAVING, why GROUP BY rules are strict, and when ROLLUP saves you a UNION.

---

## 1. Aggregation — analogy + runnable SQL

Aggregation is the spreadsheet pivot. Take many rows, bucket them, and produce one summary row per bucket.

```sql
SELECT rating, COUNT(*) AS film_count, AVG(length) AS avg_length
FROM film
GROUP BY rating;
```

Result:
```
rating | film_count | avg_length
G      | 178        | 111.0506
PG     | 194        | 112.0103
PG-13  | 223        | 120.5067
R      | 195        | 118.6615
NC-17  | 210        | 113.2286
```

Five rows summarizing 1000.

---

## 2. The aggregate functions — mechanism

### Standard aggregates

| Function           | Description                              |
|--------------------|------------------------------------------|
| COUNT(*)           | total rows in the group                  |
| COUNT(col)         | non-NULL values of col                   |
| COUNT(DISTINCT col)| distinct non-NULL values                 |
| SUM(col)           | sum (ignores NULL)                       |
| AVG(col)           | mean (ignores NULL)                      |
| MIN(col), MAX(col) | min/max (ignore NULL)                    |
| GROUP_CONCAT(col)  | concatenate values with separator        |
| JSON_ARRAYAGG(col) | aggregate values into a JSON array (8+)  |
| JSON_OBJECTAGG(k,v)| aggregate into a JSON object (8+)        |
| STDDEV, VARIANCE   | statistical                              |

### How GROUP BY works

```
1. FROM/WHERE   -- get filtered rows
2. GROUP BY     -- bucket rows by the grouping columns
3. SELECT       -- evaluate aggregates per bucket; non-aggregate columns must be in GROUP BY
4. HAVING       -- filter buckets
5. ORDER BY     -- sort results
6. LIMIT
```

Key invariant: **every column in SELECT must either be in GROUP BY or inside an aggregate function.** Otherwise the result is ambiguous (which row's value do we pick?).

```sql
-- GOOD
SELECT rating, AVG(length) FROM film GROUP BY rating;

-- BAD (in strict mode — error)
SELECT rating, title, AVG(length) FROM film GROUP BY rating;
-- Which `title` do you mean? There are ~200 titles per rating.
```

MySQL had a famously lax mode (pre-5.7) that picked an arbitrary value. **8.0 enables `ONLY_FULL_GROUP_BY` by default**, which errors on this. Good. Don't disable it.

### COUNT subtleties
```sql
SELECT COUNT(*)            FROM film;  -- 1000
SELECT COUNT(description)  FROM film;  -- 1000 (no NULLs in this column)
SELECT COUNT(original_language_id) FROM film;  -- 0 (all NULL!)
SELECT COUNT(DISTINCT rating) FROM film;       -- 5
```

---

## 3. HAVING vs WHERE — the depth that trips most people

- **WHERE filters rows** before grouping.
- **HAVING filters groups** after grouping.

```sql
-- Films per category, only categories with > 50 films
SELECT c.name, COUNT(*) AS n
FROM film_category fc
JOIN category c ON c.category_id = fc.category_id
GROUP BY c.name
HAVING n > 50;
```

You *can* sometimes write a condition in either, but the rules:
- Conditions involving non-aggregate columns: prefer WHERE (filters earlier, less work).
- Conditions involving aggregates: must be in HAVING.

```sql
-- BAD: aggregate in WHERE
SELECT rating, COUNT(*) FROM film WHERE COUNT(*) > 100 GROUP BY rating;  -- ERROR

-- GOOD
SELECT rating, COUNT(*) FROM film GROUP BY rating HAVING COUNT(*) > 100;
```

### GROUP_CONCAT — useful and dangerous

```sql
SELECT f.title, GROUP_CONCAT(a.first_name SEPARATOR ', ') AS actors
FROM film f
JOIN film_actor fa ON fa.film_id = f.film_id
JOIN actor a       ON a.actor_id = fa.actor_id
GROUP BY f.film_id, f.title
LIMIT 5;
```

Default truncation at 1024 bytes (`group_concat_max_len`). For long lists raise the limit:
```sql
SET SESSION group_concat_max_len = 1000000;
```

### ROLLUP — subtotals + grand total

```sql
SELECT rating, COUNT(*) AS n
FROM film
GROUP BY rating WITH ROLLUP;
```

Result:
```
rating | n
G      | 178
NC-17  | 210
PG     | 194
PG-13  | 223
R      | 195
NULL   | 1000   <- grand total
```

With multiple grouping columns, ROLLUP produces subtotals for each level:
```sql
SELECT rating, release_year, COUNT(*) AS n
FROM film
GROUP BY rating, release_year WITH ROLLUP;
```

You get: per (rating, year), per rating subtotal, and grand total. The NULLs in grouping columns mark the rollup rows. Distinguish with `GROUPING()`:

```sql
SELECT
  IF(GROUPING(rating)=1, 'TOTAL', rating) AS rating,
  COUNT(*) AS n
FROM film
GROUP BY rating WITH ROLLUP;
```

---

## 4. Practical application — Sakila reporting queries

```sql
-- Top 10 customers by total spending
SELECT c.customer_id, c.first_name, c.last_name,
       SUM(p.amount) AS lifetime_value,
       COUNT(p.payment_id) AS payment_count
FROM customer c
JOIN payment p ON p.customer_id = c.customer_id
GROUP BY c.customer_id, c.first_name, c.last_name
ORDER BY lifetime_value DESC
LIMIT 10;

-- Monthly revenue
SELECT DATE_FORMAT(payment_date, '%Y-%m') AS month,
       COUNT(*) AS payments,
       SUM(amount) AS revenue,
       AVG(amount) AS avg_payment
FROM payment
GROUP BY month
ORDER BY month;

-- Films never rented
SELECT f.title
FROM film f
LEFT JOIN inventory i ON i.film_id = f.film_id
LEFT JOIN rental r    ON r.inventory_id = i.inventory_id
GROUP BY f.film_id, f.title
HAVING COUNT(r.rental_id) = 0;

-- Category revenue with subtotal by store and grand total via ROLLUP
SELECT s.store_id, c.name AS category, SUM(p.amount) AS revenue
FROM payment p
JOIN rental r        ON r.rental_id = p.rental_id
JOIN inventory i     ON i.inventory_id = r.inventory_id
JOIN film_category fc ON fc.film_id = i.film_id
JOIN category c       ON c.category_id = fc.category_id
JOIN staff st         ON st.staff_id = p.staff_id
JOIN store s          ON s.store_id = st.store_id
GROUP BY s.store_id, c.name WITH ROLLUP;

-- Customer activity buckets
SELECT
  CASE
    WHEN cnt = 0 THEN 'inactive'
    WHEN cnt < 10 THEN 'casual'
    WHEN cnt < 30 THEN 'regular'
    ELSE 'power'
  END AS segment,
  COUNT(*) AS customers
FROM (
  SELECT c.customer_id, COUNT(r.rental_id) AS cnt
  FROM customer c
  LEFT JOIN rental r ON r.customer_id = c.customer_id
  GROUP BY c.customer_id
) sub
GROUP BY segment;
```

---

## 5. Common Mistakes & Gotchas

- **Selecting a non-aggregate, non-grouped column.** Errors in MySQL 8 default mode (good); silently picked a "random" value in older versions (bad).
- **`COUNT(col)` when you wanted `COUNT(*)`.** Different semantics for nullable columns.
- **HAVING without GROUP BY.** Treats the whole result set as one group. Sometimes intentional (`SELECT COUNT(*) FROM x HAVING COUNT(*) > 0`), often a bug.
- **Aggregates over LEFT JOINs without thinking.** `COUNT(*)` on a LEFT JOIN counts the left row even when right side is NULL. Use `COUNT(right_table.id)` to count only matched rows.
- **Mixing aggregates and window functions** without understanding which runs first (windows run after GROUP BY — Module 10).
- **`GROUP_CONCAT` truncated to 1024 bytes** silently. Bump `group_concat_max_len`.
- **Sort by aggregate.** `ORDER BY COUNT(*) DESC` works because ORDER BY runs after SELECT — but `WHERE COUNT(*) > 5` doesn't.
- **DISTINCT vs GROUP BY.** Often interchangeable for "unique rows," but GROUP BY can be faster when grouped column is indexed; DISTINCT is clearer when you don't need an aggregate.
- **vs. Postgres:** Postgres has GROUPING SETS, CUBE, and FILTER — richer than MySQL's ROLLUP. MySQL doesn't support `FILTER (WHERE ...)`; emulate with `SUM(CASE WHEN ... THEN 1 ELSE 0 END)`.

---

## 🎯 Key Takeaways

- **WHERE filters rows; HAVING filters groups.** Always WHERE if you can — earlier filters mean less work.
- **`ONLY_FULL_GROUP_BY` is your friend.** Don't disable it just because legacy code complains. The "fix" reveals real bugs.
- **`COUNT(*)` vs `COUNT(col)` vs `COUNT(DISTINCT col)`** — three different things. Pick consciously.
- **ROLLUP gives free subtotals** without UNION ALL — but the trailing NULLs in grouping columns will trip you if you don't expect them.
- **Conditional aggregates with `SUM(CASE WHEN ...)`** replace MySQL's missing `FILTER` clause.

*← [07 joins](./07_joins.md) | [next → Subqueries & CTEs](./09_subqueries_ctes.md)*
