# 15 — Helm & Kustomize

> **Goal:** Stop copy-pasting YAML across environments. Master Helm's templating + release model and Kustomize's overlay model, and know when each is the right tool.

---

## 1. The packaging problem — analogy + first usage

**Analogy.** A bare YAML manifest is **handwritten meeting notes** — perfect once, terrible to reuse. You need either a **mail-merge template** (Helm — substitute values into a master template) or a **layered transparency stack** (Kustomize — base sheet + overlay sheets that modify it). Both solve "I have ten near-identical environments and I don't want ten copies."

### Helm — the first 30 seconds

```bash
$ helm repo add bitnami https://charts.bitnami.com/bitnami
$ helm repo update
$ helm install my-redis bitnami/redis --version 19.0.0 --set auth.password=dev
NAME: my-redis
LAST DEPLOYED: Mon May 13 10:14:22 2026
STATUS: deployed
REVISION: 1

$ helm list
NAME      NAMESPACE   REVISION   STATUS     CHART          APP VERSION
my-redis  default     1          deployed   redis-19.0.0   7.2.4
```

`helm install` rendered the chart's templates into manifests using your values, and applied them. It also recorded a **Release** — Helm's tracking record — so you can `upgrade`, `rollback`, or `uninstall`.

### Kustomize — the first 30 seconds

```bash
$ mkdir -p k/base k/overlays/prod
$ cat > k/base/deployment.yaml <<EOF
apiVersion: apps/v1
kind: Deployment
metadata: { name: api }
spec:
  replicas: 1
  selector: { matchLabels: { app: api } }
  template:
    metadata: { labels: { app: api } }
    spec:
      containers: [{ name: api, image: api:1.0 }]
EOF
$ cat > k/base/kustomization.yaml <<EOF
resources: [deployment.yaml]
EOF
$ cat > k/overlays/prod/kustomization.yaml <<EOF
resources: [../../base]
replicas:
  - { name: api, count: 10 }
images:
  - { name: api, newTag: 1.4.2 }
EOF

$ kubectl apply -k k/overlays/prod
deployment.apps/api created
```

Kustomize is built into `kubectl` since 1.14. No templating language; it does structured patches on top of a base.

---

## 2. Mechanism — what each tool actually does

### Helm — render + release

A Helm **chart** is a directory:

```
mychart/
├── Chart.yaml            # name, version, appVersion
├── values.yaml           # default values
├── templates/
│   ├── deployment.yaml   # templated using Go's text/template + Sprig
│   ├── service.yaml
│   └── _helpers.tpl      # template helpers
└── charts/               # subcharts (dependencies)
```

`Chart.yaml`:
```yaml
apiVersion: v2
name: mychart
version: 0.1.0           # chart version
appVersion: "1.4.2"       # the app's version
dependencies:
  - name: redis
    version: ~19.0.0
    repository: https://charts.bitnami.com/bitnami
```

`templates/deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "mychart.fullname" . }}
  labels:
    {{- include "mychart.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "mychart.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "mychart.selectorLabels" . | nindent 8 }}
    spec:
      containers:
        - name: app
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
```

When you `helm install`:
1. Helm renders all templates against the merged values (chart defaults + your `--values` / `--set`).
2. It POSTs the rendered manifests to the apiserver.
3. It stores the **Release** — name, namespace, revision, manifests, values — as a Secret (or ConfigMap) in the cluster.
4. `helm upgrade` does the same with a new revision; `rollback` reapplies an old one.

### Helm commands you'll use daily

```bash
helm install my-app ./mychart --values prod.yaml
helm upgrade my-app ./mychart --values prod.yaml --atomic --timeout 5m
helm template my-app ./mychart --values prod.yaml          # render without applying
helm diff upgrade my-app ./mychart --values prod.yaml      # via helm-diff plugin — see what would change
helm list -A
helm history my-app
helm rollback my-app 3
helm uninstall my-app
helm lint ./mychart
helm package ./mychart                                      # → mychart-0.1.0.tgz
```

The `helm-diff` plugin is essential for production — never run `upgrade` blind.

### Kustomize — declarative patches

`kustomization.yaml` is a recipe:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

# Resources to start from
resources:
  - ../../base
  - extra-secret.yaml

# Set namespace + name prefix
namespace: prod
namePrefix: prod-

# Common labels & annotations
commonLabels:
  env: prod
commonAnnotations:
  git.sha: "abc123"

# Image overrides
images:
  - name: api
    newName: registry.example.com/api
    newTag: "1.4.2"

# Replicas override
replicas:
  - name: api
    count: 10

# Strategic merge patches
patches:
  - target: { kind: Deployment, name: api }
    patch: |
      - op: add
        path: /spec/template/spec/containers/0/env/-
        value: { name: ENVIRONMENT, value: prod }

# Generate ConfigMaps/Secrets with auto-hash suffix
configMapGenerator:
  - name: app-config
    files: [config.yaml]
secretGenerator:
  - name: app-secrets
    literals: [DB_PASSWORD=prod-secret]
```

```bash
$ kubectl kustomize k/overlays/prod          # render to stdout
$ kubectl apply -k k/overlays/prod           # render and apply
```

**The killer feature of Kustomize**: `configMapGenerator` auto-appends a hash suffix to the generated ConfigMap name (`app-config-7g8h2k`). The Deployment auto-references the new name. Change the config → new ConfigMap name → Deployment template changes → rolling update. **No checksum annotation needed.**

---

## 3. Variations — Helm vs Kustomize, and when to use each

| | Helm | Kustomize |
|--|------|----------|
| **Style** | Templating (Go text/template) | Patches (strategic merge + JSON patch) |
| **Reuse model** | Packaged chart published to a repo | Base + overlays in the same repo |
| **Values input** | `values.yaml` + `--set` | Patch files in overlays |
| **Cluster state** | Releases tracked in cluster | Stateless — what's on disk is the source |
| **Dependencies** | First-class (subcharts, repos) | Just point `resources:` at a base |
| **Best for** | Distributing apps to others; complex apps with many knobs; third-party software | Managing your own apps across envs; minimal extra mental model |

**They compose.** Many teams use Helm for third-party (Redis, Prometheus, ingress-nginx) and Kustomize for their own apps. Kustomize can `helm:` inflate a chart too:

```yaml
helmCharts:
  - name: redis
    repo: https://charts.bitnami.com/bitnami
    version: 19.0.0
    releaseName: cache
    valuesFile: redis-values.yaml
```

### Helm hooks
Templates annotated with `helm.sh/hook` run at lifecycle points (`pre-install`, `post-install`, `pre-upgrade`, etc.). Common for migrations: a `Job` with `helm.sh/hook: pre-upgrade`.

### Helm 3 stores releases as Secrets
No more Tiller (Helm 2's privileged in-cluster component). Just CLI talking to the apiserver. Releases are stored as Secrets in the namespace.

### Umbrella / library charts
- **Umbrella chart** — a chart that mostly just depends on others; the "platform install" for a project.
- **Library chart** (Helm 3+) — provides templates for reuse without producing its own resources.

### Other tools in the space
- **cdk8s** — write manifests in TypeScript/Python/Java/Go; synthesizes YAML.
- **Pulumi** / **Terraform K8s provider** — IaC over Kubernetes.
- **jsonnet / tanka** — programmable manifests (Grafana Labs uses heavily).
- **kpt** — Google's "configuration as data" alternative.

Most ecosystems converge on Helm + Kustomize; the rest are alternatives if they fit your culture.

---

## 4. Practical application — a chart and an overlay for the same app

### A minimal Helm chart for our api

```yaml
# Chart.yaml
apiVersion: v2
name: api
version: 0.1.0
appVersion: "1.4.2"
```

```yaml
# values.yaml — sensible defaults
replicaCount: 3
image:
  repository: registry.example.com/api
  tag: ""           # falls back to .Chart.AppVersion
  pullPolicy: IfNotPresent
resources:
  requests: { cpu: "200m", memory: "256Mi" }
  limits:   { memory: "512Mi" }
service:
  type: ClusterIP
  port: 80
ingress:
  enabled: false
```

```yaml
# templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}
  labels:
    app.kubernetes.io/name: {{ .Chart.Name }}
    app.kubernetes.io/instance: {{ .Release.Name }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ .Chart.Name }}
      app.kubernetes.io/instance: {{ .Release.Name }}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: {{ .Chart.Name }}
        app.kubernetes.io/instance: {{ .Release.Name }}
    spec:
      containers:
        - name: app
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - { name: http, containerPort: 8080 }
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
```

```bash
$ helm install api ./api --values prod.yaml --dry-run --debug
[manifest output...]
NAME: api
LAST DEPLOYED: Mon May 13 10:14:22 2026
STATUS: pending-install

$ helm install api ./api --values prod.yaml
NAME: api
NAMESPACE: default
STATUS: deployed
REVISION: 1
```

### Same app with Kustomize

```
k/
├── base/
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── configmap.yaml
│   └── kustomization.yaml
└── overlays/
    ├── dev/
    │   ├── kustomization.yaml
    │   └── config.yaml
    ├── staging/
    │   └── kustomization.yaml
    └── prod/
        ├── kustomization.yaml
        ├── replicas-patch.yaml
        └── resources-patch.yaml
```

```yaml
# k/base/kustomization.yaml
resources: [deployment.yaml, service.yaml]
configMapGenerator:
  - name: app-config
    files: [config.yaml]

# k/overlays/prod/kustomization.yaml
resources: [../../base]
namespace: prod
replicas: [{ name: api, count: 10 }]
images: [{ name: api, newTag: "1.4.2" }]
configMapGenerator:
  - name: app-config
    behavior: replace
    files: [config.yaml]            # prod-specific config
patches:
  - path: resources-patch.yaml
    target: { kind: Deployment, name: api }
```

```bash
$ kubectl kustomize k/overlays/prod | head -20
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config-h8k2m3        # auto-suffixed!
  namespace: prod
...

$ kubectl diff -k k/overlays/prod    # see what would change
$ kubectl apply -k k/overlays/prod
deployment.apps/api configured
service/api configured
configmap/app-config-h8k2m3 created
```

---

## 5. Common Mistakes & Gotchas

- **Helm: putting plaintext secrets in `values.yaml` and committing it.** Use `helm-secrets` + SOPS, or pull secrets from a Secret Manager via ESO.
- **Helm: 200-line `values.yaml` of toggles.** A sign the chart is doing too much. Smaller, opinionated charts compose better.
- **Helm: `--set` for complex YAML.** Quote/escape hell. Use `--values file.yaml`.
- **Helm: `--atomic` not used in CI.** Without it, a failed upgrade leaves you in a half-applied state. Always `--atomic --timeout`.
- **Helm: forgetting that `helm template` skips admission webhooks.** What works as a dry-run can still fail on apply. Render then `kubectl diff`.
- **Helm: not pinning subchart versions.** Today you get redis 19.0.0; tomorrow `~19.0.0` resolves to 19.7.4 with new defaults. Pin exact versions or use `Chart.lock`.
- **Helm: template indentation bugs.** `nindent` vs `indent`, leading whitespace on `{{- ... -}}`. Lint with `helm lint`.
- **Kustomize: `commonLabels` is *also* applied to selectors.** Changing it later breaks the Deployment because its selector becomes immutable. Use `labels.includeSelectors: false` for non-selector labels.
- **Kustomize: large strategic merge patches.** Hard to review. Use JSON-patch `patches:` for surgical changes; reserve strategic merge for replacement of whole sections.
- **Kustomize: `secretGenerator` with plaintext literals in Git.** Same problem as Helm. Use external secret stores.
- **Mixing Helm and Kustomize for the same release name.** Race conditions on labels and annotations (`app.kubernetes.io/managed-by` conflicts). Pick one for any given release.
- **Helm + GitOps without `--install`.** ArgoCD/Flux can manage Helm releases; if you mix `helm install` from CI with `helm upgrade` from GitOps, eventually they fight.
- **Trusting `helm rollback` for stateful upgrades.** Rolling back a chart doesn't roll back data migrations. Backup first.

---

## 🎯 Key Takeaways

- **Helm is great at distributing apps; Kustomize is great at customizing them.** A senior engineer reaches for Helm for third-party software and Kustomize (or thin Helm charts) for in-house apps.
- **Kustomize's hash-suffixed ConfigMap/Secret generators are quietly brilliant.** They make config rollouts deterministic without templating tricks.
- **Helm + `helm-diff` + `--atomic` is the production-safe combo.** Never apply Helm changes blind in CI.
- **You can use them together** — Helm for upstream charts, Kustomize as a wrapper to overlay environment-specifics on top. Many GitOps repos look exactly like that.
- **The packaging tool is your team's vocabulary.** Pick one, document it, lint it. Don't let three patterns proliferate.

*← [prev](./14_observability.md) | [next →](./16_operators_and_crds.md)*
