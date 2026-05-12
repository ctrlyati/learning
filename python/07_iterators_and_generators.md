# 07 — Iterators, Generators, itertools
> **Goal:** Understand the iterator protocol that powers half the language, then use generators and `itertools` to write fast, memory-light data pipelines.

## 1. The iterator protocol — what `for` actually does

Everything that supports `for x in ...` implements the **iterator protocol**: two dunders, `__iter__` and `__next__`.

- `iter(obj)` calls `obj.__iter__()` and returns an *iterator*.
- `next(it)` calls `it.__next__()`, which returns the next item or raises `StopIteration` when done.

`for x in xs:` is sugar for:

```python
it = iter(xs)
while True:
    try:
        x = next(it)
    except StopIteration:
        break
    # loop body
```

A simple iterator written by hand:

```python
class CountDown:
    def __init__(self, start: int) -> None:
        self.n = start
    def __iter__(self):
        return self           # an iterator returns itself
    def __next__(self):
        if self.n <= 0:
            raise StopIteration
        self.n -= 1
        return self.n + 1

list(CountDown(3))    # [3, 2, 1]
```

Distinguish **iterable** (has `__iter__`) from **iterator** (has `__next__`, advances on each call). A list is iterable; `iter(list)` gives you a fresh iterator. Iterators are one-shot — once exhausted, done.

```python
xs = [1, 2, 3]
it = iter(xs)
next(it), next(it), next(it)    # 1, 2, 3
next(it)                         # StopIteration
```

## 2. Generators — the easy way to write iterators

Writing a class with `__iter__`/`__next__` is verbose. A **generator function** uses `yield` and Python builds the iterator for you:

```python
def count_down(start: int):
    while start > 0:
        yield start
        start -= 1

list(count_down(3))    # [3, 2, 1]
```

Each `yield` pauses the function, returning the value. The next `next()` call resumes from where it left off, with all local state intact. When the function returns, `StopIteration` is raised.

### Why generators matter — they're lazy

```python
def lines_of(path):
    with open(path) as f:
        for line in f:
            yield line.rstrip()

# Process a 10GB file using almost no memory:
for line in lines_of("huge.log"):
    if "ERROR" in line:
        print(line)
```

A list comprehension would load the entire file. The generator streams it line by line.

### Generator expressions

Same as list comprehensions but with `()`:

```python
squares_gen = (x * x for x in range(10**8))    # nothing computed yet
total = sum(squares_gen)                        # streams, no allocation
```

If a generator expression is the sole argument to a function, you can drop the parens:

```python
sum(x * x for x in range(100))
max(len(line) for line in lines_of("file.txt"))
any(x < 0 for x in xs)
```

### Generator pipelines

Generators chain like Unix pipes. Each stage is lazy:

```python
def read(path):
    with open(path) as f:
        for line in f: yield line

def parse(lines):
    for line in lines:
        parts = line.split(",")
        if len(parts) == 3:
            yield {"ts": parts[0], "level": parts[1], "msg": parts[2].strip()}

def errors_only(records):
    for r in records:
        if r["level"] == "ERROR":
            yield r

pipeline = errors_only(parse(read("app.log")))
for err in pipeline:
    print(err)
```

Memory usage is one line at a time, regardless of file size. This pattern scales to gigabytes.

### `yield from` — delegating to another generator

```python
def flatten(nested):
    for item in nested:
        if isinstance(item, list):
            yield from flatten(item)     # recurse, propagate values
        else:
            yield item

list(flatten([1, [2, [3, 4]], 5]))     # [1, 2, 3, 4, 5]
```

`yield from x` is roughly `for item in x: yield item`, but also forwards `send()`, `throw()`, and the return value — the foundation of how `async` was built before native syntax existed.

## 3. itertools — the toolbox you should memorize

`itertools` is the most underused stdlib module. Lazy, composable, written in C.

### Infinite iterators

```python
from itertools import count, cycle, repeat

# count(start=0, step=1): 0, 1, 2, 3, ...
for i, x in zip(count(1), data):
    print(i, x)

# cycle('AB'): A, B, A, B, ...
# repeat(value, n): same value n times (or forever if n omitted)
```

### Combining iterables

```python
from itertools import chain, zip_longest

list(chain([1, 2], [3, 4], [5]))            # [1, 2, 3, 4, 5]
list(zip_longest([1, 2, 3], ['a', 'b'], fillvalue=None))
# [(1, 'a'), (2, 'b'), (3, None)]
```

### Slicing and grouping

```python
from itertools import islice, takewhile, dropwhile, groupby, batched
# batched is 3.12+ — replaces a recipe you'd write by hand

list(islice(count(), 2, 10, 2))               # [2, 4, 6, 8]
list(takewhile(lambda x: x < 5, [1, 3, 5, 2]))  # [1, 3]
list(batched("ABCDEFG", 3))                   # [('A','B','C'), ('D','E','F'), ('G',)]

# groupby groups *consecutive* equal elements — sort first if needed
data = [("a", 1), ("a", 2), ("b", 3), ("a", 4)]
for key, group in groupby(data, key=lambda x: x[0]):
    print(key, list(group))
# a [('a', 1), ('a', 2)]
# b [('b', 3)]
# a [('a', 4)]
```

### Combinatorics

```python
from itertools import product, permutations, combinations

list(product("AB", repeat=2))            # AA AB BA BB
list(permutations("ABC", 2))             # AB AC BA BC CA CB
list(combinations("ABC", 2))             # AB AC BC
```

### `itertools.accumulate` — running totals

```python
from itertools import accumulate
list(accumulate([1, 2, 3, 4]))           # [1, 3, 6, 10]
list(accumulate([1, 2, 3, 4], max))      # [1, 2, 3, 4] — running max
```

## 4. Practical Application — a memory-efficient CSV analyzer

Process a large CSV without loading it into memory. Compute per-category running totals and top-N rows by value:

```python
import csv
import heapq
from itertools import groupby
from operator import itemgetter
from pathlib import Path

def read_rows(path: Path):
    with path.open(newline="") as f:
        reader = csv.DictReader(f)
        for row in reader:
            yield {
                "category": row["category"],
                "amount":   float(row["amount"]),
                "date":     row["date"],
            }

def by_category_totals(rows):
    # rows must be sorted by category for groupby
    sorted_rows = sorted(rows, key=itemgetter("category"))
    for cat, group in groupby(sorted_rows, key=itemgetter("category")):
        yield cat, sum(r["amount"] for r in group)

def top_n_by_amount(rows, n: int):
    # heapq.nlargest streams — doesn't need everything in memory
    return heapq.nlargest(n, rows, key=itemgetter("amount"))


path = Path("transactions.csv")

# Two passes (each a fresh generator) — totals and top 5
print("category totals:")
for cat, total in by_category_totals(read_rows(path)):
    print(f"  {cat:<12} {total:>10.2f}")

print("\ntop 5 transactions:")
for r in top_n_by_amount(read_rows(path), 5):
    print(f"  {r['date']}  {r['category']:<12} {r['amount']:>10.2f}")
```

Key wins:

- `read_rows` yields dicts; the file is never fully loaded.
- `heapq.nlargest(5, generator)` walks the whole stream but only ever holds 5 items.
- We call `read_rows(path)` twice — generators are *one-shot*, so a second pass needs a fresh generator. That's a common gotcha.

For datasets that exceed memory, this pattern (generators + `itertools` + `heapq`) is how seniors solve problems before reaching for pandas.

## 5. Common Mistakes & Gotchas

- **Iterators are one-shot.** Reading once exhausts them. If you need to iterate twice, make it a list or rebuild the generator.
- **`list(gen)` on an infinite generator hangs.** `count()`, `cycle()`, `repeat()` produce values forever. Slice with `islice`.
- **`groupby` doesn't sort.** It groups *consecutive* equal items. Sort by the key first or you'll get fragmented groups.
- **Returning early inside a `with` in a generator.** The `with` only closes when the generator is fully exhausted or garbage-collected. Usually fine, but be aware for file handles in long-lived generators.
- **`yield` inside a `try/finally` and partial consumption.** The `finally` runs on GC, which is non-deterministic on PyPy and slightly delayed on CPython. For deterministic cleanup, use `with` inside the generator and consume it inside its own `with`/`contextlib.closing`.
- **Confusing iterator and iterable.** `list(iter(xs))` is fine; `list(iter(iter(xs)))` works but `iter(it)` on an iterator just returns it.
- **Building lists when a generator would do.** `sum([x * x for x in big])` allocates; `sum(x * x for x in big)` doesn't.
- **Forgetting `chain.from_iterable` for nested iterables.** `chain(*lists)` works but eagerly expands. `chain.from_iterable(lists)` is lazy.

## 🎯 Key Takeaways

- **The iterator protocol is the spine of Python.** Once you see `for`, comprehensions, generators, file objects, and `itertools` all as the same protocol, the language gets smaller.
- **Generators turn pipelines from a memory problem into a streaming problem.** This is the single biggest Python performance idiom for data-shaped work — well before reaching for numpy or pandas.
- **`itertools` is the standard library's most reused, least known module.** Memorize `chain`, `islice`, `groupby`, `batched`, `accumulate`, `product`. Reach for them before writing loops.
- **`heapq.nlargest`/`nsmallest` on a generator** is the go-to top-N pattern — O(n log k) memory and time, no sort.
- **One-shot iteration trips everyone up.** If a downstream stage needs a second pass, materialize once (`list(gen)`) or pass a generator factory (function that returns a fresh generator).

*← [prev](./06_oop_and_dataclasses.md) | [next →](./08_errors_and_context.md)*
