# 05 — Modules, Packages, Imports
> **Goal:** Understand how Python finds, loads, and caches modules — and structure a project that scales without circular import pain.

## 1. Modules — every `.py` file is one

A **module** is a single `.py` file. A **package** is a directory of modules (with an `__init__.py` traditionally, or a "namespace package" without one). The `import` statement loads them.

```python
# greetings.py
def hello(name: str) -> str:
    return f"hello, {name}"

GREETING = "default"
```

```python
# main.py
import greetings
greetings.hello("Yati")        # 'hello, Yati'
greetings.GREETING             # 'default'
```

When Python imports `greetings`:

1. Looks it up in `sys.modules` — if cached, returns it (no re-execution).
2. Otherwise searches `sys.path` (a list of directories) for `greetings.py` or `greetings/`.
3. Executes the file top-to-bottom in a fresh module namespace.
4. Stores the result in `sys.modules["greetings"]`.

That's the whole import system. Everything else is variations.

## 2. Mechanism — import forms, packages, `__init__.py`

### Import forms

```python
import json                          # module bound as `json`
import json as J                     # alias

from pathlib import Path             # binds only `Path`
from pathlib import Path, PurePath   # multiple
from pathlib import Path as P        # alias

from collections import *            # imports everything public — avoid except in REPL
```

Rule of thumb: `import package` for stdlib (`json.dumps(...)` reads better than `dumps(...)`), `from package import Thing` for specific classes you'll use repeatedly. Never `import *` in real code — it pollutes the namespace and breaks linters/IDEs.

### Packages

A directory becomes a package if importable:

```
mypkg/
├── __init__.py        # makes it a "regular" package; can be empty
├── core.py
├── utils.py
└── sub/
    ├── __init__.py
    └── helpers.py
```

```python
import mypkg.core
from mypkg.utils import slugify
from mypkg.sub.helpers import deep_merge
```

`__init__.py` is the package's "module file" — it runs the first time anything inside the package is imported. Common uses:

```python
# mypkg/__init__.py
from .core import Engine          # re-export so `from mypkg import Engine` works
from .utils import slugify

__version__ = "1.0.0"
__all__ = ["Engine", "slugify"]   # controls `from mypkg import *`
```

The dot in `from .core import Engine` is a **relative import** — relative to the current package. Inside `mypkg/sub/helpers.py`, `from ..utils import slugify` reaches up one level. Use relative imports inside a package; absolute (`from mypkg.utils import slugify`) elsewhere.

### Namespace packages (PEP 420)

A directory without `__init__.py` is a namespace package. Useful for plugin systems where multiple distributions contribute submodules under the same root. For most apps, just include `__init__.py` files — explicit beats implicit here.

### `__main__` — running a package

```python
# mypkg/__main__.py
def main():
    print("running mypkg")

if __name__ == "__main__":
    main()
```

```bash
python -m mypkg     # executes __main__.py
```

Combine with `[project.scripts]` in `pyproject.toml` (module 14) for installable CLI tools.

### The `if __name__ == "__main__":` guard, properly explained

When you run `python foo.py`, Python sets `foo`'s `__name__` to `"__main__"`. When you `import foo`, `__name__` is `"foo"`. The guard separates "things that should always happen on import" from "things that only happen when you run this file directly":

```python
# tools/cleanup.py
def cleanup(): ...

if __name__ == "__main__":
    cleanup()        # runs only as a script, not on import
```

Without the guard, importing `tools.cleanup` from a test would *run* the cleanup. Bad.

## 3. Variations — `sys.path`, the cache, and circular imports

### Where Python looks: `sys.path`

```python
import sys
print(sys.path)
# ['', '/usr/lib/python3.13', ..., '/path/to/.venv/lib/python3.13/site-packages']
```

Order:

1. The script's directory (or current directory for `-m`).
2. `PYTHONPATH` environment variable directories.
3. The standard library.
4. Site-packages (where pip installs things).

You *can* mutate `sys.path` at runtime, but it's a code smell. Better: structure as a proper package and `pip install -e .` so it lives in `site-packages`.

### The module cache

Modules are imported once per process. To force a reload during development:

```python
import importlib, mypkg.core
importlib.reload(mypkg.core)     # rare; useful in notebooks/REPL
```

In production code, never reload. If you need pluggability, design for it (entry points, registries).

### Circular imports

`a.py` imports from `b.py`, which imports from `a.py`. The classic failure:

```python
# a.py
from b import B
class A: pass

# b.py
from a import A
class B: pass
```

When you `import a`, Python starts executing `a.py`, hits `from b import B`, starts executing `b.py`, hits `from a import A` — but `a` is half-loaded, `A` doesn't exist yet, `ImportError`.

Fixes, in order of preference:

1. **Restructure.** Usually one of the modules has too much in it. Extract shared types into a third module that both import from.
2. **Import inside the function.** Defers the import until call time, by which point both modules are loaded:
   ```python
   def make_b():
       from b import B
       return B()
   ```
3. **Use `import b` (not `from b import B`).** This binds the module object, which exists even when partially loaded; you reference `b.B` later.

If you find yourself fighting circular imports, the architecture is telling you something.

### Editable installs

During development:

```bash
pip install -e .
```

This installs your package as a link to your source tree. Edits to `src/mypkg/foo.py` are immediately visible — no reinstall. The standard for libraries you're actively developing.

## 4. Practical Application — a layered package

Realistic structure for a small service:

```
src/myapp/
├── __init__.py
├── __main__.py        # python -m myapp
├── config.py          # constants, env loading
├── models.py          # data classes
├── services/
│   ├── __init__.py
│   ├── users.py
│   └── orders.py
├── adapters/
│   ├── __init__.py
│   ├── db.py
│   └── http.py
└── cli.py
```

```python
# src/myapp/__init__.py
from .config import settings
from .models import User, Order

__version__ = "0.1.0"
__all__ = ["settings", "User", "Order"]
```

```python
# src/myapp/config.py
import os
from dataclasses import dataclass

@dataclass(frozen=True)
class Settings:
    database_url: str
    log_level: str = "INFO"

settings = Settings(
    database_url=os.environ.get("DATABASE_URL", "sqlite:///dev.db"),
)
```

```python
# src/myapp/models.py
from dataclasses import dataclass

@dataclass
class User:
    id: int
    email: str

@dataclass
class Order:
    id: int
    user_id: int
    total: float
```

```python
# src/myapp/services/users.py
from ..models import User
from ..adapters import db

def get_user(user_id: int) -> User | None:
    row = db.fetch_one("SELECT id, email FROM users WHERE id = ?", user_id)
    return User(**row) if row else None
```

```python
# src/myapp/__main__.py
from .cli import run

if __name__ == "__main__":
    run()
```

Run it:

```bash
pip install -e .
python -m myapp
```

Why this layout works:

- `src/` layout prevents accidentally importing the source tree before installation — ensures tests run against the installed package, the same one users will get.
- Layered packages (`services`, `adapters`, `models`) keep dependencies one-directional: services depend on adapters and models, never the other way. No circular imports possible.
- Re-exporting in `__init__.py` lets users write `from myapp import User` instead of `from myapp.models import User` — short public API, deeper internal structure.

## 5. Common Mistakes & Gotchas

- **Naming a file the same as a stdlib module.** `email.py`, `string.py`, `json.py` — your file shadows the stdlib and breaks every other library. Easy mistake; cryptic error.
- **`from foo import *` outside REPL.** Pollutes namespace, breaks tools, makes "where did this come from?" unanswerable.
- **Running scripts with `python script.py` from inside a package.** Breaks relative imports because `__package__` isn't set. Use `python -m mypkg.script` instead.
- **Putting heavy work at module top level.** `MODELS = load_all_from_db()` at module scope means importing the module hits the DB. Defer to a function. Top-level code should be cheap.
- **Circular imports.** Almost always a design smell — extract shared types to a third module.
- **Modifying `sys.path` to make imports work.** Symptom of bad project layout. Fix the layout instead.
- **Forgetting `__init__.py`.** Without it (and without intentional namespace-package use), tools and IDEs get confused even if Python "works."
- **Re-exporting in `__init__.py` and creating a circular dependency.** If `mypkg/__init__.py` imports from `mypkg.services`, and `mypkg.services` does `from mypkg import settings`, you've made a cycle. Use sub-module imports inside the package, only flatten at the top.

## 🎯 Key Takeaways

- **A module is a `.py` file; a package is a directory.** Imports are cached per-process via `sys.modules`. That model explains most edge cases.
- **`src/` layout + `pip install -e .` is the modern default** for any project beyond a single script. It catches "works locally, breaks installed" bugs early.
- **Use absolute imports across packages, relative imports within one.** Linters can enforce this; consistency keeps moves and renames safe.
- **Circular imports are an architecture signal.** Don't paper over them with deferred imports — extract shared definitions to a leaf module.
- **Curate your `__init__.py`** to define a clear public API via re-exports and `__all__`. Internal structure can be deep; what users `import` should be flat.

*← [prev](./04_functions.md) | [next →](./06_oop_and_dataclasses.md)*
