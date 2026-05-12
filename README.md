# 📖 Learning

A collection of structured, self-paced deep-dive courses for programming languages, databases, cloud platforms, container tooling, and web frameworks — built for interview prep and professional upskilling.

---

## 🗂️ Courses

### 🐍 Languages

| Course | Focus | Modules |
|--------|-------|---------|
| [Go (Golang)](./golang/) | Basics, concurrency, interfaces, testing, patterns, interview Q&A | 15 |
| [Python](./python/) | Syntax, OOP, typing, async, packaging, production patterns | 17 |
| [Rust](./rust/) | Ownership, lifetimes, traits, async, unsafe, FFI, production | 18 |
| [JavaScript](./javascript/) | Core JS, event loop, DOM, Node, npm, tooling, production | 17 |
| [TypeScript](./typescript/) | Type system, generics, mapped/utility types, real-world patterns | 16 |

### 🗄️ Databases

| Course | Focus | Modules |
|--------|-------|---------|
| [MySQL](./mysql/) | Relational model, joins, indexes, transactions, replication, ops | 17 |

### ☁️ Cloud & Infrastructure

| Course | Focus | Modules |
|--------|-------|---------|
| [AWS](./aws/) | IAM, VPC, EC2, S3, RDS, Lambda, IaC, Well-Architected | 18 |
| [Azure](./azure/) | Entra ID, RBAC, VNets, Storage, Functions, Bicep, CAF | 18 |
| [Docker](./docker/) | Images, Dockerfiles, Compose, registries, multi-arch, security | 16 |
| [Kubernetes](./kubernetes/) | Pods, Services, RBAC, scheduling, autoscaling, operators, GitOps | 18 |

### 🧩 Backend Frameworks

| Course | Language | Focus | Modules |
|--------|----------|-------|---------|
| [Spring Boot](./springboot/) | Java | DI, web layer, JPA, security, testing, Actuator, production | 17 |
| [Django](./django/) | Python | MTV, ORM, forms, admin, DRF, security, deployment | 16 |
| [FastAPI](./fastapi/) | Python | Pydantic v2, DI, async, SQLAlchemy 2.0, OpenAPI, production | 15 |
| [Gin](./gin/) | Go | Routing, middleware, binding, context, testing, observability | 15 |

### 🎨 Frontend / Full-stack Frameworks

| Course | Focus | Modules |
|--------|-------|---------|
| [Next.js](./nextjs/) | App Router, RSC, Server Actions, caching, auth, deployment | 17 |

---

## 🚀 How to Use

Each course lives in its own folder. Start with `00_roadmap.md` for an overview, module table, timeline, prerequisites, and curated external resources. Then work through the numbered modules in order — each follows the same structure:

1. **Concept** — mental model + immediate working example
2. **Mechanism** — how it actually works
3. **Variations / Depth** — variants and trade-offs
4. **Practical Application** — realistic end-to-end example
5. **Common Mistakes & Gotchas**
6. **Key Takeaways** — senior-engineer lens

Suggested pacing: roughly one module per day for languages, databases, and frameworks (~2–3 weeks each), one module every 1–2 days for cloud and Kubernetes (~3–5 weeks each).

### 🛤️ Suggested Learning Paths

- **Java backend:** Go or Python → MySQL → Spring Boot → Docker → Kubernetes → AWS or Azure
- **Python backend:** Python → MySQL → FastAPI **or** Django → Docker → Kubernetes → one cloud
- **Go backend:** Go → MySQL → Gin → Docker → Kubernetes → one cloud
- **Fullstack JS/TS:** JavaScript → TypeScript → Next.js → Docker → one cloud
- **Systems / performance:** Rust → Docker → Kubernetes
- **Cloud / DevOps:** Docker → Kubernetes → AWS → Azure → IaC modules in both
- **Interview cram:** Pick your language → MySQL → one framework → revisit interview Q&A sections

---

## 🧪 Go Exercises

The `golang/exercises/` folder contains hands-on practice problems (todo list, bug fixes, HTTP server, bank system). Per-exercise `done/` solution folders are git-ignored so you can solve them without spoilers.

---

## 🛠️ Contributing

This is a personal learning repo. Feel free to fork it and adapt the material for your own study.
