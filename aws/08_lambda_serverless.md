# 08 — Lambda & Serverless

> **Goal:** Build, deploy, and operate Lambda functions in production — including the cold-start mitigations, concurrency controls, and event-source patterns that separate hobby from real.

---

## 1. Lambda — functions, events, isolation

**Mental model:** You upload code. AWS holds it in S3, references a runtime (Node, Python, Java, Go, .NET, Ruby, custom), and when something *invokes* it, AWS spins up a Firecracker microVM ("execution environment"), boots the runtime, loads your code, calls your handler with the event, returns the result, and may keep the microVM warm to reuse.

You pay per invocation × per GB-second of execution. **128 MB minimum, 10 GB max memory.** CPU scales linearly with memory.

### Hello world
```bash
# Python 3.12 handler
mkdir hello && cd hello
cat > index.py <<'EOF'
def handler(event, context):
    return {"statusCode": 200, "body": f"hello {event.get('name','world')}"}
EOF
zip -r function.zip .

# Create role for Lambda execution
ROLE=$(aws iam create-role --role-name hello-lambda-role \
  --assume-role-policy-document '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}' \
  --query Role.Arn --output text)
aws iam attach-role-policy --role-name hello-lambda-role \
  --policy-arn arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole

aws lambda create-function --function-name hello \
  --runtime python3.12 --role $ROLE --handler index.handler \
  --zip-file fileb://function.zip --architectures arm64

aws lambda invoke --function-name hello --payload '{"name":"yati"}' /dev/stdout
```

`arm64` (Graviton) = ~20% cheaper at same speed. Default to it.

---

## 2. Invocation Models

| Type | Examples | Concurrency | Errors |
|---|---|---|---|
| **Synchronous** | API Gateway, ALB, SDK invoke, Cognito triggers | 1 invocation = 1 client wait | Returned to caller |
| **Asynchronous** | S3 events, EventBridge, SNS, manual `--invocation-type Event` | Lambda owns retries | Up to 2 retries, then DLQ/destination |
| **Stream-based (poller)** | Kinesis, DynamoDB Streams, Kafka, SQS | Lambda polls the source | Stays at front of stream until success / DLQ |

Each model has different retry, ordering, and concurrency semantics — failing to know which model you're using is a top source of confusion.

### Async destinations (modern alternative to DLQ)
```bash
aws lambda put-function-event-invoke-config --function-name myFn \
  --destination-config '{
    "OnSuccess":{"Destination":"arn:aws:sqs:...:success-queue"},
    "OnFailure":{"Destination":"arn:aws:sqs:...:dlq"}
  }'
```

---

## 3. Limits, Memory, and Concurrency

### Per-function limits
- **Memory:** 128 MB – 10240 MB (in 1 MB increments).
- **Timeout:** up to 15 minutes (sync) or 6 hours (async event lifetime including retries).
- **Code (zipped):** 50 MB direct upload, 250 MB unzipped, or 10 GB container image.
- **`/tmp`:** 512 MB default, configurable up to 10 GB.
- **Payload:** 6 MB sync, 256 KB async event.

### Account concurrency
- **Default account quota:** 1000 concurrent executions across all Lambdas in a region. Increase via Service Quotas — no big deal but takes a ticket.
- **Reserved concurrency** (per function): guarantees `n` slots for this function, and caps it at `n`.
- **Provisioned concurrency** (per function alias): pre-warmed microVMs, no cold starts. Bills $$$ per GB-second even when idle.

### Cold starts
First invocation in a fresh microVM has overhead: microVM init + runtime init + your init code (imports, SDK clients, DB connections). Typically 100ms-2s; Java/.NET can be 5-10s without tuning.

**Mitigations:**
- Use `arm64` and provisioned concurrency for latency-critical APIs.
- **SnapStart** (Java, Python, .NET): snapshots the init phase; restores in <1s. Free.
- Keep packages small (no `import boto3` you don't use, no 200MB ML library if you use 1MB of it).
- Initialize SDK clients and DB pools **outside the handler** so they reuse across warm invocations:

```python
import boto3
ddb = boto3.client("dynamodb")  # initialized once per microVM

def handler(event, context):
    ddb.get_item(...)
```

---

## 4. Code Packaging

### Zip
Simplest. Upload .zip or point to S3.

### Container image
Up to 10 GB. Useful for big ML models, custom runtimes, or build parity with ECS/EKS.

```dockerfile
FROM public.ecr.aws/lambda/python:3.12
COPY app/ ${LAMBDA_TASK_ROOT}/
RUN pip install -r requirements.txt -t ${LAMBDA_TASK_ROOT}
CMD ["index.handler"]
```

### Layers
Shared dependencies attached to multiple functions. Up to 5 layers per function, 250 MB total unzipped. Common pattern: one layer with shared SDKs / utility code.

---

## 5. Event Sources — the integration toolbox

### S3
```bash
aws s3api put-bucket-notification-configuration --bucket my-bucket \
  --notification-configuration '{
    "LambdaFunctionConfigurations": [{
      "LambdaFunctionArn":"arn:aws:lambda:...:function:processor",
      "Events":["s3:ObjectCreated:*"],
      "Filter":{"Key":{"FilterRules":[{"Name":"prefix","Value":"incoming/"}]}}
    }]
  }'
```

### EventBridge — the bus
Match events by JSON pattern, route to Lambda. Module 10 covers in depth.

### SQS
Lambda polls; batches up to 10000 messages (depending on size). On failure, the batch retries. **Use partial batch failure response** (`batchItemFailures`) to avoid retrying succeeded items.

### Kinesis / DynamoDB Streams
Ordered per shard. Use `parallelizationFactor` to scale within a shard; use **bisect on error** to isolate poison messages.

### API Gateway / ALB / Function URL / AppSync
Sync invocations from HTTP front doors. Module 09.

---

## 6. Practical: an image-thumbnailer

```typescript
const fn = new lambda.Function(this, "Thumbnailer", {
  runtime: lambda.Runtime.PYTHON_3_12,
  architecture: lambda.Architecture.ARM_64,
  code: lambda.Code.fromAsset("src", { bundling: { /* pip install */ } }),
  handler: "index.handler",
  memorySize: 1024,
  timeout: cdk.Duration.seconds(30),
  environment: { OUT_BUCKET: outBucket.bucketName },
  tracing: lambda.Tracing.ACTIVE,
});
inBucket.grantRead(fn);
outBucket.grantPut(fn);
inBucket.addEventNotification(s3.EventType.OBJECT_CREATED, new s3n.LambdaDestination(fn),
  { prefix: "uploads/" });

new logs.LogRetention(this, "Retention", {
  logGroupName: `/aws/lambda/${fn.functionName}`,
  retention: logs.RetentionDays.TWO_WEEKS,
});
```

Note `tracing: ACTIVE` (X-Ray) and the explicit log retention — both critical and easy to forget.

---

## 7. Lambda inside a VPC

Required when your function needs to reach private resources (RDS, ElastiCache, internal ALB). The function gets ENIs in your subnets.

Modern Lambda VPC networking uses **shared ENIs** (Hyperplane) — no more cold-start penalty for VPC functions since 2019.

**Caveats:**
- No internet egress unless you give the subnet a NAT route — or use VPC endpoints for AWS services.
- ENI limits per subnet → spread across multiple subnets.
- Use **RDS Proxy** to share DB connections (Module 06).

---

## 8. Observability

- **Logs**: every invocation writes to `/aws/lambda/<function-name>`. **Always set a retention** (default is forever).
- **Metrics**: `Invocations`, `Errors`, `Throttles`, `Duration`, `ConcurrentExecutions`, `IteratorAge` (stream lag).
- **X-Ray**: distributed tracing. Enable `tracing: ACTIVE` and call `aws_xray_sdk` from code for sub-segments.
- **Lambda Insights**: enhanced container-level metrics (CPU, memory, network). A CloudWatch extension layer; enable per function.

---

## 9. Lambda URLs and Function URLs

A built-in HTTPS endpoint per function — no API Gateway needed for the simplest cases. Free.

```bash
aws lambda create-function-url-config --function-name hello --auth-type NONE
# returns https://xxx.lambda-url.ap-southeast-1.on.aws/
```

Auth types: `NONE` (open), `AWS_IAM` (signed requests only). For anything with real users, use API Gateway or CloudFront for WAF, rate limiting, custom domains.

---

## 10. Step Functions — when one Lambda isn't enough

When you have a workflow (sequence, branching, parallel, retries, human approvals), **AWS Step Functions** orchestrates Lambdas via a JSON state machine. Two types:
- **Standard**: long-running (up to 1 year), exactly-once, history-tracked, $$ per state transition.
- **Express**: high-volume, short-lived (5 min), at-least-once, $ per million transitions.

Use Step Functions instead of "Lambda calls Lambda calls Lambda" chains.

---

## 11. Common Mistakes & Gotchas

- **Init code in the handler.** Loading models, SDK clients, DB pools *per invocation* instead of at module scope. Single biggest source of slow Lambdas.
- **Log group never expires.** Pay forever. Set a retention.
- **Reserved concurrency = 0 by accident** = silent throttle to nothing. CloudWatch alarms!
- **Lambda calling Lambda synchronously.** Double charge, double timeout risk, hidden coupling. Use Step Functions or SQS.
- **Async retries surprise.** Two automatic retries with exponential backoff = 3 total executions. Idempotency required.
- **6 MB sync payload limit.** Larger requests fail. Stash in S3 and pass a key.
- **`/tmp` filled.** Lambda warm reuse means leftover files. Clean up, or set higher `/tmp`.
- **Cold start for Java/.NET ignored.** Use SnapStart or accept p99 pain.
- **Provisioned concurrency for a function that runs 50 times a day.** You're paying for warmth you don't need.
- **Forgot to grant `lambda:InvokeFunction` to the event source.** Permission required from the source side.
- **SQS event source without `batchItemFailures`** — one poison message reprocesses 9 good ones forever.
- **VPC Lambda without VPC endpoints** — outbound `secretsmanager.amazonaws.com` calls go through NAT, $$.
- **15-minute timeout limit forgotten** — a long-running job needs Step Functions, ECS, or Batch.
- **Hard-coded secrets** in env vars. Use Secrets Manager / Parameter Store with caching.
- **Unbounded recursion.** A Lambda that triggers itself via S3 / SNS / EventBridge → runaway concurrency and $$$. Set reserved concurrency as a circuit breaker.

---

## 🎯 Key Takeaways

- **Init outside the handler, reuse across warm invocations.** This single discipline eliminates 90% of Lambda performance complaints.
- **Architecture = arm64, plus SnapStart for Java/Python/.NET.** Cheaper and faster cold starts; almost no downside.
- **Match the invocation model to the workload.** Sync for APIs, async with destinations for fire-and-forget, polling for streams/queues — each has different retry/concurrency semantics.
- **Reserved concurrency is both a guarantee and a cap.** Use it as a per-function circuit breaker; without it, one runaway Lambda eats your whole account's 1000-concurrency budget.
- **Step Functions over Lambda chains.** Anything more complex than a single handler should be a state machine — visibility, retries, and timeouts come free.

*← [prev](./07_dynamodb.md) | [next →](./09_api_gateway_appsync.md)*
