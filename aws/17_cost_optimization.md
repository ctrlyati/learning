# 17 — Cost Optimization: Pricing, Savings Plans, RIs, and Common Waste

> **Goal:** Read an AWS bill like a balance sheet, apply Savings Plans and RIs correctly, and hunt the silent waste patterns that consume most teams' AWS budgets.

---

## 1. The AWS pricing model — your bill is a tax

**Mental model:** AWS bills you for **usage** (per-second, per-request, per-GB) across hundreds of services. There's no flat fee. The bill is a function of: what you turn on, how long you leave it on, and how much it processes.

Three core dimensions:
1. **Capacity** — CPU/RAM hours (EC2/Fargate/Lambda), provisioned IOPS, storage GB-months.
2. **Throughput** — requests, API calls, GB processed.
3. **Egress** — data leaving AWS regions (often the silent killer).

Most of your bill, in dollar order, will typically be: **EC2 + EBS + RDS + Data Transfer + S3 + the kitchen sink**. For serverless-first shops it shifts to **Lambda + DynamoDB + ELB + CloudFront + Data Transfer**.

---

## 2. The free tier and the trap

The free tier is generous but has nasty cliffs:
- **750 hours/month EC2 t2.micro/t3.micro** — running 2 instances = 1500 hours = 750 over.
- **5 GB S3 Standard storage** — easy to exceed with logs.
- **1M Lambda invocations + 400k GB-seconds**.
- **750 hours RDS db.t2.micro / db.t3.micro / db.t4g.micro** — single instance OK; Multi-AZ counts double.
- **NAT Gateway is NOT free tier.** ~$32/mo first month.

Always-free benefits keep working (DynamoDB 25 GB, Lambda 1M, SNS 1M, CloudWatch 10 metrics + 5 GB ingestion, ...).

---

## 3. Compute pricing models

| Model | Discount | Commitment | Flexibility |
|---|---|---|---|
| **On-Demand** | 0% | None | Full |
| **Spot** | up to 90% | None (2-min interruption notice) | Stateless workloads |
| **Compute Savings Plan** | up to 66% | $/hr commit, 1 or 3 yr | EC2 + Fargate + Lambda; any region; any family |
| **EC2 Instance Savings Plan** | up to 72% | $/hr commit, family-locked, 1 or 3 yr | EC2 only, family locked |
| **Standard Reserved Instance** | up to 75% | Instance type, AZ, 1 or 3 yr | Old, less flexible than SPs |
| **Convertible Reserved Instance** | up to 66% | 1 or 3 yr, exchangeable | EC2 only, exchangeable |
| **Dedicated Host** | varies | Per-host commit | BYOL, compliance |

### Savings Plans vs RIs
SPs replaced RIs for most use cases:
- **Compute SP**: most flexible. Covers EC2 + Fargate + Lambda. Region/family agnostic.
- **EC2 Instance SP**: deeper discount, family-locked but size-flexible.
- **Standard RI**: deepest discount but rigid. Niche today.

**Practical mix:** cover 60-80% of steady-state on Compute SP, leave 20-40% on-demand for flex, run batch/stateless on Spot.

### Where AWS hides the SP recommendation
**Cost Explorer → Recommendations**. The recommendations include payback period, estimated savings, and ROI. Buy 1-year SPs unless you're 100% sure about 3-year commits.

```bash
aws ce get-savings-plans-purchase-recommendation \
  --savings-plans-type COMPUTE_SP \
  --term-in-years ONE_YEAR \
  --payment-option NO_UPFRONT \
  --lookback-period-in-days SIXTY_DAYS
```

---

## 4. The biggest cost categories — and what to watch

### 4.1 EC2
- Right-size with **Compute Optimizer**. Most fleets are 30-50% overprovisioned.
- **Graviton** = 20-40% cheaper than x86 equivalents. Default to it.
- **Older generations** are 20%+ more expensive than newer for same perf. Migrate `m4` → `m6i`/`m7g`.
- **Idle instances**. Stopped ≠ free — EBS still bills. Terminate if not needed.

### 4.2 EBS
- **gp2 → gp3** = ~20% cheaper.
- **Unattached volumes** bill forever. List with:
  ```bash
  aws ec2 describe-volumes --filters Name=status,Values=available --query 'Volumes[].[VolumeId,Size,CreateTime]' --output table
  ```
- **Old snapshots.** Lifecycle them; one customer paid $200k/yr for 2017 snapshots no one knew about.

### 4.3 Data Transfer (DT)
The cost line that surprises everyone. Key facts:
- **DT IN to AWS: free** from internet.
- **DT OUT to internet**: ~$0.12/GB (ap-southeast-1), tiered down to $0.05/GB at petabytes.
- **DT between AZs**: $0.01/GB each way → **$0.02/GB round trip**. Chatty cross-AZ services = $$$.
- **DT between regions**: $0.02/GB.
- **DT via VPC peering** within a region: $0.01/GB each way.
- **DT VPC → S3 in same region via Gateway endpoint**: free. Without the endpoint → goes via NAT, paying NAT + processing fees.
- **DT VPC → CloudFront → user**: charged from CloudFront (cheaper) only; VPC → CloudFront is free.

**The single highest-leverage cost lever** for many shops: enable S3/DynamoDB Gateway endpoints (free), remove unused NAT Gateways, push egress through CloudFront.

### 4.4 NAT Gateway
- ~$32/mo per gateway + **$0.045/GB processed**. A chatty service can rack up TBs.
- Alternatives: VPC endpoints (free for S3/DDB), interface endpoints (cheaper than NAT for high traffic), a single NAT instance for non-prod.

### 4.5 RDS / Aurora
- **Stop/start non-prod DBs** outside business hours. RDS supports stop-up-to-7-days; Aurora supports cluster stop.
- **Aurora I/O-Optimized** for IO-heavy workloads (pays off ~25% IO of bill).
- **Right-size with Performance Insights** + sizing tools.
- **Backups beyond retention** bill until deleted manually.

### 4.6 S3
- **Intelligent-Tiering** if access is unknown.
- **Lifecycle rules** for IA/Glacier transitions and noncurrent version expiration.
- **Multipart upload abort lifecycle.** Required.
- **Replication** doubles cost for replicated objects.
- **Request costs at scale.** Millions of GET/HEAD/PUT add up; batch where possible.

### 4.7 CloudWatch
- **Log ingestion**: $0.50/GB. Most teams accidentally ship a lot of logs.
- **Log storage**: $0.03/GB-mo. Forever-retention buckets.
- **Custom metrics**: $0.30/metric/month. High cardinality is the bomb.
- **Logs Insights queries**: $0.005/GB scanned.
- Move logs to S3 with a Subscription Filter + Firehose if you need long retention cheaply.

### 4.8 Lambda
- Pay per ms × MB. Right-size memory — sometimes more memory = less duration = cheaper.
- **Provisioned concurrency** bills idle. Use only on latency-sensitive paths.
- **Spaghetti recursion** (Lambda triggers itself via S3/SNS) → bill spike to 5 figures fast.

---

## 5. Cost Explorer, Budgets, Anomaly Detection

### Cost Explorer
The interactive bill explorer. Group by service, linked account, tag, instance type, usage type. Forecast next month. **Tag everything** with at least `project`, `env`, `owner` — without tags, Cost Explorer is just service-level summaries.

### Cost Allocation Tags
You must **activate** tags as cost allocation tags before they show up in reports. Billing → Cost allocation tags → check the ones you care about.

### AWS Budgets
Set $ thresholds with alerts. Beyond cost, you can budget:
- RI/SP utilization.
- Coverage of usage by SPs/RIs.
- Reservation expiration alerts.

### Cost Anomaly Detection
ML-driven; alerts on unusual spend spikes per service or per linked account. Free. **Always enable.**

```bash
aws ce create-anomaly-monitor --anomaly-monitor '{
  "MonitorName": "all-services", "MonitorType": "DIMENSIONAL", "MonitorDimension": "SERVICE"
}'
```

### CUR — Cost and Usage Reports
The hourly detailed CSV/Parquet drop into S3. Query with Athena/QuickSight for any custom analysis. The source of truth for FinOps.

---

## 6. Practical: a monthly waste hunt (30 minutes)

```bash
# 1. Cost by service, last 30 days
aws ce get-cost-and-usage \
  --time-period Start=$(date -u -d "30 days ago" +%Y-%m-%d),End=$(date -u +%Y-%m-%d) \
  --granularity DAILY --metrics UnblendedCost \
  --group-by Type=DIMENSION,Key=SERVICE

# 2. Unattached EBS volumes
aws ec2 describe-volumes --filters Name=status,Values=available \
  --query 'Volumes[?Size>`0`].[VolumeId,Size,CreateTime,AvailabilityZone]' --output table

# 3. Idle Elastic IPs (only billed when not attached)
aws ec2 describe-addresses --query 'Addresses[?InstanceId==null].[AllocationId,PublicIp]' --output table

# 4. Stopped EC2 instances (still paying for EBS)
aws ec2 describe-instances --filters Name=instance-state-name,Values=stopped \
  --query 'Reservations[].Instances[].[InstanceId,InstanceType,LaunchTime]' --output table

# 5. Old EBS snapshots
aws ec2 describe-snapshots --owner-ids self \
  --query 'sort_by(Snapshots, &StartTime)[:20].[SnapshotId,VolumeSize,StartTime]' --output table

# 6. Long-retention log groups
aws logs describe-log-groups --query 'logGroups[?retentionInDays==`null` || retentionInDays>`30`].[logGroupName,retentionInDays,storedBytes]' --output table

# 7. Underutilized RDS (use Performance Insights / Compute Optimizer)
aws compute-optimizer get-rds-database-recommendations

# 8. NAT Gateway data processed
aws cloudwatch get-metric-statistics --namespace AWS/NATGateway --metric-name BytesOutToDestination \
  --start-time $(date -u -d "30 days ago" +%Y-%m-%dT00:00:00Z) --end-time $(date -u +%Y-%m-%dT00:00:00Z) \
  --period 86400 --statistics Sum
```

Plug into Cost Explorer for trend visualization. Make this a monthly ritual.

---

## 7. Architectural cost levers

### Serverless vs always-on
- Lambda at low utilization ≈ very cheap.
- Lambda at sustained high utilization can cost more than an EC2 / Fargate fleet.
- Crossover is typically a few sustained instances' worth of compute.

### Multi-region for HA — choose carefully
- Data transfer across regions = $0.02/GB.
- Aurora Global / DynamoDB Global Tables add replication cost.
- For most apps, multi-AZ (in-region) HA is enough; reserve multi-region for high-stakes systems.

### Throughput vs latency
- Aurora I/O-Optimized vs Standard: pay-per-IO vs flat-rate IO.
- DynamoDB On-Demand vs Provisioned: predictable workloads save 70% on Provisioned + Auto Scaling.
- CloudFront vs direct S3: CloudFront's egress is cheaper at scale + free between AWS+CF.

### Egress to CDN
A site serving 100 TB/month directly from S3: ~$8500. Same via CloudFront with cache hit ratio of 90%: ~$2000. CDN often pays for itself purely on egress.

---

## 8. Common Mistakes & Gotchas

- **No budget alerts.** A runaway loop spends $20k in a weekend. Budgets are free and trivial.
- **No tags / cost allocation.** You can't manage what you can't measure.
- **Buying 3-year SPs early.** Bigger discount, but locks in usage that may change. Start with 1-year SPs.
- **Buying SPs while still on `m4`/`gp2`.** You buy SP, then migrate to `m7g`/`gp3`, and your SP utilization tanks. Migrate first, then commit.
- **Buying SPs in low-egress region while running prod in high-egress region.** SPs apply by region (Compute SP is cross-region, but EC2 SP isn't).
- **NAT Gateway forgotten in dev VPCs.** $32/mo × N stacks.
- **Cross-AZ chatter.** Microservices in different AZs talking constantly. Either co-locate, or accept inter-AZ as the HA cost.
- **CloudWatch Logs forever retention.** Set every group.
- **Custom metrics with high-cardinality dimensions.** $$$ surprise.
- **S3 lifecycle to Glacier for tiny files** — transition cost exceeds savings.
- **EBS snapshots without lifecycle.** Cumulative storage.
- **Idle ALBs / NLBs.** Hourly + LCU charges; one per dev environment adds up.
- **Public IPs charging since 2024.** $0.005/hr per IPv4 address — IPv6 doesn't have this. Audit and release unused EIPs.
- **CloudFront price class wrong for audience.** Price-class 100 + Asian audience = high latency; Price-class All + only North America = wasted spend.
- **Compute Optimizer ignored.** Free, actionable, automatic — yet many teams never check.
- **Data transfer out of region for analytics.** Use S3 cross-region replication selectively; or push compute to data.

---

## 🎯 Key Takeaways

- **Tag everything from day one, activate cost allocation tags, and review Cost Explorer monthly.** Without tags, FinOps is just guessing.
- **Compute Savings Plans cover 60-80% of steady-state compute** at a 30-40% discount — and they're flexible across EC2, Fargate, and Lambda. Right-size before committing.
- **Data transfer is the silent budget eater.** NAT Gateway charges, cross-AZ chatter, and S3 → internet egress (vs CloudFront) often dwarf compute. Enable VPC endpoints, put CDN in front, audit NAT processing.
- **Lifecycle rules everywhere**: S3 (transition + noncurrent expiration + multipart abort), EBS snapshots (DLM), CloudWatch Logs retention. These are the "set it and forget it" wins.
- **Cost Anomaly Detection + AWS Budgets + monthly waste hunt** is the minimum FinOps discipline. It costs nothing and catches 90% of surprises before they become embarrassments.

*← [prev](./16_cicd_on_aws.md) | [next →](./18_well_architected_production.md)*
