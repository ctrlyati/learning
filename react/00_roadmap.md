# 00 — React Deep-Dive Roadmap

> **Goal:** Take a developer with basic React knowledge and lift them to professional-grade mastery of React internals, fiber reconciliation, performance optimization, and concurrent features.

This course focuses on React's architecture, underlying scheduling mechanisms, state batching, effect synchronization, context performance, and advanced concurrent rendering patterns (React 18/19). By the end of this course, you will understand how the Fiber tree is built, how to debug unnecessary re-renders, when to reach for Suspense, and how to write clean, concurrent components.

---

## Prerequisites

Before starting, you should be comfortable with:
- **JavaScript & TypeScript** — core JS features, ESM, scoping, variables, and type safety. See [`../javascript/00_roadmap.md`](../javascript/00_roadmap.md).
- **Basic React concepts** — writing functional components, passing props, using basic hooks (`useState`, `useEffect`).
- **DOM & Browser APIs** — basic event handlers, HTML elements, style layouts.

---

## Module Table

| #  | File                                                           | Topic                                                         | Est. Time |
|----|----------------------------------------------------------------|---------------------------------------------------------------|-----------|
| 00 | `00_roadmap.md`                                                | This file                                                     | 30 min    |
| 01 | `01_virtual_dom_and_reconciliation.md`                         | React Fiber, render vs commit, reconciliation, diffing keys   | 2.5 h     |
| 02 | `02_state_management_useState_useReducer.md`                   | State scheduling, automatic batching, dispatcher, useReducer   | 2.5 h     |
| 03 | `03_effects_and_sync_useEffect.md`                             | Syncing with external systems, effects vs handlers, dependency array| 3 h |
| 04 | `04_refs_and_dom_useRef_useImperativeHandle.md`                | Mutable refs, DOM manipulation, forwardRef, imperative handles| 2 h       |
| 05 | `05_context_and_performance_useContext.md`                     | Prop drilling, useContext, context tuning, memoization hooks  | 2.5 h     |
| 06 | `06_transition_and_suspense.md`                                | Concurrent features, useTransition, useDeferredValue, Suspense| 3 h       |
| 07 | `07_data_fetching_patterns.md`                                 | Fetch-on-render vs render-as-you-fetch, cache state sync      | 2.5 h     |
| 08 | `08_forms_and_validation.md`                                   | Controlled vs uncontrolled, form handlers, React 19 Form Actions| 2.5 h     |
| 09 | `09_testing_and_production_patterns.md`                        | Compound components, render props, RTL, MSW, bundle footprint  | 3 h       |

**Total**: ~24 hours of study. At 1 module per day, it takes about **1.5 to 2 weeks**.

---

## Core Mental Models

### 1. The Fiber Tree is a linked list representation of your UI
React doesn't perform full-tree DOM comparisons on every render. Instead, it compiles JSX into a tree of **Fiber nodes** (stored as a singly-linked list with parent, child, and sibling pointers). Fiber allows React to pause, resume, or abort UI calculations to keep the main thread responsive.

### 2. Rendering is separate from committing
- **Render Phase:** React computes which parts of the UI need to change by calling your components and diffing the virtual trees. This phase is purely computational and can be paused or restarted by React.
- **Commit Phase:** React applies the calculated mutations directly to the DOM. This phase is synchronous and cannot be interrupted.

### 3. State update functions schedule renders, they do not mutate state
When you call `setCount(count + 1)`, the variable `count` in the current function execution does *not* change. Calling the setter schedules a new render loop, telling React to run the component function again with the updated value in the next frame.

### 4. Effects are for synchronization, not state orchestration
Do not use `useEffect` to synchronize state values (e.g. updating `fullName` when `firstName` changes). Calculate derived states synchronously during render. Use `useEffect` only for synchronizing your application with external systems (like WebSockets, map overlays, or raw DOM libraries).

---

## External Resources

- **[React Docs](https://react.dev/)** — The official React documentation.
- **[React Working Group GitHub](https://github.com/reactwg/react-18/discussions)** — Invaluable details on concurrent feature designs.
- **[Overreacted.io](https://overreacted.io/)** — Dan Abramov's blog explaining React's design decisions.

---

*next →* [`01_virtual_dom_and_reconciliation.md`](./01_virtual_dom_and_reconciliation.md)
