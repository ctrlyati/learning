# 04 — File System & Path Operations

> **Goal:** Leverage the fs/promises API, safely handle cross-platform path resolution, and implement robust file watchers.

---

## 1. Concept: System Paths and the Disk

Node.js provides two core modules to interact with files:
1. **`node:path`:** Manipulates path strings safely across operating systems (resolves windows `\` vs unix `/` differences).
2. **`node:fs`:** Reads, writes, creates, deletes, and watches files and directories.

---

## 2. Mechanism: FS Methods & Path Resolution

### Three Flavors of `fs`
- **Synchronous:** `fs.readFileSync(...)`. Blocks the main thread. Avoid in server routes.
- **Callback:** `fs.readFile(...)`. Non-blocking, but leads to callback nesting.
- **Promises:** `fs.promises.readFile(...)` or `import fs from 'node:fs/promises'`. Modern, clean async/await syntax.

### Path Resolution: `path.join()` vs `path.resolve()`
Understanding path resolution prevents file-not-found exceptions:

- **`path.join('/a', 'b', '../c')`**: Concatenates fragments and normalizes separators. Returns `'/a/c'`. Does *not* force an absolute path unless the first argument is absolute.
- **`path.resolve('/a', 'b', '../c')`**: Resolves fragments into an **absolute path**, working from right to left, relative to `process.cwd()` (Current Working Directory).

```javascript
import path from 'node:path';

// Assume process.cwd() is '/Users/project'
console.log(path.join('src', 'config.json'));
// Output: 'src/config.json' (relative)

console.log(path.resolve('src', 'config.json'));
// Output: '/Users/project/src/config.json' (absolute)
```

---

## 3. Variations & Depth: File Watching

Node.js offers two ways to monitor file modifications:
- **`fs.watchFile()`**: Polls the file system at set intervals. Resource-intensive but works reliably across OS configurations.
- **`fs.watch()`**: Hooks into the native OS event system (inotify, FSEvents, or ReadDirectoryChangesW). Highly performant, but **can trigger multiple events for a single modification** and has cross-platform quirks.

```javascript
import fs from 'node:fs';

// Cross-platform watch with debouncing
let debounceTimer;
fs.watch('config.json', (eventType, filename) => {
  if (filename) {
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      console.log(`File ${filename} updated via ${eventType}`);
    }, 100);
  }
});
```

---

## 4. Practical Application: A Safe Recursive File Scanner

Let's build a scanner that recursively walks a folder structure, calculating total size and counting files, while ignoring hidden files.

**`scanner.js`**
```javascript
import fs from 'node:fs/promises';
import path from 'node:path';

async function scanDirectory(dirPath) {
  let totalSize = 0;
  let fileCount = 0;

  async function walk(currentPath) {
    const entries = await fs.readdir(currentPath, { withFileTypes: true });

    for (const entry of entries) {
      // Ignore hidden files/folders (starting with .)
      if (entry.name.startsWith('.')) continue;

      const fullPath = path.join(currentPath, entry.name);

      if (entry.isDirectory()) {
        await walk(fullPath);
      } else if (entry.isFile()) {
        const stats = await fs.stat(fullPath);
        totalSize += stats.size;
        fileCount++;
      }
    }
  }

  const absoluteStart = path.resolve(dirPath);
  await walk(absoluteStart);

  return { absoluteStart, totalSize, fileCount };
}

scanDirectory('.')
  .then(res => {
    console.log(`Scan completed for: ${res.absoluteStart}`);
    console.log(`Files found: ${res.fileCount}`);
    console.log(`Total size: ${(res.totalSize / 1024).toFixed(2)} KB`);
  })
  .catch(console.error);
```

---

## 5. Common Mistakes & Gotchas

- **Relative path bug:** Files specified as relative (like `fs.readFile('./file.txt')`) resolve relative to the current terminal working directory (`process.cwd()`), *not* the location of the running script. Always anchor path imports using `import.meta.url` or `__dirname` if absolute paths are required.
- **Writing to missing paths:** Calling `fs.writeFile('nested/dir/file.txt')` will throw an error if `nested/dir` does not exist. You must create the path first with `await fs.mkdir('nested/dir', { recursive: true })`.
- **Path Traversal Security vulnerability:** Directly passing user-input parameters into file system calls (e.g., `fs.readFile(path.join('/files', userInput))`) allows malicious users to supply `'../../etc/passwd'`. Always validate and sanitize paths to ensure they stay inside the target root directory.

---

## 🎯 Key Takeaways

- **Always anchor file operations** with absolute paths resolved via `path.resolve` or `path.dirname(fileURLToPath(import.meta.url))`.
- **Use `fs.promises`** over synchronous equivalents to keep the event loop responsive.
- **Wrap `fs.watch` with a debouncing function** to handle multi-fire event noise.

---

*← [buffers & streams](./03_buffers_and_streams.md) | [next → 05 Networking: HTTP & Net](./05_networking_http_net.md)*
