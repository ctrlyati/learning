# 00 — Next.js 16 Deep-Dive Roadmap

> **Goal:** Take a working React developer from zero Next.js to production-grade App Router fluency in ~2.5 weeks of focused study.

This course is **App Router first** (Next.js 16). The Pages Router is mentioned only where the contrast helps you understand *why* App Router exists. By the end, you should be able to architect, build, deploy, and debug a production Next.js app — and reason about the React Server Components (RSC) boundary as naturally as you reason about `useState`.

---

## Prerequisites

You should be comfortable with the following before starting:

- **JavaScript fundamentals** — closures, promises, async/await, modules. See [`../javascript/00_roadmap.md`](../javascript/00_roadmap.md).
- **TypeScript basics** — interfaces, generics, utility types. See [`../typescript/00_roadmap.md`](../typescript/00_roadmap.md).
- **React** — components, props, state, hooks (`useState`, `useEffect`, `useContext`), lifting state, controlled forms.
- **Node.js & npm/pnpm** — running scripts, installing packages.
- **HTTP basics** — methods, status codes, headers, cookies.
- A text editor with TypeScript support (VS Code recommended) and a recent **Node.js ≥ 18.17** install.

If any of those feels shaky, fix it first. App Router compounds confusion fast if your React mental model is loose.

---

## Module table

| #  | File                                                  | Topic                                              | Est. time |
|----|-------------------------------------------------------|----------------------------------------------------|-----------|
| 00 | `00_roadmap.md`                                       | This file                                          | 30 min    |
| 01 | `01_setup_and_app_router.md`                          | Setup, `create-next-app`, Turbopack, App vs Pages  | 2 h       |
| 02 | `02_routing_fundamentals.md`                          | File-system routes, layouts, groups, dynamic, parallel/intercepting | 3 h |
| 03 | `03_server_and_client_components.md`                  | RSC vs Client, the boundary, `"use client"`, React Compiler | 3 h |
| 04 | `04_data_fetching.md`                                 | `"use cache"`, cacheLife profiles, Cache Components, revalidation | 3 h |
| 05 | `05_server_actions_and_mutations.md`                  | Forms, progressive enhancement, useActionState, optimistic UI | 3 h |
| 06 | `06_rendering_strategies.md`                          | Static, dynamic, streaming, PPR, per-route choice  | 2.5 h     |
| 07 | `07_loading_and_error_ui.md`                          | `loading.tsx`, `error.tsx`, Suspense, not-found, 16.2 diff overlay | 2 h |
| 08 | `08_styling.md`                                       | CSS Modules, Tailwind CSS v4, theming              | 2 h       |
| 09 | `09_image_font_script.md`                             | `next/image` optimization, `next/font`, `next/script` | 2 h       |
| 10 | `10_metadata_and_seo.md`                              | Metadata API, sitemap, robots, OG images           | 2 h       |
| 11 | `11_proxy_and_edge.md`                                | proxy.ts, routing, headers, cookies, edge proxy    | 2.5 h     |
| 12 | `12_authentication.md`                                | Auth.js / NextAuth, sessions, React 19 auth patterns | 3 h       |
| 13 | `13_database_and_orm.md`                              | Prisma/Drizzle, Postgres, cached query patterns    | 3 h       |
| 14 | `14_route_handlers_and_api.md`                        | Route handlers, streaming, vs Actions, use cache   | 2.5 h     |
| 15 | `15_testing.md`                                       | Vitest, RTL with RSC / React 19, Playwright        | 3 h       |
| 16 | `16_performance_and_observability.md`                 | Turbopack Analyzer, RSC payload, instrumentation   | 2.5 h     |
| 17 | `17_deployment_and_production.md`                     | Vercel, self-host, Docker, ISR with updateTag()    | 3 h       |

**Total**: ~45 hours of focused work. At ~1 module/day, that's about **2.5 weeks**.

---

## Suggested timeline (~2.5 weeks, 1 module/day)

| Week | Days | Modules | Theme                              |
|------|------|---------|------------------------------------|
| 1    | 1–5  | 01–05   | Foundations & data flow            |
| 2    | 6–10 | 06–10   | UI, rendering, polish              |
| 2/3  | 11–14| 11–14   | Auth, DB, APIs                     |
| 3    | 15–17| 15–17   | Test, perf, ship                   |

If you have less time, prioritize **01–06 and 17**. Those are the load-bearing modules; the rest you can pick up on demand.

---

## Core mental models

These six ideas, once internalized, make every Next.js doc page click. Re-read them after Module 06 — they'll mean more.

### 1. The server/client component boundary is the most important line in your code

By default everything in `app/` is a **Server Component**. It runs on the server, never ships JS to the browser, and can `await` directly. The moment you add `"use client"` at the top of a file, that file *and everything it imports* becomes part of the client bundle. The boundary is one-way: server can render client, but client can only *use* server components by passing them as `children` or props.

### 2. Modern caching relies on the `"use cache"` directive

In Next.js 16, default network fetching does not cache by default. Instead, caching is component- and function-level. Use the `"use cache"` directive inside Server Components, components, or functions to granularly cache their output. You configure caching times dynamically with profiles (e.g. `unstable_cacheLife`), and update caches using `updateTag()` or `revalidateTag()`.

### 3. The RSC payload is HTML + serialized component tree, not just markup

When a server component renders, the server streams two things: HTML for first paint, and a serialized description of the component tree (props included) so React can hydrate and later patch in client components. This is why props passed from a server component to a client component must be JSON-serializable.

### 4. Server Actions are POST endpoints in disguise

`"use server"` functions look like local calls, but at runtime they compile to a POST request to your own server with a stable ID. That means: they enforce CSRF, they run in Node (not in the user's browser), and you should treat them with the same trust-no-input discipline as any API endpoint.

### 5. The file system *is* the router

Routes are not configured — they are discovered. `app/blog/[slug]/page.tsx` *is* the route `/blog/:slug`. Special filenames (`layout`, `loading`, `error`, `not-found`, `route`, `page`, `template`, `default`) have specific reserved meanings. Learn them once, use them forever. The root-level `proxy.ts` replaces the legacy `middleware.ts` for route interception.

### 6. Default to server, opt into client

The cheapest, fastest, most secure component is a Server Component. Reach for `"use client"` only when you need: `useState`, `useEffect`, browser APIs, event handlers, or class components / context that requires them. Furthermore, with the React Compiler enabled by default, client components are automatically memoized, so you rarely need manual `useMemo` or `useCallback` hooks.

---

## External links worth bookmarking

- **[nextjs.org/docs](https://nextjs.org/docs)** — the official docs are unusually good. Read the "App Router" section end-to-end at least once.
- **[Lee Robinson's blog & YouTube](https://leerob.io/)** — VP of Product at Vercel; clearest explanations of caching, streaming, and PPR.
- **[Vercel Templates](https://vercel.com/templates/next.js)** — production-quality starters. Read `next.js-commerce` and the official "Platforms Starter Kit" for real architecture.
- **[React docs — Server Components](https://react.dev/reference/rsc/server-components)** — RSC is a React feature, not a Next.js one. Read the React docs to separate the two.
- **[Next.js Conf talks (YouTube)](https://www.youtube.com/@VercelHQ)** — follow the keynotes for releases up to Next.js 16 to understand the model shifts.
- **[Twitter / X: @leeerob, @delba_oliveira, @feedthejim](https://twitter.com)** — Vercel devrel folks who post genuinely useful tips.

---

## How to use this course

Each module has the same shape:

```
1. Concept — mental model + minimal working code
2. Mechanism — what Next.js does under the hood
3. Variations / depth
4. Practical application — a realistic feature slice
5. Common mistakes & gotchas
🎯 Key Takeaways
```

**Type the code, don't paste.** Then break it. Then fix it. The footguns section of every module is where the real learning compounds.

---

## Closing note

Next.js is, more than any other framework today, a *senior-developer multiplier*. The framework hides an enormous amount of complexity (bundling, routing, edge runtime, caching, streaming, prerendering, ISR) behind conventions that are easy to misuse if you don't understand them — and breathtakingly productive once you do. The goal of this course isn't to memorize APIs; it's to build the mental models that let you read the Next.js source, debug a production cache miss, and justify architecture decisions to a skeptical staff engineer. Treat it as professional upskilling, not tutorial completion.

*next →* [`01_setup_and_app_router.md`](./01_setup_and_app_router.md)
