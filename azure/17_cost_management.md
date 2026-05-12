# 17 — Cost Management

> **Goal:** Read Azure bills, attribute spend, buy reservations and savings plans intelligently, and avoid the half-dozen mistakes that turn a healthy Azure bill into a budget incident.

## 1. Pricing fundamentals — pay-as-you-go, then optimize

Most Azure services bill on consumption: vCPU-hour, GB-month, request count, GB egress. Default is **pay-as-you-go (PAYG)** — no commitment, no discount. From there you have three discount levers:

- **Reservations** — 1- or 3-year commit to a specific SKU in a specific region (or shared). ~30-60% off PAYG. Best for steady, predictable compute and databases.
- **Savings plans for compute** — 1- or 3-year commit to a *dollar amount per hour* of compute. ~15-30% off. More flexible than reservations (covers any region, any size in a family).
- **Spot VMs** — bid on unused capacity. Up to ~90% off. Can be evicted at any time. Batch / fault-tolerant workloads only.

Plus:

- **Azure Hybrid Benefit (AHB)** — apply existing Windows Server / SQL Server licenses with Software Assurance to reduce Azure compute/DB costs. Huge for migrations.
- **Dev/Test pricing** — Visual Studio subscribers get reduced rates on dev/test subs. Switch subscription offer type if eligible.
- **Free tier** — limited free amounts for ~25 services indefinitely. Useful for tiny dev workloads.

## 2. Cost Management + Billing — the tooling

The portal feature is **Cost Management + Billing**, scoped per billing account / sub / RG. Useful surfaces:

- **Cost Analysis** — slice spend by service, resource group, tag, region, time. Save views.
- **Budgets** — alerts when spend hits 50/80/100% of a target. Optional action group integration.
- **Cost alerts** — anomaly detection (e.g., daily spend jumped 3x).
- **Advisor (cost recommendations)** — Azure suggests right-sizing, idle resources, reservation purchases.
- **Exports** — daily CSV/Parquet export to Storage for downstream BI (Power BI, Synapse).

CLI access:

```bash
# Today's running cost for a subscription
az consumption usage list --start-date $(date -u -d 'yesterday' +%Y-%m-%d) --end-date $(date -u +%Y-%m-%d) -o table | head

# Create a budget (300 USD/month, alert at 80%)
az consumption budget create \
  --resource-group rg-orders-prod \
  --budget-name budget-orders-prod \
  --amount 300 --time-grain Monthly \
  --start-date 2026-05-01 --end-date 2027-04-30 \
  --notification 'enabled=true threshold=80 contactEmails=cloud-team@acme.com operator=GreaterThan'
```

### Tags and attribution

Tags are the *only* practical way to attribute spend across RGs/subs to teams/products. Mandatory tags to adopt before you have 100 resources:

- `costcenter` — billing chargeback target.
- `owner` — team or person email.
- `env` — dev/test/staging/prod.
- `workload` — application/product name.
- `expires` (optional) — auto-cleanup date for ephemeral resources.

Enforce via Azure Policy (modules 15, 18): "audit if tag missing," then escalate to "deny if missing."

```bash
# Tag inheritance from RG → resources (set on the RG; policy applies down)
az policy assignment create --name require-tags \
  --policy "ResourceGroups should have tags" \
  --scope /subscriptions/$SUB_ID
```

## 3. Reservations and Savings Plans in depth

### When to buy what

| Workload | Best discount lever |
|----------|--------------------|
| Steady VMs of a specific size, single region | **Reserved VM Instance** |
| Steady VMs but I might resize / move regions | **Compute Savings Plan** |
| Production Azure SQL DB / Cosmos / Postgres | **Reserved Capacity** for that service |
| App Service plan you'll keep > 1 year | **Reserved App Service** |
| Storage (Blob Reserved Capacity, ADLS Reserved) | **Storage Reservation** |
| Spiky / unpredictable workloads | **No reservation; consider Spot for parts** |

Reservations and Savings Plans:

- **Scope** — single sub vs *shared* (across the billing account). **Default to Shared** for flexibility.
- **Payment** — upfront (best discount) or monthly (cash flow). Same total cost.
- **Term** — 1y or 3y. 3y costs ~10% more discount but locks longer.
- **Exchange / cancel** — reservations are *exchangeable* (swap into different SKU); Savings Plans are not.

### Estimating reservations correctly

The trap: buying a reservation matching today's footprint, then right-sizing the workload and stranding the reservation.

Process:

1. Run the workload at PAYG for ~30-90 days.
2. Right-size based on observed CPU/memory (Advisor will tell you).
3. *Then* buy a reservation for the steady-state baseline (perhaps 70% of peak), leaving the burst at PAYG or scale-out.

### Spot VMs for batch

```bash
az vm create -g rg-batch -n vm-batch-01 \
  --image Ubuntu2204 --size Standard_D8s_v5 \
  --priority Spot --max-price -1 --eviction-policy Deallocate \
  --admin-username azureuser --ssh-key-values @~/.ssh/id_ed25519.pub
```

`--max-price -1` = pay up to the on-demand price (best availability). Eviction policy: `Deallocate` (preserve disks) or `Delete`. Run with eviction-aware orchestration (Batch service, AKS spot node pools, VMSS).

## 4. Practical Application — a monthly cost-hygiene routine

The skill that pays your salary. A 30-minute, monthly checklist:

### 1. Top-spenders review

```bash
# Top 20 RGs by cost this month (requires Cost Management API or Azure Workbook)
az costmanagement query \
  --type ActualCost --timeframe MonthToDate \
  --scope "/subscriptions/$SUB" \
  --dataset-aggregation '{"totalCost":{"name":"Cost","function":"Sum"}}' \
  --dataset-grouping '[{"type":"Dimension","name":"ResourceGroupName"}]' \
  --output table
```

Eyeball: are the top 20 RGs the ones you expect? Surprises = investigate.

### 2. Idle and orphaned resources

Common waste:

- **Unattached managed disks.** Old VMs deleted, disks orphaned, billed forever.
  ```bash
  az disk list --query "[?managedBy==null].{name:name, rg:resourceGroup, sizeGB:diskSizeGB, sku:sku.name}" -o table
  ```
- **Stopped VMs that aren't deallocated.** Stopped = still billed. Use `az vm deallocate`.
- **Public IPs not associated.** Billed regardless.
- **App Service Plans without apps.** Plans bill per hour even with zero apps.
- **Premium SSDs on dev VMs.** Often Standard SSD would do.
- **Old snapshots and images.**
- **Cosmos DB with high autoscale max but low usage.** Max determines bill.
- **Log Analytics over-ingesting.** A noisy resource can 10x ingest overnight.

### 3. Reservation utilization

```bash
az reservations reservation-order list -o table
# Then look at utilization in the portal — anything <90% is wasted.
```

If utilization drops, exchange the reservation into a SKU you're actually using.

### 4. Advisor recommendations

```bash
az advisor recommendation list --category Cost -o table | head -30
```

Acknowledge or implement the top ones. Don't accumulate ignored recommendations — they become noise.

### 5. Budget alerts

Confirm every prod sub has a budget. If you have 20 subs, automate via Bicep:

```bicep
targetScope = 'subscription'
resource budget 'Microsoft.Consumption/budgets@2024-08-01' = {
  name: 'budget-${subscription().displayName}'
  properties: {
    amount: 5000
    category: 'Cost'
    timeGrain: 'Monthly'
    timePeriod: { startDate: '2026-01-01' }
    notifications: {
      thresholdReached_80: {
        enabled: true, operator: 'GreaterThan', threshold: 80,
        contactEmails: ['cloud-team@acme.com'], thresholdType: 'Actual'
      }
      forecast_100: {
        enabled: true, operator: 'GreaterThan', threshold: 100,
        contactEmails: ['cloud-team@acme.com'], thresholdType: 'Forecasted'
      }
    }
  }
}
```

### 6. Tag compliance

KQL on Resource Graph:

```kql
resources
| where subscriptionId == '<sub>'
| where tags['costcenter'] == '' or isnull(tags['costcenter'])
| project name, type, resourceGroup
| order by type asc
```

Tag the offenders or escalate to Azure Policy enforcement.

## 5. Common Mistakes & Gotchas

- **No tags from day one.** Three months in you have 1500 resources with no `costcenter`. Backfilling is painful. Adopt tags + policy *before* the second deployment.
- **Buying reservations for "always-on" VMs that turn out to be turned off nights/weekends.** Reservations charge whether the VM runs or not. Match commitments to *baseline* steady state, not peak.
- **Stopped (not deallocated) VMs billed at full rate.** Portal shows "Stopped" — that doesn't mean free. Always `Deallocate`.
- **Premium disks on dev VMs.** ~3x cost of Standard SSD. Use Standard SSD for dev/test.
- **Log Analytics ingest spiral.** A misconfigured Postgres or App Service can flood logs. Set ingest cap or daily-cap on the LAW and alert.
- **NAT Gateway and Premium AFD egress.** Egress to the internet (especially through NAT GW) is silent and expensive. Cache, CDN, region-pin services.
- **Cosmos DB max-RU autoscale set high "for safety."** You pay 10% of the max even at near-zero load. Set max realistically.
- **Premium Service Bus / Event Hubs over-provisioned.** Premium SKU bills per-MU/TU per hour, 24/7. Right-size with autoscale where supported.
- **App Service Premium v2 vs v3.** v3 is cheaper at similar performance. Migrate.
- **Container Apps min-replicas > 0** for low-traffic services. Defeats scale-to-zero. Use min 0 unless cold-start is unacceptable.
- **Public IPs Standard SKU billed per hour.** Each one. 100 leftover prod IPs = real money.
- **Dev/Test sub on PAYG offer type.** Switch eligible subs to "Dev/Test" offer to halve some prices.
- **Cross-region data transfer.** Replicating storage cross-region, GRS, Front Door egress — these line items surprise. Model them.
- **Misattributed bill due to no tags.** "Why does eastus2 spend $40k? Which team?" → no answer → can't optimize. Tagging is the precondition for FinOps.
- **Reservations bought in wrong scope.** Single-sub reservation locked to a sub that gets retired → reservation stranded. Default to shared scope.
- **Hybrid Benefit not applied.** Free money left on the table. Verify on every Windows VM and SQL DB at create time.

## 🎯 Key Takeaways

- **Tags + budgets + Cost Analysis = the foundation.** Without tagging discipline, attribution is guesswork and optimization is impossible.
- **Buy reservations and savings plans after observing real usage**, not on day one. Match commitments to steady-state baselines.
- **Run a monthly hygiene routine**: orphaned disks, stopped-but-billed VMs, idle plans, ingest spikes, Advisor recommendations. Calendar invite, 30 minutes.
- **The biggest wastes are silent**: Log Analytics ingest, unattached disks, oversized Cosmos RUs, premium-disk dev VMs, cross-region egress. Specifically hunt these.
- **Cost management is a senior-engineer skill** — show financial fluency and you become the cloud lead. Show ignorance and you're the engineer who blew the budget.

*← [prev](./16_cicd.md) | [next → 18_caf_and_production.md](./18_caf_and_production.md)*
