# 12 — Authentication

> **Goal:** Build a robust auth system in the App Router using Auth.js (NextAuth v5), understand session strategies, gate routes via RSC-aware patterns, and use middleware for cheap edge checks.

---

## 1. Concept — sessions, RSCs, and three places to check auth

Auth in an App Router app typically uses one library: **Auth.js (the rebranded NextAuth v5)** — it supports OAuth providers, email magic links, credentials, and your own custom adapters.

You'll check auth in three places, each with a different cost/benefit:

1. **Middleware (edge)** — cheap, runs on every request. Good for gating whole sections.
2. **Server Components / Server Actions** — authoritative, can read from DB. Always re-check sensitive ops here.
3. **Client Components** — UI-level (show/hide based on session), never trustworthy.

Minimal Auth.js v5 setup:

```ts
// auth.ts (at the repo root or src/)
import NextAuth from "next-auth";
import GitHub from "next-auth/providers/github";

export const { handlers, auth, signIn, signOut } = NextAuth({
  providers: [GitHub],
  session: { strategy: "jwt" },  // or "database" with an adapter
});
```

```ts
// app/api/auth/[...nextauth]/route.ts
import { handlers } from "@/auth";
export const { GET, POST } = handlers;
```

```ts
// middleware.ts
export { auth as middleware } from "@/auth";
export const config = { matcher: ["/dashboard/:path*"] };
```

```tsx
// app/dashboard/page.tsx
import { auth } from "@/auth";

export default async function Dashboard() {
  const session = await auth();
  if (!session) return <p>Sign in to continue.</p>;
  return <p>Hello {session.user?.name}</p>;
}
```

That's a working app with GitHub OAuth. Auth.js handles the OAuth dance, session cookie, CSRF, and gives you `auth()` to call from anywhere on the server.

---

## 2. Mechanism — session strategies

Auth.js supports two session strategies, and the choice is architectural:

### 2.1 JWT sessions

The session is a **signed JWT stored in an HTTP-only cookie**. Pros:

- No DB query per request — the cookie *is* the session.
- Works at the edge (middleware can verify without hitting the DB).
- Stateless, scales horizontally.

Cons:

- **Invalidation is hard.** Logging a user out doesn't truly revoke until the cookie expires (or you maintain a denylist).
- Session payload size is limited (cookie size cap).
- If a user's permissions change, the JWT doesn't reflect that until refresh.

### 2.2 Database sessions

Auth.js stores a session row in your DB (via an adapter — Prisma, Drizzle, etc.). The cookie holds a session ID; on each request, Auth.js fetches the session row.

Pros:

- Real revocation: delete the row, the user's logged out instantly.
- Multi-device session management (list active sessions).

Cons:

- One DB query per protected request (mitigated by caching).
- Middleware at the edge can't easily verify (no DB at the edge without a serverless-friendly driver).

### 2.3 Hybrid: JWT for edge gate, DB for actions

Most production apps use **JWT for middleware checks** + **DB lookup in server actions / sensitive RSCs**. The middleware bounces obvious unauth requests fast; sensitive operations (delete account, charge card) re-verify the user's row and permissions from the DB.

### 2.4 Auth.js v5 callbacks

```ts
// auth.ts
export const { handlers, auth, signIn, signOut } = NextAuth({
  providers: [GitHub],
  session: { strategy: "jwt" },
  callbacks: {
    async jwt({ token, user }) {
      if (user) {
        token.id = user.id;
        token.role = (user as any).role;
      }
      return token;
    },
    async session({ session, token }) {
      if (session.user) {
        session.user.id = token.id as string;
        (session.user as any).role = token.role;
      }
      return session;
    },
    authorized({ auth: session, request }) {
      const path = request.nextUrl.pathname;
      const isAdmin = (session?.user as any)?.role === "admin";
      if (path.startsWith("/admin") && !isAdmin) return false;
      if (path.startsWith("/app") && !session) return false;
      return true;
    },
  },
});
```

The `authorized` callback is what the `middleware` export reads — returning `false` redirects to the configured sign-in page automatically.

---

## 3. Variations / depth

### 3.1 Email magic link

```ts
import Email from "next-auth/providers/nodemailer";

providers: [
  Email({
    server: { host: "smtp.example.com", port: 587, auth: { user: "x", pass: "y" } },
    from: "noreply@example.com",
  }),
],
```

Email link providers require a database (to store one-time verification tokens), so they pair with an adapter:

```ts
import { PrismaAdapter } from "@auth/prisma-adapter";
import { prisma } from "@/lib/db";

adapter: PrismaAdapter(prisma),
```

### 3.2 Credentials (email + password)

```ts
import Credentials from "next-auth/providers/credentials";
import { verifyPassword, getUserByEmail } from "@/lib/users";
import { z } from "zod";

providers: [
  Credentials({
    credentials: { email: {}, password: {} },
    async authorize(input) {
      const parsed = z.object({ email: z.string().email(), password: z.string().min(8) }).safeParse(input);
      if (!parsed.success) return null;
      const user = await getUserByEmail(parsed.data.email);
      if (!user) return null;
      const ok = await verifyPassword(parsed.data.password, user.passwordHash);
      return ok ? { id: user.id, name: user.name, email: user.email } : null;
    },
  }),
],
```

Credentials provider **forces `session.strategy: "jwt"`** — Auth.js can't manage credential sessions via DB out of the box. Hash with bcrypt or Argon2, never store plaintext.

### 3.3 Sign in / sign out from server actions

```tsx
// app/auth/SignInButton.tsx
import { signIn } from "@/auth";

export function SignInButton() {
  return (
    <form action={async () => {
      "use server";
      await signIn("github", { redirectTo: "/dashboard" });
    }}>
      <button type="submit">Sign in with GitHub</button>
    </form>
  );
}
```

```tsx
// app/auth/SignOutButton.tsx
import { signOut } from "@/auth";

export function SignOutButton() {
  return (
    <form action={async () => {
      "use server";
      await signOut({ redirectTo: "/" });
    }}>
      <button type="submit">Sign out</button>
    </form>
  );
}
```

These are server components (no `"use client"`) — they use server actions for the form. Progressive enhancement out of the box.

### 3.4 Protecting a server action

```ts
// lib/actions/posts.ts
"use server";
import { auth } from "@/auth";
import { db } from "@/lib/db";
import { redirect } from "next/navigation";

export async function deletePost(id: string) {
  const session = await auth();
  if (!session?.user?.id) throw new Error("Unauthorized");

  const post = await db.post.findUnique({ where: { id } });
  if (!post || post.authorId !== session.user.id) throw new Error("Forbidden");

  await db.post.delete({ where: { id } });
  redirect("/posts");
}
```

Never trust client-side `disabled` buttons. The action endpoint is reachable; gate it.

### 3.5 Client-side session via `<SessionProvider>`

```tsx
// app/providers.tsx
"use client";
import { SessionProvider } from "next-auth/react";

export function Providers({ children }: { children: React.ReactNode }) {
  return <SessionProvider>{children}</SessionProvider>;
}
```

```tsx
"use client";
import { useSession } from "next-auth/react";

export function HeaderUser() {
  const { data } = useSession();
  if (!data) return <a href="/login">Sign in</a>;
  return <span>{data.user?.name}</span>;
}
```

This client-side session is convenient for instant UI changes, but it makes a call to `/api/auth/session`. For server components, prefer `await auth()` directly — it's faster and avoids the round trip.

### 3.6 Role-based access control (RBAC)

```ts
// lib/authz.ts
import "server-only";
import { auth } from "@/auth";

export async function requireRole(role: "admin" | "editor") {
  const session = await auth();
  if (!session?.user || (session.user as any).role !== role) {
    throw new Error("Forbidden");
  }
  return session;
}
```

```tsx
// app/admin/page.tsx
import { requireRole } from "@/lib/authz";

export default async function AdminPage() {
  await requireRole("admin");
  return <h1>Admin</h1>;
}
```

For complex permission systems, layer this with a policy library (Oso, Casbin) but the principle is the same.

### 3.7 Custom adapter

Drizzle adapter example:

```ts
// auth.ts
import { DrizzleAdapter } from "@auth/drizzle-adapter";
import { db } from "@/db";

export const { handlers, auth } = NextAuth({
  adapter: DrizzleAdapter(db),
  providers: [GitHub],
  session: { strategy: "database" },
});
```

Module 13 covers Drizzle in depth.

---

## 4. Practical application — a protected dashboard with GitHub OAuth + role check

```ts
// auth.ts
import NextAuth from "next-auth";
import GitHub from "next-auth/providers/github";

const ADMINS = new Set(["ada@example.com", "bob@example.com"]);

export const { handlers, auth, signIn, signOut } = NextAuth({
  providers: [
    GitHub({
      clientId: process.env.GITHUB_ID!,
      clientSecret: process.env.GITHUB_SECRET!,
    }),
  ],
  session: { strategy: "jwt" },
  pages: { signIn: "/login" },
  callbacks: {
    async jwt({ token, user }) {
      if (user?.email) {
        (token as any).role = ADMINS.has(user.email) ? "admin" : "user";
      }
      return token;
    },
    async session({ session, token }) {
      (session.user as any).role = (token as any).role ?? "user";
      return session;
    },
    authorized({ auth: session, request }) {
      const path = request.nextUrl.pathname;
      if (path.startsWith("/admin")) return (session?.user as any)?.role === "admin";
      if (path.startsWith("/app")) return !!session;
      return true;
    },
  },
});
```

```ts
// app/api/auth/[...nextauth]/route.ts
import { handlers } from "@/auth";
export const { GET, POST } = handlers;
```

```ts
// middleware.ts
export { auth as middleware } from "@/auth";

export const config = {
  matcher: ["/app/:path*", "/admin/:path*"],
};
```

```tsx
// app/login/page.tsx
import { signIn } from "@/auth";

export default function LoginPage({ searchParams }: { searchParams: Promise<{ next?: string }> }) {
  return (
    <form action={async (fd) => {
      "use server";
      const next = (await searchParams).next ?? "/app";
      await signIn("github", { redirectTo: next });
    }}>
      <button type="submit">Sign in with GitHub</button>
    </form>
  );
}
```

```tsx
// app/app/layout.tsx
import { auth } from "@/auth";
import { SignOutButton } from "@/components/SignOutButton";

export default async function AppLayout({ children }: { children: React.ReactNode }) {
  const session = await auth();   // safe to assume non-null due to middleware gate
  return (
    <div>
      <header className="flex items-center justify-between border-b p-3">
        <span>{session?.user?.name}</span>
        <SignOutButton />
      </header>
      <main className="p-4">{children}</main>
    </div>
  );
}
```

```tsx
// app/admin/page.tsx
import { auth } from "@/auth";
import { redirect } from "next/navigation";

export default async function AdminPage() {
  const session = await auth();
  // Belt-and-braces: re-check role server-side
  if ((session?.user as any)?.role !== "admin") redirect("/app");
  return <h1>Admin tools</h1>;
}
```

```tsx
// components/SignOutButton.tsx
import { signOut } from "@/auth";
export function SignOutButton() {
  return (
    <form action={async () => { "use server"; await signOut({ redirectTo: "/" }); }}>
      <button type="submit">Sign out</button>
    </form>
  );
}
```

Result: a fully working OAuth flow with role-based gates at the edge, with belt-and-braces re-checks server-side. Sub-100ms gate latency, no DB read on `/app/*` page loads (JWT strategy), and every server action that mutates data re-validates the session.

---

## 5. Common mistakes & gotchas

### Trusting the client session

Showing a "Delete" button only when `useSession()` returns admin is fine for UX, but **the server action must re-check**. Otherwise a malicious POST bypasses your UI entirely.

### Storing secrets in `NEXT_PUBLIC_*`

Auth secrets (`AUTH_SECRET`, OAuth client secrets) must be server-only env vars. Don't prefix them with `NEXT_PUBLIC_` or they'll leak into the client bundle.

### Forgetting `AUTH_SECRET` in production

Auth.js requires `AUTH_SECRET` (a random 32+ char string) in prod. In dev it falls back; in prod it throws. Add to `.env.production` or your hosting env.

### Reading `req.cookies.get("next-auth.session-token")` directly

Don't. Use `await auth()` server-side. Reading the cookie raw bypasses signature verification and is incompatible across Auth.js versions.

### Mixing `getServerSession` (v4) and `auth()` (v5)

Auth.js renamed the API in v5. Search-engine answers for "next-auth" usually still show v4. Confirm which version you have (`pnpm ls next-auth`) and use the matching docs.

### Edge middleware doing a DB lookup

With JWT sessions, middleware verifies the cookie signature at the edge — no DB needed. Avoid hitting your DB from middleware unless you've adopted an edge-compatible driver and accept the cost.

### Race between OAuth callback and protected redirect

If `/api/auth/callback/github` is matched by middleware that requires auth, the callback redirects to login → infinite loop. Always exclude `/api/auth/*` from your protected matcher.

### CSRF for non-form actions

Auth.js handles CSRF for its routes. For your server actions, the framework attaches CSRF protection automatically. For traditional `/api/...` route handlers that mutate state, you need your own CSRF token or SameSite cookies.

### Magic-link emails in dev

If you can't get SMTP to work in dev, log the magic link to the console with a dev-only `from` and a logger transport. The Auth.js docs have a recipe.

---

## 🎯 Key Takeaways

- **Auth.js (NextAuth v5) is the de facto Next.js auth library.** `auth()` from a server component gives you the session; the same function exported as `middleware` gives you an edge gate.
- **Choose JWT vs database sessions deliberately.** JWT scales and works at the edge; DB sessions give you real revocation. Hybrid (JWT gate + DB re-check in actions) is the production sweet spot.
- **Check auth in three layers** — middleware, server components/actions, and (for UX) client. Trust only the server-side checks; the client check is decoration.
- **Always re-verify ownership inside server actions** that mutate user-scoped data. Don't rely on UI gating.
- **Keep secrets server-only.** `AUTH_SECRET`, OAuth client secrets, DB URLs — never `NEXT_PUBLIC_`.

*←* [`11_middleware_and_edge.md`](./11_middleware_and_edge.md) *|* *next →* [`13_database_and_orm.md`](./13_database_and_orm.md)
