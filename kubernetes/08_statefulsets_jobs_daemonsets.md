# 08 — StatefulSets, DaemonSets, Jobs & CronJobs

> **Goal:** Know when Deployment isn't the right answer. Master StatefulSets (databases, queues), DaemonSets (node-level agents), Jobs (one-shot work), and CronJobs (scheduled work) — and what each controller actually does differently.

---

## 1. Workload variety — analogy + the cheat-sheet

**Analogy.** Deployments are the **office floor of interchangeable workers** — any one of them can do any task, scale freely, and replacement is transparent. But not every workload is like that. Some workers need a **named desk** (StatefulSet — Postgres replica `db-0` always boots before `db-1`). Some need to be **on every floor** (DaemonSet — the log collector). Some show up **once to do a job and leave** (Job — a database migration). Some show up on a **schedule** (CronJob — nightly backup).

| Controller | Use when |
|------------|----------|
| **Deployment** | Stateless workloads, interchangeable replicas |
| **StatefulSet** | Need stable identity, ordered startup, per-pod storage |
| **DaemonSet** | Exactly one pod per node (or per subset of nodes) |
| **Job** | One-time task, must run to completion |
| **CronJob** | Job, on a schedule |

---

## 2. StatefulSet — mechanism + ordered guarantees

A StatefulSet differs from a Deployment in five concrete ways:

1. **Stable pod names.** `db-0`, `db-1`, `db-2` — predictable, persistent across restarts.
2. **Stable per-pod DNS.** `db-0.db.default.svc.cluster.local` always resolves to the same pod's current IP.
3. **Ordered startup.** `db-1` doesn't start until `db-0` is Ready.
4. **Ordered, controlled rolling updates.** Same order, reversed (highest first, by default).
5. **Per-pod persistent storage.** Each pod gets its own PVC via `volumeClaimTemplates`.

### YAML

```yaml
apiVersion: v1
kind: Service
metadata: { name: db }
spec:
  clusterIP: None                # headless, required for stable DNS
  selector: { app: db }
  ports: [{ port: 5432, name: pg }]
---
apiVersion: apps/v1
kind: StatefulSet
metadata: { name: db }
spec:
  serviceName: db                # MUST match the headless Service
  replicas: 3
  selector: { matchLabels: { app: db } }
  template:
    metadata: { labels: { app: db } }
    spec:
      terminationGracePeriodSeconds: 60
      containers:
        - name: pg
          image: postgres:16-alpine
          ports: [{ containerPort: 5432, name: pg }]
          env:
            - { name: POSTGRES_PASSWORD, value: dev }
            - { name: PGDATA, value: /var/lib/postgresql/data/pgdata }
          volumeMounts:
            - { name: data, mountPath: /var/lib/postgresql/data }
  volumeClaimTemplates:
    - metadata: { name: data }
      spec:
        accessModes: [ReadWriteOnce]
        storageClassName: ssd-retain
        resources: { requests: { storage: 20Gi } }
```

```bash
$ kubectl apply -f db-sts.yaml
service/db created
statefulset.apps/db created

$ kubectl get sts,pod,pvc -l app=db
NAME                  READY   AGE
statefulset.apps/db   3/3     1m

NAME      READY   STATUS    RESTARTS   AGE
pod/db-0  1/1     Running   0          90s
pod/db-1  1/1     Running   0          75s    # started after db-0 was Ready
pod/db-2  1/1     Running   0          60s

NAME                STATUS   VOLUME    CAPACITY   STORAGECLASS
data-db-0           Bound    pvc-...   20Gi       ssd-retain
data-db-1           Bound    pvc-...   20Gi       ssd-retain
data-db-2           Bound    pvc-...   20Gi       ssd-retain

$ kubectl run dig --rm -it --image=tutum/dnsutils -- dig +short db-0.db.default.svc.cluster.local
10.244.0.20
```

The PVCs are named `<volumeClaimTemplate>-<pod>` and **not** deleted automatically when you delete the StatefulSet. This is on purpose — your data outlives the controller.

### What the StatefulSet controller does

- Maintains the **identity** (name+DNS+storage) per ordinal.
- On scale-up: creates ordinal *N* only after ordinal *N-1* is Running+Ready.
- On scale-down (or update): tears down ordinal *N* before *N-1*.
- On update: defaults to `RollingUpdate` (reverse-ordinal). You can `OnDelete` for manual control.
- `podManagementPolicy: Parallel` (vs default `OrderedReady`) skips the ordering — useful when ordering isn't needed but you want stable identity.

### `partition` updates — staged rollouts

```yaml
spec:
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      partition: 2
```

Only pods with ordinal `>= 2` are updated. Set to 1, only `db-1` and `db-2` update; `db-0` stays. Lets you canary-test a database upgrade on one replica before flipping the whole set.

---

## 3. DaemonSet, Job, CronJob

### DaemonSet — one pod per node

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata: { name: node-exporter }
spec:
  selector: { matchLabels: { app: node-exporter } }
  template:
    metadata: { labels: { app: node-exporter } }
    spec:
      tolerations:
        - operator: Exists       # tolerate any taint — run on all nodes, control plane included
      hostNetwork: true
      containers:
        - name: exporter
          image: prom/node-exporter:v1.8.0
          ports: [{ containerPort: 9100, hostPort: 9100, name: metrics }]
          volumeMounts:
            - { name: proc,    mountPath: /host/proc, readOnly: true }
            - { name: sys,     mountPath: /host/sys,  readOnly: true }
      volumes:
        - name: proc; hostPath: { path: /proc }
        - name: sys;  hostPath: { path: /sys }
```

The DaemonSet controller watches Nodes; whenever a new Node joins (and matches `nodeSelector` if any), the controller creates a Pod targeting that node. Whenever a Node is removed, the Pod goes too.

Common uses: log collectors (fluentd/fluent-bit), node monitoring (node-exporter), CNI agents (Calico, Cilium), CSI node drivers, kube-proxy itself.

### Job — run to completion

```yaml
apiVersion: batch/v1
kind: Job
metadata: { name: migrate }
spec:
  backoffLimit: 4
  activeDeadlineSeconds: 600
  ttlSecondsAfterFinished: 86400     # auto-clean after 1 day
  template:
    spec:
      restartPolicy: OnFailure
      containers:
        - name: migrate
          image: myapp:1.4.2
          command: ["./migrate", "up"]
          env:
            - { name: DATABASE_URL, valueFrom: { secretKeyRef: { name: app-secrets, key: db-url } } }
```

```bash
$ kubectl apply -f migrate.yaml
job.batch/migrate created

$ kubectl get job migrate
NAME      COMPLETIONS   DURATION   AGE
migrate   0/1           5s         5s

$ kubectl get job migrate
NAME      COMPLETIONS   DURATION   AGE
migrate   1/1           23s        45s

$ kubectl logs job/migrate
Applied 12 migrations.
```

Key fields:
- **`completions`** — total successful runs needed (default 1).
- **`parallelism`** — how many pods can run in parallel.
- **`backoffLimit`** — retries before giving up.
- **`activeDeadlineSeconds`** — hard timeout.
- **`ttlSecondsAfterFinished`** — auto-GC after success/failure (very useful).
- **`completionMode: Indexed`** — each pod gets an index (`JOB_COMPLETION_INDEX` env), good for parallel batch processing.

### CronJob — scheduled Jobs

```yaml
apiVersion: batch/v1
kind: CronJob
metadata: { name: backup }
spec:
  schedule: "0 2 * * *"        # 02:00 every day, in the controller's timezone
  timeZone: "UTC"
  concurrencyPolicy: Forbid    # don't start a new run while previous is still going
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 1
  startingDeadlineSeconds: 600
  jobTemplate:
    spec:
      backoffLimit: 2
      ttlSecondsAfterFinished: 604800
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: backup
              image: my-backup:latest
              command: ["./backup.sh"]
```

`concurrencyPolicy`: `Allow` (default), `Forbid`, or `Replace`. `Forbid` for "this job must not overlap with itself" (backups, billing rollups).

### CronJob gotchas
- **No tz field on older clusters.** Pre-1.27, schedules were always in the controller's timezone. `timeZone` is now standard.
- **Missed runs** — if the cluster was down at 02:00, the controller checks `startingDeadlineSeconds`; if too late, it skips.
- **CronJob ≠ at-most-once.** A pod could complete its work and then the kubelet could crash before reporting success → next run still triggered.

---

## 4. Practical application — Redis with a StatefulSet, a log DaemonSet, a nightly backup CronJob

Combined manifest:

```yaml
# Redis StatefulSet — 3 replicas, each with its own 5Gi disk
apiVersion: v1
kind: Service
metadata: { name: redis }
spec:
  clusterIP: None
  selector: { app: redis }
  ports: [{ port: 6379, name: redis }]
---
apiVersion: apps/v1
kind: StatefulSet
metadata: { name: redis }
spec:
  serviceName: redis
  replicas: 3
  selector: { matchLabels: { app: redis } }
  template:
    metadata: { labels: { app: redis } }
    spec:
      containers:
        - name: redis
          image: redis:7-alpine
          args: ["--appendonly", "yes"]
          ports: [{ containerPort: 6379, name: redis }]
          volumeMounts: [{ name: data, mountPath: /data }]
          readinessProbe:
            exec: { command: ["redis-cli", "ping"] }
            periodSeconds: 5
  volumeClaimTemplates:
    - metadata: { name: data }
      spec:
        accessModes: [ReadWriteOnce]
        storageClassName: ssd-retain
        resources: { requests: { storage: 5Gi } }
---
# Log-shipper DaemonSet
apiVersion: apps/v1
kind: DaemonSet
metadata: { name: logs, namespace: logging }
spec:
  selector: { matchLabels: { app: logs } }
  template:
    metadata: { labels: { app: logs } }
    spec:
      tolerations: [{ operator: Exists }]
      containers:
        - name: fluent-bit
          image: fluent/fluent-bit:3.0
          volumeMounts:
            - { name: varlog, mountPath: /var/log, readOnly: true }
      volumes:
        - name: varlog; hostPath: { path: /var/log }
---
# Nightly backup CronJob
apiVersion: batch/v1
kind: CronJob
metadata: { name: redis-backup }
spec:
  schedule: "0 2 * * *"
  timeZone: "UTC"
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      ttlSecondsAfterFinished: 86400
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: dump
              image: redis:7-alpine
              command: ["sh", "-c"]
              args:
                - |
                  for i in 0 1 2; do
                    redis-cli -h redis-$i.redis save
                    redis-cli -h redis-$i.redis --rdb /backup/redis-$i-$(date +%F).rdb
                  done
              volumeMounts: [{ name: backup, mountPath: /backup }]
          volumes:
            - name: backup
              persistentVolumeClaim: { claimName: backups }
```

```bash
$ kubectl apply -f workloads.yaml
service/redis created
statefulset.apps/redis created
daemonset.apps/logs created
cronjob.batch/redis-backup created

$ kubectl get sts,ds,cj
NAME                     READY   AGE
statefulset.apps/redis   3/3     2m

NAME                  DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR
daemonset.apps/logs   1         1         1       1            1           <none>

NAME                       SCHEDULE      SUSPEND   ACTIVE   LAST SCHEDULE
cronjob.batch/redis-backup 0 2 * * *     False     0        <none>

# Force-trigger the CronJob now to test
$ kubectl create job --from=cronjob/redis-backup backup-manual
job.batch/backup-manual created

$ kubectl logs job/backup-manual
OK
OK
OK
```

---

## 5. Common Mistakes & Gotchas

- **Using a Deployment for a database.** Two replicas mounting one EBS → contention or stuck pods. Use StatefulSet with `volumeClaimTemplates` so each replica gets its own disk.
- **Forgetting the headless Service for a StatefulSet.** Without it, pods get DNS but not stable per-pod DNS — and clustering software relying on `db-0.db.svc.cluster.local` fails silently.
- **Scaling a StatefulSet down expecting PVCs to disappear.** They don't. By design — your data is sacred. You delete them manually (`kubectl delete pvc data-db-2`) when you're sure.
- **`OrderedReady` for fast-startup workloads.** If pod 0 has a bad readiness probe, pod 1 never starts. Sometimes you want `podManagementPolicy: Parallel`.
- **DaemonSet without tolerations.** Won't schedule on tainted nodes (control-plane, GPU nodes). Add `tolerations: [{operator: Exists}]` if you really want everywhere.
- **DaemonSet with no resource requests.** Even a "small" agent eats memory on every node — and if it's noisy, can starve workloads. Always set requests/limits.
- **CronJob `schedule` in the wrong timezone.** Pre-1.27 it was the controller's TZ (usually UTC), not yours. Always set `timeZone`.
- **Long-running cron jobs without a deadline.** A 6-hour stuck Job can pile up. Set `activeDeadlineSeconds`.
- **No `ttlSecondsAfterFinished` on Jobs.** Completed Job pods accumulate in `kubectl get pods`. Frustrating. Always set it.
- **Manual `kubectl delete pod db-0` thinking it'll skip the order.** The controller just recreates it (still db-0), as expected. To trigger an actual update, change the spec.
- **`backoffLimit` and exponential backoff confusion.** Even if you set `backoffLimit: 100`, the *interval* between retries grows (up to 6 minutes). A flaky external API + Job ≠ infinite tight loop.

---

## 🎯 Key Takeaways

- **The controller is the workload's contract.** Deployment promises "fungible replicas;" StatefulSet promises "stable identity;" DaemonSet promises "one per node;" Job promises "runs to completion." Pick on the contract, not the YAML shape.
- **StatefulSet + headless Service is the canonical pattern for clustered software.** Cassandra, Kafka, MongoDB, Elasticsearch, Zookeeper — all built on this duo.
- **PVCs from a StatefulSet outlive the StatefulSet itself.** This is a feature. Treat them like the database file they represent.
- **DaemonSets are the right way to run node-level agents.** "I'll run my agent on every node by hand-scheduling" never survives the first new node joining the cluster.
- **`ttlSecondsAfterFinished` should be standard on every Job and CronJob.** Most "my namespace has 12 GB of completed pods" stories trace back to forgetting this.

*← [prev](./07_volumes_and_storage.md) | [next →](./09_namespaces_labels_selectors.md)*
