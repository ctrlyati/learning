# 07 — Networking
> **Goal:** Demystify Docker networking — the three built-in driver modes (`bridge`, `host`, `none`), user-defined networks, container DNS, and port publishing — and learn to debug network issues by reading what's actually happening at the kernel level.

---

## 1. The big picture — networking is just iptables and veth pairs

The roadmap mental model: **networking is just iptables underneath**. There's no special Docker protocol. When you run a container with `-p 8080:80`, Docker:

1. Creates a virtual ethernet device pair (`veth`). One end goes into the container's network namespace, the other into the host.
2. Attaches the host end to a Linux bridge (`docker0` by default).
3. Assigns an internal IP (`172.17.0.x` by default) to the container's end.
4. Adds iptables NAT rules so traffic to `host:8080` is DNATed to `container_ip:80`.
5. Adds a masquerade rule so outbound container traffic looks like it's coming from the host.

That's it. The whole thing is a few iptables rules and some virtual NICs. If you can read `iptables -t nat -L -n` and `ip link`, you can debug *any* Docker networking issue.

```
host
 │
 ├─ eth0           (real NIC, e.g. 10.0.0.5)
 ├─ docker0        (bridge, 172.17.0.1)
 │   ├─ veth1 ──── ┐  (one end of pair → container1)
 │   ├─ veth2 ──── ┐
 │   └─ ...        │
 │                 │
 └─ iptables NAT   │
     ┌─────────────┘
     ▼
   container's netns
     ├─ eth0 (172.17.0.2)
     └─ lo
```

The container has its own `lo`, its own routing table, its own iptables. It's a Linux network namespace.

---

## 2. The built-in network drivers

`docker network ls` always shows at least three:

```
$ docker network ls
NETWORK ID     NAME      DRIVER    SCOPE
12ab34cd56ef   bridge    bridge    local
78ef90ab12cd   host      host      local
34cd56ef78ab   none      null      local
```

### `bridge` — the default for single-host

This is the network you get when you don't specify one. New containers attach to `docker0`, get IPs from `172.17.0.0/16`, and have NAT for outbound traffic. **But the default `bridge` network has a major limitation: no built-in DNS-based service discovery.** Containers can ping each other by IP but not by name.

### `host` — share the host's network stack

```bash
docker run --rm --network host nginx
```

No namespacing, no bridge, no NAT. The container uses the host's network directly. Pros: zero overhead, no port-publishing dance. Cons: you can't have two containers binding the same port; the container can sniff or interfere with all host traffic. **Linux-only** in any useful sense; on Docker Desktop (Mac/Windows) `--network host` is partially supported via a special hack but doesn't behave the same as Linux.

Use cases: performance-sensitive workloads (high-throughput proxies), monitoring agents that need to see the host network.

### `none` — no network at all

```bash
docker run --rm --network none alpine ping 1.1.1.1
ping: bad address '1.1.1.1'
```

The container has only `lo`. Perfect for sandboxing untrusted code, or batch jobs that shouldn't reach the internet.

### `overlay` — multi-host (Swarm)

Connects containers across multiple Docker hosts using VXLAN tunnels. Exclusively used with Docker Swarm. We'll mention it in Module 16 but it's not on the "you must know this" list for most developers — Kubernetes has its own networking (CNI plugins).

### `macvlan` and `ipvlan` — each container gets a real LAN IP

The container appears on the physical LAN as a first-class citizen with its own MAC address. Useful for legacy apps that demand "real" networking, or appliances. Niche.

---

## 3. User-defined bridge networks — the way you should work

The default `bridge` is for backwards compatibility. **Always create a user-defined network for any real workload.** It gives you built-in DNS-based service discovery.

```bash
docker network create app-net

docker run -d --name db --network app-net \
  -e POSTGRES_PASSWORD=secret postgres:16

docker run -d --name api --network app-net \
  -e DATABASE_URL=postgres://postgres:secret@db:5432 myapi:latest
```

Inside the `api` container, the hostname **`db`** resolves to the Postgres container's IP automatically. No `/etc/hosts` hacks, no IP discovery. Docker runs an embedded DNS server at `127.0.0.11` inside each container connected to a user-defined network; it's pre-populated with every other container on that network.

```bash
$ docker exec api cat /etc/resolv.conf
nameserver 127.0.0.11
options ndots:0

$ docker exec api nslookup db
Server:         127.0.0.11
Address:        127.0.0.11:53
Name: db
Address: 172.18.0.2
```

A container can be attached to multiple networks (`docker network connect othernet api`), giving you network segmentation: e.g., a `frontend` network and an `internal` network so the frontend can't reach the DB except via the API.

```bash
docker network create frontend
docker network create internal

docker run -d --name db --network internal postgres:16
docker run -d --name api --network internal --network frontend myapi
docker run -d --name web --network frontend -p 80:80 mynginx
```

`web` reaches `api` over `frontend`. `api` reaches `db` over `internal`. `web` cannot reach `db` directly. Defense-in-depth at the network layer.

Compose does this for you automatically — every `docker-compose.yml` gets a project-named network (e.g., `myproj_default`) with every service joined to it. We'll see this in Module 08.

---

## 4. Port publishing — getting traffic from the host to the container

By default, a container's ports are reachable *only from other containers on the same Docker network*, not from the host or the outside world. To expose a port to the host, **publish** it:

```bash
docker run -d -p 8080:80 nginx
# host:8080 → container:80

docker run -d -p 127.0.0.1:8080:80 nginx
# only loopback on host:8080 → container:80 (not exposed externally)

docker run -d -p 8080:80/udp some-udp-app
# UDP instead of TCP

docker run -d -P nginx
# publish all EXPOSEd ports on random high host ports; see with `docker ps`
```

Format: `[HOST_IP:]HOST_PORT:CONTAINER_PORT[/PROTO]`.

`docker port api` shows the actual bindings:

```
$ docker port api
8080/tcp -> 0.0.0.0:8080
8080/tcp -> [::]:8080
```

Inspect the iptables rules that make this happen:

```bash
$ sudo iptables -t nat -L DOCKER -n
Chain DOCKER (2 references)
target     prot opt source               destination
RETURN     all  --  0.0.0.0/0            0.0.0.0/0
DNAT       tcp  --  0.0.0.0/0            0.0.0.0/0    tcp dpt:8080 to:172.17.0.2:80
```

There it is — a literal NAT rule rewriting destination `:8080` to `172.17.0.2:80`. The container has no idea any of this happened; it just sees traffic arriving on port 80.

**`EXPOSE` doesn't publish**, repeating from Module 03. It's metadata. The flag that actually publishes is `-p` (or `-P` for random ports).

### Listening on `0.0.0.0`, not `localhost`

A container that listens on `127.0.0.1` is only reachable from inside itself. Make sure your app binds to `0.0.0.0` so the port-published traffic can reach it. This is the #1 "I published port 8080 and curl times out" cause:

```python
# WRONG inside a container
app.run(host="127.0.0.1", port=8080)

# RIGHT
app.run(host="0.0.0.0", port=8080)
```

---

## 5. The whole DNS picture

Inside a container on a user-defined network:

| Name | Resolves to |
|---|---|
| Another container's `--name` on the same network | That container's IP, via Docker's embedded DNS (`127.0.0.11`) |
| External hostnames (`google.com`) | Forwarded by the embedded DNS upstream (usually the host's resolver) |
| `host.docker.internal` | The host machine (Docker Desktop only by default; available on Linux with `--add-host host.docker.internal:host-gateway`) |
| Aliases set with `--network-alias` | The container that owns the alias, on that specific network |

`--network-alias` is underused:

```bash
docker run -d --name db --network app-net --network-alias postgres --network-alias primary-db postgres:16
# Other containers can reach this as: db, postgres, OR primary-db
```

Compose uses this so service names act as DNS names regardless of container names.

---

## 6. A practical example — a tiny web stack

```bash
docker network create demo

# Database — internal only, no port published
docker run -d --name pg --network demo \
  -e POSTGRES_PASSWORD=secret postgres:16

# API — talks to db by name; not published externally
docker run -d --name api --network demo \
  -e DATABASE_URL=postgres://postgres:secret@pg:5432 \
  myapi:latest

# Nginx reverse proxy — published on host
cat > nginx.conf <<'EOF'
events {}
http {
  server {
    listen 80;
    location / { proxy_pass http://api:3000; }
  }
}
EOF

docker run -d --name web --network demo \
  -v $(pwd)/nginx.conf:/etc/nginx/nginx.conf:ro \
  -p 8080:80 \
  nginx:1.27-alpine

# Test
curl http://localhost:8080/
```

Flow: `curl` hits the host's port 8080 → iptables DNAT → nginx container's port 80 → nginx proxies to `http://api:3000` (Docker DNS resolves `api`) → API queries `pg:5432` (Docker DNS resolves `pg`).

Only `web`'s port is published. `api` and `pg` are reachable only via the `demo` network, not from the outside. That's good defense in depth — your DB isn't on the internet by accident.

### Debugging recipe

When networking misbehaves, run these in order. They almost always pinpoint the issue:

```bash
# 1. Is the container actually running and on the right network?
docker inspect -f '{{json .NetworkSettings.Networks}}' api | jq

# 2. Does Docker DNS resolve the name from inside?
docker exec api getent hosts pg

# 3. Is the upstream service actually listening?
docker exec pg ss -tlnp

# 4. Can you reach it from inside another container on the same network?
docker exec api nc -zv pg 5432

# 5. (Host networking issue) Does the host have the published port?
ss -tlnp | grep 8080      # Linux
netstat -an | grep 8080   # Windows/Mac

# 6. (Nuclear) iptables NAT rules
sudo iptables -t nat -L DOCKER -n -v
```

Going through this list once will teach you more about Docker networking than reading any doc.

---

## 7. Common mistakes & gotchas

- **Using the default `bridge` network and expecting DNS.** The default bridge intentionally lacks service discovery. Create a user-defined network: `docker network create app-net`.
- **App binds to `127.0.0.1` inside the container.** Port publishing works; nothing answers. Bind to `0.0.0.0`.
- **`--link` (legacy).** Pre-Docker 1.10 way to connect containers. Deprecated. Use user-defined networks instead.
- **Two containers, same host port.** `docker: Error response from daemon: driver failed programming external connectivity ... port is already allocated`. Either change the host-side port or stop the other container.
- **Forgetting to publish a port and `curl localhost` fails.** Check `docker ps` — if the PORTS column shows `80/tcp` (no host mapping), you forgot `-p 8080:80`.
- **`-p 80` (without colon).** Publishes container port 80 on a random host port. Probably not what you wanted; `docker ps` reveals the actual host port.
- **DNS in containers using the host's `/etc/resolv.conf`.** On corporate networks with custom DNS, you may need `--dns 10.0.0.1` or the daemon-level `dns` config in `/etc/docker/daemon.json`.
- **Host networking "isn't faster" on Mac.** Because Docker Desktop on Mac/Windows runs a VM, there is no real host network shortcut — the traffic still crosses the VM boundary. `--network host` mostly only matters on Linux.
- **macvlan and the "container can't talk to its own host" surprise.** It's by design — Linux network namespaces can't talk to themselves through the bridge they're attached to. Workaround: a second small bridge or `--add-host`.
- **`docker0` overlapping a corporate VPN subnet.** Suddenly traffic to `172.17.x.x` is dropped because the VPN owns that range. Reconfigure Docker's default bridge in `/etc/docker/daemon.json` with `"bip": "192.168.144.1/24"` or similar.
- **Embedded DNS only on user-defined networks.** Default bridge gets none. New users hit this trying to ping containers by name from `--network bridge`.
- **Container IPs change.** Don't hardcode `172.17.0.2`. Use container names + user-defined network DNS.

---

## 🎯 Key Takeaways

- **Networking is veth pairs + a bridge + iptables NAT** — not a black box. Knowing this means you can always reach the bottom of any networking issue with `ip`, `iptables`, and `ss`.
- **Always use a user-defined bridge network**, not the default one. You get DNS-based service discovery, network segmentation, and saner inspectability.
- **`EXPOSE` is documentation; `-p` is publishing.** Different jobs. Confusing them is a top-5 newbie issue.
- **Apps inside containers must bind to `0.0.0.0`**, not `127.0.0.1`, for port publishing to be useful.
- **Segment networks for defense in depth.** Frontend on one network, internal services on another, no shared blast radius. Compose makes this trivial.

*[prev ← 06_volumes_and_mounts](./06_volumes_and_mounts.md) | [next → 08_docker_compose](./08_docker_compose.md)*
