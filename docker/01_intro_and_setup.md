# 01 — Intro & Setup
> **Goal:** Build a correct mental model of what containers are (and aren't), understand Docker's client-daemon-registry architecture, and have a working Docker installation you can run commands on for the rest of the course.

---

## 1. Containers vs VMs — the right mental model

Forget every diagram you've seen with "Container OS" stacked on top of a "Container Engine." Most of them are wrong, or at best misleading.

**A container is a Linux process** running on your host kernel, isolated from other processes by Linux kernel features (namespaces and cgroups, which we'll cover in depth in Module 12). That's it. There is no guest operating system. There is no hypervisor.

Compare:

```
┌─────────────────────────────┐      ┌─────────────────────────────┐
│         VM model            │      │     Container model         │
├─────────────────────────────┤      ├─────────────────────────────┤
│  App A  │  App B  │  App C  │      │  App A  │  App B  │  App C  │
│  libs   │  libs   │  libs   │      │  libs   │  libs   │  libs   │
│ Guest OS│ Guest OS│ Guest OS│      │       (shared kernel)       │
│        Hypervisor           │      │       Host Linux OS         │
│       Host OS / HW          │      │          Hardware           │
└─────────────────────────────┘      └─────────────────────────────┘
```

A VM boots a whole OS. A container shares the host kernel and just runs its own process tree in an isolated view of the world.

**Try it right now:**

```bash
docker run --rm alpine ps aux
```

Output:

```
PID   USER     TIME  COMMAND
    1 root      0:00 ps aux
```

Inside the container, PID 1 is `ps`. The container literally *is* this process. When `ps` exits, the container exits. No kernel boot, no init system, no sshd — just a process with a private filesystem view, network stack, and PID namespace.

| Aspect          | VM                 | Container               |
|-----------------|--------------------|-------------------------|
| Boot time       | 10s-60s            | <1s                     |
| Disk overhead   | GB per VM (OS)     | MB per container        |
| Kernel          | One per VM         | Shared (host)           |
| Isolation       | Strong (hardware)  | Strong (kernel-enforced)|
| Density         | ~10s per host      | ~100s-1000s per host    |

Containers are **lighter** because they don't duplicate the OS. They're also **less isolated** in one specific way: a kernel exploit can theoretically escape a container, where it would be stopped by a hypervisor boundary in a VM. For 99% of workloads this trade-off is fine; for hostile multi-tenant workloads it isn't (use VMs, or gVisor / Kata Containers).

---

## 2. Docker's architecture — daemon, client, registry

Docker isn't one program. It's three concepts you'll deal with daily:

```
   ┌──────────────┐    REST API   ┌────────────────────┐
   │ docker CLI   │ ────────────▶ │  dockerd (daemon)  │
   │ (client)     │    /var/run/  │                    │
   └──────────────┘    docker.sock│  ┌──────────────┐  │
                                  │  │ containerd   │  │
                                  │  └──────┬───────┘  │
                                  │         │          │
                                  │  ┌──────▼───────┐  │
                                  │  │   runc       │  │
                                  │  └──────────────┘  │
                                  └────────┬───────────┘
                                           │ pull/push
                                           ▼
                                  ┌────────────────────┐
                                  │  Registry          │
                                  │  (Hub, GHCR, ECR)  │
                                  └────────────────────┘
```

- **Docker client (`docker`)** — the CLI you type into. It talks to the daemon over a Unix socket (`/var/run/docker.sock` on Linux) or TCP.
- **Docker daemon (`dockerd`)** — long-running background process. Receives API calls, manages images, networks, volumes, and containers. Delegates actual container lifecycle to **containerd**, which delegates the *actual actual* `clone()`+`exec()` to **runc** (Module 12).
- **Registry** — a server that stores and serves images. Docker Hub is the default public one; GHCR, ECR, ACR, GAR, Quay, Harbor are popular alternatives (Module 09).

The client and daemon don't have to be on the same machine. `DOCKER_HOST=tcp://remote:2376 docker ps` queries a remote daemon. Docker Desktop on macOS/Windows uses this trick — the daemon runs inside a tiny Linux VM and your `docker` CLI talks to it across the VM boundary.

**Inspect your setup:**

```bash
docker version
```

Example output:

```
Client: Docker Engine - Community
 Version:    25.0.3
 API version: 1.44
 OS/Arch:    linux/amd64

Server: Docker Engine - Community
 Engine:
  Version:    25.0.3
  API version: 1.44 (minimum 1.24)
  OS/Arch:    linux/amd64
 containerd:
  Version:    1.7.13
 runc:
  Version:    1.1.12
```

Two halves: Client (your CLI) and Server (the daemon). Note containerd and runc shown explicitly — these are real, separate binaries.

---

## 3. Docker Desktop vs Docker Engine

This trips up beginners constantly. They are different things with different licenses and different operational behaviors.

| | Docker Desktop | Docker Engine |
|---|---|---|
| **Platforms** | macOS, Windows, Linux | Linux only |
| **What it is** | GUI app + bundled VM + CLI + Compose + Kubernetes | The daemon (`dockerd`) and CLI |
| **License** | Free for personal / small business; **paid** for orgs >250 employees or >$10M revenue | Apache 2.0, fully free |
| **Daemon runs on** | A managed Linux VM (HyperKit/WSL2/Hyper-V) | The host directly |
| **GUI** | Yes | No |
| **K8s included** | Yes, optional toggle | No (install separately) |
| **Updates** | Auto, bundled | OS package manager |

**Pick:**
- **Linux developer or server** → Docker Engine.
- **macOS or Windows laptop** → Docker Desktop is the path of least resistance. On Windows specifically, install WSL2 first.
- **Org with >250 employees** → check licensing; alternatives exist (Rancher Desktop, Colima, Podman Desktop, OrbStack).

A subtle gotcha: **filesystems perform differently**. On Docker Desktop macOS, bind mounts from your host filesystem (`/Users/...`) into a Linux container cross a VM boundary and a 9p / virtiofs layer. This is *much* slower than native Linux bind mounts. For dev loops with heavy file I/O (Node.js, Rails), this is the #1 performance killer. Module 06 covers fixes.

---

## 4. Install and verify

### Linux (Ubuntu/Debian)

```bash
# Remove old versions if any
sudo apt remove docker docker-engine docker.io containerd runc

# Install via the official convenience script (or use the longer apt repo method)
curl -fsSL https://get.docker.com | sudo sh

# Allow your user to run docker without sudo (logout/login after)
sudo usermod -aG docker $USER

# Verify
docker run --rm hello-world
```

Expected output snippet:

```
Hello from Docker!
This message shows that your installation appears to be working correctly.

To generate this message, Docker took the following steps:
 1. The Docker client contacted the Docker daemon.
 2. The Docker daemon pulled the "hello-world" image from the Docker Hub.
 3. The Docker daemon created a new container from that image which runs the
    executable that produces the output you are currently reading.
 4. The Docker daemon streamed that output to the Docker client, which sent it
    to your terminal.
```

That output is itself a description of the architecture from §2. Read it.

### macOS

Download Docker Desktop from [docker.com/products/docker-desktop](https://www.docker.com/products/docker-desktop). Install. Open. Wait for the whale icon to stop animating. Then:

```bash
docker run --rm hello-world
```

### Windows

1. Install **WSL2** first (`wsl --install` in an admin PowerShell, reboot).
2. Install Docker Desktop, which auto-detects WSL2.
3. In Docker Desktop settings → Resources → WSL Integration, enable your distro.
4. From your WSL2 terminal:

```bash
docker run --rm hello-world
```

**Important:** run `docker` commands from inside WSL2, not from `cmd.exe` or PowerShell. The performance and ergonomics are night-and-day better.

### Sanity check — the four things to verify

```bash
docker info               # daemon is reachable
docker run --rm alpine echo hi   # can pull and run
docker run --rm -v $(pwd):/host alpine ls /host  # bind mounts work
docker run --rm -p 8080:80 -d nginx && curl localhost:8080   # networking works
```

If all four succeed, you're done.

---

## 5. Common mistakes & gotchas

- **Running `docker` with `sudo` forever.** Add your user to the `docker` group once (`sudo usermod -aG docker $USER`, logout/login). But understand: the `docker` group is effectively root — anyone in it can mount the host root filesystem into a container and chroot. Treat it like sudo.
- **Confusing "container" with "image."** An image is the template (a tarball of layers); a container is a running instance of that image. You can have 100 containers from 1 image. Module 02 hammers this.
- **Expecting `docker run` to be idempotent.** Each `docker run` creates a *new* container. Run it 10 times, you have 10 containers (some stopped). Use `docker rm` and `--rm`.
- **Mac/Windows users assuming Docker == Linux.** Your daemon is in a VM. Paths like `/var/lib/docker` are inside the VM, not on your Mac filesystem. `docker exec` works; `cd /var/lib/docker` on your Mac won't.
- **`hello-world` succeeds but a real image fails.** Almost always a corporate proxy or DNS issue. Check `~/.docker/config.json` and the daemon's `http-proxy` settings (`/etc/docker/daemon.json` on Linux, Settings → Resources → Proxies on Desktop).
- **The "is the daemon running?" error.** `Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?` On Linux: `sudo systemctl start docker`. On Desktop: open the app. On WSL2: ensure Docker Desktop's WSL integration is enabled for your distro.

---

## 🎯 Key Takeaways

- **A container is a Linux process, not a tiny VM.** Internalize this; it makes everything else click.
- **Docker is three things:** a CLI client, a daemon (`dockerd` → `containerd` → `runc`), and a registry. Knowing which is failing speeds up debugging by 10x.
- **Docker Desktop ≠ Docker Engine.** Same CLI, very different operational profile (VM, license, filesystem performance). Pick deliberately based on platform and org size.
- **Membership in the `docker` group is equivalent to root** on the host. Treat container access as a privilege, not a convenience.
- **Verify with four commands** (`info`, basic `run`, bind mount, port publish) any time you set up a new machine — these catch 95% of misconfigurations early.

*[prev ← 00_roadmap](./00_roadmap.md) | [next → 02_images_and_containers](./02_images_and_containers.md)*
