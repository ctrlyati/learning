# 10 — Window Functions

> **Goal:** Use ROW_NUMBER, RANK, LAG, LEAD, SUM OVER, and friends to compute per-row analytics without collapsing rows — and replace 100 lines of application code with one query.

---

## 1. Window functions — analogy + runnable SQL

A regular aggregate (`SUM`, `COUNT`) collapses many rows into one. A window function computes the same kind of thing **but keeps every row**, attaching the aggregate as a new column.

Think: a moving sidebar of context for each row.

```sql
SELECT
  film_id,
  title,
  rental_rate,
  AVG(rental_rate) OVER ()  AS overall_avg,
  rental_rate - AVG(rental_rate) OVER () AS diff_from_avg
FROM film
LIMIT 5;
```

Result: every row keeps its identity *and* gets the population average attached. No GROUP BY required.

Available in MySQL since 8.0.

---

## 2. The OVER clause — the mechanism

The general form:
```
function() OVER (
  PARTITION BY <columns>     -- buckets, like GROUP BY but doesn't collapse
  ORDER BY    <columns>      -- ordering within each partition
  <frame>                    -- ROWS or RANGE BETWEEN ... AND ...
)
```

Pieces:
- **PARTITION BY** — divide rows into buckets. The window function operates per partition.
- **ORDER BY** (inside OVER) — defines order for ranking and frame computations.
- **Frame** — the subset of rows in the partition that the function sees. Defaults vary by function.

```sql
SELECT
  customer_id,
  payment_id,
  amount,
  SUM(amount) OVER (PARTITION BY customer_id ORDER BY payment_date)
    AS running_total
FROM payment;
```

For each customer, a running sum of payments in chronological order.

### Frames
```
ROWS BETWEEN N PRECEDING AND CURRENT ROW
ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW   -- default for cumulative
ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING
ROWS BETWEEN 3 PRECEDING AND 3 FOLLOWING
```

`ROWS` counts physical rows; `RANGE` groups by the ORDER BY value (peer rows treated together).

Example — 7-day moving average of revenue:
```sql
SELECT day, revenue,
  AVG(revenue) OVER (ORDER BY day ROWS BETWEEN 6 PRECEDING AND CURRENT ROW) AS ma7
FROM (
  SELECT DATE(payment_date) AS day, SUM(amount) AS revenue
  FROM payment GROUP BY day
) d;
```

---

## 3. The function catalog — depth

### Ranking functions

| Function       | Behavior                                              |
|----------------|-------------------------------------------------------|
| ROW_NUMBER()   | 1, 2, 3, 4, 5 — unique sequential                     |
| RANK()         | 1, 2, 2, 4, 5 — ties share rank, gaps after           |
| DENSE_RANK()   | 1, 2, 2, 3, 4 — ties share, no gap                    |
| PERCENT_RANK() | (rank-1)/(N-1) — 0..1                                 |
| NTILE(N)       | divide partition into N buckets, label 1..N           |

```sql
-- Top 3 most-rented films per category
WITH rentals_by_film AS (
  SELECT i.film_id, COUNT(*) AS rentals
  FROM rental r
  JOIN inventory i ON i.inventory_id = r.inventory_id
  GROUP BY i.film_id
),
ranked AS (
  SELECT c.name AS category, f.title, rbf.rentals,
    RANK() OVER (PARTITION BY c.category_id ORDER BY rbf.rentals DESC) AS rk
  FROM rentals_by_film rbf
  JOIN film f           ON f.film_id = rbf.film_id
  JOIN film_category fc ON fc.film_id = f.film_id
  JOIN category c       ON c.category_id = fc.category_id
)
SELECT * FROM ranked WHERE rk <= 3 ORDER BY category, rk;
```

This is a top-N-per-group query — historically a nightmare in MySQL pre-8.0. Now: trivial.

### Offset / value-from-other-row functions

| Function           | Returns                                          |
|--------------------|--------------------------------------------------|
| LAG(col, N=1)      | value N rows before current                       |
| LEAD(col, N=1)     | value N rows after current                        |
| FIRST_VALUE(col)   | first value in frame                              |
| LAST_VALUE(col)    | last value in frame (watch the frame default!)    |
| NTH_VALUE(col, n)  | nth value in frame                                |

```sql
-- Day-over-day revenue change
WITH daily AS (
  SELECT DATE(payment_date) AS day, SUM(amount) AS revenue
  FROM payment GROUP BY day
)
SELECT day, revenue,
  LAG(revenue) OVER (ORDER BY day) AS prev_day,
  revenue - LAG(revenue) OVER (ORDER BY day) AS delta
FROM daily;
```

### Cumulative aggregates
Any aggregate (`SUM`, `COUNT`, `AVG`, `MIN`, `MAX`) becomes a window function with `OVER`.

```sql
-- Cumulative payments per customer
SELECT customer_id, payment_id, amount, payment_date,
  SUM(amount) OVER (PARTITION BY customer_id ORDER BY payment_date) AS running_total
FROM payment
ORDER BY customer_id, payment_date;
```

### Named windows — DRY
```sql
SELECT
  customer_id, amount,
  SUM(amount)  OVER w AS running,
  COUNT(*)     OVER w AS nth
FROM payment
WINDOW w AS (PARTITION BY customer_id ORDER BY payment_date);
```

---

## 4. Practical application — Sakila analytics

```sql
-- 1. Customer lifetime value with rank
SELECT customer_id,
       SUM(amount) AS lifetime_value,
       RANK() OVER (ORDER BY SUM(amount) DESC) AS rank_overall
FROM payment
GROUP BY customer_id
ORDER BY rank_overall
LIMIT 20;

-- 2. Per-store top spenders
WITH cust_spend AS (
  SELECT c.store_id, c.customer_id, SUM(p.amount) AS spend
  FROM customer c
  JOIN payment p ON p.customer_id = c.customer_id
  GROUP BY c.store_id, c.customer_id
)
SELECT store_id, customer_id, spend,
  ROW_NUMBER() OVER (PARTITION BY store_id ORDER BY spend DESC) AS rn
FROM cust_spend
QUALIFY rn <= 5;
-- NOTE: MySQL doesn't have QUALIFY (Snowflake/BigQuery do). Use a subquery:
SELECT * FROM (
  SELECT store_id, customer_id, spend,
    ROW_NUMBER() OVER (PARTITION BY store_id ORDER BY spend DESC) AS rn
  FROM cust_spend
) t WHERE rn <= 5;

-- 3. Rolling 7-day rentals
SELECT day, n,
  SUM(n) OVER (ORDER BY day ROWS BETWEEN 6 PRECEDING AND CURRENT ROW) AS rolling7
FROM (
  SELECT DATE(rental_date) AS day, COUNT(*) AS n
  FROM rental GROUP BY day
) d
ORDER BY day;

-- 4. Time between consecutive rentals per customer
SELECT customer_id, rental_id, rental_date,
  rental_date - LAG(rental_date) OVER (PARTITION BY customer_id ORDER BY rental_date)
    AS gap
FROM rental
LIMIT 50;

-- 5. Bucket customers into quartiles by spend
SELECT customer_id, SUM(amount) AS spend,
  NTILE(4) OVER (ORDER BY SUM(amount)) AS quartile
FROM payment
GROUP BY customer_id;

-- 6. Sessionization preview: detect new "rental session" if gap > 7 days
WITH gaps AS (
  SELECT customer_id, rental_id, rental_date,
    LAG(rental_date) OVER (PARTITION BY customer_id ORDER BY rental_date) AS prev
  FROM rental
),
flagged AS (
  SELECT *,
    CASE WHEN prev IS NULL OR rental_date > prev + INTERVAL 7 DAY THEN 1 ELSE 0 END AS new_session
  FROM gaps
)
SELECT customer_id, rental_id, rental_date,
  SUM(new_session) OVER (PARTITION BY customer_id ORDER BY rental_date) AS session_id
FROM flagged
LIMIT 50;
```

That last query — **gap-and-island sessionization** — is a famous window-function pattern. It would take ~50 lines of application code. Here it's 10 lines of SQL.

---

## 5. Common Mistakes & Gotchas

- **Using window functions in WHERE.** Windows compute *after* WHERE/GROUP BY/HAVING, so you can't filter on them in WHERE. Wrap in a subquery / CTE: `SELECT * FROM (SELECT ..., RANK() OVER ... AS rk FROM t) x WHERE rk <= 3`.
- **Forgetting that LAST_VALUE's default frame** is `RANGE BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW` — so it returns the *current* row, not the partition's last value. Use `ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING` to fix.
- **PARTITION BY too coarse.** If you only `OVER (ORDER BY ...)`, you partition over the whole result — sometimes desired, sometimes a bug.
- **Comparing window functions to GROUP BY.** They're different: GROUP BY collapses; OVER preserves rows.
- **Performance.** Window functions sort within each partition. Without supporting indexes on PARTITION BY + ORDER BY columns, MySQL does a filesort.
- **Confusing RANK vs DENSE_RANK vs ROW_NUMBER.** Tied rows: ROW_NUMBER is arbitrary, RANK gaps, DENSE_RANK no gaps. Pick deliberately.
- **No `QUALIFY` in MySQL.** Always wrap.
- **vs. SQL Server / Postgres:** Postgres and SQL Server have had window functions since the 2000s. MySQL added them in 8.0 (2018). Lots of "MySQL doesn't have X" StackOverflow answers are stale.

---

## 🎯 Key Takeaways

- **Window = aggregate without collapsing rows.** Every row gets context from its bucket.
- **PARTITION BY is the bucket; ORDER BY inside OVER is the order; the frame is the slice.** Three knobs.
- **Top-N-per-group = ROW_NUMBER + WHERE in outer query.** This pattern alone justifies learning windows.
- **LAG / LEAD replace self-joins for "previous row" patterns** — cleaner, faster, harder to get wrong.
- **Gap-and-island sessionization** is a window-function superpower for analytics work.

*← [09 subqueries & ctes](./09_subqueries_ctes.md) | [next → Indexes](./11_indexes.md)*
