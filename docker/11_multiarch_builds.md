# 11 — Multi-Architecture Builds
> **Goal:** Build images that run natively on both `linux/amd64` and `linux/arm64` (and beyond), understand `buildx` / QEMU / manifest lists, and pick the right strategy for CI throughput.

---

## 1. Why multi-arch is now table stakes

A few years ago, "Docker image" meant `linux/amd64`. That assumption is dead:

- Apple Silicon (M-series Macs) is `arm64`. Most developers are now on arm64 laptops.
- AWS Graviton, Azure Ampere, GCP Tau T2A — all `arm64` and ~30% cheaper than amd64 for the same workload.
- Raspberry Pi, edge devices, IoT — `arm64` or `arm/v7`.

If your image is amd64-only, your dev/prod environments diverge (a Mac developer pulls an emulated amd64 image, runs it slowly under Rosetta/QEMU, then deploys to amd64 servers — fine, but slow). If half your fleet is arm64, you can't deploy at all.

Modern published images (nginx, postgres, python, node) are **multi-arch**: a single tag like `python:3.12-slim` is actually a *manifest list* (a.k.a. *image index*) pointing to per-platform manifests. Docker pulls the right one for your platform automatically.

```bash
$ docker buildx imagetools inspect python:3.12-slim
Name:      docker.io/library/python:3.12-slim
MediaType: application/vnd.docker.distribution.manifest.list.v2+json

Manifests:
  Name:      python:3.12-slim@sha256:abc... linux/amd64
  Name:      python:3.12-slim@sha256:def... linux/arm64
  Name:      python:3.12-slim@sha256:ghi... linux/arm/v7
  Name:      python:3.12-slim@sha256:jkl... linux/ppc64le
  Name:      python:3.12-slim@sha256:mno... linux/s390x
  Name:      python:3.12-slim@sha256:pqr... linux/386
```

Your task is to produce the same shape for your own images.

---

## 2. The mechanics — manifest lists, `buildx`, QEMU

### Manifest lists / OCI image indexes

A multi-arch image isn't one binary blob. It's a small JSON document — the **manifest list** — that points at multiple per-platform image manifests. When a client pulls, the registry serves the list, the client picks the entry matching its platform, then pulls that manifest and its layers.

```
                 :latest tag
                      │
                      ▼
             ┌────────────────────┐
             │  manifest list /   │
             │   image index      │
             └────┬───────┬───────┘
                  │       │
              amd64       arm64
                  │       │
                  ▼       ▼
            ┌────────┐ ┌────────┐
            │ manif. │ │ manif. │
            │ + cfg  │ │ + cfg  │
            │ +layers│ │ +layers│
            └────────┘ └────────┘
```

### `docker buildx`

The classic `docker build` is single-platform. **`docker buildx`** (built-in to modern Docker) is the multi-arch-aware wrapper around BuildKit. Quick orientation:

```bash
docker buildx ls                       # list builders
docker buildx create --name multi --use   # create a new builder (BuildKit instance)
docker buildx inspect --bootstrap      # start it
```

A builder is a BuildKit instance. The **default** builder uses the local Docker daemon (single-platform). New named builders can be configured with multiple platform "nodes," enabling multi-arch.

### QEMU — emulation for the cross-platform case

To build an `arm64` image on an `amd64` host (or vice versa), one of three approaches:
1. **Cross-compile** in the build stage and emit a static binary the target arch can run. Best for Go/Rust.
2. **QEMU emulation** — register binfmt handlers so the kernel transparently emulates the foreign architecture. Works for any language, slower (especially for compilation-heavy steps).
3. **Native builders** — use real `arm64` build machines (e.g., AWS Graviton runners, M-series Mac runners). Fastest, requires infrastructure.

QEMU setup on a Linux host (Docker Desktop sets this up automatically):

```bash
docker run --rm --privileged tonistiigi/binfmt --install all
# Now your kernel knows how to emulate arm64, arm/v7, ppc64le, etc.
```

Then create a buildx builder that uses it:

```bash
docker buildx create --name multi --driver docker-container --use
docker buildx inspect --bootstrap
```

Output snippet:

```
Name:          multi
Driver:        docker-container
Platforms:     linux/amd64, linux/arm64, linux/arm/v7, linux/arm/v6, linux/386, linux/ppc64le, linux/s390x
```

---

## 3. Building multi-arch images

The headline command:

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t ghcr.io/me/app:1.0 \
  --push \
  .
```

That single command:
1. Runs the build for each platform (in parallel where possible).
2. Pushes per-platform images to the registry.
3. Creates and pushes a manifest list at `:1.0` pointing at them.

A few important wrinkles:

- **`--push` is essentially required** for multi-arch. The local Docker daemon's image store is single-platform and can't hold a manifest list. Without `--push`, you can use `--output type=oci,dest=out.tar` to produce a local OCI archive, but you can't `docker load` a multi-arch image to the daemon. Workaround: build per-platform locally with `--load` for testing, then build multi-arch with `--push` for release.
- **Cross-platform builds are slow under QEMU.** Native arm64 builders make this much faster.
- **`TARGETPLATFORM`, `TARGETOS`, `TARGETARCH`** are auto-injected build args. Use them to tailor commands per platform.

### Dockerfile tweaks for cross-arch

A naive Dockerfile that downloads a binary needs to pick the right one per arch:

```dockerfile
FROM alpine:3.20

ARG TARGETARCH        # automatically populated: amd64, arm64, arm, etc.

RUN case "$TARGETARCH" in \
        amd64) ARCH=x86_64 ;; \
        arm64) ARCH=aarch64 ;; \
        *) echo "unsupported $TARGETARCH"; exit 1 ;; \
    esac \
 && wget -qO /usr/local/bin/tool "https://example.com/tool-${ARCH}-linux" \
 && chmod +x /usr/local/bin/tool
```

For Go, cross-compiling beats emulating:

```dockerfile
# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.22 AS build
# BUILDPLATFORM = the *builder*'s arch (native, fast)
# TARGETPLATFORM = the *target*'s arch (what we're producing)

ARG TARGETOS TARGETARCH
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w" -o /out/app .

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
```

The first `FROM --platform=$BUILDPLATFORM` is the magic. It pins the build stage to the native architecture of the host, so the Go compiler runs *natively*. Go's cross-compilation handles the rest. The final stage is platform-neutral (it just copies a binary).

Result: an arm64 image built on an amd64 host in seconds, not minutes — no emulation needed.

For Rust similarly:

```dockerfile
FROM --platform=$BUILDPLATFORM rust:1.79 AS build
ARG TARGETARCH
RUN case "$TARGETARCH" in \
        amd64) rustup target add x86_64-unknown-linux-musl ;; \
        arm64) rustup target add aarch64-unknown-linux-musl ;; \
    esac
# ...
```

For Node / Python with native modules, emulation under QEMU is usually the simplest answer; cross-compiling native deps from x86 to arm gets painful.

---

## 4. Verifying the result

After a multi-arch push, verify the manifest list exists:

```bash
$ docker buildx imagetools inspect ghcr.io/me/app:1.0
Name:      ghcr.io/me/app:1.0
MediaType: application/vnd.oci.image.index.v1+json
Digest:    sha256:abcd1234...

Manifests:
  Name:      ghcr.io/me/app:1.0@sha256:ef5678...
  MediaType: application/vnd.oci.image.manifest.v1+json
  Platform:  linux/amd64

  Name:      ghcr.io/me/app:1.0@sha256:90ab12...
  MediaType: application/vnd.oci.image.manifest.v1+json
  Platform:  linux/arm64
```

Pull and run on a Mac (arm64):

```bash
$ docker pull ghcr.io/me/app:1.0
$ docker inspect ghcr.io/me/app:1.0 --format '{{.Architecture}}'
arm64
$ docker run --rm ghcr.io/me/app:1.0
```

The Mac pulls the arm64 manifest. A Linux x86 server would pull the amd64 manifest from the same tag.

If you ever want to run a non-native image (e.g., test the amd64 build on a Mac):

```bash
docker run --rm --platform linux/amd64 ghcr.io/me/app:1.0
# Runs amd64 under emulation. Slow, but works for smoke tests.
```

---

## 5. CI strategies — emulation vs native

Three patterns, in order of complexity:

### Pattern A: single runner with QEMU

Simplest. GitHub Actions amd64 runners build both platforms via QEMU.

```yaml
# .github/workflows/build.yml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/build-push-action@v6
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: true
          tags: ghcr.io/${{ github.repository }}:${{ github.sha }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

Pros: zero infrastructure, free. Cons: arm64 build under QEMU emulation is 3-10x slower than native, especially for compile-heavy languages.

### Pattern B: native runners per platform, joined into a manifest

Build each platform on its native architecture, then create a manifest list combining them.

```yaml
jobs:
  build:
    strategy:
      matrix:
        include:
          - platform: linux/amd64
            runner: ubuntu-latest
          - platform: linux/arm64
            runner: ubuntu-24.04-arm     # native arm64 runner
    runs-on: ${{ matrix.runner }}
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with: { registry: ghcr.io, username: ${{ github.actor }}, password: ${{ secrets.GITHUB_TOKEN }} }
      - uses: docker/build-push-action@v6
        id: build
        with:
          context: .
          platforms: ${{ matrix.platform }}
          outputs: type=image,name=ghcr.io/${{ github.repository }},push-by-digest=true,name-canonical=true,push=true
          cache-from: type=gha,scope=${{ matrix.platform }}
          cache-to: type=gha,mode=max,scope=${{ matrix.platform }}
      - run: mkdir -p /tmp/digests && echo "${{ steps.build.outputs.digest }}" > /tmp/digests/$(echo ${{ matrix.platform }} | tr / -)
      - uses: actions/upload-artifact@v4
        with: { name: digests-${{ matrix.platform }}, path: /tmp/digests/* }

  manifest:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with: { pattern: digests-*, path: /tmp/digests, merge-multiple: true }
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with: { registry: ghcr.io, username: ${{ github.actor }}, password: ${{ secrets.GITHUB_TOKEN }} }
      - run: |
          cd /tmp/digests
          docker buildx imagetools create \
            -t ghcr.io/${{ github.repository }}:${{ github.sha }} \
            $(for f in *; do echo "ghcr.io/${{ github.repository }}@$(cat $f)"; done)
```

Pros: each platform builds at native speed. Cons: more complex workflow.

### Pattern C: cross-compile in a single native runner

The Go/Rust trick from §3 — build everything on amd64, cross-compile arm64. Single fast runner, no emulation.

```yaml
- uses: docker/build-push-action@v6
  with:
    context: .
    platforms: linux/amd64,linux/arm64
    push: true
    tags: ghcr.io/${{ github.repository }}:${{ github.sha }}
```

With the right `FROM --platform=$BUILDPLATFORM` magic in the Dockerfile, this is the fastest of all three for static-binary languages.

---

## 6. A full multi-arch worked example

Project: a tiny Go service, cross-compiled, distroless final image, multi-arch.

```
hello/
├── Dockerfile
├── go.mod
└── main.go
```

```go
// main.go
package main

import (
    "fmt"
    "net/http"
    "runtime"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "hello from %s/%s\n", runtime.GOOS, runtime.GOARCH)
    })
    http.ListenAndServe(":8080", nil)
}
```

```dockerfile
# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w" -o /out/hello .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/hello /hello
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/hello"]
```

Build and push:

```bash
$ docker buildx create --name multi --use
$ docker buildx build \
    --platform linux/amd64,linux/arm64 \
    -t ghcr.io/me/hello:1.0 \
    --push .

[+] Building 18.4s (24/24) FINISHED
 => [linux/amd64 build 1/4] FROM golang:1.22-alpine                4.1s
 => [linux/arm64 build 1/4] FROM golang:1.22-alpine                4.1s
 => [linux/amd64 build 4/4] RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ...   3.2s
 => [linux/arm64 build 4/4] RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ...   3.4s
 => [linux/amd64] exporting to image                               1.0s
 => [linux/arm64] exporting to image                               1.0s
 => pushing manifest list                                          0.8s

$ docker buildx imagetools inspect ghcr.io/me/hello:1.0
Manifests:
  - Platform: linux/amd64  Size: 2.78MB
  - Platform: linux/arm64  Size: 2.69MB
```

Same source, same Dockerfile, two architectures, 18 seconds, no emulation. Each binary is ~3 MB; total push size after dedupe is small.

Test the amd64 version on an arm64 Mac (forcing emulation):

```bash
$ docker run --rm --platform linux/amd64 ghcr.io/me/hello:1.0 &
$ curl localhost:8080
hello from linux/amd64
```

And the native arm64 version:

```bash
$ docker run --rm ghcr.io/me/hello:1.0 &
$ curl localhost:8080
hello from linux/arm64
```

---

## 7. Common mistakes & gotchas

- **Building amd64-only and deploying to Graviton.** `exec format error`. Always multi-arch your images now, even if your fleet is currently single-arch.
- **`docker buildx build --platform linux/amd64,linux/arm64` without `--push`.** The local daemon can't store manifest lists. Either push, or build each platform separately with `--load` for testing.
- **Forgetting `FROM --platform=$BUILDPLATFORM`** in cross-compile Dockerfiles. Without it, the build stage runs under emulation on the target platform — slow.
- **Hardcoding `amd64`-specific binary downloads.** `wget .../x86_64.tar.gz` works on amd64, fails on arm64. Use `$TARGETARCH` to switch.
- **Native modules failing under cross-compile.** Node `node-gyp`, Python wheels with C extensions, Rust crates needing C deps — these may need actual emulation. Use Pattern A or B in CI, or use prebuilt wheels (`pip install --only-binary=:all:`).
- **Slow CI because emulation.** A 30-minute QEMU arm64 build can become a 5-minute cross-compile or native build. Pick the right pattern for your stack.
- **Not testing the non-native architecture.** Your CI built arm64, but did anyone run it? At least run a smoke test under emulation in CI (`--platform linux/arm64`).
- **Tag points at single-arch by accident.** `docker push ghcr.io/me/app:1.0` from a Mac (without buildx) pushes only the arm64 image, overwriting the tag. Always use `docker buildx build --push` for releases.
- **Apple Silicon "but my image is amd64" puzzle.** On a Mac, if you `docker run nginx`, you get the arm64 image (transparently). If a colleague pushes an amd64-only image, you run it under Rosetta/QEMU and it's slow. Multi-arch publishing fixes this for everyone.
- **Pulling the wrong arch in CI.** Some CI base images (`ubuntu-latest`) are amd64; if you pull `myimg:1.0` expecting arm64, you get amd64. Specify `--platform` explicitly when it matters.

---

## 🎯 Key Takeaways

- **Multi-arch is no longer optional.** Apple Silicon laptops + cheap arm64 cloud instances mean every published image should be at least amd64+arm64.
- **A multi-arch image is a manifest list** pointing to per-platform images. The registry serves the right one to each client automatically.
- **`docker buildx build --platform amd64,arm64 --push`** is the workhorse command. Combine with `FROM --platform=$BUILDPLATFORM` for fast cross-compiled builds.
- **Pick the right CI pattern:** QEMU (simple, slow), native runners per platform (fast, more complex), cross-compile (fastest for static-binary languages).
- **Verify with `docker buildx imagetools inspect`.** Trust nothing else. A successful push doesn't prove all platforms made it into the manifest list.

*[prev ← 10_image_optimization](./10_image_optimization.md) | [next → 12_runtime_internals](./12_runtime_internals.md)*
