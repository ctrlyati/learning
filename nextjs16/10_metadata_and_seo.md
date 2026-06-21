# 10 — Metadata & SEO

> **Goal:** Deploy the Metadata API, dynamic metadata hooks, `sitemap.ts`, `robots.ts`, and dynamic OpenGraph image engines to pass technical SEO audits and optimize user sharing click-through rates.

---

## 1. Concept — Declared Exports for Metadata

In Next.js 16, instead of writing raw HTML `<head>` tags, you export a static `metadata` object (or an asynchronous `generateMetadata` function) from your `page.tsx` or `layout.tsx` files. Next.js generates and appends the appropriate metadata tags dynamically.

```tsx
// src/app/layout.tsx
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: {
    default: "Acme Platform",
    template: "%s · Acme Platform", // %s gets replaced by nested segment titles
  },
  description: "Enterprise catalog dashboard built on Next.js 16.",
  metadataBase: new URL("https://acme.example.com"),
  openGraph: {
    title: "Acme Platform",
    description: "Enterprise catalog dashboard built on Next.js 16.",
    url: "https://acme.example.com",
    siteName: "Acme Corp",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
  },
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
```

```tsx
// src/app/about/page.tsx
export const metadata = { title: "About Us" };
// Output title: <title>About Us · Acme Platform</title>
```

---

## 2. Dynamic Metadata via `generateMetadata`

For dynamic paths, export an async `generateMetadata` function.

```tsx
// src/app/blog/[slug]/page.tsx
import type { Metadata } from "next";
import { getPost } from "@/lib/posts"; // Cached post fetcher

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}): Promise<Metadata> {
  const { slug } = await params;
  const post = await getPost(slug);

  if (!post) {
    return { title: "Article Not Found" };
  }

  return {
    title: post.title,
    description: post.excerpt,
    openGraph: {
      title: post.title,
      description: post.excerpt,
      images: [{ url: `/blog/${slug}/opengraph-image`, width: 1200, height: 630 }],
    },
  };
}
```

Next.js automatically deduplicates data fetching calls between `generateMetadata` and the page component render tree if you use the same caching wrappers or identical `fetch` endpoints.

---

## 3. SEO Files & Schema Structured Data

### 3.1 Sitemap & Robots Generation

Create XML sitemaps and robots rules dynamically using built-in file routes.

```typescript
// src/app/robots.ts
import type { MetadataRoute } from "next";

export default function robots(): MetadataRoute.Robots {
  return {
    rules: [
      {
        userAgent: "*",
        allow: "/",
        disallow: ["/admin", "/api"],
      },
    ],
    sitemap: "https://acme.example.com/sitemap.xml",
  };
}
```

```typescript
// src/app/sitemap.ts
import type { MetadataRoute } from "next";
import { getPosts } from "@/lib/posts";

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const posts = await getPosts();
  
  const blogUrls = posts.map((post) => ({
    url: `https://acme.example.com/blog/${post.slug}`,
    lastModified: post.updatedAt,
    changeFrequency: "weekly" as const,
    priority: 0.7,
  }));

  return [
    { url: "https://acme.example.com", changeFrequency: "daily", priority: 1.0 },
    { url: "https://acme.example.com/about", changeFrequency: "monthly", priority: 0.5 },
    ...blogUrls,
  ];
}
```

### 3.2 Dynamic Social OpenGraph (OG) Images

Create customized social sharing graphics using `ImageResponse` (which compiles JSX to PNG assets via Satori under the hood):

```tsx
// src/app/blog/[slug]/opengraph-image.tsx
import { ImageResponse } from "next/og";
import { getPost } from "@/lib/posts";

export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

export default async function OG({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const post = await getPost(slug);
  const title = post?.title ?? "Acme Platform";

  return new ImageResponse(
    (
      <div style={{
        width: "100%", height: "100%", display: "flex",
        flexDirection: "column", justifyContent: "space-between",
        background: "linear-gradient(135deg, #0f172a, #1e293b)",
        color: "white", padding: 60,
      }}>
        <div style={{ fontSize: 24, color: "#94a3b8" }}>Acme Platform · Blog</div>
        <div style={{ fontSize: 64, fontWeight: 800 }}>{title}</div>
        <div style={{ fontSize: 20, color: "#64748b" }}>acme.example.com</div>
      </div>
    ),
    size
  );
}
```

*Note: Satori (the OG engine) requires inline CSS `style` properties and only supports a subset of CSS rules (no Tailwind utility classes).*

---

## 4. Structured Data (JSON-LD)

Help search engines parse indexable page data (like products, reviews, or news posts) by embedding Schema-compliant JSON-LD markup:

```tsx
// src/app/blog/[slug]/page.tsx
import { notFound } from "next/navigation";
import { getPost } from "@/lib/posts";

export default async function PostPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const post = await getPost(slug);
  if (!post) notFound();

  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "BlogPosting",
    headline: post.title,
    description: post.excerpt,
    datePublished: post.publishedAt,
    author: {
      "@type": "Person",
      name: post.authorName,
    },
  };

  return (
    <article>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }}
      />
      <h1>{post.title}</h1>
      <p>{post.body}</p>
    </article>
  );
}
```

---

## 5. Common Mistakes & Gotchas

### Absolute URL Errors
Relative URLs passed inside `openGraph.images` will throw errors during validation unless you define a root **`metadataBase`** in your base layout.

### OpenGraph Merging Behavior
Unlike standard fields, nested segments **replace** parent `openGraph` and `twitter` properties completely rather than deep-merging. If a child segment specifies only `openGraph.title`, all parent images or configurations are discarded for that route. Define complete OG objects at the route segments where you intend them to be read.

---

## 🎯 Key Takeaways

- **Merge logic:** Child layout metadata overrides parent objects.
- **Dynamic generation:** Use `generateMetadata` for parametrized routes.
- **Convention over configuration:** Standard filenames like `sitemap.ts`, `robots.ts`, and `opengraph-image.tsx` generate SEO assets automatically.
- **JSON-LD:** Inject structured schemas into pages to support rich search indexing.

*←* [`09_image_font_script.md`](./09_image_font_script.md) *|* *next →* [`11_proxy_and_edge.md`](./11_proxy_and_edge.md)
