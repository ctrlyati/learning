# 07 — Data Fetching Patterns

> **Goal:** Eliminate network waterfalls, handle async race conditions in hooks, and master React 19's use() hook inside Suspense boundaries.

---

## 1. Concept: Fetch paradigms

How data is retrieved on the client impacts user experience and server load.

1. **Fetch-on-Render (Waterfalls):** Component mounts -> runs `useEffect` -> fetches data -> sets state -> mounts children -> children run `useEffect` and start their own fetches. This creates sequential loading delays.
2. **Fetch-then-Render:** Load all data first (via router resolvers or page loaders), then render the page.
3. **Render-as-you-Fetch (Suspense):** Start fetches and render components immediately. If a component reads data that isn't ready yet, it "suspends" execution, letting React render fallback placeholders until the promise resolves.

---

## 2. Mechanism: Race Conditions & React 19 `use()`

### The Race Condition Trap
In simple `useEffect` fetches, users can trigger actions fast (e.g. clicking tab "A" then tab "B").
- If the request for "A" finishes *after* the request for "B", "A"'s stale data will overwrite the state, leaving the user with incorrect UI.

```javascript
// BUG: Lacks cleanup guard
useEffect(() => {
  fetchData(id).then(res => setData(res));
}, [id]);
```

**The Guard:** Use an `active` boolean flag to discard stale resolutions:
```javascript
useEffect(() => {
  let active = true;
  fetchData(id).then(res => {
    if (active) setData(res);
  });
  return () => { active = false; }; // Cleanup invalidates request
}, [id]);
```

### React 19 `use()` Hook
In React 19, you can consume promises directly during the render phase using the **`use()`** hook. Unlike standard hooks, `use()` can be called conditionally or inside loops.

```typescript
const data = use(dataPromise); // Suspends parent until resolved
```

---

## 3. Variations & Depth: Query Libraries

While custom hooks work, production applications usually leverage query managers (like TanStack Query / React Query) to resolve:
- **Request Deduplication:** Merging duplicate network requests.
- **Garbage Collection:** Freeing memory of stale query cache entries.
- **Revalidation:** Refetching data on window focus or network reconnect.

---

## 4. Practical Application: Fetching with React 19 `use`

Let's write a component that uses the React 19 `use()` hook to read a promise within a `<Suspense>` wrapper.

**`UserDashboard.tsx`**
```tsx
import React, { Suspense, use } from 'react';

type User = { name: string; email: string };

// Simulated API
function fetchUser(id: string): Promise<User> {
  return new Promise((resolve) => {
    setTimeout(() => {
      resolve({ name: 'Jane Doe', email: 'jane@example.com' });
    }, 1500);
  });
}

// Global promise cache
const userPromise = fetchUser('42');

function UserDetails() {
  // Read promise directly in render. 
  // This suspends the component until resolution.
  const user = use(userPromise);

  return (
    <div>
      <p>Name: {user.name}</p>
      <p>Email: {user.email}</p>
    </div>
  );
}

export default function UserDashboard() {
  return (
    <div style={{ padding: '20px', border: '1px solid #eee' }}>
      <h3>User Profile</h3>
      {/* Suspense intercepts the loading phase */}
      <Suspense fallback={<p>Fetching profile details...</p>}>
        <UserDetails />
      </Suspense>
    </div>
  );
}
```

---

## 5. Common Mistakes & Gotchas

- **Re-creating promises on every render:** Passing an inline promise directly to `use()`:
  ```typescript
  // BUG: Creates a new fetch and suspends infinitely!
  function Buggy() {
    const data = use(fetch('/api')); 
  }
  ```
  Promises must be defined outside the component, read from cache, or passed down from parent props.
- **Forgetting error boundaries:** If a promise passed to `use()` rejects, the component crashes. Always wrap Suspense boundaries in a custom `ErrorBoundary` component to catch network failures.

---

## 🎯 Key Takeaways

- **Prevent race conditions** using cleanups that flag active requests.
- **Avoid fetch-on-render waterfalls.** Fetch data as high in the tree or as early as possible.
- **React 19 `use()` enables promise consumption** directly in the render phase, simplifying Suspense integration.

---

*← [transition](./06_transition_and_suspense.md) | [next → 08 Forms & Validation](./08_forms_and_validation.md)*
