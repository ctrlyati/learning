# 03 — Server Components vs Client Components

> **Goal:** Build an unshakable mental model of the server/client boundary — when each runs, what crosses the wire, what `"use client"` actually does, and the composition patterns that keep client islands small.

---

## 1. Concept — the boundary

Every file under `app/` is, by default, a **React Server Component (RSC)**. RSCs:

- Run **only on the server** (during build for static routes, during request for dynamic routes).
- Can `await` directly (`async function Page()`).
- Can import server-only modules (`fs`, `node:crypto`, database drivers).
- **Ship zero JavaScript to the browser** for themselves.
- Cannot use `useState`, `useEffect`, `useReducer`, refs, event handlers, or any browser-only API.

A **Client Component** is any file with `"use client"` as its first non-comment line. Client components:

- Run on the **server** (for the initial HTML) *and* on the **client** (for hydration & interactivity).
- Ship their JS to the browser.
- Can use all React hooks and DOM APIs.
- Cannot `await` at the top level (they're regular React function components).
- Cannot directly import server-only modules.

```tsx
// Server component (default) — runs on the server only
// app/posts/page.tsx
import { db } from "@/lib/db";

export default async function PostsPage() {
  const posts = await db.post.findMany();
  return (
    <ul>
      {posts.map((p) => <li key={p.id}>{p.title}</li>)}
    </ul>
  );
}
```

```tsx
// Client component — ships to the browser
// app/posts/LikeButton.tsx
"use client";
import { useState } from "react";

export function LikeButton({ initial }: { initial: number }) {
  const [count, setCount] = useState(initial);
  return <button onClick={() => setCount(count + 1)}>{count}</button>;
}
```

The server component can *render* the client component:

```tsx
// app/posts/page.tsx
import { LikeButton } from "./LikeButton";

export default async function PostsPage() {
  return (
    <div>
      <h1>Posts</h1>
      <LikeButton initial={0} />
    </div>
  );
}
```

That's the boundary in one screen. The rest of this module is consequences.

---

## 2. Mechanism — what crosses the wire

When the server renders a page:

1. It walks the React tree. For each **server component**, it renders to:
   - **HTML** (for first paint) — written into the streamed response.
   - **An RSC payload** entry — a compact serialized description of the component output.
2. When it hits a **client component**, it does NOT render its body. Instead it writes:
   - A *reference* to the client component's module (its build ID + bundle URL).
   - The serialized props passed to it.
3. The browser receives HTML + the RSC payload. React reads the payload, downloads the client component bundles, **hydrates** them with the serialized props.

```
Server                            Client
────────                          ────────
PostsPage (server)
  renders HTML
  serializes RSC tree:
    <Header />          ─────────►
    <LikeButton         ─────────►  hydrate LikeButton
       initial={0} />               with initial={0}
    <Footer />          ─────────►
```

Consequences of this model:

### Props passed across the boundary must be serializable

Functions, class instances, dates with custom prototypes — none of these survive. Plain objects, arrays, strings, numbers, booleans, null, and bigints do. (Dates serialize to ISO strings via the Flight protocol.) If you try to pass a function from a server component to a client component, you'll get an error: *"Functions cannot be passed directly to Client Components."*

### `children` is the magic escape hatch

A client component can render a server component if it receives it as `children` (or any prop holding JSX), because the server already rendered the JSX into the RSC payload — the client never sees the server component's *code*, only its output.

```tsx
// Client component that accepts server children
// app/components/Modal.tsx
"use client";
import { useState } from "react";

export function Modal({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>Open</button>
      {open && <dialog open>{children}</dialog>}
    </>
  );
}
```

```tsx
// Server component composes them
// app/page.tsx
import { Modal } from "@/components/Modal";
import { ExpensiveServerContent } from "@/components/ExpensiveServerContent"; // server component

export default function Page() {
  return (
    <Modal>
      <ExpensiveServerContent />
    </Modal>
  );
}
```

This is one of the most important composition patterns in App Router. **Push interactivity to the leaves; keep server components as the shell.**

### `"use client"` is contagious — but only down the import graph

If `A.tsx` has `"use client"` and imports `B.tsx` (no directive), `B.tsx` becomes a client module too. But if a *server component* imports `A.tsx` directly, only `A.tsx` is the boundary — anything `A.tsx` *receives as children* stays a server component. This is why composition via `children` works.

---

## 3. Variations / depth

### 3.1 `server-only` and `client-only` packages

Two tiny packages help enforce the boundary at build time:

```bash
pnpm add server-only client-only
```

```ts
// lib/db.ts
import "server-only";
import { Pool } from "pg";

export const pool = new Pool({ connectionString: process.env.DATABASE_URL });
```

If anything client-side imports `@/lib/db`, the build fails with a clear error. Use this religiously for anything that touches secrets, the database, or `node:` modules.

### 3.2 Server components rendering inside client components

A common confusion: "Can I import a server component into a client component?" Direct import: **no**. Pass-through via children: **yes**.

```tsx
// WRONG — Foo becomes a client component, breaks if it uses server-only APIs
// app/Wrapper.tsx
"use client";
import { Foo } from "./Foo"; // server component

export function Wrapper() {
  return <Foo />;
}
```

```tsx
// RIGHT — Wrapper accepts children; the server arranges the composition
// app/Wrapper.tsx
"use client";

export function Wrapper({ children }: { children: React.ReactNode }) {
  return <div className="wrap">{children}</div>;
}
```

```tsx
// app/page.tsx (server)
import { Wrapper } from "./Wrapper";
import { Foo } from "./Foo";

export default function Page() {
  return <Wrapper><Foo /></Wrapper>;
}
```

### 3.3 Async client components? No.

Client components cannot be `async` (the React renderer doesn't support top-level await in client function components). If you need data in a client component, either:

- Receive it as a prop from a server component (preferred), or
- Use the `use(promise)` hook (React 19) to suspend on a Promise prop, or
- Use a client-side data library (SWR, React Query) for client-driven fetching.

```tsx
// app/PostsClient.tsx
"use client";
import { use } from "react";

export function PostsClient({ postsPromise }: { postsPromise: Promise<{ id: string; title: string }[]> }) {
  const posts = use(postsPromise);
  return <ul>{posts.map((p) => <li key={p.id}>{p.title}</li>)}</ul>;
}
```

```tsx
// app/page.tsx (server)
import { PostsClient } from "./PostsClient";
import { Suspense } from "react";
import { fetchPosts } from "@/lib/posts";

export default function Page() {
  const postsPromise = fetchPosts();   // do NOT await — pass the promise
  return (
    <Suspense fallback={<p>Loading…</p>}>
      <PostsClient postsPromise={postsPromise} />
    </Suspense>
  );
}
```

This pattern — `use(promise)` on the client with a promise created on the server — is the modern way to stream data into client islands without blocking the rest of the page.

### 3.4 Context providers across the boundary

React Context is client-only. Wrap children in a client provider:

```tsx
// app/providers.tsx
"use client";
import { ThemeProvider } from "next-themes";
export function Providers({ children }: { children: React.ReactNode }) {
  return <ThemeProvider>{children}</ThemeProvider>;
}
```

```tsx
// app/layout.tsx (server)
import { Providers } from "./providers";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html><body>
      <Providers>{children}</Providers>
    </body></html>
  );
}
```

Children passed *through* the client provider remain server components. The provider only flips the "this subtree's interactive shell is client" bit.

### 3.5 Inspecting the boundary

In Next dev, the React DevTools can highlight client components. Better, run `pnpm build` and look at the per-route summary: the "First Load JS" column is the client-bundle weight. Any unexpected page weight tells you a `"use client"` slipped somewhere it shouldn't.

---

## 4. Practical application — a product detail page with a sticky add-to-cart island

This is a canonical pattern. The page is server-rendered for SEO and speed; only the cart button is interactive.

```tsx
// app/products/[id]/page.tsx (server)
import { notFound } from "next/navigation";
import { AddToCartButton } from "./AddToCartButton";
import { RelatedProducts } from "./RelatedProducts"; // server component
import { db } from "@/lib/db";

export default async function ProductPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const product = await db.product.findUnique({ where: { id } });
  if (!product) notFound();

  return (
    <article className="grid grid-cols-[1fr_320px] gap-8">
      <div>
        <h1 className="text-3xl font-bold">{product.name}</h1>
        <p className="mt-4 text-neutral-700">{product.description}</p>
        {/* Server component nested inside the same page */}
        <RelatedProducts category={product.category} />
      </div>

      <aside className="sticky top-4 h-fit rounded border p-4">
        <p className="text-2xl font-semibold">${(product.priceCents / 100).toFixed(2)}</p>
        {/* Client island — interactivity isolated to this leaf */}
        <AddToCartButton
          productId={product.id}
          unitPriceCents={product.priceCents}
        />
      </aside>
    </article>
  );
}
```

```tsx
// app/products/[id]/AddToCartButton.tsx
"use client";
import { useState, useTransition } from "react";
import { addToCart } from "@/lib/actions/cart"; // a server action — see Module 05

export function AddToCartButton({
  productId,
  unitPriceCents,
}: {
  productId: string;
  unitPriceCents: number;
}) {
  const [qty, setQty] = useState(1);
  const [pending, startTransition] = useTransition();
  const [msg, setMsg] = useState<string | null>(null);

  return (
    <div className="mt-4 space-y-2">
      <input
        type="number"
        min={1}
        value={qty}
        onChange={(e) => setQty(Number(e.target.value))}
        className="w-16 rounded border px-2 py-1"
      />
      <button
        disabled={pending}
        onClick={() => {
          startTransition(async () => {
            await addToCart({ productId, qty });
            setMsg(`Added ${qty} × ${unitPriceCents} ¢`);
          });
        }}
        className="w-full rounded bg-black px-3 py-2 text-white disabled:opacity-50"
      >
        {pending ? "Adding…" : "Add to cart"}
      </button>
      {msg && <p className="text-sm text-green-700">{msg}</p>}
    </div>
  );
}
```

```tsx
// app/products/[id]/RelatedProducts.tsx (server)
import Link from "next/link";
import { db } from "@/lib/db";

export async function RelatedProducts({ category }: { category: string }) {
  const items = await db.product.findMany({
    where: { category },
    take: 4,
  });
  return (
    <section className="mt-12">
      <h2 className="mb-3 text-lg font-semibold">Related</h2>
      <ul className="grid grid-cols-4 gap-3">
        {items.map((p) => (
          <li key={p.id}>
            <Link href={`/products/${p.id}`} className="block rounded border p-2">
              {p.name}
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
```

The page ships *only* the JS for `AddToCartButton` and the server action runtime. The product copy, related products, and layout are HTML + tiny RSC payload. This is what "default to server, opt into client" buys you.

---

## 5. Common mistakes & gotchas

### Sprinkling `"use client"` at the top of `layout.tsx`

This converts your entire subtree into a client tree at the import level — children passed through still work, but you've now made the layout shell client-rendered for no reason. Layouts should almost always be server components. If you need a provider, wrap *just the provider* as a client component.

### Hydration mismatches

Hydration errors happen when the server-rendered HTML and the client-rendered HTML disagree. Top causes:

- **Reading `window`, `Date.now()`, `Math.random()` during render.** Use `useEffect` for those, or feed the value in as a prop from the server.
- **Browser extensions** injecting attributes (Grammarly's `data-gramm` is infamous). Add `suppressHydrationWarning` on the `<body>` if needed.
- **Locale-sensitive number/date formatting** with different `Intl` defaults between server and client. Pin the locale.

### Passing non-serializable props to a client component

```tsx
// WRONG
<ClientThing onClick={() => doStuff()} />   // function across the boundary
<ClientThing data={new Map([["a", 1]])} />  // Map doesn't serialize
```

```tsx
// RIGHT
<ClientThing data={[["a", 1]]} />           // plain array
// For "callbacks", pass a server action (a stable string id) instead of a closure.
```

### Forgetting that client components also render on the server

The "first render" of a client component runs *on the server* to produce HTML. So if your client component does `localStorage.getItem(...)` during render, it will crash. Wrap browser-only reads in `useEffect`.

### Importing a server-only module into a client file

```tsx
// app/CartCount.tsx
"use client";
import { db } from "@/lib/db";  // ❌ leaks the DB driver into the bundle
```

`server-only` will turn this into a build-time error. Always use it on modules that touch secrets or `node:` APIs.

### Reaching for `useEffect + fetch` out of React habit

If you're fetching data on mount in a client component, ask: *can this be a server component that fetches directly?* 90% of the time, yes. Doing it server-side gives you smaller bundle, faster paint, no loading flash, and SEO.

---

## 🎯 Key Takeaways

- **The boundary is the most important line in your code.** Default to server; opt into client only at the smallest interactive leaf.
- **`"use client"` is contagious down the import graph, but not through `children`.** Use the children-pass-through pattern to keep client islands small and server content composable.
- **Props across the boundary must be serializable.** Functions don't cross — use server actions. Dates serialize; Maps and class instances don't (cleanly).
- **`server-only` and `client-only` are cheap insurance.** Use them on every module that should only ever live on one side.
- **Hydration errors almost always mean "server HTML ≠ client HTML"**, which almost always means a non-deterministic read (random, time, `window`) during render. Pin the value, or defer to `useEffect`.

*←* [`02_routing_fundamentals.md`](./02_routing_fundamentals.md) *|* *next →* [`04_data_fetching.md`](./04_data_fetching.md)
