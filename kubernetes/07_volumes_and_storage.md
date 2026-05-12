# 07 — Volumes & Storage

> **Goal:** Understand pod-scoped volumes (emptyDir, hostPath), the PV/PVC/StorageClass abstraction for durable storage, how dynamic provisioning works, and what CSI drivers actually do.

---

## 1. Volumes — analogy + YAML

**Analogy.** A container's filesystem is **scratch paper**: it disappears the moment the container exits. A Volume is the **filing cabinet** the pod attaches to — sometimes a shared notebook the roommates pass around (`emptyDir`), sometimes a deposit box at the bank that survives even when the apartment burns down (`PersistentVolume`).

### Volume scope
- **Volumes live with the Pod.** When the pod dies, ephemeral volumes (`emptyDir`) die too.
- **PersistentVolumes live with the cluster (or beyond).** They survive pod restarts, node failures, and Deployment recreations.

### emptyDir — scratch space shared between containers

```yaml
apiVersion: v1
kind: Pod
metadata: { name: cache-demo }
spec:
  containers:
    - name: writer
      image: busybox
      command: ["sh", "-c", "while true; do date >> /data/log; sleep 5; done"]
      volumeMounts: [{ name: scratch, mountPath: /data }]
    - name: reader
      image: busybox
      command: ["sh", "-c", "tail -f /data/log"]
      volumeMounts: [{ name: scratch, mountPath: /data }]
  volumes:
    - name: scratch
      emptyDir:
        sizeLimit: 100Mi   # optional cap
        medium: ""         # "" = node disk; "Memory" = tmpfs (RAM)
```

`emptyDir.medium: Memory` makes it a tmpfs — fast, but counts against the pod's memory limit. Good for ephemeral caches.

### hostPath — mount a path from the node

```yaml
spec:
  containers:
    - name: agent
      image: my-agent
      volumeMounts: [{ name: docker-sock, mountPath: /var/run/docker.sock }]
  volumes:
    - name: docker-sock
      hostPath:
        path: /var/run/docker.sock
        type: Socket
```

**Use sparingly.** hostPath binds the pod to whatever node it lands on and is a massive security surface (`/var/log`? `/etc`? `/`?). Reserved for cluster-level agents (log shippers, node-exporters) — and even those usually run as DaemonSets.

---

## 2. Mechanism — PV, PVC, StorageClass, and the dynamic provisioning flow

Persistent storage in K8s is **deliberately a two-sided market**.

- The **cluster operator** publishes available storage as `PersistentVolume` (PV) objects (or makes a `StorageClass` that mints them on demand).
- The **app developer** requests storage with a `PersistentVolumeClaim` (PVC) saying "I need 10 GiB of fast SSD, read-write-once."

The two are matched — manually pre-provisioned or dynamically created.

### The flow (dynamic provisioning, the modern default)

```
App YAML:                                Cluster:
PVC: name=data, size=10Gi, class=ssd  →  apiserver
                                          ↓
                                          watching: StorageClass provisioner
                                          (e.g., AWS EBS CSI driver)
                                          ↓
                                          creates EBS volume in AWS
                                          ↓
                                          creates PV bound to PVC
                                          ↓
                                          PVC status: Bound
                                          ↓
Pod referencing PVC scheduled
                                          ↓
                                          kubelet asks CSI driver to attach +
                                          mount the volume at the requested path
```

### StorageClass — the recipe

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata: { name: ssd }
provisioner: ebs.csi.aws.com           # which CSI driver
parameters:
  type: gp3
  iops: "3000"
  throughput: "125"
  encrypted: "true"
reclaimPolicy: Delete                  # what happens to the PV when PVC is deleted
allowVolumeExpansion: true
volumeBindingMode: WaitForFirstConsumer
```

Key fields:
- **`provisioner`** — which CSI driver creates the PV.
- **`reclaimPolicy: Delete` vs `Retain`** — `Delete` removes the underlying disk when the PVC is deleted; `Retain` keeps the data and the cluster admin reclaims manually. Production databases nearly always use `Retain`.
- **`volumeBindingMode: WaitForFirstConsumer`** — delay PV creation until a Pod schedules. Essential in cloud clusters with zones — otherwise the volume might land in `us-east-1a` and the pod in `us-east-1b` and never meet.
- **`allowVolumeExpansion: true`** — lets you resize a PVC later.

### PVC + Pod

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata: { name: data }
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: ssd
  resources:
    requests:
      storage: 10Gi
---
apiVersion: v1
kind: Pod
metadata: { name: db }
spec:
  containers:
    - name: pg
      image: postgres:16
      env:
        - { name: POSTGRES_PASSWORD, value: dev }
      volumeMounts:
        - { name: data, mountPath: /var/lib/postgresql/data }
  volumes:
    - name: data
      persistentVolumeClaim: { claimName: data }
```

```bash
$ kubectl apply -f pvc.yaml -f db.yaml
persistentvolumeclaim/data created
pod/db created

$ kubectl get pvc
NAME   STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
data   Bound    pvc-3b8d2c4a-1f8a-4f3d-aa2b-9c8a8a8a8a8a   10Gi       RWO            ssd            8s

$ kubectl get pv
NAME                                       CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM         AGE
pvc-3b8d2c4a-1f8a-4f3d-aa2b-9c8a8a8a8a8a   10Gi       RWO            Delete           Bound    default/data  8s
```

---

## 3. Variations & depth

### Access modes

| Mode | Meaning |
|------|---------|
| `ReadWriteOnce` (RWO) | Mounted RW by **one node** at a time. Most block devices (EBS, persistent disks). |
| `ReadOnlyMany` (ROX) | Mounted RO by many nodes. |
| `ReadWriteMany` (RWX) | Mounted RW by many nodes. Needs a file-share backend (EFS, NFS, FSx, CephFS, Azure Files). |
| `ReadWriteOncePod` (RWOP) | RW by exactly one **pod** (not just one node). Newer; stronger isolation. |

A common mistake: assuming a block volume can be `ReadWriteMany`. It cannot. Need shared filesystems for that.

### Volume lifecycle and reclaim policy

- **PVC deleted with `reclaimPolicy: Delete`** → PV deleted → underlying disk deleted. Data gone.
- **PVC deleted with `reclaimPolicy: Retain`** → PV moves to `Released`. Disk still exists; admin must `kubectl delete pv` and manually clean.

For anything resembling a database, default to `Retain`. The minor cleanup cost is dwarfed by the cost of an accidental data wipe.

### Volume expansion (online)

```bash
# bump the request, apply
$ kubectl patch pvc data -p '{"spec":{"resources":{"requests":{"storage":"20Gi"}}}}'
persistentvolumeclaim/data patched

$ kubectl get pvc data
NAME   STATUS   VOLUME    CAPACITY   ACCESS MODES   STORAGECLASS   AGE
data   Bound    pvc-...   20Gi       RWO            ssd            5m
```

Driver resizes the disk; CSI then resizes the filesystem online if the FS supports it (ext4, xfs).

### CSI — Container Storage Interface

CSI is the **plugin spec** for storage drivers. Pre-CSI, every storage type was hard-coded in the kubelet (`gcePersistentDisk`, `awsElasticBlockStore`, etc.). Now everything is a separate driver running as DaemonSet + controller pods, talking to the kubelet over a Unix socket. CSI added:

- Dynamic provisioning
- Snapshots + clones (`VolumeSnapshot`, `VolumeSnapshotClass`)
- Volume expansion
- Vendor independence

Modern clusters: every PV is CSI-backed. The in-tree drivers are deprecated.

### Snapshots

```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata: { name: data-snap }
spec:
  volumeSnapshotClassName: ssd-snap
  source: { persistentVolumeClaimName: data }
```

Take snapshots, then restore by creating a new PVC with `dataSource: VolumeSnapshot`.

### Generic ephemeral volumes
PVC-style storage that's tied to the Pod's lifecycle (cleaned up on Pod delete). Used inline:

```yaml
volumes:
  - name: scratch
    ephemeral:
      volumeClaimTemplate:
        spec:
          accessModes: [ReadWriteOnce]
          storageClassName: ssd
          resources: { requests: { storage: 5Gi } }
```

### Common CSI drivers

| Driver | Backend |
|--------|---------|
| `ebs.csi.aws.com` | AWS EBS (block) |
| `efs.csi.aws.com` | AWS EFS (RWX file) |
| `pd.csi.storage.gke.io` | GCP Persistent Disk |
| `disk.csi.azure.com` | Azure Managed Disk |
| `rook-ceph` | Ceph (open-source, multi-modal) |
| `longhorn.io` | Lightweight block storage from Rancher |
| `topolvm.cybozu.com` | Local LVM-based (great for fast, node-local DBs) |

---

## 4. Practical application — a Postgres with snapshot-based backup

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata: { name: ssd-retain }
provisioner: ebs.csi.aws.com
parameters: { type: gp3, encrypted: "true" }
reclaimPolicy: Retain
allowVolumeExpansion: true
volumeBindingMode: WaitForFirstConsumer
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata: { name: pg-data }
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: ssd-retain
  resources: { requests: { storage: 50Gi } }
---
apiVersion: apps/v1
kind: Deployment        # NOTE: real Postgres should use StatefulSet (next module)
metadata: { name: postgres }
spec:
  replicas: 1
  strategy: { type: Recreate }     # never two pods writing at once
  selector: { matchLabels: { app: pg } }
  template:
    metadata: { labels: { app: pg } }
    spec:
      containers:
        - name: pg
          image: postgres:16-alpine
          env:
            - { name: POSTGRES_PASSWORD, valueFrom: { secretKeyRef: { name: pg-secret, key: password } } }
            - { name: PGDATA, value: /var/lib/postgresql/data/pgdata }
          ports: [{ name: pg, containerPort: 5432 }]
          volumeMounts:
            - { name: data, mountPath: /var/lib/postgresql/data }
          readinessProbe:
            exec: { command: ["pg_isready", "-U", "postgres"] }
            periodSeconds: 5
      volumes:
        - name: data
          persistentVolumeClaim: { claimName: pg-data }
```

```bash
$ kubectl apply -f pg.yaml
storageclass.storage.k8s.io/ssd-retain created
persistentvolumeclaim/pg-data created
deployment.apps/postgres created

$ kubectl get pvc,pv
NAME                            STATUS   VOLUME    CAPACITY   ACCESS MODES   STORAGECLASS
persistentvolumeclaim/pg-data   Bound    pvc-...   50Gi       RWO            ssd-retain

$ kubectl exec deploy/postgres -- psql -U postgres -c "CREATE DATABASE app;"
CREATE DATABASE

# Snapshot before doing something risky
$ kubectl apply -f - <<EOF
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata: { name: pre-migration }
spec:
  volumeSnapshotClassName: ebs
  source: { persistentVolumeClaimName: pg-data }
EOF
volumesnapshot.snapshot.storage.k8s.io/pre-migration created

$ kubectl get volumesnapshot
NAME             READYTOUSE   SOURCEPVC   RESTORESIZE   AGE
pre-migration    true         pg-data     50Gi          12s
```

### Restoring from a snapshot

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata: { name: pg-data-restored }
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: ssd-retain
  resources: { requests: { storage: 50Gi } }
  dataSource:
    name: pre-migration
    kind: VolumeSnapshot
    apiGroup: snapshot.storage.k8s.io
```

Then point a (new) Deployment at this PVC.

---

## 5. Common Mistakes & Gotchas

- **Multi-replica Deployment with a `ReadWriteOnce` PVC.** Two pods both try to mount the EBS — second pod stuck in `ContainerCreating`. Use a StatefulSet (per-pod PVC) or switch to RWX storage.
- **`emptyDir` and "but my data!"** It's gone the moment the pod is gone. emptyDir survives container restarts within a pod, but not pod deletion. Don't use it for state.
- **`Immediate` binding mode in a zonal cluster.** PV provisions in zone A; scheduler later picks zone B; pod is stuck forever because EBS can't cross AZs. Always use `WaitForFirstConsumer`.
- **No `reclaimPolicy: Retain` on database PVs.** Some intern deletes the PVC; the underlying disk gets deleted; the runbook starts with "restore from backup." Always Retain for stateful data.
- **Resizing without `allowVolumeExpansion: true`.** Patch fails with a confusing error. Set this on the SC at creation.
- **Filesystem expansion needs an online-resize-capable FS.** Default ext4/xfs work; legacy or read-only filesystems may need the pod to be restarted to pick up the new size.
- **`hostPath` in a multi-node cluster.** Pod scheduled to a different node sees a different (or empty) directory. Massive footgun.
- **No `subPath` on shared mounts.** Multiple containers writing to the same volume root step on each other. Use `subPath` to scope.
- **StorageClass mismatch on PVC.** PVC requests `class: fast`; cluster has `class: gp2`; PVC pending forever. Check `kubectl get sc` for the actual class names.
- **Cleanup on cluster deletion.** With `reclaimPolicy: Delete`, deleting a PVC deletes the disk. With `Retain`, those disks linger in your cloud account forever, costing money. Have a teardown procedure.
- **Mixing access modes naively.** Asking for RWX from a block driver fails to provision. Check the driver's documentation.

---

## 🎯 Key Takeaways

- **PVCs decouple "I need storage" from "what kind of storage exists in this cluster."** This is what makes manifests portable across clusters/clouds — the StorageClass is the per-cluster mapping.
- **`volumeBindingMode: WaitForFirstConsumer` is non-negotiable in zonal clusters.** A senior engineer's first review comment on a new StorageClass.
- **`reclaimPolicy: Retain` is the difference between "we lost the database" and "we just need to re-bind a PV."** Default to Retain for anything stateful.
- **CSI made storage in K8s genuinely pluggable.** Snapshots, expansion, cloning all came with it. If your cluster predates CSI, you have technical debt.
- **emptyDir + Memory tmpfs is an underrated tool.** Fast caches, ML scratch space, sidecar communication — keep it in your back pocket.

*← [prev](./06_configmaps_and_secrets.md) | [next →](./08_statefulsets_jobs_daemonsets.md)*
