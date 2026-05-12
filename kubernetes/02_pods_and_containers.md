# 02 — Pods & Containers

> **Goal:** Understand the Pod as the unit of scheduling, master the multi-container patterns (init, sidecar, ambassador, adapter), and know the full Pod lifecycle from `Pending` to `Terminated`.

---

## 1. The Pod — analogy + working YAML

**Analogy.** A Pod is a **shared apartment**, not a single tenant. Multiple containers (roommates) live together: they share the same network address (Wi-Fi router), the same hostname (street address), and can share storage (the fridge). They start together, die together, and are scheduled together. The smallest deployable unit in Kubernetes is *not* a container — it's a Pod.

Most pods have **one container**. Multi-container pods exist for specific patterns (covered below) — they're powerful but not the default.

### Minimal pod, but with everything you'd actually want

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: web
  labels:
    app: web
    tier: frontend
spec:
  containers:
    - name: nginx
      image: nginx:1.27-alpine
      ports:
        - name: http
          containerPort: 80
      resources:
        requests: { cpu: "50m",  memory: "64Mi" }
        limits:   { cpu: "200m", memory: "128Mi" }
      readinessProbe:
        httpGet: { path: /, port: http }
        periodSeconds: 5
      livenessProbe:
        httpGet: { path: /, port: http }
        periodSeconds: 10
        failureThreshold: 3
      env:
        - name: ENVIRONMENT
          value: dev
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: false
        runAsNonRoot: true
        runAsUser: 101
        capabilities:
          drop: ["ALL"]
  terminationGracePeriodSeconds: 30
```

```bash
$ kubectl apply -f web-pod.yaml
pod/web created

$ kubectl get pod web -o wide
NAME   READY   STATUS    RESTARTS   AGE   IP           NODE
web    1/1     Running   0          12s   10.244.0.8   learn-control-plane

$ kubectl describe pod web | grep -A2 Conditions
Conditions:
  Type              Status
  PodReadyToStartContainers   True
  Initialized                 True
  Ready                       True
  ContainersReady             True
  PodScheduled                True
```

---

## 2. Pod lifecycle and the kubelet's role

A Pod moves through **phases**. The phase is in `.status.phase`:

| Phase     | Meaning |
|-----------|---------|
| `Pending` | Accepted by apiserver, not yet running. Could be: unscheduled, pulling image, init container running. |
| `Running` | Bound to a node, at least one container started. |
| `Succeeded` | All containers exited 0 (used by Jobs, not Deployments). |
| `Failed`  | All containers exited, at least one non-zero. |
| `Unknown` | Kubelet stopped reporting. Usually a node problem. |

The **phase is coarse**. Fine-grained truth lives in `.status.conditions` (`PodScheduled`, `Initialized`, `ContainersReady`, `Ready`) and `.status.containerStatuses[*].state` (`Waiting`, `Running`, `Terminated` — with reason codes like `CrashLoopBackOff`, `ImagePullBackOff`, `CreateContainerConfigError`).

### What the kubelet actually does
1. **Watch** the apiserver for pods bound to its node (`spec.nodeName == myself`).
2. **Pull** images (with backoff if they fail).
3. **Create network namespace** via the CNI plugin → pod gets an IP.
4. Run **init containers** sequentially. Each must exit 0.
5. Start **regular containers** (in `containers[]`) roughly in parallel.
6. Run **startup**, then **liveness** and **readiness** probes per their schedules.
7. Report status back to the apiserver.

### Probes — the trio
- **startupProbe** — runs first. While it's failing, liveness/readiness are paused. Lets slow-starting apps boot without being killed.
- **livenessProbe** — "am I still alive?" Failure → kubelet **restarts** the container.
- **readinessProbe** — "should I get traffic?" Failure → Pod is removed from Service endpoints but **not** restarted.

Liveness and readiness are not the same thing. Confusing them is one of the most common reasons production traffic stutters.

Probe types: `httpGet`, `tcpSocket`, `exec`, `grpc`.

### Restart policy
- `Always` (default, used by Deployments) — always restart on exit.
- `OnFailure` (Jobs) — restart only on non-zero exit.
- `Never` (one-shot tasks).

---

## 3. Multi-container patterns

A Pod's containers share a **network namespace** (same `localhost`, same ports — they cannot bind the same port) and can share **volumes**. They do *not* share a PID namespace by default (set `shareProcessNamespace: true` if you need it).

### Init containers
Run **before** regular containers, **sequentially**, and **must each exit 0** before the pod proceeds. Use them for setup that needs special tools you don't want in your main image.

```yaml
apiVersion: v1
kind: Pod
metadata: { name: web-with-init }
spec:
  initContainers:
    - name: wait-for-db
      image: busybox:1.36
      command: ["sh", "-c", "until nc -z db 5432; do echo waiting; sleep 2; done"]
  containers:
    - name: app
      image: myapp:1.0
      ports: [{containerPort: 8080}]
```

The pod stays in `Init:0/1` until `wait-for-db` exits 0. Then the app starts.

### Sidecar containers
A second container running **alongside** the main one for the pod's lifetime. Classic uses: log shipper, metrics exporter, service-mesh proxy, secret refresher.

```yaml
apiVersion: v1
kind: Pod
metadata: { name: web-with-sidecar }
spec:
  containers:
    - name: app
      image: myapp:1.0
      volumeMounts: [{ name: logs, mountPath: /var/log/app }]
    - name: log-shipper
      image: fluent/fluent-bit:3.0
      volumeMounts: [{ name: logs, mountPath: /logs, readOnly: true }]
  volumes:
    - name: logs
      emptyDir: {}
```

Both containers see the same `logs` directory; the app writes, the shipper reads and forwards.

> Since Kubernetes **1.29 (GA in 1.33)**, "native sidecar" is a real concept — declared as an init container with `restartPolicy: Always`. They start before regular containers and terminate after them. Use this for true sidecars (like a service mesh proxy) so the main app can rely on them during startup *and* shutdown.

```yaml
spec:
  initContainers:
    - name: proxy           # acts as a sidecar
      image: envoyproxy/envoy:v1.30
      restartPolicy: Always
  containers:
    - name: app
      image: myapp:1.0
```

### Ambassador containers
A sidecar that **proxies outbound** traffic on behalf of the main container. The app talks to `localhost:6379`; the ambassador handles auth, sharding, TLS, retries to a remote Redis cluster. The app stays simple; the network complexity lives in the sidecar.

### Adapter containers
A sidecar that **normalizes output** from the main container. Common case: legacy app exposes stats in a custom format on port 9000; the adapter scrapes and re-exposes them in Prometheus format on port 9100.

---

## 4. Practical application — a realistic, multi-container Pod

A small web app that logs to a file, has a sidecar that ships logs, an init container that runs migrations, and proper probes:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: realapp
  labels: { app: realapp }
spec:
  initContainers:
    - name: migrate
      image: myapp:1.4
      command: ["./migrate", "up"]
      env:
        - name: DATABASE_URL
          valueFrom: { secretKeyRef: { name: app-secrets, key: db-url } }
  containers:
    - name: app
      image: myapp:1.4
      ports: [{ name: http, containerPort: 8080 }]
      env:
        - name: DATABASE_URL
          valueFrom: { secretKeyRef: { name: app-secrets, key: db-url } }
        - name: LOG_PATH
          value: /var/log/app/app.log
      resources:
        requests: { cpu: "100m", memory: "128Mi" }
        limits:   { cpu: "500m", memory: "256Mi" }
      readinessProbe:
        httpGet: { path: /healthz/ready, port: http }
        periodSeconds: 5
      livenessProbe:
        httpGet: { path: /healthz/live, port: http }
        initialDelaySeconds: 10
        periodSeconds: 15
      lifecycle:
        preStop:
          exec:
            command: ["sh", "-c", "sleep 5"]   # let LB stop sending traffic
      volumeMounts:
        - { name: logs, mountPath: /var/log/app }
    - name: log-shipper
      image: fluent/fluent-bit:3.0
      volumeMounts:
        - { name: logs, mountPath: /logs, readOnly: true }
        - { name: shipper-config, mountPath: /fluent-bit/etc, readOnly: true }
  volumes:
    - name: logs
      emptyDir: {}
    - name: shipper-config
      configMap: { name: fluentbit-config }
  terminationGracePeriodSeconds: 45
```

Verify:

```bash
$ kubectl apply -f realapp.yaml
pod/realapp created

$ kubectl get pod realapp -w
NAME      READY   STATUS     RESTARTS   AGE
realapp   0/2     Init:0/1   0          1s
realapp   0/2     Init:0/1   0          3s
realapp   0/2     PodInitializing   0   8s
realapp   2/2     Running           0   12s

$ kubectl logs realapp -c app --tail 5
{"level":"info","msg":"listening on :8080"}

$ kubectl logs realapp -c log-shipper --tail 3
[2026/05/13 10:15:01] [ info] [output:stdout] worker #0 started

$ kubectl exec realapp -c app -- curl -s localhost:8080/healthz/ready
{"status":"ready"}
```

### Graceful shutdown — what really happens on `kubectl delete pod`

1. apiserver writes `deletionTimestamp` on the Pod.
2. Pod is removed from Service endpoints (because endpoint controller sees it's terminating).
3. kubelet sees the deletionTimestamp and runs **`preStop`** hook (if defined).
4. kubelet sends **`SIGTERM`** to PID 1 of each container.
5. kubelet waits up to `terminationGracePeriodSeconds` (default 30).
6. If containers still running, kubelet sends **`SIGKILL`**.

If your app doesn't handle SIGTERM, you'll lose in-flight requests. The `preStop: sleep 5` trick is industry standard — it gives the load balancer time to deprogram the endpoint before the app starts shutting down.

---

## 5. Common Mistakes & Gotchas

- **Treating Pods as long-lived.** You almost never `kubectl apply` a bare Pod in production. You apply a Deployment/StatefulSet/Job, which *manages* Pods. Bare pods don't get rescheduled if the node dies.
- **No `resources.requests`.** The scheduler treats the pod as wanting nothing, packs nodes too tightly, and the kernel OOMKills you when there's contention. Always set requests at minimum.
- **`livenessProbe` set to the same thing as `readinessProbe`.** A flaky downstream dependency now restarts your container in a loop instead of just temporarily draining traffic. Liveness should answer "is my process wedged?" — usually just "is the HTTP server responding at all?"
- **Probe before app is ready.** Setting `initialDelaySeconds: 0` with a slow-booting app → liveness fails → restart → boot → liveness fails → CrashLoopBackOff that's actually just a slow start. Use `startupProbe`.
- **`latest` image tag.** No version pinning → different replicas of the same Deployment can run different code if the registry updated mid-rollout. Pin to digests or immutable tags.
- **Two containers binding the same port.** Same network namespace = same port table. Pick different ports.
- **Forgetting `terminationGracePeriodSeconds`.** Default 30s; many DB clients/queue workers need longer for clean drain.
- **`imagePullPolicy: Always` on every image.** Fine for `:latest`; wastes time and registry quota for pinned tags. Default is `IfNotPresent` for pinned, `Always` for `:latest`.

---

## 🎯 Key Takeaways

- **The Pod, not the container, is the scheduling unit.** Multi-container pods exist for tight coupling (shared lifecycle, shared volume, shared network). Anything looser belongs in a separate Pod with a Service between them.
- **Liveness and readiness are different verbs.** Senior engineers always notice when you've conflated them — it's a tell for "this team will page itself awake."
- **Init containers are the cleanest way to express ordering** that the rest of your manifest can't. Schema migrations, certificate fetching, dependency-waiting — all init containers, not retry loops inside your app.
- **Native sidecars (1.33+) finally fix the shutdown-ordering bug** that plagued service meshes for years. If your cluster's on a modern version, prefer them for mesh/proxy sidecars.
- **A pod's lifecycle is a state machine you can trace with `describe` + `events`.** Reach for those two before logs when something's stuck — most production incidents are scheduling, image pull, or probe failures, not application bugs.

*← [prev](./01_intro_and_setup.md) | [next →](./03_deployments_and_rollouts.md)*
