# 📖 Learning

A collection of structured, self-paced deep-dive courses for programming languages, databases, cloud platforms, and container tooling — built for interview prep and professional upskilling.

---

## 🗂️ Courses

### Languages

| Course | Focus | Modules |
|--------|-------|---------|
| [Go (Golang)](./golang/) | Basics, concurrency, interfaces, testing, patterns, interview Q&A | 15 |
| [Python](./python/) | Syntax, OOP, typing, async, packaging, production patterns | 17 |
| [Rust](./rust/) | Ownership, lifetimes, traits, async, unsafe, FFI, production | 18 |
| [JavaScript](./javascript/) | Core JS, event loop, DOM, Node, npm, tooling, production | 17 |
| [TypeScript](./typescript/) | Type system, generics, mapped/utility types, real-world patterns | 16 |

### Databases

| Course | Focus | Modules |
|--------|-------|---------|
| [MySQL](./mysql/) | Relational model, joins, indexes, transactions, replication, ops | 17 |

### Cloud & Infrastructure

| Course | Focus | Modules |
|--------|-------|---------|
| [AWS](./aws/) | IAM, VPC, EC2, S3, RDS, Lambda, IaC, Well-Architected | 18 |
| [Azure](./azure/) | Entra ID, RBAC, VNets, Storage, Functions, Bicep, CAF | 18 |
| [Docker](./docker/) | Images, Dockerfiles, Compose, registries, multi-arch, security | 16 |
| [Kubernetes](./kubernetes/) | Pods, Services, RBAC, scheduling, autoscaling, operators, GitOps | 18 |

---

## 🚀 How to Use

Each course lives in its own folder. Start with `00_roadmap.md` for an overview, module table, timeline, prerequisites, and curated external resources. Then work through the numbered modules in order — each follows the same structure:

1. **Concept** — mental model + immediate working example
2. **Mechanism** — how it actually works
3. **Variations / Depth** — variants and trade-offs
4. **Practical Application** — realistic end-to-end example
5. **Common Mistakes & Gotchas**
6. **Key Takeaways** — senior-engineer lens

Suggested pacing: one module per day for languages and databases (~3 weeks each), one module every 1–2 days for cloud and Kubernetes (~3–5 weeks each).

### Suggested Learning Paths

- **Backend engineer:** Go or Python → MySQL → Docker → Kubernetes → AWS or Azure
- **Frontend → fullstack:** JavaScript → TypeScript → Node sections → Docker → one cloud
- **Systems / performance:** Rust → Docker → Kubernetes
- **Cloud / DevOps:** Docker → Kubernetes → AWS → Azure

---

## 🧪 Go Exercises

The `golang/exercises/` folder contains hands-on practice problems (todo list, bug fixes, HTTP server, bank system). Per-exercise `done/` solution folders are git-ignored so you can solve them without spoilers.

---

## 🛠️ Contributing

This is a personal learning repo. Feel free to fork it and adapt the material for your own study.
