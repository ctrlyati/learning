# 09 — App Service and Container Apps

> **Goal:** Pick between Azure App Service (PaaS web hosting) and Container Apps (managed-Kubernetes-ish), configure deployment slots, scaling, and identity for each, and avoid the classic PaaS traps.

## 1. App Service — the OG PaaS

**Azure App Service** is the original PaaS web host: drop a ZIP, JAR, WAR, or container image, get a managed HTTPS endpoint, autoscale, slots, log streaming, and ~zero ops. Languages: .NET, Java, Node, Python, PHP, Ruby (Linux only for most), or any container.

The unit is a **Web App** (Microsoft.Web/sites) running on an **App Service Plan** (Microsoft.Web/serverfarms). The plan is the compute; the app is the workload. One plan can host many apps — useful for siblings that share a region and tier.

```bash
RG=rg-web-prod
az appservice plan create -g $RG -n asp-web-prod \
  --is-linux --sku P1v3 --zone-redundant true

az webapp create -g $RG -p asp-web-prod -n app-orders-prod \
  --runtime "NODE:20-lts" --https-only true
```

### Plan SKUs that matter

| SKU | Tier | When to pick |
|-----|------|--------------|
| `F1`, `D1` | Free / Shared | Tiny dev, no SLA |
| `B1`-`B3` | Basic | Dev/staging, no autoscale, no slots |
| `P0v3`-`P5v3` | Premium v3 | Production. Autoscale, slots, VNet, zone-redundant |
| `I1v2`-`I6v2` | Isolated v2 (App Service Environment v3) | Dedicated tenant, full VNet integration, regulated industries |

Always Premium v3 for production. Premium v3 is *cheaper* than Premium v2 for similar performance — there's no reason to be on v2.

### Configuration model

- **App settings** — env vars (`process.env.FOO`).
- **Connection strings** — typed env vars with optional secret-class tagging.
- **General settings** — HTTPS-only, TLS version, FTP state, stack version, startup command.
- **Identity** — system or user-assigned MI.
- **Networking** — VNet integration (outbound), private endpoint (inbound), access restrictions.

```bash
az webapp config appsettings set -g $RG -n app-orders-prod --settings \
  WEBSITES_PORT=3000 \
  "DatabaseUrl=@Microsoft.KeyVault(SecretUri=https://kv.../secrets/db-url)"

az webapp identity assign -g $RG -n app-orders-prod
```

### Deployment slots — the PaaS killer feature

A **slot** is a sibling app with its own URL (`app-orders-prod-staging.azurewebsites.net`), config (or shared, your choice), and possibly its own code. Use slots for:

- **Zero-downtime deploys.** Deploy to staging → smoke test → **swap**. Swap is near-instant: just a hostname/binding swap.
- **A/B testing.** Send 10% of traffic to staging via `slot traffic routing`.
- **Pre-warming.** Slots are warmed before the swap so users don't hit cold instances.

```bash
az webapp deployment slot create -g $RG -n app-orders-prod --slot staging
az webapp deploy -g $RG -n app-orders-prod --slot staging --src-path ./app.zip --type zip
az webapp deployment slot swap -g $RG -n app-orders-prod --slot staging --target-slot production
```

Slots count as separate instances against your plan's instance limit. Premium v3 P1v3 allows 5 slots; P3v3 allows 20.

### Slot config — "sticky" settings

Some settings should *stay* with their slot during a swap (e.g., `DatabaseUrl=prod-db` in production, `DatabaseUrl=staging-db` in staging). Mark those as **slot settings** so they don't follow the code during swap.

```bash
az webapp config appsettings set -g $RG -n app-orders-prod --slot-settings DatabaseUrl="..."
```

## 2. Scaling, identity, networking

### Scaling

- **Scale up** — change SKU (P1v3 → P2v3). Restarts the app.
- **Scale out** — add instances. No restart. Premium v3 supports manual (1-30) or autoscale.

```bash
az monitor autoscale create \
  --resource-group $RG --resource app-orders-prod --resource-type Microsoft.Web/sites \
  --name as-app-orders --min-count 2 --max-count 10 --count 2

az monitor autoscale rule create -g $RG --autoscale-name as-app-orders \
  --condition "CpuPercentage > 70 avg 5m" --scale out 1
```

Premium v3 also offers **automatic scaling** (newer feature) where the platform decides — set min/max and forget the rules. Less control but lower-friction.

### Identity and Key Vault refs

Same model as Functions (module 08). Enable system MI, reference Key Vault secrets via `@Microsoft.KeyVault(SecretUri=...)`. No connection strings, no rotation.

### Networking

- **VNet Integration** — outbound. App can call resources in your VNet (private endpoints, peered VNets). Configure a dedicated `/27` (Premium v3) integration subnet.
- **Private Endpoint** — inbound. App is only reachable from inside the VNet.
- **Access Restrictions** — IP allowlists, Service Tag allowlists. Per-app or per-slot.

```bash
az webapp vnet-integration add -g $RG -n app-orders-prod \
  --vnet vnet-prod-eastus2 --subnet snet-appservice
```

App Service is a managed multi-tenant service; even with VNet integration outbound, the inbound IP is on the App Service shared frontend. For "fully VNet-isolated" you need Private Endpoint or App Service Environment v3.

## 3. Container Apps — managed Kubernetes-lite

**Azure Container Apps (ACA)** is built on Kubernetes + Dapr + KEDA, but you never touch Kubernetes. You define a container, set scale rules, and ACA does the rest. Compared to App Service:

| | App Service | Container Apps |
|--|-------------|----------------|
| Workload | Any code or container | Container only |
| Scale to zero | No | Yes |
| Scale on queue/event | Limited | Native (KEDA) |
| Microservices fabric | Single-app | Multi-container env with internal DNS |
| Dapr integration | No | Yes |
| Cold start | None on Premium | Some (scaling from zero) |
| VNet | Configurable | Configurable, internal-only mode available |
| Pricing | Plan-hour | vCPU-sec + memory-sec |

Pick ACA when:

- You have a containerized microservice fleet.
- You need scale-to-zero (cost-sensitive low-traffic services).
- You want Dapr (state, pub/sub, bindings without coupling to providers).
- You don't want to operate AKS.

### Anatomy

```
Container App Environment      (VNet, Log Analytics workspace, Dapr)
   ├── Container App: orders-api   (revisions, ingress, scale rules, secrets, MI)
   ├── Container App: orders-worker
   └── Container App: pricing-api
```

The **environment** is the shared boundary (one VNet, one Log Analytics workspace). Container apps inside an environment can call each other by short name over an internal-only Envoy ingress.

```bash
ENV=cae-orders-prod
RG=rg-orders-prod

az containerapp env create -g $RG -n $ENV \
  --location eastus2 \
  --infrastructure-subnet-resource-id $(az network vnet subnet show -g rg-network-prod -n snet-aca --vnet-name vnet-prod-eastus2 --query id -o tsv) \
  --internal-only true \
  --zone-redundant true

az containerapp create -g $RG -n orders-api \
  --environment $ENV \
  --image acmecr.azurecr.io/orders-api:1.0.0 \
  --target-port 8080 --ingress external \
  --min-replicas 1 --max-replicas 10 \
  --cpu 0.5 --memory 1.0Gi \
  --registry-server acmecr.azurecr.io --registry-identity system \
  --system-assigned \
  --scale-rule-name http --scale-rule-http-concurrency 50
```

`--registry-identity system` tells ACA to pull from ACR using its system MI — no registry username/password needed (grant the MI `AcrPull` on the ACR; module 12).

### Revisions

Every config change creates a **revision**. By default the latest revision serves 100% of traffic, but you can split:

```bash
az containerapp ingress traffic set -g $RG -n orders-api \
  --revision-weight orders-api--rev10=90 orders-api--rev11=10
```

This is canary deploys built-in. Plus single-revision mode for simple rollouts: each new revision replaces the prior.

### Scale rules — KEDA

ACA scales on:

- **HTTP** — concurrent requests per replica.
- **TCP** — connections per replica.
- **CPU / memory** — classic.
- **KEDA-driven** — Service Bus queue depth, Event Hub lag, Kafka, Postgres queries, custom.

```bash
az containerapp update -g $RG -n orders-worker \
  --scale-rule-name sb-queue \
  --scale-rule-type azure-servicebus \
  --scale-rule-metadata "queueName=orders-incoming" "namespace=sb-orders-prod" "messageCount=5" \
  --scale-rule-auth "connection=service-bus-connection"
```

Min replicas 0 = scale to zero when idle. Max replicas up to 300.

## 4. Practical Application — decision matrix

Workload | Choose
---|---
"REST API serving JSON, single language, low ops" | App Service Premium v3 + slots
"Containerized microservices fleet, internal-only" | Container Apps (internal-only environment)
"Worker that processes a Service Bus queue and idles overnight" | Container Apps (scale to zero on queue length)
"Existing .NET monolith with WebJobs" | App Service Premium v3
"Public-facing site behind Front Door, multi-region" | App Service Premium v3 with WAF policy on Front Door
"Need full Kubernetes, sidecars, network policies" | AKS (separate course)

A complete Container Apps Bicep snippet with a system MI pulling from ACR and writing to Service Bus:

```bicep
resource env 'Microsoft.App/managedEnvironments@2024-03-01' = {
  name: 'cae-orders-prod'
  location: location
  properties: {
    appLogsConfiguration: {
      destination: 'log-analytics'
      logAnalyticsConfiguration: { customerId: lawCustomerId, sharedKey: lawSharedKey }
    }
    vnetConfiguration: { internal: true, infrastructureSubnetId: subnetId }
    zoneRedundant: true
  }
}

resource api 'Microsoft.App/containerApps@2024-03-01' = {
  name: 'orders-api'
  location: location
  identity: { type: 'SystemAssigned' }
  properties: {
    managedEnvironmentId: env.id
    configuration: {
      ingress: { external: false, targetPort: 8080, transport: 'auto' }
      registries: [{ server: 'acmecr.azurecr.io', identity: 'system' }]
    }
    template: {
      containers: [{
        name: 'orders-api'
        image: 'acmecr.azurecr.io/orders-api:1.0.0'
        resources: { cpu: json('0.5'), memory: '1.0Gi' }
        env: [
          { name: 'SERVICEBUS_NAMESPACE', value: 'sb-orders-prod.servicebus.windows.net' }
        ]
      }]
      scale: {
        minReplicas: 1
        maxReplicas: 10
        rules: [{
          name: 'http'
          http: { metadata: { concurrentRequests: '50' } }
        }]
      }
    }
  }
}
```

## 5. Common Mistakes & Gotchas

- **Slot config not marked as slot-sticky.** Default behavior of swap is to move app settings with the code. Mark environment-specific settings (`DatabaseUrl`, `RedisHost`) as slot settings or you'll swap prod into staging's DB.
- **Slots count against plan instances and confuse autoscale.** Plans bill for the SKU, not per slot. But slot instances share the plan's pool.
- **`WEBSITES_PORT` mistake on Linux containers.** App Service Linux expects the container to listen on the `PORT` env var (default 8080). Either configure your app to read `PORT` or set `WEBSITES_PORT` explicitly.
- **Premium v2 still in use.** Premium v3 is cheaper and faster. Migrate.
- **VNet integration not in the right subnet size.** Premium v3 needs a `/27` minimum; the subnet must be empty and delegated to `Microsoft.Web/serverFarms`.
- **Access Restrictions + Front Door header.** When fronting App Service with Front Door, restrict to the `AzureFrontDoor.Backend` service tag *and* require the `X-Azure-FDID` header matching your Front Door's ID — otherwise anyone can hit App Service directly.
- **Container Apps environment is one VNet.** All apps share it. To isolate, create another environment.
- **Min replicas 0 has cold starts.** Scale from 0 → 1 takes seconds. For latency-sensitive APIs, min 1 (or use App Service).
- **KEDA scale-to-zero with HTTP scaler** — HTTP scaler can't scale from zero in some configurations because there's no traffic yet. Use a custom scaler or always min 1.
- **Container image must be linux/amd64 unless explicitly running on ARM nodes.** Pushing an Apple-silicon-built image (linux/arm64) without `--platform linux/amd64` produces a Container App that won't start.
- **App Service Easy Auth (built-in auth) is great but opaque.** It injects a sidecar that handles OIDC. Wonderful for "Sign in with Microsoft" without code; surprising when you debug 401s. Enable structured logging.
- **Soft delete on App Service Plan.** Deleting an app doesn't delete its plan. Plans are billed by the hour. Audit orphaned plans monthly.

## 🎯 Key Takeaways

- **App Service** for traditional web apps and APIs; Premium v3 + slots + VNet integration + MI is the modern baseline.
- **Container Apps** for containerized microservices, event-driven workers, and anything that wants scale-to-zero or KEDA scaling.
- **Slots are the killer feature** of App Service — use them for zero-downtime deploys, A/B testing, and pre-warming. Don't forget slot-sticky settings.
- **Use system MI + Key Vault references**; never put connection strings in App Settings directly.
- **For multi-microservice systems**, Container Apps Environment as the shared substrate beats one App Service per service in operational simplicity and cost.

*← [prev](./08_functions_serverless.md) | [next → 10_messaging.md](./10_messaging.md)*
