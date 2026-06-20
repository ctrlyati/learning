# 06 — RDS and Aurora: Managed Relational Databases

> **Goal:** Choose between RDS engines and Aurora variants confidently, design for HA and read scale, and run a relational database in production without 3 AM pages.

---

## 1. RDS — managed relational DB-as-a-service

**Mental model:** RDS is "AWS runs your database server for you." You still pick the engine, configure parameters, design schemas, write queries. AWS handles the OS, patching, backups, failover, and replication mechanics.

### Engines supported
- **PostgreSQL** — modern, feature-rich, the default choice for new apps.
- **MySQL** — battle-tested, huge ecosystem.
- **MariaDB** — MySQL fork.
- **Oracle** — commercial, BYOL or License Included.
- **SQL Server** — commercial, multiple editions.
- **Db2** — IBM (added 2023).

Plus **Aurora** (separately) — AWS-built engines compatible with MySQL and PostgreSQL, with very different architecture.

### Launch one
```bash
aws rds create-db-instance \
  --db-instance-identifier demo-pg \
  --engine postgres --engine-version 16.3 \
  --db-instance-class db.t4g.micro \
  --allocated-storage 20 --storage-type gp3 \
  --master-username dbadmin --master-user-password "$(openssl rand -base64 18)" \
  --vpc-security-group-ids $DB_SG \
  --db-subnet-group-name my-db-subnets \
  --backup-retention-period 7 \
  --multi-az \
  --storage-encrypted \
  --enable-iam-database-authentication \
  --enable-performance-insights \
  --deletion-protection
```

Critical flags:
- `--multi-az` — synchronous standby in another AZ. Doubles compute cost, gives HA.
- `--storage-encrypted` — KMS-encrypted EBS volumes. Cannot be enabled after creation (must snapshot + restore).
- `--enable-iam-database-authentication` — log in with IAM tokens instead of passwords.
- `--deletion-protection` — prevents `delete-db-instance` until disabled. Always set in prod.

### Subnet group
RDS needs a DB Subnet Group of **at least 2 subnets in different AZs** (for Multi-AZ). Should be private subnets — your DB never gets a public IP.

```bash
aws rds create-db-subnet-group --db-subnet-group-name my-db-subnets \
  --db-subnet-group-description "private DB subnets" \
  --subnet-ids subnet-data-a subnet-data-b
```

---

## 2. High Availability: Multi-AZ

**Multi-AZ deployment**: a synchronous standby replica in another AZ. Writes go to primary, replicate synchronously to standby. On failure (hardware, AZ outage, patching), AWS fails over by changing the DNS endpoint to point to the standby — typical RTO 60–120s, **RPO = 0** (no data loss).

The standby **doesn't serve reads.** Its sole purpose is HA.

There's a newer flavor — **Multi-AZ Cluster Deployments** (PostgreSQL, MySQL only) — that gives 2 readable standbys with semi-synchronous replication and faster failover (~35s). Worth it for read-heavy workloads that also need HA.

### Read Replicas — for scaling reads, not HA
Asynchronous replicas (can be in same region, cross-region, or even cross-account). Can be promoted to standalone DBs. Used for:
- Offloading read traffic.
- Running heavy analytics queries.
- Disaster recovery (cross-region replica).
- Zero-downtime engine upgrade staging.

```bash
aws rds create-db-instance-read-replica \
  --db-instance-identifier demo-pg-ro-1 \
  --source-db-instance-identifier demo-pg
```

Replicas can lag — minutes in pathological cases. Apps reading from them must tolerate this.

---

## 3. Backups, Snapshots, and PITR

**Automated backups**: enabled by default with 1–35 day retention. RDS takes a daily snapshot + ships transaction logs every 5 minutes to S3. **Point-in-time recovery** lets you restore to any second within retention.

```bash
aws rds restore-db-instance-to-point-in-time \
  --source-db-instance-identifier demo-pg \
  --target-db-instance-identifier demo-pg-restored \
  --restore-time 2026-05-11T03:00:00Z
```

**Manual snapshots**: indefinite retention (until deleted). Survive instance termination. Use before any schema migration or major upgrade.

### Snapshot copy + share
Cross-region for DR; cross-account for handing data to another team.

```bash
aws rds copy-db-snapshot \
  --source-db-snapshot-identifier arn:aws:rds:ap-southeast-1:123:snapshot:s1 \
  --target-db-snapshot-identifier s1-dr \
  --kms-key-id $DR_KMS_KEY --region ap-east-1
```

---

## 4. Parameter Groups & Option Groups

The DB's configuration knobs.

- **Parameter group** = `postgresql.conf` / `my.cnf` equivalents. `shared_buffers`, `max_connections`, `work_mem`, query log thresholds.
- **Option group** = engine-specific add-ons. MS SQL Server agent, MariaDB audit plugin, Oracle TDE.

You **can't modify the default groups.** Create custom ones, attach to your instance.

```bash
aws rds create-db-parameter-group --db-parameter-group-name pg-tuned-16 \
  --db-parameter-group-family postgres16 --description "tuned"
aws rds modify-db-parameter-group --db-parameter-group-name pg-tuned-16 \
  --parameters "ParameterName=log_min_duration_statement,ParameterValue=500,ApplyMethod=immediate"
aws rds modify-db-instance --db-instance-identifier demo-pg \
  --db-parameter-group-name pg-tuned-16 --apply-immediately
```

Some parameters are **static** (require reboot); others are **dynamic** (apply immediately). RDS tells you which when you try.

---

## 5. Aurora — AWS's rewritten storage engine

Same SQL dialects (Postgres/MySQL), totally different internals.

### What's different
- **Storage is decoupled from compute**. Compute nodes are stateless; the storage layer is a distributed, log-structured cluster of 6 storage nodes across 3 AZs.
- **6-way replication, write quorum 4/6, read quorum 3/6.** Survives AZ + node failures with no data loss.
- **Storage auto-scales** from 10 GB to 128 TB. You don't allocate.
- **Up to 15 read replicas** in a cluster, all reading from the shared storage layer (no replication lag in the traditional sense).
- **~5x throughput of MySQL, ~3x of PostgreSQL** on AWS's own benchmarks.

### Cluster shape
```
                   ┌─── Writer Endpoint  ────► writer instance
Cluster (shared    │
 storage volume) ──┤─── Reader Endpoint ────► (load balanced across readers)
                   │
                   └─── Custom Endpoints ───► (you-defined groups)
```

### Launch
```bash
aws rds create-db-cluster \
  --db-cluster-identifier demo-aurora \
  --engine aurora-postgresql --engine-version 16.2 \
  --master-username dbadmin --master-user-password "..." \
  --db-subnet-group-name my-db-subnets \
  --vpc-security-group-ids $DB_SG \
  --storage-encrypted

aws rds create-db-instance --db-instance-identifier aurora-writer \
  --db-cluster-identifier demo-aurora --engine aurora-postgresql \
  --db-instance-class db.r6g.large
aws rds create-db-instance --db-instance-identifier aurora-reader-1 \
  --db-cluster-identifier demo-aurora --engine aurora-postgresql \
  --db-instance-class db.r6g.large
```

### Aurora Serverless v2
Auto-scales compute capacity in fractional ACUs (Aurora Capacity Units, ~2GB memory each) in real time. Pay per-second.

- **v1 (legacy):** scales by pausing and resuming — cold starts of seconds-to-minutes.
- **v2 (current):** scales without pausing. Sub-second.

Configure min/max ACUs (e.g., 0.5 to 16).

```bash
aws rds create-db-cluster --db-cluster-identifier serverless-aurora \
  --engine aurora-postgresql --engine-mode provisioned \
  --serverless-v2-scaling-configuration MinCapacity=0.5,MaxCapacity=16 \
  ...
```

**Aurora Serverless v2 minimum is 0.5 ACU = ~$43/mo idle** — not "scales to zero." (v1 did pause to zero, with cold starts.)

### Aurora Global Database
1 primary region writer + up to 5 read-only regions, replicated via dedicated infrastructure with <1s lag. Failover to a secondary region in <1 min. DR for Tier-0 systems.

### Aurora I/O-Optimized
A pricing mode that drops per-I/O charges in favor of higher per-instance cost. Pays off for high-IO workloads — pivot point is ~25% of bill being I/O.

---

## 6. Practical: a production Aurora Postgres stack

```typescript
// CDK
const cluster = new rds.DatabaseCluster(this, "AppDb", {
  engine: rds.DatabaseClusterEngine.auroraPostgres({ version: rds.AuroraPostgresEngineVersion.VER_16_2 }),
  credentials: rds.Credentials.fromGeneratedSecret("dbadmin"),  // goes to Secrets Manager
  serverlessV2MinCapacity: 0.5,
  serverlessV2MaxCapacity: 8,
  writer: rds.ClusterInstance.serverlessV2("writer"),
  readers: [
    rds.ClusterInstance.serverlessV2("reader1", { scaleWithWriter: true }),
  ],
  vpc,
  vpcSubnets: { subnetType: ec2.SubnetType.PRIVATE_ISOLATED },
  storageEncrypted: true,
  backupRetention: cdk.Duration.days(14),
  deletionProtection: true,
  iamAuthentication: true,
  cloudwatchLogsExports: ["postgresql"],
  monitoringInterval: cdk.Duration.seconds(60),
});

// App role gets DB auth via IAM tokens, not password
cluster.grantConnect(appRole, "app_user");
```

### Connecting from app code (IAM auth)
```python
import boto3, psycopg2
rds = boto3.client("rds", region_name="ap-southeast-1")
token = rds.generate_db_auth_token(DBHostname=host, Port=5432, DBUsername="app_user")
conn = psycopg2.connect(host=host, port=5432, user="app_user",
                       password=token, dbname="app", sslmode="require")
```

No password to rotate. IAM controls who can mint tokens.

---

## 7. RDS Proxy — connection pooling at the edge

EC2/Lambda apps with bursty traffic open and close DB connections rapidly. Postgres/MySQL handle dozens-to-hundreds of connections, not thousands. **RDS Proxy** is a managed PgBouncer/ProxySQL equivalent that multiplexes connections, supports IAM auth, and survives DB failovers with sub-second reconnection.

```bash
aws rds create-db-proxy --db-proxy-name app-proxy --engine-family POSTGRESQL \
  --auth '[{"AuthScheme":"SECRETS","SecretArn":"arn:...","IAMAuth":"REQUIRED"}]' \
  --role-arn $PROXY_ROLE --vpc-subnet-ids subnet-data-a subnet-data-b \
  --require-tls
```

Use it whenever:
- Lambda calls Postgres (Lambda concurrency × N connections / function = exhaustion).
- High connection-churn apps.
- You want zero-downtime DB failover for clients.

Costs: ~$0.015/hr per vCPU of underlying DB.

---

## 8. Performance & Monitoring

- **Performance Insights** (free up to 7 days retention, $$$ for longer): visualizes DB load by SQL/wait event. **Enable on every DB.**
- **Enhanced Monitoring**: OS-level metrics at 1-15s granularity.
- **Slow query log**: enable via parameter group.
- **CloudWatch metrics**: `CPUUtilization`, `DatabaseConnections`, `FreeableMemory`, `ReadIOPS`, `ReplicaLag`.

---

## 9. Common Mistakes & Gotchas

- **No Multi-AZ on production DBs.** Single-AZ DB = your app dies for an hour during routine AZ events.
- **Backups disabled** (or 1-day retention) to "save money." Saves pennies, loses jobs.
- **Encryption not enabled at create.** Can't be turned on later. Snapshot, copy with encryption, restore.
- **Master password in env vars or code.** Use **Secrets Manager** (auto-rotation) or **IAM auth** (no password at all).
- **Public RDS instance.** Don't give DBs public IPs unless absolutely required + WAF + restrictive SG.
- **Default parameter group.** Can't be modified. Always create a custom one.
- **Connection storms on Lambda.** Postgres → exhaustion. Use RDS Proxy or DynamoDB.
- **Long-running transactions** holding locks during deploys. Set `idle_in_transaction_session_timeout`.
- **Replica lag in code path.** Your "read after write" against a replica returns stale data. Use writer for read-your-writes flows.
- **Engine version pinning.** Auto minor version upgrades will reboot you at unpredictable times. Disable in prod; manage via maintenance windows.
- **Forgot `--deletion-protection`.** A typo in IaC = data gone.
- **Aurora Serverless v2 min capacity 0.5 ≠ free.** $43/mo just for being on.
- **Multi-AZ ≠ Read Replica.** Multi-AZ standby isn't readable. (Multi-AZ Cluster is — different feature.)
- **`db.t` instance class for prod.** Burstable CPU + a long query = throttling. Use `db.m`/`db.r` for steady workloads.
- **Storage autoscaling off.** Disk fills → DB stops accepting writes. Enable storage autoscaling, set a sane cap.
- **Cross-region replica without `--storage-encrypted`** or with wrong KMS key. Replication fails confusingly.
- **Aurora cluster endpoint vs instance endpoint.** Connect to the cluster's *writer endpoint*, not a specific instance — failover updates the cluster endpoint's DNS.

---

## 🎯 Key Takeaways

- **Aurora's storage layer is the killer feature.** Decoupled compute/storage with 6-way replication across 3 AZs gives you HA, scale-out reads, and storage that grows automatically — all impossible on stock MySQL/Postgres.
- **Multi-AZ is for HA; Read Replicas are for read scaling.** They don't substitute for each other. Production = both.
- **Use IAM database auth + Secrets Manager**, not a password in your config. No human should know the DB password.
- **RDS Proxy is mandatory for serverless DB clients.** Without it, Lambda × Postgres = scheduled outages.
- **Performance Insights from day one.** It's nearly free and the single best tool for "why is the DB slow?" — far more useful than CPU graphs alone.

*← [prev](./05_s3_object_storage.md) | [next →](./07_dynamodb.md)*
