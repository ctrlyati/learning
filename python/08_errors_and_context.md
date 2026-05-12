# 08 — Errors & Context Managers
> **Goal:** Use exceptions the Pythonic way (EAFP), design useful custom exceptions, and write context managers that guarantee cleanup.

## 1. Exceptions — control flow that says "I can't"

Exceptions are how Python signals "I cannot complete this operation." They unwind the stack until something catches them.

```python
def divide(a, b):
    return a / b

try:
    divide(10, 0)
except ZeroDivisionError as e:
    print(f"oops: {e}")
```

Mental model: an exception is an *object* (subclass of `BaseException`) that gets raised, propagated up the call stack, and either caught by an `except` or terminates the program with a traceback.

### EAFP, not LBYL

Python prefers **EAFP** — "Easier to Ask Forgiveness than Permission." Try the operation, handle failure:

```python
# LBYL — un-Pythonic, race-conditional
if "key" in d and isinstance(d["key"], str) and len(d["key"]) > 0:
    use(d["key"])

# EAFP — idiomatic
try:
    use(d["key"])
except KeyError:
    handle_missing()
```

LBYL is brittle (the conditions can drift from reality, especially with files or network) and verbose. EAFP makes the happy path obvious. Use LBYL only for cheap checks that prevent expensive failures.

## 2. Mechanism — the exception hierarchy and how to catch

### The hierarchy (abridged)

```
BaseException
 ├── SystemExit
 ├── KeyboardInterrupt
 ├── GeneratorExit
 └── Exception
      ├── ArithmeticError → ZeroDivisionError
      ├── LookupError → KeyError, IndexError
      ├── OSError → FileNotFoundError, PermissionError, ConnectionError
      ├── ValueError
      ├── TypeError
      ├── RuntimeError
      └── ... and many more
```

Two rules:

1. **Catch `Exception`, not `BaseException`.** The latter swallows `KeyboardInterrupt` and `SystemExit` — you'll wonder why Ctrl-C doesn't work.
2. **Catch the most specific exception that makes sense.** Bare `except:` is forbidden by linters; `except Exception:` is the broadest you should ever go, and only at the top of a service.

### `try` / `except` / `else` / `finally`

```python
try:
    data = parse(payload)
except ValueError as e:
    log.warning("bad payload: %s", e)
    data = default
else:
    log.info("parsed ok")        # only runs if no exception
finally:
    cleanup()                     # always runs — even if try returns or raises
```

`else` runs when the `try` block finished without raising — useful to keep `try` minimal (only the call that might fail) while having follow-up that should *not* be caught.

### `raise` and `raise ... from ...`

```python
def get_user(user_id):
    try:
        return db.fetch(user_id)
    except DBError as e:
        raise UserNotFoundError(user_id) from e   # preserves cause chain
```

`raise X from Y` chains exceptions: the traceback shows "the above exception was the direct cause of the following exception." Use it whenever you re-raise as a different type so debuggers can see the original failure.

`raise X from None` suppresses the chain — use sparingly when the inner exception leaks implementation details (e.g., a JSON parse error from a "user not found" code path).

### Custom exceptions

Define a small hierarchy for your domain:

```python
class AppError(Exception):
    """Base for all application errors."""

class ValidationError(AppError):
    pass

class NotFoundError(AppError):
    def __init__(self, kind: str, id: object) -> None:
        super().__init__(f"{kind} not found: {id!r}")
        self.kind = kind
        self.id = id
```

Why a base class? Callers can `except AppError:` to catch any of yours without catching unrelated `ValueError`s from a third-party library. Worth doing on day one.

### Exception groups (3.11+)

For things that legitimately fail in parallel (concurrent tasks, batch validation):

```python
errors = [ValidationError("bad email"), ValidationError("missing name")]
raise ExceptionGroup("validation failed", errors)

# caller:
try:
    validate(payload)
except* ValidationError as eg:    # except* matches inside the group
    for e in eg.exceptions:
        print(e)
```

Mostly relevant in `asyncio` (TaskGroup raises ExceptionGroup) and validation libraries.

## 3. Variations — context managers and `contextlib`

### The `with` statement

A context manager guarantees setup and teardown, even on exception:

```python
with open("file.txt") as f:
    data = f.read()
# file is closed here, whether read() raised or not
```

`with X as y:` is roughly:

```python
mgr = X
y = mgr.__enter__()
try:
    # body
finally:
    mgr.__exit__(exc_type, exc_value, traceback)
```

If `__exit__` returns truthy, the exception is suppressed. Almost always return `None` (implicit) to let it propagate.

### Multiple context managers

```python
with open("in.txt") as src, open("out.txt", "w") as dst:
    dst.write(src.read())

# Parenthesized form (3.10+) for long lists:
with (
    open("a") as a,
    open("b") as b,
    open("c") as c,
):
    ...
```

### Writing one with `contextlib.contextmanager`

The decorator turns a generator into a context manager. Code before `yield` is `__enter__`; code after is `__exit__`:

```python
from contextlib import contextmanager
import time

@contextmanager
def timed(label: str):
    start = time.perf_counter()
    try:
        yield                                   # control returns to the with-body
    finally:
        elapsed = time.perf_counter() - start
        print(f"{label}: {elapsed*1000:.1f}ms")

with timed("load"):
    big_compute()
```

The `try/finally` is critical — without it, an exception in the body would skip the cleanup.

### Useful `contextlib` helpers

```python
from contextlib import suppress, ExitStack, closing, redirect_stdout, nullcontext
import io, os

# Silence a specific exception cleanly
with suppress(FileNotFoundError):
    os.remove("maybe-exists.txt")

# Manage a dynamic number of context managers
with ExitStack() as stack:
    files = [stack.enter_context(open(p)) for p in paths]
    process(files)        # all closed at exit, even on exception

# Capture stdout
buf = io.StringIO()
with redirect_stdout(buf):
    print("hidden")
captured = buf.getvalue()

# Conditional context — useful when a manager may or may not be needed
ctx = lock if needs_lock else nullcontext()
with ctx:
    ...
```

`ExitStack` is the answer to "I have N things to clean up and I don't know N at compile time."

### Class-based context manager

For more state or reuse:

```python
class Transaction:
    def __init__(self, conn): self.conn = conn
    def __enter__(self):
        self.conn.execute("BEGIN")
        return self
    def __exit__(self, exc_type, exc, tb):
        if exc_type is None:
            self.conn.execute("COMMIT")
        else:
            self.conn.execute("ROLLBACK")
        # return None → propagate any exception

with Transaction(conn) as tx:
    tx.conn.execute("INSERT ...")
```

## 4. Practical Application — a robust file processor

Combines custom exceptions, `try/except/else/finally`, a context manager for atomic writes, and `ExitStack`:

```python
from contextlib import contextmanager, ExitStack
from pathlib import Path
import json
import os
import tempfile

class ProcessingError(Exception):
    """Base for all processing errors."""

class InvalidRecord(ProcessingError):
    def __init__(self, line_no: int, reason: str) -> None:
        super().__init__(f"line {line_no}: {reason}")
        self.line_no = line_no

@contextmanager
def atomic_write(path: Path):
    """Write to a temp file, rename on success — never leaves a half-written file."""
    tmp = path.with_suffix(path.suffix + ".tmp")
    f = tmp.open("w", encoding="utf-8")
    try:
        yield f
        f.flush()
        os.fsync(f.fileno())
    except Exception:
        f.close()
        tmp.unlink(missing_ok=True)
        raise
    else:
        f.close()
        tmp.replace(path)         # atomic on POSIX & modern Windows

def process(input_paths: list[Path], output_path: Path) -> int:
    written = 0
    with ExitStack() as stack:
        sources = [stack.enter_context(p.open()) for p in input_paths]
        out = stack.enter_context(atomic_write(output_path))
        for src, src_path in zip(sources, input_paths):
            for i, line in enumerate(src, 1):
                line = line.strip()
                if not line:
                    continue
                try:
                    record = json.loads(line)
                except json.JSONDecodeError as e:
                    raise InvalidRecord(i, str(e)) from e
                if "id" not in record:
                    raise InvalidRecord(i, "missing 'id'")
                out.write(json.dumps(record) + "\n")
                written += 1
    return written

# Usage
try:
    n = process([Path("a.jsonl"), Path("b.jsonl")], Path("merged.jsonl"))
    print(f"wrote {n} records")
except InvalidRecord as e:
    print(f"validation failed: {e}")
except OSError as e:
    print(f"I/O error: {e}")
```

What's idiomatic here:

- `atomic_write` is the canonical "write to temp, rename" pattern. Without it, a crash mid-write leaves corrupted output.
- `ExitStack` opens N input files plus the output — all guaranteed to close.
- `raise InvalidRecord(...) from e` preserves the original `JSONDecodeError` for debugging.
- Custom hierarchy means callers can catch `ProcessingError` for "domain failure" or `OSError` for "infra failure" — different responses.

## 5. Common Mistakes & Gotchas

- **Bare `except:` or `except Exception:` everywhere.** Hides bugs. Catch what you handle; let the rest crash so you see them.
- **Catching `BaseException`.** Eats `KeyboardInterrupt`. If you must, re-raise in the catch block.
- **`except ... pass`.** Sometimes correct (with a comment), usually a code smell. Log at minimum.
- **Forgetting `from e` when re-raising.** Loses the chain; future-you debugging at 2am hates past-you.
- **Using exceptions for normal control flow at hot-path scale.** Exceptions are cheap *to raise* but not free. In a tight loop, a sentinel return or `dict.get(k, default)` is faster than `try/except KeyError`.
- **`finally` that returns a value.** Swallows exceptions silently:
  ```python
  try: raise ValueError
  finally: return 1     # ValueError is lost
  ```
- **Long `try` blocks.** Wrap only the line that can fail. Everything else moves to `else`. This narrows what's caught and clarifies intent.
- **Forgetting `__enter__` returns the value bound by `as`.** A common bug in custom managers — returning `None` and being surprised `as x` is `None`.
- **Assuming `with open(...)` flushes on exception.** It does close — but on POSIX, "close" doesn't guarantee data on disk without `os.fsync`. Use atomic writes for durability.

## 🎯 Key Takeaways

- **EAFP is the Python style.** `try/except` reads cleaner than nested `if`s and avoids race conditions with files and network.
- **Always raise specific exception types and chain with `from e`.** Diagnostic gold; trivial to do; constantly skipped.
- **Build a small custom exception hierarchy rooted in `AppError`** for any non-trivial codebase. Lets callers catch your errors without catching unrelated ones.
- **`with` is mandatory for resources.** Files, locks, DB connections, sockets — never manage cleanup by hand. `contextlib.contextmanager` makes writing your own a five-line job.
- **`ExitStack` and `suppress` are the two `contextlib` tools most people don't know they need.** Dynamic resource lists and "ignore this specific error" become one-liners.

*← [prev](./07_iterators_and_generators.md) | [next →](./09_typing.md)*
