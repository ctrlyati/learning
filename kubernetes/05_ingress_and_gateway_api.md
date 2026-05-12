# 05 — Ingress & Gateway API

> **Goal:** Understand the difference between an Ingress resource and an ingress controller, write working Ingress YAML, see how nginx/traefik consume those resources, and know why the Gateway API is replacing Ingress.

---

## 1. Ingress — analogy + working YAML

**Analogy.** A Service is a phone extension; Ingress is the **office switchboard**. One public phone number, many internal extensions; the switchboard reads who you asked for ("Sales, please") and routes you. Ingress is HTTP-layer (L7) routing: it reads hostnames and URL paths to decide which Service receives the request, terminates TLS, and rewrites headers.

Crucially, **the Ingress object is just configuration** — a piece of declarative YAML. The thing that *actually* moves bytes is an **ingress controller** (nginx, traefik, HAProxy, etc.) running as a Deployment in the cluster. No controller installed → your Ingress YAML does nothing.

### Minimal Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
spec:
  ingressClassName: nginx     # which controller should handle this
  rules:
    - host: app.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: web
                port:
                  number: 80
```

```bash
$ kubectl apply -f ingress.yaml
ingress.networking.k8s.io/web created

$ kubectl get ingress
NAME   CLASS   HOSTS             ADDRESS         PORTS   AGE
web    nginx   app.example.com   34.120.12.45    80      12s

$ curl -H "Host: app.example.com" http://34.120.12.45
<!DOCTYPE html>
<html>...
```

`ADDRESS` is populated by the controller after it provisions itself (often a cloud LB). `CLASS` is the IngressClass — the routing target.

---

## 2. Mechanism — how a controller turns Ingress YAML into a running proxy

An ingress controller is a normal Deployment + Service that:

1. **Watches the apiserver** for `Ingress` and `IngressClass` resources.
2. On every change, **regenerates its proxy config** (an `nginx.conf`, a Traefik configuration, etc.).
3. **Reloads itself** to pick up the new config.
4. Continues serving the existing connections during reload (the good controllers, anyway).

The controller's own Service is typically `type: LoadBalancer` — one cloud LB serves all your HTTP traffic. That's the cost savings vs. one LB per Service.

### IngressClass

```yaml
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: nginx
spec:
  controller: k8s.io/ingress-nginx
```

Multiple controllers can coexist in a cluster (`nginx` for internal traffic, `nginx-external` for public, etc.). Each has its own IngressClass; each Ingress picks one via `spec.ingressClassName`.

The annotation `kubernetes.io/ingress.class` is the legacy way to pin to a controller. It still works but is deprecated; use the field.

### Path matching semantics

| `pathType` | Matches |
|------------|---------|
| `Exact`    | `/foo` matches `/foo` only |
| `Prefix`   | `/foo` matches `/foo`, `/foo/`, `/foo/bar` (path-element-aware — does NOT match `/foobar`) |
| `ImplementationSpecific` | Controller decides. Avoid. |

### Installing an ingress controller (ingress-nginx)

```bash
$ kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.10.0/deploy/static/provider/cloud/deploy.yaml
$ kubectl -n ingress-nginx get pods
NAME                                        READY   STATUS    RESTARTS   AGE
ingress-nginx-controller-7f6c98d4b7-h4x29   1/1     Running   0          45s

$ kubectl -n ingress-nginx get svc
NAME                       TYPE           CLUSTER-IP     EXTERNAL-IP     PORT(S)
ingress-nginx-controller   LoadBalancer   10.96.45.12    34.120.12.45    80:31044/TCP,443:32109/TCP
```

For kind, use `provider/kind/deploy.yaml` instead (NodePort).

---

## 3. Variations — controllers, TLS, rewrites, the Gateway API

### Popular controllers
| Controller | Notes |
|-----------|-------|
| **ingress-nginx** | Kubernetes-project-maintained. Most common. Annotation-heavy. |
| **nginx-ingress** (NGINX Inc.) | Different project, similar name. Commercial features available. |
| **Traefik** | Dynamic config, no reload required, easy autoconfiguration. Loved by Docker-Compose veterans. |
| **HAProxy Ingress** | If your org already lives on HAProxy. |
| **Contour** | Envoy-based. Strong Gateway API support. |
| **Istio / Linkerd gateways** | Service-mesh gateways; usually configured via Gateway API or mesh-specific CRDs, not Ingress. |
| **Cloud-native** (AWS ALB Controller, GKE GCLB) | Map Ingress directly to cloud LB rules — no in-cluster proxy. |

### TLS termination

```yaml
spec:
  tls:
    - hosts: [app.example.com]
      secretName: app-tls         # a Secret of type kubernetes.io/tls
  rules:
    - host: app.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend: { service: { name: web, port: { number: 80 } } }
```

The Secret must have `tls.crt` and `tls.key`. Use **cert-manager** to auto-issue from Let's Encrypt — almost universal in modern clusters.

### Annotations — the dark side of Ingress

Out-of-the-box Ingress is intentionally minimal. Anything fancier (rewrites, rate limits, auth, WebSocket, gRPC, custom timeouts) is done via **controller-specific annotations**:

```yaml
metadata:
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /$2
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/proxy-body-size: 50m
    nginx.ingress.kubernetes.io/rate-limit-rps: "100"
```

These are **not portable** across controllers. Switching from nginx to Traefik means rewriting all annotations. This is the biggest pain point of Ingress, and the reason the Gateway API exists.

### The Gateway API — Ingress, properly designed

Ingress was bolted onto K8s in 2015 and stayed minimal. The Gateway API (GA in 1.30) is a from-scratch redesign with three resource layers:

| Resource | Owned by | Role |
|----------|---------|------|
| **GatewayClass** | Infra team / vendor | Declares "this is a controller you can use" (analogous to StorageClass) |
| **Gateway** | Platform team | Provisions a listener (IP, ports, TLS). One per cluster or per tenant. |
| **HTTPRoute** (and `GRPCRoute`, `TCPRoute`, `TLSRoute`) | App team | Routing rules attached to a Gateway. |

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata: { name: prod-gateway }
spec:
  gatewayClassName: nginx
  listeners:
    - name: https
      port: 443
      protocol: HTTPS
      tls:
        certificateRefs: [{ name: app-tls }]
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata: { name: web }
spec:
  parentRefs: [{ name: prod-gateway }]
  hostnames: [app.example.com]
  rules:
    - matches: [{ path: { type: PathPrefix, value: / } }]
      backendRefs:
        - { name: web, port: 80, weight: 90 }
        - { name: web-canary, port: 80, weight: 10 }
```

Two things to notice:
1. **Traffic splitting is built in.** That `weight` is standard, not an annotation.
2. **Separation of concerns.** Platform owns the Gateway; app teams own the HTTPRoutes pointing at it. RBAC can enforce this.

Gateway API is supported by Contour, Istio, Cilium, ingress-nginx (limited), Kong, Envoy Gateway, and others. New clusters in 2026 should default to it.

---

## 4. Practical application — host + path routing, TLS, canary

The realistic shape of a production setup:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app
  annotations:
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  ingressClassName: nginx
  tls:
    - hosts: [app.example.com, api.example.com]
      secretName: app-tls
  rules:
    - host: app.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend: { service: { name: web,    port: { number: 80 } } }
    - host: api.example.com
      http:
        paths:
          - path: /v1
            pathType: Prefix
            backend: { service: { name: api-v1, port: { number: 80 } } }
          - path: /v2
            pathType: Prefix
            backend: { service: { name: api-v2, port: { number: 80 } } }
```

```bash
$ kubectl apply -f app-ingress.yaml
ingress.networking.k8s.io/app created

$ kubectl get ingress app
NAME   CLASS   HOSTS                              ADDRESS         PORTS     AGE
app    nginx   app.example.com,api.example.com    34.120.12.45    80, 443   30s

$ kubectl describe ingress app | grep -A3 Events
Events:
  Type    Reason  Age   From                      Message
  Normal  Sync    15s   nginx-ingress-controller  Scheduled for sync

$ curl -sI https://app.example.com | head -1
HTTP/2 200
```

### Canary with ingress-nginx annotations (pre-Gateway-API pattern)

Two Ingresses pointing at the same host, with a "canary" flag on the secondary:

```yaml
# primary
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata: { name: web }
spec:
  ingressClassName: nginx
  rules:
    - host: app.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend: { service: { name: web, port: { number: 80 } } }
---
# 10% canary
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web-canary
  annotations:
    nginx.ingress.kubernetes.io/canary: "true"
    nginx.ingress.kubernetes.io/canary-weight: "10"
spec:
  ingressClassName: nginx
  rules:
    - host: app.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend: { service: { name: web-canary, port: { number: 80 } } }
```

10% of traffic goes to `web-canary`. This is exactly the kind of thing that's a first-class `weight: 10` in Gateway API.

---

## 5. Common Mistakes & Gotchas

- **Ingress with no controller installed.** YAML applies, `ADDRESS` stays empty forever. `kubectl get pods -A | grep ingress` to check.
- **`ingressClassName` omitted with multiple controllers.** Either no controller picks it up, or both do. Always set it.
- **Path matching surprises.** `Prefix /foo` does not match `/foobar`, but `ImplementationSpecific` might. Use `Exact` or `Prefix` and avoid surprises.
- **Annotations on the wrong resource.** Some controller annotations go on the Service, some on the Ingress. Check the controller's docs.
- **Annotations from a different controller's docs.** Copy-pasting `traefik.ingress.kubernetes.io/...` onto an nginx Ingress does nothing. Painfully easy to miss.
- **TLS Secret in the wrong namespace.** Ingress can only reference Secrets in **its own namespace**. Cross-namespace TLS isn't supported in core Ingress (it is in Gateway API via `ReferenceGrant`).
- **Forgetting `ssl-redirect`.** Plain HTTP works, but you also want it to upgrade. Most controllers default to redirect; some don't.
- **Long-running connections + controller reloads.** Each reload may interrupt connections briefly. ingress-nginx >1.0 mitigates this with Lua + dynamic upstreams. Worth knowing.
- **WebSocket / gRPC.** Both need controller-specific tuning (headers, timeouts, h2 enabled). Plain Ingress YAML isn't enough.
- **Putting auth into Ingress annotations.** Works, but couples auth to ingress controller. Service mesh or an auth proxy is more portable.
- **Mixing Gateway API + Ingress for the same hostname.** Two routing systems disagreeing on the same `app.example.com` is a long debugging session. Pick one per host.

---

## 🎯 Key Takeaways

- **Ingress is *data*; the controller is the *code*.** This decoupling is what lets you swap nginx for Traefik for AWS ALB without changing app manifests... in theory. Annotations break the abstraction in practice.
- **One LoadBalancer + one Ingress controller is the standard production pattern.** Per-service LoadBalancers are a smell.
- **TLS via cert-manager + Let's Encrypt is essentially free and table stakes.** A senior engineer expects to see it within an hour of joining a new platform team.
- **The Gateway API is the future.** New clusters should adopt it; legacy clusters can run both side-by-side during migration. Vendor-neutral, role-separated, and traffic-splitting is first-class.
- **Most "weird ingress bugs" are actually controller-specific annotation bugs.** When debugging, read the *controller's* docs (and its access/error logs), not just the Ingress spec.

*← [prev](./04_services_and_kube_proxy.md) | [next →](./06_configmaps_and_secrets.md)*
