# 17 — Modern Patterns & Production: Error Tracking, Performance, Security, TypeScript Migration Prep

> **Goal:** Take everything from modules 1–16 and apply it the way real teams ship JavaScript: observability, performance budgets, security hygiene, and a clean glide path to TypeScript.

---

## 1. Production Mindset — Mental Model

Code in development optimizes for *iteration speed*. Code in production optimizes for *reliability, observability, and security*. The mental shift:

- Every error must be **surfaced** somewhere a human will see.
- Every slow path must be **measured** before being optimized.
- Every input must be **validated**; every output must be **escaped**.
- Every credential must be **rotated** and never in source.
- Every behavior change is a **deployment risk** until proven otherwise (feature flags, canaries, rollbacks).

```js
// Dev mindset
console.log(user); // good enough

// Prod mindset
logger.info({ userId: user.id, requestId, route }, "user fetched");
metrics.histogram("user_fetch_ms", duration);
if (duration > SLOW_THRESHOLD) Sentry.captureMessage("slow user fetch", { level: "warning", extra: { duration } });
```

The same line of code, three different concerns: structured log, metric, alert.

---

## 2. Error Tracking & Observability — Under the Hood

### Structured logging
Use a real logger (`pino`, `winston`); don't ship `console.log`. Logs should be machine-parsable JSON in prod.

```js
import pino from "pino";

export const logger = pino({
  level: process.env.LOG_LEVEL ?? "info",
  redact: ["req.headers.authorization", "*.password", "*.token"],
});

logger.info({ userId, route: req.url }, "request received");
```

Structured fields → searchable in Datadog/Loki/Cloudwatch. Always include a `requestId` propagated through the call chain.

### Error tracking — Sentry, Rollbar, Honeybadger
```js
import * as Sentry from "@sentry/node";
Sentry.init({
  dsn: process.env.SENTRY_DSN,
  environment: process.env.NODE_ENV,
  release: process.env.APP_VERSION,
  tracesSampleRate: 0.1,    // 10% performance sampling
});

// Breadcrumbs (auto + manual)
Sentry.addBreadcrumb({ category: "auth", message: "user logged in", level: "info" });

// Capturing
try { await dangerous(); }
catch (err) {
  Sentry.captureException(err, { tags: { module: "billing" }, user: { id: userId } });
  throw err;
}
```

**Rules:**
- Upload source maps to Sentry on every release (don't host them publicly).
- Use `release` tag = git SHA so you can correlate errors with deploys.
- Filter PII before sending: emails, IPs, request bodies.
- Sample heavy-volume errors; don't pay for noise.

### Metrics — RED method
For each request: **R**ate (req/sec), **E**rrors (count), **D**uration (latency). Three numbers tell you whether things are healthy.

For background jobs/queues: **USE** method (Utilization, Saturation, Errors).

### Tracing
OpenTelemetry is the standard. One config; traces flow to Datadog, Honeycomb, Tempo, etc.

```js
// Pseudo: tracing wrapper
import { trace } from "@opentelemetry/api";
const tracer = trace.getTracer("my-service");

async function getUser(id) {
  return tracer.startActiveSpan("getUser", async (span) => {
    try {
      span.setAttribute("user.id", id);
      const user = await db.users.findById(id);
      return user;
    } catch (err) {
      span.recordException(err);
      throw err;
    } finally {
      span.end();
    }
  });
}
```

### Browser observability
- **Performance API:** `performance.mark`, `measure`, `getEntriesByType("navigation")`.
- **Web Vitals:** LCP, FID/INP, CLS — measure with `web-vitals` package; ship to analytics.
- **Long Tasks API:** `PerformanceObserver` for tasks >50ms blocking the main thread.

```js
import { onLCP, onINP, onCLS } from "web-vitals";
onLCP((m) => report("LCP", m.value));
onINP((m) => report("INP", m.value));
onCLS((m) => report("CLS", m.value));
```

---

## 3. Performance & Security

### Performance — measure, then optimize

#### Browser
- **Network:** ship less. Code-split. Use HTTP/2 + Brotli. Set `Cache-Control` aggressively for hashed assets.
- **JS:** lazy-load below-the-fold features. Defer or `async` non-critical scripts. Avoid blocking the main thread (>50ms tasks).
- **Rendering:** read DOM dimensions in batches; mutate in batches. Use `transform`/`opacity` for animations (GPU-accelerated).
- **Use `Performance` panel in devtools** rather than guessing.

```js
// Quick perf measurement
performance.mark("op:start");
doExpensive();
performance.mark("op:end");
performance.measure("op", "op:start", "op:end");
console.log(performance.getEntriesByName("op")[0].duration);
```

#### Node
- Profile with `node --inspect` + Chrome DevTools, or `clinic.js` (`clinic doctor`, `clinic flame`).
- Watch event-loop lag: `monitorEventLoopDelay`, `perf_hooks`.
- Move CPU-bound work to `worker_threads`.
- Stream large payloads; never `JSON.parse(buffer.toString())` on multi-MB bodies.
- Connection pool DBs (`pg`, `mysql2`) — don't create one per request.

```js
import { monitorEventLoopDelay } from "node:perf_hooks";
const h = monitorEventLoopDelay({ resolution: 20 });
h.enable();
setInterval(() => {
  if (h.mean / 1e6 > 50) logger.warn({ meanMs: h.mean / 1e6 }, "event loop lag");
}, 5000);
```

### Security pitfalls

#### XSS (Cross-Site Scripting)
Untrusted input rendered as HTML can execute. The fix is escape-by-default.
```js
// VULNERABLE
el.innerHTML = "<p>Hello " + userName + "</p>";

// SAFE — textContent
el.textContent = `Hello ${userName}`;

// SAFE — tagged template / sanitizer
import DOMPurify from "dompurify";
el.innerHTML = DOMPurify.sanitize(userMarkup);
```

Frameworks (React, Vue, Svelte) escape by default in `{ }`. The danger is escape hatches: `dangerouslySetInnerHTML`, `v-html`, etc. Treat each one as a security review point.

Set a **Content Security Policy** header:
```
Content-Security-Policy: default-src 'self'; script-src 'self' 'sha256-...'; object-src 'none'
```

#### Prototype pollution
Merging untrusted JSON into objects can poison `Object.prototype`:
```js
// Naive merge — vulnerable
function merge(target, source) {
  for (const k in source) {
    if (typeof source[k] === "object") merge(target[k] ??= {}, source[k]);
    else target[k] = source[k];
  }
  return target;
}
merge({}, JSON.parse('{"__proto__": {"isAdmin": true}}'));
({}).isAdmin; // true — every object now has it
```

Defenses:
- Use `Object.create(null)` for maps you don't want to inherit.
- Validate input with a schema (`zod`, `valibot`, `ajv`).
- Reject `__proto__`, `constructor`, `prototype` keys explicitly.
- Use battle-tested libs: `lodash.merge` had this bug — `Object.assign` doesn't recurse and is safe.

#### Other top issues
- **CSRF** — server-side tokens for state-changing requests; SameSite cookies.
- **SSRF** — never `fetch(userProvidedUrl)` server-side without strict allowlisting.
- **Secrets in client bundles** — anything in `VITE_*` env vars is shipped to the client. Audit.
- **`eval`, `new Function`, `setTimeout(string)`** — never with untrusted input.
- **Open redirects** — validate redirect targets server-side.
- **Dependency CVEs** — `npm audit`, Dependabot, Snyk. Update regularly.
- **`postinstall` scripts** in dependencies can run arbitrary code. Use `--ignore-scripts` in CI for added safety; vet new deps.

#### Cryptography
Use the platform: `crypto.subtle` (browser, Node). Never roll your own. Argon2/bcrypt for passwords. UUIDv4 for IDs (`crypto.randomUUID()`).

```js
// Hash a password (Node)
import { scrypt, randomBytes } from "node:crypto";
import { promisify } from "node:util";
const scryptAsync = promisify(scrypt);

async function hash(password) {
  const salt = randomBytes(16);
  const key = await scryptAsync(password, salt, 64);
  return `${salt.toString("hex")}:${key.toString("hex")}`;
}
```

---

## 4. Practical Application — A Production-Ready Service Skeleton + TS Migration Plan

Putting much of the course together — a Node service with logging, error handling, validation, graceful shutdown.

```js
// server.js
import http from "node:http";
import pino from "pino";
import { z } from "zod";
import * as Sentry from "@sentry/node";
import { AppError, ValidationError } from "./errors.js";

Sentry.init({ dsn: process.env.SENTRY_DSN, release: process.env.APP_VERSION });
const logger = pino({ redact: ["*.password", "*.token"] });

const CreateUser = z.object({
  email: z.string().email(),
  name: z.string().min(1).max(100),
});

const handlers = {
  "POST /users": async (req, body, ctx) => {
    const parsed = CreateUser.safeParse(body);
    if (!parsed.success) throw new ValidationError("Bad input", parsed.error.flatten());
    const user = await ctx.db.users.create({ data: parsed.data });
    return { status: 201, body: user };
  },
};

const server = http.createServer(async (req, res) => {
  const requestId = req.headers["x-request-id"] ?? crypto.randomUUID();
  const log = logger.child({ requestId, method: req.method, path: req.url });
  const t0 = performance.now();

  try {
    const route = `${req.method} ${new URL(req.url, "http://x").pathname}`;
    const handler = handlers[route];
    if (!handler) { res.writeHead(404).end(); return; }

    let body = "";
    for await (const chunk of req) body += chunk;
    body = body ? JSON.parse(body) : {};

    const { status = 200, body: out = {} } = await handler(req, body, { db, log });
    res.writeHead(status, { "Content-Type": "application/json", "X-Request-Id": requestId });
    res.end(JSON.stringify(out));
  } catch (err) {
    const status = err instanceof AppError ? err.statusCode : 500;
    const expose = err instanceof AppError && err.expose;
    log.error({ err }, "request failed");
    Sentry.captureException(err, { tags: { requestId } });
    res.writeHead(status, { "Content-Type": "application/json" });
    res.end(JSON.stringify({
      error: { message: expose ? err.message : "Internal Server Error", requestId },
    }));
  } finally {
    log.info({ ms: (performance.now() - t0).toFixed(1) }, "request done");
  }
});

const port = Number(process.env.PORT) || 3000;
server.listen(port, () => logger.info({ port }, "listening"));

// Graceful shutdown
const shutdown = async (sig) => {
  logger.info({ sig }, "shutting down");
  server.close();
  await Sentry.close(2000);
  process.exit(0);
};
["SIGINT", "SIGTERM"].forEach((s) => process.on(s, () => shutdown(s)));

process.on("unhandledRejection", (err) => {
  logger.fatal({ err }, "unhandledRejection");
  Sentry.captureException(err);
  process.exit(1);
});
```

This single file demonstrates: structured logging, schema validation (`zod`), typed errors, error exposure control, tracing IDs, graceful shutdown, fatal-on-unhandled-rejection, observability integration.

### TypeScript migration prep
TypeScript is a layer of types over JavaScript. Migrating a JS codebase usually goes:

1. **Add `tsconfig.json`** with `allowJs: true`, `checkJs: false`. Keep all `.js` files.
2. **Add JSDoc types** to high-leverage modules first. TS understands JSDoc almost as well as `.ts`:
   ```js
   /** @typedef {{ id: number, name: string }} User */
   /** @param {string} id @returns {Promise<User|null>} */
   export async function getUser(id) { /* ... */ }
   ```
3. **Enable `checkJs: true`** to start type-checking JSDoc.
4. **Rename file by file to `.ts`** as you have time. Tooling stays unchanged (Vite/Vitest support TS natively).
5. **Tighten `tsconfig`** progressively: `strict: true`, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`.
6. **Adopt schema-first validation** (`zod`) so runtime validators *are* your types: `type User = z.infer<typeof UserSchema>`.

Your knowledge from this course transfers 1-to-1: closures, prototypes, `this`, async semantics — TS doesn't change any of it. TS only adds compile-time guarantees on top.

---

## 5. Common Mistakes & Gotchas (Production)

- **`console.log` in production** — no levels, no structure, hard to filter. Use a real logger.
- **Catching errors and swallowing** — silent failures are the worst kind. Log + rethrow OR log + escalate to error tracking.
- **Crashing on every error** — distinguish operational (recoverable) from programmer (crash) errors.
- **No graceful shutdown** — in-flight requests dropped, transactions half-committed. Handle SIGTERM.
- **Premature optimization** — profile before refactoring. Memoize the bottleneck, not "everything that might be slow."
- **Unbounded concurrency** — `Promise.all(items.map(call))` over 10k items DDoSes the upstream and runs you out of file descriptors. Limit it.
- **Logging secrets** — passwords, tokens, JWTs in plain logs. Redact.
- **Source maps published to public CDNs** — hands the world your source. Upload to error tracker, don't expose.
- **`eval`, `Function`, dynamic `setTimeout(string)`** — CSP violations and code injection vectors. Don't.
- **Trusting client-side validation** — always re-validate server-side.
- **Stale dependencies** — yesterday's safe library is today's CVE. Set up automated PRs.
- **Skipping postmortems** — every prod incident teaches something. Write it down, change the system, not the human.

```js
// "Wat" — production edition
JSON.parse(undefined);    // throws SyntaxError, not "undefined is not valid"
process.exit(0);          // skips pending I/O — your last log may not flush
"".repeat(2 ** 30);       // crashes Node with OOM (RangeError, then heap)
```

---

## 🎯 Key Takeaways

- **Observability is a feature.** Structured logs + error tracking + metrics + traces. If you can't see it in prod, it didn't happen.
- **Validate at every boundary** with a real schema lib (`zod`/`valibot`). Coerce explicitly; never trust input.
- **Security is layered:** escape output (XSS), validate input (proto pollution), CSP headers, no secrets in clients, audit deps, rotate keys. No single fix.
- **Measure before optimizing.** Devtools, `clinic.js`, `performance.mark` — find the actual bottleneck, then act.
- **You are TypeScript-ready.** Every module of this course (closures, `this`, prototypes, async, modules, tooling) is the foundation TS sits on. The TS course is the natural next step — see [`00_roadmap.md`](./00_roadmap.md).

---

*← [16 Tooling](./16_tooling.md) | [back to roadmap](./00_roadmap.md)*

**Course complete. Next stop: TypeScript.**
