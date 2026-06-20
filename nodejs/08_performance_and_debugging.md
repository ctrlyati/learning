# 08 — Performance & Debugging

> **Goal:** Detect and repair memory leaks in the V8 heap, profile execution bottlenecks, and connect the Chrome DevTools inspector to a running Node.js process.

---

## 1. Concept: Memory & CPU Profiling

To build stable, high-throughput Node.js servers, you must understand how Node.js manages memory and execution time.
- **Heap Allocation:** Where V8 stores objects, arrays, and closures.
- **Garbage Collection (GC):** V8 dynamically frees memory that is no longer reachable from the root (global) scope.
- **Memory Leak:** Occurs when JavaScript retains references to unused objects, preventing the GC from releasing their memory.

---

## 2. Mechanism: Memory Footprint & Heap Analysis

### Reading Process Memory
You can fetch memory statistics inside your code using `process.memoryUsage()`:

```javascript
console.log(process.memoryUsage());
/* Output:
{
  rss: 34213888,      // Resident Set Size (total memory allocated by OS for the process)
  heapTotal: 6512640, // Total size of V8's allocated heap
  heapUsed: 4210344,  // Actual memory consumed by JavaScript objects
  external: 1821030,  // C++ object memory usage (e.g. buffers)
  arrayBuffers: 18023 // Buffer allocations
}
*/
```

### Profiling Tools
1. **`node --inspect`**: Boots the Node.js process with a WebSocket debugging agent listening on a local port. You can connect Chrome DevTools (`chrome://inspect`) to inspect the call stack, set breakpoints, and run profiles.
2. **Heap Snapshot**: A capture of V8's memory state. Comparing two snapshots taken before and after a workload exposes which objects are multiplying.
3. **CPU Profiler**: Tracks code execution times, helping you identify hot functions causing delays.

---

## 3. Variations & Depth: Anatomy of a Memory Leak

A common memory leak pattern in Node.js is the **Closure Leak**.

```javascript
// Leaking closure example
let leakCollector = [];

function createLeak() {
  const largeArray = new Array(1000000).fill('garbage');
  
  // This nested function retains a reference to its parent scope
  return function() {
    if (largeArray) return "leak";
  };
}

setInterval(() => {
  // leakCollector keeps holding the nested function, 
  // which holds `largeArray` in its closure scope forever.
  leakCollector.push(createLeak());
}, 100);
```

---

## 4. Practical Application: Programmatic Heap Dumps

Let's write a script that generates a heap snapshot automatically when memory usage exceeds a specific threshold.

**`heap_dumper.js`**
```javascript
import fs from 'node:fs';
import v8 from 'node:v8';

function checkMemoryLimit() {
  const stats = process.memoryUsage();
  const heapUsedMB = stats.heapUsed / 1024 / 1024;
  console.log(`Heap usage: ${heapUsedMB.toFixed(2)} MB`);

  const LIMIT_MB = 100; // Trigger dump at 100MB heap usage
  if (heapUsedMB > LIMIT_MB) {
    console.warn(`[ALERT] Heap exceeded ${LIMIT_MB}MB. Generating snapshot...`);
    
    // Generate snapshot stream
    const snapshotStream = v8.getHeapSnapshot();
    const fileName = `./snapshot-${Date.now()}.heapsnapshot`;
    const fileStream = fs.createWriteStream(fileName);
    
    snapshotStream.pipe(fileStream);
    
    fileStream.on('finish', () => {
      console.log(`Snapshot saved to: ${fileName}`);
      process.exit(1); // Exit process to avoid complete OOM crash
    });
  }
}

// Deliberately leak memory to trigger snapshot
const leakArray = [];
setInterval(() => {
  for (let i = 0; i < 100000; i++) {
    leakArray.push({ leakedObject: true, timestamp: Date.now() });
  }
  checkMemoryLimit();
}, 200);
```

### Inspecting the Snapshot
1. Open Google Chrome.
2. Navigate to `chrome://inspect` and open "DevTools for Node".
3. Select the **Memory** tab, click **Load**, and select the generated `.heapsnapshot` file.
4. Filter by class name (e.g., `Object` or `Array`) and look at the **Retainers** tree to see what is holding the reference.

---

## 5. Common Mistakes & Gotchas

- **Exposing the debugger globally:** Running `node --inspect=0.0.0.0:9229` in public environments allows anyone on the network to connect and execute arbitrary code on your server. **Only bind to `127.0.0.1`**.
- **Forgetting to remove console.logs in hot paths:** Writing `console.log` is a synchronous block that goes to standard output. If run in a critical loop processing 10,000 requests/second, it will degrade performance by up to 90%.
- **Forgetting to clear timeouts/intervals:** Timers are attached to V8's global clock. If you start a `setInterval` inside a class constructor and never call `clearInterval`, the class instance will never be garbage collected.

---

## 🎯 Key Takeaways

- **Debug leaks with Heap Snapshots** inside Chrome DevTools.
- **Use `process.memoryUsage().heapUsed`** to monitor the true memory size of your JS variables.
- **Always keep the Node inspector bound locally** (`127.0.0.1`) to prevent remote execution security threats.

---

*← [child processes](./07_child_processes_and_worker_threads.md) | [next → 09 Security & Production](./09_security_and_production.md)*
