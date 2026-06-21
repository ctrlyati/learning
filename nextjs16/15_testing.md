# 15 — Testing

> **Goal:** Configure Vitest, React Testing Library, and Playwright for Next.js 16 and React 19 testing environments; mock database queries, authentication sessions, and Next.js caching APIs.

---

## 1. Concept — The Testing Pyramid

A modern test suite spans three distinct layers:

- **Unit Tests:** Fast, in-memory validation of pure logic, schema parsers, and utilities. (Vitest).
- **Component Tests:** Validates user interactions within client components. (Vitest + React Testing Library + JSDOM).
- **End-to-End (E2E) Tests:** Verifies integrated user flows (database writes, routing transitions) in a real browser. (Playwright).

---

## 2. Testing App Router Patterns

### 2.1 Server Components (RSC) rendering limits
React Testing Library does not render asynchronous Server Components natively. The pragmatic testing split is:
- **Client Components:** Test user interactions in isolation using React Testing Library.
- **Data Query Layers:** Unit-test query and business logic functions directly by mocking database adapters.
- **Complete Pages:** Write E2E Playwright tests targeting the running application.

### 2.2 Mocking Next.js 16 Caching APIs
When testing Server Actions or data loading modules, mock caching APIs (like `updateTag` and `revalidateTag`) to assert cache eviction side effects:

```typescript
import { vi } from "vitest";

vi.mock("next/cache", () => ({
  updateTag: vi.fn(),
  revalidateTag: vi.fn(),
}));
```

---

## 3. Operations & Setup

### 3.1 Vitest Setup

```bash
pnpm add -D vitest @vitejs/plugin-react jsdom @testing-library/react @testing-library/user-event @testing-library/jest-dom
```

Configure your test execution rules:

```typescript
// vitest.config.ts
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import { resolve } from "path";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
    globals: true,
  },
  resolve: {
    alias: { "@": resolve(__dirname, "./src") },
  },
});
```

Import testing matches:

```typescript
// vitest.setup.ts
import "@testing-library/jest-dom/vitest";
```

### 3.2 Mocking Actions & Navigation Router

For client components utilizing routing hooks, mock `next/navigation`:

```typescript
// components/Counter.test.tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: vi.fn(),
    replace: vi.fn(),
    refresh: vi.fn(),
  }),
  usePathname: () => "/current-path",
}));
```

---

## 4. Practical Application — Creating & Testing Posts

Here is a full vertical test suite for validating post creation logic.

```typescript
// src/lib/slug.ts
export function slugify(s: string): string {
  return s.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
}
```

```typescript
// src/lib/slug.test.ts
import { describe, it, expect } from "vitest";
import { slugify } from "./slug";

describe("slugify utility", () => {
  it("lowercases and dashes string characters", () => {
    expect(slugify("Hello NextJS 16")).toBe("hello-nextjs-16");
  });
});
```

Verify Server Actions with mocked environments:

```typescript
// src/lib/actions/posts.test.ts
import { describe, it, expect, vi, beforeEach } from "vitest";

// Mock dependencies
vi.mock("@/auth", () => ({ auth: vi.fn() }));
vi.mock("@/lib/db", () => ({
  db: { post: { create: vi.fn() } },
}));
vi.mock("next/cache", () => ({ updateTag: vi.fn() }));
vi.mock("next/navigation", () => ({ redirect: vi.fn() }));

import { auth } from "@/auth";
import { db } from "@/lib/db";
import { updateTag } from "next/cache";
import { createPost } from "./posts";

describe("createPost Server Action", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("fails authorization if session is absent", async () => {
    (auth as any).mockResolvedValue(null);
    
    const formData = new FormData();
    formData.set("title", "Valid Title");
    formData.set("body", "Long body content matches schema rules.");

    const res = await createPost(null, formData);
    
    expect(res).toMatchObject({ ok: false, error: "Unauthorized" });
    expect(db.post.create).not.toHaveBeenCalled();
  });

  it("inserts new post and evicts cache on success", async () => {
    (auth as any).mockResolvedValue({ user: { id: "u123" } });
    (db.post.create as any).mockResolvedValue({ id: "p456" });

    const formData = new FormData();
    formData.set("title", "Valid Title");
    formData.set("body", "Long body content matches schema rules.");

    await createPost(null, formData);

    expect(db.post.create).toHaveBeenCalledWith({
      data: expect.objectContaining({ authorId: "u123", title: "Valid Title" }),
    });
    expect(updateTag).toHaveBeenCalledWith("posts");
  });
});
```

Verify in-browser UX routing flows:

```typescript
// e2e/create-post.spec.ts
import { test, expect } from "@playwright/test";

test.use({ storageState: ".auth/user.json" }); // Seed authenticated session

test("navigates and saves new post successfully", async ({ page }) => {
  await page.goto("/posts/new");
  await page.getByLabel("Title").fill("E2E Post Title");
  await page.getByLabel("Body").fill("Content payload passed by Playwright E2E framework.");
  await page.getByRole("button", { name: /create/i }).click();

  await expect(page).toHaveURL(/\/posts\/[a-z0-9]+/);
  await expect(page.getByRole("heading", { name: "E2E Post Title" })).toBeVisible();
});
```

---

## 5. Common Mistakes & Gotchas

### Stale Mock Leakages
If you assert mock call executions, mock states can easily carry over between tests. Always clean registers within `beforeEach()` loops:

```typescript
beforeEach(() => {
  vi.clearAllMocks();
});
```

### Playwright Login Overheads
Avoid loading and submitting your login form before every E2E execution. Authenticate once during setup and export session cookies using Playwright's **`storageState`** feature:

```typescript
// e2e/auth.setup.ts
import { test as setup, expect } from "@playwright/test";

setup("authenticate user session", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Email").fill("user@example.com");
  await page.getByLabel("Password").fill("password");
  await page.getByRole("button", { name: /sign in/i }).click();
  await expect(page).toHaveURL(/\/dashboard/);
  await page.context().storageState({ path: ".auth/user.json" });
});
```

---

## 🎯 Key Takeaways

- **Pragmatic boundary splits:** Verify pure code with unit tests, interactions in component tests, and integrated rendering via Playwright.
- **Mock at structural bounds:** Isolate dependencies by mocking database layers and external authentication handlers.
- **Isolate test environments:** Keep test databases clean by running migrations separately and wrapping writes in rollback hooks.

*←* [`14_route_handlers_and_api.md`](./14_route_handlers_and_api.md) *|* *next →* [`16_performance_and_observability.md`](./16_performance_and_observability.md)
