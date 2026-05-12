# 03 — Deployments & Rollouts

> **Goal:** Stop creating Pods directly. Understand how Deployments → ReplicaSets → Pods chain together, master rolling updates and rollbacks, and know what each strategy parameter actually controls.

---

## 1. The Deployment — analogy + working YAML

**Analogy.** A Deployment is a **standing order**, not a single action. "Keep 5 replicas of this app running, and when I change the image, swap them out a few at a time without taking the service down." Behind the scenes a Deployment creates ReplicaSets, which create Pods. You manage Deployments; you almost never touch ReplicaSets or Pods directly.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  labels: { app: web }
spec:
  replicas: 3
  selector:
    matchLabels: { app: web }
  template:
    metadata:
      labels: { app: web }
    spec:
      containers:
        - name: nginx
          image: nginx:1.27.0-alpine
          ports: [{ containerPort: 80 }]
          resources:
            requests: { cpu: "50m",  memory: "64Mi" }
            limits:   { cpu: "200m", memory: "128Mi" }
          readinessProbe:
            httpGet: { path: /, port: 80 }
            periodSeconds: 5
```

```bash
$ kubectl apply -f web-deploy.yaml
deployment.apps/web created

$ kubectl get deploy,rs,pod -l app=web
NAME                  READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/web   3/3     3            3           18s

NAME                            DESIRED   CURRENT   READY   AGE
replicaset.apps/web-7c8d5f9b6   3         3         3       18s

NAME                        READY   STATUS    RESTARTS   AGE
pod/web-7c8d5f9b6-2k9zp     1/1     Running   0          18s
pod/web-7c8d5f9b6-9rgmd     1/1     Running   0          18s
pod/web-7c8d5f9b6-jxd4q     1/1     Running   0          18s
```

Note the **hash in the ReplicaSet name** (`7c8d5f9b6`). That's the hash of the Pod template. Change anything in `spec.template`, you get a new RS.

---

## 2. Mechanism — how the chain reconciles

There are **two controllers** at play:

1. **Deployment controller** — watches Deployments. On change, creates/scales ReplicaSets to roll the change out.
2. **ReplicaSet controller** — watches ReplicaSets. Creates/deletes Pods to match `spec.replicas`.

### What happens on a normal apply

1. You change `image: nginx:1.27.0-alpine` → `image: nginx:1.27.1-alpine`.
2. apiserver writes the new Deployment spec.
3. Deployment controller computes the hash of `spec.template` — it differs from the existing RS.
4. Deployment controller **creates a new RS** (`web-9d3a...`) with the new template, `replicas: 0` initially.
5. Following the **strategy**, the Deployment controller scales the new RS up and old RS down in steps.
6. Each RS independently reconciles its Pod count.
7. Once the new RS is at `replicas: 3` and the old is at `0`, the rollout is complete. The old RS stays around (default 10 retained) so you can roll back.

```bash
$ kubectl set image deploy/web nginx=nginx:1.27.1-alpine
deployment.apps/web image updated

$ kubectl rollout status deploy/web
Waiting for deployment "web" rollout to finish: 1 out of 3 new replicas have been updated...
Waiting for deployment "web" rollout to finish: 2 out of 3 new replicas have been updated...
deployment "web" successfully rolled out

$ kubectl get rs -l app=web
NAME             DESIRED   CURRENT   READY   AGE
web-7c8d5f9b6    0         0         0       5m   # old
web-9d3a1c2bf    3         3         3       45s  # new
```

The old RS is **retained at 0** — this is what makes rollback fast.

### Strategy: `RollingUpdate` (default)

```yaml
spec:
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 25%        # how many extra pods can exist above replicas during rollout
      maxUnavailable: 25%  # how many pods can be missing below replicas during rollout
```

With `replicas: 4`, defaults mean: at most 5 pods exist at once, at least 3 are available. The controller cycles through:
- scale new RS +1, wait for ready,
- scale old RS -1,
- repeat.

### Strategy: `Recreate`

```yaml
spec:
  strategy: { type: Recreate }
```

Kills all old pods, *then* creates new ones. Causes downtime. Use only when concurrent old+new is unsafe (incompatible DB migration, exclusive lock on shared resource).

### Rollback

```bash
$ kubectl rollout history deploy/web
REVISION  CHANGE-CAUSE
1         <none>
2         <none>

$ kubectl rollout undo deploy/web
deployment.apps/web rolled back

$ kubectl rollout undo deploy/web --to-revision=1
```

Rollback just scales the previous RS up and the current one down — no image pulls, no surprises. This is why retaining old RSes matters.

### `kubectl rollout` toolkit
```bash
kubectl rollout status deploy/web
kubectl rollout history deploy/web
kubectl rollout pause deploy/web      # stop the rollout midway
kubectl rollout resume deploy/web
kubectl rollout restart deploy/web    # roll *without* changing the spec — useful for picking up new ConfigMaps/Secrets
```

---

## 3. Variations & depth

### `minReadySeconds`
After a pod becomes Ready, wait N seconds before considering it part of the available count. Smooths over apps that report Ready too eagerly.

### `progressDeadlineSeconds`
If no progress for N seconds (default 600), the Deployment is marked `Progressing=False` with reason `ProgressDeadlineExceeded`. CI systems and ArgoCD watch this to know a rollout is stuck.

### `revisionHistoryLimit`
Default 10. How many old RSes to keep. Setting too low (`0`) breaks rollback; too high inflates etcd.

### `selector` immutability
You **cannot change `spec.selector`** of a Deployment after creation. The apiserver rejects it. If you need to, delete and recreate.

### The hash-based label
Look at a managed Pod:
```bash
$ kubectl get pod web-9d3a1c2bf-x... -o jsonpath='{.metadata.labels}'
{"app":"web","pod-template-hash":"9d3a1c2bf"}
```
That `pod-template-hash` is auto-added; it's how RSes tell their own pods apart from other RSes' pods that share `app=web`.

### Blue/green and canary — not built in
Kubernetes Deployments do *not* natively support blue/green or canary in the way Argo Rollouts or Flagger do. The rolling update strategy is what you get out of the box. For real canaries (5% of traffic for an hour, then 25%, etc.) you reach for:
- **Argo Rollouts** — drop-in replacement for Deployment with `analysis` steps.
- **Flagger** — operator that drives traffic shifts using a service mesh.
- **Manual two-deployment** pattern: `web-stable` and `web-canary` Deployments behind one Service with weighted Endpoints (clunky).

### ReplicaSets directly?
You almost never use them alone. The exception is some operator patterns. For 99% of workloads: Deployment.

---

## 4. Practical application — a production-grade Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  labels:
    app: api
    tier: backend
    owner: platform
  annotations:
    kubernetes.io/change-cause: "v1.4.2 — fix login timeout (TICKET-1234)"
spec:
  replicas: 4
  revisionHistoryLimit: 5
  minReadySeconds: 10
  progressDeadlineSeconds: 300
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0      # never less than 4 ready — zero-downtime
  selector:
    matchLabels: { app: api }
  template:
    metadata:
      labels: { app: api, tier: backend }
    spec:
      terminationGracePeriodSeconds: 45
      containers:
        - name: api
          image: registry.example.com/api@sha256:5e7c4...   # digest-pinned
          ports: [{ name: http, containerPort: 8080 }]
          env:
            - name: PORT
              value: "8080"
            - name: ENV
              value: prod
          envFrom:
            - configMapRef: { name: api-config }
            - secretRef:    { name: api-secrets }
          resources:
            requests: { cpu: "200m", memory: "256Mi" }
            limits:   { cpu: "1",    memory: "512Mi" }
          startupProbe:
            httpGet: { path: /healthz, port: http }
            failureThreshold: 30
            periodSeconds: 2
          readinessProbe:
            httpGet: { path: /healthz/ready, port: http }
            periodSeconds: 5
            failureThreshold: 2
          livenessProbe:
            httpGet: { path: /healthz/live, port: http }
            periodSeconds: 15
            failureThreshold: 3
          lifecycle:
            preStop:
              exec: { command: ["sh", "-c", "sleep 10"] }
          securityContext:
            runAsNonRoot: true
            runAsUser: 10001
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            capabilities: { drop: ["ALL"] }
```

The `kubernetes.io/change-cause` annotation shows up in `rollout history` if you add `--record` or set it explicitly — invaluable for post-incident review.

### Verifying a rollout end-to-end

```bash
$ kubectl apply -f api.yaml
deployment.apps/api configured

$ kubectl rollout status deploy/api --timeout=5m
Waiting for deployment "api" rollout to finish: 1 out of 4 new replicas have been updated...
Waiting for deployment "api" rollout to finish: 2 out of 4 new replicas have been updated...
deployment "api" successfully rolled out

$ kubectl get deploy api -o jsonpath='{.status}' | jq
{
  "availableReplicas": 4,
  "conditions": [
    { "type": "Available",   "status": "True", "reason": "MinimumReplicasAvailable" },
    { "type": "Progressing", "status": "True", "reason": "NewReplicaSetAvailable" }
  ],
  "observedGeneration": 7,
  "readyReplicas": 4,
  "replicas": 4,
  "updatedReplicas": 4
}
```

The two conditions to monitor in CI: `Available=True` and `Progressing=True` with reason `NewReplicaSetAvailable`.

### Forcing a restart to pick up new config

You changed a ConfigMap that the Deployment reads via `envFrom`. Pods won't pick it up automatically (env vars are read at container start). Trigger a rolling restart:

```bash
$ kubectl rollout restart deploy/api
deployment.apps/api restarted
```

This works by setting an annotation on the Pod template with the current timestamp, which changes the template hash, which creates a new RS.

---

## 5. Common Mistakes & Gotchas

- **`maxUnavailable > 0` for critical services.** Zero-downtime requires `maxUnavailable: 0`. The default 25% will yank a quarter of your pods during a rollout — usually fine, occasionally not.
- **No readiness probe.** Pod becomes Ready as soon as the container starts. New pods get traffic before the app finished initializing → 502s during every rollout.
- **Selector that's too narrow.** `matchLabels: {app: api, version: v1}` means changing version requires a new Deployment, not a rollout. Selectors should match the *identity* of the workload, not the version.
- **Changing the selector.** Forbidden after creation. People often try this and get a confusing apiserver error.
- **Using `kubectl edit deploy/foo` in production.** Tempting in incidents; loses the change in your Git source of truth. Always edit YAML and apply.
- **Rolling restart to "fix" a flaky pod.** Sometimes warranted, but if the same RS keeps producing flaky pods you have an image or config bug, not a rollout problem.
- **No `progressDeadlineSeconds` tuned.** Default 600s means CI waits 10 minutes for a stuck rollout. Lower it for fast-feedback environments.
- **Image pull secrets missing in the namespace.** Deployment scales up, pods stay in `ImagePullBackOff` for a private registry. The Deployment doesn't carry pull secrets — the ServiceAccount does.
- **`Recreate` strategy without thinking.** Yes, it's simpler. It also drops the service during every deploy. Use only when the app demands it.
- **HPA + manual `replicas` change fighting.** If an HPA is attached, your `replicas: 3` in YAML is overwritten on every reconcile. Don't set `replicas` in YAML for HPA-managed Deployments — or use `spec.replicas` as a min via HPA's `minReplicas`.

---

## 🎯 Key Takeaways

- **Deployment → ReplicaSet → Pod is the canonical chain.** Knowing which controller does what makes "why didn't my rollout move?" answerable in seconds.
- **Rolling updates are configurable, not magic.** `maxSurge` and `maxUnavailable` are the two dials that decide your downtime profile. Production services usually want `maxUnavailable: 0, maxSurge: 1+`.
- **Rollback is cheap because old RSes stay scaled to 0.** Don't trim `revisionHistoryLimit` to 1 to save etcd — you'll regret it during an incident.
- **`kubectl rollout restart` is the safe way to pick up new ConfigMaps/Secrets.** Avoids the temptation of deleting pods one by one.
- **For real progressive delivery (canary, blue/green, traffic-shifting analysis), reach for Argo Rollouts or Flagger.** Vanilla Deployments are great at "swap pods slowly"; they're not built for "5% of traffic for an hour, then judge."

*← [prev](./02_pods_and_containers.md) | [next →](./04_services_and_kube_proxy.md)*
