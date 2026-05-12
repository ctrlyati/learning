# 10 — Image Optimization
> **Goal:** Build images that are as small and safe as practical — choose base images deliberately (distroless, alpine, scratch), strip unneeded artifacts, and integrate vulnerability scanning into your pipeline.

---

## 1. Why size matters (it's not just disk)

Image bloat costs at every step of the lifecycle:
- **Build time** — more layers, more transfer over the wire.
- **Push/pull bandwidth** — every CI run, every node pulling it.
- **Storage** — registry costs, host disk pressure.
- **Cold start latency** — pulling a 2 GB image to a new K8s node delays the first pod by minutes.
- **Attack surface** — every binary in the image is potential vulnerability surface. A shell, a package manager, libcurl, openssl... each is a CVE waiting to happen.

A 1.2 GB image is not just slower; it has hundreds of binaries you don't run but that show up in CVE scans. A 12 MB image has effectively none.

Reasonable size targets (final runtime image, not build stage):

| Stack | Realistic target | Achievable with |
|---|---|---|
| Static Go / Rust binary | < 20 MB | `scratch` or distroless `static` |
| Python service | < 200 MB | `python:3.12-slim` + careful deps |
| Node service | < 250 MB | `node:20-slim` + `npm ci --production` |
| JVM service | < 250 MB | jlink + `eclipse-temurin:21-jre-alpine` |
| .NET service | < 200 MB | `mcr.microsoft.com/dotnet/aspnet:8.0-alpine` |

If you're way above these, there's almost always low-hanging fruit.

---

## 2. The base image ladder, from biggest to smallest

```
┌─────────────────────────────────────────┐
│ ubuntu:24.04         ~80 MB             │   "Like my laptop"
├─────────────────────────────────────────┤
│ debian:12-slim       ~50 MB             │   Comfortable, slim
├─────────────────────────────────────────┤
│ python:3.12-slim     ~50 MB + python    │   Vendor-managed slim
├─────────────────────────────────────────┤
│ alpine:3.20          ~7 MB              │   musl libc; tiny
├─────────────────────────────────────────┤
│ gcr.io/distroless    ~20 MB (lang dep)  │   No shell, no apt
├─────────────────────────────────────────┤
│ scratch              0 MB               │   Literally empty
└─────────────────────────────────────────┘
```

### Ubuntu / Debian
Full-fat. Familiar. ~80 MB starting point. Use only when you genuinely need a full userspace (you don't, usually).

### Debian slim (`debian:12-slim`, `python:3.12-slim`)
Stripped-down Debian, ~30-50 MB. **The default safe choice** for most languages. Same glibc, same packaging, just less stuff.

### Alpine
Built around musl libc and BusyBox. Tiny (~7 MB). Two caveats:
- **musl ≠ glibc.** Some prebuilt binaries (Python wheels with C extensions, Node native modules) won't run, or have to recompile from source — slow and sometimes broken.
- **DNS quirks** historically (improved in recent versions, but still surprises occasionally).

Use Alpine when you've confirmed your stack works on it. Test thoroughly.

### Distroless (`gcr.io/distroless/...`)
Google's "no shell, no package manager, just the runtime" images. Variants per language: `static-debian12`, `base-debian12`, `cc-debian12`, `python3`, `nodejs20`, `java21`. ~20-50 MB. **No shell** means `docker exec myimg sh` *doesn't work* — and that's the point: an attacker who breaks in can't `bash` around. Forces multi-stage builds (you must build elsewhere, copy the artifact in).

### Scratch
The empty image. Zero bytes. Anything you `COPY` in is the image. Works for fully static binaries (Go without CGO, Rust with musl target). Will not work for dynamic binaries — there's no glibc, no `/lib64/ld-linux-x86-64.so.2`.

```dockerfile
FROM scratch
COPY hello /
ENTRYPOINT ["/hello"]
```

A `scratch` image has no `/etc/passwd`, no `/etc/ssl/certs`, no `/tmp`. You may need to copy those in if your binary uses TLS or sets user IDs.

---

## 3. The art of dropping junk

After base choice, the next layer of savings is **cleaning up inside `RUN` steps**.

### Don't install what you don't need

```dockerfile
# BAD
RUN apt-get update && apt-get install -y python3 python3-pip git curl vim build-essential

# GOOD (only what runtime needs)
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
        python3 python3-pip ca-certificates \
 && rm -rf /var/lib/apt/lists/*
```

`--no-install-recommends` is the single biggest apt savings — it skips packages that are "Recommended" but not "Required." `vim`, `build-essential`, and `git` are for *building*, not running; if you need them, put them in a build stage and don't carry them forward.

### Clean caches in the same `RUN`

Already covered in Module 03/05 but worth repeating: deleting cached files in a later layer doesn't shrink the image — the cache still exists in the earlier layer. Always clean in the same `RUN`:

```dockerfile
RUN pip install --no-cache-dir -r requirements.txt
RUN apt-get install ... && rm -rf /var/lib/apt/lists/*
RUN npm ci --production && npm cache clean --force
```

`--no-cache-dir` flags exist for pip and others; use them.

### Production-only deps

```dockerfile
# Node
RUN npm ci --omit=dev

# Python (using pip-tools style)
COPY requirements.txt requirements-dev.txt ./
RUN pip install --no-cache-dir -r requirements.txt    # NOT requirements-dev.txt

# Go (this is automatic with multi-stage)
FROM golang:1.22 AS build
RUN go build ...
FROM scratch
COPY --from=build /out/app /app
# No dev tooling carried forward — multi-stage gives you this for free.
```

### Strip binaries

Compiled languages can shrink binaries significantly:

```dockerfile
# Go
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o /out/app .
# -s -w strips symbols and DWARF info; trimpath removes filesystem paths
```

Typical Go service: 25 MB unstripped → 12 MB stripped → 4 MB with UPX (UPX has tradeoffs — slower startup, AV false positives — usually skip it).

### Strip dev metadata from images

Some images come with man pages, locales, docs. Slim variants drop most of this. Going further on Debian:

```dockerfile
RUN apt-get install ... \
 && rm -rf /usr/share/doc /usr/share/man /usr/share/locale/* \
 && rm -rf /var/lib/apt/lists/*
```

Diminishing returns at this point — focus on multi-stage and base choice before micro-optimizing.

---

## 4. Multi-stage as the size superpower

Repeating Module 05's punchline because it bears repeating: **multi-stage builds are the single biggest optimization.** Build heavy, ship light.

```dockerfile
# syntax=docker/dockerfile:1.7

# STAGE 1: build everything
FROM golang:1.22 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o /out/app ./cmd/server

# STAGE 2: collect runtime extras we need
FROM alpine:3.20 AS extras
RUN apk add --no-cache ca-certificates tzdata \
 && cp /etc/ssl/certs/ca-certificates.crt /ca.crt

# STAGE 3: final, minimal
FROM scratch
COPY --from=extras /ca.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=extras /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /out/app /app
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app"]
```

Result: ~6 MB image with TLS-capable static Go binary and timezones. The build stage was 1.2 GB.

For Python — no static binary, but you can install into a virtualenv and copy *just that*:

```dockerfile
# syntax=docker/dockerfile:1.7

FROM python:3.12-slim AS build
WORKDIR /app
RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"
COPY requirements.txt .
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --no-cache-dir -r requirements.txt

FROM gcr.io/distroless/python3-debian12
COPY --from=build /opt/venv /opt/venv
COPY app.py /app/app.py
ENV PATH="/opt/venv/bin:$PATH"
WORKDIR /app
USER nonroot
ENTRYPOINT ["python", "app.py"]
```

Distroless Python image is ~50 MB. With your venv, maybe 150-200 MB. No shell, no apt, no pip in the final image.

---

## 5. Security scanning — the other half of "optimization"

Smaller is safer, but you still need to verify. Three scanners worth knowing:

### `docker scout` (built into Docker Desktop and Docker CLI)

```bash
$ docker scout cves myapp:1.0
    ✗ HIGH    CVE-2024-12345  libcrypto3  3.3.0-r0   → 3.3.2-r0
    ✗ MEDIUM  CVE-2024-23456  curl        8.5.0      → 8.9.1
    ...
2 vulnerabilities found (0 critical, 1 high, 1 medium)
```

```bash
$ docker scout quickview myapp:1.0
  ✓  Image stored locally
  ✓  Image analyzed
  ✗  1 HIGH, 3 MEDIUM, 12 LOW vulnerabilities
  i  Recommended update: alpine:3.20 → alpine:3.20.3 (resolves 2 of 4)
```

Built-in, no setup, decent UX. Compares against newer base image tags and suggests upgrades.

### Trivy (Aqua Security)

The de-facto open-source scanner. Fast, free, ubiquitous.

```bash
$ trivy image myapp:1.0
myapp:1.0 (alpine 3.20.0)
═════════════════════════
Total: 4 (HIGH: 1, MEDIUM: 3)

┌─────────────┬────────────────┬──────────┬─────────────┬─────────────────────┐
│   Library   │ Vulnerability  │ Severity │ Fixed Vers  │       Title         │
├─────────────┼────────────────┼──────────┼─────────────┼─────────────────────┤
│ libcrypto3  │ CVE-2024-12345 │ HIGH     │ 3.3.0-r1    │ openssl: ...        │
└─────────────┴────────────────┴──────────┴─────────────┴─────────────────────┘
```

Trivy also scans Dockerfiles for misconfigs (`trivy config Dockerfile`) and IaC. The Swiss Army knife of container security.

In CI, fail builds on high+ vulnerabilities:

```bash
trivy image --severity HIGH,CRITICAL --exit-code 1 myapp:1.0
```

### Grype + Syft (Anchore)

Similar pairing — `syft` produces an SBOM (Software Bill of Materials), `grype` scans it.

```bash
syft myapp:1.0 -o spdx-json > sbom.json
grype sbom:sbom.json
```

Useful when you want to detach SBOM generation from scanning (e.g., produce SBOMs at build time, scan continuously against a fresh CVE DB).

### Scanning as a CI gate

```yaml
# .github/workflows/build.yml (excerpt)
- name: Build image
  run: docker build -t app:${{ github.sha }} .

- name: Scan with trivy
  uses: aquasecurity/trivy-action@master
  with:
    image-ref: app:${{ github.sha }}
    severity: CRITICAL,HIGH
    exit-code: '1'         # fail the pipeline on findings
    ignore-unfixed: true   # don't fail on CVEs with no fix available
```

Module 13 covers signing, SBOMs, and provenance more deeply; Module 15 wires it all together in CI.

---

## 6. A worked optimization — same app, four ways

A trivial Python web app. Same `app.py`, four Dockerfiles, four wildly different outcomes.

```python
# app.py
from flask import Flask
app = Flask(__name__)
@app.route("/")
def hi(): return "hi\n"
if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8080)
```

```
requirements.txt:
flask==3.0.3
gunicorn==22.0.0
```

### v1 — naive: 1.05 GB

```dockerfile
FROM python:3.12
WORKDIR /app
COPY . .
RUN pip install -r requirements.txt
CMD ["python", "app.py"]
```

### v2 — slim base: 160 MB

```dockerfile
FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
CMD ["python", "app.py"]
```

### v3 — multi-stage + venv: 145 MB

```dockerfile
FROM python:3.12-slim AS build
RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

FROM python:3.12-slim
COPY --from=build /opt/venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"
WORKDIR /app
COPY app.py .
RUN useradd -r -u 1001 app
USER app
CMD ["gunicorn", "-b", "0.0.0.0:8080", "app:app"]
```

### v4 — distroless: 90 MB, no shell

```dockerfile
FROM python:3.12-slim AS build
RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

FROM gcr.io/distroless/python3-debian12:nonroot
COPY --from=build /opt/venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"
WORKDIR /app
COPY app.py .
CMD ["app.py"]
```

Comparison:

| Version | Image size | Pull time (CI) | Shell? | Notable CVEs (trivy) |
|---|---|---|---|---|
| v1 naive | 1.05 GB | ~20s | yes | dozens |
| v2 slim | 160 MB | ~3s | yes | a few |
| v3 venv+slim | 145 MB | ~3s | yes | a few |
| v4 distroless | 90 MB | ~2s | **no** | minimal |

v4 is the production answer. v2 is the "I want to be able to `exec` into prod" compromise — sometimes acceptable, sometimes not (Module 13 has the rant).

---

## 7. Common mistakes & gotchas

- **Choosing Alpine reflexively.** Half the time it works great; half the time you spend a day debugging musl ABI issues or DNS quirks. Default to `:slim`; switch to Alpine when you've confirmed it works for your stack.
- **`apt-get install` without `--no-install-recommends`.** Pulls in fonts, docs, locales for "convenience." Drops 50-100 MB on the floor.
- **Carrying build tools into the final image.** `gcc`, `make`, `python3-dev`, `git` are build-stage only. Multi-stage solves this; otherwise list them in their own `RUN` and uninstall in the same `RUN`.
- **`COPY . .` after `RUN install`.** Module 05 again — your image now contains every file in your repo, including test data, secrets, dev configs. `.dockerignore` aggressively.
- **Distroless and the "no shell" surprise.** Operators expect to `docker exec app sh`. With distroless they get "exec format error." Document this. Make sure your logging is excellent because you can't shell in to investigate.
- **`scratch` without CA certs and TLS calls failing.** Add `/etc/ssl/certs/ca-certificates.crt` from another stage.
- **Stripped Go binaries triggering AV/EDR.** UPX-packed binaries especially. Some corporate AV flags them as suspicious. Skip UPX in enterprise environments.
- **Not running scanners at all.** "Our image is small" doesn't mean "our image is secure." Scan in CI. Make HIGH/CRITICAL CVEs block merges.
- **Treating scanner output as binary.** Some HIGH CVEs aren't exploitable in your config (e.g., a curl vulnerability when you don't use curl). Use `.trivyignore` / scout policies for justified exceptions, but document *why*.
- **Forgetting that base image updates are free fixes.** Many CVEs are fixed by rebuilding against today's `alpine:3.20`. Rebuild weekly even without code changes.

---

## 🎯 Key Takeaways

- **Choose your base image deliberately.** `:slim` is a safe default; `distroless` for production seriousness; `scratch` for static binaries; `alpine` only when you've tested it works.
- **Multi-stage builds are the biggest single optimization.** Build heavy, ship light — fat compilers and toolchains never reach the runtime image.
- **Clean up in the same `RUN`** that created the mess. Apt lists, pip caches, npm caches — all in the same layer, or they still live in the image forever.
- **Scan every image in CI** with trivy / scout / grype. Gate on HIGH+ severities. Rebuild weekly so base-image CVEs get patched automatically.
- **Smaller is safer.** Less code in the image = less to exploit. Distroless plus a static binary is the gold standard for production runtime images.

*[prev ← 09_registries_distribution](./09_registries_distribution.md) | [next → 11_multiarch_builds](./11_multiarch_builds.md)*
