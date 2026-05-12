# 01 — Introduction & Setup

> **Goal:** Get a mental model of what Kubernetes is (and is *not*), understand the control plane / node split, spin up a local cluster, and run your first commands with `kubectl`.

---

## 1. What Kubernetes Is (and Isn't) — analogy + first commands

**Analogy.** Think of Kubernetes as the **operations team for a container fleet**. You hand the ops team a stack of forms ("I need 3 instances of this app, with this much memory, behind this URL, and please keep it that way"). The ops team — which never sleeps and is allergic to manual work — figures out which machines to run it on, restarts crashed instances, replaces machines that died, and adjusts when you change the forms. The forms are YAML. The ops team is a set of controllers. The clipboard is `etcd`.

**Kubernetes is:**
- A **container orchestrator** — it schedules and supervises containers across many machines.
- A **declarative API** — you describe desired state, controllers reconcile it.
- A **platform for platforms** — most teams build their internal developer platform *on top of* it.

**Kubernetes is NOT:**
- A PaaS like Heroku — there's no `git push` to deploy by default.
- A CI/CD system — it runs your apps, it doesn't build them.
- A service mesh — it has primitive networking; mesh features (mTLS, traffic shaping) come from Istio/Linkerd.
- A drop-in replacement for Docker Compose — the conceptual surface is much bigger.
- "Easy" — anyone who tells you it is, is selling something.

**First commands** (we'll set up the cluster in section 4, but here's where you're headed):

```bash
kubectl version --client          # what kubectl version you have
kubectl cluster-info              # is there a cluster behind this kubeconfig?
kubectl get nodes                 # what machines is the cluster running on?
kubectl get pods -A               # every pod, every namespace
kubectl run hello --image=nginx   # imperative — quick and dirty
```

---

## 2. Mechanism — control plane vs nodes, and what actually happens when you run `kubectl apply`

A Kubernetes cluster has two kinds of machines: **control plane nodes** (the brain) and **worker nodes** (where your containers run). In production they're separate; in your laptop kind/minikube cluster they're the same box.

### Control plane components

| Component | What it does |
|-----------|--------------|
| **kube-apiserver** | The only thing that talks to `etcd`. Every read/write goes through it. REST API over HTTPS. |
| **etcd** | The cluster's database. Strongly consistent key/value store. The "source of truth." |
| **kube-scheduler** | Watches for unscheduled Pods, picks a Node for each one based on resources, affinities, taints. |
| **kube-controller-manager** | Runs all the built-in controllers (Deployment, ReplicaSet, Node, ServiceAccount, etc.) as one process. |
| **cloud-controller-manager** | (Cloud clusters only.) Talks to the cloud provider — provisions load balancers, attaches disks. |

### Node components

| Component | What it does |
|-----------|--------------|
| **kubelet** | Agent on every node. Watches the apiserver for Pods bound to its node, then tells the container runtime to run them. |
| **kube-proxy** | Programs iptables/IPVS rules so Service IPs route to the right Pod IPs. |
| **container runtime** | containerd (most common today), CRI-O, etc. Actually creates Linux namespaces/cgroups and starts containers. |

### The lifecycle of `kubectl apply -f pod.yaml`

1. `kubectl` reads your kubeconfig, finds the apiserver URL and credentials.
2. `kubectl` POSTs the YAML (as JSON) to `/api/v1/namespaces/default/pods`.
3. **Authn** — apiserver verifies your identity (cert, token, OIDC).
4. **Authz** — RBAC check: are you allowed to create Pods here?
5. **Admission controllers** — mutating (e.g., inject defaults) then validating webhooks run.
6. **etcd write** — apiserver persists the Pod object with `spec` set, `status` empty.
7. **Scheduler watches** — sees a Pod with no `nodeName`, picks a Node, PATCHes the Pod to set `spec.nodeName`.
8. **Kubelet on that Node watches** — sees a Pod bound to it, pulls the image, calls containerd to start the container.
9. **Kubelet updates status** — `status.phase: Running`, `containerStatuses` filled in.
10. Your `kubectl get pod` reads `status` and shows you `Running`.

Every step is async. None of them block each other. This is why "I applied the YAML but nothing happened" usually means "I haven't waited 2 seconds yet" or "something blocked at step 5 (admission) or step 7 (scheduling)."

---

## 3. Local cluster options — kind vs minikube vs Docker Desktop vs k3d

| Tool | What it is | Best for |
|------|-----------|----------|
| **kind** | "Kubernetes IN Docker" — control plane and nodes are each a Docker container. Fast. | Day-to-day dev, CI pipelines, learning. **Recommended.** |
| **minikube** | Runs K8s in a VM (or Docker). Mature, lots of addons. | When you need addons (ingress, metrics-server) one command away. |
| **Docker Desktop** | One-click K8s built into Docker Desktop on Mac/Windows. | If you already have Docker Desktop and want zero setup. Limited customization. |
| **k3d** | Runs **k3s** (lightweight K8s distro) in Docker. Smallest footprint. | Resource-constrained laptops, edge-like experiments. |

For this course you only need **one**. `kind` is the default I'll show. Everything works on the others — manifests are portable; only the bootstrap differs.

### Installing kubectl

- **macOS:** `brew install kubectl`
- **Windows:** `winget install Kubernetes.kubectl` or `choco install kubernetes-cli`
- **Linux:** `curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && sudo install kubectl /usr/local/bin/`

Verify:
```bash
$ kubectl version --client
Client Version: v1.30.0
Kustomize Version: v5.0.4
```

### Installing kind

- **macOS:** `brew install kind`
- **Windows:** `winget install Kubernetes.kind`
- **Linux:** see [kind.sigs.k8s.io](https://kind.sigs.k8s.io/docs/user/quick-start/).

---

## 4. Practical application — create a cluster, deploy a real Pod, verify

```bash
# 1. Create a cluster (takes ~60s the first time)
$ kind create cluster --name learn
Creating cluster "learn" ...
 ✓ Ensuring node image (kindest/node:v1.30.0) ...
 ✓ Preparing nodes ...
 ✓ Writing configuration ...
 ✓ Starting control-plane ...
 ✓ Installing CNI ...
 ✓ Installing StorageClass ...
Set kubectl context to "kind-learn"
You can now use your cluster with:

kubectl cluster-info --context kind-learn
```

```bash
# 2. Confirm it's alive
$ kubectl get nodes
NAME                  STATUS   ROLES           AGE   VERSION
learn-control-plane   Ready    control-plane   45s   v1.30.0

$ kubectl get pods -A
NAMESPACE            NAME                                          READY   STATUS
kube-system          coredns-7db6d8ff4d-h9k82                      1/1     Running
kube-system          coredns-7db6d8ff4d-vfmnm                      1/1     Running
kube-system          etcd-learn-control-plane                      1/1     Running
kube-system          kube-apiserver-learn-control-plane            1/1     Running
kube-system          kube-controller-manager-learn-control-plane   1/1     Running
kube-system          kube-proxy-zb5wj                              1/1     Running
kube-system          kube-scheduler-learn-control-plane            1/1     Running
local-path-storage   local-path-provisioner-7577fdbbfb-pxctw       1/1     Running
```

Look at that pod list — the entire control plane is itself running as pods. **K8s runs K8s.** This is called *self-hosted control plane* and it's the convention.

### Your first manifest

Create `hello.yaml`:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: hello
  labels:
    app: hello
spec:
  containers:
    - name: web
      image: nginx:1.27-alpine
      ports:
        - containerPort: 80
```

Apply and verify:

```bash
$ kubectl apply -f hello.yaml
pod/hello created

$ kubectl get pod hello
NAME    READY   STATUS    RESTARTS   AGE
hello   1/1     Running   0          7s

$ kubectl describe pod hello | head -20
Name:             hello
Namespace:        default
Priority:         0
Service Account:  default
Node:             learn-control-plane/172.18.0.2
Start Time:       Mon, 13 May 2026 10:14:22 +0000
Labels:           app=hello
Status:           Running
IP:               10.244.0.5

$ kubectl exec hello -- curl -s localhost | head -3
<!DOCTYPE html>
<html>
<head>
```

Clean up:

```bash
kubectl delete -f hello.yaml          # delete this pod
kind delete cluster --name learn      # tear the whole thing down
```

### `kubectl` flags worth memorizing

```bash
kubectl get pods -o wide              # extra columns (node, IP)
kubectl get pods -o yaml              # full YAML of the resource
kubectl get pods -o json | jq .       # pipe to jq for queries
kubectl get pods -w                   # watch — stream changes
kubectl get pods -l app=hello         # filter by label
kubectl get pods -A                   # all namespaces
kubectl get pods --field-selector=status.phase=Running
kubectl logs hello -f                 # follow logs
kubectl exec -it hello -- sh          # shell inside container
kubectl explain pod.spec.containers   # built-in schema docs!
kubectl --v=8 get pods                # dump HTTP requests — magic gone
```

`kubectl explain` is criminally underused. It's offline docs for every field of every resource.

---

## 5. Common Mistakes & Gotchas

- **`kubectl` version skew.** Client should be within ±1 minor of the server. `kubectl version` shows both. A 1.25 client against a 1.30 server *might* work but corner cases break silently.
- **Multiple kubeconfigs / wrong context.** `kubectl config get-contexts` and `kubectl config current-context`. The most common production incident from new engineers is running a destructive command against the wrong cluster. Use `kubectl config use-context` deliberately, or install `kubectx`.
- **"It says Running but nothing works."** `Running` only means the pod started. The process inside might be crash-looping inside a single `Running` cycle, or the readiness probe might be failing. Always check `kubectl describe pod` and `kubectl logs`.
- **Forgetting the namespace.** `kubectl get pods` shows the *current* namespace only. If something's "missing," try `-A` first.
- **`apply` vs `create` vs `replace`.** Use `apply` always; it does a 3-way merge against the last-applied annotation. `create` fails if it exists. `replace` wipes server-side fields you didn't put in your file.
- **Imperative `kubectl run` in production.** Fine for experiments. Never in CI. Every production change should be a YAML file in Git.
- **YAML indentation.** Tabs are illegal. Two spaces. `kubectl apply` will give you a parser line number but not a friendly one — use an editor with YAML linting (`yamllint`, VS Code's YAML extension with the Kubernetes schema).

---

## 🎯 Key Takeaways

- **Kubernetes is declarative-by-API, not by tool.** Everything goes through the apiserver; `kubectl` is a thin client. Internalize this and you can debug from first principles.
- **The control plane is six processes you can name.** Apiserver, etcd, scheduler, controller-manager, kubelet, kube-proxy. If you can describe what each does, you've already cleared the bar most "K8s users" stop at.
- **Local clusters are real clusters.** kind/minikube run the same code path as EKS. Practice locally aggressively — it's free and fast.
- **Async by default.** A senior engineer never says "I applied it, it's broken" — they say "I applied it, the scheduler hasn't bound the pod yet, here's why."
- **`kubectl explain` and `--v=8` are the only secrets you need.** One gives you offline docs for every field; the other unmasks the API calls. New hires who learn these in week one move twice as fast.

*← [prev](./00_roadmap.md) | [next →](./02_pods_and_containers.md)*
