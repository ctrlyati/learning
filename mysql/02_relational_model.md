# 02 — The Relational Model

> **Goal:** Internalize tables, rows, columns, keys, and the first three normal forms — the substrate everything else in this course rests on.

---

## 1. The relational model — analogy + runnable SQL

Imagine a filing cabinet. Each **drawer** is a table. Each **folder** in a drawer is a row. Each folder has the same set of **labeled tabs** — those are columns. You can pull any folder out, slide it back, or cross-reference it to a folder in another drawer using a label both share (a **key**).

That's it. The relational model is a 50-year-old idea (E. F. Codd, IBM, 1970) that databases are sets of tables, where rows are tuples and columns are attributes, and you query them with set operations.

```sql
CREATE TABLE author (
    author_id INT PRIMARY KEY,
    name      VARCHAR(100) NOT NULL
);

CREATE TABLE book (
    book_id   INT PRIMARY KEY,
    title     VARCHAR(200) NOT NULL,
    author_id INT NOT NULL,
    FOREIGN KEY (author_id) REFERENCES author(author_id)
);

INSERT INTO author VALUES (1, 'Ursula K. Le Guin'), (2, 'Ted Chiang');
INSERT INTO book VALUES
  (1, 'The Dispossessed', 1),
  (2, 'A Wizard of Earthsea', 1),
  (3, 'Stories of Your Life', 2);

SELECT a.name, b.title
FROM author a
JOIN book b ON b.author_id = a.author_id;
```

Two tables, one foreign-key relationship, and a join. You've just used the entire relational model.

---

## 2. Keys — the mechanism that makes it all work

Without keys, a table is just a spreadsheet. Keys are how rows are identified, related, and constrained.

### Primary key (PK)
- Unique, NOT NULL, exactly one per table.
- Identifies a row.
- In InnoDB (MySQL's default storage engine), the PK *is* the physical sort order on disk — this is huge, see Module 11.

```sql
CREATE TABLE customer (
  customer_id INT AUTO_INCREMENT PRIMARY KEY,
  email       VARCHAR(255) NOT NULL
);
```

### Surrogate vs. natural keys
- **Surrogate** = meaningless ID assigned by the system (`AUTO_INCREMENT`, UUID). Stable, opaque.
- **Natural** = real-world identifier (email, ISBN, SSN). Meaningful but mutable and sometimes non-unique.

Industry default: **use surrogate PKs**. Natural identifiers go in `UNIQUE` constraints.

```sql
CREATE TABLE customer (
  customer_id INT AUTO_INCREMENT PRIMARY KEY,  -- surrogate
  email       VARCHAR(255) NOT NULL UNIQUE     -- natural, enforced
);
```

### Foreign key (FK)
A column that references another table's PK. Enforces referential integrity: you can't insert a `book.author_id` that doesn't exist in `author.author_id`.

```sql
ALTER TABLE book
  ADD CONSTRAINT fk_book_author
  FOREIGN KEY (author_id) REFERENCES author(author_id)
  ON DELETE RESTRICT
  ON UPDATE CASCADE;
```

`ON DELETE` options:
- `RESTRICT` (default) — refuse to delete the parent if children exist.
- `CASCADE` — delete children too. Dangerous; explicit.
- `SET NULL` — children's FK becomes NULL.
- `NO ACTION` — same as RESTRICT in InnoDB.

### Composite key
A PK or unique constraint over multiple columns:
```sql
CREATE TABLE film_actor (
  film_id  INT,
  actor_id INT,
  PRIMARY KEY (film_id, actor_id)
);
```
Common in junction tables (many-to-many).

---

## 3. Normalization — depth in five steps

Normalization is the process of organizing data to eliminate redundancy. It's expressed as **normal forms** (1NF, 2NF, 3NF, BCNF, 4NF, 5NF). In practice 99% of OLTP schemas aim for **3NF** (with deliberate denormalization where measured perf demands it).

### Starting point: an unnormalized mess
```
| order_id | customer_name | customer_email   | items                              |
|----------|---------------|------------------|------------------------------------|
| 1        | Alice         | alice@x.com      | "shirt:2:25.00, hat:1:15.00"       |
| 2        | Alice         | alice@x.com      | "shoes:1:60.00"                    |
```

Three problems:
- `items` is a string holding multiple values (violates 1NF).
- Customer name/email duplicated across orders (update anomaly: change Alice's email and you must update every row).
- Hard to query: "how many shirts sold last month?" requires string parsing.

### 1NF — Atomic values, no repeating groups
Each cell holds one value. No comma-separated lists, no JSON shoehorned for relational data, no `item1, item2, item3` columns.

```sql
CREATE TABLE order_item (
  order_id  INT,
  product   VARCHAR(50),
  qty       INT,
  unit_price DECIMAL(10,2),
  PRIMARY KEY (order_id, product)
);
```

### 2NF — No partial dependencies on a composite key
Every non-key column depends on the **whole** PK, not just part of it.

If `order_item` had a `product_name` column, it would only depend on `product` (part of the PK), not on `order_id`. Move it out:
```sql
CREATE TABLE product (
  product_id   INT PRIMARY KEY,
  product_name VARCHAR(100),
  unit_price   DECIMAL(10,2)
);
```

### 3NF — No transitive dependencies
Non-key columns must depend on the PK *directly*, not via another non-key column.

In an `employee` table with `(emp_id, dept_id, dept_name)`, `dept_name` depends on `dept_id`, which depends on `emp_id`. That's transitive. Split:
```sql
CREATE TABLE department (
  dept_id   INT PRIMARY KEY,
  dept_name VARCHAR(100)
);
CREATE TABLE employee (
  emp_id  INT PRIMARY KEY,
  dept_id INT REFERENCES department(dept_id)
);
```

### Rule of thumb (Codd's rhyme)
> Every non-key attribute must provide a fact about **the key** (1NF), **the whole key** (2NF), and **nothing but the key** (3NF) — so help me Codd.

### When to denormalize
Deliberately, with measurements. Common cases:
- **Materialized aggregates** (`order_count` on `customer`) for read-heavy reporting.
- **Caching joined data** (denormalized `country_name` on `address`) when the join cost dominates.
- **JSON columns** for sparse, schema-flexible data (Module 03).

Never denormalize "because it'll be faster" without an EXPLAIN-backed reason.

---

## 4. Practical application — read Sakila as a normalized schema

Sakila is a textbook 3NF design. Open Workbench → reverse-engineer schema → see the diagram. Key relationships:

```
customer ──┐
           ├── rental ── inventory ── film ──┬── film_category ── category
staff ─────┘                                 └── film_actor ── actor
payment ── rental
address ── city ── country
```

Notice:
- `address`, `city`, `country` are three tables, not three columns on `customer`. Why? An address belongs to many things (customer, store, staff). Extracting it = no duplication.
- `film_actor` is a pure junction table for the many-to-many between films and actors.
- `payment` references `rental`, `customer`, and `staff` — a transactional fact table.

A query that uses the model:
```sql
SELECT c.first_name, c.last_name, ci.city, co.country
FROM customer c
JOIN address a  ON a.address_id = c.address_id
JOIN city ci    ON ci.city_id   = a.city_id
JOIN country co ON co.country_id = ci.country_id
WHERE co.country = 'Canada';
```

Four joins to get a denormalized view. That's 3NF: storage is cheap, joins are fast (with indexes), updates are safe.

---

## 5. Common Mistakes & Gotchas

- **Storing CSVs/JSON arrays in a column** to "save a join." You will regret it the first time you need to query for items. Use a junction table.
- **Using natural keys as PKs.** Email changes. Phone numbers change. ISBNs get reissued. Use a surrogate, then put the natural key in a `UNIQUE`.
- **No PK at all.** InnoDB will silently invent a hidden 6-byte rowid. You can't reference it, can't optimize on it. Always declare a PK.
- **Wide PKs (e.g., a composite of 4 VARCHARs).** Every secondary index in InnoDB stores the PK as the row pointer. A 200-byte PK explodes every index. Prefer a slim INT/BIGINT PK.
- **`ON DELETE CASCADE` everywhere.** One day someone deletes a `country` row and 600 customers vanish. Use RESTRICT by default, CASCADE only where it semantically makes sense (e.g., deleting an `order` should cascade to `order_item`).
- **Forgetting that NULL is not a value.** A `UNIQUE` constraint allows multiple NULLs in MySQL (and most RDBMS). Don't rely on uniqueness if the column is nullable.
- **vs. NoSQL mental models:** if you've come from MongoDB, your instinct is to embed. Resist it. The relational model rewards splitting, then joining at read time.

---

## 🎯 Key Takeaways

- **Surrogate PKs by default**, natural identifiers as `UNIQUE`. Surrogates are stable; reality isn't.
- **3NF is the OLTP target.** Denormalize only when measurements force it, never speculatively.
- **Foreign keys aren't optional.** They're how the database enforces invariants you'd otherwise re-implement (badly) in app code.
- **Junction tables model many-to-many.** No arrays, no comma-separated lists. Ever.
- **InnoDB's PK is the on-disk sort order.** Every column choice you make for the PK has perf consequences in Module 11.

*← [01 setup](./01_setup_and_client.md) | [next → Data Types](./03_data_types.md)*
