# 05 — Server Actions & Mutations

> **Goal:** Master Server Actions for forms and mutations with progressive enhancement, input validation, optimistic UI, and immediate cache invalidation via `updateTag`.

---

## 1. Concept — `"use server"` and Progressive Enhancement

A **Server Action** is an asynchronous function marked with the `"use server"` directive (either at the top of the file, or as the first line of the function itself). It runs **only on the server**.

You can pass a reference to a Server Action directly to the `action` prop of an HTML `<form>`. Next.js handles the network serialization and POST request under the hood.

```tsx
// app/contact/page.tsx
async function submitContact(formData: FormData) {
  "use server";
  const name = String(formData.get("name") ?? "");
  const email = String(formData.get("email") ?? "");
  
  // Persist contact details (e.g. Database insert)
  console.log("Contact submission:", name, email);
}

export default function ContactPage() {
  return (
    <form action={submitContact} className="flex flex-col gap-2 max-w-sm">
      <input name="name" placeholder="Name" required className="border p-2" />
      <input name="email" type="email" placeholder="Email" required className="border p-2" />
      <button type="submit" className="bg-black text-white p-2">Send</button>
    </form>
  );
}
```

### Progressive Enhancement
If a user submits the form with JavaScript disabled in their browser, it still works. The browser performs a native HTML form POST to the current route segment, which Next.js intercepts on the server and dispatches to your Server Action. Once JavaScript hydrates, the form transitions to a single-page, no-reload interaction.

Server Actions can also be called as standard async functions from Client Components:

```tsx
// app/components/Like.tsx
"use client";
import { useTransition } from "react";
import { likePost } from "@/lib/actions/posts"; // server action

export function Like({ postId }: { postId: string }) {
  const [pending, startTransition] = useTransition();
  return (
    <button
      disabled={pending}
      onClick={() => startTransition(() => likePost(postId))}
      className="border p-2 disabled:opacity-50"
    >
      {pending ? "Saving..." : "Like"}
    </button>
  );
}
```

---

## 2. Mechanism — How Actions Work Under the Hood

During the compilation phase (powered by Turbopack in Next.js 16), Next.js performs static analysis on your code:

1. It extracts `"use server"` functions and replaces them with a generated **stable hash ID** in a server-side registry.
2. In the client bundle, it replaces the direct function import with a thin proxy function. When called, the proxy serializes the arguments and POSTs them to the origin with the action ID set in a request header.
3. The server receives the POST request, verifies the CSRF token (handled automatically by Next.js), decodes the payload, executes the function, and streams the return value or updated RSC payload back to the client.

Treat Server Actions with the same security and validation rigor as any REST or GraphQL API endpoint. They are public POST routes.

---

## 3. Variations / Depth

### 3.1 Input Validation with Zod

Never trust client inputs. Always validate schema types server-side before acting on form submissions:

```typescript
// lib/actions/posts.ts
"use server";
import { z } from "zod";
import { db } from "@/lib/db";
import { updateTag } from "next/cache";
import { redirect } from "next/navigation";

const CreatePostSchema = z.object({
  title: z.string().min(3, "Title must be at least 3 chars").max(120),
  body: z.string().min(10, "Content must be at least 10 chars"),
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
  
  // Force cache invalidation immediately
  updateTag("posts");
  
  redirect(`/posts/${post.id}`);
}
```

### 3.2 Stable Form Hooks: `useActionState` and `useFormStatus`

In React 19 / Next.js 16, **`useActionState`** (formerly `useFormState` in Next 14) and **`useFormStatus`** are fully stable.

* **`useActionState`** connects form submission state (errors, response payloads) with the Server Action.
* **`useFormStatus`** reads the pending state of the parent form. It must be used inside a child component of `<form>`.

```tsx
// app/posts/new/NewPostForm.tsx
"use client";
import { useActionState } from "react";
import { useFormStatus } from "react-dom";
import { createPost } from "@/lib/actions/posts";

type FormState = { ok: boolean; errors?: Record<string, string[]> } | null;

function SubmitButton() {
  const { pending } = useFormStatus();
  return (
    <button type="submit" disabled={pending} className="bg-black text-white p-2 disabled:bg-neutral-400">
      {pending ? "Creating..." : "Create Post"}
    </button>
  );
}

export function NewPostForm() {
  const action = async (_prev: FormState, formData: FormData) => {
    const result = await createPost(formData);
    return result as FormState;
  };
  
  const [state, formAction] = useActionState(action, null);

  return (
    <form action={formAction} className="flex flex-col gap-4 max-w-md">
      <div>
        <input name="title" placeholder="Title" className="border p-2 w-full" />
        {state?.errors?.title?.map((err) => <p key={err} className="text-sm text-red-600">{err}</p>)}
      </div>
      <div>
        <textarea name="body" placeholder="Write something..." rows={6} className="border p-2 w-full" />
        {state?.errors?.body?.map((err) => <p key={err} className="text-sm text-red-600">{err}</p>)}
      </div>
      <SubmitButton />
    </form>
  );
}
```

### 3.3 Optimistic Updates with `useOptimistic`

`useOptimistic` allows you to immediately update the UI state upon submit, reverting to the canonical server state if the action fails or when revalidation completes.

```tsx
// app/posts/[id]/LikeButton.tsx
"use client";
import { useOptimistic, useTransition } from "react";
import { likePost } from "@/lib/actions/posts";

export function LikeButton({ postId, initialLikes }: { postId: string; initialLikes: number }) {
  const [likes, addOptimisticLike] = useOptimistic(
    initialLikes,
    (state, _delta: number) => state + 1
  );
  const [pending, startTransition] = useTransition();

  return (
    <button
      disabled={pending}
      onClick={() => startTransition(async () => {
        addOptimisticLike(1);
        await likePost(postId);
      })}
      className="border p-2 rounded"
    >
      ♥ {likes}
    </button>
  );
}
```

### 3.4 Handling Redirects in Actions

`redirect()` and `notFound()` are implemented by throwing special internal Next.js errors.
- Never place a `redirect()` call inside a generic `try/catch` block that catches all errors, otherwise the redirect will be caught and suppressed.
- Always call `redirect` as the final statement of your action.

```typescript
"use server";
import { redirect } from "next/navigation";
import { updateTag } from "next/cache";

export async function deletePost(id: string) {
  try {
    await db.post.delete({ where: { id } });
  } catch (error) {
    return { error: "Failed to delete post" };
  }
  
  updateTag("posts");
  redirect("/posts"); // Called outside the try/catch block
}
```

---

## 4. Practical Application — Todo Checklist

Here is a complete interactive list implementation utilizing validation, optimistic updates, and clean cache updates.

```typescript
// lib/actions/todos.ts
"use server";
import { z } from "zod";
import { updateTag } from "next/cache";
import { db } from "@/lib/db";

const CreateTodoSchema = z.object({
  title: z.string().trim().min(1, "Title cannot be empty").max(200),
});

export type CreateTodoState = { ok: boolean; error?: string } | null;

export async function createTodo(_prev: CreateTodoState, formData: FormData): Promise<CreateTodoState> {
  const parsed = CreateTodoSchema.safeParse({ title: formData.get("title") });
  if (!parsed.success) {
    return { ok: false, error: parsed.error.flatten().fieldErrors.title?.[0] };
  }
  await db.todo.create({ data: { title: parsed.data.title } });
  updateTag("todos");
  return { ok: true };
}

export async function toggleTodo(id: string, done: boolean) {
  await db.todo.update({ where: { id }, data: { done } });
  updateTag("todos");
}

export async function deleteTodo(id: string) {
  await db.todo.delete({ where: { id } });
  updateTag("todos");
}
```

```tsx
// app/todos/NewTodoForm.tsx
"use client";
import { useActionState, useRef, useEffect } from "react";
import { useFormStatus } from "react-dom";
import { createTodo, type CreateTodoState } from "@/lib/actions/todos";

function SubmitButton() {
  const { pending } = useFormStatus();
  return <button disabled={pending} className="bg-black text-white px-4 py-2">{pending ? "..." : "Add"}</button>;
}

export function NewTodoForm() {
  const [state, action] = useActionState<CreateTodoState, FormData>(createTodo, null);
  const formRef = useRef<HTMLFormElement>(null);

  useEffect(() => {
    if (state?.ok) formRef.current?.reset();
  }, [state]);

  return (
    <form ref={formRef} action={action} className="flex gap-2">
      <input name="title" placeholder="New todo..." className="border p-2 flex-1" />
      <SubmitButton />
      {state?.error && <p className="text-red-600 text-sm mt-1">{state.error}</p>}
    </form>
  );
}
```

---

## 5. Common Mistakes & Gotchas

### Swallowing Redirect Errors
`redirect()` throws an error to initiate routing. Wrapping the entire action body in a generic `try { ... } catch (e) { ... }` will swallow the routing event and make the redirect fail. Either perform redirects outside of the `try/catch` or check if the error is a redirect error.

### Not Invalidating the Cache
Remember that in Next.js 16, components cached with `"use cache"` will continue serving stale data unless you call `updateTag()` (immediate) or `revalidateTag()` (background) for their associated tags inside the Server Action.

### Authentication Leaks in `.bind`
While `.bind()` is helpful for passing arguments securely (like `deletePost.bind(null, id)`), never trust the bound values blindly. Verify that the authenticated session user has ownership permissions over the database item inside the action logic.

---

## 🎯 Key Takeaways

- **Actions are POST endpoints:** Treat them with typical API security discipline (validate input, check authz).
- **Progressive enhancement:** Keep forms functional even without JavaScript, then hydrate them.
- **`useActionState` and `useFormStatus`** provide clean states for forms and loading feedback.
- **Cache updates:** Always pair successful mutations with `updateTag()` to ensure immediate updates in `"use cache"` views.

*←* [`04_data_fetching.md`](./04_data_fetching.md) *|* *next →* [`06_rendering_strategies.md`](./06_rendering_strategies.md)
