# 04 — Data Fetching and Caching

> **Goal:** Master the Next.js 16 data fetching patterns, the `"use cache"` directive, cache life profiles, granular cache tagging, and the mechanics of tag-based revalidation.

---

## 1. Concept — `await` data inside a component

The core feature of Next.js App Router: you can `await` data fetching directly in a server component. You don't need `getStaticProps`, special lifecycle hooks, or client-side `useEffect` fetch waterfalls.

```tsx
// app/products/page.tsx
type Product = { id: string; name: string };

async function getProducts(): Promise<Product[]> {
  const res = await fetch("https://api.example.com/products");
  if (!res.ok) throw new Error("Failed to load products");
  return res.json();
}

export default async function ProductsPage() {
  const products = await getProducts();
  return (
    <ul>
      {products.map((p) => <li key={p.id}>{p.name}</li>)}
    </ul>
  );
}
```

The function is `async`, the page renders on the server, and the resulting HTML streams to the browser. 

For non-`fetch` data sources (Prisma, Drizzle, raw SQL, S3, filesystem, etc.), you write the same pattern — simply `await` whatever database or API client returns a Promise.

---

## 2. Mechanism — Next.js 16 Caching Layers

Next.js 16 provides three distinct caching layers:

### 2.1 Request Memoization (per request, automatic)

During a single request's render pass, identical `fetch(url, options)` calls are automatically deduplicated. If two nested server components both request user data from the same endpoint, Next.js calls the network only once. This is an in-memory optimization scoped strictly to a single render lifecycle.

### 2.2 The `"use cache"` Directive (persistent cache)

In Next.js 16, **standard `fetch` calls are dynamic and do not cache by default**. Instead, caching is component- and function-level using the **`"use cache"`** directive.

When you add `"use cache"` to the top of an asynchronous function, a component, or a file, Next.js caches the returned value of that function or component. This works for `fetch` requests, database queries, and expensive computations alike.

```tsx
import { cacheLife, cacheTag } from "next/cache";

// Cache this function's output
async function getCachedProducts() {
  "use cache";
  
  // Set cache lifetime using a profile (e.g., 'minutes', 'hours', 'days')
  cacheLife("minutes");
  
  // Set a cache tag for on-demand invalidation
  cacheTag("products");

  const res = await fetch("https://api.example.com/products");
  return res.json();
}
```

### 2.3 The Full Route Cache

For routes that are statically prerendered at build time or cache-rendered on-demand, Next.js caches the final rendered HTML and RSC payload. When a user requests that page, Next.js serves the cached HTML immediately without running React on the server.

---

## 3. Caching Control: Life, Tags, and Revalidation

### 3.1 Cache Life Profiles

The `cacheLife` function sets the expiration behavior of your cached block. You pass it a string identifier matching a profile (defined in `next.config.ts` or using standard defaults):

* `cacheLife("seconds")` — Short-lived caching.
* `cacheLife("minutes")` — Medium-lived caching.
* `cacheLife("days")` — Long-lived caching.

You can customize these profiles or define new ones in your `next.config.ts`:

```typescript
// next.config.ts
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  experimental: {
    cacheHandlers: {
      // Custom cache lifetime values
    }
  }
};
export default nextConfig;
```

### 3.2 Cache Tags and Revalidation (`updateTag` vs `revalidateTag`)

To clear cached data on-demand, associate it with a tag via `cacheTag("tag-name")`. When a mutation happens, you can invalidate the tag using one of two APIs from `next/cache`:

#### 1. `updateTag(tag)` — Immediate Consistency
Use `updateTag` inside **Server Actions** when you need **immediate consistency** (e.g., user deletes an item and expects to see it gone on the next redirect/render). It immediately expires the cached entries containing that tag.

```typescript
"use server";
import { updateTag } from "next/cache";

export async function deleteProduct(id: string) {
  await db.product.delete({ where: { id } });
  updateTag("products"); // Next render is guaranteed to fetch fresh data
}
```

#### 2. `revalidateTag(tag)` — Eventual Consistency
Use `revalidateTag` when eventual consistency is acceptable (e.g., updates triggered by CMS webhooks or background cron jobs). It marks the cache as stale, serving the old data to the next user while fetching fresh data in the background (stale-while-revalidate).

```typescript
// app/api/revalidate/route.ts
import { revalidateTag } from "next/cache";

export async function POST() {
  revalidateTag("products"); // Background revalidation
  return Response.json({ revalidated: true });
}
```

### 3.3 Path-based Revalidation

You can also invalidate entire routing paths using `revalidatePath`:

```typescript
import { revalidatePath } from "next/cache";

revalidatePath("/products");                  // Revalidates specific route layout/page
revalidatePath("/products/[id]", "page");     // Revalidates all dynamic product pages
```

---

## 4. Practical Application — Cached DB Queries & CMS Integration

We want our product catalog to be statically cached, fast, and instantly updated when a CMS webhook runs or when a database edit is submitted.

```typescript
// lib/products.ts
import "server-only";
import { db } from "@/lib/db";
import { cacheLife, cacheTag } from "next/cache";

export async function getProducts() {
  "use cache";
  cacheLife("days");
  cacheTag("products");

  return db.product.findMany({
    orderBy: { createdAt: "desc" }
  });
}

export async function getProduct(id: string) {
  "use cache";
  cacheLife("days");
  cacheTag(`product:${id}`, "products");

  return db.product.findUnique({ where: { id } });
}
```

By adding `"use cache"` inside these functions, Next.js caches the database query results directly.

The page rendering these functions:

```tsx
// app/products/page.tsx
import Link from "next/link";
import { getProducts } from "@/lib/products";

export default async function ProductsPage() {
  const products = await getProducts();

  return (
    <main>
      <h1>Product Catalog</h1>
      <ul className="grid gap-4 mt-4">
        {products.map((product) => (
          <li key={product.id}>
            <Link href={`/products/${product.id}`} className="block border p-4 hover:bg-neutral-100">
              <h2 className="font-bold">{product.name}</h2>
              <p className="text-sm text-neutral-600">${(product.priceCents / 100).toFixed(2)}</p>
            </Link>
          </li>
        ))}
      </ul>
    </main>
  );
}
```

When an admin updates a product, we trigger an immediate update in our Server Action:

```typescript
// app/actions/products.ts
"use server";
import { db } from "@/lib/db";
import { updateTag } from "next/cache";
import { redirect } from "next/navigation";

export async function editProduct(id: string, formData: FormData) {
  const name = formData.get("name") as string;
  const priceCents = Number(formData.get("priceCents"));

  await db.product.update({
    where: { id },
    data: { name, priceCents },
  });

  // Force immediate update of the catalog and the individual product page
  updateTag(`product:${id}`);
  updateTag("products");

  redirect(`/products/${id}`);
}
```

---

## 5. Common Mistakes & Gotchas

### Placing `"use cache"` in Client Components
`"use cache"` is a server-side directive. Placing it in a file marked with `"use client"` or inside a client component function will cause a compilation error.

### Confusing `updateTag` vs `revalidateTag`
- Use `updateTag()` when the invalidation is triggered by a **user action** (Read-Your-Own-Writes consistency is critical).
- Use `revalidateTag()` when the invalidation is triggered by **background tasks** or webhooks (eventual consistency).

### Data Leaks via Cache sharing
Since cache entries created by `"use cache"` are shared globally on the server, **do not cache user-specific data** under a generic tag. If you cache data that varies by user, include their user ID in the cache parameters or keep user-specific fetching dynamic.

### Dynamic APIs poisoning the static shell
Accessing `cookies()` or `headers()` forces page rendering to run per-request (dynamic). If you want to cache the page structure, fetch the dynamic header value inside an isolated, suspended child component, keeping the layout static.

---

## 🎯 Key Takeaways

- **Default to no-store:** Next.js 16 `fetch` requests do not cache by default.
- **`"use cache"` is the primary tool:** Use it on functions or server components to cache their output.
- **`cacheLife` and `cacheTag`** are imported directly from `next/cache` (no `unstable_` prefix).
- **`updateTag` ensures immediate consistency** for user mutations; **`revalidateTag` handles background updates** using stale-while-revalidate.

*←* [`03_server_and_client_components.md`](./03_server_and_client_components.md) *|* *next →* [`05_server_actions_and_mutations.md`](./05_server_actions_and_mutations.md)
