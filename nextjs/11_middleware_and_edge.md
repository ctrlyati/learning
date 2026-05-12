# 11 — Middleware & The Routing Edge

> **Goal:** Write middleware that runs at the edge for auth, rewrites, redirects, headers, and A/B routing — and know its limits.

---

## 1. Concept — one file, intercepts every request

`middleware.ts` at the project root (or inside `src/`) runs **before every request** that matches its `config.matcher`. It executes on the **edge runtime**, returns a `NextResponse`, and can:

- Redirect (`NextResponse.redirect`)
- Rewrite (`NextResponse.rewrite`) — change what URL renders, while preserving the user's URL
- Continue (`NextResponse.next()`) with modified headers or cookies
- Return a response directly (e.g., 401)

```ts
// middleware.ts
import { NextResponse, type NextRequest } from "next/server";

export function middleware(req: NextRequest) {
  const isLoggedIn = req.cookies.get("session")?.value;
  if (!isLoggedIn && req.nextUrl.pathname.startsWith("/dashboard")) {
    const url = req.nextUrl.clone();
    url.pathname = "/login";
    url.searchParams.set("next", req.nextUrl.pathname);
    return NextResponse.redirect(url);
  }
  return NextResponse.next();
}

export const config = {
  matcher: ["/dashboard/:path*"],
};
```

That's a complete auth guard. Every `/dashboard/*` request runs through it, at the edge, before any RSC renders.

---

## 2. Mechanism — how it runs

### 2.1 Runtime

Middleware **always runs on the edge runtime**, never on Node. This means:

- No `node:` modules (no `fs`, no `node:crypto` legacy APIs, no Prisma without the edge driver).
- Use Web APIs (`fetch`, `Request`, `Response`, `crypto.subtle`).
- Cold starts are minimal; latency from middleware is typically a few milliseconds.

### 2.2 Matcher

`config.matcher` decides which requests the middleware handles. It supports:

```ts
export const config = {
  matcher: [
    "/dashboard/:path*",
    "/api/:path*",
    // negative lookahead: skip static assets, RSC payloads, images
    "/((?!_next/static|_next/image|favicon.ico).*)",
  ],
};
```

Matching is regex-like. Use the negative lookahead pattern to apply middleware to *all* paths except framework internals.

### 2.3 Returning responses

```ts
// Continue and add a header
const res = NextResponse.next();
res.headers.set("x-server-region", process.env.VERCEL_REGION ?? "local");
return res;

// Redirect
return NextResponse.redirect(new URL("/login", req.url));

// Rewrite (user sees /old, server renders /new)
return NextResponse.rewrite(new URL("/new", req.url));

// Block (return immediately)
return new NextResponse("Forbidden", { status: 403 });
```

### 2.4 Cookies and headers

```ts
const session = req.cookies.get("session");

const res = NextResponse.next();
res.cookies.set("flag", "on", { httpOnly: true, secure: true, path: "/" });
res.cookies.delete("session");
return res;
```

You can read request headers directly, and set response headers on the returned `NextResponse`.

### 2.5 Reading the geo / IP

```ts
const country = req.geo?.country ?? "XX";
const ip = req.ip ?? req.headers.get("x-forwarded-for");
```

(Provider-dependent: on Vercel these are populated; self-hosted, you'll need to wire up trusted headers.)

---

## 3. Variations / depth

### 3.1 Rewrites and redirects in `next.config.mjs`

For static rewrites/redirects that don't need request-time logic, prefer config — it's evaluated at build, no middleware overhead:

```js
// next.config.mjs
export default {
  async redirects() {
    return [
      { source: "/old-blog", destination: "/blog", permanent: true },
    ];
  },
  async rewrites() {
    return [
      { source: "/api/legacy/:path*", destination: "https://legacy.example.com/:path*" },
    ];
  },
};
```

Use middleware when the decision depends on cookies, headers, geo, or A/B-test buckets.

### 3.2 A/B testing with rewrites

Bucket users via cookie, rewrite to a variant route:

```ts
// middleware.ts
import { NextResponse, type NextRequest } from "next/server";

const COOKIE = "ab-bucket";

export function middleware(req: NextRequest) {
  if (req.nextUrl.pathname !== "/") return NextResponse.next();

  let bucket = req.cookies.get(COOKIE)?.value;
  if (!bucket) bucket = Math.random() < 0.5 ? "a" : "b";

  const url = req.nextUrl.clone();
  url.pathname = `/_variants/${bucket}`;
  const res = NextResponse.rewrite(url);
  res.cookies.set(COOKIE, bucket, { path: "/", maxAge: 60 * 60 * 24 * 30 });
  return res;
}

export const config = { matcher: ["/"] };
```

Folders `app/_variants/a/page.tsx` and `app/_variants/b/page.tsx` provide the variants. Underscore folders are private so they can't be visited directly.

### 3.3 Auth at the edge

You can verify a JWT or signed session cookie at the edge — fast, before any RSC runs. Use a library that works with `crypto.subtle`:

```ts
// middleware.ts
import { NextResponse, type NextRequest } from "next/server";
import { jwtVerify } from "jose";

const SECRET = new TextEncoder().encode(process.env.AUTH_SECRET!);

async function getSession(req: NextRequest) {
  const token = req.cookies.get("session")?.value;
  if (!token) return null;
  try {
    const { payload } = await jwtVerify(token, SECRET);
    return payload as { sub: string; role: string };
  } catch {
    return null;
  }
}

export async function middleware(req: NextRequest) {
  const session = await getSession(req);
  const path = req.nextUrl.pathname;

  if (path.startsWith("/admin")) {
    if (session?.role !== "admin") {
      return NextResponse.redirect(new URL("/login", req.url));
    }
  }
  if (path.startsWith("/app")) {
    if (!session) {
      const url = new URL("/login", req.url);
      url.searchParams.set("next", path);
      return NextResponse.redirect(url);
    }
  }

  // forward identity to downstream RSC via header
  const res = NextResponse.next();
  if (session) res.headers.set("x-user-id", session.sub);
  return res;
}

export const config = { matcher: ["/admin/:path*", "/app/:path*"] };
```

Read `x-user-id` in a server component:

```tsx
import { headers } from "next/headers";

export default async function Page() {
  const h = await headers();
  const userId = h.get("x-user-id");
  // ...
}
```

This pattern keeps your DB sessions (Module 12) but offloads the *quick* gate to the edge.

### 3.4 Security headers

Set CSP, HSTS, Referrer-Policy, etc. once in middleware:

```ts
// middleware.ts
import { NextResponse, type NextRequest } from "next/server";

export function middleware(req: NextRequest) {
  const res = NextResponse.next();
  res.headers.set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload");
  res.headers.set("X-Frame-Options", "DENY");
  res.headers.set("X-Content-Type-Options", "nosniff");
  res.headers.set("Referrer-Policy", "strict-origin-when-cross-origin");
  res.headers.set("Permissions-Policy", "camera=(), microphone=(), geolocation=()");
  return res;
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
```

For CSP with nonces, generate per-request nonces in middleware and inject them into your `<Script>` tags — there's an official Next.js guide for this.

### 3.5 Localization routing

Detect locale from `Accept-Language` and rewrite:

```ts
const LOCALES = ["en", "es", "fr"];

export function middleware(req: NextRequest) {
  const path = req.nextUrl.pathname;
  const hasLocale = LOCALES.some((l) => path.startsWith(`/${l}/`) || path === `/${l}`);
  if (hasLocale) return NextResponse.next();

  const accept = req.headers.get("accept-language") ?? "";
  const preferred = LOCALES.find((l) => accept.startsWith(l)) ?? "en";
  return NextResponse.redirect(new URL(`/${preferred}${path}`, req.url));
}
```

Combine with `app/[lang]/...` segments.

### 3.6 What middleware can't do

- It can't render React.
- It can't access Node-only APIs (Prisma, bcrypt, node-postgres without edge driver).
- It can't access the database directly *unless* you use an edge-compatible driver (Neon, `@vercel/postgres`).
- Its execution time and bundle size are limited (Vercel caps it; self-hosted, you set the limit). Keep it lean — sub-50ms is typical.

---

## 4. Practical application — protected app with security headers, auth gate, and country-based redirect

```ts
// middleware.ts
import { NextResponse, type NextRequest } from "next/server";
import { jwtVerify } from "jose";

const SECRET = new TextEncoder().encode(process.env.AUTH_SECRET!);
const PUBLIC_PATHS = ["/", "/login", "/about", "/pricing"];

async function getUserId(req: NextRequest): Promise<string | null> {
  const token = req.cookies.get("session")?.value;
  if (!token) return null;
  try {
    const { payload } = await jwtVerify(token, SECRET);
    return String(payload.sub);
  } catch {
    return null;
  }
}

function applySecurityHeaders(res: NextResponse) {
  res.headers.set("X-Frame-Options", "DENY");
  res.headers.set("X-Content-Type-Options", "nosniff");
  res.headers.set("Referrer-Policy", "strict-origin-when-cross-origin");
  res.headers.set("Permissions-Policy", "camera=(), microphone=(), geolocation=()");
  return res;
}

export async function middleware(req: NextRequest) {
  const path = req.nextUrl.pathname;

  // Country-based marketing redirect — only on homepage
  if (path === "/" && req.geo?.country === "DE") {
    return applySecurityHeaders(NextResponse.redirect(new URL("/de", req.url)));
  }

  // Public paths: passthrough with security headers
  if (PUBLIC_PATHS.includes(path)) {
    return applySecurityHeaders(NextResponse.next());
  }

  // Protected
  const userId = await getUserId(req);
  if (!userId) {
    const url = new URL("/login", req.url);
    url.searchParams.set("next", path);
    return applySecurityHeaders(NextResponse.redirect(url));
  }

  const res = NextResponse.next();
  res.headers.set("x-user-id", userId);
  return applySecurityHeaders(res);
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico|api/health).*)"],
};
```

```tsx
// app/dashboard/page.tsx — server component reads forwarded identity
import { headers } from "next/headers";

export default async function Dashboard() {
  const h = await headers();
  const userId = h.get("x-user-id");
  // optionally re-validate user in DB
  return <h1>Welcome, {userId}</h1>;
}
```

The middleware runs in low-millisecond time at every PoP nearest the user. Bots and unauth requests hit the edge and bounce — they never spin up a Node function. That's where edge auth earns its keep at scale.

---

## 5. Common mistakes & gotchas

### Importing Node-only modules

Middleware fails to compile if you import Prisma client (non-edge), bcrypt, node-postgres, etc. Use edge-compatible drivers (Neon serverless, jose for JWT, Upstash Redis, Vercel KV). For complex logic that needs Node APIs, do that work in a route handler with `runtime = "nodejs"` instead.

### Matcher that catches `_next/*` and static assets

If middleware runs on `_next/static/*.js` etc., every static asset request waits on your auth check. Always exclude `_next/static`, `_next/image`, and `favicon.ico` (and your sitemap/robots).

### Middleware modifies cookies but doesn't return the response

Setting `res.cookies.set(...)` and then returning `NextResponse.next()` (a different `res`) loses the cookie. Modify the *returned* response.

### Heavy work in middleware

Calling your auth DB on every request, hitting external APIs synchronously, parsing large bodies — these add latency to *every* page load. Cache, defer, push to RSC. Middleware should be cheap.

### Redirecting to a URL that the middleware itself matches

Infinite redirect loops. If `/login` is matched and the middleware redirects unauthenticated users to `/login`, you've made `/login` a public exception.

### `req.geo` is undefined locally

`req.geo` is populated by Vercel (or compatible hosting) — not in `next dev`. Use a fallback or mock via env in dev.

### Reading request body in middleware

You can't reliably read the body in middleware — the body would have to be drained, breaking the downstream handler. Do body inspection in route handlers instead.

### Modifying the URL with mutation

`req.nextUrl` is read-only-ish. Use `req.nextUrl.clone()` and mutate the clone.

### Edge runtime size limits

Middleware bundle is capped (Vercel ~1 MB compressed). Importing the kitchen sink (lodash, moment) blows the budget. Keep imports tight and prefer Web APIs.

---

## 🎯 Key Takeaways

- **Middleware runs on the edge, before any RSC render.** It's the ideal place for auth gates, redirects, A/B testing, security headers, and geo routing.
- **Edge runtime ≠ Node runtime.** No `node:` modules, no traditional Postgres clients. Use Web APIs and edge-compatible drivers.
- **`config.matcher` is critical** — without an exclusion for `_next/static` and friends, every static asset pays the middleware tax.
- **Prefer `next.config.mjs` `redirects`/`rewrites`** for static rules; reach for middleware when the decision is per-request.
- **Forward identity downstream via custom headers** (`x-user-id`) and read them with `headers()` in server components. Keeps the heavy DB lookup out of the edge while keeping cheap gates fast.

*←* [`10_metadata_and_seo.md`](./10_metadata_and_seo.md) *|* *next →* [`12_authentication.md`](./12_authentication.md)
