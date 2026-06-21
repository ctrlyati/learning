# 16 — Performance & Observability

> **Goal:** Analyze and optimize Next.js 16 performance metrics — managing bundle weights, monitoring Web Vitals, parsing RSC payloads, and implementing server telemetry using the instrumentation hooks.

---

## 1. Concept — Performance Metrics

Next.js provides several telemetry points:

- **Build Summary Output:** Visualizes package weights and route caching categories (`Static`, `Dynamic`, `PPR`).
- **Turbopack Bundle Analyzer:** Visualizes package distribution and chunk weight maps.
- **RUM Metrics:** Logs Core Web Vitals (LCP, CLS, INP) directly from real-world visits using `@vercel/speed-insights`.
- **Instrumentation Hook (`instrumentation.ts`):** Hook into server-side runtime operations and handle centralized error tracking.

Every performance review should start with a build:

```bash
pnpm build
```

This logs a size legend:

```
Route (app)                              Size     First Load JS
┌ ○ /                                    1.2 kB   88 kB
├ ○ /about                               420 B    87 kB
├ ƒ /dashboard                           4.1 kB   124 kB
└ ● /blog/[slug]                         2.0 kB   92 kB

○  Static    ●  SSG    ƒ  Dynamic
```

Any route exceeding the ~80 kB baseline is loading user-imported modules that should be audited or lazy-loaded.

---

## 2. Telemetry Primaries

### 2.1 First Load JS
The total minified Javascript footprint loaded on page entry (framework engine, React runtime, and segment-specific code). Keeping this small directly improves **Interaction to Next Paint (INP)**.

### 2.2 Server Component Serialization Speed (Next.js 16.2 Upgrade)
Next.js 16 (specifically 16.2) contains updates to React's rendering engine that speed up Server Component payload deserialization by up to **350%**, ensuring that dynamic client hydration completes quickly.

### 2.3 Core Web Vitals (RUM targets)
Verify performance targets against real-user monitoring (RUM) values:

| Metric | Measurement Goal | Good Threshold (p75) |
| :--- | :--- | :--- |
| **LCP** (Largest Contentful Paint) | Speed at which main content loads | < 2.5s |
| **INP** (Interaction to Next Paint) | Latency of all page user interactions | < 200ms |
| **CLS** (Cumulative Layout Shift) | Visual stability of layout blocks | < 0.1 |

---

## 3. Operations & Setup

### 3.1 Turbopack Bundle Analyzer
Next.js 16 features integrated support for Turbopack-based bundle analysis. To inspect client-side bundles, install and configure `@next/bundle-analyzer` in `next.config.ts`:

```typescript
// next.config.ts
import type { NextConfig } from "next";
import withBundleAnalyzer from "@next/bundle-analyzer";

const nextConfig: NextConfig = {
  // Your base config options here
};

const configureAnalyzer = withBundleAnalyzer({
  enabled: process.env.ANALYZE === "true",
});

export default configureAnalyzer(nextConfig);
```

Build with analysis enabled to generate visualization HTML files:

```bash
ANALYZE=true pnpm build
```

### 3.2 Dynamic Imports with `next/dynamic`
Optimize bundle size by dynamically importing heavy components only when they are needed:

```tsx
// src/app/dashboard/page.tsx
import dynamic from "next/dynamic";

const InteractiveChart = dynamic(() => import("@/components/InteractiveChart"), {
  ssr: false, // Prevents loading window-dependent components during server pass
  loading: () => <div className="h-64 bg-neutral-100 animate-pulse" />,
});

export default function Page() {
  return (
    <main>
      <h1>Metrics</h1>
      <InteractiveChart />
    </main>
  );
}
```

### 3.3 Instrumentation & Centralized Error Hook

Create an `instrumentation.ts` file in your root directory to track trace metrics (using OpenTelemetry or Sentry) and capture application exceptions:

```typescript
// src/instrumentation.ts
export async function register() {
  if (process.env.NEXT_RUNTIME === "nodejs") {
    // Load Node-native tracing instrumentation
  }
}

export async function onRequestError(
  err: Error,
  request: { path: string; method: string },
  context: {
    routerKind: "Pages Router" | "App Router";
    routePath: string;
    routeType: "render" | "route" | "action" | "middleware";
  }
) {
  // Ship diagnostic payloads to your error logging service (e.g. Sentry/Datadog)
  console.error(`Request Error at ${request.path}:`, err.message, context);
}
```

---

## 4. Practical Application — Performance Diagnostic Flow

If a route becomes slow, apply this diagnostic checklist:

1. **Check the Build footprint:** Run `pnpm build` and verify if the route is dynamically rendering (`ƒ`) or has a large size.
2. **Review with Bundle Analyzer:** If JS weight is high, run `ANALYZE=true pnpm build` to identify large packages (e.g. `lodash` instead of `lodash-es`, or loading complete icons sets).
3. **Lazy-load Client elements:** Wrap browser-only libraries using `next/dynamic` with `ssr: false`.
4. **Bundle optimization overrides:** Declare package optimizer settings inside `next.config.ts` to automatically tree-shake common heavy libraries:

```typescript
// next.config.ts
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  experimental: {
    optimizePackageImports: ["lucide-react", "@my-org/ui-components"],
  },
};
export default nextConfig;
```

5. **Track Web Vitals:** Add speed monitoring tags in your base layout to collect Real User Metrics:

```tsx
// src/app/layout.tsx
import { SpeedInsights } from "@vercel/speed-insights/next";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        {children}
        <SpeedInsights />
      </body>
    </html>
  );
}
```

---

## 5. Common Mistakes & Gotchas

### Optimizing Without Baseline Metrics
Never spent developer cycles lazy-loading small UI components. Focus strictly on large chunks (>40kB) highlighted by your bundle analyzer.

### Missing `ssr: false` on browser-only components
Components referencing global browser variables (`window`, `document`) will throw server-side exceptions during SSR compilation. Always set `ssr: false` when dynamically importing elements utilizing Canvas, maps, or WebGL contexts.

---

## 🎯 Key Takeaways

- **Build summaries are baselines:** Monitor file sizes and routing symbols after structural changes.
- **`onRequestError` is standard:** Set up centralized error capture early using the instrumentation API.
- **Tree-shaking via config:** Declare `optimizePackageImports` in `next.config.ts` to avoid bundling unused library chunks.

*←* [`15_testing.md`](./15_testing.md) *|* *next →* [`17_deployment_and_production.md`](./17_deployment_and_production.md)
