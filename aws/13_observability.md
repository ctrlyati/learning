# 13 — Observability: CloudWatch, X-Ray, CloudTrail

> **Goal:** See what's happening in production — metrics, logs, traces, audit — at a cost you control, with alarms that fire when they matter.

---

## 1. The three pillars (plus audit)

**Mental model:** Observability = your ability to answer "what's the system doing right now" and "what happened just before that failure". Four data streams:
- **Metrics** (CloudWatch Metrics): numeric time series. Fast, cheap, summary view.
- **Logs** (CloudWatch Logs): text events. Rich, expensive, detail view.
- **Traces** (X-Ray): per-request flow across services. Causality view.
- **Audit** (CloudTrail): API calls in the account. Who did what, when.

You need all four. Beginners overweight logs; pros lean on metrics + alarms first, traces for hard cases, logs only for forensic dives.

---

## 2. CloudWatch Metrics

### What's already there
Every AWS service publishes default metrics. Lambda → Invocations/Errors/Duration. EC2 → CPUUtilization. ALB → RequestCount/TargetResponseTime/HTTPCode_Target_5XX_Count. RDS → CPU, FreeableMemory, ReadIOPS.

### Custom metrics
Two methods:
- **API**: `PutMetricData` (slow, $$, 1-min granularity by default, 1-sec for high-res).
- **Embedded Metric Format (EMF)**: write a JSON log line to stdout from your container/Lambda; CloudWatch parses it and extracts metrics — **much cheaper** at scale.

```python
# EMF: emit "OrdersCreated" metric and "tier" dimension
import json, time
print(json.dumps({
    "_aws": {
        "Timestamp": int(time.time() * 1000),
        "CloudWatchMetrics": [{
            "Namespace": "MyApp",
            "Dimensions": [["tier"]],
            "Metrics": [{"Name": "OrdersCreated", "Unit": "Count"}],
        }],
    },
    "tier": "premium",
    "OrdersCreated": 1,
}))
```

### Pricing surprise
Custom metrics are **$0.30 per metric per month** (first 10000, then tiered down). A single high-cardinality dimension (e.g., `customer_id` with 50000 unique values) = $15000/month. **Never use unbounded cardinality in dimensions.**

### Alarms
The output of metrics. Configure thresholds, evaluation periods, datapoints-to-alarm.

```bash
aws cloudwatch put-metric-alarm \
  --alarm-name api-5xx-spike \
  --metric-name HTTPCode_Target_5XX_Count --namespace AWS/ApplicationELB \
  --dimensions Name=LoadBalancer,Value=app/myapp/abc \
  --statistic Sum --period 60 --evaluation-periods 5 --datapoints-to-alarm 3 \
  --threshold 10 --comparison-operator GreaterThanThreshold \
  --treat-missing-data notBreaching \
  --alarm-actions arn:aws:sns:...:pager
```

**Composite alarms** combine multiple sub-alarms with AND/OR logic, cutting noise.

### Anomaly Detection
ML-based bands instead of fixed thresholds. Great for cyclical metrics (traffic by hour of day).

### Metrics Insights
SQL-like queries over metrics. Useful for ad-hoc:
```sql
SELECT AVG(Duration) FROM "AWS/Lambda" WHERE FunctionName = 'myapp' GROUP BY Resource
```

---

## 3. CloudWatch Logs

### Structure
- **Log Group**: a container, e.g., `/aws/lambda/myapp`. Set retention here.
- **Log Stream**: usually one per container instance / Lambda execution environment.
- **Log Event**: a single line with timestamp and message.

### Ingestion
- AWS services (Lambda, ECS with awslogs, RDS slow log, VPC Flow Logs, CloudTrail) push automatically.
- **CloudWatch Agent** on EC2 ships system + app logs.
- **Firelens** (FluentBit/Fluentd) for ECS — more control, multiple destinations.

### Retention — set it
Default = forever. Common policy: 14-90 days for app logs, 365+ for audit. Cost: $0.03/GB ingested + $0.03/GB stored/mo (us-east-1) — log volume bills add up fast.

```bash
aws logs put-retention-policy --log-group-name /aws/lambda/myapp --retention-in-days 14
```

### Logs Insights — the query language
SQL-like for logs. Indispensable.

```sql
fields @timestamp, @message
| filter level = "ERROR" and customerId = "c-1"
| sort @timestamp desc
| limit 100
```

Common queries:
```sql
-- Top latency outliers
stats max(latency_ms) by route | sort by max desc | limit 10

-- Error rate by service
filter level = "ERROR" | stats count() by service

-- Slowest Lambda invocations
filter @type = "REPORT" | sort @duration desc | limit 20
```

### Structured logging
Always log JSON in production. Logs Insights parses fields automatically:
```python
logger.info(json.dumps({
    "event": "order_created", "orderId": "o-1", "customerId": "c-1",
    "tier": "premium", "amount": 49.95
}))
```

### Subscriptions
Stream logs to Lambda / Kinesis / Firehose / OpenSearch in real time.

### Log Anomaly Detection (newer)
ML over log volume + content to flag unusual patterns.

---

## 4. X-Ray — distributed tracing

A trace = the full chain of calls for one request (across Lambda → DynamoDB → SNS → Lambda → S3). Each segment = a service's work; sub-segments are child calls.

### Instrumentation
- **Auto-instrumentation**: enable on Lambda (`tracing: ACTIVE`), API Gateway, App Mesh, etc.
- **SDK**: call `aws-xray-sdk` from code to add sub-segments and metadata.

```python
from aws_xray_sdk.core import patch_all, xray_recorder
patch_all()  # auto-instruments boto3, requests, httpx, psycopg2, etc.

@xray_recorder.capture("compute_total")
def compute_total(items):
    ...
```

### Service Map
The auto-built diagram of services + edges + error rates + latencies. Spot bottlenecks visually.

### Pricing
$5 per million traces recorded. Sampling reduces cost: by default, 1 trace/s + 5% of additional requests.

### OpenTelemetry — the future
**AWS Distro for OpenTelemetry (ADOT)** is becoming the recommended path. Vendor-neutral, supports X-Ray + your other backends (Honeycomb, Datadog, Jaeger). For greenfield, prefer ADOT.

---

## 5. CloudTrail — every API call, audited

CloudTrail records every API call in the account: who, what, when, from where, with what params, returning what.

### Two trail types
- **Management events** (free, 90-day retention in CloudTrail Events history). Console logins, IAM changes, instance launches.
- **Data events** ($, opt-in). S3 object-level operations, Lambda invocations, DynamoDB item access. Huge volume — only enable for sensitive resources.

### Setting up a trail
```bash
aws cloudtrail create-trail --name org-trail --s3-bucket-name audit-logs \
  --is-multi-region-trail --enable-log-file-validation
aws cloudtrail start-logging --name org-trail
```

**Always enable a multi-region trail to S3** in every account. Add log file validation (`enable-log-file-validation`) so tampering is detectable.

### CloudTrail Lake
A SQL-queryable data store for trails. Replaces the Athena-on-S3 pattern. ~$2.50/GB ingested.

```sql
SELECT eventTime, userIdentity.arn, eventName, requestParameters
FROM <event-data-store-id>
WHERE eventName = 'DeleteBucket' AND eventTime > timestamp('2026-05-01')
ORDER BY eventTime DESC
```

### CloudTrail Insights
ML-detected unusual API activity (e.g., spike in `TerminateInstances`). Costs extra.

---

## 6. Practical: a baseline observability setup

```typescript
// CDK snippets for "every service should have"

// 1. Log retention on every log group
new logs.LogGroup(this, "AppLogs", {
  logGroupName: "/myapp/app",
  retention: logs.RetentionDays.TWO_WEEKS,
});

// 2. Service-level SLO alarm (5xx error rate)
const errorRate = new cw.MathExpression({
  expression: "(e/r)*100",
  usingMetrics: {
    e: alb.metricHttpCodeTarget(elbv2.HttpCodeTarget.TARGET_5XX_COUNT),
    r: alb.metricRequestCount(),
  },
  period: cdk.Duration.minutes(1),
});
new cw.Alarm(this, "ErrorRateAlarm", {
  metric: errorRate,
  threshold: 1, // 1% error rate
  evaluationPeriods: 5, datapointsToAlarm: 3,
  alarmAction: new actions.SnsAction(pagerTopic),
});

// 3. X-Ray sampling rule for important traces
new xray.CfnSamplingRule(this, "ImportantSampling", {
  samplingRule: {
    priority: 100, fixedRate: 1, reservoirSize: 5,
    serviceName: "orders", urlPath: "/api/checkout*", host: "*", httpMethod: "*",
    serviceType: "*", resourceArn: "*", ruleName: "checkout-all"
  }
});

// 4. CloudTrail to S3
new cloudtrail.Trail(this, "AuditTrail", {
  bucket: auditBucket, isMultiRegionTrail: true, includeGlobalServiceEvents: true,
  enableFileValidation: true,
  managementEvents: cloudtrail.ReadWriteType.ALL,
});
```

---

## 7. Dashboards

CloudWatch Dashboards: $3/dashboard/month. Build one per service. Include:
- Request rate, error rate, p99 latency (RED method).
- Saturation: CPU, memory, queue depth, DB connections.
- Business KPIs: orders/minute, signups/hour.

**Grafana** managed service (AMG) is the upgrade — supports multiple CloudWatch accounts, Prometheus, X-Ray, OpenSearch as data sources. Better for serious dashboarding.

---

## 8. The SLO mindset

The Google SRE book popularized **SLI / SLO / error budgets**. AWS supports this via:
- **CloudWatch Metric Math** for SLI calculation (e.g., good requests / total requests).
- **AWS Application Signals** (newer) — managed SLO tracking on top of CloudWatch + X-Ray + ADOT, with auto-discovered services and golden signals.

```typescript
// Application Signals — auto-instruments Lambda/ECS/EC2 with ADOT and tracks SLOs
new applicationsignals.CfnServiceLevelObjective(this, "CheckoutSLO", {
  name: "checkout-availability",
  goal: {
    attainmentGoal: 99.9,
    intervalInDays: 30,
  },
  sli: { /* RequestCount with statusCode < 500 */ },
});
```

---

## 9. Common Mistakes & Gotchas

- **No log retention.** Logs forever = bill forever. Set every group, day one.
- **High-cardinality custom metric dimensions** — instant $$$. Dimensions are for aggregation, not identification.
- **Alarming on a single 1-minute datapoint.** Flappy alarms = ignored alarms. Use `evaluation-periods` + `datapoints-to-alarm`.
- **`TreatMissingData: missing`** = alarm goes INSUFFICIENT instead of OK or ALARM, doesn't notify. Pick `notBreaching` or `breaching` deliberately.
- **CPU% as a primary SLO.** Customers don't care. Track error rate + latency.
- **Alarms without runbooks.** Pager fires at 3 AM; on-call has no idea what to do. Every alarm → runbook link in the description.
- **CloudTrail data events on every S3 bucket.** Enormous bill. Scope to sensitive buckets/objects.
- **X-Ray not enabled on Lambda.** One toggle, huge debugging value. Free for the first 100k traces/month.
- **Logging PII in plain text.** GDPR/HIPAA disaster. Centralize logging through a redaction layer (FluentBit filters, Lambda extensions).
- **`console.log` from every request line.** Logs Insights bills by GB scanned. Structured logs + filters > unstructured spam.
- **Forgetting Logs Insights query cost.** $0.005 per GB scanned. Frequent broad queries add up.
- **Custom metric for every API call** when EMF would work. EMF is much cheaper at volume.
- **Container Insights / Lambda Insights not enabled.** They cost something but transform debugability.
- **Single-region trail.** Misses events in other regions (some services are global). Multi-region trail is the answer.
- **CloudWatch alarms in the wrong region.** Billing in `us-east-1`; resource alarms in resource's region.

---

## 🎯 Key Takeaways

- **Metrics first, traces second, logs third.** Alerting and SLOs live in metrics; X-Ray finds bottlenecks; logs are forensic. Most teams over-invest in logs.
- **Embedded Metric Format (EMF)** is the cost-conscious way to emit custom metrics at scale. Plain `PutMetricData` is expensive past a few hundred metric series.
- **Cardinality discipline.** Dimensions are aggregation keys — `route`, `method`, `region` are fine; `customerId`, `requestId` are bombs.
- **CloudTrail multi-region + log file validation + S3 with Object Lock**: the audit baseline every account should have, often required for compliance.
- **Set log retention on every log group, set alarms with runbooks, dashboard the golden signals.** These three habits separate teams that operate well from teams that fight fires.

*← [prev](./12_ecs_ecr_fargate.md) | [next →](./14_security_compliance.md)*
