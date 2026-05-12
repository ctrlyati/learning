# 01 — Setup & The App Router

> **Goal:** Stand up a Next.js 14/15 project from scratch, understand the App Router model in contrast to the Pages Router, and know exactly where every file goes.

---

## 1. Concept — your first App Router project

The App Router is Next.js's modern routing system, built on top of **React Server Components**. Routes live under the `app/` directory; every folder is a URL segment; a file named `page.tsx` makes the segment publicly routable.

Create a new project:

```bash
pnpm create next-app@latest my-app --typescript --eslint --tailwind --app --src-dir --import-alias "@/*"
cd my-app
pnpm dev
```

Flags worth knowing:
- `--app` — opt into the App Router (default in 14/15; explicit is better).
- `--src-dir` — put `app/` inside `src/`. Recommended for non-trivial projects.
- `--turbopack` — opt into Turbopack dev server (default in 15).

Your minimal first page lives at `src/app/page.tsx`:

```tsx
// src/app/page.tsx
export default function HomePage() {
  return (
    <main>
      <h1>Hello, App Router</h1>
      <p>The file system is the router.</p>
    </main>
  );
}
```

That file *is* the route `/`. No `react-router`, no `pages.js`, no configuration. The framework discovered it.

Now add a layout that wraps every page in the segment (and below):

```tsx
// src/app/layout.tsx
import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "My App",
  description: "Learning Next.js App Router",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
```

`layout.tsx` is the *only* place you write `<html>` and `<body>` — and the root layout is **required**. Run `pnpm dev` and visit `http://localhost:3000`. You're rendering a React Server Component on the server, streaming HTML to the browser, with zero client-side JavaScript for the page itself.

---

## 2. Mechanism — what `next dev` actually does

When you run `pnpm dev`:

1. **Next.js boots a Node.js HTTP server** (default port 3000). On Next 15 with Turbopack, the bundler is Rust-based; on 14, it's webpack.
2. The server **scans `app/`** for special files: `layout`, `page`, `loading`, `error`, `not-found`, `template`, `route`, `default`. Each file becomes part of a route's component tree.
3. On request, Next matches the URL to a segment chain (e.g. `/blog/hello` → `app/layout.tsx` > `app/blog/layout.tsx` > `app/blog/[slug]/page.tsx`).
4. The server **renders React Server Components to an RSC payload** — a streamable serialized tree (HTML for first paint, plus a wire format React uses to hydrate and update).
5. **Client components** in the tree have their bundles shipped to the browser; the browser hydrates only those islands.
6. Subsequent in-app navigations fetch new RSC payloads, not full HTML pages — this is the "soft" navigation model.

```
Request flow (simplified):

Browser  ──GET /blog──>  Next.js server
                          ├─ resolve segments
                          ├─ run Server Components (await fetch, db calls...)
                          ├─ stream RSC payload
                          └─ ship HTML + JSON wire format
Browser  <──stream──     React hydrates client islands
```

The Pages Router, by contrast, used `getServerSideProps` / `getStaticProps` and shipped *all* page code to the browser. App Router separates "code that runs on the server" from "code that runs on the client" at the file level. This is the central shift.

---

## 3. Variations / depth

### 3.1 Project structure conventions

A typical App Router project (`--src-dir`):

```
my-app/
├── src/
│   ├── app/
│   │   ├── layout.tsx          # root layout (required)
│   │   ├── page.tsx            # /
│   │   ├── globals.css
│   │   ├── (marketing)/        # route group — does NOT add a URL segment
│   │   │   ├── about/page.tsx  # /about
│   │   │   └── pricing/page.tsx
│   │   ├── blog/
│   │   │   ├── layout.tsx      # nested layout for /blog/*
│   │   │   └── [slug]/page.tsx # /blog/:slug
│   │   └── api/
│   │       └── health/route.ts # GET /api/health
│   ├── components/             # shared components
│   ├── lib/                    # server-side utilities (db, auth, etc.)
│   └── styles/
├── public/                     # static assets served at /
├── next.config.mjs
├── tsconfig.json
├── package.json
└── .env.local
```

Convention I recommend:

- `components/ui/` for presentational client components,
- `components/server/` for reusable server components (data loaders),
- `lib/db.ts`, `lib/auth.ts`, `lib/env.ts` for server-only utilities,
- co-locate route-specific components *inside* the route folder (e.g. `app/blog/[slug]/_components/PostBody.tsx`). Folders prefixed with `_` are **private** and never become routes.

### 3.2 The `next.config.mjs`

Minimal but useful config:

```js
// next.config.mjs
/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  experimental: {
    // ppr: 'incremental',   // Partial Prerendering (15+) — see Module 06
    // typedRoutes: true,    // typed Link href
  },
  images: {
    remotePatterns: [{ protocol: "https", hostname: "images.unsplash.com" }],
  },
};

export default nextConfig;
```

Use `.mjs` (ESM). Most modern Next.js features document examples in ESM, and async config (`async () => ({...})`) only works there.

### 3.3 `tsconfig.json` highlights

`create-next-app` generates a good `tsconfig.json`. The two settings worth knowing:

- `"moduleResolution": "bundler"` — lets you import without `.js` extensions.
- `"paths": { "@/*": ["./src/*"] }` — the `@/lib/db` import you'll see everywhere.

### 3.4 App Router vs Pages Router — at a glance

| Aspect              | Pages Router (legacy)           | App Router (modern)                       |
|---------------------|---------------------------------|-------------------------------------------|
| Folder              | `pages/`                        | `app/`                                    |
| Default component   | Client component                | **Server component**                      |
| Data fetching       | `getServerSideProps` / `getStaticProps` | `await fetch` directly in component |
| Layouts             | Manual `_app.tsx` patterns      | Nested `layout.tsx` per segment           |
| Loading state       | Manual                          | `loading.tsx` + Suspense                  |
| Mutations           | API routes + client fetch       | **Server Actions** (`"use server"`)       |
| Streaming           | Limited                         | First-class via RSC                       |
| Bundle              | Everything ships to client      | Server components stay server-side        |

You can run both routers in the same project (`pages/` and `app/` coexist), but mixing is a transitional tool, not a destination. New projects: App Router only.

---

## 4. Practical application — a real "Hello, blog" slice

Let's build a tiny structure that exercises layouts, a static page, and a dynamic route. This is the spine you'll extend in every subsequent module.

```tsx
// src/app/layout.tsx
import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "Acme", template: "%s · Acme" },
  description: "An example Next.js app.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="min-h-screen bg-neutral-50 text-neutral-900">
        <header className="border-b">
          <nav className="mx-auto flex max-w-3xl items-center gap-4 p-4">
            <Link href="/" className="font-semibold">Acme</Link>
            <Link href="/blog">Blog</Link>
            <Link href="/about">About</Link>
          </nav>
        </header>
        <main className="mx-auto max-w-3xl p-4">{children}</main>
      </body>
    </html>
  );
}
```

```tsx
// src/app/page.tsx
export default function HomePage() {
  return (
    <section>
      <h1 className="text-3xl font-bold">Welcome to Acme</h1>
      <p className="mt-2 text-neutral-600">Read our latest posts in the blog.</p>
    </section>
  );
}
```

```tsx
// src/app/about/page.tsx
import type { Metadata } from "next";

export const metadata: Metadata = { title: "About" };

export default function AboutPage() {
  return <h1 className="text-2xl font-bold">About Acme</h1>;
}
```

```tsx
// src/app/blog/layout.tsx
export default function BlogLayout({ children }: { children: React.ReactNode }) {
  return (
    <div>
      <h2 className="mb-4 text-sm uppercase tracking-wide text-neutral-500">Blog</h2>
      {children}
    </div>
  );
}
```

```tsx
// src/app/blog/page.tsx
import Link from "next/link";

const POSTS = [
  { slug: "hello-app-router", title: "Hello, App Router" },
  { slug: "rsc-mental-model", title: "An RSC mental model" },
];

export default function BlogIndex() {
  return (
    <ul className="space-y-2">
      {POSTS.map((p) => (
        <li key={p.slug}>
          <Link href={`/blog/${p.slug}`} className="underline">
            {p.title}
          </Link>
        </li>
      ))}
    </ul>
  );
}
```

```tsx
// src/app/blog/[slug]/page.tsx
// NOTE (Next 15): `params` is now a Promise. In Next 14 it was a plain object.
import { notFound } from "next/navigation";

const POSTS: Record<string, { title: string; body: string }> = {
  "hello-app-router": { title: "Hello, App Router", body: "Welcome." },
  "rsc-mental-model": { title: "An RSC mental model", body: "Default to server." },
};

export default async function PostPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const post = POSTS[slug];
  if (!post) notFound();

  return (
    <article>
      <h1 className="text-2xl font-bold">{post.title}</h1>
      <p className="mt-2">{post.body}</p>
    </article>
  );
}
```

That's a complete, multi-page, layout-aware, statically-renderable site. Zero `useState`. Zero hydration cost. The `<Link>` between pages does soft navigation — observe in DevTools: only an RSC payload comes back, not a full HTML page.

---

## 5. Common mistakes & gotchas

### `"use client"` reflex from React habit

Old React tutorials often start with `"use client"` for the root because *everything* used to be client. In the App Router, **omit `"use client"` by default**. Add it only when you need state, effects, browser APIs, or event handlers. Putting `"use client"` on `app/layout.tsx` will balloon your client bundle and disable a lot of optimizations.

### Forgetting the root layout

If you delete `app/layout.tsx`, the dev server will throw at startup. A root layout (with `<html>` and `<body>`) is mandatory. Don't write `<html>` anywhere else.

### Mixing `pages/` and `app/` for the same URL

If both `pages/about.tsx` and `app/about/page.tsx` exist, `app/` wins, silently — but the legacy file still ships JS to the bundle. Delete one or the other.

### Next 14 vs 15: async `params` and `searchParams`

In Next.js 15, `params` and `searchParams` are `Promise`s — you must `await` them. In Next.js 14 they were plain objects. This is the single most common upgrade footgun. Always check your version (`package.json`) before copy-pasting examples from blog posts.

```tsx
// Next 15
export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  // ...
}

// Next 14
export default function Page({ params }: { params: { id: string } }) {
  const { id } = params;
  // ...
}
```

### `node_modules` is huge, dev start is slow

Use `pnpm` and Turbopack (`next dev --turbopack`, default in 15). The webpack-based dev server in older versions is noticeably slower on cold start.

### Putting secrets in client code

Any variable not prefixed with `NEXT_PUBLIC_` is server-only. If you accidentally `console.log(process.env.DATABASE_URL)` inside a client component, you get `undefined` *and* the build won't warn you loudly. Always read secrets in server components, route handlers, or server actions.

---

## 🎯 Key Takeaways

- **The App Router is the modern Next.js model**; the Pages Router is legacy. Set up new projects with `--app`, and treat Pages Router patterns you find online as historical context unless you're maintaining a migration.
- **The file system is the router**: folders become URL segments, `page.tsx` makes them routable, `layout.tsx` wraps them. Special filenames have reserved meanings — memorize them once.
- **Default to Server Components**. They are faster, more secure, smaller-bundle, and require less code than client components for read-only UI. Reach for `"use client"` deliberately.
- **`next dev` runs a real Node server even in dev**, and renders RSC payloads — not full HTML pages — for in-app navigation. Watching the network tab in DevTools will demystify the model faster than any blog post.
- **Pin your Next version in your mental model**: Next 14 vs 15 has real API breaks (async `params`, default `fetch` cache). Whenever you copy code from the internet, look at when it was written.

*next →* [`02_routing_fundamentals.md`](./02_routing_fundamentals.md)
