# 10 — Standard Library Essentials
> **Goal:** Know the seven stdlib modules you'll use every week so you stop installing packages for things Python already does.

Python's stdlib is famously "batteries included." This module covers the seven you'll touch daily: `collections`, `pathlib`, `json`, `datetime`, `subprocess`, `logging`, plus `os`/`sys`/`io` highlights.

## 1. `collections` — the better dicts and lists

```python
from collections import Counter, defaultdict, deque, ChainMap, namedtuple
```

### `Counter`

```python
words = "the quick brown fox jumps over the lazy dog the".split()
c = Counter(words)
c.most_common(2)         # [('the', 3), ('quick', 1)]
c["the"]                 # 3
c["never-seen"]          # 0 — no KeyError, returns 0
c + Counter(["the"])     # Counter arithmetic
```

The "what shows up most often" tool. Replaces 90% of "I need to count things" loops.

### `defaultdict`

```python
from collections import defaultdict
groups = defaultdict(list)
for name, dept in employees:
    groups[dept].append(name)
# groups['eng'] = ['alice', 'bob']  — auto-created
```

Cleaner than `dict.setdefault(key, []).append(x)` for the same job. The factory (`list`, `set`, `int`, `lambda: ...`) is called when a missing key is read.

### `deque` — double-ended queue

```python
from collections import deque
buf = deque(maxlen=5)         # bounded; old items drop
for x in stream:
    buf.append(x)
# Always the last 5 items, no manual trimming.

q = deque()
q.appendleft(1); q.append(2)  # both ends are O(1)
q.popleft()                    # O(1) — list.pop(0) is O(n)
```

Use `deque` for FIFO queues and sliding windows. Lists are slow at the front.

### `ChainMap`

```python
from collections import ChainMap
defaults = {"theme": "dark", "lang": "en"}
overrides = {"theme": "light"}
config = ChainMap(overrides, defaults)
config["theme"]   # 'light'  — first match wins
config["lang"]    # 'en'
```

Layered configuration without copying.

## 2. `pathlib` — stop doing string surgery on paths

`pathlib.Path` is the modern, OS-agnostic way to handle filesystem paths. **Stop using `os.path.join` and string concatenation.**

```python
from pathlib import Path

home = Path.home()
project = Path(__file__).resolve().parent.parent
data = project / "data" / "input.csv"     # / operator joins

data.exists()
data.is_file()
data.suffix              # '.csv'
data.stem                # 'input'
data.parent              # Path('.../data')
data.name                # 'input.csv'

# Reading
text = data.read_text(encoding="utf-8")
blob = data.read_bytes()

# Writing
out = project / "out.txt"
out.write_text("hello\n")

# Iterating
for p in (project / "src").rglob("*.py"):    # recursive
    print(p.relative_to(project))

# Mkdir
(project / "build" / "tmp").mkdir(parents=True, exist_ok=True)
```

`Path` works on Windows and POSIX. `__file__` plus `.resolve().parent` is the canonical "find my project root" idiom.

## 3. `json` — parse and serialize

```python
import json

# String <-> dict
data = json.loads('{"a": 1, "b": [2, 3]}')
text = json.dumps(data, indent=2, sort_keys=True)

# File round-trip
with open("config.json") as f:
    cfg = json.load(f)
with open("out.json", "w") as f:
    json.dump(cfg, f, indent=2)

# Custom encoder for non-JSON types
from datetime import datetime
def default(o):
    if isinstance(o, datetime):
        return o.isoformat()
    raise TypeError(f"can't serialize {type(o).__name__}")

json.dumps({"now": datetime.now()}, default=default)
```

The trick most people miss: `default=` for custom serialization. For decoding, `object_hook=` lets you transform dicts during parse.

For high-throughput JSON, `orjson` (3rd party) is 5–10× faster.

## 4. `datetime` — handle time correctly the first time

The single biggest rule: **always use timezone-aware UTC datetimes** unless you have an explicit, documented reason not to.

```python
from datetime import datetime, timedelta, timezone, UTC

now_utc = datetime.now(UTC)              # 3.12+; equivalent: datetime.now(timezone.utc)
now_local = datetime.now()                # naive — no tz info, dangerous

# Parse / format
ts = datetime.fromisoformat("2026-05-11T12:00:00+00:00")
ts.isoformat()                            # '2026-05-11T12:00:00+00:00'

# Arithmetic
later = now_utc + timedelta(hours=2, minutes=30)
delta = later - now_utc
delta.total_seconds()                     # 9000.0

# Convert to local for display only
from zoneinfo import ZoneInfo            # stdlib, no pytz needed
ts.astimezone(ZoneInfo("America/New_York"))
```

Naive vs aware: a naive `datetime` has no `tzinfo`, so comparing or subtracting an aware and a naive raises `TypeError`. Pick aware-UTC and stick to it inside your code.

For dates without time, use `datetime.date`. For durations, `timedelta`.

## 5. `subprocess` — run external commands safely

```python
import subprocess

# The right form: list of args, no shell
r = subprocess.run(
    ["git", "rev-parse", "HEAD"],
    capture_output=True,
    text=True,                # decodes stdout/stderr as str instead of bytes
    check=True,                # raise CalledProcessError on non-zero exit
    timeout=30,
)
print(r.stdout.strip())

# Don't do this — shell injection waiting to happen:
# subprocess.run(f"git log {user_input}", shell=True)

# Pipe between processes
p1 = subprocess.Popen(["cat", "big.log"], stdout=subprocess.PIPE)
p2 = subprocess.run(["grep", "ERROR"], stdin=p1.stdout, capture_output=True, text=True)
p1.stdout.close()
print(p2.stdout)
```

Rules of thumb:

- Always use the **list form** (`["cmd", "arg1", "arg2"]`), never `shell=True` unless you've sanitized the input (and even then, prefer not to).
- Always set `check=True` and `timeout=` — silent failures and hangs are the two most common bugs.
- Use `text=True` for human-readable output; bytes for binary.

## 6. `logging` — your one and only `print` replacement in production

```python
import logging

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)-7s %(name)s: %(message)s",
)
log = logging.getLogger(__name__)

log.debug("not shown by default")
log.info("user %s logged in", user_id)
log.warning("retrying after %s", err)
log.error("payment failed", extra={"order_id": 42})
log.exception("unhandled")    # inside an except — includes traceback
```

Key idioms:

- **One logger per module:** `log = logging.getLogger(__name__)`. Lets you tune verbosity per-package.
- **Use `%s` lazy formatting,** not f-strings: `log.info("user %s", uid)` — only formats if the level is enabled. Avoids cost when DEBUG is off.
- **`log.exception` only inside `except`:** automatically includes the traceback.
- **Configure once at the entry point.** Libraries should never call `basicConfig`.

For production-grade structured logging, see module 17.

## 7. `os`, `sys`, `io` — the system glue

```python
import os, sys

os.environ.get("API_KEY", "default")
os.environ["DEBUG"] = "1"
os.cpu_count()

sys.argv                  # command-line args
sys.exit(1)                # exit with status
sys.stderr.write("oops\n")
sys.version_info >= (3, 12)

# In-memory file objects (for tests, intermediate buffers)
import io
buf = io.StringIO()
buf.write("hello"); buf.getvalue()
bytes_buf = io.BytesIO(b"\x00\x01")
```

Note: for CLI parsing, prefer `argparse` (stdlib) or `typer`/`click` (3rd party). Don't roll your own from `sys.argv`.

## Practical Application — a daily-rotating log analyzer

Combines `pathlib`, `Counter`, `datetime`, `json`, `logging`, and `subprocess`:

```python
import json
import logging
import subprocess
from collections import Counter
from datetime import datetime, UTC
from pathlib import Path

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)-7s %(name)s: %(message)s",
)
log = logging.getLogger(__name__)

LOG_DIR = Path.home() / "logs"
OUT_DIR = Path.home() / "reports"

def gather_log_files(days: int = 1) -> list[Path]:
    cutoff = datetime.now(UTC).timestamp() - days * 86400
    files = [p for p in LOG_DIR.rglob("*.log") if p.stat().st_mtime >= cutoff]
    log.info("found %d log files in last %d day(s)", len(files), days)
    return files

def grep_errors(paths: list[Path]) -> list[str]:
    if not paths:
        return []
    r = subprocess.run(
        ["grep", "-h", "ERROR", *(str(p) for p in paths)],
        capture_output=True, text=True, check=False,
    )
    return [line for line in r.stdout.splitlines() if line]

def summarize(lines: list[str]) -> dict:
    counts = Counter(line.split(" ", 3)[-1] for line in lines)
    return {
        "generated_at": datetime.now(UTC).isoformat(),
        "total_errors": len(lines),
        "top_errors": counts.most_common(5),
    }

def main() -> None:
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    files = gather_log_files(days=1)
    errors = grep_errors(files)
    summary = summarize(errors)
    out = OUT_DIR / f"errors-{datetime.now(UTC):%Y%m%d}.json"
    out.write_text(json.dumps(summary, indent=2))
    log.info("wrote summary to %s (%d errors)", out, summary["total_errors"])

if __name__ == "__main__":
    main()
```

This is the kind of script every team has — and it's all stdlib, no dependencies.

## 5. Common Mistakes & Gotchas

- **`os.path.join` and string concatenation for paths.** Use `pathlib.Path`. It works on Windows. It composes. It's 2026.
- **Naive datetimes in production.** Sooner or later DST will burn you. Always aware-UTC.
- **`json.dumps` failing on `datetime`/`Decimal`/custom objects.** Pass a `default=` function or use `pydantic`/`orjson`.
- **`subprocess.run(cmd, shell=True)` with unsanitized input.** RCE waiting to happen.
- **`print()` for diagnostics in libraries.** Use `logging`. `print` to stderr at minimum, never stdout from a library.
- **`logging.basicConfig` from a library.** It only takes effect once and steals the user's config. Libraries get a logger and emit; the application configures.
- **Building `Counter` from huge generators when you don't need the full count.** For top-k, `heapq.nlargest` over a generator is lighter.
- **Forgetting `text=True` in `subprocess`.** You get bytes and confusing decode errors.
- **Using `os.system`.** Old, no error capture, shell-quoting nightmare. Forget it exists.
- **`from os import *` or `from datetime import *`.** Shadows `time`, `path`, etc. Always import the module or specific names.

## 🎯 Key Takeaways

- **`pathlib` is the new `os.path`.** Cleaner, OS-aware, composable. Convert old code as you touch it.
- **Always log; never `print` in libraries or services.** Set up `logging.getLogger(__name__)` per module and let the application choose the handlers and level.
- **All datetimes in your code are aware-UTC.** Convert at the I/O boundary (display, storage formats that demand otherwise). Saves real outages.
- **`subprocess.run([...], check=True, text=True, timeout=...)` is the safe call shape.** Memorize it. Never use `shell=True` with user input.
- **`collections` covers half your "I need a smarter dict" cases.** `Counter`, `defaultdict`, `deque` should be reflexes.

*← [prev](./09_typing.md) | [next →](./11_file_io_and_serialization.md)*
