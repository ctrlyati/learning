# 07 — JOINs: INNER, LEFT, RIGHT, CROSS, SELF

> **Goal:** Combine tables fluently. Know which join to reach for, what the engine does behind the scenes, and which join bugs are most likely to ship to production.

---

## 1. JOINs — analogy + runnable SQL

A join takes two tables and produces one result by matching rows. Imagine two stacks of cards: one of customers, one of orders. A join is the act of laying them out and pairing them up by some common attribute (customer_id).

```sql
SELECT c.first_name, c.last_name, r.rental_date
FROM customer c
JOIN rental r ON r.customer_id = c.customer_id
LIMIT 10;
```

Read it: "for every customer row, find rental rows where customer_id matches; emit one combined row per match."

---

## 2. The five join types — diagrams + mechanism

Two tables A and B:

```
A          B
┌─┬─┐      ┌─┬─┐
│1│a│      │1│x│
│2│b│      │3│y│
│3│c│      │4│z│
└─┴─┘      └─┴─┘
```

Joining `A.id = B.id`:

### INNER JOIN — rows that match in BOTH

```
       A   B
      ┌───┬───┐
      │ A ∩ B │
      └───┬───┘

INNER:  A.id = B.id
Result: (1,a,x), (3,c,y)
```

```sql
SELECT * FROM A INNER JOIN B ON A.id = B.id;
-- (`JOIN` alone = INNER JOIN)
```

### LEFT JOIN — all rows from A, matched B (or NULLs)

```
       A   B
   ┌───┬───┬───┐
   │ A_only│ A∩B │
   └───────┴─────┘

LEFT:   keep all A
Result: (1,a,x), (2,b,NULL), (3,c,y)
```

```sql
SELECT * FROM A LEFT JOIN B ON A.id = B.id;
```

Use to find "everything in A, plus matching B if any." Also the canonical "find A rows with no B" pattern:
```sql
SELECT A.*
FROM A LEFT JOIN B ON A.id = B.id
WHERE B.id IS NULL;
```

### RIGHT JOIN — mirror of LEFT
```sql
SELECT * FROM A RIGHT JOIN B ON A.id = B.id;
-- equivalent to: SELECT * FROM B LEFT JOIN A ON A.id = B.id;
```
**Convention:** never use RIGHT JOIN. Always rewrite as LEFT JOIN with tables swapped — uniform direction makes queries easier to read.

### FULL OUTER JOIN — everything from both
MySQL **does not support FULL OUTER JOIN** directly. Emulate:
```sql
SELECT * FROM A LEFT JOIN B ON A.id = B.id
UNION
SELECT * FROM A RIGHT JOIN B ON A.id = B.id;
```

### CROSS JOIN — Cartesian product
Every row in A paired with every row in B. M × N rows.

```
CROSS:
(1,a,1,x) (1,a,3,y) (1,a,4,z)
(2,b,1,x) (2,b,3,y) (2,b,4,z)
(3,c,1,x) (3,c,3,y) (3,c,4,z)
```

```sql
SELECT * FROM A CROSS JOIN B;
-- equivalent to: SELECT * FROM A, B;   (comma join — old, avoid)
```

Useful for: generating combinations (e.g., date dimension × product dimension), tally tables. Disastrous if accidental.

### SELF JOIN — table joined with itself
For hierarchies and pairwise comparisons. Sakila has a friendlier example with the `staff` table; here's an employee/manager pattern:

```sql
CREATE TABLE employee (
  emp_id INT PRIMARY KEY,
  name   VARCHAR(50),
  manager_id INT
);

INSERT INTO employee VALUES
  (1, 'Alice', NULL),
  (2, 'Bob', 1),
  (3, 'Carol', 1),
  (4, 'Dave', 2);

SELECT e.name AS employee, m.name AS manager
FROM employee e
LEFT JOIN employee m ON m.emp_id = e.manager_id;
```

Result:
```
employee | manager
Alice    | NULL
Bob      | Alice
Carol    | Alice
Dave     | Bob
```

For deeper trees, use **recursive CTEs** (Module 09).

---

## 3. JOIN execution — how MySQL actually does it

MySQL's optimizer picks one of these algorithms:

### Nested-loop join (default)
```
for each row r1 in driving_table:
    for each matching row r2 in joined_table:
        emit (r1, r2)
```
Speed depends entirely on whether the inner loop has an **index** on the join key. Without it: O(N×M). With it: O(N × log M).

### Block nested-loop join
Buffers chunks of the driving table to amortize inner-table scans.

### Hash join (MySQL 8.0.18+, default in 8.0.20+ for equi-joins without indexes)
Builds a hash table on the smaller side, probes with the larger. Fast for big unindexed equi-joins.

### EXPLAIN to see which
```sql
EXPLAIN SELECT * FROM customer c JOIN rental r ON r.customer_id = c.customer_id;
```
Look for `Using join buffer (hash join)` in the Extra column on 8.0.18+, or per-row indexes in the `key` column.

### Join order
The optimizer reorders joins by cost. Generally it picks the smallest filtered table to drive the loop. You can force order with `STRAIGHT_JOIN`:
```sql
SELECT STRAIGHT_JOIN ... FROM small_table sj JOIN big_table bt ON ...;
```
Rarely needed; the optimizer is usually right in MySQL 8.

---

## 4. Practical application — Sakila join patterns

```sql
-- 1. Films and their language
SELECT f.title, l.name AS language
FROM film f
JOIN language l ON l.language_id = f.language_id;

-- 2. Customers and their addresses (drill-down through normalization)
SELECT c.first_name, c.last_name, a.address, ci.city, co.country
FROM customer c
JOIN address a  ON a.address_id = c.address_id
JOIN city ci    ON ci.city_id = a.city_id
JOIN country co ON co.country_id = ci.country_id
LIMIT 10;

-- 3. LEFT JOIN to find customers who never rented
SELECT c.customer_id, c.first_name, c.last_name
FROM customer c
LEFT JOIN rental r ON r.customer_id = c.customer_id
WHERE r.rental_id IS NULL;

-- 4. Many-to-many: actors in a film
SELECT a.first_name, a.last_name
FROM film f
JOIN film_actor fa ON fa.film_id = f.film_id
JOIN actor a       ON a.actor_id = fa.actor_id
WHERE f.title = 'ACADEMY DINOSAUR';

-- 5. Self-join: find pairs of actors who appeared in the same film
SELECT DISTINCT a1.actor_id, a2.actor_id
FROM film_actor a1
JOIN film_actor a2
  ON a1.film_id = a2.film_id
 AND a1.actor_id < a2.actor_id   -- avoid duplicates and self-pairs
LIMIT 10;

-- 6. Aggregation across a join (preview of Module 08)
SELECT c.customer_id, c.first_name, COUNT(r.rental_id) AS rental_count
FROM customer c
LEFT JOIN rental r ON r.customer_id = c.customer_id
GROUP BY c.customer_id, c.first_name
ORDER BY rental_count DESC
LIMIT 10;

-- 7. CROSS JOIN: all combinations of category × rating (good for reporting templates)
SELECT cat.name, ratings.rating
FROM category cat
CROSS JOIN (SELECT 'G' AS rating UNION SELECT 'PG' UNION SELECT 'R') ratings;
```

### Critical gotcha: filtering after a LEFT JOIN

```sql
-- BUG: this becomes effectively an INNER JOIN
SELECT c.*, r.rental_date
FROM customer c
LEFT JOIN rental r ON r.customer_id = c.customer_id
WHERE r.rental_date > '2005-08-01';
-- Customers with no rentals have NULL rental_date,
-- and NULL > anything is NULL, so they're filtered out.

-- FIX: move the predicate into the ON clause
SELECT c.*, r.rental_date
FROM customer c
LEFT JOIN rental r
  ON r.customer_id = c.customer_id
  AND r.rental_date > '2005-08-01';
```

This is one of the highest-frequency SQL bugs in code review. Memorize it.

---

## 5. Common Mistakes & Gotchas

- **WHERE-vs-ON on outer joins.** As above. Conditions on the *outer* (LEFT side) table go in WHERE; conditions on the *inner* (RIGHT side, possibly NULL) go in ON.
- **Forgetting a join condition.** `FROM A, B` with no WHERE is a CROSS JOIN — millions of rows. Always be explicit with `JOIN ... ON`.
- **Ambiguous column names.** When two joined tables both have `id`, `SELECT id` errors. Always alias tables and qualify columns: `SELECT c.customer_id, r.rental_id`.
- **Joining on the wrong key.** `ON c.customer_id = r.staff_id` — typo, but produces an answer (just a wrong one). Always sanity-check row counts.
- **N+1 in app code.** Iterating a parent list and fetching children one-by-one. Use a single JOIN instead.
- **Self-join without inequality.** Pair-finding self-joins need `a1.id < a2.id` to avoid (X,X) and (X,Y)+(Y,X).
- **DISTINCT to mask duplicate-row bugs from joins.** If `SELECT DISTINCT` is needed, you probably have a too-permissive join — fix the cause, not the symptom.
- **`USING (col)` vs `ON`.** `USING (customer_id)` is a shorthand when both columns share a name. Cleaner, but most teams stick with `ON` for explicitness.
- **vs. older MySQL:** before 8.0.18, no hash join. Big un-indexed joins were brutally slow. If you're on 5.7, indexes on join keys are non-negotiable.

---

## 🎯 Key Takeaways

- **INNER for matches; LEFT for "all of A, optional B".** RIGHT and FULL OUTER you'll rarely need (and FULL OUTER doesn't exist in MySQL).
- **Filter outer-join inner table conditions in ON, not WHERE** — or you'll silently degrade to an INNER JOIN.
- **Indexes on join keys are mandatory** for any production-scale join. Hash join (8.0.18+) saves you when they're missing, but at IO cost.
- **Always alias and qualify** in multi-table queries. Future-you reading the code will thank present-you.
- **Self-joins model hierarchies** but only one level. Recursive CTEs (Module 09) handle arbitrary depth.

*← [06 filtering](./06_filtering_deeper.md) | [next → Aggregations](./08_aggregations.md)*
