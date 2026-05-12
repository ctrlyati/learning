# 07 — Azure SQL and Cosmos DB

> **Goal:** Decide between Azure SQL Database, Managed Instance, and Cosmos DB for any workload — and configure each with sensible defaults for HA, security, and cost.

## 1. The two-database universe and the question to ask first

Azure's two flagship databases serve fundamentally different workloads:

- **Azure SQL Database** — managed SQL Server, relational, ACID, vertical-scale-first. Pick when your data model is relational and your reads/writes mostly fit in one region.
- **Cosmos DB** — distributed, schema-flexible, multi-region active-active, ms-latency, horizontal-scale-first. Pick when you need global distribution, predictable single-digit-ms latency, or a non-relational data model.

The question to ask first: *"Does the workload need to write in multiple regions concurrently?"* If yes → Cosmos. If no → almost certainly Azure SQL. The rest is detail.

Two adjacent services worth knowing:

- **Azure Database for PostgreSQL / MySQL** — managed Postgres/MySQL (the *Flexible Server* SKU is the modern one). Use these for open-source stacks; the operational model is similar to Azure SQL.
- **Azure SQL Managed Instance** — for SQL Server features you can't get on Azure SQL DB (CLR, SQL Agent, cross-database queries). Slower to provision, pricier, but lift-and-shift-friendly.

## 2. Azure SQL — flavors, mechanics, scaling

### Three deployment options

| | Azure SQL Database | Azure SQL Managed Instance | SQL Server on Azure VM |
|--|--|--|--|
| Surface | One database, no instance | Full instance (~95% of on-prem SQL) | Bring-your-own |
| Provisioning | minutes | ~4-6 hours initial | minutes |
| Ops you keep | almost none | some (instance config) | all (patching, backups) |
| Cost (rough) | $ | $$ | $$$ (you pay VM + SQL license) |
| Use when | Cloud-native or new builds | Lift-and-shift legacy SQL Server | Total control, OS-level access |

We'll focus on **Azure SQL Database** — the cloud-native default.

### Purchasing models

- **DTU-based** — legacy. A bundle of CPU/memory/IO. Avoid for new builds.
- **vCore-based** — modern. Pick cores, memory, storage independently. Two tiers worth knowing:
  - **General Purpose** — remote storage, recovers via re-attach. The default. Cheap.
  - **Business Critical** — local SSD, AlwaysOn replicas, read-only replica included. High IOPS, ms latency.
  - **Hyperscale** — for >4 TB single-DB. Decoupled storage and compute, fast restore. Pricing different.
- **Serverless** (vCore subset) — auto-pause when idle. Bills per second. Perfect for dev DBs.

```bash
RG=rg-data-prod
SRV=sql-orders-prod-$(openssl rand -hex 3)

az sql server create -g $RG -n $SRV -l eastus2 \
  --admin-user sqladmin --admin-password 'ChangeMe!Now' \
  --enable-public-network false \
  --enable-ad-only-auth true \
  --external-admin-name "DBA Group" \
  --external-admin-sid $(az ad group show --group "DBA Group" --query id -o tsv) \
  --external-admin-type Group

az sql db create -g $RG -s $SRV -n orders \
  --edition GeneralPurpose --family Gen5 --capacity 2 \
  --backup-storage-redundancy Zone \
  --zone-redundant true \
  --maintenance-configuration-id /subscriptions/<sub>/providers/Microsoft.Maintenance/publicMaintenanceConfigurations/SQL_EastUS2_DB_2
```

Two flags worth highlighting:

- `--enable-ad-only-auth true` — disables SQL authentication entirely. Only Entra identities can connect. This should be the default.
- `--enable-public-network false` + a Private Endpoint = SQL is fully off the public internet.

### Connecting with Entra + managed identity

Once Entra-only auth is on, your app (Function, App Service, Container App) authenticates via its MI:

```csharp
// .NET — using Microsoft.Data.SqlClient with Active Directory Default
var connStr = "Server=sql-orders-prod.database.windows.net;Database=orders;Authentication=Active Directory Default;Encrypt=true;";
using var conn = new SqlConnection(connStr);
await conn.OpenAsync();
```

The driver picks up the MI's token automatically via `DefaultAzureCredential`. Grant the MI database-level permissions:

```sql
-- run as Entra admin
CREATE USER [fn-orders-prod] FROM EXTERNAL PROVIDER;
ALTER ROLE db_datareader ADD MEMBER [fn-orders-prod];
ALTER ROLE db_datawriter ADD MEMBER [fn-orders-prod];
```

The `[fn-orders-prod]` is the Function App's managed identity display name. No password, no secret, no rotation.

### Backups and HA

- Built-in automated backups: 7-35 days PITR (point-in-time restore). LTR (Long-Term Retention) up to 10 years for compliance.
- **Zone-redundant** databases (Business Critical and Premium tiers, and GP with the zone-redundant flag) survive zone outage. **Active geo-replication** to another region for DR.
- **Failover groups** — automatic failover policy across regions with read-write listener endpoint that follows the primary.

## 3. Cosmos DB — APIs, consistency, partitioning

Cosmos DB is conceptually a globally-distributed JSON document store with **five API surfaces** layered on the same engine:

| API | What you write | When to pick |
|-----|----------------|--------------|
| **NoSQL (Core)** | SQL-like queries over JSON | The default; most features land here first |
| **MongoDB** | Mongo wire protocol | Migrating from Mongo |
| **Cassandra** | CQL | Migrating from Cassandra |
| **Gremlin** | Graph traversal | Graph data |
| **Table** | Azure Table API | Supersedes Storage Table |

For new builds use **NoSQL API**. The others are migration paths.

### The partition key — the single most important schema decision

A Cosmos container is partitioned by a single property you pick (e.g., `/tenantId`, `/userId`, `/deviceId`). The partition key determines:

- How requests are routed (in-partition queries are cheap, cross-partition are expensive).
- Throughput ceiling per logical partition (20 GB and 10k RU/s).
- Cost — a bad key means hot partitions.

Rules of thumb: high cardinality, even distribution, queryable in most reads. `/userId` for multi-tenant apps; `/orderId` for time-series; `/eventId` for events. *Test partition strategy with realistic data before committing.*

### Consistency levels — five flavors

From strongest to weakest:

1. **Strong** — Linearizable. Reads always see latest committed write. Restricts you to a single write region. Slowest.
2. **Bounded Staleness** — reads lag by ≤ K versions or T seconds. Tunable. Good middle ground for multi-region writes with predictable bounds.
3. **Session** — within a session (client) you read your own writes, monotonic reads, monotonic writes. The default and what 95% of apps need.
4. **Consistent Prefix** — reads see writes in order, no gaps, but may be stale.
5. **Eventual** — fastest, no ordering guarantees. Counters and similar.

You set a default on the account; the client SDK can downgrade per request (e.g., a "show me anything" endpoint can use Eventual on a Session account). You cannot upgrade past the account default per request.

### Throughput models

- **Manual RU/s (provisioned)** — fixed RU/s, predictable cost.
- **Autoscale** — set max RU/s; Cosmos scales between 10% and 100% per second.
- **Serverless** — pay per request. Great for dev or unpredictable spiky workloads. Caps apply.

```bash
az cosmosdb create -g rg-data-prod -n cosmos-orders-prod \
  --locations regionName=eastus2 failoverPriority=0 isZoneRedundant=true \
  --locations regionName=westus3 failoverPriority=1 isZoneRedundant=true \
  --default-consistency-level Session \
  --enable-multiple-write-locations true \
  --enable-automatic-failover true \
  --public-network-access Disabled

az cosmosdb sql database create -g rg-data-prod -a cosmos-orders-prod -n orders

az cosmosdb sql container create \
  -g rg-data-prod -a cosmos-orders-prod -d orders \
  -n orderItems --partition-key-path /orderId \
  --max-throughput 4000   # autoscale between 400 and 4000 RU/s
```

### Cosmos data-plane RBAC

Like SQL, Cosmos has its own data-plane RBAC layered on Azure RBAC. Built-in roles:

- `Cosmos DB Built-in Data Reader` (id `00000000-0000-0000-0000-000000000001`)
- `Cosmos DB Built-in Data Contributor` (id `00000000-0000-0000-0000-000000000002`)

Grant to a managed identity:

```bash
az cosmosdb sql role assignment create \
  -g rg-data-prod -a cosmos-orders-prod \
  --scope "/dbs/orders" \
  --principal-id $FN_OID \
  --role-definition-id 00000000-0000-0000-0000-000000000002
```

Combine with `disableLocalAuth: true` on the account → keys are useless and only Entra works.

## 4. Practical Application — pick-the-right-one decision sketch

Workload A: **"Customer-facing e-commerce orders, multi-region active-active, must work if East US goes down."**
- Cosmos DB NoSQL, two write regions (eastus2 + westus3), Session consistency, `/customerId` partition key, autoscale 1000-10000 RU/s, Private Endpoint, MI auth.

Workload B: **"Internal ERP database, lots of joins, 200 GB, single region OK."**
- Azure SQL Database, vCore General Purpose, 4 cores, zone-redundant, Private Endpoint, Entra-only auth, 14-day PITR, LTR for monthly snapshots.

Workload C: **"Existing on-prem SQL with SQL Agent jobs, CLR, cross-DB queries."**
- Azure SQL Managed Instance. Lift, shift, migrate later if appropriate.

Workload D: **"Event store: append-only, billions of events, queries always by aggregateId."**
- Cosmos DB NoSQL, `/aggregateId` partition key, change feed enabled, TTL on cold events, eventual consistency for read replicas, Session for write paths.

Workload E: **"Cheap dev/test database that's idle 80% of the day."**
- Azure SQL Database Serverless, 0.5-2 vCore, auto-pause after 60 min. Or Cosmos Serverless.

The deciding question is always: **shape of the data + shape of the access pattern + cross-region requirement**. Cost falls out.

## 5. Common Mistakes & Gotchas

- **Picking Cosmos because "NoSQL is faster" with no actual reason.** Cosmos is brilliant when you need its distribution model and pay for its consistency tradeoffs. For a single-region relational workload it's strictly worse and more expensive than Azure SQL.
- **Bad partition key.** "I'll partition by `/createdDate`" → every write goes to today's partition → hot partition → 429 errors at 10k RU/s. Pick high-cardinality keys distributed across the keyspace.
- **Defaulting to Strong consistency.** Restricts you to a single write region and crushes latency. Session is the right default for 95% of apps.
- **Confusing Azure SQL Database with Managed Instance.** They are different SKUs with different limits, pricing, network models. MI requires a dedicated subnet.
- **Leaving SQL authentication enabled.** Username/password creds get leaked. Set `azureADOnlyAuthentication: true` from day one.
- **Public network access on production DBs.** Default-deny + Private Endpoint. The "Allow Azure services" tick is *not* the same as a Private Endpoint and is much broader than people realize.
- **Cosmos local-auth keys.** Cosmos generates two keys per account; if leaked, full data access. Disable: `--disable-local-auth true` (when CLI supports it; otherwise via property `disableLocalAuth: true` in ARM/Bicep).
- **Backup strategy assumed.** Azure SQL gives you PITR but not LTR by default. Cosmos has continuous backup but you must enable PITR. Confirm RPO/RTO before incident time.
- **DTU model.** Don't pick it for new builds. vCore is strictly more flexible and the modern path.
- **Maxing autoscale RU.** Cosmos autoscale charges based on the *maximum* you set, even at low load. Set max realistically.
- **Index everything in Cosmos.** Default indexing is "all properties" — great for ad-hoc queries, expensive for write-heavy workloads. Tune indexing policy.
- **Cross-database queries on Azure SQL DB.** Not supported. Use MI for that, or model around it.
- **Connection limits.** Azure SQL has per-DB connection caps that scale with tier. App with bad connection pooling + spiky load = `error 18456` storms.

## 🎯 Key Takeaways

- **Ask "single-region writes?" first.** No → Cosmos. Yes → Azure SQL.
- **Partition key is destiny in Cosmos.** High cardinality, even distribution, in every query — pick well.
- **Default to Session consistency** — strong consistency in Cosmos has serious cost in availability and latency.
- **Entra-only auth + Private Endpoint + MI-based connection strings** is the modern data-tier baseline for both services.
- **vCore + zone-redundant + LTR retention** for production Azure SQL. **Autoscale + multi-region writes + change feed** for production Cosmos.

*← [prev](./06_storage.md) | [next → 08_functions_serverless.md](./08_functions_serverless.md)*
