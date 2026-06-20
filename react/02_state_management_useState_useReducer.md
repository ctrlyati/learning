# 02 — State Management: useState & useReducer

> **Goal:** Trace state update pipelines, master Automatic Batching, and choose between useState and useReducer under a senior-engineer lens.

---

## 1. Concept: Hooks are a Linked List

When a component renders, React stores its hooks in a singly-linked list attached to the component's Fiber node (`fiber.memoizedState`). 

```
[Fiber Node]
     |
     v
[Hook 1: useState] ---> [Hook 2: useEffect] ---> [Hook 3: useReducer]
```

Every time React runs your component function:
1. It reads hooks sequentially from the beginning of this list.
2. **This is why hooks cannot be placed inside `if` statements, loops, or nested functions.** If the order of hook calls shifts, React will map the wrong state record to your hook variables.

---

## 2. Mechanism: Rendering & Automatic Batching

### What happens when you update state?
Calling `setCount(5)` does not change the local `count` variable in your active closure. Instead, it creates an **update object** containing the new value and pushes it onto a queue on the Fiber node. It then schedules a re-render. When React calls the component function again, it applies the updates sequentially to compute the new state.

### Automatic Batching (React 18+)
Instead of executing a re-render immediately for every individual state change, React batches multiple state updates together into a single render pass.
- In React 17, batching only occurred inside React event handlers.
- In React 18+, updates inside promises, timeouts, and native event handlers are **automatically batched** as well.

```javascript
// React 18: Only 1 re-render occurs here
setTimeout(() => {
  setCount(c => c + 1);
  setFlag(f => !f);
}, 100);
```

### Functional Updates vs Value Updates
If you call state updates referencing the stale state variable, you'll overwrite concurrent changes. Always use functional updates when the next state depends on the previous state:

```javascript
// BUG: If called multiple times in a single event loop, count increases by only 1
setCount(count + 1);
setCount(count + 1);

// CORRECT: Count increases by 2
setCount(c => c + 1);
setCount(c => c + 1);
```

---

## 3. Variations & Depth: useState vs useReducer

- **`useState`**: Syntactic sugar wrapping `useReducer`. Ideal for primitive, isolated states.
- **`useReducer`**: Best when complex state changes share dependency rules (e.g. form validations or state-machine architectures).

```typescript
// useReducer structure
const [state, dispatch] = useReducer(reducer, initialState);
```
Using a reducer separates state update logic from UI rendering files, enabling cleaner testing.

---

## 4. Practical Application: Automatic Batching & Closures

This component demonstrates how state batching operates and illustrates how closures capture stale state.

**`BatchingDemo.tsx`**
```tsx
import React, { useState } from 'react';

export default function BatchingDemo() {
  const [count, setCount] = useState(0);
  const [renderCount, setRenderCount] = useState(0);

  // Track total renders
  React.useEffect(() => {
    setRenderCount(r => r + 1);
  }, [count]);

  const handleAsyncUpdate = () => {
    // Both state changes run inside an async callback
    setTimeout(() => {
      setCount(c => c + 1);
      setCount(c => c + 2);
      // Under React 18, the above updates trigger exactly ONE render.
      // Notice: If you logged `count` here, it would still show the old value (0) 
      // because of closure boundaries.
    }, 100);
  };

  return (
    <div style={{ padding: '15px', border: '1px solid #ddd' }}>
      <p>Count: {count}</p>
      <p>Render count: {renderCount}</p>
      <button onClick={handleAsyncUpdate}>
        Trigger Double Async Increment
      </button>
    </div>
  );
}
```

---

## 5. Common Mistakes & Gotchas

- **Direct State Mutation:** Modifying arrays or objects directly (e.g. `state.user.name = "John"` followed by `setUser(state)`) will not trigger a re-render. React compares objects by reference. If the object reference remains identical, React skips rendering.
  ```typescript
  // CORRECT: Always shallow copy
  setUser({ ...state.user, name: "John" });
  ```
- **Expecting synchronous state updates:** Setting state and reading it on the next line:
  ```javascript
  const [active, setActive] = useState(false);
  const handleToggle = () => {
    setActive(!active);
    console.log(active); // BUG: prints old state value!
  };
  ```

---

## 🎯 Key Takeaways

- **Keep Hook calls top-level and unconditional** to preserve their linked-list index order.
- **Utilize functional updates (`c => c + 1`)** to guarantee calculations read current values instead of stale closures.
- **State updates are batched asynchronously.** Never count on state changes appearing in the DOM synchronously.

---

*← [virtual dom](./01_virtual_dom_and_reconciliation.md) | [next → 03 Effects & Synchronization](./03_effects_and_sync_useEffect.md)*
