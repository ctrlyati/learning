# 14 — Security: Key Vault, Defender for Cloud, Sentinel

> **Goal:** Operate Azure's three pillars of cloud security — secret management (Key Vault), security posture and workload protection (Defender for Cloud), and SIEM/SOAR (Sentinel) — and revisit managed identities as the connective tissue.

## 1. Key Vault — secrets, keys, certificates

Azure Key Vault is a managed HSM-backed store for three artifact types:

- **Secrets** — arbitrary strings up to 25 KB. The 90% case (DB passwords, API keys, connection strings — to the extent you still have any).
- **Keys** — cryptographic keys (RSA, EC) for sign/verify, encrypt/decrypt, wrap/unwrap. Never exported. Used for CMK on Storage, SQL, Cosmos.
- **Certificates** — X.509 certs with lifecycle management (auto-renewal from supported CAs).

Two tiers: **Standard** (software-protected keys) and **Premium** (HSM-protected with FIPS 140-2 Level 2). Plus the higher-isolation **Azure Managed HSM** for Level 3 / regulated workloads — a separate, more expensive service.

```bash
RG=rg-security-prod
KV=kv-acme-prod-$(openssl rand -hex 3)

az keyvault create -g $RG -n $KV -l eastus2 \
  --sku standard \
  --enable-rbac-authorization true \
  --enable-purge-protection true \
  --retention-days 90 \
  --public-network-access Disabled \
  --default-action Deny --bypass AzureServices
```

Two flags absolutely worth memorizing:

- `--enable-rbac-authorization true` — use Azure RBAC for vault access, not legacy Access Policies. Modern default.
- `--enable-purge-protection true` — soft-deleted secrets cannot be permanently deleted before retention expires. Prevents the "rogue admin nukes the vault" scenario and is required for CMK scenarios. *Cannot be disabled once enabled.*

### Setting and reading secrets

```bash
# Store
az keyvault secret set --vault-name $KV --name stripe-api-key --value 'sk_live_...'

# Read with your own credentials
az keyvault secret show --vault-name $KV --name stripe-api-key --query value -o tsv
```

For applications, *don't* call the CLI — use the SDK with DefaultAzureCredential, which picks up the MI:

```python
from azure.identity import DefaultAzureCredential
from azure.keyvault.secrets import SecretClient

cred = DefaultAzureCredential()
client = SecretClient(f"https://{kv}.vault.azure.net", cred)
secret = client.get_secret("stripe-api-key").value
```

Grant the MI the data-plane role:

```bash
az role assignment create \
  --assignee-object-id $APP_MI_OID \
  --assignee-principal-type ServicePrincipal \
  --role "Key Vault Secrets User" \
  --scope $(az keyvault show -n $KV --query id -o tsv)
```

Roles to know:

| Role | Use case |
|------|----------|
| Key Vault Secrets Officer | Manage secrets (create, delete, list). Humans/admins. |
| Key Vault Secrets User | Read secret values. App identities. |
| Key Vault Certificates Officer / User | Same split for certs. |
| Key Vault Crypto Officer / User | Same split for keys. |
| Key Vault Reader | Metadata only. |
| Key Vault Administrator | Full plane. |

### Certificates with auto-renewal

```bash
# Self-signed cert (for dev) or issuer-managed (DigiCert, GlobalSign).
az keyvault certificate create --vault-name $KV --name acme-com \
  --policy "$(az keyvault certificate get-default-policy)"
```

For real certs, configure an issuer (DigiCert/GlobalSign) on the vault, then create the cert with the issuer name. Key Vault renews ~30 days before expiry. Bind to App Service / App Gateway / Front Door via the vault reference — they fetch the new version automatically.

### Network and key rotation

- **Private Endpoint + `publicNetworkAccess: Disabled`** = vault is reachable only from your VNet. The `AzureServices` bypass is a narrow exception for first-party services and is fine to leave.
- **Customer-managed keys (CMK)** for Storage, SQL TDE, Cosmos, Service Bus, Disk Encryption Sets. Vault holds the key; Azure service unwraps the data-encryption key via the vault. **Purge protection mandatory.**
- **Key rotation policy** — automatic rotation of keys/certs at vault level (Standard+ supports this).

## 2. Defender for Cloud — posture + workload protection

**Microsoft Defender for Cloud (MDC)** is two things bundled:

- **Cloud Security Posture Management (CSPM)** — free tier and Defender CSPM (paid). Scans your configuration against Microsoft Cloud Security Benchmark (MCSB) and other regulatory standards (PCI, ISO 27001, NIST). Outputs a Secure Score.
- **Cloud Workload Protection (CWP)** — paid plans per workload type that add runtime detection: Defender for Servers, Containers, Storage, SQL, App Service, Key Vault, etc.

### Enable basic posture (free) and selected workload plans

```bash
# Enable Defender CSPM (paid) on the subscription
az security pricing create --name CloudPosture --tier Standard

# Enable Defender for Servers Plan 2
az security pricing create --name VirtualMachines --tier Standard --subplan P2

# Enable Defender for SQL
az security pricing create --name SqlServers --tier Standard
```

Workload plans you should turn on for production:

- **Servers** (P2 includes vulnerability assessment + Defender for Endpoint).
- **Containers** (image scanning, runtime protection in AKS/ACA).
- **SQL** (incl. Atlas threat protection, advanced threat protection).
- **Storage** (malware scanning of uploaded blobs).
- **Key Vault** (anomaly detection — strange access patterns).
- **App Service** (anomalous-access detection).

### Secure Score and recommendations

MDC outputs a Secure Score (0-100%) with prioritized recommendations: "Encryption at rest with CMK," "MFA on subscription owners," "Diagnostic settings on storage accounts." Treat the top-10 recommendations as your security backlog.

Automate fixes via Azure Policy (modules 15 and 18). For example, "Storage accounts must have HTTPS-only" is both an MDC recommendation and a built-in policy.

### Regulatory compliance dashboard

MDC maps your environment to PCI DSS, ISO 27001, NIST 800-53, SOC 2, FedRAMP, CIS, etc. Even if you're not formally certified, the dashboards highlight gaps. Senior engineers track this.

## 3. Microsoft Sentinel — SIEM/SOAR on Azure

**Sentinel** is a SIEM (Security Information & Event Management) and SOAR (Security Orchestration, Automation & Response) layered on a Log Analytics Workspace. Same KQL, same data lake; Sentinel adds:

- **Data connectors** — pre-built ingest for Entra ID, M365, Defender, AWS CloudTrail, GCP, Palo Alto, Cisco, GitHub, etc.
- **Analytics rules** — KQL queries that schedule and create incidents.
- **Incidents** — grouped alerts with investigation graph.
- **Hunting queries** — pre-built threat-hunt KQL library.
- **Watchlists** — reference data (VIP users, known-bad IPs).
- **Automation rules + playbooks** — Logic Apps triggered on incident events.

```bash
# Enable Sentinel on an existing LAW.
az sentinel onboarding-state create --workspace-name law-platform-prod --name default
```

A simple analytics rule (KQL → incident):

```kql
// Detect risky sign-ins followed by admin-role activations
let lookback = 1h;
SigninLogs
| where TimeGenerated > ago(lookback)
| where RiskLevelDuringSignIn == "high"
| project SignInTime = TimeGenerated, UserPrincipalName, IPAddress
| join kind=inner (
    AuditLogs
    | where TimeGenerated > ago(lookback)
    | where OperationName == "Add eligible member to role"
) on $left.UserPrincipalName == $right.InitiatedBy.user.userPrincipalName
| project SignInTime, UserPrincipalName, IPAddress, OperationName, TargetResources
```

Save as analytics rule → "create incident on each result."

Sentinel pricing rides on top of LAW ingestion + a per-GB analyzed charge. Plan budget — it's not cheap, but for orgs that need a SIEM the alternative (Splunk, QRadar) is rarely cheaper.

## 4. Practical Application — defense-in-depth checklist for a typical web workload

For a single-region web app (App Service + SQL DB + Storage + Service Bus + Key Vault), the security baseline should look like:

**Identity & access**
- [ ] All admin users have MFA enforced via Conditional Access.
- [ ] No standing "Owner" assignments — use PIM eligible roles.
- [ ] All app-to-app auth is MI-based; no client secrets stored.
- [ ] Entra-only auth on SQL (`azureADOnlyAuthentication: true`), Cosmos, Service Bus, Storage (`allowSharedKeyAccess: false`).
- [ ] Federated credentials for CI/CD (no SP client secrets in pipelines).

**Network**
- [ ] All PaaS services with Private Endpoint + `publicNetworkAccess: Disabled` or restricted.
- [ ] NSGs at subnet level deny by default; explicit allows.
- [ ] No public IPs on VMs; access via Bastion.
- [ ] Front Door / App Gateway with WAF in prevention mode.
- [ ] DDoS Standard at the hub VNet (if cost permits).

**Secrets & data**
- [ ] Key Vault with RBAC auth, purge protection, soft delete 90d.
- [ ] All secrets in Key Vault; app code reads via MI or Key Vault references.
- [ ] CMK on storage and SQL TDE for regulated data.
- [ ] Storage versioning + soft delete + point-in-time restore.

**Monitoring**
- [ ] Diagnostic Settings on every resource → LAW (module 13).
- [ ] Activity Log → LAW + alerts on privileged operations.
- [ ] Defender for Cloud enabled (CSPM + Servers + SQL + Storage + Key Vault + App Service plans).
- [ ] Sentinel onboarded if org has SOC.
- [ ] Alerts on Key Vault access patterns (unusual identities, off-hours bulk fetches).

**Governance**
- [ ] Azure Policy enforcing baseline (allowed regions, required tags, no public IPs).
- [ ] Resource Locks (`CanNotDelete`) on critical resources (Key Vault, prod RGs).
- [ ] Cost alerts at sub and RG level (module 17).

Bicep snippet for a hardened Key Vault used by an App Service:

```bicep
resource kv 'Microsoft.KeyVault/vaults@2024-04-01-preview' = {
  name: 'kv-${workload}-${env}'
  location: location
  properties: {
    tenantId: subscription().tenantId
    sku: { family: 'A', name: 'standard' }
    enableRbacAuthorization: true
    enableSoftDelete: true
    softDeleteRetentionInDays: 90
    enablePurgeProtection: true
    publicNetworkAccess: 'Disabled'
    networkAcls: { defaultAction: 'Deny', bypass: 'AzureServices' }
  }
}

resource ra 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  scope: kv
  name: guid(kv.id, appServicePrincipalId, '4633458b-17de-408a-b874-0445c86b69e6')  // Key Vault Secrets User
  properties: {
    principalId: appServicePrincipalId
    principalType: 'ServicePrincipal'
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', '4633458b-17de-408a-b874-0445c86b69e6')
  }
}
```

## 5. Common Mistakes & Gotchas

- **Vault without purge protection.** Required for any CMK scenario; recommended for everything. Cannot be enabled retroactively if you've already disabled it on a vault holding production secrets — sometimes requires re-creation.
- **Mixing RBAC and Access Policies.** A vault uses one or the other based on `enableRbacAuthorization`. Switching mid-life leaves all the previous-mode permissions inert.
- **Hardcoded secret versions.** `@Microsoft.KeyVault(SecretUri=.../secrets/foo/v1abc)` pins to v1abc forever. Use `.../secrets/foo/` (no version) so rotation flows through.
- **Forgetting Key Vault network ACLs.** `publicNetworkAccess: 'Disabled'` makes it unreachable except via Private Endpoint. Confirm your app's VNet routing actually works *before* flipping in prod.
- **Defender plans not enabled where they matter.** It's per-plan, per-sub. New subs default to off for most workload plans.
- **MDC recommendations ignored in bulk.** Secure Score plummets, alerts pile up, nobody reads them. Treat top-10 as a tracked backlog.
- **Sentinel without tuning.** Default analytics rules create incidents you'll never act on. Disable noisy ones, tune thresholds, document false-positives.
- **Logic-app playbooks with broad MI.** A playbook MI with `Contributor` on the sub can shut things down on bad signals. Scope tightly.
- **Encryption-at-rest panic.** Azure encrypts everything at rest by default with platform-managed keys. CMK is for *compliance/key custody*, not for "encryption is on." Don't conflate.
- **Forgotten `default-action Deny` on storage / key vault** when adding Private Endpoint. PE alone doesn't block public access; you must explicitly deny.
- **Defender for Storage enabling malware scanning bills per-GB scanned.** Surprising at high upload rates. Filter by container if appropriate.
- **Managed HSM vs Key Vault confusion.** Managed HSM is *its own service*, paid per hour for the entire HSM pool. Switch only when FIPS 140-2 Level 3 or single-tenant HSM is required — most workloads are fine on Vault Standard or Premium.

## 🎯 Key Takeaways

- **Key Vault: RBAC + purge protection + Private Endpoint + soft delete.** That quartet is the production baseline. MI-based reads in app code.
- **Defender for Cloud is two products**: free posture (CSPM) and paid workload plans. Enable both selectively; treat Secure Score as a tracked backlog.
- **Sentinel adds SIEM/SOAR on top of LAW.** Worth the cost when you have a SOC or need centralized cross-source detection.
- **Managed identities revisited:** with Entra-only auth + RBAC + Key Vault references, you can run a production Azure workload with *zero* stored secrets. That's the goal.
- **Security is policy + identity + network + observability**. No one product covers it — you compose Entra (02), RBAC (03), Networking (04), Key Vault, MDC, Sentinel, Policy (18), Monitor (13).

*← [prev](./13_observability.md) | [next → 15_iac.md](./15_iac.md)*
