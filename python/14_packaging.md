# 14 — Packaging & Distribution
> **Goal:** Turn your project into a real installable package — `pyproject.toml`, wheels, entry points, and a clean PyPI release.

## 1. The packaging story — modernized

The Python packaging history is a swamp. The modern story (2026) is simple:

- **One config file:** `pyproject.toml` (PEP 621). Forget `setup.py`, `setup.cfg`, and `requirements.txt` for source-of-truth.
- **A build backend** (hatchling, setuptools, flit, poetry-core, pdm-backend) builds a *wheel* — a zip with a `.whl` extension that pip installs.
- **`twine`** uploads to PyPI. Or modern `hatch publish` / `uv publish`.
- **`build`** is the standard CLI for producing wheels (and the older sdist tarball).

Mental model: your project + `pyproject.toml` + a build backend → wheel → PyPI → `pip install yourpkg`. That's the whole pipeline.

```bash
pip install build twine
python -m build           # produces dist/yourpkg-0.1.0.tar.gz and *.whl
twine upload dist/*       # uploads to PyPI
```

## 2. Mechanism — `pyproject.toml` end to end

A complete, realistic config:

```toml
# pyproject.toml
[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[project]
name = "yourpkg"
version = "0.1.0"
description = "A short, scannable description"
readme = "README.md"
requires-python = ">=3.12"
license = "MIT"
authors = [{ name = "Yati", email = "yati@example.com" }]
keywords = ["cli", "tools"]
classifiers = [
    "Development Status :: 3 - Alpha",
    "Programming Language :: Python :: 3 :: Only",
    "Programming Language :: Python :: 3.12",
    "Programming Language :: Python :: 3.13",
    "License :: OSI Approved :: MIT License",
]
dependencies = [
    "httpx>=0.27",
    "pydantic>=2.5",
    "rich>=13",
]

[project.optional-dependencies]
dev = [
    "pytest>=8",
    "pytest-cov",
    "mypy>=1.10",
    "ruff>=0.5",
]

[project.urls]
Homepage = "https://github.com/you/yourpkg"
Issues = "https://github.com/you/yourpkg/issues"

[project.scripts]
yourcli = "yourpkg.cli:main"        # creates a `yourcli` command on install

[tool.hatch.build.targets.wheel]
packages = ["src/yourpkg"]

[tool.pytest.ini_options]
testpaths = ["tests"]
addopts = "-ra --strict-markers"

[tool.mypy]
strict = true
files = ["src", "tests"]

[tool.ruff]
target-version = "py312"
line-length = 100
```

Important fields:

- **`name`**: must be unique on PyPI. Hyphens; the import name is the package directory (typically lowercased without hyphens).
- **`version`**: PEP 440 string. SemVer is conventional but not enforced.
- **`requires-python`**: pip refuses to install on incompatible interpreters. Set it.
- **`dependencies`**: runtime requirements. Use `>=X` minimums; avoid `==` in libraries (locks downstream users).
- **`optional-dependencies`**: groups installed via `pip install yourpkg[dev]` or `pip install yourpkg[test,dev]`.
- **`[project.scripts]`**: each entry installs a console command. `yourcli = "yourpkg.cli:main"` means "create a `yourcli` command that runs `yourpkg.cli.main()`."
- **`[project.urls]`**: shows up on the PyPI page.

## 3. Variations — choosing a build backend, src layout, versioning

### Build backends

All produce identical wheels — they differ in features and ergonomics:

| Backend | Best for |
|---------|----------|
| **hatchling** | Modern default. Fast, no boilerplate, dynamic versioning. Recommended starting point. |
| **setuptools** | Legacy projects, C extensions, anything with deeply customized builds. |
| **flit-core** | Pure-Python, single-module simplicity. |
| **poetry-core** | If you're already using poetry for envs/locking. |
| **pdm-backend** | If you're using pdm. |
| **maturin** | Rust + Python (PyO3). |

For a new pure-Python project, use hatchling.

### `src/` layout vs flat layout

Recommended: **`src/` layout.**

```
yourpkg/
├── pyproject.toml
├── README.md
├── src/
│   └── yourpkg/
│       ├── __init__.py
│       └── ...
└── tests/
```

vs. flat:

```
yourpkg/
├── pyproject.toml
├── yourpkg/
│   └── __init__.py
└── tests/
```

Why `src/`: it forces an install (`pip install -e .`) before the package is importable. That means tests run against the *installed* package — exactly what users will see — instead of accidentally importing from the source tree. Catches missing files, broken metadata, and import path bugs early.

### Editable installs in 2026

```bash
pip install -e .            # edits to source visible immediately
pip install -e ".[dev]"     # plus the dev extras
```

Modern editable installs use PEP 660. With hatchling, this just works.

### Wheels and sdists

`python -m build` produces two artifacts in `dist/`:

- **`yourpkg-0.1.0.tar.gz`** — source distribution (sdist). Required on PyPI as a fallback.
- **`yourpkg-0.1.0-py3-none-any.whl`** — the wheel. Pre-built, fast install. Pure-Python wheels are universal (`any` platform).

For projects with C/Rust extensions, wheels are platform-specific (`yourpkg-0.1.0-cp312-cp312-manylinux_2_17_x86_64.whl`). Build for each platform you care about, often via GitHub Actions + `cibuildwheel`.

### Versioning strategies

- **Manual version in `pyproject.toml`**: simple, fine for early projects. Bump it when you cut a release.
- **Dynamic version from git tags** (`hatch-vcs`): version derived from the latest git tag. No manual bumping, no version-skew bugs:

  ```toml
  [project]
  dynamic = ["version"]

  [tool.hatch.version]
  source = "vcs"

  [build-system]
  requires = ["hatchling", "hatch-vcs"]
  build-backend = "hatchling.build"
  ```

- **`__version__` attribute**: expose for runtime introspection:

  ```python
  # src/yourpkg/__init__.py
  from importlib.metadata import version
  __version__ = version("yourpkg")
  ```

### The TestPyPI workflow

Never test releases against real PyPI. Use the test instance:

```bash
twine upload --repository testpypi dist/*
pip install -i https://test.pypi.org/simple/ yourpkg
```

Once you're happy:

```bash
twine upload dist/*
```

Use **API tokens**, not your password. Create them on PyPI under Account Settings → API tokens.

### Trusted publishing (CI without secrets)

PyPI supports OIDC-based trusted publishing from GitHub Actions — no API tokens at all. Configure once on PyPI; in CI:

```yaml
# .github/workflows/release.yml
on:
  push:
    tags: ["v*"]
permissions:
  id-token: write       # required for OIDC

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with: { python-version: "3.13" }
      - run: pip install build
      - run: python -m build
      - uses: pypa/gh-action-pypi-publish@release/v1
```

This is the modern, secret-less release pipeline.

## 4. Practical Application — building and publishing a CLI tool

A complete, releasable mini-package. The CLI counts words in stdin and outputs a top-N list.

```
wordfreq/
├── pyproject.toml
├── README.md
├── LICENSE
├── src/
│   └── wordfreq/
│       ├── __init__.py
│       └── cli.py
└── tests/
    └── test_cli.py
```

```toml
# pyproject.toml
[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[project]
name = "wordfreq"
version = "0.1.0"
description = "Tiny word-frequency CLI"
readme = "README.md"
requires-python = ">=3.12"
license = "MIT"
authors = [{ name = "Yati" }]
dependencies = []

[project.optional-dependencies]
dev = ["pytest>=8"]

[project.scripts]
wordfreq = "wordfreq.cli:main"

[tool.hatch.build.targets.wheel]
packages = ["src/wordfreq"]
```

```python
# src/wordfreq/__init__.py
__version__ = "0.1.0"
```

```python
# src/wordfreq/cli.py
import argparse
import sys
from collections import Counter

def count(text: str) -> Counter[str]:
    return Counter(w.lower() for w in text.split())

def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="word-frequency from stdin")
    parser.add_argument("-n", "--top", type=int, default=10)
    args = parser.parse_args(argv)

    counts = count(sys.stdin.read())
    for word, n in counts.most_common(args.top):
        print(f"{n:>5}  {word}")
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
```

```python
# tests/test_cli.py
from wordfreq.cli import count

def test_count_basic():
    c = count("the quick brown the lazy the")
    assert c["the"] == 3
    assert c["quick"] == 1
```

Build and check locally:

```bash
pip install build twine
python -m build
ls dist/
# wordfreq-0.1.0-py3-none-any.whl  wordfreq-0.1.0.tar.gz

# Install from the wheel into a fresh venv to verify
python -m venv /tmp/check && source /tmp/check/bin/activate
pip install dist/*.whl
echo "the quick brown the lazy the" | wordfreq -n 3
#     3  the
#     1  quick
#     1  brown

# Upload to TestPyPI to verify metadata
twine upload --repository testpypi dist/*

# Real release
twine upload dist/*
```

Now `pip install wordfreq` works for anyone, and they get a `wordfreq` command on their PATH.

## 5. Common Mistakes & Gotchas

- **Pinning dependencies with `==` in libraries.** Forces conflicts on downstream users. Use `>=` minimums; pin only in *applications* via a lockfile.
- **Forgetting `requires-python`.** Pip happily installs onto an incompatible interpreter, then your code crashes on first import.
- **Flat layout + tests that pass locally but fail on install.** Symptom: a missing file in `MANIFEST.in` or `tool.hatch.build`. The `src/` layout makes this impossible.
- **Uploading to real PyPI when you meant TestPyPI.** Once a version is on PyPI, you cannot reupload it (you must bump the version). Use TestPyPI first.
- **No `LICENSE` file.** Some users (and some `pip-licenses` audits) treat unlicensed as proprietary.
- **Versioning out of sync with git tags.** Symptom: a release marked `0.2.0` on PyPI doesn't match any commit. Use `hatch-vcs` or a release script.
- **`setup.py develop` / `python setup.py install`.** Old API. Use `pip install -e .` and `python -m build`.
- **Including secrets / build artifacts / venvs in the wheel.** Check `python -m build` output and inspect the resulting wheel (it's just a zip).
- **Publishing without a `README.md` declared as `readme`.** PyPI shows the package's "long description" from this file — without it, the project page is blank.
- **Putting test files inside the package.** They get installed for users who don't need them. Keep `tests/` outside `src/`.

## 🎯 Key Takeaways

- **`pyproject.toml` + hatchling + `src/` layout is the modern default.** Set up a new project this way once; copy the template forever.
- **Build backends are interchangeable; dependencies aren't.** Pick hatchling for new pure-Python projects. Switch only when you have a reason (Rust, complex C, monorepo tooling).
- **`[project.scripts]` is how you ship CLIs.** Cleaner than telling users to `python -m yourpkg`. Every installed Python tool you use is doing this.
- **Use TestPyPI before real PyPI, and trusted publishing instead of long-lived tokens.** Both are free, both prevent classes of mistakes.
- **Pin in apps, range in libraries.** Applications use a lockfile (uv lock, poetry lock, pip-tools); libraries declare lower bounds and let users compose.

*← [prev](./13_testing.md) | [next →](./15_performance.md)*
