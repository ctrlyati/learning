# 02 — Microsoft Entra ID (formerly Azure AD)

> **Goal:** Internalize that Entra ID is the identity substrate for *everything* on Azure — users, apps, managed identities, RBAC — and learn the day-to-day commands and concepts every cloud engineer needs.

## 1. What Entra ID actually is

Microsoft renamed Azure Active Directory to **Microsoft Entra ID** in July 2023. The product is unchanged; the rename is part of the broader Entra family (Permissions Management, Verified ID, etc.). Most CLIs still say `aad`. Documentation says "Entra ID, formerly Azure AD." Get used to seeing both.

Entra ID is a **cloud identity provider** — a SaaS multi-tenant directory of users, groups, devices, and applications. It is **not** Active Directory Domain Services. It speaks OAuth 2.0, OpenID Connect, SAML 2.0, and WS-Federation. It does not speak Kerberos, LDAP, or Group Policy. (You can layer **Entra Domain Services** on top if you need LDAP/Kerberos for legacy apps, but that's a separate paid service.)

The mental model: **Entra is to Azure (and M365 and your SaaS apps) what an IdP like Okta is to enterprise SaaS.** Every authentication into Azure ARM, every Microsoft 365 sign-in, every "Sign in with Microsoft" button on a SaaS app — all Entra.

Inspect your tenant:

```bash
az account show --query "{tenantId:tenantId, tenantDomain:user.name}"
az rest --method GET --url "https://graph.microsoft.com/v1.0/organization" \
  --query "value[0].{name:displayName, id:id, defaultDomain:verifiedDomains[?isDefault].name | [0]}"
```

That last command hits **Microsoft Graph**, the unified REST API for Entra and M365. Anything `az ad` doesn't cover, Graph does. You'll use it a lot.

## 2. Object types — the four you must know

Entra ID stores four kinds of security principals. Memorize them — every RBAC role assignment, every conditional access policy, every audit log entry references one of these.

### Users

Human (or human-shaped) identities. Each has a `userPrincipalName` (UPN, looks like an email) and an immutable `objectId` (a GUID). Two flavors:

- **Member** — internal to the tenant.
- **Guest** (B2B) — an external user invited via email. They sign in with their *home* tenant's credentials but appear as an object in *your* tenant. This is the right way to give a contractor access; never create a member account for them.

```bash
az ad user list --filter "startswith(displayName, 'Yati')" --query "[].{upn:userPrincipalName, id:id}" -o table
az ad user show --id yati@amityrobotics.com
```

### Groups

Containers for users (and other groups). Two flavors that matter:

- **Security groups** — for permissions. Assign RBAC to a group, add/remove users — done.
- **Microsoft 365 groups** — also carry a mailbox/Teams/SharePoint site. Can also be used for RBAC.

Plus **dynamic groups** (membership defined by a rule, e.g. `department eq "Engineering"`) and **assignable groups** (can be assigned Entra *roles*, not Azure RBAC roles — different concept).

```bash
az ad group create --display-name "Cloud Engineers" --mail-nickname cloud-eng
az ad group member add --group "Cloud Engineers" --member-id $(az ad signed-in-user show --query id -o tsv)
```

### Service Principals and App Registrations

This is the part that confuses everyone. There are **two objects**:

- **Application (App Registration)** — the *definition* of an app: its name, redirect URIs, API permissions, certificates, secrets. Lives in the tenant where it was registered (the **home tenant**). One object globally.
- **Service Principal** — an *instance* of the app inside a tenant. If your app is single-tenant, app + SP are in the same tenant. If multi-tenant (e.g., GitHub's Azure app), the app lives in GitHub's tenant and a service principal exists in *every customer tenant* that consents to it.

When you "give an app access to Azure," you grant RBAC to its **service principal** (not the app). When you "add a secret to an app," you add it on the **app registration**.

```bash
# Create an app + SP for an automation script.
az ad app create --display-name "ci-deploy-app" \
  --query "{appId:appId, id:id}" -o jsonc

APP_ID=<from above>

az ad sp create --id $APP_ID

# Add a password (client secret).
az ad app credential reset --id $APP_ID --years 1 --query "{appId:appId, password:password}" -o jsonc
```

In 2026 you should almost never use client secrets — use **federated credentials** (OIDC) or **certificates**. We'll cover this in module 16 (CI/CD).

### Managed Identities

A managed identity is "a service principal that Microsoft creates and rotates for you, attached to an Azure resource." When your VM, Function, App Service, Container App, or AKS pod needs to call Azure (Storage, Key Vault, SQL, anything), it asks the platform for a token tied to its managed identity. No secret to store, no rotation to schedule, no leak to fear.

Two kinds:

- **System-assigned** — tied 1:1 to a resource. Created with the resource, deleted with the resource. The default choice.
- **User-assigned** — a standalone resource. One MI can be attached to many compute resources. Use this when (a) multiple resources need the same identity, or (b) you want the MI to outlive the compute.

```bash
# System-assigned on a VM.
az vm identity assign --name vm-payments-01 --resource-group rg-payments

# Create a user-assigned MI as its own resource.
az identity create --name mi-payments-runtime --resource-group rg-payments
az vm identity assign --name vm-payments-01 --resource-group rg-payments \
  --identities $(az identity show --name mi-payments-runtime --resource-group rg-payments --query id -o tsv)
```

We will revisit managed identities throughout the course — they are the single most useful Azure feature.

## 3. Authentication, tokens, and consent

Every call to Azure (or any Entra-protected API) is an HTTP request with `Authorization: Bearer <token>`. The token is a JWT issued by Entra, valid ~60 minutes, signed by the tenant's signing key.

The flow your CLI uses:

1. `az login` opens a browser, you sign in, Entra issues an **ID token** and a **refresh token** to the CLI.
2. When you run `az vm list`, the CLI silently exchanges the refresh token for an **access token** scoped to `https://management.azure.com`.
3. The CLI calls ARM with that bearer token. ARM validates it via Entra and authorizes via Azure RBAC.

You can inspect the token:

```bash
az account get-access-token --resource https://management.azure.com --query accessToken -o tsv \
  | awk -F. '{print $2}' | base64 -d 2>/dev/null
```

You'll see claims like `aud` (audience — the API), `oid` (your Entra object ID — *this* is what RBAC role assignments target), `tid` (tenant ID), `roles` and `groups` (for app permissions), `scp` (delegated scopes).

### Consent — admin vs user

When an app needs permissions (e.g., "read your calendar"), an Entra admin or the user themselves must **consent**. Two flavors:

- **Delegated permissions** — the app acts *on behalf of* a signed-in user. Scope is whatever both the app *and* the user have.
- **Application permissions** — the app acts *as itself* (daemon mode). Always requires admin consent.

```bash
# Grant the Microsoft Graph "User.Read.All" application permission to your SP.
az ad app permission add --id $APP_ID \
  --api 00000003-0000-0000-c000-000000000000 \
  --api-permissions df021288-bdef-4463-88db-98f22de89214=Role
az ad app permission admin-consent --id $APP_ID
```

The GUID `00000003-0000-0000-c000-000000000000` is Microsoft Graph — memorize it; it shows up everywhere.

## 4. Conditional Access — the "if X then require Y" engine

Conditional Access (CA) is Entra's policy engine. Policies look like:

> *IF* user-in-group `Finance` *AND* accessing `Azure Management` *AND* signing-in from `untrusted location`
> *THEN* require `MFA` *AND* `compliant device`.

CA requires Entra ID **P1** licensing (or P2 for the more advanced features). It is the single most impactful security control in any Entra tenant — much more so than password policies in 2026.

CA policies are managed via Graph API (the `az` CLI doesn't directly support them). A minimal "require MFA for Azure Portal" example via Graph:

```bash
az rest --method POST \
  --url "https://graph.microsoft.com/v1.0/identity/conditionalAccess/policies" \
  --body '{
    "displayName": "Require MFA for Azure Portal",
    "state": "enabledForReportingButNotEnforced",
    "conditions": {
      "users": { "includeUsers": ["All"] },
      "applications": { "includeApplications": ["797f4846-ba00-4fd7-ba43-dac1f8f63013"] }
    },
    "grantControls": {
      "operator": "OR",
      "builtInControls": ["mfa"]
    }
  }'
```

The GUID `797f4846-ba00-4fd7-ba43-dac1f8f63013` is the Azure Management API. *Always* deploy CA policies in `enabledForReportingButNotEnforced` (report-only) first; verify in sign-in logs; then flip to `enabled`. CA policies have locked admins out of their own tenants more than once.

## 5. Practical Application — wire up a CI/CD service principal *the modern way*

Goal: A GitHub Actions workflow that deploys to Azure, with **no stored secret**.

```bash
# 1. Create the app + SP.
APP=$(az ad app create --display-name "gha-payments-deploy" --query appId -o tsv)
az ad sp create --id $APP
SP_OID=$(az ad sp show --id $APP --query id -o tsv)

# 2. Grant Contributor on the target RG.
az role assignment create \
  --assignee-object-id $SP_OID \
  --assignee-principal-type ServicePrincipal \
  --role Contributor \
  --scope $(az group show --name rg-payments-prod --query id -o tsv)

# 3. Create a federated credential trusting GitHub's OIDC issuer for the main branch of acme/payments.
az ad app federated-credential create \
  --id $APP \
  --parameters '{
    "name": "gha-main",
    "issuer": "https://token.actions.githubusercontent.com",
    "subject": "repo:acme/payments:ref:refs/heads/main",
    "audiences": ["api://AzureADTokenExchange"]
  }'

# 4. In GitHub repo secrets, set AZURE_CLIENT_ID=$APP, AZURE_TENANT_ID=<tenant>, AZURE_SUBSCRIPTION_ID=<sub>.
# In the workflow:
#   - uses: azure/login@v2
#     with:
#       client-id: ${{ secrets.AZURE_CLIENT_ID }}
#       tenant-id: ${{ secrets.AZURE_TENANT_ID }}
#       subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}
```

No client secret anywhere. GitHub's OIDC token is exchanged for an Entra access token at runtime. Rotating the credential = changing the federated subject (e.g., a different branch). This is the *only* pattern you should use for CI/CD in 2026 — module 16 has the full version.

## 5. Common Mistakes & Gotchas

- **Confusing Entra roles with Azure RBAC roles.** They are different role systems. *Entra roles* (`Global Administrator`, `User Administrator`, `Conditional Access Administrator`) govern the tenant — who can manage users, apps, etc. *Azure RBAC roles* (`Owner`, `Contributor`, `Reader`) govern Azure resources. A user can be a Global Admin and have *zero* access to any subscription's resources. The portal makes this distinction subtle; always ask "Entra role or Azure role?"
- **Confusing App Registration with Service Principal.** App = the definition. SP = the instance in this tenant. RBAC grants go on the SP's `objectId`, but `az login --service-principal` uses the App's `appId`. *Both IDs appear together everywhere*; using the wrong one in the wrong place is a classic 30-minute debugging session.
- **Storing client secrets.** Every long-lived secret is a future leak. Use federated credentials (OIDC) for CI/CD, managed identities for Azure-hosted compute, and certificates only when neither is possible.
- **Creating member accounts for contractors.** Use B2B guest invites. Cheaper (guests are free up to a generous monthly active limit), safer, and revocable in one click.
- **Forgetting to grant admin consent.** You configure an app's API permissions, run it, and get 403. Permissions need admin consent before they take effect for application-permission scopes.
- **Not enabling MFA from day one.** Modern Entra tenants get **security defaults** on by default — leave them on, or replace with proper CA policies. A new tenant without MFA enforcement is a P1 vulnerability.
- **Group sprawl.** It is *very* tempting to make a new group per project. Within a year you'll have 800 groups and no idea who owns what. Use **Privileged Identity Management (PIM)** for privileged groups, **access reviews** for permanent groups, and a naming convention from day one.
- **Misunderstanding `Directory.Read.All` vs `User.Read`.** The first is an admin-consent application permission that reads every user in your tenant. The second is the trivial "let me read my own profile" delegated scope. Apps frequently ask for far more than they need.
- **B2B guest data residency.** A guest user's home tenant controls their authentication. If their home tenant has weak security (no MFA, no CA), they bring that weakness into yours. Apply CA to guests.
- **"Why is my Azure CLI logged in as the wrong identity?"** Because `az account show` shows the **subscription**, not the **identity**. Use `az ad signed-in-user show --query "{name:displayName, upn:userPrincipalName, id:id}"` to confirm *who* you are.

## 🎯 Key Takeaways

- **Entra ID is the identity substrate for all of Azure.** Every auth decision — human or workload — flows through it.
- **Four object types: User, Group, App Registration + Service Principal, Managed Identity.** Internalize the App-vs-SP distinction and the system-assigned-vs-user-assigned MI distinction.
- **Managed identities replace stored secrets for any Azure-hosted compute.** Use federated credentials (OIDC) for any external CI/CD. Treat client secrets as a code smell.
- **Conditional Access is your highest-leverage security control.** Roll out in report-only first, watch sign-in logs, then enforce.
- **Entra roles ≠ Azure RBAC roles.** A Global Admin has tenant power, not subscription power, unless explicitly granted. We make that distinction concrete in module 03.

*← [prev](./01_account_and_entra.md) | [next → 03_rbac.md](./03_rbac.md)*
