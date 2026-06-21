# 13 — Database & ORM Integration

> **Goal:** Connect database ORMs (Prisma or Drizzle) to a Next.js 16 project, write high-performance queries inside Server Components and Server Actions, and integrate them with the modern `"use cache"` directive.

---

## 1. Concept — Querying Directly from Server Components

In the App Router model, React Server Components can run database queries directly. You do not need to create API endpoints or route handlers simply to fetch database records for rendering.

```tsx
// src/app/posts/page.tsx
import { db } from "@/lib/db";

export default async function PostsPage() {
  const posts = await db.post.findMany({
    orderBy: { createdAt: "desc" }
  });

  return (
    <main>
      <h1>Latest Posts</h1>
      <ul>
        {posts.map((post) => (
          <li key={post.id} className="border-b py-2">
            <h2>{post.title}</h2>
          </li>
        ))}
      </ul>
    </main>
  );
}
```

The data query runs securely on your database servers during the Server Component render cycle, streaming pre-rendered HTML back to the browser.

---

## 2. Connection Management & Runtimes

### 2.1 Developer Hot-Reloading (HMR)
In local development, Turbopack reloads modules upon save events. If your database connection client is instantiated at the top level of a file, every reload will open a new database connection client, quickly exhausting your database's connection pool.

Avoid this by caching the client on the global namespace during local development:

```typescript
// src/lib/db.ts (Prisma setup)
import "server-only";
import { PrismaClient } from "@prisma/client";

const globalForPrisma = globalThis as unknown as { prisma?: PrismaClient };

export const db = globalForPrisma.prisma ?? new PrismaClient({
  log: ["error", "warn"],
});

if (process.env.NODE_ENV !== "production") {
  globalForPrisma.prisma = db;
}
```

### 2.2 Serverless Connection Limits
In serverless environments, individual containers spin up and down to handle traffic. This can result in many concurrent instances opening independent connections to your database.
- **Connection Poolers:** Use poolers (like PgBouncer or those provided natively by Supabase and Neon) to multiplex connections.
- **WebSocket/HTTP Clients:** Use lightweight HTTP drivers (such as `@neondatabase/serverless` or `@vercel/postgres`) designed for high-concurrency environments.

---

## 3. Persistent Query Caching with `"use cache"`

In Next.js 16, you cache database queries using the **`"use cache"`** directive. This eliminates the need for manual `react.cache()` or `unstable_cache()` boilerplate.

### 3.1 Caching Database Reads

```typescript
// src/lib/data/posts.ts
import "server-only";
import { db } from "@/lib/db";
import { cacheLife, cacheTag } from "next/cache";

export async function getPostsCached() {
  "use cache";
  cacheLife("minutes"); // Cache for a few minutes
  cacheTag("posts");    // Associate with the "posts" tag

  return db.post.findMany({
    orderBy: { createdAt: "desc" },
    include: { author: { select: { name: true } } },
  });
}

export async function getPostDetail(id: string) {
  "use cache";
  cacheLife("days");
  cacheTag(`post:${id}`, "posts");

  return db.post.findUnique({
    where: { id },
  });
}
```

### 3.2 Cache Invalidation on Mutations

When writing database records inside Server Actions, clear the cached queries instantly using **`updateTag()`**:

```typescript
// src/lib/actions/posts.ts
"use server";
import { z } from "zod";
import { updateTag } from "next/cache";
import { db } from "@/lib/db";
import { auth } from "@/auth";

const PostSchema = z.object({
  title: z.string().min(3).max(120),
  body: z.string().min(10),
});

export async function createPost(_prev: unknown, formData: FormData) {
  const session = await auth();
  if (!session?.user?.id) {
    return { ok: false, error: "Unauthorized" };
  }

  const parsed = PostSchema.safeParse({
    title: formData.get("title"),
    body: formData.get("body"),
  });

  if (!parsed.success) {
    return { ok: false, error: "Invalid form inputs" };
  }

  await db.post.create({
    data: {
      ...parsed.data,
      authorId: session.user.id,
    },
  });

  // Evict cache immediately so list shows the new post on redirect
  updateTag("posts");

  return { ok: true };
}
```

---

## 4. Drizzle ORM (SQL-native Alternative)

Drizzle is a lightweight, SQL-like ORM that is fully compatible with Edge runtimes:

```typescript
// src/db/schema.ts
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

Querying with Drizzle:

```typescript
import { db } from "@/db";
import { posts } from "@/db/schema";
import { desc, eq } from "drizzle-orm";

// Select posts list
const allPosts = await db.select().from(posts).orderBy(desc(posts.createdAt));
```

Drizzle transactions:

```typescript
await db.transaction(async (tx) => {
  const [user] = await tx.insert(users).values({ email: "user@example.com" }).returning();
  await tx.insert(posts).values({ title: "First Post", body: "Hello", authorId: user.id });
});
```

---

## 5. Common Mistakes & Gotchas

### N+1 Query Loading Loops
Avoid fetching child references in dynamic loops, which causes sequential queries. Fetch relationships eagerly instead:

```typescript
// WRONG
const articles = await db.post.findMany();
for (const a of articles) {
  a.author = await db.user.findUnique({ where: { id: a.authorId } }); // N queries!
}

// CORRECT
const articles = await db.post.findMany({ include: { author: true } }); // Single join query!
```

### Leaking Database Drivers to Client Code
Always make sure database initialization files are protected with the `import "server-only";` declaration. This guarantees that your database connection credentials and drivers cannot be accidentally imported into Client Component bundles, which would trigger a build error.

### Cache Key Collisions with closures
When caching parametric database queries, pass the variables explicitly to ensure separate cache keys:

```typescript
// WRONG - Caches under a single global key
export function getProduct(id: string) {
  "use cache";
  return db.product.findUnique({ where: { id } });
}

// CORRECT - Parameter-scoped unique keys
export async function getProduct(id: string) {
  "use cache";
  cacheTag(`product:${id}`);
  return db.product.findUnique({ where: { id } });
}
```

---

## 🎯 Key Takeaways

- **Server-side query execution:** Fetch database records directly within Server Components and Server Actions.
- **Cache Client globally:** Use global connection mapping locally to prevent HMR socket exhaustion.
- **Declarative Caching:** Place `"use cache"` inside query functions, setting cache tags and profiles using `cacheTag()` and `cacheLife()`.
- **Authorized mutations:** Always verify ownership and authentication tokens inside Server Actions before mutating database records.

*←* [`12_authentication.md`](./12_authentication.md) *|* *next →* [`14_route_handlers_and_api.md`](./14_route_handlers_and_api.md)
