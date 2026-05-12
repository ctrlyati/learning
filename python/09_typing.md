# 09 — Typing & Type Hints
> **Goal:** Use modern type hints (3.12+ syntax) so mypy or pyright catches whole classes of bugs before they ship.

## 1. Why types in a dynamic language?

Python is dynamically typed at runtime — annotations don't change behavior. They're metadata that:

1. **Static checkers (mypy, pyright)** read to find bugs without running your code.
2. **IDEs** use for completion, refactoring, and inline errors.
3. **Humans** read as enforced documentation.
4. **Some libraries** (pydantic, FastAPI, attrs, dataclasses) use *at runtime* to drive behavior.

Mental model: hints are an opt-in, gradually-typed overlay. You can annotate one function and leave the rest untyped. Add a type checker to CI and the value compounds.

```python
def greet(name: str, times: int = 1) -> str:
    return ("hello, " + name + "\n") * times
```

Run mypy:

```bash
pip install mypy
mypy --strict src/
```

`--strict` rejects untyped code. New projects: turn it on day one. Existing projects: enable per-module with `# mypy: strict-optional` or in `pyproject.toml`.

## 2. The core vocabulary

### Built-in generics (3.9+ syntax — drop the imports)

You don't need `from typing import List, Dict, Tuple, Optional` anymore:

```python
def parse(text: str) -> list[int]:
    return [int(x) for x in text.split()]

def index(items: list[str]) -> dict[str, int]:
    return {s: i for i, s in enumerate(items)}

# Optional → use | None
def find(name: str) -> User | None:
    ...

# Union of types — | (3.10+)
def to_int(x: int | str) -> int:
    return int(x)
```

### Common building blocks

```python
from typing import Any, Callable, Iterable, Iterator, Sequence, Mapping

# Callable — function type
on_done: Callable[[int, str], None] = my_handler   # (int, str) -> None

# Iterable / Iterator / Sequence / Mapping — abstract types
def total(values: Iterable[float]) -> float:       # accepts list, tuple, generator
    return sum(values)

def first(seq: Sequence[int]) -> int:              # supports len/index, not just iter
    return seq[0]

def lookup(m: Mapping[str, int], key: str) -> int: # dict-like, read-only contract
    return m[key]
```

Prefer abstract types in **parameters** (be permissive in what you accept) and concrete types in **return values** (be specific in what you produce). Postel's law applied to types.

### `Any` and `object` — different things

```python
def f(x: Any) -> Any:    # opt out of type checking; methods unchecked
    return x.anything()

def f(x: object) -> str:   # accepts anything but you can't call methods until you narrow
    if isinstance(x, str):
        return x
    return str(x)
```

`Any` is a hatch — use it sparingly, ideally with a `# TODO` to fix later. `object` keeps you honest.

## 3. Variations — Protocols, TypedDict, Generics, Self

### `Protocol` — structural (duck) typing

`Protocol` says "anything with these methods qualifies." No inheritance required:

```python
from typing import Protocol

class SupportsClose(Protocol):
    def close(self) -> None: ...

def shutdown(resource: SupportsClose) -> None:
    resource.close()

# Anything with .close() works — no isinstance check, no base class.
shutdown(open("x"))
shutdown(some_socket)
```

Protocols are how you do "interfaces" in Python without forcing inheritance. They're how `typing.Iterable`, `Mapping`, and friends are defined.

### `TypedDict` — typed JSON-shaped dicts

```python
from typing import TypedDict, NotRequired

class UserPayload(TypedDict):
    id: int
    name: str
    email: NotRequired[str]      # 3.11+; field may be absent

def handle(u: UserPayload) -> None:
    print(u["id"], u["name"])
    if "email" in u:
        print(u["email"])
```

TypedDicts give static-type safety to the JSON-style dicts you'd never want to convert into full classes (e.g., HTTP request bodies during prototyping). They're erased at runtime — they're literally just `dict` for the interpreter.

For runtime validation, use **pydantic** (module 16).

### Generics — write reusable code

PEP 695 syntax (3.12+) is the new clean way:

```python
def first[T](items: list[T]) -> T:
    return items[0]

class Stack[T]:
    def __init__(self) -> None:
        self._items: list[T] = []
    def push(self, item: T) -> None:
        self._items.append(item)
    def pop(self) -> T:
        return self._items.pop()

s: Stack[int] = Stack()
s.push(1)
x: int = s.pop()
```

Old syntax (still common in older codebases):

```python
from typing import TypeVar, Generic
T = TypeVar("T")

def first(items: list[T]) -> T: ...

class Stack(Generic[T]):
    ...
```

Bounded generics — `T` must be a subtype:

```python
def max_of[T: (int, float)](xs: list[T]) -> T: ...        # one of int or float
def serialize[T: SupportsClose](x: T) -> bytes: ...       # T must satisfy protocol
```

### `Self` — return the actual subclass

```python
from typing import Self     # 3.11+

class Builder:
    def add(self, x: int) -> Self:
        ...
        return self

class JSONBuilder(Builder):
    pass

JSONBuilder().add(1)   # static type: JSONBuilder, not Builder
```

Without `Self`, the return type would be `Builder`, breaking method-chaining on subclasses.

### `Literal`, `Final`, `Annotated`

```python
from typing import Literal, Final, Annotated

def open_file(mode: Literal["r", "w", "rb", "wb"]) -> object:
    ...

MAX: Final = 100      # mypy will error if you reassign

# Annotated — attach metadata for libraries (FastAPI, pydantic, etc.)
PositiveInt = Annotated[int, "must be > 0"]
```

`Literal` is great for "magic strings" — replaces `Enum` for simple cases.

### `cast` and `assert isinstance` — narrowing

When the checker can't follow your logic:

```python
from typing import cast

x: object = get_thing()
s = cast(str, x)               # tell the checker; no runtime check
print(s.upper())

# Better: actual runtime narrowing
if isinstance(x, str):
    print(x.upper())           # checker knows x is str inside the if
```

Prefer `isinstance` (real check) over `cast` (lie to the checker).

## 4. Practical Application — a typed repository pattern

Combines protocols, generics, and dataclasses to build a typed in-memory repository that any DB-backed implementation can substitute for:

```python
from dataclasses import dataclass
from typing import Protocol

@dataclass(frozen=True, slots=True)
class User:
    id: int
    email: str

class Repository[T](Protocol):
    def get(self, id: int) -> T | None: ...
    def add(self, item: T) -> None: ...
    def all(self) -> list[T]: ...


class InMemoryRepo[T]:
    def __init__(self, key: str = "id") -> None:
        self._items: dict[int, T] = {}
        self._key = key

    def get(self, id: int) -> T | None:
        return self._items.get(id)

    def add(self, item: T) -> None:
        key = getattr(item, self._key)
        self._items[key] = item

    def all(self) -> list[T]:
        return list(self._items.values())


def first_email_starting_with(repo: Repository[User], prefix: str) -> str | None:
    for u in repo.all():
        if u.email.startswith(prefix):
            return u.email
    return None


users: Repository[User] = InMemoryRepo[User]()
users.add(User(1, "yati@example.com"))
users.add(User(2, "bob@example.com"))
print(first_email_starting_with(users, "y"))      # yati@example.com
```

What the type system buys here:

- `Repository[T]` is a **structural** interface — `InMemoryRepo` doesn't inherit from it but matches its shape, so it's accepted.
- `first_email_starting_with` accepts *any* `Repository[User]` — easy to swap an in-memory implementation for a SQL-backed one in tests.
- Generics flow through: `repo.all()` returns `list[User]`, so `u.email` is type-checked.
- mypy will catch `users.add("not a user")`, `users.get("not an int")`, and missing return types.

## 5. Common Mistakes & Gotchas

- **`Optional[T]` vs `T | None`.** Same thing. Prefer `T | None` (3.10+) — more readable and matches the runtime `isinstance(x, T | None)` syntax.
- **Forgetting `-> None` on functions that return nothing.** mypy strict mode flags this. Easy habit.
- **`list[T]` parameter when you really mean "any sequence."** Forces callers to convert. Use `Iterable[T]` or `Sequence[T]` and your function accepts more.
- **`Any` everywhere.** Defeats the point. Use `object` (have to narrow) or specific unions.
- **Mutable defaults retained from typed signatures.** Annotations don't fix the mutable-default trap. `def f(xs: list[int] = []):` is still wrong.
- **Type-checking only the function signatures, not bodies.** Annotated parameters but `# type: ignore` everywhere defeats the purpose.
- **Using `cast` to silence the checker without checking.** Almost always means the type is wrong upstream — fix it there.
- **Ignoring `Generator[Yield, Send, Return]`.** Generator return type is `Iterator[T]` for the common case; full form is for advanced uses.
- **Skipping `py.typed`.** If you publish a typed library, include an empty `py.typed` marker file (PEP 561) so downstream mypy actually checks against your hints.

## 🎯 Key Takeaways

- **Turn on `mypy --strict` (or pyright `strict`) day one for new projects.** The "I'll add types later" plan never executes; doing it from scratch is painless.
- **Modern syntax: `list[int]`, `X | None`, `def foo[T]`.** Drop the `typing` imports for built-in generics. Cleaner; matches what you'll see in new codebases.
- **`Protocol` is duck typing made type-safe.** Use it instead of `ABC`/`abstractmethod` — no inheritance required, fewer constraints on callers.
- **`TypedDict` for incoming JSON shapes; pydantic for runtime validation; dataclasses for owned values.** Three roles, three tools.
- **Be permissive in what you accept (`Iterable`, `Mapping`), specific in what you return (`list`, `dict`).** This is the type-system version of the robustness principle and ages well.

*← [prev](./08_errors_and_context.md) | [next →](./10_stdlib_essentials.md)*
