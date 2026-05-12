# 14 — Observability

> **Goal:** Know what's actually happening in your cluster. Master `kubectl logs` and the log-sidecar pattern, metrics-server vs Prometheus, what Grafana actually visualizes, how Kubernetes Events work, and the trio of pillars: logs, metrics, traces.

---

## 1. The three pillars — analogy + first commands

**Analogy.** Running blind production traffic without observability is like driving with the windshield painted black. **Logs** are what each driver said over the radio (what happened). **Metrics** are the dashboard gauges (rates, levels). **Traces** are the GPS trail (where one request actually went). All three exist as separate concerns; modern platforms instrument all three.

### Quick first commands

```bash
$ kubectl logs deploy/api
$ kubectl logs deploy/api -f --tail=100
$ kubectl logs deploy/api --previous          # the *last* container — useful after a crash
$ kubectl logs deploy/api -c sidecar          # specific container
$ kubectl top pods                            # metrics-server CPU/mem
$ kubectl top nodes
$ kubectl get events --sort-by=.lastTimestamp
$ kubectl get events --field-selector type=Warning -A
$ kubectl describe pod foo                    # shows the events for this pod
```

These four commands answer 80% of "what's wrong" questions on day one.

---

## 2. Mechanism — how logs and metrics actually flow

### Container logs

A container's `stdout` and `stderr` are written by the container runtime (containerd) to a file under `/var/log/pods/...` on the node. The kubelet exposes them through the apiserver, which is what `kubectl logs` reads.

This means:
- Logs persist on the *node* — if the node dies, the logs die.
- If a pod restarts, the previous container's logs are kept (one back) — that's what `--previous` reads.
- Rotation is handled by the kubelet/container runtime, configurable per node.

For durable, searchable logs you need a **log shipping** layer: a DaemonSet on every node that tails the log files and forwards them to a backend (Loki, Elastic, CloudWatch, Datadog, etc.). Common shippers: fluent-bit, fluentd, Vector, Promtail.

### Events — Kubernetes' own log

Every controller emits **Events** when it does something interesting: scheduling decisions, image pulls, probe failures, scaling actions, OOMKills, evictions.

```bash
$ kubectl get events -n payments --sort-by=.lastTimestamp | tail -10
LAST SEEN   TYPE      REASON              OBJECT          MESSAGE
3m          Normal    Scheduled           pod/api-x9z     Successfully assigned to node1
3m          Normal    Pulled              pod/api-x9z     Container image already present
3m          Normal    Created             pod/api-x9z     Created container api
3m          Normal    Started             pod/api-x9z     Started container api
30s         Warning   Unhealthy           pod/api-x9z     Readiness probe failed: HTTP 503
```

Events are **stored in etcd** with a TTL of 1 hour by default. Crucial gotcha: **they expire**. If you didn't read them in the first hour, they're gone.

Modern clusters ship Events to a long-term store (via `kube-state-metrics` for some, or `event-exporter`/`kubernetes-event-exporter`).

### metrics-server vs Prometheus

| | metrics-server | Prometheus |
|--|---------------|-----------|
| **Scope** | CPU + memory only, current values | Any metric, time-series |
| **Source** | Kubelet cAdvisor endpoint | Scrapes `/metrics` HTTP endpoints |
| **Retention** | None — only "right now" | Configurable (typically days/weeks) |
| **Powers** | `kubectl top`, HPA | Dashboards, alerts, historical analysis |

You need **both**. metrics-server for `kubectl top` and HPA's resource metrics. Prometheus for everything else.

### Prometheus + Grafana — the standard stack

The de-facto open-source observability stack for Kubernetes:

- **Prometheus** — scrape-based time-series DB. Pull metrics from `/metrics` endpoints (exposition format is plain text, very simple).
- **kube-state-metrics** — turns Kubernetes API state (Deployment status, pod phases, etc.) into Prometheus metrics.
- **node-exporter** — Linux host-level metrics (per node).
- **Grafana** — dashboards on top of Prometheus (and other data sources).
- **Alertmanager** — handles alert routing, grouping, silencing.

Most teams install **kube-prometheus-stack** (Helm chart) — it bundles all of the above, configured to discover Kubernetes Services automatically via `ServiceMonitor` and `PodMonitor` CRDs.

```yaml
# Tell Prometheus to scrape your app
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata: { name: api }
spec:
  selector:
    matchLabels: { app: api }
  endpoints:
    - port: metrics
      interval: 15s
      path: /metrics
```

Apps expose `/metrics` in Prometheus format (libraries exist for every language). Prometheus discovers the Service, scrapes every 15s.

---

## 3. Variations — log shipping, distributed tracing, the OTel future

### Log shipping options

| Stack | Notes |
|-------|-------|
| **fluent-bit + Loki** | Loki is "Prometheus for logs" — cheap, label-driven, integrates beautifully with Grafana. Modern default for open-source. |
| **fluentd + Elasticsearch + Kibana (EFK)** | Older, heavier, full-text search. Powerful but expensive. |
| **fluent-bit + CloudWatch / Stackdriver / Azure Monitor** | Managed cloud logs. Pay-per-GB; no operational burden. |
| **Vector + ClickHouse** | High-performance newer option. Worth watching. |

### Sidecar logging pattern

When your app *cannot* log to stdout (legacy, multi-file, structured outputs), run a sidecar:

```yaml
spec:
  containers:
    - name: app
      image: legacy-app:1.0
      volumeMounts: [{ name: logs, mountPath: /var/log/app }]
    - name: tailer
      image: busybox
      command: ["sh", "-c", "tail -F /logs/app.log /logs/error.log"]
      volumeMounts: [{ name: logs, mountPath: /logs, readOnly: true }]
  volumes:
    - name: logs
      emptyDir: {}
```

The sidecar's stdout becomes the shipper-collectable log. Slightly more pods to run, but legacy apps live happily.

### Distributed tracing

Spans cross service boundaries. The standard now is **OpenTelemetry** (OTel) — vendor-neutral SDKs that emit spans (and increasingly logs + metrics too). Backends: Jaeger, Tempo, Datadog APM, Honeycomb, AWS X-Ray.

```yaml
# OTel Collector as a DaemonSet — receives spans from apps, exports to backend
apiVersion: opentelemetry.io/v1beta1
kind: OpenTelemetryCollector
metadata: { name: agent }
spec:
  mode: daemonset
  config:
    receivers:
      otlp: { protocols: { grpc: {}, http: {} } }
    processors:
      batch: {}
    exporters:
      otlp: { endpoint: tempo:4317, tls: { insecure: true } }
    service:
      pipelines:
        traces:
          receivers: [otlp]
          processors: [batch]
          exporters: [otlp]
```

The OTel Operator can also auto-inject SDKs (`InstrumentationCRD`) for many languages.

### eBPF observability

Tools like **Pixie**, **Cilium Hubble**, **Parca** use eBPF to capture network, profile, and trace data with no app instrumentation. Powerful but newer; complements (not replaces) the OTel approach.

---

## 4. Practical application — wiring up a service with metrics + logs + alerts

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  labels: { app: api }
spec:
  replicas: 3
  selector: { matchLabels: { app: api } }
  template:
    metadata:
      labels: { app: api }
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9090"
    spec:
      containers:
        - name: api
          image: api:1.4.2
          ports:
            - { containerPort: 8080, name: http }
            - { containerPort: 9090, name: metrics }
          # ... resources, probes
---
apiVersion: v1
kind: Service
metadata: { name: api, labels: { app: api } }
spec:
  selector: { app: api }
  ports:
    - { name: http,    port: 80,   targetPort: http }
    - { name: metrics, port: 9090, targetPort: metrics }
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata: { name: api, labels: { release: kube-prometheus-stack } }
spec:
  selector: { matchLabels: { app: api } }
  endpoints:
    - port: metrics
      interval: 15s
---
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata: { name: api-alerts }
spec:
  groups:
    - name: api.rules
      rules:
        - alert: APIHighErrorRate
          expr: |
            sum(rate(http_requests_total{app="api",status=~"5.."}[5m])) /
            sum(rate(http_requests_total{app="api"}[5m])) > 0.05
          for: 10m
          labels: { severity: page }
          annotations:
            summary: "API 5xx rate > 5% for 10 min"
            runbook_url: "https://wiki.example.com/runbooks/api-5xx"
```

```bash
$ kubectl apply -f api-observability.yaml

$ kubectl exec deploy/api -- curl -s localhost:9090/metrics | head
# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET",path="/",status="200"} 4421
http_requests_total{method="POST",path="/api/v1/orders",status="200"} 1284
http_requests_total{method="POST",path="/api/v1/orders",status="500"} 7

# Check Prometheus has discovered the target
$ kubectl -n monitoring port-forward svc/prometheus 9090 &
$ curl -s 'http://localhost:9090/api/v1/targets' | jq '.data.activeTargets[] | select(.labels.app=="api")'
{
  "scrapeUrl": "http://10.244.0.5:9090/metrics",
  "health": "up",
  "lastScrape": "2026-05-13T10:14:00Z"
}
```

### USE & RED — the metrics taxonomies you should know

- **USE** (Brendan Gregg) for *resources*: **U**tilization, **S**aturation, **E**rrors. Apply to nodes, disks, queues.
- **RED** for *services*: **R**ate, **E**rrors, **D**uration. Apply to every HTTP endpoint.

A senior SRE's dashboard is mostly USE + RED panels. Most other charts are diagnostic, not first-line.

---

## 5. Common Mistakes & Gotchas

- **`kubectl logs` after a CrashLoopBackOff shows nothing.** The pod's most recent container is still booting and has no output yet. Use `--previous` for the last container that crashed.
- **Logging to a file inside the container.** No collector sees it (unless you sidecar). Always log to stdout/stderr.
- **Multi-line stack traces split across lines in the log shipper.** Most shippers need a multi-line parser config to keep Python/Java tracebacks as one event.
- **Forgetting that `kubectl describe` is the events-for-this-object shortcut.** Faster than `kubectl get events --field-selector involvedObject.name=foo`.
- **Events have expired by the time you look.** Default 1-hour TTL in etcd. Ship them to durable storage if you do postmortems beyond an hour later.
- **High-cardinality labels in Prometheus.** Putting `user_id` or `request_id` as a label = millions of time series = Prometheus dies. Labels are for low-cardinality dimensions.
- **No metrics-server → HPA stuck.** Status reads "FailedGetResourceMetric." Install it; verify with `kubectl top`.
- **Prometheus retention too short.** 24h is fine for alerts, useless for "this incident last week." Plan retention or use remote-write to a long-term store (Thanos, Mimir, VictoriaMetrics).
- **Sidecar logger competing with the app for resources.** A 256Mi limit on a sidecar can OOMKill it under burst → log gap. Right-size.
- **`kubectl logs -f` on hundreds of pods.** Bring your apiserver to its knees. Use a log UI (`stern`, `k9s`, Grafana Loki) for multi-pod tailing.
- **No correlation between logs, metrics, traces.** They're useless when you can't pivot. Use a common request ID emitted by all three pillars.
- **Forgetting Grafana behind auth.** Default install is open. One Shodan scan later, your dashboards are public.
- **Alert noise.** Every alert paged → on-call burnout → alerts ignored. Tune to "actionable, urgent, real" or move to ticketing.

---

## 🎯 Key Takeaways

- **Stdout logs + sidecar shipping is the default; configurable from there.** If your app writes log files inside the container, fix the app or sidecar around it.
- **Events expire — capture them.** "We saw OOMKills last Thursday" is unanswerable without an event exporter.
- **metrics-server and Prometheus are different things.** Both required for any serious cluster. New engineers confuse them; senior engineers install both before the first workload.
- **High-cardinality labels kill Prometheus.** Resist the urge to "just add user_id." Histograms + summaries with low-card labels are the right tool.
- **USE + RED + a runbook URL on every alert.** It's amazing how much of "the SRE job" reduces to those three habits.

*← [prev](./13_autoscaling.md) | [next →](./15_helm_and_kustomize.md)*
