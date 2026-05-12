# 05 — Server Actions & Mutations

> **Goal:** Use Server Actions for forms and mutations with progressive enhancement, validation, optimistic UI, and proper cache invalidation — and understand exactly what they compile to.

---

## 1. Concept — `"use server"` and forms that work without JavaScript

A **Server Action** is an async function with the `"use server"` directive (at top of file, or inline as the first line of the function). It runs *only* on the server. You can pass a reference to it to a client form via the `action` prop — Next.js will set up the network plumbing for you.

```tsx
// app/contact/page.tsx
async function submitContact(formData: FormData) {
  "use server";
  const name = String(formData.get("name") ?? "");
  const email = String(formData.get("email") ?? "");
  // ... persist to db
  console.log("Contact:", name, email);
}

export default function ContactPage() {
  return (
    <form action={submitContact} className="space-y-2">
      <input name="name" placeholder="Name" required />
      <input name="email" type="email" placeholder="Email" required />
      <button type="submit">Send</button>
    </form>
  );
}
```

Submit the form with JavaScript disabled in your browser — it still works. The browser does a native form POST to the same URL; Next.js dispatches to `submitContact` server-side. This is **progressive enhancement** out of the box: forms that work as plain HTML forms first, then upgrade to no-reload interactions when JS is available.

Server Actions can also be called from client components as plain async functions:

```tsx
// app/components/Like.tsx
"use client";
import { useTransition } from "react";
import { likePost } from "@/lib/actions/posts"; // a server action

export function Like({ postId }: { postId: string }) {
  const [pending, start] = useTransition();
  return (
    <button
      disabled={pending}
      onClick={() => start(() => likePost(postId))}
    >
      {pending ? "…" : "Like"}
    </button>
  );
}
```

The call *looks* synchronous and local. Under the hood, it's a POST request with a stable function ID.

---

## 2. Mechanism — what a Server Action actually is

When you mark a function with `"use server"`, Next.js does the following at build time:

1. Generates a **stable ID** for the function (a hash) and registers it in a server-only registry.
2. In the client bundle, replaces the import with a thin proxy: when called, it serializes the arguments (FormData or JSON-able values) and POSTs them to a special endpoint on the same origin with the ID in a header.
3. The framework verifies the request, decodes the arguments, looks up the function in the registry, and executes it on the server.
4. The action returns serializable data (or void). Next.js streams the response back, optionally **including an updated RSC payload** for the current route if you call `revalidatePath`/`revalidateTag` from within.

Key consequences:

- **Server Actions are POST endpoints in disguise.** You can `curl` them. You should treat their inputs as untrusted (CSRF token is enforced by Next, but input validation is on you).
- The action runs in the **server environment** — Node by default, or edge if the file is configured for the edge runtime.
- You **must not** pass non-serializable args (class instances, functions). FormData is the natural shape.
- You can **mutate caches** from inside: `revalidateTag`, `revalidatePath`, `redirect`.

### `"use server"` placement

```ts
// Variant A: top-of-file. Every export in this file is a server action.
// lib/actions/posts.ts
"use server";
import { db } from "@/lib/db";
export async function createPost(formData: FormData) { /* ... */ }
export async function deletePost(id: string) { /* ... */ }
```

```tsx
// Variant B: inline, inside an async function in a server component.
// app/posts/page.tsx (server component)
export default function Page() {
  async function createPost(formData: FormData) {
    "use server";
    // ...
  }
  return <form action={createPost}>{/* ... */}</form>;
}
```

**Important**: `"use server"` is the opposite of `"use client"`. Don't confuse them — they look similar and do opposite things.

---

## 3. Variations / depth

### 3.1 Validation with Zod

Trust nothing from the client. Validate inputs the same way you would in any API:

```ts
// lib/actions/posts.ts
"use server";
import { z } from "zod";
import { db } from "@/lib/db";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";

const CreatePostSchema = z.object({
  title: z.string().min(3).max(120),
  body: z.string().min(10),
});

export async function createPost(formData: FormData) {
  const parsed = CreatePostSchema.safeParse({
    title: formData.get("title"),
    body: formData.get("body"),
  });
  if (!parsed.success) {
    return { ok: false as const, errors: parsed.error.flatten().fieldErrors };
  }
  const post = await db.post.create({ data: parsed.data });
  revalidatePath("/posts");
  redirect(`/posts/${post.id}`);
}
```

### 3.2 `useActionState` (React 19) and `useFormStatus`

`useActionState` (formerly `useFormState`) wires a server action to a client form with returned state — perfect for surfacing validation errors.

```tsx
// app/posts/new/NewPostForm.tsx
"use client";
import { useActionState } from "react";
import { useFormStatus } from "react-dom";
import { createPost } from "@/lib/actions/posts";

type FormState = { ok: boolean; errors?: Record<string, string[]> } | null;

const initialState: FormState = null;

function SubmitButton() {
  const { pending } = useFormStatus();
  return (
    <button type="submit" disabled={pending}>
      {pending ? "Saving…" : "Create"}
    </button>
  );
}

export function NewPostForm() {
  // Wrap the action so its signature matches useActionState's (prevState, formData)
  const action = async (_prev: FormState, formData: FormData) => {
    const result = await createPost(formData);
    return result as FormState;
  };
  const [state, formAction] = useActionState(action, initialState);

  return (
    <form action={formAction} className="space-y-3">
      <div>
        <input name="title" placeholder="Title" />
        {state?.errors?.title?.map((m) => <p key={m} className="text-sm text-red-600">{m}</p>)}
      </div>
      <div>
        <textarea name="body" placeholder="Write…" rows={6} />
        {state?.errors?.body?.map((m) => <p key={m} className="text-sm text-red-600">{m}</p>)}
      </div>
      <SubmitButton />
    </form>
  );
}
```

`useFormStatus` reads the *pending* state of the nearest parent `<form>`. Place a `<SubmitButton />` inside any form and it auto-disables during submission. Note the slight earlier-React naming: `useFormState` was the React 18 / Next 14 name; `useActionState` is the React 19 / Next 15 rename.

### 3.3 Optimistic updates with `useOptimistic`

For instant feedback (likes, cart adds, toggles), apply the change in the UI immediately and reconcile with the server response.

```tsx
// app/posts/[id]/LikeButton.tsx
"use client";
import { useOptimistic, useTransition } from "react";
import { likePost } from "@/lib/actions/posts";

export function LikeButton({ postId, initialLikes }: { postId: string; initialLikes: number }) {
  const [likes, addOptimisticLike] = useOptimistic(initialLikes, (state, _delta: number) => state + 1);
  const [, start] = useTransition();
  return (
    <button
      onClick={() => start(async () => {
        addOptimisticLike(1);
        await likePost(postId);
      })}
    >
      ♥ {likes}
    </button>
  );
}
```

If the server action fails, the optimistic state is rolled back to the canonical value once revalidation completes.

### 3.4 Returning data vs redirecting

A server action can:
- Return data (used by `useActionState`),
- Call `revalidatePath` / `revalidateTag` (re-renders affected RSCs and streams the new payload back),
- Call `redirect("/somewhere")` (must be the *last* call; throws a special redirect "error" that Next catches).

```ts
"use server";
import { redirect } from "next/navigation";
import { revalidateTag } from "next/cache";

export async function publishPost(id: string) {
  await db.post.update({ where: { id }, data: { status: "PUBLISHED" } });
  revalidateTag("posts");
  redirect(`/posts/${id}`);   // last statement
}
```

`redirect()` and `notFound()` are implemented by throwing — never wrap them in a try/catch that swallows everything, or you'll suppress the redirect.

### 3.5 Server actions from non-form events

Anywhere a client component has a server-action reference, it can call it:

```tsx
"use client";
import { deletePost } from "@/lib/actions/posts";

export function DeleteRow({ id }: { id: string }) {
  return (
    <button onClick={async () => {
      if (!confirm("Sure?")) return;
      await deletePost(id);  // POST under the hood
    }}>Delete</button>
  );
}
```

You can also bind arguments with `.bind`:

```tsx
const deleteThis = deletePost.bind(null, id);
<form action={deleteThis}><button>Delete</button></form>
```

`.bind` is safer than passing the ID via a hidden input — the bound value can't be tampered with from the client (it's appended on the server side during dispatch via the action ID).

### 3.6 Authorization inside an action

Always re-check auth inside the action — never trust the UI:

```ts
"use server";
import { auth } from "@/lib/auth";

export async function deletePost(id: string) {
  const session = await auth();
  if (!session?.user) throw new Error("Unauthorized");
  const post = await db.post.findUnique({ where: { id } });
  if (!post || post.authorId !== session.user.id) throw new Error("Forbidden");
  await db.post.delete({ where: { id } });
  revalidatePath("/posts");
}
```

Anyone who reverse-engineers the action ID can POST to it. Defend in depth.

---

## 4. Practical application — a todo list with progressive enhancement, validation, and optimistic UI

```ts
// lib/actions/todos.ts
"use server";
import { z } from "zod";
import { revalidatePath } from "next/cache";
import { db } from "@/lib/db";

const CreateTodoSchema = z.object({
  title: z.string().trim().min(1, "Required").max(200),
});

export type CreateTodoState = {
  ok: boolean;
  error?: string;
} | null;

export async function createTodo(_prev: CreateTodoState, formData: FormData): Promise<CreateTodoState> {
  const parsed = CreateTodoSchema.safeParse({ title: formData.get("title") });
  if (!parsed.success) {
    return { ok: false, error: parsed.error.flatten().fieldErrors.title?.[0] ?? "Invalid" };
  }
  await db.todo.create({ data: { title: parsed.data.title } });
  revalidatePath("/todos");
  return { ok: true };
}

export async function toggleTodo(id: string, done: boolean) {
  await db.todo.update({ where: { id }, data: { done } });
  revalidatePath("/todos");
}

export async function deleteTodo(id: string) {
  await db.todo.delete({ where: { id } });
  revalidatePath("/todos");
}
```

```tsx
// app/todos/page.tsx (server component)
import { db } from "@/lib/db";
import { NewTodoForm } from "./NewTodoForm";
import { TodoRow } from "./TodoRow";

export default async function TodosPage() {
  const todos = await db.todo.findMany({ orderBy: { createdAt: "desc" } });
  return (
    <section className="mx-auto max-w-md">
      <h1 className="text-2xl font-bold">Todos</h1>
      <NewTodoForm />
      <ul className="mt-4 space-y-2">
        {todos.map((t) => (
          <TodoRow
            key={t.id}
            id={t.id}
            title={t.title}
            done={t.done}
          />
        ))}
      </ul>
    </section>
  );
}
```

```tsx
// app/todos/NewTodoForm.tsx
"use client";
import { useActionState, useRef, useEffect } from "react";
import { useFormStatus } from "react-dom";
import { createTodo, type CreateTodoState } from "@/lib/actions/todos";

function Submit() {
  const { pending } = useFormStatus();
  return <button disabled={pending} className="rounded bg-black px-3 py-1 text-white">{pending ? "…" : "Add"}</button>;
}

export function NewTodoForm() {
  const [state, action] = useActionState<CreateTodoState, FormData>(createTodo, null);
  const formRef = useRef<HTMLFormElement>(null);

  // reset on success
  useEffect(() => {
    if (state?.ok) formRef.current?.reset();
  }, [state]);

  return (
    <form ref={formRef} action={action} className="mt-4 flex gap-2">
      <input name="title" placeholder="What needs doing?" className="flex-1 rounded border px-2 py-1" />
      <Submit />
      {state?.error && <p className="text-sm text-red-600">{state.error}</p>}
    </form>
  );
}
```

```tsx
// app/todos/TodoRow.tsx
"use client";
import { useOptimistic, useTransition } from "react";
import { toggleTodo, deleteTodo } from "@/lib/actions/todos";

export function TodoRow({ id, title, done }: { id: string; title: string; done: boolean }) {
  const [optimisticDone, setOptimisticDone] = useOptimistic(done, (_s, next: boolean) => next);
  const [, start] = useTransition();

  return (
    <li className="flex items-center gap-2">
      <input
        type="checkbox"
        checked={optimisticDone}
        onChange={(e) => {
          const next = e.target.checked;
          start(async () => {
            setOptimisticDone(next);
            await toggleTodo(id, next);
          });
        }}
      />
      <span className={optimisticDone ? "line-through text-neutral-500" : ""}>{title}</span>
      <button
        onClick={() => start(() => deleteTodo(id))}
        className="ml-auto text-xs text-red-600"
      >
        delete
      </button>
    </li>
  );
}
```

The form works without JavaScript (try it!). With JS, the toggle is instant; the list refreshes via `revalidatePath` after each mutation. There is no client-side state store, no API route, no fetch boilerplate.

---

## 5. Common mistakes & gotchas

### Confusing `"use server"` with `"use client"`

They look similar; they do opposite things. `"use server"` marks **functions** as server actions; `"use client"` marks **modules** as client modules. Putting `"use server"` at the top of a UI component file makes *every export* a server action, which is rarely what you want for components.

### Forgetting to call `revalidatePath` / `revalidateTag`

You mutate the DB, return success, but the page still shows old data. Server components only re-render when something tells them to. Either call `revalidatePath("/...")` from the action, or design your fetch to be uncached (`no-store`), or rely on time-based revalidation.

### Swallowing `redirect()` in a try/catch

`redirect()` throws internally. Inside `try { await action(); } catch {}` it gets caught and silenced. Either don't catch broadly, or re-throw if the error is `isRedirectError(err)`:

```ts
import { isRedirectError } from "next/dist/client/components/redirect";
try { /* ... */ } catch (e) {
  if (isRedirectError(e)) throw e;
  // handle real errors
}
```

### Passing non-serializable args to actions

Arguments to a server action are serialized by React Server Components Flight. Pass plain objects, strings, numbers, arrays, FormData — not functions, class instances, or Dates with custom prototypes.

### Trusting `.bind` for auth

`.bind` prevents *casual* tampering but a determined attacker can still call the action's endpoint with arbitrary args. Always re-check auth & ownership inside the action.

### Heavy work blocking the form response

A server action that takes 10 seconds blocks the form's response and the user sees a long pending state. For long jobs, enqueue a background task and return immediately with a job ID; poll or push status separately.

### Returning huge payloads

The action's return value is serialized over the wire. Returning the whole new database table is wasteful. Return small status / error data, and rely on `revalidate*` to refresh the page's data via RSC.

### Race conditions on optimistic UI

Two quick clicks can interleave server responses out of order. Use `useTransition` to coalesce, debounce on the client, or design the action idempotently (e.g., "set done to true" is idempotent; "increment likes" isn't).

### Server Actions and rate-limiting

There is no built-in rate limit. Add one (e.g., via Upstash Ratelimit or your own middleware) before exposing actions that hit external paid APIs or expensive DB queries.

---

## 🎯 Key Takeaways

- **Server Actions are async POST endpoints with a great DX.** Treat them with API-level discipline: validate inputs, check authz, rate-limit, log.
- **Forms work without JavaScript.** `<form action={serverAction}>` is real progressive enhancement. Build forms that degrade gracefully and you ship to a wider audience for free.
- **`useActionState` + `useFormStatus` + `useOptimistic`** is the modern client-side trio for form state, pending UI, and optimistic updates. Memorize their roles.
- **Mutations must invalidate caches.** Pair every successful mutation with `revalidatePath` or `revalidateTag`. Otherwise the page lies to the user.
- **Don't swallow `redirect()`** in try/catch, and don't pass non-serializable args. These two bugs together account for most "my action does nothing" reports.

*←* [`04_data_fetching.md`](./04_data_fetching.md) *|* *next →* [`06_rendering_strategies.md`](./06_rendering_strategies.md)
