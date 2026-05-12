# 09 — Namespaces, Labels, Annotations & Selectors

> **Goal:** Master the organizing primitives. Namespaces for hard partitioning, labels as the universal join key, selectors as the query language, annotations for tool-level metadata. Get this wrong and the cluster becomes unmanageable; get it right and everything composes.

---

## 1. Namespace — analogy + YAML

**Analogy.** A Namespace is a **floor in an office building**. Same building (cluster), same elevators (apiserver), but you don't see what's happening on other floors unless you go there. Useful for separating teams, environments (dev/staging/prod in one cluster), or tenants. **Not** a security boundary by default — pods on floor 5 can still talk to pods on floor 7 unless a NetworkPolicy stops them.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: payments
  labels:
    team: payments
    env: prod
    pod-security.kubernetes.io/enforce: restricted
```

```bash
$ kubectl create namespace payments
namespace/payments created

$ kubectl get ns
NAME              STATUS   AGE
default           Active   2h
kube-node-lease   Active   2h
kube-public       Active   2h
kube-system       Active   2h
payments          Active   5s

$ kubectl apply -f deploy.yaml -n payments
deployment.apps/api created

$ kubectl config set-context --current --namespace=payments    # default for this kubeconfig
Context "kind-learn" modified.
```

### What's namespaced vs cluster-scoped?

**Namespaced** (most common): Pods, Services, Deployments, ConfigMaps, Secrets, PVCs, Roles, RoleBindings, Ingresses, NetworkPolicies, Jobs, CronJobs.

**Cluster-scoped**: Nodes, PersistentVolumes, StorageClasses, ClusterRoles, ClusterRoleBindings, IngressClasses, CRDs, Namespaces themselves.

`kubectl api-resources --namespaced=true` lists all namespaced types.

---

## 2. Labels — the universal join key

**Labels are queryable key/value pairs on objects.** They're how Kubernetes does "loose coupling" — Services, ReplicaSets, NetworkPolicies, Deployments, HPAs all use **label selectors** to find what they manage or target. No object directly references another by name (with rare exceptions like ConfigMap references).

```yaml
metadata:
  labels:
    app.kubernetes.io/name: api
    app.kubernetes.io/instance: api-prod
    app.kubernetes.io/version: "1.4.2"
    app.kubernetes.io/component: backend
    app.kubernetes.io/part-of: shop
    app.kubernetes.io/managed-by: helm
    env: prod
    team: payments
```

The `app.kubernetes.io/*` keys are the **recommended labels** — every tool in the ecosystem (kubectl, Helm, Grafana dashboards, Lens, ArgoCD) understands them. Use them.

### Label selectors

Two forms:

**Equality-based** (simplest):
```yaml
selector:
  matchLabels:
    app: api
    env: prod
```
All labels must match (AND).

**Set-based** (richer):
```yaml
selector:
  matchExpressions:
    - { key: env, operator: In, values: [prod, staging] }
    - { key: team, operator: Exists }
    - { key: deprecated, operator: DoesNotExist }
```

Operators: `In`, `NotIn`, `Exists`, `DoesNotExist`.

### Query labels with kubectl

```bash
$ kubectl get pods -l env=prod                             # equality
$ kubectl get pods -l 'env in (prod,staging)'              # set
$ kubectl get pods -l 'env=prod,team=payments'             # AND
$ kubectl get pods -l '!deprecated'                        # not present
$ kubectl get pods --show-labels                           # see all labels
```

---

## 3. Annotations vs labels — the distinction senior engineers always make

| | Labels | Annotations |
|---|--------|-------------|
| **Purpose** | Identify & select | Attach metadata for tools/humans |
| **Queryable by selectors?** | Yes | No |
| **Used by controllers for matching?** | Yes | No |
| **Size limit** | Short strings | Up to ~256 KB |
| **Examples** | `app=api`, `env=prod` | git SHAs, last-applied configuration, ingress-controller hints, change-cause, build IDs |

```yaml
metadata:
  labels:
    app: api
  annotations:
    kubernetes.io/change-cause: "v1.4.2 — fix login timeout"
    nginx.ingress.kubernetes.io/proxy-body-size: 50m
    argocd.argoproj.io/sync-wave: "1"
    iam.amazonaws.com/role: arn:aws:iam::123:role/api-role
    git.sha: "5f9e2bc7d3..."
```

The rule of thumb: **if a controller needs to find this object, it's a label. If a human or tool needs to read this on the object, it's an annotation.**

---

## 4. Practical application — a namespace per tenant, labels for blast-radius

### Organizing principle: namespaces by environment OR team OR tenant

There's no single right answer. Common patterns:

| Pattern | When | Tradeoffs |
|---------|------|-----------|
| **One ns per app** (`api`, `web`, `payments`) | Small/medium org, single env per cluster | Clean RBAC; multiple clusters for env separation. |
| **Env in name** (`api-prod`, `api-staging`) | One cluster, multiple envs | Cheaper infra; risk of cross-env mistakes; RBAC by name prefix. |
| **Team-scoped** (`team-payments`) | Platform teams own clusters; app teams own namespaces | Clear ownership; need namespace-internal organization. |
| **Per-tenant** (`tenant-acme`, `tenant-globex`) | SaaS multi-tenancy | Network policies + RBAC are critical; quotas mandatory. |

### A realistic namespace with quotas and limits

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: payments
  labels:
    team: payments
    env: prod
    pod-security.kubernetes.io/enforce: restricted
---
apiVersion: v1
kind: ResourceQuota
metadata: { name: payments-quota, namespace: payments }
spec:
  hard:
    requests.cpu: "20"
    requests.memory: 40Gi
    limits.cpu: "40"
    limits.memory: 80Gi
    pods: "100"
    persistentvolumeclaims: "10"
    services.loadbalancers: "2"
---
apiVersion: v1
kind: LimitRange
metadata: { name: defaults, namespace: payments }
spec:
  limits:
    - type: Container
      default:        { cpu: "200m", memory: "256Mi" }
      defaultRequest: { cpu: "50m",  memory: "64Mi" }
      max:            { cpu: "2",    memory: "2Gi" }
```

- **ResourceQuota** caps total resources for the namespace.
- **LimitRange** sets default/min/max per container — so pods without explicit requests/limits get reasonable defaults instead of running unbounded.

```bash
$ kubectl describe quota -n payments
Name:            payments-quota
Resource         Used   Hard
--------         ----   ----
limits.cpu       4      40
limits.memory    8Gi    80Gi
pods             12     100
requests.cpu     2      20
requests.memory  4Gi    40Gi
```

When the quota is hit, new pods fail to create with a clear error — much better than letting one team starve the cluster.

### Labels for SRE: blast-radius queries

A senior platform engineer wants to answer "what's our exposure to this incident?" in 30 seconds. Good labeling makes that one query:

```bash
$ kubectl get pods -A -l 'env=prod,tier=backend' --show-labels
$ kubectl get deploy -A -l 'team=payments,app.kubernetes.io/version=1.4.2'
```

If the answer requires grepping pod names, you have a labeling problem.

### Cross-namespace references — they don't exist (mostly)

Services in `payments` can be reached from `web` namespace as `<svc>.payments.svc.cluster.local`. But you cannot:
- Mount a ConfigMap/Secret from another namespace into a Pod.
- Reference a PVC across namespaces.
- Bind a Role to a SA from another namespace via RoleBinding (use ClusterRoleBinding instead).

This is intentional. Namespaces are designed to be soft boundaries you can lean against without locking everything down.

---

## 5. Common Mistakes & Gotchas

- **Working in the wrong namespace.** `kubectl config current-context` and `kubectl config view --minify -o jsonpath='{..namespace}'`. Burn `kubens` (from `kubectx`) into your tooling.
- **Treating namespaces as security boundaries by default.** They're not. NetworkPolicies + RBAC are. A pod in `dev` can curl a Service in `prod` unless you stop it.
- **Inconsistent labels across a workload.** Deployment selector says `app=api`, but a CI tool added `app: api-svc` to one pod. Service stops including it. Lint your labels in CI.
- **Changing a Deployment's `selector` after creation.** Immutable. Apiserver rejects. Delete and recreate.
- **No `app.kubernetes.io/*` labels.** Lens / k9s / ArgoCD / Grafana dashboards all look for them. Missing → broken UX.
- **Long labels with bad characters.** Labels: max 63 chars per value, alphanumeric + `-_.`. Don't try to put a URL in there.
- **Putting secrets in annotations.** Annotations are visible to anyone with `get` on the object — same as label visibility. No security.
- **Cluster-wide listing without selectors as a habit.** `kubectl get pods -A` returns everything in big clusters and ruins your day. Always select.
- **Forgetting LimitRange in shared namespaces.** A pod without resources defaults to "wants nothing, accepts everything" — recipe for noisy-neighbor incidents.
- **Quota fights with autoscalers.** HPA wants 20 replicas; quota allows 15. Pods stuck pending. Watch your `ResourceQuotaExceeded` events.
- **Namespace name as an environment indicator that doesn't match labels.** `kubectl get pods -n prod -l env=staging` returning results is the moment someone realizes the system is inconsistent.

---

## 🎯 Key Takeaways

- **Labels are the join key for everything.** A senior engineer audits a new cluster by reading the labels first — they reveal the team's mental model.
- **Pick a labeling scheme on day one and lint it.** Retrofitting labels across thousands of objects is misery. The `app.kubernetes.io/*` set is a free, well-supported starting point.
- **Namespaces partition, they don't isolate.** Treat them as visual/RBAC scopes; reach for NetworkPolicy and quotas for real boundaries.
- **ResourceQuota + LimitRange in every shared namespace is table stakes.** Without them, one bad Deployment can take down everyone else.
- **Annotations are a metadata side-channel — use them aggressively.** Change-cause, deploy SHA, owner, runbook URL: future-you will thank you when paged at 3am.

*← [prev](./08_statefulsets_jobs_daemonsets.md) | [next →](./10_rbac_and_serviceaccounts.md)*
