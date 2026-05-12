# 12 — DOM & Browser APIs

> **Goal:** Manipulate the DOM, handle events, fetch over the network, and use storage with the modern web platform — without reaching for jQuery in 2026.

---

## 1. The DOM — Mental Model

The **DOM (Document Object Model)** is a live tree of `Node` objects representing the parsed HTML document. The browser exposes it as the global `document`. Any change you make to the DOM is reflected in what the user sees on next paint.

```html
<body>
  <h1 id="title">Hello</h1>
  <ul class="items">
    <li>One</li><li>Two</li>
  </ul>
</body>
```

```js
// Querying
document.getElementById("title");                    // single element by id
document.querySelector(".items li");                  // first match (CSS selector)
document.querySelectorAll(".items li");               // static NodeList
document.getElementsByClassName("items");             // LIVE HTMLCollection (avoid)

// Reading / writing
const h1 = document.getElementById("title");
h1.textContent = "Hi";                                // safe (no parsing)
h1.innerHTML = "<em>Hi</em>";                         // ⚠️ parses HTML — XSS risk
h1.classList.add("bold");
h1.classList.toggle("dark");
h1.dataset.userId = "42";                             // <h1 data-user-id="42">
h1.style.color = "red";                               // inline style
h1.setAttribute("aria-label", "title");

// Creating & inserting
const li = document.createElement("li");
li.textContent = "Three";
document.querySelector(".items").append(li);
// Modern alternatives:
ul.prepend(node); ul.before(node); ul.after(node); ul.replaceWith(node);
node.remove();
```

`textContent` is safe because it never parses HTML. `innerHTML` parses, which means user input concatenated into it can execute scripts (cross-site scripting — covered in module 17).

---

## 2. Events — Under the Hood

Events propagate in three phases: **capture** → **target** → **bubble**.

```
window → document → html → body → div → button   (capture going down)
                                       ▼
                                   target
                                       ▲
   window ← document ← html ← body ← div ← button (bubble going up)
```

```js
button.addEventListener("click", (e) => {
  console.log("clicked", e.target);
});
// Third arg: useCapture (true = capture phase)
parent.addEventListener("click", handler, true);
```

### Common event object props
```js
e.target         // element that triggered the event
e.currentTarget  // element with the listener attached (different during bubbling!)
e.preventDefault();
e.stopPropagation();
e.stopImmediatePropagation(); // also prevents other listeners on same element
```

### Event delegation — the killer pattern
Instead of attaching N listeners, attach one to the parent:
```js
document.querySelector(".items").addEventListener("click", (e) => {
  const li = e.target.closest("li");
  if (!li) return;
  console.log("clicked item:", li.textContent);
});
```
Works for items added later. Scales to 10,000 items with one listener.

### `addEventListener` options
```js
el.addEventListener("scroll", handler, { passive: true });   // promise not to preventDefault — perf
el.addEventListener("click", handler, { once: true });       // auto-remove after first call
el.addEventListener("click", handler, { signal: ctrl.signal }); // remove via AbortController
```

`AbortController` for cleanup is gorgeous:
```js
const ctrl = new AbortController();
window.addEventListener("resize", onResize, { signal: ctrl.signal });
window.addEventListener("scroll", onScroll, { signal: ctrl.signal });
// Later, cleanup all at once:
ctrl.abort();
```

### Custom events
```js
const event = new CustomEvent("user:login", { detail: { id: 42 } });
window.dispatchEvent(event);

window.addEventListener("user:login", (e) => console.log(e.detail));
```

---

## 3. Fetch, Forms, Storage, and Other Browser APIs

### `fetch` — modern HTTP
```js
const res = await fetch("/api/users", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ name: "Ada" }),
  signal: AbortSignal.timeout(5000),
  credentials: "include",      // send cookies cross-origin
});

if (!res.ok) {
  // fetch only rejects on network failure — HTTP errors do NOT throw
  throw new Error(`HTTP ${res.status}`);
}

const data = await res.json();
// or .text(), .arrayBuffer(), .blob(), .formData()
```

**Important:** `fetch` does **not** reject on 4xx/5xx. You must check `res.ok` yourself.

### `FormData`
```js
const form = document.querySelector("form");
form.addEventListener("submit", async (e) => {
  e.preventDefault();
  const data = new FormData(form);
  await fetch("/submit", { method: "POST", body: data });
});

// Read individual fields:
data.get("email");
Object.fromEntries(data); // → plain object (single-value fields only)
```

### Storage APIs

```js
// localStorage — persists, ~5–10MB per origin, sync, strings only
localStorage.setItem("token", "abc");
localStorage.getItem("token");        // "abc"
localStorage.removeItem("token");
localStorage.clear();

// sessionStorage — same API, cleared when tab closes

// Cookies — for things the server needs (auth)
document.cookie = "k=v; Max-Age=3600; Path=/; Secure; SameSite=Lax";

// IndexedDB — async, structured, big (hundreds of MB+). Use a wrapper:
//   "idb" or "dexie" libraries make it pleasant.
```

Don't put secrets (JWTs) in `localStorage` if your app might have XSS — module 17 covers this.

### URL & URLSearchParams
```js
const u = new URL("https://example.com/path?a=1&b=2");
u.searchParams.get("a");           // "1"
u.searchParams.set("b", "9");
u.searchParams.append("tag", "js");
u.toString();                      // "https://example.com/path?a=1&b=9&tag=js"

// Build query strings cleanly
const qs = new URLSearchParams({ q: "ada lovelace", page: 2 }).toString();
fetch(`/search?${qs}`);
```

### `History` — SPA navigation
```js
history.pushState({ page: 2 }, "", "/page/2");
window.addEventListener("popstate", (e) => render(e.state));
```

### `IntersectionObserver` — efficient "is it visible?"
```js
const obs = new IntersectionObserver((entries) => {
  for (const e of entries) {
    if (e.isIntersecting) lazyLoad(e.target);
  }
});
document.querySelectorAll("img[data-src]").forEach((img) => obs.observe(img));
```
Use this for lazy-loading, infinite scroll, animation triggers — it's vastly cheaper than scroll listeners.

### Web Components (briefly)
```js
class GreetButton extends HTMLElement {
  connectedCallback() {
    this.innerHTML = `<button>Hi ${this.getAttribute("name")}</button>`;
  }
}
customElements.define("greet-button", GreetButton);
```
```html
<greet-button name="Ada"></greet-button>
```
Native, framework-free reusable elements.

---

## 4. Practical Application — A Tiny SPA Snippet

```html
<!doctype html>
<body>
  <input id="q" placeholder="search" />
  <ul id="results"></ul>

  <script type="module">
    const q = document.getElementById("q");
    const results = document.getElementById("results");

    const debounce = (fn, ms) => {
      let t;
      return (...args) => { clearTimeout(t); t = setTimeout(() => fn(...args), ms); };
    };

    let currentCtrl;
    async function search(term) {
      currentCtrl?.abort();           // cancel previous in-flight request
      currentCtrl = new AbortController();
      try {
        const res = await fetch(
          `/api/search?${new URLSearchParams({ q: term })}`,
          { signal: currentCtrl.signal }
        );
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        render(data.items);
      } catch (err) {
        if (err.name !== "AbortError") console.error(err);
      }
    }

    function render(items) {
      results.replaceChildren(
        ...items.map((it) => {
          const li = document.createElement("li");
          li.textContent = it.title;          // SAFE — no innerHTML
          li.dataset.id = it.id;
          return li;
        })
      );
    }

    q.addEventListener("input", debounce((e) => search(e.target.value), 200));

    // Event delegation on results
    results.addEventListener("click", (e) => {
      const li = e.target.closest("li");
      if (!li) return;
      history.pushState({ id: li.dataset.id }, "", `/item/${li.dataset.id}`);
    });
  </script>
</body>
```

This shows: debouncing, abort-on-new-request, safe rendering with `textContent`, event delegation, History API. That's a solid third of "real frontend" right there.

---

## 5. Common Mistakes & Gotchas

- **`fetch` doesn't throw on 4xx/5xx.** Always check `res.ok`.
- **`innerHTML` with user input = XSS.** Use `textContent`, or sanitize with DOMPurify, or use a framework that escapes by default.
- **`getElementsByClassName` returns a live `HTMLCollection`** that updates as the DOM changes — confusing in loops. Use `querySelectorAll` (returns a static `NodeList`).
- **Forgetting `e.preventDefault()` on form submit** → page navigates and reloads.
- **Memory leaks via listeners on detached elements.** Use `AbortController` or remove listeners explicitly.
- **`scroll`/`mousemove` listeners without `{ passive: true }`** can block scrolling. Always passive unless you need `preventDefault`.
- **Synchronous layout thrashing:**
  ```js
  for (const el of items) {
    el.style.left = el.offsetWidth + "px"; // read forces layout, write invalidates — repeat
  }
  ```
  Read all measurements first, then write all changes (or use `getBoundingClientRect` once + `requestAnimationFrame`).
- **`localStorage` is synchronous** and blocks the main thread. Don't store huge things there.
- **Cookies with `SameSite=None`** require `Secure`. Browsers reject otherwise.
- **CORS:** `fetch` from one origin to another is restricted unless the server sends the right `Access-Control-Allow-*` headers. The browser silently strips response data otherwise.
- **DOM operations are slow at scale.** Build a `DocumentFragment` and append once instead of N appends:
  ```js
  const frag = document.createDocumentFragment();
  for (const item of items) frag.append(makeNode(item));
  list.append(frag);
  ```
- **`setTimeout`/`setInterval` IDs leak in long-running pages.** Always pair `setTimeout` → `clearTimeout`, `setInterval` → `clearInterval`. Or use `AbortSignal.timeout`.

```js
// "Wat"
typeof null === "object";
NodeList.prototype.forEach;             // exists; HTMLCollection's doesn't
const a = document.querySelectorAll("p"); // static
const b = document.getElementsByTagName("p"); // live — adding a <p> mutates b
```

### Browser vs Node
| | Browser | Node |
|---|---------|------|
| `document`, `window` | yes | no |
| `fetch` | yes | yes (Node 18+) |
| `localStorage` | yes | no (use a file) |
| `setTimeout`/`setInterval` | yes | yes |
| `URL`, `URLSearchParams` | yes | yes |
| `crypto.subtle` | yes | yes (Node 16+) |

---

## 🎯 Key Takeaways

- **`textContent` is safe; `innerHTML` is not.** Default to safe; reach for innerHTML only with sanitized input.
- **Event delegation** scales to thousands of elements with one listener and "just works" with dynamic content.
- **`AbortController` is the universal cancellation primitive** — events, fetch, timers (ES2024). Use it for cleanup.
- **`fetch` returns on any HTTP response;** check `res.ok` and throw your own errors. Never assume success.
- **Modern observers (`IntersectionObserver`, `ResizeObserver`, `MutationObserver`)** are dramatically cheaper than scroll/resize listeners. Reach for them by default.

---

*← [11 The Event Loop](./11_event_loop.md) | [next → 13 Node.js Essentials](./13_node_essentials.md)*
