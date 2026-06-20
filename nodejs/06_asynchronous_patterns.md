# 06 — Asynchronous Patterns & Context

> **Goal:** Avoid EventEmitter memory leaks, understand Promise microtask scheduling, and use AsyncLocalStorage to track request context without prop drilling.

---

## 1. Concept: Managing Async Flow

Node.js is asynchronous by design. To manage flows across time, Node provides three main abstractions:
1. **Callbacks & Promises:** For one-shot asynchronous values.
2. **EventEmitters:** For pub/sub style event streams (multi-shot values).
3. **AsyncLocalStorage:** For preserving contextual storage across nested asynchronous callbacks and promises.

---

## 2. Mechanism: EventEmitters & AsyncContext

### EventEmitter Memory Leaks
An `EventEmitter` maintains an array of callback functions for each event type.
- If you dynamically subscribe to events inside a request handler (e.g. `eventEmitter.on(...)`) and fail to unsubscribe when the request ends, the event emitter retains references to those callbacks.
- Over time, the garbage collector cannot free the callback functions or their closure scopes, leading to a memory leak.

```javascript
import { EventEmitter } from 'node:events';
const emitter = new EventEmitter();

// Node alerts you if you register more than 10 listeners to avoid leaks.
emitter.setMaxListeners(20);
```

### AsyncLocalStorage (ALS)
In multi-threaded platforms, you can use Thread-Local Storage (TLS) to store session context (like user ID or transaction trace IDs). Because Node.js is single-threaded, TLS doesn't work.
Node.js provides `AsyncLocalStorage` from `node:async_hooks`. It tracks context flow across asynchronous call chains (Promises, timer phases, and I/O callbacks).

```
Incoming Request -> [ Assign Request ID ] -> Store in ALS
                               |
                               v
                       Async I/O Call
                               |
                               v
                       Database Query
                               |
                               v
                       [ Retrieve Request ID from ALS ] -> Log request ID
```

---

## 3. Variations & Depth: Promisifying Callbacks

Older Node.js APIs use error-first callbacks: `fs.readFile('path', (err, data) => {})`.
You can wrap these in Promise-based flows using `node:util`:

```javascript
import { promisify } from 'node:util';
import dns from 'node:dns';

const lookupPromise = promisify(dns.lookup);
const { address } = await lookupPromise('google.com');
```
*Note: Most core modules now ship native `/promises` subpaths, making manual promisification rarely necessary for built-in APIs.*

---

## 4. Practical Application: A Contextual Request Logger

Let's build an HTTP server that automatically injects a `requestId` into all nested log statements using `AsyncLocalStorage` without passing a context object down the call stack.

**`logger_server.js`**
```javascript
import http from 'node:http';
import { AsyncLocalStorage } from 'node:async_hooks';
import { randomUUID } from 'node:crypto';

// Initialize context store
const asyncLocalStorage = new AsyncLocalStorage();

// Centralized log wrapper
function log(message) {
  const store = asyncLocalStorage.getStore();
  const reqId = store ? store.get('requestId') : 'SYSTEM';
  console.log(`[${new Date().toISOString()}] [${reqId}] ${message}`);
}

// Simulated database service
async function getUserData(userId) {
  log(`Querying database for user: ${userId}`);
  return new Promise((resolve) => {
    setTimeout(() => {
      log('Database fetch completed.');
      resolve({ id: userId, role: 'admin' });
    }, 200); // Async boundary
  });
}

const server = http.createServer((req, res) => {
  const reqId = randomUUID();
  const contextMap = new Map();
  contextMap.set('requestId', reqId);

  // Run all downstream operations inside the async context
  asyncLocalStorage.run(contextMap, async () => {
    log(`Request received: ${req.method} ${req.url}`);
    
    try {
      const data = await getUserData('user_42');
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify(data));
      log('Response sent successfully.');
    } catch (err) {
      res.writeHead(500);
      res.end('Error');
      log(`Error processing request: ${err.message}`);
    }
  });
});

server.listen(4000, () => {
  console.log('Server running on http://localhost:4000');
});
```

---

## 5. Common Mistakes & Gotchas

- **Leaking EventEmitter subscriptions:** Forgetting to clean up listeners inside reusable modules:
  ```javascript
  function handleSocket(socket) {
    const onData = (data) => console.log(data);
    socket.on('data', onData);
    
    // BUG: If socket closes, onData remains registered in other components unless removed:
    socket.on('close', () => {
      socket.removeListener('data', onData); // Must cleanup!
    });
  }
  ```
- **Losing ALS Context:** Be careful with libraries that wrap callbacks using global queue managers or microtask pools. If the callback runs outside the async boundary execution loop, the store value will return `undefined`.
- **Unhandled Promise Rejections:** If you do not catch rejected promises, modern Node.js processes exit with a non-zero exit code by default. Always write try/catch blocks or define global error listeners:
  ```javascript
  process.on('unhandledRejection', (reason, promise) => {
    console.error('Unhandled Rejection:', reason);
    // Perform graceful shutdown if needed
  });
  ```

---

## 🎯 Key Takeaways

- **Always pair emitter subscriptions with cleanup handlers** to avoid scaling memory leaks.
- **Use `AsyncLocalStorage`** to carry diagnostics, trace IDs, and user sessions without polluting parameter footprints.
- **Uncaught promise rejections are fatal** in modern Node.js versions. Implement global hooks for crash prevention.

---

*← [networking](./05_networking_http_net.md) | [next → 07 Child Processes & Worker Threads](./07_child_processes_and_worker_threads.md)*
