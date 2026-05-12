# 09 — Registries & Distribution
> **Goal:** Push and pull images confidently across Docker Hub, GHCR, ECR/ACR/GAR, and private registries. Understand auth flows, image references end-to-end, manifests/digests, and how to operate a private registry when you need one.

---

## 1. What a registry is, mechanically

A registry is an HTTPS server speaking the **OCI Distribution Spec** (formerly Docker Registry V2 API). It stores **blobs** (the layer tarballs and config JSONs) and **manifests** (small JSON documents that describe an image: which blobs, which config, which platform). Tags are pointers from human-friendly names to manifests; digests are cryptographic SHA-256 references to specific manifest content.

You interact with it via four primitives:

```bash
docker login registry.example.com      # auth
docker pull registry.example.com/team/app:1.0       # GET manifest + blobs
docker push registry.example.com/team/app:1.0       # PUT manifest + blobs
docker logout registry.example.com
```

Under the hood, a push does:
1. PUT each blob (skipping ones the registry already has — content-addressable, so the registry can dedupe).
2. PUT the image config blob.
3. PUT the manifest pointing at those blobs.
4. Update the tag to point at the manifest digest.

This is why pushing a near-identical image is fast: most blobs already exist.

You can poke a registry with raw HTTP. For a public Docker Hub image:

```bash
curl -s https://registry-1.docker.io/v2/library/alpine/manifests/3.20 \
  -H "Authorization: Bearer <token>" \
  -H "Accept: application/vnd.oci.image.index.v1+json" | jq .
```

(Docker Hub requires a token from `auth.docker.io`; many private registries are simpler.)

---

## 2. The major registries you'll encounter

| Registry | Hostname | Public free tier | Auth model | Common use |
|---|---|---|---|---|
| **Docker Hub** | `docker.io` | Yes (rate-limited) | Username + PAT | Default for open-source images |
| **GitHub Container Registry (GHCR)** | `ghcr.io` | Yes (unlimited public) | GH token / PAT | Open-source + private alongside code |
| **AWS ECR** | `*.dkr.ecr.region.amazonaws.com` | No (pay-as-you-go) | IAM (temporary tokens) | AWS workloads |
| **Azure ACR** | `*.azurecr.io` | Free tier | AAD / service principals | Azure workloads |
| **Google Artifact Registry (GAR)** | `*-docker.pkg.dev` | Free tier | Google IAM | GCP workloads |
| **GitLab Container Registry** | `registry.gitlab.com` | Yes | GitLab token | Alongside GitLab repos |
| **Quay (Red Hat)** | `quay.io` | Yes | Various | Enterprise / OpenShift |
| **Harbor** | self-hosted | n/a | OIDC, robots | Air-gapped enterprise |
| **Self-hosted `registry:2`** | self-hosted | n/a | basic / token | Local cache / private mirror |

Default registry is `docker.io`. If you don't prefix the image name with a hostname, Docker assumes Docker Hub. Everything else needs a hostname.

```bash
docker pull alpine                          # → docker.io/library/alpine:latest
docker pull myteam/api:1.0                  # → docker.io/myteam/api:1.0
docker pull ghcr.io/me/app:1.0              # → ghcr.io/me/app:1.0
docker pull 123456.dkr.ecr.us-east-1.amazonaws.com/api:1.0
```

### Docker Hub rate limits (the thing that bites every team eventually)

Unauthenticated pulls from Docker Hub are limited to 100 per 6 hours per IP. CI runners share an IP. Authenticated free accounts get 200/6h. Once your CI starts pulling `node:20-slim` and `postgres:16` repeatedly, you'll hit it.

Fixes, in order of effort:
1. **Authenticate in CI** with a Docker Hub PAT — cheapest fix.
2. **Use a registry mirror** — point your daemon's `registry-mirrors` config at a local cache (or your cloud's mirror, e.g. AWS public ECR mirror).
3. **Move base images to your private registry** — once.

---

## 3. Authentication, the right way

### Docker Hub

Create a **Personal Access Token** in Account Settings → Security → New Access Token. Never use your account password.

```bash
echo $DOCKERHUB_PAT | docker login -u myusername --password-stdin
```

Credentials are stored in `~/.docker/config.json`:

```json
{
  "auths": {
    "https://index.docker.io/v1/": {
      "auth": "base64-of-user:pat"
    }
  }
}
```

That `base64-of-user:pat` is **not encryption** — it's base64. Treat `config.json` like a secret.

Better: use a **credential helper** that pulls credentials from your OS keychain (macOS Keychain, Windows Credential Manager, libsecret, pass). Docker Desktop sets this up automatically. On Linux servers, configure `credsStore`:

```json
{ "credsStore": "secretservice" }
```

### GHCR

Create a GitHub PAT (classic) with `read:packages` (and `write:packages` if you'll push). Or use a fine-grained token with package permissions.

```bash
echo $GH_PAT | docker login ghcr.io -u myusername --password-stdin
```

In GitHub Actions, the auto-provided `GITHUB_TOKEN` works directly:

```yaml
- run: echo "${{ secrets.GITHUB_TOKEN }}" | docker login ghcr.io -u ${{ github.actor }} --password-stdin
```

### AWS ECR

ECR uses short-lived tokens (12-hour TTL) backed by IAM:

```bash
aws ecr get-login-password --region us-east-1 \
  | docker login --username AWS --password-stdin 123456.dkr.ecr.us-east-1.amazonaws.com
```

In production (on EC2/EKS/ECS), use the **`amazon-ecr-credential-helper`** so Docker fetches a fresh token transparently when it expires.

### Azure ACR

```bash
az acr login --name myregistry
# or service-principal-based:
docker login myregistry.azurecr.io -u $SP_APP_ID -p $SP_PASSWORD
```

### Google Artifact Registry

```bash
gcloud auth configure-docker us-central1-docker.pkg.dev
# Now docker push/pull use your gcloud credentials.
```

---

## 4. Push/pull workflow in practice

A typical release flow:

```bash
# Build with a meaningful tag
docker build -t myapp:1.4.2 -t myapp:latest .

# Add registry-prefixed tags (a tag is just a reference; the image bits are the same)
docker tag myapp:1.4.2 ghcr.io/myorg/myapp:1.4.2
docker tag myapp:1.4.2 ghcr.io/myorg/myapp:1.4
docker tag myapp:1.4.2 ghcr.io/myorg/myapp:latest

# Auth, push
echo $GH_PAT | docker login ghcr.io -u me --password-stdin
docker push ghcr.io/myorg/myapp:1.4.2
docker push ghcr.io/myorg/myapp:1.4
docker push ghcr.io/myorg/myapp:latest
```

Output:

```
The push refers to repository [ghcr.io/myorg/myapp]
8a4b2c3d4e5f: Pushed
b5c6d7e8f9a0: Layer already exists
c6d7e8f9a0b1: Layer already exists
1.4.2: digest: sha256:9f8e7d6c5b4a... size: 1789
```

"Layer already exists" lines are the content-addressable dedup at work — those blobs were pushed by earlier images.

Pulling:

```bash
docker pull ghcr.io/myorg/myapp:1.4.2
# or pull by digest for immutability
docker pull ghcr.io/myorg/myapp@sha256:9f8e7d6c5b4a...
```

In production, deploy by digest, not tag — that way "the same version" really means *the exact same bytes*, immune to someone overwriting the tag.

### Looking at remote manifests without pulling

`docker manifest inspect` and `docker buildx imagetools inspect` query the registry directly:

```bash
$ docker buildx imagetools inspect ghcr.io/myorg/myapp:1.4.2
Name:      ghcr.io/myorg/myapp:1.4.2
MediaType: application/vnd.oci.image.index.v1+json
Digest:    sha256:9f8e7d6c5b4a3...

Manifests:
  Name:      ghcr.io/myorg/myapp:1.4.2@sha256:abc...
  MediaType: application/vnd.oci.image.manifest.v1+json
  Platform:  linux/amd64

  Name:      ghcr.io/myorg/myapp:1.4.2@sha256:def...
  MediaType: application/vnd.oci.image.manifest.v1+json
  Platform:  linux/arm64
```

This is a **multi-arch index** (Module 11). The top-level manifest is itself a list pointing to per-platform manifests.

---

## 5. Pulling from private registries on the *target* host

Pushing is half the story. The other half is the production host pulling. The host needs to authenticate.

**Single-host with Docker:** `docker login` once on the host, credentials persist.

**Compose with private images:** same — make sure the host is logged in.

**Kubernetes:** create an `imagePullSecret` containing a `~/.docker/config.json`-style auth blob, reference it on the Pod. Or use a credential helper / workload identity (e.g. IRSA on EKS) so pods get credentials via IAM, no static secrets.

**CI deploys:** in your deploy step, log in to the registry (with a token that has `pull` rights only, scoped to the relevant repos) before `docker compose pull && docker compose up -d`.

Principle: **separate push credentials from pull credentials.** Build/push tokens can write; deploy tokens can only read. Compromised deploy credentials shouldn't be able to upload malicious images.

---

## 6. Running your own registry

Sometimes you need a private registry: air-gapped envs, latency-sensitive pulls, regulated industries. The CNCF Distribution project (the `registry:2` image) is the open-source reference.

Simplest: insecure local registry for testing.

```bash
docker run -d --name registry \
  -p 5000:5000 \
  -v registry-data:/var/lib/registry \
  registry:2

docker tag myapp:1.0 localhost:5000/myapp:1.0
docker push localhost:5000/myapp:1.0
docker pull localhost:5000/myapp:1.0
```

For real use:
- TLS (give it certs via env vars or a reverse proxy).
- Basic auth (htpasswd file).
- Storage backend (S3, Azure Blob, GCS — built-in).
- Garbage collection scripts so deleted tags actually reclaim disk.

```yaml
# docker-compose.yml — production-ish private registry
services:
  registry:
    image: registry:2
    restart: always
    ports: ["5000:5000"]
    environment:
      REGISTRY_HTTP_TLS_CERTIFICATE: /certs/tls.crt
      REGISTRY_HTTP_TLS_KEY: /certs/tls.key
      REGISTRY_AUTH: htpasswd
      REGISTRY_AUTH_HTPASSWD_REALM: Registry
      REGISTRY_AUTH_HTPASSWD_PATH: /auth/htpasswd
      REGISTRY_STORAGE: s3
      REGISTRY_STORAGE_S3_REGION: us-east-1
      REGISTRY_STORAGE_S3_BUCKET: my-docker-registry
    volumes:
      - ./certs:/certs:ro
      - ./auth:/auth:ro
```

For anything beyond "internal cache," consider **Harbor**: registry + vuln scanning + replication + RBAC + image signing. It's what most regulated environments end up running.

### Registry mirrors

If you just need to cut Docker Hub rate limits and latency, configure the daemon to use a mirror:

```json
// /etc/docker/daemon.json
{
  "registry-mirrors": ["https://mirror.gcr.io"]
}
```

`mirror.gcr.io` is Google's free public mirror of Docker Hub. Solves rate limits for many shops with zero infrastructure.

---

## 7. A worked example: build, push, deploy

Project: simple service, push to GHCR, deploy by digest.

```bash
# CI build step
$ docker build -t ghcr.io/myorg/api:sha-$GITHUB_SHA \
               -t ghcr.io/myorg/api:1.4.2 \
               -t ghcr.io/myorg/api:latest .

$ echo $GHCR_TOKEN | docker login ghcr.io -u $GHCR_USER --password-stdin
Login Succeeded

$ docker push --all-tags ghcr.io/myorg/api

# Capture the digest for immutable deploy
$ DIGEST=$(docker inspect --format='{{index .RepoDigests 0}}' ghcr.io/myorg/api:1.4.2)
$ echo $DIGEST
ghcr.io/myorg/api@sha256:9f8e7d6c5b4a3210...

# Production deploy uses the digest, not the tag
$ ssh prod 'docker pull '$DIGEST' && docker tag '$DIGEST' api:current && docker compose up -d'
```

In Compose:

```yaml
services:
  api:
    image: ghcr.io/myorg/api@sha256:9f8e7d6c5b4a3210...
```

Bit-identical deploys. Even if someone rewrites the `:1.4.2` tag (incident or malice), this Compose file still pulls the verified bytes.

---

## 8. Common mistakes & gotchas

- **Plaintext credentials in `~/.docker/config.json`.** Base64 isn't encryption. Use a credential helper (`credsStore`) or short-lived tokens (ECR helper, `gcloud configure-docker`).
- **Same token for push and pull.** Compromised deploy creds shouldn't be able to push. Separate roles, separate tokens.
- **Pushing without prefixing the registry.** `docker push myapp:1.0` defaults to Docker Hub. You probably wanted `ghcr.io/me/myapp:1.0`. Tag first.
- **Forgetting to push all tags.** `docker push myapp:1.4.2` pushes only that tag. To push every tag for a repo: `docker push --all-tags ghcr.io/me/myapp`.
- **Docker Hub rate limit hitting CI out of nowhere.** Authenticate CI, or move base images to your own registry/mirror.
- **Mutable tags assumed immutable.** Tags are pointers; they can be rewritten. For real reproducibility deploy by digest.
- **ECR token expiry.** 12 hours, then `unauthorized: authentication required`. Install the credential helper.
- **Private registry with self-signed certs and "x509: certificate signed by unknown authority."** Add the CA to the daemon's trust (`/etc/docker/certs.d/registry.example.com/ca.crt`) or use a real cert (Let's Encrypt is free).
- **Registry filling up and you can't figure out why.** Tag deletes don't reclaim blobs until garbage collection runs. Run `registry garbage-collect /etc/docker/registry/config.yml` periodically (or use Harbor which handles this).
- **Pushing development-quality `:latest` to a shared registry.** `:latest` becomes meaningless. Only push `:latest` from a release pipeline; CI builds should tag by commit SHA.
- **Image lives in registry A, you `docker pull` and it goes to Docker Hub.** Always include the registry hostname for non-Hub images, every time. Aliases in shell don't help when you copy commands into automation.

---

## 🎯 Key Takeaways

- **A registry is just an HTTPS API for blobs + manifests.** Pushes are content-addressable and dedupe globally; pulls fetch only missing layers.
- **Use the right registry for the right job:** Docker Hub for OSS bases (with auth!), GHCR for code-adjacent storage, cloud-native registries (ECR/ACR/GAR) for workloads inside that cloud, Harbor self-hosted for regulated environments.
- **Deploy by digest, not tag**, in production. Tags lie; SHA-256 doesn't.
- **Separate push and pull credentials**, and use credential helpers / cloud workload identity instead of long-lived static tokens whenever possible.
- **Plan for Docker Hub rate limits early** — auth your CI, configure a registry mirror, or mirror frequently-used base images to your own registry. Hitting the limit during an incident is a bad day.

*[prev ← 08_docker_compose](./08_docker_compose.md) | [next → 10_image_optimization](./10_image_optimization.md)*
