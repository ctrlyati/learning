# 02 — Images & Containers
> **Goal:** Become fluent with `pull / run / exec / stop / rm`, understand image layers as a content-addressable filesystem, and know exactly what state a container can be in at any moment.

---

## 1. Images vs containers — the distinction that pays for itself daily

An **image** is a read-only template — a stack of filesystem layers plus a bit of metadata (which command to run, env vars, exposed ports, etc.). A **container** is a *running* (or stopped) instance of an image: image + a thin writable layer on top + a process tree + namespaces + cgroups.

Analogy: image is to container as class is to object, or as `.exe` is to running process.

```bash
docker pull nginx:1.27-alpine     # download image (template)
docker run -d --name web nginx:1.27-alpine   # create + start container (instance)
docker run -d --name web2 nginx:1.27-alpine  # another container, same image
docker images                     # list images
docker ps                         # list running containers
```

Sample output:

```
$ docker images
REPOSITORY  TAG         IMAGE ID       CREATED      SIZE
nginx       1.27-alpine 8a4b2c3d4e5f   2 weeks ago  48.3MB

$ docker ps
CONTAINER ID   IMAGE                COMMAND                  STATUS         NAMES
a1b2c3d4e5f6   nginx:1.27-alpine    "/docker-entrypoint.…"   Up 5 seconds   web2
f6e5d4c3b2a1   nginx:1.27-alpine    "/docker-entrypoint.…"   Up 20 seconds  web
```

Two containers, same image. The image is downloaded once and shared.

---

## 2. Image layers — how Docker actually stores images

An image is **not** a single tarball. It's a stack of **layers**, each a tarball of filesystem diffs, identified by a SHA-256 digest. When you `pull` an image, Docker downloads the layers (skipping any it already has), then *stacks them with an overlay filesystem* (overlay2 on modern Linux) to present a unified view.

```bash
docker pull nginx:1.27-alpine
```

Sample output:

```
1.27-alpine: Pulling from library/nginx
8e87ff28f1b5: Pull complete    # base alpine layer
d3eba9a3da80: Pull complete    # nginx binary layer
e4d80766b0c1: Pull complete    # config files
2dffa6df6862: Pull complete    # entrypoint script
1ad1f29e4d5e: Pull complete    # docker-entrypoint.d
8d3a0a3322d2: Pull complete    # default.conf
ad5b48f9c9b9: Pull complete    # CMD wrapper
Digest: sha256:a45ee5d042aaa9e81e013f97ae40c3dda26fbe98f22b6251acdf28e579560d55
Status: Downloaded newer image for nginx:1.27-alpine
```

Each `Pull complete` line is one layer. Inspect them:

```bash
docker history nginx:1.27-alpine
```

```
IMAGE          CREATED        CREATED BY                                      SIZE
8a4b2c3d4e5f   2 weeks ago    CMD ["nginx" "-g" "daemon off;"]                0B
<missing>      2 weeks ago    STOPSIGNAL SIGQUIT                              0B
<missing>      2 weeks ago    EXPOSE map[80/tcp:{}]                           0B
<missing>      2 weeks ago    ENTRYPOINT ["/docker-entrypoint.sh"]            0B
<missing>      2 weeks ago    COPY 30-tune-worker-processes.sh /docker-en…    4.62kB
<missing>      2 weeks ago    RUN /bin/sh -c set -x ...                       43.5MB
<missing>      2 weeks ago    ENV NGINX_VERSION=1.27.3                        0B
<missing>      4 weeks ago    /bin/sh -c #(nop)  CMD ["/bin/sh"]              0B
<missing>      4 weeks ago    /bin/sh -c #(nop) ADD file:abc123... in /       7.62MB
```

Every line is the result of one Dockerfile instruction. Zero-byte lines are metadata-only (no filesystem change). The 43.5MB line is where `apk add nginx` happened.

**Why layers matter:**
1. **Sharing** — if 50 images all derive from `alpine:3.20`, the 7.6 MB alpine layer is stored once.
2. **Caching** — rebuilds reuse unchanged layers. Order your Dockerfile instructions from least-to-most-likely-to-change (Module 05).
3. **Transfer efficiency** — `docker pull` only downloads layers you don't already have.
4. **Content-addressable** — layers are referenced by SHA-256 of their content. Same content → same hash → automatic deduplication.

The thin writable layer added when you `docker run` is **copy-on-write**: reading sees through to the lower layers; writing copies the file up to the writable layer first. Delete that container → the writable layer is gone. This is why containers are ephemeral by default and you need **volumes** for persistence (Module 06).

---

## 3. The container lifecycle

A container is a state machine. Knowing the states saves you from "why isn't my container running?" confusion.

```
                     docker run
                          │
                          ▼
   ┌──────────┐  start  ┌──────────┐  pause   ┌──────────┐
   │ created  │────────▶│ running  │─────────▶│  paused  │
   └──────────┘         └────┬─────┘          └────┬─────┘
                             │                     │
                       stop  │                     │ unpause
                             ▼                     │
                        ┌──────────┐               │
                        │ exited   │◀──────────────┘
                        └────┬─────┘
                             │ rm
                             ▼
                          (gone)
```

| State | How you got here | Container exists? | Process running? |
|-------|------------------|-------------------|------------------|
| `created` | `docker create` | yes | no |
| `running` | `docker run` or `start` | yes | yes |
| `paused` | `docker pause` (SIGSTOP) | yes | yes (frozen) |
| `exited` | process exited or `docker stop` | yes | no |
| `dead` | daemon couldn't clean up | yes (broken) | no |
| removed | `docker rm` | no | no |

**See it:**

```bash
docker run -d --name demo alpine sleep 30
docker ps              # demo is running
sleep 35
docker ps              # nothing shown (only running by default)
docker ps -a           # shows demo as Exited (0)
docker rm demo         # now gone
docker ps -a           # demo no longer listed
```

A common surprise: `docker ps` only shows **running** containers. `docker ps -a` shows all of them, including stopped ones eating disk space. `docker container prune` cleans the latter.

---

## 4. The daily-driver commands

Here's a complete workflow you'll repeat a thousand times. Read every command and what it does.

```bash
# --- Get an image ---
docker pull python:3.12-slim
docker pull python:3.12-slim@sha256:0a1b2c...   # pin by digest (immutable!)

# --- Run a container ---
docker run python:3.12-slim python -c "print('hi')"
# Foreground, ephemeral. Exits immediately because the process exits.

docker run --rm python:3.12-slim python -c "print('hi')"
# Same, but --rm removes the container after exit (no garbage).

docker run -it --rm python:3.12-slim          # interactive REPL
# -i: keep STDIN open. -t: allocate a TTY. Together: an interactive shell experience.

docker run -d --name api -p 8000:8000 myapi:latest
# -d: detached (background). --name: give it a stable name. -p: publish port.

# --- Look at what's running ---
docker ps                                  # running containers
docker ps -a                               # all containers, including exited
docker ps -a --filter "status=exited"      # just exited ones
docker ps --format 'table {{.Names}}\t{{.Status}}'   # custom output

# --- Get inside / interact ---
docker logs api                            # all logs
docker logs -f --tail 50 api               # follow live, last 50 lines
docker exec -it api bash                   # shell into running container
docker exec api env                        # one-off command, see env vars

# --- Stop / restart / kill ---
docker stop api          # SIGTERM, then SIGKILL after grace period (10s default)
docker stop -t 30 api    # 30-second grace period
docker kill api          # immediate SIGKILL (use sparingly)
docker restart api
docker pause api && docker unpause api     # freeze/thaw

# --- Clean up ---
docker rm api                # remove a stopped container
docker rm -f api             # force-remove a running container (stop then rm)
docker container prune       # remove all stopped containers
docker image prune           # remove dangling images (untagged)
docker image prune -a        # remove all unused images (careful!)
docker system prune          # everything unused (containers, images, networks, build cache)
docker system prune -a --volumes   # nuclear option. THINK before running.

# --- Inspect ---
docker inspect api           # full JSON state (long!)
docker inspect --format '{{.NetworkSettings.IPAddress}}' api
docker stats                 # live CPU/mem/IO per container
docker top api               # processes inside the container (host's view)
docker diff api              # what's changed in the writable layer

# --- Copy files in/out ---
docker cp ./local.txt api:/tmp/local.txt
docker cp api:/var/log/app.log ./app.log
```

A worked example to glue these together. Pretend you're debugging a misbehaving web app:

```bash
$ docker run -d --name web -p 8080:80 nginx:1.27-alpine
a1b2c3d4...

$ curl -s localhost:8080 | head -1
<!DOCTYPE html>

$ docker logs --tail 5 web
192.168.65.1 - - [13/May/2026:09:14:22 +0000] "GET / HTTP/1.1" 200 615 ...

$ docker exec -it web sh
/ # cat /etc/nginx/nginx.conf | head -3
user  nginx;
worker_processes  auto;
error_log  /var/log/nginx/error.log notice;
/ # exit

$ docker stop web && docker rm web
web
web
```

That's the loop: run, observe, exec to dig in, stop, remove. Burn it into your muscle memory.

---

## 5. Common mistakes & gotchas

- **Forgetting `--rm` during exploration.** After a week of "let me just try this image," `docker ps -a` shows 300 stopped containers eating gigabytes. Use `--rm` for throwaway runs; reserve named containers for things you'll come back to.
- **`docker run -it ... bash` on an image without bash.** Alpine images have `sh`, not `bash`. Distroless images (Module 10) have *no shell at all*. `docker exec -it foo bash` returns `executable file not found` — try `sh`, or accept you can't shell in.
- **Treating containers as durable storage.** `docker rm` deletes the writable layer. Everything you wrote inside the container is gone. Want persistence? Mount a volume (Module 06).
- **Pulling `:latest` and being surprised when it changes.** `:latest` is a tag, not a guarantee. Tomorrow's `:latest` is a different image. Pin tags (`python:3.12.7-slim`) or, better, digests (`@sha256:...`) for reproducible builds.
- **`docker stop` "doing nothing" for 10 seconds.** Your container ignored SIGTERM and got SIGKILLed after the grace period. Make your apps handle SIGTERM gracefully (close DB connections, flush logs).
- **Exec'ing in to make changes "permanent."** Whatever you do inside a running container vanishes on `rm`. To change an image, change the Dockerfile and rebuild. `docker commit` exists but is a smell — use it only for forensic snapshots.
- **`-p 8080:80` and "why can't I reach it from another host?"** By default, `-p` binds to all interfaces — but if the daemon's `--ip` is `127.0.0.1` (some Desktop configs) or there's a firewall, you're stuck. Test with `curl localhost:8080` first; if that works the container is fine and the issue is host networking.
- **Confusing `docker stop` and `docker kill`.** Stop is polite (SIGTERM, then SIGKILL). Kill is immediate (SIGKILL by default, or `-s SIGNAL` for others). Use `kill` for stuck containers, `stop` for normal shutdown.

---

## 🎯 Key Takeaways

- **Image = template (read-only layers). Container = running instance (image + thin writable layer + process).** This single distinction unlocks half of Docker.
- **Layers are content-addressable and shared** — same SHA-256 means same content, deduplicated across images. This is why pulls and builds are fast.
- **Containers are ephemeral.** The writable layer dies with `docker rm`. Plan persistence via volumes from day one; don't retrofit it.
- **Master the lifecycle states** (`created → running → exited → removed`) and the eight commands that move between them. They're your debugging vocabulary.
- **Pin your tags, preferably by digest** for production. `:latest` is fine for `docker run hello-world`; it's a footgun for anything you deploy.

*[prev ← 01_intro_and_setup](./01_intro_and_setup.md) | [next → 03_dockerfile_fundamentals](./03_dockerfile_fundamentals.md)*
