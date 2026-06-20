# 03 — Buffers & Streams

> **Goal:** Understand binary memory handling via Buffers, write stream pipelines with backpressure controls, and process huge datasets without memory starvation.

---

## 1. Concept: Buffers and Streams

In Node.js, we deal with raw binary data (files, network packets) using **Buffers**. A `Buffer` is a chunk of memory allocated outside the V8 V8 heap, representing a fixed-size sequence of bytes.

A **Stream** is an abstract interface for working with streaming data. Instead of loading an entire file (e.g., a 2GB log) into a Buffer inside RAM, streams read it block-by-block (typically 64KB chunks), processing it incrementally.

```
File on Disk (2GB)
  [==== Chunk 1 ====] ---> [ Buffer (64KB) ] ---> Processed/Sent
  [==== Chunk 2 ====] ---> [ Buffer (64KB) ] ---> Processed/Sent
  ...
```

---

## 2. Mechanism: Buffer Allocation & Stream Types

### Buffer Allocation
- `Buffer.alloc(size)`: Allocates zero-filled memory. Safe but slower.
- `Buffer.allocUnsafe(size)`: Allocates raw memory without clearing it. Much faster, but **contains old, potentially sensitive memory garbage**. You must overwrite it before reading.

### Stream Types
Node.js offers four base stream classes:
1. **Readable:** Source of data (e.g., `fs.createReadStream`).
2. **Writable:** Destination for data (e.g., `fs.createWriteStream`).
3. **Duplex:** Bidirectional (e.g., a TCP socket, `net.Socket`).
4. **Transform:** A duplex stream that modifies data as it passes through (e.g., `zlib.createGzip`).

### Backpressure
When a Readable stream produces data faster than a Writable stream can consume it, data accumulates in the write queue. This is called **backpressure**.
- If a Writable stream's internal queue exceeds `highWaterMark`, `write()` returns `false`.
- The Readable stream must then pause writing until the Writable stream emits the `'drain'` event.

---

## 3. Variations & Depth: pipe() vs pipeline()

In older code, streams were connected using `.pipe()`:
```javascript
// WARNING: Memory Leak Hazard
readable.pipe(writable);
```
**The Trap:** If `readable` throws an error, `pipe()` does **not** close `writable` or cleanup handlers. The file descriptors remain open, causing memory leaks.

**The Solution:** Always use `pipeline` from `node:stream/promises` for modern code. It handles errors and automatic cleanups.

```javascript
import { pipeline } from 'node:stream/promises';
import fs from 'node:fs';
import zlib from 'zlib';

await pipeline(
  fs.createReadStream('large.txt'),
  zlib.createGzip(),
  fs.createWriteStream('large.txt.gz')
);
// Handles errors automatically, closes all descriptors, and returns a promise.
```

---

## 4. Practical Application: A Custom Transform Stream

Let's write a custom Transform stream that censors specific words in a text stream.

**`censor_stream.js`**
```javascript
import { Transform } from 'node:stream';
import { pipeline } from 'node:stream/promises';
import { Readable, Writable } from 'node:stream';

class CensorTransform extends Transform {
  constructor(forbiddenWord, options) {
    super(options);
    this.forbiddenWord = forbiddenWord;
  }

  // _transform is called for every incoming chunk
  _transform(chunk, encoding, callback) {
    const text = chunk.toString();
    const regex = new RegExp(this.forbiddenWord, 'gi');
    const censored = text.replace(regex, '****');
    
    // Push the modified chunk to the next stream in the pipeline
    this.push(censored);
    callback(); // Indicate processing is done for this chunk
  }
}

async function run() {
  const source = Readable.from(['Node.js is awesome. ', 'Malware is dangerous. ', 'Javascript is fun.']);
  const censor = new CensorTransform('malware');
  const destination = new Writable({
    write(chunk, encoding, callback) {
      console.log('Received:', chunk.toString());
      callback();
    }
  });

  await pipeline(source, censor, destination);
  console.log('Pipeline execution finished successfully.');
}

run().catch(console.error);
```

---

## 5. Common Mistakes & Gotchas

- **Leaking sensitive data with `Buffer.allocUnsafe()`:** If you allocate unsafe memory and write less than the allocated size, the unwritten bytes will contain whatever was in that memory address previously (e.g., user passwords, database queries).
- **Ignoring the return value of `stream.write()`:** Writing without checking for backpressure will inflate the Node.js process heap memory until it crashes.
- **Handling stream errors manually:** Attaching `.on('error', ...)` on every stream segment is error-prone. Use `pipeline` instead.

---

## 🎯 Key Takeaways

- **Never read entire files into memory** for I/O operations. Use streams.
- **Prefer `pipeline()`** from `node:stream/promises` over legacy `.pipe()`.
- **Beware of `Buffer.allocUnsafe()`**; ensure you overwrite or initialize the buffer immediately to avoid leaking memory secrets.

---

*← [modules](./02_modules_esm_vs_cjs.md) | [next → 04 File System & Path](./04_file_system_and_path.md)*
