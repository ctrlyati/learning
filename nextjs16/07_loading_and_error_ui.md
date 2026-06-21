# 07 — Loading & Error UI

> **Goal:** Utilize `loading.tsx`, `error.tsx`, `<Suspense>`, `not-found.tsx`, and redirects to deliver resilient and polished user interfaces for every route, handling errors and loading states gracefully.

---

## 1. Concept — Boundary Files

The App Router provides built-in special files that wrap your route in the correct React boundaries:

- `loading.tsx` — Automatic `<Suspense>` fallback wrapper for the route segment.
- `error.tsx` — React error boundary wrapper for the segment (must be a Client Component).
- `not-found.tsx` — Rendered when `notFound()` is explicitly invoked, or a route doesn't match.
- `global-error.tsx` — The final fallback error boundary wrapping the root layout.

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
    console.error("Logged boundary error:", error);
  }, [error]);

  return (
    <div className="rounded border border-red-300 bg-red-50 p-4">
      <h2 className="font-semibold text-red-900">Something went wrong.</h2>
      <p className="text-sm text-red-700">{error.message}</p>
      <button onClick={reset} className="mt-2 text-sm underline font-bold">Try again</button>
    </div>
  );
}
```

---

## 2. Mechanism — Subtree Boundaries

Under the hood:

- `loading.tsx` becomes the `fallback` for an implicit `<Suspense>` boundary wrapping the `page.tsx` component.
- `error.tsx` wraps the segment in a standard React Error Boundary. It must be marked `"use client"` because Error Boundaries require lifecycle methods.
- `global-error.tsx` runs outside the root layout. If an error is thrown in `layout.tsx` (the root), `global-error.tsx` renders its own root `<html>` and `<body>` tags to recover.

### Next.js 16 Debugging Upgrades
Next.js 16 (specifically 16.2+) introduces several developer experience improvements for boundary errors:
- **Hydration Diff Indicator:** If there is a hydration mismatch between the server HTML and client-rendered HTML, the dev error overlay displays a visual code diff comparing the mismatched elements.
- **Server Function Logging:** Server Actions and Server Function calls print diagnostic trace logs directly in the terminal, making execution failures easy to spot.
- **Modern 500 Pages:** Provides a redesigned, modern default server-side error page.

### Loading: `loading.tsx` vs. `<Suspense>`

`loading.tsx` covers the entire route segment. For granular, independent loading skeletons on a single page, use `<Suspense>` explicitly:

```tsx
// app/dashboard/page.tsx
import { Suspense } from "react";
import { Revenue } from "./Revenue";
import { RecentActivity } from "./RecentActivity";

export default function Dashboard() {
  return (
    <main>
      <h1>Dashboard</h1>
      <Suspense fallback={<p>Loading revenue stats...</p>}>
        <Revenue />
      </Suspense>
      <Suspense fallback={<p>Loading recent activity...</p>}>
        <RecentActivity />
      </Suspense>
    </main>
  );
}
```

This prevents the entire page from waiting for the slowest data source to load.

---

## 3. Variations / Depth

### 3.1 Error Recovery with `reset()`

The `reset` parameter passed to `error.tsx` lets you trigger a re-render of the segment. If the error was a temporary database connection spike or network timeout, clicking retry re-runs the RSC fetch.

```tsx
"use client";

export default function ErrorPage({ reset }: { reset: () => void }) {
  return <button onClick={reset} className="border p-2">Retry</button>;
}
```

### 3.2 Global Error Fallback

`global-error.tsx` acts as the safety net for your entire app. Since it replaces the root layout, you must provide basic document markup:

```tsx
// app/global-error.tsx
"use client";

export default function GlobalError({ reset }: { reset: () => void }) {
  return (
    <html>
      <body className="flex flex-col items-center justify-center min-h-screen">
        <h2>A critical error occurred.</h2>
        <button onClick={reset} className="mt-2 bg-black text-white p-2">Try again</button>
      </body>
    </html>
  );
}
```

### 3.3 Dynamic Redirects

You can use `redirect()` and `permanentRedirect()` inside Server Components and Actions:

```typescript
import { redirect } from "next/navigation";

export default async function Page() {
  const user = await fetchUser();
  if (!user) {
    redirect("/login"); // Throws a special Next.js routing error
  }
}
```

---

## 4. Practical Application — Clean Blog Segment

Here is a robust structure for a `/blog` section with fallback boundaries.

```
app/
└── blog/
    ├── layout.tsx
    ├── loading.tsx
    ├── error.tsx
    ├── page.tsx
    └── [slug]/
        ├── not-found.tsx
        └── page.tsx
```

```tsx
// app/blog/layout.tsx
export default function BlogLayout({ children }: { children: React.ReactNode }) {
  return (
    <section className="mx-auto max-w-2xl py-8">
      <h2 className="text-neutral-500 uppercase text-xs">Articles</h2>
      <div className="mt-4">{children}</div>
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

  if (!post) {
    notFound(); // Triggers app/blog/[slug]/not-found.tsx
  }

  return (
    <article>
      <h1 className="text-2xl font-bold">{post.title}</h1>
      <p className="mt-4">{post.body}</p>
    </article>
  );
}
```

```tsx
// app/blog/[slug]/not-found.tsx
import Link from "next/link";

export default function PostNotFound() {
  return (
    <div className="border border-dashed p-6 text-center">
      <p>This blog post does not exist or has been archived.</p>
      <Link href="/blog" className="text-blue-600 underline mt-2 inline-block">
        Return to Catalog
      </Link>
    </div>
  );
}
```

---

## 5. Common Mistakes & Gotchas

### Missing `"use client"` in `error.tsx`
This will cause compilation errors since Next.js cannot render React class/lifecycle error boundaries inside Server Components.

### Throwing inside `error.tsx`
If your error boundary component throws an error, the application bubbles up to the next outer boundary. Keep error recovery components simple and self-contained.

### Synch-redirect try/catch swallow
`redirect()` works by throwing a specific error. Wrapping `redirect()` in a general `try/catch` without checking for Next.js routing exceptions will swallow the redirect:

```typescript
// WRONG
try {
  redirect("/dashboard");
} catch (e) {
  console.log("Failed action"); // Swallows the routing redirect error!
}
```

---

## 🎯 Key Takeaways

- **Segment files are layout-aware boundaries:** Map `loading`, `error`, and `not-found` files within routing segments to preserve shell layout integrity.
- **Hydration visual differences:** Next.js 16.2 provides hydration code diff layouts directly in your browser during dev runtime.
- **Isolate dynamic loading:** Prefer explicit React `<Suspense>` wrappers around slow components for progressive streaming.

*←* [`06_rendering_strategies.md`](./06_rendering_strategies.md) *|* *next →* [`08_styling.md`](./08_styling.md)
