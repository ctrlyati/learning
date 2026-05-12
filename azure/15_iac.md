# 15 — Infrastructure as Code: ARM, Bicep, Terraform

> **Goal:** Compare the three IaC choices for Azure, master Bicep for first-party authoring, and know when Terraform is the better tool — so you stop clicking the portal forever.

## 1. The three IaC options in one paragraph each

- **ARM JSON templates** — the *raw* Azure Resource Manager language. Everything Azure deploys is ARM under the hood. JSON, verbose, painful. You don't write these by hand in 2026; you may read them when debugging a Bicep compilation.
- **Bicep** — Microsoft's DSL that compiles 1:1 to ARM. Clean syntax, modules, what-if, idempotent. First-party. The right default for new Azure-only work.
- **Terraform** — HashiCorp's IaC. Multi-cloud, vast provider ecosystem. State file-based. The right default for multi-cloud, for teams already on Terraform, or when you need providers Microsoft doesn't ship (Databricks, Snowflake, etc.).

A close fourth: **Pulumi** (IaC in real programming languages — TS, Python, Go, C#). Valid choice, especially for teams that prefer code over DSL, but smaller community than Terraform.

If you're starting greenfield on Azure-only and you don't already use Terraform: pick **Bicep**. If you're multi-cloud or your team has Terraform muscle: stay on **Terraform**.

## 2. Bicep — syntax, modules, deployment scopes

### Syntax fundamentals

```bicep
// params with constraints
@description('Workload name')
@minLength(3) @maxLength(20)
param workload string

@allowed(['dev','test','prod'])
param env string

param location string = resourceGroup().location

// variables
var saName = toLower('st${workload}${env}${uniqueString(resourceGroup().id)}')

// resource
resource sa 'Microsoft.Storage/storageAccounts@2023-05-01' = {
  name: saName
  location: location
  sku: { name: 'Standard_LRS' }
  kind: 'StorageV2'
  properties: {
    minimumTlsVersion: 'TLS1_2'
    allowBlobPublicAccess: false
  }
  tags: { workload: workload, env: env }
}

// outputs
output storageId string = sa.id
output storageName string = sa.name
```

Things to know:

- `resource <symbolic-name> 'type@apiVersion' = { ... }`. The symbolic name (`sa`) is local; the actual resource name is `.name`.
- `existing` keyword: `resource sa 'Microsoft.Storage/storageAccounts@2023-05-01' existing = { name: 'stfoo' }`. Reference, don't create.
- `if` for conditional deploy: `resource pe '...' = if (env == 'prod') { ... }`.
- `for` for loops: `resource subnets '...' = [for name in subnetNames: { ... }]`.
- Outputs are typed; can be used by parent module.
- No mutation: every resource is *declared*, not *changed*.

### Modules

A `.bicep` file is a module. Compose:

```bicep
// main.bicep
module storage 'modules/storage.bicep' = {
  name: 'storage-deploy'
  params: { workload: workload, env: env, location: location }
}

module sql 'modules/sql.bicep' = {
  name: 'sql-deploy'
  params: {
    workload: workload, env: env, location: location
    adminGroupId: dbaGroupId
    privateEndpointSubnetId: networkOutputs.subnetPeId
  }
}
```

Reference module outputs: `storage.outputs.storageId`. Module dependencies are inferred from references — explicit `dependsOn` is rarely needed.

### Deployment scopes

Bicep can deploy at four scopes via `targetScope`:

```bicep
targetScope = 'resourceGroup'   // default
targetScope = 'subscription'
targetScope = 'managementGroup'
targetScope = 'tenant'
```

You deploy with the matching CLI:

```bash
az deployment group create  --resource-group $RG     --template-file main.bicep
az deployment sub create    --location eastus2       --template-file main.bicep
az deployment mg create     --management-group-id mg --location eastus2 --template-file main.bicep
az deployment tenant create --location eastus2       --template-file main.bicep
```

A typical landing-zone Bicep is *subscription-scoped* and creates RGs + deploys modules into them.

### What-if and validation

```bash
az deployment group what-if -g $RG --template-file main.bicep --parameters env=prod
```

`what-if` shows ARM's predicted diff: Create / Modify / Delete / Ignore / NoChange. Always run before applying in prod. It's not perfect (ARM can't predict every server-side default) but it's the closest thing to a `terraform plan`.

## 3. Variations — Terraform on Azure, the AzAPI provider, deployment stacks

### Terraform on Azure

Two providers worth knowing:

- **`azurerm`** — the mainline provider. Curated, opinionated resource schemas. Lags ARM by weeks/months for new services.
- **`azapi`** — thin pass-through to the ARM REST API. Use when `azurerm` doesn't yet support a property. The Bicep team's "AzAPI Terraform provider" is the bridge.

Sample:

```hcl
terraform {
  required_providers {
    azurerm = { source = "hashicorp/azurerm", version = "~> 4.0" }
  }
  backend "azurerm" {
    resource_group_name  = "rg-tfstate"
    storage_account_name = "sttfstateprod"
    container_name       = "tfstate"
    key                  = "platform.tfstate"
    use_oidc             = true     # GitHub OIDC, no client secret
  }
}

provider "azurerm" {
  features {}
  use_oidc = true
}

resource "azurerm_resource_group" "rg" {
  name     = "rg-platform-prod"
  location = "eastus2"
  tags     = { workload = "platform", env = "prod" }
}

resource "azurerm_storage_account" "sa" {
  name                     = "stplatformprod${random_id.rand.hex}"
  resource_group_name      = azurerm_resource_group.rg.name
  location                 = azurerm_resource_group.rg.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
  min_tls_version          = "TLS1_2"
  allow_nested_items_to_be_public = false
}

resource "random_id" "rand" { byte_length = 3 }
```

Terraform-specific concerns:

- **State file.** Stored in a backend; usually a Blob in a dedicated storage account. *Locked* during apply to prevent concurrent writes. Lose the state file = re-import or rebuild.
- **State drift.** Manual portal changes diverge from state. `terraform plan` detects drift.
- **Provider versions.** Pin major version; let minor float. Test upgrades.
- **Workspaces** or **separate state files** per env. Most teams pick separate state files.

### Bicep vs Terraform — choosing

| Concern | Bicep | Terraform |
|---------|-------|-----------|
| Azure-only | First-class | Excellent but second-party |
| Multi-cloud | No | Yes |
| State management | No state file (ARM is source of truth) | Required; you manage backend |
| Day-1 coverage of new Azure services | Immediate (uses ARM directly) | Provider lag (use `azapi` for cutting edge) |
| Drift detection | `what-if` (limited) | `terraform plan` (excellent) |
| Resource removal on no-longer-declared | `complete` mode deploys delete; `incremental` (default) doesn't | Always removes |
| Modules | First-class | First-class |
| Secret handling | `secure()` params, Key Vault refs in params files | Sensitive vars + remote state encryption |
| Learning curve | Lower if you know JSON/ARM | Moderate |

### Deployment Stacks

A newer ARM feature (~2024): a **Deployment Stack** is a managed group of resources with auto-cleanup. Resources declared in the stack but later removed from the template can be configured to auto-delete on next deploy — addresses the "Bicep incremental mode doesn't remove things" pain.

```bash
az stack group create -g $RG -n stack-platform \
  --template-file main.bicep \
  --action-on-unmanage deleteResources \
  --deny-settings-mode denyDelete
```

This is the modern Bicep equivalent of Terraform's "I removed this resource → please destroy it." Adopt for new Bicep deployments.

## 4. Practical Application — a small Bicep landing pattern

Layout:

```
infra/
├── main.bicep                 # subscription-scoped entry point
├── main.bicepparam            # typed parameter file
└── modules/
    ├── rg.bicep
    ├── network.bicep
    ├── kv.bicep
    └── webapp.bicep
```

`main.bicep`:

```bicep
targetScope = 'subscription'

param location string = 'eastus2'
param workload string = 'orders'
param env string = 'prod'
param dbaGroupId string

resource rg 'Microsoft.Resources/resourceGroups@2024-03-01' = {
  name: 'rg-${workload}-${env}'
  location: location
  tags: { workload: workload, env: env, owner: 'cloud-team' }
}

module network 'modules/network.bicep' = {
  scope: rg
  name: 'network'
  params: { workload: workload, env: env, location: location }
}

module kv 'modules/kv.bicep' = {
  scope: rg
  name: 'kv'
  params: {
    workload: workload, env: env, location: location
    pdzVaultId: network.outputs.pdzVaultId
    peSubnetId:  network.outputs.peSubnetId
    adminGroupId: dbaGroupId
  }
}

module web 'modules/webapp.bicep' = {
  scope: rg
  name: 'web'
  params: {
    workload: workload, env: env, location: location
    kvName: kv.outputs.name
    vnetIntegrationSubnetId: network.outputs.appServiceSubnetId
  }
}

output webHostname string = web.outputs.defaultHostname
```

`main.bicepparam`:

```bicep
using 'main.bicep'

param location   = 'eastus2'
param workload   = 'orders'
param env        = 'prod'
param dbaGroupId = readEnvironmentVariable('DBA_GROUP_OID')
```

Deploy:

```bash
az deployment sub what-if --location eastus2 --template-file infra/main.bicep --parameters infra/main.bicepparam
az deployment sub create  --location eastus2 --template-file infra/main.bicep --parameters infra/main.bicepparam
```

In CI (GitHub Actions, module 16):

```yaml
- uses: azure/login@v2
  with:
    client-id: ${{ secrets.AZURE_CLIENT_ID }}
    tenant-id: ${{ secrets.AZURE_TENANT_ID }}
    subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}
- run: az deployment sub create --location eastus2 --template-file infra/main.bicep --parameters infra/main.bicepparam
```

No secrets stored anywhere.

## 5. Common Mistakes & Gotchas

- **Incremental mode confusion.** ARM/Bicep default is *incremental*: creates and updates, never deletes resources you stopped declaring. Use **Deployment Stacks** with `--action-on-unmanage deleteResources` to get Terraform-style cleanup.
- **`what-if` blind spots.** Some properties (server-side computed defaults) show as "Modify" on every run even when nothing changes. Don't panic-debug; check whether the diff is real.
- **`uniqueString()` non-determinism.** Across different deployments of the "same" template (different RG / sub), `uniqueString(resourceGroup().id)` is different. Use it for *unique-within-scope* names but expect different values across environments.
- **Module deployments named identically.** ARM deployments are named, and re-using a name with different parameters overwrites the previous deployment record. Use stable, unique deployment names per module.
- **Putting secrets in parameter files.** Use `@secure()` params + read from Key Vault refs or from CI runtime env. Never commit secret param files.
- **Storage account or Key Vault name length/uniqueness.** Bicep deploys, ARM rejects. Validate at template time using `length()` constraints in `@minLength`/`@maxLength`.
- **Cross-RG references requiring `existing`.** A child module that needs to read a resource in another RG must use `existing` with the correct `scope: resourceGroup(rgName)`.
- **Terraform state in the same sub it manages.** Bootstrap problem. Solve by creating the state-storage RG by hand once (or with a small `bootstrap` script) before running Terraform.
- **Terraform `azurerm` lag.** New Azure features land in ARM weeks before `azurerm` supports them. Use `azapi` provider as a stopgap.
- **Drift on tags.** Azure Policy adds tags automatically; Terraform/Bicep keeps trying to remove them. Use `ignore_changes` (TF) or move tag enforcement into the template (Bicep).
- **Soft-deleted Key Vaults.** Re-deploying a vault with the same name within 90 days of deletion fails with "vault name in use." Restore-from-soft-delete or use a different name.
- **API versions stale.** Bicep extension flags newer API versions; use the freshest stable one. Mixing very old API versions can yield resources missing properties.
- **Linting ignored.** `az bicep lint` and the VS Code extension catch many issues at authoring time. Fix warnings; they're rarely false positives.

## 🎯 Key Takeaways

- **Bicep is the modern ARM.** For Azure-only work greenfield, it's the right default. Compiles 1:1 to ARM; first-party Microsoft; clean syntax.
- **Terraform shines for multi-cloud or teams already invested.** Use `azapi` provider when `azurerm` lags behind a new Azure feature.
- **Deployment Stacks fix Bicep's "no auto-delete" gap.** Adopt for new work.
- **Always `what-if` (Bicep) or `plan` (Terraform) before apply** in production. Pair with CI gates.
- **Modules + parameter files + per-environment params** is the standard pattern. One Bicep tree, multiple `.bicepparam` files, deployed via OIDC-federated CI.

*← [prev](./14_security.md) | [next → 16_cicd.md](./16_cicd.md)*
