# 12 — Container Runtime Internals
> **Goal:** Understand what a container *actually is* at the Linux kernel level — namespaces, cgroups, the OCI specs, and the runc/containerd/dockerd hierarchy — so you can debug from first principles instead of memorizing commands.

---

## 1. A container is a process, with isolation flags

Roadmap mental model #1, expanded: a container is *literally* a Linux process started with a particular set of kernel features that give it an isolated view of the system. There is no "container runtime" in the sense of "JVM" or "Node runtime." The runtime is `clone()` + a config struct.

Concretely, when you `docker run alpine sh`, the kernel does roughly this:

```c
clone(child_fn, stack,
      CLONE_NEWPID | CLONE_NEWNS | CLONE_NEWUTS |
      CLONE_NEWIPC | CLONE_NEWNET | CLONE_NEWUSER |
      CLONE_NEWCGROUP | SIGCHLD,
      ...);
```

Then in the child:
1. Set up the rootfs (overlayfs of the image layers).
2. `pivot_root()` into it.
3. Mount `/proc`, `/sys`, `/dev`, etc.
4. Drop capabilities.
5. Apply seccomp filter.
6. `execve("/bin/sh", ...)`.

Now you have a process whose view of "the system" is entirely contained — its own PID 1, its own filesystem, its own network — but running on the host kernel like any other process.

Prove it: from another terminal, `ps -ef` on the host will show the `sh` process, just with a host PID like 24917. Inside, that same process sees itself as PID 1.

---

## 2. Namespaces — the isolation primitives

A **namespace** isolates a kernel resource so that processes in one namespace see a different view than processes in another. Linux has eight:

| Namespace | What it isolates | Visible via |
|---|---|---|
| **mnt** | Mount points (filesystem view) | `cat /proc/$$/mounts` |
| **pid** | Process IDs (each ns has its own PID 1) | `ps`, `kill` |
| **net** | Network stack: interfaces, routing, iptables | `ip link`, `ss` |
| **ipc** | SysV IPC, POSIX message queues | `ipcs` |
| **uts** | Hostname and domain name | `hostname` |
| **user** | UID/GID mappings | `id`, `cat /proc/$$/uid_map` |
| **cgroup** | View of cgroup hierarchy | `cat /proc/$$/cgroup` |
| **time** | CLOCK_MONOTONIC/BOOTTIME offsets | `date` etc. (newest, less used) |

A container is just a process with private versions of these. Look at one:

```bash
$ docker run -d --name demo alpine sleep 1000
$ docker inspect --format '{{.State.Pid}}' demo
24917
$ sudo ls -la /proc/24917/ns
total 0
lrwxrwxrwx 1 root root 0 May 13 09:32 cgroup -> 'cgroup:[4026532789]'
lrwxrwxrwx 1 root root 0 May 13 09:32 ipc    -> 'ipc:[4026532787]'
lrwxrwxrwx 1 root root 0 May 13 09:32 mnt    -> 'mnt:[4026532785]'
lrwxrwxrwx 1 root root 0 May 13 09:32 net    -> 'net:[4026532790]'
lrwxrwxrwx 1 root root 0 May 13 09:32 pid    -> 'pid:[4026532788]'
lrwxrwxrwx 1 root root 0 May 13 09:32 user   -> 'user:[4026531837]'
lrwxrwxrwx 1 root root 0 May 13 09:32 uts    -> 'uts:[4026532786]'
```

The inode numbers in brackets are the namespace IDs. Same number = same namespace. Compare against your shell:

```bash
$ sudo ls -la /proc/self/ns | grep mnt
lrwxrwxrwx 1 root root 0 May 13 09:32 mnt -> 'mnt:[4026531840]'
```

Different inode → different mount namespace → why your shell sees `/usr` and the container sees `/usr` differently.

Enter a namespace from the host (powerful debugging trick):

```bash
sudo nsenter -t 24917 -m -p -n -u sh
# Now this shell is "inside" the container without using docker exec
```

`docker exec` is essentially a friendlier wrapper around this.

### PID namespace — why your container has a PID 1

Inside the container:

```bash
$ docker exec demo ps
PID   USER     TIME  COMMAND
    1 root      0:00 sleep 1000
   10 root      0:00 ps
```

`sleep` is PID 1. There's no `init`, no `systemd`. This has consequences:

- **PID 1 has special signal handling.** Default action for any signal (except SIGKILL/SIGSTOP) is *ignored*. Your app, as PID 1, won't get SIGTERM correctly unless it explicitly installs a handler. Most languages' runtimes handle this; some don't (old Bash scripts especially).
- **Zombie reaping is PID 1's job.** If your app spawns child processes that exit and never `wait()`s on them, they accumulate as zombies. Tools like `tini` or `dumb-init` provide a tiny init that reaps zombies. Use them via `docker run --init` (which injects `tini` automatically) or in your `ENTRYPOINT`.
- **`kill 1` inside the container** kills the container (PID 1 dies → the entire PID namespace is destroyed → all other processes in it die too).

---

## 3. cgroups — the resource accounting & limits

Namespaces give isolation. **Control groups (cgroups)** give resource limits and accounting. cgroup v2 is the modern unified hierarchy (most distros default to it now).

cgroups limit/account: **CPU, memory, block I/O, network (with helpers), and PIDs**.

```bash
# Set limits at run time
docker run -d --name limited \
  --cpus="1.5" \
  --memory="512m" --memory-swap="512m" \
  --pids-limit=100 \
  myapp

# Inspect the cgroup configuration
$ docker inspect limited --format '{{json .HostConfig}}' | jq '{NanoCpus, Memory, PidsLimit}'
{
  "NanoCpus": 1500000000,
  "Memory": 536870912,
  "PidsLimit": 100
}

# Or peek at the actual cgroup pseudo-fs (cgroup v2):
$ cat /sys/fs/cgroup/system.slice/docker-<id>.scope/memory.max
536870912
$ cat /sys/fs/cgroup/system.slice/docker-<id>.scope/cpu.max
150000 100000      # 1.5 CPU = 150ms out of every 100ms slice
```

**Memory limits in particular:** exceeding `memory.max` triggers the **OOM killer**, which kills the offender (almost always your container's main process). Container exits with status 137 (= 128 + SIGKILL 9). When you see exit 137, check `dmesg` for `Memory cgroup out of memory`.

```bash
$ docker run --rm --memory=10m python:3.12-slim python -c "x = bytearray(50_000_000)"
$ echo $?
137
$ dmesg | tail -3
[ ... ] Memory cgroup out of memory: Killed process 24917 (python)
```

`docker stats` shows live cgroup numbers per container:

```
$ docker stats --no-stream
CONTAINER   CPU %    MEM USAGE / LIMIT     MEM %    NET I/O          PIDS
demo        0.00%    412KiB / 512MiB       0.08%    1.2kB / 0B       1
api         12.5%    180MiB / 1GiB         17.6%    5.2MB / 1.1MB    24
```

### Why "set limits" matters

A container with no memory limit can eat all the host's RAM and trigger the *host's* OOM killer, which may kill *other* containers (or even system services). Set memory limits on production containers. Always.

CPU limits use a quota system, not pinning — `--cpus=1.5` doesn't pin to specific cores, it allots 150% of one core's worth of CPU time per period. For real pinning use `--cpuset-cpus="0-3"`.

---

## 4. The runtime stack — dockerd → containerd → runc

`dockerd` is the friendly front-end. The actual *running* of a container is delegated to lower layers:

```
┌──────────────┐
│   docker CLI │  user
└──────┬───────┘
       │ REST API (/var/run/docker.sock)
┌──────▼───────┐
│   dockerd    │  high-level daemon: builds, networks, volumes
└──────┬───────┘
       │ gRPC
┌──────▼───────┐
│  containerd  │  container lifecycle, image pull, content store
└──────┬───────┘
       │ creates per-container...
┌──────▼───────┐
│ containerd-  │  one per container; survives containerd restart
│   shim       │
└──────┬───────┘
       │ execs...
┌──────▼───────┐
│    runc      │  the low-level OCI runtime; one-shot — clone+exec, exit
└──────────────┘
```

- **dockerd** does Docker-flavored stuff: image building, networks (bridges/iptables), volume drivers, the friendly CLI API. Removing it leaves you with no `docker build`, but containers keep running.
- **containerd** is the workhorse. CNCF graduated, used directly by Kubernetes (no `dockerd` involved in modern K8s). Tracks images, manages container lifecycle, talks to `runc` via shims.
- **containerd-shim** sits between containerd and runc. One per container. It's what allows you to restart `containerd` without killing your running containers — the shim is the parent process holding the container's stdin/stdout.
- **runc** is the OCI reference runtime. Reads an OCI runtime spec JSON, calls `clone()` with the right flags, sets up namespaces/cgroups/mounts/caps/seccomp, `execve`s the command, exits. It's literally the program that "creates a container."

Alternatives at each level:
- Lower runtime: **crun** (faster, smaller, written in C), **youki** (Rust), **gVisor** (a userspace kernel for stronger isolation), **Kata Containers** (lightweight VMs per container).
- Higher daemon: **podman** (daemonless, drop-in `docker` CLI), **CRI-O** (purpose-built for Kubernetes).

Knowing this stack helps when things go wrong: a hung container that won't `docker stop` might still respond to `runc kill` directly; a daemon crash doesn't mean your containers stopped (because the shim holds them).

---

## 5. The OCI specs — the standards behind it all

The Open Container Initiative defines three specs that *everyone* in the container ecosystem agrees on:

1. **Image Spec** — what an image is on disk and in a registry. JSON manifests, layer tarballs, config files. This is why `docker pull`, `podman pull`, and `crictl pull` all read the same images.
2. **Runtime Spec** — what `runc` (or any compliant runtime) takes as input. A `config.json` describing namespaces, capabilities, mounts, hooks, the command to run.
3. **Distribution Spec** — the registry HTTP API (formerly Docker Registry V2). Why every registry — Docker Hub, GHCR, ECR — speaks the same protocol.

See an OCI runtime config in the wild:

```bash
$ runc spec     # generates a sample config.json
$ cat config.json | jq '.process, .linux.namespaces'
```

```json
{
  "process": {
    "user": { "uid": 0, "gid": 0 },
    "args": ["sh"],
    "env": ["PATH=/usr/local/sbin:..."],
    "capabilities": { "bounding": ["CAP_AUDIT_WRITE", ...] },
    "noNewPrivileges": true
  },
  "namespaces": [
    {"type": "pid"},
    {"type": "network"},
    {"type": "ipc"},
    {"type": "uts"},
    {"type": "mount"}
  ]
}
```

That JSON is what `runc` reads to construct a container. `dockerd`/`containerd` build this JSON from your `docker run` flags. So when you run `docker run --cap-drop=ALL`, what you're really doing is editing the `capabilities` array in the runtime spec.

Understanding this opens up an interesting trick: you can build images with one tool, run them with another. A Dockerfile-built image can be loaded by `podman`, `nerdctl`, or even hand-driven `runc`. The image is the universal artifact; the runtime is interchangeable.

---

## 6. A worked deep-dive: trace a `docker run` end-to-end

Let's do `docker run --rm --memory 64m alpine sh -c 'echo hi'` and watch what really happens.

**Step 1: client → daemon.** `docker` CLI sends a POST to `/containers/create` with the spec, then `/start`.

```bash
$ strace -e trace=connect docker run --rm alpine echo hi 2>&1 | head -3
connect(3, {sa_family=AF_UNIX, sun_path="/var/run/docker.sock"}, ...) = 0
```

**Step 2: daemon → containerd.** dockerd makes a gRPC call to containerd's API to create a container.

**Step 3: containerd → image fetch.** containerd already has `alpine` in its content store. It composes the rootfs by stacking the alpine layer (via overlay2) into a directory under `/var/lib/containerd/`.

**Step 4: containerd → shim → runc.** containerd spawns `containerd-shim-runc-v2`, which spawns `runc create`. `runc` writes a `config.json`, calls `clone()` with the namespace flags, sets up the cgroup (writing `64m` to `memory.max`), runs the `prestart` hooks (CNI-style networking for Docker), and execs `/bin/sh -c 'echo hi'`.

**Step 5: child runs.** `echo hi` writes "hi" to stdout. Stdout is the shim's pipe → containerd → dockerd → your terminal.

**Step 6: child exits.** SIGCHLD bubbles back to the shim. Shim tells containerd, containerd tells dockerd, dockerd tells you exit code 0 and (because `--rm`) removes the container metadata.

Total elapsed: ~200ms. Most of it is the inter-process round trips, not the actual `clone()` (which is essentially free).

You can see this hierarchy in `ps`:

```bash
$ docker run -d --name demo alpine sleep 60
$ pstree -p $(pidof dockerd)
dockerd(1234)─┬─containerd(1245)─┬─containerd-shim(24917)─┬─sleep(24938)
              └─...
```

(In practice the shim is parented to PID 1, not containerd, so it survives containerd restarts. The hierarchy in `pstree` may show this differently depending on distro.)

---

## 7. Common mistakes & gotchas

- **App as PID 1 ignoring SIGTERM.** Default signal handling for PID 1 ignores most signals. Use `docker run --init`, or `tini`/`dumb-init` in your `ENTRYPOINT`, or make sure your runtime installs proper handlers.
- **Zombies accumulating.** A multi-process container without a proper init reaps no children. Symptoms: weird hangs, FD exhaustion, `ps` showing `<defunct>`. Same fix: `--init` or a tiny init binary.
- **No memory limit on a container that leaks.** Container eats host RAM, host OOM killer murders something important (maybe sshd). Always set `--memory` on prod containers.
- **Exit code 137 confusion.** That's `128 + SIGKILL(9)`. Almost always OOM killed by the memory cgroup. Check `dmesg`.
- **Exit code 139.** That's `128 + SIGSEGV(11)`. Your app crashed; investigate the app, not Docker.
- **Trying to run systemd inside a container.** Possible but painful; systemd expects a lot of host privileges. For workloads needing systemd, you probably want a VM. For multi-process containers, use `supervisord` or split into multiple containers + Compose.
- **`docker exec` "doesn't show" a process you know is running.** `docker exec`'s `ps` is in the container's PID namespace — only sees container processes. From the *host*, `ps -ef` sees them all with host PIDs.
- **Bind-mounting `/var/run/docker.sock` "for convenience."** Anyone inside that container can issue Docker API calls — meaning they can spawn a privileged container with `-v /:/host`. Full host root, instantly. Treat `docker.sock` like the master key it is (Module 13).
- **Confusing "container runtime" with "container engine."** runc is the runtime. containerd is the engine/manager. dockerd is a user-facing wrapper around the engine. Knowing which is which speeds up troubleshooting.
- **Assuming cgroups v1.** Many older articles assume cgroup v1 paths (`/sys/fs/cgroup/memory/...`). Modern Linux (RHEL 9+, Ubuntu 22.04+, Fedora 31+) uses cgroup v2 with a unified hierarchy.

---

## 🎯 Key Takeaways

- **A container is a Linux process** with private namespaces, cgroup limits, dropped caps, and a seccomp filter. There is no special "container kernel" — it's all features of the host kernel.
- **Eight namespaces** isolate views; **cgroups** enforce limits and account resources. Together they constitute "containerization."
- **PID 1 is special.** Use `--init` or a tiny init to handle signals and reap zombies; don't run your app as PID 1 without thinking.
- **dockerd → containerd → shim → runc** is the runtime hierarchy. Knowing it lets you debug daemon crashes (containers survive!) and pick alternatives (podman, crun, gVisor) when needed.
- **The OCI specs** make images and runtimes interchangeable. Your `docker build` artifact runs identically under Docker, Podman, containerd, Kubernetes — that universality is intentional, standardized, and worth respecting.

*[prev ← 11_multiarch_builds](./11_multiarch_builds.md) | [next → 13_security](./13_security.md)*
