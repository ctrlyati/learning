# 01 — Setup & Syntax
> **Goal:** Install a modern Python, understand the runtime/tooling landscape, and run code three ways (script, REPL, module).

## 1. The Python Ecosystem — pick the right interpreter and isolate it

Python is a *language*; the thing that runs your code is an *interpreter*. The reference implementation is **CPython**, written in C, and that's what 95% of jobs mean by "Python." Others exist (PyPy for speed, Jython for JVM, MicroPython for embedded), but learn CPython first.

Mental model: think of CPython as a virtual machine that compiles your `.py` files to bytecode (`.pyc`, cached in `__pycache__/`) and executes them in an interpreter loop. The same source runs on Mac/Linux/Windows because the bytecode VM abstracts the OS.

Install **Python 3.12+**:

```bash
# macOS
brew install python@3.13

# Ubuntu/Debian
sudo apt install python3.13 python3.13-venv

# Windows
winget install Python.Python.3.13
```

Verify and immediately make a virtual environment — never install packages into the system Python:

```bash
python3 --version           # Python 3.13.x
python3 -m venv .venv
source .venv/bin/activate   # Windows: .venv\Scripts\activate
which python                # should point inside .venv
python -m pip install --upgrade pip
```

A venv is just a folder with its own `python` binary and `site-packages/`. Activating it prepends the venv's `bin/` to `PATH`. That's the whole magic.

## 2. Three ways to run code — and when each is right

**(a) Script.** A `.py` file you execute. The 95% case.

```python
# hello.py
def greet(name: str) -> str:
    return f"hello, {name}"

if __name__ == "__main__":
    print(greet("world"))
```

```bash
python hello.py
```

The `if __name__ == "__main__":` guard means "only run this when invoked directly, not when imported." Critical idiom — module 05 covers why.

**(b) REPL.** Interactive prompt. Best tool for exploration.

```bash
$ python
>>> import json
>>> json.dumps({"a": 1})
'{"a": 1}'
>>> exit()
```

Use `python -i hello.py` to run a script *and* drop into the REPL with its globals available. Better REPLs exist: `pip install ptpython` then run `ptpython` for autocomplete, multi-line edit, and syntax highlighting.

**(c) Module.** `python -m foo` runs the package `foo`'s `__main__.py`. This is how you should invoke tools to avoid PATH ambiguity:

```bash
python -m pip install requests   # not just `pip install`
python -m pytest                  # not `pytest`
python -m http.server 8000        # built-in static file server
```

## 3. Tooling depth — pip, pyproject, and the modern alternatives

### pip and requirements

`pip` installs from PyPI. The classic workflow:

```bash
python -m pip install requests
python -m pip freeze > requirements.txt
python -m pip install -r requirements.txt
```

`freeze` pins exact versions. Fine for apps; bad for libraries (too rigid). Modern projects use `pyproject.toml` instead.

### pyproject.toml — the modern standard (PEP 621)

A single file declares dependencies, metadata, and build config. Replaces `setup.py`, `setup.cfg`, `requirements.txt`, and most of `MANIFEST.in`.

```toml
# pyproject.toml
[project]
name = "myapp"
version = "0.1.0"
requires-python = ">=3.12"
dependencies = [
    "requests>=2.31",
    "pydantic>=2.5",
]

[project.optional-dependencies]
dev = ["pytest>=8", "mypy>=1.10", "ruff>=0.5"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
```

Install with `pip install -e .` (editable mode — changes to source are visible without reinstall) or `pip install -e ".[dev]"` to include the `dev` extras.

### When to reach for which tool

| Tool | Use when |
|------|----------|
| `pip` + `venv` | Simple, universal, zero-config. Default choice. |
| `uv` | Want it 10–100× faster, drop-in replacement for pip. Rapidly becoming the default. |
| `poetry` | Want lockfiles + dependency resolution + publishing in one tool. |
| `conda` | Scientific stack with non-Python binaries (CUDA, MKL, etc.). |
| `pipx` | Install *applications* (ruff, black, httpie) globally but isolated. |

`uv` (from Astral, written in Rust) is what most new projects in 2025–2026 reach for:

```bash
pipx install uv
uv venv             # creates .venv
uv pip install requests
uv sync             # resolves & installs from pyproject + lockfile
```

## 4. Practical Application — a minimal, runnable project

Create a real project the way you would at work:

```bash
mkdir wordcount && cd wordcount
python -m venv .venv && source .venv/bin/activate
python -m pip install --upgrade pip
```

Project layout:

```
wordcount/
├── pyproject.toml
├── src/
│   └── wordcount/
│       ├── __init__.py
│       └── __main__.py
└── tests/
    └── test_basic.py
```

```toml
# pyproject.toml
[project]
name = "wordcount"
version = "0.1.0"
requires-python = ">=3.12"

[project.scripts]
wordcount = "wordcount.__main__:main"

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[tool.hatch.build.targets.wheel]
packages = ["src/wordcount"]
```

```python
# src/wordcount/__main__.py
import sys
from collections import Counter

def main() -> None:
    text = sys.stdin.read()
    counts = Counter(text.lower().split())
    for word, n in counts.most_common(10):
        print(f"{n:>5}  {word}")

if __name__ == "__main__":
    main()
```

Install editable and run:

```bash
pip install -e .
echo "the quick brown fox the lazy dog the" | wordcount
# 3  the
# 1  quick
# ...
```

You now have a real, installable, runnable Python package. The `[project.scripts]` entry put a `wordcount` command on your PATH (inside the venv). This is exactly how `pip`, `pytest`, and `black` install themselves.

## 5. Common Mistakes & Gotchas

- **Installing into system Python.** Pollutes your OS. Always use a venv. On macOS/Linux, never `sudo pip install` — that breaks your package manager.
- **Confusing `python` and `python3`.** On many systems, `python` is Python 2 or missing. Inside an active venv, `python` is correct. Outside it, prefer `python3` or `py -3` (Windows launcher).
- **Forgetting to activate the venv.** Symptom: `pip install foo` succeeds but `import foo` fails. Check `which python`.
- **Tabs vs spaces.** Python 3 forbids mixing them in the same block. Configure your editor to insert 4 spaces. PEP 8 mandates spaces.
- **Editing `.pyc` files or worrying about `__pycache__/`.** Don't. Add `__pycache__/` to `.gitignore` and ignore it.
- **Shadowing stdlib names.** Naming your file `email.py`, `json.py`, or `string.py` will silently break imports. Symptom: weird `AttributeError` from a stdlib call.
- **`python script.py` vs `python -m pkg`.** The first sets `sys.path[0]` to the script's directory; the second sets it to the *current* directory. This causes "works on my machine" import failures. Use `-m` for anything packaged.

## 🎯 Key Takeaways

- **CPython + venv + pip + pyproject.toml** is the baseline literacy. Everything else (poetry, uv, conda) is a faster path to the same outcome.
- **Always isolate dependencies in a venv.** Per project. No exceptions. Seniors notice this in PRs.
- **`python -m foo` is the safe way to run tools** — it uses the *current* interpreter and avoids PATH games. Get into the habit.
- **`pyproject.toml` is the only config file** a new project needs. `setup.py` is legacy; `requirements.txt` is for deployment artifacts only.
- **The `if __name__ == "__main__":` guard** isn't ceremony — it's the difference between a script that's also importable and one that runs side-effects on import.

*← [roadmap](./00_roadmap.md) | [next →](./02_data_types.md)*
