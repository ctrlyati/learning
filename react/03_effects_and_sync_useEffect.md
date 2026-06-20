# 03 — Effects & Synchronization: useEffect

> **Goal:** Align useEffect with React's synchronization mental model, avoid common dependency array mistakes, and choose between useEffect and useLayoutEffect.

---

## 1. Concept: What is an Effect?

In React, **Effects are escape hatches** used to synchronize your component state with external systems (such as a WebSocket, a browser API, a map widget, or an analytical service).

**Effects are not for updating state based on state.** If you can calculate a value during the render phase (e.g. formatting a string, calculating a sum, filtering an array), do it synchronously inside the component body, not inside a `useEffect`.

---

## 2. Mechanism: Lifecycle and Cleanup

An effect's lifecycle is simple: **Start synchronization, and then stop it.**

```
Render -> Mount Component -> Run Effect (Sync Start)
                                 |
State Update -> Run Cleanup (Sync Stop) -> Run Effect (Sync Start with new deps)
                                 |
Unmount Component -> Run Cleanup (Sync Stop)
```

### The Cleanup Function
Every effect can return a cleanup function. React runs this cleanup function:
1. Every time *before* the effect runs again (due to changing dependencies).
2. Once when the component unmounts.

Failing to clean up timers, intervals, socket connections, or event listeners leads to resource leaks.

---

## 3. Variations & Depth: useEffect vs useLayoutEffect

React provides three hooks for side effects:
- **`useEffect` (Default):** Runs **asynchronously** *after* the browser has painted the DOM updates to the screen. Does not block UI updates.
- **`useLayoutEffect`:** Runs **synchronously** *before* the browser paints. Use this if you need to measure the DOM (like element width) and adjust layout styles before the user sees the screen to prevent visual flickering.
- **`useInsertionEffect` (React 18+):** Runs before DOM mutations. Primarily used by CSS-in-JS libraries to inject `<style>` tags dynamically.

---

## 4. Practical Application: A Resilient Resize Hook

Let's write a custom hook that wraps a debounced browser window resize event listener, utilizing proper cleanup and dependency arrays.

**`useWindowSize.ts`**
```typescript
import { useState, useEffect } from 'react';

export function useWindowSize(debounceMs: number = 100) {
  const [size, setSize] = useState({
    width: typeof window !== 'undefined' ? window.innerWidth : 0,
    height: typeof window !== 'undefined' ? window.innerHeight : 0
  });

  useEffect(() => {
    let timeoutId: number;

    const handleResize = () => {
      // Clear previous timeout to debounce the state update
      clearTimeout(timeoutId);
      
      timeoutId = window.setTimeout(() => {
        setSize({
          width: window.innerWidth,
          height: window.innerHeight
        });
      }, debounceMs);
    };

    window.addEventListener('resize', handleResize);

    // CLEANUP: Must remove listener when deps change or component unmounts
    return () => {
      window.removeEventListener('resize', handleResize);
      clearTimeout(timeoutId);
    };
  }, [debounceMs]); // Re-run hook if debounce time configuration changes

  return size;
}
```

---

## 5. Common Mistakes & Gotchas

- **Missing Cleanup Functions:** Registering `window.addEventListener` or starting a `setInterval` without returning a cleanup function. Over time, multiple copies of these events pile up in memory.
- **Object dependencies:** Passing an un-memoized object or array directly into the dependency list:
  ```typescript
  // BUG: This effect runs on EVERY render!
  // {} !== {} in JavaScript shallow comparison.
  useEffect(() => {
    console.log("Config changed");
  }, [{ theme: "dark" }]); 
  ```
- **Infinite Loop Cascades:** Updating state inside an effect without a dependency array, or where the updated state is listed as a dependency itself:
  ```typescript
  // BUG: Infinite rendering loops
  useEffect(() => {
    setCount(count + 1);
  }, [count]);
  ```

---

## 🎯 Key Takeaways

- **Effects synchronize, they don't orchestrate.** Avoid writing chains of effects updating other state variables.
- **Always return cleanups** for open connections, subscriptions, intervals, and EventListeners.
- **Prefer event handlers over effects** for user-triggered business logic (like form submissions or button clicks).

---

*← [state management](./02_state_management_useState_useReducer.md) | [next → 04 Refs & DOM Manipulation](./04_refs_and_dom_useRef_useImperativeHandle.md)*
