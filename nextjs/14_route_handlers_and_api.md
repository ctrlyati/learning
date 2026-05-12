# 14 — Route Handlers & APIs

> **Goal:** Build typed HTTP endpoints with route handlers — including GET, POST, streaming, edge vs Node runtime — and know when to use a handler vs a Server Action.

---

## 1. Concept — `route.ts` *is* the API

In `app/`, any folder containing `route.ts` (or `.js`) becomes an HTTP endpoint. Export one async function per method:

```ts
// app/api/health/route.ts
import { NextResponse } from "next/server";

export async function GET() {
  return NextResponse.json({ ok: true, ts: Date.now() });
}
```

Visit `http://localhost:3000/api/health` — JSON response. No middleware ceremony, no routing config.

```ts
// app/api/posts/route.ts
import { NextRequest, NextResponse } from "next/server";
import { db } from "@/lib/db";

export async function GET(req: NextRequest) {
  const q = req.nextUrl.searchParams.get("q") ?? "";
  const posts = await db.post.findMany({
    where: { title: { contains: q, mode: "insensitive" } },
    take: 20,
  });
  return NextResponse.json(posts);
}

export async function POST(req: NextRequest) {
  const body = await req.json();
  const post = await db.post.create({ data: body });
  return NextResponse.json(post, { status: 201 });
}
```

Dynamic segments work too:

```ts
// app/api/posts/[id]/route.ts (Next 15)
import { NextResponse } from "next/server";
import { db } from "@/lib/db";

export async function GET(_req: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const post = await db.post.findUnique({ where: { id } });
  if (!post) return NextResponse.json({ error: "not found" }, { status: 404 });
  return NextResponse.json(post);
}

export async function DELETE(_req: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  await db.post.delete({ where: { id } });
  return new NextResponse(null, { status: 204 });
}
```

Note: `route.ts` and `page.tsx` **cannot coexist** in the same folder. A segment is either a page or an endpoint.

---

## 2. Mechanism — Web standards, not Express

Route handlers use the **Web Fetch API** primitives: `Request`, `Response`, `Headers`, `URL`. Next.js wraps these as `NextRequest` / `NextResponse` with helpers, but you can return a plain `Response`.

The handler runs on the **Node runtime** by default, or **edge** if you opt in:

```ts
export const runtime = "edge";   // or "nodejs"
```

Other route segment options:
```ts
export const dynamic = "force-dynamic";
export const revalidate = 60;
export const maxDuration = 30;
```

### Reading request data

```ts
export async function POST(req: NextRequest) {
  const url = req.nextUrl;              // parsed URL
  const path = url.pathname;
  const q = url.searchParams.get("q");

  const json = await req.json();        // parse JSON body
  const fd = await req.formData();      // parse multipart/form-data
  const text = await req.text();        // raw text
  const buf = await req.arrayBuffer();  // raw bytes

  const cookie = req.cookies.get("session")?.value;
  const ua = req.headers.get("user-agent");
}
```

You can only consume the body **once**. Pick the parser that matches the content-type.

### Returning responses

```ts
// JSON
return NextResponse.json({ ok: true });
return NextResponse.json({ ok: true }, { status: 201, headers: { "x-id": "1" } });

// Plain text
return new NextResponse("hello", { status: 200 });

// Redirect
return NextResponse.redirect(new URL("/login", req.url));

// Streaming
return new NextResponse(readableStream, { headers: { "content-type": "text/event-stream" } });

// File / binary
return new NextResponse(buffer, { headers: { "content-type": "image/png" } });
```

---

## 3. Variations / depth

### 3.1 Validation with Zod

```ts
// app/api/posts/route.ts
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { db } from "@/lib/db";

const CreatePost = z.object({
  title: z.string().min(3).max(120),
  body: z.string().min(10),
});

export async function POST(req: NextRequest) {
  const raw = await req.json().catch(() => null);
  const parsed = CreatePost.safeParse(raw);
  if (!parsed.success) {
    return NextResponse.json(
      { error: parsed.error.flatten() },
      { status: 400 }
    );
  }
  const post = await db.post.create({ data: parsed.data });
  return NextResponse.json(post, { status: 201 });
}
```

### 3.2 Streaming responses

Stream chunks as they're produced — for AI completions, large JSON, server-sent events:

```ts
// app/api/stream/route.ts
export const runtime = "edge";

export async function GET() {
  const stream = new ReadableStream({
    async start(controller) {
      const encoder = new TextEncoder();
      for (let i = 1; i <= 5; i++) {
        controller.enqueue(encoder.encode(`chunk ${i}\n`));
        await new Promise((r) => setTimeout(r, 500));
      }
      controller.close();
    },
  });
  return new Response(stream, {
    headers: {
      "content-type": "text/plain; charset=utf-8",
      "cache-control": "no-store",
    },
  });
}
```

For Server-Sent Events specifically:

```ts
export const runtime = "edge";

export async function GET() {
  const stream = new ReadableStream({
    async start(controller) {
      const enc = new TextEncoder();
      const send = (event: string, data: string) =>
        controller.enqueue(enc.encode(`event: ${event}\ndata: ${data}\n\n`));
      send("tick", "1");
      await new Promise((r) => setTimeout(r, 1000));
      send("tick", "2");
      controller.close();
    },
  });
  return new Response(stream, {
    headers: {
      "content-type": "text/event-stream",
      "cache-control": "no-store",
      "connection": "keep-alive",
    },
  });
}
```

Client side:
```ts
const es = new EventSource("/api/stream");
es.addEventListener("tick", (e) => console.log(e.data));
```

### 3.3 Edge vs Node — picking per route

Edge runtime gives you:
- Faster cold start (V8 isolate).
- Streaming-friendly (built for it).
- Smaller bundle limit.
- No Node APIs (`node:fs`, `node:net`, etc.).

Choose edge for:
- Streaming AI handlers (`/api/chat`).
- Lightweight redirect/auth endpoints.
- High-frequency, low-cost handlers.

Choose Node for:
- Anything using Prisma (non-edge), bcrypt with native bindings, `sharp`, etc.
- Long-running computation.
- Heavy DB operations with persistent pool.

### 3.4 Caching route handlers

By default, GET handlers **are not cached** in Next 14.x+ (this changed; older docs may say otherwise). Opt in explicitly:

```ts
export const dynamic = "force-static";
// or rely on data-cache via fetch options inside the handler
```

For data-fetch caching *within* a handler:

```ts
export async function GET() {
  const data = await fetch("https://api.example.com/data", {
    next: { revalidate: 60 },
  }).then((r) => r.json());
  return NextResponse.json(data);
}
```

### 3.5 CORS

Route handlers don't add CORS headers automatically. For cross-origin APIs:

```ts
export async function GET() {
  return NextResponse.json({ ok: true }, {
    headers: {
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": "GET",
    },
  });
}

export async function OPTIONS() {
  return new NextResponse(null, {
    status: 204,
    headers: {
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
      "Access-Control-Allow-Headers": "content-type, authorization",
      "Access-Control-Max-Age": "86400",
    },
  });
}
```

For finer control, set CORS in middleware.

### 3.6 File uploads

```ts
// app/api/upload/route.ts
import { NextRequest, NextResponse } from "next/server";
import { writeFile } from "fs/promises";
import { join } from "path";

export const runtime = "nodejs";   // file system is Node-only

export async function POST(req: NextRequest) {
  const fd = await req.formData();
  const file = fd.get("file");
  if (!(file instanceof File)) {
    return NextResponse.json({ error: "missing file" }, { status: 400 });
  }
  const buf = Buffer.from(await file.arrayBuffer());
  await writeFile(join("/tmp", file.name), buf);
  return NextResponse.json({ ok: true, size: buf.length });
}
```

For production, never write to the local FS — upload to S3 / R2 / Vercel Blob with a presigned URL or direct stream.

### 3.7 When to use a Route Handler vs a Server Action

| Use a **Server Action** when... | Use a **Route Handler** when... |
|--------------------------------|--------------------------------|
| You're mutating data from a form | You're building a public API |
| The caller is your own Next.js UI | You need to be called by mobile clients, webhooks, third-parties |
| You want progressive enhancement | You need streaming/SSE/WebSockets |
| You want auto-CSRF | You need CORS for cross-origin callers |
| It's a one-off action            | You need REST-style verbs (GET/POST/DELETE/PATCH) |

The rule of thumb: **internal mutations → Server Actions, external APIs → Route Handlers, streaming → Route Handlers (edge).**

---

## 4. Practical application — a typed API for posts plus a streaming chat endpoint

```ts
// app/api/posts/route.ts  (CRUD)
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/auth";
import { db } from "@/lib/db";
import { revalidateTag } from "next/cache";

const CreatePost = z.object({
  title: z.string().min(3).max(120),
  body: z.string().min(10),
});

export async function GET(req: NextRequest) {
  const q = req.nextUrl.searchParams.get("q") ?? "";
  const take = Math.min(Number(req.nextUrl.searchParams.get("take") ?? "20"), 100);
  const posts = await db.post.findMany({
    where: q ? { title: { contains: q, mode: "insensitive" } } : undefined,
    take,
    orderBy: { createdAt: "desc" },
  });
  return NextResponse.json(posts);
}

export async function POST(req: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }
  const parsed = CreatePost.safeParse(await req.json().catch(() => null));
  if (!parsed.success) {
    return NextResponse.json({ error: parsed.error.flatten() }, { status: 400 });
  }
  const post = await db.post.create({
    data: { ...parsed.data, authorId: session.user.id },
  });
  revalidateTag("posts");
  return NextResponse.json(post, { status: 201 });
}
```

```ts
// app/api/posts/[id]/route.ts
import { NextResponse } from "next/server";
import { auth } from "@/auth";
import { db } from "@/lib/db";
import { revalidateTag } from "next/cache";

export async function GET(_req: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const post = await db.post.findUnique({ where: { id } });
  return post
    ? NextResponse.json(post)
    : NextResponse.json({ error: "not found" }, { status: 404 });
}

export async function DELETE(_req: Request, { params }: { params: Promise<{ id: string }> }) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }
  const { id } = await params;
  const post = await db.post.findUnique({ where: { id } });
  if (!post) return NextResponse.json({ error: "not found" }, { status: 404 });
  if (post.authorId !== session.user.id) {
    return NextResponse.json({ error: "Forbidden" }, { status: 403 });
  }
  await db.post.delete({ where: { id } });
  revalidateTag("posts");
  return new NextResponse(null, { status: 204 });
}
```

```ts
// app/api/chat/route.ts  (streaming, edge)
export const runtime = "edge";

export async function POST(req: Request) {
  const { prompt } = (await req.json()) as { prompt: string };

  // Simulate token-by-token streaming (replace with OpenAI/Anthropic SDK)
  const stream = new ReadableStream({
    async start(controller) {
      const enc = new TextEncoder();
      const tokens = `You asked: ${prompt}. Here is a streamed reply.`.split(" ");
      for (const t of tokens) {
        controller.enqueue(enc.encode(t + " "));
        await new Promise((r) => setTimeout(r, 80));
      }
      controller.close();
    },
  });

  return new Response(stream, {
    headers: {
      "content-type": "text/plain; charset=utf-8",
      "cache-control": "no-store",
    },
  });
}
```

```tsx
// app/chat/ChatBox.tsx
"use client";
import { useState } from "react";

export function ChatBox() {
  const [out, setOut] = useState("");
  const [prompt, setPrompt] = useState("");

  async function send() {
    setOut("");
    const res = await fetch("/api/chat", {
      method: "POST",
      body: JSON.stringify({ prompt }),
    });
    const reader = res.body!.getReader();
    const dec = new TextDecoder();
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      setOut((s) => s + dec.decode(value));
    }
  }

  return (
    <div>
      <input value={prompt} onChange={(e) => setPrompt(e.target.value)} />
      <button onClick={send}>Send</button>
      <pre className="mt-2 whitespace-pre-wrap">{out}</pre>
    </div>
  );
}
```

Together: REST-style API on Node, streaming AI endpoint on edge, all in one project, both typed and validated.

---

## 5. Common mistakes & gotchas

### `route.ts` and `page.tsx` in the same folder

Build error. Pick one role per segment.

### Reading the body twice

```ts
const json = await req.json();
const text = await req.text(); // error: body already used
```

Read once, store, reuse.

### Forgetting to validate input

Route handlers are public endpoints. Anyone can `curl` them with any body. Always validate (Zod, valibot) and check auth.

### Returning Response with `application/json` content-type, but body is a string

```ts
return new Response(JSON.stringify(x), { headers: { "content-type": "application/json" } });
// fine, but easier:
return NextResponse.json(x);
```

### `runtime = "edge"` with `import { db } from "@/lib/db"` (Node Prisma)

Build error. Either switch to an edge-compatible driver, or remove the `edge` runtime export.

### Caching POST handlers

`fetch` cache and route segment config don't cache mutation endpoints. POST is always dynamic — that's correct, but new developers expect it to "auto-cache."

### Calling route handlers from your own server components

You *can* do `fetch("/api/posts")` from a server component, but it's wasteful — a localhost HTTP hop to call your own code. Call the data layer (`getPosts()`) directly. Route handlers are for *external* consumers.

### Forgetting `cache-control: no-store` on streaming

Without it, some proxies buffer the stream. Always set `cache-control: no-store` on streamed responses, plus appropriate content type.

### Throwing inside a handler

Throwing returns a 500 with a generic message. Catch and return structured errors:

```ts
try { /* ... */ } catch (e) {
  console.error(e);
  return NextResponse.json({ error: "Server error" }, { status: 500 });
}
```

---

## 🎯 Key Takeaways

- **Route Handlers are Web-standard HTTP endpoints.** Use `NextRequest` / `NextResponse`, or plain `Request` / `Response`.
- **Use handlers for external APIs and streaming.** For internal mutations from your own UI, prefer Server Actions — better DX, automatic CSRF, progressive enhancement.
- **Edge runtime for streaming and lightweight handlers, Node for heavy DB/native deps.** Choose per route with `export const runtime`.
- **Validate inputs and check auth in every mutation handler.** They're public; treat the inputs as hostile.
- **Don't `fetch` your own route handlers from server components.** Call your data-layer functions directly — same data, no HTTP hop.

*←* [`13_database_and_orm.md`](./13_database_and_orm.md) *|* *next →* [`15_testing.md`](./15_testing.md)
