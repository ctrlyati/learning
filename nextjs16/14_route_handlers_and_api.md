# 14 — Route Handlers & APIs

> **Goal:** Build typed HTTP endpoints using Next.js 16 Route Handlers (GET, POST, streaming, and runtimes) and understand when to choose handlers over Server Actions.

---

## 1. Concept — Route Handlers

Any folder under `src/app/` containing a `route.ts` (or `route.js`) file acts as a public HTTP API endpoint. Export asynchronous functions named after standard HTTP methods (GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD).

```typescript
// src/app/api/health/route.ts
import { NextResponse } from "next/server";

export async function GET() {
  return NextResponse.json({ ok: true, timestamp: Date.now() });
}
```

Dynamic route segments work identically to page parameters, receiving params as Promises:

```typescript
// src/app/api/posts/[id]/route.ts
import { NextResponse } from "next/server";
import { db } from "@/lib/db";

export async function GET(_req: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const post = await db.post.findUnique({ where: { id } });

  if (!post) {
    return NextResponse.json({ error: "Post not found" }, { status: 404 });
  }

  return NextResponse.json(post);
}
```

*Note: A folder cannot contain both a `page.tsx` and a `route.ts` file. A route segment must be either a public webpage or an HTTP endpoint.*

---

## 2. Mechanism — Web Standards

Next.js Route Handlers extend standard web primitives: `Request`, `Response`, `Headers`, and `URL`. You can return custom Web API `Response` objects or use the `NextResponse` wrapper helpers.

By default, Route Handlers run on the **Node.js runtime**. You can configure them to run on the **Edge runtime** instead:

```typescript
export const runtime = "edge"; // Or "nodejs"
```

### Parsing Requests

```typescript
import { type NextRequest } from "next/server";

export async function POST(req: NextRequest) {
  const url = req.nextUrl; // Parsed request URL
  const queryParam = url.searchParams.get("query");

  const jsonBody = await req.json(); // Parses application/json payload
  const formData = await req.formData(); // Parses multipart/form-data
  const rawText = await req.text(); // Parses string payload

  const sessionCookie = req.cookies.get("session")?.value;
  const userAgent = req.headers.get("user-agent");
}
```

---

## 3. Operations & Patterns

### 3.1 Caching GET Handlers
In Next.js 16, you can cache GET Route Handlers using the **`"use cache"`** directive. This stores the generated Response payload on the server:

```typescript
// src/app/api/posts/route.ts
import { NextResponse } from "next/server";
import { db } from "@/lib/db";
import { cacheLife, cacheTag } from "next/cache";

export async function GET() {
  "use cache";
  cacheLife("minutes");
  cacheTag("posts");

  const posts = await db.post.findMany({ take: 20 });
  return NextResponse.json(posts);
}
```

### 3.2 Streaming Server-Sent Events (SSE)
You can stream data chunk-by-chunk using readable streams. This is useful for AI completions or live dashboard indicators:

```typescript
// src/app/api/stream/route.ts
export const runtime = "edge";

export async function GET() {
  const stream = new ReadableStream({
    async start(controller) {
      const encoder = new TextEncoder();
      const sendEvent = (event: string, data: string) => {
        controller.enqueue(encoder.encode(`event: ${event}\ndata: ${data}\n\n`));
      };

      sendEvent("tick", "1");
      await new Promise((r) => setTimeout(r, 1000));
      sendEvent("tick", "2");
      controller.close();
    },
  });

  return new Response(stream, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-store",
      "Connection": "keep-alive",
    },
  });
}
```

### 3.3 Server Actions vs. Route Handlers

| Scenario | Server Action | Route Handler |
| :--- | :--- | :--- |
| **Mutations from UI forms** | Yes (Progressive Enhancement) | No |
| **Integrations for Mobile / Webhooks** | No | Yes |
| **Cross-Origin Requests (CORS)** | No | Yes |
| **WebSockets / SSE Streams** | No | Yes |

*Rule of thumb: Use **Server Actions** for internal UI-driven data mutations. Use **Route Handlers** for public API design, webhooks, cross-origin resources, or custom streaming.*

---

## 4. Practical Application — Typed Post API with CORS

```typescript
// src/app/api/posts/route.ts
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { db } from "@/lib/db";
import { auth } from "@/auth";
import { updateTag } from "next/cache";

const PostSchema = z.object({
  title: z.string().min(3).max(120),
  body: z.string().min(10),
});

export async function POST(req: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const json = await req.json().catch(() => null);
  const parsed = PostSchema.safeParse(json);
  if (!parsed.success) {
    return NextResponse.json({ errors: parsed.error.flatten() }, { status: 400 });
  }

  const post = await db.post.create({
    data: {
      ...parsed.data,
      authorId: session.user.id,
    },
  });

  updateTag("posts");

  return NextResponse.json(post, {
    status: 201,
    headers: {
      "Access-Control-Allow-Origin": "*",
    },
  });
}

// OPTIONS handler for CORS preflight
export async function OPTIONS() {
  return new NextResponse(null, {
    status: 204,
    headers: {
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": "POST, OPTIONS",
      "Access-Control-Allow-Headers": "Content-Type, Authorization",
    },
  });
}
```

---

## 5. Common Mistakes & Gotchas

### localhost Fetch Loop
Never fetch your own Next.js API Route Handlers inside Server Components (e.g. `await fetch('/api/posts')`). Doing so initiates an unnecessary localhost HTTP request loop on the server. Import and run your database query logic directly:

```typescript
// WRONG
export default async function Page() {
  const posts = await fetch("http://localhost:3000/api/posts").then((res) => res.json());
}

// CORRECT
import { getPosts } from "@/lib/data/posts";
export default async function Page() {
  const posts = await getPosts();
}
```

### Missing CORS Preflights
If third-party sites access your route handlers via browsers, your calls will fail unless you declare both the target handler logic and corresponding `OPTIONS` preflight headers.

---

## 🎯 Key Takeaways

- **Web Standard APIs:** Route Handlers expose native `Request` and `Response` interfaces.
- **`"use cache"` Support:** You can cache GET response payloads directly using the `"use cache"` directive.
- **Server Action Separation:** Actions are for internal mutations; Route Handlers are for APIs, webhooks, and streaming.

*←* [`13_database_and_orm.md`](./13_database_and_orm.md) *|* *next →* [`15_testing.md`](./15_testing.md)
