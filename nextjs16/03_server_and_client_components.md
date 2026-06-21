# 03 — Server Components vs Client Components

> **Goal:** Build an unshakable mental model of the server/client boundary — when each runs, what crosses the wire, what `"use client"` actually does, and the composition patterns that keep client islands small and optimized.

---

## 1. Concept — the boundary

Every file under `app/` is, by default, a **React Server Component (RSC)**. RSCs:

- Run **only on the server** (during build for static routes, during request for dynamic routes).
- Can `await` directly (`async function Page()`).
- Can import server-only modules (`fs`, database drivers, environment secrets).
- **Ship zero JavaScript to the browser** for themselves.
- Cannot use hooks like `useState`, `useEffect`, `useReducer`, refs, event handlers, or any browser-only API.

A **Client Component** is any file with `"use client"` as its first non-comment line. Client components:

- Run on the **server** (for the initial HTML) *and* on the **client** (for hydration & interactivity).
- Ship their JS code to the browser.
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
3. The browser receives HTML + the RSC payload. React reads the payload, downloads the client component bundles, and **hydrates** them with the serialized props.

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

Functions, class instances, maps/sets, or elements with custom prototypes do not survive serialization. Plain objects, arrays, strings, numbers, booleans, null, and bigints do. (Dates serialize to ISO strings). If you try to pass a function from a server component to a client component, you will get an error: *"Functions cannot be passed directly to Client Components."*

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

Two packages help enforce the boundary at build time:

```bash
pnpm add server-only client-only
```

```ts
// lib/db.ts
import "server-only";
import { Pool } from "pg";

export const pool = new Pool({ connectionString: process.env.DATABASE_URL });
```

If anything client-side imports `@/lib/db`, the build fails immediately. Use this for anything that touches secrets or database drivers.

### 3.2 Server components rendering inside client components

Direct import is **not** supported, but pass-through via children **is**.

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

### 3.3 React 19: The `use()` Hook

Client components cannot be `async`. In React 19 / Next.js 16, you can stream data to client components using the stable `use()` hook. Pass a promise directly from a Server Component to a Client Component, and wrap the client component in `<Suspense>`.

```tsx
// app/PostsClient.tsx
"use client";
import { use } from "react";

export function PostsClient({ postsPromise }: { postsPromise: Promise<{ id: string; title: string }[]> }) {
  // use() suspends the client component until the server promise resolves
  const posts = use(postsPromise);
  return <ul>{posts.map((p) => <li key={p.id}>{p.title}</li>)}</ul>;
}
```

```tsx
// app/page.tsx (server)
import { PostsClient } from "./PostsClient";
import { Suspense } from "react";
import { fetchPosts } from "@/lib/posts"; // async fetch

export default function Page() {
  const postsPromise = fetchPosts();   // do NOT await — pass the promise
  return (
    <Suspense fallback={<p>Loading posts…</p>}>
      <PostsClient postsPromise={postsPromise} />
    </Suspense>
  );
}
```

This is the standard modern way to stream data into client interactive elements without blocking page rendering.

### 3.4 React Compiler: Automatic Memoization

Next.js 16 integrates the **React Compiler** (stable in React 19). The compiler parses your React code and automatically injects memoization where necessary.
- You no longer need to write `useMemo` or `useCallback` to prevent unnecessary client component re-renders.
- Keep client code clean and focus on business logic rather than manual dependency arrays.

### 3.5 Context providers across the boundary

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
    <html>
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
```

Children passed *through* the client provider remain server components. The provider only manages client interactivity.

---

## 4. Practical application — a product detail page with a sticky add-to-cart island

The page is server-rendered for SEO and speed; only the cart button is interactive.

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
        <RelatedProducts category={product.category} />
      </div>

      <aside className="sticky top-4 h-fit rounded border p-4">
        <p className="text-2xl font-semibold">${(product.priceCents / 100).toFixed(2)}</p>
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
import { addToCart } from "@/lib/actions/cart"; // a server action

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
            setMsg(`Added ${qty} × ${(unitPriceCents / 100).toFixed(2)} to cart`);
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
      <h2 className="mb-3 text-lg font-semibold">Related Products</h2>
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

---

## 5. Common mistakes & gotchas

### Sprinkling `"use client"` at the top of layouts
This converts layout components into client components. Keep layouts as server components. Use providers in a dedicated, isolated client component.

### Hydration mismatches
Mismatches occur when server-rendered HTML and client-rendered HTML differ.
- Do not read non-deterministic values (`window`, `Date.now()`, `Math.random()`) during render. Wrap these in `useEffect` or read them after mount.
- Pin your formatting locale to avoid mismatching dates/currencies.

### Passing non-serializable props to a client component
Never pass functions (other than Server Actions), closures, Maps, Sets, or complex classes. Pass raw data structures (objects, arrays, strings).

### Forgetting client components compile on the server too
The first render of a client component runs on the server to produce the initial HTML page. Do not access browser global APIs (`window`, `localStorage`) at the top level of the component body.

---

## 🎯 Key Takeaways

- **Isolate interactivity to the leaves.** Default to Server Components for UI shell, structure, and data.
- **`use()` Hook:** Use `use(promise)` inside Client Components to gracefully stream data.
- **No manual memoization:** Let the React Compiler handle performance optimization; avoid `useMemo` and `useCallback` by default.
- **Strict serialization:** Keep props crossing the boundaries simple and serializable.

*←* [`02_routing_fundamentals.md`](./02_routing_fundamentals.md) *|* *next →* [`04_data_fetching.md`](./04_data_fetching.md)
