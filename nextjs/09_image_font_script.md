# 09 — Image, Font & Script Optimization

> **Goal:** Use `next/image`, `next/font`, and `next/script` correctly so Core Web Vitals (LCP, CLS, INP) stay green without manual tuning.

---

## 1. Concept — three components that earn their keep

Next.js ships three "optimization" components that do non-trivial work behind the scenes:

- **`<Image>`** (`next/image`) — automatic resizing, AVIF/WebP, lazy loading, sized to prevent CLS.
- **`next/font`** — self-host Google/local fonts at build, eliminate FOUT, no extra HTTP request.
- **`<Script>`** (`next/script`) — load third-party scripts with strategy control to keep them off the critical path.

```tsx
// app/page.tsx
import Image from "next/image";
import { Inter } from "next/font/google";
import hero from "./hero.jpg"; // imported -> dimensions known at build

const inter = Inter({ subsets: ["latin"] });

export default function Home() {
  return (
    <main className={inter.className}>
      <Image src={hero} alt="A nice hero" priority placeholder="blur" />
      <h1>Hello</h1>
    </main>
  );
}
```

That's a fast page: the image is sized at build time, the font is bundled with the CSS, and there are no third-party requests on the critical path. You can copy that to a marketing page and ship.

---

## 2. Mechanism — why each one matters

### 2.1 `next/image`

`next/image` renders an `<img>` wrapped in proper sizing markup, and:

- Generates multiple sizes on-demand (configurable in `next.config`), serves the right one via `srcset`.
- Converts to modern formats (AVIF, then WebP) when supported.
- Adds `loading="lazy"` by default, with `loading="eager"` + `fetchpriority="high"` when `priority` is set.
- Reserves space using `width`/`height` (preventing CLS) — required for remote images; auto-inferred from imported local images.

The optimization endpoint is `/_next/image?url=...&w=...&q=...`. On Vercel it runs on their image-optimization workers; self-hosted, you'll need either the built-in optimizer (uses `sharp`) or a remote loader.

### 2.2 `next/font`

`next/font/google` downloads Google Fonts **at build time**, hosts them on your origin, and emits CSS that uses `font-display: swap` plus a CSS `size-adjust` declaration to match the fallback metrics. This:

- Eliminates the third-party request to `fonts.googleapis.com` (privacy + performance).
- Avoids FOUT/CLS via metric matching.
- Subsets to the characters you need (`subsets: ["latin"]`).

`next/font/local` works the same way with files you ship.

### 2.3 `next/script`

`<Script>` lets you choose *when* a third-party script loads, with explicit strategies:

| `strategy`         | When it loads                                   | Use for                          |
|--------------------|-------------------------------------------------|----------------------------------|
| `beforeInteractive`| Before page becomes interactive                 | Rare — bootstrappers, polyfills  |
| `afterInteractive` (default) | After hydration                       | Most analytics, chat widgets     |
| `lazyOnload`       | After full page load                            | Heavy widgets the user may not see |
| `worker`           | In a web worker (experimental, Partytown)       | Heavy analytics                  |

Crucially, `<Script>` deduplicates: rendering the same `<Script id="x">` twice (e.g., across navigations) only loads it once.

---

## 3. Variations / depth

### 3.1 Local vs remote images

```tsx
// Local image (imported) — dimensions known, blur placeholder free
import hero from "./hero.jpg";
<Image src={hero} alt="…" placeholder="blur" />

// Remote image — must declare width/height (or fill) and allow the host
<Image src="https://images.unsplash.com/photo-…" alt="…" width={1200} height={630} />
```

Remote hosts must be allow-listed in `next.config.mjs`:

```js
const nextConfig = {
  images: {
    remotePatterns: [{ protocol: "https", hostname: "images.unsplash.com" }],
  },
};
```

### 3.2 `sizes` and responsive images

For an image that fills a responsive container, you need `sizes`:

```tsx
<Image
  src={hero}
  alt="…"
  sizes="(min-width: 1024px) 1024px, 100vw"
  className="w-full h-auto"
/>
```

`sizes` tells the browser which `srcset` candidate to pick. Skipping it on responsive images wastes bandwidth.

### 3.3 `priority` and LCP

Mark exactly **one** above-the-fold image per page as `priority`. It removes lazy loading and signals `fetchpriority="high"`. Setting it on multiple images doesn't speed anything up — it just deprioritizes them all.

### 3.4 `fill` for unknown-aspect images

```tsx
<div className="relative aspect-video">
  <Image src={url} alt="…" fill className="object-cover" />
</div>
```

`fill` makes the image expand to its parent. The parent must be `position: relative` (or absolute/fixed) and have a sized aspect.

### 3.5 Fonts with CSS variables

For Tailwind integration:

```tsx
// app/layout.tsx
import { Inter, JetBrains_Mono } from "next/font/google";

const sans = Inter({ subsets: ["latin"], variable: "--font-sans" });
const mono = JetBrains_Mono({ subsets: ["latin"], variable: "--font-mono" });

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${sans.variable} ${mono.variable}`}>
      <body>{children}</body>
    </html>
  );
}
```

```ts
// tailwind.config.ts
theme: {
  extend: {
    fontFamily: {
      sans: ["var(--font-sans)", "ui-sans-serif", "system-ui"],
      mono: ["var(--font-mono)", "ui-monospace"],
    },
  },
},
```

Now `font-sans` and `font-mono` use your self-hosted fonts with system fallbacks.

### 3.6 Local fonts

```tsx
// app/layout.tsx
import localFont from "next/font/local";

const display = localFont({
  src: [
    { path: "./fonts/Display-Regular.woff2", weight: "400" },
    { path: "./fonts/Display-Bold.woff2", weight: "700" },
  ],
  variable: "--font-display",
  display: "swap",
});
```

### 3.7 Loading analytics

```tsx
// app/layout.tsx
import Script from "next/script";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html><body>
      {children}
      <Script
        src="https://www.googletagmanager.com/gtag/js?id=GA_MEASUREMENT_ID"
        strategy="afterInteractive"
      />
      <Script id="ga-init" strategy="afterInteractive">{`
        window.dataLayer = window.dataLayer || [];
        function gtag(){dataLayer.push(arguments);}
        gtag('js', new Date());
        gtag('config', 'GA_MEASUREMENT_ID');
      `}</Script>
    </body></html>
  );
}
```

For Vercel hosting, prefer `@vercel/analytics` and `@vercel/speed-insights` — they're tuned for the platform and don't need GTM.

### 3.8 Self-hosted image optimizer

If you're self-hosting Next.js, the default loader uses `sharp`. Make sure it's installed:

```bash
pnpm add sharp
```

For very high traffic, point `next/image` to an external image CDN with a custom loader:

```js
// next.config.mjs
const nextConfig = {
  images: {
    loader: "custom",
    loaderFile: "./image-loader.ts",
  },
};
```

```ts
// image-loader.ts
export default function loader({ src, width, quality }: { src: string; width: number; quality?: number }) {
  return `https://cdn.example.com/${src}?w=${width}&q=${quality ?? 75}`;
}
```

---

## 4. Practical application — a marketing hero with all three primitives

```tsx
// app/layout.tsx
import { Inter, JetBrains_Mono } from "next/font/google";
import "./globals.css";

const sans = Inter({ subsets: ["latin"], variable: "--font-sans", display: "swap" });
const mono = JetBrains_Mono({ subsets: ["latin"], variable: "--font-mono", display: "swap" });

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${sans.variable} ${mono.variable}`}>
      <body className="font-sans antialiased">{children}</body>
    </html>
  );
}
```

```tsx
// app/page.tsx
import Image from "next/image";
import Script from "next/script";
import hero from "./hero.jpg";

export default function Home() {
  return (
    <main>
      <section className="relative">
        <div className="relative aspect-[16/9] w-full">
          <Image
            src={hero}
            alt="Product hero"
            fill
            sizes="100vw"
            className="object-cover"
            placeholder="blur"
            priority
          />
        </div>
        <div className="absolute inset-0 flex flex-col items-center justify-center text-white">
          <h1 className="text-4xl font-bold drop-shadow">Welcome</h1>
          <p className="mt-2 font-mono text-sm opacity-80">code/welcome.tsx</p>
        </div>
      </section>

      <section className="mx-auto max-w-3xl p-6">
        <h2 className="text-2xl font-semibold">Features</h2>
        <FeatureGrid />
      </section>

      {/* Analytics, deferred */}
      <Script
        src="https://plausible.io/js/plausible.js"
        data-domain="example.com"
        strategy="afterInteractive"
      />
    </main>
  );
}

function FeatureGrid() {
  return (
    <ul className="mt-4 grid grid-cols-3 gap-4">
      {["Fast", "Secure", "Open"].map((f) => (
        <li key={f} className="rounded border p-4">
          <h3 className="font-semibold">{f}</h3>
        </li>
      ))}
    </ul>
  );
}
```

What you get with no manual perf tuning:

- **LCP**: the hero is `priority`, sized at build, served as AVIF/WebP at the right resolution. Sub-1.5s on a fast connection.
- **CLS**: image dimensions reserved space; fonts metric-matched. Layout doesn't shift.
- **INP**: no analytics on the critical path; everything that can defer, does.

Run Lighthouse on this and you're typically at 95+ on perf without lifting a finger.

---

## 5. Common mistakes & gotchas

### `<img>` instead of `<Image>` for "just a logo"

Even tiny images benefit — they get AVIF, get lazy-loaded, get blur placeholders. The boilerplate cost of `<Image>` is minimal. Default to it; opt out only for SVG (where it provides nothing and adds wrapper markup).

### Missing `width`/`height` on remote images

Without them, the browser doesn't know the aspect, and CLS spikes. For remote, always pass `width` + `height` (or `fill` with a sized parent).

### `priority` on three images

Only one image per page should be `priority` — your LCP candidate. More than one defeats the purpose.

### Forgetting `sizes` on responsive images

A 4K image gets served to a phone — wasting bandwidth. Pair `fill` or fluid layouts with a `sizes` value.

### Hot-linking remote images without allow-listing

Throws a hard error in dev/build. Add the hostname to `images.remotePatterns`. For untrusted user-uploaded images, you'll want stricter validation (size limits, content sniffing).

### Fonts: too many subsets / weights

Each subset and weight inflates the CSS payload. Only load what you use. For Latin-only English sites, `subsets: ["latin"]` and 2–3 weights is plenty.

### Self-hosted image optimization at huge scale

The built-in optimizer works fine until you serve millions of images. At that point, you want a dedicated image CDN (Cloudflare Images, Imgix, Cloudinary) via a custom loader — the `sharp`-based optimizer wasn't built for that volume.

### `<Script>` with inline content but no `id`

If you use inline script content (`<Script>{`...`}</Script>`), provide an `id` — Next uses it for deduplication. Forgetting one logs a warning in dev.

### Loading scripts with `beforeInteractive` carelessly

`beforeInteractive` blocks page interactivity. Only use for genuine polyfills or critical bootstrappers. Default to `afterInteractive`.

### `next/image` on raw SVGs

SVGs don't need raster optimization. Either import them as React components (with `@svgr/webpack`) or serve them statically. `<Image>` will work but adds overhead for nothing.

---

## 🎯 Key Takeaways

- **`<Image>` is non-optional for raster images.** AVIF/WebP, lazy loading, CLS prevention, all free. Mark the LCP image `priority` and supply `sizes` for responsive layouts.
- **`next/font` self-hosts Google fonts at build**, eliminating third-party requests and avoiding FOUT/CLS via metric matching. Expose them as CSS variables for Tailwind.
- **`<Script>` strategies control the timing of third-party JS.** Default to `afterInteractive`; reach for `beforeInteractive` only for true polyfills.
- **Core Web Vitals are mostly about discipline, not magic.** Following the defaults of these three primitives gets you a solid Lighthouse score with zero perf wizardry.
- **Allow-list image hosts in `next.config.mjs`.** A common cause of "image works locally but breaks in prod" reports.

*←* [`08_styling.md`](./08_styling.md) *|* *next →* [`10_metadata_and_seo.md`](./10_metadata_and_seo.md)
