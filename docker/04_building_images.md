# 04 — Building Images
> **Goal:** Understand `docker build` end to end — what the build context is, how to control it with `.dockerignore`, how to tag and inspect images, and the BuildKit improvements that make modern builds fast and capable.

---

## 1. `docker build` and the build context

```bash
docker build -t myapp:0.1 .
```

That trailing `.` is the **build context**: the directory whose contents the build engine has access to. Before anything runs, Docker (or BuildKit) **packages up the context** and sends it to the engine. Anything outside the context is invisible to the build.

This means:
- `COPY ../foo .` doesn't work. The build can't see parents of the context.
- `docker build -t app /home/me/repo` uses `/home/me/repo` as context.
- `docker build -f Dockerfile.prod -t app .` keeps the context as `.` but uses a different Dockerfile.
- `docker build -t app https://github.com/me/repo.git#main` uses a remote git repo as context.
- `docker build -t app -` reads a Dockerfile from stdin (no context at all).

**Watch context size:**

```bash
$ docker build -t myapp .
[+] Building 2.3s (8/8) FINISHED
 => [internal] load build context                           1.2s
 => => transferring context: 412.3MB
 ...
```

`transferring context: 412.3MB`. That's bad. Probably `node_modules` and `.git` and dist artifacts got shoveled to the engine. Fix it with `.dockerignore` (next section).

A core mental model from the roadmap: **the build context matters**. If your context is 4 GB, every build starts with a 4 GB upload to the daemon — even if your Dockerfile only `COPY`s 200 KB of source.

---

## 2. `.dockerignore` — the second most important file

Put a `.dockerignore` at the root of your context (alongside the Dockerfile). It uses gitignore syntax. Anything matched is excluded from the build context.

```gitignore
# .dockerignore — sensible starter
.git
.gitignore
.github/
.vscode/
.idea/

# Dependencies (will be reinstalled inside image)
node_modules
__pycache__/
*.pyc
.venv/
venv/
target/                  # Rust/Java
bin/ obj/                # .NET

# Build artifacts
dist/
build/
*.log

# Environment files (never want these in images)
.env
.env.*
secrets/
*.pem
*.key

# OS/editor junk
.DS_Store
Thumbs.db

# Docker itself
Dockerfile*
docker-compose*.yml
.dockerignore
```

Why exclude `Dockerfile` and `.dockerignore`? They're read by the build engine *before* the context transfer, so they don't need to be inside the image. Including them is harmless but bloats the context.

**Verify it works:**

```bash
docker build -t myapp . 2>&1 | grep "transferring context"
# Should drop from hundreds of MB to a few MB.
```

A subtle gotcha: `.dockerignore` is **per-context**, not per-Dockerfile. If you `docker build -f sub/Dockerfile .`, the `.dockerignore` at the root applies, *not* one in `sub/`.

---

## 3. Tags, image names, and digests

An image reference has this anatomy:

```
registry.example.com:5000/team/project:tag@sha256:abcdef...
└──────────registry──────┘└──path─┘└tag┘ └────digest─────┘
```

- **Registry** — server hostname (default: `docker.io`). Optional port.
- **Path** — repository within the registry (default user: `library` on Docker Hub for official images).
- **Tag** — human-friendly label like `1.27-alpine`. Default `latest`.
- **Digest** — SHA-256 of the image manifest. Immutable. Optional but powerful.

Examples and what they expand to:

| Short form | Full form |
|---|---|
| `nginx` | `docker.io/library/nginx:latest` |
| `nginx:1.27` | `docker.io/library/nginx:1.27` |
| `myteam/api` | `docker.io/myteam/api:latest` |
| `ghcr.io/me/app:v2` | `ghcr.io/me/app:v2` |
| `nginx@sha256:abc...` | `docker.io/library/nginx@sha256:abc...` (no tag, just digest) |

**Tagging on build:**

```bash
docker build -t myapp:0.1 -t myapp:latest .
docker tag myapp:0.1 ghcr.io/me/myapp:0.1     # add another reference
docker push ghcr.io/me/myapp:0.1
```

Tags are mutable — `nginx:1.27` today might be different bits next month if upstream rebuilds. Digests are immutable. For reproducible deployments, **resolve a tag to a digest once** and deploy by digest:

```bash
$ docker pull nginx:1.27-alpine
$ docker inspect --format='{{index .RepoDigests 0}}' nginx:1.27-alpine
nginx@sha256:a45ee5d042aaa9e81e013f97ae40c3dda26fbe98f22b6251acdf28e579560d55
```

Pin that digest in your prod manifests / Compose files and you're guaranteed bit-identical images at every deploy.

### A sane tagging scheme

A practical convention many teams use:

```
myapp:latest              # the latest mainline build (mutable, dev convenience only)
myapp:1.4.2               # semver release (immutable in practice once published)
myapp:1.4                 # rolling minor (auto-promoted on patch releases)
myapp:1                   # rolling major
myapp:sha-a1b2c3d         # git commit SHA (perfectly unique, recommend in CI)
myapp:pr-127              # ephemeral preview build
```

In CI, tag by both commit SHA *and* by version/branch. The SHA tag is your immutable artifact; the version tag is your human-friendly pointer.

---

## 4. BuildKit — the modern build engine

The legacy Docker builder is deprecated. **BuildKit** is the modern engine, default in Docker Desktop and modern Docker Engine. It's better in every way: parallel stages, better caching, secret mounts, SSH agent forwarding, output flexibility, multi-arch.

**Enable globally (Linux):**

```bash
# /etc/docker/daemon.json
{
  "features": { "buildkit": true }
}
sudo systemctl restart docker
```

Or per-build:

```bash
DOCKER_BUILDKIT=1 docker build -t myapp .
```

Docker Desktop has BuildKit on by default. Modern `docker build` invokes it transparently.

**Tell BuildKit to use the newest Dockerfile features** by adding this as the first line of your Dockerfile:

```dockerfile
# syntax=docker/dockerfile:1.7
```

This pins a Dockerfile frontend version and unlocks newer instructions (`--mount`, heredoc syntax, etc.).

BuildKit features we'll use heavily later:

- **`--mount=type=cache`** — persistent caches for `apt`, `pip`, `npm`, etc., that survive across builds without bloating the image.
- **`--mount=type=secret`** — secrets at build time without baking them into a layer.
- **`--mount=type=ssh`** — forward your SSH agent to clone private repos.
- **Parallel stage execution** — independent stages run concurrently.
- **`docker buildx`** — the modern CLI for advanced builds (multi-arch, named builders, remote cache; Module 11).

A taste of the cache mount:

```dockerfile
# syntax=docker/dockerfile:1.7
FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install -r requirements.txt
```

The pip cache directory persists between builds on the same host — first build downloads wheels, subsequent builds reuse them without making the image larger.

---

## 5. Inspecting what you built

After `docker build -t myapp .`, you have an image. Tools to look inside:

```bash
docker images myapp                    # tag, size, age
docker history myapp                   # layer-by-layer breakdown
docker inspect myapp                   # full JSON metadata
docker image inspect myapp --format '{{.Config.Cmd}}'
```

`docker history` is the most useful daily tool:

```bash
$ docker history hello-flask
IMAGE         CREATED          CREATED BY                                     SIZE
8a2b1c3d4e5f  10 minutes ago   CMD ["python" "app.py"]                        0B
<missing>     10 minutes ago   HEALTHCHECK &{["CMD" "python" "-c" "import …   0B
<missing>     10 minutes ago   USER appuser                                   0B
<missing>     10 minutes ago   EXPOSE map[8080/tcp:{}]                        0B
<missing>     10 minutes ago   ENV PORT=8080 PYTHONUNBUFFERED=1               0B
<missing>     10 minutes ago   COPY --chown=appuser:appuser app.py . # b…    422B
<missing>     12 hours ago     RUN /bin/sh -c pip install --no-cache-dir …   18.4MB
<missing>     12 hours ago     COPY requirements.txt . # buildkit             18B
<missing>     12 hours ago     WORKDIR /app                                   0B
<missing>     12 hours ago     RUN /bin/sh -c useradd -r -u 1001 -m appus…   336kB
<missing>     2 weeks ago      /bin/sh -c #(nop)  CMD ["python3"]             0B
...
```

Find the fat layers. If your image is 1.2 GB and `docker history` shows one 800 MB layer that's a `RUN apt-get install ...`, you know exactly where to cut.

For deeper inspection, **dive** (third-party CLI tool) lets you browse layers interactively:

```bash
dive myapp
```

It shows file-level diffs per layer and a "wasted space" score. Worth installing.

---

## 6. A complete worked example: a Go service

Source tree:

```
hello-go/
├── .dockerignore
├── Dockerfile
├── go.mod
├── go.sum
└── main.go
```

```go
// main.go
package main

import (
    "fmt"
    "net/http"
    "os"
)

func main() {
    port := os.Getenv("PORT")
    if port == "" { port = "8080" }
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        h, _ := os.Hostname()
        fmt.Fprintf(w, "hello from %s\n", h)
    })
    fmt.Printf("listening on :%s\n", port)
    http.ListenAndServe(":"+port, nil)
}
```

```gitignore
# .dockerignore
.git
.github
*.md
Dockerfile*
.dockerignore
hello-go            # any local build artifact
```

```dockerfile
# syntax=docker/dockerfile:1.7
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/hello .

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/hello /hello
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/hello"]
```

(We'll go deep on multi-stage builds in Module 05; introducing it here so you see a realistic prod pattern.)

Build and inspect:

```bash
$ docker build -t hello-go:0.1 .
[+] Building 14.7s (14/14) FINISHED
...

$ docker images hello-go
REPOSITORY  TAG   IMAGE ID       CREATED         SIZE
hello-go    0.1   c4d5e6f7a8b9   5 seconds ago   8.42MB

$ docker history hello-go:0.1
IMAGE         CREATED         CREATED BY                                SIZE
c4d5e6f7a8b9  6 seconds ago   ENTRYPOINT ["/hello"]                     0B
<missing>     6 seconds ago   USER nonroot:nonroot                      0B
<missing>     6 seconds ago   EXPOSE map[8080/tcp:{}]                   0B
<missing>     6 seconds ago   COPY /out/hello /hello # buildkit         6.3MB
<missing>     3 months ago    /bin/sh -c #(nop)  ENTRYPOINT [...]       0B
<missing>     3 months ago    (... distroless base ...)                 2.1MB
```

8.4 MB total image. That's the sweet spot: distroless base + static Go binary + nothing else. No shell, no package manager — minimum attack surface, fastest startup.

Run it:

```bash
$ docker run -d --name hg -p 8080:8080 hello-go:0.1
$ curl localhost:8080
hello from a1b2c3d4e5f6
$ docker stop hg && docker rm hg
```

---

## 7. Common mistakes & gotchas

- **Massive build context.** `transferring context: 412.3MB` because `node_modules`, `.git`, and `dist/` got included. `.dockerignore` is not optional — write it on day one.
- **Building from `~` or `/`.** `docker build -t app ~` packs your *entire home directory* as the build context. Always build from a project directory with a tight `.dockerignore`.
- **`COPY * /app`** is rarely what you want — it doesn't recurse the way you think. Prefer `COPY . /app` plus a good `.dockerignore`.
- **Cache invalidation by accident.** `COPY . .` before `RUN npm install` busts cache on every edit. Copy `package.json` first (Module 05 covers this in depth).
- **Trusting `:latest` in `FROM`.** Today's `python:latest` is 3.12; tomorrow it's 3.13. Pin to specific minor/patch versions (or digests).
- **Building without BuildKit and wondering why builds are slow / why `--mount` doesn't work.** Older Docker Engine versions default to the legacy builder. Set `DOCKER_BUILDKIT=1` or upgrade.
- **Tag confusion in CI.** `docker build -t myapp .` overwrites the `:latest` tag on the host. In CI, always tag with the commit SHA (`-t myapp:sha-$GIT_SHA`) so artifacts are unique.
- **Pushing `latest` from feature branches.** Now `:latest` doesn't point at main anymore. CI should only push `:latest` from your release/main branch.
- **`docker build` succeeds but the image won't run.** First `docker run -it --rm myimg sh` (or `bash`) and poke around. If even that fails, your `ENTRYPOINT` is broken — try `--entrypoint sh` to override.

---

## 🎯 Key Takeaways

- **The build context is everything in your context directory** that isn't `.dockerignore`d. Keep it small or builds are slow forever.
- **`.dockerignore` is mandatory**, not optional. Write it before your first build, alongside your `.gitignore`.
- **Pin tags (or digests).** `:latest` is fine for exploration; for anything reproducible, use semantic tags or sha256 digests. Always tag CI builds by commit SHA.
- **BuildKit is the default for a reason.** Cache mounts, secret mounts, parallel stages, and multi-arch builds make modern Dockerfiles vastly more capable. Add `# syntax=docker/dockerfile:1.7` to unlock new features.
- **`docker history` and `dive`** are your forensic tools for image size. Run them after every meaningful build — surprise bloat is normal until you make a habit of looking.

*[prev ← 03_dockerfile_fundamentals](./03_dockerfile_fundamentals.md) | [next → 05_layer_caching_multistage](./05_layer_caching_multistage.md)*
