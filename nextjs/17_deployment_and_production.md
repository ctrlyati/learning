# 17 — Deployment & Production

> **Goal:** Ship a Next.js app to Vercel and to a self-hosted environment (Docker, Node, standalone), configure env vars properly, understand ISR/cache behavior in production, and avoid the gotchas that bite teams on their first deploy.

---

## 1. Concept — three deployment shapes

Three deployment targets cover 95% of Next.js apps:

1. **Vercel** — easiest path; Next.js is Vercel's flagship product. Edge, image optimization, ISR, all configured out of the box.
2. **Self-hosted Node** — your own server (EC2, VPS, k8s). Run `next start` against a built app.
3. **Docker / standalone output** — a single container image with everything needed. Use `output: "standalone"` to slim down.

```bash
# Build once
pnpm build

# Run on a Node host
pnpm start

# Or, with standalone output:
node .next/standalone/server.js
```

The framework is **deployment-target-aware**: features like `next/image` will use Vercel's image-optimization endpoint on Vercel, and `sharp`-based optimization elsewhere. Most things "just work"; the rest is in the gotchas.

---

## 2. Mechanism — what `next build` produces

After `pnpm build`:

```
.next/
├── server/                      # server-side renderer + bundles
│   └── app/                     # per-route built RSC code
├── static/                      # client-side static assets (chunks, images)
├── cache/                       # build cache, ISR cache, fetch cache
├── BUILD_ID                     # unique build ID
└── standalone/                  # if output: "standalone"
    └── server.js                # a portable Node server
```

`pnpm start` reads this and serves at port 3000 (configurable). It expects a writable `.next/cache/` directory at runtime — that's where ISR and fetch caches live. On read-only filesystems, you need an external cache (Vercel handles it; self-hosted needs Redis or similar).

### Runtime modes

| File              | Runtime                  | Where it runs                          |
|-------------------|--------------------------|----------------------------------------|
| Static pages      | None                     | CDN edge cache (HTML)                  |
| Dynamic pages     | Node (default) or Edge   | Serverless function or edge function   |
| Middleware        | Edge                     | Every edge PoP                         |
| Route handlers    | Node (default) or Edge   | Serverless function or edge function   |
| Image optimization| Node (Vercel: separate)  | On-demand per image size               |

On Vercel, each maps to a serverless or edge function automatically. Self-hosted, it's all one `next start` process.

---

## 3. Variations / depth

### 3.1 Vercel deployment

```bash
pnpm dlx vercel
```

Connects to git; every push gets a preview deploy, every merge to `main` deploys to production. Environment variables go in the Vercel dashboard (or `vercel env pull` to sync locally to `.env.local`).

Key Vercel features:
- **Preview environments** per branch with their own env scope.
- **Edge runtime** for middleware + opted-in routes.
- **Image optimization** via dedicated workers.
- **Vercel Postgres / KV / Blob** integrated stores.
- **Speed Insights / Web Analytics** with one-line setup.
- **Vercel Cron** for scheduled route handlers.

The cost: you're locked into Vercel for the optimization layer. For most teams, the productivity win is worth it; for cost-sensitive or compliance-heavy deployments, self-host.

### 3.2 Standalone Docker

```js
// next.config.mjs
const nextConfig = {
  output: "standalone",
};
export default nextConfig;
```

This makes `next build` emit `.next/standalone/` containing a minimal Node server + only the necessary `node_modules`. A typical Dockerfile:

```dockerfile
# syntax=docker/dockerfile:1
FROM node:20-alpine AS deps
WORKDIR /app
COPY package.json pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile

FROM node:20-alpine AS builder
WORKDIR /app
COPY . .
COPY --from=deps /app/node_modules ./node_modules
RUN corepack enable && pnpm build

FROM node:20-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
RUN addgroup -S nodejs && adduser -S nextjs -G nodejs
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static
COPY --from=builder --chown=nextjs:nodejs /app/public ./public
USER nextjs
EXPOSE 3000
ENV PORT=3000 HOSTNAME=0.0.0.0
CMD ["node", "server.js"]
```

Image size with this approach: typically 150-200 MB compared to 1+ GB with naive `COPY node_modules`. The standalone server includes only what's actually used.

### 3.3 Self-hosted Node (no Docker)

```bash
# On the server:
git clone <repo>
pnpm install --frozen-lockfile
pnpm build
pnpm start
```

Pair with **PM2** or **systemd** for process management:

```ini
# /etc/systemd/system/myapp.service
[Unit]
Description=Next.js app
After=network.target

[Service]
Type=simple
User=app
WorkingDirectory=/srv/myapp
ExecStart=/usr/bin/node node_modules/.bin/next start -p 3000
Restart=on-failure
Environment=NODE_ENV=production
EnvironmentFile=/etc/myapp.env

[Install]
WantedBy=multi-user.target
```

Front it with nginx or Caddy as a reverse proxy:

```nginx
server {
  server_name app.example.com;
  location / {
    proxy_pass http://127.0.0.1:3000;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection 'upgrade';
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_cache_bypass $http_upgrade;
  }
}
```

### 3.4 Environment variables

Three files, in priority order (highest first):

1. `.env.production.local` (only on production builds; not committed)
2. `.env.local` (any environment except test; not committed)
3. `.env.production` / `.env.development` / `.env.test`
4. `.env`

Variables prefixed with `NEXT_PUBLIC_` are inlined at build time and shipped to the client bundle. Everything else is server-only.

```bash
# .env.production
DATABASE_URL=postgres://...
AUTH_SECRET=super-secret
NEXT_PUBLIC_SITE_URL=https://example.com
```

**Critical**: changing env vars on a host requires a **rebuild** because `NEXT_PUBLIC_*` values are inlined. For server-only env, a restart suffices. On Vercel, dashboard changes trigger a rebuild.

### 3.5 Caching in production

Three caches you care about at runtime:

- **Full Route Cache** (HTML + RSC payload) — written to `.next/cache/` on first request to a static route or after revalidation.
- **Data Cache** (fetch results) — also in `.next/cache/`.
- **CDN cache** — `Cache-Control` headers Next emits.

For self-hosted at multi-instance scale, `.next/cache/` on local disk is per-instance — each pod has its own. Solutions:

- **Custom cache handler**: implement `cacheHandler` in `next.config.mjs` pointing at Redis or S3.

```js
// next.config.mjs
const nextConfig = {
  cacheHandler: require.resolve("./cache-handler.js"),
  cacheMaxMemorySize: 0, // disable in-memory cache to force handler
};
```

Vercel handles this transparently with their Edge Cache + KV.

### 3.6 ISR in production

`revalidate` (time or tag) regenerates pages **in the background** after they expire. The current visitor sees the stale page; the next visitor sees fresh. To force-purge from your dashboard:

```ts
// app/api/revalidate/route.ts
import { revalidateTag } from "next/cache";
import { NextResponse } from "next/server";

export async function POST(req: Request) {
  const url = new URL(req.url);
  if (url.searchParams.get("secret") !== process.env.REVALIDATE_SECRET) {
    return NextResponse.json({ ok: false }, { status: 401 });
  }
  const { tag } = await req.json();
  revalidateTag(tag);
  return NextResponse.json({ revalidated: tag });
}
```

Hit it from your CMS webhook on content publish — see Module 04.

### 3.7 Vercel Cron / scheduled tasks

```json
// vercel.json
{
  "crons": [
    { "path": "/api/cron/nightly-report", "schedule": "0 2 * * *" }
  ]
}
```

The cron POSTs the URL on schedule. Verify the `authorization: Bearer <CRON_SECRET>` header inside the handler.

For self-hosted, use your OS cron + `curl`, or a queue service.

### 3.8 Health checks

```ts
// app/api/health/route.ts
import { db } from "@/lib/db";
import { NextResponse } from "next/server";

export const dynamic = "force-dynamic";

export async function GET() {
  try {
    await db.$queryRaw`SELECT 1`;
    return NextResponse.json({ ok: true });
  } catch (e) {
    return NextResponse.json({ ok: false, error: String(e) }, { status: 503 });
  }
}
```

Wire your load balancer / k8s liveness probe to `/api/health`. Always use `force-dynamic` so it doesn't cache a stale "healthy".

### 3.9 Logs

`console.log` in your route handlers and RSCs goes to stdout — your hosting platform's log aggregator picks it up. For structured logs, use `pino`:

```ts
import pino from "pino";
export const logger = pino({ level: process.env.LOG_LEVEL ?? "info" });
```

```ts
logger.info({ userId, route: "/api/posts" }, "post created");
```

Pipe to your log platform (Datadog, Better Stack, Axiom).

### 3.10 Common cloud configs

- **AWS / Fargate**: deploy the standalone Docker image; put ALB in front.
- **GCP Cloud Run**: deploy the standalone Docker image; needs `--port=3000`.
- **Fly.io**: built-in Next.js support; `fly launch` and `fly deploy`.
- **Render / Railway**: detect Next.js automatically; minimal config.
- **Cloudflare Pages + `@cloudflare/next-on-pages`**: experimental; runs everything on Cloudflare Workers — strict edge runtime.

---

## 4. Practical application — production-ready Dockerfile + CI

```dockerfile
# Dockerfile (multi-stage, standalone, ~150 MB final)
FROM node:20-alpine AS base
RUN corepack enable

FROM base AS deps
WORKDIR /app
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

FROM base AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
# Pass build-time public env via --build-arg in CI; do not bake secrets
ARG NEXT_PUBLIC_SITE_URL
ENV NEXT_PUBLIC_SITE_URL=$NEXT_PUBLIC_SITE_URL
RUN pnpm build

FROM base AS runner
WORKDIR /app
ENV NODE_ENV=production
RUN addgroup -S nodejs && adduser -S nextjs -G nodejs
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static
COPY --from=builder --chown=nextjs:nodejs /app/public ./public
USER nextjs
EXPOSE 3000
ENV PORT=3000 HOSTNAME=0.0.0.0
HEALTHCHECK --interval=30s --timeout=5s CMD wget -qO- http://localhost:3000/api/health || exit 1
CMD ["node", "server.js"]
```

```yaml
# .github/workflows/deploy.yml
name: Deploy
on:
  push:
    branches: [main]
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v3
        with: { version: 9 }
      - uses: actions/setup-node@v4
        with: { node-version: 20, cache: pnpm }
      - run: pnpm install --frozen-lockfile
      - run: pnpm test:run
      - run: pnpm build
        env:
          NEXT_PUBLIC_SITE_URL: ${{ secrets.NEXT_PUBLIC_SITE_URL }}
      # Run DB migrations BEFORE deploying new code that depends on them
      - run: pnpm prisma migrate deploy
        env:
          DATABASE_URL: ${{ secrets.DATABASE_URL }}
      # Build & push image
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with: { registry: ghcr.io, username: ${{ github.actor }}, password: ${{ secrets.GITHUB_TOKEN }} }
      - uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          tags: ghcr.io/${{ github.repository }}:${{ github.sha }}
          build-args: |
            NEXT_PUBLIC_SITE_URL=${{ secrets.NEXT_PUBLIC_SITE_URL }}
      # Update deployment (provider-specific — k8s, Fly, etc.)
      - name: Trigger rolling deploy
        run: curl -X POST ${{ secrets.DEPLOY_HOOK }}
```

That's a complete pipeline: test → build → migrate → containerize → deploy, with proper env separation.

---

## 5. Common mistakes & gotchas

### Changing `NEXT_PUBLIC_*` env in production without a rebuild

The value is inlined at build time. Your "fix" doesn't take effect until you rebuild. Confirm by `grep -r NEXT_PUBLIC_FOO .next/static` after build — you'll see the literal value.

### Leaking secrets via `NEXT_PUBLIC_`

Anything prefixed with `NEXT_PUBLIC_` is in the client bundle. Don't prefix DB URLs, API keys, signing secrets. The framework happily inlines them.

### `.next/cache` on read-only filesystem

On serverless platforms with read-only filesystems, ISR can't write to disk. Configure a custom `cacheHandler` (Redis, S3) or use a hosting platform that handles this (Vercel).

### Image optimization at scale on a tiny VPS

The built-in `sharp` optimizer chews CPU. On a 1-vCPU VPS, a few concurrent image requests can spike the load. Either use a remote loader (Cloudflare, Imgix) or pre-generate critical sizes.

### Migrations after code deploy

Deploy new code that expects schema v2, but v1 is still live → broken queries until migration runs. Always run migrations *before* shipping dependent code, or use backward-compatible migrations (add columns nullable first, deploy code, remove old).

### `next start` behind a non-streaming proxy

If your reverse proxy buffers responses, streaming breaks (the whole page waits). Nginx by default doesn't buffer chunked responses, but check `proxy_buffering off;` if streaming feels broken in prod but works locally.

### Memory leaks from cached clients

If you forget the `globalThis` guard for the Prisma client in dev *and* deploy that code, each Lambda cold start opens new connections. Compounds with high concurrency to exhaust the DB pool. Always use the guard.

### Forgetting `output: "standalone"` in Docker

Without it, your image includes all `node_modules`, ~1 GB. The standalone server is a drop-in replacement and slashes image size.

### Robots accidentally noindex in prod

A leftover `robots: { index: false }` from staging. Test with `curl example.com/robots.txt` after every prod deploy.

### Region mismatch between app and DB

If your app deploys to `iad1` (US East) and your DB is in `fra1` (Frankfurt), every query pays 100ms+ RTT. Pin both to the same region.

### Treating `next dev` perf as production perf

Dev mode does on-demand compilation, no minification, extra source maps. Real perf only shows after `pnpm build && pnpm start`. Always benchmark the production build.

### Forgetting to set `NEXT_TELEMETRY_DISABLED` in CI

The default telemetry isn't sensitive but you may not want it. Set `NEXT_TELEMETRY_DISABLED=1` in your CI environment for cleaner logs.

---

## 🎯 Key Takeaways

- **Vercel for fastest path; self-host for control.** Both are first-class. The framework adapts; you mostly pick based on cost, compliance, and team preference.
- **`output: "standalone"` for production Docker images.** Slimmer images, faster cold starts, smaller attack surface. Multi-stage builds get you to ~150 MB.
- **Env vars: rebuild required for `NEXT_PUBLIC_*` changes.** And never put secrets behind that prefix — they ship to the browser.
- **Migrations before code.** Use backward-compatible schema changes. Roll forward; roll back the code, not the database.
- **Benchmark on `pnpm start`, not `pnpm dev`.** Dev mode is intentionally slower; production is the only number that matters.

You finished the course. From here:

- Build a small project end-to-end (auth, DB, server actions, deployed). Pick something you'd actually use.
- Read the Next.js source. The `packages/next/src/server/app-render/` directory is dense but worth a slow pass.
- Watch the Next.js Conf talks for the version you're using — they explain the *intent* behind features in a way docs can't.

The App Router is, as of 2026, the most ergonomic full-stack React framework in existence. The next thing you build with it will feel light, fast, and (eventually) effortless. That's the payoff for the mental-model work you just did.

*←* [`16_performance_and_observability.md`](./16_performance_and_observability.md) *|* *back to roadmap →* [`00_roadmap.md`](./00_roadmap.md)
