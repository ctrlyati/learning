# 04 — Data Fetching

> **Goal:** Understand the augmented `fetch`, async server components, request memoization, and time- and tag-based revalidation, then know exactly when each one applies.

---

## 1. Concept — `await fetch` inside a component

The headline feature of the App Router: you can `await` data directly in a server component. No `getStaticProps`, no API route hop, no client-side `useEffect`.

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

That's it. The function is `async`, the page renders only after data resolves (unless wrapped in `<Suspense>` — Module 07), and the resulting HTML ships to the browser. No client JS for this page beyond what React needs for hydration of any client islands.

For non-`fetch` data sources (Prisma, Drizzle, raw SQL, S3, file system, etc.), the same pattern applies — `await` whatever returns a Promise. Module 13 covers ORMs specifically.

---

## 2. Mechanism — the augmented `fetch` and Next.js caching layers

Next.js **monkey-patches the global `fetch`** so it integrates with the framework's caches. There are *three* distinct caches you need to keep straight.

### 2.1 Request Memoization (per request, automatic, can't disable)

During a single request's render pass, identical `fetch(url, options)` calls are deduplicated. Two server components that both call `fetch("/api/user/me")` will hit the network once. This is the React `cache()` function applied at the fetch level — purely an in-memory optimization scoped to one render.

### 2.2 Data Cache (persistent, controls revalidation)

The Data Cache lives on the server (filesystem in dev, on the deploy in prod). It's HTTP-style and indexed by URL + options. You control it via fetch options:

```tsx
// Static: cached forever (until a revalidation tag fires)
const res = await fetch(url, { cache: "force-cache" });

// Time-based: cache and re-validate at most every 60s
const res = await fetch(url, { next: { revalidate: 60 } });

// Tag-based: cache and re-validate when a tag is invalidated
const res = await fetch(url, { next: { tags: ["products"] } });

// Dynamic: never cache, always fetch fresh
const res = await fetch(url, { cache: "no-store" });
```

**Important defaults differ across Next versions:**

- **Next 14 default**: `fetch` is `"force-cache"` (cached indefinitely) unless overridden.
- **Next 15 default**: `fetch` is `"no-store"` (no cache) unless overridden. The team flipped the default after hearing user feedback that the previous behavior was surprising.

This single change accounts for an enormous percentage of "I upgraded and now my data is stale / now my app is slow" reports. **Always check your `package.json`.**

### 2.3 Full Route Cache (the rendered HTML/RSC payload)

For statically-rendered routes, Next.js stores the final HTML + RSC payload on disk after `next build`. If a route's data is fully static (or has been revalidated via tag/time), subsequent requests serve from this cache without re-rendering React.

The Full Route Cache is invalidated when:
- A `revalidateTag(...)` matching any fetch in the route fires,
- A `revalidatePath(...)` for the route fires,
- A time-based `revalidate` interval elapses,
- A new deploy occurs.

### 2.4 Route Segment Config

You can pin caching at the route level via `export const`s at the top of `page.tsx` or `layout.tsx`:

```tsx
// app/feed/page.tsx
export const dynamic = "force-dynamic";  // never cache the route
export const revalidate = 60;            // re-render at most every 60s
export const fetchCache = "force-no-store"; // override fetch defaults for this route
```

Available values:
- `dynamic`: `"auto" | "force-dynamic" | "error" | "force-static"`
- `revalidate`: `false | 0 | <seconds>`
- `fetchCache`: many; usually you don't need this.

---

## 3. Variations / depth

### 3.1 Triggering revalidation: time vs tag vs path

**Time-based** (set when fetching):

```tsx
const res = await fetch(url, { next: { revalidate: 3600 } }); // 1h
```

**Tag-based** (set when fetching, invalidate from anywhere):

```tsx
// when fetching
const res = await fetch("https://api.example.com/products", {
  next: { tags: ["products"] },
});

// later, from a Server Action or Route Handler:
import { revalidateTag } from "next/cache";
revalidateTag("products"); // every fetch tagged "products" is now stale
```

**Path-based** (invalidate everything for a route):

```tsx
import { revalidatePath } from "next/cache";
revalidatePath("/products");          // invalidate /products
revalidatePath("/products/[id]", "page");  // invalidate every product detail page
revalidatePath("/", "layout");        // invalidate the whole layout chain
```

Tag-based revalidation is the most powerful: you can have one mutation invalidate *exactly* the data it affects across many routes.

### 3.2 `cache()` from React — for non-fetch sources

For database calls, use `react.cache()` to opt into request-scoped memoization:

```ts
// lib/data/user.ts
import "server-only";
import { cache } from "react";
import { db } from "@/lib/db";

export const getUser = cache(async (id: string) => {
  return db.user.findUnique({ where: { id } });
});
```

Now `getUser("u1")` called from three server components in the same request makes one DB query. Persistent caching for non-fetch sources requires `unstable_cache`:

```ts
import { unstable_cache } from "next/cache";

export const getTrendingPosts = unstable_cache(
  async () => db.post.findMany({ orderBy: { views: "desc" }, take: 10 }),
  ["trending-posts"],
  { revalidate: 300, tags: ["posts"] }
);
```

The name is intentional: `unstable_cache` may rename in future Next versions. The semantic is stable — wrap a function, get HTTP-cache-like behavior.

### 3.3 Parallel fetching

Sequential awaits are a common performance footgun:

```tsx
// Sequential — slow
const user = await getUser();
const posts = await getPosts(user.id);
const comments = await getComments(); // unrelated to posts, but waits anyway
```

If two awaits don't depend on each other, fire them in parallel:

```tsx
// Parallel — fast
const [user, comments] = await Promise.all([
  getUser(),
  getComments(),
]);
const posts = await getPosts(user.id);
```

### 3.4 Streaming and `<Suspense>` with data

A server component blocks the entire route until it resolves. Wrap slow children in `<Suspense>` to stream them in:

```tsx
// app/dashboard/page.tsx
import { Suspense } from "react";
import { Stats } from "./Stats";
import { RecentActivity } from "./RecentActivity";

export default function Dashboard() {
  return (
    <div>
      <h1>Dashboard</h1>
      <Suspense fallback={<p>Loading stats…</p>}>
        <Stats />
      </Suspense>
      <Suspense fallback={<p>Loading activity…</p>}>
        <RecentActivity />
      </Suspense>
    </div>
  );
}
```

```tsx
// app/dashboard/Stats.tsx — slow server component
async function getStats() {
  // simulate slow DB
  await new Promise((r) => setTimeout(r, 1500));
  return { revenue: 12345 };
}
export async function Stats() {
  const stats = await getStats();
  return <p>Revenue: ${stats.revenue}</p>;
}
```

The shell + headings render immediately; `Stats` and `RecentActivity` stream in independently when ready. The browser sees the page progressively. See Module 06 for the full streaming/PPR story.

### 3.5 Reading cookies, headers — and what they cost

```tsx
// Next 15
import { cookies, headers } from "next/headers";

export default async function Page() {
  const c = await cookies();
  const h = await headers();
  const userId = c.get("uid")?.value;
  // ...
}
```

Reading cookies/headers opts the route into **dynamic rendering** (per-request). That's correct — you can't prerender something that depends on the request — but it disables the Full Route Cache for that route. Use sparingly; push the dynamic part deeper into a Suspense boundary if you want the static shell to still prerender (Partial Prerendering, Module 06).

In Next 14, `cookies()` and `headers()` were synchronous. In Next 15, they're async.

### 3.6 Client-side fetching: when?

Use client-side data fetching (SWR, React Query, or `fetch` in `useEffect`) when:

- The data changes very frequently and is per-user (live chat, ticker, notifications),
- It depends on client state that doesn't exist in the URL (a chart with zoom level),
- You need true real-time via WebSockets or SSE.

For nearly everything else, server fetching is simpler, faster, and cheaper.

```tsx
// app/components/UserSearchClient.tsx
"use client";
import useSWR from "swr";
const fetcher = (url: string) => fetch(url).then((r) => r.json());

export function UserSearchClient({ q }: { q: string }) {
  const { data, isLoading } = useSWR(q ? `/api/users?q=${q}` : null, fetcher);
  if (isLoading) return <p>…</p>;
  return <ul>{data?.map((u: any) => <li key={u.id}>{u.name}</li>)}</ul>;
}
```

---

## 4. Practical application — a blog index with tag-based revalidation

The pattern: blog posts come from a CMS. We want the index page to be statically rendered, fast, and to stay fresh — but to invalidate immediately when an editor publishes a new post via webhook.

```ts
// lib/posts.ts
import "server-only";

export type Post = { slug: string; title: string; excerpt: string; publishedAt: string };

export async function getPosts(): Promise<Post[]> {
  const res = await fetch("https://cms.example.com/api/posts", {
    next: { tags: ["posts"], revalidate: 3600 }, // hourly fallback + tag invalidation
  });
  if (!res.ok) throw new Error(`CMS error: ${res.status}`);
  return res.json();
}

export async function getPost(slug: string): Promise<Post | null> {
  const res = await fetch(`https://cms.example.com/api/posts/${slug}`, {
    next: { tags: ["posts", `post:${slug}`], revalidate: 3600 },
  });
  if (res.status === 404) return null;
  if (!res.ok) throw new Error(`CMS error: ${res.status}`);
  return res.json();
}
```

```tsx
// app/blog/page.tsx
import Link from "next/link";
import { getPosts } from "@/lib/posts";

export default async function BlogIndex() {
  const posts = await getPosts();
  return (
    <section>
      <h1 className="text-2xl font-bold">Blog</h1>
      <ul className="mt-4 space-y-3">
        {posts.map((p) => (
          <li key={p.slug}>
            <Link href={`/blog/${p.slug}`} className="block hover:underline">
              <h2 className="text-lg font-semibold">{p.title}</h2>
              <p className="text-sm text-neutral-600">{p.excerpt}</p>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
```

```tsx
// app/blog/[slug]/page.tsx
import { notFound } from "next/navigation";
import { getPost } from "@/lib/posts";

export default async function PostPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const post = await getPost(slug);
  if (!post) notFound();
  return (
    <article>
      <h1 className="text-3xl font-bold">{post.title}</h1>
      <time className="text-sm text-neutral-500">{post.publishedAt}</time>
      <p className="mt-4">{post.excerpt}</p>
    </article>
  );
}
```

The CMS webhook hits this route handler:

```ts
// app/api/revalidate/route.ts
import { revalidateTag } from "next/cache";
import { NextRequest, NextResponse } from "next/server";

export async function POST(req: NextRequest) {
  const secret = req.nextUrl.searchParams.get("secret");
  if (secret !== process.env.REVALIDATE_SECRET) {
    return NextResponse.json({ ok: false }, { status: 401 });
  }
  const { tag } = await req.json();
  revalidateTag(tag ?? "posts");
  return NextResponse.json({ ok: true, revalidated: tag });
}
```

Behavior:
- Blog index serves from static cache, hourly fallback revalidation in the background.
- The instant an editor hits "Publish" in the CMS, the CMS pings `/api/revalidate?secret=...` with `{ "tag": "posts" }`.
- Next.js invalidates every fetch tagged `"posts"`. Next request rebuilds. Visitors after that see fresh data, with no full deploy.

This is the same pattern that runs production e-commerce and content sites.

---

## 5. Common mistakes & gotchas

### Assuming the default

A junior dev sees `fetch(url)` and assumes the default — but the default flipped between Next 14 and 15. **Always be explicit** when caching matters:

```tsx
await fetch(url, { cache: "force-cache" });   // I want this cached
await fetch(url, { cache: "no-store" });      // I want this fresh every request
```

Reviewers will thank you.

### `cache: "no-store"` is route-poisoning

Any `no-store` fetch (or read of `cookies()`/`headers()`/`searchParams`) opts the *entire route* out of static rendering. If you only need one dynamic piece, push it into a Suspense child so the rest of the route can prerender. Or use PPR (Module 06).

### Forgetting `next: { tags: [...] }` until you need revalidation

You can only `revalidateTag(...)` tags that were registered on a fetch *before* it was cached. Tagging is opt-in at fetch time. Make it a team rule: every external `fetch` gets a tag, even if you haven't wired up the invalidation yet.

### `unstable_cache` swallowing errors

If the wrapped function throws on first call, `unstable_cache` caches the error. Subsequent calls within the revalidation window get the same error. Defend with try/catch inside, or accept and tolerate.

### Sequential awaits in components

```tsx
const a = await getA();
const b = await getB();  // independent of a — slows page for no reason
```

Use `Promise.all` whenever possible. The dev tools "rendering time" hint is your friend.

### Caching responses with `Authorization` headers

Fetches with `Authorization` headers default to *not* being cached even in Next 14. This is a security default. If you really want them cached, set `cache: "force-cache"` explicitly — but think hard about whether that's correct (probably not).

### Per-user data in a static route

If your route is static (no `no-store`, no `cookies()`) but you fetch per-user data, every user gets the *same* cached HTML. Either make the route dynamic, push the user-specific part into a Suspense client island, or read cookies (which forces dynamic).

### Forgetting that `revalidatePath` only invalidates the *server-side* cache

It does NOT invalidate any browser cache or any CDN cache outside Next's control. If you've added an external CDN with its own TTL, you need to purge that too.

---

## 🎯 Key Takeaways

- **`fetch` is augmented**, and the default behavior changed between Next 14 (`force-cache`) and Next 15 (`no-store`). Be explicit; never rely on the default in production code.
- **There are three caches**: request memoization (automatic), Data Cache (URL+options), Full Route Cache (rendered output). Knowing which one to invalidate is half of debugging.
- **Tag-based revalidation is the production sweet spot.** Tag at fetch, invalidate from your mutation (or a webhook). One mutation, scoped invalidation, zero stale data.
- **Use `react.cache()` for per-request memoization of non-fetch sources**, and `unstable_cache` for persistent caching of DB queries. Pair both with tags.
- **Parallelize whenever you can** with `Promise.all`, and **stream slow stuff** with `<Suspense>`. The framework rewards letting it stream.

*←* [`03_server_and_client_components.md`](./03_server_and_client_components.md) *|* *next →* [`05_server_actions_and_mutations.md`](./05_server_actions_and_mutations.md)
