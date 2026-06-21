# 12 — Authentication

> **Goal:** Build a robust auth system in the App Router using Auth.js (NextAuth v5) with React 19 compatibility, configure sessions, gate routes via the `proxy.ts` edge layer, and secure server-side components.

---

## 1. Concept — Layered Security Gates

Authentication in a Next.js 16 project is typically managed by **Auth.js** (formerly NextAuth v5), which supports OAuth providers, credentials (email/password), and custom database adapters.

You verify sessions across three distinct architectural layers:

1. **Proxy Layer (`proxy.ts` at the edge):** Low-latency request interception to redirect unauthenticated visitors away from private path trees (e.g. `/dashboard/*`).
2. **Server Components & Server Actions:** Authoritative backend checks. Always re-evaluate user session credentials here before executing database operations.
3. **Client Components:** UI decoration only (e.g. toggling menu visibility). Never trust client-state for access control.

### Basic Auth.js v5 Configuration:

```typescript
// src/auth.ts (Root or src directory)
import NextAuth from "next-auth";
import GitHub from "next-auth/providers/github";

export const { handlers, auth, signIn, signOut } = NextAuth({
  providers: [GitHub],
  session: { strategy: "jwt" }, // JWT strategy for stateless edge gates
});
```

Mount the OAuth endpoints dynamically:

```typescript
// src/app/api/auth/[...nextauth]/route.ts
import { handlers } from "@/auth";
export const { GET, POST } = handlers;
```

Export the auth handler directly from `proxy.ts` to secure paths:

```typescript
// src/proxy.ts
import { auth } from "@/auth";

export const proxy = auth; // Auth.js handler matches Next.js 16 proxy interfaces

export const config = {
  matcher: ["/dashboard/:path*", "/admin/:path*"],
};
```

Access the session inside a Server Component:

```tsx
// src/app/dashboard/page.tsx
import { auth } from "@/auth";

export default async function DashboardPage() {
  const session = await auth();
  if (!session) {
    return <p>Access Denied. Please sign in.</p>;
  }

  return <p>Welcome back, {session.user?.name}!</p>;
}
```

---

## 2. Session Strategies

### 2.1 JWT (Stateless Cookies)
Session tokens are cryptographically signed JWTs stored in secure, `httpOnly` client cookies.
- **Pros:** Fast local signature verification at the edge without querying databases.
- **Cons:** Revocation is difficult (tokens remain valid until expiration unless checked against a blacklist).

### 2.2 Database Sessions
Tokens represent session records stored in your database (via adapters like Prisma or Drizzle).
- **Pros:** Real-time revocation (deleting the database row immediately logs the user out).
- **Cons:** Introduces a database lookup query on every request.

### 2.3 Hybrid Security Model
A robust production approach combines both strategies:
- Use **JWT sessions** in `proxy.ts` to filter unauthenticated traffic quickly at the network edge.
- Perform **Database checks** inside Server Actions or sensitive Server Components (like payment or profile deletions) to authorize the request using real-time database state.

---

## 3. Operations & Patterns

### 3.1 Authentication Actions

Handle login and logout operations inside standard forms using Server Actions:

```tsx
// src/components/SignInButton.tsx
import { signIn } from "@/auth";

export function SignInButton() {
  return (
    <form
      action={async () => {
        "use server";
        await signIn("github", { redirectTo: "/dashboard" });
      }}
    >
      <button type="submit" className="border p-2 bg-black text-white">
        Sign in with GitHub
      </button>
    </form>
  );
}
```

```tsx
// src/components/SignOutButton.tsx
import { signOut } from "@/auth";

export function SignOutButton() {
  return (
    <form
      action={async () => {
        "use server";
        await signOut({ redirectTo: "/" });
      }}
    >
      <button type="submit" className="border p-2">
        Sign out
      </button>
    </form>
  );
}
```

### 3.2 Protecting Server Actions

Always re-check auth inside mutations — never trust client components or layout restrictions:

```typescript
// src/lib/actions/posts.ts
"use server";
import { auth } from "@/auth";
import { db } from "@/lib/db";
import { updateTag } from "next/cache";

export async function deletePost(id: string) {
  const session = await auth();
  if (!session?.user?.id) {
    throw new Error("Unauthorized");
  }

  // Verify ownership
  const post = await db.post.findUnique({ where: { id } });
  if (!post || post.authorId !== session.user.id) {
    throw new Error("Forbidden");
  }

  await db.post.delete({ where: { id } });
  updateTag("posts");
}
```

---

## 4. Practical Application — GitHub OAuth & Role Checks

Configure role-based access control (RBAC) callbacks and secure paths.

```typescript
// src/auth.ts
import NextAuth from "next-auth";
import GitHub from "next-auth/providers/github";

const ADMIN_EMAILS = new Set(["admin@example.com"]);

export const { handlers, auth, signIn, signOut } = NextAuth({
  providers: [
    GitHub({
      clientId: process.env.GITHUB_ID!,
      clientSecret: process.env.GITHUB_SECRET!,
    }),
  ],
  session: { strategy: "jwt" },
  pages: {
    signIn: "/login",
  },
  callbacks: {
    async jwt({ token, user }) {
      if (user?.email) {
        token.role = ADMIN_EMAILS.has(user.email) ? "admin" : "user";
      }
      return token;
    },
    async session({ session, token }) {
      if (session.user) {
        (session.user as any).role = token.role ?? "user";
      }
      return session;
    },
    authorized({ auth: session, request }) {
      const path = request.nextUrl.pathname;
      if (path.startsWith("/admin")) {
        return (session?.user as any)?.role === "admin";
      }
      if (path.startsWith("/dashboard")) {
        return !!session;
      }
      return true;
    },
  },
});
```

Ensure you exclude authentication endpoints from your `proxy.ts` matcher to prevent infinite redirect loops:

```typescript
// src/proxy.ts
import { auth } from "@/auth";

export const proxy = auth;

export const config = {
  // Protect app paths but exclude NextAuth API routes and static assets
  matcher: ["/((?!api/auth|_next/static|_next/image|favicon.ico).*)"],
};
```

---

## 5. Common Mistakes & Gotchas

### Public secrets
Ensure secrets (like `AUTH_SECRET`, OAuth client keys, database URLs) are **never** prefixed with `NEXT_PUBLIC_`. If they are, Webpack/Turbopack will bundle them into the client JS files.

### OAuth Callback redirects
If your `proxy.ts` blocks all paths by default, it will intercept the OAuth redirect URL (`/api/auth/callback/...`). Always exclude `/api/auth` routes from your proxy configuration matcher.

### Production `AUTH_SECRET`
In development, Auth.js runs without throwing errors if `AUTH_SECRET` is missing. In production, however, it will immediately throw exceptions and crash. Generate a secure secret and set it in your environment:

```bash
npx auth secret
```

---

## 🎯 Key Takeaways

- **Auth.js is standard:** Integrate `auth()` inside layout components and actions.
- **Edge verification:** Export `auth` inside `proxy.ts` to gate access at the edge.
- **Hybrid checks:** Use lightweight tokens for fast redirects, and query DB sessions for state mutations.

*←* [`11_proxy_and_edge.md`](./11_proxy_and_edge.md) *|* *next →* [`13_database_and_orm.md`](./13_database_and_orm.md)
