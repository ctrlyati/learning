# 06 — Volumes & Bind Mounts
> **Goal:** Persist data outside the container's ephemeral writable layer, choose between volumes / bind mounts / tmpfs intelligently, and avoid the file-ownership and Mac-performance traps that bite everyone.

---

## 1. Containers are ephemeral; mounts are how you keep things

The container's writable layer dies with `docker rm`. To keep data past container lifetime — databases, uploaded files, logs, dev source code that the container should see live — you mount external storage into the container.

Three flavors of mount, each for a different job:

| Mount type | Stored where | Managed by | Best for |
|---|---|---|---|
| **Volume** | `/var/lib/docker/volumes/...` on host (managed area) | Docker | Production data: databases, queues, persistent state |
| **Bind mount** | An arbitrary host path | You | Dev source code, host config files, host-specific secrets |
| **tmpfs** | RAM, not disk | Kernel | Secrets at runtime, scratch space, sensitive temp data |

Quick syntax:

```bash
# Named volume
docker run -v mydata:/var/lib/postgresql/data postgres:16

# Bind mount (host path on the left, must be absolute)
docker run -v /home/me/code:/app -w /app node:20 npm test

# tmpfs
docker run --tmpfs /tmp:size=64m,mode=1777 myapp

# Modern --mount syntax (more explicit, recommended for new code)
docker run --mount type=volume,source=mydata,target=/var/lib/postgresql/data postgres:16
docker run --mount type=bind,source=/home/me/code,target=/app node:20
docker run --mount type=tmpfs,destination=/tmp,tmpfs-size=64m myapp
```

`--mount` is more explicit and harder to misuse than `-v`. Both are supported; pick one and be consistent (most teams use `-v` for ergonomics, `--mount` when you need options like `readonly` or `bind-propagation`).

---

## 2. Volumes — Docker's managed storage

A named volume is a directory on the host that Docker manages. You refer to it by name; you never deal with the underlying path.

```bash
# Create explicitly (or let docker run auto-create)
docker volume create pgdata

# Use it
docker run -d --name db -v pgdata:/var/lib/postgresql/data \
  -e POSTGRES_PASSWORD=secret postgres:16

# Inspect
docker volume ls
docker volume inspect pgdata
```

```
$ docker volume inspect pgdata
[
    {
        "CreatedAt": "2026-05-13T09:14:32Z",
        "Driver": "local",
        "Mountpoint": "/var/lib/docker/volumes/pgdata/_data",
        "Name": "pgdata",
        "Options": null,
        "Scope": "local"
    }
]
```

The `Mountpoint` is where the bytes actually live. You *can* poke around in it, but you're not supposed to — that's why volumes exist (to abstract that path away).

**Why volumes for stateful workloads** (the right answer most of the time):
- Survive `docker rm` (use `docker volume rm` to actually delete).
- Performance is native on Linux (just a directory in `/var/lib/docker`).
- Portable: backups via `docker run --rm -v pgdata:/data -v $(pwd):/backup alpine tar czf /backup/pg.tgz /data`.
- Permissions are managed by the container (no host UID/GID alignment dance).
- Survive container recreation: stop your DB container, change its image to a new Postgres minor version, recreate — the data is still there.

**Anonymous volumes:** if a Dockerfile has `VOLUME /data` and you don't specify a name, Docker creates a randomly-named anonymous volume. These accumulate quickly and are easy to lose track of:

```bash
docker volume ls -f dangling=true     # find orphaned anonymous volumes
docker volume prune                   # clean them up (careful!)
```

---

## 3. Bind mounts — host paths exposed to the container

A bind mount maps a specific host path into the container. The container sees whatever's at that path; writes inside the container immediately appear on the host.

```bash
docker run --rm -v /home/me/project:/app -w /app python:3.12-slim python app.py
```

This is the **development workflow killer feature**: live-edit code on the host, the container picks it up immediately (with a framework that supports hot reload).

```bash
# Node.js dev with auto-restart
docker run --rm -it \
  -v $(pwd):/app -w /app \
  -p 3000:3000 \
  node:20-slim \
  npx nodemon server.js
```

You edit `server.js` in VS Code on the host; nodemon inside the container sees the file change; the server restarts. Magic.

**Bind mounts have host-specific paths**, so a Compose file with `./src:/app` only works because the relative path resolves on each host. Bind mounts are a development convenience and an occasional production tool (mounting config files); they are *not* portable artifact storage.

**Readonly bind mounts** are useful for injecting config:

```bash
docker run -v /etc/myapp/config.yaml:/etc/app/config.yaml:ro myapp
```

The `:ro` (or `,readonly` with `--mount`) makes the mount read-only inside the container — defense in depth against a compromised app overwriting its own config.

---

## 4. tmpfs — RAM-backed scratch space

`tmpfs` mounts a slice of host RAM as a filesystem inside the container. Contents vanish when the container stops.

```bash
docker run --tmpfs /tmp:size=128m,mode=1777,exec myapp
```

Use cases:
- **Secrets at runtime** — write a token to `/run/secrets`, RAM-only, no disk trace.
- **Scratch directories** — heavy I/O on temp data without thrashing the writable layer.
- **Sensitive data** — credentials decrypted at start, used in memory, never persisted.

tmpfs is **Linux-only** in containers. On Mac/Windows Docker Desktop it works because the daemon runs on Linux.

Options of note:
- `size=128m` — cap the size.
- `mode=1777` — sticky bit for /tmp-style behavior.
- `noexec` — defense in depth: written files can't be executed.

---

## 5. The file-ownership trap (and how to escape it)

The single most painful bind-mount issue is **UID/GID mismatch**.

On Linux, file ownership is by numeric UID/GID. There's no global user database; each system maps numbers to names locally. When you bind-mount your host dir into a container:

```bash
docker run -v $(pwd):/app -u 1000:1000 ubuntu:22.04 touch /app/created.txt
ls -la created.txt
```

You see `created.txt` owned by your host UID 1000. Good. But:

```bash
docker run -v $(pwd):/app ubuntu:22.04 touch /app/root-owned.txt
```

By default the container runs as root (UID 0). `root-owned.txt` is now owned by root *on your host*. You can't delete it without `sudo`. Now multiply this across 50 files a real dev workflow generates.

**Mitigations:**

**Option A: run the container as your host user.**

```bash
docker run -v $(pwd):/app -u $(id -u):$(id -g) -w /app node:20 npm install
```

Works for most stateless dev containers. Falls over when the container expects to be root (e.g., binding port 80, package managers).

**Option B: build the image with a matching UID.**

```dockerfile
ARG UID=1000
ARG GID=1000
RUN groupadd -g ${GID} dev && useradd -u ${UID} -g ${GID} -m dev
USER dev
```

```bash
docker build --build-arg UID=$(id -u) --build-arg GID=$(id -g) -t mydev .
```

**Option C: use named volumes for state, bind mounts only for read-only or per-user scratch.** Volumes don't have this issue because Docker manages them.

**On macOS / Windows Desktop:** the daemon runs in a Linux VM. The bind mount crosses Linux → VM → macOS, with translation. On macOS the file appears owned by your host user *regardless* of the container's view — the kernel does the translation. So you avoid the chown-from-hell issue on Mac, but you get a different problem (next section).

---

## 6. The Mac/Windows bind-mount performance trap

On Docker Desktop for Mac (and to a lesser extent Windows), bind mounts cross a VM boundary using either `gRPC FUSE`, `9p`, or modern `virtiofs`. Each is slow for high-frequency file I/O. Symptoms: `npm install` that takes 10 seconds on Linux takes 90 seconds on a Mac because every file write round-trips through the VM.

**Mitigations:**

1. **`virtiofs` mode** — enable in Docker Desktop settings (Mac, macOS 12.5+). Major speedup over gRPC FUSE.
2. **`:cached` / `:delegated` mount flags** (legacy, Mac-specific): relax consistency for speed. Mostly obsolete with virtiofs.
3. **Put node_modules in a named volume**, even in dev:
   ```yaml
   # docker-compose.yml
   services:
     app:
       image: node:20-slim
       volumes:
         - ./:/app                # source code: bind mount
         - node_modules:/app/node_modules   # this: named volume, never touches host fs
   volumes:
     node_modules:
   ```
   The expensive directory lives in a Docker-managed volume; you still hot-reload your source.
4. **Mutagen / docker-sync** — third-party file sync tools that copy files into a volume instead of bind-mounting. Largely superseded by virtiofs but still useful on older setups.

The TL;DR: if your Mac-based dev container feels glacial, suspect bind-mount I/O before anything else.

---

## 7. A full worked example: Postgres with proper persistence and backup

```yaml
# docker-compose.yml
services:
  db:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: app
      POSTGRES_USER: app
      POSTGRES_PASSWORD_FILE: /run/secrets/db_password
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./init.sql:/docker-entrypoint-initdb.d/init.sql:ro
    secrets:
      - db_password
    ports:
      - "5432:5432"

volumes:
  pgdata:

secrets:
  db_password:
    file: ./secrets/db_password.txt
```

What's happening:
- `pgdata` is a named volume — survives container recreation, lives outside the writable layer.
- `init.sql` bind-mounted read-only — host file, can't be modified by the DB.
- `db_password` injected via Docker secrets (mounted as a tmpfs file at `/run/secrets/db_password`) — not in env, not on disk, not in image history.

**Backup recipe** (one-liner, runs alongside without stopping):

```bash
docker run --rm \
  -v $(docker compose ps -q db | xargs docker inspect -f '{{ range .Mounts }}{{ if eq .Destination "/var/lib/postgresql/data" }}{{ .Name }}{{ end }}{{ end }}'):/data \
  -v $(pwd)/backups:/backup \
  alpine \
  tar czf /backup/pgdata-$(date +%F).tgz -C /data .
```

Simpler in practice: use `pg_dump` inside the running container:

```bash
docker compose exec db pg_dump -U app app > backups/app-$(date +%F).sql
```

For demonstration that the volume actually persists:

```bash
$ docker compose up -d
$ docker compose exec db psql -U app -c "CREATE TABLE t(x int); INSERT INTO t VALUES (42);"
INSERT 0 1
$ docker compose down                    # stops AND removes the container
$ docker compose up -d                   # fresh container, same volume
$ docker compose exec db psql -U app -c "SELECT * FROM t;"
 x
----
 42
(1 row)
```

The container was destroyed and recreated. The data survived because it lived in `pgdata`, not in the container's writable layer.

---

## 8. Common mistakes & gotchas

- **Storing data in the writable layer "for now."** `docker rm` deletes it. Always plan persistence on day one; never "we'll add a volume later."
- **Bind-mount over a directory the image already populated.** If the image has files at `/app` and you `-v $(pwd):/app`, the host dir *shadows* the image content. The container now sees only your host files. Surprising for newcomers. Sometimes you want this (dev); sometimes you don't (you wanted the image's files plus a writable mount somewhere else).
- **Root-owned files appearing on the host.** Container ran as root, wrote to a bind mount, files now have UID 0 on host. Either run the container as your host UID (`-u $(id -u)`) or use a named volume.
- **Anonymous volumes piling up.** `VOLUME` in a Dockerfile + no explicit name = new anonymous volume per container. `docker volume ls -f dangling=true` and `docker volume prune` periodically.
- **Postgres "permission denied" on volume.** The Postgres image expects to be UID 999 on the data dir. Pre-existing data dirs with different ownership fail. Either let Postgres initialize on a fresh volume, or `chown -R 999:999` first.
- **`docker volume rm` failing.** You can't remove a volume in use. `docker ps -a --filter volume=myvol` to find the container holding it; remove that first.
- **Confusing `/data` (in-container) with the host path.** `-v mydata:/data` mounts `mydata` to `/data` inside the container. The host path is `/var/lib/docker/volumes/mydata/_data`, but you should treat that as opaque and use volume tooling instead of poking at the host path.
- **`./relative/path:/data` in `docker run` doesn't expand `.`** the way Compose does. In `docker run`, use absolute paths or `$(pwd)/relative/path`. Compose resolves relative paths against the file's location.
- **Bind-mount performance on Mac.** Massive speed regression versus Linux native. Use virtiofs + named volumes for hot directories like `node_modules`.
- **Forgetting `:ro`** on config bind mounts. A compromised app can rewrite its own config and persist malicious changes. Default mount config files read-only.
- **Volume drivers nobody knows about.** Default is `local`. Cloud-native drivers exist (NFS, AWS EBS via plugins) but are mostly irrelevant once you reach Kubernetes; on single-host Docker, stick with `local`.

---

## 🎯 Key Takeaways

- **Three mount types, three jobs:** **volumes** for managed persistence (databases, state), **bind mounts** for live host files (dev workflow, config), **tmpfs** for RAM-only scratch and runtime secrets.
- **The writable layer is not storage.** Anything important must be on a volume from the first `docker run`. Treat the container filesystem as throwaway.
- **File ownership is by numeric UID/GID.** Bind mounts will create root-owned files on your host if the container runs as root — plan UIDs, or use volumes.
- **Mac/Windows bind mounts are slow** due to the VM boundary. Enable virtiofs and put hot directories like `node_modules` in named volumes, even for dev.
- **Volumes outlive containers — and so does sloppiness.** Audit volumes regularly (`docker volume ls`), prune dangling ones, and back up real data on a schedule. Just because data is in a volume doesn't mean it's safe.

*[prev ← 05_layer_caching_multistage](./05_layer_caching_multistage.md) | [next → 07_networking](./07_networking.md)*
