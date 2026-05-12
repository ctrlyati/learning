# 16 — Performance & Observability

> **Goal:** Measure what matters in Next.js (bundle size, Web Vitals, RSC payload, server timing), debug regressions with the bundle analyzer and the instrumentation API, and ship dashboards you can actually act on.

---

## 1. Concept — measure first, optimize second

Next.js gives you several built-in instruments. The four you'll use most:

- **`next build` output** — first-load JS, route mode, sitemap of cached vs dynamic.
- **`@next/bundle-analyzer`** — visualize what's in every chunk.
- **`@vercel/speed-insights` / Web Vitals** — real-user metrics in production.
- **Instrumentation API (`instrumentation.ts`)** — hook into the runtime for tracing/error reporting.

Start every perf investigation with a build:

```bash
pnpm build
```

```
Route (app)                              Size     First Load JS
┌ ○ /                                    1.2 kB   88 kB
├ ○ /about                               420 B    87 kB
├ ƒ /dashboard                           4.1 kB   124 kB
└ ● /blog/[slug]                         2.0 kB   92 kB

○  Static    ●  SSG    ƒ  Dynamic
```

If `/dashboard` shows 124 kB first-load JS, you have a starting point. Don't speculate — open the analyzer and find the culprit.

---

## 2. Mechanism — what numbers come from where

### 2.1 First Load JS

The total minified JS the browser parses on initial load for that route. Includes:
- React runtime,
- Framework runtime (Next.js client),
- Shared chunks,
- Route-specific JS (client components on that route).

The framework's baseline is typically ~80 kB. Anything above that is *your* JS.

### 2.2 RSC payload size

The RSC payload is the serialized component tree streamed to the client for hydration / navigation. View its size in DevTools Network tab — look for the `?_rsc=...` request on soft navigations. A big RSC payload means you're sending too many props (or too much text) to client islands.

### 2.3 Web Vitals (LCP, INP, CLS)

| Metric | What it measures               | Good (p75) |
|--------|-------------------------------|------------|
| LCP    | Largest Contentful Paint       | < 2.5s     |
| INP    | Interaction to Next Paint      | < 200ms    |
| CLS    | Cumulative Layout Shift        | < 0.1      |
| TTFB   | Time to First Byte             | < 800ms    |
| FCP    | First Contentful Paint         | < 1.8s     |

These are *user-perceived* metrics. Lab tools (Lighthouse) approximate them; only real-user monitoring (RUM) gives p75.

### 2.4 Server timing

Add `Server-Timing` headers from route handlers/middleware so DevTools shows breakdowns:

```ts
return new NextResponse(body, {
  headers: { "Server-Timing": "db;dur=42, render;dur=87" },
});
```

---

## 3. Variations / depth

### 3.1 Bundle analyzer

```bash
pnpm add -D @next/bundle-analyzer
```

```js
// next.config.mjs
import withBundleAnalyzer from "@next/bundle-analyzer";
const analyze = withBundleAnalyzer({ enabled: process.env.ANALYZE === "true" });
export default analyze({ /* your nextConfig */ });
```

```bash
ANALYZE=true pnpm build
```

Two HTML reports open (client + edge). Look for:
- Surprisingly large dependencies (e.g., moment, lodash full).
- Duplicate copies of the same library (yarn/pnpm sometimes ships two).
- Polyfills you don't need.

Common wins:

- Replace `moment` with `date-fns` or `dayjs` (saves 60+ kB).
- Use `lodash-es` and tree-shake, or per-function imports (`lodash/debounce`).
- Lazy-load heavy client components with `next/dynamic`.

### 3.2 `next/dynamic`

```tsx
import dynamic from "next/dynamic";

const HeavyChart = dynamic(() => import("./HeavyChart"), {
  loading: () => <p>Loading chart…</p>,
  ssr: false,   // if it needs window
});
```

`ssr: false` for components that *only* work in the browser (Mapbox, charting libs with canvas, etc.). This omits them from the SSR bundle entirely.

### 3.3 Tracking Web Vitals

For Vercel: just install `@vercel/speed-insights`:

```tsx
// app/layout.tsx
import { SpeedInsights } from "@vercel/speed-insights/next";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return <html><body>{children}<SpeedInsights /></body></html>;
}
```

For self-hosted, use the `useReportWebVitals` hook:

```tsx
// app/web-vitals.tsx
"use client";
import { useReportWebVitals } from "next/web-vitals";

export function WebVitals() {
  useReportWebVitals((metric) => {
    // POST to your collector
    navigator.sendBeacon(
      "/api/vitals",
      JSON.stringify({ ...metric, ts: Date.now() })
    );
  });
  return null;
}
```

Then create `/api/vitals/route.ts` to ingest.

### 3.4 Instrumentation API

For server-side tracing and error reporting:

```ts
// instrumentation.ts (at the root)
export async function register() {
  if (process.env.NEXT_RUNTIME === "nodejs") {
    await import("./instrumentation.node");
  }
  if (process.env.NEXT_RUNTIME === "edge") {
    await import("./instrumentation.edge");
  }
}
```

```ts
// instrumentation.node.ts
import * as Sentry from "@sentry/nextjs";
Sentry.init({
  dsn: process.env.SENTRY_DSN,
  tracesSampleRate: 0.1,
});
```

Sentry's Next.js SDK has automatic integration; you mostly just install and configure DSN.

For OpenTelemetry:

```ts
// instrumentation.node.ts
import { NodeSDK } from "@opentelemetry/sdk-node";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http";

const sdk = new NodeSDK({
  traceExporter: new OTLPTraceExporter({ url: process.env.OTEL_EXPORTER_OTLP_ENDPOINT }),
});
sdk.start();
```

You'll get spans for every server component render, route handler, and outgoing fetch.

### 3.5 The `onRequestError` hook

```ts
// instrumentation.ts
export async function onRequestError(
  err: Error,
  request: { path: string; method: string },
  context: { routerKind: "Pages Router" | "App Router"; routePath: string; routeType: "render" | "route" | "action" | "middleware" }
) {
  console.error("Request error", err, request, context);
  // ship to Sentry / Datadog
}
```

Centralized error capture without scattering try/catches.

### 3.6 Reading the RSC payload size

Open DevTools Network, navigate within your app, find the document/fetch with `RSC` in the response or `?_rsc=1` query. The transferred size is the RSC payload. If it's > 100 kB on a typical page, you're sending too much data — either too many props to client components, or huge JSON across the boundary.

### 3.7 Caching and CDN observation

For Vercel, the dashboard shows cache HIT/MISS per route. For self-hosted, log the `x-vercel-cache` (or your CDN's) header to your access logs. A surprise MISS on a "static" page tells you something flipped the route to dynamic.

### 3.8 Memory and CPU profiling in dev

Run with `--inspect`:

```bash
NODE_OPTIONS='--inspect' pnpm dev
```

Chrome → `chrome://inspect` → attach. Take heap snapshots, profile a slow render.

For prod, your APM (Datadog, New Relic, Sentry Profiler) covers this.

---

## 4. Practical application — diagnosing a slow page

Suppose `/dashboard` is slow. The investigation flow:

**Step 1 — Look at `next build`:**

```
ƒ /dashboard            10.4 kB    240 kB
```

240 kB First Load JS is high. The `ƒ` confirms it's dynamic.

**Step 2 — Bundle analyzer:**

```bash
ANALYZE=true pnpm build
```

The chart shows: `recharts` (62 kB), `react-icons` (28 kB), `lodash` (22 kB). The recharts component is only on the dashboard.

**Step 3 — Defer heavy components:**

```tsx
// app/dashboard/page.tsx
import dynamic from "next/dynamic";
const RevenueChart = dynamic(() => import("./RevenueChart"), {
  ssr: false,
  loading: () => <div className="h-64 animate-pulse rounded bg-neutral-100" />,
});
```

Now `recharts` only loads when the dashboard renders (and only on the client, after hydration). Subsequent visits to other routes don't pay the cost.

**Step 4 — Trim `react-icons`:**

```ts
// Before — imports the whole package
import { FiHome, FiUser } from "react-icons/fi";

// After — per-icon imports tree-shake
// (react-icons supports this via its module exports)
```

Or switch to `lucide-react`, which is tree-shake-friendly out of the box.

**Step 5 — Use `lodash-es`:**

```ts
import { debounce } from "lodash-es";   // tree-shakes
// Not: import _ from "lodash"
```

**Step 6 — Add server timing:**

```ts
// route or RSC
const start = performance.now();
const data = await getDashboardData();
const dur = (performance.now() - start).toFixed(0);
// pass to a wrapper that sets Server-Timing
```

**Step 7 — Verify with real users:**

```tsx
import { SpeedInsights } from "@vercel/speed-insights/next";
```

After deploy, watch p75 LCP/INP drop in Speed Insights. Build is forecast; RUM is truth.

**Step 8 — Set up an error pipeline:**

```ts
// instrumentation.ts
export async function onRequestError(err, request, context) {
  if (process.env.NODE_ENV === "production") {
    fetch(process.env.ERROR_WEBHOOK!, {
      method: "POST",
      body: JSON.stringify({ err: String(err), request, context }),
    });
  }
}
```

Now any unhandled error in any RSC, route, or action lands in your alerting channel with route and method.

---

## 5. Common mistakes & gotchas

### Optimizing without measuring

Lazy-loading a 2 kB component "to be safe" adds complexity for no benefit. Measure first. Optimize what shows up on the report.

### Confusing Lighthouse with reality

Lighthouse runs on a simulated mobile with throttled CPU; real users are faster (or slower) than that. Use it for relative comparisons; use RUM for absolute numbers.

### Forgetting `ssr: false` on browser-only components

```tsx
// blows up at SSR — `window` is undefined
import Map from "./Map";
```

```tsx
const Map = dynamic(() => import("./Map"), { ssr: false });
```

### Importing barrels

```ts
import { Button } from "@my-org/ui";   // could pull in everything
```

If `@my-org/ui` has a barrel `index.ts` that re-exports everything, you might include the whole library. Use per-file imports (`@my-org/ui/button`) or rely on `optimizePackageImports`:

```js
// next.config.mjs
experimental: { optimizePackageImports: ["@my-org/ui", "lucide-react"] }
```

Next will tree-shake the barrel for you.

### Large server-side fetches blocking the route

A 3-second DB query in a top-level RSC blocks the entire page. Wrap with `<Suspense>` so the shell streams; the slow part fills in.

### Heavy logging in production

`console.log` is cheap individually, but in a hot path multiplies. Use a structured logger with sampling.

### Ignoring CLS

CLS is easy to hit a green on if you size every image (`next/image`), reserve space for fonts (`next/font`), and avoid late-injected ads/widgets. If CLS spikes, it's almost always one of these three.

### `useReportWebVitals` mounted on the wrong route

If you only mount it inside `app/dashboard/layout.tsx`, you only collect vitals on `/dashboard/*`. Put it in `app/layout.tsx` for full coverage.

### Trace sample rate too high in prod

100% sampling kills your tracing bill and your tail latency. 1-10% is typical.

---

## 🎯 Key Takeaways

- **`next build` output is your starting point.** First Load JS and the route legend (`○ ● ƒ`) tell you what shipped and how it renders. Read it after every meaningful change.
- **Bundle analyzer + `next/dynamic`** is the canonical client-bundle optimization loop. Lazy-load heavy charts/maps; tree-shake barrels via `optimizePackageImports`.
- **Web Vitals are the contract.** Use `@vercel/speed-insights` or `useReportWebVitals` to collect real-user metrics. p75 LCP < 2.5s, INP < 200ms, CLS < 0.1.
- **`instrumentation.ts` is the single hook** for OpenTelemetry, Sentry, and `onRequestError`. Set it up once on day one, not on day one hundred.
- **Measure before you optimize; verify after.** Forecasted improvements (`next build`) and observed improvements (RUM) often disagree — RUM wins.

*←* [`15_testing.md`](./15_testing.md) *|* *next →* [`17_deployment_and_production.md`](./17_deployment_and_production.md)
