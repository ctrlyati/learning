# 15 — Performance: Profiling, Caching, Vectorization
> **Goal:** Find the slow part with profilers, then speed it up with the right tool — caching, better data structures, numpy, or a C extension.

## 1. The performance mantra — measure, don't guess

> "Premature optimization is the root of all evil." — Knuth

Three rules every senior follows:

1. **Profile before optimizing.** Intuition about Python performance is wrong ~70% of the time.
2. **Algorithm beats micro-optimization.** O(n²) → O(n log n) trumps any constant-factor tweak.
3. **The hot path is rarely where you think.** Usually it's a `dict` lookup in an unexpected loop, an N+1 query, or string concatenation.

The order of escalation:

1. Better algorithm or data structure.
2. Caching (`functools.lru_cache`, `cached_property`).
3. Vectorization (numpy/polars).
4. Concurrency (module 12).
5. Cython/numba/Rust extension.
6. Rewrite the service in another language.

You almost never get past step 3.

## 2. Mechanism — profilers and `timeit`

### `cProfile` — function-level profiling

Built into stdlib. Measures call counts and time per function:

```bash
python -m cProfile -o profile.out myscript.py
```

Then explore:

```python
import pstats
p = pstats.Stats("profile.out")
p.sort_stats("cumulative").print_stats(20)
```

Or programmatically:

```python
import cProfile, pstats, io

pr = cProfile.Profile()
pr.enable()
slow_function()
pr.disable()
stats = pstats.Stats(pr).sort_stats("cumulative")
stats.print_stats(20)
```

For visual flame graphs use **snakeviz**:

```bash
pip install snakeviz
snakeviz profile.out
```

### `timeit` — micro-benchmarks

```python
import timeit
timeit.timeit("sum(range(1000))", number=10_000)        # seconds for 10k runs

# Compare alternatives
timeit.timeit("'-'.join(str(i) for i in range(100))", number=10_000)
timeit.timeit("'-'.join(map(str, range(100)))", number=10_000)
```

Run from the shell:

```bash
python -m timeit -n 10000 "sum(range(1000))"
```

Use `timeit` for "is A faster than B?" questions on small snippets. For real workloads, `cProfile` is the right tool.

### Line profiler

Function-level isn't enough? `line_profiler` (3rd party) shows time per *line*:

```bash
pip install line_profiler
```

```python
@profile        # injected by kernprof
def hot():
    ...
```

```bash
kernprof -lv myscript.py
```

### Memory — `tracemalloc`

stdlib. Tracks memory allocations:

```python
import tracemalloc
tracemalloc.start()

run_code()

snapshot = tracemalloc.take_snapshot()
for stat in snapshot.statistics("lineno")[:10]:
    print(stat)
```

For richer analysis, **memory-profiler** or **pyinstrument** (a sampling profiler — faster than cProfile and easier to read).

## 3. Variations — caching, data structures, vectorization, C extensions

### Caching with `functools`

```python
from functools import lru_cache, cache, cached_property

@lru_cache(maxsize=1024)
def expensive(n: int) -> int:
    ...

@cache                      # 3.9+, equivalent to lru_cache(maxsize=None)
def stable(key: str) -> dict:
    return load_from_disk(key)

class Report:
    def __init__(self, rows): self.rows = rows
    @cached_property         # computed once per instance, then cached as attribute
    def total(self) -> float:
        return sum(r.amount for r in self.rows)
```

Rules:

- **Arguments must be hashable.** Lists, dicts, sets won't cache — use tuples and frozensets.
- **Cache invalidation is hard.** `lru_cache.cache_clear()` is the only built-in answer. For TTL or distributed caches, use `cachetools` or Redis.
- **`cached_property` is per-instance.** Replace with a class-level `lru_cache` if you want shared.

Caching turns O(N · cost) into O(N + cost) when calls repeat — order-of-magnitude wins for free.

### Right data structure beats clever code

| Operation | List | Dict / Set | Notes |
|-----------|------|------------|-------|
| Membership (`x in xs`) | O(n) | O(1) | Set/dict is the answer. |
| Lookup by key | O(n) (search) | O(1) | Always dict. |
| Append | O(1) | O(1) | List is fine. |
| Insert at front | O(n) | n/a | Use `deque` (module 10). |
| Sorted insert | O(n) | n/a | Use `bisect` or `SortedList`. |

The single most common Python perf bug:

```python
# O(n²) — `seen` is a list, `in` scans every time
seen = []
for x in items:
    if x not in seen:
        seen.append(x)

# O(n) — set lookup is constant
seen = set()
for x in items:
    if x not in seen:
        seen.add(x)
```

Also memorize:

- **`bisect.insort`** for keeping a list sorted as you go.
- **`heapq`** for priority queue / top-N.
- **`array.array`** for homogeneous numeric data (less overhead than list).
- **`__slots__`** for many-instance classes (module 6).

### Vectorization with numpy

For numeric work, numpy is 10–100× faster than pure-Python loops because the inner loop runs in C and operates on contiguous memory:

```python
import numpy as np

# Slow: Python loop
def normalize_py(xs):
    total = sum(xs)
    return [x / total for x in xs]

# Fast: vectorized
def normalize_np(xs):
    a = np.asarray(xs)
    return a / a.sum()

# For 1M floats, numpy is ~50× faster.
```

Key idea: reach for numpy when you're doing the *same operation* across a *large* collection of numbers. For mixed-type rows or non-numeric data, pandas/polars build on numpy and add labeled columns.

### C extensions and JIT — when Python isn't enough

In escalating effort:

| Tool | When |
|------|------|
| **numpy / polars** | Numeric/tabular hot loops. First reach. |
| **`functools.lru_cache`** | Repeating pure-function calls. |
| **`numba`** | JIT-compile numeric Python with a decorator. Almost free win for math-heavy loops. |
| **Cython** | Compile annotated Python to C. Mature, ugly syntax, good results. |
| **PyO3 + maturin** (Rust) | Modern, safe, fast. The 2020s answer for "rewrite the hot path." |
| **C extension** | Last resort. Lots of footguns (refcounts, GIL handling). |
| **PyPy** (alternative interpreter) | If your code is pure-Python and CPU-bound, can be 5–10× faster. Dependency compatibility is the catch. |

Modern reach order: numpy → numba → PyO3. Skip Cython unless you inherit it.

## 4. Practical Application — profiling and optimizing a slow function

A realistic example: a function that ranks log entries by frequency. Naive version is O(n²); the optimization shows the workflow.

```python
import cProfile, pstats
import io

# Slow version — O(n²): list lookups + repeated sort
def top_k_slow(events: list[str], k: int) -> list[tuple[str, int]]:
    keys = []
    counts = []
    for e in events:
        if e in keys:                        # O(n)
            counts[keys.index(e)] += 1       # another O(n)
        else:
            keys.append(e)
            counts.append(1)
    pairs = list(zip(keys, counts))
    pairs.sort(key=lambda kv: -kv[1])         # O(n log n)
    return pairs[:k]

# Profile
events = ["a", "b", "a", "c", "a", "b", "d"] * 10_000
pr = cProfile.Profile()
pr.enable()
top_k_slow(events, 3)
pr.disable()
pstats.Stats(pr).sort_stats("cumulative").print_stats(10)
# top_k_slow takes ~seconds, with most time in .index() and `in`
```

Optimized:

```python
from collections import Counter
import heapq

# Fast — O(n) counting + O(n log k) for top-k
def top_k_fast(events: list[str], k: int) -> list[tuple[str, int]]:
    counts = Counter(events)
    return counts.most_common(k)
    # Counter.most_common uses heapq.nlargest under the hood for small k

# Or with heapq directly when you have a streaming generator
def top_k_streaming(events_iter, k):
    counts = Counter()
    for e in events_iter:
        counts[e] += 1
    return heapq.nlargest(k, counts.items(), key=lambda kv: kv[1])
```

Verify with `timeit`:

```python
import timeit
events = ["a", "b", "a", "c", "a", "b", "d"] * 10_000

t_slow = timeit.timeit(lambda: top_k_slow(events, 3), number=5)
t_fast = timeit.timeit(lambda: top_k_fast(events, 3), number=5)
print(f"slow: {t_slow:.3f}s   fast: {t_fast:.3f}s   speedup: {t_slow/t_fast:.1f}x")
```

Typical result: **100–1000× speedup**, just from the right data structure. No C, no async, no clever tricks. This is the highest-ROI optimization most code needs.

Now layer caching for repeat-call workloads:

```python
from functools import lru_cache

@lru_cache(maxsize=128)
def parse_event(line: str) -> dict:
    # Pretend this is expensive (regex, JSON, whatever)
    parts = line.split("|")
    return {"ts": parts[0], "level": parts[1], "msg": parts[2]}
```

If the same lines repeat, second-and-later calls are free.

## 5. Common Mistakes & Gotchas

- **Optimizing without profiling.** You'll spend a day shaving 5% off a function that takes 0.1% of total time.
- **List for membership tests.** O(n) per check, O(n²) total in a loop. Use a set.
- **String concatenation in a loop with `+`.** Quadratic. Build a list and `"".join(parts)`.
- **`functools.lru_cache` on a method.** It caches `(self, ...)` — `self` is part of the key, so memory grows with instances; and unhashable instances fail. Use `cached_property` or move to a free function.
- **Caching unhashable arguments.** `lru_cache` raises `TypeError` for lists/dicts. Pass tuples/frozensets.
- **`pandas` for tiny datasets.** Pandas overhead is significant; for a few hundred rows, plain dicts are faster.
- **numpy for non-numeric data.** Object arrays defeat the point. Stick to native dtypes.
- **Premature `__slots__` / Cython / numba.** All have costs (introspection, build complexity). Earn them by profiling first.
- **Concurrency without measuring.** Threading something that's already fast adds overhead. Benchmark serial first.
- **Forgetting `os.fsync` / `db.commit()` are expensive.** Batching writes is a huge win — reviewers always notice "one commit per row" loops.
- **Hot loops printing or logging.** I/O is slow. Buffer or downgrade to debug.

## 🎯 Key Takeaways

- **Profile first; optimize second.** `cProfile` + `snakeviz` (or pyinstrument) is the workflow. Intuition lies; profilers don't.
- **Most "Python is slow" bugs are algorithm/data-structure bugs.** A `set` instead of a `list` for membership; a `Counter` instead of a hand-rolled dict; `heapq.nlargest` instead of full sort. Internalize the table in section 3.
- **`functools.lru_cache` and `cached_property` are nearly-free wins** for any pure function or computed attribute that gets called repeatedly.
- **For numeric hot loops, vectorize with numpy.** For everything else, ask whether you really need C — usually you don't.
- **Cython/numba/Rust are the right answer ~5% of the time.** When you do reach for them, prefer numba (zero-rewrite) or PyO3 (modern, safe, fast). Skip raw C extensions unless you have to.

*← [prev](./14_packaging.md) | [next →](./16_web_and_apis.md)*
