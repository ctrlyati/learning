# 12 — Containers on Azure: ACR, ACI, Container Apps

> **Goal:** Understand the Azure container story without AKS — registry, throwaway containers, and serverless container hosting — and know how they compose with the rest of the platform.

## 1. Azure Container Registry (ACR) — your container artifact store

**ACR** is a managed OCI-compliant registry. Same surface as Docker Hub or ECR. Three SKUs:

- **Basic** — for dev. No geo-replication, no private endpoint.
- **Standard** — for most prod use.
- **Premium** — geo-replication, Private Link, content trust, customer-managed keys, scoped tokens, retention policies. Production-grade.

Pick Premium unless you have a clear reason not to.

```bash
RG=rg-platform-prod
ACR=acmecrprod

az acr create -g $RG -n $ACR \
  --sku Premium \
  --admin-enabled false \
  --public-network-enabled false \
  --zone-redundancy Enabled

# Geo-replicate to a second region.
az acr replication create -r $ACR -l westus3 --resource-group $RG --zone-redundancy Enabled
```

`--admin-enabled false` disables the admin user (a username/password that bypasses RBAC) — always disable in production.

### Authenticating to ACR

Three sane options:

1. **Entra MI + `AcrPull` role** (push: `AcrPush`). The way to consume from Azure Container Apps / App Service / AKS.
2. **`az acr login`** for developers. Uses your Entra token.
3. **Scoped tokens** (Premium) for CI/CD if OIDC federation isn't available.

```bash
# Grant a Container App's MI pull access.
az role assignment create \
  --assignee-object-id $CONTAINER_APP_MI \
  --assignee-principal-type ServicePrincipal \
  --role AcrPull \
  --scope $(az acr show -n $ACR --query id -o tsv)

# Developer push.
az acr login -n $ACR
docker build -t $ACR.azurecr.io/orders-api:1.0.0 .
docker push $ACR.azurecr.io/orders-api:1.0.0
```

### ACR Tasks — build in the cloud

You don't need a local Docker daemon. ACR Tasks build container images server-side on ACR.

```bash
# Quick build from local context.
az acr build -r $ACR --image orders-api:1.0.0 .

# Multi-step task with triggers (git push, base image update).
az acr task create -r $ACR -n orders-api-task \
  --image orders-api:{{.Run.ID}} \
  --context https://github.com/acme/orders.git \
  --file Dockerfile \
  --branch main \
  --git-access-token $GH_PAT
```

Use Tasks for:

- Cross-platform builds (linux/amd64 + linux/arm64).
- Base-image-update-driven rebuilds (security patches).
- CI that doesn't need a full Azure DevOps / GitHub Actions runner.

### Image scanning

Premium ACR + **Microsoft Defender for Containers** (module 14) scans every image at push and at runtime, flags CVEs, surfaces in Defender for Cloud.

## 2. Azure Container Instances (ACI) — single throwaway containers

ACI runs a container (or a small group) without a cluster. You give it an image, CPU/memory, and ports; Azure runs it.

Use cases:

- **One-off jobs** — batch processing, data migration scripts, scheduled rebuilds.
- **CI build agents** ephemeral runners.
- **Burst from AKS** via Virtual Kubelet (older pattern).
- **Throwaway dev containers** for testing.

```bash
az container create -g rg-jobs -n migrate-2026-04 \
  --image acmecrprod.azurecr.io/db-migrate:1.0.0 \
  --cpu 1 --memory 2 \
  --restart-policy OnFailure \
  --acr-identity system \
  --assign-identity \
  --vnet vnet-prod-eastus2 --subnet snet-aci \
  --no-wait

# Tail logs while it runs.
az container logs -g rg-jobs -n migrate-2026-04 --follow
```

ACI is **not** for long-lived production services in 2026 — Container Apps is strictly better for that. ACI is right when you want a container *for an hour* and then gone.

Other ACI knobs:

- **GPU** — `--gpu-count 1 --gpu-sku V100`. Rare, niche.
- **Container Groups** — multiple containers sharing a network and lifecycle (like a Kubernetes pod). Useful for sidecar logging or tightly-coupled helpers.
- **Confidential containers** — SGX-based. For regulated workloads.

## 3. Container Apps in the container context

We covered Container Apps in module 09 from the *web hosting* angle. From the *container* angle, the points worth re-emphasizing:

- **No Kubernetes to operate.** ACA runs on K8s + KEDA + Dapr under the hood, but you never see it.
- **Pulls from ACR via system MI**, with `AcrPull` granted to the MI.
- **Revisions = image versions**, with traffic-splitting for canary.
- **Dapr sidecars** give you state stores, pub/sub, secrets, bindings as a config-driven sidecar — useful for polyglot microservices.
- **Containers App Jobs** — sibling resource to Container Apps for finite jobs (cron, event-triggered, manual). Replaces a lot of what ACI used to do for scheduled batch.

```bash
az containerapp job create -g $RG -n nightly-export \
  --environment $ENV \
  --image $ACR.azurecr.io/exporter:1.2.0 \
  --trigger-type Schedule \
  --cron-expression "0 2 * * *" \
  --replica-timeout 3600 --replica-retry-limit 2 --parallelism 1 \
  --registry-identity system --registry-server $ACR.azurecr.io
```

## 4. Practical Application — build, push, deploy, scan

End-to-end "I have code, I want a running container in ACA":

```bash
# 0. Prereqs: ACR, Container Apps Environment, target Container App's MI granted AcrPull.

# 1. Build in ACR (no local Docker daemon).
az acr build -r acmecrprod --image orders-api:$(git rev-parse --short HEAD) --platform linux/amd64 .

# 2. Update the Container App to the new revision.
TAG=$(git rev-parse --short HEAD)
az containerapp update -g rg-orders-prod -n orders-api \
  --image acmecrprod.azurecr.io/orders-api:$TAG \
  --revision-suffix r$TAG

# 3. Canary: 10% to new revision.
az containerapp ingress traffic set -g rg-orders-prod -n orders-api \
  --revision-weight latest=10 prior-revision-stable=90

# 4. Monitor success / errors via App Insights or Log Analytics.
# 5. Promote: latest=100, prior=0. Old revision gets scaled to zero by ACA.
```

Bicep for the ACR + Container App pull setup:

```bicep
resource acr 'Microsoft.ContainerRegistry/registries@2023-11-01-preview' = {
  name: 'acmecrprod'
  location: location
  sku: { name: 'Premium' }
  properties: {
    adminUserEnabled: false
    publicNetworkAccess: 'Disabled'
    zoneRedundancy: 'Enabled'
  }
}

resource ca 'Microsoft.App/containerApps@2024-03-01' = {
  name: 'orders-api'
  location: location
  identity: { type: 'SystemAssigned' }
  properties: {
    managedEnvironmentId: env.id
    configuration: {
      registries: [{ server: '${acr.name}.azurecr.io', identity: 'system' }]
    }
    template: { /* containers + scale */ }
  }
}

var acrPullRoleId = '7f951dda-4ed3-4680-a7ca-43fe172d538d'

resource raAcrPull 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  scope: acr
  name: guid(acr.id, ca.id, acrPullRoleId)
  properties: {
    principalId: ca.identity.principalId
    principalType: 'ServicePrincipal'
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', acrPullRoleId)
  }
}
```

That's the canonical pattern: registry private, container app's MI granted AcrPull, image referenced by tag.

## 5. Common Mistakes & Gotchas

- **Admin user enabled on ACR.** A username/password that bypasses RBAC and shows up in connection strings everywhere. Disable.
- **Storing image tags as `latest` in production.** ACA will pull a new image only when *the tag's referenced digest changes*. `latest` is non-deterministic; rollbacks are guesswork. Use immutable tags (commit SHA, semver).
- **No image cleanup policy.** ACR fills with old images, costs balloon. Premium supports retention policies; configure one (`az acr config retention update --status enabled --days 30 --type UntaggedManifests`).
- **Pull from ACR fails with 401 in Container Apps.** Usually means the MI doesn't have `AcrPull` *or* the assignment is at the wrong scope *or* the assignment hasn't propagated. Wait 60s after assignment in CI.
- **ACR Private Link without DNS.** `acmecrprod.azurecr.io` resolves to the public IP. With Private Link you also need the `privatelink.azurecr.io` private DNS zone + VNet link. Plus an extra zone for the data plane (`acmecrprod.<region>.data.azurecr.io`).
- **Wrong CPU/memory ratio in ACA.** Container Apps requires a specific CPU/memory combo (e.g., `0.5 cpu` ↔ `1.0Gi` mem). Off-combo configs fail deployment.
- **Building ARM64 by accident.** `docker build` on Apple Silicon defaults to linux/arm64. The container fails to start in ACA (`exec format error`). Use `docker build --platform linux/amd64` or `az acr build --platform linux/amd64`.
- **Geo-replication misunderstood.** Geo-replication makes a single ACR appear in multiple regions with the same login server — *not* two separate ACRs. Pulling from `acmecrprod.azurecr.io` in westus3 hits the local replica automatically. You don't change image references between regions.
- **ACR Tasks running out of budget.** Tasks bill per CPU-minute. A misconfigured base-image-update trigger can fire a hundred builds an hour. Set alerts.
- **ACI for production.** ACI is great for jobs; for steady-state services it's pricier, has no autoscaling, and harder to integrate with VNet/observability than Container Apps. Use Container Apps.
- **Container Apps Jobs vs ACI confusion.** Jobs is the modern replacement for "I want a container to run on a schedule." ACI is the right call for one-off interactive things from `az` CLI.
- **Image vulnerability backlog.** Defender for Containers shows you 800 CVEs; you ignore them; nothing improves. Set policy: no `Critical` CVEs in production; rebuild base images monthly.

## 🎯 Key Takeaways

- **ACR (Premium) is the registry.** Disable admin user, enable Private Link, set retention policies, attach Defender for image scanning.
- **ACI is for ephemeral / one-off containers.** Long-lived services belong in Container Apps (or AKS).
- **Container Apps + ACR with MI auth (`AcrPull`)** is the standard managed-container pattern in 2026. No registry passwords anywhere.
- **Immutable tags (commit SHA, semver), not `latest`.** Deterministic deploys and clean rollbacks.
- **`az acr build` builds without a local Docker daemon** — great in CI runners or developer workflows when Docker Desktop is overkill.

*← [prev](./11_edge_and_traffic.md) | [next → 13_observability.md](./13_observability.md)*
