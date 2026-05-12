# 15 — Testing: Vitest/Jest, Mocking, e2e

> **Goal:** Write fast, trustworthy tests for JavaScript code — unit, integration, and end-to-end — using modern tooling (Vitest, Playwright) and good practices around mocking.

---

## 1. The Testing Pyramid — Mental Model

```
                /\
               /e2e\         few — slow, brittle, but check real behavior
              /------\
             /integ.  \      some — verify wiring across modules
            /----------\
           /   unit     \    many — fast, focused, deterministic
          /--------------\
```

A healthy codebase has many unit tests, fewer integration tests, and a small number of critical end-to-end tests. Writing tests upside-down (mostly e2e) leads to slow, flaky CI.

### What a unit test looks like
```js
// sum.js
export const sum = (a, b) => a + b;

// sum.test.js
import { describe, it, expect } from "vitest";
import { sum } from "./sum.js";

describe("sum", () => {
  it("adds two numbers", () => {
    expect(sum(2, 3)).toBe(5);
  });
  it("handles negatives", () => {
    expect(sum(-1, 1)).toBe(0);
  });
});
```

### Vitest vs Jest
- **Jest** — Facebook's classic. Mature, huge ecosystem.
- **Vitest** — Vite-native. Vastly faster, ESM-first, near-Jest-compatible API. **Default choice for new projects in 2026.**

For Node-only libs without Vite, Vitest still works great. Use Jest if you've inherited it or need a specific Jest plugin.

```bash
npm i -D vitest
```

```json
// package.json
"scripts": {
  "test": "vitest",
  "test:ci": "vitest run --coverage"
}
```

---

## 2. Matchers, Setup, Async — Under the Hood

### Common matchers (Vitest = Jest-compatible)
```js
expect(value).toBe(other);                 // === equality (primitives)
expect(obj).toEqual({ a: 1 });             // deep equality
expect(obj).toStrictEqual({ a: 1 });       // also checks undefined / type
expect(arr).toContain(3);
expect(arr).toHaveLength(5);
expect(str).toMatch(/regex/);
expect(fn).toThrow("expected message");
expect(value).toBeNull(); .toBeUndefined(); .toBeDefined(); .toBeTruthy();
expect(value).toBeGreaterThan(5);
expect(promise).resolves.toBe(42);
expect(promise).rejects.toThrow();
expect(spy).toHaveBeenCalledWith(1, 2);
expect(spy).toHaveBeenCalledTimes(2);
```

### Setup / teardown
```js
import { describe, it, beforeEach, afterEach, beforeAll, afterAll, expect } from "vitest";

describe("DB", () => {
  let db;
  beforeAll(async () => { db = await openTestDb(); });
  afterAll(async () => { await db.close(); });
  beforeEach(async () => { await db.reset(); });

  it("creates a user", async () => { /* ... */ });
});
```

### Async tests
```js
it("fetches data", async () => {
  const data = await fetchUser(1);
  expect(data.name).toBe("Ada");
});

// Promise rejection
it("rejects on 404", async () => {
  await expect(fetchUser(-1)).rejects.toThrow("Not found");
});
```

### Snapshot testing
```js
expect(renderUser(user)).toMatchSnapshot();
```
First run records to `__snapshots__/`; subsequent runs compare. Run `vitest -u` to update intentionally. Use snapshots sparingly — they degrade if reviewers stop reading the diffs.

### Coverage
```bash
vitest run --coverage
```
Vitest uses `v8` coverage by default — fast and reasonably accurate. Don't chase 100%; aim for meaningful coverage of branches and edge cases.

---

## 3. Mocks, Spies, Stubs

### Mock functions
```js
import { vi, expect, it } from "vitest";

it("calls callback", () => {
  const cb = vi.fn();
  [1, 2, 3].forEach(cb);
  expect(cb).toHaveBeenCalledTimes(3);
  expect(cb).toHaveBeenCalledWith(2, 1, [1,2,3]);
});

// Mock an implementation
const fetcher = vi.fn().mockResolvedValue({ ok: true });
const f2 = vi.fn().mockReturnValueOnce(1).mockReturnValueOnce(2).mockReturnValue(3);
```

### Mocking modules
```js
// myModule.js calls userService.find
import * as userService from "./userService.js";
vi.mock("./userService.js");
vi.mocked(userService.find).mockResolvedValue({ id: 1, name: "Ada" });
```

### Spying without replacing
```js
const spy = vi.spyOn(console, "log").mockImplementation(() => {});
runThing();
expect(spy).toHaveBeenCalledWith("expected message");
spy.mockRestore();
```

### Faking time
```js
import { vi } from "vitest";

vi.useFakeTimers();
const cb = vi.fn();
setTimeout(cb, 1000);
vi.advanceTimersByTime(1000);
expect(cb).toHaveBeenCalled();
vi.useRealTimers();
```

### When to mock — and when NOT to
- **Mock the boundary** (network, filesystem, time, randomness). Tests should be deterministic.
- **Don't mock what you're testing.** If `userService.find` is your subject, don't mock it; mock its dependencies.
- **Avoid mocking your own internals deeply.** Heavy mocking → tests that pass while production breaks. Prefer integration tests where realistic.

### MSW for HTTP mocking
```js
import { setupServer } from "msw/node";
import { http, HttpResponse } from "msw";

const server = setupServer(
  http.get("https://api.example.com/users/:id", ({ params }) =>
    HttpResponse.json({ id: params.id, name: "Ada" })
  ),
);

beforeAll(() => server.listen());
afterEach(() => server.resetHandlers());
afterAll(() => server.close());
```
MSW (Mock Service Worker) intercepts at the network layer — your code calls real `fetch`, MSW responds. The same handlers can run in browser tests too.

---

## 4. Practical Application — A Realistic Test Suite

A small `Cart` class with tests covering pure logic, async behavior, and a mocked dependency.

```js
// cart.js
export class Cart {
  constructor({ inventory }) {
    this.inventory = inventory; // injected — easy to mock
    this.items = new Map();
  }

  async add(productId, qty = 1) {
    const stock = await this.inventory.getStock(productId);
    if (stock < qty) throw new Error("Out of stock");
    this.items.set(productId, (this.items.get(productId) ?? 0) + qty);
  }

  remove(productId) { this.items.delete(productId); }

  total(prices) {
    let sum = 0;
    for (const [id, qty] of this.items) sum += (prices[id] ?? 0) * qty;
    return sum;
  }
}
```

```js
// cart.test.js
import { describe, it, beforeEach, expect, vi } from "vitest";
import { Cart } from "./cart.js";

describe("Cart", () => {
  let inventory, cart;

  beforeEach(() => {
    inventory = { getStock: vi.fn() };
    cart = new Cart({ inventory });
  });

  describe("add", () => {
    it("adds an item when stock is sufficient", async () => {
      inventory.getStock.mockResolvedValue(5);
      await cart.add("p1", 2);
      expect(cart.items.get("p1")).toBe(2);
      expect(inventory.getStock).toHaveBeenCalledWith("p1");
    });

    it("accumulates quantities", async () => {
      inventory.getStock.mockResolvedValue(10);
      await cart.add("p1", 1);
      await cart.add("p1", 2);
      expect(cart.items.get("p1")).toBe(3);
    });

    it("rejects when stock is insufficient", async () => {
      inventory.getStock.mockResolvedValue(0);
      await expect(cart.add("p1", 1)).rejects.toThrow("Out of stock");
      expect(cart.items.has("p1")).toBe(false);
    });
  });

  describe("total", () => {
    it("sums quantity * price for each item", async () => {
      inventory.getStock.mockResolvedValue(10);
      await cart.add("p1", 2);
      await cart.add("p2", 3);
      expect(cart.total({ p1: 5, p2: 10 })).toBe(2 * 5 + 3 * 10);
    });

    it("treats missing prices as zero", async () => {
      inventory.getStock.mockResolvedValue(10);
      await cart.add("missing", 5);
      expect(cart.total({})).toBe(0);
    });
  });
});
```

Patterns shown:
- Dependency injection (`inventory`) makes the class trivial to test.
- Each `it` does one thing.
- Group related tests with nested `describe`.
- Test the rejection path explicitly.

### End-to-end with Playwright
For a web app, e2e tests automate a real browser:

```js
// tests/e2e/login.spec.js
import { test, expect } from "@playwright/test";

test("user can log in", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.getByLabel("Email").fill("ada@example.com");
  await page.getByLabel("Password").fill("hunter2");
  await page.getByRole("button", { name: "Submit" }).click();
  await expect(page.getByText("Welcome, Ada")).toBeVisible();
});
```

Playwright runs Chromium, Firefox, WebKit. Fast, reliable, screenshots/video on failure. Strongly preferred over Cypress in 2026 for new projects.

---

## 5. Common Mistakes & Gotchas

- **Tests that share state** between runs ("ordered tests") — make every test independent.
- **Real network/filesystem in unit tests** — slow, flaky. Mock at the boundary or use MSW.
- **Time-dependent flakes:** `Date.now()` differs between runs. Use `vi.useFakeTimers()` or inject a clock.
- **Over-mocking:** mocking everything your subject calls means your test verifies the mocks, not the code. Mock external deps; let internal ones run.
- **Asserting implementation details:** brittle to refactors. Test outcomes, not internals.
- **Forgetting to `await`:** test passes by accident because the assertion runs before the promise rejects.
- **One giant test that asserts ten things.** When it fails you don't know which thing broke. One concept per test.
- **Snapshot dumping ground:** giant snapshots no one reviews. Snapshot small, intentional outputs.
- **Skipping flaky tests with `.skip`** instead of fixing them. Flake compounds; track and triage.
- **Mocking modules transitively imported** can desync between ESM/CJS. Vitest's `vi.mock` is hoisted; place at top.
- **Coverage chasing:** 100% line coverage doesn't mean correctness. Mutation testing (Stryker) is a more honest signal.

```js
// "Wat"
expect({a: 1}).toBe({a: 1}); // FAILS — different references; use toEqual
expect(NaN).toBe(NaN);       // PASSES — Jest/Vitest use Object.is, NOT ===
```

### Test naming convention
Be descriptive. `describe("when user not logged in", () => it("redirects to /login", ...))` reads naturally in failure output.

---

## 🎯 Key Takeaways

- **Vitest is the new default.** Faster than Jest, near-identical API, ESM-native.
- **Mock at the boundary, not within.** Dependency injection makes mocking trivial without messing with module systems.
- **Use MSW for HTTP** instead of mocking `fetch` directly — your real code path is exercised.
- **Pyramid: many unit, some integration, few e2e.** Inverted pyramids cause slow, flaky CI.
- **Test outcomes, not internals.** Tests that fail when you refactor without changing behavior are a tax, not a safety net.

---

*← [14 npm, package.json, semver](./14_npm_package_json_semver.md) | [next → 16 Tooling](./16_tooling.md)*
