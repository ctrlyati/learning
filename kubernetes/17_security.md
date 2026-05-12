# 17 — Security

> **Goal:** Harden a cluster end-to-end. Master Pod Security Standards, image security & supply chain, admission controllers (OPA Gatekeeper, Kyverno), runtime security, and the principle of least privilege in Kubernetes' specific shape.

---

## 1. The threat model — analogy + the layers

**Analogy.** Think of the cluster as a **building with multiple security layers**: the perimeter (network), the lobby (apiserver authn), security checks at every floor entry (authz), what tools each person can carry (PSS, capabilities), what they bring with them (images, supply chain), and cameras + guards inside (runtime detection). Bypass any one layer and you don't necessarily own the building — but each is a layer of defense in depth.

### The K8s attack surface

| Layer | Concerns |
|-------|----------|
| **Cluster / nodes** | Kernel CVEs, exposed kubelet, etcd access |
| **API server** | Authentication, RBAC, audit logging |
| **Network** | NetworkPolicy gaps, lateral movement, exfiltration |
| **Pod** | Privileged containers, hostPath mounts, capabilities |
| **Image** | Vulnerabilities, malicious base images, supply chain |
| **Runtime** | Container escape, malicious workloads |
| **Secrets** | Encryption at rest, secret leakage |

We've touched many of these in earlier modules. This module focuses on the *security-specific* primitives: PSS, admission, image security, supply chain.

---

## 2. Pod Security Standards (PSS) — mechanism

PSS replaced the deprecated PodSecurityPolicy. It defines three **standardized policy profiles**:

| Profile | Allowed | Use case |
|---------|---------|----------|
| `privileged` | Anything (essentially no policy) | Trusted infrastructure workloads |
| `baseline` | Prevents known privilege escalations; minimal restrictions | The bare minimum for any workload |
| `restricted` | Hardened — non-root, read-only root FS, no caps, seccomp, etc. | Application workloads |

The profiles are enforced by the **Pod Security Admission Controller** (built into the apiserver since 1.23, GA in 1.25). You opt in **per namespace** by labeling:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: payments
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/enforce-version: latest
    pod-security.kubernetes.io/audit: restricted        # log violations
    pod-security.kubernetes.io/warn: restricted          # surface in kubectl warning
```

Three modes:
- **enforce** — reject violating pods.
- **audit** — record in audit log.
- **warn** — show a warning to the user on create.

Mature pattern: start with `warn`/`audit` to find offenders, then move to `enforce`.

### What `restricted` actually requires

```yaml
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 10001
    seccompProfile: { type: RuntimeDefault }
  containers:
    - name: app
      image: api:1.0
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        capabilities: { drop: ["ALL"] }
      # for /tmp, /var/cache etc., mount emptyDir if needed
```

Try to violate it:

```bash
$ kubectl apply -f bad-pod.yaml -n payments
Error from server (Forbidden): pods "bad-pod" is forbidden: violates PodSecurity "restricted:latest":
  allowPrivilegeEscalation != false (containers "app" must set securityContext.allowPrivilegeEscalation=false),
  unrestricted capabilities (containers "app" must drop ALL or set drop=["ALL"]),
  runAsNonRoot != true,
  seccompProfile (pod or containers "app" must set securityContext.seccompProfile.type to "RuntimeDefault")
```

The apiserver itself rejected the create. No need for extra tooling for the standard profiles.

---

## 3. Variations — admission controllers, image security, supply chain

### Admission controllers

The apiserver pipeline runs **admission controllers** after authz, before persisting to etcd. Two types:

- **Mutating** — can modify the object (inject sidecars, set defaults).
- **Validating** — accept or reject.

Built-in controllers handle PSS, ResourceQuota, ServiceAccount token mounting, etc. You add custom policies via **dynamic admission webhooks** or, increasingly, **CEL-based policies in-cluster**.

### OPA Gatekeeper

Open Policy Agent in Kubernetes form. You write policies in **Rego**:

```yaml
apiVersion: templates.gatekeeper.sh/v1
kind: ConstraintTemplate
metadata: { name: k8srequiredlabels }
spec:
  crd:
    spec:
      names: { kind: K8sRequiredLabels }
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package k8srequiredlabels
        violation[{"msg": msg}] {
          required := input.parameters.labels
          provided := {label | input.review.object.metadata.labels[label]}
          missing := required - provided
          count(missing) > 0
          msg := sprintf("missing labels: %v", [missing])
        }
---
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: K8sRequiredLabels
metadata: { name: require-team-label }
spec:
  match:
    kinds: [{ apiGroups: ["apps"], kinds: ["Deployment"] }]
  parameters:
    labels: [team, owner]
```

Now any Deployment without `team` and `owner` labels is rejected.

### Kyverno

Same role as Gatekeeper, but policies are written in **YAML** — no Rego. Lower barrier.

```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata: { name: require-labels }
spec:
  validationFailureAction: Enforce
  rules:
    - name: require-team
      match:
        any:
          - resources: { kinds: [Deployment] }
      validate:
        message: "label 'team' is required"
        pattern:
          metadata:
            labels:
              team: "?*"
```

Kyverno can also mutate (add labels, inject sidecars) and generate (create related resources). It's the popular choice in 2026 for teams that don't want to learn Rego.

### ValidatingAdmissionPolicy (built-in, no webhook)

Since 1.30, K8s has **native CEL-based admission policies**:

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata: { name: require-labels }
spec:
  matchConstraints:
    resourceRules:
      - apiGroups: [apps]
        apiVersions: [v1]
        operations: [CREATE, UPDATE]
        resources: [deployments]
  validations:
    - expression: "has(object.metadata.labels.team)"
      message: "team label required"
```

No external webhook needed; runs in-process. Great for simple rules. For complex logic, Kyverno/Gatekeeper still win.

### Image security

A handful of standards:

- **Pin to immutable digests** (`image@sha256:...`), not tags. `latest` is the worst possible choice in prod.
- **Scan images** in CI (Trivy, Grype, Snyk) for CVEs.
- **Sign images** with cosign (Sigstore). Verify in cluster with policy-controller, Kyverno, or Connaisseur.
- **Limit registries** — admission policy that only allows pulls from your trusted registry.
- **Distroless / minimal base images** (e.g., `gcr.io/distroless/static`) — smaller attack surface.

```yaml
# Kyverno policy: only allow images from our registry, must be signed
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata: { name: verify-signature }
spec:
  validationFailureAction: Enforce
  rules:
    - name: only-trusted-registry
      match: { any: [{ resources: { kinds: [Pod] } }] }
      validate:
        message: "Images must come from registry.example.com"
        pattern:
          spec:
            containers:
              - image: "registry.example.com/*"
    - name: verify-cosign-signature
      match: { any: [{ resources: { kinds: [Pod] } }] }
      verifyImages:
        - imageReferences: ["registry.example.com/*"]
          attestors:
            - entries:
                - keys:
                    publicKeys: |-
                      -----BEGIN PUBLIC KEY-----
                      MFkwEwY...
                      -----END PUBLIC KEY-----
```

### Software supply chain

Standards converging around **SLSA** (Supply-chain Levels for Software Artifacts):

- **SBOM** (Software Bill of Materials) — list every dependency in every image. Tools: Syft, Trivy.
- **In-toto attestations** — signed metadata about the build (who, when, from what source).
- **SLSA provenance** — bundle of SBOM + attestation + signature.

The future: cluster only runs images with cosign-verified provenance proving they were built by your CI from your Git, with no manual interference.

### Runtime security

Even with all the above, a vulnerability in your code is still a risk. Runtime tools observe pod behavior:

- **Falco** — eBPF/kernel-module-based rule engine. "Alert if a shell spawned in a container," "Alert if a container reads /etc/shadow."
- **Tetragon** (Cilium) — eBPF-based observability + enforcement.
- **Tracee** (Aqua) — similar.

```yaml
# Example Falco rule (default ruleset)
- rule: Run shell untrusted
  desc: An attempt to spawn a shell below a non-shell application
  condition: spawned_process and shell_procs and not parent_shell_processes
  output: A shell was spawned in a container (container=%container.id ...)
  priority: NOTICE
```

### etcd encryption at rest

Mentioned in module 6 but worth repeating: secrets in etcd should be **encrypted at rest** via apiserver's `--encryption-provider-config`. Managed clusters do this by default; self-managed clusters often miss it.

---

## 4. Practical application — a hardened pod + a starter policy set

### A `restricted`-compliant pod

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  labels: { app: api, team: payments }
spec:
  replicas: 3
  selector: { matchLabels: { app: api } }
  template:
    metadata: { labels: { app: api } }
    spec:
      automountServiceAccountToken: false
      securityContext:
        runAsNonRoot: true
        runAsUser: 10001
        fsGroup: 10001
        seccompProfile: { type: RuntimeDefault }
      containers:
        - name: api
          image: registry.example.com/api@sha256:8c2c...     # digest-pinned
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities: { drop: ["ALL"] }
          ports: [{ containerPort: 8080, name: http }]
          resources:
            requests: { cpu: "200m", memory: "256Mi" }
            limits:   { memory: "512Mi" }
          volumeMounts:
            - { name: tmp,   mountPath: /tmp }
            - { name: cache, mountPath: /var/cache }
      volumes:
        - { name: tmp,   emptyDir: {} }
        - { name: cache, emptyDir: {} }
```

Every box checked: non-root, read-only root FS, no caps, seccomp default, digest-pinned image, no SA token, requests/limits set.

### Starter policy set (Kyverno)

```yaml
# 1) Restrict namespaces to baseline+ (use PSS labels for restricted)
# 2) Block images from outside our registry
# 3) Require team/owner labels on Deployments
# 4) Block hostPath mounts in non-system namespaces
# 5) Require non-root + dropped caps even if PSS isn't restricted

apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata: { name: baseline-hardening }
spec:
  validationFailureAction: Enforce
  rules:
    - name: registry-allowlist
      match: { any: [{ resources: { kinds: [Pod] } }] }
      exclude:
        any:
          - resources: { namespaces: [kube-system, monitoring, cert-manager] }
      validate:
        message: "Images must be from registry.example.com"
        pattern:
          spec:
            containers:
              - image: "registry.example.com/*"
    - name: required-labels
      match: { any: [{ resources: { kinds: [Deployment, StatefulSet] } }] }
      validate:
        message: "Deployments must have team and owner labels"
        pattern:
          metadata:
            labels:
              team: "?*"
              owner: "?*"
    - name: block-hostpath
      match: { any: [{ resources: { kinds: [Pod] } }] }
      exclude:
        any:
          - resources: { namespaces: [kube-system, monitoring] }
      validate:
        message: "hostPath volumes are not allowed"
        pattern:
          spec:
            =(volumes):
              - X(hostPath): "null"
```

```bash
$ kubectl apply -f kyverno-policies.yaml
$ kubectl get clusterpolicy
NAME                   BACKGROUND   VALIDATE ACTION   READY   AGE
baseline-hardening     true         Enforce           True    20s

# Try a violation
$ kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata: { name: external-img }
spec:
  containers: [{ name: nope, image: nginx:1.27 }]
EOF
Error from server: admission webhook "validate.kyverno.svc-fail" denied the request:
  policy baseline-hardening/registry-allowlist failed: Images must be from registry.example.com
```

---

## 5. Common Mistakes & Gotchas

- **PSS labels missing on namespaces.** Default is `privileged` — i.e., no enforcement. Label every app namespace.
- **PSS `enforce: restricted` on a namespace with operators in it.** Many operators legitimately need privileged pods. Use exemptions or split into namespaces with different profiles.
- **`runAsNonRoot: true` + image whose `USER` is root.** Pod stays in `CreateContainerConfigError`. Rebuild the image with a non-root user, or set `runAsUser: <uid>` explicitly.
- **`readOnlyRootFilesystem: true` and an app that writes to `/tmp`.** Crashes on first write. Mount `emptyDir` at `/tmp` and other paths it needs.
- **`hostPath` for "just one log directory."** Massive escape vector. Use `hostPath` only in DaemonSets that strictly need node-level access.
- **`hostNetwork`, `hostPID`, `hostIPC: true` left in for "debugging."** Same.
- **Granting privileged ServiceAccount tokens to every pod.** Most pods don't call the apiserver — `automountServiceAccountToken: false`.
- **No image scanning gate in CI.** A 12-month-old base image had 30 high CVEs. Scan on every push; fail on critical.
- **`latest` tag in production.** Same image name today and tomorrow can be different bytes. Digest-pin.
- **Admission webhook unavailable → cluster-wide outage.** A misconfigured Kyverno/OPA can block every Pod creation. Set `failurePolicy: Ignore` for non-critical rules and run webhook controllers in HA.
- **Network policies allowing 0.0.0.0/0 egress.** Exfil path. At minimum, restrict to a whitelist of known external endpoints.
- **Secrets visible in pod env vars + logged on crash.** Apps logging their env on startup is shockingly common. Audit your bootstrap code.
- **No audit log.** Without `--audit-log-path`, you can't reconstruct who deleted what. Enable; ship to durable storage.
- **Falco/Tetragon installed but no one watches the alerts.** Compliance theater. Pipe to your incident system or don't install.
- **Cluster admin via cloud console with no MFA.** All the in-cluster hardening doesn't help if a stolen IAM token has cluster-admin.

---

## 🎯 Key Takeaways

- **Defense in depth — no single layer is enough.** PSS, NetworkPolicy, RBAC, admission policy, image signing, runtime detection — each catches different attacks.
- **`restricted` PSS + digest-pinned images + signed-image admission is the modern minimum.** Senior engineers consider anything less unfinished.
- **Admission control is your enforcement gate.** Whether built-in (PSS, ValidatingAdmissionPolicy) or via OPA/Kyverno, this is where "you wrote the policy" becomes "the cluster actually enforces it."
- **Supply chain matters as much as image content now.** SBOM + signature + provenance. CVE scanning catches yesterday's bugs; signing prevents tomorrow's substitution.
- **Don't run a hardened cluster with a soft perimeter.** A `cluster-admin` IAM role with no MFA defeats every PSS rule. Audit cloud IAM as you audit RBAC.

*← [prev](./16_operators_and_crds.md) | [next →](./18_production_kubernetes.md)*
