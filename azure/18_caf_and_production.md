# 18 — Cloud Adoption Framework + Production Patterns

> **Goal:** Wrap the course with the "senior engineer" layer — Microsoft's Cloud Adoption Framework, landing zones, governance via Policy/Blueprints, hub-spoke topologies, and disaster recovery — so you can architect, not just deploy.

## 1. Cloud Adoption Framework (CAF) — the org-level mental model

Microsoft's **Cloud Adoption Framework** is a prescriptive guide for how an enterprise adopts Azure. It is *not* a product; it's a methodology with patterns, reference architectures, and templates. Eight methodologies:

1. **Strategy** — business motivation, outcomes.
2. **Plan** — digital estate, workloads, skills.
3. **Ready** — environment foundation (landing zones).
4. **Adopt** — migrate or innovate.
5. **Govern** — policies, cost, security baselines.
6. **Manage** — operational baseline (monitoring, backups, DR).
7. **Secure** — security posture.
8. **Organize** — operating model (CCoE, federated, central).

Engineers most often touch **Ready** (landing zones) and **Govern/Manage/Secure** day-to-day. Read CAF cover-to-cover once; reference it later.

## 2. Landing Zones — the production environment skeleton

A **landing zone** is "a pre-configured subscription you can deploy workloads into safely." It comes with networking, identity integration, monitoring, security, and policies *already in place*. Two CAF reference implementations:

- **Azure Landing Zones (ALZ)** — the canonical Bicep/Terraform reference. Implements the full enterprise-scale design.
- **Sandbox / start-small** — for small teams, a simplified version.

The full ALZ layout looks like this management-group structure:

```
Tenant Root
├── Top-level MG (e.g., "Contoso")
│   ├── Platform
│   │   ├── Management        ← log analytics, automation accounts, monitoring
│   │   ├── Connectivity      ← hub VNet, ExpressRoute/VPN GW, firewall, DNS
│   │   └── Identity          ← Entra Domain Services, on-prem ADDS sync
│   ├── Landing Zones
│   │   ├── Corp              ← internal-only workloads, peered to on-prem
│   │   ├── Online            ← internet-facing workloads
│   │   └── Confidential      ← regulated, additional controls
│   ├── Decommissioned        ← subs being retired
│   └── Sandbox               ← experimentation; loose policy
```

Each MG hosts one or more subscriptions. Policies cascade. **Connectivity** sub holds the hub VNet that everything peers to. **Management** holds the central LAW that every workload sends diagnostics to.

### Hub-spoke topology

The standard network architecture:

```
       ┌─────────────────┐
       │  on-prem DCs    │
       └────────┬────────┘
         ExpressRoute / S2S VPN
                │
       ┌────────▼─────────┐
       │  Hub VNet        │     ← Connectivity sub
       │  - Azure Firewall│
       │  - VPN/ER GW     │
       │  - Bastion       │
       │  - Private DNS   │
       │  - DDoS Std      │
       └───┬─────┬────┬───┘  peerings
           │     │    │
   ┌───────▼┐ ┌──▼──┐ ┌▼──────┐
   │  prod  │ │ dev │ │shared │  Landing-Zone subs (workload spokes)
   └────────┘ └─────┘ └───────┘
```

Every workload VNet peers to the hub; all egress, on-prem traffic, and inter-spoke traffic flows through the hub firewall. This is what every enterprise eventually builds; ALZ ships it.

## 3. Governance — Policy, Blueprints, Locks

### Azure Policy

Declarative rules evaluated at create and on a schedule:

- **Effect** — `Audit`, `Deny`, `DeployIfNotExists` (auto-remediate), `AuditIfNotExists`, `Modify`, `Append`.
- **Scope** — assign at MG, sub, or RG. Cascades to children.
- **Built-ins** — Microsoft ships hundreds; covered by initiatives like *Azure Security Benchmark* and *Microsoft Cloud Security Benchmark*.

```bash
# Assign "Storage accounts should use HTTPS only" as Deny at the prod MG.
az policy assignment create \
  --name require-https-storage \
  --policy "404c3081-a854-4457-ae30-26a93ef643f9" \
  --scope /providers/Microsoft.Management/managementGroups/landingzones-prod \
  --enforcement-mode Default
```

Common baseline policies to assign on landing-zone MGs:

- Allowed locations (limit to your regions).
- Required tags (`costcenter`, `owner`).
- Audit/Deny public IP creation on workload subs.
- Audit/Deny key vaults without purge protection.
- Audit storage accounts without `minimumTlsVersion: 1.2`.
- Audit RBAC: no custom-role assignments at sub scope.
- Audit Defender for Cloud plans.
- DeployIfNotExists: diagnostic settings to central LAW.

Bundle policies into **initiatives** (`policySetDefinitions`) for assignment as a group.

### Blueprints (deprecated; use Deployment Stacks)

Azure Blueprints (orchestrated ARM templates + RBAC + policies) is being deprecated in 2026. New work uses **Deployment Stacks** (module 15) + Policy + ALZ Bicep modules. Mention Blueprints in interviews to show you know it's the *deprecated* answer.

### Resource Locks

Two kinds, applied at any scope:

- `CanNotDelete` — prevents deletion. Modification allowed.
- `ReadOnly` — prevents deletion and modification.

```bash
az lock create -g rg-orders-prod -n no-delete --lock-type CanNotDelete
```

Apply `CanNotDelete` on:

- Production resource groups.
- Key Vaults (anywhere).
- Storage accounts holding important data.
- Hub-VNet, Firewall, ExpressRoute Gateway.

Locks inherit downward. They're advisory in the sense that anyone with `Microsoft.Authorization/locks/delete` can remove them — combine with RBAC + audit on lock-delete events.

## 4. Production patterns — DR, backups, regions

### Disaster Recovery levels

For each workload, define **RTO** (recovery time objective) and **RPO** (recovery point objective). Common patterns:

| Strategy | RTO | RPO | Cost | Notes |
|----------|-----|-----|------|-------|
| **Backup only** | hours-days | hours | $ | Rebuild from scratch in another region |
| **Pilot Light** | tens of minutes | minutes | $$ | Minimal infra always running in DR region |
| **Warm Standby** | minutes | seconds | $$$ | Half-capacity infra ready in DR region |
| **Active-Active** | seconds | seconds | $$$$ | Full capacity, both regions serving |

Pick per workload. Most "tier-1" production services land on Warm Standby or Active-Active; tier-3 internal tools accept Backup Only.

### Backup strategy

- **Azure Backup** — VMs, files/folders, SQL on VM, file shares. PITR, GFS retention, immutable backups.
- **Azure Site Recovery (ASR)** — replicates whole VMs across regions for DR failover.
- **PaaS-native backups** — Azure SQL PITR (7-35d) + LTR, Cosmos continuous backup, Blob soft-delete + versioning + point-in-time restore.

Verify restores at least quarterly. A backup you've never restored is a backup that doesn't work.

### Region selection and pairing

- Pick a primary region close to your users.
- Pick a secondary region that's a **paired region** (module 01) so Microsoft staggers updates and runs GZRS replication automatically.
- Beware: not every service is in every region. Validate before architecting.

### Failover mechanics by service

| Service | Failover mechanism |
|---------|--------------------|
| Front Door | Health-probe based, automatic, near-instant |
| Traffic Manager | DNS TTL-bound (60s typical) |
| Azure SQL DB | Failover groups (auto policy + manual override) |
| Cosmos DB | Multi-region writes (automatic) or manual failover priority |
| Storage GZRS / GRS | Customer-initiated failover (irrevocable!), or RA-GRS for read-only secondary |
| App Service | Deploy in both regions, front with Front Door |
| Key Vault | Replicated automatically by Microsoft; secondary read endpoint |

### A production-grade single-workload deployment (composite)

For a tier-1 web app:

- 2 regions (`eastus2`, `westus3` or `westeurope`).
- App Service Premium v3 in both, zone-redundant.
- Front Door Premium with Private Link origins to both App Services + WAF.
- Azure SQL Database with failover group, zone-redundant in each region.
- Cosmos DB with multi-region writes, Session consistency.
- Storage GZRS with versioning + soft delete.
- Key Vault per region (replication is automatic for Microsoft-managed; for CMK use multi-region keys).
- Service Bus Premium with geo-DR pairing (1-way replication).
- All resources in landing-zone subs; hub-spoke peered; egress through Azure Firewall in hub.
- Diagnostic settings → central LAW in Management sub.
- Defender for Cloud + Sentinel on the LAW.
- Bicep + GHA OIDC pipeline; promotion through dev/test/staging/prod environments.
- Budget alerts at sub level; tags enforced via Policy.

That's a "senior engineer can speak to this without notes" architecture.

## 5. Common Mistakes & Gotchas

- **Trying to do CAF in a weekend.** It's a multi-month organizational change. Apply CAF *patterns* incrementally; don't try to "deploy CAF."
- **Single-tenant deployment, no MGs.** Works at small scale; doesn't at 50 subs. Bring MGs in early so policy/RBAC inheritance is meaningful.
- **Workload subs writing to local LAW instead of central.** Telemetry fragmentation. Force diagnostic settings to a central Management-sub LAW via DeployIfNotExists policy.
- **Hub bypass.** A spoke that egresses directly to the internet bypasses the central firewall. UDRs + NSGs must enforce hub-egress.
- **Firewall rules blocking platform health probes.** Standard LB probes from `168.63.129.16`; AppGw/AFD probes from specific ranges. Allowlist them or break health.
- **Forgetting to test DR failover.** Drills uncover misconfigured DNS TTLs, missing failover-group settings, secrets that only exist in primary Key Vault. Test annually at minimum.
- **GZRS failover is irrevocable.** Once you trigger storage failover to the secondary, the primary becomes a fresh LRS-only account. Recovery is rebuild-from-scratch. Confirm the regional outage is real before failing over.
- **Locks misunderstood as security.** Locks prevent *accidental* deletion. A motivated insider with `Owner` can remove them. Combine with RBAC + alerts on lock-removal events.
- **Policy effects misapplied.** `Deny` policies on dev sub block experimentation; `Audit` policies on prod let drift accumulate. Tier policy strictness with the environment.
- **DeployIfNotExists policies failing silently** because the managed identity attached to the policy assignment doesn't have permission to deploy the corrective resource. Always validate the policy's MI has appropriate roles.
- **Hub-spoke without DDoS Protection** on the hub when you have public ingress. DDoS Standard is pricey per VNet — apply only on hub VNets that host public IPs.
- **Connectivity sub without redundant gateways.** ExpressRoute gateway is a single point of failure if not deployed in zone-redundant mode (`Standard` SKU, multi-AZ).
- **ALZ adoption all-or-nothing.** ALZ has dozens of modules; you can adopt incrementally (MG hierarchy first, then policies, then connectivity). Don't be intimidated.
- **Backup tested by restoring to the same VM** — proves nothing. Always restore to a clean target, verify integrity, document the runbook.
- **Cost not budgeted at landing-zone level.** Each landing-zone sub should have a default budget + alerts as part of its bootstrap. Without it, runaway spend hits Finance before you.
- **No CMDB / inventory.** Azure Resource Graph queries are your CMDB. Have saved KQL queries for "every prod resource without diagnostic settings," "every key vault without purge protection," "every VM without backup," etc. Run weekly.

## 🎯 Key Takeaways

- **CAF and ALZ are the senior-engineer vocabulary.** Even at small scale, MGs + landing-zone subs + central management + hub-spoke is the right *direction* to architect toward.
- **Governance is policy + RBAC + locks + tags.** They compose; each alone is insufficient. Enforce at MG scope so workloads inherit.
- **Hub-spoke with central firewall, central DNS, central LAW** is the de facto enterprise pattern. Know it cold.
- **Define RTO/RPO per workload and design the right DR posture for each.** Active-active for tier-1, backup-only for tier-3. Test annually.
- **Production-readiness is a checklist** — identity, network, secrets, observability, IaC, CI/CD, cost, governance, DR. We've covered all of them. The job is now stitching them into your specific organization's reality.

---

Congratulations: you've reached the end. If you've done the modules hands-on, you can now design, deploy, secure, observe, and operate non-trivial Azure workloads. The remaining growth happens on real systems with real users — there is no substitute. Go build something.

*← [prev](./17_cost_management.md) | [↑ roadmap](./00_roadmap.md)*
