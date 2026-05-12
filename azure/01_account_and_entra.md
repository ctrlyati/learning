# 01 — Accounts, Subscriptions, and the Azure Control Plane

> **Goal:** Understand the hierarchy (tenant → management group → subscription → resource group → resource) so that every later module has somewhere to put things — and so you never accidentally deploy prod resources into your dev sub.

## 1. The Tenant — your slice of Microsoft Entra ID

A **tenant** is a dedicated instance of Microsoft Entra ID (formerly Azure AD). It is the *identity* container, not a resource container. Every Azure account, every Microsoft 365 organization, every GitHub Enterprise org that federates with Microsoft — they each live inside exactly one tenant. The tenant has a globally unique ID (a GUID) and a primary domain like `contoso.onmicrosoft.com`.

The crucial mental model: **the tenant is identity; subscriptions are billing-and-resources**. A single tenant can own zero or many subscriptions. A subscription cannot exist without a tenant. We will explore Entra ID itself in module 02 — this module is about the resource hierarchy that hangs off it.

See which tenant you're logged into:

```bash
az account show --query "{tenantId:tenantId, name:user.name, subscription:name}" --output table
```

If you have access to multiple tenants (common at consultancies and enterprises), `az login --tenant <tenant-id-or-domain>` re-anchors your CLI session.

## 2. Subscriptions — the billing and quota boundary

A **subscription** is what you actually deploy resources into. It is:

- A **billing boundary** — every resource cost rolls up to one subscription, which has one payment method.
- A **quota boundary** — vCPU limits, public IP limits, etc. are per-region-per-subscription.
- An **RBAC scope** — you can assign roles at the subscription level and they cascade down.
- A **security/governance boundary** — Azure Policy, Defender for Cloud, and many other services configure per-sub.

Subscriptions are *cheap to create* and the standard production pattern is **many subs, not one giant sub**. A typical enterprise layout is one sub per environment per workload (`acme-payments-prod`, `acme-payments-nonprod`, `acme-data-prod`, …). This gives you blast-radius isolation, clear cost attribution, and quota headroom.

List subscriptions you can see and pin a default:

```bash
az account list --output table
az account set --subscription "<sub-id-or-name>"
```

A subscription has exactly one **home tenant** — the Entra tenant whose identities can be granted RBAC into it. You can move a sub between tenants (rare; "tenant transfer") but it is a heavyweight operation that breaks all existing role assignments.

## 3. Management Groups — the grouping layer above subscriptions

Once you have more than 3-5 subscriptions, you want a way to apply governance (Azure Policy, RBAC) across many of them at once. That's a **management group**. MGs form a tree, with a single **root MG** ("Tenant Root Group") at the top and arbitrary child MGs underneath. Subscriptions are leaves.

```
Tenant Root Group (auto)
├── platform-mg
│   ├── connectivity-sub
│   └── identity-sub
├── landingzones-mg
│   ├── corp-mg
│   │   ├── acme-payments-prod
│   │   └── acme-payments-nonprod
│   └── online-mg
│       └── acme-public-site
└── decommissioned-mg
```

Create one and move a sub:

```bash
az account management-group create --name landingzones-mg --display-name "Landing Zones"
az account management-group subscription add \
  --name landingzones-mg \
  --subscription "<sub-id>"
```

A policy or role assignment at `landingzones-mg` is inherited by every child MG and every subscription beneath it. This is the foundation of Cloud Adoption Framework landing zones (module 18) and the reason MGs matter even at small scale.

## 4. Resource Groups — your day-to-day unit of work

Inside a subscription, every resource lives in exactly one **resource group**. The RG is:

- A **logical container**, not a network or security boundary. (Common misconception from ex-AWS folks.)
- A **lifecycle unit** — `az group delete --name foo --yes` deletes everything inside, recursively. This is the most useful and most dangerous command in Azure.
- A **deployment unit** — Bicep/ARM deployments target an RG by default.
- An **RBAC scope** — a role at the RG level applies to every resource in it.
- A **tag inheritance source** — with Azure Policy, you can force RG tags down to resources.

Create one in a region:

```bash
az group create --name rg-learn-azure --location eastus2 --tags owner=yati env=sandbox
```

Note `--location` on the RG. That's the *metadata home* of the RG (where ARM stores the deployment record), not a constraint on resources. You can put a West Europe VM in an East US RG — Azure will let you. Whether you *should* is a different question (latency, cost, governance).

### Resource group naming convention (adopt one early)

A sane scheme: `rg-<workload>-<env>-<region>`, e.g. `rg-payments-prod-eastus2`. Resources inside follow: `<service-abbrev>-<workload>-<env>-<region>-<seq>` like `vm-payments-prod-eastus2-01`, `kv-payments-prod-eastus2`, `st<workload><env><region>01` (storage names can't have dashes and are length-limited — see module 06). Microsoft publishes a recommended [naming-and-tagging cheat-sheet](https://learn.microsoft.com/azure/cloud-adoption-framework/ready/azure-best-practices/resource-naming) — copy it.

## 5. Regions and availability zones — the physical layer

A **region** is a set of datacenters within a metro area — `eastus`, `westeurope`, `southeastasia`, `centralindia`, etc. Inside many (not all) regions, Azure has 3 physically separate **availability zones** (AZs) — each is one or more datacenters with independent power, cooling, and network.

```bash
az account list-locations --query "[].{name:name, displayName:displayName}" --output table
```

Three concepts every Azure engineer must distinguish:

- **Regional services** — exist once per region (e.g., most network resources). No zone awareness.
- **Zonal services** — you pick a zone at create time (e.g., a VM in zone 2). Failure of zone 2 = your VM is down.
- **Zone-redundant services** — Azure runs the service across all 3 zones for you (e.g., zone-redundant Storage, Premium SQL with zone-redundancy enabled).

You'll see this in disks, public IPs, load balancers, SQL, AKS — every infra decision asks "regional, zonal, or zone-redundant?"

### Paired regions (Azure-specific)

Azure pre-pairs most regions: `eastus ↔ westus`, `northeurope ↔ westeurope`, `centralus ↔ eastus2`, etc. The pairing is used by Azure for:

- **GRS storage replication** — a GRS-redundant storage account is asynchronously copied to its pair. You don't choose the destination.
- **Staged platform updates** — Microsoft updates one region of a pair at a time so an outage doesn't hit both.
- **Service Health communications** for paired regions.

You cannot change a region's pair. This is the biggest mental shift from AWS, where you choose every replication destination explicitly. Check yours:

```bash
az account list-locations \
  --query "[?metadata.pairedRegion!=null].{region:name, pairedWith:metadata.pairedRegion[0].name}" \
  --output table
```

## 6. The control plane — portal, CLI, PowerShell, SDKs

Every Azure operation hits **Azure Resource Manager (ARM)**, a REST API at `management.azure.com`. The portal, `az` CLI, `Az` PowerShell module, Bicep, Terraform, and the SDKs are all just clients of ARM. This is liberating: anything you can do in the portal you can script.

The four front-doors:

- **Azure Portal** (`portal.azure.com`) — fine for browsing, terrible for repeatability. Use it to *learn*, never to *deploy production*.
- **`az` CLI** — cross-platform, the default for this course. Subcommands are organized by service: `az vm`, `az storage`, `az network`. Universal flags: `--output table|json|jsonc|tsv|yaml`, `--query <JMESPath>`, `--subscription <id>`.
- **`Az` PowerShell** — feature-equivalent, idiomatic PowerShell. Pick CLI *or* PowerShell and stop debating.
- **SDKs** — `@azure/*` (Node), `azure-*` (Python), `Azure.*` (.NET), `azure-sdk-for-go`. All built on the same ARM API.

A practical CLI tip: every command supports `--output jsonc` (colorized JSON) for exploration, `--query` for filtering, and `--debug` when something inexplicable happens.

```bash
# List all VMs across all subs, with their power state, as a table.
az graph query -q "Resources | where type =~ 'microsoft.compute/virtualmachines'
  | project name, location, sub=subscriptionId, state=tostring(properties.extended.instanceView.powerState.displayStatus)" \
  --output table
```

(That uses **Azure Resource Graph**, a KQL-queryable index of every resource you can see across all subs. It is criminally underused — get familiar with it.)

## 4. Practical Application — bootstrap a sandbox correctly

Let's set up a clean sandbox you'll use for the rest of the course.

```bash
# 1. Login and pick the sandbox sub.
az login
az account set --subscription "<sandbox-sub-id>"

# 2. Create a learning RG.
az group create \
  --name rg-learn-azure \
  --location eastus2 \
  --tags owner="yati@amityrobotics.com" purpose="learning" autodelete="2026-06-30"

# 3. Confirm.
az group show --name rg-learn-azure --output table

# 4. Set the default RG and location so you can stop typing them.
az configure --defaults group=rg-learn-azure location=eastus2

# 5. Verify defaults.
az configure --list-defaults
```

Now create a trivial resource to prove plumbing works — a storage account (covered properly in module 06):

```bash
az storage account create \
  --name stlearnyati$RANDOM \
  --sku Standard_LRS \
  --kind StorageV2
```

Equivalent Bicep (`main.bicep`):

```bicep
targetScope = 'resourceGroup'

@description('Globally unique storage account name.')
param storageName string = 'stlearn${uniqueString(resourceGroup().id)}'
param location string = resourceGroup().location

resource sa 'Microsoft.Storage/storageAccounts@2023-05-01' = {
  name: storageName
  location: location
  sku: { name: 'Standard_LRS' }
  kind: 'StorageV2'
  properties: {
    minimumTlsVersion: 'TLS1_2'
    allowBlobPublicAccess: false
  }
}

output id string = sa.id
```

Deploy:

```bash
az deployment group create \
  --resource-group rg-learn-azure \
  --template-file main.bicep
```

That's the full lifecycle of an Azure resource: `az login` → pick scope → declare resource → deploy → query. Every subsequent module is a variant on these five steps.

## 5. Common Mistakes & Gotchas

- **Confusing tenant with subscription.** They are different objects. A user can be a member of multiple tenants. A subscription belongs to one tenant. Tenant transfer is rare and disruptive — *do not* casually move subs between tenants without reading the docs.
- **Deploying real workloads at the management-group scope.** Resources do not live at MG scope. Only policies and role assignments do. New engineers occasionally try `az deployment mg create` for a VM and are confused.
- **RG region == resource region.** It doesn't. The RG location is metadata only. You can (and sometimes must) put resources of different regions in the same RG. The downside: this makes "delete everything in eastus2" by RG impossible; you need tags or Resource Graph queries.
- **`az group delete` is permanent.** No undo. No recycle bin. Some resources (Key Vaults, Storage soft-deletes, Recovery Services Vaults) have *resource-level* soft delete, but the RG itself is gone. Run `--dry-run` mental check or use `az group delete --no-wait --yes --name X` only when sure.
- **Quota surprises.** A fresh subscription often has a default vCPU quota of 10 in a given region. You'll create 3 medium VMs, try to scale, and get `OperationNotAllowed`. Increase via `az vm list-usage` and a portal-only support request. Plan ahead.
- **Region availability.** Not every service is in every region. New services often debut in `eastus2`, `westus2`, `westeurope`, `northeurope`. Check with `az provider list --query "[?namespace=='Microsoft.App'].resourceTypes[?resourceType=='containerApps'].locations" -o tsv`. Cosmos DB serverless, certain VM SKUs, and Confidential Computing are often the laggards.
- **Classic vs ARM.** "Azure Service Management" (ASM, aka "classic") is the pre-2014 deployment model. It is deprecated. If you ever see "Classic VM" or "Classic Storage" in the portal, you are looking at legacy. Don't build new things there; migrate.
- **Subscription name vs ID.** The ID is the GUID — stable, scriptable. The name is human-friendly but can be changed by admins. *Pin scripts to subscription IDs*.
- **Default subscription drift.** `az account set` only affects your local context. In CI/CD, *always* pass `--subscription` explicitly — never trust an implicit default.

## 🎯 Key Takeaways

- **Hierarchy is non-negotiable:** Tenant → Management Group → Subscription → Resource Group → Resource. Every later module slots into this tree.
- **The tenant is identity, the subscription is billing-and-resources.** They are separate concepts and conflating them will burn you.
- **Resource groups are deployment, lifecycle, and RBAC units** — adopt a naming + tagging convention before you have 200 of them.
- **Regions ≠ AZs ≠ paired regions.** Three different concepts; senior engineers can speak to all three without hesitation.
- **Master `az` CLI early.** Portal is for exploration. CLI + Bicep is for delivery. Resource Graph is your cross-sub Swiss Army knife.

*← [prev](./00_roadmap.md) | [next → 02_entra_id.md](./02_entra_id.md)*
