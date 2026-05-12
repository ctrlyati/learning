# 10 — Metadata & SEO

> **Goal:** Use the Metadata API, dynamic metadata, `sitemap.ts`, `robots.ts`, and dynamic OpenGraph images to ship pages that rank, share well, and pass technical SEO audits.

---

## 1. Concept — metadata is just an export

In the App Router, you don't write `<head>` tags by hand. Instead, you export a `metadata` object (or function) from `layout.tsx` / `page.tsx`, and Next.js renders the appropriate tags in `<head>`.

```tsx
// app/layout.tsx
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: {
    default: "Acme",
    template: "%s · Acme",
  },
  description: "An example Next.js app.",
  metadataBase: new URL("https://acme.example.com"),
  openGraph: {
    title: "Acme",
    description: "An example Next.js app.",
    url: "https://acme.example.com",
    siteName: "Acme",
    type: "website",
  },
  twitter: { card: "summary_large_image" },
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return <html><body>{children}</body></html>;
}
```

```tsx
// app/about/page.tsx
export const metadata = { title: "About" };
// Renders: <title>About · Acme</title>
```

The `template: "%s · Acme"` in the root layout means every nested page's `title` is substituted into the template. Saves repetition.

---

## 2. Mechanism — how metadata flows

Next.js walks the layout/page chain, collecting metadata from each segment, and merges them with this rule:

- **Most-deeply nested wins**, except `openGraph` and `twitter`, which **do not deep-merge** — set them where they belong, fully.
- `title` honors the nearest `template`.
- `metadataBase` is required for relative URLs in `openGraph.images`, `alternates.canonical`, etc.

At render time, Next.js emits the corresponding tags inside `<head>`. The work happens **on the server** (RSC), so there's no client-side flash of missing metadata.

### Dynamic metadata with `generateMetadata`

For routes with dynamic params, export an async function:

```tsx
// app/blog/[slug]/page.tsx
import type { Metadata } from "next";
import { getPost } from "@/lib/posts";

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}): Promise<Metadata> {
  const { slug } = await params;
  const post = await getPost(slug);
  if (!post) return { title: "Not found" };
  return {
    title: post.title,
    description: post.excerpt,
    openGraph: {
      title: post.title,
      description: post.excerpt,
      images: [{ url: `/og?slug=${slug}`, width: 1200, height: 630 }],
    },
  };
}
```

Note: the framework dedupes fetches between `generateMetadata` and the page component when they hit the same `fetch`/cached function. Reuse a `cache()`-wrapped helper for the post fetch so you don't query twice.

---

## 3. Variations / depth

### 3.1 `metadataBase` and absolute URLs

```ts
export const metadata = {
  metadataBase: new URL(process.env.NEXT_PUBLIC_SITE_URL ?? "http://localhost:3000"),
};
```

Without `metadataBase`, relative URLs in `openGraph.images` etc. won't resolve, and Next will warn. Set it once in the root layout.

### 3.2 `alternates` and canonicals

```ts
export const metadata: Metadata = {
  alternates: {
    canonical: "/blog/hello-world",
    languages: {
      "en-US": "/blog/hello-world",
      "es-ES": "/es/blog/hello-world",
    },
  },
};
```

Resolved against `metadataBase`. Canonicals are critical for content that appears at multiple URLs (UTM params, pagination).

### 3.3 `robots.ts` and `sitemap.ts`

These are special **route handlers** that produce `robots.txt` and `sitemap.xml`:

```ts
// app/robots.ts
import type { MetadataRoute } from "next";

export default function robots(): MetadataRoute.Robots {
  return {
    rules: [
      { userAgent: "*", allow: "/", disallow: ["/admin", "/api"] },
    ],
    sitemap: "https://acme.example.com/sitemap.xml",
  };
}
```

```ts
// app/sitemap.ts
import type { MetadataRoute } from "next";
import { getPosts } from "@/lib/posts";

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const posts = await getPosts();
  const postEntries = posts.map((p) => ({
    url: `https://acme.example.com/blog/${p.slug}`,
    lastModified: p.publishedAt,
    changeFrequency: "weekly" as const,
    priority: 0.7,
  }));
  return [
    { url: "https://acme.example.com", changeFrequency: "daily", priority: 1 },
    { url: "https://acme.example.com/about", changeFrequency: "monthly", priority: 0.5 },
    ...postEntries,
  ];
}
```

For very large sites, split the sitemap:

```
app/
└── sitemap/
    ├── [id]/
    │   └── route.ts
    └── route.ts   // returns an index
```

### 3.4 Dynamic OG images with `opengraph-image`

Next generates social card images on demand. Place a file:

```tsx
// app/blog/[slug]/opengraph-image.tsx
import { ImageResponse } from "next/og";
import { getPost } from "@/lib/posts";

export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

export default async function OG({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const post = await getPost(slug);
  return new ImageResponse(
    (
      <div style={{
        display: "flex", flexDirection: "column", justifyContent: "center",
        width: "100%", height: "100%", background: "#0f172a", color: "white",
        padding: "60px", fontSize: 64,
      }}>
        <div>{post?.title ?? "Acme"}</div>
        <div style={{ fontSize: 28, color: "#94a3b8", marginTop: 20 }}>
          acme.example.com
        </div>
      </div>
    ),
    size
  );
}
```

`ImageResponse` is from `next/og` — it renders a JSX tree using Satori (server-side SVG renderer) and outputs a PNG. **Restrictions**: only inline `style` (no CSS classes), limited flexbox features, no JS. Read the `next/og` docs carefully.

Once that file exists, the URL `/blog/<slug>/opengraph-image` serves the PNG, and your page's `openGraph.images` will reference it automatically without manually configuring.

You can also add `twitter-image.tsx` similarly.

### 3.5 Verification, icons, manifest

```ts
export const metadata: Metadata = {
  icons: {
    icon: [
      { url: "/favicon.ico", sizes: "any" },
      { url: "/icon.png", type: "image/png" },
    ],
    apple: "/apple-icon.png",
  },
  manifest: "/manifest.webmanifest",
  verification: {
    google: "google-verification-token",
  },
};
```

Or use convention files in `app/`:
- `app/favicon.ico` — auto-detected
- `app/icon.tsx` — generated dynamically via `ImageResponse`
- `app/apple-icon.tsx` — same for Apple touch icon
- `app/manifest.ts` — generates `/manifest.webmanifest`

### 3.6 `noindex` on staging

```ts
// in a layout reachable only on preview
export const metadata: Metadata = {
  robots: { index: false, follow: false },
};
```

Or use the `X-Robots-Tag` HTTP header in middleware for the whole environment:

```ts
// middleware.ts (excerpt)
if (process.env.VERCEL_ENV !== "production") {
  response.headers.set("X-Robots-Tag", "noindex, nofollow");
}
```

### 3.7 Structured data (JSON-LD)

Schema.org JSON-LD goes in the page as a `<script type="application/ld+json">`:

```tsx
// app/products/[id]/page.tsx
export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const product = await getProduct(id);
  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "Product",
    name: product.name,
    description: product.description,
    offers: {
      "@type": "Offer",
      price: (product.priceCents / 100).toFixed(2),
      priceCurrency: "USD",
    },
  };
  return (
    <>
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }} />
      <h1>{product.name}</h1>
      {/* ... */}
    </>
  );
}
```

Google's Rich Results test will pick this up.

---

## 4. Practical application — a blog post with full SEO

```tsx
// app/blog/[slug]/page.tsx
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { cache } from "react";
import { getPost as _getPost } from "@/lib/posts";

const getPost = cache(_getPost);  // dedupe between generateMetadata and the page

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}): Promise<Metadata> {
  const { slug } = await params;
  const post = await getPost(slug);
  if (!post) return { title: "Not found" };

  return {
    title: post.title,
    description: post.excerpt,
    alternates: { canonical: `/blog/${slug}` },
    openGraph: {
      title: post.title,
      description: post.excerpt,
      url: `/blog/${slug}`,
      type: "article",
      publishedTime: post.publishedAt,
      authors: [post.authorName],
    },
    twitter: {
      card: "summary_large_image",
      title: post.title,
      description: post.excerpt,
    },
  };
}

export default async function PostPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const post = await getPost(slug);
  if (!post) notFound();

  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "BlogPosting",
    headline: post.title,
    datePublished: post.publishedAt,
    author: { "@type": "Person", name: post.authorName },
    description: post.excerpt,
  };

  return (
    <article>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }}
      />
      <h1 className="text-3xl font-bold">{post.title}</h1>
      <time className="text-sm text-neutral-500">{post.publishedAt}</time>
      <div className="prose mt-6" dangerouslySetInnerHTML={{ __html: post.html }} />
    </article>
  );
}
```

```tsx
// app/blog/[slug]/opengraph-image.tsx
import { ImageResponse } from "next/og";
import { cache } from "react";
import { getPost as _getPost } from "@/lib/posts";

const getPost = cache(_getPost);
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

export default async function OG({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const post = await getPost(slug);
  const title = post?.title ?? "Acme";

  return new ImageResponse(
    (
      <div style={{
        width: "100%", height: "100%", display: "flex",
        flexDirection: "column", justifyContent: "space-between",
        background: "linear-gradient(135deg, #0f172a, #1e293b)",
        color: "white", padding: 60,
      }}>
        <div style={{ fontSize: 28, color: "#94a3b8" }}>Acme · Blog</div>
        <div style={{ fontSize: 72, fontWeight: 800, lineHeight: 1.1 }}>{title}</div>
        <div style={{ fontSize: 24, color: "#94a3b8" }}>acme.example.com</div>
      </div>
    ),
    size
  );
}
```

```ts
// app/sitemap.ts
import type { MetadataRoute } from "next";
import { getPosts } from "@/lib/posts";

const BASE = "https://acme.example.com";

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const posts = await getPosts();
  return [
    { url: BASE, changeFrequency: "daily", priority: 1 },
    { url: `${BASE}/blog`, changeFrequency: "daily", priority: 0.8 },
    ...posts.map((p) => ({
      url: `${BASE}/blog/${p.slug}`,
      lastModified: p.publishedAt,
      changeFrequency: "weekly" as const,
      priority: 0.6,
    })),
  ];
}
```

```ts
// app/robots.ts
import type { MetadataRoute } from "next";
export default function robots(): MetadataRoute.Robots {
  return {
    rules: [{ userAgent: "*", allow: "/", disallow: ["/api", "/admin"] }],
    sitemap: "https://acme.example.com/sitemap.xml",
  };
}
```

That's a full SEO setup: dynamic per-post metadata, JSON-LD, dynamic OG images, sitemap, robots — under 200 lines.

---

## 5. Common mistakes & gotchas

### Forgetting `metadataBase`

If your OG image URLs don't resolve in Twitter/Slack previews, you forgot `metadataBase`. Set it once in the root layout.

### Setting `openGraph` in the root layout and expecting deep merge

`openGraph` and `twitter` **replace** at each level — they don't merge. If a page sets `openGraph.title` only, the parent's `openGraph.images` is lost. Set the full object where you intend to use it.

### Calling `getPost` twice — once in `generateMetadata`, once in the page

Wrap with `react.cache()` and import the wrapped version in both, or rely on `fetch` request memoization. Otherwise you double-query for every blog post.

### Title not appearing in tab

If you set `title: "X"` but a parent layout has `template: "%s · Acme"`, the rendered title is "X · Acme". This is usually what you want. If you don't want the template, use `title: { absolute: "X" }`.

### `noindex` accidentally in production

Forgetting to remove `robots: { index: false }` from a staging branch before merging. Use env-based logic and verify in production with `curl -s example.com/robots.txt`.

### Huge sitemaps from a single function

`app/sitemap.ts` produces one file capped at ~50k URLs per spec. For larger sites, use the split-sitemap pattern with an index file.

### Dynamic OG images: CSS classes don't work

`next/og` (Satori) only supports inline `style`. No Tailwind classes, no className. Many people copy a Tailwind UI sample and wonder why nothing renders. Convert to inline styles.

### Cached OG images don't update after content edit

OG images get cached aggressively by social platforms (Facebook, Twitter) and by your CDN. Use Facebook's URL Debugger and Twitter's Card Validator to force refresh.

---

## 🎯 Key Takeaways

- **Metadata is a server-side export**, merged top-down. `title` honors `template`; `openGraph` and `twitter` replace, not merge. Set them where they belong.
- **`generateMetadata` enables per-route dynamic SEO**. Cache the data fetch (`react.cache()`) so you don't query twice between metadata and the page.
- **`sitemap.ts`, `robots.ts`, `opengraph-image.tsx`, `icon.tsx`, `manifest.ts`** are convention-based files that produce real assets at the corresponding URLs. No manual setup.
- **`next/og` (`ImageResponse`) renders dynamic OG images via Satori.** Inline styles only, limited CSS subset — read the docs before designing.
- **Add JSON-LD structured data** for content types (Article, Product, FAQ) — it's free SEO mileage and Google's Rich Results Test will validate it instantly.

*←* [`09_image_font_script.md`](./09_image_font_script.md) *|* *next →* [`11_middleware_and_edge.md`](./11_middleware_and_edge.md)
