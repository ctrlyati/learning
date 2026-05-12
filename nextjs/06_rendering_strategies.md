# 06 — Rendering Strategies

> **Goal:** Choose between static, dynamic, streamed, and partially prerendered rendering per route — and know exactly what flips a route between modes.

---

## 1. Concept — every route is static, dynamic, or streamed

In the App Router, rendering is per route segment and is decided **automatically** based on what the route reads. Your job is to understand what flips a route between modes — then choose intentionally.

| Mode             | When it runs       | What's cached            | Used when                                |
|------------------|--------------------|--------------------------|------------------------------------------|
| **Static**       | Build time         | HTML + RSC payload       | Marketing pages, blog posts, docs        |
| **Dynamic**      | Each request       | Nothing (per-request)    | Auth-gated, request-specific             |
| **Streamed**     | Each request, progressive | Nothing (streamed) | Slow data, mixed fast/slow components    |
| **PPR**          | Build time *and* request | Static shell + dynamic holes | Best-of-both: fast paint + per-user data |
| **ISR**          | Build + revalidation | HTML + RSC payload (refreshed) | Mostly-static content that changes occasionally |

```tsx
// Statically rendered (no dynamic reads, no no-store fetch)
export default async function StaticPage() {
  const posts = await fetch("https://api.example.com/posts", {
    next: { revalidate: 3600 },
  }).then((r) => r.json());
  return <ul>{posts.map((p: any) => <li key={p.id}>{p.title}</li>)}</ul>;
}
```

```tsx
// Dynamically rendered (reads cookies → per-request)
import { cookies } from "next/headers";

export default async function DynamicPage() {
  const c = await cookies();
  const theme = c.get("theme")?.value ?? "light";
  return <p>Theme: {theme}</p>;
}
```

Same component shape, different rendering modes. The framework decided, based on what each one touched.

---

## 2. Mechanism — what flips a route to dynamic

A route is **static by default**. It becomes dynamic the instant it touches a "dynamic API" or an uncached data source. The dynamic APIs are:

- `cookies()` from `next/headers`
- `headers()` from `next/headers`
- `draftMode()` from `next/headers`
- `searchParams` prop on a page component
- `unstable_noStore()` from `next/cache`
- `fetch(..., { cache: "no-store" })` (or `next: { revalidate: 0 }`)

Or you can force the mode explicitly via route segment config:

```tsx
// app/feed/page.tsx
export const dynamic = "force-dynamic";
// or
export const dynamic = "force-static";
// or
export const dynamic = "error"; // throw at build if anything would force dynamic
```

For `force-static`, any dynamic read becomes a build error — useful as a guardrail on routes you want to keep cheap.

### `generateStaticParams` — choosing what to prerender

For dynamic-segment routes (e.g., `/blog/[slug]`), pre-render a known set at build time:

```tsx
// app/blog/[slug]/page.tsx
export async function generateStaticParams() {
  const posts = await fetch("https://api.example.com/posts").then((r) => r.json());
  return posts.slice(0, 100).map((p: { slug: string }) => ({ slug: p.slug }));
}
```

By default, slugs *not* in the returned list are rendered on-demand and cached (`dynamicParams = true`). Set `export const dynamicParams = false` to 404 unknown slugs.

### Streaming with `<Suspense>` and `loading.tsx`

A dynamic route doesn't have to block the entire response. Wrap slow children in `<Suspense>` and the shell streams immediately:

```tsx
// app/dashboard/page.tsx
import { Suspense } from "react";

export default function Dashboard() {
  return (
    <main>
      <h1>Dashboard</h1>
      <Suspense fallback={<StatsSkeleton />}>
        <SlowStats />
      </Suspense>
      <Suspense fallback={<ActivitySkeleton />}>
        <RecentActivity />
      </Suspense>
    </main>
  );
}
```

`loading.tsx` is a shorthand: anything inside `loading.tsx` becomes the fallback for an implicit `<Suspense>` wrapping the whole segment.

### Partial Prerendering (PPR)

PPR is Next 14.x experimental / 15 stable (depending on minor) and is the answer to: *"I want the static shell of a per-user page to ship instantly, with the user-specific bits streamed in."* You opt in per app or per route:

```js
// next.config.mjs
const nextConfig = {
  experimental: { ppr: "incremental" },
};
export default nextConfig;
```

```tsx
// app/products/[id]/page.tsx
export const experimental_ppr = true;

import { Suspense } from "react";
import { Cart } from "./Cart"; // reads cookies → dynamic

export default function ProductPage() {
  return (
    <div>
      <ProductDetails />               {/* static — prerendered */}
      <Suspense fallback={<CartSkeleton />}>
        <Cart />                       {/* dynamic — streamed at request time */}
      </Suspense>
    </div>
  );
}
```

The static shell ships from CDN immediately; the `<Cart>` hole is filled in via a streamed RSC payload. You get TTFB of a static site with the per-user data of a dynamic one.

---

## 3. Variations / depth

### 3.1 ISR — Incremental Static Regeneration

ISR is "static, but periodically refreshed." It's just time- or tag-based revalidation on a statically-rendered page:

```tsx
export const revalidate = 60; // re-render at most every 60s
```

Or, more commonly, set it on the fetch:

```tsx
const posts = await fetch(url, { next: { revalidate: 60 } });
```

The first request after 60s triggers a background rebuild; visitors continue to see the stale page until the rebuild completes (stale-while-revalidate). Tag-based revalidation (from Module 04) is the targeted version.

### 3.2 Route segment config reference

Place at the top of `page.tsx`, `layout.tsx`, or `route.ts`:

```ts
export const dynamic = "auto" | "force-dynamic" | "error" | "force-static";
export const dynamicParams = true | false;       // for [param] routes
export const revalidate = false | 0 | number;    // seconds, or 0 for dynamic
export const fetchCache = "auto" | "default-cache" | "only-cache" | "force-cache" | "default-no-store" | "only-no-store" | "force-no-store";
export const runtime = "nodejs" | "edge";
export const preferredRegion = "auto" | "global" | "home" | string[];
export const maxDuration = number;               // serverless timeout
```

You will most often use `dynamic`, `revalidate`, and `runtime`. The rest are advanced knobs.

### 3.3 Edge runtime vs Node runtime

The **edge runtime** (`runtime = "edge"`) runs your code on Vercel's edge network (or your provider's equivalent) — close to the user, V8 isolates rather than Node processes. It has a much smaller API surface: no `node:fs`, no Node-only crypto, limited memory, faster cold start. The **Node runtime** (default) gives you the full Node API at the cost of slower cold starts.

Use edge for:
- Lightweight, latency-critical handlers (auth checks, geo routing, A/B tests),
- Streaming AI responses,
- Middleware (always edge).

Use Node for:
- Anything touching a Node-only library (Prisma, sharp, bcrypt with binaries),
- Long-running tasks,
- Most of your routes.

### 3.4 Picking a strategy

Decision tree:

```
Is the data per-user / per-request?
├── Yes
│   ├── Is the rest of the page identical for everyone?
│   │   ├── Yes  → PPR (static shell, dynamic hole)
│   │   └── No   → Dynamic + Suspense streaming
└── No
    ├── Does the data change often (< few minutes)?
    │   ├── Yes  → ISR with short revalidate
    │   └── No   → Pure static (revalidate via tag on publish)
```

Most of a real app is static + tag-revalidated; auth-aware pages use PPR or are fully dynamic; dashboards stream. There is no "one right answer" — there are seven answers, and each route picks the cheapest one that works.

### 3.5 Checking what `next build` produced

After `pnpm build`, Next prints a per-route legend:

```
Route (app)                          Size     First Load JS
┌ ○ /                                ...      ...
├ ○ /about                           ...      ...
├ λ /api/health                      ...      ...
├ ● /blog/[slug]                     ...      ...   (prerendered, 25 paths)
└ ƒ /dashboard                       ...      ...

○  Static            (no data, no dynamic APIs)
●  SSG               (data, statically generated)
ƒ  Dynamic           (server-rendered on demand)
λ  Route handler
```

Read this output religiously. A `ƒ` next to a page you thought was static means *something* in that route's tree touched a dynamic API — find it.

---

## 4. Practical application — an e-commerce category page with PPR

The goal: a category page that is statically prerendered for SEO and instant TTFB, with a personalized cart bubble that streams in per request.

```tsx
// app/products/[category]/page.tsx
export const experimental_ppr = true;

import { Suspense } from "react";
import { CartBubble } from "@/components/CartBubble";

async function getProducts(category: string) {
  const res = await fetch(`https://api.example.com/products?category=${category}`, {
    next: { tags: [`category:${category}`], revalidate: 3600 },
  });
  return res.json() as Promise<{ id: string; name: string; priceCents: number }[]>;
}

export async function generateStaticParams() {
  const categories = ["shoes", "shirts", "hats"];
  return categories.map((c) => ({ category: c }));
}

export default async function CategoryPage({
  params,
}: {
  params: Promise<{ category: string }>;
}) {
  const { category } = await params;
  const products = await getProducts(category);

  return (
    <main className="mx-auto max-w-5xl p-4">
      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-bold capitalize">{category}</h1>
        <Suspense fallback={<div className="h-8 w-16 animate-pulse rounded bg-neutral-200" />}>
          <CartBubble />
        </Suspense>
      </header>

      <ul className="mt-6 grid grid-cols-3 gap-4">
        {products.map((p) => (
          <li key={p.id} className="rounded border p-3">
            <h2 className="font-semibold">{p.name}</h2>
            <p>${(p.priceCents / 100).toFixed(2)}</p>
          </li>
        ))}
      </ul>
    </main>
  );
}
```

```tsx
// components/CartBubble.tsx
import { cookies } from "next/headers";

async function getCartCount(sessionId: string | undefined): Promise<number> {
  if (!sessionId) return 0;
  // pretend we look this up
  await new Promise((r) => setTimeout(r, 200));
  return 3;
}

export async function CartBubble() {
  const c = await cookies();
  const sid = c.get("sid")?.value;
  const count = await getCartCount(sid);
  return (
    <div className="rounded-full bg-black px-3 py-1 text-sm text-white">
      Cart ({count})
    </div>
  );
}
```

Behavior:

- `next build` prerenders `/products/shoes`, `/products/shirts`, `/products/hats` at build time — products list HTML is baked in.
- On every request, `CartBubble` runs (it reads cookies), suspends at first, and streams in with the user's count.
- TTFB is CDN-fast; the cart bubble pops in ~200ms later.

Compare this to a fully dynamic version (TTFB = data fetch latency) or a fully static version (no per-user cart). PPR is the modern win.

---

## 5. Common mistakes & gotchas

### A single `cookies()` call somewhere deep made the whole page dynamic

Cookies/headers/searchParams are infectious upward — any read forces the route dynamic. To preserve the static shell, isolate the dynamic read behind a `<Suspense>` boundary so PPR can keep the rest prerendered. Or move that piece into a client component fetched from a route handler.

### `force-static` on a page that fetches with `no-store`

If you set `dynamic = "force-static"` but one of your fetches is `no-store`, the build fails with a clear error. That's by design — pick one position.

### Confusing `revalidate` and `cache`

`revalidate: 60` says "this is cached, refresh in the background after 60s". `cache: "no-store"` says "never cache; always fetch fresh". They are mutually exclusive on the same fetch.

### `generateStaticParams` with too many entries

If you return 10,000 slugs, your build will take ten minutes. Use `dynamicParams = true` (default) and return only the high-traffic slugs; on-demand requests for the rest get cached after first hit. Watch your `next build` time.

### Forgetting PPR is feature-flagged

PPR requires `experimental.ppr` in `next.config.mjs` AND, depending on version, `export const experimental_ppr = true` per route. If your "PPR" route is fully dynamic, check both.

### Edge runtime imports a Node-only module

You set `runtime = "edge"`, then import Prisma. Build error or runtime crash. Edge has no `node:` APIs. For DB access at the edge, use a Postgres driver designed for it (e.g., `@neondatabase/serverless`, `@vercel/postgres`).

### Streaming breaks behind a non-streaming proxy

Some corporate proxies and naive CDN configs buffer responses. If `<Suspense>` falls back to blocking the whole response in production but not locally, your hosting provider's chunked-transfer support may be off. Vercel does this correctly out of the box.

### `dynamic = "force-dynamic"` on a marketing page

A common reflex: "I added an `await fetch` so I'd better mark it dynamic." Almost never necessary. Tag your fetch, set a revalidation interval, and let it stay static.

---

## 🎯 Key Takeaways

- **Static is the default**. Routes become dynamic only when they touch a dynamic API or a `no-store` fetch. Read `next build` output to confirm what you got.
- **Streaming with `<Suspense>` decouples the page shell from slow data**. A dynamic page can still ship a fast first paint.
- **Partial Prerendering is the future default.** Static shell + dynamic islands gives you the SEO/TTFB of a static site with the personalization of dynamic.
- **`revalidate` (ISR) is the middle ground** for content that changes occasionally — pair with tag revalidation for instant invalidation on publish.
- **Choose runtime per route**. Edge for low-latency stateless work; Node for everything that touches a Node-only library. Don't over-edge.

*←* [`05_server_actions_and_mutations.md`](./05_server_actions_and_mutations.md) *|* *next →* [`07_loading_and_error_ui.md`](./07_loading_and_error_ui.md)
