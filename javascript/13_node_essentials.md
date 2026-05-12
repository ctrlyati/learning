# 13 — Node.js Essentials: fs, path, streams, http, process

> **Goal:** Build, read, and modify backend Node.js code confidently — file I/O the right way, streams for large data, an HTTP server from scratch, and the process API.

---

## 1. The Node Standard Library — Mental Model

Node ships a sizable standard library. Every module has a `node:` prefix in modern code (makes intent explicit and avoids shadowing by an npm package).

```js
import fs from "node:fs";
import { promises as fsp } from "node:fs";
import path from "node:path";
import http from "node:http";
import { Readable } from "node:stream";
import process from "node:process";
import os from "node:os";
import crypto from "node:crypto";
```

Three flavors of file APIs:
- **Sync:** `fs.readFileSync` — blocks the event loop. Use only at startup.
- **Callback:** `fs.readFile(path, cb)` — old style.
- **Promise:** `fs.promises.readFile(path)` — preferred.

---

## 2. fs & path — Under the Hood

### path — never concatenate paths manually
```js
import path from "node:path";

path.join("a", "b", "c.txt");          // "a/b/c.txt" (or "a\\b\\c.txt" on Windows)
path.resolve("a", "b");                 // absolute path from cwd
path.dirname("/a/b/c.txt");             // "/a/b"
path.basename("/a/b/c.txt");            // "c.txt"
path.basename("/a/b/c.txt", ".txt");    // "c"
path.extname("/a/b/c.txt");             // ".txt"
path.parse("/a/b/c.txt");               // { root, dir, base, name, ext }
```

In ESM, get current dir/file:
```js
import { fileURLToPath } from "node:url";
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
```

### fs basics (Promises)
```js
import { promises as fs } from "node:fs";

await fs.writeFile("hello.txt", "hi", "utf8");
const text = await fs.readFile("hello.txt", "utf8");

await fs.mkdir("a/b/c", { recursive: true });
const entries = await fs.readdir(".", { withFileTypes: true });
for (const ent of entries) {
  console.log(ent.isDirectory() ? "DIR " : "FILE", ent.name);
}

const stat = await fs.stat("hello.txt");
console.log(stat.size, stat.mtime, stat.isFile());

await fs.rename("hello.txt", "hi.txt");
await fs.rm("hi.txt");
await fs.rm("a", { recursive: true, force: true }); // like rm -rf
```

### File handles for partial reads
```js
const fh = await fs.open("big.bin", "r");
try {
  const buf = Buffer.alloc(64);
  const { bytesRead } = await fh.read(buf, 0, 64, 1024); // read 64 bytes from offset 1024
} finally {
  await fh.close();
}
```

### Watching files
```js
import { watch } from "node:fs";
const watcher = watch("./src", { recursive: true }, (event, filename) => {
  console.log(event, filename); // "change" or "rename"
});
// watcher.close() when done
```
For dev-server quality, prefer `chokidar` from npm — it normalizes platform quirks.

---

## 3. Streams, HTTP, Process

### Streams — for things bigger than RAM
A **stream** is data delivered piece-by-piece. Four kinds: `Readable`, `Writable`, `Duplex`, `Transform`.

```js
import { createReadStream, createWriteStream } from "node:fs";
import { pipeline } from "node:stream/promises";
import { createGzip } from "node:zlib";

// Compress a 4GB file without ever loading it all in memory:
await pipeline(
  createReadStream("big.log"),
  createGzip(),
  createWriteStream("big.log.gz"),
);
```

`pipeline` handles errors and cleanup. **Always use `pipeline`** instead of `.pipe()` chains.

### Async iteration over streams
```js
import { createReadStream } from "node:fs";

for await (const chunk of createReadStream("file.txt", { encoding: "utf8" })) {
  process.stdout.write(chunk);
}
```

### Custom transform stream
```js
import { Transform } from "node:stream";

class Upper extends Transform {
  _transform(chunk, _enc, cb) {
    cb(null, chunk.toString().toUpperCase());
  }
}
```

### `Buffer` — Node's binary data
```js
const b = Buffer.from("hello", "utf8");
b.length;            // 5
b.toString("hex");   // "68656c6c6f"
Buffer.alloc(16);    // zero-filled, length 16
Buffer.allocUnsafe(16); // faster but contains old memory — always overwrite first
```

In modern code, prefer `Uint8Array`/`ArrayBuffer` (cross-platform) when you can.

### HTTP server from scratch
```js
import http from "node:http";

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, `http://${req.headers.host}`);
  if (req.method === "GET" && url.pathname === "/health") {
    res.writeHead(200, { "Content-Type": "application/json" });
    return res.end(JSON.stringify({ ok: true }));
  }
  if (req.method === "POST" && url.pathname === "/echo") {
    let body = "";
    for await (const chunk of req) body += chunk;
    res.writeHead(200, { "Content-Type": "application/json" });
    return res.end(body);
  }
  res.writeHead(404).end("Not found");
});

server.listen(3000, () => console.log("http://localhost:3000"));
```

For real apps, use a framework (Express, Fastify, Hono) — but knowing the underlying API saves you when frameworks misbehave.

### `process` essentials
```js
process.argv;           // [node, script, ...args]
process.env.NODE_ENV;   // env vars
process.cwd();          // current working dir
process.platform;       // "linux", "darwin", "win32"
process.versions.node;  // "22.x.x"
process.pid;
process.memoryUsage();
process.exit(0);

// Listen for shutdown
process.on("SIGINT", async () => {
  console.log("received SIGINT, shutting down");
  await server.close();
  process.exit(0);
});
```

### Reading env files (Node 20.6+)
```bash
node --env-file=.env app.js
```
No more `dotenv` dependency for new projects.

---

## 4. Practical Application — A Resilient File Processor

A small CLI that reads a CSV, hashes the email column, writes a new CSV — using streams so it works on multi-GB files.

```js
// hash-emails.js
import { createReadStream, createWriteStream } from "node:fs";
import { pipeline } from "node:stream/promises";
import { Transform } from "node:stream";
import { createHash } from "node:crypto";
import { createInterface } from "node:readline";

const [, , inputPath, outputPath] = process.argv;
if (!inputPath || !outputPath) {
  console.error("usage: node hash-emails.js <in.csv> <out.csv>");
  process.exit(2);
}

const input = createReadStream(inputPath, { encoding: "utf8" });
const output = createWriteStream(outputPath);
const lines = createInterface({ input });

let header;
let count = 0;

const transform = new Transform({
  writableObjectMode: true,
  readableObjectMode: false,
  transform(line, _enc, cb) {
    if (!header) {
      header = line.split(",");
      this.push(line + "\n");
      return cb();
    }
    const cols = line.split(",");
    const emailIdx = header.indexOf("email");
    if (emailIdx >= 0 && cols[emailIdx]) {
      cols[emailIdx] = createHash("sha256").update(cols[emailIdx]).digest("hex");
    }
    count++;
    cb(null, cols.join(",") + "\n");
  },
});

// Bridge readline (line events) → transform (object writes)
const lineSource = (async function* () {
  for await (const line of lines) yield line;
})();

await pipeline(
  Readable.from(lineSource),
  transform,
  output,
);

console.log(`Processed ${count} rows → ${outputPath}`);

// Helper import (top of file in real code)
import { Readable } from "node:stream";
```

Run: `node hash-emails.js users.csv users-hashed.csv`.

This script:
- Doesn't load the file into memory.
- Uses `pipeline` for proper backpressure and error handling.
- Exits cleanly with a non-zero code on bad usage.
- Uses crypto from the std lib — no dependencies.

---

## 5. Common Mistakes & Gotchas

- **Using sync APIs in request handlers:** `fs.readFileSync` blocks the **entire** server. Sync only at startup.
- **Not handling stream errors:**
  ```js
  src.pipe(dst);   // if src errors, dst leaks
  // Use:
  await pipeline(src, dst);
  ```
- **Forgetting to consume the request body:** in HTTP, if you ignore POST body, the connection hangs until timeout.
- **`process.env.PORT` is a string.** `Number(process.env.PORT) || 3000`.
- **Forgetting `Path.join`:** hardcoded `/` breaks on Windows.
- **Reading huge files with `readFile`:** crashes with OOM. Use streams.
- **Buffer encoding default:** if you forget `"utf8"` in `readFile`, you get a `Buffer`, not a string.
- **`__dirname` doesn't exist in ESM.** See module 08.
- **Unhandled stream `error` events crash the process** in modern Node. Always attach `error` handlers or use `pipeline`.
- **`fs.exists` is deprecated.** Use `fs.access` or a `stat` + try/catch (or `fs.existsSync` for sync — still supported).
- **`process.exit()` is abrupt:** logs may not flush, files may not close. Set `process.exitCode = 1` and let the loop drain.
- **CommonJS `require` is cached** by absolute path. Two scripts importing the same module share state.
- **Watching files cross-platform** is inconsistent — `chokidar` exists for a reason.

```js
// "Wat"
process.cwd() === path.resolve(".");   // not always — cwd can change
typeof require;                         // "function" in CJS, "undefined" in ESM
new URL("./x.js", import.meta.url);     // resolves relative to current ESM file
```

### Quick comparison: Node vs Bun vs Deno
| Feature | Node | Bun | Deno |
|---------|------|-----|------|
| `fs.promises` | yes | yes | uses `Deno.readFile` (or Node compat) |
| Native `fetch` | yes | yes | yes |
| TS support | needs loader | built-in | built-in |
| ESM/CJS | both | both | ESM only |
| Permissions | none | none | granular flags |
| Speed | baseline | usually fastest | fast, secure |

---

## 🎯 Key Takeaways

- **Use `fs.promises` and `await`** by default. Sync only at startup.
- **Always use `path.join` / `path.resolve`** — never string-concatenate paths.
- **Use streams + `pipeline`** for anything bigger than memory or for I/O pipelines. They handle errors and backpressure.
- **`new URL(req.url, base)`** is the right way to parse incoming HTTP URLs — don't regex them.
- **Handle SIGINT/SIGTERM** for graceful shutdown in services. Set `process.exitCode` and let the loop drain instead of `process.exit()`-ing abruptly.

---

*← [12 DOM & Browser APIs](./12_dom_browser_apis.md) | [next → 14 npm, package.json, semver, monorepos](./14_npm_package_json_semver.md)*
