# 09 — Production & Security operations

> **Goal:** Leverage the cluster module for multi-core scaling, configure max V8 heap size limits inside containers, and implement essential security guards.

---

## 1. Concept: Hardening and Scaling

Shipping Node.js to production requires shifting from a sandbox mindset to an enterprise operations focus.
- **Scaling:** Scaling across CPU cores (since Node is single-threaded, a single instance can only utilize one CPU core).
- **Security:** Preventing SQL injection, cross-site scripting (XSS), Command Injection, and memory-exhaustion DoS attacks.
- **Resource Management:** Preventing the container runtime from killing your process due to memory limits.

---

## 2. Mechanism: Clustering & Memory Limits

### Node.js Clustering
The **`node:cluster`** module enables you to spawn worker processes that share the same underlying TCP port.
- The master process coordinates and distributes incoming connections to child processes (workers) using a round-robin load-balancing algorithm.
- If one worker crashes, the master process is notified and can immediately boot a replacement worker, achieving zero-downtime crash resilience.

```
                  Client Requests
                         |
                         v
                +-----------------+
                | Master Process  | (Load balancer)
                +-----------------+
                  /      |      \
                 v       v       v
            Worker 1  Worker 2  Worker 3  (Active CPU cores)
```

### V8 Heap limits inside Docker
By default, V8 sets its max heap limit to roughly 1.4GB on 64-bit machines.
- **The Trap:** If you run Node.js in a Docker container constrained to 512MB RAM, V8 remains unaware of the container constraint and will keep allocating heap memory past 512MB. The OS kernel will instantly terminate the container using `SIGKILL` (Out Of Memory / OOM), leaving no log trace.
- **The Fix:** Tune V8's memory limits using the `--max-old-space-size` flag.

```dockerfile
# Inside your Dockerfile / startup script
CMD ["node", "--max-old-space-size=400", "server.js"]
```

---

## 3. Variations & Depth: Command Injection Guards

A severe Node.js vulnerability occurs when executing terminal commands containing user inputs.

```javascript
// EXTREMELY VULNERABLE
import { exec } from 'node:child_process';
app.get('/ping', (req, res) => {
  exec(`ping -c 1 ${req.query.host}`, (err, stdout) => { ... });
});
// A request like /ping?host=127.0.0.1;cat+/etc/passwd runs the malicious script.
```

**The Guard:** Use `spawn` or `execFile` where inputs are passed as a safe argument array:
```javascript
// SAFE
import { spawn } from 'node:child_process';
const ping = spawn('ping', ['-c', '1', req.query.host]);
```

---

## 4. Practical Application: A Clustered HTTP Server

Let's build a clustered web server that scales across all available CPU cores and restarts workers when they crash.

**`cluster_server.js`**
```javascript
import cluster from 'node:cluster';
import http from 'node:http';
import { availableParallelism } from 'node:os';
import process from 'node:process';

const numCPUs = availableParallelism();

if (cluster.isPrimary) {
  console.log(`Primary process ${process.pid} is running`);

  // Fork workers based on available core count
  for (let i = 0; i < numCPUs; i++) {
    cluster.fork();
  }

  // Listen for worker crashes
  cluster.on('exit', (worker, code, signal) => {
    console.error(`Worker process ${worker.process.pid} died. Forking replacement...`);
    cluster.fork();
  });

} else {
  // Workers share the TCP port 5000
  http.createServer((req, res) => {
    res.writeHead(200);
    res.end(`Handled by worker PID: ${process.pid}\n`);
    
    // Simulate random crash to test auto-healing
    if (req.url === '/crash') {
      console.warn(`Worker ${process.pid} is crashing...`);
      process.exit(1);
    }
  }).listen(5000, () => {
    console.log(`Worker process ${process.pid} started`);
  });
}
```

---

## 5. Common Mistakes & Gotchas

- **Exceeding container bounds:** Running Node in containers without setting `--max-old-space-size`. Ensure the value is set to ~75–80% of the total container RAM limit to leave room for RSS and native allocations.
- **Storing API keys in code:** Hardcoding credentials inside files. Always read secrets from environmental variables (`process.env.DB_PASSWORD`) and inject them at run time.
- **Using `eval` or `new Function`:** Executing dynamically compiled strings is a massive cross-site scripting (XSS) / remote code execution threat. Avoid both completely.

---

## 🎯 Key Takeaways

- **Scale horizontally on VMs** using the `cluster` module.
- **Tune V8 space allocation** (`--max-old-space-size`) inside Docker to match container RAM limits.
- **Always isolate system arguments** using `spawn`/`execFile` to eliminate command injection vectors.

---

*← [performance](./08_performance_and_debugging.md) | [roadmap](../README.md)*
