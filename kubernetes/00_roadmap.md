# 00 — Kubernetes Deep-Dive Roadmap

> **Goal:** Take a working developer from "I've heard of pods" to "I can design, ship, and operate production workloads on Kubernetes" — with the depth a senior platform engineer expects.

Kubernetes is dense. It's not one tool; it's a distributed system, an API server, a controller framework, a networking spec, a storage spec, and a packaging ecosystem all bolted together. The goal of this course is to make every layer legible — not just "how to write a Deployment YAML", but **why** the apiserver, scheduler, and controllers behave the way they do, what happens when things go wrong, and how teams actually run this in production.

---

## Module Table

| #  | File                                  | Topic                                              | Why it matters |
|----|---------------------------------------|----------------------------------------------------|----------------|
| 00 | `00_roadmap.md`                       | This file — orientation, mental models             | The map        |
| 01 | `01_intro_and_setup.md`               | What K8s is/isn't, control plane, local clusters   | Foundation     |
| 02 | `02_pods_and_containers.md`           | Pods, multi-container patterns, lifecycle          | The atom       |
| 03 | `03_deployments_and_rollouts.md`      | Deployments, ReplicaSets, rolling updates          | How you ship   |
| 04 | `04_services_and_kube_proxy.md`       | Services, kube-proxy, endpoints                    | How traffic flows in |
| 05 | `05_ingress_and_gateway_api.md`       | Ingress, controllers, Gateway API                  | How traffic flows from outside |
| 06 | `06_configmaps_and_secrets.md`        | Config, secrets, external secret stores            | The 12-factor parts |
| 07 | `07_volumes_and_storage.md`           | PV, PVC, StorageClass, CSI                         | State           |
| 08 | `08_statefulsets_jobs_daemonsets.md`  | StatefulSet, DaemonSet, Job, CronJob               | Beyond Deployment |
| 09 | `09_namespaces_labels_selectors.md`   | Org model, labels as the join key                  | How you carve up a cluster |
| 10 | `10_rbac_and_serviceaccounts.md`      | Roles, Bindings, SAs, auth flow                    | Who can do what |
| 11 | `11_networking_cni_dns_netpol.md`     | CNI, pod networking, NetworkPolicy, DNS            | The plumbing    |
| 12 | `12_scheduling_and_affinity.md`       | Requests/limits, affinity, taints, topology        | Where pods land |
| 13 | `13_autoscaling.md`                   | HPA, VPA, Cluster Autoscaler, KEDA                 | Elasticity      |
| 14 | `14_observability.md`                 | Logs, metrics, Prometheus, Grafana, events         | Day-2 sanity    |
| 15 | `15_helm_and_kustomize.md`            | Helm, Kustomize, packaging                         | How you reuse   |
| 16 | `16_operators_and_crds.md`            | CRDs, controllers, Operator pattern                | Extending K8s   |
| 17 | `17_security.md`                      | PSS, image security, admission, supply chain       | The blast radius |
| 18 | `18_production_kubernetes.md`         | EKS/GKE/AKS, GitOps, DR, day-2 ops                 | The real job    |

---

## Timeline

Kubernetes rewards spaced repetition. Plan for **3–5 weeks** of part-time study (one module per 1–2 days) or about **two focused weeks** full-time. Don't speedrun modules 2–4 — pods, deployments, and services together are about 60% of what you'll use daily.

| Week | Modules           | Theme                                       |
|------|-------------------|---------------------------------------------|
| 1    | 01–05             | Core workload + traffic surface             |
| 2    | 06–10             | Config, storage, workload variety, org/RBAC |
| 3    | 11–14             | Networking, scheduling, scale, observability|
| 4    | 15–18             | Packaging, extension, security, prod ops    |
| 5    | (buffer)          | Build something end-to-end                  |

Each module ends with **Key Takeaways** through a professional-upskilling lens: what a senior platform engineer would actually notice or push back on.

---

## Prerequisites

- **Docker basics** — images, containers, registries, `docker run`, `Dockerfile`. See [`../docker/00_roadmap.md`](../docker/00_roadmap.md) if you need a refresher.
- **YAML comfort** — indentation matters, lists vs maps, `---` separators.
- **Linux basics** — processes, signals, file descriptors, `ps`, `top`, `curl`, `dig`.
- **Networking basics** — IPs, ports, DNS, TCP vs UDP, what a reverse proxy does, what NAT means.
- **Some HTTP** — you'll talk to the kube-apiserver over HTTP/JSON whether you realize it or not.

You don't need to know Go, distributed systems theory, or any specific cloud. Those help, but the course doesn't assume them.

---

## Core Mental Models

Internalize these six things and 80% of K8s stops being surprising.

### 1. Declarative state + reconciliation loops
You don't tell Kubernetes "do this." You tell it "**this is what should be true**" by writing to the apiserver. Controllers run forever in a loop: *observe current state → compare to desired state → act to close the gap.* The Deployment controller, the ReplicaSet controller, the Node controller, the kubelet — they're all doing this. If you understand this single pattern, you understand K8s.

### 2. Everything is a resource
Pods, Services, ConfigMaps, Nodes, even Events and RBAC rules — all of them are records in `etcd`, exposed by the kube-apiserver as a REST resource. `kubectl get X` is `GET /api/v1/X`. This uniformity is why CRDs (custom resources) work: you just add new types to the same API server.

### 3. Labels are the universal join key
Services don't reference Pods by name. ReplicaSets don't reference Pods by name. NetworkPolicies don't reference Pods by name. They all use **label selectors**. Labels are how loose coupling is implemented inside the cluster. Get the labeling discipline right and most other things click.

### 4. Controllers all the way down
The whole system is small controllers stacked on top of each other. A Deployment makes a ReplicaSet, which makes Pods, which the scheduler binds to Nodes, which the kubelet then runs. No central "orchestrator" thread — just lots of loops watching the same API. This is also why K8s is **extensible**: you write your own controller, and it composes naturally.

### 5. The cluster is eventually consistent
You apply a YAML. The apiserver writes to etcd. Controllers notice. Things happen. **None of this is synchronous.** `kubectl apply` returning success does not mean your pod is running — it means the desired state was accepted. Always check actual state (`kubectl get`, `describe`, events) before concluding something is broken.

### 6. `kubectl` is just an HTTP client
`kubectl get pods` becomes an HTTP request to the apiserver authenticated with your kubeconfig. There's nothing magic. `kubectl --v=8 get pods` shows the raw HTTP. Once you see this, debugging auth, RBAC, and "why is my CI not deploying" gets dramatically easier.

---

## External Resources

- **[kubernetes.io/docs](https://kubernetes.io/docs/)** — the canonical reference. Always check it before Stack Overflow.
- **[kubectl cheat sheet](https://kubernetes.io/docs/reference/kubectl/cheatsheet/)** — bookmark this; you'll come back daily.
- **[learnk8s.io](https://learnk8s.io/)** — long-form, deep articles on networking, scheduling, autoscaling. Excellent.
- **[CNCF / KubeAcademy](https://www.cncf.io/training/)** — free vendor-neutral training tracks.
- **"Kubernetes Up & Running" (Hightower, Burns, Beda)** — still the best book for getting fluent. Chapters age well.
- **[This Week in Kubernetes](https://thisweekinkubernetes.com/)** + the [Kubernetes Podcast](https://kubernetespodcast.com/) — keep pace with the moving ecosystem.

---

## Closing

By the end of this course you should be able to walk into any team running Kubernetes and be useful on day one — read their manifests, debug their pods, reason about their traffic, and have informed opinions on their cluster choices. That's the upskilling bar. Let's go.

*[next → 01_intro_and_setup](./01_intro_and_setup.md)*
