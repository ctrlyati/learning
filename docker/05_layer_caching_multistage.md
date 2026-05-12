# 05 — Layer Caching & Multi-Stage Builds
> **Goal:** Make builds dramatically faster by ordering instructions for cache hits, leveraging BuildKit's cache mounts and registry-backed cache, and slashing image size with multi-stage builds.

---

## 1. How the cache actually works

When the build engine encounters an instruction, it checks: "have I built this *exact* instruction on *exactly* the same input before?" If yes → reuse the existing layer (a CACHED hit). If no → execute and create a new layer.

What "same input" means depends on the instruction:

| Instruction | Cache key includes |
|---|---|
| `FROM` | base image digest |
| `RUN cmd` | the literal command string + parent layer |
| `COPY src dst` | file contents (checksums) + parent layer + `--chown`/`--chmod` |
| `ADD src dst` | same as `COPY`, plus URL contents if any |
| `ENV`, `LABEL`, `EXPOSE`, etc. | the literal string + parent layer |

**Crucially:** once *any* instruction misses the cache, **every instruction after it also misses**. The cache is a single chain.

That's why **ordering matters enormously**. Put stable things at the top, volatile things at the bottom.

---

## 2. The cache-busting trap

The canonical anti-pattern:

```dockerfile
# BAD
FROM node:20-slim
WORKDIR /app
COPY . .                      # <-- any source edit busts cache here
RUN npm install               # <-- forced to reinstall every single time
CMD ["node", "server.js"]
```

Edit one character in your source code → `COPY . .` produces a different layer → `RUN npm install` re-runs → 90 seconds wasted.

The fix is to copy the dependency manifest first, install, *then* copy the rest:

```dockerfile
# GOOD
FROM node:20-slim
WORKDIR /app
COPY package.json package-lock.json ./   # only changes when deps change
RUN npm ci                                # cached unless lockfile changes
COPY . .                                  # busts on every code edit, but...
CMD ["node", "server.js"]                 # ...the slow step above is reused
```

Now `npm ci` is cached until you actually change a dependency. Source edits only re-run the tiny `COPY . .` and `CMD` setup.

The same pattern applies to every ecosystem:

```dockerfile
# Python
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .

# Go
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Rust (cargo-chef pattern, more advanced)
COPY Cargo.toml Cargo.lock ./
RUN mkdir src && echo "fn main(){}" > src/main.rs && cargo build --release
COPY src ./src
RUN cargo build --release

# Maven / Java
COPY pom.xml .
RUN mvn dependency:go-offline
COPY src ./src
RUN mvn package
```

The pattern: **declare deps, install deps, then copy code**.

---

## 3. BuildKit cache mounts — the next level

Even with good ordering, when you *do* change `requirements.txt`, you re-run `pip install` from scratch. The cache mount fixes that:

```dockerfile
# syntax=docker/dockerfile:1.7
FROM python:3.12-slim
WORKDIR /app

COPY requirements.txt .
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --no-cache-dir -r requirements.txt

COPY . .
CMD ["python", "app.py"]
```

The `--mount=type=cache,target=/root/.cache/pip` creates a persistent volume *for the duration of this RUN step*, on the host's BuildKit cache. Subsequent builds — even ones that change `requirements.txt` — find already-downloaded wheels.

Crucially: the cache is **not** included in the resulting image layer. It exists only during the build. That's the magic: persistent across builds, invisible to the image.

Common cache mount targets:

```dockerfile
# pip
RUN --mount=type=cache,target=/root/.cache/pip pip install ...

# apt
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update && apt-get install -y --no-install-recommends ...

# npm
RUN --mount=type=cache,target=/root/.npm npm ci

# go
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build go build ...

# cargo
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=/target cargo build --release
```

`sharing=locked` (the default is `shared`) prevents concurrent builds from racing on the same cache directory — useful for apt where lockfile contention causes ugly failures.

For apt specifically, you also need to disable the auto-clean behavior:

```dockerfile
RUN rm -f /etc/apt/apt.conf.d/docker-clean \
 && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' \
    > /etc/apt/apt.conf.d/keep-cache
```

(Or use a base image that doesn't have `docker-clean` pre-configured.)

---

## 4. Registry-backed cache (`--cache-from` / `--cache-to`)

Local cache only helps you on the machine that did the build. In CI, every job is a fresh runner. That's where **registry-backed cache** earns its keep.

```bash
# Push cache to the registry alongside the image
docker buildx build \
  --tag ghcr.io/me/app:latest \
  --cache-to type=registry,ref=ghcr.io/me/app:buildcache,mode=max \
  --cache-from type=registry,ref=ghcr.io/me/app:buildcache \
  --push \
  .
```

- `--cache-to type=registry,ref=...,mode=max` pushes all layers (not just the final ones) to a separate tag (`:buildcache`) acting as a cache repository.
- `--cache-from type=registry,ref=...` pulls that cache *before* building, populating the build cache with prior layers.
- `mode=max` keeps intermediate layers; `mode=min` (default) only stores final stage layers.

Other cache backends:
- `type=gha` — GitHub Actions cache. Free, ergonomic, ~10GB limit per repo.
- `type=local,dest=/tmp/cache` — local filesystem (useful in self-hosted CI).
- `type=s3,...` / `type=azblob,...` — cloud blob storage.
- `type=inline` — embed cache metadata directly into the image's manifest. Simple but only works for single-stage builds and pollutes the image.

A typical GitHub Actions snippet:

```yaml
- uses: docker/setup-buildx-action@v3
- uses: docker/login-action@v3
  with:
    registry: ghcr.io
    username: ${{ github.actor }}
    password: ${{ secrets.GITHUB_TOKEN }}
- uses: docker/build-push-action@v6
  with:
    context: .
    push: true
    tags: ghcr.io/${{ github.repository }}:${{ github.sha }}
    cache-from: type=gha
    cache-to: type=gha,mode=max
```

We'll go deeper into CI/CD in Module 15.

---

## 5. Multi-stage builds — the biggest single optimization

A multi-stage build has multiple `FROM` lines. Each `FROM` starts a new **stage**. You can copy artifacts between stages with `COPY --from=<stage>`. Only the **final** stage becomes the image; intermediate stages are discarded.

This unlocks the build-vs-runtime separation. Build with a fat toolchain image; ship a tiny runtime image.

**Before (single-stage Node.js):**

```dockerfile
# 350 MB image, has TypeScript compiler, dev deps, build artifacts
FROM node:20
WORKDIR /app
COPY . .
RUN npm ci && npm run build
CMD ["node", "dist/server.js"]
```

**After (multi-stage):**

```dockerfile
# syntax=docker/dockerfile:1.7

# Stage 1: build
FROM node:20 AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY . .
RUN npm run build && npm prune --production

# Stage 2: runtime
FROM node:20-slim AS runtime
WORKDIR /app
COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/dist ./dist
COPY --from=build /app/package.json ./
USER node
CMD ["node", "dist/server.js"]
```

The runtime image (~150 MB) contains only production `node_modules` and compiled JS — no TypeScript compiler, no dev dependencies, no source files, no `.git`.

A Go example pushes this even further:

```dockerfile
# syntax=docker/dockerfile:1.7

FROM golang:1.22 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/app ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
```

The build stage is 1.2 GB (Go toolchain). The runtime is **~10 MB** — a distroless image with a static binary. Same software, 99% size reduction, dramatically smaller attack surface.

### Tricks with multi-stage

**Build target selection:**

```bash
docker build --target build -t myapp:dev .       # stop at the build stage
docker build -t myapp:prod .                     # build all the way through
```

Useful for "the dev image is the build image (so you have all the tooling), but the prod image is the trimmed final stage."

**Copy from arbitrary images:**

```dockerfile
COPY --from=alpine:3.20 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
```

You don't need an `AS` alias — you can copy from any image reference.

**Parallel stages execute concurrently** under BuildKit. If you have two independent build stages (frontend + backend), BuildKit builds them in parallel and joins them in the final stage.

```dockerfile
FROM node:20 AS frontend
WORKDIR /app
COPY frontend/ .
RUN npm ci && npm run build

FROM golang:1.22 AS backend
WORKDIR /src
COPY backend/ .
RUN go build -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12
COPY --from=backend /out/api /api
COPY --from=frontend /app/dist /www
ENTRYPOINT ["/api"]
```

BuildKit builds `frontend` and `backend` simultaneously. Linear builds would take frontend_time + backend_time; parallel takes max(frontend_time, backend_time).

---

## 6. A worked end-to-end example with timings

Let's compare the same Python app three ways and see the numbers.

```
hello-py/
├── Dockerfile.v1     # naive
├── Dockerfile.v2     # cache-friendly ordering
├── Dockerfile.v3     # + cache mount + multi-stage
├── requirements.txt
└── app.py
```

```dockerfile
# Dockerfile.v1 — the bad one
FROM python:3.12
WORKDIR /app
COPY . .
RUN pip install -r requirements.txt
CMD ["python", "app.py"]
```

```dockerfile
# Dockerfile.v2 — order matters
FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
CMD ["python", "app.py"]
```

```dockerfile
# Dockerfile.v3 — multistage + cache mount
# syntax=docker/dockerfile:1.7
FROM python:3.12-slim AS build
WORKDIR /app
COPY requirements.txt .
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --prefix=/install --no-warn-script-location -r requirements.txt

FROM python:3.12-slim
WORKDIR /app
COPY --from=build /install /usr/local
COPY app.py .
RUN useradd -r -u 1001 appuser
USER appuser
CMD ["python", "app.py"]
```

Indicative times on a typical laptop:

| Scenario | v1 | v2 | v3 |
|---|---|---|---|
| Cold first build | 42s | 28s | 28s |
| Rebuild after editing `app.py` only | 38s (re-installs deps!) | 1s | 1s |
| Rebuild after changing `requirements.txt` | 38s | 18s | 4s (cache mount) |
| Final image size | 1.08 GB | 168 MB | 152 MB |

The size savings (1 GB → 152 MB) come from `:slim` instead of full `python`. The speed savings come from cache ordering and BuildKit's cache mount.

---

## 7. Common mistakes & gotchas

- **`COPY . .` before installing deps.** The single most common build-performance bug. Always copy lockfiles first, install, *then* copy source.
- **Touching unrelated files breaks the cache.** Editing a README that's *included in the context* changes the `COPY . .` checksum. Use `.dockerignore` aggressively, or copy only what you need: `COPY src/ ./src/`.
- **Forgetting `# syntax=docker/dockerfile:1.7`** when using `--mount=type=cache`. Older frontends don't support it; you'll get a confusing parse error.
- **`apt-get update` cached separately from `apt-get install`.** Always combine: `RUN apt-get update && apt-get install -y ...`. Otherwise a cached `update` layer can be paired with a freshly invalidated `install` layer, installing stale or missing packages.
- **Cache mount for apt without disabling docker-clean.** Slim Debian images include `/etc/apt/apt.conf.d/docker-clean` which purges downloads. Remove it as shown above.
- **Multi-stage with `COPY --from=` pulling in too much.** If you `COPY --from=build / /`, you copied the entire build stage, defeating the point. Copy specific paths (`/out/app`, not `/`).
- **Cache mount target on a non-writable path.** Don't mount onto a path the image needs to keep — the mount is ephemeral. Use vendor-specific cache directories (`/root/.cache/pip`, `/go/pkg/mod`).
- **Order between `--cache-from` and the actual build.** BuildKit fetches cache lazily; first runs of a new CI pipeline will appear "slow" until cache exists. Don't conclude cache isn't working from a single cold run.
- **Stage names that look like image references.** `FROM ubuntu AS build` is fine. `FROM ubuntu AS ubuntu` would shadow the actual image. Pick distinct names.
- **Forgetting that intermediate stages still exist on disk** (locally) until pruned. They're not in the final image, but they take build cache space. `docker builder prune` if disk pressure shows up.

---

## 🎯 Key Takeaways

- **Cache is a chain.** One miss invalidates everything below. Order Dockerfiles from least-volatile (`FROM`, lockfile copies, deps) to most-volatile (source copies, `CMD`).
- **Always: copy manifest → install → copy source.** This single pattern saves more build time than every other optimization combined.
- **BuildKit cache mounts** (`--mount=type=cache,target=...`) make even cold dependency installs fast by persisting package caches across builds without bloating the image.
- **Registry-backed cache** (`--cache-to/--cache-from`) is essential in CI — local cache doesn't survive between runs, but a `:buildcache` tag in your registry does.
- **Multi-stage builds give you both fat build images and tiny runtime images.** A 10 MB Go runtime image with the same software as a 1.2 GB build image is normal, not exceptional. Make this your default for compiled languages.

*[prev ← 04_building_images](./04_building_images.md) | [next → 06_volumes_and_mounts](./06_volumes_and_mounts.md)*
