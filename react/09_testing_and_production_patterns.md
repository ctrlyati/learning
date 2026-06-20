# 09 — Testing & Component Patterns

> **Goal:** Build reusable Compound Components, write behavior-driven tests with React Testing Library (RTL), and optimize bundle size via code-splitting.

---

## 1. Concept: Advanced UI Architecture

Writing clean React means building components that are easy to extend and simple to test.
- **Compound Components:** A pattern where a group of components work together to manage shared state implicitly (e.g. `<Select>` and `<Option>`). It avoids passing props down manually.
- **Behavior-Driven Testing:** Testing *what the user sees and interacts with*, rather than testing a component's internal state variables or private methods.

---

## 2. Mechanism: Compound Components & RTL

### Implicit State Sharing
Compound components utilize a small Context provider inside the parent component to share states with children dynamically, enabling highly flexible layouts:

```tsx
<Tabs defaultValue="home">
  <Tabs.List>
    <Tabs.Trigger value="home">Home</Tabs.Trigger>
    <Tabs.Trigger value="profile">Profile</Tabs.Trigger>
  </Tabs.List>
  <Tabs.Content value="home">Welcome home.</Tabs.Content>
  <Tabs.Content value="profile">Your Profile.</Tabs.Content>
</Tabs>
```

### React Testing Library (RTL)
RTL provides utilities to query the DOM just like a user would.
- **Bad practice:** Checking if `component.state.active` is true.
- **Best practice:** Querying by role: `screen.getByRole('button', { name: /submit/i })` and simulating clicks using `userEvent`.

---

## 3. Variations & Depth: Production Code-Splitting

As React applications grow, bundle sizes expand, delaying page load times.
- Use **`React.lazy()`** to import components dynamically only when they are needed.
- Wrap lazy components in `<Suspense>` boundaries to provide smooth loading transitions.

```typescript
const HeavyChart = React.lazy(() => import('./HeavyChart'));

function Dashboard() {
  return (
    <Suspense fallback={<p>Loading chart data...</p>}>
      <HeavyChart />
    </Suspense>
  );
}
```

---

## 4. Practical Application: A Compound Tabs Component

Let's build a functional Compound Tabs component and write a Vitest test suite for it.

**`Tabs.tsx`**
```tsx
import React, { createContext, useContext, useState } from 'react';

const TabsContext = createContext<{
  activeTab: string;
  setActiveTab: (val: string) => void;
} | undefined>(undefined);

export function Tabs({ defaultValue, children }: { defaultValue: string; children: React.ReactNode }) {
  const [activeTab, setActiveTab] = useState(defaultValue);

  return (
    <TabsContext.Provider value={{ activeTab, setActiveTab }}>
      <div style={{ fontFamily: 'sans-serif' }}>{children}</div>
    </TabsContext.Provider>
  );
}

Tabs.Trigger = function Trigger({ value, children }: { value: string; children: React.ReactNode }) {
  const context = useContext(TabsContext);
  if (!context) throw new Error('Tabs.Trigger must be inside Tabs');

  const isActive = context.activeTab === value;
  return (
    <button
      onClick={() => context.setActiveTab(value)}
      style={{
        padding: '8px 16px',
        border: 'none',
        borderBottom: isActive ? '2px solid blue' : 'none',
        background: 'none',
        cursor: 'pointer',
        fontWeight: isActive ? 'bold' : 'normal'
      }}
    >
      {children}
    </button>
  );
};

Tabs.Content = function Content({ value, children }: { value: string; children: React.ReactNode }) {
  const context = useContext(TabsContext);
  if (!context) throw new Error('Tabs.Content must be inside Tabs');

  if (context.activeTab !== value) return null;
  return <div style={{ padding: '15px' }}>{children}</div>;
};
```

**`Tabs.test.tsx` (Vitest & RTL)**
```tsx
import React from 'react';
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Tabs } from './Tabs';

describe('Tabs Component', () => {
  it('should render default content and switch tabs on click', async () => {
    render(
      <Tabs defaultValue="one">
        <Tabs.Trigger value="one">Tab One</Tabs.Trigger>
        <Tabs.Trigger value="two">Tab Two</Tabs.Trigger>
        <Tabs.Content value="one">Content One</Tabs.Content>
        <Tabs.Content value="two">Content Two</Tabs.Content>
      </Tabs>
    );

    // Assert initial active tab content is visible
    expect(screen.getByText('Content One')).toBeInTheDocument();
    expect(screen.queryByText('Content Two')).not.toBeInTheDocument();

    // Click Tab Two trigger
    const tabTwoTrigger = screen.getByRole('button', { name: /tab two/i });
    await userEvent.click(tabTwoTrigger);

    // Assert state changed and target content loaded
    expect(screen.getByText('Content Two')).toBeInTheDocument();
    expect(screen.queryByText('Content One')).not.toBeInTheDocument();
  });
});
```

---

## 5. Common Mistakes & Gotchas

- **Nesting component declarations:** Defining a component inside another component's render loop:
  ```typescript
  // BUG: Re-creates child component reference on EVERY render, wiping its DOM state!
  function Parent() {
    function Child() {
      return <input />;
    }
    return <Child />;
  }
  ```
  Always declare components separately at the root file level.
- **Testing implementation details:** Writing tests that assert component internals (like private functions or states). This makes refactoring impossible without breaking your test suite, even when component behavior remains unchanged.

---

## 🎯 Key Takeaways

- **Compound components encapsulate state** while offering flexible UI arrangements.
- **Mock networks at the network layer** using tools like MSW to avoid mocking internal fetching methods.
- **Adopt lazy loading** for dashboard widgets or heavy sub-pages to accelerate initial load performance.

---

*← [forms](./08_forms_and_validation.md) | [roadmap](../README.md)*
