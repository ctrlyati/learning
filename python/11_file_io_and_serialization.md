# 11 — File I/O & Serialization
> **Goal:** Read and write text, binary, JSON, CSV, pickle, and SQLite correctly — including encodings, streaming, and atomic writes.

## 1. The `open()` mental model — text vs binary, encoding always

`open(path, mode, encoding=...)` returns a file object that's also a context manager and an iterator. The mode is a short string:

| Mode | Meaning |
|------|---------|
| `r`  | read text (default) |
| `w`  | write text, truncate |
| `a`  | append text |
| `x`  | create-exclusive, fail if exists |
| `rb`/`wb`/`ab` | binary versions — get/give `bytes` |
| `r+` | read+write, no truncate |

Two rules that prevent 90% of file bugs:

1. **Always use `with`.** Files left open are leaked descriptors and (on Windows) locked.
2. **Always pass `encoding=` for text mode.** The default is platform-dependent (`utf-8` on Linux/Mac, often `cp1252` on Windows). Explicit avoids cross-platform surprises.

```python
from pathlib import Path

# Text — always pass encoding
with open("notes.txt", "w", encoding="utf-8") as f:
    f.write("héllo\n")

with open("notes.txt", "r", encoding="utf-8") as f:
    for line in f:                  # iterates lines lazily
        print(line.rstrip())

# Or using pathlib for one-shot reads/writes
text = Path("notes.txt").read_text(encoding="utf-8")
Path("out.txt").write_text(text, encoding="utf-8")

# Binary
data = Path("image.png").read_bytes()
Path("copy.png").write_bytes(data)
```

`newline=""` is the right choice when reading/writing CSVs (the `csv` module handles line endings itself).

## 2. Mechanism — streaming, encodings, atomic writes

### Streaming large files

Don't `.read()` a 10 GB file. Iterate:

```python
with open("huge.log", "r", encoding="utf-8") as f:
    for line in f:                # lazy — one line at a time
        process(line)

# For binary, read in chunks
with open("blob.bin", "rb") as f:
    while chunk := f.read(64 * 1024):     # walrus, 64KB at a time
        process(chunk)
```

### Encodings, briefly

`str` is unicode; `bytes` is raw. Encoding converts between them.

```python
"héllo".encode("utf-8")          # b'h\xc3\xa9llo'   — 6 bytes
b'h\xc3\xa9llo'.decode("utf-8")  # 'héllo'

# UTF-8 is the right default for everything new.
# Legacy data may need cp1252, latin-1, shift_jis, etc.
```

If you don't know the encoding, `chardet` (3rd party) can guess. But ideally: ask, document, and assume UTF-8.

### Atomic writes — never leave a half-written file

Crashes mid-write cause silent data corruption. The fix: write to a temp file in the same directory, fsync, rename.

```python
import os
from pathlib import Path

def atomic_write(path: Path, data: str, encoding: str = "utf-8") -> None:
    tmp = path.with_suffix(path.suffix + ".tmp")
    with tmp.open("w", encoding=encoding) as f:
        f.write(data)
        f.flush()
        os.fsync(f.fileno())          # ensure on disk
    tmp.replace(path)                  # atomic on POSIX & modern Windows

atomic_write(Path("config.json"), '{"theme": "dark"}')
```

`Path.replace` is atomic. Always use this pattern for files that must not be corrupted (configs, state, caches, output of long jobs).

## 3. Variations — JSON, CSV, pickle, SQLite

### JSON (recap from module 10, with details)

```python
import json
from pathlib import Path

# Streaming-friendly: one JSON object per line (JSONL/NDJSON)
with open("events.jsonl", "w", encoding="utf-8") as f:
    for event in events:
        f.write(json.dumps(event) + "\n")

with open("events.jsonl", encoding="utf-8") as f:
    for line in f:
        event = json.loads(line)
        process(event)
```

JSONL is the right format for append-only, streaming-friendly data. A single huge JSON array forces you to load it all to parse it.

### CSV — DictReader/DictWriter

```python
import csv

with open("rows.csv", newline="", encoding="utf-8") as f:
    reader = csv.DictReader(f)
    for row in reader:
        print(row["name"], row["age"])

with open("out.csv", "w", newline="", encoding="utf-8") as f:
    writer = csv.DictWriter(f, fieldnames=["name", "age"])
    writer.writeheader()
    writer.writerows([{"name": "Yati", "age": 30}, {"name": "Bob", "age": 40}])
```

The `newline=""` is *not optional* — without it, you'll get blank lines on Windows or mis-parsed quoted fields.

For larger or messier data, **pandas** or **polars** is the next step. CSV stdlib is fine for scripts and small data.

### Pickle — serialize Python objects, with a giant warning

```python
import pickle

with open("state.pkl", "wb") as f:
    pickle.dump(obj, f)

with open("state.pkl", "rb") as f:
    obj = pickle.load(f)
```

**Pickle is unsafe.** Loading a pickle from an untrusted source can execute arbitrary code. Use it only for *trusted* internal storage (cache, snapshots, multiprocessing IPC). For interchange, JSON or msgpack.

### SQLite — a real database in stdlib

```python
import sqlite3
from pathlib import Path

db = Path("app.db")
with sqlite3.connect(db) as conn:
    conn.execute("""
        CREATE TABLE IF NOT EXISTS users (
            id INTEGER PRIMARY KEY,
            email TEXT NOT NULL UNIQUE,
            created_at TEXT NOT NULL
        )
    """)
    conn.execute(
        "INSERT INTO users (email, created_at) VALUES (?, ?)",
        ("yati@example.com", "2026-05-11"),
    )

# Querying
with sqlite3.connect(db) as conn:
    conn.row_factory = sqlite3.Row     # rows act like dicts
    for row in conn.execute("SELECT id, email FROM users WHERE email LIKE ?", ("%example%",)):
        print(row["id"], row["email"])
```

**Always use `?` placeholders.** Never f-string SQL — that's injection. SQLite is shockingly capable: WAL mode, JSON1 extension, full-text search, decent for read-heavy services up to ~100GB.

For richer ORM, **SQLAlchemy** (`sqlalchemy.orm`) is the standard.

### `tomllib` (3.11+) — read TOML configs

```python
import tomllib
from pathlib import Path

with open("pyproject.toml", "rb") as f:    # must be binary
    config = tomllib.load(f)
```

stdlib only *reads* TOML. To write, use `tomli-w` or `tomlkit`.

## 4. Practical Application — a deduping CSV → JSONL pipeline with atomic output

Realistic ETL job: read a possibly large CSV, dedupe by a key, write a JSONL file atomically. Logs progress and survives crashes:

```python
import csv
import json
import logging
import os
from pathlib import Path

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger(__name__)


def csv_to_jsonl(src: Path, dst: Path, key: str) -> tuple[int, int]:
    """Convert CSV → JSONL, dedupe by `key`. Returns (read, written)."""
    seen: set[str] = set()
    read = 0
    written = 0

    tmp = dst.with_suffix(dst.suffix + ".tmp")

    try:
        with src.open(newline="", encoding="utf-8") as inp, \
             tmp.open("w", encoding="utf-8") as out:
            reader = csv.DictReader(inp)
            for row in reader:
                read += 1
                k = row.get(key, "")
                if not k or k in seen:
                    continue
                seen.add(k)
                out.write(json.dumps(row, ensure_ascii=False) + "\n")
                written += 1
                if read % 10_000 == 0:
                    log.info("processed %d rows (%d unique)", read, written)
            out.flush()
            os.fsync(out.fileno())
    except Exception:
        tmp.unlink(missing_ok=True)
        log.exception("conversion failed; cleaned up temp file")
        raise

    tmp.replace(dst)
    log.info("wrote %s: %d rows in, %d unique out", dst, read, written)
    return read, written


if __name__ == "__main__":
    csv_to_jsonl(Path("input.csv"), Path("output.jsonl"), key="email")
```

What's idiomatic:

- Streams the CSV — works on files larger than RAM.
- Dedupes with a `set`, O(1) per check.
- Writes to `.tmp`, fsyncs, then `replace` — atomic; a crash never produces a half-written `output.jsonl`.
- `logging` (not `print`), with periodic progress, and `log.exception` inside `except` to capture the full traceback.

## 5. Common Mistakes & Gotchas

- **No `encoding=` argument.** Default depends on the OS locale. Production bug waiting to happen, especially on Windows. Always explicit, almost always `"utf-8"`.
- **`open()` without `with`.** Leaks descriptors; on Windows, locks files until the next GC.
- **Reading a huge file into memory.** Iterate the file object directly; chunked binary reads with the walrus pattern.
- **Loading untrusted pickles.** It's RCE. Treat pickle as "trusted, internal use only."
- **CSV without `newline=""`.** Blank lines, broken quoting on Windows.
- **f-strings in SQL.** SQL injection. Always parameterize.
- **Writing files non-atomically when they matter.** Configs, state, caches — all need `tmp + fsync + rename`.
- **Mixing text and binary modes.** A `bytes` write into a text file (or vice versa) raises `TypeError`. Pick one and stick with it for the file.
- **Forgetting `flush()`/`fsync()` for durability.** Closing a file flushes user-space buffers, but the OS may still cache. `fsync` is required for crash safety.
- **Using `csv` for big-data.** It's fine for ~GB; for tens of GB use polars or DuckDB.

## 🎯 Key Takeaways

- **`with open(..., encoding="utf-8")` is the only correct way to open a text file.** No exceptions. Reviewers should and do flag missing `encoding=`.
- **Stream by default.** `for line in f`, walrus-chunked binary reads, JSONL over one giant JSON array — these scale; `read()`/`load()` doesn't.
- **Atomic writes (`tmp` + `fsync` + `replace`)** are the cheap insurance that turns a crash from "data corruption" into "no-op."
- **SQLite is in the stdlib and underrated.** For local state, caches, small services, it beats a flat file or a JSON blob and avoids a real DB dependency.
- **Pickle is for trusted, internal data only.** For interchange or anything user-facing, JSON/JSONL/msgpack with explicit schemas (pydantic, module 16).

*← [prev](./10_stdlib_essentials.md) | [next →](./12_concurrency.md)*
