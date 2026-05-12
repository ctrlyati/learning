# 16 — Operators & CRDs

> **Goal:** Extend the Kubernetes API. Understand CustomResourceDefinitions (CRDs), the controller pattern (the reconcile loop), and how the Operator pattern turns deep operational knowledge into automated software.

---

## 1. Custom Resources & the Operator Pattern — analogy + first CRD

**Analogy.** Kubernetes ships with a fixed list of nouns: Pod, Service, Deployment. A **CRD** is "I'd like to add a new word to your vocabulary." Once added, the apiserver knows what a `Postgres` or `Certificate` is — same REST endpoints, same RBAC, same `kubectl get`. An **Operator** is a controller that knows what to *do* when you create one of those new objects: a software engineer's encoded version of a DBA's runbook.

The Operator Pattern is:
> "A method of packaging, deploying, and managing a Kubernetes application — typically a stateful or complex one — using custom resources to manage the application and its components."

### A minimal CRD

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: backups.example.com
spec:
  group: example.com
  scope: Namespaced
  names:
    plural: backups
    singular: backup
    kind: Backup
    shortNames: [bk]
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              required: [source, schedule]
              properties:
                source:
                  type: string
                  description: "PVC name to back up"
                schedule:
                  type: string
                  description: "Cron expression"
                retention:
                  type: integer
                  default: 7
            status:
              type: object
              properties:
                lastBackup: { type: string, format: date-time }
                phase: { type: string, enum: [Pending, Running, Succeeded, Failed] }
      subresources:
        status: {}
      additionalPrinterColumns:
        - { name: Schedule,    type: string, jsonPath: .spec.schedule }
        - { name: LastBackup,  type: string, jsonPath: .status.lastBackup }
        - { name: Phase,       type: string, jsonPath: .status.phase }
```

```bash
$ kubectl apply -f backup-crd.yaml
customresourcedefinition.apiextensions.k8s.io/backups.example.com created

$ kubectl api-resources | grep backups
backups       bk    example.com/v1   true   Backup

# Now Backup is a real K8s resource!
$ kubectl apply -f - <<EOF
apiVersion: example.com/v1
kind: Backup
metadata: { name: nightly-pg }
spec:
  source: pg-data
  schedule: "0 2 * * *"
  retention: 14
EOF
backup.example.com/nightly-pg created

$ kubectl get bk
NAME         SCHEDULE     LASTBACKUP   PHASE
nightly-pg   0 2 * * *
```

The CRD gave us the noun. **Nothing happens** when you create one yet — there's no controller. The cluster has accepted "yes, this is a Backup object." Acting on it is the operator's job.

---

## 2. Mechanism — the reconcile loop

A **controller** is a process that:

1. **Watches** the apiserver for changes to specific resources (its CRD + supporting types like Pods, Jobs).
2. On each change, **enqueues a reconcile key** (`namespace/name`) into a work queue.
3. A worker pulls keys, fetches **current state** (the CR + related objects).
4. Compares to **desired state** (the CR's `spec`).
5. Takes action to converge (create/update/delete child resources).
6. Updates the CR's `status` to reflect what happened.
7. Returns; the loop will fire again on the next change or after a periodic resync.

```
       ┌────────────────────────┐
       │ apiserver (etcd)       │
       │  Backup spec changes   │
       └────────────────────────┘
                  │
            watch │
                  ↓
        ┌─────────────────┐
        │   Controller    │  ← never sleeps
        │  Reconcile(key) │
        └─────────────────┘
                  │
   create/update  │  read/write
                  ↓
        Pods, Jobs, Snapshots, CRs...
```

**Idempotency is everything.** Reconcile can be called for the same key many times — it must always do the right thing, regardless of how many times it ran before. The pattern:
- "If a Job for this Backup doesn't exist, create it."
- "If it exists and finished, update status."
- "If it failed, increment retry count or surface the error."

You **never write imperative scripts** like "step 1, step 2." You write *level-based* reconcilers that observe state and act.

### Where controllers run

A controller is just a Pod in the cluster (or, sometimes, outside it, hitting the apiserver via kubeconfig). The Operator pattern bundles:
- The CRD(s)
- The controller binary
- RBAC (its own SA + Role to watch/modify its resources)
- Often a Helm chart or Operator Lifecycle Manager (OLM) integration

### Building one — frameworks

| Framework | Language | Notes |
|-----------|----------|-------|
| **Kubebuilder** | Go | The official toolkit. Generates scaffolding for CRDs + controllers. |
| **Operator SDK** | Go / Ansible / Helm | Red Hat-led; bundles Kubebuilder + Ansible/Helm modes. |
| **controller-runtime** | Go library | Underlies Kubebuilder. |
| **kopf** | Python | Python-friendly; less common in prod but great for prototypes. |
| **Metacontroller** | YAML + webhook | Write reconcile logic as a web service in any language. |

### A taste of a Go controller (controller-runtime)

```go
func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var backup examplev1.Backup
    if err := r.Get(ctx, req.NamespacedName, &backup); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Find or create the Job that runs the backup
    job := &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      backup.Name + "-job",
            Namespace: backup.Namespace,
        },
        Spec: r.buildJobSpec(&backup),
    }
    if err := ctrl.SetControllerReference(&backup, job, r.Scheme); err != nil {
        return ctrl.Result{}, err
    }
    if err := r.Create(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
        return ctrl.Result{}, err
    }

    // Update status
    backup.Status.Phase = "Running"
    if err := r.Status().Update(ctx, &backup); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{RequeueAfter: time.Minute}, nil
}
```

The `SetControllerReference` adds an `ownerReference` so when the Backup is deleted, K8s garbage-collects the Job too.

---

## 3. Variations — operator maturity, popular operators, conversion webhooks

### Operator capability levels (CNCF model)

| Level | Capability |
|-------|-----------|
| 1 | Basic install — deploy the app via CR |
| 2 | Seamless upgrades — operator handles version migrations |
| 3 | Full lifecycle — backups, failure recovery |
| 4 | Deep insights — metrics, logs, alerts |
| 5 | Auto-pilot — auto-scaling, auto-config-tuning, anomaly detection |

Most production-grade operators (Postgres, Kafka, Elastic) are level 3-4. Level 5 is rare and worth the premium.

### Popular operators

| Operator | What it manages |
|----------|----------------|
| **cert-manager** | TLS certificate issuance & renewal (Let's Encrypt, internal CAs) |
| **CloudNative-PG** / **Zalando Postgres Operator** / **CrunchyData PGO** | Postgres clusters |
| **Strimzi** | Kafka clusters |
| **Elastic ECK** | Elasticsearch / Kibana |
| **MongoDB Community Operator** | MongoDB replica sets |
| **Argo CD / Argo Rollouts / Argo Workflows** | GitOps + progressive delivery + workflow orchestration |
| **External Secrets Operator** | Sync secrets from Vault/AWS SM/etc. |
| **Prometheus Operator** | Prometheus/Alertmanager + ServiceMonitor CRDs |
| **Knative** | Serverless on Kubernetes |
| **Istio / Linkerd** | Service mesh |
| **Velero** | Cluster backup/restore |
| **Karpenter** | Just-in-time node provisioning |

Notice how much of the modern K8s ecosystem is "an operator + its CRDs." This is the *plug-in architecture* of the platform.

### Conversion webhooks
When a CRD's schema evolves (`v1alpha1` → `v1`), conversion webhooks transform old-stored objects into the new shape on read. Lets you bump versions without rewriting etcd.

### Validation webhooks (and ValidatingAdmissionPolicy)
- **OpenAPI schema in the CRD** — basic validation (type, enum, required).
- **Validating webhook** — arbitrary validation logic in a service the apiserver calls.
- **ValidatingAdmissionPolicy** (recent) — declarative CEL-based policies, no webhook needed.

### Finalizers
A CRD instance can have `metadata.finalizers: ["example.com/cleanup"]`. When you delete it, K8s sets a `deletionTimestamp` but doesn't remove the object until *all* finalizers are gone. Operators use this for "clean up the underlying resource before letting the API record disappear" — like draining a Postgres replica before deletion.

### When *not* to write an operator
- Your app is stateless and a Deployment fits. (No.)
- A CronJob would suffice. (Probably no.)
- An existing operator does 80% of what you need. (Use it.)
- You don't have someone to maintain Go controller code in the long run. (Definitely no.)

Operators are great until you own one in production for two years.

---

## 4. Practical application — installing cert-manager and using its CRDs

cert-manager is the canonical example: clean operator, useful CRDs, deeply integrated with the ecosystem.

```bash
$ kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.15.0/cert-manager.yaml

$ kubectl -n cert-manager get pods
NAME                                       READY   STATUS    RESTARTS   AGE
cert-manager-7f59c8c9c4-h82xk              1/1     Running   0          45s
cert-manager-cainjector-687b5bc94f-n5jqx   1/1     Running   0          45s
cert-manager-webhook-7d4d4f4d5d-rqd2v      1/1     Running   0          45s

$ kubectl api-resources | grep cert-manager
certificates             cert      cert-manager.io/v1                  true    Certificate
certificaterequests      cr,crs    cert-manager.io/v1                  true    CertificateRequest
clusterissuers                     cert-manager.io/v1                  false   ClusterIssuer
issuers                            cert-manager.io/v1                  true    Issuer
```

Three pods. Three new CRDs (and a few more). That's the entire operator.

Use it:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata: { name: letsencrypt-prod }
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: ops@example.com
    privateKeySecretRef: { name: letsencrypt-prod-key }
    solvers:
      - http01:
          ingress: { class: nginx }
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata: { name: app-tls, namespace: default }
spec:
  secretName: app-tls
  issuerRef: { name: letsencrypt-prod, kind: ClusterIssuer }
  commonName: app.example.com
  dnsNames: [app.example.com]
```

```bash
$ kubectl apply -f cert.yaml
clusterissuer.cert-manager.io/letsencrypt-prod created
certificate.cert-manager.io/app-tls created

$ kubectl get certificate
NAME       READY   SECRET    AGE
app-tls    False   app-tls   12s    # operator is working...

$ kubectl get certificate
NAME       READY   SECRET    AGE
app-tls    True    app-tls   90s    # done — Let's Encrypt issued

$ kubectl get secret app-tls
NAME       TYPE                DATA   AGE
app-tls    kubernetes.io/tls   2      30s

$ kubectl describe certificate app-tls | tail -20
Status:
  Conditions:
    Type:    Ready
    Status:  True
    Reason:  Ready
    Message: Certificate is up to date and has not expired
  Not After: 2026-08-13T...
  Renewal Time: 2026-07-14T...
Events:
  Normal  Issuing     2m  cert-manager  Issuing certificate as Secret does not exist
  Normal  Generated   2m  cert-manager  Generated temporary private key
  Normal  Requested   2m  cert-manager  Created new CertificateRequest
  Normal  Issued      1m  cert-manager  The certificate has been successfully issued
```

The operator handled: ACME challenge orchestration, DNS/HTTP solver coordination, certificate retrieval, Secret creation, scheduled renewal 30 days before expiry. All from one `Certificate` CR.

---

## 5. Common Mistakes & Gotchas

- **CRDs without schema.** Pre-v1, CRDs could have no schema — anything went. Now `openAPIV3Schema` is required. Be strict; a CRD without validation lets users write nonsense that crashes your operator.
- **Storing secrets in CR spec.** The CR is visible to anyone with `get` permission. Reference a Secret by name; don't inline credentials.
- **Mutating an object inside the reconcile loop without retrying on conflict.** Two reconciles racing → resource-version conflict → silent failure. Always handle `errors.IsConflict`.
- **No status subresource.** Updates to `spec` and `status` go through the same endpoint → permissions become coarse, and your status updates can collide with user spec changes. Always enable `subresources.status`.
- **Status fields used for control.** Status is observation-only. Anything that drives decisions belongs in spec or in another resource.
- **Reconcile time longer than the lease.** Long-running reconciles → leader-election lease expires → another instance takes over → state corruption. Keep reconciles short and idempotent.
- **Finalizer left behind.** Operator crashes before removing its finalizer → CR stuck in `Terminating` forever. `kubectl patch ... --type=merge -p '{"metadata":{"finalizers":[]}}'` is the emergency rescue.
- **CRD scope mismatch.** You change a CRD from `Namespaced` to `Cluster` (or vice versa) — apiserver rejects. Means delete + recreate (data loss). Pick scope carefully on day one.
- **Cluster-wide watch on a namespaced operator.** Performance + RBAC headache. Scope the watch.
- **`kubebuilder` markers out of sync with code.** Generated manifests stop matching the controller. Re-run `make manifests` in CI.
- **Two operators reconciling the same CR.** Edge case — happens when migrating between operators. Use leader election + non-overlapping ownership labels.
- **Conversion webhook with no fallback.** Webhook crashes → all reads/writes to that CRD fail. Always keep a `served: true` for the latest version even if older is `storage: true`.
- **No metrics from the operator.** `controller-runtime` exposes Prometheus metrics for free; many operator authors forget to scrape them.

---

## 🎯 Key Takeaways

- **CRDs + controllers are how K8s is meant to be extended.** Bolt-on automation is a smell; new nouns + reconcilers is the idiom.
- **The reconcile loop is level-based, not event-based.** Always: "look at the world, make it look like the spec." This is the deepest mental model in K8s.
- **A senior engineer's first question about a new operator: 'what's its capability level and what does the reconcile do?'** Levels 4-5 save real ops time; level 1 is just packaged YAML.
- **Don't write an operator until you've exhausted built-ins.** Many problems are solved by Deployment + ConfigMap + a CronJob. The bar for adding code-you-own is high.
- **`ownerReferences` are how K8s garbage-collects.** Set them on every child resource your controller creates — leaks become tracked relationships.

*← [prev](./15_helm_and_kustomize.md) | [next →](./17_security.md)*
