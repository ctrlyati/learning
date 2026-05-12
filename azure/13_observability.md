# 13 — Observability: Azure Monitor, Log Analytics, App Insights, KQL

> **Goal:** Understand how Azure's observability pieces fit together, write basic-to-intermediate KQL, and instrument apps and infra so you can actually debug production at 2 a.m.

## 1. The Azure Monitor pyramid

Three families of telemetry, one query language.

```
                                ┌────────────────────────┐
                                │   Azure Monitor (UI)   │
                                └───────────┬────────────┘
                                            │
              ┌─────────────────────────────┼─────────────────────────────┐
              ▼                             ▼                             ▼
        ┌──────────┐                  ┌──────────┐                  ┌──────────┐
        │ Metrics  │                  │   Logs   │                  │  Traces  │
        │ (TSDB)   │                  │ (LAW/KQL)│                  │(App Insights)│
        └──────────┘                  └──────────┘                  └──────────┘
        per-resource,                 free-form text +              distributed
        numeric, fast                 structured rows                request flow
```

- **Metrics** — numeric time series, pre-aggregated, ms latency to query. Live in a per-resource metrics store. CPU%, request count, message lag. Cheap.
- **Logs** — flexible structured records ingested into a **Log Analytics Workspace (LAW)**. Queryable with **KQL** (Kusto Query Language). Slower, richer.
- **Traces / requests / dependencies** — **Application Insights** (which is really a special schema *inside* LAW). End-to-end distributed traces with sampling.

In 2026 every Azure observability conversation goes through these three lenses.

## 2. Log Analytics Workspace — your data lake of telemetry

A **LAW** is the storage and query engine. Many resources emit into a LAW via **Diagnostic Settings** (per resource: "send these log categories and metrics to LAW X").

```bash
RG=rg-platform-prod
LAW=law-platform-prod

az monitor log-analytics workspace create -g $RG -n $LAW \
  --location eastus2 \
  --retention-time 90 \
  --sku PerGB2018

# Stream Storage account diagnostics to LAW.
SA_ID=$(az storage account show -n stappdataprod -g rg-data --query id -o tsv)
LAW_ID=$(az monitor log-analytics workspace show -g $RG -n $LAW --query id -o tsv)

az monitor diagnostic-settings create \
  --name to-law \
  --resource $SA_ID \
  --workspace $LAW_ID \
  --logs    '[{"categoryGroup":"audit","enabled":true}]' \
  --metrics '[{"category":"AllMetrics","enabled":true}]'
```

Plus the **Azure Activity Log** (all ARM control-plane events for a sub) can be piped to LAW: `az monitor diagnostic-settings subscription create ...`.

### Tables you'll query often

| Table | What's in it |
|-------|--------------|
| `AzureActivity` | Control-plane events (who did what to which resource) |
| `AzureDiagnostics` | Legacy multi-service catch-all (now mostly per-resource-specific tables) |
| `AppRequests`, `AppDependencies`, `AppExceptions`, `AppTraces` | App Insights schemas |
| `Heartbeat` | VM agent / Azure Monitor agent heartbeats |
| `Perf` | VM perf counters (from AMA) |
| `Syslog`, `Event` | Linux syslog, Windows event log (from AMA) |
| `ContainerLogV2` | AKS / Container Apps container stdout/stderr |
| `SigninLogs`, `AuditLogs` | Entra sign-ins and audits |
| `<Resource>` tables | Service-specific (e.g., `AzureMetrics`, `SQLSecurityAuditEvents`) |

### KQL — the language you must know

KQL is read-only, pipeline-oriented, declarative. Always: `table | filter | project | aggregate | render`.

```kql
// Top 10 most-failing requests in the last hour
AppRequests
| where TimeGenerated > ago(1h)
| where Success == false
| summarize count() by Name, ResultCode
| top 10 by count_

// P95 dependency latency by target, last 24h, hourly buckets
AppDependencies
| where TimeGenerated > ago(24h)
| summarize p95 = percentile(DurationMs, 95) by bin(TimeGenerated, 1h), Target
| render timechart

// Anybody granting Owner role in the last 7 days?
AzureActivity
| where TimeGenerated > ago(7d)
| where OperationNameValue == "MICROSOFT.AUTHORIZATION/ROLEASSIGNMENTS/WRITE"
| extend role = tostring(Properties_d.requestbody)
| where role contains "8e3af657-a8ff-443c-a75c-2fe8c4bcb635"  // Owner role def id
| project TimeGenerated, Caller, ResourceId = ResourceId, role

// Slow SQL queries from Azure SQL audit
AzureDiagnostics
| where ResourceType == "SERVERS/DATABASES" and Category == "QueryStoreRuntimeStatistics"
| where duration_d > 5000
| project TimeGenerated, ResourceId, query_hash_s, duration_d
| top 50 by duration_d desc

// Cross-table: requests joined with their exceptions
let recent = AppRequests | where TimeGenerated > ago(1h);
recent
| join kind=leftouter (AppExceptions | where TimeGenerated > ago(1h))
       on OperationId
| where Success == false
| project TimeGenerated, Name, ResultCode, Type, OuterMessage
```

Operators worth memorizing: `where`, `summarize`, `project`, `extend`, `top`, `take`, `join`, `union`, `let`, `bin()`, `ago()`, `count()`, `percentile()`, `make-list()`, `parse_json()`, `mv-expand`, `render`.

## 3. Variations and depth — App Insights, alerts, workbooks

### Application Insights

App Insights is an opinionated *application* telemetry layer. It captures:

- **Requests** — every HTTP/gRPC request your app handles.
- **Dependencies** — every outbound call your app makes (SQL, HTTP, Service Bus, Cosmos).
- **Exceptions** — unhandled and tracked.
- **Traces / Custom events / Metrics** — what you log explicitly.
- **Live Metrics** — sub-second view of activity.

Two ingestion models:

- **Workspace-based** (modern) — App Insights data lives in a LAW. Use this. KQL queries everything together.
- **Classic** — separate store; deprecated.

Instrumentation:

```python
# Python — OpenTelemetry + Azure Monitor exporter (the 2026 way)
from azure.monitor.opentelemetry import configure_azure_monitor
configure_azure_monitor(connection_string="InstrumentationKey=...;IngestionEndpoint=...")

# Now any opentelemetry-instrumented library auto-emits requests, deps, logs.
```

OpenTelemetry is the standard going forward. Classic per-language SDKs (`ApplicationInsights.SDK`) still work; new projects should be OTel.

### Alerts

Three alert types:

- **Metric alerts** — threshold on a metric (CPU > 90% for 5 min). Fast, cheap.
- **Log alerts** — KQL query returning rows triggers. Slower, flexible.
- **Activity log alerts** — subscription-level events (role assigned, resource deleted).

```bash
az monitor metrics alert create -g $RG -n alert-high-5xx \
  --scopes $APPINSIGHTS_ID \
  --condition "count requests/failed > 50" --window-size 5m --evaluation-frequency 1m \
  --action-groups $AG_ID
```

**Action groups** define the notification fan-out: email, SMS, Teams webhook, Logic App, Function, ITSM (PagerDuty, ServiceNow). Reuse one action group across many alerts.

### Workbooks, dashboards, Grafana

- **Workbooks** — Azure-native interactive dashboards with KQL + Markdown + parameters. Great for ops runbooks.
- **Azure Dashboards** — pinned-tile dashboards in the portal. Simpler than workbooks.
- **Managed Grafana** — Azure-hosted Grafana that auto-connects to your subs. The right pick if your org already uses Grafana.

### Cost knobs

LAW pricing is per GB ingested + retention. You will pay more for observability than you expect; the levers:

- **Data sampling** in App Insights (especially traces). Default adaptive sampling keeps 5/s; configurable.
- **Diagnostic settings categories** — only enable what you'll actually query.
- **Data tiers** — Basic Logs (cheap ingest, expensive query, limited retention) for verbose categories you rarely query.
- **Retention** — 30 days included; longer costs per GB-month. Pair with Archive for compliance.
- **Commitment tiers** — buy 100/200/500/1000 GB/day for a discount over pay-as-you-go.

## 4. Practical Application — a minimum-viable production observability setup

For a typical web app (App Service, SQL DB, Service Bus, Storage):

1. **One LAW per environment.** `law-prod`, `law-nonprod`. Centralizing simplifies queries.
2. **Diagnostic settings** on every resource → LAW. Categories to enable:
   - App Service: `AppServiceHTTPLogs`, `AppServiceConsoleLogs`, `AppServiceAppLogs`, `AppServiceAuditLogs`. AllMetrics.
   - SQL: `SQLSecurityAuditEvents`, `Errors`, `QueryStoreRuntimeStatistics`. AllMetrics.
   - Service Bus: `OperationalLogs`, `RuntimeAuditLogs`. AllMetrics.
   - Storage: `audit` category group.
   - Key Vault: `AuditEvent`. *Critical.*
3. **App Insights** (workspace-based, same LAW). Connection string in App Service config. OpenTelemetry SDK in app code.
4. **Activity Log** → LAW at sub scope.
5. **Action group** with on-call email + Teams webhook + PagerDuty.
6. **Baseline alerts**:
   - App availability ping fails.
   - 5xx error rate > 1% over 5 min.
   - SQL DTU/CPU > 90% over 10 min.
   - Service Bus DLQ messages > 0.
   - Key Vault secret accessed by a non-app principal (KQL alert).
   - Activity log: role assignment at sub or MG scope (audit).
7. **One workbook** per service area (web, data, messaging) with SLOs and recent incidents.

Bicep for diagnostic settings on an existing storage account:

```bicep
param storageAccountId string
param lawId string

resource diag 'Microsoft.Insights/diagnosticSettings@2021-05-01-preview' = {
  name: 'to-law'
  scope: any(storageAccountId)
  properties: {
    workspaceId: lawId
    logs: [{ categoryGroup: 'audit', enabled: true }]
    metrics: [{ category: 'AllMetrics', enabled: true }]
  }
}
```

Repeat that pattern across every prod resource — ideally in a module called from your landing-zone Bicep (module 18).

## 5. Common Mistakes & Gotchas

- **Multiple LAWs for one workload.** "Send App Service logs to LAW A, SQL logs to LAW B" — and now your KQL `join` doesn't work across workspaces without `workspace("LAW B")`. Consolidate.
- **App Insights *classic* vs workspace-based.** Workspace-based is the only modern choice. Migrate classic instances; their feature set is frozen.
- **Adaptive sampling unintentionally dropping critical errors.** Sampling drops at the SDK level before ingestion. For a "report every exception" guarantee, override sampling on the exception path or disable sampling for that category.
- **Diagnostic-setting log category typos.** ARM accepts wrong category names silently (you get no rows). Use `az monitor diagnostic-settings categories list --resource <id>` to find valid names.
- **Forgetting `AllMetrics`.** Logs categories ≠ metrics. You need both.
- **KQL `union *` performance.** Querying every table is expensive. Always include `where TimeGenerated > ago(...)` as the *first* filter.
- **Costs ballooning quietly.** A noisy app or a misconfigured Postgres flexible server can push GB/day from 5 to 500 overnight. Cost alerts on LAW ingestion are essential.
- **Action groups not idempotent in Bicep.** Re-deploying often re-creates or reorders members. Use `existing` references for stable ones.
- **Alerts on raw 5xx without smart-grouping.** Bursts during deploys trigger 50 alerts. Use Azure Monitor's **dynamic thresholds** or **alert processing rules** to suppress or group.
- **No OpenTelemetry correlation.** Without W3C TraceContext propagation, your distributed trace stops at each service boundary. Verify `traceparent` headers flow.
- **VM logs require the Azure Monitor Agent (AMA).** The old Log Analytics agent (MMA) is retired. Install AMA via VM Insights or extension; configure Data Collection Rules (DCR) to scope what's collected.
- **`azureDiagnostics` legacy table** still surfaces in some services and is being migrated to per-service tables. Prefer the specific table (`AppServiceHTTPLogs`, etc.) when available.
- **Sentinel on the same LAW.** Sentinel (module 14) layers on top of LAW. If you run Sentinel, retention and pricing change.

## 🎯 Key Takeaways

- **Three telemetry families, one query language.** Metrics for speed, Logs for richness, Traces for distributed flow — all queryable via KQL through Log Analytics.
- **One LAW per environment, App Insights workspace-based, OpenTelemetry SDK.** That trio is the modern standard.
- **Diagnostic Settings on every resource at create time.** Bake into Bicep modules so it cannot be forgotten.
- **Alert sparingly, route through Action Groups.** Tune sampling and category selection or you'll pay for telemetry nobody reads.
- **Master 10 KQL operators** (`where`, `summarize`, `project`, `extend`, `bin`, `join`, `union`, `let`, `top`, `parse_json`). They cover 90% of production debugging.

*← [prev](./12_containers.md) | [next → 14_security.md](./14_security.md)*
