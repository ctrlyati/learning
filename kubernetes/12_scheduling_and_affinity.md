# 12 — Scheduling, Resources, and Affinity

> **Goal:** Influence where pods land. Master requests/limits/QoS, node selectors, affinity/anti-affinity, taints/tolerations, topology spread constraints — and know what the kube-scheduler is doing under the hood.

---

## 1. Resources & QoS — analogy + YAML

**Analogy.** When a pod schedules, it's like booking a room at a hotel. **Requests** are "I need at least this much space" — the front desk uses them to find a free room. **Limits** are "I'll never demand more than this" — go over and the hotel kicks you out (memory) or makes you wait (CPU).

```yaml
spec:
  containers:
    - name: api
      image: api:1.0
      resources:
        requests:
          cpu: "200m"        # 0.2 CPU cores
          memory: "256Mi"
        limits:
          cpu: "1"           # 1 CPU core
          memory: "512Mi"
```

### How the kernel enforces them

| Resource | Request | Limit |
|----------|---------|-------|
| **CPU**  | Used by scheduler to find a node with capacity. Sets `cpu.shares` in cgroups — proportional CPU under contention. | Sets cgroup CPU quota. **Throttled**, not killed, when exceeded. |
| **Memory** | Scheduler capacity. Sets `memory.min` (kernel hint). | Hard limit. Exceed → **OOMKilled** by the kernel. |

CPU is **compressible** (you get less, app runs slower). Memory is **incompressible** (no more, you die).

### QoS classes — derived from your requests/limits

| QoS | Condition | Eviction priority |
|-----|-----------|-------------------|
| **Guaranteed** | Requests == Limits for *all* resources, on every container | Last to be evicted under node pressure |
| **Burstable** | Requests < Limits (or some unspecified) | Middle |
| **BestEffort** | No requests, no limits | First to be evicted |

Production workloads should be **Guaranteed** (predictable + protected) or carefully tuned **Burstable**. BestEffort is for batch/experimental.

```bash
$ kubectl get pod api -o jsonpath='{.status.qosClass}'
Burstable
```

---

## 2. Mechanism — how the kube-scheduler picks a node

The scheduler runs in a loop:

1. **Watch** the apiserver for unscheduled pods (`spec.nodeName == ""`).
2. For each, run **two phases**:
   - **Filtering** — eliminate nodes that don't meet hard requirements (insufficient resources, doesn't tolerate the node's taints, node selector mismatch, port conflict, volume zone affinity, etc.).
   - **Scoring** — for surviving nodes, run plugins that score 0–100. Default scorers include: balanced resource allocation, image locality, taint toleration, inter-pod affinity, topology spread.
3. **Bind** — pick the highest-scoring node, PATCH `spec.nodeName`.
4. Kubelet on that node takes over.

If filtering eliminates all nodes, pod stays `Pending` with a `FailedScheduling` event:

```bash
$ kubectl describe pod stuck
...
Events:
  Warning  FailedScheduling  20s  default-scheduler
    0/3 nodes are available: 3 Insufficient memory.
```

These events are gold for debugging — read them first.

### The scheduler is pluggable

Since 1.19, the scheduler is a framework of plugins. You can:
- Disable built-in plugins.
- Write custom plugins (Go).
- Run **multiple schedulers** in the cluster, picking which one schedules a pod via `spec.schedulerName`.

This is how specialized schedulers (Volcano for batch, Yunikorn for big-data, Karpenter's just-in-time provisioning) coexist with the default.

---

## 3. Variations — placement controls

### nodeSelector — the simplest

```yaml
spec:
  nodeSelector:
    disktype: ssd
    zone: us-east-1a
```

Pod schedules only on nodes with all those labels. Hard requirement, no flexibility.

### Node affinity — nodeSelector++

```yaml
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - { key: disktype, operator: In, values: [ssd, nvme] }
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 80
          preference:
            matchExpressions:
              - { key: zone, operator: In, values: [us-east-1a] }
```

- `required...` = hard filter.
- `preferred...` = soft scoring (still schedules if not satisfiable).
- `IgnoredDuringExecution` means *if the node's labels change after scheduling, the pod isn't evicted*. (There's no `RequiredDuringExecution` — yet.)

### Pod affinity / anti-affinity — co-locate or spread

```yaml
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchLabels: { app: api }
          topologyKey: kubernetes.io/hostname
```

"Don't schedule on any node that already has an `app=api` pod." Result: HA — replicas spread across nodes.

`topologyKey` is the level of the topology you care about:
- `kubernetes.io/hostname` — one per node
- `topology.kubernetes.io/zone` — one per AZ
- `topology.kubernetes.io/region` — one per region

Pod affinity (vs anti-affinity) is the inverse — "co-locate with pods having these labels." Useful for cache locality.

### Taints & tolerations — exclude pods from nodes

A **taint** on a node says "don't schedule things here." A **toleration** on a pod says "I can handle that taint."

```bash
# Taint a node — e.g., GPU node reserved for ML workloads
$ kubectl taint nodes node-gpu-1 hardware=gpu:NoSchedule
node/node-gpu-1 tainted
```

Taint effects:
- `NoSchedule` — new pods without matching toleration don't schedule.
- `PreferNoSchedule` — soft version.
- `NoExecute` — existing pods without matching toleration are **evicted**.

```yaml
# A pod that can run on the GPU node
spec:
  tolerations:
    - key: "hardware"
      operator: "Equal"
      value: "gpu"
      effect: "NoSchedule"
```

Control-plane taints (`node-role.kubernetes.io/control-plane:NoSchedule`) keep workloads off masters by default. DaemonSets typically tolerate everything.

### Topology spread constraints — the modern way to spread

```yaml
spec:
  topologySpreadConstraints:
    - maxSkew: 1
      topologyKey: topology.kubernetes.io/zone
      whenUnsatisfiable: DoNotSchedule
      labelSelector:
        matchLabels: { app: api }
```

"Distribute `app=api` pods across zones; no zone should have more than 1 more than another." More expressive than pod anti-affinity for "spread evenly" — and faster (anti-affinity has O(n²) cost in big clusters).

### Priority and preemption

```yaml
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata: { name: critical }
value: 1000000
preemptionPolicy: PreemptLowerPriority
```

Pods with `priorityClassName: critical` get scheduled first; if there's no room, the scheduler **evicts** lower-priority pods to make space. Tune carefully — preemption can ripple through a cluster.

---

## 4. Practical application — production placement for a stateless API

```yaml
apiVersion: apps/v1
kind: Deployment
metadata: { name: api }
spec:
  replicas: 6
  selector: { matchLabels: { app: api } }
  template:
    metadata: { labels: { app: api } }
    spec:
      priorityClassName: standard
      tolerations:
        - key: workload
          operator: Equal
          value: app
          effect: NoSchedule
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - { key: workload, operator: In, values: [app] }
                  - { key: kubernetes.io/arch, operator: In, values: [amd64] }
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: ScheduleAnyway
          labelSelector: { matchLabels: { app: api } }
        - maxSkew: 1
          topologyKey: kubernetes.io/hostname
          whenUnsatisfiable: ScheduleAnyway
          labelSelector: { matchLabels: { app: api } }
      containers:
        - name: api
          image: api:1.4.2
          resources:
            requests: { cpu: "500m", memory: "512Mi" }
            limits:   { cpu: "1",    memory: "512Mi" }     # Guaranteed QoS on memory
```

This pod:
- Lands only on `workload=app` nodes (a dedicated nodepool).
- Tolerates the taint on those nodes (which keeps other workloads out).
- Spreads across zones AND nodes.
- Has Guaranteed memory (no surprise OOM under pressure).

```bash
$ kubectl get pods -l app=api -o wide
NAME             READY   STATUS    NODE                   ZONE
api-7c8d-2k9zp   1/1     Running   ip-10-0-1-10           us-east-1a
api-7c8d-9rgmd   1/1     Running   ip-10-0-2-15           us-east-1b
api-7c8d-jxd4q   1/1     Running   ip-10-0-3-22           us-east-1c
api-7c8d-vx7t2   1/1     Running   ip-10-0-1-11           us-east-1a
api-7c8d-9zm8r   1/1     Running   ip-10-0-2-16           us-east-1b
api-7c8d-pmkqd   1/1     Running   ip-10-0-3-23           us-east-1c
```

Beautiful — two pods per zone, no pod on the same host.

### Diagnosing scheduler decisions

```bash
$ kubectl get events --field-selector reason=FailedScheduling -A
LAST SEEN   TYPE      REASON             OBJECT          MESSAGE
2m          Warning   FailedScheduling   pod/stuck       0/3 nodes are available:
                                                          3 didn't match topology spread constraints.

$ kubectl describe pod stuck | tail -15
Conditions:
  Type           Status
  PodScheduled   False
Events:
  ...
```

Most "pod stuck pending" debug sessions are one `describe` away.

---

## 5. Common Mistakes & Gotchas

- **No requests → BestEffort → first to die.** "It works on my laptop" pods get evicted under node pressure. Always set requests.
- **`limits.memory == requests.memory` to get Guaranteed.** Right idea, but it also means you can't burst. If your app sometimes legitimately needs more memory, you'll get OOMKilled at that moment. Profile first.
- **CPU limits causing latency.** CPU throttling can be brutal — even apps using a fraction of their request can be throttled if the kernel CFS quota math hits them at the wrong moment. **Some shops drop CPU limits entirely** (keep memory limits) for latency-sensitive workloads. Controversial; understand the tradeoff.
- **Pod anti-affinity with required & many replicas.** `requiredDuringScheduling` + 50 replicas + one-per-node + 30 nodes = 20 pods stuck pending. Use `preferred` or topology spread with `whenUnsatisfiable: ScheduleAnyway`.
- **No spread → all replicas in one zone.** AZ goes down, app goes down. Always spread by zone for HA.
- **Taints on every new node from cloud autoscaler.** Karpenter or cluster autoscaler with custom taints — your pods don't tolerate, never schedule there, autoscaler scales it down. Watch the loop.
- **`nodeSelector` for a label that doesn't exist on any node.** Pod stuck pending forever. Check `kubectl get nodes --show-labels`.
- **PriorityClass abuse.** Everything is "high priority" → preemption goes wild. Reserve high priority for system or genuine SLO-critical workloads.
- **Cluster Autoscaler/Karpenter doesn't see the pending pod.** Pod request way bigger than any node shape can satisfy. The autoscaler logs explain why.
- **`limits` smaller than the app's true working set under load.** OOMKill mid-traffic-spike. Profile under load; right-size; don't guess.
- **Two schedulers fighting.** Set `spec.schedulerName` explicitly for pods you want a custom scheduler to handle; otherwise both may try (and one will lose).

---

## 🎯 Key Takeaways

- **Requests are how you book; limits are how you don't get kicked out.** Senior engineers tune these by reading real metrics, not gut-feeling.
- **Guaranteed QoS for production workloads is non-negotiable.** A node under pressure will keep them; BestEffort dies first.
- **Topology spread constraints have largely replaced pod anti-affinity for "spread evenly."** Cheaper at scale, more expressive.
- **CPU limits cause more outages than they prevent.** This isn't universal advice, but it's a debate every mature platform team has. Memory limits, yes; CPU limits, *measure* before adopting.
- **`FailedScheduling` events answer "why won't it schedule?" 95% of the time.** Reach for them first; reach for the scheduler logs only if events lie.

*← [prev](./11_networking_cni_dns_netpol.md) | [next →](./13_autoscaling.md)*
