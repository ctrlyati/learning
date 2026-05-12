# 06 — OOP & Dataclasses
> **Goal:** Write classes that are readable and idiomatic — using dataclasses, dunder methods, and inheritance only when they pay rent.

## 1. Classes — namespaces with state and behavior

A class bundles data (attributes) and operations (methods). Convention: `PascalCase` names; methods take `self` (the instance) as their first parameter.

```python
class Counter:
    def __init__(self, start: int = 0) -> None:
        self.value = start

    def bump(self, by: int = 1) -> None:
        self.value += by

    def __repr__(self) -> str:
        return f"Counter(value={self.value})"

c = Counter(10)
c.bump(); c.bump(5)
c.value      # 16
print(c)     # Counter(value=16)
```

Mental model: `Counter(10)` calls `Counter.__init__(self, 10)` on a fresh instance. `c.bump()` is sugar for `Counter.bump(c)`. `self` isn't a keyword — it's just the conventional name for the first parameter.

## 2. Mechanism — dunders, properties, classmethods, staticmethods

### Dunder ("double-underscore") methods

These hook into Python's syntax. Implementing them lets your class behave like built-ins:

| Dunder | Triggered by | Purpose |
|--------|--------------|---------|
| `__init__` | `Cls(...)` | construct |
| `__repr__` | `repr(x)`, REPL display | unambiguous debug string |
| `__str__` | `str(x)`, `print(x)` | human-readable string |
| `__eq__`, `__hash__` | `==`, `set`, `dict` keys | equality and hashing |
| `__lt__`, `__le__`, etc. | `<`, `sorted` | ordering |
| `__len__`, `__iter__`, `__contains__` | `len`, `for`, `in` | container behavior |
| `__getitem__`, `__setitem__` | `x[k]` | indexing |
| `__call__` | `x(...)` | make instances callable |
| `__enter__`, `__exit__` | `with` | context manager (module 8) |

Practical example: a vector class that supports `+`, equality, and printing:

```python
from math import hypot

class Vec2:
    def __init__(self, x: float, y: float) -> None:
        self.x, self.y = x, y

    def __repr__(self) -> str:
        return f"Vec2({self.x}, {self.y})"

    def __eq__(self, other: object) -> bool:
        return isinstance(other, Vec2) and (self.x, self.y) == (other.x, other.y)

    def __hash__(self) -> int:
        return hash((self.x, self.y))

    def __add__(self, other: "Vec2") -> "Vec2":
        return Vec2(self.x + other.x, self.y + other.y)

    def __abs__(self) -> float:
        return hypot(self.x, self.y)

a = Vec2(1, 2)
b = Vec2(3, 4)
a + b           # Vec2(4, 6)
abs(a + b)      # 7.21...
{a, b}          # set works because __hash__ is defined
```

**Always implement `__repr__`.** It's the single biggest debuggability win. Aim for `eval(repr(x)) == x` when feasible.

### Properties — computed attributes that look like fields

```python
class Temperature:
    def __init__(self, celsius: float) -> None:
        self._celsius = celsius

    @property
    def celsius(self) -> float:
        return self._celsius

    @celsius.setter
    def celsius(self, value: float) -> None:
        if value < -273.15:
            raise ValueError("below absolute zero")
        self._celsius = value

    @property
    def fahrenheit(self) -> float:
        return self._celsius * 9 / 5 + 32

t = Temperature(20)
t.fahrenheit       # 68.0  — looks like an attribute, runs code
t.celsius = 100    # validates
```

Don't reach for properties immediately. Start with plain attributes; convert to properties only when you need validation, lazy computation, or backward compatibility (an attribute becomes a property without changing the public API).

### `@classmethod` and `@staticmethod`

```python
class User:
    def __init__(self, name: str, email: str) -> None:
        self.name = name
        self.email = email

    @classmethod
    def from_dict(cls, d: dict) -> "User":
        return cls(d["name"], d["email"])     # cls = User; subclass-friendly

    @staticmethod
    def is_valid_email(s: str) -> bool:
        return "@" in s and "." in s
```

`@classmethod` for **alternative constructors** and factory patterns. `@staticmethod` for utilities that logically belong to the class but don't use `self` or `cls` — often these should just be module-level functions instead.

### `@dataclass` — the right default for "record" classes

Most classes are bags of attributes with `__init__`, `__repr__`, and `__eq__`. `@dataclass` writes those for you:

```python
from dataclasses import dataclass, field

@dataclass
class User:
    id: int
    name: str
    email: str
    tags: list[str] = field(default_factory=list)   # mutable default — use factory

u = User(1, "Yati", "y@example.com")
u                  # User(id=1, name='Yati', email='y@example.com', tags=[])
u == User(1, "Yati", "y@example.com", [])    # True
```

Useful flavors:

```python
@dataclass(frozen=True)      # immutable + hashable; great for value objects
@dataclass(slots=True)       # uses __slots__, smaller memory, faster attr access
@dataclass(kw_only=True)     # all fields keyword-only — safer APIs
@dataclass(order=True)       # generates __lt__ etc. for sorting
```

Use `dataclass` for ~80% of your classes. Reach for plain `class` only when you need real behavior (lots of methods) or unusual construction logic.

For data validation/parsing, **pydantic** is the de facto choice — covered in module 16.

## 3. Variations — inheritance, MRO, composition, slots

### Inheritance and `super()`

```python
class Animal:
    def __init__(self, name: str) -> None:
        self.name = name
    def speak(self) -> str:
        return "..."

class Dog(Animal):
    def __init__(self, name: str, breed: str) -> None:
        super().__init__(name)
        self.breed = breed
    def speak(self) -> str:
        return "woof"
```

`super()` looks up the next class in the **MRO** (method resolution order) — Python's algorithm for diamond inheritance. Check it with `Cls.__mro__` or `Cls.mro()`.

```python
class Loggable:
    def save(self): print("logging"); super().save()
class Persistable:
    def save(self): print("saving")
class Doc(Loggable, Persistable):
    pass

Doc().save()
# logging
# saving
Doc.__mro__   # (Doc, Loggable, Persistable, object)
```

This is called **cooperative multiple inheritance** and powers mixins. It works as long as every class in the chain calls `super()`. Use mixins sparingly.

### Composition over inheritance

The Pythonic default: **prefer composition**. Inheritance couples you tightly to the parent's API; composition lets you swap implementations.

```python
# Inheritance — strong coupling
class CachedFetcher(HTTPFetcher):
    def get(self, url):
        ...

# Composition — looser
class CachedFetcher:
    def __init__(self, fetcher: Fetcher, cache: Cache) -> None:
        self.fetcher = fetcher
        self.cache = cache
    def get(self, url):
        if url in self.cache: return self.cache[url]
        result = self.fetcher.get(url)
        self.cache[url] = result
        return result
```

Real codebases use inheritance for "is-a" (a `BadRequestError` is an `HTTPError`) and composition for "has-a" (a `Service` *has* a `DB`).

### Duck typing & `Protocol`

You don't usually need an interface — Python uses **duck typing**. If your function calls `obj.read(n)`, anything with a `read` method works:

```python
def process(stream):
    return stream.read(1024)

process(open("file.txt"))    # works
process(io.BytesIO(b"..."))  # works
```

When you want type-checked duck typing, use `Protocol` (module 09):

```python
from typing import Protocol

class Readable(Protocol):
    def read(self, n: int) -> bytes: ...

def process(stream: Readable) -> bytes:
    return stream.read(1024)
```

### `__slots__` — when memory matters

By default, instances store attributes in a dict (`__dict__`). For millions of small objects that's wasteful. `__slots__` swaps the dict for a fixed slot array:

```python
class Point:
    __slots__ = ("x", "y")
    def __init__(self, x: float, y: float) -> None:
        self.x, self.y = x, y
```

Or with dataclasses: `@dataclass(slots=True)`. Trade-off: no dynamic attributes, slightly more friction with inheritance. Use when you've measured and it matters.

## 4. Practical Application — a small domain model

A realistic example using dataclass, frozen value objects, classmethods, and composition:

```python
from dataclasses import dataclass, field
from datetime import datetime
from typing import Iterable

@dataclass(frozen=True, slots=True)
class Money:
    amount: int          # cents, integer to avoid float issues
    currency: str = "USD"

    def __add__(self, other: "Money") -> "Money":
        if self.currency != other.currency:
            raise ValueError(f"can't add {self.currency} and {other.currency}")
        return Money(self.amount + other.amount, self.currency)

    def __str__(self) -> str:
        return f"{self.amount/100:.2f} {self.currency}"


@dataclass
class LineItem:
    sku: str
    qty: int
    unit_price: Money

    @property
    def subtotal(self) -> Money:
        return Money(self.unit_price.amount * self.qty, self.unit_price.currency)


@dataclass
class Order:
    id: int
    customer_email: str
    items: list[LineItem] = field(default_factory=list)
    created_at: datetime = field(default_factory=datetime.now)

    @classmethod
    def from_payload(cls, data: dict) -> "Order":
        items = [
            LineItem(i["sku"], i["qty"], Money(i["unit_price_cents"]))
            for i in data["items"]
        ]
        return cls(id=data["id"], customer_email=data["email"], items=items)

    def add(self, item: LineItem) -> None:
        self.items.append(item)

    @property
    def total(self) -> Money:
        if not self.items:
            return Money(0)
        return sum((i.subtotal for i in self.items[1:]), self.items[0].subtotal)


order = Order.from_payload({
    "id": 42,
    "email": "y@example.com",
    "items": [
        {"sku": "book", "qty": 2, "unit_price_cents": 1500},
        {"sku": "pen",  "qty": 5, "unit_price_cents": 200},
    ],
})
print(order.total)        # 40.00 USD
order.add(LineItem("mug", 1, Money(800)))
print(order.total)        # 48.00 USD
```

Notice:

- `Money` is `frozen=True, slots=True` — value objects should be immutable and hashable.
- Integer cents prevents float arithmetic bugs (cross-reference module 02).
- `LineItem` uses `@property` for a derived value (no need to store `subtotal`).
- `Order.from_payload` is a `@classmethod` constructor for parsing dicts.
- Composition: `Order` *has* `LineItem`s, which *have* `Money`. No inheritance chain to remember.

## 5. Common Mistakes & Gotchas

- **Mutable class-level defaults.** `class C: items = []` shares one list across all instances. Use `__init__` or `field(default_factory=list)`.
- **Forgetting `super().__init__(...)`.** Subclass init should call parent init unless you really mean to skip it. Easy bug in larger hierarchies.
- **Implementing `__eq__` without `__hash__`.** Defining `__eq__` sets `__hash__` to `None` (instance becomes unhashable). Either define both or use `@dataclass(eq=True, frozen=True)`.
- **Reaching for inheritance to share code.** Almost always a mixin or a helper function/composition is cleaner.
- **Properties for everything.** Trivial getters/setters with no logic add ceremony without value. Just use a public attribute.
- **Confusing `@classmethod` and `@staticmethod`.** Use `@classmethod` when you need `cls` (alternative constructors, polymorphic factories). Otherwise prefer module-level functions over `@staticmethod`.
- **Deep inheritance trees.** Three levels is suspicious; five is wrong. Composition.
- **Overriding `__init__` in dataclasses.** Use `__post_init__` for validation/derived fields instead — keeps the generated init signature clean.

## 🎯 Key Takeaways

- **Default to `@dataclass`** for record-like classes. Add `frozen=True, slots=True` for value objects. You'll write less code and get correct `__repr__`/`__eq__` for free.
- **Implement `__repr__` early.** It's the cheapest debugging investment in Python — every print, every traceback, every REPL inspection improves.
- **Composition first; inheritance only for true "is-a" relationships.** Mixins are powerful and dangerous — keep them shallow and well-named.
- **Properties are for *replacing* an attribute with logic, not for ceremony.** Public attributes are fine in Python. Don't import Java's getter/setter culture.
- **Duck typing is the default; `Protocol` (module 09) is how you type-check it.** You almost never need `ABC`/`abstractmethod` in modern Python.

*← [prev](./05_modules_and_packages.md) | [next →](./07_iterators_and_generators.md)*
