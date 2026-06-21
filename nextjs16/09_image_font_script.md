# 09 — Image, Font & Script Optimization

> **Goal:** Optimize site assets using `next/image`, `next/font`, and `next/script` to maintain green Core Web Vitals (LCP, CLS, INP) automatically.

---

## 1. Concept — The Optimization Triad

Next.js provides three core components that automate performance tuning behind the scenes:

- **`<Image>`** (`next/image`) — Automates responsive resizing, format conversion (AVIF/WebP), lazy loading, and reserves size to prevent layout shifts (CLS).
- **`next/font`** — Downloads Google or local fonts at build time, hosts them locally, and uses font-metric matching to eliminate flash of unstyled text (FOUT).
- **`<Script>`** (`next/script`) — Controls script loading behavior and sequencing to prevent heavy third-party code from blocking interactive elements.

```tsx
// src/app/page.tsx
import Image from "next/image";
import { Inter } from "next/font/google";
import heroImage from "./hero.jpg"; // dimensions calculated at compile time

const inter = Inter({ subsets: ["latin"] });

export default function Home() {
  return (
    <main className={inter.className}>
      <Image src={heroImage} alt="Marketing Hero" priority placeholder="blur" />
      <h1>Hello, optimized world!</h1>
    </main>
  );
}
```

---

## 2. Deep Dive: Mechanism & Usage

### 2.1 `next/image`

`next/image` extends standard HTML `<img>` elements with these capabilities:

- **Modern Formats:** Re-encodes images to AVIF or WebP formats based on browser support.
- **Sizing Requirements:** Requires explicit `width` and `height` properties for remote images (or uses the `fill` layout) to reserve space and prevent Layout Shifts (CLS).
- **Lazy Loading:** Adds `loading="lazy"` by default. Images marked with `priority` override this behavior to load instantly (`fetchpriority="high"`).

If you are self-hosting Next.js, ensure you install `sharp` to handle high-performance image resizing:

```bash
pnpm add sharp
```

For external CDNs or heavy image loads, you can declare custom image loaders in `next.config.ts`:

```typescript
// next.config.ts
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  images: {
    loader: "custom",
    loaderFile: "./src/image-loader.ts",
  },
};
export default nextConfig;
```

```typescript
// src/image-loader.ts
export default function imageLoader({ src, width, quality }: { src: string; width: number; quality?: number }) {
  return `https://cdn.example.com/${src}?w=${width}&q=${quality ?? 75}`;
}
```

### 2.2 `next/font`

`next/font/google` fetches font files at build time and embeds them into your application bundles, removing external HTTP roundtrips to `fonts.googleapis.com` at request time.

To integrate with Tailwind CSS v4, define variable subsets inside your root layout and map them in your global CSS:

```tsx
// src/app/layout.tsx
import { Inter, JetBrains_Mono } from "next/font/google";
import "./globals.css";

const sans = Inter({ subsets: ["latin"], variable: "--font-sans" });
const mono = JetBrains_Mono({ subsets: ["latin"], variable: "--font-mono" });

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${sans.variable} ${mono.variable}`}>
      <body className="font-sans antialiased">{children}</body>
    </html>
  );
}
```

Now override families natively within your Tailwind CSS v4 theme:

```css
/* src/app/globals.css */
@import "tailwindcss";

@theme {
  --font-sans: var(--font-sans), ui-sans-serif, system-ui;
  --font-mono: var(--font-mono), ui-monospace;
}
```

### 2.3 `next/script`

`<Script>` coordinates third-party javascript tags using these strategies:

| Strategy | Behavior | Ideal Use Cases |
| :--- | :--- | :--- |
| `afterInteractive` (Default) | Injected after hydration. | Google Analytics, Hotjar tracking |
| `lazyOnload` | Loaded during browser idle times. | Support chat widgets, maps |
| `beforeInteractive` | Injected before hydration. | Critical runtime polyfills |

*Note: Always provide a unique `id` when embedding inline scripts to ensure Next.js can deduplicate them.*

```tsx
import Script from "next/script";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html>
      <body>
        {children}
        <Script
          id="custom-tracker"
          strategy="afterInteractive"
          src="https://example.com/tracker.js"
        />
      </body>
    </html>
  );
}
```

---

## 3. Remote Image Allow-listing

To render images hosted on external servers, allow-list their domains in `next.config.ts`:

```typescript
// next.config.ts
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  images: {
    remotePatterns: [
      {
        protocol: "https",
        hostname: "images.unsplash.com",
      },
    ],
  },
};
export default nextConfig;
```

---

## 4. Practical Application — Hero & Layout Setup

Here is a typical layout structure utilizing these three components:

```tsx
// src/app/page.tsx
import Image from "next/image";
import Script from "next/script";
import bannerImage from "./banner.jpg";

export default function HomePage() {
  return (
    <main>
      <section className="relative w-full aspect-[16/9]">
        {/* Full layout image with responsive sizes */}
        <Image
          src={bannerImage}
          alt="Campaign Hero Banner"
          fill
          sizes="100vw"
          className="object-cover"
          placeholder="blur"
          priority
        />
        <div className="absolute inset-0 flex items-center justify-center bg-black/40 text-white">
          <h1 className="text-3xl font-bold">Unlocking Premium Performance</h1>
        </div>
      </section>

      {/* Heavy third-party script deferred to browser idle time */}
      <Script
        id="help-widget"
        src="https://example.com/help.js"
        strategy="lazyOnload"
      />
    </main>
  );
}
```

---

## 5. Common Mistakes & Gotchas

### Forgetting the `sizes` Attribute on Fluid Images
When using `fill` on responsive container elements, always supply a `sizes` attribute (e.g. `sizes="(max-width: 768px) 100vw, 50vw"`). Otherwise, Next.js generates massive source candidates, and desktop browsers may pull 4K image resolutions for mobile layout viewport sizes.

### Excess Font Subsets and Weights
Each Google Font weight and character subset (e.g. Greek, Cyrillic) compiled into your build increases your CSS bundle size. Select only the weights you intend to use.

### Inline Script Errors
If you run inline code inside a `<Script>` tag, you **must** supply an `id` prop so the Next.js runtime can register it.

```tsx
// WRONG
<Script>{`console.log('hello')`}</Script>

// CORRECT
<Script id="debug-log">{`console.log('hello')`}</Script>
```

---

## 🎯 Key Takeaways

- **Default to `<Image>`:** Avoid raw native `<img>` tags to ensure you benefit from compression, lazy loading, and dimension reservation.
- **Self-host with `next/font`:** Avoid fetching fonts from Google's servers at runtime; download and pack them in your build assets.
- **Deferred scripts:** Keep third-party scripts off the critical path using `afterInteractive` or `lazyOnload` script loading strategies.

*←* [`08_styling.md`](./08_styling.md) *|* *next →* [`10_metadata_and_seo.md`](./10_metadata_and_seo.md)
