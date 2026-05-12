# 13 — Database & ORM Integration

> **Goal:** Wire Prisma or Drizzle into an App Router project with serverless-aware connection management, write idiomatic queries from RSCs and Server Actions, and pair them with the Next.js cache.

---

## 1. Concept — query from inside a Server Component

Server Components can `await` database calls directly. No `getServerSideProps`, no API route hop.

```tsx
// app/posts/page.tsx
import { db } from "@/lib/db";

export default async function PostsPage() {
  const posts = await db.post.findMany({ orderBy: { createdAt: "desc" } });
  return (
    <ul>
      {posts.map((p) => <li key={p.id}>{p.title}</li>)}
    </ul>
  );
}
```

That's the entire developer experience. The interesting work is in `lib/db.ts` — making sure connections are pooled correctly, the client is reused across hot reloads in dev, and (for edge/serverless deployments) the driver supports the environment.

---

## 2. Mechanism — connection pooling and instance reuse

### 2.1 The dev hot-reload problem

`next dev` HMR keeps the process alive but reloads modules. A naive `new PrismaClient()` per import creates dozens of connections in minutes, exhausting your Postgres pool.

Fix: cache the client on `globalThis` in dev:

```ts
// lib/db.ts (Prisma)
import "server-only";
import { PrismaClient } from "@prisma/client";

const globalForPrisma = globalThis as unknown as { prisma?: PrismaClient };

export const db = globalForPrisma.prisma ?? new PrismaClient({
  log: ["error", "warn"],
});

if (process.env.NODE_ENV !== "production") globalForPrisma.prisma = db;
```

### 2.2 Serverless connection pooling

In serverless (Vercel, AWS Lambda), each function instance opens its own DB connection. With dozens or hundreds of concurrent instances, you blow past Postgres's connection limit (default ~100).

Solutions:

- **PgBouncer / connection pooler** in front of Postgres (Supabase and Neon include this).
- **Neon serverless driver** (`@neondatabase/serverless`) — uses Postgres over WebSockets, no persistent connection.
- **Vercel Postgres / `@vercel/postgres`** — connection pooling baked in.
- **Prisma Accelerate** — managed connection pool + cache.

### 2.3 Edge vs Node runtime for DB calls

- **Edge** (middleware, edge routes) needs a fetch-based or WebSocket-based driver: `@neondatabase/serverless`, `@vercel/postgres`, Drizzle with `neon-http`.
- **Node** (default route handlers, server actions) can use anything: Prisma, `pg`, mysql2.

The **server runtime** of an App Router route defaults to Node, which is what you want for DB-heavy work. Only opt into edge for routes that truly need it.

---

## 3. Variations / depth

### 3.1 Prisma setup

```bash
pnpm add @prisma/client
pnpm add -D prisma
npx prisma init
```

```prisma
// prisma/schema.prisma
datasource db {
  provider = "postgresql"
  url      = env("DATABASE_URL")
}

generator client {
  provider = "prisma-client-js"
}

model Post {
  id        String   @id @default(cuid())
  title     String
  body      String
  authorId  String
  createdAt DateTime @default(now())
  updatedAt DateTime @updatedAt
  author    User     @relation(fields: [authorId], references: [id])
}

model User {
  id    String @id @default(cuid())
  email String @unique
  name  String?
  posts Post[]
}
```

```bash
npx prisma generate
npx prisma migrate dev --name init
```

Query patterns:

```ts
// lib/data/posts.ts
import "server-only";
import { cache } from "react";
import { db } from "@/lib/db";

export const getPost = cache(async (id: string) => {
  return db.post.findUnique({
    where: { id },
    include: { author: { select: { id: true, name: true } } },
  });
});

export const getPostsByAuthor = cache(async (authorId: string) => {
  return db.post.findMany({
    where: { authorId },
    orderBy: { createdAt: "desc" },
  });
});
```

`react.cache()` dedupes calls **per request** — multiple components calling `getPost("p1")` on the same request hit the DB once.

For persistent caching, wrap with `unstable_cache`:

```ts
import { unstable_cache } from "next/cache";

export const getTrendingPosts = unstable_cache(
  async () => db.post.findMany({ orderBy: { views: "desc" }, take: 10 }),
  ["trending-posts"],
  { revalidate: 300, tags: ["posts"] }
);
```

### 3.2 Drizzle setup

Drizzle is a thinner, SQL-first ORM that ships less runtime and works at the edge:

```bash
pnpm add drizzle-orm postgres
pnpm add -D drizzle-kit
```

```ts
// db/schema.ts
import { pgTable, text, timestamp } from "drizzle-orm/pg-core";
import { createId } from "@paralleldrive/cuid2";

export const users = pgTable("users", {
  id: text("id").primaryKey().$defaultFn(() => createId()),
  email: text("email").notNull().unique(),
  name: text("name"),
});

export const posts = pgTable("posts", {
  id: text("id").primaryKey().$defaultFn(() => createId()),
  title: text("title").notNull(),
  body: text("body").notNull(),
  authorId: text("author_id").notNull().references(() => users.id),
  createdAt: timestamp("created_at").notNull().defaultNow(),
});
```

```ts
// db/index.ts (Node runtime)
import "server-only";
import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";
import * as schema from "./schema";

const client = postgres(process.env.DATABASE_URL!, { max: 1 });
export const db = drizzle(client, { schema });
```

```ts
// db/edge.ts (Edge runtime)
import { drizzle } from "drizzle-orm/neon-http";
import { neon } from "@neondatabase/serverless";
import * as schema from "./schema";

const sql = neon(process.env.DATABASE_URL!);
export const db = drizzle(sql, { schema });
```

Query patterns:

```ts
import { db } from "@/db";
import { posts } from "@/db/schema";
import { desc, eq } from "drizzle-orm";

const all = await db.select().from(posts).orderBy(desc(posts.createdAt));
const one = await db.select().from(posts).where(eq(posts.id, id)).limit(1);

// Or relational query API
const withAuthor = await db.query.posts.findFirst({
  where: eq(posts.id, id),
  with: { author: true },
});
```

Drizzle's relational API mirrors Prisma's developer experience without the runtime cost.

### 3.3 Migrations

**Prisma**:
```bash
npx prisma migrate dev --name add_tags     # dev
npx prisma migrate deploy                   # prod
```

**Drizzle**:
```bash
pnpm drizzle-kit generate                  # generate SQL
pnpm drizzle-kit migrate                   # apply
```

Always commit your migration files. Auto-applying migrations on deploy is fine for early projects; for serious work, run them as a separate CI step with proper review.

### 3.4 Server Action with DB write + cache invalidation

```ts
// lib/actions/posts.ts
"use server";
import { z } from "zod";
import { revalidateTag } from "next/cache";
import { auth } from "@/auth";
import { db } from "@/lib/db";

const CreatePost = z.object({
  title: z.string().min(3).max(120),
  body: z.string().min(10),
});

export async function createPost(_prev: unknown, formData: FormData) {
  const session = await auth();
  if (!session?.user?.id) return { ok: false, error: "Unauthorized" };

  const parsed = CreatePost.safeParse({
    title: formData.get("title"),
    body: formData.get("body"),
  });
  if (!parsed.success) return { ok: false, error: "Invalid input" };

  const post = await db.post.create({
    data: { ...parsed.data, authorId: session.user.id },
  });

  revalidateTag("posts");
  return { ok: true, id: post.id };
}
```

Pair with `unstable_cache(..., { tags: ["posts"] })` for the read side. Mutation → invalidation → next read rebuilds.

### 3.5 Transactions

```ts
// Prisma
await db.$transaction(async (tx) => {
  const user = await tx.user.create({ data: { email } });
  await tx.post.create({ data: { authorId: user.id, title, body } });
});

// Drizzle
await db.transaction(async (tx) => {
  const [user] = await tx.insert(users).values({ email }).returning();
  await tx.insert(posts).values({ authorId: user.id, title, body });
});
```

Wrap multi-step operations that must be atomic. Be wary of long-running transactions on serverless (timeouts).

### 3.6 Type-safe SQL (raw)

```ts
// Prisma
const result = await db.$queryRaw<{ id: string; count: bigint }[]>`
  SELECT author_id AS id, COUNT(*) AS count FROM "Post" GROUP BY author_id
`;

// Drizzle
import { sql } from "drizzle-orm";
const result = await db.execute<{ id: string; count: number }>(
  sql`SELECT author_id AS id, COUNT(*) AS count FROM posts GROUP BY author_id`
);
```

For analytics queries, drop to SQL; for CRUD, use the typed query builder.

---

## 4. Practical application — a small CMS slice with caching

```ts
// lib/db.ts
import "server-only";
import { PrismaClient } from "@prisma/client";
const g = globalThis as unknown as { prisma?: PrismaClient };
export const db = g.prisma ?? new PrismaClient();
if (process.env.NODE_ENV !== "production") g.prisma = db;
```

```ts
// lib/data/posts.ts
import "server-only";
import { cache } from "react";
import { unstable_cache, revalidateTag } from "next/cache";
import { db } from "@/lib/db";

export const getPostFresh = cache((id: string) =>
  db.post.findUnique({ where: { id }, include: { author: true } })
);

export const listPostsCached = unstable_cache(
  async () => db.post.findMany({
    orderBy: { createdAt: "desc" },
    include: { author: { select: { name: true } } },
  }),
  ["posts:list"],
  { revalidate: 300, tags: ["posts"] }
);

export function invalidatePosts() {
  revalidateTag("posts");
}
```

```tsx
// app/posts/page.tsx
import Link from "next/link";
import { listPostsCached } from "@/lib/data/posts";

export default async function PostsPage() {
  const posts = await listPostsCached();
  return (
    <ul className="space-y-2">
      {posts.map((p) => (
        <li key={p.id}>
          <Link href={`/posts/${p.id}`} className="block hover:underline">
            <h3 className="font-semibold">{p.title}</h3>
            <p className="text-sm text-neutral-600">by {p.author.name ?? "anon"}</p>
          </Link>
        </li>
      ))}
    </ul>
  );
}
```

```tsx
// app/posts/[id]/page.tsx
import { notFound } from "next/navigation";
import { getPostFresh } from "@/lib/data/posts";

export default async function PostPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const post = await getPostFresh(id);
  if (!post) notFound();
  return (
    <article>
      <h1 className="text-2xl font-bold">{post.title}</h1>
      <p className="mt-2 text-sm text-neutral-500">by {post.author.name ?? "anon"}</p>
      <p className="mt-4">{post.body}</p>
    </article>
  );
}
```

```ts
// lib/actions/posts.ts
"use server";
import { z } from "zod";
import { auth } from "@/auth";
import { db } from "@/lib/db";
import { invalidatePosts } from "@/lib/data/posts";
import { redirect } from "next/navigation";

const Schema = z.object({
  title: z.string().min(3).max(120),
  body: z.string().min(10),
});

export async function createPost(_prev: unknown, fd: FormData) {
  const session = await auth();
  if (!session?.user?.id) return { ok: false, error: "Unauthorized" };
  const parsed = Schema.safeParse({ title: fd.get("title"), body: fd.get("body") });
  if (!parsed.success) return { ok: false, error: "Invalid" };
  const post = await db.post.create({
    data: { ...parsed.data, authorId: session.user.id },
  });
  invalidatePosts();
  redirect(`/posts/${post.id}`);
}
```

Behavior:
- Index page uses `unstable_cache` with a 5-minute TTL and the `posts` tag.
- Detail page uses `react.cache` for per-request dedup (always fresh).
- Creating a post invalidates the `posts` tag — index rebuilds on next request.
- Server action enforces auth before writing.

---

## 5. Common mistakes & gotchas

### `new PrismaClient()` per import

Without the `globalThis` guard, every HMR reload spawns a new client. After 10 minutes of dev, you're at "too many connections" errors. Always use the cached-on-global pattern.

### N+1 queries from naive loops

```ts
// BAD
const posts = await db.post.findMany();
for (const p of posts) {
  p.author = await db.user.findUnique({ where: { id: p.authorId } });
}
```

```ts
// GOOD
const posts = await db.post.findMany({ include: { author: true } });
```

Or use `dataloader` patterns. Modern ORMs make this trap obvious — but it still happens in code reviews.

### Importing the DB client into a client component

`"use client"` + `import { db } from "@/lib/db"` will fail at build (with `server-only`) or pollute the bundle (without it). Always re-export query functions, never the raw client.

### Long transactions on serverless

Lambdas / Vercel functions have execution limits (~10s for Hobby plans). A transaction that holds a DB connection for 30s is doomed. Keep transactions short; for long-running work, use a queue.

### Forgetting to mark DB modules `"server-only"`

The package `server-only` will turn an accidental client import into a build error. Add it to every module that touches the DB or secrets.

### `unstable_cache` with closures

```ts
// BAD — the cached function closes over user, but the cache key doesn't include it
function getUserPosts(user: User) {
  return unstable_cache(
    () => db.post.findMany({ where: { authorId: user.id } }),
    ["user-posts"],  // same key for every user!
    { tags: ["posts"] }
  )();
}
```

```ts
// GOOD
function getUserPosts(userId: string) {
  return unstable_cache(
    () => db.post.findMany({ where: { authorId: userId } }),
    ["user-posts", userId],   // include the variant in the key
    { tags: ["posts", `user:${userId}`] }
  )();
}
```

### Returning DB objects with circular relations

Prisma `include`s can return objects with circular references (rare but possible). When you pass them across the RSC boundary, serialization fails. Map to a plain shape:

```ts
return { id: post.id, title: post.title, author: { name: post.author.name } };
```

### Running migrations in production at boot

Auto-migrating on every container start can cause race conditions when scaling. Run migrations as a CI/CD step or a separate one-shot job.

### Using Postgres `pg` from the edge

The `pg` package needs TCP sockets and Node's net module — not available on edge. Use `@neondatabase/serverless`, `@vercel/postgres`, or `postgres.js` (HTTP variant).

---

## 🎯 Key Takeaways

- **Querying directly from RSCs and Server Actions is the modern pattern.** Skip the API-route hop unless you have a public/external API need.
- **Cache the client on `globalThis` in dev**, and use serverless-friendly drivers (Neon, Vercel Postgres) or a connection pooler in production.
- **Layer the caches**: `react.cache()` for per-request dedup, `unstable_cache(..., { tags })` for persistent caching, `revalidateTag` from mutations for invalidation.
- **Prisma vs Drizzle is a real choice.** Prisma is more opinionated and friendlier; Drizzle is leaner and edge-friendly. Both are production-grade.
- **Always re-check auth & ownership inside Server Actions** before any write. The DB is the source of truth, not the UI.

*←* [`12_authentication.md`](./12_authentication.md) *|* *next →* [`14_route_handlers_and_api.md`](./14_route_handlers_and_api.md)
