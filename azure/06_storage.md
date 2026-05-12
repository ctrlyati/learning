# 06 — Azure Storage: Blob, File, Queue, Table

> **Goal:** Pick the right storage service and tier for each workload, configure redundancy intelligently, and authenticate with managed identities + SAS only when absolutely necessary.

## 1. The Storage Account — one resource, four data services

An Azure **Storage Account** is *one* ARM resource that exposes *four* data services on subdomains:

| Service | Endpoint | Use case |
|---------|----------|----------|
| **Blob** | `<acct>.blob.core.windows.net` | Object storage (S3-equivalent) |
| **File** | `<acct>.file.core.windows.net` | SMB/NFS shares for VMs and on-prem |
| **Queue** | `<acct>.queue.core.windows.net` | Simple FIFO-ish queues |
| **Table** | `<acct>.table.core.windows.net` | NoSQL key-value (legacy; Cosmos DB Table API supersedes this) |

Plus **Static Website** hosting via Blob (`<acct>.z*.web.core.windows.net`) and **Data Lake Gen2** = Blob with hierarchical namespace turned on.

Account name is **globally unique**, 3-24 lowercase letters/digits. No dashes. Plan a naming scheme like `st<workload><env><region><nn>` early — you cannot rename later.

```bash
az storage account create \
  --name stappdataprod01 \
  --resource-group rg-data-prod \
  --location eastus2 \
  --sku Standard_LRS \
  --kind StorageV2 \
  --access-tier Hot \
  --allow-blob-public-access false \
  --min-tls-version TLS1_2 \
  --enable-hierarchical-namespace false
```

Always set `allow-blob-public-access false` and `min-tls-version TLS1_2` on new accounts. Both are *off-by-default in modern defaults* but explicit is safer.

## 2. Tiers and redundancy — the two cost dials

### Performance tiers

- **Standard** — HDD-backed metadata, tiered hot/cool/cold/archive. The default.
- **Premium** — SSD-backed, single-digit-ms latency. Choose only when you need it:
  - `Premium_LRS` + `BlockBlobStorage` kind for high-IOPS blob.
  - `Premium_LRS` + `FileStorage` kind for high-throughput SMB.
  - `Premium_LRS` + `StorageV2` kind for page blobs (VHDs).

### Access tiers (Blob only)

| Tier | $/GB-month | Read cost | Min retention | Latency |
|------|-----------|-----------|---------------|---------|
| Hot | high | cheap | none | ms |
| Cool | medium | medium | 30 days | ms |
| Cold | low | higher | 90 days | ms |
| Archive | very low | very high + rehydrate hours | 180 days | hours |

Tiers are set per-account (default) or per-blob. Use **lifecycle management** to move blobs automatically:

```bash
az storage account management-policy create \
  --account-name stappdataprod01 \
  --resource-group rg-data-prod \
  --policy '{
    "rules": [{
      "name": "tier-down",
      "enabled": true,
      "type": "Lifecycle",
      "definition": {
        "filters": { "blobTypes": ["blockBlob"], "prefixMatch": ["logs/"] },
        "actions": {
          "baseBlob": {
            "tierToCool":    { "daysAfterModificationGreaterThan": 30  },
            "tierToCold":    { "daysAfterModificationGreaterThan": 90  },
            "tierToArchive": { "daysAfterModificationGreaterThan": 365 },
            "delete":        { "daysAfterModificationGreaterThan": 2555 }
          }
        }
      }
    }]
  }'
```

### Redundancy options

| SKU | Replication | Region pair? | Read access to secondary? |
|-----|-------------|--------------|---------------------------|
| **LRS** | 3 copies, single datacenter | no | no |
| **ZRS** | 3 copies across 3 zones, same region | no | no |
| **GRS** | LRS + async copy to paired region | yes | no |
| **GZRS** | ZRS + async copy to paired region | yes | no |
| **RA-GRS** | GRS with read access on the secondary | yes | yes (eventually consistent) |
| **RA-GZRS** | GZRS with secondary read | yes | yes |

Picking guide:

- **LRS** — dev, easily reproducible data, cheapest.
- **ZRS** — modern prod default for non-critical data; no cross-region copy.
- **GZRS** — modern prod default for data you actually want to survive a regional outage. Slightly more than GRS, zone-redundant within region too.
- **RA-** prefix — only if you need read-only access at the secondary endpoint. Most apps don't.

You can change replication after the fact for some transitions (LRS↔GRS, ZRS↔GZRS) but not all. Plan up front.

## 3. Mechanism — authentication, SAS, and immutable storage

### Authentication options (in order of preference)

1. **Microsoft Entra (RBAC + token)** — your VM/Function/App Service uses its managed identity. Roles like `Storage Blob Data Reader/Contributor`. *This is what you want.*
2. **Shared Key** — the account key. Two per account; rotate by regenerating one while apps use the other. Avoid in 2026; *disable* with `--allow-shared-key-access false` once you've migrated.
3. **SAS (Shared Access Signature)** — time-limited signed URL. Two kinds:
   - **User Delegation SAS** — signed by an Entra user/MI key. Best practice when SAS is unavoidable.
   - **Account SAS / Service SAS** — signed by the storage account key. Legacy.
4. **Anonymous public access** — disable globally with `--allow-blob-public-access false`. Use a CDN if you genuinely need public.

Issue a user-delegation SAS for an upload:

```bash
EXPIRY=$(date -u -d '+1 hour' +%Y-%m-%dT%H:%MZ)
az storage blob generate-sas \
  --account-name stappdataprod01 \
  --container-name uploads \
  --name customer-import.csv \
  --permissions rcw \
  --expiry $EXPIRY \
  --auth-mode login \
  --as-user
```

`--auth-mode login --as-user` is what makes it a user-delegation SAS (signed via Entra), not an account-key SAS.

### Versioning, soft delete, immutability

- **Soft delete** — recover deleted blobs/containers/account for N days. *Enable always.* `--enable-delete-retention --delete-retention-days 14`.
- **Blob versioning** — automatic new version per overwrite. Pair with lifecycle to delete old versions.
- **Point-in-time restore** — restore a *container* to a past time. Requires versioning + soft delete + change feed.
- **Immutable blob storage** — WORM. Legal-hold or time-based policies, configured per container. Required for some compliance frameworks (SEC 17a-4, GDPR right-to-be-forgotten exemptions).

### Encryption

Always-on AES-256 with Microsoft-managed keys. You can swap to **customer-managed keys (CMK)** stored in Key Vault for compliance — module 14. **Infrastructure encryption** adds a second layer (data is encrypted twice with two keys); enable at account-create time only.

## 3. Variations and depth — Blob, File, Queue, Table

### Blob

Three blob types you may encounter:

- **Block blob** — files. The 99% case. Up to ~190 TiB.
- **Append blob** — log-style append-only. Each "append" is atomic.
- **Page blob** — random-access in 512-byte pages. Used internally for VHDs (managed disks). Don't author these manually.

Upload and download:

```bash
az storage blob upload \
  --account-name stappdataprod01 \
  --container-name uploads \
  --name customer-import.csv \
  --file ./customer-import.csv \
  --auth-mode login

az storage blob download \
  --account-name stappdataprod01 \
  --container-name uploads \
  --name customer-import.csv \
  --file ./out.csv \
  --auth-mode login
```

### File — Azure Files

SMB 3.1.1 / NFS 4.1 shares. Use cases:

- **Lift-and-shift** SMB workloads from on-prem.
- **Shared content** between VMs (e.g., shared upload folder for a multi-VM web tier — though Blob is usually cheaper).
- **AKS persistent volumes** via the Azure Files CSI driver.

```bash
az storage account create -n stfilesprod01 -g rg-data-prod \
  --kind FileStorage --sku Premium_LRS
az storage share-rm create --resource-group rg-data-prod \
  --storage-account stfilesprod01 \
  --name uploads --quota 1024 \
  --enabled-protocols SMB
```

For SMB, **Entra-joined identity-based access** is available (no more storage key on every client). Set up Entra Domain Services or Entra Kerberos auth depending on your needs.

### Queue

Simple, cheap (`Standard_LRS` ~USD 0.0004/10k ops), at-most-1 delivery, no ordering guarantees beyond FIFO-best-effort. Limits: 64 KB message, 7-day max visibility. For anything non-trivial use **Service Bus** (module 10) instead — Queue Storage is only a fit when cost-minimization beats features.

### Table

Cheap key-value at the partition+row-key level. Microsoft considers **Cosmos DB Table API** the strategic successor. For new builds don't pick Table Storage unless you're absolutely sure cost matters more than features.

## 4. Practical Application — secure data lake bootstrap

Goal: a Data Lake Gen2 (Blob with hierarchical namespace) for an analytics workload, with private endpoint, RBAC, lifecycle, and locked-down public access.

```bicep
param location string = resourceGroup().location
param subnetPeId string
param tenantId string = subscription().tenantId
param pdzBlobId string

resource sa 'Microsoft.Storage/storageAccounts@2023-05-01' = {
  name: 'stdatalake${uniqueString(resourceGroup().id)}'
  location: location
  sku: { name: 'Standard_GZRS' }
  kind: 'StorageV2'
  properties: {
    isHnsEnabled: true
    minimumTlsVersion: 'TLS1_2'
    allowBlobPublicAccess: false
    allowSharedKeyAccess: false
    publicNetworkAccess: 'Disabled'
    networkAcls: { defaultAction: 'Deny', bypass: 'AzureServices' }
    encryption: {
      keySource: 'Microsoft.Storage'
      requireInfrastructureEncryption: true
      services: {
        blob: { enabled: true, keyType: 'Account' }
        file: { enabled: true, keyType: 'Account' }
      }
    }
  }
}

resource blobSvc 'Microsoft.Storage/storageAccounts/blobServices@2023-05-01' = {
  parent: sa
  name: 'default'
  properties: {
    isVersioningEnabled: true
    deleteRetentionPolicy: { enabled: true, days: 30 }
    containerDeleteRetentionPolicy: { enabled: true, days: 30 }
    changeFeed: { enabled: true, retentionInDays: 30 }
  }
}

resource raw 'Microsoft.Storage/storageAccounts/blobServices/containers@2023-05-01' = {
  parent: blobSvc
  name: 'raw'
  properties: { publicAccess: 'None' }
}

resource pe 'Microsoft.Network/privateEndpoints@2023-09-01' = {
  name: 'pe-${sa.name}-dfs'
  location: location
  properties: {
    subnet: { id: subnetPeId }
    privateLinkServiceConnections: [{
      name: 'dfs'
      properties: { privateLinkServiceId: sa.id, groupIds: ['dfs'] }
    }]
  }
}

resource peDns 'Microsoft.Network/privateEndpoints/privateDnsZoneGroups@2023-09-01' = {
  parent: pe
  name: 'default'
  properties: {
    privateDnsZoneConfigs: [{
      name: 'dfs'
      properties: { privateDnsZoneId: pdzBlobId }
    }]
  }
}
```

Notable hardening choices:

- `allowSharedKeyAccess: false` — *only* Entra auth works. Closes the largest "leaked connection string" attack class.
- `publicNetworkAccess: 'Disabled'` — only reachable via Private Endpoint.
- `requireInfrastructureEncryption: true` — second encryption layer.
- `isHnsEnabled: true` — Data Lake Gen2.
- GZRS replication.

## 5. Common Mistakes & Gotchas

- **Leaking the account key in a connection string.** It's a 50-character secret with full access to the account. Disable key auth (`allowSharedKeyAccess: false`) and migrate to RBAC + MI.
- **`allowBlobPublicAccess` true.** A single misconfigured container is one of the most common data breaches. Disable at account level so nobody can opt-in.
- **Mixing private endpoint + service endpoint** — they interact in surprising ways. Pick one strategy per account.
- **Tier transitions cost real money.** Moving a 10 TB blob to archive then accessing it = rehydration cost + read costs that dwarf the storage savings. Model lifecycle costs before applying.
- **Cold/Archive minimum-retention penalty.** Delete a blob from Cool before 30 days → you pay the missing days anyway. Lifecycle rules must respect this.
- **Storage account name globally unique** — and you can't get it back if someone else takes it. Reserve names in CI/CD with `uniqueString()` deterministic patterns or org prefixes.
- **`StorageV2` vs `BlockBlobStorage` vs `FileStorage`.** The latter two are *Premium-only* and lock you into that service. `StorageV2` is the flexible general-purpose one.
- **Hierarchical namespace is one-way.** Once enabled, you can't disable. ADLS Gen2 features are great for analytics but break some Blob-only tools.
- **SAS without a stored access policy** — you can't revoke an issued SAS short of rotating the signing key, which breaks every other SAS. Use stored access policies for revocability.
- **Soft delete doesn't help against an account deletion.** Account-level deletion has a separate setting (`Allow purging soft-deleted accounts`). Also use Resource Locks (`CanNotDelete`) on prod storage.
- **NFS shares require Premium FileStorage** and don't support snapshots or backup the way SMB shares do. Pick the protocol with eyes open.
- **Cross-region GRS lag.** Asynchronous; RPO ~15 min. Don't assume your secondary is current.
- **Azure vs AWS:** Storage accounts are *containers of containers*; there's no per-bucket policy like S3 bucket policies. RBAC and network ACLs are at the account level (with container-scope role assignments possible). Mental shift required.

## 🎯 Key Takeaways

- **One storage account, four services.** Don't create a separate account per data service unless you have a real isolation/scale reason.
- **GZRS + RBAC + Private Endpoint + Shared Key disabled** is the modern secure prod baseline. Memorize that quartet.
- **Tier with lifecycle rules**, not hand-moves. Model the minimum-retention penalties before flipping policies.
- **Authenticate with Entra/MI** wherever possible; SAS only for narrow use cases (browser uploads, external sharing), and prefer user-delegation SAS over account-key SAS.
- **Storage account names are global and forever.** Adopt a naming convention before the second account exists.

*← [prev](./05_vms.md) | [next → 07_sql_and_cosmos.md](./07_sql_and_cosmos.md)*
