# 00 — Docker Deep-Dive Roadmap

> **Goal:** Take a working developer from "I've run `docker run`" to confidently building, shipping, and securing containerized workloads in production — and primed to step into Kubernetes.

---

## Who this is for

You write code for a living. You've probably typed `docker run hello-world` at some point. You can navigate a Linux shell, know what a process is, and can read a Dockerfile without panicking. You want the *real* model — not just commands, but **why containers work the way they do**, so that when something breaks at 2am you can reason about it instead of cargo-culting Stack Overflow answers.

This course is professional upskilling material. It's opinionated, command-heavy, and unapologetically Linux-centric (Docker Desktop on macOS / Windows is covered, but the truth is on Linux).

---

## Module table

| #  | File | Topic | Why it matters |
|----|------|-------|----------------|
| 01 | [`01_intro_and_setup.md`](./01_intro_and_setup.md) | Containers vs VMs, Docker architecture, install | Foundational mental model |
| 02 | [`02_images_and_containers.md`](./02_images_and_containers.md) | `pull/run/exec/stop/rm`, layers, lifecycle | Day-1 fluency |
| 03 | [`03_dockerfile_fundamentals.md`](./03_dockerfile_fundamentals.md) | `FROM`, `RUN`, `COPY`, `CMD` vs `ENTRYPOINT` | You can't ship without this |
| 04 | [`04_building_images.md`](./04_building_images.md) | `docker build`, context, `.dockerignore`, tags | Builds that aren't 3 GB |
| 05 | [`05_layer_caching_multistage.md`](./05_layer_caching_multistage.md) | Cache ordering, BuildKit, multi-stage | Fast, lean images |
| 06 | [`06_volumes_and_mounts.md`](./06_volumes_and_mounts.md) | Volumes, bind mounts, tmpfs, ownership | Don't lose data |
| 07 | [`07_networking.md`](./07_networking.md) | Bridge, host, none, DNS, port publishing | How containers talk |
| 08 | [`08_docker_compose.md`](./08_docker_compose.md) | Services, networks, profiles, healthchecks | Multi-container apps |
| 09 | [`09_registries_distribution.md`](./09_registries_distribution.md) | Hub, GHCR, ECR/ACR/GAR, auth | Shipping artifacts |
| 10 | [`10_image_optimization.md`](./10_image_optimization.md) | Distroless, alpine, scratch, scanning | Size + security |
| 11 | [`11_multiarch_builds.md`](./11_multiarch_builds.md) | `buildx`, QEMU, manifests | amd64 + arm64 |
| 12 | [`12_runtime_internals.md`](./12_runtime_internals.md) | Namespaces, cgroups, OCI, runc | What's *really* happening |
| 13 | [`13_security.md`](./13_security.md) | Rootless, caps, seccomp, cosign, SBOM | Don't ship CVEs |
| 14 | [`14_observability.md`](./14_observability.md) | Logs, healthchecks, stats, metrics | Production visibility |
| 15 | [`15_cicd_with_docker.md`](./15_cicd_with_docker.md) | Pipelines, caching, scanning gates | Automating builds |
| 16 | [`16_production_and_next.md`](./16_production_and_next.md) | Compose vs Swarm vs K8s, footguns | The transition |

---

## Timeline

One module per day is the sweet spot — **~2.5 weeks** to finish. Each module is 30-60 minutes reading plus 30-90 minutes of hands-on work. Don't skip the hands-on parts; Docker is a tactile subject and your fingers need to learn the commands.

| Pace | Duration | Profile |
|------|----------|---------|
| 1 module/day | 16 days | Recommended — sustainable, retention sticks |
| 2 modules/day | 8 days | Crunch mode (job interview next week) |
| 1 module/week | 4 months | Background pace alongside other work |

A good completion test: at the end, you can take an unfamiliar repo, write a multi-stage Dockerfile for it, push to GHCR via GitHub Actions, and deploy it with Compose — without Googling.

---

## Prerequisites

- **Linux CLI basics** — `cd`, `ls`, `cat`, `grep`, pipes, `ps`, `kill`, `chmod`. If `sudo` and PATH are foreign, do a quick Linux intro first.
- **Any one programming language** — examples use Python, Node.js, and Go interchangeably. You don't need to know all three; just be able to read them.
- **Git** — you'll clone example repos and read Dockerfiles in the wild.
- **A machine that can run Docker** — Linux native is best, but Docker Desktop on macOS or Windows (with WSL2) is fine. Module 01 covers setup.

You do **not** need to know: Kubernetes, cloud providers, Go internals, or "DevOps." Those grow naturally out of containers — don't put the cart before the horse.

---

## Core mental models

If you forget every command in this course but internalize these six ideas, you'll still be ahead of 80% of working developers:

### 1. Containers are processes, not VMs
A container is a Linux process (or process tree) running on your host kernel, isolated by **namespaces** and constrained by **cgroups**. There is no guest OS. `docker run nginx` is closer to `nginx &` than it is to "boot a virtual machine." Internalize this and half of Docker stops being magic.

### 2. Images are layered, immutable, content-addressable
An image is a stack of read-only filesystem layers identified by SHA-256 hashes. Layers are shared between images, cached aggressively, and never modified — when you "change" an image, you build a new one on top. This is why image storage is efficient and rebuilds are fast (when you order your Dockerfile correctly).

### 3. The build context matters
`docker build .` ships the entire current directory to the daemon. If your `.git` folder is 4 GB and you forgot `.dockerignore`, your build is now 4 GB slower. The context is **not** "where Docker reads files from at runtime" — it's "the tarball Docker uploads to the build engine before it can do anything."

### 4. One process per container (by convention)
You *can* run init, cron, sshd, and your app in one container. You *shouldn't*. The convention is one container = one concern, because it makes logs sane, restarts cheap, scaling sensible, and lifecycle obvious. If you need multiple processes, you probably want Compose, not a fatter container.

### 5. Networking is just iptables underneath
Docker's bridge networks, port publishing, and DNS are implemented with Linux bridges, veth pairs, iptables NAT rules, and an embedded DNS server. There's no special "Docker network protocol." If you can read `iptables -t nat -L`, you can debug any Docker networking issue.

### 6. Secrets are not environment variables
`ENV API_KEY=...` in a Dockerfile bakes the secret into a layer **forever**, visible to anyone with `docker pull` access. Compose `environment:` blocks land in process listings. Real secrets go through Docker secrets, runtime mounts, or your platform's secret manager. The whole industry learns this lesson the hard way; you can skip the hard way.

---

## External resources

Bookmark these. The official docs are surprisingly good and you'll come back to them constantly.

- **[docs.docker.com](https://docs.docker.com/)** — the canonical reference; Dockerfile spec and CLI docs are first-rate.
- **[Docker Deep Dive](https://nigelpoulton.com/books/docker-deep-dive/) by Nigel Poulton** — the book this course is spiritually descended from; great paired reading.
- **[Play with Docker](https://labs.play-with-docker.com/)** — free in-browser Docker playground; useful when your machine can't run Docker (corporate laptops, etc).
- **[OCI Specifications](https://github.com/opencontainers/runtime-spec)** — Image Spec, Runtime Spec, Distribution Spec. Skim once; reread when something weird happens.
- **[BuildKit documentation](https://docs.docker.com/build/buildkit/)** — the modern build engine; everything in Module 05+ relies on it.
- **[Docker Captains](https://www.docker.com/captains/)** — community experts whose blogs and talks are gold. Bret Fisher and Nigel Poulton in particular.

---

## What comes after this course

Docker is the foundation. Once you've finished Module 16, the natural next mountain is **orchestration**: how do you run hundreds of these containers across a fleet of servers, with health checks, autoscaling, rolling updates, and self-healing?

That's Kubernetes. We've intentionally drawn the line at "production-ready single-host Docker + Compose" in this course because Kubernetes is its own beast. When you're ready:

→ **[`../kubernetes/00_roadmap.md`](../kubernetes/00_roadmap.md)** *(companion course)*

You'll find that 80% of your Docker knowledge transfers directly — pods are still containers, images still come from registries, networking is still namespaces. Kubernetes adds orchestration on top; it doesn't replace what you've learned here.

---

## A closing word on professional upskilling

Containers are the most leveraged piece of infrastructure knowledge you can pick up as a developer in this decade. Cloud, CI/CD, microservices, edge compute, ML serving — they all run on containers. Becoming the person on your team who *actually understands* Docker (not just types commands) pays back compounding interest for years.

Don't just read the modules. Build something. Break it. Read the error. Fix it. That's the loop.

Let's go.

*[next → 01_intro_and_setup](./01_intro_and_setup.md)*
