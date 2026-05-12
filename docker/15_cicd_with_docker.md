# 15 — CI/CD with Docker
> **Goal:** Build, test, scan, sign, and push container images from CI pipelines reliably and quickly. Master cache strategies (GHA cache, registry cache), set up vulnerability scanning gates, and produce immutable, verifiable artifacts.

---

## 1. What a "good" container CI pipeline does

A production-grade pipeline for a containerized service does, in order:

1. **Lint** — `hadolint` on the Dockerfile, fail on style/security smells.
2. **Build** — `docker buildx build` with cache (for speed) and multi-arch (for portability).
3. **Test** — run unit/integration tests *inside the built image*, not in a different environment.
4. **Scan** — trivy/scout/grype on the built image; fail on HIGH+ vulnerabilities.
5. **Sign** — `cosign sign` the image so downstream can verify authenticity.
6. **Push** — to your registry, tagged by commit SHA *and* (on release) semantic version.
7. **Generate SBOM + provenance** as OCI attestations attached to the image.
8. **Trigger deploy** — to staging on every main-branch push, to prod on tagged release.

Skipping any of these is "fine" for hobby projects; you'll regret it for production.

---

## 2. Caching strategies — the speed lever

A CI pipeline that takes 15 minutes is a pipeline developers avoid running. Cache aggressively.

### Three levels of cache, in increasing order of effectiveness

1. **Docker's own layer cache (`docker build`).** Useless in CI without persistence — every job starts with an empty cache.
2. **BuildKit cache mounts** (`--mount=type=cache`, Module 05). In-build caches for apt/pip/npm/go-build. Persistent only if you persist the *whole BuildKit cache* externally.
3. **External cache backends** — `type=gha`, `type=registry`, `type=s3`, etc. These persist the build's cache layers between CI runs.

In GitHub Actions, the **GHA cache backend** is the easiest win:

```yaml
- uses: docker/setup-buildx-action@v3
- uses: docker/build-push-action@v6
  with:
    context: .
    tags: ghcr.io/${{ github.repository }}:${{ github.sha }}
    push: true
    cache-from: type=gha
    cache-to: type=gha,mode=max
```

`type=gha` is per-repo, ~10 GB free, automatic. `mode=max` stores intermediate layers (not just final ones), which makes multi-stage caches actually useful.

For non-GHA CI (GitLab, Jenkins, CircleCI), the **registry cache** works everywhere:

```yaml
cache-from: type=registry,ref=ghcr.io/me/app:buildcache
cache-to: type=registry,ref=ghcr.io/me/app:buildcache,mode=max
```

Push your build cache to a separate tag in your image registry. Every CI runner with registry access can use it.

For very large monorepos, **S3 / blob cache** scales to whatever budget you have:

```yaml
cache-to: type=s3,region=us-east-1,bucket=my-ci-cache,name=app
cache-from: type=s3,region=us-east-1,bucket=my-ci-cache,name=app
```

### Cache invalidation pitfalls in CI

- **Multi-arch builds need per-arch cache scopes.** `cache-to: type=gha,scope=amd64` and a separate `scope=arm64` keeps them from stepping on each other.
- **`cache-to: mode=min` (default)** stores only final stage layers. Multi-stage builds get terrible cache reuse with min. Use `mode=max`.
- **Long-lived cache rot.** If the cache is months old and your base image moved on, hits are unlikely and you're paying transfer time. Periodic cache eviction (or short cache TTLs in GHA) is healthy.

---

## 3. Test inside the image — "the build is the test environment"

Anti-pattern: build the image, then run tests in a separate Python/Node/Go environment. Tests pass; image runs differently in production. Better: use multi-stage builds and run tests in a stage that uses the built artifact.

```dockerfile
# syntax=docker/dockerfile:1.7

FROM node:20 AS base
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY . .

FROM base AS test
RUN npm test
RUN npm run lint

FROM base AS build
RUN npm run build && npm prune --production

FROM gcr.io/distroless/nodejs20-debian12
WORKDIR /app
COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/dist ./dist
COPY --from=build /app/package.json ./
USER nonroot
CMD ["dist/server.js"]
```

In CI, run tests as a target:

```yaml
- uses: docker/build-push-action@v6
  with:
    context: .
    target: test           # build only up to the test stage
    cache-from: type=gha
    cache-to: type=gha,mode=max
```

Tests fail → build fails → no image pushed. Tests pass → continue to a separate `build-push` step that builds the final stage and pushes.

Alternatively, run tests with `docker compose run`:

```yaml
- run: docker compose -f compose.yml -f compose.test.yml run --rm api npm test
```

This brings up the full app stack (DB, cache, etc.) and runs the test suite against it — proper integration testing in CI.

---

## 4. Full GitHub Actions pipeline — annotated

```yaml
# .github/workflows/release.yml
name: build-and-release

on:
  push:
    branches: [main]
    tags: ['v*']
  pull_request:

permissions:
  contents: read
  packages: write          # push to GHCR
  id-token: write          # cosign keyless signing via OIDC

jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Lint Dockerfile
        uses: hadolint/hadolint-action@v3.1.0
        with:
          dockerfile: Dockerfile

      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3

      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Compute tags
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=ref,event=branch          # main → :main
            type=ref,event=pr              # PR #42 → :pr-42
            type=semver,pattern={{version}}      # v1.2.3 → :1.2.3
            type=semver,pattern={{major}}.{{minor}}   # v1.2.3 → :1.2
            type=sha,prefix=sha-           # always :sha-<7chars>

      - name: Build (test stage) — fails on test failure
        uses: docker/build-push-action@v6
        with:
          context: .
          target: test
          load: false
          cache-from: type=gha
          cache-to: type=gha,mode=max

      - name: Build and push image
        id: build
        uses: docker/build-push-action@v6
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: ${{ github.event_name != 'pull_request' }}
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
          sbom: true
          provenance: true

      - name: Scan with trivy (HIGH+ fails the build)
        uses: aquasecurity/trivy-action@master
        with:
          image-ref: ghcr.io/${{ github.repository }}@${{ steps.build.outputs.digest }}
          severity: CRITICAL,HIGH
          exit-code: '1'
          ignore-unfixed: true
          format: 'sarif'
          output: 'trivy.sarif'

      - name: Upload trivy SARIF to GitHub code scanning
        if: always()
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: trivy.sarif

      - name: Install cosign
        if: github.event_name != 'pull_request'
        uses: sigstore/cosign-installer@v3

      - name: Sign image (keyless via OIDC)
        if: github.event_name != 'pull_request'
        env:
          DIGEST: ${{ steps.build.outputs.digest }}
          TAGS: ${{ steps.meta.outputs.tags }}
        run: |
          for tag in $TAGS; do
            cosign sign --yes "${tag}@${DIGEST}"
          done

  deploy-staging:
    needs: ci
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    environment: staging
    steps:
      - name: Deploy
        run: |
          ssh deploy@staging "
            docker pull ghcr.io/${{ github.repository }}:sha-${GITHUB_SHA::7} &&
            docker compose -f /srv/app/compose.yml up -d
          "
```

What's happening in plain English:
- Lint Dockerfile before anything else (cheap, fast feedback).
- Set up buildx (for caching, multi-arch, attestations).
- Compute a set of tags from branch/PR/version metadata.
- Run tests as a build target — fails fast.
- Build for amd64 + arm64, push (unless it's a PR — PRs build but don't push), with SBOM and provenance attached.
- Scan for vulnerabilities — fail on HIGH or CRITICAL.
- Upload scan results to GitHub's code scanning UI.
- Sign the image with keyless cosign (OIDC).
- On main branch, deploy to staging.

A tagged release (`git tag v1.2.3 && git push --tags`) produces signed `:1.2.3`, `:1.2`, `:sha-abc1234` tags ready to deploy to production.

---

## 5. GitLab CI equivalent

GitLab's pattern is similar but with its own glue:

```yaml
# .gitlab-ci.yml
stages: [build, scan, deploy]

variables:
  DOCKER_BUILDKIT: "1"
  IMAGE: ${CI_REGISTRY_IMAGE}:${CI_COMMIT_SHORT_SHA}

build:
  stage: build
  image: docker:27
  services: [docker:27-dind]
  before_script:
    - docker buildx create --use --name multi
    - echo "$CI_REGISTRY_PASSWORD" | docker login -u "$CI_REGISTRY_USER" --password-stdin "$CI_REGISTRY"
  script:
    - |
      docker buildx build \
        --platform linux/amd64,linux/arm64 \
        --cache-from type=registry,ref=${CI_REGISTRY_IMAGE}:buildcache \
        --cache-to   type=registry,ref=${CI_REGISTRY_IMAGE}:buildcache,mode=max \
        --sbom=true --provenance=true \
        --tag $IMAGE \
        --push .

scan:
  stage: scan
  image: aquasec/trivy:latest
  script:
    - trivy image --severity CRITICAL,HIGH --ignore-unfixed --exit-code 1 $IMAGE

deploy:
  stage: deploy
  only: [main]
  script:
    - ssh deploy@prod "docker pull $IMAGE && docker compose -f /srv/app/compose.yml up -d"
```

Same pattern: build with cache, scan with severity gate, deploy on the right branch.

---

## 6. Self-hosted runners — when GHA shared runners aren't enough

For large or arm64 builds, GitHub's shared runners can be slow. Options:

- **GitHub-hosted larger runners / arm64 runners** (`ubuntu-latest-large`, `ubuntu-24.04-arm`) — paid, but a one-line workflow change.
- **Self-hosted runners on your own hardware.** Faster iteration; you own the build cache too. Security caveat: a malicious PR can run arbitrary code on your runner. Use ephemeral runners (one job, then discarded) and never run PR jobs from forks on self-hosted runners without review gates.
- **ARC (Actions Runner Controller) on K8s** — autoscaling runners that come up on demand. Industry default for serious GHA-on-K8s setups.

For the build itself, **remote BuildKit instances** can be shared across CI jobs to give a persistent, warm cache without registry round-trips. `docker buildx create --driver remote tcp://buildkit:1234` connects a CI job to a long-running BuildKit pod.

---

## 7. Promoting images through environments

A common pattern: build once, promote the *same digest* through dev → staging → prod.

```bash
# Build (happens once in CI)
DIGEST=$(docker buildx imagetools inspect ghcr.io/me/app:sha-abc123 --format '{{json .Manifest.Digest}}' | tr -d '"')

# Promote a digest to a stage by adding a new tag (no rebuild)
docker buildx imagetools create \
  --tag ghcr.io/me/app:staging \
  ghcr.io/me/app@$DIGEST

# Later, promote staging → prod the same way
docker buildx imagetools create \
  --tag ghcr.io/me/app:prod \
  ghcr.io/me/app@$DIGEST
```

`imagetools create` is purely a manifest-level operation; no layers are uploaded again. The `:staging` and `:prod` tags now point at the exact bytes that were built, scanned, and signed once.

Verification before deploy:

```bash
cosign verify ghcr.io/me/app@$DIGEST \
  --certificate-identity-regexp '^https://github.com/me/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

If the signature doesn't verify, fail the deploy. This catches both tampering and unsigned-by-mistake images.

---

## 8. Common mistakes & gotchas

- **No cache → 15-minute builds.** Add `cache-from`/`cache-to: type=gha,mode=max` (or registry) on day one.
- **`mode=min` cache for multi-stage builds.** Cache hits look fine; rebuild times are still bad. Use `mode=max`.
- **Building on PRs with `push: true`.** Now `:latest` follows every feature branch. Push only on main / tags. PR builds build-only.
- **Tagging `:latest` from CI builds.** Means "latest of whatever ran last" — usually not what you want. Reserve `:latest` for release tags only.
- **Secrets in `--build-arg`.** They appear in image history and CI logs. Use `--secret id=...,src=...` with the BuildKit syntax.
- **No vulnerability gate.** Scanner runs, output ignored, vulns ship. Use `exit-code: 1` and `severity: CRITICAL,HIGH`.
- **`ignore-unfixed: true` misuse.** This skips CVEs with no upstream fix. Reasonable for "we can't do anything about it"; not reasonable if you're using a stale base. Pair with regular base-image bumps.
- **Self-hosted runners running fork PRs.** Major supply-chain risk. Configure your repo to require approval for first-time contributors before workflows run.
- **Multi-arch builds without per-platform cache scope.** Cross-pollination makes the cache useless. `scope=${{ matrix.platform }}`.
- **Building on every push to every branch, including chat-bot doc updates.** Filter `paths-ignore` for docs-only changes.
- **Signing tags instead of digests.** Tags are mutable. Signing `:latest` is mostly meaningless. Sign by digest.
- **Forgetting `permissions:` for OIDC.** Cosign keyless needs `id-token: write`. Without it, signing silently fails (or asks for a key file you don't have).
- **No image promotion model.** Building separately for each environment guarantees drift. Build once, promote the digest.

---

## 🎯 Key Takeaways

- **A complete pipeline:** lint → build (with cache, multi-arch) → test inside the image → scan with severity gate → sign with cosign → push by SHA + version + branch → optionally deploy by digest. Skipping the back half is "fine, until it isn't."
- **Cache aggressively.** GHA cache or registry cache with `mode=max`. The difference between a 2-minute and a 15-minute build is the difference between a beloved CI and a hated one.
- **Test inside the built image**, not in a parallel environment. Use multi-stage build targets in CI to fail fast on test failures before you push.
- **Build once, promote by digest.** Tags are mutable pointers; digests are immutable bytes. Production deploys should reference digests so you know the bits.
- **Sign and scan as build gates**, not as paperwork. Cosign + trivy + SBOMs/provenance attached as OCI attestations give you a verifiable supply chain — and an audit trail when you need one.

*[prev ← 14_observability](./14_observability.md) | [next → 16_production_and_next](./16_production_and_next.md)*
