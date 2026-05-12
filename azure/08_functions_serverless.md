# 08 — Azure Functions and Serverless

> **Goal:** Build, configure, and operate Azure Functions confidently — triggers and bindings, the plan-pricing matrix, Durable Functions for orchestration, and the gotchas that bite production deployments.

## 1. The mental model — trigger + bindings + your code

An Azure Function is a single piece of code (a "function") invoked by *one* **trigger** with zero or more **input bindings** and zero or more **output bindings**. The runtime handles the trigger plumbing; your code just sees parameters.

```
[ Trigger ] → [ Input bindings ] → [ Your function code ] → [ Output bindings ]
```

This is the conceptual lift over AWS Lambda: Lambda gives you `event` and expects you to call AWS SDKs yourself for everything; Functions lets you *declare* "I take a queue message and emit two blobs" and the binding machinery handles auth and SDK calls. You can still call SDKs when bindings aren't enough.

A Python HTTP function with a Cosmos input binding and a Queue output binding:

```python
# function_app.py
import azure.functions as func
import logging

app = func.FunctionApp(http_auth_level=func.AuthLevel.ANONYMOUS)

@app.route(route="orders/{id}")
@app.cosmos_db_input(
    arg_name="doc",
    database_name="orders",
    container_name="items",
    connection="CosmosConnection",
    id="{id}",
    partition_key="{id}",
)
@app.queue_output(arg_name="msg", queue_name="audit-log", connection="StorageConnection")
def get_order(req: func.HttpRequest, doc: func.Document, msg: func.Out[str]) -> func.HttpResponse:
    if not doc:
        return func.HttpResponse("Not found", status_code=404)
    msg.set(f"order-fetched:{req.url}")
    return func.HttpResponse(doc.to_json(), mimetype="application/json")
```

Notice no `boto3`-equivalent SDK calls for the Cosmos read or queue send — the bindings handle it. `connection="CosmosConnection"` references an app setting that, when omitted (empty), tells the runtime to use the function app's **managed identity**.

## 2. Mechanism — host, runtime, and scaling

A **Function App** is the ARM resource. It hosts one or more individual functions sharing:

- The same **plan** (compute model).
- The same **runtime stack** (Node 20, Python 3.12, .NET 8 isolated, Java 21, PowerShell 7).
- The same **storage account** (used internally for triggers, leases, host metadata — *not optional*).
- The same **app settings** (env vars).
- The same **managed identity**.

Each function's trigger and bindings are declared in code (modern model) or `function.json` (older model). The Functions host polls/subscribes per trigger, scales workers up/down, and invokes your code.

### Trigger types worth knowing

| Trigger | Source | Note |
|---------|--------|------|
| HTTP | Direct HTTPS call | Most common public surface |
| Timer | CRON | NCRON expression: 6 fields incl seconds |
| Queue Storage | Storage queue | Cheap, simple |
| Service Bus | Queue or Topic subscription | The right choice for serious messaging |
| Event Hub | Event Hub partition | High-throughput streaming |
| Event Grid | Event Grid topic | System events (blob created, RG updated) |
| Blob Storage | Blob created/updated | Now polls event grid by default for low latency |
| Cosmos DB | Change feed | Streaming changes from a container |
| Durable Orchestration / Activity / Entity | Durable Functions | See §3 |

A function app can mix triggers — one HTTP, one Service Bus, one timer, all in the same app.

### App settings → environment variables

`AzureWebJobsStorage` (the storage account for the runtime), `FUNCTIONS_WORKER_RUNTIME`, custom settings like `CosmosConnection` — all surface as env vars. In code, `os.environ["CosmosConnection"]`. Set via:

```bash
az functionapp config appsettings set -g rg-orders-fn-prod -n fn-orders-prod \
  --settings "CosmosConnection__accountEndpoint=https://cosmos-orders.documents.azure.com:443/" \
             "ApplicationInsights__InstrumentationKey=@Microsoft.KeyVault(SecretUri=https://kv.../secrets/aiKey)"
```

Two things worth noting:

- `CosmosConnection__accountEndpoint` (note the `__` and no key) is the **identity-based** form. The runtime uses the MI to get a token; no connection string needed.
- `@Microsoft.KeyVault(SecretUri=...)` is a **Key Vault reference** — runtime resolves at startup. Pair with MI for full secret-free config.

## 3. Plans, scaling, and Durable Functions

### Hosting plans

| Plan | When to pick | Cold start? | Max scale | VNet | Cost model |
|------|--------------|-------------|-----------|------|------------|
| **Consumption** | Spiky, low-volume, dev | Yes (~1-3s) | ~200 instances | No (VNet integration limited) | Per-execution + GB-s |
| **Flex Consumption** | Modern consumption replacement; production-grade serverless | Lower | 1000 instances | Full VNet | Per-execution, pre-warmed instances |
| **Premium** | Steady traffic + VNet + always-warm | Eliminated | configurable | Full VNet | Per-vCPU/hour |
| **Dedicated (App Service)** | When you already pay for an ASP | Depends | Manual/auto | Yes | Already paid for |
| **Container Apps** | When you want Functions on KEDA scale | Lower | KEDA scale | Yes | vCPU/memory |

**Use Flex Consumption** for new production builds in 2026 — it's the modern, VNet-capable, low-cold-start serverless plan. Premium is fine; Consumption (classic) is legacy.

### Concurrency knobs

- `FUNCTIONS_WORKER_PROCESS_COUNT` — multiple worker processes per instance (Python/Node where the GIL/event loop matters).
- `maxConcurrentRequests` (HTTP) — per-instance.
- Per-trigger settings in `host.json` (e.g., `serviceBus.maxConcurrentCalls`, `eventHubs.batchCheckpointFrequency`).
- The runtime scales out *based on the trigger's queue depth or HTTP load*.

### Durable Functions — orchestration on top of Functions

Stateless functions can't easily express "do A, wait for an external approval, do B if approved else C, retry D three times." That's what **Durable Functions** is for. Three function types:

- **Orchestrator** — declarative workflow. Must be **deterministic** (no `Date.now()`, no random). Replays from history.
- **Activity** — the actual work. Called by the orchestrator.
- **Entity** — a small actor with state, optionally signaled or queried.

Sample orchestrator (Python):

```python
@app.orchestration_trigger(context_name="context")
def order_workflow(context: df.DurableOrchestrationContext):
    order = context.get_input()

    reserved = yield context.call_activity("reserve_inventory", order)
    if not reserved:
        return {"status": "rejected", "reason": "out-of-stock"}

    charged = yield context.call_activity_with_retry(
        "charge_card", df.RetryOptions(first_retry_interval_in_milliseconds=5000, max_number_of_attempts=3), order)
    if not charged:
        yield context.call_activity("release_inventory", order)
        return {"status": "rejected", "reason": "payment-failed"}

    yield context.call_activity("ship_order", order)
    return {"status": "complete"}
```

Patterns Durable handles natively: function chaining, fan-out/fan-in, async HTTP API, monitor, human interaction (`wait_for_external_event`), aggregator (entities).

Storage backend options: Azure Storage (default), Netherite (faster), MSSQL. Storage is fine for ≤200 ops/sec; pick Netherite for higher throughput.

## 4. Practical Application — production-ready Function App with MI + Key Vault refs

Bicep:

```bicep
param location string = resourceGroup().location
param appName string
param storageName string
param appInsightsConnectionString string
param keyVaultName string

resource storage 'Microsoft.Storage/storageAccounts@2023-05-01' = {
  name: storageName
  location: location
  sku: { name: 'Standard_ZRS' }
  kind: 'StorageV2'
  properties: {
    minimumTlsVersion: 'TLS1_2'
    allowBlobPublicAccess: false
    allowSharedKeyAccess: false   // MI-based AzureWebJobsStorage
  }
}

resource plan 'Microsoft.Web/serverfarms@2023-12-01' = {
  name: '${appName}-plan'
  location: location
  sku: { name: 'FC1', tier: 'FlexConsumption' }
  kind: 'functionapp,linux'
  properties: { reserved: true }
}

resource fn 'Microsoft.Web/sites@2023-12-01' = {
  name: appName
  location: location
  kind: 'functionapp,linux'
  identity: { type: 'SystemAssigned' }
  properties: {
    serverFarmId: plan.id
    httpsOnly: true
    functionAppConfig: {
      deployment: {
        storage: {
          type: 'blobContainer'
          value: '${storage.properties.primaryEndpoints.blob}deployment'
          authentication: { type: 'SystemAssignedIdentity' }
        }
      }
      runtime: { name: 'python', version: '3.12' }
      scaleAndConcurrency: {
        maximumInstanceCount: 100
        instanceMemoryMB: 2048
      }
    }
    siteConfig: {
      appSettings: [
        { name: 'AzureWebJobsStorage__accountName', value: storage.name }
        { name: 'AzureWebJobsStorage__credential', value: 'managedidentity' }
        { name: 'APPLICATIONINSIGHTS_CONNECTION_STRING', value: appInsightsConnectionString }
        { name: 'CosmosConnection__accountEndpoint', value: 'https://cosmos-orders.documents.azure.com:443/' }
        { name: 'StripeApiKey', value: '@Microsoft.KeyVault(SecretUri=https://${keyVaultName}.vault.azure.net/secrets/stripe-api-key/)' }
      ]
    }
  }
}

// Grant the function's MI Storage Blob Data Owner on its own storage account.
resource raStorage 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  scope: storage
  name: guid(storage.id, fn.id, 'b7e6dc6d-f1e8-4753-8033-0f276bb0955b')
  properties: {
    principalId: fn.identity.principalId
    principalType: 'ServicePrincipal'
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', 'b7e6dc6d-f1e8-4753-8033-0f276bb0955b')  // Storage Blob Data Owner
  }
}
```

Three things to notice:

- `AzureWebJobsStorage__accountName` + `AzureWebJobsStorage__credential=managedidentity` → the runtime uses the function's MI to read/write its own host storage. No connection string, no key.
- The Key Vault reference `@Microsoft.KeyVault(SecretUri=...)` resolves at startup. You must also grant the MI `Key Vault Secrets User`.
- `FlexConsumption` plan (`FC1` SKU) with 2 GB per instance.

Deploy code:

```bash
func azure functionapp publish $appName --python
```

## 5. Common Mistakes & Gotchas

- **Storage account shared across function apps.** `AzureWebJobsStorage` is *per-app* state (host locks, trigger leases). Sharing one storage account between two function apps causes singleton-lock conflicts and lost messages. One storage account per function app.
- **Non-deterministic code in Durable orchestrators.** `datetime.utcnow()`, `random.random()`, `requests.get()` will produce different values on replay and corrupt state. Use `context.current_utc_datetime`, `context.new_guid()`, and call activities for any I/O.
- **Long-running HTTP triggers.** Function app HTTP requests time out (230s on Consumption/Premium; configurable on Flex). For long work, use Durable's async HTTP pattern: return 202 with a status URL.
- **Cold starts on Consumption.** First request after idle: 1-3s for Node/Python, 5-10s for .NET (first time). If you can't tolerate, move to Premium or Flex.
- **Forgetting `host.json` for batch settings.** `serviceBusTrigger.maxConcurrentCalls` defaults to 16 — fine until you scale to 50 instances and overwhelm downstream. Tune.
- **VNet integration with Consumption.** Classic Consumption can't reach private endpoints. Use Premium or Flex.
- **Connection-string secrets in App Settings.** Use Key Vault references + MI. App Settings are stored encrypted but visible to anyone with Contributor.
- **Mixing `function.json` and decorators.** Pick one model per language. Python and Node v4 model use decorators; older Python and Node v3 used `function.json`.
- **Forgetting `WEBSITE_RUN_FROM_PACKAGE`** for deployment artifacts in older plans. On Flex Consumption deployment is configured at the plan/app level via `functionAppConfig.deployment` — different model. Don't mix.
- **Application Insights sampling.** Default sampling drops a lot of telemetry under load. For services where every trace matters, disable adaptive sampling in `host.json`.
- **Durable history grows unbounded.** Sample large fan-outs explode the orchestration history. Use `continue_as_new` for long-running orchestrations and `purgeInstanceHistoryAsync` after completion.
- **Triggers paused after credential rotation.** Rotating storage account keys (if not using MI) silently breaks `AzureWebJobsStorage`. Symptoms: triggers stop firing, no errors. Use MI.

## 🎯 Key Takeaways

- **Triggers + bindings + small functions** — that's the whole programming model. SDKs are still there when needed.
- **Flex Consumption is the modern serverless plan.** VNet, fast scale, low cold-start, MI-backed storage. Use it for new builds.
- **Managed identity everywhere.** `__accountName` + `__credential=managedidentity` patterns replace connection strings; Key Vault references handle the rest.
- **Durable Functions for any non-trivial workflow.** Don't reinvent orchestration with queues + cron + retry.
- **One function app = one storage account = one set of triggers**. Don't share storage across function apps.

*← [prev](./07_sql_and_cosmos.md) | [next → 09_app_service_container_apps.md](./09_app_service_container_apps.md)*
