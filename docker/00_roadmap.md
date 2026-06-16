# Docker Deep Dive — Interview Prep Course

> **By the end of this course**, you'll understand Docker from the ground up — how containers work, how to build and ship images, manage networks, volumes, multi-container apps, security, CI/CD integration, and orchestration. You'll be ready to answer any Docker question an interviewer throws at you.

---

## Module Table

| # | Title | Key Focus Areas |
|---|-------|----------------|
| 01 | Introduction to Containerization | VMs vs containers, why Docker, the Linux primitives underneath, Docker architecture |
| 02 | Installing & Setting Up Docker | Docker Desktop, Docker Engine on Linux, verifying setup, daemon config |
| 03 | Images & Containers | What an image is, layers, `docker run`, container lifecycle, key CLI commands |
| 04 | Dockerfile Fundamentals | FROM, RUN, COPY, CMD, ENTRYPOINT, ENV, EXPOSE, ARG |
| 05 | Advanced Dockerfiles | Multi-stage builds, layer caching, .dockerignore, image size optimization |
| 06 | Volumes & Persistent Storage | Bind mounts vs volumes vs tmpfs, `docker volume`, data persistence patterns |
| 07 | Docker Networking | Bridge, host, overlay, none networks; port mapping; DNS; custom networks |
| 08 | Docker Compose | `docker-compose.yml`, services, depends_on, health checks, profiles |
| 09 | Registry & Image Management | Docker Hub, private registries, tagging, pushing/pulling, image pruning |
| 10 | Container Lifecycle & Resource Management | Start/stop/restart policies, CPU/memory limits, health checks, `docker stats` |
| 11 | Docker Security | Least privilege, non-root users, read-only filesystems, secrets management, scanning |
| 12 | Docker in CI/CD | GitHub Actions, GitLab CI, build caching in pipelines, multi-arch builds |
| 13 | Docker Swarm | Swarm mode, services vs containers, replicas, rolling updates, stacks |
| 14 | Introduction to Kubernetes | How K8s relates to Docker, pods, deployments, services, why K8s replaced Swarm |
| 15 | Monitoring & Logging | `docker logs`, log drivers, cAdvisor, Prometheus, Grafana integration |
| 16 | Docker Best Practices & Patterns | 12-factor apps, sidecar pattern, init containers, distroless images |
| 17 | Real-world Project: Containerizing a Full-stack App | Node.js API + PostgreSQL + Nginx, Compose, production vs dev configs |
| 18 | Interview Q&A — 60 Common Docker Questions | Grouped by topic: concepts, Dockerfile, networking, security, orchestration |

---

## Suggested Timeline

| Pace | Schedule |
|------|----------|
| Intensive | 2 modules/day → done in **9 days** |
| Comfortable | 1 module/day → done in **18 days** |
| Part-time | 3 modules/week → done in **6 weeks** |

**Recommendation for interview prep:** 1 module/day + spend the last 2 days reviewing Module 18 and practicing answers out loud.

---

## How to Use This Course

1. **Start with Module 01** even if you've seen Docker before — the conceptual foundation matters for interviews.
2. **Type the commands yourself.** Don't just read — open a terminal and run everything. Muscle memory counts.
3. **Each module ends with interview Q&A.** Read them, then close the file and try to answer from memory.
4. **Module 18** is your final review — use it 1–2 days before your interview as a rapid-fire drill.
5. **Module 17** (the project) should be built locally so you have something concrete to talk about.

---

## Prerequisites & Setup

**What you should know first:**
- Basic command-line comfort (cd, ls, cat, grep)
- A general idea of what a "server" is
- Basic understanding of what an application is (no coding required to start)

**What to install:**
- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (Mac/Windows) — includes Docker Engine + Compose
- On Linux: `curl -fsSL https://get.docker.com | sh` then `sudo usermod -aG docker $USER`
- Verify: `docker --version` and `docker compose version`
- Optional: [VS Code Docker extension](https://marketplace.visualstudio.com/items?itemName=ms-azuretools.vscode-docker)

---

## Core Mental Models

These 5 ideas run through the entire course. Return to them whenever something feels confusing.

1. **Images are blueprints; containers are running instances.** Like a class vs an object in OOP — the image is immutable, containers are ephemeral by default.

2. **Every Docker layer is a diff.** Images are built as a stack of read-only layers. The container adds one writable layer on top. This is why image size and layer count matter.

3. **Containers are isolated Linux processes.** Not VMs. Docker uses Linux kernel features — namespaces (isolation) and cgroups (resource limits) — to make a regular process feel like its own machine.

4. **The Docker daemon does the heavy lifting.** The `docker` CLI is just a client that sends commands to the daemon (`dockerd`) via a Unix socket. The daemon manages containers, images, networks, and volumes.

5. **Networking is just virtual switches.** Docker creates virtual networks (bridges) that containers connect to. Port mapping is NAT from the host into a container's network namespace.

---

## External Resources

- 📖 [Official Docker Docs](https://docs.docker.com/) — the canonical reference, keep it open
- 🎓 [Play with Docker](https://labs.play-with-docker.com/) — free browser-based Docker playground, no install needed
- 📘 [Docker Deep Dive (book) — Nigel Poulton](https://nigelpoulton.com/books/) — the clearest Docker book for beginners
- 🎥 [TechWorld with Nana — Docker Tutorial](https://www.youtube.com/watch?v=3c-iBn73dDE) — popular 3-hour video walkthrough
- 🛠 [Dockerfile Best Practices](https://docs.docker.com/develop/develop-images/dockerfile_best-practices/) — official guide, essential reading before interviews
- 🔍 [dive](https://github.com/wagoodman/dive) — CLI tool to inspect image layers, great for optimization

---

Good luck with the interviews! Docker is one of the highest-signal skills you can demonstrate — knowing it well (not just the commands, but *why* it works) sets you apart. Let's go.

---

### Modules
- [01 — Introduction to Containerization](./01_introduction_containerization.md)
- [02 — Installing & Setting Up Docker](./02_installing_docker.md)
- [03 — Images & Containers](./03_images_containers.md)
- [04 — Dockerfile Fundamentals](./04_dockerfile_fundamentals.md)
- [05 — Advanced Dockerfiles](./05_advanced_dockerfiles.md)
- [06 — Volumes & Persistent Storage](./06_volumes_storage.md)
- [07 — Docker Networking](./07_networking.md)
- [08 — Docker Compose](./08_docker_compose.md)
- [09 — Registry & Image Management](./09_registry_image_management.md)
- [10 — Container Lifecycle & Resource Management](./10_lifecycle_resources.md)
- [11 — Docker Security](./11_security.md)
- [12 — Docker in CI/CD](./12_cicd.md)
- [13 — Docker Swarm](./13_swarm.md)
- [14 — Introduction to Kubernetes](./14_kubernetes_intro.md)
- [15 — Monitoring & Logging](./15_monitoring_logging.md)
- [16 — Best Practices & Patterns](./16_best_practices.md)
- [17 — Real-world Project](./17_project.md)
- [18 — Interview Q&A](./18_interview_qa.md)
