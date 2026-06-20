# 06 — Transition & Suspense: Concurrent React

> **Goal:** Leverage Concurrent features to keep inputs responsive under heavy loads, use useDeferredValue, and coordinate Suspense boundaries.

---

## 1. Concept: Responsive UIs under High Load

Historically, React rendering was synchronous and blocking. If a user typed in a search box and React had to render a list of 1,000 complex items, the typing input would lag.

In React 18+, **Concurrent Rendering** allows React to interrupt a slow, low-priority rendering loop to handle an urgent user event (like keyboard typing or click selectors), then resume the background rendering.

---

## 2. Mechanism: Transitions & Suspense

### `useTransition` / `startTransition`
Transitions split state updates into two categories:
1. **Urgent Updates:** Direct interactions (typing, clicking check boxes). Must be synchronous.
2. **Transition Updates:** Moving from one view to another or rendering results. Can be deferred.

```typescript
const [isPending, startTransition] = useTransition();

// Typing is urgent: updates input immediately
setQuery(e.target.value); 

// Filtering is a transition: runs in the background
startTransition(() => {
  setFilterQuery(e.target.value); 
});
```

### `useDeferredValue`
If you do not have control over the state setter (e.g. state is passed down as a prop from a library), you can use `useDeferredValue`. It yields a copy of the value that lags behind the current state value during heavy renders.

```typescript
const deferredQuery = useDeferredValue(query);
```

### Suspense Boundaries
`<Suspense>` allows you to specify a loading fallback UI for child trees that are not yet ready to render (due to dynamic imports, code splitting, or pending data loads).

---

## 3. Variations & Depth: Fiber Branching

Under the hood, when you trigger a transition, React starts rendering the new state in a **work-in-progress branch** inside memory.
- If the user types a new character before the background branch finishes, React discards the current background branch work.
- It starts a new rendering pass based on the newest character, keeping the screen responsive.

---

## 4. Practical Application: A Typing-Safe Search Filter

Let's build a component where a user can type rapidly without input lag, even while rendering a large, heavy list.

**`HeavySearch.tsx`**
```tsx
import React, { useState, useDeferredValue, useMemo } from 'react';

// A mock heavy component that wastes CPU cycles
const HeavyListItem = React.memo(({ name }: { name: string }) => {
  const startTime = performance.now();
  // Artificial CPU block
  while (performance.now() - startTime < 3) {}
  return <li>{name}</li>;
});
HeavyListItem.displayName = 'HeavyListItem';

function HeavyList({ query }: { query: string }) {
  const items = useMemo(() => {
    return Array.from({ length: 500 }, (_, i) => `Item ${i + 1} matching "${query}"`);
  }, [query]);

  return (
    <ul>
      {items.map((item, idx) => (
        <HeavyListItem key={idx} name={item} />
      ))}
    </ul>
  );
}

export default function HeavySearch() {
  const [query, setQuery] = useState('');
  
  // Defer the query passed to the heavy list
  const deferredQuery = useDeferredValue(query);
  
  const isStale = query !== deferredQuery;

  return (
    <div style={{ padding: '20px' }}>
      <input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Type to filter..."
        style={{ padding: '8px', width: '250px' }}
      />
      {isStale && <p style={{ color: 'gray' }}>Calculating updates...</p>}
      
      <div style={{ opacity: isStale ? 0.5 : 1, transition: 'opacity 0.2s' }}>
        <HeavyList query={deferredQuery} />
      </div>
    </div>
  );
}
```

---

## 5. Common Mistakes & Gotchas

- **Putting input values in transitions:** Wrapping an input's value state modifier in a transition:
  ```typescript
  // BUG: Typing inside this input will lag severely!
  onChange={(e) => startTransition(() => setVal(e.target.value))}
  ```
  Inputs must remain synchronous to reflect keyboard strokes instantly. Only transition the *result* calculations.
- **Forgetting Suspense boundaries:** Using async components or libraries (like `React.lazy`) without wrapping them in a `<Suspense fallback={<Spinner />}>` block will throw a crash error at runtime.
- **Triggering transitions on every state update:** Transitions add background branches. If overused, memory footprint increases, degrading performance.

---

## 🎯 Key Takeaways

- **Transitions make rendering interruptible**, ensuring keyboard/cursor actions remain responsive.
- **Use `useDeferredValue`** when you receive incoming data props that trigger expensive child-tree updates.
- **Always keep control input states synchronous.** Defer only the derived heavy operations.

---

*← [context](./05_context_and_performance_useContext.md) | [next → 07 Data Fetching Patterns](./07_data_fetching_patterns.md)*
