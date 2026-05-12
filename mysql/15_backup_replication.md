# 15 — Backup, Restore, Replication, and the Binary Log

> **Goal:** Never lose data. Know how to back up MySQL three ways, how to restore reliably, and how replication and the binlog work end-to-end.

---

## 1. Backup — analogy + runnable shell

A backup is a parachute. You don't need it 99.9% of the time, and the 0.1% you do, untested parachutes kill people. **An untested backup is not a backup.**

```bash
# Logical backup — dumps SQL statements
mysqldump -u root -p \
  --single-transaction \
  --routines --events --triggers \
  --databases sakila \
  > sakila.sql

# Restore
mysql -u root -p < sakila.sql
```

That's the simplest workflow. Now the depth.

---

## 2. The three backup strategies — mechanism

### A. Logical backup — `mysqldump` / `mysqlpump`
- Output: SQL text file (CREATE TABLE, INSERT statements).
- **Pros:** human-readable, portable across MySQL versions, can dump subsets, restores anywhere.
- **Cons:** slow for large DBs (rebuilds indexes on restore), point-in-time-recovery awkward without binlog.

Important flags:
```bash
mysqldump \
  --single-transaction \   # consistent snapshot via REPEATABLE READ; needs InnoDB
  --routines --events --triggers \  # don't forget these!
  --master-data=2 \        # writes binlog coordinates as comment (for replication)
  --skip-lock-tables \     # only safe with --single-transaction
  --quick \                # row-at-a-time, low memory
  --hex-blob \             # binary-safe
  --set-gtid-purged=OFF \  # GTID handling for cross-server restore
  sakila > sakila.sql
```

### B. Physical backup — `Percona XtraBackup`
- Copies InnoDB datafiles and applies redo log to make them consistent.
- **Pros:** fast (no SQL parsing/rebuilding), suitable for terabyte-scale.
- **Cons:** binary, version-tied, more setup.

```bash
xtrabackup --backup --target-dir=/backups/full --user=root --password=...
xtrabackup --prepare --target-dir=/backups/full
# To restore: stop MySQL, copy files into datadir, start MySQL.
```

### C. Snapshot — filesystem / cloud
- LVM snapshot, EBS snapshot, ZFS snapshot.
- **Pros:** instant, atomic at the FS level.
- **Cons:** must `FLUSH TABLES WITH READ LOCK` (or use XtraBackup) for InnoDB consistency. RDS/Aurora handle this automatically.

### Point-in-time recovery (PITR)

Every backup is a "moment." To get back to a *specific* second after the backup, you replay the **binary log** on top.

```bash
# Restore last full backup
mysql < sakila_2026_05_10.sql

# Replay binlog from coordinate after backup, to disaster moment
mysqlbinlog --start-position=1234 --stop-datetime='2026-05-11 09:42:00' \
  /var/lib/mysql/binlog.000042 | mysql
```

This is your "oh no the prod DB caught fire at 09:42, restore to 09:41:59" workflow. Test it before you need it.

---

## 3. The binary log — depth

The binlog is a sequential log of every change-statement, used for:
- **Replication** — replicas tail the binlog.
- **Point-in-time recovery** — replay between two coordinates.
- **CDC (change data capture)** — debezium, maxwell tail it for downstream pipelines.

Three formats:
- **STATEMENT** — logs the SQL text. Compact, but non-deterministic statements (NOW(), UUID(), AUTO_INCREMENT in some cases) replay differently. Avoid.
- **ROW** — logs the actual before/after row images. Verbose but always deterministic. **Default in MySQL 8 and the right answer.**
- **MIXED** — STATEMENT, falls back to ROW for non-deterministic.

```sql
SHOW VARIABLES LIKE 'binlog_format';
SHOW BINARY LOGS;
SHOW MASTER STATUS;
SHOW BINLOG EVENTS IN 'binlog.000001' LIMIT 10;
```

### Replication architecture

Async master-replica is the default:

```
┌──────────┐   binlog   ┌──────────┐
│ primary  │───────────▶│ replica  │
│ (writes) │            │ (reads)  │
└──────────┘            └──────────┘
```

On the primary, every committed transaction goes into the binlog. On the replica, two threads work:
1. **IO thread** — connects to primary, pulls binlog events, writes them to the replica's **relay log**.
2. **SQL thread** (or worker threads, parallel replication) — applies relay log events.

Setup outline:
```sql
-- On primary (my.cnf)
[mysqld]
server_id = 1
log_bin = /var/log/mysql/binlog
binlog_format = ROW
gtid_mode = ON
enforce_gtid_consistency = ON

-- On replica
[mysqld]
server_id = 2
relay_log = /var/log/mysql/relay
gtid_mode = ON
enforce_gtid_consistency = ON
read_only = ON

-- On replica (after creating user on primary)
CHANGE REPLICATION SOURCE TO
  SOURCE_HOST='primary.host',
  SOURCE_USER='repl',
  SOURCE_PASSWORD='...',
  SOURCE_AUTO_POSITION = 1;   -- GTID

START REPLICA;
SHOW REPLICA STATUS\G
```

(Pre-8.0.22 syntax: `MASTER` instead of `SOURCE`, `SLAVE` instead of `REPLICA`. The new terms are inclusive — both work in 8.0.22+.)

### GTIDs (Global Transaction Identifiers)

Each transaction gets a globally unique ID like `<server_uuid>:<sequence>`. Replication then operates on "apply all transactions with GTIDs not yet seen" rather than file/position. Survives failover, makes setup simpler. **Use GTID; don't use file-based replication in 2026.**

### Replication topologies
- **Async (default)** — primary doesn't wait. Lowest latency, possible data loss on primary failure.
- **Semi-sync** — primary waits for at least one replica to ack. Some durability, small latency cost.
- **Group Replication** — multi-primary with consensus (similar to Galera). Stronger consistency.
- **InnoDB Cluster** — Group Replication + MySQL Router + Shell, packaged.

### Replication lag

The replica is always behind. Watch:
```sql
SHOW REPLICA STATUS\G
-- Look at: Seconds_Behind_Source
```

Causes: long transactions on primary, single-threaded SQL applier on replica (use parallel applier flags), big writes, network. Lag is the biggest operational issue with async replication.

---

## 4. Practical application — backup script and restore drill

```bash
#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR=/backups/mysql
DATE=$(date +%Y_%m_%d)
mkdir -p "$BACKUP_DIR/$DATE"

mysqldump \
  --single-transaction \
  --routines --events --triggers \
  --master-data=2 \
  --all-databases \
  | gzip > "$BACKUP_DIR/$DATE/full.sql.gz"

# Keep last 7 days
find $BACKUP_DIR -mindepth 1 -maxdepth 1 -mtime +7 -exec rm -rf {} +

# Verify backup is restorable (this is the part most teams skip!)
TEMP_DIR=$(mktemp -d)
docker run --rm -d --name verify -e MYSQL_ROOT_PASSWORD=x mysql:8.0
sleep 30
gunzip -c "$BACKUP_DIR/$DATE/full.sql.gz" | docker exec -i verify mysql -u root -px
docker exec verify mysql -u root -px -e "SELECT COUNT(*) FROM sakila.film;"
docker rm -f verify
```

### Disaster recovery drill (do this quarterly)
1. Pick a random recent backup.
2. Restore to a fresh server.
3. Replay binlogs to a chosen "incident time."
4. Run application smoke tests.
5. Document time-to-restore. That's your **RTO**.

---

## 5. Common Mistakes & Gotchas

- **Untested backups.** The classic. Every team has a story of a backup that didn't restore.
- **Forgetting `--routines --events --triggers`.** Default `mysqldump` skips these. Procedures vanish.
- **`mysqldump` without `--single-transaction` on InnoDB.** You'll get an inconsistent snapshot.
- **Backing up to the same disk.** Disk fails → backup fails too. Off-machine + off-region.
- **Statement-based binlog with non-deterministic SQL.** Replicas drift. Use ROW.
- **Not monitoring replication lag.** Quietly hours behind, until the day you fail over and lose data.
- **Promoting a lagging replica blindly.** You lose every transaction the replica hadn't applied.
- **Stopping replication "for a quick maintenance" and forgetting.** Lag grows unboundedly; relay log fills disk.
- **DDL on a replica.** Read-only enforced via `read_only`/`super_read_only`, but a privileged user can break things.
- **Restoring with FK checks on.** A multi-table dump can fail mid-restore. `mysqldump` writes `SET FOREIGN_KEY_CHECKS=0;` at the top — keep it.
- **vs. Postgres:** Postgres uses logical decoding + WAL shipping. MySQL's binlog and Postgres's WAL serve similar roles but aren't interchangeable. CDC tools (Debezium) abstract over both.

---

## 🎯 Key Takeaways

- **Untested backup ≠ backup.** Run a quarterly restore drill or you have nothing.
- **mysqldump + binlog = PITR.** Full backup nightly, binlog continuously archived.
- **ROW format binlog + GTID** is the modern default. Don't deviate without a reason.
- **Replication is async by default — replicas lag.** Measure it, alert on it, plan for data loss on unplanned failover.
- **XtraBackup for terabyte-scale.** mysqldump runs out of steam by ~50–100 GB.

*← [14 procedures](./14_procedures_triggers.md) | [next → Security](./16_security.md)*
