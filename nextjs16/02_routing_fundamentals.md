# 02 — Routing Fundamentals

> **Goal:** Master every routing primitive in the App Router — segments, layouts, groups, dynamic params, parallel & intercepting routes — so URLs and component trees become trivially mappable.

---

## 1. Concept — folders are URL segments

In the App Router, **the file system is the router**. There is no route config, no JSX-based `<Route>` declarations. To make `/products/socks`, you create:

```
app/
└── products/
    └── socks/
        └── page.tsx
```

A folder without a `page.tsx` is just a path that can't be navigated to (but can still host layouts, loading states, etc.). A `page.tsx` makes that segment a *public route*.

```tsx
// app/products/socks/page.tsx
export default function SocksPage() {
  return <h1>Comfortable socks.</h1>;
}
```

That's the entire mental model. Everything else is a refinement of it.

### Special filenames

| File           | Purpose                                                            |
|----------------|--------------------------------------------------------------------|
| `page.tsx`     | Makes the segment routable. The "leaf" UI.                         |
| `layout.tsx`   | Wraps the segment and all child segments. Persists across nav.     |
| `template.tsx` | Like `layout`, but re-mounts on navigation. Rare; use for animations. |
| `loading.tsx`  | Suspense fallback for the segment (see Module 07).                 |
| `error.tsx`    | Error boundary for the segment (must be client component).         |
| `not-found.tsx`| Rendered when `notFound()` is called or unmatched route.           |
| `route.ts`     | API route handler (see Module 14). Cannot coexist with `page.tsx`. |
| `default.tsx`  | Fallback for unmatched parallel route slots (see §3.4).            |

---

## 2. Mechanism — how the matcher works at request time

When a request arrives, Next.js:

1. **Tokenizes the URL** into segments: `/blog/2026/hello` → `["blog", "2026", "hello"]`.
2. **Walks `app/`** matching folders to segments in order. Static names match first; dynamic names (`[param]`) match anything; catch-all (`[...slug]`) absorbs the rest.
3. **Builds a layout chain** — every `layout.tsx` from root down to the matched `page.tsx` becomes part of the render tree, in order.
4. **Renders the chain** as a single React tree: `RootLayout > BlogLayout > YearLayout > PostPage`.
5. **Streams the RSC payload** (HTML + serialized tree).

For an in-app `<Link>` navigation, the existing layout chain is *preserved* if it overlaps — only the diff renders. That's why moving from `/blog/a` to `/blog/b` keeps your blog sidebar mounted and only the `[slug]/page.tsx` re-renders.

Route-group folders `(name)` and private folders `_name` don't affect the URL — they are organizational.

---

## 3. Variations / depth

### 3.1 Layouts and nesting

```
app/
├── layout.tsx           # root: <html>, <body>, global nav
├── dashboard/
│   ├── layout.tsx       # sidebar
│   ├── page.tsx         # /dashboard
│   └── settings/
│       ├── layout.tsx   # settings tabs
│       ├── page.tsx     # /dashboard/settings
│       └── billing/page.tsx  # /dashboard/settings/billing
```

```tsx
// app/dashboard/layout.tsx
import Sidebar from "@/components/server/Sidebar";

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="grid grid-cols-[200px_1fr] gap-8">
      <Sidebar />
      <section>{children}</section>
    </div>
  );
}
```

`/dashboard/settings/billing` renders `RootLayout > DashboardLayout > SettingsLayout > BillingPage`. When you navigate between settings tabs, `DashboardLayout` is **not re-rendered** — it persists. That's the structural advantage over the Pages Router.

### 3.2 Route groups — organization without URL impact

Folders wrapped in parentheses are **route groups**. They group routes for layout sharing or code organization without adding a URL segment.

```
app/
├── (marketing)/
│   ├── layout.tsx       # marketing-specific layout
│   ├── page.tsx         # /
│   ├── about/page.tsx   # /about
│   └── pricing/page.tsx # /pricing
├── (app)/
│   ├── layout.tsx       # app-shell layout (auth, sidebar)
│   ├── dashboard/page.tsx       # /dashboard
│   └── settings/page.tsx        # /settings
```

URLs are `/`, `/about`, `/pricing`, `/dashboard`, `/settings`. The two groups can have completely different layouts and even root-level `<html>`/`<body>` wrappers in their own root layouts (advanced pattern: multiple root layouts).

### 3.3 Dynamic segments

```
app/
├── blog/[slug]/page.tsx               # /blog/anything
├── shop/[category]/[product]/page.tsx # /shop/shoes/airmax
└── docs/[...slug]/page.tsx            # /docs/anything/many/levels (catch-all)
└── docs/[[...slug]]/page.tsx          # /docs AND /docs/anything... (optional catch-all)
```

```tsx
// app/blog/[slug]/page.tsx (Next 16)
export default async function PostPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return <h1>{slug}</h1>;
}
```

To pre-build a known set of dynamic pages at build time, export `generateStaticParams`:

```tsx
// app/blog/[slug]/page.tsx
export async function generateStaticParams() {
  const posts = await fetch("https://api.example.com/posts").then((r) => r.json());
  return posts.map((p: { slug: string }) => ({ slug: p.slug }));
}
```

This is the App Router equivalent of `getStaticPaths`. Combined with `export const dynamicParams = false`, unknown slugs 404; with `true` (default), they render on-demand and are cached.

### 3.4 Parallel routes

A folder prefixed with `@` is a **slot** that renders alongside `page.tsx`, with its own loading/error state. The parent layout receives it as a named prop.

```
app/
└── dashboard/
    ├── layout.tsx
    ├── page.tsx
    ├── @analytics/
    │   ├── page.tsx
    │   └── loading.tsx
    └── @notifications/
        ├── page.tsx
        └── default.tsx
```

```tsx
// app/dashboard/layout.tsx
export default function DashboardLayout({
  children,
  analytics,
  notifications,
}: {
  children: React.ReactNode;
  analytics: React.ReactNode;
  notifications: React.ReactNode;
}) {
  return (
    <div className="grid grid-cols-2 gap-4">
      <div className="col-span-2">{children}</div>
      <div>{analytics}</div>
      <div>{notifications}</div>
    </div>
  );
}
```

Each slot streams independently. They're the structural building block for things like modals, tabs that survive navigation, and split views. `default.tsx` is the fallback rendered when the URL doesn't match the slot.

### 3.5 Intercepting routes

You can "intercept" a route from another route to render it in a different layout context — most famously, Instagram-style photo modals where clicking a thumbnail opens a modal *over the feed* but the URL still becomes `/photo/123` (and a direct visit shows a full page).

Convention: `(.)` matches same level, `(..)` matches one level up, `(...)` matches from root.

```
app/
├── feed/
│   ├── page.tsx
│   └── @modal/
│       ├── (.)photo/[id]/page.tsx   # intercepts /photo/[id] when navigated from /feed
│       └── default.tsx
└── photo/[id]/page.tsx              # the full-page version
```

This is one of the App Router's most powerful and most confusing features. Build it once, you'll never forget it.

### 3.6 Linking and programmatic navigation

```tsx
// Client navigation
import Link from "next/link";

<Link href="/blog/hello" prefetch>Read</Link>;

// Programmatic from client component
"use client";
import { useRouter } from "next/navigation";

const router = useRouter();
router.push("/blog/hello");
router.replace("/login");
router.refresh();   // re-fetch RSC payload for current route
router.back();
```

```tsx
// Programmatic from server component / action
import { redirect } from "next/navigation";
redirect("/login");
```

`useRouter` is from `next/navigation` (App Router), NOT `next/router` (Pages Router). This trips up everyone who learned Next.js before 13.

### 3.7 Reading the URL in a server component

```tsx
// app/search/page.tsx (Next 16)
export default async function SearchPage({
  searchParams,
}: {
  searchParams: Promise<{ q?: string }>;
}) {
  const { q } = await searchParams;
  return <p>You searched for: {q ?? "(nothing)"}</p>;
}
```

`searchParams` makes a page **dynamically rendered** by default (it can't be statically prerendered because the query is unknown at build time). See Module 06 for details.

---

## 4. Practical application — a multi-tenant docs site

A realistic slice: a `/docs/[org]/[...path]` route with a sidebar layout, a 404 for unknown orgs, and a modal-style "version picker" via parallel routes.

```
app/
└── docs/
    ├── layout.tsx
    ├── page.tsx
    ├── [org]/
    │   ├── layout.tsx
    │   ├── @versions/
    │   │   ├── page.tsx
    │   │   └── default.tsx
    │   └── [...path]/
    │       ├── page.tsx
    │       └── not-found.tsx
    └── not-found.tsx
```

```tsx
// app/docs/layout.tsx
export default function DocsRoot({ children }: { children: React.ReactNode }) {
  return <div className="docs-shell">{children}</div>;
}
```

```tsx
// app/docs/page.tsx
import Link from "next/link";
export default function DocsHome() {
  return (
    <div>
      <h1>Docs</h1>
      <ul>
        <li><Link href="/docs/acme">Acme</Link></li>
        <li><Link href="/docs/contoso">Contoso</Link></li>
      </ul>
    </div>
  );
}
```

```tsx
// app/docs/[org]/layout.tsx
import { notFound } from "next/navigation";

const ORGS = new Set(["acme", "contoso"]);

export default async function OrgLayout({
  children,
  versions,
  params,
}: {
  children: React.ReactNode;
  versions: React.ReactNode;
  params: Promise<{ org: string }>;
}) {
  const { org } = await params;
  if (!ORGS.has(org)) notFound();

  return (
    <div className="grid grid-cols-[1fr_220px] gap-6">
      <main>{children}</main>
      <aside>{versions}</aside>
    </div>
  );
}
```

```tsx
// app/docs/[org]/@versions/page.tsx
export default async function VersionsSlot({
  params,
}: {
  params: Promise<{ org: string }>;
}) {
  const { org } = await params;
  const versions = ["v1", "v2", "v3"];
  return (
    <div className="rounded border p-3">
      <h3 className="text-sm font-semibold">{org} versions</h3>
      <ul>
        {versions.map((v) => <li key={v}>{v}</li>)}
      </ul>
    </div>
  );
}
```

```tsx
// app/docs/[org]/@versions/default.tsx
export default function VersionsDefault() {
  return null;
}
```

```tsx
// app/docs/[org]/[...path]/page.tsx
import { notFound } from "next/navigation";

async function loadDoc(org: string, path: string[]) {
  const key = path.join("/");
  // Replace with real DB / CMS:
  const docs: Record<string, string> = { "getting-started": "Welcome", "api/auth": "Auth API" };
  return docs[key];
}

export default async function DocPage({
  params,
}: {
  params: Promise<{ org: string; path: string[] }>;
}) {
  const { org, path } = await params;
  const body = await loadDoc(org, path);
  if (!body) notFound();

  return (
    <article>
      <p className="text-sm text-neutral-500">{org} / {path.join("/")}</p>
      <h1 className="text-2xl font-bold">{path[path.length - 1]}</h1>
      <p className="mt-2">{body}</p>
    </article>
  );
}
```

```tsx
// app/docs/[org]/[...path]/not-found.tsx
export default function NotFound() {
  return <p>Doc not found in this org.</p>;
}
```

Navigate around — observe in DevTools how only the changed segment refetches, and the versions sidebar stays mounted.

---

## 5. Common mistakes & gotchas

### Confusing `layout.tsx` with `template.tsx`
Layouts **persist** across navigations within the same segment subtree. Templates **re-mount** on every navigation. Use templates for per-navigation animations or `useEffect` reset behavior. Default to layouts.

### `useRouter` from the wrong package
Always use `import { useRouter } from "next/navigation";` instead of `next/router`. The latter is Pages Router legacy.

### Forgetting `default.tsx` in parallel routes
If a slot has no match for the current URL, Next looks for `default.tsx` in the slot folder. Without one, you get a hard error on navigation. Always add a `default.tsx` (it can return `null`).

### Using `[slug]` and a static folder of the same name
If `app/blog/[slug]/` and `app/blog/featured/` coexist, `/blog/featured` will match the static folder, not the dynamic one. Static wins.

### `searchParams` and the static cache
Accessing `searchParams` opts the page into dynamic rendering. If you want static rendering, wrap the logic in a Suspense boundary or use Partial Prerendering.

### Async `params` in Next 15 & 16
`params` and `searchParams` are Promises. You must `await` them inside page components.

### Catch-all matches `/`?
`[...slug]` does NOT match the empty path. Use `[[...slug]]` (double brackets) for an optional catch-all that matches both `/docs` and `/docs/a/b/c`.

---

## 🎯 Key Takeaways

- **Folders are segments, files are roles.** Use `page`, `layout`, `loading`, `error`, `not-found`, `route`, `template`, and `default`.
- **Layouts persist; templates remount.**
- **Route groups `(name)` and private folders `_name` shape your code without shaping the URL.**
- **`useRouter` lives in `next/navigation`**, and `params`/`searchParams` are Promises.

*←* [`01_setup_and_app_router.md`](./01_setup_and_app_router.md) *|* *next →* [`03_server_and_client_components.md`](./03_server_and_client_components.md)
