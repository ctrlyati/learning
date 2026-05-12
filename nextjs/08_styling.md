# 08 — Styling

> **Goal:** Pick and configure the right styling approach for App Router projects — CSS Modules, Tailwind, global CSS — and understand why CSS-in-JS is constrained in Server Components.

---

## 1. Concept — global CSS, CSS Modules, and the styling triad

Three styling approaches work seamlessly with the App Router out of the box:

1. **Global CSS** — imported once in the root layout, applies everywhere.
2. **CSS Modules** — scoped class names per file (`Button.module.css`).
3. **Tailwind CSS** — utility classes; the de facto default in 2025 Next.js projects.

```css
/* app/globals.css */
@tailwind base;
@tailwind components;
@tailwind utilities;

:root {
  --background: #fff;
  --foreground: #000;
}

body {
  background: var(--background);
  color: var(--foreground);
}
```

```tsx
// app/layout.tsx
import "./globals.css";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return <html><body>{children}</body></html>;
}
```

A CSS Module:

```css
/* app/components/Button.module.css */
.button {
  background: black;
  color: white;
  padding: 0.5rem 0.75rem;
  border-radius: 0.25rem;
}
.button:hover { opacity: 0.9; }
```

```tsx
// app/components/Button.tsx
import styles from "./Button.module.css";

export function Button({ children }: { children: React.ReactNode }) {
  return <button className={styles.button}>{children}</button>;
}
```

CSS Modules work in server *and* client components. They're scoped at build time — the class names you import are hashed (e.g., `Button_button__a1b2c`).

---

## 2. Mechanism — how Next.js handles CSS

At build time, the bundler:

- **Imports `globals.css`** from the root layout, emits it once. It's served as a `<link rel="stylesheet">` in the `<head>`.
- For **CSS Modules**, generates a hashed class map per file. The map is imported as a JS object; the CSS is collected into a per-route stylesheet (or a chunked stylesheet for shared components).
- For **Tailwind**, runs the Tailwind compiler over your source files, generating a single CSS file with only the utilities you actually used (`content` array in `tailwind.config.js`).
- Inlines critical CSS where appropriate; defers the rest.

For **App Router**, Next.js automatically chunks CSS per route so a page doesn't ship CSS for components it doesn't use. There's nothing to configure.

### Why CSS-in-JS is constrained in RSC

Server Components render to HTML on the server with no client runtime. A CSS-in-JS library that injects styles at *runtime* on the client (e.g., styled-components in its classic mode, Emotion) has nothing to do during the server pass — the result is unstyled HTML, or the library has to ship a runtime to the client anyway, eliminating the bundle-size benefit of RSC.

Libraries that work well with RSC are ones with **zero-runtime / compile-time** extraction (vanilla-extract, Panda, StyleX, Linaria). They extract styles to static CSS at build, so the server can ship pre-baked styles.

If you want styled-components or Emotion specifically:
- They can be used in **Client Components** with a registry pattern (Next provides docs).
- They cannot be used in Server Components.
- The runtime cost is small but real; most teams have moved to Tailwind or vanilla-extract.

---

## 3. Variations / depth

### 3.1 Tailwind setup

`create-next-app --tailwind` already wires this up. The pieces:

```js
// tailwind.config.ts
import type { Config } from "tailwindcss";

export default {
  content: [
    "./src/app/**/*.{ts,tsx}",
    "./src/components/**/*.{ts,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        brand: { 500: "#6366f1", 700: "#4338ca" },
      },
    },
  },
  plugins: [],
} satisfies Config;
```

```css
/* app/globals.css */
@tailwind base;
@tailwind components;
@tailwind utilities;
```

Tailwind v4 (preview at time of writing) inverts the configuration into CSS:

```css
/* app/globals.css (Tailwind v4) */
@import "tailwindcss";
@theme {
  --color-brand-500: #6366f1;
}
```

Check the Tailwind docs for your installed version.

### 3.2 `cn` / `clsx` for conditional classes

A small utility most projects add:

```ts
// lib/cn.ts
import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
```

```tsx
import { cn } from "@/lib/cn";

<button className={cn(
  "rounded px-3 py-1",
  disabled && "opacity-50 cursor-not-allowed",
  primary ? "bg-black text-white" : "bg-white text-black border"
)} />
```

`twMerge` resolves conflicting Tailwind classes (`px-2` + `px-4` → `px-4`). Essential for component variants.

### 3.3 Variants with `class-variance-authority`

For design-system primitives:

```tsx
// components/ui/Button.tsx
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/cn";

const button = cva("inline-flex items-center rounded font-medium", {
  variants: {
    intent: {
      primary: "bg-black text-white hover:opacity-90",
      ghost: "bg-transparent text-neutral-900 hover:bg-neutral-100",
    },
    size: {
      sm: "h-7 px-2 text-sm",
      md: "h-9 px-3 text-base",
    },
  },
  defaultVariants: { intent: "primary", size: "md" },
});

type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & VariantProps<typeof button>;

export function Button({ intent, size, className, ...rest }: ButtonProps) {
  return <button className={cn(button({ intent, size }), className)} {...rest} />;
}
```

This is the pattern shadcn/ui uses; it works flawlessly with RSC because everything is just class names.

### 3.4 Theming with CSS variables

The cleanest approach (works in RSC, no runtime):

```css
/* app/globals.css */
:root {
  --background: 0 0% 100%;
  --foreground: 0 0% 9%;
  --card: 0 0% 98%;
}
.dark {
  --background: 0 0% 9%;
  --foreground: 0 0% 98%;
  --card: 0 0% 12%;
}
```

```ts
// tailwind.config.ts excerpt
extend: {
  colors: {
    background: "hsl(var(--background))",
    foreground: "hsl(var(--foreground))",
    card: "hsl(var(--card))",
  },
},
```

Toggle dark mode by adding/removing `.dark` on `<html>` (e.g., via `next-themes`). Because variables are CSS-native, no JS reads them — the theme switch is instant.

### 3.5 `next-themes`

```bash
pnpm add next-themes
```

```tsx
// app/providers.tsx
"use client";
import { ThemeProvider } from "next-themes";

export function Providers({ children }: { children: React.ReactNode }) {
  return <ThemeProvider attribute="class" defaultTheme="system" enableSystem>{children}</ThemeProvider>;
}
```

```tsx
// app/layout.tsx
import { Providers } from "./providers";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
```

`suppressHydrationWarning` on `<html>` is necessary because `next-themes` sets the `class` attribute before React hydrates, causing a deliberate mismatch.

### 3.6 vanilla-extract (zero-runtime CSS-in-TS)

For projects that want TypeScript-typed styles without runtime:

```ts
// styles/button.css.ts
import { style } from "@vanilla-extract/css";

export const button = style({
  background: "black",
  color: "white",
  padding: "0.5rem 0.75rem",
  borderRadius: "0.25rem",
});
```

```tsx
import { button } from "./styles/button.css";
<button className={button}>Click</button>
```

Requires the `@vanilla-extract/next-plugin`. Outputs static CSS at build, works in RSC.

---

## 4. Practical application — a typed Button + Card design system slice

```tsx
// components/ui/Button.tsx
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/cn";
import { forwardRef } from "react";

const button = cva(
  "inline-flex items-center justify-center rounded font-medium transition focus:outline-none focus:ring-2 focus:ring-brand-500 disabled:opacity-50",
  {
    variants: {
      intent: {
        primary: "bg-brand-500 text-white hover:bg-brand-700",
        secondary: "bg-neutral-200 text-neutral-900 hover:bg-neutral-300",
        ghost: "bg-transparent hover:bg-neutral-100",
        danger: "bg-red-600 text-white hover:bg-red-700",
      },
      size: { sm: "h-7 px-2 text-sm", md: "h-9 px-3", lg: "h-11 px-4 text-lg" },
    },
    defaultVariants: { intent: "primary", size: "md" },
  }
);

type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & VariantProps<typeof button>;

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { intent, size, className, ...rest },
  ref
) {
  return <button ref={ref} className={cn(button({ intent, size }), className)} {...rest} />;
});
```

```tsx
// components/ui/Card.tsx
import { cn } from "@/lib/cn";

export function Card({ className, ...rest }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("rounded-lg border bg-card p-4 shadow-sm", className)} {...rest} />;
}

export function CardTitle({ className, ...rest }: React.HTMLAttributes<HTMLHeadingElement>) {
  return <h3 className={cn("text-lg font-semibold", className)} {...rest} />;
}
```

```tsx
// app/page.tsx
import { Button } from "@/components/ui/Button";
import { Card, CardTitle } from "@/components/ui/Card";

export default function Home() {
  return (
    <main className="mx-auto max-w-xl p-6 space-y-4">
      <Card>
        <CardTitle>Welcome</CardTitle>
        <p className="mt-2 text-sm text-neutral-600">A typed, themable design system.</p>
        <div className="mt-4 flex gap-2">
          <Button intent="primary">Save</Button>
          <Button intent="secondary">Cancel</Button>
          <Button intent="danger" size="sm">Delete</Button>
        </div>
      </Card>
    </main>
  );
}
```

Both `Button` and `Card` are **server components** by default. They work in any RSC tree. The only client islands you'll need are interactive ones (dropdowns, modals).

---

## 5. Common mistakes & gotchas

### Forgetting Tailwind's `content` paths

If a class doesn't show up, check that the file using it is included in `content` in `tailwind.config.ts`. Especially common when adding a new top-level directory.

### Importing global CSS outside the root layout

Global CSS must be imported in `app/layout.tsx` (root layout). Importing it from a deeper component works but loses the route-aware chunking guarantees Next gives you for top-level CSS. Stick to one global import.

### Dynamic class names with Tailwind

Tailwind's tree-shake is **string-based**. `bg-${color}-500` won't work — the compiler can't see the eventual class. Use a map:

```ts
const colorClass = { red: "bg-red-500", blue: "bg-blue-500" } as const;
<div className={colorClass[color]} />
```

Or list the dynamic classes in a comment for the safelist.

### Mixing styled-components with RSC

Won't work. The styled-components runtime expects a client environment. If you have legacy styled-components and want App Router, either migrate to vanilla-extract/Tailwind, or isolate styled components inside client component leaves.

### CSS Module conflicts in nested layouts

Each `.module.css` is scoped, but multiple modules can define a class named `.button` — *and* you might also have a Tailwind utility `.button`. Avoid: use distinct names, especially in shared libraries.

### `suppressHydrationWarning` overuse

`next-themes` needs it on `<html>`. Beyond that, sprinkling it elsewhere hides real hydration bugs. Don't use it as a generic suppressor.

### Theme flicker (FOUC)

If the theme is applied via JS after hydration, users on dark mode see a white flash. `next-themes` sets `class` before hydration to avoid this. If you roll your own, do the same: read the preference in an inline `<script>` before React loads.

---

## 🎯 Key Takeaways

- **Tailwind + CSS variables is the modern default.** It works perfectly in RSC, has zero runtime, and tree-shakes aggressively. Most production Next.js apps in 2025 use it.
- **CSS Modules remain a great option** for component-scoped CSS without a utility framework. They work in both server and client components with no fuss.
- **Classic CSS-in-JS (styled-components, Emotion classic) is constrained in RSC.** Prefer zero-runtime alternatives (vanilla-extract, Panda) or class-based systems if you need typed styles.
- **`cva` + `cn`/`tailwind-merge`** is the variant pattern that scales. Adopt it once and reuse across your design system.
- **Theme via CSS variables** + `next-themes` gives you flicker-free dark mode that survives SSR. Don't compute theme in JS at hydration time.

*←* [`07_loading_and_error_ui.md`](./07_loading_and_error_ui.md) *|* *next →* [`09_image_font_script.md`](./09_image_font_script.md)
