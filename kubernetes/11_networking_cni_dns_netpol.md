# 11 — Networking: CNI, DNS & NetworkPolicy

> **Goal:** Understand the Kubernetes networking model (every pod gets a unique IP, all pods can reach all pods), what a CNI plugin does, how CoreDNS is wired in, and how NetworkPolicies actually firewall pods.

---

## 1. The networking model — analogy + the four invariants

**Analogy.** Imagine the cluster as a **city with a flat phone book**. Every resident (Pod) gets a unique phone number (IP). Anyone can dial anyone (no NAT between pods). The phone company (CNI plugin) figures out the cables and routes. The phone book (CoreDNS) is how names become numbers. The city ordinances (NetworkPolicies) decide who's allowed to dial whom.

The Kubernetes networking model has **four hard invariants**:

1. Every Pod gets its own IP address.
2. Pods on a node can reach all Pods in the cluster **without NAT**.
3. Agents on a node (kubelet, kube-proxy) can reach all Pods on that node.
4. Pods in `hostNetwork: true` see the node's network namespace directly.

Kubernetes doesn't implement networking itself — it delegates to a **CNI plugin** that must satisfy these rules. There are dozens of plugins (Calico, Cilium, Flannel, Weave, AWS VPC CNI, Azure CNI, GKE's native, etc.). The choice has big implications: performance, NetworkPolicy support, IP exhaustion handling, dual-stack, eBPF features.

```bash
$ kubectl get pods -n kube-system -o wide | grep -i cni
calico-node-h4x29        1/1     Running   0   3d   172.18.0.2   learn-control-plane
calico-kube-controllers  1/1     Running   0   3d   10.244.0.5   learn-control-plane
```

---

## 2. Mechanism — what a CNI plugin actually does

When the kubelet creates a Pod:

1. It creates a **network namespace** for the Pod (Linux primitive).
2. It calls the configured CNI plugin (a binary in `/opt/cni/bin/`) with an `ADD` command.
3. The plugin:
   - Allocates an IP from its pool (IPAM).
   - Creates a **veth pair** — one end in the pod's namespace, one on the host.
   - Programs routes (or eBPF maps, or BGP advertisements) so packets to that IP find the right node.
4. Returns the IP to the kubelet, which sets `pod.status.podIP`.

On Pod delete, the plugin gets `DEL` and cleans up.

### How packets actually flow

| CNI approach | How |
|--------------|-----|
| **Overlay** (Flannel VXLAN, Calico IP-in-IP) | Pod IP packets are wrapped in another packet between nodes. Works on any underlying network. Small overhead. |
| **Underlay/Routed** (Calico BGP, Cilium native routing, AWS VPC CNI) | Pod IPs are routable directly on the underlying network. No encapsulation. Faster, requires network cooperation. |
| **eBPF** (Cilium) | Replaces iptables with kernel programs for routing, policy, and service load balancing. Lower latency at scale. |

### CoreDNS — cluster DNS

Every cluster runs **CoreDNS** (formerly kube-dns) as a Deployment in `kube-system`. The kubelet wires every pod's `/etc/resolv.conf` to point at the CoreDNS Service IP.

```bash
$ kubectl run shell --rm -it --image=alpine -- sh
/ # cat /etc/resolv.conf
search default.svc.cluster.local svc.cluster.local cluster.local
nameserver 10.96.0.10
options ndots:5

/ # nslookup kubernetes
Name:    kubernetes.default.svc.cluster.local
Address: 10.96.0.1
```

CoreDNS resolves:
- `<svc>.<ns>.svc.cluster.local` → Service's ClusterIP
- `<svc>.<ns>.svc.cluster.local` for Headless Service → list of pod IPs (A records)
- `<pod-ordinal>.<svc>.<ns>.svc.cluster.local` for StatefulSet pods → that pod's IP
- External names → forwarded upstream (configured in the CoreDNS ConfigMap)

The `ndots:5` in resolv.conf is famous: any name with fewer than 5 dots tries every search domain first. Means `kubernetes.default.svc.cluster.local.` (with trailing dot) is one lookup; `google.com` is *six* (try `google.com.default.svc.cluster.local`, fail, etc.). Apps doing tons of DNS to external domains slow down — common reason to either fully-qualify external lookups or lower `ndots`.

### kube-proxy's role in this stack
We covered it in module 4 — kube-proxy isn't a CNI plugin; it programs Service VIPs *on top* of whatever the CNI gave you. CNI handles pod-to-pod; kube-proxy handles pod-to-service.

---

## 3. NetworkPolicy — variations and depth

By default, **all traffic between pods is allowed**. NetworkPolicy is how you lock it down.

### Default-deny ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny
  namespace: payments
spec:
  podSelector: {}            # all pods in this namespace
  policyTypes: [Ingress]
```

With this in place, no pod in `payments` can receive traffic from anyone unless another policy explicitly allows it.

### Allow specific traffic

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: api-can-talk-to-db
  namespace: payments
spec:
  podSelector:
    matchLabels: { app: db }
  policyTypes: [Ingress]
  ingress:
    - from:
        - podSelector:
            matchLabels: { app: api }
      ports:
        - protocol: TCP
          port: 5432
```

Now `db` pods only accept TCP/5432 from `app=api` pods *in the same namespace*.

### Cross-namespace allows

```yaml
ingress:
  - from:
      - namespaceSelector:
          matchLabels: { team: payments }
        podSelector:
          matchLabels: { app: api }
```

The `namespaceSelector` matches against **namespace labels** — make sure you've labeled your namespaces (`kubernetes.io/metadata.name` is auto-applied by recent K8s).

### Egress policies

```yaml
spec:
  podSelector: { matchLabels: { app: api } }
  policyTypes: [Egress]
  egress:
    - to:
        - podSelector: { matchLabels: { app: db } }
      ports: [{ protocol: TCP, port: 5432 }]
    - to:
        - namespaceSelector: { matchLabels: { kubernetes.io/metadata.name: kube-system } }
          podSelector: { matchLabels: { k8s-app: kube-dns } }
      ports: [{ protocol: UDP, port: 53 }]
```

**Don't forget DNS** when writing egress policies. If you only allow egress to `db`, pods can't resolve `db` because DNS to CoreDNS is blocked. Always include UDP/53 (and TCP/53) to the kube-dns SA.

### Important caveat — NetworkPolicy requires CNI support

The NetworkPolicy *resource* exists in every cluster. But its rules are only enforced if the CNI plugin implements policy. **Flannel does not**. **Calico, Cilium, Weave, and AWS VPC CNI (with calico add-on) do**. Always check.

```bash
$ kubectl apply -f netpol.yaml         # this succeeds either way
$ # ...but only blocks anything if your CNI enforces it.
```

### What NetworkPolicy can't do
- **L7 rules** (block specific HTTP paths, methods). Need Cilium NetworkPolicy CRD, Istio AuthorizationPolicy, or a service mesh.
- **Reference IPs by hostname.** Egress rules use IP CIDRs, not DNS. Cilium has FQDN policies; vanilla NP doesn't.
- **Stop a pod with `hostNetwork: true`** from doing whatever the node can do.

---

## 4. Practical application — zero-trust namespace

Lock down `payments` so:
- No traffic in or out by default.
- `api` pods can receive HTTP from the `ingress-nginx` namespace.
- `api` pods can reach `db` on 5432.
- All pods can reach DNS.
- `api` pods can reach the internet (e.g., Stripe API).

```yaml
# 1. Default-deny everything
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: { name: default-deny, namespace: payments }
spec:
  podSelector: {}
  policyTypes: [Ingress, Egress]
---
# 2. Allow DNS for everyone
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: { name: allow-dns, namespace: payments }
spec:
  podSelector: {}
  policyTypes: [Egress]
  egress:
    - to:
        - namespaceSelector: { matchLabels: { kubernetes.io/metadata.name: kube-system } }
      ports:
        - { protocol: UDP, port: 53 }
        - { protocol: TCP, port: 53 }
---
# 3. Allow inbound HTTP to api from ingress-nginx
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: { name: api-ingress, namespace: payments }
spec:
  podSelector: { matchLabels: { app: api } }
  policyTypes: [Ingress]
  ingress:
    - from:
        - namespaceSelector: { matchLabels: { kubernetes.io/metadata.name: ingress-nginx } }
      ports: [{ protocol: TCP, port: 8080 }]
---
# 4. Allow api → db
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: { name: api-to-db, namespace: payments }
spec:
  podSelector: { matchLabels: { app: db } }
  policyTypes: [Ingress]
  ingress:
    - from:
        - podSelector: { matchLabels: { app: api } }
      ports: [{ protocol: TCP, port: 5432 }]
---
# 5. Allow api egress to db AND internet
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: { name: api-egress, namespace: payments }
spec:
  podSelector: { matchLabels: { app: api } }
  policyTypes: [Egress]
  egress:
    - to:
        - podSelector: { matchLabels: { app: db } }
      ports: [{ protocol: TCP, port: 5432 }]
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
            except: [10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16]   # internet, not RFC1918
      ports: [{ protocol: TCP, port: 443 }]
```

### Verifying

```bash
$ kubectl apply -f netpol.yaml
$ kubectl get netpol -n payments
NAME          POD-SELECTOR   AGE
default-deny  <none>         3m
allow-dns     <none>         3m
api-ingress   app=api        3m
api-to-db     app=db         3m
api-egress    app=api        3m

# From inside an api pod
$ kubectl exec -n payments deploy/api -- nc -zv db 5432
db (10.244.0.20) 5432 open

$ kubectl exec -n payments deploy/api -- nc -zv api.stripe.com 443
api.stripe.com (54.187.x.x) 443 open

# From an unrelated pod in another namespace — should be blocked
$ kubectl exec -n default shell -- nc -zv db.payments 5432
nc: db.payments (10.244.0.20:5432): Connection timed out
```

### Debugging tools

- **`kubectl exec pod -- nc/curl/wget`** — quick connectivity test.
- **Cilium `cilium connectivity test`** — full mesh test if Cilium is the CNI.
- **Calico `calicoctl`** — view what policies apply to which pod.
- **`kubectl get networkpolicy -A`** — start here.
- **CNI logs** — `kubectl -n kube-system logs ds/calico-node` etc.

---

## 5. Common Mistakes & Gotchas

- **Writing a deny-only policy.** Default-deny is the *absence* of allow rules. There's no `Deny` action in NetworkPolicy. The mental shift to "allow-list everything" trips up firewall veterans.
- **Forgetting DNS in egress rules.** Pods can't resolve anything. App logs full of "name resolution failure." Always whitelist UDP/53 to kube-dns.
- **NetworkPolicy applied but doing nothing.** Your CNI doesn't enforce. Flannel is the common culprit. Switch CNI or layer Calico on top.
- **Namespace selector matching by label that doesn't exist.** Recent K8s auto-adds `kubernetes.io/metadata.name`. Old clusters may not. Apply labels explicitly.
- **Policy targeting a pod that doesn't exist yet.** Policy is fine; it'll start working when matching pods appear. Beware: if the pod's labels change, the policy stops matching.
- **`hostNetwork: true` pod ignoring policy.** Correct — it's not in a pod network namespace. Audit privileged pods carefully.
- **L7 expectations.** NetworkPolicy is L3/L4. "Allow only GET requests to /healthz" is a mesh/Cilium-NetworkPolicy job.
- **One CIDR for "internet."** Easy to forget your own cluster ranges, ending up with policies that allow pod-to-pod traffic through the "0.0.0.0/0" hole. Use `except:` carefully.
- **CoreDNS scale issues.** Default replicas=2; in big clusters with chatty apps, CoreDNS becomes a bottleneck. Enable autoscaler, or use NodeLocal DNSCache.
- **`ndots:5` slowness for external lookups.** App makes 1000 requests/sec to `api.example.com` → 5x more DNS queries than needed. Set `dnsConfig.options[].ndots=1` in pod spec or fully-qualify domain names.
- **Stale CNI state after node reboot.** Rare, but possible. Symptom: pods scheduled but can't reach anything. Restart kubelet + CNI daemon on the node.
- **IP exhaustion in AWS VPC CNI.** Pod IPs come from the VPC; node has fixed ENIs/IPs. Big nodes with many small pods can starve. Tune `WARM_IP_TARGET` or switch to prefix delegation.

---

## 🎯 Key Takeaways

- **The model is "every pod gets a real, routable IP" — and the CNI's job is to make that true.** Internalize this and most networking weirdness reduces to "which CNI?" + "did it succeed?"
- **CoreDNS is your dependency.** When DNS gets weird (`ndots`, NXDOMAIN, latency), apps look broken. Always check kube-system before blaming the app.
- **NetworkPolicy is allow-list, and DNS is the egress rule everyone forgets.** Senior engineers spot the missing DNS allow in a PR within seconds.
- **CNI choice has long-term consequences.** Flannel works for labs but limits you (no policy enforcement). Calico/Cilium are the safer production choices; Cilium is the eBPF future.
- **Zero-trust networking inside a cluster is real.** It just costs some YAML. Pair NetworkPolicy with RBAC and you've moved well beyond "everything trusts everything."

*← [prev](./10_rbac_and_serviceaccounts.md) | [next →](./12_scheduling_and_affinity.md)*
