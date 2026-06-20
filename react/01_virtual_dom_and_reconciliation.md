# 01 — Virtual DOM & Reconciliation

> **Goal:** Explore React's Fiber architecture, trace the Render vs Commit lifecycle, and understand the internal role of keys.

---

## 1. Concept: What is React Fiber?

React uses a **Virtual DOM** to avoid expensive direct manipulation of the browser DOM. But what actually is it?

In React 16+, the virtual representation of UI is the **Fiber Tree**. A `Fiber` is a plain JavaScript object representing a unit of work. Fibers are organized in a singly-linked list tree structure using three main pointers:
- **`child`**: Points to the first direct child.
- **`sibling`**: Points to the next sibling node.
- **`return`**: Points to the parent node (where this fiber returns when finished).

```
        [App Fiber]
             |
          (child)
             v
       [Header Fiber] -- (sibling) --> [Main Fiber]
```

This linked-list layout enables **incremental rendering**: React can pause rendering computations to handle urgent user inputs, then resume where it left off.

---

## 2. Mechanism: Render Phase vs Commit Phase

The lifecycle of a state update is split into two phases:

### Phase 1: Render (Asynchronous & Interruptible)
- React starts at the root fiber, walks the tree, and calls your component functions.
- It builds a **work-in-progress** tree.
- It compares (diffs) the work-in-progress tree with the currently visible **current** tree to compile a list of mutations (effects).
- *This phase does not touch the actual DOM.* React can interrupt, discard, or reschedule this work.

### Phase 2: Commit (Synchronous & Uninterruptible)
- React takes the list of mutations and writes them to the DOM using native calls (`appendChild`, `removeChild`, etc.).
- Life-cycle hooks and layout effects (`useLayoutEffect`) are executed.
- V8 references update so the work-in-progress tree becomes the new "current" tree (Double Buffering).

### The Diffing Heuristic
Reconciliation compares two trees in $O(n)$ time using two assumptions:
1. Two elements of different HTML types will produce different trees (React tears down the old tree and builds a new one).
2. Elements can indicate stability across renders using a unique `key` prop.

---

## 3. Variations & Depth: Double Buffering & Keys

To prevent flickering, React builds its updates in a shadow tree. Only once the work-in-progress tree is fully prepared is it swapped into view. This is identical to **double buffering** in graphics rendering.

```
Visible to User:  [ Current Tree ]
                        ^ (Swap)
                        |
Calculated in Bg: [ Work-In-Progress Tree ]
```

### The role of `key`
Keys tell React which items in a list have changed, been added, or been removed.
- **Without keys (or index-based keys):** React diffs elements positionally. If you insert an item at the beginning of a list, React thinks *every single item* has changed, re-rendering them all.
- **With unique keys:** React shifts the actual DOM elements around without re-evaluating the inner child components.

---

## 4. Practical Application: Measuring Render and Keys

This component demonstrates how changing a key forces React to discard a component's internal state.

**`KeyDemo.tsx`**
```tsx
import React, { useState } from 'react';

function CounterInput() {
  const [value, setValue] = useState('');
  
  return (
    <div style={{ border: '1px solid #ccc', padding: '10px', margin: '5px' }}>
      <p>Type something to test state retention:</p>
      <input value={value} onChange={(e) => setValue(e.target.value)} />
    </div>
  );
}

export default function KeyDemo() {
  const [toggleKey, setToggleKey] = useState(false);

  return (
    <div>
      <h3>Scenario A: Normal Render (State Persists)</h3>
      <CounterInput />

      <h3>Scenario B: Key Changes (State Destroys)</h3>
      {/* Changing the key resets the internal input state because React treats it as a brand new component tree slice */}
      <CounterInput key={toggleKey ? 'a' : 'b'} />

      <button onClick={() => setToggleKey(!toggleKey)}>
        Trigger Key Change Toggle
      </button>
    </div>
  );
}
```

---

## 5. Common Mistakes & Gotchas

- **Using `Math.random()` as a key:** Since the key changes on *every single render*, React destroys and rebuilds the component DOM element from scratch. This causes input fields to lose focus immediately and degrades performance.
- **Array index as key in dynamic lists:** If you sort, filter, or prepend items, index keys (`key={0}`, `key={1}`) point to the wrong components. Form state or checkboxes will fail to move with their labels.
- **Changing tag wrapper types dynamically:** Changing a container wrapper (e.g. going from `<div><Child /></div>` to `<section><Child /></section>`) will tear down the entire `<Child />` component state and DOM nodes, even though `<Child />` itself was not modified.

---

## 🎯 Key Takeaways

- **React Fiber splits work into chunks** to prevent layout blocking during calculation.
- **Rendering is pure computation** (Render Phase); DOM mutations are applied in one atomic block (Commit Phase).
- **A key is an identity token, not just an index.** Keep it unique and stable to preserve component states.

---

*← [roadmap](./00_roadmap.md) | [next → 02 State Management](./02_state_management_useState_useReducer.md)*
