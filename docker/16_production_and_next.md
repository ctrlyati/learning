# 16 — Production Patterns & What's Next
> **Goal:** Understand where Docker (single-host + Compose) is the right answer and where it isn't, recognize the common production footguns that bite real teams, and know exactly when (and how) to graduate to Kubernetes.

---

## 1. When Docker (and Compose) is the right answer

Docker + Compose is sometimes *exactly* the right tool, and that's worth saying because the industry's gravitational pull toward Kubernetes has made teams over-engineer absurdly small workloads.

**Docker + Compose is great for:**
- **Single-server production** — a VPS, a small VM, an internal app on one box. A 95% reliability tier, on one machine, with one Compose file, is honest engineering.
- **Edge / IoT** — Raspberry Pi, small appliances, retail kiosks. Compose + watchtower (or Portainer) is plenty.
- **Dev / staging environments** — fast spin-up, full-fidelity local copies of prod.
- **Side projects and proofs of concept** — the cost of running K8s for two services is laughable.
- **CI build environments** — Compose to spin up your test deps (DB, Redis, mock services) for each test job.

The threshold question: **does this workload need horizontal scaling, rolling updates without downtime, self-healing across hosts, and autoscaling?** If yes → orchestration. If no → Compose is fine.

A startup with a Rails app, a Postgres, and a Redis on one $40/month VPS, deployed via `git pull && docker compose up -d --build`, is not a worse engineer than someone running the same thing across an EKS cluster. They've just chosen a cheaper, simpler operating model. Many companies stay there profitably for years.

---

## 2. When Docker is *not* the right answer

Honest list of where containers aren't the right primitive:

- **Stateful databases at scale.** A single Postgres in Docker on one host is fine. A high-availability Postgres cluster with synchronous replication, automated failover, and PITR is *better* on managed services (RDS, Cloud SQL) or specifically-built operators (CloudNativePG, Crunchy) on K8s. Don't roll your own.
- **GUI applications.** You *can* run desktop apps in containers (X11 forwarding, web-rendered) but it's painful. Use containers for headless services.
- **Workloads needing direct hardware access** — kernel-module debugging, specialized I/O on bare metal, certain GPU/TPU workloads with tight driver coupling. Containers add friction without much benefit here.
- **Latency-critical / kernel-bypass workloads** — DPDK, RDMA. The veth+iptables overhead matters. Use `--network host` or move to bare metal.
- **Heavy multi-tenant isolation between hostile workloads** — kernel namespaces are weaker than VMs. For "running customer code that might be malicious," use VMs, Firecracker (Lambda-style microVMs), or gVisor.
- **Anything where you'd genuinely benefit from running on a specific OS the container can't realistically provide** — Windows workloads on Linux hosts, niche kernel modules.

Most teams won't hit these. But knowing where containers stop helps you not force them.

---

## 3. Compose vs Swarm vs Kubernetes — pick deliberately

| | Compose | Swarm | Kubernetes |
|---|---|---|---|
| **Scope** | One host | Multi-host cluster | Multi-host cluster (massive) |
| **HA / failover** | Restart on same host | Reschedule to another node | Full reschedule + autoscale |
| **Rolling updates** | Manual | Built-in | Built-in, sophisticated |
| **Autoscaling** | No | Limited | Yes (HPA, VPA, CA) |
| **Service discovery** | Compose DNS | Swarm DNS | Service objects + DNS |
| **Secrets** | Compose secrets | Swarm secrets | Secrets + external KMS |
| **Networking** | Bridges per project | Overlay across nodes | CNI plugins, NetworkPolicy |
| **Ecosystem** | Modest | Stagnant | Enormous, vibrant |
| **Learning curve** | Hours | Days | Weeks-months |
| **Right for** | 1 server, dev, edge | 2-20 servers, simple HA | Anything beyond that |

### Docker Swarm — the in-between

Swarm is built into Docker and turns N hosts into a cluster with a Compose-like CLI. It's underrated and overhated. For 2-20 nodes, simple HA, and Compose-grade ergonomics, it's a perfectly fine choice. The catch is **the ecosystem is largely frozen** — almost all new tooling (operators, service meshes, GitOps) targets Kubernetes. If you don't need ecosystem, Swarm is great. If you'll want one of those tools, skip Swarm.

A quick Swarm taste:

```bash
docker swarm init
docker stack deploy -c compose.yml mystack
docker service ls
docker service update --image myapp:v2 mystack_api    # rolling update
```

### Kubernetes — when you graduate

You should consider Kubernetes when you have:
- Multiple services that need to scale independently.
- Multiple machines (or you need HA that survives one host dying).
- Need for sophisticated rollout strategies (canaries, blue/green).
- An ecosystem dependency (Istio, ArgoCD, Prometheus Operator, KEDA, cert-manager).
- A team large enough that the K8s operational tax pays for itself.

Don't fall into the trap of: "we have 3 services on 1 server, let's start on K8s in case we grow." That's premature optimization that costs you months of yak-shaving.

When you *do* graduate to K8s, **80% of your Docker knowledge transfers**: pods run containers, containers come from images, images come from registries, networks are still namespaces, volumes are still mounts. K8s is mostly an orchestrator on top of the runtime stack you already understand. The new concepts are pods (a group of co-scheduled containers), deployments (declarative desired state), services (stable virtual IPs in front of pod sets), ingress (HTTP routing), and the controller pattern (reconcile loops).

→ **Next course: [`../kubernetes/00_roadmap.md`](../kubernetes/00_roadmap.md)**

---

## 4. Running Docker on servers — production-grade single-host

If Compose-on-a-VPS is your destination, do it well.

### Host setup checklist

- **OS:** Ubuntu LTS, Debian stable, or AL2023. Boring is good.
- **Docker:** install via the official apt repo (not `apt install docker.io`, which lags). Pin a major version; enable unattended security updates for the docker package.
- **Storage driver:** overlay2 (default; verify with `docker info | grep Storage`). Use ext4 or xfs with `-i size=2048`.
- **Logging:** configure rotation in `/etc/docker/daemon.json`:
  ```json
  {
    "log-driver": "local",
    "log-opts": { "max-size": "10m", "max-file": "3" },
    "live-restore": true,
    "default-ulimits": { "nofile": { "Name": "nofile", "Soft": 65536, "Hard": 65536 } }
  }
  ```
- **`live-restore: true`** keeps containers running across daemon restarts/upgrades. Huge quality-of-life win.
- **Firewall:** `ufw` or `firewalld`. Note that Docker bypasses `ufw` by default for published ports — use `DOCKER-USER` chain rules or `--iptables=false` (advanced).
- **TLS / reverse proxy:** Caddy or Traefik or nginx in front of your services. Caddy gives automatic Let's Encrypt for almost zero config.
- **Backups:** schedule `docker exec db pg_dumpall` (or your DB's equivalent) and `tar` of bind-mounted config to off-site storage. Test restores.
- **Monitoring:** cAdvisor + Prometheus + Grafana from Module 14, or a hosted equivalent (UptimeRobot for liveness, Grafana Cloud for metrics).
- **Updates:** Watchtower (auto-update images), or a deploy script. Auto-update is risky for stateful services; prefer explicit deploys.

### A reasonable production Compose

```yaml
services:
  caddy:                     # reverse proxy + auto-TLS
    image: caddy:2
    restart: unless-stopped
    ports: ["80:80", "443:443"]
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy-data:/data
      - caddy-config:/config

  api:
    image: ghcr.io/me/api@sha256:abc...   # pinned digest
    restart: unless-stopped
    deploy:
      resources:
        limits: { cpus: '1.0', memory: 512M }
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"]
      interval: 30s
      retries: 3
    environment:
      DATABASE_URL: postgres://app@db/app
      DB_PASSWORD_FILE: /run/secrets/db_password
    secrets: [db_password]
    networks: [internal]
    depends_on:
      db: { condition: service_healthy }

  db:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: app
      POSTGRES_USER: app
      POSTGRES_PASSWORD_FILE: /run/secrets/db_password
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app"]
      interval: 5s
    secrets: [db_password]
    networks: [internal]

  backup:
    image: postgres:16-alpine
    restart: "no"
    entrypoint: ["sh", "-c"]
    command:
      - |
        while true; do
          PGPASSWORD="$(cat /run/secrets/db_password)" \
            pg_dump -h db -U app app | gzip > /backups/app-$(date +%F-%H%M).sql.gz
          find /backups -name 'app-*.sql.gz' -mtime +14 -delete
          sleep 86400
        done
    volumes:
      - ./backups:/backups
    secrets: [db_password]
    networks: [internal]
    depends_on:
      db: { condition: service_healthy }

networks:
  internal:

volumes:
  pgdata:
  caddy-data:
  caddy-config:

secrets:
  db_password:
    file: ./secrets/db_password.txt
```

Plus a `Caddyfile`:

```
api.example.com {
    reverse_proxy api:8080
}
```

That's a real production stack: TLS auto-renew, healthchecks, restart policy, resource limits, daily backups with retention, secrets via files, internal network segmentation. Many companies run things like this for years, profitably.

---

## 5. Common production footguns

In the time I've watched teams operate Docker in production, these are the recurring mistakes — fix them once and skip the pain:

- **No log rotation.** Disk fills. Always set `max-size`/`max-file` (Module 14).
- **`docker system prune -a` on a cron.** Removes images you actually need (cached bases that take 5 minutes to re-pull). Use targeted pruning: `docker image prune` for dangling, schedule conservatively.
- **No memory limits.** One container leaks, host OOMs, *something else* gets killed. Set `mem_limit` everywhere.
- **`restart: always` on every service.** Combined with a crash-loop bug, this hammers the daemon and confuses ops. `unless-stopped` lets you `docker stop` and have it *stay* stopped.
- **`docker-compose down -v` in muscle memory.** Wipes volumes. Wipes your DB. Wipes the company. Train fingers to never type this on prod.
- **`:latest` in production Compose files.** Tomorrow's `:latest` is different. Pin tags or digests.
- **Single point of failure ignored.** "It's one server, what could go wrong?" Disk failure, network partition, power outage. Have a runbook for "host is down" — even if the answer is "restore from backup on a new VPS."
- **Backups that have never been restored.** Backups not restored are wishful thinking. Test restore quarterly.
- **Secrets in `.env` checked into git.** Search your repo for `.env`, `password`, `SECRET`. Add a pre-commit hook for `gitleaks`.
- **The forgotten test container.** `docker run -d --name probe ...`, debugger left running, six months later still consuming memory and CPU. `docker ps -a --filter "since=30d"` audits.
- **Bind-mounting `/var/run/docker.sock` "for monitoring."** That container now has root on the host. Use socket proxies (Module 13).
- **Privileged containers running as services.** Compose review must reject `privileged: true` outside of explicit, justified cases.
- **No drain step on deploys.** `docker compose up -d --no-deps api` will kill the old `api` before the new one is ready, dropping requests. Use rolling-update-style deploys (Swarm, K8s, or a load balancer in front).
- **`docker-compose` (v1) still in shell scripts.** Old, deprecated, behaviorally divergent. Migrate to `docker compose` (v2).
- **No CPU/IO quotas on a noisy neighbor.** One pathological container starves everything else on the host. Set `cpus:` and `blkio-weight`.
- **The `node_modules` host bind mount in production.** Dev habits leaking into prod compose files. Production should build the image with its deps baked in.
- **Hot patches in running containers.** "Just `docker exec` and edit the file" → next deploy reverts the fix. If you must, treat it as a hotfix branch: make the change, rebuild, deploy properly.
- **Image registries with no eviction.** GHCR/ECR fill up with thousands of CI builds. Set retention policies (keep last 50 SHAs, all semver releases, all signed releases).

---

## 6. The transition cue — recognizing when to leave

You'll know it's time to consider Kubernetes (or a managed equivalent like ECS, Cloud Run, App Runner) when any of these become real:

1. **Multi-host requirements.** You need more than one machine for capacity or HA, and want a unified deploy model across them.
2. **Rolling updates with zero downtime.** Compose can do this with care (extra healthchecks, careful sequencing); K8s does it as a built-in.
3. **Autoscaling.** Traffic varies by 10x; you want to scale up replicas on load and down off-peak.
4. **Operator ecosystem.** You want `cert-manager` for certs, `prometheus-operator` for monitoring, `argocd` for GitOps. These are K8s-native; bringing them to Compose is rowing upstream.
5. **Many services, many teams.** Coordinating deploys, rollouts, and namespaces across many teams is what K8s was designed for.

Until then, **stay with Compose**. The Kubernetes operational cost is real (a small platform team's worth of human effort, ongoing). Don't pay it before you need to.

When you *do* graduate, here's the sequence that minimizes pain:

1. **Containerize and Compose-ify everything first.** This module's worth, end-to-end. You can't usefully move to K8s if your image hygiene, healthchecks, and config management aren't already in good shape.
2. **Pick managed K8s** (EKS, GKE, AKS, DigitalOcean). Running your own control plane is a 5-engineer job; let someone else.
3. **Start with one service**, not the whole stack. Migrate the most cattle-like (stateless, horizontally scalable) service first. Keep stateful services on Compose or managed services initially.
4. **Reuse the images you already built.** Same Dockerfiles, same registry, same tags. K8s pulls from where your CI already pushes.
5. **Translate Compose to K8s manifests.** `kompose` does a passable first draft; hand-edit it. Or use Helm from the start.
6. **Keep dev on Compose.** "Dev on Compose, prod on K8s" is a totally legitimate, very common setup. Don't force developers to run K8s locally.

→ **[`../kubernetes/00_roadmap.md`](../kubernetes/00_roadmap.md)** picks up exactly where this leaves off.

---

## 7. Final mental models for production Docker

Six things to internalize for production:

1. **The image is the artifact.** Once built, scanned, signed, and pushed, it's immutable. Everything downstream should reference it by digest.
2. **The host is a hostile environment for containers, and vice versa.** Defense in depth. Non-root, capabilities dropped, read-only filesystems, limits everywhere.
3. **Stateful workloads need explicit, tested storage.** Volumes that survive container recreation, backups that survive disk failure, restore procedures that survive panic.
4. **Observability is not optional.** Logs to stdout, structured if you can. Healthchecks that mean something. Per-container metrics via cAdvisor. Application metrics via Prometheus.
5. **Reproducibility beats cleverness.** Pin everything. Same image, same digest, same config in dev, staging, and prod. Surprises in prod are almost always things that weren't reproduced earlier.
6. **Know what you're not doing.** If your workload genuinely needs orchestration, don't fake it with bash scripts on top of Compose. Either accept the constraint (single host, manual ops) or move to the right tool. Half-measures combine the costs of both.

---

## 8. Common mistakes & gotchas (one last batch)

- **"We don't need Compose, we use shell scripts."** Six months later, the shell scripts are 800 lines and nobody understands them. Compose is the shell script you would have written, but standardized.
- **Overcomplicating early.** Two services on one server doesn't need service mesh, Istio, Vault Operator, ArgoCD, and a Linkerd sidecar. Start simple.
- **Undercomplicating late.** Twenty services across five hosts probably *does* need orchestration. Don't be religious about staying on Compose.
- **`docker info` ignored.** When a daemon is misbehaving, `docker info` shows storage driver, kernel version, cgroup driver, security options, and swap usage. First thing to capture in any incident.
- **Backups that take 4 hours.** Test how long restore actually takes, on the actual hardware. "We have backups" without a restore-time number isn't a recovery plan.
- **DNS inside Docker breaking on host network changes.** Daemon caches DNS; restart it (`systemctl restart docker`) after fundamental network changes (new VPN, new DNS servers).
- **Trying to make Compose do canary deploys.** You can fake it with two services and an external load balancer, but it's painful. If you need canaries, that's a K8s sign.
- **Ignoring node-level upgrades.** Docker, the kernel, the host OS — they all need patching. Schedule it, automate it, test recovery from "host suddenly reboots."

---

## 🎯 Key Takeaways

- **Docker + Compose is the right answer for surprisingly many workloads.** Don't move to Kubernetes prematurely just because the industry talks about it; the operational cost is real and often unjustified.
- **Pin everything in production:** image digests, tag conventions, resource limits, secrets, log rotation, restart policies, backups, healthchecks. Defaults are for development; production is explicit.
- **Know the footgun list cold:** no log rotation, `down -v`, `:latest` in prod, missing memory limits, `/var/run/docker.sock` exposed, untested backups. These are the incidents you can prevent on a quiet afternoon.
- **Kubernetes is the natural next step when (and only when) you need multi-host orchestration, sophisticated rollouts, or the K8s ecosystem.** 80% of your Docker knowledge transfers — pods are still containers, images still come from registries.
- **Building Docker fluency is one of the highest-leverage skills in modern software.** Whether you stay at Compose or grow into Kubernetes, every modern workload — cloud, edge, ML, CI — runs on the foundation you've now built. From here, you can go anywhere.

*[prev ← 15_cicd_with_docker](./15_cicd_with_docker.md) | [next → ../kubernetes/00_roadmap](../kubernetes/00_roadmap.md)*
