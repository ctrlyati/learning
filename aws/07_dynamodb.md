# 07 — DynamoDB: NoSQL at Any Scale

> **Goal:** Model data for Dynamo's key-based access pattern, understand capacity and cost, and know when single-table design is genius vs. masochism.

DynamoDB is AWS's premier NoSQL database: a managed, infinitely scalable, predictable-latency (single-digit ms) key-value/document store. It also has the steepest learning curve in AWS — because data modeling for Dynamo is **the opposite** of relational design.

---

## 1. The Core Model — partition key, sort key, items

**Mental model:** DynamoDB is a giant distributed hash map. Each table is sharded by **partition key** (PK). Within a partition, items are sorted by an optional **sort key** (SK), enabling range queries. There are no joins, no SQL, no schema beyond keys.

```
Table: Users
  PK: userId
  SK: (none)

  | userId | name  | email           |
  |--------|-------|-----------------|
  | u-1    | Alice | a@example.com   |
  | u-2    | Bob   | b@example.com   |
```

Or, more realistically with sort key:
```
Table: Orders
  PK: customerId
  SK: orderId

  | customerId | orderId  | total | status   |
  |------------|----------|-------|----------|
  | c-1        | o-2024-1 | 49.95 | shipped  |
  | c-1        | o-2024-2 | 12.50 | pending  |
  | c-2        | o-2024-3 | 99.00 | pending  |
```

This lets you do `Query(customerId=c-1)` → all of c-1's orders cheaply (single partition read, sorted by SK).

### Create one
```bash
aws dynamodb create-table \
  --table-name Orders \
  --attribute-definitions AttributeName=customerId,AttributeType=S AttributeName=orderId,AttributeType=S \
  --key-schema AttributeName=customerId,KeyType=HASH AttributeName=orderId,KeyType=RANGE \
  --billing-mode PAY_PER_REQUEST \
  --tags Key=project,Value=aws-course
```

### Items aren't rows
- Different items in the same table can have totally different attributes — Dynamo is schemaless.
- Items max 400 KB each.
- Attribute types: `S` string, `N` number, `B` binary, `BOOL`, `NULL`, `L` list, `M` map, `SS`/`NS`/`BS` sets.

---

## 2. The Two API Patterns

### `GetItem` / `BatchGetItem`
Direct lookup by PK (and SK). Fastest, cheapest. O(1).

```python
ddb = boto3.resource("dynamodb")
table = ddb.Table("Orders")
r = table.get_item(Key={"customerId": "c-1", "orderId": "o-2024-1"})
print(r["Item"])
```

### `Query`
Lookup within a single partition (PK = value, optional SK condition). Fast, range-aware.

```python
r = table.query(
    KeyConditionExpression=Key("customerId").eq("c-1") & Key("orderId").begins_with("o-2024"),
    Limit=20, ScanIndexForward=False,    # newest first
)
```

### `Scan`
**Reads every item in the table** and filters. O(N), expensive, slow. Use only for ETL / one-offs.

### Filter expressions ≠ free
`FilterExpression` runs *after* the read. You still pay for all bytes scanned, even those filtered out.

### `UpdateItem` — atomic in-place updates
```python
table.update_item(
    Key={"customerId": "c-1", "orderId": "o-2024-1"},
    UpdateExpression="SET #s = :new_status, updatedAt = :now",
    ConditionExpression="#s = :expected",   # optimistic concurrency
    ExpressionAttributeNames={"#s": "status"},
    ExpressionAttributeValues={":new_status": "shipped", ":expected": "pending", ":now": "2026-05-11T10:00Z"},
)
```

### Transactions
`TransactWriteItems` / `TransactGetItems`: up to 100 items, ACID, **2x cost**.

---

## 3. Indexes — escape the primary key

If you need to query by an attribute that isn't the PK/SK, create an index.

### Global Secondary Index (GSI)
A separate index with its own PK (and optional SK) over the table. Updated asynchronously (eventually consistent). Can be added/removed anytime. **The 95% answer.**

```bash
aws dynamodb update-table --table-name Orders \
  --attribute-definitions AttributeName=status,AttributeType=S AttributeName=createdAt,AttributeType=S \
  --global-secondary-index-updates '[{
    "Create": {
      "IndexName": "ByStatus",
      "KeySchema": [
        {"AttributeName": "status", "KeyType": "HASH"},
        {"AttributeName": "createdAt", "KeyType": "RANGE"}
      ],
      "Projection": {"ProjectionType": "ALL"}
    }
  }]'
```

Now `Query(IndexName=ByStatus, status="pending")` → all pending orders sorted by createdAt.

### Local Secondary Index (LSI)
Alternative SK, same PK. Strongly consistent. Must be created at table creation, max 5 per table. Niche.

### Projection types
- `KEYS_ONLY` — index stores only keys, cheapest.
- `INCLUDE` — index stores specific attributes.
- `ALL` — full item copy, easiest, most $$$.

---

## 4. Capacity Modes

### On-Demand (PAY_PER_REQUEST)
Pay per request. Auto-scales instantly. Great for:
- Unpredictable workloads.
- New apps where you haven't profiled traffic.
- Spiky traffic.

~$1.25 per million writes, $0.25 per million reads (us-east-1).

### Provisioned
You set RCUs (Read Capacity Units) and WCUs (Write Capacity Units). Throttles if exceeded.
- 1 RCU = 1 strongly consistent read of 4KB/s, or 2 eventually-consistent reads.
- 1 WCU = 1 write of 1KB/s.

Cheaper at scale (often 70%+ savings) **if** you know your traffic and use auto-scaling.

```bash
aws dynamodb update-table --table-name Orders --billing-mode PROVISIONED \
  --provisioned-throughput ReadCapacityUnits=100,WriteCapacityUnits=50
```

**Always pair provisioned with Auto Scaling.** Otherwise you over-provision (waste) or under-provision (throttle).

### Reserved Capacity
Commit to 1 or 3 years of provisioned capacity → significant discount. For mature, predictable workloads.

---

## 5. Single-Table Design — Dynamo's signature pattern

The advanced (and controversial) approach: store **all of your application's data in one table** by using generic PK/SK names (`PK`, `SK`) and overloading them.

```
Table: AppData
  PK: PK
  SK: SK

PK            SK             Type     Other attrs
USER#u-1      PROFILE        User     {name, email}
USER#u-1      ORDER#o-2024-1 Order    {total, status}
USER#u-1      ORDER#o-2024-2 Order    {total, status}
ORDER#o-2024-1 PRODUCT#p-7   LineItem {qty, price}
```

One Query on `PK=USER#u-1` returns the user + all their orders. One Query on `PK=ORDER#o-2024-1` returns the order + its line items. **All your "joins" become single Queries.**

### When it's brilliant
- You know your access patterns up front (list them all *before* designing).
- Read-heavy app where joins would be painful.
- You want minimal latency and predictable cost.

### When it's a trap
- Access patterns evolve frequently — schema migrations are painful.
- Multiple teams sharing a table become coupled.
- Ad-hoc analytics (Scan or export to S3 → Athena instead).

Pragmatic advice: **most apps should use multiple tables** (one per entity), and reach for single-table only when patterns are stable and performance/cost demands it. The Rick Houlihan talks are great, but they're not gospel.

---

## 6. DynamoDB Streams & Change Data Capture

A stream is an ordered log of item changes (insert/modify/delete) on a table, retained for 24 hours. Each shard is processed in order.

**Use cases:** replication to another store, search indexing (→ OpenSearch), audit trails, fan-out to downstream systems.

```bash
aws dynamodb update-table --table-name Orders \
  --stream-specification StreamEnabled=true,StreamViewType=NEW_AND_OLD_IMAGES
```

Most common consumer: **Lambda triggered by stream** with batching:
```typescript
new lambda.EventSourceMapping(this, "OrdersStream", {
  target: indexerFn,
  eventSourceArn: ordersTable.tableStreamArn,
  startingPosition: lambda.StartingPosition.LATEST,
  batchSize: 100, parallelizationFactor: 4,
  bisectBatchOnError: true,
  retryAttempts: 3,
  onFailure: new SqsDlq(deadLetterQueue),
});
```

### Kinesis Data Streams for Dynamo
An alternative stream destination, with longer retention (up to 365 days) and Kinesis ecosystem integration.

---

## 7. Other Capabilities

### Global Tables
Multi-region, multi-active replication. Writes anywhere replicate to all regions in <1s usually. Built on Streams + custom replication.

### TTL
Auto-delete expired items. Add a `ttl` attribute (Unix epoch seconds), enable TTL on table.

```bash
aws dynamodb update-time-to-live --table-name Sessions \
  --time-to-live-specification "Enabled=true, AttributeName=ttl"
```
Items are deleted lazily within 48 hours of expiry. **Free** — no WCU consumed.

### DAX (DynamoDB Accelerator)
In-memory cache cluster fronting Dynamo. Microsecond reads for hot keys. Useful for read-heavy workloads with hot items. Costs ~$0.27/hr for a small node.

### Backup
- **PITR**: continuous backup, restore to any second within 35 days. **Always enable.**
- **On-demand backups**: snapshots, indefinite retention.

```bash
aws dynamodb update-continuous-backups --table-name Orders \
  --point-in-time-recovery-specification PointInTimeRecoveryEnabled=true
```

### Import/Export to S3
Bulk load from S3 or export to S3 (Parquet/JSON/Ion) without consuming RCUs. For analytics.

---

## 8. Practical: a session store

```typescript
// CDK
const sessions = new dynamodb.Table(this, "Sessions", {
  partitionKey: { name: "sessionId", type: dynamodb.AttributeType.STRING },
  billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
  timeToLiveAttribute: "ttl",
  pointInTimeRecovery: true,
  encryption: dynamodb.TableEncryption.AWS_MANAGED,
  removalPolicy: cdk.RemovalPolicy.RETAIN,
});

// GSI for sessions by userId
sessions.addGlobalSecondaryIndex({
  indexName: "ByUser",
  partitionKey: { name: "userId", type: dynamodb.AttributeType.STRING },
  sortKey:      { name: "createdAt", type: dynamodb.AttributeType.STRING },
});

sessions.grantReadWriteData(appRole);
```

App pseudocode:
```python
table.put_item(Item={
    "sessionId": session_id,
    "userId": user_id,
    "createdAt": now_iso,
    "ttl": int(time.time()) + 3600,  # auto-expire in 1h
    "data": session_data,
})
```

---

## 9. Common Mistakes & Gotchas

- **`Scan` in hot paths.** Slow, expensive, eventually consistent. Use Query + GSIs.
- **Hot partitions.** A few keys getting all traffic → throttling even with plenty of total capacity. Distribute writes by adding random suffixes ("write sharding"), or rethink the PK.
- **Filter expression mistake** — paying for data you filter out. Use sparingly; prefer indexed queries.
- **Items growing past 400 KB.** Hard limit. Split items or offload to S3 and store an S3 pointer.
- **Forgetting PITR.** Accidental delete = unrecoverable.
- **Strong consistency assumed but not asked for.** Reads default to eventually consistent. Add `ConsistentRead=True` (costs 2x).
- **GSI throttling not understood.** GSIs have their own capacity. A skewed PK on a GSI throttles writes to the table.
- **Single-table design adopted too early.** Now you can't add a new access pattern without a migration that touches every item.
- **Wrong cost model.** On-Demand is great for low/spiky traffic but expensive at scale. Switch to Provisioned + Auto Scaling once you have data.
- **Storing huge documents.** Dynamo is for KV, not for blobs. Use S3.
- **Joins in app code.** If you find yourself doing many round-trips per request, your access pattern was wrong; redesign keys.
- **TTL not auto-deleting fast enough.** Up to 48-hour grace. Don't rely on TTL for security-critical expiry.
- **Long item names / attribute names.** Storage is billed by total bytes including attribute names — short names save real money on high-throughput tables.
- **No backups before bulk delete.** Always export to S3 first.
- **`update_item` without `ConditionExpression`** = lost updates under concurrency.
- **VPC endpoint to Dynamo not configured.** Private subnet → traffic through NAT = data charges. Gateway endpoint is free.

---

## 🎯 Key Takeaways

- **Model from access patterns backwards.** Write down every query your app will make *before* defining keys. Dynamo design starts with "how will I read this?"
- **Query + GSI is the bread and butter.** Scan is a code smell. Add a GSI rather than scanning + filtering.
- **On-Demand for new/spiky; Provisioned + Auto Scaling for mature/predictable.** Switching is one API call — start with On-Demand to gather profile data.
- **PITR + TTL + Streams are the three "always enable" features.** PITR for recovery, TTL for cleanup, Streams for downstream integration.
- **Single-table design is powerful but premature optimization for most apps.** Don't adopt it until you have stable, well-understood access patterns and the cost/perf justification.

*← [prev](./06_rds_aurora.md) | [next →](./08_lambda_serverless.md)*
