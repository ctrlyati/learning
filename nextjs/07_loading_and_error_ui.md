# 07 — Loading & Error UI

> **Goal:** Use `loading.tsx`, `error.tsx`, `<Suspense>`, `not-found.tsx`, and redirects to deliver fast, resilient UI for every route, not just the happy path.

---

## 1. Concept — files that *are* the boundaries

The App Router gives you special files that automatically wrap your route in the right React boundaries:

- `loading.tsx` — auto-Suspense fallback for the segment.
- `error.tsx` — error boundary for the segment (client component).
- `not-found.tsx` — rendered when `notFound()` is called or no route matches.
- `global-error.tsx` — top-level error boundary (replaces the root layout when it fires).

```tsx
// app/blog/loading.tsx
export default function BlogLoading() {
  return (
    <div className="animate-pulse space-y-2">
      <div className="h-6 w-1/2 rounded bg-neutral-200" />
      <div className="h-4 w-3/4 rounded bg-neutral-200" />
      <div className="h-4 w-2/3 rounded bg-neutral-200" />
    </div>
  );
}
```

```tsx
// app/blog/error.tsx
"use client";
import { useEffect } from "react";

export default function BlogError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // log to Sentry / your tool
    console.error(error);
  }, [error]);

  return (
    <div className="rounded border border-red-300 bg-red-50 p-4">
      <h2 className="font-semibold">Something went wrong.</h2>
      <p className="text-sm text-red-700">{error.message}</p>
      <button onClick={reset} className="mt-2 underline">Try again</button>
    </div>
  );
}
```

```tsx
// app/blog/not-found.tsx
export default function NotFound() {
  return <p>No such post.</p>;
}
```

Those three files turn the `/blog/*` segment into a robust, well-behaved subtree. Visiting it shows a skeleton until data resolves; a crash shows a recoverable error UI; an unknown post 404s gracefully — *without* the rest of your site disappearing.

---

## 2. Mechanism — what each file becomes

Under the hood:

- `loading.tsx` is rendered as the `fallback` of a React `<Suspense>` wrapping the segment's `page.tsx` (and its sub-segments).
- `error.tsx` becomes a React error boundary wrapping the segment. **It must be a Client Component** (`"use client"`) because error boundaries need lifecycle that doesn't exist in RSC.
- `not-found.tsx` is rendered when:
  - A server component calls `notFound()` from `next/navigation`,
  - Middleware/route matcher returns a 404,
  - A static route doesn't exist and no catch-all matches.
- `global-error.tsx` wraps the entire app, including the root layout. It only fires for errors thrown *outside* of any other boundary.

The granularity is per-segment. An error in `/blog/[slug]/page.tsx` is caught by the nearest `error.tsx` (e.g., `app/blog/error.tsx`), which means **other segments stay alive**. The blog's error UI shows; the global nav and footer keep working.

### Suspense vs `loading.tsx`

`loading.tsx` covers the entire segment with a single Suspense boundary. For finer control (multiple independent loading states within one page), use `<Suspense>` explicitly:

```tsx
// app/dashboard/page.tsx
import { Suspense } from "react";

export default function Dashboard() {
  return (
    <main>
      <h1>Dashboard</h1>
      <Suspense fallback={<RevenueSkeleton />}>
        <Revenue />
      </Suspense>
      <Suspense fallback={<ActivitySkeleton />}>
        <Activity />
      </Suspense>
    </main>
  );
}
```

Both `Revenue` and `Activity` stream independently. If you only had `loading.tsx`, the entire dashboard would be one skeleton until the slowest part resolved.

### `redirect` and `permanentRedirect`

Server components and server actions can redirect:

```tsx
import { redirect, permanentRedirect } from "next/navigation";

export default async function Page() {
  const session = await auth();
  if (!session) redirect("/login");      // 307 by default
  // ...
}

// In an action that moves a resource:
permanentRedirect("/new-url");           // 308
```

Both throw internally — don't try/catch them. From client code, use `router.push` / `router.replace` instead.

---

## 3. Variations / depth

### 3.1 Streaming with `loading.tsx` is automatic

The moment you add `app/dashboard/loading.tsx`, Next.js streams the route: the layout + the loading fallback render immediately, the `page.tsx` streams in when ready. There's nothing else to do — the file *is* the streaming setup.

### 3.2 Granular Suspense for different speeds

A common pattern: one section depends on fast data, another on slow:

```tsx
// app/feed/page.tsx
import { Suspense } from "react";

async function FastFeed() {
  const items = await fetch("https://fast.example.com").then((r) => r.json());
  return <ul>{items.map((i: any) => <li key={i.id}>{i.title}</li>)}</ul>;
}

async function SlowAnalytics() {
  await new Promise((r) => setTimeout(r, 2000));
  return <p>Analytics: …</p>;
}

export default function Feed() {
  return (
    <main>
      <Suspense fallback={<p>Loading feed…</p>}>
        <FastFeed />
      </Suspense>
      <Suspense fallback={<p>Loading analytics…</p>}>
        <SlowAnalytics />
      </Suspense>
    </main>
  );
}
```

The feed renders quickly; the analytics block sits with its skeleton until ready.

### 3.3 `error.tsx` and `reset()`

The `reset` function passed to `error.tsx` re-renders the segment from scratch. It's effectively a "retry" button:

```tsx
"use client";
export default function Error({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <div>
      <p>{error.message}</p>
      <button onClick={reset}>Retry</button>
    </div>
  );
}
```

`reset` re-runs the segment's RSC fetch. If the failure was transient (network blip), retry succeeds.

The `error.digest` property is a stable hash of the error (sent to the client without the full stack, for privacy). Log it on your server to correlate.

### 3.4 `global-error.tsx`

Only fires for errors in the root layout or above all per-segment `error.tsx` boundaries. Must render its own `<html>` and `<body>` because the root layout has failed:

```tsx
// app/global-error.tsx
"use client";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <html>
      <body>
        <h2>App crashed.</h2>
        <button onClick={reset}>Try again</button>
      </body>
    </html>
  );
}
```

You will rarely see this in production if your per-segment `error.tsx` files are comprehensive — but always have one for safety.

### 3.5 `notFound()` vs returning null

If a route can't find data, decide:

- **404 the user** → call `notFound()`. The nearest `not-found.tsx` renders, the response is 404.
- **Show empty state** → return JSX with an empty-state message. The response is 200.

Don't conflate the two. SEO and analytics care a lot.

```tsx
import { notFound } from "next/navigation";

export default async function PostPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const post = await getPost(slug);
  if (!post) notFound();          // -> 404 + not-found.tsx
  return <article>{post.body}</article>;
}
```

### 3.6 Loading skeletons that match real layout

A skeleton that looks like the eventual content prevents jarring layout shift. Match the heights, the column structure, and the spacing. Reuse a component:

```tsx
// components/Skeleton.tsx
export function Skeleton({ className = "" }: { className?: string }) {
  return <div className={`animate-pulse rounded bg-neutral-200 ${className}`} />;
}
```

```tsx
// app/blog/loading.tsx
import { Skeleton } from "@/components/Skeleton";

export default function Loading() {
  return (
    <div className="space-y-4">
      {Array.from({ length: 5 }).map((_, i) => (
        <div key={i} className="space-y-2">
          <Skeleton className="h-5 w-1/2" />
          <Skeleton className="h-4 w-3/4" />
        </div>
      ))}
    </div>
  );
}
```

### 3.7 `unauthorized()` and `forbidden()` (Next 15.x)

Recent Next versions add `unauthorized()` and `forbidden()` from `next/navigation`, plus per-segment `unauthorized.tsx` and `forbidden.tsx`. They work like `notFound()` but return 401 / 403. Check your version before relying on them; they're new and still stabilizing.

---

## 4. Practical application — a robust blog segment

The setup: a blog with a list page, detail page, a slow related-posts widget, and explicit error/not-found UI.

```
app/
└── blog/
    ├── layout.tsx
    ├── loading.tsx
    ├── error.tsx
    ├── not-found.tsx
    ├── page.tsx
    └── [slug]/
        ├── loading.tsx
        ├── error.tsx
        ├── not-found.tsx
        └── page.tsx
```

```tsx
// app/blog/layout.tsx
export default function BlogLayout({ children }: { children: React.ReactNode }) {
  return (
    <section className="mx-auto max-w-2xl py-8">
      <h2 className="mb-6 text-xs uppercase tracking-wider text-neutral-500">Blog</h2>
      {children}
    </section>
  );
}
```

```tsx
// app/blog/loading.tsx
import { Skeleton } from "@/components/Skeleton";
export default function Loading() {
  return (
    <ul className="space-y-3">
      {Array.from({ length: 4 }).map((_, i) => (
        <li key={i} className="space-y-2">
          <Skeleton className="h-5 w-1/2" />
          <Skeleton className="h-3 w-3/4" />
        </li>
      ))}
    </ul>
  );
}
```

```tsx
// app/blog/error.tsx
"use client";
import { useEffect } from "react";

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // send to your logger
  }, [error]);
  return (
    <div className="rounded bg-red-50 p-4 text-red-800">
      <p className="font-semibold">Couldn’t load the blog.</p>
      <p className="text-sm">{error.message}</p>
      {error.digest && <p className="text-xs text-red-500">Ref: {error.digest}</p>}
      <button onClick={reset} className="mt-2 underline">Try again</button>
    </div>
  );
}
```

```tsx
// app/blog/not-found.tsx
import Link from "next/link";
export default function NotFound() {
  return (
    <div>
      <p>That blog section doesn’t exist.</p>
      <Link href="/blog" className="underline">Back to all posts</Link>
    </div>
  );
}
```

```tsx
// app/blog/page.tsx
import Link from "next/link";
import { getPosts } from "@/lib/posts";

export default async function BlogIndex() {
  const posts = await getPosts();
  if (posts.length === 0) {
    // Empty state, not a 404 — the section exists, just empty
    return <p>No posts yet. Check back soon.</p>;
  }
  return (
    <ul className="space-y-3">
      {posts.map((p) => (
        <li key={p.slug}>
          <Link href={`/blog/${p.slug}`} className="block hover:underline">
            <h3 className="font-semibold">{p.title}</h3>
            <p className="text-sm text-neutral-600">{p.excerpt}</p>
          </Link>
        </li>
      ))}
    </ul>
  );
}
```

```tsx
// app/blog/[slug]/page.tsx
import { notFound } from "next/navigation";
import { Suspense } from "react";
import { getPost } from "@/lib/posts";
import { RelatedPosts } from "./RelatedPosts";

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
      <h1 className="text-2xl font-bold">{post.title}</h1>
      <p className="mt-4">{post.body}</p>

      <Suspense fallback={<RelatedSkeleton />}>
        <RelatedPosts slug={slug} />
      </Suspense>
    </article>
  );
}

function RelatedSkeleton() {
  return <p className="mt-8 text-sm text-neutral-500">Loading related…</p>;
}
```

```tsx
// app/blog/[slug]/RelatedPosts.tsx (server)
async function getRelated(slug: string) {
  await new Promise((r) => setTimeout(r, 800)); // simulate slow
  return [{ slug: "another", title: "Another post" }];
}

export async function RelatedPosts({ slug }: { slug: string }) {
  const related = await getRelated(slug);
  return (
    <aside className="mt-8 border-t pt-4">
      <h3 className="text-sm font-semibold">Related</h3>
      <ul>{related.map((r) => <li key={r.slug}>{r.title}</li>)}</ul>
    </aside>
  );
}
```

What you've built:
- The blog index has its own skeleton, error UI, and not-found.
- Each post page has its own skeleton + error + not-found, *plus* a granular Suspense for related posts that streams in independently.
- If the related-posts service fails, only that block fails — not the whole post.

This is what "production polish" looks like in the App Router: cheap to add, dramatically improves perceived quality.

---

## 5. Common mistakes & gotchas

### `error.tsx` without `"use client"`

Error boundaries require client lifecycle. Forgetting the directive causes a build error or, worse, a confusing runtime failure. Always start `error.tsx` with `"use client"`.

### `error.tsx` can't catch errors in the layout

`error.tsx` is wrapped *inside* its sibling `layout.tsx`. If the layout itself throws, the `error.tsx` of the same segment can't catch it — only an ancestor's error boundary (or `global-error.tsx`) can. Keep layouts simple.

### `loading.tsx` covers the *whole* segment

If you want one part of the page to load while another renders instantly, `loading.tsx` won't help — it covers everything. Use explicit `<Suspense>` boundaries for granular control.

### Throwing inside an error boundary

If your `error.tsx` itself throws, you bubble up to the next ancestor boundary (and so on, up to `global-error.tsx`). Keep `error.tsx` simple and defensive.

### Treating empty results as 404

A blog with zero posts is not "not found" — the route exists, the data is empty. 404 changes SEO behavior. Use 404 only when the *resource* doesn't exist.

### Forgetting `not-found.tsx`

Without a per-segment `not-found.tsx`, `notFound()` walks up to the nearest ancestor's `not-found.tsx`, or falls back to the default Next 404. Add one per major section for consistent UX.

### Loading flash on fast navigations

If your data is cached, the route may resolve in 10ms and your skeleton flashes for one frame. Solutions: use `useTransition` on the trigger to delay the loading state, or accept the flash (it's rarely visible).

### `redirect()` in client components

`redirect()` only works in server components and server actions. From client components, use `router.push` / `router.replace`. Calling `redirect()` from a client throws an unhelpful error.

---

## 🎯 Key Takeaways

- **`loading.tsx`, `error.tsx`, `not-found.tsx`** are per-segment files that compose into your component tree. Add them by default for every meaningful section.
- **`error.tsx` must be a client component** and provides a `reset` function for retries. Always log the `digest` for production debugging.
- **For finer control than `loading.tsx`**, use explicit `<Suspense>` around independent slow components. Streaming is per-boundary.
- **Distinguish empty from missing.** 404 means the resource doesn't exist; an empty list is a 200 with empty state.
- **`global-error.tsx` is your last line of defense.** It must render its own `<html>`/`<body>` because the root layout has already failed. Add one even if you hope to never see it.

*←* [`06_rendering_strategies.md`](./06_rendering_strategies.md) *|* *next →* [`08_styling.md`](./08_styling.md)
