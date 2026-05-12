# 03 — Azure RBAC: Roles, Assignments, Scope

> **Goal:** Understand the three-part role assignment model (security principal + role definition + scope), the scope inheritance hierarchy, and when to reach for a custom role — so least-privilege stops being a vague aspiration.

## 1. The role assignment triangle

Every Azure permission boils down to a **role assignment**: three pieces glued together.

```
┌──────────────────┐       ┌──────────────────┐       ┌──────────────────┐
│ Security         │       │ Role             │       │ Scope            │
│ Principal        │ ──+── │ Definition       │ ──@── │ (MG/Sub/RG/Res)  │
│ (Entra objectId) │       │ (e.g. Reader)    │       │                  │
└──────────────────┘       └──────────────────┘       └──────────────────┘
```

- **Security principal** — *who?* An Entra user, group, service principal, or managed identity. Identified by an `objectId` (GUID).
- **Role definition** — *what can they do?* A collection of allowed `Actions` and forbidden `NotActions` on the Azure ARM API.
- **Scope** — *where?* A management group, subscription, resource group, or single resource.

The assignment itself is a separate ARM resource of type `Microsoft.Authorization/roleAssignments`. You can list them, create them, delete them — all via CLI.

```bash
# Grant the signed-in user "Reader" on a resource group.
az role assignment create \
  --assignee $(az ad signed-in-user show --query id -o tsv) \
  --role Reader \
  --scope $(az group show --name rg-learn-azure --query id -o tsv)

# Verify.
az role assignment list --resource-group rg-learn-azure -o table
```

## 2. Mechanism — how Azure evaluates a request

When you call `az vm restart`, this happens server-side:

1. ARM extracts your Entra `objectId` from the bearer token.
2. ARM looks up *all* role assignments that target your `objectId` *or* any group/MI/MG you're a transitive member of.
3. For each assignment, ARM checks whether its **scope** is the requested resource or an ancestor of it.
4. ARM unions all matching role definitions' `Actions`, subtracts their `NotActions`, then checks whether `Microsoft.Compute/virtualMachines/restart/action` is permitted.
5. If yes → allow. If no → 403.

Key behaviors that follow from this:

- **Allow-only model.** There are no explicit "deny" assignments in standard RBAC. (There is a separate, rarely-used **Azure Deny Assignments** mechanism, populated only by Azure Blueprints, Managed Apps, and System processes — you can read them but cannot author them.)
- **Union of permissions.** If you're Reader at sub-level *and* Contributor at RG-level, you are Contributor inside the RG. Permissions accumulate.
- **Group membership is transitive.** Group-of-groups works.
- **Eventually consistent.** New role assignments take a few seconds (usually <30s) to propagate. CI scripts that grant then immediately use a role often need a small wait.

Inspect a role's actions:

```bash
az role definition list --name "Storage Blob Data Contributor" \
  --query "[].{Name:roleName, Actions:permissions[0].actions, NotActions:permissions[0].notActions, DataActions:permissions[0].dataActions}" \
  -o jsonc
```

Notice **`dataActions` vs `actions`**. Two control planes:

- **Management plane** (`actions`) — ARM operations: create the storage account, list keys, set tags.
- **Data plane** (`dataActions`) — operations on the *data inside* a resource: read a blob, write a queue message.

`Storage Account Contributor` lets you manage the account but not read blobs. `Storage Blob Data Reader` lets you read blobs but not manage the account. This split is everywhere in modern Azure: Key Vault, Service Bus, Cosmos DB. Always grant the most specific data role, never the broad management role, for app workloads.

## 3. Built-in roles, scope hierarchy, and custom roles

### Built-in roles to memorize

There are ~400 built-in roles. The interview-relevant subset:

| Role | What it does | Watch out for |
|------|--------------|---------------|
| **Owner** | Everything, including granting access to others. | Almost never assign this to a human; use PIM. |
| **Contributor** | Manage everything except role assignments. | The default "developer" role for an RG. |
| **Reader** | Read-only. | Doesn't grant data plane reads. |
| **User Access Administrator** | Manage role assignments only. | Combine with Reader for delegated permission admins. |
| **Storage Blob Data Contributor / Reader** | Data plane on blobs. | App identities want these, not "Storage Account Contributor." |
| **Key Vault Secrets User / Officer** | Read / manage secrets via RBAC. | Vaults can use *either* RBAC (modern) or access policies (legacy) — pick RBAC. |
| **Virtual Machine Contributor** | Manage VMs but not the VNet or disks they sit on. | You'll need extras. |
| **Network Contributor** | Manage VNets, NSGs, etc. | Powerful — scope it tightly. |
| **Reservations Reader / Purchaser** | View/purchase reservations. | Cost role, lives at the billing-account scope. |
| **Monitoring Reader / Contributor** | Observability stack. | You need this on a Log Analytics workspace, not the resource being monitored. |

### Scope hierarchy

```
Tenant Root MG
   └── Management Group (intermediate)
         └── Subscription
               └── Resource Group
                     └── Resource
```

A role assigned at any node applies to **that node and everything beneath it**. Practical implication:

- Grant `Reader` at the MG → user can read every resource in every sub under that MG.
- Grant `Contributor` at the RG → user can manage resources in that RG only.
- Grant `Storage Blob Data Reader` at the blob container scope → user can read just that container.

Yes, **scope can go below the resource** for a few services that support sub-resource scopes (blob containers, queue, individual key-vault secret, etc.). Use it when applicable.

```bash
# Assign at sub-resource scope.
az role assignment create \
  --assignee $UPN \
  --role "Storage Blob Data Reader" \
  --scope "/subscriptions/<sub>/resourceGroups/rg-data/providers/Microsoft.Storage/storageAccounts/stdata01/blobServices/default/containers/landing"
```

### Custom roles

When no built-in fits — usually for tightly-scoped operational tasks ("restart any VM but not delete it") — you author a custom role. JSON:

```json
{
  "Name": "VM Operator",
  "Description": "Start/stop/restart VMs.",
  "Actions": [
    "Microsoft.Compute/virtualMachines/read",
    "Microsoft.Compute/virtualMachines/start/action",
    "Microsoft.Compute/virtualMachines/restart/action",
    "Microsoft.Compute/virtualMachines/deallocate/action",
    "Microsoft.Compute/virtualMachines/instanceView/read"
  ],
  "NotActions": [],
  "DataActions": [],
  "AssignableScopes": ["/subscriptions/<sub-id>"]
}
```

Create and use:

```bash
az role definition create --role-definition vm-operator.json
az role assignment create --assignee $UPN --role "VM Operator" --scope /subscriptions/<sub>
```

Two custom-role gotchas:

- **`AssignableScopes` is a list of MGs/subs.** It defines *where the role can be assigned*, not who can use it. Pick wisely — broader = harder to delete later.
- **Custom roles are per-tenant.** They don't cross tenants. There's a cap (currently 5,000 per tenant) but you'll never approach it.

## 4. Practical Application — least-privilege a Function App's identity

Realistic scenario: A Function App reads from a Service Bus queue and writes to a Cosmos DB container, both in different resource groups. Set up minimal RBAC.

```bash
# 0. Variables.
RG_FN=rg-orders-fn-prod
RG_BUS=rg-orders-bus-prod
RG_DB=rg-orders-db-prod
FN_NAME=fn-orders-prod
BUS_NS=sb-orders-prod
COSMOS=cosmos-orders-prod

# 1. Enable system-assigned MI on the Function App.
az functionapp identity assign --name $FN_NAME --resource-group $RG_FN
FN_OID=$(az functionapp identity show --name $FN_NAME --resource-group $RG_FN --query principalId -o tsv)

# 2. Grant Service Bus Data Receiver on just the orders queue.
QUEUE_ID=$(az servicebus queue show \
  --resource-group $RG_BUS \
  --namespace-name $BUS_NS \
  --name orders-incoming \
  --query id -o tsv)

az role assignment create \
  --assignee-object-id $FN_OID \
  --assignee-principal-type ServicePrincipal \
  --role "Azure Service Bus Data Receiver" \
  --scope $QUEUE_ID

# 3. For Cosmos SQL API, RBAC is split: control plane (ARM) vs data plane (Cosmos-specific).
#    Use Cosmos's built-in data-plane role at the database scope.
COSMOS_DB_ID=$(az cosmosdb show --name $COSMOS --resource-group $RG_DB --query id -o tsv)
az cosmosdb sql role assignment create \
  --account-name $COSMOS \
  --resource-group $RG_DB \
  --scope "$COSMOS_DB_ID/dbs/orders" \
  --principal-id $FN_OID \
  --role-definition-id 00000000-0000-0000-0000-000000000002  # Cosmos DB Built-in Data Contributor
```

What we did *not* grant: management on the namespace, list-keys, network changes, anything in `RG_FN` beyond what the Function App already inherited from its own creator. This is what least-privilege looks like in practice.

Bicep equivalent for the Service Bus assignment:

```bicep
resource queue 'Microsoft.ServiceBus/namespaces/queues@2022-10-01-preview' existing = {
  name: '${busNamespace}/orders-incoming'
}

var roleId = '4f6f3318-aae3-4cf3-83a4-65f4f6e8ba43'  // Azure Service Bus Data Receiver

resource ra 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  scope: queue
  name: guid(queue.id, functionPrincipalId, roleId)
  properties: {
    principalId: functionPrincipalId
    principalType: 'ServicePrincipal'
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', roleId)
  }
}
```

Two things worth memorizing from that snippet: (a) role assignment `name` is a *deterministic GUID* derived from scope+principal+role so Bicep is idempotent, and (b) `principalType: 'ServicePrincipal'` matters when assigning to a brand-new MI — without it Azure may reject the assignment because the principal hasn't propagated yet.

## 5. Common Mistakes & Gotchas

- **Granting `Contributor` at the subscription** because it's easier than figuring out the right scope. Two months later, an intern's compromised credentials wreak havoc. Always assign at the *lowest scope that works*.
- **Forgetting the management plane vs data plane split.** Giving `Storage Account Contributor` to your app's MI doesn't let it read blobs. You also need `Storage Blob Data Reader/Contributor`. New engineers add the management role, debug for an hour, then read this paragraph.
- **`--assignee` ambiguity.** `az role assignment create --assignee <name-or-id>` resolves the name → object ID via Entra, which is *slow* and racy for brand-new SPs. Always pass `--assignee-object-id` and `--assignee-principal-type` explicitly in automation.
- **PIM eligible vs active assignments.** With Entra ID P2 + PIM, you can make a role assignment *eligible* — the user activates it temporarily (with MFA + reason). `az role assignment list` shows only *active* assignments. PIM eligible assignments are visible via the PIM API. Don't conclude "no one is Owner" by looking at active list.
- **Propagation delay.** New role assignments need a few seconds. CI pipelines that create an MI, grant a role, and immediately use it sometimes flake. Add a retry with backoff.
- **Deny assignments confusing.** You see a deny assignment in the portal, you can't delete it, you don't remember creating it. That's because it came from a Managed App or Blueprint. Find the source resource and modify or delete *that*.
- **`AssignableScopes` lock-in.** A custom role's scopes can be expanded but not narrowed without recreating. Plan ahead.
- **Cosmos DB and SQL Database have their own role systems** layered *under* Azure RBAC. Cosmos DB has its own data-plane RBAC (`az cosmosdb sql role assignment`). Azure SQL has its own users-and-roles inside the database. Don't expect "Contributor on the resource" to give you data access.
- **Key Vault: RBAC mode vs Access Policies.** Older vaults use Access Policies. New vaults should use RBAC (`--enable-rbac-authorization true`). Once a vault is on RBAC, its Access Policies tab is ignored — and vice versa. Migrating between modes requires reissuing all permissions.
- **`AssignableScopes` and management groups.** A custom role assignable at MG scope is *only* assignable at that MG or below. You can't later use it in a sub outside that MG.
- **Reader is not a read-everything pass.** It reads ARM. It does not read blob contents, queue messages, or key vault secrets — those need data-plane reader roles.

## 🎯 Key Takeaways

- **Three pieces: principal + role + scope.** Memorize the triangle; every Azure access question is "which one is wrong?"
- **Management plane and data plane are separate.** App identities almost always want a *data* role, not a *management* role.
- **Scope inheritance is the source of every "too much access" incident.** Assign at the lowest scope, prefer RGs over subs, prefer resources over RGs when feasible.
- **Custom roles are easy and underused.** When no built-in fits, a 20-line JSON file gives you exact least-privilege.
- **Pair RBAC with PIM and Access Reviews (Entra ID P2)** for production — eligible role activation with MFA + reason is the difference between a typo and a breach.

*← [prev](./02_entra_id.md) | [next → 04_networking.md](./04_networking.md)*
