# 05 — Networking: HTTP & Net Modules

> **Goal:** Build TCP socket servers, create custom HTTP agents to optimize connection pooling, and implement a streaming reverse proxy.

---

## 1. Concept: TCP vs HTTP Runtimes

Node.js provides low-level networking primitives using the **`node:net`** module (for TCP socket streaming) and high-level client/server adapters using the **`node:http`** and **`node:https`** modules.

```
Incoming Request (HTTP / TCP)
       |
       v
+-------------+
| node:net    |  <-- Manages raw TCP sockets & data frames
+-------------+
       |
       v
+-------------+
| node:http   |  <-- Parses HTTP headers, streams, cookies
+-------------+
```

---

## 2. Mechanism: Connections & HTTP Agents

### TCP Connections (`node:net`)
TCP is a connection-oriented protocol. A server listens on a port, and clients establish a persistent connection. Data is transferred as a continuous stream of bytes.

### HTTP Agents (`node:http`)
When making outgoing HTTP requests, Node.js uses an `Agent` to manage socket reuse (connection pooling).
- By default, outgoing requests create a new socket unless `keepAlive` is enabled.
- Reusing connections avoids the 3-way handshake overhead for consecutive requests, greatly increasing throughput.

```javascript
import http from 'node:http';

// Create a custom agent configured for reuse
const keepAliveAgent = new http.Agent({
  keepAlive: true,
  maxSockets: 100,       // Max sockets per host
  maxFreeSockets: 10,    // Keep up to 10 idle sockets open
  timeout: 60000         // Socket idle timeout (ms)
});

// Use it in requests
http.get('http://example.com', { agent: keepAliveAgent }, (res) => {
  // Read response
});
```

---

## 3. Variations & Depth: Streaming Payloads

In standard web frameworks, request bodies are buffered entirely in memory before route handlers run:
```javascript
// WARNING: High memory usage pattern
let body = '';
req.on('data', chunk => body += chunk);
req.on('end', () => {
  const json = JSON.parse(body); // entire body in memory
});
```
For large uploads, this invites memory starvation. In pure Node.js, you can parse input streams on-the-fly or pipe request streams directly to storage systems or upstream servers.

---

## 4. Practical Application: A Streaming Reverse Proxy

Let's write a streaming reverse proxy that intercepts client requests on port `8080` and proxies them to an upstream mock server on port `3000` without buffering anything.

**`proxy.js`**
```javascript
import http from 'node:http';

// 1. Create a dummy upstream server on port 3000
const upstreamServer = http.createServer((req, res) => {
  console.log(`[Upstream] Received: ${req.method} ${req.url}`);
  res.writeHead(200, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({ status: 'ok', proxyWorked: true }));
});
upstreamServer.listen(3000, () => {
  console.log('Upstream server running on port 3000');
});

// 2. Create the Proxy Server on port 8080
const proxyServer = http.createServer((req, res) => {
  console.log(`[Proxy] Proxying request: ${req.url}`);

  const options = {
    hostname: 'localhost',
    port: 3000,
    path: req.url,
    method: req.method,
    headers: req.headers
  };

  // Create client request to upstream
  const proxyReq = http.request(options, (proxyRes) => {
    // Write upstream response headers back to client
    res.writeHead(proxyRes.statusCode, proxyRes.headers);
    // Pipe the upstream response stream directly back to client
    proxyRes.pipe(res);
  });

  proxyReq.on('error', (err) => {
    console.error('Proxy request failed:', err);
    res.writeHead(502);
    res.end('Bad Gateway');
  });

  // Pipe the incoming client request payload (if any) directly to upstream
  req.pipe(proxyReq);
});

proxyServer.listen(8080, () => {
  console.log('Proxy Gateway running on http://localhost:8080');
});
```

---

## 5. Common Mistakes & Gotchas

- **Socket Exhaustion:** Outgoing requests without `keepAlive: true` create and destroy sockets rapidly. Under high load, the OS will run out of available ephemeral ports (port exhaustion), throwing `connect EADDRINUSE` errors.
- **Leaking Sockets on Timeout:** If an outgoing request times out, you must explicitly destroy or abort the request (`req.destroy()`), otherwise the connection remains open, blocking resources.
- **Forgetting to resume paused streams:** If you attach a `'data'` handler to an incoming request, the stream starts flowing. If you detach it without consuming all remaining bytes, the stream remains paused and the socket will never close.

---

## 🎯 Key Takeaways

- **Always enable `keepAlive`** in your HTTP Agent when making high-volume outgoing requests to databases or external API services.
- **Use streams for proxies and file uploads** to avoid buffer overflows.
- **Handle request errors and timeouts robustly** to prevent leaking file descriptors or sockets.

---

*← [file system](./04_file_system_and_path.md) | [next → 06 Asynchronous Patterns](./06_asynchronous_patterns.md)*
