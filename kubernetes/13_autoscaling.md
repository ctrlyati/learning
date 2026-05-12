# 13 — Autoscaling

> **Goal:** Make workloads elastic. Understand the HPA control loop, when to use VPA, how the Cluster Autoscaler and Karpenter add/remove nodes, and when KEDA's event-driven scaling beats CPU-based.

---

## 1. The three layers — analogy + the cheat-sheet

**Analogy.** Autoscaling K8s has **three independent thermostats**:

1. **HPA (Horizontal Pod Autoscaler)** — "this room is too crowded, add more chairs." Scales pod *count*.
2. **VPA (Vertical Pod Autoscaler)** — "the chair is too small for this person." Scales pod *size* (resources).
3. **Cluster Autoscaler / Karpenter** — "we're out of chairs in this room, expand the room." Scales node count.

They don't know about each other and run in parallel. Most production clusters use HPA + Cluster Autoscaler (or Karpenter) constantly; VPA selectively; **KEDA** when you need to scale on custom metrics like queue depth.

| Tool | Scales | Trigger | Typical use |
|------|--------|---------|-------------|
| **HPA** | Replicas | CPU/mem/custom metric | Stateless HTTP services |
| **VPA** | Container requests/limits | Historical usage | Right-sizing; often "recommend-only" |
| **Cluster Autoscaler** | Nodes | Unschedulable pods | Cloud cluster, fixed nodegroups |
| **Karpenter** | Nodes (just-in-time) | Unschedulable pods | AWS (now multi-cloud), flexible instance types |
| **KEDA** | HPA replicas (incl. to 0) | External event source (queue length, Kafka lag, cron, etc.) | Event-driven workloads |

---

## 2. HPA — mechanism + working YAML

HPA is a controller in `kube-controller-manager`. Every `--horizontal-pod-autoscaler-sync-period` (default 15s):

1. Query metrics — the **metrics-server** (CPU/mem) or **custom metrics API** (Prometheus adapter, KEDA).
2. Compute desired replicas via the standard formula:

```
desiredReplicas = ceil(currentReplicas * currentMetricValue / desiredMetricValue)
```

3. Apply `behavior` policies (max scale-up rate, stabilization window).
4. PATCH the target's `spec.replicas`.

### Requirements

- **metrics-server** must be installed (it's not bundled in vanilla K8s, though most managed clusters install it):

```bash
$ kubectl top pods
NAME    CPU(cores)   MEMORY(bytes)
api-1   83m          120Mi
api-2   77m          118Mi
```

If that works, metrics-server is fine. If you get `Metrics API not available`, install it:

```bash
$ kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

- Your target Deployment/StatefulSet must have **`resources.requests`** set. HPA computes "current utilization" as a percentage of the request. No request → no HPA.

### A real HPA

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata: { name: api }
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api
  minReplicas: 3
  maxReplicas: 30
  metrics:
    - type: Resource
      resource:
        name: cpu
        target: { type: Utilization, averageUtilization: 60 }
    - type: Resource
      resource:
        name: memory
        target: { type: Utilization, averageUtilization: 75 }
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 30
      policies:
        - { type: Pods,    value: 4, periodSeconds: 60 }   # +4 pods / minute
        - { type: Percent, value: 50, periodSeconds: 60 }  # or +50%
      selectPolicy: Max
    scaleDown:
      stabilizationWindowSeconds: 300                       # wait 5 min after a peak
      policies:
        - { type: Pods,    value: 2, periodSeconds: 60 }
        - { type: Percent, value: 10, periodSeconds: 60 }
      selectPolicy: Min
```

```bash
$ kubectl apply -f hpa.yaml
horizontalpodautoscaler.autoscaling/api created

$ kubectl get hpa
NAME   REFERENCE         TARGETS              MINPODS   MAXPODS   REPLICAS
api    Deployment/api    45%/60%, 60%/75%     3         30        4

$ kubectl describe hpa api
...
Events:
  Normal   SuccessfulRescale   2m   horizontal-pod-autoscaler   New size: 5; reason: cpu resource utilization (percentage of request) above target
```

### Custom metrics

Want to scale on requests-per-second, queue depth, or external metrics? Install the **Prometheus Adapter** (or KEDA) and reference custom metrics:

```yaml
spec:
  metrics:
    - type: Pods
      pods:
        metric: { name: http_requests_per_second }
        target: { type: AverageValue, averageValue: 500 }
```

---

## 3. Variations — VPA, Cluster Autoscaler, Karpenter, KEDA

### VPA (Vertical Pod Autoscaler)

Adjusts container `requests` based on historical usage. Components:
- **Recommender** — analyzes metrics, suggests requests.
- **Updater** — evicts pods whose requests are off-target (so they restart with new values).
- **Admission controller** — sets requests on new pods.

```yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata: { name: api }
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api
  updatePolicy:
    updateMode: "Auto"   # or "Initial" (only at pod creation), "Off" (recommend only)
  resourcePolicy:
    containerPolicies:
      - containerName: '*'
        minAllowed: { cpu: "100m", memory: "128Mi" }
        maxAllowed: { cpu: "4",    memory: "8Gi" }
```

**Caveat:** VPA in `Auto` mode and HPA on the same workload **fight each other** when both are CPU-driven (HPA wants more pods, VPA wants bigger pods). Best practice: HPA on the metric, VPA in `Off` mode just for recommendations.

### Cluster Autoscaler

Watches for pods stuck in `Pending` due to insufficient resources. Scales up the cluster's nodegroups (ASGs in AWS, MIGs in GCP, etc.) to add nodes. Also scales down nodes that have been underutilized for a configurable period (default 10 min).

Requirements:
- Cloud integration (the autoscaler knows how to call the cloud provider's scale API).
- Nodegroup definition (per-AZ ASGs for cloud).
- Pods must be able to land on new nodes (no impossible affinity/taints).

```bash
$ kubectl -n kube-system logs deploy/cluster-autoscaler --tail 20
I0513 10:00:01 scale_up.go:340] Successfully scaled up nodegroup spot-1a from 3 to 5
I0513 10:02:14 scale_down.go:1023] Node ip-10-0-1-22 is unneeded since 10m, removing
```

### Karpenter — just-in-time nodes

A modern alternative (AWS-first, now multi-cloud). Instead of pre-defined nodegroups, Karpenter picks the right instance type *for each batch of pending pods*. Faster (no warm pool), cheaper (right-sized), supports spot directly.

```yaml
apiVersion: karpenter.sh/v1
kind: NodePool
metadata: { name: default }
spec:
  template:
    spec:
      requirements:
        - key: karpenter.sh/capacity-type
          operator: In
          values: [spot, on-demand]
        - key: kubernetes.io/arch
          operator: In
          values: [amd64, arm64]
        - key: karpenter.k8s.aws/instance-family
          operator: In
          values: [c6i, c7i, m6i, m7i]
      nodeClassRef: { name: default }
  disruption:
    consolidationPolicy: WhenUnderutilized
```

Karpenter is increasingly the default for new AWS clusters; cluster-autoscaler is still fine for "I want one fixed nodegroup shape."

### KEDA — event-driven autoscaling

KEDA installs as an operator. It exposes HPA-compatible custom metrics for **60+ event sources**: Kafka lag, RabbitMQ queue depth, SQS messages in flight, Azure Service Bus, Redis streams, Prometheus queries, cron, even pub/sub topics.

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata: { name: consumer-scaler }
spec:
  scaleTargetRef: { name: consumer, kind: Deployment }
  minReplicaCount: 0           # ← scale to ZERO when idle. HPA can't do this.
  maxReplicaCount: 100
  pollingInterval: 15
  cooldownPeriod: 300
  triggers:
    - type: kafka
      metadata:
        bootstrapServers: kafka:9092
        consumerGroup: payments
        topic: payments-events
        lagThreshold: "1000"
```

**Scale-to-zero** is the killer feature. A consumer with no messages: 0 replicas. First message arrives: KEDA activates the Deployment to 1, hands off to HPA-style scaling. For event-driven and bursty workloads this saves an enormous amount of compute.

---

## 4. Practical application — HPA + Karpenter for a web service

```yaml
apiVersion: apps/v1
kind: Deployment
metadata: { name: web }
spec:
  replicas: 3
  selector: { matchLabels: { app: web } }
  template:
    metadata: { labels: { app: web } }
    spec:
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: ScheduleAnyway
          labelSelector: { matchLabels: { app: web } }
      containers:
        - name: web
          image: web:1.0
          resources:
            requests: { cpu: "200m", memory: "256Mi" }
            limits:   { memory: "512Mi" }
          readinessProbe:
            httpGet: { path: /healthz, port: 8080 }
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata: { name: web }
spec:
  scaleTargetRef: { apiVersion: apps/v1, kind: Deployment, name: web }
  minReplicas: 3
  maxReplicas: 50
  metrics:
    - type: Resource
      resource:
        name: cpu
        target: { type: Utilization, averageUtilization: 65 }
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 30
      policies: [{ type: Percent, value: 100, periodSeconds: 60 }]
    scaleDown:
      stabilizationWindowSeconds: 600
      policies: [{ type: Percent, value: 25,  periodSeconds: 60 }]
```

Behind the scenes:
- A load spike pushes CPU to 80%.
- HPA scales up `web` from 3 → 6 replicas.
- Two of the new pods can't schedule (nodes full).
- Cluster Autoscaler/Karpenter sees pending pods, provisions a node within ~60 seconds.
- New pods schedule, traffic absorbed.
- Load drops, pods scale down after 10 min stabilization, autoscaler removes the underutilized node 10 min later.

```bash
$ kubectl get hpa web -w
NAME   REFERENCE        TARGETS    MINPODS   MAXPODS   REPLICAS
web    Deployment/web   42%/65%    3         50        3
web    Deployment/web   78%/65%    3         50        4
web    Deployment/web   82%/65%    3         50        6
web    Deployment/web   71%/65%    3         50        8
```

### Pod Disruption Budgets — pair with autoscaling

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata: { name: web }
spec:
  minAvailable: 3
  selector: { matchLabels: { app: web } }
```

A PDB tells the cluster autoscaler/Karpenter "don't evict more than this during voluntary disruptions." Without it, autoscaler can drain a node mid-deploy and take all replicas down at once.

---

## 5. Common Mistakes & Gotchas

- **HPA without resource requests.** Silently does nothing — utilization is `<unknown>`. Set requests.
- **HPA on memory utilization for non-leak-prone apps.** Memory often stays near limit even when load is light (caches, JVM heap). CPU is usually the more responsive signal.
- **`minReplicas: 1`.** Single pod → no redundancy; one bad request crashes the only replica. Set `minReplicas: 2` (or 3 across zones).
- **`maxReplicas` not actually thought through.** Too low → throttled under load. Too high → blew the budget when a bug caused a feedback loop. Bound it by capacity AND budget.
- **Scaling up too fast.** Cold caches + cold connections + huge spike = thundering herd. Use `behavior.scaleUp` policies to ramp.
- **Scaling down too fast.** A 5-second valley between bursts shouldn't unmount all the pods. Long `stabilizationWindowSeconds` for scaleDown.
- **HPA + VPA(Auto) on the same workload.** Fight. Use VPA in `Off` mode for recommendations only.
- **No PDB → autoscaler drains all replicas at once during a node consolidation.** Outage waiting to happen.
- **metrics-server unhealthy.** HPA goes "ScalingActive=False" with reason `FailedGetResourceMetric`. Always check `kubectl top` works.
- **Cluster Autoscaler can't scale up because of a taint / nodeSelector / affinity the pod has but no nodegroup matches.** Read the CA logs; they're verbose but they always tell you.
- **Karpenter consolidating during a deploy.** Combine with PDBs and `do-not-disrupt` annotations on batch jobs.
- **KEDA scale-from-zero latency.** First message after idle waits for: KEDA activates → HPA scales to 1 → pod schedules → image pull → container starts → app boots. Can be 30+ seconds. Pre-warm if SLA matters.

---

## 🎯 Key Takeaways

- **HPA + Cluster Autoscaler/Karpenter is the production duo.** Pods scale on demand; nodes follow. Master both control loops.
- **Autoscaling needs accurate requests.** Garbage in, garbage out. Use VPA recommendations to right-size, then let HPA take over.
- **CPU utilization is usually a better signal than memory.** Memory misleads. Custom metrics (RPS, queue depth) are better still when you have them.
- **Scale-to-zero is real with KEDA.** For event-driven workloads it's transformative — but plan for cold-start latency.
- **A PDB is the autoscaler's safety belt.** Without one, every node consolidation is a roll of the dice. Senior engineers add PDBs alongside HPAs as a reflex.

*← [prev](./12_scheduling_and_affinity.md) | [next →](./14_observability.md)*
