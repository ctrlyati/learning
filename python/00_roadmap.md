# Python Deep-Dive — Roadmap

> A 17-module path from "I can write a script" to "I can ship Python in production."

This course is for a working developer doing professional upskilling. It assumes you already program — maybe in Go, Java, JS, or another language — and want Python at the level a senior engineer uses it: idiomatic, typed, tested, packaged, and observable.

## Module Table

| #  | Title                                | Focus areas |
|----|--------------------------------------|-------------|
| 01 | Setup & Syntax                       | CPython, venvs, pip, `pyproject.toml`, REPL workflow, running code |
| 02 | Data Types & Operators               | int/float/str/bytes, list/tuple/set/dict, mutability, truthiness |
| 03 | Control Flow & Comprehensions        | if/for/while, `match`, comprehensions, generator expressions, walrus |
| 04 | Functions, Scope, Closures, Decorators | `*args`/`**kwargs`, default-arg trap, LEGB, closures, decorators |
| 05 | Modules, Packages, Imports           | import system, `__init__.py`, namespace pkgs, `__main__`, circular imports |
| 06 | OOP & Dataclasses                    | classes, MRO, `super()`, dunders, `@dataclass`, `__slots__` |
| 07 | Iterators, Generators, itertools     | iterator protocol, `yield`, generator pipelines, `itertools` recipes |
| 08 | Errors & Context Managers            | exception hierarchy, EAFP, custom exceptions, `with`, `contextlib` |
| 09 | Typing & Type Hints                  | mypy, `Protocol`, `TypedDict`, generics, `Self`, PEP 695 syntax |
| 10 | Standard Library Essentials          | collections, pathlib, json, datetime, subprocess, logging |
| 11 | File I/O & Serialization             | text vs binary, encodings, JSON/CSV/pickle/sqlite, streaming |
| 12 | Concurrency                          | threading, multiprocessing, asyncio, GIL, when to pick what |
| 13 | Testing                              | pytest, fixtures, parametrize, mocking, coverage, property-based |
| 14 | Packaging & Distribution             | pyproject, build backends, wheels, PyPI, entry points |
| 15 | Performance                          | profiling, caching, vectorization, C extensions, numpy basics |
| 16 | Web & APIs                           | requests, httpx, FastAPI tour, pydantic models |
| 17 | Production Patterns                  | structured logging, config, secrets, project layout, observability |

## Suggested Timeline

One module per day = ~3 weeks. Realistic pacing for a working engineer:

- **Week 1 (foundations):** modules 01–06. Focus on idioms — many Python pitfalls come from translating habits from other languages literally.
- **Week 2 (depth):** modules 07–12. This is where Python's expressiveness pays off. Don't skip generators or `asyncio` — they show up everywhere.
- **Week 3 (pro):** modules 13–17. Testing, packaging, performance, and production discipline. This is the gap between "writes Python" and "ships Python."

If you have less time, a 5-day fast track: 01, 04, 06, 09, 13. That gets you syntax, idioms, OOP, types, and tests — enough to contribute to a real codebase.

## How to Use

1. Read top-to-bottom — each module references earlier ones.
2. **Type the code.** Don't copy-paste. Muscle memory matters.
3. Keep a scratch venv open and a REPL (`python -i` or `bpython`/`ptpython`).
4. After each module, build something tiny that exercises the new idea.
5. Run `mypy --strict` on your code from module 09 onward. The errors teach you Python.

## Prerequisites & Setup

- Python **3.12 or 3.13**. Earlier versions miss `match`, PEP 695 generics, and improved error messages.
- A real editor: VS Code with Pylance, or PyCharm.
- `git`, a terminal, and the willingness to read tracebacks bottom-up.

Quick install check:

```bash
python --version          # 3.12.x or higher
python -m venv .venv
source .venv/bin/activate # Windows: .venv\Scripts\activate
python -m pip install --upgrade pip
```

## Core Mental Models

These show up in every module. Internalize them early.

1. **Everything is an object.** Functions, classes, modules, even `None` — all have attributes and a type. `dir(x)` is your friend.
2. **Names are bindings, not boxes.** `x = [1,2,3]; y = x` makes `y` point to the same list. Mutation is shared; rebinding is not.
3. **Duck typing.** Python cares what an object *does*, not what it *is*. If it has `__iter__`, it iterates. `Protocol` formalizes this.
4. **EAFP over LBYL.** "Easier to ask forgiveness than permission." `try/except` is idiomatic; pre-checking with `if hasattr(...)` often is not.
5. **The GIL is real but narrower than people say.** It blocks CPU-bound threading, not I/O. Pick concurrency model based on workload (module 12).
6. **Iteration is a protocol, not a loop.** Generators, comprehensions, and `for` all rest on `__iter__`/`__next__`. Once you see this, half the stdlib clicks.

## Curated External Resources

- **Official docs** — https://docs.python.org/3/ — the tutorial and library reference are first-class. Use them.
- **Real Python** — https://realpython.com — high-quality tutorials, especially on typing and asyncio.
- **Fluent Python, 2nd ed.** by Luciano Ramalho — the book to read after this course. Deep, idiomatic, modern.
- **PEP index** — https://peps.python.org — read PEP 8, 20, 257, 484, 612, 695. They're short and explain *why*.
- **Python Bytes podcast** — https://pythonbytes.fm — weekly news, low effort, keeps you current.
- **Talk Python To Me** — https://talkpython.fm — long-form interviews; great for absorbing how seniors think.

## Why This Course Exists

Python is easy to start and hard to master. Most tutorials stop at syntax. Most books are reference manuals. This course threads the needle: enough depth to write code a senior would approve in code review, focused on the parts that pay off in real jobs — typing, testing, packaging, concurrency, and production hygiene.

Finish this and you should be able to walk into a Python codebase, read it, extend it, test it, and ship it.

*[start →](./01_setup_and_syntax.md)*
