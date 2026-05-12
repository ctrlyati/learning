# 18 — Production Kubernetes

> **Goal:** Operate a real cluster. Understand managed services (EKS/GKE/AKS), GitOps with Argo CD/Flux, upgrade strategies, disaster recovery, and the day-2 operations practices that separate "running K8s" from "running K8s well."

---

## 1. Managed Kubernetes — analogy + comparison

**Analogy.** Self-managing a cluster is **owning a chef's kitchen** — total control, total responsibility. Managed K8s (EKS, GKE, AKS, DigitalOcean DOKS, Linode LKE, etc.) is **the building's commercial kitchen rented to you** — they own the building, the gas, the inspections; you own what you cook. For 95% of teams, managed is the right answer.

### What managed actually manages

| Layer | Self-managed | EKS | GKE Autopilot | AKS |
|-------|-------------|-----|---------------|-----|
| etcd | You | They | They | They |
| apiserver | You | They (HA) | They (HA) | They (HA) |
| controllers | You | They | They | They |
| Node OS patches | You | You | They | Optional |
| Node provisioning | You | You / Karpenter | They (Autopilot) | You / VMSS |
| Upgrade orchestration | You | Semi-auto | Auto | Semi-auto |
| Networking plugin | You | AWS VPC CNI | GKE native | Azure CNI |
| Pricing | Infra cost | $0.10/hr/cluster + nodes | Per-pod | Free control plane + nodes |

The big tradeoffs:

- **EKS** — most flexibility, most "you still operate the kubelet." Karpenter is the now-canonical node scaler.
- **GKE Autopilot** — most "just run pods." You don't see nodes; Google bills per pod-second. Restrictive on what features you can use.
- **AKS** — competitive, deep AAD integration; good if you're already Azure.

### Cluster API and the future of cluster lifecycle
**Cluster API (CAPI)** is "K8s-style API for managing clusters themselves." You write a `Cluster` CR; an operator provisions infrastructure and bootstraps a cluster on it. Used by RKE2, Rancher, Tanzu, and increasingly by platform teams that want a fleet-of-clusters model with consistent provisioning. Worth knowing exists.

---

## 2. GitOps — mechanism + Argo CD vs Flux

**The pitch.** Stop running `kubectl apply` from CI. Instead:

1. Cluster state lives in Git (manifests, Helm values, Kustomize overlays).
2. An in-cluster controller continuously reconciles **Git → cluster**.
3. To change anything, you merge a PR. The controller picks up the change within seconds.
4. To roll back, you `git revert`. Same flow.

Benefits: auditable (every change is a commit), pull-based (no CI credentials with cluster access), self-healing (drift gets reverted), disaster recovery (re-bootstrap a new cluster from Git).

### Argo CD — the popular choice

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata: { name: api, namespace: argocd }
spec:
  project: default
  source:
    repoURL: https://github.com/acme/manifests
    targetRevision: main
    path: apps/api/overlays/prod
  destination:
    server: https://kubernetes.default.svc
    namespace: prod
  syncPolicy:
    automated:
      prune: true       # delete resources removed from Git
      selfHeal: true    # revert manual edits in cluster
    syncOptions: [CreateNamespace=true]
```

Argo CD adds a great UI for visualizing the manifest tree and diff. **ApplicationSets** generate Applications programmatically — one Application per environment, per cluster, etc. — the standard pattern for fleet management.

### Flux

YAML-only, no UI by default. Composed of small controllers (`source-controller`, `kustomize-controller`, `helm-controller`, `notification-controller`). Lighter, more GitOps-purist. Often pairs with Weave GitOps UI.

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata: { name: manifests, namespace: flux-system }
spec:
  url: https://github.com/acme/manifests
  ref: { branch: main }
  interval: 1m
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata: { name: api, namespace: flux-system }
spec:
  sourceRef: { kind: GitRepository, name: manifests }
  path: ./apps/api/overlays/prod
  prune: true
  interval: 5m
  targetNamespace: prod
```

### Which to pick

Both work. Argo CD wins on UX/visibility; Flux wins on minimalism + multi-tenancy. Most teams pick Argo and don't regret it.

### Progressive delivery alongside GitOps
**Argo Rollouts** + Argo CD: GitOps for what should be deployed, plus weighted traffic shifts (canary, blue/green) for *how*. Flagger (typically with Flux) plays the equivalent role.

---

## 3. Variations — upgrades, DR, multi-cluster, the operating manual

### Upgrading Kubernetes

A K8s minor release every ~4 months; 14 months of support per release. You will be upgrading approximately forever.

The order matters:
1. **kubectl** (your laptop) — keep within ±1 minor of the server.
2. **Control plane** (apiserver, scheduler, controller-manager, etcd).
3. **Nodes** (kubelet, kube-proxy, container runtime, OS).
4. **Add-ons** (CNI, ingress controller, CSI drivers, cert-manager, etc.).

**Skew rules.** kubelet can be up to 3 minors *behind* the apiserver, never ahead. kubectl ±1.

**Strategy (managed):**
- Use a separate non-prod cluster to test the new version against your add-ons.
- Read release notes for **API deprecations** (e.g., `policy/v1beta1` PDB → `policy/v1`).
- Use `kubectl deprecations` (plugin) or `pluto` to find deprecated APIs in your manifests.
- Drain and replace nodes (managed clusters do this for you with surge/replace strategies).
- Always have a **rollback plan** — usually a parallel old-version cluster you can shift traffic back to (cluster-level rollback is rarely a viable in-place option).

```bash
$ kubectl get nodes
NAME     STATUS                     ROLES   AGE   VERSION
node-1   Ready,SchedulingDisabled   <none>  90d   v1.29.5    # being drained
node-2   Ready                      <none>  60s   v1.30.0    # new node
```

### Disaster recovery

**What you must be able to restore:**
1. The cluster control plane (managed: nothing; self-managed: etcd snapshot).
2. The cluster's intent (GitOps: Git is the source).
3. The state stored in pods (PVCs, databases).
4. The secrets (if not stored externally).

**etcd backups** for self-managed clusters — `etcdctl snapshot save` on a schedule.

**Velero** is the standard for backing up cluster *state*:
```bash
$ velero backup create nightly --include-namespaces=prod
$ velero backup get
$ velero restore create --from-backup=nightly-2026-05-12
```

Velero can also snapshot CSI volumes — captures the PVC contents at the time of backup.

**RPO and RTO** decisions are not Kubernetes-specific but worth being explicit about:
- **RPO** (Recovery Point Objective) — how much data can you lose? 1 minute? 1 hour?
- **RTO** (Recovery Time Objective) — how long to restore? 15 minutes? 4 hours?

These dictate backup frequency, replication strategy, and whether you run multi-region.

### Multi-cluster patterns

Single cluster is fine until it isn't. Reasons to multi-cluster:
- **Blast radius** — one cluster failure shouldn't take down everything.
- **Compliance** — data residency, FedRAMP, etc.
- **Scale** — etcd performance ceiling (~5000 nodes, very lumpy).
- **Upgrade safety** — staged rollout across clusters.

Patterns:
- **One per region** + DNS-level routing.
- **Hub + spokes** (a control cluster hosts Argo CD/Flux managing many app clusters).
- **Fleet management** — Cluster API + Argo ApplicationSets generating per-cluster Applications.
- **Multi-cluster networking** (Cilium Cluster Mesh, Submariner, Istio multi-primary) — pods in different clusters address each other directly. Complex; reserve for actual need.

### Day-2 operations checklist

Things mature teams actually do:

- **Cost visibility** — Kubecost or OpenCost broken down by namespace, label, team.
- **Capacity planning** — track p95 CPU/mem usage, headroom for failover.
- **Chaos engineering** — kill pods/nodes intentionally (chaos-mesh, litmus). Build trust in your HA story.
- **Runbooks** — every alert points to a wiki page with the diagnostic and remediation steps.
- **Postmortems** — blameless, written, shared.
- **Quarterly DR drills** — restore a cluster from backup. Find out the broken parts before you need them.
- **Right-sizing reviews** — Goldilocks / VPA recommendations vs actual requests. Many clusters are 60% over-provisioned.
- **Image hygiene** — base image refreshes; CVE scans gating CI.
- **Dependency upgrades** — every controller you installed is shipping security patches. Track them.

---

## 4. Practical application — production-readiness checklist

A checklist a senior engineer applies to any new workload before it hits prod:

### Workload itself

- [ ] **Deployment with `maxUnavailable: 0`** (or StatefulSet with appropriate updateStrategy).
- [ ] **Replicas ≥ 2** (3+ across zones for tier-1 services).
- [ ] **`requests` and `limits` set;** at least Burstable, preferably Guaranteed for memory.
- [ ] **readinessProbe, livenessProbe, startupProbe** (the last for slow starters).
- [ ] **`terminationGracePeriodSeconds`** matched to drain time.
- [ ] **`preStop` sleep** to let LB deprogram.
- [ ] **PodDisruptionBudget** matching replica count.
- [ ] **TopologySpreadConstraints** across zones and nodes.

### Networking

- [ ] **Service + Ingress (or Gateway)** with TLS via cert-manager.
- [ ] **NetworkPolicy** — default-deny + explicit allows (incl. DNS).
- [ ] **externalTrafficPolicy: Local** if source IP matters.

### Security

- [ ] **PSS `restricted` namespace label** (or equivalent Kyverno enforcement).
- [ ] **Non-root, readOnlyRootFilesystem, drop ALL caps, seccomp RuntimeDefault**.
- [ ] **Image digest-pinned;** from approved registry; signed.
- [ ] **Secrets from external store** (ESO/Vault/IRSA); not in Git.
- [ ] **ServiceAccount-bound RBAC,** least privilege; no `cluster-admin`.

### Observability

- [ ] **`/metrics` endpoint** + ServiceMonitor.
- [ ] **Logs to stdout,** shipped to durable store.
- [ ] **PrometheusRule alerts** for RED metrics; each with a runbook URL.
- [ ] **Dashboards** in Grafana.
- [ ] **Distributed traces** if you have a tracing backend.

### Autoscaling

- [ ] **HPA configured;** sensible min/max; PDB matches min.
- [ ] **Cluster Autoscaler / Karpenter** can satisfy max.
- [ ] **Tested under load.**

### Delivery

- [ ] **Managed by GitOps** (Argo CD Application / Flux Kustomization).
- [ ] **Helm / Kustomize values** per environment.
- [ ] **Rollout strategy** (rolling, or Argo Rollouts canary).
- [ ] **`kubernetes.io/change-cause` or git SHA annotation** for traceability.

### Operational

- [ ] **Runbook URL** in Slack channel topic and alert annotations.
- [ ] **On-call rotation** owns it.
- [ ] **Postmortem template** ready.
- [ ] **DR plan:** what does "restore" mean for this workload?

This is roughly what an SRE asks before signing off on a service launch.

---

## 5. Common Mistakes & Gotchas

- **Treating managed clusters as fully managed.** EKS/AKS still need you to upgrade nodes, manage add-ons, configure networking, monitor IRSA/Workload Identity. GKE Autopilot is closer to truly hands-off.
- **No GitOps, deploys from individual laptops.** Inevitable drift. The cluster doesn't match Git, and Git doesn't match reality. Adopt GitOps early.
- **GitOps with all secrets in Git as plaintext.** Sealed-secrets, SOPS, or external secret manager — pick one before your first prod deploy.
- **Upgrading without testing add-ons.** A new K8s minor breaks ingress-nginx, or your CSI driver, or your custom operator. Stage in non-prod.
- **`kubectl edit` in prod** — drift the GitOps controller will fight with you about. If you must, then commit it back to Git.
- **Self-managed etcd with no backup schedule.** One bad node, one corrupted snapshot, one human error → cluster gone. Snapshot every 15 minutes minimum to off-cluster storage.
- **One Argo CD instance running everything.** A bad sync brings down all environments. Use multiple instances, or scope by project, or run a per-environment instance.
- **GitOps + drift correction on namespaces with manual experimentation.** Some namespaces want to be hands-on. Don't have Argo `selfHeal` everything.
- **No DR drills.** Backups are theatrical until you've restored from them. Quarterly minimum.
- **Multi-cluster networking before single-cluster is solid.** The complexity spike is real. Earn the second cluster.
- **Karpenter consolidating during a deploy.** Set `karpenter.sh/do-not-disrupt: "true"` on pods that shouldn't be moved mid-rollout, or use PDBs aggressively.
- **Skipping minor versions.** K8s officially supports one-minor-at-a-time. Going 1.27 → 1.30 in one shot violates the version skew rules during the upgrade.
- **Letting EKS auth defaults stand.** EKS access used to be cluster-creator-only via `aws-auth` ConfigMap (easy to break). New "EKS access entries" API is much safer; adopt it.
- **The "stack" no one updates.** Ingress controller, cert-manager, Argo CD, Prometheus — all need their own update cadence. Track them like dependencies.

---

## 🎯 Key Takeaways

- **Managed clusters are the right default in 2026.** Use them. Pour your engineering energy into your apps and platform glue, not into running etcd.
- **GitOps + a hardened image pipeline + admission policies is the *minimum viable platform*.** Below that, you're shipping by hand and hoping; above it, you're operating an actual platform.
- **DR is a verb, not a configuration.** Untested backups don't exist. Quarterly drills are cheap insurance.
- **The senior engineer's value isn't writing the perfect Deployment; it's the operating manual around it.** Alert routing, runbooks, postmortems, on-call rotations, upgrade cadence — these compound.
- **A platform is judged by what happens at 3am.** Everything you've learned in this course matters most then. Build for that scenario; design for the median day.

---

## You're done.

If you worked through all 18 modules, ran the manifests, and broke things on purpose, you're now operating at the level most teams call **"senior K8s engineer."** You can:

- Read any manifest and predict what controllers will do with it.
- Debug pod, service, networking, RBAC, storage, and scheduling issues from first principles.
- Decide between Deployment / StatefulSet / DaemonSet / Job without thinking.
- Design a hardened, observable, autoscaling service from a blank YAML file.
- Hold an informed opinion on Helm vs Kustomize, EKS vs GKE, ingress-nginx vs Gateway API.
- Build, deploy, and operate a custom controller if the situation calls for it.

The ecosystem keeps moving. Subscribe to **This Week in Kubernetes**, follow the SIG release notes, and circle back to this course when a new release lands. Good luck.

*← [prev](./17_security.md) | [home → 00_roadmap](./00_roadmap.md)*
