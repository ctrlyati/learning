# 13 — Security
> **Goal:** Build a layered security posture for containers — rootless containers, minimal capabilities, seccomp/AppArmor, image signing with cosign, SBOMs, secret handling — and recognize the common misconfigurations that cause real-world container breaches.

---

## 1. The threat model — what we're actually defending against

Container security is **defense in depth**. No single control is sufficient; the goal is to make each layer of the stack as hostile to an attacker as practical so that compromising one layer doesn't lead to compromising the host.

Realistic threats (roughly in order of likelihood you'll hit them):

1. **Vulnerable dependency in your image** (CVE in a library). Most common; usually fixed by rebuilding against patched bases.
2. **Misconfigured container running as root.** Application RCE → container root → host root via volume-mount escape.
3. **Leaked secret in an image layer or env var.** Anyone with pull access reads it. Frequently happens to keys committed to repos, then COPIED into images.
4. **Compromised supply chain.** Malicious image pulled from a typosquatted name (`nginx-` instead of `nginx`), or a legitimate image whose maintainer's account was compromised.
5. **Privileged container or `docker.sock` mount.** A compromised app inside has full host access in one step.
6. **Kernel exploit escaping the namespace.** Rare but real; mitigated by user namespaces, seccomp, and keeping the kernel patched.

Each of the following sections addresses one or more of these.

---

## 2. Don't run as root inside the container

If your container is rooted (UID 0 inside), any container-escape technique gets host root. Even without escape, if you bind-mount any host path the container can chmod/chown freely. Drop privileges.

**The right pattern in Dockerfile:**

```dockerfile
FROM python:3.12-slim

RUN groupadd -r app && useradd -r -g app -u 1001 app
WORKDIR /app

# Install as root...
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# ...but copy code with right ownership and switch to non-root before CMD
COPY --chown=app:app . .
USER app
CMD ["python", "app.py"]
```

Verify:

```bash
$ docker run --rm myimg id
uid=1001(app) gid=1001(app) groups=1001(app)
```

For images that need to bind privileged ports (<1024), give the binary `CAP_NET_BIND_SERVICE` instead of running as root:

```dockerfile
RUN setcap 'cap_net_bind_service=+ep' /usr/local/bin/myapp
USER app
```

Or just use a high port (8080) and let your reverse proxy / load balancer / ingress do the privileged binding.

---

## 3. Rootless Docker — daemon as non-root too

Even with `USER` in your Dockerfile, the *daemon* (`dockerd`) typically runs as root, which is itself a target. **Rootless Docker** runs the entire daemon as an unprivileged user, using user namespaces to map the in-container "root" to a high host UID.

```bash
dockerd-rootless-setuptool.sh install
systemctl --user enable docker
systemctl --user start docker
```

Now `docker` commands talk to a user-owned daemon at `$XDG_RUNTIME_DIR/docker.sock`. Tradeoffs:

- Good: daemon compromise doesn't give root. Membership in `docker` group is no longer equivalent to root.
- Cost: some features become awkward (binding low ports, certain networking modes). Performance is slightly lower (slirp4netns vs native bridge).
- Compatibility: most workloads "just work"; some don't.

**Podman is rootless by default** — if rootless is a hard requirement, Podman often wins. Its CLI is `docker`-compatible enough that `alias docker=podman` works for many teams.

---

## 4. Capabilities — drop what you don't need

By default, a Docker container has a curated subset of Linux capabilities (powers that come with being root, like `CAP_NET_BIND_SERVICE`, `CAP_CHOWN`, etc.). The default set is *much* smaller than full root, but still more than most apps need.

```bash
docker run --cap-drop=ALL --cap-add=NET_BIND_SERVICE nginx
```

`--cap-drop=ALL --cap-add=<only what you need>` is the production gold standard. Common minimal sets:

| Workload | Capabilities needed |
|---|---|
| Bind low ports (nginx, etc.) | `NET_BIND_SERVICE` |
| Chown files (some package managers) | `CHOWN`, `FOWNER`, `DAC_OVERRIDE` |
| Most pure userland apps | none — `--cap-drop=ALL` and call it a day |

Inspect what a container has:

```bash
$ docker run --rm --cap-drop=ALL alpine sh -c 'apk add libcap 2>/dev/null && capsh --print'
Current: cap_chown,cap_dac_override,...    # default set
# vs
$ docker run --rm --cap-drop=ALL alpine sh -c 'apk add libcap 2>/dev/null && capsh --print'
Current: =                                  # nothing
```

In Compose:

```yaml
services:
  api:
    image: myapi
    cap_drop: [ALL]
    cap_add: [NET_BIND_SERVICE]
    security_opt:
      - no-new-privileges:true   # disallow setuid binaries escalating privs
```

`--security-opt no-new-privileges` is cheap defense in depth: prevents a process from gaining more privileges via `setuid` binaries even if one ends up in the image.

---

## 5. Seccomp and LSMs (AppArmor / SELinux)

### Seccomp — system call filtering

Seccomp lets you whitelist or blacklist specific kernel system calls. Docker ships a **default seccomp profile** that already blocks dozens of dangerous syscalls (`reboot`, `kexec`, `swapon`, ...). You can replace it with a stricter one:

```bash
docker run --security-opt seccomp=/path/to/my-profile.json myapp
```

Or disable it (DON'T):

```bash
docker run --security-opt seccomp=unconfined myapp
```

For most apps, the default is fine. For high-security workloads, generate a tight profile with `bane`, `oci-seccomp-bpf-hook`, or by tracing the app with `strace` and generating an allow-list.

### AppArmor (Debian/Ubuntu) and SELinux (RHEL/Fedora)

LSMs (Linux Security Modules) enforce mandatory access controls at the kernel level. Docker uses the default LSM of the host. The docker-default AppArmor profile and the `container_t` SELinux type confine containers from accessing inappropriate files on the host.

```bash
# Check what's applied
$ docker inspect --format '{{.AppArmorProfile}} {{.HostConfig.SecurityOpt}}' demo
docker-default []
```

You usually don't need to author your own; the defaults block the obvious things (e.g., writing to `/proc/sys`).

---

## 6. Never run `--privileged`, never bind `docker.sock`

Two things make almost all container hardening irrelevant:

### `--privileged`

```bash
docker run --privileged ...   # DON'T
```

`--privileged` disables almost every security feature: gives all capabilities, removes the seccomp profile, removes the LSM confinement, exposes all host devices. Container can mount the host filesystem and `chroot` into it. **Effectively, no isolation.**

Legitimate uses are rare (specific dev tools that need to manage Docker-in-Docker, or kernel-module-loading workloads). For everything else: don't.

### `-v /var/run/docker.sock:/var/run/docker.sock`

Mounting the Docker socket gives the container full control over the daemon — including the ability to spawn a new privileged container with `-v /:/host`, instantly gaining root on the host. Tools like CI runners, Portainer, etc., often want this for legitimate reasons; isolate them in a separate host or use `socket-proxy` to expose only the API endpoints they actually need.

---

## 7. Secrets — not env vars, not ARGs

Stop. Take a breath. **Do not put secrets in `ENV` or `ARG`.** Both bake into the image history, viewable to anyone with `docker pull` (or even `docker history` against your local cache):

```dockerfile
# BAD — secret in image forever
ENV API_KEY=sk_live_abc123

# BAD — secret in image and visible in build logs
ARG API_KEY
RUN curl -H "Authorization: $API_KEY" ...
```

The right tools, by lifecycle stage:

### Build-time secrets — BuildKit `--mount=type=secret`

```dockerfile
# syntax=docker/dockerfile:1.7
FROM alpine
RUN --mount=type=secret,id=mytoken,target=/run/secrets/mytoken \
    curl -H "Authorization: Bearer $(cat /run/secrets/mytoken)" https://api.example.com/something
```

```bash
echo "$MY_TOKEN" | docker buildx build --secret id=mytoken,src=/dev/stdin .
# or
docker buildx build --secret id=mytoken,src=./secrets/token.txt .
```

The secret is mounted as a file during that `RUN` and never persisted to a layer.

### Runtime secrets

| Mechanism | When to use |
|---|---|
| **Docker Compose secrets** (mounted as tmpfs files at `/run/secrets/<name>`) | Single-host Compose stacks |
| **Docker secrets** (Swarm) | Swarm clusters |
| **Kubernetes Secrets / external secret managers (Vault, AWS Secrets Manager, GCP Secret Manager)** | K8s deployments |
| **Sidecar that fetches secrets and writes to a shared tmpfs** | When the app can't be modified |

A Compose example:

```yaml
services:
  api:
    image: myapi
    environment:
      DB_PASSWORD_FILE: /run/secrets/db_password   # the app reads from a file
    secrets:
      - db_password

secrets:
  db_password:
    file: ./secrets/db_password.txt
```

The app reads `$DB_PASSWORD_FILE`'s path, then reads the secret from there. The secret is mounted on tmpfs (RAM), not written to disk inside the container.

### Don't bake secrets into images, period.

`docker history` reveals layer-by-layer commands. ARG values appear. ENV values appear. Files COPYed in are still in earlier layers even if you "delete" them in a later layer. The only safe assumption: anything inside the image is public.

Audit:

```bash
docker history --no-trunc myimg | grep -iE 'token|password|secret|key='
```

---

## 8. Image signing and provenance — cosign

Pulling an image by tag means "I trust whatever the registry currently points at." Pulling by digest gives bit-level reproducibility but not authenticity (the digest could be a malicious image you happen to have the digest for). **Signing** binds an identity to a digest.

**Cosign** (part of Sigstore) is the de-facto tool. It signs OCI artifacts and stores signatures in the same registry as a related tag (`<digest>.sig`).

```bash
# Sign (keyless — uses an OIDC identity, no key file to manage)
cosign sign ghcr.io/me/app@sha256:abc123...

# Verify
cosign verify ghcr.io/me/app@sha256:abc123... \
  --certificate-identity-regexp '^https://github.com/me/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

In CI:

```yaml
- uses: sigstore/cosign-installer@v3
- run: |
    cosign sign --yes ghcr.io/${{ github.repository }}@${{ steps.build.outputs.digest }}
```

Then in your deploy pipeline, *verify* before running:

```bash
cosign verify ghcr.io/me/app@${DIGEST} ... || exit 1
docker pull ghcr.io/me/app@${DIGEST}
```

Kubernetes integrations (`policy-controller`, Kyverno, OPA Gatekeeper) can enforce verification cluster-wide.

---

## 9. SBOMs — what's actually in this image?

A **Software Bill of Materials** lists every package, library, and version in an image. Two common formats: SPDX and CycloneDX. Generate one with `syft`:

```bash
syft ghcr.io/me/app:1.0 -o spdx-json > app-sbom.spdx.json
syft ghcr.io/me/app:1.0 -o cyclonedx-json > app-sbom.cdx.json
```

Modern BuildKit can generate SBOMs as part of the build, attached as an OCI attestation:

```bash
docker buildx build \
  --sbom=true \
  --provenance=true \
  --push \
  -t ghcr.io/me/app:1.0 .
```

Why SBOMs matter:
- When the next Log4Shell hits, you can grep SBOMs across your fleet in seconds: "which of our 400 images have log4j-core 2.x?"
- Compliance (US Executive Order 14028, EU CRA, FedRAMP, etc.) increasingly requires SBOMs for shipped software.
- Vulnerability scanners (grype) can run against SBOMs without re-scanning images — much faster.

---

## 10. A worked example: a security-hardened service

Goal: a Python web service with every reasonable hardening in place.

```dockerfile
# syntax=docker/dockerfile:1.7
FROM python:3.12-slim AS build
WORKDIR /app

RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"
COPY requirements.txt .
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --no-cache-dir -r requirements.txt

FROM gcr.io/distroless/python3-debian12:nonroot
COPY --from=build /opt/venv /opt/venv
WORKDIR /app
COPY app.py .
ENV PATH="/opt/venv/bin:$PATH"
EXPOSE 8080
USER nonroot
ENTRYPOINT ["python", "app.py"]
```

```yaml
# compose.yml — runtime hardening
services:
  api:
    image: ghcr.io/me/api@sha256:abcdef...   # pin by digest
    read_only: true                          # root FS is RO
    tmpfs:
      - /tmp:size=64m
      - /run:size=8m
    cap_drop: [ALL]
    cap_add: [NET_BIND_SERVICE]              # only if needed
    security_opt:
      - no-new-privileges:true
      - seccomp=default                      # explicit
    pids_limit: 100
    mem_limit: 512m
    cpus: 1.0
    user: "65532:65532"                      # nonroot
    networks: [internal, public]
    depends_on:
      db: { condition: service_healthy }
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"]
      interval: 30s
      timeout: 3s
      retries: 3
    restart: unless-stopped
    secrets:
      - db_password
    environment:
      DATABASE_URL: postgres://app@db/app
      DB_PASSWORD_FILE: /run/secrets/db_password

  db:
    image: postgres:16-alpine
    secrets: [db_password]
    environment:
      POSTGRES_PASSWORD_FILE: /run/secrets/db_password
    volumes: [pgdata:/var/lib/postgresql/data]
    networks: [internal]
    restart: unless-stopped

networks:
  internal:
    internal: true                            # no internet from these
  public:

volumes:
  pgdata:

secrets:
  db_password:
    file: ./secrets/db_password.txt
```

Plus a CI gate (Module 15 expands on this):

```yaml
# .github/workflows/release.yml (excerpt)
- name: Build and push
  id: build
  uses: docker/build-push-action@v6
  with:
    push: true
    tags: ghcr.io/${{ github.repository }}:${{ github.sha }}
    sbom: true
    provenance: true

- name: Vulnerability scan (fail on HIGH+)
  uses: aquasecurity/trivy-action@master
  with:
    image-ref: ghcr.io/${{ github.repository }}:${{ github.sha }}
    severity: CRITICAL,HIGH
    exit-code: '1'
    ignore-unfixed: true

- name: Sign image (keyless)
  uses: sigstore/cosign-installer@v3
- run: cosign sign --yes ghcr.io/${{ github.repository }}@${{ steps.build.outputs.digest }}
```

Together: minimal image (distroless), nonroot, capabilities dropped, read-only root, seccomp + no-new-privs, pinned digest, signed, scanned, segmented network, secrets via mounted files. That's the production posture.

---

## 11. Common mistakes & gotchas

- **Running as root inside the image** "just for now." It stays root forever. Add `USER nonroot` from day one.
- **`--privileged` in a Compose file checked in to git.** Code review must catch this. Static-analyze your Compose / Dockerfiles for `privileged: true` and reject.
- **Mounting `/var/run/docker.sock` "because Portainer/CI needs it."** Use socket-proxy (e.g., `tecnativa/docker-socket-proxy`) to expose only the API endpoints needed. Otherwise the container has god-mode.
- **Secrets in `ENV` because "we'll fix it later."** It's in image history forever, in `docker inspect`, in CI logs. Fix on the first commit, not the third.
- **`ARG SECRET` in build args.** Anyone can `docker history` the image and read the `ARG`. Use `--mount=type=secret`.
- **Trusting `:latest` from Docker Hub.** Typosquats happen. Pulls from compromised accounts happen. Pin tags, verify signatures, mirror to your own registry.
- **No memory/PID limits.** A leak or runaway recursion takes down the host. Always limit (Module 12).
- **Disabling seccomp / AppArmor because "the app didn't work."** The default Docker profiles block <1% of syscalls; if your app fails, investigate which syscall and either fix the app or whitelist that specific call, don't disable the whole profile.
- **Scanning images but never patching.** A trivy report nobody acts on is paperwork. Wire it into pre-merge / pre-deploy gates with severity thresholds.
- **Believing distroless = secure.** Distroless is small and shellless, not magically immune to vulnerable libs. Still scan.
- **Forgetting your CI is a high-privilege environment.** A compromised CI runner can sign malicious images with your keys. Use ephemeral runners, short-lived OIDC tokens (cosign keyless), and require multi-party review for changes to your release pipelines.

---

## 🎯 Key Takeaways

- **Defense in depth.** No single control is sufficient — non-root + capabilities dropped + seccomp + read-only FS + signed images + scanned + network-segmented adds up to a hard target.
- **Never run as root inside the container.** `USER` is one line; the cost of skipping it is potentially total host compromise.
- **Treat the Docker daemon and `docker.sock` as crown jewels.** Anyone with access to either has host root. Prefer rootless daemons, restrict the `docker` group, and proxy the socket if you must share it.
- **Secrets are not env vars, not build args, and not bytes in any image layer.** Use BuildKit `--mount=type=secret` at build time, mounted files / external secret managers at runtime.
- **Sign and scan everything.** Cosign for authenticity (this is the bytes you built), trivy/grype for vulnerabilities (this is what's inside), SBOMs for "tell me which images have *that library* when CVE-of-the-day drops."

*[prev ← 12_runtime_internals](./12_runtime_internals.md) | [next → 14_observability](./14_observability.md)*
