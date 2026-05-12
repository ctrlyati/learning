# 17 — Production Operations

> **Goal:** Run MySQL like an SRE — connection pools, slow query logs, performance schema, online schema migrations, and the operational patterns that keep production healthy.

---

## 1. Operations — analogy + runnable SQL

If everything to here was learning to fly the plane, this module is the maintenance crew, the air traffic control, and the pre-flight checklists. A "good query" doesn't matter if the connection pool is exhausted, the disk is full, or the migration locks the table for two hours.

```sql
-- Three queries every production engineer runs every day
SHOW PROCESSLIST;                              -- what's running right now
SELECT * FROM sys.session;                     -- richer view via sys schema
SHOW ENGINE INNODB STATUS\G                    -- locks, deadlocks, buffer pool
```

---

## 2. Connection pooling — mechanism

A TCP+auth handshake costs milliseconds. Multiplied by every web request, that's a meaningful chunk of latency *and* it can exhaust the server's `max_connections`.

Solution: a **connection pool** between app and DB that keeps a warm set of connections, hands them out per request, and reclaims them.

Two flavors:
- **In-process pool** (HikariCP, SQLAlchemy QueuePool, pgbouncer-equivalent in app) — one pool per process, fine for monoliths and small fleets.
- **External pooler** — **ProxySQL** is the canonical MySQL one. Sits between app and DB; multiplexes many app connections over fewer DB connections. Also offers query routing, caching, failover.

Sizing rules of thumb:
- `max_connections` on the server should be > total of all pools' max sizes + admin headroom.
- Each connection costs ~256KB-1MB of RAM on the server. 10,000 idle connections is real cost.
- Pool size per app instance is usually 5-20. More just queues at the server instead of in your app.
- **Wait time + active time** = total request latency for a query. If pool is undersized, wait dominates.

Inspect:
```sql
SHOW STATUS LIKE 'Threads_connected';   -- currently open
SHOW STATUS LIKE 'Max_used_connections'; -- peak since start
SHOW VARIABLES LIKE 'max_connections';
```

---

## 3. Observability — slow log, performance_schema, sys — depth

### Slow query log

```sql
SET GLOBAL slow_query_log = ON;
SET GLOBAL slow_query_log_file = '/var/log/mysql/slow.log';
SET GLOBAL long_query_time = 0.5;     -- log anything > 500ms
SET GLOBAL log_queries_not_using_indexes = ON;  -- aggressive
```

Then aggregate with **pt-query-digest** (Percona Toolkit):
```bash
pt-query-digest /var/log/mysql/slow.log
```
Output: a ranked list of "the queries costing you the most cumulative time." This is the single best place to start when "the DB is slow."

### performance_schema

The introspection database. Hundreds of tables; the highlights:
```sql
-- Top 10 statements by total latency
SELECT digest_text, count_star, avg_timer_wait/1e9 AS avg_ms,
       sum_timer_wait/1e9 AS total_ms
FROM performance_schema.events_statements_summary_by_digest
ORDER BY sum_timer_wait DESC LIMIT 10;

-- Currently running statements
SELECT * FROM performance_schema.events_statements_current;

-- Table I/O activity
SELECT object_schema, object_name, count_read, count_write
FROM performance_schema.table_io_waits_summary_by_table
ORDER BY count_read + count_write DESC LIMIT 20;
```

### sys schema

A friendlier wrapper over performance_schema:
```sql
SELECT * FROM sys.statements_with_runtimes_in_95th_percentile LIMIT 10;
SELECT * FROM sys.schema_unused_indexes;
SELECT * FROM sys.schema_index_statistics;
SELECT * FROM sys.host_summary;
SELECT * FROM sys.io_global_by_file_by_bytes ORDER BY total DESC LIMIT 10;
```

`sys.schema_unused_indexes` is a goldmine — finds indexes nothing has used since the last server restart, candidates for removal.

### InnoDB monitoring

```sql
SHOW ENGINE INNODB STATUS\G
```
Key sections:
- **BUFFER POOL AND MEMORY** — hit rate; should be >99% for hot data.
- **TRANSACTIONS** — long-running transactions, blocking.
- **LATEST DETECTED DEADLOCK** — last deadlock anatomy.
- **ROW OPERATIONS** — read/insert/update/delete rates.

### Key metrics to alert on

- `innodb_buffer_pool_wait_free` rising — buffer pool too small.
- `Threads_running` spike — query storm.
- `Innodb_row_lock_time_avg` rising — contention.
- Replication lag (`Seconds_Behind_Source`) > some threshold.
- Disk usage of datadir, binlog dir.
- Slow query count per minute.
- `aborted_clients`, `aborted_connects`.

---

## 4. Practical application — schema migrations and operational patterns

### Schema migrations on big tables

A naive `ALTER TABLE ADD COLUMN` on a 500GB table can lock writes for hours (older MySQL) or take hours of online rebuild (modern MySQL). Either is unacceptable. Use:

**gh-ost (GitHub's online schema change):**
- Creates shadow table with desired schema.
- Replicates writes via binlog tail.
- Backfills existing rows in batches.
- Atomic table swap at the end.
- Throttles based on replica lag.

```bash
gh-ost \
  --user="root" --password="..." \
  --host=primary.host \
  --database=sakila --table=film \
  --alter="ADD COLUMN slug VARCHAR(200)" \
  --execute
```

**pt-online-schema-change (Percona):** trigger-based equivalent. Older, more battle-tested for tiny edge cases; uses triggers (some risk under heavy write load).

**Native online DDL (MySQL 8):** for many ALTERs, MySQL 8 supports `ALGORITHM=INSTANT` (metadata change only) or `ALGORITHM=INPLACE` with `LOCK=NONE`. Always specify both — fail loudly if not supported, rather than silently degrading to a copy.

### Migration discipline

- **Backwards-compatible** schemas. Deploy DDL first, code second. Two phases:
  1. Add new column nullable, deploy code that writes both old and new.
  2. Backfill, switch reads, then drop old column in a later release.
- **Never** rename in one step (column or table). Add new, dual-write, switch, drop. The renamed-mid-deploy bug is brutal.
- **Migrations in version control** (Flyway, Liquibase, dbmate, atlas, gh-migrate, sqitch).
- **Forward-only** — no `down` migrations in production. Roll forward with a compensating migration.

### Capacity planning checklist

- Buffer pool ≥ working set (active data + indexes).
- Disk: 2-3x current size for headroom + binlog retention.
- IOPS: enough for peak writes + replication catch-up.
- Connection limit ≥ pool sum × 1.5.
- Read replicas for analytics so OLTP isn't impacted.

### Common operational patterns

- **Read replicas for reporting.** Route heavy analytic queries off the primary.
- **Read-after-write consistency** — replicas lag. Either route critical reads to primary or use bounded staleness.
- **Sharding** — horizontal split when one server can't hold the data. ProxySQL, Vitess.
- **Partitioning** (MySQL native) — split a table by range/list/hash within one server. Useful for time-series purges (drop old partition, instant).
- **Archival** — move cold data to a separate "archive" table or external store; keep hot table small.

```sql
-- Range partitioning by date
CREATE TABLE event_log (
  id BIGINT, ts DATETIME, payload JSON,
  PRIMARY KEY (id, ts)
) PARTITION BY RANGE (TO_DAYS(ts)) (
  PARTITION p2024 VALUES LESS THAN (TO_DAYS('2025-01-01')),
  PARTITION p2025 VALUES LESS THAN (TO_DAYS('2026-01-01')),
  PARTITION p2026 VALUES LESS THAN (TO_DAYS('2027-01-01')),
  PARTITION pmax  VALUES LESS THAN MAXVALUE
);

-- Drop a year of data instantly
ALTER TABLE event_log DROP PARTITION p2024;
```

### A "first-day-on-call" cheat sheet

```sql
-- What's running, who's blocking
SHOW PROCESSLIST;
SELECT * FROM sys.session WHERE conn_id != CONNECTION_ID();
SELECT * FROM sys.innodb_lock_waits;

-- What's slow lately
SELECT digest_text, count_star, avg_timer_wait/1e9 AS avg_ms
FROM performance_schema.events_statements_summary_by_digest
ORDER BY sum_timer_wait DESC LIMIT 10;

-- Replication
SHOW REPLICA STATUS\G
-- Seconds_Behind_Source, Last_SQL_Error, Last_IO_Error

-- Buffer pool health
SELECT (1 - (variable_value/(SELECT variable_value FROM performance_schema.global_status WHERE variable_name='Innodb_buffer_pool_read_requests'))) AS hit_rate
FROM performance_schema.global_status WHERE variable_name='Innodb_buffer_pool_reads';

-- Disk
SHOW VARIABLES LIKE 'datadir';
-- Then `df -h` on the OS

-- Recent errors
-- tail -f /var/log/mysql/error.log
```

---

## 5. Common Mistakes & Gotchas

- **No connection pool.** Every request opens a new connection. Server CPU spent on TLS handshakes.
- **Pool too large.** Doesn't increase throughput once DB is the bottleneck; just adds queue depth and lock contention. Start small, increase only with evidence.
- **No slow query log enabled.** You're flying blind.
- **Long-running transactions in idle connections.** A connection in a transaction holds locks and prevents purge. `SET GLOBAL innodb_rollback_on_timeout = ON;` and a sane `wait_timeout`.
- **`SELECT * FROM big_table` from a developer's GUI** locking up the buffer pool.
- **Online migrations without throttling.** gh-ost / pt-osc default settings are sane; tune if running on weaker hardware.
- **Down-migrations in production.** They lie about safety. Roll forward.
- **Forgetting to monitor binlog disk.** Binlogs accumulate; old ones must be purged after replicas have consumed them.
- **Tuning `innodb_buffer_pool_size` to 100% of RAM.** Leave headroom for OS, replication threads, etc. 70-80% is the rule of thumb.
- **Replicas drifting silently.** Alert on `Seconds_Behind_Source` and on stopped replication.
- **Single point of failure.** No replica = no high availability. Aurora/RDS multi-AZ, or self-managed primary + standby.
- **Not using `sys.schema_unused_indexes`.** Months of bloat from indexes nothing reads.
- **vs. Postgres:** Postgres needs `VACUUM` (autovacuum) to clean dead tuples — there's no MySQL equivalent because InnoDB's purge thread runs continuously. Different gotcha; same general "background maintenance matters" lesson.

---

## 🎯 Key Takeaways

- **Connection pool sizing is throughput's silent ceiling.** Start small (10-20/instance), measure, grow with evidence.
- **Slow query log + pt-query-digest** is the highest-ROI tool for "make MySQL fast."
- **`sys` schema views are your friends.** Especially `schema_unused_indexes` and `statements_with_runtimes_in_95th_percentile`.
- **gh-ost / pt-osc for big-table migrations.** Native online DDL is great when applicable; specify ALGORITHM/LOCK explicitly.
- **Backwards-compatible, forward-only migrations.** Two-phase column changes; never rename in one step.

---

## Course wrap

You started not knowing the difference between a server and a client. You can now design a normalized schema, type each column deliberately, write subqueries, CTEs, and window functions, read EXPLAIN plans, design composite covering indexes, reason about MVCC and isolation levels, secure a production server, run online schema migrations, and diagnose a slow database from `sys.session`.

That's the skill set of a senior backend engineer who *owns* the database, instead of being intimidated by it. Most of your peers don't have it. Use that.

Next steps:
- Read **High Performance MySQL** (4th ed.) cover to cover.
- Read **Use The Index, Luke** (free, online).
- Pick a project, instrument it, and tune one slow query a week.
- Subscribe to the Planetscale and Percona blogs.

*← [16 security](./16_security.md) | [back to roadmap](./00_roadmap.md)*
