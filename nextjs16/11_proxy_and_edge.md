# 11 — The Proxy Layer & Edge Routing

> **Goal:** Build request interception flows using the new Next.js 16 `proxy.ts` file — executing redirects, rewrites, security headers, and geo-routing at the edge runtime, while respecting edge constraints.

---

## 1. Concept — Request Interception

In Next.js 16, **`proxy.ts`** (or `proxy.js`), located in your project root or `src/` directory, replaces the legacy `middleware.ts` convention. 

It intercepts **every incoming request** before it reaches your application rendering logic. This allows you to apply routing rules, auth checks, and header mutations globally at the network edge.

```typescript
// src/proxy.ts
import { NextResponse, type NextRequest } from "next/server";

export function proxy(req: NextRequest) {
  const isLoggedIn = req.cookies.get("session")?.value;

  // Protect /dashboard routes at the edge
  if (!isLoggedIn && req.nextUrl.pathname.startsWith("/dashboard")) {
    const loginUrl = req.nextUrl.clone();
    loginUrl.pathname = "/login";
    loginUrl.searchParams.set("next", req.nextUrl.pathname);
    return NextResponse.redirect(loginUrl);
  }

  return NextResponse.next();
}

export const config = {
  matcher: ["/dashboard/:path*"],
};
```

This auth check executes within a low-latency edge runtime nearest to the user, blocking unauthenticated traffic before the React Server Components start rendering.

---

## 2. Mechanism — Execution Environment

### 2.1 The Edge Runtime
The proxy layer **always executes on the Edge Runtime**. It runs on lightweight V8 isolates instead of Node.js virtual environments.
- **Limited API Surface:** You cannot access Node-native modules (`node:fs`, `node:crypto` legacy APIs) or standard ORM drivers directly.
- **Web API Compatibility:** You must use Web APIs (like `fetch`, `Request`, `Response`, `crypto.subtle`, `TextEncoder`).
- **Low Overhead:** A proxy must run fast (sub-50ms) to avoid introducing latency on page requests.

### 2.2 Matching Routes
Declare a `config` object with a `matcher` array to limit which routes trigger the proxy layer:

```typescript
export const config = {
  matcher: [
    "/dashboard/:path*",
    "/api/:path*",
    // Negative lookahead: intercept all routes except public assets and internal folders
    "/((?!_next/static|_next/image|favicon.ico).*)",
  ],
};
```

### 2.3 Caching Restrictions in `proxy.ts`
Because `proxy.ts` is a request interceptor running at the network edge, **caching features are completely disabled** inside this file:
- You cannot use the `"use cache"` directive.
- Calling cached helper functions or using caching APIs (`cacheLife`, `cacheTag`) will result in runtime errors.
- Any `fetch` call made inside `proxy.ts` is evaluated per-request (defaulting to `{ cache: 'no-store' }`).

---

## 3. Operations & Patterns

### 3.1 Rewrites, Redirects, and Custom Headers

```typescript
// Redirect users to another URL
return NextResponse.redirect(new URL("/login", req.url));

// Rewrite paths (displays /old-path to the user but resolves /new-path on the server)
return NextResponse.rewrite(new URL("/new-path", req.url));

// Modify headers
const res = NextResponse.next();
res.headers.set("x-custom-region", req.geo?.region ?? "unknown");
return res;
```

### 3.2 Verification & Edge Auth
Use lightweight JWT verification libraries (like `jose`) inside `proxy.ts` to gate access and pass user identity downwards:

```typescript
// src/proxy.ts
import { NextResponse, type NextRequest } from "next/server";
import { jwtVerify } from "jose";

const SECRET = new TextEncoder().encode(process.env.AUTH_SECRET!);

async function getUserSession(req: NextRequest) {
  const token = req.cookies.get("session")?.value;
  if (!token) return null;
  try {
    const { payload } = await jwtVerify(token, SECRET);
    return payload as { sub: string; role: string };
  } catch {
    return null;
  }
}

export async function proxy(req: NextRequest) {
  const session = await getUserSession(req);
  const path = req.nextUrl.pathname;

  if (path.startsWith("/admin") && session?.role !== "admin") {
    return NextResponse.redirect(new URL("/login", req.url));
  }

  // Inject session details into downstream request headers
  const res = NextResponse.next();
  if (session) {
    res.headers.set("x-user-id", session.sub);
  }
  return res;
}

export const config = {
  matcher: ["/admin/:path*", "/dashboard/:path*"],
};
```

Read this header in your Server Components securely:

```tsx
// src/app/dashboard/page.tsx
import { headers } from "next/headers";

export default async function DashboardPage() {
  const h = await headers();
  const userId = h.get("x-user-id");
  
  return <h1>Dashboard for User: {userId}</h1>;
}
```

---

## 4. Practical Application — Geo-IP & Security Headers

Here is a proxy setup combining geolocation redirects and global security headers:

```typescript
// src/proxy.ts
import { NextResponse, type NextRequest } from "next/server";

function applySecurityHeaders(res: NextResponse) {
  res.headers.set("X-Frame-Options", "DENY");
  res.headers.set("X-Content-Type-Options", "nosniff");
  res.headers.set("Referrer-Policy", "strict-origin-when-cross-origin");
  res.headers.set("Permissions-Policy", "camera=(), microphone=(), geolocation=()");
  return res;
}

export function proxy(req: NextRequest) {
  const path = req.nextUrl.pathname;

  // Localized homepage redirects for German visitors
  if (path === "/" && req.geo?.country === "DE") {
    const redirectUrl = new URL("/de", req.url);
    return applySecurityHeaders(NextResponse.redirect(redirectUrl));
  }

  return applySecurityHeaders(NextResponse.next());
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico|api/health).*)"],
};
```

---

## 5. Common Mistakes & Gotchas

### Importing Node.js-specific modules
If you attempt to import native packages (like `fs`, or standard ORM drivers like Prisma) in `proxy.ts`, the build compiler will throw an edge runtime compilation error. Keep imports restricted to edge-compatible libraries.

### Incomplete Matcher Configuration
If your matcher is too broad and fails to exclude `_next/static`, `_next/image`, and asset types, **every javascript file, image render, and font download** will trigger proxy evaluation. This introduces substantial network overhead.

### Modifying `req` directly
The `req.nextUrl` object is read-only. Always clone the URL object before modifying attributes:

```typescript
// WRONG
req.nextUrl.pathname = "/new-path";

// CORRECT
const url = req.nextUrl.clone();
url.pathname = "/new-path";
```

---

## 🎯 Key Takeaways

- **`proxy.ts` replaces `middleware.ts`:** Handles request interception at the root folder level.
- **Edge Runtime execution:** Uses lightweight Web APIs; no access to traditional Node libraries.
- **Zero caching:** Caching directives (`"use cache"`, `cacheLife`) are completely unsupported in the proxy layer.
- **Headers for identity:** Forward authenticated sessions downstream via request headers (`x-user-id`) to keep page evaluations lightweight.

*←* [`10_metadata_and_seo.md`](./10_metadata_and_seo.md) *|* *next →* [`12_authentication.md`](./12_authentication.md)
