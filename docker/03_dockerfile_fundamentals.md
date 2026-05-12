# 03 — Dockerfile Fundamentals
> **Goal:** Read and write Dockerfiles fluently: understand every common instruction, the difference between `CMD` and `ENTRYPOINT` (the #1 source of confusion), and why instruction order matters.

---

## 1. A Dockerfile is a recipe for an image

A Dockerfile is a plain-text file with a sequence of **instructions**. Each instruction creates (or modifies) a layer in the resulting image. The Docker build engine reads it top-to-bottom, executes each instruction, and snapshots the result.

Minimum viable example:

```dockerfile
# Dockerfile
FROM alpine:3.20
RUN apk add --no-cache curl
CMD ["curl", "--version"]
```

Build and run:

```bash
docker build -t curler .
docker run --rm curler
```

Output:

```
curl 8.9.1 (x86_64-alpine-linux-musl) libcurl/8.9.1 OpenSSL/3.3.2 zlib/1.3.1 ...
```

Three instructions, three layers (well — `CMD` is metadata-only, so one filesystem layer). That's the whole pattern: pick a base, install stuff, declare what runs by default.

---

## 2. The core instruction set

Here's every instruction you need for 95% of Dockerfiles. We'll deep-dive `CMD` vs `ENTRYPOINT` in §3 because they deserve their own conversation.

### `FROM` — the base layer

```dockerfile
FROM python:3.12-slim
```

Every Dockerfile starts with `FROM` (or `ARG` then `FROM`). It declares the base image — what your image inherits. Choosing well matters enormously:

- `python:3.12` — full Debian, ~120 MB before your code. Comfortable, big.
- `python:3.12-slim` — Debian slim, ~50 MB. Sweet spot.
- `python:3.12-alpine` — Alpine, ~20 MB. Tiny, but musl libc breaks some wheels.
- `gcr.io/distroless/python3` — no shell, no package manager, ~50 MB. Production-grade.
- `scratch` — literally nothing. For static binaries (Go, Rust).

You can also build multi-stage with multiple `FROM`s (Module 05).

### `RUN` — execute a command at build time

```dockerfile
RUN apt-get update && apt-get install -y --no-install-recommends \
        curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
```

`RUN` creates a new layer with whatever filesystem changes that command produced. Two forms:

```dockerfile
RUN apt-get update          # shell form: runs in /bin/sh -c
RUN ["apt-get", "update"]   # exec form: no shell, direct exec
```

Shell form is more common for build-time scripts. Exec form avoids shell escaping issues but doesn't expand shell variables.

**The single biggest lesson:** combine related `RUN` steps with `&&` to keep the layer count down and let the cleanup happen *in the same layer*. This:

```dockerfile
RUN apt-get update
RUN apt-get install -y curl
RUN rm -rf /var/lib/apt/lists/*
```

Creates three layers, and the apt cache from step 1 is **still in the image** because step 3 only removes it from the writable view, not from layer 1. Versus:

```dockerfile
RUN apt-get update \
 && apt-get install -y curl \
 && rm -rf /var/lib/apt/lists/*
```

One layer, no cache pollution.

### `COPY` and `ADD` — pull files into the image

```dockerfile
COPY package.json package-lock.json ./
COPY src/ ./src/
COPY --chown=node:node . /app
```

`COPY` takes one or more files/dirs from the build context (Module 04) and copies them into the image. That's it. Use `--chown` to set ownership at copy time (avoids a follow-up `chown -R` that doubles your layer size).

`ADD` does the same plus two extra tricks: it auto-extracts local tar archives, and accepts URLs. Both behaviors are footguns:
- Auto-extraction is surprising and rarely what you want.
- URLs don't get checksummed unless you do it manually; you've just made your build non-reproducible.

**Rule:** use `COPY`. Use `ADD` only when you specifically need tar auto-extraction.

### `WORKDIR` — set the working directory

```dockerfile
WORKDIR /app
COPY . .
RUN npm install
```

`WORKDIR` is `cd`. It creates the directory if missing. Use it instead of `RUN cd /foo && ...` because `cd` in a `RUN` doesn't persist to the next layer.

### `ENV` and `ARG` — environment variables

```dockerfile
ARG NODE_VERSION=20            # build-time only
FROM node:${NODE_VERSION}-slim

ENV NODE_ENV=production        # baked into image, available at runtime
ENV PORT=8080
```

- **`ARG`** is build-time. Available inside the Dockerfile and to subsequent `RUN`s in the same stage. Gone at runtime. Override with `--build-arg KEY=value`.
- **`ENV`** is runtime. Baked into the image as a default; overridable with `docker run -e KEY=value`.

Never put secrets in `ENV`. They become part of the image layer forever. We'll cover the right way in Module 13.

### `EXPOSE` — document a port

```dockerfile
EXPOSE 8080
```

This is **purely documentation**. It does *not* publish the port. It tells humans (and tools) "this container listens on 8080." You still need `docker run -p 8080:8080` to actually bind it on the host.

### `USER` — drop privileges

```dockerfile
RUN useradd -r -u 1001 appuser
USER appuser
```

By default, containers run as root *inside* the container. This is a security smell. Create a non-root user and `USER` it before the final `CMD`. Most managed base images (`node`, `nginx-unprivileged`, distroless) already do this for you.

### `VOLUME` — declare a mount point

```dockerfile
VOLUME ["/var/lib/postgresql/data"]
```

Tells Docker "this path should be a volume." If the user doesn't supply one with `-v`, Docker creates an anonymous volume at runtime. Mostly useful for databases. Slightly controversial; many shops omit `VOLUME` and let the user choose.

### `HEALTHCHECK` — tell Docker how to test liveness

```dockerfile
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -fsS http://localhost:8080/health || exit 1
```

Docker runs this periodically and marks the container `healthy`/`unhealthy` accordingly. Compose and orchestrators use this for restart logic. Covered in Module 14.

### `LABEL` — metadata

```dockerfile
LABEL org.opencontainers.image.source="https://github.com/me/repo"
LABEL org.opencontainers.image.licenses="MIT"
```

Pure metadata, queryable via `docker inspect`. Useful for tooling, irrelevant to runtime. The OCI-standard label names are recommended for tooling compatibility.

---

## 3. `CMD` vs `ENTRYPOINT` — the great clarifier

This is the #1 source of Dockerfile confusion. Once you grok it, you can read any Dockerfile.

Both declare what the container does when it starts. The difference is **what the user can override** and how the two combine.

### The shapes

```dockerfile
CMD ["nginx", "-g", "daemon off;"]            # exec form (recommended)
CMD nginx -g "daemon off;"                    # shell form: runs as /bin/sh -c "..."
ENTRYPOINT ["nginx"]                          # exec form (recommended)
ENTRYPOINT nginx                              # shell form (avoid; breaks signal handling)
```

### How they combine

| ENTRYPOINT | CMD | What runs when you `docker run img` |
|---|---|---|
| (none) | `["nginx","-g","daemon off;"]` | `nginx -g "daemon off;"` |
| `["nginx"]` | (none) | `nginx` |
| `["nginx"]` | `["-g","daemon off;"]` | `nginx -g "daemon off;"` |
| `["echo"]` | `["hello"]` | `echo hello` |

The rule: if both are present, the final command is `ENTRYPOINT + CMD` concatenated.

### What `docker run` arguments override

```bash
# Image with: ENTRYPOINT ["nginx"] CMD ["-g","daemon off;"]
docker run myimg                          # → nginx -g "daemon off;"
docker run myimg -t                       # → nginx -t  (CMD replaced by user args)
docker run --entrypoint sh myimg          # → sh        (ENTRYPOINT replaced)
docker run --entrypoint sh myimg -c 'ls'  # → sh -c ls
```

**User CLI args replace `CMD`**, never `ENTRYPOINT`. To replace `ENTRYPOINT`, use `--entrypoint`. This is intentional: `ENTRYPOINT` is meant to be the "essence" of the image (the binary), and `CMD` is the "default arguments" that the user can swap.

### The three usable patterns

**Pattern A: just `CMD`.** Image is a flexible toolbox; user freely overrides.

```dockerfile
FROM alpine:3.20
RUN apk add --no-cache curl
CMD ["sh"]
```

`docker run myimg` gives you a shell. `docker run myimg curl example.com` runs curl. Easy.

**Pattern B: just `ENTRYPOINT` + `CMD` for defaults.** Image is a wrapped binary with sensible defaults.

```dockerfile
FROM nginx:1.27-alpine
ENTRYPOINT ["nginx"]
CMD ["-g", "daemon off;"]
```

`docker run myimg` runs `nginx -g "daemon off;"`. `docker run myimg -V` runs `nginx -V` to print version info. The image is *always* nginx; CMD is just defaults.

**Pattern C: `ENTRYPOINT` script with `CMD` as the "real" command.** Powerful, common in official images.

```dockerfile
FROM python:3.12-slim
COPY entrypoint.sh /
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
CMD ["python", "app.py"]
```

```sh
# entrypoint.sh
#!/bin/sh
set -e
# Do setup: wait for DB, run migrations, etc.
echo "Container starting..."
exec "$@"     # exec the CMD as PID 1 so signals propagate
```

The `exec "$@"` trick is critical: it replaces the shell with the actual command so SIGTERM reaches your app. Without `exec`, you get a zombie shell as PID 1 that swallows signals.

### Shell form pitfalls

```dockerfile
CMD python app.py    # shell form → actually runs: /bin/sh -c "python app.py"
```

This makes `sh` the PID 1. Signals go to `sh`, not Python. Your app won't shut down cleanly. **Always prefer exec form** (`CMD ["python", "app.py"]`) unless you specifically need shell features (variable expansion, pipes).

---

## 4. A real working Dockerfile

Let's build a simple Python web app the right way. App code:

```python
# app.py
from flask import Flask
import os

app = Flask(__name__)

@app.route("/")
def hello():
    return f"Hello from {os.uname().nodename}!\n"

@app.route("/health")
def health():
    return "ok", 200

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=int(os.environ.get("PORT", "8080")))
```

```
# requirements.txt
flask==3.0.3
```

Dockerfile:

```dockerfile
# syntax=docker/dockerfile:1.7
FROM python:3.12-slim

# Metadata
LABEL org.opencontainers.image.source="https://github.com/example/hello-flask"

# Install OS deps if needed (none here, keeping example minimal)

# Create non-root user
RUN useradd -r -u 1001 -m appuser

# Set working directory
WORKDIR /app

# Copy and install deps FIRST (cache-friendly — see Module 05)
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Now copy app code (changes more often than deps)
COPY --chown=appuser:appuser app.py .

# Runtime config
ENV PORT=8080 PYTHONUNBUFFERED=1
EXPOSE 8080

# Drop privileges before running
USER appuser

# Healthcheck
HEALTHCHECK --interval=30s --timeout=3s CMD \
    python -c "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://localhost:8080/health').status==200 else 1)"

# Run the app — exec form so Flask gets SIGTERM directly
CMD ["python", "app.py"]
```

Build, run, hit it:

```bash
$ docker build -t hello-flask .
[+] Building 8.2s (10/10) FINISHED
 => [internal] load build definition from Dockerfile        0.0s
 => [internal] load .dockerignore                           0.0s
 => [internal] load metadata for docker.io/library/python:3.12-slim   0.5s
 => [1/5] FROM docker.io/library/python:3.12-slim@sha256:…  0.0s
 => [internal] load build context                           0.0s
 => CACHED [2/5] RUN useradd -r -u 1001 -m appuser          0.0s
 => CACHED [3/5] WORKDIR /app                               0.0s
 => CACHED [4/5] COPY requirements.txt .                    0.0s
 => CACHED [5/5] RUN pip install --no-cache-dir -r requirements.txt   0.0s
 => [6/5] COPY --chown=appuser:appuser app.py .             0.0s
 => exporting to image                                      0.1s

$ docker run -d --name hello -p 8080:8080 hello-flask
b1c2d3e4...

$ curl localhost:8080
Hello from b1c2d3e4f5a6!

$ docker inspect --format='{{.State.Health.Status}}' hello
healthy

$ docker stop hello && docker rm hello
```

Notice: editing `app.py` and rebuilding skips the slow `pip install` step thanks to layer caching (Module 05). That ordering wasn't accidental.

---

## 5. Common mistakes & gotchas

- **Shell-form `CMD`/`ENTRYPOINT` breaking signal handling.** `CMD python app.py` makes `sh` PID 1 and your app a child; SIGTERM never reaches Python. Use exec form: `CMD ["python", "app.py"]`.
- **Forgetting `exec "$@"` in entrypoint scripts.** Without `exec`, the script (and its parent shell) remain PID 1. Signals get eaten. Always `exec "$@"` at the end of an entrypoint shell script.
- **Putting secrets in `ENV` or `ARG`.** Both end up in image history. `docker history myimg` (or `docker inspect`) reveals them to anyone who can pull the image. Use BuildKit secrets (`--mount=type=secret`, Module 13) at build time, and runtime mounts or secret managers at runtime.
- **`apt-get install` without `--no-install-recommends` or cleanup.** Doubles or triples image size. Always: `apt-get update && apt-get install -y --no-install-recommends ... && rm -rf /var/lib/apt/lists/*`.
- **Multiple `COPY . .` calls.** Every change to *any* tracked file busts every subsequent layer's cache. Copy `requirements.txt`/`package.json` separately first, install, *then* copy the rest (Module 05).
- **`RUN cd /foo && something`.** `cd` doesn't persist between `RUN` instructions. Use `WORKDIR /foo`.
- **`EXPOSE` "doesn't work."** It's documentation. You still need `-p` to publish a port. `EXPOSE` alone publishes nothing.
- **`USER` set, then container fails because the user can't write to a directory.** Create the directory with the right ownership *before* `USER`: `RUN mkdir -p /app/data && chown -R appuser:appuser /app/data`.
- **Using `:latest` in `FROM`.** Your build is now a moving target. Pin: `FROM python:3.12.7-slim`. Better yet, pin by digest for true reproducibility.

---

## 🎯 Key Takeaways

- **Every Dockerfile instruction creates a layer (or layer metadata).** Order them from least to most volatile and combine related shell commands with `&&` to keep images lean.
- **`COPY` over `ADD`** — predictable behavior. Reserve `ADD` for the rare case you need tar auto-extraction.
- **`CMD` is overridable defaults; `ENTRYPOINT` is the binary essence.** Three usable patterns: bare `CMD`, `ENTRYPOINT+CMD` for defaults, `ENTRYPOINT` script that `exec`s `CMD`.
- **Always use exec form** (`["bin", "arg"]`) for `CMD`/`ENTRYPOINT` so PID 1 is your real process and signals propagate correctly.
- **Run as non-root.** Add `USER` before the final `CMD`. Containers running as root are one bad CVE away from being a host-takeover vector (Module 13).

*[prev ← 02_images_and_containers](./02_images_and_containers.md) | [next → 04_building_images](./04_building_images.md)*
