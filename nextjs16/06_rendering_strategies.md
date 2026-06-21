# 06 — Rendering Strategies

> **Goal:** Master the choices between static, dynamic, streamed, and partially prerendered (PPR) rendering routes, and understand the triggers that transition routes between these rendering strategies.

---

## 1. Concept — Static, Dynamic, Streamed, or PPR

In the Next.js 16 App Router, rendering strategies are determined **automatically** per route segment based on the dynamic dependencies read during render. Your task is to design routing layouts intentionally.

| Mode             | When it runs       | What's cached            | Ideal Use Cases                          |
|------------------|--------------------|--------------------------|------------------------------------------|
| **Static**       | Build time         | HTML + RSC payload       | Marketing pages, blog indices, docs      |
| **Dynamic**      | Each request       | Nothing (rendered on demand) | Auth-gated dashboard pages               |
| **Streamed**     | Progressive request | Shell first, components later | Slow third-party API pages                |
| **PPR**          | Build + Request    | Static shell + dynamic holes | E-commerce PDPs, dashboards with cart bubbles |
| **ISR**          | Build + On-demand  | Prerendered HTML (refreshed) | Blogs/Catalogs updated via Webhooks      |

```tsx
// Statically rendered (no dynamic reads, caches output on server)
import { cacheLife } from "next/cache";

export default async function StaticPage() {
  "use cache";
  cacheLife("hours");
  
  const res = await fetch("https://api.example.com/posts");
  const posts = await res.json();
  
  return <ul>{posts.map((p: any) => <li key={p.id}>{p.title}</li>)}</ul>;
}
```

```tsx
// Dynamically rendered (reads cookies per request)
import { cookies } from "next/headers";

export default async function DynamicPage() {
  const c = await cookies();
  const theme = c.get("theme")?.value ?? "light";
  return <p>Theme: {theme}</p>;
}
```

---

## 2. Triggers for Dynamic Rendering

A route segment is **static by default**. It dynamically compiles on every request if it accesses any of these dynamic data sources:

- `cookies()` from `next/headers`
- `headers()` from `next/headers`
- `draftMode()` from `next/headers`
- The `searchParams` Promise on a page component
- Any dynamic resource fetch without a `"use cache"` directive

You can override this automatic behavior by declaring route segment configurations at the top of your `page.tsx` or `layout.tsx`:

```typescript
export const dynamic = "force-dynamic";  // Always render dynamically
export const dynamic = "force-static";   // Force build-time compilation
export const dynamic = "error";          // Build fails if any dynamic APIs are used
```

### Pre-rendering Dynamic Segments: `generateStaticParams`

For paths containing dynamic parameters (e.g., `/blog/[slug]`), you can precompile pages at build time:

```tsx
// app/blog/[slug]/page.tsx
export async function generateStaticParams() {
  const res = await fetch("https://api.example.com/posts");
  const posts = await res.json();
  return posts.slice(0, 100).map((p: { slug: string }) => ({ slug: p.slug }));
}
```

Unmatched parameters are compiled on demand (`dynamicParams = true` by default).

### Streaming with `<Suspense>`

You do not need to block page loads for slow data. Wrap slow components in React `<Suspense>` boundaries to let the static shell load immediately while data streams in:

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
    </main>
  );
}
```

---

## 3. Partial Prerendering (PPR)

PPR is stable in Next.js 16. It generates the static parts of a page (like header layouts and navigation) at build time, while streaming the dynamic elements (like cart bubbles or user settings) when requested.

### 1. Enable PPR in config
Activate PPR incrementally inside your typed `next.config.ts`:

```typescript
// next.config.ts
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  experimental: {
    ppr: "incremental",
  },
};
export default nextConfig;
```

### 2. Opt-in at Route Level
Declare opt-in inside a page component file:

```tsx
// app/products/[id]/page.tsx
export const experimental_ppr = true;

import { Suspense } from "react";
import { UserCartIsland } from "./UserCartIsland"; // reads cookies (dynamic)

export default function ProductPage() {
  return (
    <div>
      <ProductDescription /> {/* Prerendered Static Shell */}
      <Suspense fallback={<CartSkeleton />}>
        <UserCartIsland /> {/* Dynamic hole loaded at request time */}
      </Suspense>
    </div>
  );
}
```

The user receives the precompiled HTML instantly, and the `UserCartIsland` is filled in over the same HTTP connection as soon as the server evaluates the request cookies.

---

## 4. Advanced: Runtimes & Selection

### 4.1 Node.js vs. Edge Runtime

You can choose the server execution environment per route:

```typescript
export const runtime = "edge"; // Runs on global V8 Edge Isolates
// or
export const runtime = "nodejs"; // Default: Runs in standard Node.js serverless containers
```

- **Edge Runtime:** Lightweight APIs, faster cold starts, ideal for global streaming or geolocation logic. No standard `node:fs` access.
- **Node.js Runtime:** Default workspace. Required if you use libraries containing native C/C++ binaries (like Prisma, `bcrypt`, `sharp`).

---

## 5. Practical Application — PDP with PPR and Cart

```tsx
// app/products/[category]/page.tsx
export const experimental_ppr = true;

import { Suspense } from "react";
import { CartBubble } from "@/components/CartBubble";
import { cacheLife, cacheTag } from "next/cache";

async function getProducts(category: string) {
  "use cache";
  cacheLife("hours");
  cacheTag(`category:${category}`);

  const res = await fetch(`https://api.example.com/products?category=${category}`);
  return res.json();
}

export async function generateStaticParams() {
  return [{ category: "shoes" }, { category: "shirts" }];
}

export default async function CategoryPage({
  params,
}: {
  params: Promise<{ category: string }>;
}) {
  const { category } = await params;
  const products = await getProducts(category);

  return (
    <main className="p-4">
      <header className="flex justify-between items-center">
        <h1 className="text-xl font-bold capitalize">{category} Catalog</h1>
        <Suspense fallback={<div className="h-8 w-16 bg-neutral-200 animate-pulse" />}>
          <CartBubble />
        </Suspense>
      </header>

      <ul className="grid grid-cols-2 gap-4 mt-6">
        {products.map((p: any) => (
          <li key={p.id} className="border p-4">
            <h3>{p.name}</h3>
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

export async function CartBubble() {
  const c = await cookies();
  const sessionId = c.get("sid")?.value;

  // Perform dynamic database read for user cart count
  const count = sessionId ? await fetchCartCount(sessionId) : 0;

  return (
    <div className="bg-black text-white px-3 py-1 rounded-full text-sm">
      Cart ({count})
    </div>
  );
}

async function fetchCartCount(sid: string): Promise<number> {
  // Mock fetch
  return 3;
}
```

---

## 6. Common Mistakes & Gotchas

### Unintentional Dynamic Poisoning
Reading a cookie or `searchParams` inside a component high up in the tree (without surrounding it in a `<Suspense>` boundary) makes the **entire page** render dynamically. Always isolate dynamic component reads inside `<Suspense>` tags so that PPR can cache the rest of the layout as static.

### Mutually Exclusive Configs
Setting `dynamic = "force-static"` and simultaneously importing `"use cache"` with a dynamic fetch bypasses compiler logic and throws errors. Ensure configurations are aligned.

### Expecting Streaming over Buffered Proxies
If you test `<Suspense>` chunks locally and they stream perfectly, but in production the whole page loads in one block, check if your hosting platform's proxy or CDN buffers chunked transfer responses. Vercel supports streaming by default.

---

## 🎯 Key Takeaways

- **Static is the default:** Routes are precompiled unless dynamic reads or dynamic fetches force request-time evaluation.
- **Isolate dynamic parts:** Keep layouts static and wrap dynamic reads in `<Suspense>` to compile the page shell at build time.
- **Partial Prerendering (PPR):** Opt-in incrementally for the best combination of static loading performance and dynamic user personalization.

*←* [`05_server_actions_and_mutations.md`](./05_server_actions_and_mutations.md) *|* *next →* [`07_loading_and_error_ui.md`](./07_loading_and_error_ui.md)
