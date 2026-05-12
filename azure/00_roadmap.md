# 00 — Azure Deep-Dive: Roadmap

> **Goal:** Take a working developer from "I have an Azure account" to "I can design, deploy, secure, and operate production workloads on Azure" — with enough depth to hold your own in a senior cloud-engineer interview.

This course is opinionated. It assumes you already write code for a living and you want Azure as a *durable* tool, not a certification badge. We optimize for: mental models you can reuse, `az` CLI muscle memory, Bicep over portal-clicking, and the gotchas that only show up at 2 a.m.

---

## Module Table

| #  | Module | Why it matters |
|----|--------|----------------|
| 01 | [Accounts, subscriptions, and the Azure control plane](./01_account_and_entra.md) | Everything else is scoped under a tenant + subscription. Get this wrong and RBAC is chaos. |
| 02 | [Microsoft Entra ID (formerly Azure AD)](./02_entra_id.md) | Identity is the new perimeter. Entra is the substrate for every Azure auth decision. |
| 03 | [Azure RBAC](./03_rbac.md) | Who can do what, where. Built on Entra. Scope hierarchy is the single most-asked design question. |
| 04 | [Networking: VNets, NSGs, Private Link](./04_networking.md) | If you can't draw the network, you can't debug it. |
| 05 | [Virtual Machines and VMSS](./05_vms.md) | Still the workhorse. Disks, sizing, scale sets, zones. |
| 06 | [Azure Storage](./06_storage.md) | Blob/File/Queue/Table. Redundancy, tiers, SAS, lifecycle. |
| 07 | [Azure SQL and Cosmos DB](./07_sql_and_cosmos.md) | The two flagship data services. Pick the right one or pay forever. |
| 08 | [Azure Functions and serverless](./08_functions_serverless.md) | Triggers, bindings, Durable Functions, plans. The lambda-equivalent. |
| 09 | [App Service and Container Apps](./09_app_service_container_apps.md) | PaaS web hosting + serverless containers. Slots, scaling, deployment. |
| 10 | [Service Bus, Event Grid, Event Hubs](./10_messaging.md) | Three messaging services. Knowing which to pick is the test. |
| 11 | [Front Door, App Gateway, Traffic Manager, DNS](./11_edge_and_traffic.md) | Edge, load balancing, DNS — the user-facing layer. |
| 12 | [Containers: ACR, ACI, Container Apps](./12_containers.md) | The container story minus full Kubernetes. |
| 13 | [Observability: Monitor, Log Analytics, App Insights, KQL](./13_observability.md) | If it isn't observable, it doesn't exist. |
| 14 | [Security: Key Vault, Defender, Sentinel](./14_security.md) | Secrets, posture, SIEM — the security trio. |
| 15 | [Infrastructure as Code: ARM, Bicep, Terraform](./15_iac.md) | Portal-clicked infra is technical debt the minute it exists. |
| 16 | [CI/CD: Azure DevOps and GitHub Actions with OIDC](./16_cicd.md) | Pipelines + federated identity. Stop storing service-principal secrets. |
| 17 | [Cost management](./17_cost_management.md) | The skill that gets you promoted (and not fired). |
| 18 | [Cloud Adoption Framework + production patterns](./18_caf_and_production.md) | Landing zones, governance, hub-spoke, DR. The "senior engineer" layer. |

---

## Suggested Timeline

One module per day = ~3 weeks. Realistic pacing for someone with a day job:

- **Week 1 — Foundations:** Modules 01-06 (identity, RBAC, networking, VMs, storage).
- **Week 2 — Platform services:** Modules 07-12 (data, serverless, web, messaging, edge, containers).
- **Week 3 — Operations & maturity:** Modules 13-18 (observability, security, IaC, CI/CD, cost, governance).

If you can only do one a day, do it *with hands on a sandbox sub* — not just reading. Modules that are pure conceptual reading (10, 13, 18) can be doubled up; modules with heavy hands-on (04, 05, 15) deserve their own day.

---

## Prerequisites

- **An Azure account.** Either:
  - The [Azure Free Account](https://azure.microsoft.com/free) (USD 200 credit + 12 months of selected free services), or
  - A **Visual Studio subscription** (Pro/Enterprise gives ~USD 50-150/month of Azure credit — *use it*), or
  - A **sandbox subscription** at your employer. If you're learning on a prod tenant, isolate yourself: create a dedicated subscription and a dedicated resource group, and never deploy at management-group scope from a laptop.
- **`az` CLI** installed: `winget install Microsoft.AzureCLI` on Windows, `brew install azure-cli` on macOS, `apt install azure-cli` on Debian/Ubuntu. Verify with `az version`.
- **Bicep** (ships inside the `az` CLI): `az bicep install` then `az bicep version`.
- **An editor with the Bicep extension** (VS Code + the official Bicep extension is the smooth path).
- **A terminal with `jq`** for parsing `az` JSON output: `winget install jqlang.jq` or `apt install jq`.
- Optional but useful: `git`, Docker Desktop (for module 12), and Postman / `curl` for testing HTTP triggers in module 08.

Set a default subscription up front so every command in this course just works:

```bash
az login
az account list --output table
az account set --subscription "<your-sandbox-subscription-id>"
az account show --query "{name:name, id:id, tenant:tenantId}" --output table
```

---

## Core Mental Models (memorize these)

These six show up in every module. If you internalize nothing else from this course, internalize these.

1. **The resource group is the deployment and lifecycle unit.** Tags, RBAC, costs, deletions, and Bicep deployments all naturally pivot on the RG. *Resources can move between RGs, but the operation is rarely free of pain.* Treat an RG like a small product: name it, tag it, document its owner.

2. **Microsoft Entra ID is identity for *everything*** — humans, apps, managed identities, Azure resources, even your Bicep deployments. Anything that authenticates to Azure does so through Entra, and any RBAC role assignment ultimately points at an Entra object ID.

3. **RBAC scope hierarchy: Management Group → Subscription → Resource Group → Resource.** A role assignment at a higher scope is inherited by everything below it. *Inheritance is the source of every "why can this person see prod?" incident.* Always assign at the lowest scope that works.

4. **Managed identities > stored secrets.** If a piece of Azure compute (VM, Function, App Service, Container App, AKS pod) needs to call another Azure service, use a managed identity, not a stored connection string or service principal secret. The platform rotates the token for you and there is nothing to leak.

5. **Bicep is the modern ARM.** ARM JSON templates still exist and they're what Azure executes under the hood, but no human should be writing them by hand in 2026. Bicep compiles 1:1 to ARM, is dramatically more readable, and is first-party Microsoft. Terraform is also fine — pick one and stick with it.

6. **Azure regions ≠ AWS regions in pairing semantics.** Azure has the concept of **paired regions** (e.g., East US ↔ West US, North Europe ↔ West Europe) for platform-managed replication and staged updates. Some services (like GRS storage) default to replicating to the paired region — you don't pick the secondary, Microsoft does. AWS has no equivalent; in AWS you choose every replication target. This trips up every ex-AWS engineer at least once.

---

## External Links Worth Bookmarking

- **[Microsoft Learn](https://learn.microsoft.com/azure/)** — the canonical docs. Better than AWS docs in many places. Start here for every service.
- **[Azure Architecture Center](https://learn.microsoft.com/azure/architecture/)** — reference architectures, well-architected framework, design patterns. Read one diagram a day.
- **[az CLI reference](https://learn.microsoft.com/cli/azure/)** — searchable, complete. Bookmark and use it instead of `--help`.
- **[Bicep playground](https://bicepdemo.z22.web.core.windows.net/)** — paste ARM JSON, get Bicep (and vice versa). Invaluable when learning.
- **[Azure Weekly newsletter](https://azureweekly.info/)** by Chris Pietschmann — the one newsletter to subscribe to.
- **[John Savill's YouTube channel](https://www.youtube.com/@NTFAQGuy)** — the gold standard for Azure deep-dives. His AZ-104/AZ-305 master classes are free and excellent even if you don't sit the exam.

---

## If You've Done the AWS Course

Most concepts map 1:1, but the *philosophies* differ. Cross-reference: [`../aws/00_roadmap.md`](../aws/00_roadmap.md).

| AWS | Azure | Notes |
|-----|-------|-------|
| IAM users + roles | Entra ID + Azure RBAC | Azure splits identity (Entra) from resource permissions (RBAC). AWS conflates them in IAM. |
| AWS account | Subscription | And Azure's **tenant** has no real AWS equivalent. |
| VPC | VNet | Conceptually identical. Subnets are similar. NSGs ≈ Security Groups. |
| S3 | Blob Storage | Blob has hot/cool/cold/archive tiers; LRS/ZRS/GRS for redundancy. |
| Lambda | Azure Functions | Triggers/bindings model is *much* richer in Azure. |
| ECS/Fargate | Container Apps + ACI | Container Apps ≈ Fargate-ish. ACI is for one-off containers. |
| CloudFormation | ARM / Bicep | Bicep is the day-to-day authoring language. |
| CloudWatch | Azure Monitor + Log Analytics + App Insights | Three products you have to wire together. |
| KMS | Key Vault | Key Vault also stores secrets and certificates — broader than KMS. |
| Route 53 | Azure DNS + Traffic Manager + Front Door | Routing is split across multiple services. |
| Cost Explorer | Cost Management + Billing | Same idea, similar UI. |

The biggest cultural difference: **Azure leans on identity (Entra) for nearly every authorization decision**, while AWS leans on resource-policies + IAM roles. Once you grok that, Azure stops feeling weird.

---

## Closing

If you are doing this for professional upskilling — promotion, role change, or a senior cloud engineer interview — the highest-leverage modules are **02 (Entra), 03 (RBAC), 04 (Networking), 13 (Observability), 15 (IaC), and 18 (CAF)**. Those six are where senior people are separated from intermediates. Everything else is table stakes you can look up.

Now: open module 01, log into Azure, and start typing.

*[next → 01_account_and_entra.md](./01_account_and_entra.md)*
