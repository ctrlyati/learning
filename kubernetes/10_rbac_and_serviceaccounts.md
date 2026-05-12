# 10 — RBAC & ServiceAccounts

> **Goal:** Authoritatively answer "who can do what in this cluster?" Understand the auth flow (authn → authz → admission), the four RBAC objects (Role, ClusterRole, RoleBinding, ClusterRoleBinding), and how ServiceAccounts give pods identity.

---

## 1. RBAC — analogy + YAML

**Analogy.** RBAC is a **building access card system**. A *Role* is the permission set ("can enter floor 5, can use the conference rooms"). A *RoleBinding* is the act of giving that card to a person or group ("Alice, here's your floor-5 card"). The cards work only in their assigned building (namespace) — unless they're *master cards* (ClusterRole + ClusterRoleBinding) that work everywhere.

```yaml
# A Role — what permissions exist (namespace-scoped)
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: pod-reader
  namespace: payments
rules:
  - apiGroups: [""]
    resources: ["pods", "pods/log"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods/exec"]
    verbs: ["create"]
---
# A RoleBinding — who gets those permissions
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: alice-can-read-pods
  namespace: payments
subjects:
  - kind: User
    name: alice@example.com
    apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: pod-reader
  apiGroup: rbac.authorization.k8s.io
```

```bash
$ kubectl apply -f rbac.yaml
role.rbac.authorization.k8s.io/pod-reader created
rolebinding.rbac.authorization.k8s.io/alice-can-read-pods created

# Test: can alice (impersonated) list pods?
$ kubectl auth can-i list pods -n payments --as=alice@example.com
yes

$ kubectl auth can-i delete pods -n payments --as=alice@example.com
no

$ kubectl auth can-i list pods -n production --as=alice@example.com
no                              # wrong namespace
```

`kubectl auth can-i` is the single most useful debugging tool for RBAC. Memorize it.

---

## 2. Mechanism — the auth pipeline + the four RBAC objects

Every request to the apiserver goes through three stages:

```
┌──────────┐    ┌──────────┐    ┌────────────┐    ┌─────┐
│ Authn    │ →  │ Authz    │ →  │ Admission  │ →  │ etcd│
│ Who?     │    │ Can?     │    │ Mutate/Val │    │     │
└──────────┘    └──────────┘    └────────────┘    └─────┘
```

1. **Authentication** — who is making this request? Cert, bearer token (SA), OIDC JWT, webhook. Outputs a *user* + *groups*.
2. **Authorization** — is that user allowed to do this verb on this resource? RBAC is the default authorizer; Node, Webhook, ABAC are others.
3. **Admission control** — mutating (defaults, sidecar injection) and validating (PSS, OPA/Kyverno policies).

RBAC happens in step 2. It's purely *additive*: with no bindings, you can do nothing. There's no deny rule — restrict by *not* granting.

### The four RBAC objects

| Object | Scope | What it does |
|--------|-------|--------------|
| **Role** | Namespace | Lists allowed verbs on resources |
| **ClusterRole** | Cluster | Same, but cluster-wide (and for cluster-scoped resources like Nodes, PVs) |
| **RoleBinding** | Namespace | Grants a Role (or ClusterRole) to a subject *within one namespace* |
| **ClusterRoleBinding** | Cluster | Grants a ClusterRole to a subject *across all namespaces* |

The non-obvious combo: a **RoleBinding can reference a ClusterRole** to grant cluster-defined permissions but only within one namespace. This is the idiom — define a `developer` ClusterRole once, bind it per-namespace with RoleBindings.

### Verbs

Standard verbs: `get`, `list`, `watch`, `create`, `update`, `patch`, `delete`, `deletecollection`.
Special: `bind` (lets you create RoleBindings referencing this Role), `escalate` (lets you create Roles with more privileges than you have), `impersonate`.

### Resources, subresources, and apiGroups

- Plural: `pods`, `services`, `deployments`.
- Subresources are pathed: `pods/log`, `pods/exec`, `pods/portforward`, `deployments/scale`, `deployments/status`.
- `apiGroups: [""]` is core; `apiGroups: ["apps"]` is Deployments/StatefulSets/etc.; `apiGroups: ["*"]` is all.

```bash
$ kubectl api-resources -o wide       # list every resource + verbs + apiGroup
```

### Built-in ClusterRoles you should know

| ClusterRole | Use |
|-------------|-----|
| `cluster-admin` | Full superuser. Be miserly. |
| `admin` | Full namespace admin (for RoleBinding into a ns). |
| `edit` | Read/write most things in a namespace, no RBAC changes. |
| `view` | Read-only, no Secrets. |

```bash
$ kubectl describe clusterrole view | head -20
```

---

## 3. ServiceAccounts — identity for pods

A **ServiceAccount** is the namespace-scoped identity a pod runs as. Every pod has one (defaulting to `default`). The pod can use that identity to call the apiserver.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: deploy-bot
  namespace: payments
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: { name: deployer, namespace: payments }
rules:
  - apiGroups: ["apps"]; resources: ["deployments"]; verbs: ["get","list","watch","patch","update"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata: { name: deploy-bot-can-deploy, namespace: payments }
subjects:
  - kind: ServiceAccount
    name: deploy-bot
    namespace: payments
roleRef:
  kind: Role
  name: deployer
  apiGroup: rbac.authorization.k8s.io
```

Use it in a Pod/Deployment:

```yaml
apiVersion: v1
kind: Pod
metadata: { name: deployer-pod, namespace: payments }
spec:
  serviceAccountName: deploy-bot
  automountServiceAccountToken: true
  containers:
    - name: kubectl
      image: bitnami/kubectl
      command: ["sleep", "infinity"]
```

```bash
$ kubectl exec -n payments deployer-pod -- kubectl get deploy
NAME   READY
api    3/3

$ kubectl exec -n payments deployer-pod -- kubectl get pods -n production
Error from server (Forbidden): pods is forbidden: User "system:serviceaccount:payments:deploy-bot"
cannot list resource "pods" in API group "" in the namespace "production"
```

The username for an SA is always `system:serviceaccount:<namespace>:<name>`. Groups: `system:serviceaccounts`, `system:serviceaccounts:<namespace>`.

### How tokens are mounted into pods

Since K8s 1.22+, the default is **projected service-account tokens** with TTL and audience:

- A token is generated by the apiserver, short-lived (default 1h), bound to the pod and the SA.
- The kubelet refreshes it before expiry.
- Available at `/var/run/secrets/kubernetes.io/serviceaccount/token`.

Pre-1.22 used long-lived Secret-backed tokens. These still work but are deprecated.

You can request additional projected tokens with custom audiences and TTLs — this is how cloud "workload identity" (AWS IRSA, GKE Workload Identity, Azure AD Workload Identity) works:

```yaml
volumes:
  - name: aws-token
    projected:
      sources:
        - serviceAccountToken:
            path: token
            audience: sts.amazonaws.com
            expirationSeconds: 3600
```

The pod presents this token to AWS STS, which validates it against the cluster's OIDC issuer URL and hands back temporary AWS credentials. **No static credentials anywhere.**

### Don't mount tokens you don't need

```yaml
spec:
  automountServiceAccountToken: false
```

For pods that never call the apiserver, set this. Reduces blast radius if the pod is compromised.

---

## 4. Practical application — RBAC for a CI deploy bot

A CI runner (GitHub Actions, GitLab, ArgoCD) needs to deploy into multiple namespaces. Two approaches:

### Approach A — one SA per environment

```yaml
# In the cluster, in each environment namespace:
apiVersion: v1
kind: ServiceAccount
metadata: { name: ci, namespace: prod }
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: { name: ci-deployer, namespace: prod }
rules:
  - apiGroups: ["apps"]; resources: ["deployments", "statefulsets"]; verbs: ["get","list","watch","create","update","patch"]
  - apiGroups: [""]; resources: ["services", "configmaps"]; verbs: ["get","list","watch","create","update","patch"]
  - apiGroups: [""]; resources: ["secrets"]; verbs: ["get","list"]   # read-only on secrets
  - apiGroups: ["batch"]; resources: ["jobs"]; verbs: ["create","get","list","watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata: { name: ci-can-deploy, namespace: prod }
subjects: [{ kind: ServiceAccount, name: ci, namespace: prod }]
roleRef: { kind: Role, name: ci-deployer, apiGroup: rbac.authorization.k8s.io }
```

The CI system exchanges its OIDC token (from GitHub Actions, say) for this SA's token via the apiserver's TokenRequest API.

### Approach B — one cluster-wide `deployer` ClusterRole, bound per-namespace

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: { name: deployer }
rules:
  - apiGroups: ["apps"]; resources: ["deployments", "statefulsets"]; verbs: ["*"]
  - apiGroups: [""]; resources: ["services", "configmaps"]; verbs: ["*"]
---
# Per-namespace binding — reuses the ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata: { name: ci-deployer, namespace: prod }
subjects: [{ kind: ServiceAccount, name: ci, namespace: prod }]
roleRef: { kind: ClusterRole, name: deployer, apiGroup: rbac.authorization.k8s.io }
```

Less duplication, easier to evolve the role.

### Auditing your permissions

```bash
$ kubectl auth can-i --list -n prod --as=system:serviceaccount:prod:ci
Resources                                       Non-Resource URLs   Resource Names   Verbs
deployments.apps                                []                  []               [get list watch create update patch]
services                                        []                  []               [get list watch create update patch]
...

$ kubectl auth can-i create rolebindings -n prod --as=system:serviceaccount:prod:ci
no
```

A scheduled job that runs `kubectl auth can-i --list` for every SA and diffs against a known-good baseline is a great drift detector.

### Inspecting a real request

```bash
$ kubectl --v=8 get pod foo 2>&1 | grep -E 'Authorization|GET'
```

Shows the exact bearer token (in `--token` setups) and the URL — useful for debugging "why is this failing through the LB but works locally?"

---

## 5. Common Mistakes & Gotchas

- **`cluster-admin` to everyone.** "Just give the dev team admin, we'll fix it later." Never gets fixed. Start narrow and widen on request.
- **Granting `secrets: list`.** That's effectively granting *all secrets in the namespace* — even ones with passwords for resources the user shouldn't reach. Be specific.
- **`apiGroups: ["*"]` and `resources: ["*"]`.** Almost always too broad. Granting `*` over the entire `apps` group is one thing; over `*` is "you can list all the Secrets."
- **RoleBinding referencing a ClusterRole, expecting cluster-wide effect.** No — the RoleBinding still namespaces it. To go cluster-wide you need ClusterRoleBinding.
- **Forgetting ClusterRole/ClusterRoleBinding for cluster-scoped resources.** A Role with `resources: ["nodes"]` is useless — nodes aren't namespaced.
- **`escalate` and `bind` verbs.** Without them, a user can't create a Role/Binding more privileged than themselves (good). With them, they can. Audit who has these.
- **Default SA mounted into every pod by default.** If the default SA has no special bindings (it shouldn't), this is harmless — but if someone bound it to anything, every pod inherits. Set `automountServiceAccountToken: false` for pods that don't need it.
- **Long-lived SA token Secrets.** Pre-1.22 auto-created Secrets are forever-valid. Rotate or remove; modern projected tokens are TTL'd.
- **Pod can call `kubectl get pods` even with no role.** That's because the default SA has no special permissions — but the *namespace-level* default ClusterRoleBinding (`system:authenticated`) grants harmless things. Double-check.
- **RBAC changes don't apply to currently-open watch connections.** A user with `watch` permission revoked still gets events until their stream closes. Restart sensitive controllers if you tightened their access.
- **OIDC group claims missing.** You bind `RoleBinding` to a group, but the apiserver doesn't see groups in the OIDC token. `kubectl --v=8` to debug; ensure OIDC config maps the `groups` claim.

---

## 🎯 Key Takeaways

- **RBAC is additive and namespace-aware.** Knowing both rules (no deny; bindings are scoped) unlocks instant intuition about why permissions work or don't.
- **`kubectl auth can-i --list --as=<subject>` is the senior engineer's first move.** Far faster than reading YAML.
- **ServiceAccount = pod identity, full stop.** Whether bridging to AWS IAM (IRSA), GCP, Vault, or just calling kubectl — every modern auth pattern starts with a SA.
- **Projected tokens with audiences are the future.** Static long-lived tokens are a smell. If you see them in a cluster, file a ticket.
- **Audit roles, not just bindings.** A role granting `secrets: list` looks innocuous on its own; combined with a wide binding, it's the keys to the kingdom.

*← [prev](./09_namespaces_labels_selectors.md) | [next →](./11_networking_cni_dns_netpol.md)*
