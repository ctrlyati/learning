# 17 — Deployment & Production

> **Goal:** Deploy Next.js 16 applications to Vercel and self-hosted environments (Docker standalone containers), configure runtime environment variables, optimize ISR cache invalidations, and avoid common production deployment pitfalls.

---

## 1. Concept — Deployment Formats

Three deployment layouts cover the majority of Next.js production deployments:

1. **Vercel:** Zero-configuration deployment optimized for Next.js. Automatic edge routing, image optimization caching, and serverless scaling.
2. **Self-hosted Node.js:** Running a persistent process runner (`next start`) on virtual servers (VPS, EC2).
3. **Standalone Docker Containers:** Compiles Next.js into a single container image containing only the minimal standalone files needed.

```bash
# Compile build assets once
pnpm build

# Run the production Node server
pnpm start

# Or trigger standalone server directly
node .next/standalone/server.js
```

Next.js automatically adapts image optimization and static caching handlers based on the target deployment host.

---

## 2. Standalone Container Compilation

To build lightweight Docker container images (typically ~150MB instead of 1GB+), enable standalone compilation within your typed `next.config.ts`:

```typescript
// next.config.ts
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
};
export default nextConfig;
```

This instructs Next.js to trace dependencies and output a compiled server bundle containing only the active assets under `.next/standalone/`.

### 2.1 Multi-Stage Production Dockerfile

```dockerfile
# Dockerfile
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

---

## 3. Production Caching & ISR

In production environments, cache management scales across three layers:

- **Full Route Cache:** Prerendered layout static templates stored on server caches.
- **Data Cache:** Shared JSON records stored by `"use cache"` query blocks.
- **CDN Edge Cache:** Stale-while-revalidate properties served directly from edge locations.

### On-Demand Invalidation

Trigger targeted cash evictions from database webhooks or admin portals by dispatching tag evictions:

```typescript
// src/app/api/revalidate/route.ts
import { updateTag, revalidateTag } from "next/cache";
import { NextResponse } from "next/server";

export async function POST(req: Request) {
  const { searchParams } = new URL(req.url);
  const secret = searchParams.get("secret");

  if (secret !== process.env.REVALIDATE_SECRET) {
    return NextResponse.json({ error: "Invalid signature" }, { status: 401 });
  }

  const { tag, immediate } = await req.json();

  if (immediate) {
    updateTag(tag); // Evicts cache immediately (user mutations)
  } else {
    revalidateTag(tag); // Evicts cache in the background (eventual consistency)
  }

  return NextResponse.json({ revalidated: true, tag });
}
```

---

## 4. Environment Variable Execution

Next.js separates environment variables into two categories:

- **Server-only Variables (Default):** Accessed only during server evaluations (e.g. `DATABASE_URL`, `AUTH_SECRET`).
- **Client-exposed Variables (Prefixed with `NEXT_PUBLIC_`):** Inlined into client javascript bundles during compilation.

> [!WARNING]
> Because `NEXT_PUBLIC_` variables are literally inlined into bundle files at build time, **changing public environment variables requires a complete rebuild of the application** to take effect. Server-only variables can be updated simply by restarting the server process.

---

## 5. Common Mistakes & Gotchas

### Storing Credentials in Public Scopes
Never prefix API keys, database connection URIs, or token encryption keys with the `NEXT_PUBLIC_` prefix. This would expose them to the browser client.

### Read-only Filesystems
Some serverless deployment targets mount filesystems as read-only. In this case, Next.js cannot write cache entries to `.next/cache`. Configure custom redis or S3 cache adapters in `next.config.ts` or deploy to Vercel (which manages caching automatically).

---

## 🎯 Key Takeaways

- **Container Optimization:** Use `output: "standalone"` to compile minimal Node.js server bundles.
- **Environment variables:** Public variables require compilation rebuilds; server-only variables require simple process restarts.
- **Run migrations before deployments:** Execute database schema changes inside CI pipelines prior to rolling out new application code.

*←* [`16_performance_and_observability.md`](./16_performance_and_observability.md) *|* *back to roadmap →* [`00_roadmap.md`](./00_roadmap.md)
