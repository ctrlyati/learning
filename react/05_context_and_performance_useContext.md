# 05 — Context & Performance: useContext

> **Goal:** Optimize Context API structures, prevent consumer rendering cascades, and master useMemo, useCallback, and React.memo.

---

## 1. Concept: Context is not a State Manager

A common architectural error is using the **Context API** as a global state manager (like Redux or Zustand). 
- Context is a **dependency injection tool**. It passes values down a deep component tree without prop-drilling.
- **The catch:** Whenever the `value` provided to a Context Provider changes, **every single component that calls `useContext(MyContext)` re-renders automatically**, regardless of whether it uses the specific property that changed.

---

## 2. Mechanism: Optimizing Re-renders

To optimize renders, React provides three optimization primitives:

1. **`React.memo`**: A Higher-Order Component (HOC) that wraps your component. It skips re-rendering if props are shallowly equal to the previous render.
2. **`useMemo`**: Cache the *result* of a calculation across renders:
   ```javascript
   const expensiveValue = useMemo(() => computeHeavy(a), [a]);
   ```
3. **`useCallback`**: Cache the *function definition* itself across renders:
   ```javascript
   const handleAction = useCallback(() => doSomething(b), [b]);
   ```

### The Reference Equality Trap
If you pass a function to a memoized child component, the child will re-render anyway unless the function is memoized with `useCallback`. This is because in JS, `() => {} !== () => {}` (different memory references).

---

## 3. Variations & Depth: Split Context Pattern

To prevent static headers or control buttons from re-rendering when state values change, use the **Split Context Pattern**. Split your context provider into two:
1. `StateContext`: Contains state data.
2. `DispatchContext`: Contains state modifiers (updater functions or dispatch).

```typescript
// Split Context Pattern
const StateContext = createContext<State | null>(null);
const DispatchContext = createContext<Dispatch | null>(null);
```

Since the dispatch function reference never changes, components subscribing only to `DispatchContext` will never re-render.

---

## 4. Practical Application: Performance-Tuned Shopping Cart

Let's write a split-context system for a shopping cart to ensure item details don't re-render when user actions occur.

**`CartContext.tsx`**
```tsx
import React, { createContext, useContext, useReducer, useMemo, Dispatch } from 'react';

type CartState = { items: string[] };
type CartAction = { type: 'ADD'; payload: string };

const CartStateContext = createContext<CartState | undefined>(undefined);
const CartDispatchContext = createContext<Dispatch<CartAction> | undefined>(undefined);

function cartReducer(state: CartState, action: CartAction): CartState {
  switch (action.type) {
    case 'ADD':
      return { items: [...state.items, action.payload] };
    default:
      return state;
  }
}

export function CartProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(cartReducer, { items: [] });

  // Memoize state object to preserve reference stability
  const memoizedState = useMemo(() => state, [state]);

  return (
    <CartStateContext.Provider value={memoizedState}>
      <CartDispatchContext.Provider value={dispatch}>
        {children}
      </CartDispatchContext.Provider>
    </CartStateContext.Provider>
  );
}

// Custom hooks for consumption
export function useCartState() {
  const context = useContext(CartStateContext);
  if (!context) throw new Error('useCartState must be used within CartProvider');
  return context;
}

export function useCartDispatch() {
  const context = useContext(CartDispatchContext);
  if (!context) throw new Error('useCartDispatch must be used within CartProvider');
  return context;
}
```

**`CartDemo.tsx`**
```tsx
import React from 'react';
import { CartProvider, useCartState, useCartDispatch } from './CartContext';

// Memoized leaf node
const AddItemButton = React.memo(() => {
  const dispatch = useCartDispatch();
  console.log('[RENDER] AddItemButton'); // This will only render ONCE!

  return (
    <button onClick={() => dispatch({ type: 'ADD', payload: 'Laptop' })}>
      Add Laptop
    </button>
  );
});
AddItemButton.displayName = 'AddItemButton';

function CartList() {
  const state = useCartState();
  console.log('[RENDER] CartList'); // Renders on every item addition

  return (
    <ul>
      {state.items.map((item, idx) => (
        <li key={idx}>{item}</li>
      ))}
    </ul>
  );
}

export default function CartDemo() {
  return (
    <CartProvider>
      <AddItemButton />
      <CartList />
    </CartProvider>
  );
}
```

---

## 5. Common Mistakes & Gotchas

- **Exposing new objects in Context values:** Writing `<MyContext.Provider value={{ count, increment }}>` creates a brand-new object reference on *every single render*. This breaks all consumer-level memoization. Use `useMemo` to protect the context value object.
- **Over-memoizing primitive calculations:** Using `useMemo` for simple logic like `const upper = useMemo(() => name.toUpperCase(), [name])` is slower than calculating it directly, due to dependency array search overhead and memory reservation.
- **Forgetting dependency array values:** Omitting variables inside `useCallback` or `useMemo` lists can cause the cached function to reference stale closures forever.

---

## 🎯 Key Takeaways

- **Split Context into State and Dispatch channels** to protect action triggers from data update cascades.
- **Only memoize when performance issues are measured.** Check re-renders in React Profiler first.
- **React.memo only runs shallow comparisons.** If you pass inline objects as props, they will fail prop-diff checks.

---

*← [refs](./04_refs_and_dom_useRef_useImperativeHandle.md) | [next → 06 Transition & Suspense](./06_transition_and_suspense.md)*
