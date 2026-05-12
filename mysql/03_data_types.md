# 03 — Data Types

> **Goal:** Pick the right MySQL type for every column — narrow enough to save space and indexing cost, wide enough to never overflow.

---

## 1. Why types matter — analogy + runnable SQL

A column type is a contract. It tells MySQL: "Every value in here looks like *this*, takes *this much* space, sorts in *this* order." If you pick wrong, you pay forever — in disk, in RAM (the buffer pool caches pages), in index size, in query speed, in subtle bugs (truncated strings, rounded money, timezone drift).

Think of it like containers in a warehouse. You wouldn't ship a single screw in a shipping container, nor a refrigerator in a padded envelope. Same with columns.

```sql
-- Wrong: 'BIGINT for a boolean' wastes 7 bytes per row, 7 GB per billion rows.
CREATE TABLE bad (is_active BIGINT);

-- Right
CREATE TABLE good (is_active TINYINT(1) NOT NULL DEFAULT 0);
```

---

## 2. The major type families — how MySQL stores them

### Numeric

| Type        | Bytes | Range (signed)                                    |
|-------------|-------|---------------------------------------------------|
| TINYINT     | 1     | -128 .. 127                                       |
| SMALLINT    | 2     | -32,768 .. 32,767                                 |
| MEDIUMINT   | 3     | -8M .. 8M                                         |
| INT         | 4     | -2.1B .. 2.1B                                     |
| BIGINT      | 8     | -9.2 quintillion .. 9.2 quintillion               |
| DECIMAL(M,D)| varies| exact, up to 65 digits                            |
| FLOAT       | 4     | ~7 significant digits, **inexact**                |
| DOUBLE      | 8     | ~15 significant digits, **inexact**               |

**Rule:** integers for IDs and counters, `DECIMAL` for money, `DOUBLE` for science. Never `FLOAT`/`DOUBLE` for currency — `0.1 + 0.2 != 0.3`.

```sql
CREATE TABLE payment (
  payment_id  INT AUTO_INCREMENT PRIMARY KEY,
  amount      DECIMAL(10,2) NOT NULL,    -- max 99,999,999.99
  rate        DOUBLE                      -- e.g., interest %
);
```

`UNSIGNED` doubles the positive range and is appropriate for IDs and counts that can never go negative. (MySQL 8.0.17 deprecated `UNSIGNED` for `FLOAT`/`DOUBLE`/`DECIMAL` — use a `CHECK` instead.)

### String

| Type      | Storage                              | Notes                              |
|-----------|--------------------------------------|------------------------------------|
| CHAR(N)   | fixed N chars (padded)               | Fast for fixed-width (e.g., country code) |
| VARCHAR(N)| 1-2 byte length + N chars max        | Default for variable text          |
| TINYTEXT  | up to 255 bytes                      | rarely used                        |
| TEXT      | up to 64 KB                          | stored off-page in InnoDB          |
| MEDIUMTEXT| up to 16 MB                          |                                    |
| LONGTEXT  | up to 4 GB                           |                                    |
| BLOB      | binary versions of above             |                                    |

**Rule:** `VARCHAR` for almost everything text. `TEXT` only when you genuinely need >64KB or know it'll be off-page. Use `CHAR` only for truly fixed-length codes (`CHAR(2)` for ISO country).

**Charset/collation matters.** Always:
```sql
CREATE TABLE article (
  ...
) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
```
- `utf8mb4` is real UTF-8 (4 bytes per code point — emoji, Asian scripts work).
- `utf8` (no `mb4`) is a MySQL legacy 3-byte version. **Never use plain `utf8`.**
- `_0900_ai_ci` = MySQL 8 Unicode 9 collation, accent-insensitive, case-insensitive.

### Date/time

| Type       | Bytes | Format/Range                                      |
|------------|-------|--------------------------------------------------|
| DATE       | 3     | 1000-01-01 .. 9999-12-31                          |
| TIME       | 3+    | -838:59:59 .. 838:59:59                          |
| DATETIME   | 5+    | 1000-01-01 00:00:00 .. 9999-12-31 23:59:59       |
| TIMESTAMP  | 4+    | 1970-01-01 UTC .. 2038-01-19 UTC, **converted to session TZ** |
| YEAR       | 1     | 1901..2155                                        |

```sql
CREATE TABLE event (
  event_id   INT AUTO_INCREMENT PRIMARY KEY,
  starts_at  DATETIME NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

`TIMESTAMP` auto-converts to the connection's `time_zone`. `DATETIME` does not — it's a literal wall-clock string. Choose `DATETIME` if you store user-facing local times; `TIMESTAMP` for "when did this row change."

You can add fractional seconds: `DATETIME(6)` = microseconds.

### JSON (MySQL 5.7+, much improved in 8)

```sql
CREATE TABLE event_log (
  id      INT AUTO_INCREMENT PRIMARY KEY,
  payload JSON NOT NULL
);

INSERT INTO event_log (payload) VALUES
  ('{"type":"login","user":42,"ip":"10.0.0.1"}');

-- Path expressions
SELECT payload->>'$.type', payload->'$.user'
FROM event_log
WHERE payload->>'$.type' = 'login';
```

`->` returns a JSON value, `->>` returns unquoted text. `JSON_EXTRACT()`, `JSON_SET()`, `JSON_ARRAYAGG()`, etc.

JSON is stored as a binary tree, not a text blob — fast to access by path, slow to fully rewrite.

### Other useful types
- **`ENUM('a','b','c')`** — stored as 1-2 byte integer. Fast, type-safe-ish, but adding values requires `ALTER`. Use sparingly.
- **`SET('a','b','c')`** — bitmap of multiple enum values. Almost never the right answer; use a junction table.
- **`BOOLEAN`** = synonym for `TINYINT(1)`. There is no real boolean.
- **Spatial** (`POINT`, `POLYGON`, `GEOMETRY`) — for GIS work.

---

## 3. Choosing — depth and tradeoffs

### Narrow > wide
Every byte multiplies. A 4-byte INT vs. an 8-byte BIGINT on a billion-row table = 4 GB. That fits in RAM vs. doesn't.

### Indexed columns: extra narrow
If you'll index it, every byte saves index size and lookup time. A `VARCHAR(255)` indexed column where 30 chars is the actual max is wasteful — and the index can only use a prefix anyway.

### NULL takes space
NULL columns get a 1-bit flag in the row header — basically free. But they break index optimizations (NULL is not orderable in the same way) and complicate query semantics. Default to `NOT NULL` unless you have a reason.

### Generated columns (MySQL 5.7+)
```sql
CREATE TABLE invoice (
  id        INT PRIMARY KEY,
  subtotal  DECIMAL(10,2),
  tax_rate  DECIMAL(4,3),
  total     DECIMAL(10,2) AS (subtotal * (1 + tax_rate)) STORED,
  INDEX (total)
);
```
`STORED` = computed on write, takes disk. `VIRTUAL` = computed on read, free disk but no index possible (well, partially). Great for indexing JSON paths or normalized search forms.

---

## 4. Practical application — type the Sakila `film` table from scratch

What types would you choose? Here's Sakila's actual definition (slightly trimmed):

```sql
CREATE TABLE film (
  film_id              SMALLINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  title                VARCHAR(128)      NOT NULL,
  description          TEXT,
  release_year         YEAR,
  language_id          TINYINT UNSIGNED  NOT NULL,
  original_language_id TINYINT UNSIGNED,
  rental_duration      TINYINT UNSIGNED  NOT NULL DEFAULT 3,
  rental_rate          DECIMAL(4,2)      NOT NULL DEFAULT 4.99,
  length               SMALLINT UNSIGNED,
  replacement_cost     DECIMAL(5,2)      NOT NULL DEFAULT 19.99,
  rating               ENUM('G','PG','PG-13','R','NC-17') DEFAULT 'G',
  special_features     SET('Trailers','Commentaries','Deleted Scenes','Behind the Scenes'),
  last_update          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

Walk through every choice:
- `SMALLINT UNSIGNED` for film_id — max 65,535 films. A DVD store doesn't need 4 billion film slots.
- `VARCHAR(128)` for title — generous but bounded.
- `TEXT` for description — long, off-page.
- `YEAR` for release_year — 1 byte vs. 3 for DATE; we don't need month/day.
- `TINYINT UNSIGNED` for language_id — max 255 languages, fine.
- `DECIMAL(4,2)` for rental_rate — exact, max 99.99.
- `ENUM` for rating — small, fixed, never changes. (Sakila got away with it; if MPAA invented "PG-12" you'd `ALTER`.)
- `TIMESTAMP ... ON UPDATE` — automatic last-modified tracking.

This is a tight, professional schema.

---

## 5. Common Mistakes & Gotchas

- **`VARCHAR(255)` for everything.** It's a lazy default. Pick the actual bound — and remember InnoDB indexes can't index more than ~3KB of key, so 255 utf8mb4 chars (1020 bytes) blows that quickly when composite.
- **`FLOAT`/`DOUBLE` for money.** Floating-point math is binary, money is decimal. You'll be off by a penny in production and someone will file a bug.
- **`TIMESTAMP` for historical dates.** It can't represent dates before 1970 or after 2038-01-19. Use `DATETIME`.
- **`utf8` instead of `utf8mb4`.** Save yourself the migration. It's `utf8mb4` in 2026.
- **`ENUM` you'll need to extend.** Adding a value is an `ALTER TABLE`, which on a huge table is painful. If the set will grow, use a lookup table.
- **`TEXT`/`BLOB` columns selected casually** in `SELECT *`. They're stored off-page; pulling them is extra IO. Only select them when needed.
- **`INT(11)` doesn't mean 11-digit limit.** That number is a *display width*, ignored by most clients, and fully removed in MySQL 8.0.17+. Just write `INT`.
- **Implicit type conversion in WHERE clauses.** `WHERE varchar_col = 42` will silently cast every row's column to a number to compare — and not use the index. Quote the literal: `WHERE varchar_col = '42'`.
- **vs. PostgreSQL:** Postgres has real BOOLEAN, native arrays, native UUID, true `TEXT` (no length penalty). MySQL doesn't. Don't assume parity.

---

## 🎯 Key Takeaways

- **Pick the narrowest type that fits forever.** Disk is cheap, RAM and indexes aren't.
- **Money is `DECIMAL`. Always.** Float-pointing currency is a career-defining bug source.
- **`utf8mb4` always.** Plain `utf8` in MySQL is a 3-byte abomination — known footgun since 2010.
- **`DATETIME` for wall-clock, `TIMESTAMP` for "when changed."** Know the timezone and 2038 implications.
- **Generated columns + JSON** are the modern combo for semi-structured data with queryable indexes.

*← [02 relational model](./02_relational_model.md) | [next → DDL & Constraints](./04_ddl_constraints.md)*
