# 04 — DDL and Constraints

> **Goal:** Use `CREATE`, `ALTER`, `DROP`, and constraints (PK, FK, UNIQUE, CHECK, NOT NULL, DEFAULT) to express invariants in the schema rather than the application.

---

## 1. DDL — analogy + runnable SQL

DDL = Data Definition Language. If DML (Module 05) is the verbs of "what data does," DDL is the nouns of "what tables exist and what shape they take." Think of DDL as setting up the rooms in a house, and DML as moving furniture in and out.

```sql
CREATE DATABASE shop CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
USE shop;

CREATE TABLE category (
  category_id INT AUTO_INCREMENT PRIMARY KEY,
  name        VARCHAR(50) NOT NULL UNIQUE
);

CREATE TABLE product (
  product_id   INT AUTO_INCREMENT PRIMARY KEY,
  sku          VARCHAR(32) NOT NULL UNIQUE,
  name         VARCHAR(200) NOT NULL,
  price_cents  INT UNSIGNED NOT NULL CHECK (price_cents > 0),
  category_id  INT NOT NULL,
  is_active    TINYINT(1) NOT NULL DEFAULT 1,
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT fk_product_category
    FOREIGN KEY (category_id) REFERENCES category(category_id)
    ON DELETE RESTRICT
);
```

Six constraints in one table, every invariant the data must satisfy is now the database's job to enforce.

---

## 2. The five constraint kinds — how MySQL enforces them

### NOT NULL
The simplest and most underused. NULL means "unknown" and propagates through expressions in surprising ways (Module 06). Default to `NOT NULL` and add NULL only with intent.

### UNIQUE
A constraint backed by a unique index. Allows multiple NULLs (per SQL standard, MySQL implements it).
```sql
ALTER TABLE product ADD UNIQUE (sku);
```

### PRIMARY KEY
NOT NULL + UNIQUE + clusters the row physically (in InnoDB).

### FOREIGN KEY
Enforces that a value exists in another table.
```sql
ALTER TABLE product
  ADD CONSTRAINT fk_product_category
  FOREIGN KEY (category_id) REFERENCES category(category_id)
  ON DELETE RESTRICT
  ON UPDATE CASCADE;
```
- MySQL silently creates an index on the FK column if one doesn't exist (because every parent-side modification must check).
- FKs are only enforced by **InnoDB**, not MyISAM.

### CHECK (MySQL 8.0.16+)
```sql
ALTER TABLE product ADD CONSTRAINT chk_price CHECK (price_cents > 0);
```
Before 8.0.16, MySQL parsed CHECK constraints but ignored them — a famous footgun. In 8.0.16+ they actually enforce.

### DEFAULT
A value used when the column is omitted from INSERT.
```sql
created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
```
MySQL 8 finally allows expressions in DEFAULT (`DEFAULT (UUID())`, etc.).

---

## 3. ALTER TABLE — depth and operational reality

`ALTER` is where production engineers get scared, because:
- For large tables, ALTER may rebuild the whole table.
- It can lock writes (or reads) while running.
- It can take hours.

MySQL 8 supports several **algorithms**:
- `ALGORITHM=INSTANT` (8.0.12+) — metadata-only changes (add column at end, drop default, rename). Sub-second on any size.
- `ALGORITHM=INPLACE` — rebuilds in place, allows concurrent reads/writes for many ops.
- `ALGORITHM=COPY` — full table copy, blocks writes. Old behavior.

```sql
ALTER TABLE product
  ADD COLUMN weight_grams INT,
  ALGORITHM=INSTANT, LOCK=NONE;
```

If the requested algorithm isn't supported, MySQL errors instead of silently downgrading. That's good — you find out before production.

For huge tables in production, use:
- **gh-ost** (GitHub's online schema change tool)
- **pt-online-schema-change** (Percona Toolkit)

These do the change on a shadow table while replicating writes, then atomically swap.

### Common ALTER operations

```sql
-- Add a column
ALTER TABLE product ADD COLUMN brand VARCHAR(100);

-- Add with default and position (position is MySQL-specific)
ALTER TABLE product ADD COLUMN slug VARCHAR(200) NOT NULL DEFAULT '' AFTER name;

-- Modify type (be careful: data may be truncated/rejected)
ALTER TABLE product MODIFY COLUMN name VARCHAR(300);

-- Rename column (8.0+ syntax)
ALTER TABLE product RENAME COLUMN brand TO manufacturer;

-- Drop column
ALTER TABLE product DROP COLUMN manufacturer;

-- Rename table
RENAME TABLE old_name TO new_name;

-- Add index
ALTER TABLE product ADD INDEX idx_category (category_id);

-- Drop index
ALTER TABLE product DROP INDEX idx_category;
```

### DROP — mostly irreversible

```sql
DROP TABLE product;
DROP DATABASE shop;
```

There's no recycle bin. Always have backups (Module 15). Some teams enforce `RENAME TABLE product TO _trash_product_2026_05_11` then drop after a week.

---

## 4. Practical application — generated columns and JSON indexing

Suppose you have a JSON column with mixed event payloads, and you want to index `payload.user_id`:

```sql
CREATE TABLE event_log (
  id      BIGINT AUTO_INCREMENT PRIMARY KEY,
  payload JSON NOT NULL,
  user_id INT GENERATED ALWAYS AS (payload->>'$.user_id') STORED,
  INDEX idx_user (user_id)
);

INSERT INTO event_log (payload) VALUES
  ('{"user_id": 42, "type": "login"}'),
  ('{"user_id": 17, "type": "logout"}');

SELECT * FROM event_log WHERE user_id = 42;  -- uses idx_user
```

`STORED` writes the computed value to disk; `VIRTUAL` doesn't and recomputes on read (but you can still index virtual cols on InnoDB — MySQL stores the index entries).

This is the modern pattern for indexed JSON: keep schema-flexible writes, get B-tree lookup speed on the queries you care about.

### A complete normalized + constrained schema

```sql
CREATE TABLE customer (
  customer_id  INT AUTO_INCREMENT PRIMARY KEY,
  email        VARCHAR(255) NOT NULL UNIQUE,
  full_name    VARCHAR(200) NOT NULL,
  status       ENUM('active','suspended','deleted') NOT NULL DEFAULT 'active',
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK (email LIKE '%@%')
);

CREATE TABLE address (
  address_id   INT AUTO_INCREMENT PRIMARY KEY,
  customer_id  INT NOT NULL,
  line1        VARCHAR(200) NOT NULL,
  city         VARCHAR(100) NOT NULL,
  postal_code  VARCHAR(20)  NOT NULL,
  country_code CHAR(2)      NOT NULL,
  is_primary   TINYINT(1)   NOT NULL DEFAULT 0,
  CONSTRAINT fk_address_customer
    FOREIGN KEY (customer_id) REFERENCES customer(customer_id)
    ON DELETE CASCADE
);

CREATE INDEX idx_address_customer ON address(customer_id);
```

Notice:
- Every column has a deliberate type and nullability.
- Every relationship has an FK.
- A CHECK enforces a basic email shape.
- An ENUM constrains status values.
- An index supports the most common access pattern (`WHERE customer_id = ?`).

This schema doesn't need application-side validation for these invariants. The DB rejects bad data. That's defense in depth.

---

## 5. Common Mistakes & Gotchas

- **Trusting CHECK constraints in MySQL <8.0.16.** They were syntactically accepted, silently ignored. A *lot* of legacy schemas have non-enforced CHECKs.
- **Schema changes that lock writes for hours.** Test ALTERs on a copy of production-sized data first. Use `ALGORITHM=INSTANT/INPLACE` or gh-ost.
- **Adding a NOT NULL column without DEFAULT** to a table with rows. MySQL needs a value for existing rows; if no default, it'll either error or silently use type's "zero value" depending on `sql_mode`. Always specify DEFAULT.
- **`SET sql_mode=''`** to silence errors. Don't. The strict modes (`STRICT_TRANS_TABLES`, `NO_ZERO_DATE`) catch real bugs. Default mode in MySQL 8 is sensible — leave it.
- **No FK because "FKs are slow."** They cost a fast index lookup per write. The cost of *not* having them — orphan rows, undetected app bugs — is larger.
- **Implicit DEFAULT '0000-00-00'** for DATE in old MySQL. Modern strict mode rejects it. Set defaults explicitly.
- **Renaming columns referenced by views or stored procs.** MySQL doesn't always cascade-update; the view breaks at run time. Audit before renaming.
- **vs. PostgreSQL:** Postgres has DDL transactions — you can `BEGIN; ALTER ...; ROLLBACK;`. **MySQL does not.** Every DDL is auto-committed. There's no undo. Plan accordingly.

---

## 🎯 Key Takeaways

- **Constraints are the schema's contract.** Express invariants in the database, not just the app.
- **CHECK works in 8.0.16+.** If you're on older MySQL, audit — you may have CHECKs that do nothing.
- **DDL in MySQL is auto-committed.** No transactional rollback. Test on a copy.
- **Use INSTANT/INPLACE algorithms** for online ALTERs; use gh-ost / pt-osc for big tables in production.
- **FK constraints are cheap insurance.** The few apps that "turned them off for perf" usually regretted it.

*← [03 data types](./03_data_types.md) | [next → DML Basics](./05_dml_basics.md)*
