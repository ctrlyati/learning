# 14 — Observability
> **Goal:** Get useful logs, healthchecks, metrics, and live diagnostics out of running containers. Pick the right logging driver, instrument healthchecks that mean something, and integrate Docker with Prometheus / cAdvisor for real metrics.

---

## 1. The pillars: logs, metrics, traces, events

For any production service you need:
- **Logs** — what happened, ordered, queryable.
- **Metrics** — counters/gauges over time (CPU, memory, request rate, error rate, latency).
- **Traces** — flows of requests across services (out of Docker's scope; instrument your app with OpenTelemetry).
- **Events** — Docker daemon's own state-change stream (container started/died/OOMed).

Docker itself helps directly with logs, container-level metrics, and events; you bring your own application metrics (via Prometheus, statsd, etc.) and traces.

---

## 2. Logs — the basics and the drivers

By default, anything your container writes to **stdout / stderr** is captured by Docker. That's the convention: **don't write log files inside the container; write to stdout.** This is one of the 12-factor app rules and the container world has fully adopted it.

```bash
docker logs api                       # all logs since container start
docker logs -f --tail 100 api         # follow live, last 100 lines
docker logs --since 5m api            # last 5 minutes
docker logs --since 2026-05-13T09:00 --until 2026-05-13T10:00 api
docker logs -t api                    # add timestamps
```

Where are those logs *stored*? It depends on the **logging driver**.

### The default: `json-file`

By default, Docker writes each line as a JSON object to `/var/lib/docker/containers/<id>/<id>-json.log`:

```bash
$ sudo cat /var/lib/docker/containers/$(docker inspect -f '{{.Id}}' api)/*-json.log | head -2
{"log":"listening on :8080\n","stream":"stdout","time":"2026-05-13T09:14:32.10Z"}
{"log":"db connected\n","stream":"stdout","time":"2026-05-13T09:14:32.83Z"}
```

These files **grow without bound by default**. Always configure rotation:

```json
// /etc/docker/daemon.json
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3",
    "compress": "true"
  }
}
```

Per-container override:

```bash
docker run --log-driver json-file --log-opt max-size=10m --log-opt max-file=3 myapp
```

Or in Compose:

```yaml
services:
  api:
    image: myapi
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
        compress: "true"
```

A disk filling up because a chatty container ran for a month is the most common production Docker incident. Configure rotation as a host policy in `/etc/docker/daemon.json` once, never worry about it again.

### Other drivers

| Driver | Use case |
|---|---|
| `json-file` | Default. Fine for local + small prod. |
| `local` | Like `json-file` but with built-in rotation and a smaller on-disk format. Good default for new setups. |
| `journald` | Send logs to systemd journal. Integrates with `journalctl`. Common on RHEL/Fedora hosts. |
| `syslog` | Ship to a remote syslog server. Old-school but ubiquitous. |
| `fluentd` | Ship to a Fluentd collector. Standard for K8s and many cloud envs. |
| `gelf` | Ship to Graylog. |
| `awslogs` / `gcplogs` | Direct to CloudWatch / Cloud Logging. |
| `splunk` | Direct to Splunk HEC. |
| `none` | Discard logs (rarely useful). |

**Heads-up:** with most non-`json-file` drivers, `docker logs <container>` stops working — the logs aren't local anymore. Workaround on modern Docker: `--log-driver local` keeps `docker logs` working *and* gives you sane defaults.

### The "containers stop logging" pitfall

If your log-shipping endpoint goes down and the driver is synchronous (default), your application's stdout/stderr writes start blocking. Production containers slow down or hang. Mitigations:

```yaml
logging:
  driver: fluentd
  options:
    fluentd-async: "true"        # buffer in memory, don't block
    fluentd-sub-second-precision: "true"
```

Use async modes for any network-bound logging driver in production.

---

## 3. Healthchecks — meaningful liveness signals

Containers have a `Health` status, exposed via:

```bash
$ docker inspect --format '{{.State.Health.Status}}' api
healthy
$ docker ps --format 'table {{.Names}}\t{{.Status}}'
NAMES   STATUS
api     Up 2 minutes (healthy)
db      Up 2 minutes (healthy)
```

Possible states: `starting` (in the `start_period` grace), `healthy`, `unhealthy`. Restarts / Compose dependencies key off this.

Define a healthcheck in the Dockerfile (preferred — travels with the image):

```dockerfile
HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1
```

Or per-container at run time:

```bash
docker run --health-cmd='wget -qO- http://localhost:8080/health' \
           --health-interval=30s --health-timeout=3s --health-retries=3 \
           myapp
```

In Compose (Module 08):

```yaml
healthcheck:
  test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"]
  interval: 30s
  timeout: 3s
  retries: 3
  start_period: 15s
```

**Design healthchecks that mean something:**
- Bad: `pidof nginx` — tells you the process is running, nothing about the app.
- Better: `curl localhost/` — proves the app is responding.
- Best: `curl localhost/health` where `/health` actually verifies critical paths (DB connection, downstream dependencies in non-strict mode).

Make `/health` cheap and side-effect-free. A healthcheck that does a full request flow is too expensive at 30s intervals.

Distroless images have no `wget`/`curl`. Either ship a tiny healthcheck binary or use the language's runtime:

```dockerfile
HEALTHCHECK CMD ["/myapp", "--healthcheck"]   # have the binary self-check
```

```dockerfile
# Python distroless
HEALTHCHECK CMD ["python", "-c", "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://localhost:8080/health').status==200 else 1)"]
```

---

## 4. Live diagnostics — `stats`, `top`, `events`, `inspect`

### `docker stats` — real-time resource use

```bash
$ docker stats --no-stream
CONTAINER ID   NAME      CPU %     MEM USAGE / LIMIT     MEM %     NET I/O           BLOCK I/O         PIDS
a1b2c3d4...    api       12.50%    180.4MiB / 1GiB       17.62%    5.21MB / 1.12MB   12.3MB / 0B       24
e5f6g7h8...    db        2.10%     412.0MiB / 1GiB       40.23%    1.10MB / 2.50MB   125MB / 64MB      18
```

Reads straight from cgroups (Module 12). For programmatic monitoring, the Docker API exposes the same data; better, run cAdvisor / Prometheus node-exporter for a proper time series.

### `docker top` — host's process view inside a container

```bash
$ docker top api
UID    PID    PPID   ...   CMD
app    24917  1234   ...   python app.py
app    24918  24917  ...   python app.py
```

Useful for figuring out what's actually running, and how the container's processes map to the host.

### `docker events` — daemon's event stream

```bash
$ docker events --since 1h
2026-05-13T09:14:32 container start a1b2c3d4 (image=myapi:1.0, name=api)
2026-05-13T09:18:47 container die a1b2c3d4 (exitCode=137, image=myapi:1.0, name=api)
2026-05-13T09:18:47 container restart a1b2c3d4
```

Filter with `--filter event=oom`, `--filter container=api`, `--filter type=image`. Stream it into a log aggregator for an audit trail of every container lifecycle event.

OOM kills specifically:

```bash
docker events --filter event=oom
```

When `--restart unless-stopped` is masking a crash loop, this is how you spot it.

### `docker inspect` — everything Docker knows

```bash
docker inspect api | jq '.[0].State'
{
  "Status": "running",
  "Running": true,
  "Pid": 24917,
  "ExitCode": 0,
  "StartedAt": "2026-05-13T09:14:32.10Z",
  "Health": {
    "Status": "healthy",
    "FailingStreak": 0,
    "Log": [
      { "Start": "...", "End": "...", "ExitCode": 0, "Output": "OK" }
    ]
  }
}
```

The `Health.Log` field is fantastic when debugging healthcheck failures — it shows the last several check outputs and exit codes.

---

## 5. Metrics — Prometheus + cAdvisor

Docker has a built-in metrics endpoint, but most production setups use **cAdvisor** (Google) — a container that watches the local Docker socket / cgroups and exposes per-container metrics in Prometheus format. Add **Prometheus** to scrape and **Grafana** to visualize.

```yaml
# monitoring.yml
services:
  cadvisor:
    image: gcr.io/cadvisor/cadvisor:v0.49.1
    privileged: true                           # needs broad host access
    ports: ["8080:8080"]
    volumes:
      - /:/rootfs:ro
      - /var/run:/var/run:ro
      - /sys:/sys:ro
      - /var/lib/docker:/var/lib/docker:ro
      - /dev/disk:/dev/disk:ro
    devices: ["/dev/kmsg"]

  prometheus:
    image: prom/prometheus:v2.54.0
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prom-data:/prometheus
    ports: ["9090:9090"]

  grafana:
    image: grafana/grafana:11.1.0
    ports: ["3000:3000"]
    environment:
      GF_SECURITY_ADMIN_PASSWORD: admin
    volumes:
      - grafana-data:/var/lib/grafana

volumes:
  prom-data:
  grafana-data:
```

```yaml
# prometheus.yml
global: { scrape_interval: 15s }
scrape_configs:
  - job_name: cadvisor
    static_configs: [{ targets: ['cadvisor:8080'] }]
  - job_name: app
    static_configs: [{ targets: ['api:9100'] }]    # your app's /metrics
```

Now you have per-container CPU/mem/network/disk metrics in Prometheus, plus whatever your app exports on `/metrics`. Plug the standard cAdvisor dashboard into Grafana (dashboard ID 14282 is a good starter) and you have a real container monitoring setup in 10 minutes.

### Docker daemon metrics

The daemon itself exposes Prometheus metrics if you turn it on:

```json
// /etc/docker/daemon.json
{
  "metrics-addr": "127.0.0.1:9323",
  "experimental": true
}
```

Then scrape `http://host:9323/metrics` for daemon-level info (build counts, container counts, image counts).

### Application metrics

Don't try to derive application metrics (request rate, error rate, P99 latency) from Docker. Instrument your app — Prometheus client libraries exist for every language. Expose `/metrics` and let Prometheus scrape.

---

## 6. Log aggregation in production

`docker logs` is fine for one host with a handful of containers. Beyond that you need centralized logs. Three popular stacks:

- **Loki + Promtail + Grafana** — log-shaped Prometheus. Cheap, simple, queryable like log search. The friendliest entry point.
- **ELK (Elasticsearch + Logstash + Kibana)** — heavy but powerful full-text search. The legacy default.
- **Fluentd / Fluent Bit + your cloud's log service** — Fluent Bit is lightweight and ships natively to CloudWatch, GCP Logging, Datadog, Splunk, etc.

A Loki + Promtail Compose snippet:

```yaml
services:
  loki:
    image: grafana/loki:3.1.1
    ports: ["3100:3100"]
    command: -config.file=/etc/loki/local-config.yaml

  promtail:
    image: grafana/promtail:3.1.1
    volumes:
      - /var/lib/docker/containers:/var/lib/docker/containers:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro    # ⚠ see Module 13
      - ./promtail-config.yml:/etc/promtail/config.yml:ro
    command: -config.file=/etc/promtail/config.yml
```

Promtail tails `/var/lib/docker/containers/*/*-json.log`, parses the JSON wrapper, attaches container labels, ships to Loki. In Grafana, query: `{container="api"} |= "error"` and you have searchable logs across every container.

---

## 7. A complete worked example: instrumented app

A Python service with structured stdout logs, a Prometheus metrics endpoint, and a working healthcheck.

```python
# app.py
from flask import Flask, jsonify, Response
from prometheus_client import Counter, Histogram, generate_latest, CONTENT_TYPE_LATEST
import logging, json, time, os, sys

logging.basicConfig(
    level=logging.INFO,
    format='%(message)s',
    stream=sys.stdout,
)
log = logging.getLogger()

def jlog(**kv):
    log.info(json.dumps({"ts": time.time(), **kv}))

app = Flask(__name__)
requests_total = Counter("app_requests_total", "Total requests", ["route", "code"])
request_latency = Histogram("app_request_seconds", "Request latency", ["route"])

@app.before_request
def before(): pass

@app.route("/")
def index():
    with request_latency.labels("/").time():
        time.sleep(0.01)
        requests_total.labels("/", "200").inc()
        jlog(route="/", status=200)
        return "hi\n"

@app.route("/health")
def health():
    return "ok\n", 200

@app.route("/metrics")
def metrics():
    return Response(generate_latest(), mimetype=CONTENT_TYPE_LATEST)

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=int(os.environ.get("PORT", "8080")))
```

```dockerfile
# Dockerfile
FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py .
RUN useradd -r -u 1001 app
USER app
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD python -c "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://localhost:8080/health').status==200 else 1)"
CMD ["python", "app.py"]
```

```yaml
# compose.yml
services:
  api:
    build: .
    ports: ["8080:8080"]
    logging:
      driver: json-file
      options: { max-size: "10m", max-file: "3" }
    restart: unless-stopped

  prometheus:
    image: prom/prometheus:v2.54.0
    volumes: [./prometheus.yml:/etc/prometheus/prometheus.yml:ro]
    ports: ["9090:9090"]
```

```yaml
# prometheus.yml
scrape_configs:
  - job_name: app
    static_configs: [{ targets: ['api:8080'] }]
```

Bring it up, hit it, inspect:

```bash
$ docker compose up -d --build
$ curl localhost:8080/                       # generate some traffic
hi
$ docker compose logs --tail 2 api
api-1  | {"ts": 1715593234.21, "route": "/", "status": 200}
api-1  | {"ts": 1715593235.05, "route": "/", "status": 200}

$ curl -s localhost:8080/metrics | grep -E '^app_'
app_requests_total{code="200",route="/"} 2.0
app_request_seconds_count{route="/"} 2.0
app_request_seconds_sum{route="/"} 0.022891

$ docker inspect --format='{{.State.Health.Status}}' compose-api-1
healthy

$ open http://localhost:9090     # Prometheus UI; query `app_requests_total`
```

Structured stdout logs, scrapable metrics, a healthcheck that proves the app is responding. That's the minimum for any production-grade container.

---

## 8. Common mistakes & gotchas

- **No log rotation, disk fills up.** Configure `max-size`/`max-file` in `/etc/docker/daemon.json` once and forget about it.
- **Apps writing to log files inside the container.** The files vanish on `docker rm`, and `docker logs` shows nothing. Log to stdout. If the app insists on file logging, symlink the log path to `/dev/stdout`.
- **Healthcheck that always returns OK.** Common with junior implementations: `/health` returns 200 unconditionally. Now Docker says "healthy" while the DB is on fire. Make health reflect *reality* (or split into liveness + readiness like K8s).
- **Heavyweight healthcheck.** `pg_dump` from `/health` is a real thing people have done. At 5s intervals you've DOSed your own DB. Healthchecks must be cheap.
- **`docker logs` empty for `--log-driver fluentd`.** That driver doesn't keep logs locally. Use `local` if you want both `docker logs` and remote shipping, or accept `docker logs` as remote-only.
- **Synchronous log driver in production.** Logging endpoint blip → containers stall. Enable async modes.
- **No `docker events` monitoring.** Restart loops go unnoticed because `docker ps` always shows "Up 2 seconds." Tail `docker events` (or scrape daemon metrics) to catch them.
- **cAdvisor running unprivileged.** It needs broad host access. Run it privileged (or with the specific capabilities/mounts it requires), but isolate it on a host-only network.
- **OOM kills appearing as "the app just exits."** Exit code 137, no obvious cause in app logs (because the kernel killed it cold). Check `docker events --filter event=oom` and `dmesg`.
- **Treating container metrics as application metrics.** cAdvisor tells you CPU and RAM. It doesn't tell you P99 latency or queue depth — those come from inside the app via Prometheus client libraries.

---

## 🎯 Key Takeaways

- **Log to stdout, configure rotation, choose your driver.** `max-size`/`max-file` defaults at the daemon level prevent the most common Docker incident (disk full).
- **Healthchecks must reflect reality.** Cheap, side-effect-free, and *actually* indicate readiness. They drive Compose `depends_on`, restart logic, and orchestrator decisions everywhere.
- **`docker stats`, `top`, `events`, `inspect`** are your daily diagnostic toolkit. Learn them; they replace `top`/`ps`/`syslog` for container worlds.
- **cAdvisor + Prometheus + Grafana** in 30 lines of Compose gives you a real observability stack for a single Docker host. Scale it horizontally and you have multi-host. K8s setups follow the same pattern.
- **Don't conflate container metrics with application metrics.** Docker tells you cgroup stats; your app must tell you its own (requests, errors, latencies). Instrument both.

*[prev ← 13_security](./13_security.md) | [next → 15_cicd_with_docker](./15_cicd_with_docker.md)*
