# 04 — Functions, Scope, Closures, Decorators
> **Goal:** Define functions the right way (positional/keyword/defaults), understand LEGB scope, and write decorators that don't break their wrapped functions.

## 1. Functions are first-class objects — pass them around

A function in Python is just an object you can name. You can pass it as an argument, return it from another function, store it in a list, attach attributes to it. This is what makes decorators, callbacks, and functional patterns natural:

```python
def shout(s: str) -> str:
    return s.upper() + "!"

f = shout                    # f is now another name for the same function
f("hello")                   # 'HELLO!'

def apply(fn, x):            # functions as parameters
    return fn(x)

apply(shout, "hi")           # 'HI!'

list(map(shout, ["a", "b"])) # ['A!', 'B!']
```

Mental model: `def name(...)` is `name = <function object>`. The name is a binding; the value is the function.

## 2. Mechanism — argument modes, defaults, scope

### Positional, keyword, defaults

```python
def greet(name: str, greeting: str = "hello", *, loud: bool = False) -> str:
    msg = f"{greeting}, {name}"
    return msg.upper() if loud else msg

greet("Yati")                       # 'hello, Yati'
greet("Yati", "hi")                 # 'hi, Yati'        positional
greet("Yati", greeting="hey")       # 'hey, Yati'       keyword
greet("Yati", loud=True)            # 'HELLO, YATI'     keyword-only after *
```

The bare `*` makes everything after it **keyword-only** — callers must name it. Use this for boolean flags and rarely-used options. Symmetric: `/` (PEP 570) makes prior args **positional-only**:

```python
def divide(a, b, /, *, precision=2):
    ...
divide(10, 3, precision=4)      # ok
divide(a=10, b=3)               # TypeError — a, b are positional-only
```

### `*args` and `**kwargs`

Variable arguments. `*args` collects extra positionals into a tuple; `**kwargs` collects extra keywords into a dict:

```python
def log(level, *messages, **fields):
    parts = [f"[{level}]"] + list(messages)
    parts += [f"{k}={v}" for k, v in fields.items()]
    print(" ".join(parts))

log("INFO", "user logged in", user="alice", ip="1.2.3.4")
# [INFO] user logged in user=alice ip=1.2.3.4
```

You can also *unpack* with the same syntax at the call site:

```python
args = ("user logged in",)
kwargs = {"user": "alice"}
log("INFO", *args, **kwargs)
```

### The mutable default argument trap

```python
def append_to(item, target=[]):     # WRONG
    target.append(item)
    return target

append_to(1)        # [1]
append_to(2)        # [1, 2]   — same list, surprise
```

Default values are evaluated **once**, when the `def` runs, not on each call. Fix:

```python
def append_to(item, target=None):
    if target is None:
        target = []
    target.append(item)
    return target
```

This bites every Python developer at least once. Reviewers always catch it.

### LEGB — how Python finds names

When you reference `x`, Python searches in this order: **L**ocal → **E**nclosing (any outer function) → **G**lobal (module) → **B**uilt-in.

```python
x = "global"

def outer():
    x = "enclosing"
    def inner():
        # x = "local"   # uncomment to see local win
        print(x)
    inner()

outer()             # 'enclosing'
```

Assignment in a function makes the name *local* unless declared otherwise:

```python
counter = 0
def bump():
    counter += 1     # UnboundLocalError — Python sees the assignment, treats counter as local
```

Fixes:

```python
def bump():
    global counter      # write to module-level
    counter += 1

def make_counter():
    n = 0
    def bump():
        nonlocal n      # write to enclosing scope
        n += 1
        return n
    return bump
```

`nonlocal` is rare in app code but central to closures.

### Closures

A closure is a function that remembers variables from its enclosing scope, even after that scope has finished executing:

```python
def make_multiplier(factor: int):
    def multiply(x: int) -> int:
        return x * factor
    return multiply

double = make_multiplier(2)
triple = make_multiplier(3)
double(10)    # 20
triple(10)    # 30
```

Closures are how Python implements callbacks, partial application, and decorators.

## 3. Variations — lambdas, partials, decorators

### Lambdas — single-expression anonymous functions

```python
sorted(words, key=lambda w: len(w))
list(filter(lambda x: x > 0, xs))
```

Use lambdas only as throwaway one-liners passed to higher-order functions. If you'd give it a name or it spans multiple lines, use `def`. PEP 8 explicitly says **don't** assign lambdas to names:

```python
# Bad
square = lambda x: x * x

# Good
def square(x): return x * x
```

### `functools.partial` — pre-fill some arguments

```python
from functools import partial

def power(base, exp): return base ** exp
square = partial(power, exp=2)
square(7)      # 49
```

Cleaner than a lambda when you're capturing values.

### Decorators — functions that wrap functions

A decorator takes a function, returns a (usually) modified function. The `@` syntax is sugar:

```python
@my_decorator
def f(): ...

# is exactly equivalent to:
def f(): ...
f = my_decorator(f)
```

A real example — timing:

```python
import time
from functools import wraps

def timed(fn):
    @wraps(fn)                        # critical — preserves name, docstring, signature
    def wrapper(*args, **kwargs):
        start = time.perf_counter()
        try:
            return fn(*args, **kwargs)
        finally:
            elapsed = time.perf_counter() - start
            print(f"{fn.__name__} took {elapsed*1000:.1f}ms")
    return wrapper

@timed
def slow():
    time.sleep(0.1)

slow()    # slow took 100.4ms
```

Without `@wraps`, the wrapped function's `__name__` becomes `"wrapper"`, which breaks introspection, logging, and `help(slow)`. Always use `functools.wraps`.

### Decorators with arguments — one extra layer

```python
def retry(times: int):
    def decorator(fn):
        @wraps(fn)
        def wrapper(*args, **kwargs):
            for attempt in range(1, times + 1):
                try:
                    return fn(*args, **kwargs)
                except Exception as e:
                    if attempt == times:
                        raise
                    print(f"attempt {attempt} failed: {e}")
        return wrapper
    return decorator

@retry(times=3)
def flaky(): ...
```

Three layers: `retry` takes args and returns a decorator; the decorator takes a function and returns a wrapper; the wrapper does the actual work.

### Stacking decorators

```python
@timed
@retry(times=3)
def fetch(): ...
```

Reads bottom-up: `fetch` is wrapped by `retry`, then the result is wrapped by `timed`. Outer decorator runs last.

## 4. Practical Application — a memoizing, logging cache decorator

A realistic decorator combining everything: closures, `*args/**kwargs`, `wraps`, defaults:

```python
from functools import wraps
from typing import Callable, Any
import time

def memoize(maxsize: int = 128, log: bool = False):
    def decorator(fn: Callable[..., Any]):
        cache: dict[tuple, Any] = {}
        order: list[tuple] = []

        @wraps(fn)
        def wrapper(*args, **kwargs):
            key = (args, tuple(sorted(kwargs.items())))
            if key in cache:
                if log:
                    print(f"cache HIT {fn.__name__}{args}")
                return cache[key]

            if log:
                print(f"cache MISS {fn.__name__}{args}")
            result = fn(*args, **kwargs)
            cache[key] = result
            order.append(key)

            if len(order) > maxsize:
                evict = order.pop(0)
                cache.pop(evict, None)

            return result

        wrapper.cache_clear = cache.clear   # type: ignore[attr-defined]
        wrapper.cache_info  = lambda: {"size": len(cache), "max": maxsize}  # type: ignore
        return wrapper
    return decorator


@memoize(maxsize=2, log=True)
def fib(n: int) -> int:
    return n if n < 2 else fib(n - 1) + fib(n - 2)

fib(10)
# cache MISS fib(10)
# cache MISS fib(9)
# ... lots of MISSes for first call ...
# cache HIT  fib(8)  ...

print(fib.cache_info())     # {'size': 2, 'max': 2}
fib.cache_clear()
```

In real code, just use `functools.lru_cache` — it's the same thing, written in C, with proper LRU eviction. But writing one yourself once teaches you closures, decorators, and attribute attachment all at once.

## 5. Common Mistakes & Gotchas

- **Mutable default arguments.** Use `None` and assign inside. Linters (ruff, pylint) flag this.
- **Forgetting `@functools.wraps`.** Wrapped function loses its name/docstring; `help()` becomes useless.
- **Closure capturing the *variable*, not the value.** Classic bug:
  ```python
  funcs = [lambda: i for i in range(3)]
  [f() for f in funcs]    # [2, 2, 2] — all see final i
  # Fix:
  funcs = [lambda i=i: i for i in range(3)]   # default arg captures value
  ```
- **`global` everywhere.** A sign you should be passing arguments or using a class. `global` is for scripts, not modules.
- **Reassigning a parameter and being surprised it doesn't affect the caller.** Names are local. `def f(x): x = 5` does nothing to the caller's `x`. *Mutating* a passed mutable does (`def f(xs): xs.append(1)`).
- **Decorators that don't pass `*args, **kwargs` through.** Wrapper signature must accept whatever the wrapped function does, or reuse won't work.
- **Using `lambda` for anything beyond one-liners.** Write a real `def`. Future-you will thank you.
- **Not annotating callables.** `Callable[[int, str], bool]` from `typing` documents intent and helps mypy. Module 09 covers types in depth.

## 🎯 Key Takeaways

- **Keyword-only arguments (`*`) are how you build APIs that age well.** Booleans and rarely-used flags should never be positional — `connect(host, 5, True, False)` is unreadable.
- **Mutable defaults and late-bound closures are the two scope traps every Python dev gets wrong once.** Burn them in now and you'll spot them in code review forever.
- **`@functools.wraps` is non-negotiable in decorators.** Without it, you silently break tracebacks, logging, and `help()`.
- **`functools.lru_cache`, `partial`, and `cached_property` cover 90% of what people roll by hand.** Reach for the stdlib before writing your own.
- **A function that takes a function and returns a function is just a closure.** Decorators are the most visible application; callbacks, factories, and test fixtures are the rest. Once closures click, a huge chunk of Python clicks with them.

*← [prev](./03_control_flow.md) | [next →](./05_modules_and_packages.md)*
