# 10 — SQS, SNS, EventBridge: Messaging & Event-Driven Patterns

> **Goal:** Build event-driven systems where producers and consumers don't know about each other, failures don't cascade, and the right messaging primitive shows up in the right place.

---

## 1. The three primitives

**Mental model:**
- **SQS** — a queue. One message, one consumer (group). Pull-based. *"Job done."*
- **SNS** — a topic (pub/sub). One message, many subscribers. Push-based. *"Tell everyone."*
- **EventBridge** — a smart bus with filtering, routing, schemas, partner integrations. *"Tell whoever cares about this kind of thing."*

| | Direction | Consumer model | Filtering | Ordering | Use case |
|---|---|---|---|---|---|
| SQS Standard | 1→1 | Poll | Limited (FilterPolicy via SNS only) | None | Worker jobs, decoupling |
| SQS FIFO | 1→1 | Poll | Limited | Per message group | Strict ordering, idempotency |
| SNS | 1→N | Push | Subscription filter policies | None (FIFO variant exists) | Fan-out |
| EventBridge | M→N | Push | Rich JSON pattern | None | Cross-team events, SaaS integrations, scheduled |

---

## 2. SQS — durable queues

**Standard queue**: at-least-once delivery (so consumers must be idempotent), best-effort ordering, virtually unlimited throughput.

**FIFO queue**: exactly-once delivery, strict ordering within a message group ID, 300 TPS (3000 with batching). Names end in `.fifo`.

### Create & use
```bash
aws sqs create-queue --queue-name jobs \
  --attributes '{
    "MessageRetentionPeriod": "1209600",
    "VisibilityTimeout": "30",
    "ReceiveMessageWaitTimeSeconds": "20",
    "RedrivePolicy": "{\"deadLetterTargetArn\":\"arn:aws:sqs:...:jobs-dlq\",\"maxReceiveCount\":5}"
  }'

QURL=$(aws sqs get-queue-url --queue-name jobs --query QueueUrl --output text)
aws sqs send-message --queue-url $QURL --message-body '{"job":"resize","key":"img.jpg"}'
aws sqs receive-message --queue-url $QURL --wait-time-seconds 20 --max-number-of-messages 10
```

### Critical settings
- **Visibility timeout**: when a consumer receives a message, it's *hidden* for this duration. If the consumer doesn't delete it in time, the message reappears (assumed crashed). Tune to slightly longer than your worst-case processing time.
- **Long polling** (`WaitTimeSeconds=20`): consumer waits for messages instead of polling empty queues. **Always use.** Cuts polling costs ~95%.
- **DLQ + maxReceiveCount**: poison messages move to a dead letter queue after N failed receives. **Always set.** Otherwise a bad message loops forever.
- **MessageRetentionPeriod**: max 14 days. Defaults to 4 days.

### Batching
- `SendMessageBatch` / `ReceiveMessage` (up to 10) / `DeleteMessageBatch` — much cheaper at volume.

### Lambda + SQS pattern
```typescript
const queue = new sqs.Queue(this, "Jobs", {
  visibilityTimeout: cdk.Duration.seconds(60),
  retentionPeriod: cdk.Duration.days(14),
  deadLetterQueue: { queue: dlq, maxReceiveCount: 5 },
});
new lambda.EventSourceMapping(this, "JobsHandler", {
  target: workerFn,
  eventSourceArn: queue.queueArn,
  batchSize: 10,
  maxBatchingWindow: cdk.Duration.seconds(5),
  reportBatchItemFailures: true,
});
```

In the Lambda handler, return `batchItemFailures` listing only the failed message IDs so the good ones don't reprocess:

```python
def handler(event, context):
    failures = []
    for r in event["Records"]:
        try:
            process(r)
        except Exception:
            failures.append({"itemIdentifier": r["messageId"]})
    return {"batchItemFailures": failures}
```

Cost: **$0.40 per million** requests + free tier 1M/month. Negligible for most workloads.

---

## 3. SNS — pub/sub topics

A topic has subscribers (Lambda, SQS, HTTP/S, email, SMS, mobile push, Firehose, Kinesis Data Stream). Publish once, all subscribers receive.

```bash
TOPIC=$(aws sns create-topic --name orders --query TopicArn --output text)

# Fan-out to two queues
aws sns subscribe --topic-arn $TOPIC --protocol sqs --notification-endpoint $Q1_ARN
aws sns subscribe --topic-arn $TOPIC --protocol sqs --notification-endpoint $Q2_ARN

aws sns publish --topic-arn $TOPIC --message '{"orderId":"o-1","total":42}'
```

### Subscription filters (FilterPolicy)
Each subscriber can declare a JSON pattern; SNS only forwards matching messages. Saves Lambda invocations on uninterested subscribers.

```json
{"orderType": ["premium"], "amount": [{"numeric": [">", 100]}]}
```

### SNS FIFO
Pair with SQS FIFO for ordered fan-out.

### Mobile push
APNs, FCM, etc. Useful for mobile apps; otherwise this is a side feature.

Cost: $0.50 per million requests (most subscriptions). SMS is way more expensive — $$$.

---

## 4. EventBridge — the event bus

EventBridge is **SNS' more sophisticated cousin**: schema-aware, with richer filtering, partner SaaS integrations (Stripe, Auth0, Shopify, etc.), scheduled events, archives + replay, schema registry.

### The default bus
Every account has a `default` bus. AWS services publish events to it (EC2 state changes, S3 events via EventBridge, GuardDuty findings, ...).

### Custom buses
Create per-team or per-bounded-context buses to isolate events.

```bash
aws events create-event-bus --name orders-bus
```

### Rules: pattern + targets
```bash
aws events put-rule --name premium-order-paid \
  --event-bus-name orders-bus \
  --event-pattern '{
    "source": ["myapp.orders"],
    "detail-type": ["OrderPaid"],
    "detail": {
      "tier": ["premium"],
      "amount": [{"numeric": [">", 100]}]
    }
  }'

aws events put-targets --rule premium-order-paid --event-bus-name orders-bus \
  --targets "Id"="1","Arn"="$LAMBDA_ARN","RoleArn"="$INVOKE_ROLE"
```

### Publishing
```python
import boto3, json
eb = boto3.client("events")
eb.put_events(Entries=[{
    "EventBusName": "orders-bus",
    "Source": "myapp.orders",
    "DetailType": "OrderPaid",
    "Detail": json.dumps({"orderId":"o-1","tier":"premium","amount":150})
}])
```

### EventBridge Pipes
A point-to-point integration with optional filter and transformation. *"Take from this source, optionally transform, send to this target."* Source can be SQS, Kinesis, DynamoDB Streams, MSK; target almost anything.

### EventBridge Scheduler
Better cron-as-a-service than CloudWatch Events: per-second precision, one-off schedules, schedule groups, flexible time windows. Replace ALL EventBridge rate/cron rules with Scheduler for new projects.

```bash
aws scheduler create-schedule --name daily-report --schedule-expression "cron(0 9 * * ? *)" \
  --target '{"Arn":"'$LAMBDA_ARN'","RoleArn":"'$ROLE'"}' \
  --flexible-time-window 'Mode=OFF'
```

### Archives & Replay
Store events for X days; replay them to a rule later. **Powerful** for debugging, replay-after-fix, populating new consumers.

---

## 5. Patterns

### Decouple producer from consumer
Web service → SQS → worker. The web service returns 202 fast; the worker grinds.

### Fan-out
Producer → SNS → multiple SQS queues → multiple consumers. Each consumer team owns its own queue (controls retries, DLQ).

### Saga / choreography
Each step publishes an event; subsequent services react. Use EventBridge.

### Buffering for variable downstream
SQS in front of a flaky downstream (third-party API). Lambda processes 10 messages, retries with exponential backoff, DLQs failures.

### Idempotency
Standard SQS / SNS / EventBridge are at-least-once. **Every consumer must be idempotent.** Common technique: dedupe by a request ID in Dynamo (conditional put).

### Outbox pattern
Don't have your service both write to DB and publish to EventBridge in one transaction (impossible). Write to an `outbox` table in the same DB transaction; a separate process (Step Function / Lambda + DynamoDB Streams) reads and publishes. Guarantees consistency.

---

## 6. Practical: an order processing pipeline

```
Web API → DynamoDB (orders table) → Stream → Lambda publishes to EventBridge
                                                                │
                            ┌─────────────────────────────────┴────────────┐
                            ▼                                              ▼
                EventBridge rule: OrderCreated                EventBridge rule: OrderPaid
                  → SQS → fulfillment worker                    → SQS → invoice generator
                                                                  → SQS → analytics ingest
```

Each downstream owns its own queue + DLQ. Adding a new consumer = new rule + queue, zero changes to producers.

---

## 7. Step Functions vs EventBridge — when to use which

- **Step Functions**: orchestration of a known workflow with deterministic steps and explicit state. *"First A, then B, retry on failure, branch on result."*
- **EventBridge**: choreography of independent services reacting to events. *"Something happened; whoever cares can act."*

Real systems use both: Step Functions runs internal workflows; EventBridge propagates outcomes across team/service boundaries.

---

## 8. Common Mistakes & Gotchas

- **No DLQ.** A poison message loops forever, accruing receives. **DLQ + maxReceiveCount on every queue.**
- **Visibility timeout < processing time.** Message reappears mid-processing → double processing. Set with safety margin.
- **Short polling** (default). Burns money. Set `WaitTimeSeconds=20`.
- **Standard queue + non-idempotent consumer.** At-least-once delivery → duplicate side effects. Add dedupe.
- **`batchItemFailures` not used with SQS+Lambda.** Whole batch retries on partial failure → 9 successes reprocess.
- **SNS subscription filter policies forgotten.** Lambda invoked for every event regardless of whether it cares. $$ on busy topics.
- **EventBridge pattern uses `prefix` on numeric fields.** Patterns are string-based by default; use `numeric` matchers for ranges.
- **Cross-account events not allowed.** Default bus rejects cross-account events; you must add resource policies allowing the source account.
- **Forgot the target IAM role.** EventBridge can't invoke your target without a role granting the right permission. Confusing error.
- **FIFO throughput hit.** 300 TPS / queue (3000 with high-throughput mode). Plan message group IDs carefully — too few groups = contention, too many = no ordering.
- **EventBridge size limit: 256 KB.** Larger payloads → use S3 + event with object key.
- **SNS to Lambda with no retry config.** Failed Lambda → SNS gives up. Use SQS in between for retry control.
- **CloudWatch Events vs EventBridge confusion.** Same service, two names. Prefer the EventBridge name and API.
- **SQS message size 256 KB.** Larger via SQS Extended Client (stash in S3, reference in message).
- **Scheduler vs `rate()/cron()` rules.** EventBridge Scheduler is the new home; old rule-based schedules still work but are not the strategic direction.
- **DLQ alarms missing.** A DLQ silently filling = silent outage. CloudWatch alarm on `ApproximateNumberOfMessagesVisible > 0`.

---

## 🎯 Key Takeaways

- **Pick the primitive that matches the relationship**: SQS for one-to-one work, SNS for simple fan-out, EventBridge for typed cross-service events.
- **DLQ + maxReceiveCount + DLQ alarms is a non-negotiable baseline.** Without these, every messaging system silently fails.
- **All consumers must be idempotent.** Standard messaging is at-least-once. Build dedupe into the consumer, not the message protocol.
- **EventBridge Pipes + Scheduler are the modern replacements** for Lambda glue and rate/cron rules. Use them in new designs.
- **Event-driven architectures shine in team boundaries.** A team that publishes events doesn't need to know who consumes; consumers don't need to coordinate with each other. This is the organizational benefit, not just a technical one.

*← [prev](./09_api_gateway_appsync.md) | [next →](./11_cloudfront_route53.md)*
