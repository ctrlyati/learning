# 04 — Refs & DOM: useRef & useImperativeHandle

> **Goal:** Differentiate between ref mutations and state changes, expose targeted component interfaces via useImperativeHandle, and master React 19 ref updates.

---

## 1. Concept: Refs are Escape Hatches

A **ref** is a generic container containing a mutable value inside its `.current` property.
There are two major differences between states and refs:
1. **Mutating a ref does not trigger a re-render.**
2. **Refs are synchronous.** If you write a value to a ref, it updates instantly.

```javascript
const countRef = useRef(0);
const handleClick = () => {
  countRef.current += 1; // updates immediately, does not trigger render
};
```

---

## 2. Mechanism: DOM Refs & Ref Forwarding

When you pass a ref to a built-in HTML node (e.g. `<input ref={myRef} />`), React writes the actual browser DOM element node to `myRef.current` during the commit phase.

### Ref Forwarding (`forwardRef`)
In React 18, functional components do not expose their internal DOM nodes to parents. To support this, you must wrap components in `forwardRef`:

```typescript
// React 18 syntax
const MyInput = forwardRef((props, ref) => {
  return <input ref={ref} {...props} />;
});
```

*Note: In React 19, `forwardRef` is deprecated. You can pass `ref` directly as a standard prop, just like `className` or `id`.*

---

## 3. Variations & Depth: Callback Refs & useImperativeHandle

### `useImperativeHandle`
Instead of exposing the raw DOM element node (which allows parents to modify classes or styles directly, bypassing React), you can customize the exposed reference value using `useImperativeHandle`. This lets you expose only a minimal, safe API.

```typescript
useImperativeHandle(ref, () => ({
  focus() {
    inputRef.current.focus();
  }
}));
```

### Callback Refs
Instead of passing a ref object, you can pass a callback function to the `ref` prop: `<div ref={(node) => console.log(node)} />`. React calls this function with the DOM element when it mounts, and with `null` when it unmounts. This is useful for responding to element size measurements or attaching third-party libraries.

---

## 4. Practical Application: A Custom Media Controller

Let's build a custom video player component that encapsulates the `<video>` element, exposing only `play()` and `pause()` methods to the parent component.

**`VideoPlayer.tsx`**
```tsx
import React, { useRef, useImperativeHandle, forwardRef } from 'react';

export interface VideoPlayerRef {
  playVideo: () => void;
  pauseVideo: () => void;
}

// React 18 version (compatible)
export const VideoPlayer = forwardRef<VideoPlayerRef, { src: string }>((props, ref) => {
  const videoRef = useRef<HTMLVideoElement>(null);

  // Expose clean controls, hiding raw HTMLVideoElement methods
  useImperativeHandle(ref, () => ({
    playVideo() {
      if (videoRef.current) videoRef.current.play();
    },
    pauseVideo() {
      if (videoRef.current) videoRef.current.pause();
    }
  }));

  return (
    <video
      ref={videoRef}
      src={props.src}
      style={{ width: '100%', maxWidth: '400px', borderRadius: '8px' }}
    />
  );
});

VideoPlayer.displayName = 'VideoPlayer';
```

**`App.tsx` (Parent)**
```tsx
import React, { useRef } from 'react';
import { VideoPlayer, VideoPlayerRef } from './VideoPlayer';

export default function App() {
  const playerRef = useRef<VideoPlayerRef>(null);

  return (
    <div style={{ padding: '20px' }}>
      <VideoPlayer 
        ref={playerRef} 
        src="https://www.w3schools.com/html/mov_bbb.mp4" 
      />
      <div style={{ marginTop: '10px' }}>
        <button onClick={() => playerRef.current?.playVideo()}>Play</button>
        <button onClick={() => playerRef.current?.pauseVideo()}>Pause</button>
      </div>
    </div>
  );
}
```

---

## 5. Common Mistakes & Gotchas

- **Reading/Writing refs during render:** You should not write or read `ref.current` inside the body of your component function (during render). This breaks render predictability and causes issues with React's concurrent mode. **Only interact with refs inside event handlers or `useEffect`.**
  ```typescript
  // BAD: Render-phase side effect
  function Component() {
    const renderCount = useRef(0);
    renderCount.current++; // BUG!
    return <div>{renderCount.current}</div>;
  }
  ```
- **Failing to check for null:** When using refs, `ref.current` is initialized to your default argument (often `null`). Always verify it exists before calling native APIs (`myRef.current?.focus()`).

---

## 🎯 Key Takeaways

- **Refs store values that shouldn't re-trigger renders.**
- **Use `useImperativeHandle`** to avoid leaky abstractions by hiding raw DOM handles.
- **Never interact with refs during the render phase.** Restrict mutations to lifecycle events or handlers.

---

*← [effects](./03_effects_and_sync_useEffect.md) | [next → 05 Context & Performance: useContext](./05_context_and_performance_useContext.md)*
