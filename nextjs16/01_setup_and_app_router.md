# 01 — Setup & The App Router

> **Goal:** Stand up a Next.js 16 project from scratch, understand the App Router model in contrast to the Pages Router, and know exactly where every file goes.

---

## 1. Concept — your first App Router project

The App Router is Next.js's modern routing system, built on top of **React Server Components**. Routes live under the `app/` directory; every folder is a URL segment; a file named `page.tsx` makes the segment publicly routable.

Create a new project:

```bash
pnpm create next-app@latest my-app --typescript --eslint --tailwind --app --src-dir --import-alias "@/*"
cd my-app
pnpm dev
```

Key things to know about Next.js 16 defaults:
- **Turbopack by default:** Turbopack is now the default compiler engine for both `next dev` and `next build` (providing instant startup, filesystem-caching, and ultra-fast hot reloading). You no longer need to pass `--turbopack`.
- **React Compiler by default:** The React Compiler is integrated and enabled by default in React 19 / Next.js 16. It automatically optimizes and memoizes your rendering, drastically reducing the need for manual performance hooks.
- `--src-dir` — puts `app/` inside `src/`. Recommended for keeping project config separate from application code.

Your minimal first page lives at `src/app/page.tsx`:

```tsx
// src/app/page.tsx
export default function HomePage() {
  return (
    <main>
      <h1>Hello, Next.js 16!</h1>
      <p>The file system is the router.</p>
    </main>
  );
}
```

That file *is* the route `/`. The framework automatically discovers it.

Now add a layout that wraps every page in the segment (and below):

```tsx
// src/app/layout.tsx
import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "My App",
  description: "Learning Next.js 16 App Router",
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

`layout.tsx` is the *only* place you write `<html>` and `<body>` — and the root layout is **required**. Run `pnpm dev` and visit `http://localhost:3000`. You're rendering a React Server Component on the server, streaming HTML to the browser, with zero client-side JavaScript for the static parts of the page.

---

## 2. Mechanism — what `next dev` actually does

When you run `pnpm dev`:

1. **Next.js boots the Turbopack dev server** (default port 3000). Turbopack handles fast incremental compilation, caching resolved import paths in a local file-system cache (`.next/cache/`).
2. The server **scans `app/`** for special files: `layout`, `page`, `loading`, `error`, `not-found`, `template`, `route`, `default`. Each file becomes part of a route's component tree.
3. On request, Next matches the URL to a segment chain (e.g. `/blog/hello` → `src/app/layout.tsx` > `src/app/blog/layout.tsx` > `src/app/blog/[slug]/page.tsx`).
4. The server **renders React Server Components to an RSC payload** — a streamable serialized tree (HTML for first paint, plus a wire format React uses to hydrate and update).
5. **Client components** in the tree have their bundles shipped to the browser; the browser hydrates only those interactive components.
6. Subsequent in-app navigations fetch new RSC payloads, not full HTML pages — this is the "soft" navigation model.

```
Request flow (simplified):

Browser  ──GET /blog──>  Next.js server (Turbopack)
                          ├─ resolve segments
                          ├─ run Server Components (await DB, APIs, fetch...)
                          ├─ stream RSC payload
                          └─ ship HTML + JSON wire format
Browser  <──stream──     React hydrates client islands
```

The Pages Router, by contrast, used `getServerSideProps` / `getStaticProps` and shipped all page code to the browser. App Router separates "code that runs on the server" from "code that runs on the client" at the file level. This is the central shift.

---

## 3. Variations / depth

### 3.1 Project structure conventions

A typical App Router project in Next.js 16 (`--src-dir`):

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
│   │   └── blog/
│   │       ├── layout.tsx      # nested layout for /blog/*
│   │       └── [slug]/page.tsx # /blog/:slug
│   ├── components/             # shared components
│   ├── lib/                    # server-side utilities (db, auth, etc.)
│   └── styles/
├── public/                     # static assets served at /
├── next.config.ts              # TypeScript configuration by default
├── tsconfig.json
├── package.json
└── .env.local
```

Recommended conventions:
- `components/ui/` for presentational client components (UI primitives like buttons, inputs).
- `components/server/` for reusable server components (data loaders).
- `lib/db.ts`, `lib/auth.ts`, `lib/env.ts` for server-only utilities.
- Co-locate route-specific components *inside* the route folder (e.g. `app/blog/[slug]/_components/PostBody.tsx`). Folders prefixed with `_` are **private** and never become routes.

### 3.2 The `next.config.ts`

Next.js 16 projects generate `next.config.ts` (using TypeScript) by default:

```typescript
// next.config.ts
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  images: {
    remotePatterns: [{ protocol: "https", hostname: "images.unsplash.com" }],
  },
};

export default nextConfig;
```

Using a typed config file eliminates configuration guesswork and errors.

### 3.3 `tsconfig.json` highlights

`create-next-app` generates a modern `tsconfig.json`. The two settings worth knowing:

- `"moduleResolution": "bundler"` — lets you import without `.js` extensions.
- `"paths": { "@/*": ["./src/*"] }` — enables clean absolute imports using `@/lib/db`, etc.

### 3.4 App Router vs Pages Router — at a glance

| Aspect              | Pages Router (legacy)           | App Router (modern in Next 16)             |
|---------------------|---------------------------------|-------------------------------------------|
| Folder              | `pages/`                        | `app/`                                    |
| Default component   | Client component                | **Server component**                      |
| Data fetching       | `getServerSideProps` / `getStaticProps` | `await fetch` or direct DB query in RSC |
| Layouts             | Manual `_app.tsx` patterns      | Nested `layout.tsx` per segment           |
| Loading state       | Manual                          | `loading.tsx` + Suspense                  |
| Mutations           | API routes + client fetch       | **Server Actions** (`"use server"`)       |
| Streaming           | Limited                         | First-class via RSC                       |
| Bundle              | Everything ships to client      | Server components stay server-side        |

---

## 4. Practical application — a real "Hello, blog" slice

Let's build a structure that exercises layouts, a static page, and a dynamic route. This is the core setup you will extend in subsequent modules.

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
// In Next.js 16, dynamic params are asynchronous Promises and must be awaited.
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

That's a complete, multi-page, layout-aware website. Navigations between routes use soft navigation: Next.js fetches only the diff in RSC payloads, not a full new page.

---

## 5. Common mistakes & gotchas

### `"use client"` reflex from React habit
Do not add `"use client"` by default. Add it only when you need state, effects, browser APIs, or event handlers. Putting `"use client"` at the root layout levels bloats the client bundle and disables optimization.

### Forgetting the root layout
A root layout (with `<html>` and `<body>`) is mandatory. If you delete it, the dev server will fail to build.

### Mixing `pages/` and `app/` for the same URL
If both `pages/about.tsx` and `src/app/about/page.tsx` exist, `app/` takes priority silently, but the legacy file still compiles. Delete one or the other to avoid bundle bloat.

### Forgetting to `await params`
In Next.js 16, `params` and `searchParams` are Promises. You **must** await them before accessing their keys. Accessing them synchronously will cause runtime warnings or errors.

```tsx
// Correct
export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  // ...
}
```

---

## 🎯 Key Takeaways

- **Next.js 16 defaults to Turbopack and React Compiler** out of the box, optimizing compile times and rendering without manual config.
- **The file system is the router**: folders become URL segments, `page.tsx` makes them routable, `layout.tsx` wraps them.
- **Default to Server Components**. They require less code, reduce bundle sizes, and run securely on the server.
- **`params` are Promises**. Always await parameters when dealing with dynamic routing in your page components.

*←* [`00_roadmap.md`](./00_roadmap.md) *|* *next →* [`02_routing_fundamentals.md`](./02_routing_fundamentals.md)
