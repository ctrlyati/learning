# 10 — Service Bus, Event Grid, Event Hubs

> **Goal:** Pick the right Azure messaging service for any pattern (command, event, stream) and know when each one shines versus when it traps you.

## 1. The three services in one sentence each

- **Service Bus** — enterprise message broker. Queues and topics. At-least-once or exactly-once with sessions/dedup. Pub-sub with subscriptions and filters. For **commands** and important **business events**.
- **Event Grid** — pub-sub for **discrete events**. Push-based delivery to webhooks/functions/storage queues. For **system events** (blob created, RG updated) and lightweight custom events.
- **Event Hubs** — high-throughput **event stream** (Kafka-like). Partitioned, pull-based via consumer groups, retains data for replay. For **telemetry**, **logs**, **clickstream**, **IoT**.

A simple memory hook:

```
Service Bus  =  one consumer per message, ordered, transactional, slow
Event Grid   =  many subscribers per event, push, fan-out, simple
Event Hubs   =  millions of events/sec, replayable, partitioned, stream
```

If you're not sure: **Service Bus is almost always the right default** for inter-service communication in a business app.

## 2. Service Bus — the workhorse

### Tiers

- **Basic** — queues only. Avoid.
- **Standard** — queues + topics. Pay-per-operation. Fine for low volume.
- **Premium** — dedicated capacity (Messaging Units, MUs). Predictable latency. Required for VNet/Private Endpoint, large messages (100 MB), and partitioning. Production default.

### Queues vs Topics

- **Queue** — one sender, one consumer per message. Competing consumers pattern.
- **Topic + Subscriptions** — one sender, many subscribers each with their own filtered view of the topic.

```bash
NS=sb-orders-prod
RG=rg-messaging-prod

az servicebus namespace create -g $RG -n $NS --location eastus2 \
  --sku Premium --capacity 1 --zone-redundant true \
  --disable-local-auth true

az servicebus queue create -g $RG --namespace-name $NS -n orders-incoming \
  --max-size 5120 --enable-dead-lettering-on-message-expiration true \
  --max-delivery-count 10 --lock-duration PT5M

az servicebus topic create -g $RG --namespace-name $NS -n order-events
az servicebus topic subscription create -g $RG --namespace-name $NS \
  --topic-name order-events -n shipping --max-delivery-count 10
az servicebus topic subscription rule create -g $RG --namespace-name $NS \
  --topic-name order-events --subscription-name shipping --name only-paid \
  --filter-sql-expression "eventType = 'OrderPaid'"
```

### Key Service Bus mechanics

- **Lock duration** — when a consumer receives a message in Peek-Lock mode, it gets a lock (default 30s, max 5 min). The consumer must `Complete` (success) or `Abandon` (retry) before lock expires, or the message is redelivered.
- **Max delivery count** — after N failures the message goes to the **dead-letter queue (DLQ)**. Inspect, fix, requeue.
- **Sessions** — partitioning by session ID for ordering. Only one consumer at a time per session.
- **Duplicate detection** — Premium-only. Sender provides `MessageId`; broker dedupes within a window.
- **Scheduled messages** — defer delivery to a future time.
- **Transactions** — receive-and-send within a single namespace atomic.

### Receiving with Entra MI

```python
from azure.servicebus.aio import ServiceBusClient
from azure.identity.aio import DefaultAzureCredential

cred = DefaultAzureCredential()
async with ServiceBusClient("sb-orders-prod.servicebus.windows.net", cred) as client:
    receiver = client.get_queue_receiver("orders-incoming")
    async with receiver:
        async for msg in receiver:
            try:
                await process(msg)
                await receiver.complete_message(msg)
            except TransientError:
                await receiver.abandon_message(msg)   # retry
            except PoisonError:
                await receiver.dead_letter_message(msg, reason="poison")
```

Grant the consumer MI `Azure Service Bus Data Receiver` (scope: queue). The sender gets `Azure Service Bus Data Sender`. *Never* use connection strings or SAS rules in production.

## 3. Event Grid and Event Hubs — variations

### Event Grid

Push-based pub-sub for discrete events. Events are CloudEvents 1.0 or Event Grid schema JSON, typically <64 KB. Subscriptions deliver to:

- Azure Functions (Event Grid trigger).
- Webhooks (HTTPS endpoints).
- Storage Queues / Service Bus queues/topics.
- Event Hubs.
- Hybrid Connections / Relay.

Two product flavors:

- **Event Grid Basic** — the classic. System topics (Azure services emit events) and custom topics. Push-only, at-least-once.
- **Event Grid Namespaces** — newer (2024+). MQTT broker support, namespace-scoped topics, pull delivery, larger payloads (1 MB).

Subscribe a Function to blob-created events:

```bash
SA_ID=$(az storage account show -n stuploadsprod -g rg-data -o tsv --query id)
FN_RG=rg-orders-fn-prod
FN_NAME=fn-orders-prod
FN_RESOURCE_ID="/subscriptions/<sub>/resourceGroups/$FN_RG/providers/Microsoft.Web/sites/$FN_NAME/functions/handle_upload"

az eventgrid event-subscription create \
  --name upload-handler \
  --source-resource-id $SA_ID \
  --endpoint $FN_RESOURCE_ID \
  --endpoint-type azurefunction \
  --included-event-types Microsoft.Storage.BlobCreated \
  --subject-begins-with "/blobServices/default/containers/uploads/blobs/"
```

Use Event Grid when:

- You need to react to Azure platform events (RG deleted, blob uploaded, secret rotated).
- You have a small number of subscribers per event type and at-least-once is fine.
- You need fan-out without buying into Kafka.

Don't use it as a general inter-service message bus — Service Bus is better.

### Event Hubs

Kafka-compatible (since the introduction of the Kafka surface). Designed for **millions of events per second** with **retention** for replay.

- **Namespace** → **Event Hub (topic)** → **Partitions**.
- Each partition is an ordered, append-only log. Consumers track their own offset per **consumer group**.
- Retention 1-90 days (Premium), 1-7 days (Standard).
- Capture: auto-archive every batch to Blob/ADLS Gen2.

```bash
EHNS=ehns-telemetry-prod
az eventhubs namespace create -g $RG -n $EHNS \
  --location eastus2 --sku Premium --capacity 1 --zone-redundant true \
  --disable-local-auth true

az eventhubs eventhub create -g $RG --namespace-name $EHNS -n device-telemetry \
  --partition-count 16 --retention-time-in-hours 168 \
  --cleanup-policy Delete
```

Two consumer patterns:

- **High-level (EventProcessorClient)** — checkpoints per partition to a storage container, balances partitions across instances. The way to consume.
- **Low-level (per-partition receiver)** — full control. Rare; only for specialty.

Use Event Hubs when:

- Volume is >1k events/sec sustained, or you need durable replay.
- You're emitting telemetry / logs / IoT data.
- You want stream-processing in Stream Analytics, Spark, or Flink downstream.

### MQTT and IoT — quick context

For real device-to-cloud messaging, **Event Grid Namespaces** speaks MQTT v3.1.1/v5 natively, and **IoT Hub** wraps Event Hubs + device identity + device twins for the full IoT stack. Outside the scope of this course; know they exist.

## 4. Practical Application — picking patterns by example

### Pattern 1: Order placement → process → ship

- HTTP request → API writes order to SQL, posts `OrderPlaced` *event* to **Service Bus topic** `order-events`.
- Subscription `shipping` (filter `eventType = 'OrderPaid'`) → shipping worker.
- Subscription `notifications` → email sender.
- Subscription `analytics-export` → archives to blob.

Why Service Bus: ordered per-order, retriable, DLQ for bad messages, no replay required.

### Pattern 2: Telemetry from 10,000 devices

- Devices → IoT Hub → Event Hub Capture → ADLS Gen2.
- Stream Analytics job over the Event Hub → real-time aggregates → Power BI.
- Anomaly detection job → Event Grid event → Service Bus → ops queue.

Why Event Hubs: volume + durable replay.

### Pattern 3: New blob in `uploads/` → virus scan + thumbnail + index

- Blob upload → Event Grid system topic on storage account.
- Three subscriptions: Defender scan function, thumbnail function, search-indexer function.

Why Event Grid: small volume, fan-out, push delivery, native blob events.

### Pattern 4: Internal "audit log" of every business action

- Every service emits to **Event Hubs** (`namespace=audit, hub=actions`).
- Capture to ADLS for long-term retention; Stream Analytics for live dashboards; Service Bus for ops-relevant subsets.

## 5. Common Mistakes & Gotchas

- **Using Service Bus for high-volume telemetry.** Service Bus is for *messages*, not *events at scale*. At 10k/s you'll bankrupt yourself (and hit per-namespace limits). Use Event Hubs.
- **Using Event Hubs for inter-service commands.** No competing consumers per message — each consumer group sees every event. If you need "exactly one shipping service handles this order," you need Service Bus.
- **Using Event Grid as a queue.** It's not. There's no peek-lock, no DLQ semantics until 2024-era Namespaces, no ordering. At-least-once push delivery, retries based on subscriber 5xx — that's it.
- **Lock duration too short.** Worker takes 90 seconds to process; lock expires at 30s; same message redelivered to another worker; double work. Set lock duration > P99 processing time. Use `RenewLock` for long jobs.
- **Forgetting DLQ monitoring.** A queue silently filling its DLQ is invisible until a bug report says "shipping never happens." Alert on `DeadLetteredMessages > 0`.
- **Sender that doesn't propagate trace context.** Distributed tracing across messaging requires `Diagnostic-Id` / W3C TraceContext propagation. Application Insights does this for the .NET SDK; verify for Python/Node.
- **Service Bus connection strings in app settings.** Use MI + RBAC. `disableLocalAuth: true` on the namespace.
- **Event Hubs partition count cannot decrease.** Pick carefully; 4 or 8 partitions is a safe start. Adding is allowed (Premium), removing is not.
- **Consumer scaling > partition count.** No benefit. One consumer per partition is the ceiling on parallelism in Event Hubs.
- **Stream Analytics or Function not checkpointing.** Restart re-reads days of data. Always store checkpoints; for Functions, the runtime handles this via the `eventHubTrigger` extension config — verify it's actually committing.
- **Service Bus message size 256 KB (Standard) / 100 MB (Premium with `enablePartitioning=true`).** Anything bigger → store payload in blob, send a reference message ("claim-check" pattern).
- **Event Grid subscriber 5xx loop.** Event Grid retries with exponential backoff for 24h then dead-letters (if configured). A persistently-broken endpoint = events lost forever unless dead-lettering is on.
- **Premium MU planning.** Each MU is ~1k msg/sec. Buy too few = throttling; too many = wasted spend. Use 1 MU baseline + autoscale (Premium SKU supports it).

## 🎯 Key Takeaways

- **Three services, three patterns.** Memorize: Service Bus = messages (commands, business events), Event Grid = small system events with fan-out, Event Hubs = high-volume durable streams.
- **Default to Service Bus for inter-service comms.** Reach for Event Grid only for "react to system events," Event Hubs for telemetry/logs/streams.
- **MI + RBAC everywhere.** Disable local auth, grant `Data Sender` / `Data Receiver` at the queue/topic/hub scope.
- **DLQ + monitoring + lock duration sized to P99.** The three knobs that determine reliability in Service Bus.
- **Choose Event Hubs partition count carefully** — you can grow but not shrink. Plan with throughput targets in mind.

*← [prev](./09_app_service_container_apps.md) | [next → 11_edge_and_traffic.md](./11_edge_and_traffic.md)*
