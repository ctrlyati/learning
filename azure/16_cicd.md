# 16 — CI/CD: Azure DevOps Pipelines and GitHub Actions with OIDC

> **Goal:** Build CI/CD pipelines that deploy to Azure *without storing service-principal secrets* — using workload identity federation (OIDC) — and know when to pick Azure DevOps versus GitHub Actions.

## 1. The two platforms in 60 seconds

- **Azure DevOps Pipelines (ADO)** — Microsoft's traditional CI/CD. YAML pipelines + a "Library" of variable groups, secret files, service connections. Strong on enterprise governance (approvals, release gates, integration with Azure Boards / Repos). Hosted agents and self-hosted agents.
- **GitHub Actions (GHA)** — GitHub's CI/CD. Runs in `.github/workflows/*.yml`. Marketplace of reusable actions. Stronger ecosystem outside the Microsoft world. Hosted runners and self-hosted runners.

If you live on GitHub: use GHA. If your repos are in Azure Repos and your org is on ADO: use ADO. Both are first-class for Azure deployments. They are *converging* — same `azure/login` pattern, same OIDC story — so the choice is mostly organizational.

Either way: **never store a client secret in CI/CD**. Use OIDC.

## 2. Federated identity (OIDC) — the secretless mechanism

Workload identity federation makes Entra trust an external IdP's tokens. GitHub Actions and Azure DevOps both issue OIDC JWTs that identify the workflow/pipeline run. Entra exchanges that JWT for an Azure access token — no client secret in the middle.

```
GHA workflow run
  ├─ requests OIDC token from GitHub's `token.actions.githubusercontent.com` issuer
  ├─ presents JWT to Entra with `subject=repo:acme/orders:ref:refs/heads/main`
  ├─ Entra validates issuer + subject against federated credential on app
  └─ returns access token scoped to ARM / Graph / etc.
```

### Setup (one-time)

```bash
# 1. App registration + service principal
APP=$(az ad app create --display-name "gha-orders-deploy" --query appId -o tsv)
az ad sp create --id $APP
SP_OID=$(az ad sp show --id $APP --query id -o tsv)

# 2. Grant Contributor on the target subscription (scope as tightly as possible)
az role assignment create \
  --assignee-object-id $SP_OID \
  --assignee-principal-type ServicePrincipal \
  --role Contributor \
  --scope "/subscriptions/$SUB_ID"

# 3. Add the federated credential for GitHub main branch
az ad app federated-credential create --id $APP --parameters '{
  "name": "gha-main",
  "issuer": "https://token.actions.githubusercontent.com",
  "subject": "repo:acme/orders:ref:refs/heads/main",
  "audiences": ["api://AzureADTokenExchange"]
}'

# Optional: a second credential for PR previews (environment-scoped, not branch-scoped)
az ad app federated-credential create --id $APP --parameters '{
  "name": "gha-pr-preview",
  "issuer": "https://token.actions.githubusercontent.com",
  "subject": "repo:acme/orders:environment:pr-preview",
  "audiences": ["api://AzureADTokenExchange"]
}'
```

The `subject` claim is the tight binding. `repo:acme/orders:ref:refs/heads/main` means "only workflow runs on the main branch of acme/orders" can get a token. Other supported subjects: `:ref:refs/tags/*`, `:environment:production`, `:pull_request`.

The same pattern with ADO uses a workload-identity-federated service connection (ADO does the OIDC handshake under the hood when you create a "Service Connection — Azure Resource Manager — Workload Identity Federation").

## 3. GitHub Actions example — build, push, deploy

`.github/workflows/deploy.yml`:

```yaml
name: deploy

on:
  push: { branches: [main] }
  workflow_dispatch:

permissions:
  id-token: write    # required for OIDC
  contents: read

env:
  REGISTRY: acmecrprod.azurecr.io
  IMAGE: orders-api

jobs:
  build:
    runs-on: ubuntu-latest
    outputs:
      tag: ${{ steps.sha.outputs.tag }}
    steps:
      - uses: actions/checkout@v4

      - id: sha
        run: echo "tag=$(git rev-parse --short HEAD)" >> "$GITHUB_OUTPUT"

      - uses: azure/login@v2
        with:
          client-id: ${{ secrets.AZURE_CLIENT_ID }}
          tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}

      - name: ACR build (no local Docker daemon)
        run: |
          az acr build \
            --registry ${{ env.REGISTRY %% .* }} \
            --image ${{ env.IMAGE }}:${{ steps.sha.outputs.tag }} \
            --platform linux/amd64 .

  deploy-infra:
    needs: build
    runs-on: ubuntu-latest
    environment: production
    steps:
      - uses: actions/checkout@v4
      - uses: azure/login@v2
        with:
          client-id: ${{ secrets.AZURE_CLIENT_ID }}
          tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}

      - name: Bicep what-if
        run: |
          az deployment sub what-if --location eastus2 \
            --template-file infra/main.bicep --parameters infra/main.bicepparam

      - name: Bicep deploy
        run: |
          az deployment sub create --location eastus2 \
            --template-file infra/main.bicep --parameters infra/main.bicepparam

  deploy-app:
    needs: [build, deploy-infra]
    runs-on: ubuntu-latest
    environment: production
    steps:
      - uses: azure/login@v2
        with:
          client-id: ${{ secrets.AZURE_CLIENT_ID }}
          tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}

      - name: Update Container App
        run: |
          az containerapp update -g rg-orders-prod -n orders-api \
            --image ${{ env.REGISTRY }}/${{ env.IMAGE }}:${{ needs.build.outputs.tag }} \
            --revision-suffix r${{ needs.build.outputs.tag }}
```

Notable details:

- `permissions: id-token: write` is *required* for OIDC. Forgetting this is the #1 setup mistake.
- The repo's GitHub `environment: production` can require manual approval before this job runs — a free production gate.
- No secret tokens. `AZURE_CLIENT_ID` / `AZURE_TENANT_ID` / `AZURE_SUBSCRIPTION_ID` are *identifiers*, not credentials.

### Reusable workflows

For a fleet of repos, factor common steps into a **reusable workflow**:

```yaml
# .github/workflows/azure-bicep-deploy.yml (in a central repo)
on:
  workflow_call:
    inputs:
      template-file: { type: string, required: true }
      parameters-file: { type: string, required: true }
      location: { type: string, default: eastus2 }
    secrets:
      azure-client-id: { required: true }
      azure-tenant-id: { required: true }
      azure-subscription-id: { required: true }

jobs:
  deploy:
    runs-on: ubuntu-latest
    permissions: { id-token: write, contents: read }
    steps:
      - uses: actions/checkout@v4
      - uses: azure/login@v2
        with:
          client-id: ${{ secrets.azure-client-id }}
          tenant-id: ${{ secrets.azure-tenant-id }}
          subscription-id: ${{ secrets.azure-subscription-id }}
      - run: az deployment sub create --location ${{ inputs.location }} --template-file ${{ inputs.template-file }} --parameters ${{ inputs.parameters-file }}
```

Consumers:

```yaml
jobs:
  deploy:
    uses: acme/devops/.github/workflows/azure-bicep-deploy.yml@v1
    with:
      template-file: infra/main.bicep
      parameters-file: infra/main.bicepparam
    secrets: inherit
```

## 4. Azure DevOps Pipelines — the equivalent

`azure-pipelines.yml`:

```yaml
trigger: [main]

variables:
  - group: shared-azure-ids   # contains subscription/tenant IDs

stages:
  - stage: Build
    jobs:
      - job: BuildImage
        pool: { vmImage: ubuntu-latest }
        steps:
          - task: AzureCLI@2
            inputs:
              azureSubscription: 'sc-azure-prod'    # workload-identity-federated service connection
              scriptType: bash
              scriptLocation: inlineScript
              inlineScript: |
                TAG=$(git rev-parse --short HEAD)
                az acr build --registry acmecrprod --image orders-api:$TAG --platform linux/amd64 .
                echo "##vso[task.setvariable variable=tag;isOutput=true]$TAG"
            name: build_step

  - stage: DeployInfra
    dependsOn: Build
    jobs:
      - deployment: BicepDeploy
        environment: production           # ADO Environments support approvals
        strategy:
          runOnce:
            deploy:
              steps:
                - checkout: self
                - task: AzureCLI@2
                  inputs:
                    azureSubscription: 'sc-azure-prod'
                    scriptType: bash
                    scriptLocation: inlineScript
                    inlineScript: |
                      az deployment sub what-if --location eastus2 \
                        --template-file infra/main.bicep --parameters infra/main.bicepparam
                      az deployment sub create --location eastus2 \
                        --template-file infra/main.bicep --parameters infra/main.bicepparam
```

The **service connection** `sc-azure-prod` is created in ADO Project Settings → Service Connections → "Azure Resource Manager" → "Workload Identity Federation (automatic)." ADO sets up the federated credential on a managed app registration for you. No client secret to rotate.

ADO **Environments** give you approvals, gates (e.g., "open ServiceNow change ticket first"), and resource permissions — the "release management" layer GHA addresses via GitHub Environments.

## 4.5. Practical Application — pipeline patterns

### Branch strategy

- `main` deploys to `production` (with environment approvals).
- PRs deploy to a **PR preview environment** (auto-named per PR, federated credential subject `repo:acme/orders:pull_request`).
- Tags `v*` deploy production releases with strict gates.

### Environment promotion

```
build → dev (auto)
       → test (auto on main)
       → staging (auto, smoke tests)
       → production (manual approval, 2 reviewers)
```

Use deployment slots (App Service) or Container Apps revisions for blue/green within each environment.

### Drift detection

A scheduled workflow that runs `az deployment sub what-if` (or `terraform plan`) and *fails* if there's drift. Catch portal changes early.

### Tests as gates

- **Bicep**: PR pipeline runs `az deployment what-if` against a scratch sub. Block merge if "Modify" rows appear unexpected.
- **App**: unit + integration + smoke tests at each stage. Lighthouse / security scans on PR for web apps.

## 5. Common Mistakes & Gotchas

- **Forgetting `permissions: id-token: write`** on GitHub Actions. The `azure/login@v2` step fails cryptically; the fix is one line of YAML.
- **Federated credential subject mismatch.** GitHub sends `repo:org/repo:ref:refs/heads/main`. If your credential has `repo:org/repo:ref:refs/heads/MAIN` (case sensitive!) it fails. Use exactly the same casing.
- **Federated credential per branch is overkill.** Use a single credential bound to `repo:acme/orders:environment:production` and gate via GitHub/ADO Environments, which require approval before the OIDC token is even issued.
- **Storing AZURE_CLIENT_SECRET.** No. Delete it. Migrate to OIDC. Every CI client-secret leak is an avoidable incident.
- **Self-hosted runners with no isolation.** A compromised PR can read whatever the runner has. Use ephemeral runners (one-shot containers/VMs) and avoid running PR workflows from forks with privileged credentials. GitHub has explicit `pull_request_target` rules for this — read them.
- **`azure/login@v1`.** `v2` is the supported version with newer OIDC handling. Migrate.
- **What-if not run.** Bicep deploys go straight to apply with no diff review. Add the `what-if` step as a required check.
- **Hosted runner IPs change.** Don't add hosted-runner IPs to App Gateway / Storage firewall — use Service Tags (`GitHubActions`) or self-hosted runners in your VNet.
- **Long pipelines without caching.** Re-downloading deps every run. Use `actions/cache`, `azure/setup-bicep`, `setup-node`, `setup-dotnet` with caching.
- **Secrets in YAML.** Variable groups (ADO) and repository/environment secrets (GHA) are encrypted at rest. Use Key Vault references for app-level secrets (read via `azure/login` then `az keyvault secret show`) so rotations don't require pipeline edits.
- **Approval fatigue.** Every job requires approvals → approvers become rubber stamps. Reserve approvals for production stages and prod-shape changes.
- **Bicep deployments running in parallel from multiple PRs.** ARM mostly handles this but resource-level locks (Key Vault, SQL server) can collide. Serialize prod deploys with a concurrency group.

```yaml
concurrency:
  group: deploy-prod
  cancel-in-progress: false   # queue, don't cancel
```

## 🎯 Key Takeaways

- **Workload identity federation (OIDC) replaces stored client secrets.** Both GHA and ADO support it; the setup is a one-time 10-minute task per app.
- **GitHub Actions and Azure DevOps are converging.** Pick based on where your code already lives; both deliver the same Azure capabilities.
- **`azure/login@v2` + `az` CLI is the universal deployment recipe.** Bicep deploys, Container App updates, App Service zip deploys — all flow through `az`.
- **Environments (GHA or ADO) give you approvals, gates, and audit trails for free.** Use them; don't roll your own.
- **Always run `what-if` / `plan` in PR**, deploy in main, and bake drift detection into a nightly job. Promote through dev → test → staging → prod with progressive automation.

*← [prev](./15_iac.md) | [next → 17_cost_management.md](./17_cost_management.md)*
