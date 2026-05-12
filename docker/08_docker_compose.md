# 08 — Docker Compose
> **Goal:** Define and run multi-container applications declaratively. Master services, networks, volumes, profiles, `depends_on`, and healthchecks — and understand why Compose is the right tool for single-host stacks (and the wrong tool for production orchestration).

---

## 1. Compose is "Dockerfile for the whole stack"

`docker run` with twelve flags is a fine way to start one container. For an app with a database, a cache, a backend, a frontend, and a worker — five `docker run` invocations with linked networks and volumes — it falls apart. **Docker Compose** is a YAML file that describes the whole topology and a CLI to bring it up and down.

A bare minimum example:

```yaml
# docker-compose.yml
services:
  web:
    image: nginx:1.27-alpine
    ports:
      - "8080:80"
```

```bash
docker compose up -d
docker compose ps
docker compose logs -f web
docker compose down
```

`docker compose up -d` reads the file, creates a network (`<projectname>_default`), starts every service as a container attached to that network, and sets up port mappings, volumes, and env vars as specified. `down` tears it all back down.

**`docker compose` vs `docker-compose`:** the v1 (`docker-compose`, Python) is end-of-life. v2 (`docker compose`, Go, built into the modern Docker CLI) is what you want. Everything in this module assumes v2.

The Compose file is also the **single source of truth** for "how my app runs locally" — onboard a new developer with `git clone && docker compose up`. That alone justifies adopting Compose for every multi-container project.

---

## 2. The Compose file anatomy

Modern Compose files don't need a `version:` key (the spec is now version-less). Top-level keys:

```yaml
# docker-compose.yml
services:        # the containers
  web:
    ...
  db:
    ...

networks:        # custom networks (optional; a default one is created automatically)
  internal:
  public:

volumes:         # named volumes
  pgdata:

secrets:         # docker secrets
  db_password:
    file: ./secrets/db_password.txt

configs:         # config files (Swarm-flavored, but local works too)
  nginx_conf:
    file: ./nginx.conf
```

A more substantial example combining most everything:

```yaml
services:
  web:
    image: nginx:1.27-alpine
    ports:
      - "8080:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
    depends_on:
      api:
        condition: service_healthy
    networks: [frontend]
    restart: unless-stopped

  api:
    build:
      context: ./api
      dockerfile: Dockerfile
      args:
        NODE_ENV: production
    environment:
      DATABASE_URL: postgres://app:${DB_PASSWORD}@db:5432/app
      REDIS_URL: redis://cache:6379
    depends_on:
      db:
        condition: service_healthy
      cache:
        condition: service_started
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:3000/health"]
      interval: 10s
      timeout: 3s
      retries: 5
      start_period: 10s
    networks: [frontend, internal]
    restart: unless-stopped

  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: app
      POSTGRES_USER: app
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app"]
      interval: 5s
      timeout: 3s
      retries: 5
    networks: [internal]
    restart: unless-stopped

  cache:
    image: redis:7-alpine
    command: ["redis-server", "--maxmemory", "256mb", "--maxmemory-policy", "allkeys-lru"]
    networks: [internal]
    restart: unless-stopped

  seed:
    profiles: ["dev"]
    build: ./seed
    depends_on:
      db:
        condition: service_healthy
    networks: [internal]

networks:
  frontend:
  internal:

volumes:
  pgdata:
```

Read it as a system diagram. `web` is on `frontend` only. `api` is on both `frontend` and `internal` (bridging). `db` and `cache` are on `internal` only — unreachable from `web` directly. `seed` only runs when you opt in (`docker compose --profile dev up`).

---

## 3. `depends_on` and healthchecks — startup ordering done right

`depends_on` alone only orders **container start**, not application readiness. Postgres "started" doesn't mean it's accepting connections. The robust pattern is `depends_on` with `condition: service_healthy`, paired with a `healthcheck`.

```yaml
services:
  db:
    image: postgres:16
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app -d app"]
      interval: 5s
      timeout: 3s
      retries: 5
      start_period: 10s   # grace period before failures count

  api:
    image: myapi
    depends_on:
      db:
        condition: service_healthy
```

Now `api` won't start until `db`'s healthcheck reports healthy. No more "first deploy of the day always fails because the API beat the DB by 2 seconds."

Healthcheck conditions:

| Condition | Meaning |
|---|---|
| `service_started` | The container has started (PID 1 exists). Doesn't say it's ready. |
| `service_healthy` | The container's healthcheck reports healthy. |
| `service_completed_successfully` | The container ran and exited 0. Useful for migration jobs. |

If your image doesn't have its own healthcheck baked in (Module 14), define one in the Compose file. Choose a test that *actually exercises readiness* — `curl localhost/health` is better than `pidof nginx`, because the former proves the application is responding.

`restart: unless-stopped` is the right default for almost everything. `no` for one-shot jobs, `on-failure[:n]` if you only want restarts on crashes (not on clean exits), `always` if you really mean always (rare).

---

## 4. Profiles — services that only run sometimes

Profiles let you keep optional services in the same file without running them by default.

```yaml
services:
  api:
    image: myapi
  db:
    image: postgres:16
  adminer:                # DB GUI, only for dev
    image: adminer
    profiles: ["dev", "debug"]
    ports: ["8081:8080"]
  loadtest:               # k6 stress test, opt-in
    image: grafana/k6
    profiles: ["test"]
```

```bash
docker compose up -d                              # api, db only
docker compose --profile dev up -d                # + adminer
docker compose --profile dev --profile test up -d # + adminer + loadtest
```

Use cases: GUI admin tools you don't want in production, load-test harnesses, mock services, debug sidecars.

---

## 5. Networks in Compose — segmentation for free

By default, Compose creates one network for the project (`<projectname>_default`) and all services join it. For better security, define multiple networks and attach services explicitly:

```yaml
services:
  web:    { networks: [frontend] }
  api:    { networks: [frontend, internal] }
  db:     { networks: [internal] }

networks:
  frontend:
  internal:
    internal: true       # no NAT to the host → no internet from these containers
```

`internal: true` cuts the bridge off from the host's NAT — containers on that network can't reach the internet. Useful for DBs (no need for them to talk outbound) and tightens your blast radius if one is compromised.

You can also fix a subnet to avoid clashes with your corporate VPN:

```yaml
networks:
  internal:
    driver: bridge
    ipam:
      config:
        - subnet: 192.168.250.0/24
```

---

## 6. Environment variables and `.env` files

Compose reads variables from three sources, in increasing priority:

1. **The shell** Compose was invoked from.
2. **A `.env` file** next to `docker-compose.yml` (auto-loaded).
3. **`environment:` / `env_file:` keys** in the service spec.

```bash
# .env (auto-loaded by Compose)
DB_PASSWORD=changeme
IMAGE_TAG=v1.2.3
```

```yaml
services:
  api:
    image: myapi:${IMAGE_TAG}            # interpolated from env / .env
    environment:
      DATABASE_URL: postgres://app:${DB_PASSWORD}@db/app
    env_file:
      - ./api.env                        # file with KEY=VALUE pairs loaded into the container
```

Two distinct things to keep straight:
- **`.env` next to compose file** → variables Compose substitutes into the YAML.
- **`env_file:` inside a service** → file whose contents become **env vars inside the container**.

Add `.env` to `.gitignore`. Commit a `.env.example` with placeholder values.

For secrets, prefer Docker secrets (`secrets:` block) over plain env vars — Module 13 details why.

---

## 7. `build:` — Compose can build images, too

```yaml
services:
  api:
    build:
      context: ./api
      dockerfile: Dockerfile
      args:
        VERSION: "1.0"
      target: runtime              # multi-stage target
      cache_from:
        - ghcr.io/me/api:buildcache
```

`docker compose build` builds; `docker compose up --build` builds then starts. Tag the resulting image so you can push it:

```yaml
services:
  api:
    image: ghcr.io/me/api:${IMAGE_TAG}     # used as the tag for `build` *and* the image for `pull`
    build: ./api
```

When you `compose build`, this tag is applied. When you `compose up` on another machine without source, Compose pulls the image from the registry under that tag. Same file, both worlds.

---

## 8. Lifecycle commands you'll use weekly

```bash
docker compose up                # start all (foreground, logs streamed)
docker compose up -d             # detached
docker compose up -d --build     # rebuild images first
docker compose up -d api db      # just these services + their deps
docker compose down              # stop and remove containers + default network
docker compose down -v           # ...and remove named volumes (DATA LOSS!)
docker compose stop              # stop without removing
docker compose start             # start previously-stopped
docker compose restart api       # restart one service

docker compose ps                # list services
docker compose logs -f --tail 100 api
docker compose exec api sh       # shell into running service
docker compose run --rm api npm test    # one-off ad-hoc command
docker compose pull              # update all images
docker compose config            # render the resolved YAML (great for debugging interpolation)
docker compose top               # processes per service
```

`compose exec` runs in an *already running* container. `compose run` creates a *new* container from the service definition — great for one-shot tasks like running tests or DB migrations.

### Override files — environment-specific overlays

Compose automatically merges `docker-compose.override.yml` on top of `docker-compose.yml` when present. The convention:

- `docker-compose.yml` — base.
- `docker-compose.override.yml` — dev overrides (mounts, debug ports). Auto-loaded, gitignored or committed.
- `docker-compose.prod.yml` — explicit production overrides. Used with `-f`.

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

Example:

```yaml
# docker-compose.override.yml — dev only
services:
  api:
    build: ./api
    volumes:
      - ./api:/app          # live-reload source
    command: ["npm", "run", "dev"]
    ports:
      - "9229:9229"         # debugger port
```

---

## 9. A complete worked example

A small full-stack: React frontend served by nginx, Express API, Postgres, Redis. Project tree:

```
fullstack/
├── docker-compose.yml
├── docker-compose.override.yml
├── .env.example
├── frontend/
│   ├── Dockerfile
│   └── ...
├── api/
│   ├── Dockerfile
│   └── ...
└── db/
    └── init.sql
```

```yaml
# docker-compose.yml
services:
  frontend:
    build: ./frontend
    ports: ["3000:80"]
    networks: [public]
    depends_on:
      api: { condition: service_healthy }
    restart: unless-stopped

  api:
    build: ./api
    environment:
      DATABASE_URL: postgres://app:${DB_PASSWORD}@db:5432/app
      REDIS_URL: redis://cache:6379
      NODE_ENV: production
    networks: [public, internal]
    depends_on:
      db: { condition: service_healthy }
      cache: { condition: service_started }
    healthcheck:
      test: ["CMD", "node", "-e", "fetch('http://localhost:3000/health').then(r=>process.exit(r.ok?0:1))"]
      interval: 10s
      timeout: 3s
      retries: 5
      start_period: 15s
    restart: unless-stopped

  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: app
      POSTGRES_USER: app
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./db/init.sql:/docker-entrypoint-initdb.d/init.sql:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app"]
      interval: 5s
      retries: 5
    networks: [internal]
    restart: unless-stopped

  cache:
    image: redis:7-alpine
    networks: [internal]
    restart: unless-stopped

networks:
  public:
  internal:
    internal: true

volumes:
  pgdata:
```

```bash
$ cp .env.example .env && $EDITOR .env
$ docker compose up -d --build
[+] Running 5/5
 ✔ Network fullstack_public      Created
 ✔ Network fullstack_internal    Created
 ✔ Container fullstack-db-1      Healthy
 ✔ Container fullstack-cache-1   Started
 ✔ Container fullstack-api-1     Healthy
 ✔ Container fullstack-frontend-1 Started

$ docker compose ps
NAME                    SERVICE     STATUS      PORTS
fullstack-api-1         api         healthy
fullstack-cache-1       cache       running
fullstack-db-1          db          healthy
fullstack-frontend-1    frontend    running     0.0.0.0:3000->80/tcp

$ curl localhost:3000
<!DOCTYPE html>...

$ docker compose logs --tail 3 api
fullstack-api-1  | listening on :3000
fullstack-api-1  | db connected
fullstack-api-1  | redis connected
```

Notice the startup order: db came up healthy *first* (api waited), then cache, then api healthy, then frontend. That's `depends_on` + healthchecks doing their job.

---

## 10. Common mistakes & gotchas

- **Using `version: '3'` at the top.** Modern Compose ignores it; older docs mislead you. Just omit it.
- **`depends_on` without `condition: service_healthy`.** Containers start in the right order but your app still races a DB that's "started, not ready." Always pair with a real healthcheck.
- **No healthcheck → `condition: service_healthy` waits forever.** The service has no concept of healthy. Define one.
- **`docker-compose` (v1) vs `docker compose` (v2).** v1 is unmaintained. If `docker-compose --version` prints `1.x`, install v2 or use `docker compose` (space).
- **Forgetting `--build`.** `docker compose up -d` doesn't rebuild by default if the image exists. Edited the Dockerfile? Need `--build` (or `docker compose build` first).
- **Secrets in `environment:`.** They show up in `docker inspect`, `ps aux` views, logs. Use `secrets:` instead.
- **`docker compose down -v` muscle memory.** `-v` removes volumes. Run it on a real database accidentally and your data is gone. Aliasing this away or banning it is reasonable team policy.
- **Override file silently changing prod.** `docker-compose.override.yml` is loaded automatically. If it gets deployed to prod, it overrides production settings. Convention: keep override for dev only, gitignore it or name your prod overlay file `docker-compose.prod.yml` and load it explicitly.
- **Bind mount masking the image's installed dependencies** (`./:/app` mounts your local repo over the image, hiding `node_modules` that the image installed). Solution from Module 06: `node_modules` in a named volume.
- **`docker compose up` and "port already allocated."** Another Compose project (or stray container) holds the port. Tear it down first, or change the published port.
- **Compose project name collisions.** Two repos at `~/work/api/` and `~/personal/api/` both default to project name `api` and step on each other's volumes/networks. Set `COMPOSE_PROJECT_NAME` in `.env`, or use `-p`.
- **Treating Compose as a production orchestrator.** Compose is for one host. No native HA, no rolling updates, no autoscaling. It's perfect for dev and small single-server prod; for fleets you want Kubernetes (Module 16).

---

## 🎯 Key Takeaways

- **Compose makes the topology declarative.** `git clone && docker compose up` is the modern "works on my machine."
- **`depends_on` + healthchecks** is how you order *application readiness*, not just container start. Make this your default for any service-with-dependencies.
- **Networks are for segmentation, not just connectivity.** Put DBs on an `internal: true` network so they can't reach the internet and the frontend can't reach them.
- **Profiles** keep optional services (admin UIs, load tests, seeders) in the same file without running them by default. Cleaner than per-environment file forks.
- **Compose is for one host.** Excellent for dev and modest production; not a replacement for Kubernetes. Know where it stops (Module 16) so you don't try to make it do orchestration.

*[prev ← 07_networking](./07_networking.md) | [next → 09_registries_distribution](./09_registries_distribution.md)*
