# 09 — Database Migrations

> **Goal:** Treat the database schema as code: versioned, reviewed, applied automatically by Flyway or Liquibase, identical across every environment.

---

## 1. Migrations — mental model + working code

**The principle:** the **code repository is the source of truth** for the database schema. Each schema change is a checked-in file with a version number. The app runs the migration tool at startup; the schema converges on the declared state.

Without this:
- Dev DB drifts from staging drifts from prod.
- Hot-fix SQL run manually in prod isn't reproducible.
- Onboarding a new developer takes a day of "what tables do I need?"

With this:
- `git clone && docker compose up && ./mvnw spring-boot:run` produces a working schema.
- Every change is reviewable in a PR.
- Rollback is a forward fix (additive) or a reverse migration.

### The two contenders

| Tool         | Format       | Strengths                                            |
| ------------ | ------------ | ---------------------------------------------------- |
| **Flyway**   | Plain SQL    | Simple, SQL-native, low magic                        |
| **Liquibase**| XML/YAML/JSON/SQL | DB-agnostic abstractions, rollback, preconditions |

This module shows Flyway (more popular in modern Spring Boot stacks) and a Liquibase intro.

---

## 2. Flyway setup — what Spring Boot does

Add the dependency:

```xml
<dependency>
    <groupId>org.flywaydb</groupId>
    <artifactId>flyway-core</artifactId>
</dependency>
<!-- For Postgres 16+ (and other newer DBs) Boot 3.x requires the db-specific module -->
<dependency>
    <groupId>org.flywaydb</groupId>
    <artifactId>flyway-database-postgresql</artifactId>
</dependency>
```

Gradle:
```kotlin
implementation("org.flywaydb:flyway-core")
implementation("org.flywaydb:flyway-database-postgresql")
```

Spring Boot's `FlywayAutoConfiguration` then:

1. Detects Flyway on the classpath.
2. Looks for SQL files in `classpath:db/migration/`.
3. Runs them in version order **before** Hibernate validates the schema.
4. Records applied versions in a `flyway_schema_history` table.

### File naming convention

```
src/main/resources/db/migration/
├── V1__create_books_table.sql
├── V2__create_authors_table.sql
├── V3__add_isbn_to_books.sql
└── V4__seed_demo_data.sql
```

- `V<version>__<description>.sql` — versioned, immutable once applied.
- `R__<description>.sql` — **repeatable**, re-run on checksum change (great for views, functions).
- `U<version>__<description>.sql` — **undo** (Enterprise only).

### Example migrations

`V1__create_books_table.sql`:
```sql
CREATE TABLE books (
    id          BIGSERIAL PRIMARY KEY,
    title       VARCHAR(200) NOT NULL,
    author      VARCHAR(100) NOT NULL,
    price       NUMERIC(10, 2) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    version     BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_books_author ON books(author);
```

`V2__add_isbn_column.sql`:
```sql
ALTER TABLE books ADD COLUMN isbn VARCHAR(20);
CREATE UNIQUE INDEX idx_books_isbn ON books(isbn) WHERE isbn IS NOT NULL;
```

### Configure Flyway via properties

```yaml
spring:
  flyway:
    enabled: true
    locations: classpath:db/migration
    baseline-on-migrate: true     # if applying to an existing DB
    baseline-version: 0
    validate-on-migrate: true
    out-of-order: false           # set true ONLY if you know what you're doing
  jpa:
    hibernate:
      ddl-auto: validate          # MUST be validate now — Flyway owns schema
```

`baseline-on-migrate` is crucial if you're adopting Flyway on an existing prod DB.

---

## 3. Migration patterns — additive, careful, environment-aware

### The golden rule: migrations are immutable

Once a migration runs in any shared environment, **never edit it**. Edit means checksum changes; Flyway refuses to start. Always write a new `V<n+1>__...` file.

### Additive over destructive

Renames and drops are expensive. Prefer **expand-then-contract**:

1. `V10__add_new_column.sql` — add new column, keep old.
2. Code writes to both, reads from new (with fallback).
3. Backfill data.
4. `V11__drop_old_column.sql` — remove old once nothing reads it.

This pattern is required for zero-downtime deploys.

### Environment-specific migrations

You can scope migrations by profile:

```yaml
spring:
  flyway:
    locations: classpath:db/migration,classpath:db/migration-${spring.profiles.active}
```

Then:
```
db/migration/V1__schema.sql                  ← runs everywhere
db/migration-dev/V2__seed_demo_data.sql      ← runs only in dev
```

Use sparingly. Most schema should be identical across environments — diverging adds another way for "works on staging" bugs to appear.

### Repeatable migrations for views/functions

`R__book_search_view.sql`:
```sql
CREATE OR REPLACE VIEW v_book_search AS
SELECT id, title, author, lower(title) || ' ' || lower(author) AS searchable
FROM books;
```

Edit the file, redeploy, Flyway notices the checksum changed and re-applies.

---

## 4. Practical application — bookstore migration sequence

### Migration 1 — initial schema

`V1__init.sql`:
```sql
CREATE TABLE authors (
    id    BIGSERIAL PRIMARY KEY,
    name  VARCHAR(100) NOT NULL UNIQUE
);

CREATE TABLE books (
    id          BIGSERIAL PRIMARY KEY,
    title       VARCHAR(200) NOT NULL,
    author_id   BIGINT NOT NULL REFERENCES authors(id),
    price       NUMERIC(10, 2) NOT NULL CHECK (price >= 0),
    stock       INT NOT NULL DEFAULT 0 CHECK (stock >= 0),
    isbn        VARCHAR(20) UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    version     BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_books_author ON books(author_id);
CREATE INDEX idx_books_title_lower ON books(lower(title));
```

### Migration 2 — orders

`V2__orders.sql`:
```sql
CREATE TABLE orders (
    id          BIGSERIAL PRIMARY KEY,
    book_id     BIGINT NOT NULL REFERENCES books(id),
    quantity    INT NOT NULL CHECK (quantity > 0),
    total       NUMERIC(12, 2) NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Migration 3 — soft delete (additive)

`V3__books_soft_delete.sql`:
```sql
ALTER TABLE books ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE INDEX idx_books_active ON books(id) WHERE deleted_at IS NULL;
```

The matching entity change:

```java
@Entity
@SQLDelete(sql = "UPDATE books SET deleted_at = NOW() WHERE id = ?")
@Where(clause = "deleted_at IS NULL")
public class Book {
    @Id @GeneratedValue Long id;
    private Instant deletedAt;
    // ...
}
```

### Migration 4 — seed data (dev only)

`db/migration-dev/V4__seed.sql`:
```sql
INSERT INTO authors (name) VALUES ('Joshua Bloch'), ('Robert C. Martin');
INSERT INTO books (title, author_id, price, stock, isbn) VALUES
    ('Effective Java', 1, 49.99, 10, '978-0134685991'),
    ('Clean Code', 2, 39.99, 5, '978-0132350884');
```

### Running migrations standalone (CI/CD)

The Flyway Maven plugin lets you run migrations without starting the app:

```xml
<plugin>
    <groupId>org.flywaydb</groupId>
    <artifactId>flyway-maven-plugin</artifactId>
    <version>10.11.0</version>
    <configuration>
        <url>${env.DATABASE_URL}</url>
        <user>${env.DATABASE_USER}</user>
        <password>${env.DATABASE_PASSWORD}</password>
    </configuration>
</plugin>
```

```bash
./mvnw flyway:migrate
./mvnw flyway:info
./mvnw flyway:validate
```

This is how you migrate **before** rolling out app instances (recommended in production).

### Liquibase alternative — XML-driven

```xml
<dependency>
    <groupId>org.liquibase</groupId>
    <artifactId>liquibase-core</artifactId>
</dependency>
```

`db/changelog/db.changelog-master.xml`:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog">
    <changeSet id="1" author="yati">
        <createTable tableName="books">
            <column name="id" type="BIGINT" autoIncrement="true">
                <constraints primaryKey="true"/>
            </column>
            <column name="title" type="VARCHAR(200)">
                <constraints nullable="false"/>
            </column>
        </createTable>
    </changeSet>
</databaseChangeLog>
```

Liquibase shines for multi-DB shops (one changeset, many dialects) and where you want rollbacks defined inline. Most Spring Boot shops still pick Flyway for simplicity.

---

## 5. Common Mistakes & Gotchas

- **Letting Hibernate's `ddl-auto: update` co-exist with Flyway.** Now you have two tools fighting over the schema. Set `ddl-auto: validate` and let Flyway own it.

- **Editing an already-applied migration.** Checksum mismatch → Flyway refuses to start. Always write a new versioned file.

- **`out-of-order: true` as a "fix."** Hides ordering problems. Use it only if you genuinely understand merge-conflict implications.

- **Running migrations as part of app startup in containerized prod.** Three replicas start simultaneously → race for the migration. Flyway uses a metadata-table lock, but you can still hit edge cases. **Best practice: run migrations as a separate job before rollout** (Kubernetes init container or pipeline step).

- **Forgetting to commit `flyway_schema_history` permissions.** The DB user needs DDL rights. Many shops use a separate "migration" user with elevated rights and a "runtime" user without.

- **Seeding production data via migrations.** Test fixtures don't belong in production. Keep `db/migration-{profile}` separation strict.

- **Big single-file migration on a hot table.** `ALTER TABLE huge_table ADD COLUMN ...` can lock the table. Use online schema-change tools (pt-online-schema-change for MySQL, native ALTER with care in Postgres) or break into smaller steps.

- **No backups before migration.** Always. Especially the first time you run any new tool against prod.

- **Schema baseline confusion.** Adopting Flyway on existing DB? Set `baseline-on-migrate: true` and `baseline-version` to a number above any future migration you'll write.

---

## 🎯 Key Takeaways

- **Schema is code.** Versioned, reviewed, deployed like any other artifact.
- **Flyway for simplicity, Liquibase for cross-DB or rollback needs.** Pick one, stick with it.
- **Migrations are immutable** once applied. New file, not edits.
- **Expand-then-contract** is the pattern for any rename/drop in a live system.
- **Run migrations before app rollout** in production — separate job, separate credentials.

*[← prev](./08_transactions.md) | [next →](./10_security.md)*
