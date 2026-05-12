# 06 — ConfigMaps & Secrets

> **Goal:** Externalize config and secrets cleanly. Know the consumption modes (env, file, projected), understand why Secrets are *not* encrypted by default, and learn how external secret stores (Vault, AWS/GCP/Azure secret managers) integrate.

---

## 1. ConfigMap & Secret — analogy + YAML

**Analogy.** Your container image is the **shrink-wrapped product**. ConfigMaps and Secrets are the **stickers you slap on at delivery** — environment-specific values that vary across dev/staging/prod but don't justify rebuilding the image. ConfigMaps are for non-sensitive values; Secrets are the *intent* "this is sensitive" (though, as we'll see, the storage difference is minimal by default).

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
data:
  LOG_LEVEL: "info"
  FEATURE_FLAGS: "feature_x=on,feature_y=off"
  application.yaml: |
    server:
      port: 8080
    cache:
      ttl: 300
```

### Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: app-secrets
type: Opaque
stringData:                   # plaintext at apply-time; stored base64'd
  DB_PASSWORD: "s3cret!"
  API_KEY: "ak_prod_4f7c..."
```

```bash
$ kubectl apply -f config.yaml -f secret.yaml
configmap/app-config created
secret/app-secrets created

$ kubectl get secret app-secrets -o yaml | grep -A2 data:
data:
  API_KEY: YWtfcHJvZF80ZjdjLi4u
  DB_PASSWORD: czNjcmV0IQ==
```

That `base64` is **encoding, not encryption**. Anyone with `get secret` permission sees the values trivially:

```bash
$ kubectl get secret app-secrets -o jsonpath='{.data.DB_PASSWORD}' | base64 -d
s3cret!
```

---

## 2. Mechanism — consumption modes, mounts, and what updates trigger

There are **three ways** to consume a ConfigMap or Secret from a Pod:

### Mode 1: Env vars from one key

```yaml
env:
  - name: LOG_LEVEL
    valueFrom:
      configMapKeyRef: { name: app-config, key: LOG_LEVEL }
  - name: DB_PASSWORD
    valueFrom:
      secretKeyRef: { name: app-secrets, key: DB_PASSWORD }
```

### Mode 2: All keys as env (envFrom)

```yaml
envFrom:
  - configMapRef: { name: app-config }
  - secretRef:    { name: app-secrets }
```

Every key becomes an env var of the same name. Convenient; means renaming a key in the ConfigMap silently renames the env var.

**Env vars are baked in at container start.** Updating the ConfigMap/Secret does *not* update env vars in a running container. You must restart the pod (`kubectl rollout restart deploy/foo`).

### Mode 3: Mounted as files (volumes)

```yaml
volumes:
  - name: config
    configMap: { name: app-config }
  - name: secrets
    secret: { secretName: app-secrets, defaultMode: 0400 }
containers:
  - name: app
    volumeMounts:
      - { name: config,  mountPath: /etc/app }
      - { name: secrets, mountPath: /etc/secrets, readOnly: true }
```

`/etc/app/LOG_LEVEL`, `/etc/app/application.yaml`, `/etc/secrets/DB_PASSWORD` — each key is a file.

**Mounted ConfigMaps and Secrets DO update** when the source changes — but with a delay (kubelet refreshes them periodically, typically within a minute). Apps that watch the file (`inotify`) can reload without a restart. This is the major reason to prefer mount over env for things that *might* change.

### Mode 4: Projected volumes — combine multiple sources

```yaml
volumes:
  - name: app-config-bundle
    projected:
      sources:
        - configMap: { name: app-config }
        - secret:    { name: app-secrets }
        - serviceAccountToken:
            path: token
            expirationSeconds: 3600
            audience: vault
```

Used heavily by service-mesh sidecars and identity systems (audience-scoped tokens for Vault, AWS IRSA, etc.).

### How the kubelet delivers ConfigMaps/Secrets

The kubelet projects them via an **in-memory tmpfs** mount. They never hit the node's disk (good for secrets). When you update the ConfigMap, the kubelet's syncloop notices on its next pass and atomically swaps the files on the tmpfs — via symlink rotation, so readers see a consistent snapshot.

---

## 3. Variations — Secret types, encryption at rest, external stores

### Secret types

| Type | Purpose |
|------|---------|
| `Opaque` | Generic key/value (default) |
| `kubernetes.io/tls` | Has `tls.crt` and `tls.key` — used by Ingress |
| `kubernetes.io/dockerconfigjson` | Image-pull secrets |
| `kubernetes.io/service-account-token` | Auto-generated SA token (legacy; modern SAs use projected tokens) |
| `bootstrap.kubernetes.io/token` | Cluster bootstrap |
| `kubernetes.io/ssh-auth` | SSH key |

### Encryption at rest

By default, **Secrets are stored unencrypted in etcd** — only base64'd. Anyone with etcd read access (or a backup) has all your secrets.

Two layers fix this:

1. **Enable encryption-at-rest in the apiserver.** A `--encryption-provider-config` file tells the apiserver to encrypt Secret values before writing to etcd. Most managed clusters (EKS, GKE, AKS) enable this with a KMS-backed key by default.

```yaml
# Example provider config (cluster admins only)
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources: [secrets]
    providers:
      - kms: { name: aws-kms, endpoint: unix:///var/run/kmsplugin.sock }
      - identity: {}    # fallback, decrypt old data
```

2. **External secret stores** — keep the actual secret outside K8s entirely.

### External Secrets Operator (ESO) — the modern pattern

You store secrets in Vault / AWS Secrets Manager / GCP Secret Manager / Azure Key Vault / 1Password. ESO syncs them *into* K8s Secrets so workloads can consume them normally.

```yaml
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata: { name: aws-secrets }
spec:
  provider:
    aws:
      service: SecretsManager
      region: us-east-1
      auth: { jwt: { serviceAccountRef: { name: eso-sa } } }
---
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata: { name: app-secrets }
spec:
  refreshInterval: 1h
  secretStoreRef: { name: aws-secrets, kind: SecretStore }
  target: { name: app-secrets }
  data:
    - secretKey: DB_PASSWORD
      remoteRef: { key: prod/api, property: db_password }
```

ESO creates and refreshes the K8s `Secret` named `app-secrets` from AWS. Pods consume it as a normal Secret. Rotation in AWS propagates to the cluster within `refreshInterval`.

### Other approaches

- **Vault Agent Injector** — mutating webhook that adds a sidecar fetching secrets from Vault into a shared `emptyDir`, no K8s Secret involved.
- **CSI Secret Store driver** — mounts secrets directly from an external store via a CSI volume. No K8s Secret object created (unless you opt in to sync).
- **SOPS + sealed-secrets / age** — encrypt secrets *in Git*, decrypt in-cluster. Good for GitOps when you can't use an external store.

### Immutable ConfigMaps/Secrets
Set `immutable: true`. Locks the data field; updates require delete-and-recreate. Improves kubelet performance with very large numbers of ConfigMaps (no need to watch for changes).

---

## 4. Practical application — typed config + projected secrets, restart-on-change

```yaml
apiVersion: v1
kind: ConfigMap
metadata: { name: api-config }
data:
  app.yaml: |
    server:
      port: 8080
      timeout: 30s
    logging:
      level: info
      format: json
---
apiVersion: v1
kind: Secret
metadata: { name: api-secrets }
type: Opaque
stringData:
  DATABASE_URL: "postgres://api:s3cret@db:5432/api"
  JWT_SIGNING_KEY: "Zm9vYmFyYmF6"
---
apiVersion: apps/v1
kind: Deployment
metadata: { name: api }
spec:
  replicas: 3
  selector: { matchLabels: { app: api } }
  template:
    metadata:
      labels: { app: api }
      annotations:
        # checksum annotation — change with the data → triggers rollout
        config/checksum: "REPLACE_WITH_HASH"
    spec:
      containers:
        - name: api
          image: myapp:1.4.2
          ports: [{ name: http, containerPort: 8080 }]
          envFrom:
            - secretRef: { name: api-secrets }
          volumeMounts:
            - { name: config, mountPath: /etc/api, readOnly: true }
      volumes:
        - name: config
          configMap:
            name: api-config
            items: [{ key: app.yaml, path: app.yaml }]
```

```bash
$ kubectl apply -f api.yaml
configmap/api-config created
secret/api-secrets created
deployment.apps/api created

$ kubectl exec deploy/api -- ls /etc/api
app.yaml

$ kubectl exec deploy/api -- cat /etc/api/app.yaml
server:
  port: 8080
  timeout: 30s
...

$ kubectl exec deploy/api -- printenv DATABASE_URL
postgres://api:s3cret@db:5432/api
```

### Triggering a rollout when config changes

The **checksum annotation** pattern: Helm/Kustomize calculate a hash of the ConfigMap and write it as a Pod template annotation. Updating the ConfigMap changes the hash, which changes the Pod template, which triggers a normal rolling update. This is the standard idiom — much cleaner than scripting `kubectl rollout restart`.

```yaml
# Helm: annotations: { config/checksum: {{ .Values.config | sha256sum }} }
# Kustomize: configMapGenerator auto-suffixes the name with a hash (best UX)
```

Kustomize is even nicer — it appends the hash to the ConfigMap name itself, so consumers reference `api-config-7g8h2k`, and a config change creates `api-config-9j2l3m`, forcing the Deployment template to update.

---

## 5. Common Mistakes & Gotchas

- **Treating Secrets as encrypted.** They're base64 *encoded*. Anyone with `get secret` RBAC sees them. Enable encryption-at-rest in production, or use an external secret store.
- **Expecting env vars to update.** They don't. ConfigMap change + envFrom → still old env until a pod restart. Use file mounts for things that change, or wire up a controller (Stakater Reloader, kustomize hash, Helm checksum).
- **Mounting a Secret with mode 0644 by default.** Other processes in the container (or shared with another container in the Pod) could read them. Set `defaultMode: 0400` and run as non-root.
- **Putting Secrets in Git.** Stop. Use SOPS, sealed-secrets, or an external store. The "we'll fix it later" Secret in Git is the start of every postmortem.
- **`kubectl apply` of a Secret with `data:` (base64) — base64 *of* base64.** People sometimes pre-base64 their value and put it in `data`. Now it's double-encoded. Use `stringData` for plaintext.
- **ConfigMap as a place to ship application code.** Don't. Code goes in the image. ConfigMaps are for config/templates/sql migrations small enough to fit (limit is 1 MiB).
- **One huge ConfigMap for the whole app.** A change to one value re-rolls the whole Deployment. Split by lifecycle (frequently-changed vs stable).
- **Secret references across namespaces.** Not supported in core K8s. Use ESO (or duplicate the secret per namespace).
- **`imagePullSecrets` on the Pod vs the ServiceAccount.** Both work; SA-level is the cleaner pattern (every Pod in the namespace inherits).
- **Stale ConfigMap after a `kubectl create` + edit.** `kubectl create configmap` from a file does *not* update on re-run; use `kubectl create configmap foo --from-file=... --dry-run=client -o yaml | kubectl apply -f -`.
- **Forgetting `subPath` consequences.** Mounting a ConfigMap key as a file via `subPath` **disables automatic updates** — the kubelet only refreshes whole-directory mounts.

---

## 🎯 Key Takeaways

- **Mount instead of env when the value might change.** Env baking is the source of countless "I updated the ConfigMap but nothing happened" tickets.
- **`type: Opaque` is not encryption.** Get encryption-at-rest enabled and prefer an external secret store (ESO + Vault/Secrets Manager) for anything sensitive. A senior engineer always asks where the secrets live.
- **Checksum/hash annotations make config rollouts deterministic.** This is the single biggest UX win Helm and Kustomize offer over hand-rolled YAML.
- **Projected volumes are how identity flows in modern K8s.** SA tokens with audience+expiry power Vault, IRSA (AWS), Workload Identity (GCP) — the world is moving away from long-lived static secrets.
- **Separate ConfigMaps by change frequency, not by feature.** A login feature flag and a TLS truststore live on completely different timescales and shouldn't co-trigger rollouts.

*← [prev](./05_ingress_and_gateway_api.md) | [next →](./07_volumes_and_storage.md)*
