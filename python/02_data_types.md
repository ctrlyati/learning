# 02 — Data Types & Operators
> **Goal:** Know Python's built-in types cold — what they store, when they're mutable, and which operator surprises bite people.

## 1. Everything is an object — the unifying mental model

In Python, `1`, `"hi"`, `[1,2,3]`, the function `print`, and the class `int` are *all* objects with a type and attributes. There is no primitive/object split like Java's `int` vs `Integer`. This sounds abstract but pays off constantly:

```python
>>> (42).bit_length()       # int has methods
6
>>> "hi".upper()            # str has methods
'HI'
>>> type(print)             # functions are objects
<class 'builtin_function_or_method'>
>>> type(int)               # classes are objects too
<class 'type'>
```

Two questions to ask of any value:

1. **What's its type?** `type(x)` or `isinstance(x, T)`.
2. **Is it mutable?** Mutable objects can change in place; immutable ones can only be replaced.

| Type    | Mutable? | Literal       | Notes                              |
|---------|----------|---------------|------------------------------------|
| `int`   | no       | `42`, `0xff`  | arbitrary precision, no overflow   |
| `float` | no       | `3.14`, `1e9` | IEEE 754 double                    |
| `bool`  | no       | `True`/`False`| subclass of `int` (yes, really)    |
| `str`   | no       | `"hi"`        | unicode by default                 |
| `bytes` | no       | `b"hi"`       | raw bytes                          |
| `tuple` | no       | `(1, 2)`      | fixed-size record                  |
| `list`  | yes      | `[1, 2]`      | dynamic array                      |
| `dict`  | yes      | `{"a": 1}`    | hash table, insertion-ordered      |
| `set`   | yes      | `{1, 2}`      | hash table, no duplicates          |
| `frozenset` | no   | `frozenset({1,2})` | hashable variant of set       |
| `None`  | n/a      | `None`        | the singleton "no value"           |

## 2. Mechanism — numbers, strings, and the sequence protocol

### Numbers

```python
2 ** 100               # 1267650600228229401496703205376 — no overflow
10 / 3                 # 3.333... (always float)
10 // 3                # 3      (floor division)
10 % 3                 # 1
divmod(10, 3)          # (3, 1)

0.1 + 0.2 == 0.3       # False — IEEE 754 strikes
from math import isclose
isclose(0.1 + 0.2, 0.3)  # True

from decimal import Decimal
Decimal("0.1") + Decimal("0.2") == Decimal("0.3")   # True — for money
```

`bool` is an `int` subclass: `True == 1`, `False == 0`, `True + True == 2`. Useful (`sum(x > 0 for x in xs)` counts positives) and occasionally surprising.

### Strings — immutable unicode

```python
s = "héllo"
len(s)                  # 5 (code points, not bytes)
s.encode("utf-8")       # b'h\xc3\xa9llo' — 6 bytes

# f-strings are the modern format (PEP 701 in 3.12 made them more flexible)
name, n = "Yati", 7
f"{name} ate {n} cookies"          # 'Yati ate 7 cookies'
f"{n:>5}"                          # '    7'   right-align width 5
f"{3.14159:.2f}"                   # '3.14'
f"{n=}"                            # 'n=7'   debug form (3.8+)

# Useful methods
"  hi  ".strip()                   # 'hi'
"a,b,c".split(",")                 # ['a', 'b', 'c']
",".join(["a", "b", "c"])          # 'a,b,c'
"foo".startswith(("f", "g"))       # True
"abc".replace("b", "X")            # 'aXc'
```

Strings are *immutable*: `s[0] = "H"` raises `TypeError`. You build new strings; never mutate.

### Sequences — `list`, `tuple`, `range`

All support indexing, slicing, `len`, `in`, iteration, concatenation:

```python
xs = [10, 20, 30, 40, 50]
xs[0]            # 10
xs[-1]           # 50  (negative = from end)
xs[1:4]          # [20, 30, 40]   half-open: includes 1, excludes 4
xs[::2]          # [10, 30, 50]   step
xs[::-1]         # [50, 40, 30, 20, 10]  reverse

xs.append(60)
xs.extend([70, 80])
xs.pop()         # 80
xs.remove(20)    # removes first occurrence by value
20 in xs         # False
sorted(xs)       # new sorted list; xs.sort() sorts in place
```

Tuples are immutable lists, used for fixed-size records:

```python
point = (3, 4)
x, y = point             # unpacking
first, *rest = [1,2,3,4] # first=1, rest=[2,3,4]
a, b = b, a              # swap, no temp
```

### Dicts — the workhorse

Insertion-ordered since 3.7. Lookups are O(1) average.

```python
user = {"name": "Yati", "age": 30}
user["name"]                  # KeyError if missing
user.get("missing", "n/a")    # safe lookup with default
user.setdefault("tags", []).append("admin")   # init-if-missing pattern

# Iteration
for k, v in user.items(): ...
list(user.keys()), list(user.values())

# Merging (3.9+)
defaults = {"theme": "dark"}
combined = defaults | user        # new dict, user wins on conflicts
defaults |= user                  # in-place merge

# Comprehension
{k: v for k, v in user.items() if v}   # drop falsy values
```

### Sets — uniqueness and set algebra

```python
a = {1, 2, 3}
b = {2, 3, 4}
a | b      # union        {1, 2, 3, 4}
a & b      # intersection {2, 3}
a - b      # difference   {1}
a ^ b      # symmetric    {1, 4}
3 in a     # O(1)
```

Use `set` to dedupe (`set(xs)`) and for fast membership tests on large collections. **Empty set is `set()`, not `{}`** — `{}` is an empty dict.

## 3. Variations — mutability and identity in depth

### `is` vs `==`

`==` asks "are the values equal?" `is` asks "are these the same object in memory?" Use `is` only for singletons:

```python
x is None        # right
x == None        # works but un-Pythonic; linters flag it
[] == []         # True
[] is []         # False — two separate list objects

a = [1, 2, 3]
b = a            # same object
c = list(a)      # shallow copy
a is b           # True
a is c           # False
a == c           # True
```

CPython caches small ints (`-5` to `256`) and short strings, so `200 is 200` happens to be `True` and `1000 is 1000` may be `False`. **Never rely on this** — it's an implementation detail.

### Shallow vs deep copy

```python
import copy
nested = [[1, 2], [3, 4]]
shallow = nested.copy()           # or list(nested), nested[:]
deep = copy.deepcopy(nested)

nested[0].append(99)
shallow[0]    # [1, 2, 99]  — inner list is shared
deep[0]       # [1, 2]      — fully independent
```

### Truthiness

`if x:` is true for non-empty containers, non-zero numbers, and non-`None`/`False` singletons. The Pythonic check for "has items":

```python
if items:        # not  if len(items) > 0:
    process(items)
```

Falsy values: `False`, `None`, `0`, `0.0`, `""`, `[]`, `{}`, `()`, `set()`.

### Operator chaining

```python
0 <= age < 150            # idiomatic; age must be in [0, 150)
0 <= age and age < 150    # same thing, less Pythonic
```

Walrus operator (`:=`) assigns inside an expression — useful in `while` and comprehensions:

```python
while (line := input("> ")) != "quit":
    print(line)
```

## 4. Practical Application — a tiny inventory tracker

Combines lists, dicts, sets, sorting, and f-strings:

```python
inventory: dict[str, int] = {}
restock_history: list[tuple[str, int]] = []

def restock(item: str, qty: int) -> None:
    inventory[item] = inventory.get(item, 0) + qty
    restock_history.append((item, qty))

def sell(item: str, qty: int) -> bool:
    if inventory.get(item, 0) < qty:
        return False
    inventory[item] -= qty
    return True

def report() -> str:
    lines = [f"{'item':<12}{'qty':>5}"]
    for item, qty in sorted(inventory.items(), key=lambda kv: -kv[1]):
        lines.append(f"{item:<12}{qty:>5}")
    distinct = len({item for item, _ in restock_history})
    lines.append(f"\n{distinct} distinct items restocked")
    return "\n".join(lines)

restock("apple", 10)
restock("banana", 5)
restock("apple", 3)
sell("apple", 4)
print(report())
# item          qty
# apple           9
# banana          5
#
# 2 distinct items restocked
```

Notice: `dict.get` for safe lookups, set comprehension for uniqueness, `sorted` with a `key` lambda, and f-string alignment. All idiomatic.

## 5. Common Mistakes & Gotchas

- **`{}` is a dict, not a set.** Empty set is `set()`.
- **Mutating while iterating.** `for x in xs: xs.remove(x)` skips elements. Iterate over a copy (`xs[:]`) or build a new list with a comprehension.
- **`==` vs `is` confusion.** `if x == None:` works but `if x is None:` is the rule. Same for `True`/`False`.
- **Float equality.** Use `math.isclose` or `decimal.Decimal` for money/precision.
- **Mixing `bytes` and `str`.** Python 3 will not silently convert. Encode (`s.encode("utf-8")`) and decode (`b.decode("utf-8")`) explicitly.
- **`list * n` with mutables.** `[[]] * 3` creates three references to the *same* list — append to one, all change. Use `[[] for _ in range(3)]`.
- **Misusing `+` to build strings in a loop.** O(n²). Use `"".join(parts)`.
- **Assuming `dict` order in code that has to run on Python <3.7.** Fine for 3.7+, but don't rely on order semantics for set/frozenset.

## 🎯 Key Takeaways

- **Mutability is the most important attribute of a type.** Mutable defaults, shared references, and "why did my list change?" all trace back to it. Internalize the table.
- **Use the right container.** `dict` for keyed lookup, `set` for uniqueness/membership, `tuple` for fixed records, `list` for ordered mutable sequences. Picking wrong shows up immediately in code review.
- **Slicing is half-open and forgiving.** `xs[:n]`, `xs[n:]`, `xs[::-1]` are everywhere; out-of-range slices don't raise. Embrace them.
- **f-strings with format specs (`:.2f`, `:>10`, `=` debug)** are how seniors format output. Stop concatenating with `+`.
- **`is` is for singletons (`None`, `True`, `False`); `==` is for value equality.** Linters and reviewers will catch the difference.

*← [prev](./01_setup_and_syntax.md) | [next →](./03_control_flow.md)*
