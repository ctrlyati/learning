# 00 — MySQL Deep-Dive Roadmap

> **Goal:** Take a working backend developer from "I can write SELECT *" to "I can design schemas, tune queries, and operate MySQL 8 in production."

This course is built for a developer doing professional upskilling. It assumes you know your way around a terminal and have written code that talks to a database, but treats SQL itself as a first-class craft worth mastering.

---

## Module Table

| #  | File | Topic | Why it matters |
|----|------|-------|----------------|
| 01 | [setup_and_client](./01_setup_and_client.md) | MySQL 8 install, CLI, Workbench, sample DB | You can't learn what you can't run. |
| 02 | [relational_model](./02_relational_model.md) | Tables, keys, normalization | Schema is destiny. |
| 03 | [data_types](./03_data_types.md) | Numeric, string, date, JSON | Wrong type = wasted disk + slow queries. |
| 04 | [ddl_constraints](./04_ddl_constraints.md) | CREATE/ALTER/DROP, constraints, generated cols | The contract your data must obey. |
| 05 | [dml_basics](./05_dml_basics.md) | SELECT, WHERE, ORDER BY, LIMIT | The 80% you'll write daily. |
| 06 | [filtering_deeper](./06_filtering_deeper.md) | LIKE, IN, BETWEEN, NULL | NULL is not a value, and that bites. |
| 07 | [joins](./07_joins.md) | INNER, LEFT, RIGHT, CROSS, SELF | Where most bugs live. |
| 08 | [aggregations](./08_aggregations.md) | GROUP BY, HAVING, ROLLUP | Reporting and analytics. |
| 09 | [subqueries_ctes](./09_subqueries_ctes.md) | Subqueries, CTEs, recursive CTEs | Compose queries cleanly. |
| 10 | [window_functions](./10_window_functions.md) | ROW_NUMBER, RANK, LAG/LEAD | The tool that replaces 100 lines of app code. |
| 11 | [indexes](./11_indexes.md) | B-tree, composite, covering | The single biggest perf lever. |
| 12 | [query_optimization](./12_query_optimization.md) | EXPLAIN, ANALYZE, hints | Turn 10s queries into 10ms. |
| 13 | [transactions_isolation](./13_transactions_isolation.md) | ACID, locking, MVCC | Correctness under concurrency. |
| 14 | [procedures_triggers](./14_procedures_triggers.md) | Stored procs, functions, triggers, events | When to use them — and when not. |
| 15 | [backup_replication](./15_backup_replication.md) | mysqldump, binlog, replication | Don't lose data. |
| 16 | [security](./16_security.md) | Users, roles, injection, encryption | The boring stuff that ends careers if ignored. |
| 17 | [production_ops](./17_production_ops.md) | Pooling, slow log, perf schema, migrations | What ops actually looks like. |

---

## Timeline (~3 weeks at 1 module/day)

| Week | Modules | Theme |
|------|---------|-------|
| 1 | 01–06 | Foundations: install, model, types, DDL, basic DML |
| 2 | 07–12 | Querying mastery: joins, aggregates, CTEs, windows, indexes, EXPLAIN |
| 3 | 13–17 | Production: transactions, procedures, backup, security, ops |

If you're cramming for an interview, do 11, 12, 13 first — they show up the most.

---

## Prerequisites

- A terminal you're comfortable with (cmd, PowerShell, bash, zsh — any).
- A laptop where you can install MySQL 8 (Docker is fine — see Module 01).
- Comfort reading code in any C-family language (we never assume one).
- Optional but useful: prior exposure to *some* database (SQLite, Postgres, even Excel pivot tables).

---

## Mental Models You'll Build

These are the lenses that make everything else click. Re-read this list at the end of the course.

1. **Relational algebra is the substrate.** SQL is just a syntax over set operations: selection (σ), projection (π), join (⋈), union (∪). When a query feels confusing, ask "what sets am I combining?"
2. **Indexes are just sorted lookups.** A B-tree index is a sorted phone book. Every query optimization conversation reduces to: "can the planner do a sorted lookup, or must it scan?"
3. **The optimizer is a cost estimator, not an oracle.** It guesses, based on statistics, which plan is cheapest. When it guesses wrong, you'll learn why with EXPLAIN.
4. **Transactions are isolation guarantees.** ACID is not magic — it's a set of promises about what concurrent transactions can and cannot see. Choosing an isolation level is choosing which anomalies you tolerate.
5. **The database is shared mutable state.** Every concurrency bug you've ever had in code, the database has at scale. Locks, deadlocks, and race conditions are the norm.
6. **Schema is harder to change than code.** A bad column type costs a migration; a bad normalization decision costs a rewrite. Spend the time upfront.

---

## Reference Links

- [MySQL 8.0 Reference Manual](https://dev.mysql.com/doc/refman/8.0/en/) — the canonical source. Bookmark it.
- [Use The Index, Luke](https://use-the-index-luke.com/) — Markus Winand's free book on indexes. Best resource on the planet for this topic.
- [High Performance MySQL, 4th Edition](https://www.oreilly.com/library/view/high-performance-mysql/9781492080503/) (O'Reilly) — the production-MySQL bible.
- [Planetscale Blog](https://planetscale.com/blog) — modern MySQL operations, written by people running it at scale.
- [Modern SQL](https://modern-sql.com/) — Markus Winand again, on SQL features beyond SELECT *.
- [Percona Blog](https://www.percona.com/blog/) — deep MySQL internals, war stories, perf tuning.

---

## Sample Schema

We'll use the **Sakila** sample database (DVD rental store) throughout. It's the canonical MySQL teaching DB: customers, films, rentals, payments, staff, stores. You'll install it in Module 01. Where Sakila is awkward (e.g., recursive trees), we'll define a small auxiliary schema inline.

---

## Closing

By the end of this course you should be the person on your team who other engineers ping when their query is slow, when a migration scares them, or when production behaves weirdly under load. That is a high-leverage skill — backend systems live or die by their database, and most developers never invest the time. You will.

Let's begin. → [01 — Setup and Client](./01_setup_and_client.md)
