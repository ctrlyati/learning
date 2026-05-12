# 15 — Testing

> **Goal:** Set up Vitest + React Testing Library for unit/component tests (with RSC awareness), Playwright for end-to-end, and know how to mock `fetch`, the database, and Auth.js.

---

## 1. Concept — three layers, three tools

A modern Next.js test suite has three layers:

- **Unit tests** for pure functions, validators, utilities. Vitest, fast, no DOM.
- **Component tests** for React components with a DOM, mocking data. Vitest + React Testing Library + happy-dom/jsdom.
- **End-to-end** for full user flows against a running app. Playwright.

A pyramid: many unit, fewer component, few e2e. The lower in the pyramid, the faster and cheaper the test.

```ts
// Unit test
import { describe, it, expect } from "vitest";
import { slugify } from "@/lib/slug";

describe("slugify", () => {
  it("lowercases and dashes", () => {
    expect(slugify("Hello World")).toBe("hello-world");
  });
});
```

```tsx
// Component test
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Counter } from "./Counter";

it("increments", async () => {
  render(<Counter />);
  await userEvent.click(screen.getByRole("button", { name: /increment/i }));
  expect(screen.getByText("1")).toBeInTheDocument();
});
```

```ts
// E2E
import { test, expect } from "@playwright/test";
test("can sign in and see dashboard", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Email").fill("ada@example.com");
  await page.getByLabel("Password").fill("password");
  await page.getByRole("button", { name: /sign in/i }).click();
  await expect(page).toHaveURL(/\/dashboard/);
});
```

---

## 2. Mechanism — what makes Next.js testing tricky

### 2.1 Server Components in jest/vitest

React Testing Library doesn't natively render async server components — they require a different render path. There are two pragmatic strategies:

- **Skip RSC rendering in component tests.** Test client components in isolation; test the data-layer functions that RSCs call separately. This is the most common approach in 2026.
- **Use the experimental RSC test runner** (`@testing-library/react` 16+ has limited support). Still rough; only use if you really want to render an RSC tree.

The pragmatic split:

```ts
// Pure data layer — unit-tested with vitest
import { getPostsByAuthor } from "@/lib/data/posts";
// mock the DB, call the function, assert.

// Client components — RTL
import { LikeButton } from "./LikeButton";
// render, click, assert.

// Whole-page behavior — Playwright e2e against a running app.
```

### 2.2 Mocking `fetch` and the patched cache

Next's patched fetch lives only in the runtime — in vitest, you just have the standard global `fetch`. Mock it with `vi.fn()` or MSW:

```ts
import { vi } from "vitest";

beforeEach(() => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(JSON.stringify({ ok: true }), { status: 200 })
  );
});
```

For more realistic HTTP mocking, MSW (Mock Service Worker) intercepts `fetch` at the network layer — same setup works in Node tests and the browser.

### 2.3 Mocking the database

Pick one strategy:

- **Mock the client** (`vi.mock("@/lib/db")`) — fastest but uncovers fewer real bugs.
- **Use a test database** (Postgres in Docker / Testcontainers) — slower but tests the actual SQL/ORM behavior.
- **Use Prisma's `jest-prisma`-style adapters** or transactions that roll back after each test.

For most app code, a test DB with per-test transaction rollback is the best balance.

---

## 3. Variations / depth

### 3.1 Vitest setup

```bash
pnpm add -D vitest @vitejs/plugin-react jsdom @testing-library/react @testing-library/user-event @testing-library/jest-dom
```

```ts
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

```ts
// vitest.setup.ts
import "@testing-library/jest-dom/vitest";
```

```json
// package.json scripts
{
  "test": "vitest",
  "test:run": "vitest run",
  "test:e2e": "playwright test"
}
```

### 3.2 Mocking modules with vi.mock

```ts
// app/posts/__tests__/createPost.test.ts
import { describe, it, expect, vi } from "vitest";
import { createPost } from "@/lib/actions/posts";

vi.mock("@/lib/db", () => ({
  db: {
    post: {
      create: vi.fn().mockResolvedValue({ id: "p1", title: "x", body: "y" }),
    },
  },
}));

vi.mock("@/auth", () => ({
  auth: vi.fn().mockResolvedValue({ user: { id: "u1" } }),
}));

vi.mock("next/cache", () => ({ revalidateTag: vi.fn() }));

describe("createPost action", () => {
  it("rejects invalid input", async () => {
    const fd = new FormData();
    fd.set("title", "no");
    fd.set("body", "short");
    const result = await createPost(null, fd);
    expect(result?.ok).toBe(false);
  });

  it("creates a post for an authed user", async () => {
    const fd = new FormData();
    fd.set("title", "Hello world");
    fd.set("body", "This is a long enough body.");
    const result = await createPost(null, fd);
    expect(result?.ok).toBe(true);
  });
});
```

### 3.3 Component testing

```tsx
// components/Counter.tsx
"use client";
import { useState } from "react";
export function Counter({ initial = 0 }: { initial?: number }) {
  const [n, setN] = useState(initial);
  return (
    <div>
      <p data-testid="value">{n}</p>
      <button onClick={() => setN(n + 1)}>Increment</button>
    </div>
  );
}
```

```ts
// components/Counter.test.tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Counter } from "./Counter";

it("starts at the initial value", () => {
  render(<Counter initial={5} />);
  expect(screen.getByTestId("value")).toHaveTextContent("5");
});

it("increments on click", async () => {
  render(<Counter />);
  await userEvent.click(screen.getByRole("button", { name: /increment/i }));
  expect(screen.getByTestId("value")).toHaveTextContent("1");
});
```

### 3.4 Mocking Server Actions in components

A client component that imports a server action will, in test, see the function as just a function — no special wiring. Mock the action module:

```ts
vi.mock("@/lib/actions/likes", () => ({
  likePost: vi.fn().mockResolvedValue(undefined),
}));
```

### 3.5 Playwright setup

```bash
pnpm dlx create-playwright
```

```ts
// playwright.config.ts
import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  webServer: {
    command: "pnpm dev",
    url: "http://localhost:3000",
    reuseExistingServer: !process.env.CI,
  },
  use: { baseURL: "http://localhost:3000" },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
    { name: "firefox", use: { ...devices["Desktop Firefox"] } },
  ],
});
```

```ts
// e2e/auth.spec.ts
import { test, expect } from "@playwright/test";

test("redirects unauth users to login", async ({ page }) => {
  await page.goto("/dashboard");
  await expect(page).toHaveURL(/\/login/);
});

test("can sign in and see dashboard", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Email").fill("ada@example.com");
  await page.getByLabel("Password").fill("password");
  await page.getByRole("button", { name: /sign in/i }).click();
  await expect(page).toHaveURL(/\/dashboard/);
  await expect(page.getByRole("heading", { name: /welcome/i })).toBeVisible();
});
```

### 3.6 Authenticated Playwright tests

Logging in for every test is slow. Use a setup script + storage state:

```ts
// e2e/auth.setup.ts
import { test as setup, expect } from "@playwright/test";

setup("authenticate", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Email").fill("ada@example.com");
  await page.getByLabel("Password").fill("password");
  await page.getByRole("button", { name: /sign in/i }).click();
  await expect(page).toHaveURL(/\/dashboard/);
  await page.context().storageState({ path: ".auth/user.json" });
});
```

```ts
// playwright.config.ts excerpt
projects: [
  { name: "setup", testMatch: /.*\.setup\.ts/ },
  {
    name: "authenticated",
    dependencies: ["setup"],
    use: { storageState: ".auth/user.json" },
  },
],
```

### 3.7 Visual regression

Playwright snapshots:

```ts
test("home looks correct", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveScreenshot("home.png", { maxDiffPixelRatio: 0.01 });
});
```

For finer-grained component snapshots, consider Chromatic or Percy.

### 3.8 Testing route handlers directly

```ts
import { GET } from "@/app/api/posts/route";

it("GET /api/posts returns 200", async () => {
  const req = new Request("http://test/api/posts");
  const res = await GET(req as any);
  expect(res.status).toBe(200);
  const json = await res.json();
  expect(Array.isArray(json)).toBe(true);
});
```

You're calling the exported function directly with a `Request`. No HTTP server needed.

---

## 4. Practical application — a vertical test slice for a "create post" feature

```ts
// lib/slug.ts (the unit under test)
export function slugify(s: string): string {
  return s.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
}
```

```ts
// lib/slug.test.ts
import { describe, it, expect } from "vitest";
import { slugify } from "./slug";

describe("slugify", () => {
  it.each([
    ["Hello World", "hello-world"],
    [" Trim Me ", "trim-me"],
    ["Special!@# Chars", "special-chars"],
    ["multi   spaces", "multi-spaces"],
  ])("%s -> %s", (input, expected) => {
    expect(slugify(input)).toBe(expected);
  });
});
```

```ts
// lib/actions/posts.test.ts
import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@/auth", () => ({ auth: vi.fn() }));
vi.mock("@/lib/db", () => ({
  db: { post: { create: vi.fn() } },
}));
vi.mock("next/cache", () => ({ revalidateTag: vi.fn() }));
vi.mock("next/navigation", () => ({ redirect: vi.fn() }));

import { auth } from "@/auth";
import { db } from "@/lib/db";
import { createPost } from "./posts";

describe("createPost", () => {
  beforeEach(() => vi.clearAllMocks());

  it("returns unauthorized if no session", async () => {
    (auth as any).mockResolvedValue(null);
    const fd = new FormData();
    fd.set("title", "Hello world");
    fd.set("body", "This is a long enough body.");
    const result = await createPost(null, fd);
    expect(result).toMatchObject({ ok: false, error: "Unauthorized" });
    expect(db.post.create).not.toHaveBeenCalled();
  });

  it("creates a post for an authed user", async () => {
    (auth as any).mockResolvedValue({ user: { id: "u1" } });
    (db.post.create as any).mockResolvedValue({ id: "p1" });
    const fd = new FormData();
    fd.set("title", "Hello world");
    fd.set("body", "This is a long enough body.");
    await createPost(null, fd);
    expect(db.post.create).toHaveBeenCalledWith({
      data: expect.objectContaining({ authorId: "u1", title: "Hello world" }),
    });
  });

  it("rejects invalid input", async () => {
    (auth as any).mockResolvedValue({ user: { id: "u1" } });
    const fd = new FormData();
    fd.set("title", "no");
    fd.set("body", "short");
    const result = await createPost(null, fd);
    expect(result?.ok).toBe(false);
  });
});
```

```ts
// e2e/create-post.spec.ts
import { test, expect } from "@playwright/test";

test.use({ storageState: ".auth/user.json" });

test("can create a post", async ({ page }) => {
  await page.goto("/posts/new");
  await page.getByLabel("Title").fill("E2E created post");
  await page.getByLabel("Body").fill("This was created by a Playwright test.");
  await page.getByRole("button", { name: /create/i }).click();
  await expect(page).toHaveURL(/\/posts\/[a-z0-9]+/);
  await expect(page.getByRole("heading", { name: "E2E created post" })).toBeVisible();
});
```

Together: deterministic unit tests for the validator, mocked-dependency tests for the action, real-browser confirmation for the user flow. Fast feedback loop, real confidence.

---

## 5. Common mistakes & gotchas

### Trying to render an async server component in RTL

Today's testing libraries don't cleanly support this. Split: test the data layer separately, test client components in RTL, test the integrated page in Playwright.

### Forgetting `vi.clearAllMocks()` between tests

Mock state leaks between tests, especially with module-level mocks. Add `beforeEach(() => vi.clearAllMocks())` or set `clearMocks: true` in vitest config.

### Mocking `next/cache` without mocking `revalidateTag`

If your action calls `revalidateTag` and you only mock `unstable_cache`, the import fails. Mock the full surface you use.

### Mocking auth too generously

If every test gets a logged-in user, the unauth code path is untested. Test both — `auth` returns null and `auth` returns a user.

### Brittle selectors

`page.locator("div > div > div:nth-child(2)")` breaks the moment you tweak markup. Use accessible queries: `getByRole`, `getByLabel`, `getByText`. They map to how users perceive the UI.

### Slow Playwright suites that always log in

Use `storageState` to seed an authenticated session once, reuse across tests. Drops total run time dramatically.

### Tests that depend on global state

Per-test transactions in your test DB (rolled back at the end) or per-test reset. Otherwise tests are order-dependent — a flake nightmare.

### Snapshot tests of everything

Use snapshots sparingly. They drift, they bloat PRs, they encourage "just regenerate" instead of inspecting. Use them for stable, narrow output (e.g., a serialized object).

### Mocking `useRouter` in component tests

For client components that call `useRouter()`:

```ts
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn() }),
  usePathname: () => "/",
}));
```

---

## 🎯 Key Takeaways

- **Three layers, three tools**: Vitest (unit + component, with RTL), Playwright (e2e). Don't reach for e2e to test what unit tests can cover.
- **Don't fight RSC in component tests.** Test the data layer separately, client components in isolation, and full pages in Playwright.
- **Mock dependencies at the module boundary**: `vi.mock("@/lib/db")`, `vi.mock("@/auth")`, `vi.mock("next/cache")`. Set per-test mock return values; clear between tests.
- **Use accessible queries** (`getByRole`, `getByLabel`) — they survive markup churn and double as accessibility checks.
- **Authenticate once in Playwright** with `storageState` and reuse. Run tests in parallel against a single dev server with `reuseExistingServer`.

*←* [`14_route_handlers_and_api.md`](./14_route_handlers_and_api.md) *|* *next →* [`16_performance_and_observability.md`](./16_performance_and_observability.md)
