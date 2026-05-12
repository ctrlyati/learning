# 04 — Services & kube-proxy

> **Goal:** Understand how stable virtual IPs route to ephemeral pods, the four Service types, what kube-proxy actually does with iptables/IPVS, and when to reach for a Headless Service.

---

## 1. The Service — analogy + working YAML

**Analogy.** Pods are ephemeral — they get new IPs every restart. A Service is a **phone number that always reaches whoever's on call**, regardless of which person it actually is. You publish the number once; the call routing keeps up with shift changes. The phone number is a *virtual IP* (cluster IP) and the routing is maintained by kube-proxy on every node.

```yaml
# A Deployment to back the Service
apiVersion: apps/v1
kind: Deployment
metadata: { name: web }
spec:
  replicas: 3
  selector: { matchLabels: { app: web } }
  template:
    metadata: { labels: { app: web } }
    spec:
      containers:
        - name: nginx
          image: nginx:1.27-alpine
          ports: [{ name: http, containerPort: 80 }]
---
apiVersion: v1
kind: Service
metadata: { name: web }
spec:
  type: ClusterIP            # default — only reachable inside the cluster
  selector: { app: web }
  ports:
    - name: http
      port: 80               # the Service's port (clients hit this)
      targetPort: http       # which container port (named ref)
```

```bash
$ kubectl apply -f web.yaml
deployment.apps/web created
service/web created

$ kubectl get svc,ep web
NAME          TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)   AGE
service/web   ClusterIP   10.96.184.241   <none>        80/TCP    7s

NAME             ENDPOINTS                                    AGE
endpoints/web    10.244.0.5:80,10.244.0.6:80,10.244.0.7:80    7s

$ kubectl run shell --rm -it --image=alpine -- sh
/ # apk add curl >/dev/null
/ # curl -s web.default.svc.cluster.local | head -3
<!DOCTYPE html>
<html>
<head>
```

The Service has its own IP (`10.96.184.241`) that never changes. The **Endpoints** object lists the actual pod IPs. The two are kept in sync by the **endpoint controller** based on the Service's `selector` matching Pod labels.

---

## 2. Mechanism — kube-proxy, Endpoints, EndpointSlices, and DNS

Three things have to be true for `curl web` to work:

1. **DNS** resolves `web` to `10.96.184.241` (CoreDNS is doing this — see module 11).
2. **The cluster IP `10.96.184.241` is routable from any pod.** This is *not* a real IP — there's no NIC anywhere with that address. Magic.
3. **The TCP connection ends up at one of the pod IPs.** Also magic.

The magic is **kube-proxy**, running on every node.

### How kube-proxy programs the kernel

kube-proxy watches the apiserver for Services and EndpointSlices. For every Service it sees, it programs **iptables** rules (or **IPVS** rules, depending on mode) on its own node. When a pod sends a packet to `10.96.184.241:80`:

1. The packet leaves the pod's network namespace and hits the node's network stack.
2. iptables/IPVS sees a rule: *"if destination is `10.96.184.241:80`, DNAT to one of these backend IPs at random."*
3. The packet is rewritten with the chosen pod IP and forwarded.
4. The reverse-NAT happens on the response.

The pod never knew. The Service IP is **purely a forwarding fiction** maintained by kernel-level rules on every node.

```bash
# Look at the iptables rules (kind/Docker Desktop)
$ docker exec learn-control-plane iptables-save | grep web
-A KUBE-SERVICES -d 10.96.184.241/32 -p tcp -m tcp --dport 80 -j KUBE-SVC-XYZ
-A KUBE-SVC-XYZ -j KUBE-SEP-AAA  (1/3 probability)
-A KUBE-SVC-XYZ -j KUBE-SEP-BBB  (1/2 probability of remainder)
-A KUBE-SVC-XYZ -j KUBE-SEP-CCC  (rest)
-A KUBE-SEP-AAA -p tcp -j DNAT --to-destination 10.244.0.5:80
```

That's the whole trick.

### iptables mode vs IPVS mode

| Mode | Rule lookup | Scale |
|------|-------------|-------|
| **iptables** | Linear evaluation of chains | Fine up to a few thousand Services. Default. |
| **IPVS** | Hash-table lookup in kernel | Scales to tens of thousands. Used in big clusters. |
| **nftables** | Newer replacement for iptables | Beta/GA in recent K8s; future default. |

### Endpoints vs EndpointSlices

Originally each Service had a single **Endpoints** object listing *every* backing pod. With huge Services (thousands of pods) this object became enormous and every kube-proxy on every node had to re-receive it on every change. **EndpointSlices** (GA in 1.21) split that into chunks of ~100 endpoints — only changed slices propagate. EndpointSlices are what kube-proxy actually consumes now; the `Endpoints` object is kept around for backward compatibility.

```bash
$ kubectl get endpointslices -l kubernetes.io/service-name=web
NAME        ADDRESSTYPE   PORTS   ENDPOINTS                          AGE
web-h4r92   IPv4          80      10.244.0.5,10.244.0.6,10.244.0.7   3m
```

### Service DNS

CoreDNS exposes every Service as:
```
<svc>.<namespace>.svc.cluster.local
```
And every Pod as:
```
<pod-ip-with-dashes>.<namespace>.pod.cluster.local
```
A pod's `/etc/resolv.conf` has search domains so within a namespace you can just use `web` or `web.default`.

---

## 3. Variations — the four Service types + Headless

### ClusterIP (default)
Internal-only. Most Services in your cluster are this.

### NodePort
Allocates a port (default 30000–32767) on **every node** that forwards to the Service.

```yaml
apiVersion: v1
kind: Service
metadata: { name: web }
spec:
  type: NodePort
  selector: { app: web }
  ports:
    - port: 80
      targetPort: http
      nodePort: 30080      # optional — let it auto-pick if you omit
```

`curl http://<any-node-ip>:30080` → reaches a pod. Useful for bare-metal labs or when you don't want a cloud LB. Crude in production — you have to know the node IPs and there's no health-checked single entry point.

### LoadBalancer
Asks the cloud provider for an external load balancer pointed at the NodePort. Works on EKS/GKE/AKS out of the box; on bare metal you need MetalLB.

```yaml
apiVersion: v1
kind: Service
metadata: { name: web }
spec:
  type: LoadBalancer
  selector: { app: web }
  ports:
    - port: 80
      targetPort: http
```

```bash
$ kubectl get svc web
NAME   TYPE           CLUSTER-IP      EXTERNAL-IP        PORT(S)        AGE
web    LoadBalancer   10.96.184.241   34.120.12.45       80:30943/TCP   90s
```

One LoadBalancer per Service gets expensive fast. In practice, big clusters use **one Ingress** (next module) that's behind one LoadBalancer.

### Headless Service (`clusterIP: None`)
No cluster IP. DNS returns the pod IPs **directly** as A records.

```yaml
apiVersion: v1
kind: Service
metadata: { name: db }
spec:
  clusterIP: None
  selector: { app: db }
  ports: [{ port: 5432 }]
```

```bash
$ kubectl run dig --rm -it --image=tutum/dnsutils -- dig +short db.default.svc.cluster.local
10.244.0.10
10.244.0.11
10.244.0.12
```

Why? Two reasons:
- **Clients want to do their own load balancing** (gRPC clients, database drivers with read-replicas).
- **StatefulSets need stable, addressable per-pod DNS** like `db-0.db.default.svc.cluster.local` — and that requires a Headless Service.

### ExternalName
A DNS-level alias for an external hostname. No selector, no proxying.

```yaml
apiVersion: v1
kind: Service
metadata: { name: stripe }
spec:
  type: ExternalName
  externalName: api.stripe.com
```

`curl stripe.default.svc.cluster.local` → CNAME to `api.stripe.com`. Useful for migrating off external services or hiding vendor URLs.

---

## 4. Practical application — a multi-port Service with session affinity

```yaml
apiVersion: v1
kind: Service
metadata:
  name: api
  labels: { app: api }
spec:
  type: ClusterIP
  selector: { app: api }
  sessionAffinity: ClientIP
  sessionAffinityConfig:
    clientIP: { timeoutSeconds: 600 }
  ports:
    - name: http
      port: 80
      targetPort: http
    - name: metrics
      port: 9090
      targetPort: metrics
```

```bash
$ kubectl apply -f api-svc.yaml
service/api created

$ kubectl get svc api
NAME   TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)            AGE
api    ClusterIP   10.96.32.18     <none>        80/TCP,9090/TCP    5s

# Same client IP → same backend pod (within 10 min)
$ kubectl exec shell -- sh -c 'for i in 1 2 3; do curl -s api/whoami; done'
api-7c8d5f9b6-2k9zp
api-7c8d5f9b6-2k9zp
api-7c8d5f9b6-2k9zp
```

### `internalTrafficPolicy` and `externalTrafficPolicy`

- `internalTrafficPolicy: Local` — traffic from a pod stays on its node if a backend exists locally. Reduces cross-AZ traffic costs in cloud.
- `externalTrafficPolicy: Local` — external traffic (NodePort/LoadBalancer) is only sent to pods on the same node it arrived at. **Preserves the client source IP** (otherwise SNAT'd) but uneven load.

```yaml
spec:
  externalTrafficPolicy: Local
```

Without it, an LB sending traffic to a node that has no pod would SNAT and forward to another node — extra hop + losing client IP. With it, the node returns failure if it has no local pod and the LB skips it (health-check-based).

### Verifying Endpoint registration

```bash
$ kubectl get endpointslices -l kubernetes.io/service-name=api
NAME        ADDRESSTYPE   PORTS   ENDPOINTS                          AGE
api-xy7z3   IPv4          80,9090 10.244.0.20,10.244.0.21,10.244.0.22   3m

$ kubectl describe endpointslice api-xy7z3
Addresses:    10.244.0.20
Conditions:
  Ready: true
  Serving: true
  Terminating: false
```

If a pod becomes NotReady, it appears with `Ready: false` and kube-proxy stops sending traffic to it.

---

## 5. Common Mistakes & Gotchas

- **Selector that doesn't match any pod.** Service still gets a ClusterIP but no Endpoints. Requests time out. `kubectl get endpoints <svc>` showing nothing is the signal — check labels.
- **`targetPort` is a name but no container port has that name.** Service has Endpoints but DNAT goes nowhere. Always name container ports (`name: http`) and reference by name; renaming the port number then doesn't break the Service.
- **Using NodePort in production without thinking.** Means whoever connects must know all the node IPs. Use LoadBalancer or Ingress.
- **Expecting source IP preservation by default.** Cluster does SNAT through kube-proxy. Set `externalTrafficPolicy: Local` if you need the real client IP — and accept the load-balancing tradeoff.
- **`sessionAffinity: ClientIP` to "fix" stateful apps.** It's a workaround, not a solution. Real sticky sessions need a real cookie/header-based router (Ingress with affinity, service mesh).
- **Long-lived gRPC connections + scale-down.** A gRPC client opens a TCP connection to one pod (because L4 LB picks once at connect time). That pod stays loaded; new pods stay idle. Use Headless + client-side LB, or use a service mesh / L7 proxy.
- **One LoadBalancer per microservice.** $$$. Most teams put one Ingress in front and route by hostname/path.
- **Cluster IP collisions / range exhaustion.** Cluster has a fixed Service CIDR (`--service-cluster-ip-range`). Run out → no new Services. Sized at cluster creation; resizing is painful.
- **kube-proxy mode mismatch across nodes.** If some nodes are iptables and others IPVS, behavior diverges. Standardize.
- **Stale iptables on a busy node.** Very rarely, kube-proxy can lag on a node under pressure. `journalctl -u kube-proxy` and the `kubeproxy_sync_proxy_rules_duration` metric will tell you.

---

## 🎯 Key Takeaways

- **A Service is a kernel-level forwarding rule, not a process.** No proxy daemon sits in the data path (in iptables mode); it's just DNAT. Latency overhead is microseconds. This is why K8s networking can scale.
- **Endpoints are derived from labels + Pod readiness, not from the Service spec.** Get labels and readiness probes right and Services "just work."
- **Headless + StatefulSet is how you address individual pods by name.** This unlocks databases, queues, and anything needing stable per-instance identity.
- **`externalTrafficPolicy: Local` is the lever for source-IP preservation** — required by some auth flows, WAFs, and rate limits. Senior engineers always check this when an app suddenly sees "all traffic from one IP."
- **One LoadBalancer per Service doesn't scale economically.** The mature pattern is one Ingress (next module) or one Gateway in front of all your HTTP services.

*← [prev](./03_deployments_and_rollouts.md) | [next →](./05_ingress_and_gateway_api.md)*
