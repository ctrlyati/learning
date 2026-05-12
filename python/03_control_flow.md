# 03 — Control Flow & Comprehensions
> **Goal:** Write loops, branches, and `match` statements the way a senior would — and replace most of them with comprehensions.

## 1. Branching — `if`, `match`, and the truthiness shortcut

### `if` / `elif` / `else`

No parentheses, no braces, indentation is the syntax:

```python
def classify(n: int) -> str:
    if n < 0:
        return "negative"
    elif n == 0:
        return "zero"
    else:
        return "positive"
```

Conditional expression (Python's ternary) — readable when short:

```python
status = "adult" if age >= 18 else "minor"
```

### `match` statement (3.10+) — structural pattern matching

Not just a `switch`. It destructures.

```python
def describe(point: tuple[int, int]) -> str:
    match point:
        case (0, 0):
            return "origin"
        case (0, y):
            return f"on y-axis at {y}"
        case (x, 0):
            return f"on x-axis at {x}"
        case (x, y) if x == y:
            return f"on diagonal at {x}"
        case (x, y):
            return f"at {x},{y}"
        case _:
            return "not a point"
```

Patterns can match types, dict shapes, class instances, and bind variables:

```python
def handle(event: dict) -> None:
    match event:
        case {"type": "click", "x": int(x), "y": int(y)}:
            print(f"click at {x},{y}")
        case {"type": "key", "key": str(k)} if k.isalpha():
            print(f"letter {k}")
        case {"type": t}:
            print(f"unknown event type {t}")
```

`match` shines when you'd otherwise write `isinstance` ladders. For simple value dispatch, plain `if/elif` is fine.

## 2. Looping — `for`, `while`, `enumerate`, `zip`

### `for` is for iteration, not counting

Python's `for` loops over iterables, not indices:

```python
for fruit in ["apple", "banana", "cherry"]:
    print(fruit)

# Need indices? Use enumerate.
for i, fruit in enumerate(fruits, start=1):
    print(f"{i}. {fruit}")

# Need parallel iteration? zip.
for name, score in zip(names, scores, strict=True):
    print(f"{name}: {score}")

# strict=True (3.10+) raises if lengths differ — catch bugs early.
```

`range(stop)`, `range(start, stop)`, `range(start, stop, step)` produce lazy integer sequences:

```python
for i in range(0, 10, 2):    # 0, 2, 4, 6, 8
    ...
```

### `while` — for "until a condition" loops

```python
while not done:
    work()

# Walrus makes "read until sentinel" clean (PEP 572)
while (chunk := f.read(4096)):
    process(chunk)
```

### `break`, `continue`, and the unloved `for/else`

`break` exits the loop; `continue` skips to next iteration. The `else` clause on a loop runs *if the loop completed without break* — useful for search:

```python
for x in xs:
    if predicate(x):
        result = x
        break
else:
    result = None    # nothing matched
```

Many seniors avoid `for/else` because it's misread. Acceptable, but a comment helps.

## 3. Comprehensions — replace 80% of your loops

A comprehension is "build a collection from an iterable, optionally filtered, optionally transformed." It's the Python idiom — using a `for` loop with `.append()` is a code smell when a comprehension fits.

```python
# List
squares = [x * x for x in range(10)]
evens   = [x for x in xs if x % 2 == 0]
flat    = [item for row in matrix for item in row]    # nested

# Set — dedupes
domains = {email.split("@")[1] for email in emails}

# Dict
by_id = {user.id: user for user in users}
inverted = {v: k for k, v in mapping.items()}

# Generator expression — lazy, no parens needed inside a call
total = sum(x * x for x in xs)
any(x < 0 for x in xs)
```

The pattern is `[expr for var in iter if cond]`. Multiple `for`s nest left-to-right (outer first). Comprehensions are faster than equivalent `for`/`append` loops because the bytecode is specialized.

**When NOT to use a comprehension:** if the body has side effects, multiple statements, or grows beyond ~80 chars/two clauses, write a normal loop. Readability wins.

```python
# Bad: comprehension abused for side effects
[print(x) for x in xs]      # builds a list of None and discards it

# Good
for x in xs:
    print(x)
```

### Generator expressions vs list comprehensions

Same syntax, different brackets. List builds the whole list in memory; generator yields one at a time:

```python
sum([x * x for x in range(10**8)])    # allocates 100M ints — bad
sum(x * x for x in range(10**8))      # streams — fine
```

Rule of thumb: if you're feeding the result into another function (`sum`, `max`, `any`, `"".join`), use a generator expression. If you'll iterate twice or index into it, use a list.

## 4. Practical Application — a log analyzer

Combines all of this on a realistic input:

```python
from collections import Counter

logs = [
    "2026-05-11 12:01 INFO  user=alice action=login",
    "2026-05-11 12:02 ERROR user=bob action=login msg=bad-password",
    "2026-05-11 12:03 INFO  user=alice action=view path=/home",
    "2026-05-11 12:04 WARN  user=carol action=login",
    "2026-05-11 12:05 ERROR user=bob action=login msg=bad-password",
]

def parse(line: str) -> dict[str, str]:
    parts = line.split(maxsplit=3)
    record = {"date": parts[0], "time": parts[1], "level": parts[2]}
    for kv in parts[3].split():
        if "=" in kv:
            k, v = kv.split("=", 1)
            record[k] = v
    return record

records = [parse(line) for line in logs]

# Filter + project with comprehensions
errors = [r for r in records if r["level"] == "ERROR"]
users  = {r["user"] for r in records}

# Count with Counter (built on dict)
action_counts = Counter(r["action"] for r in records)

# match for dispatch
def summarize(r: dict[str, str]) -> str:
    match r:
        case {"level": "ERROR", "user": u, "msg": m}:
            return f"!! {u} failed: {m}"
        case {"level": "WARN", "user": u}:
            return f" ? {u} warning"
        case {"level": "INFO", "user": u, "action": a}:
            return f" . {u} {a}"
        case _:
            return "?? unknown"

for r in records:
    print(summarize(r))

print(f"\n{len(errors)} errors across {len(users)} users")
print(f"actions: {dict(action_counts)}")
```

Walk through what's happening:

- `parse` returns a dict — the natural Python record type.
- `records = [parse(line) for line in logs]` is the right way to transform a sequence.
- `errors` and `users` are derived with comprehensions. `users` is a set so it dedupes for free.
- `Counter` is a `dict` subclass that counts hashable items.
- `match` cleanly dispatches on log level and presence of fields.

## 5. Common Mistakes & Gotchas

- **`for i in range(len(xs)):`** — un-Pythonic. Use `for x in xs` or `for i, x in enumerate(xs)`.
- **Mutating while iterating.** `for x in xs: if cond: xs.remove(x)` is buggy. Filter into a new list, or iterate over `xs[:]`.
- **List comprehension for side effects.** Builds a throwaway list. Use a real `for` loop.
- **`if x == True:`** — write `if x:`. Same for `if x == None:` → `if x is None:`.
- **Forgetting `zip`'s short-circuit.** Default `zip` stops at the shortest iterable silently. Use `strict=True` (3.10+) or `itertools.zip_longest` if lengths must match.
- **`match` without `case _`.** No `default` is fine if you've covered every shape, but partial matches just fall through silently. Add `case _:` and raise if "shouldn't happen."
- **Using `match` as a glorified `if`.** For `if x == 1: ... elif x == 2: ...`, plain `if/elif` is clearer. Use `match` when destructuring.

## 🎯 Key Takeaways

- **Iterate values, not indices.** `for x in xs`, `enumerate`, `zip(strict=True)` — these are the Pythonic shapes. Reviewers spot `range(len(...))` instantly.
- **Comprehensions are the loop you should reach for first.** They're faster, more declarative, and signal "I'm transforming a collection." Keep them one-liner; if they grow, fall back to a real loop.
- **Generator expressions stream; list comprehensions allocate.** Pick based on whether you need the full collection. Critical for memory.
- **`match` is for shape dispatch (dicts, tuples, classes), not value comparison.** Used right, it replaces ugly `isinstance` ladders. Used wrong, it's a verbose `if`.
- **Walrus (`:=`) earns its keep in `while` loops and filtering comprehensions.** Don't sprinkle it elsewhere — it hurts readability.

*← [prev](./02_data_types.md) | [next →](./04_functions.md)*
