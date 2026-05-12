# 13 — Testing with pytest
> **Goal:** Write tests a senior would approve — using pytest, fixtures, parametrize, mocking, and coverage — so refactors stop being scary.

## 1. Why pytest, and the minimal test

`unittest` is in the stdlib but verbose. `pytest` is the de facto standard:

- Tests are plain functions named `test_*`.
- Assertions are plain `assert` statements.
- Rich fixtures, parametrize, and a huge plugin ecosystem.

```bash
pip install pytest
```

```python
# src/calc.py
def add(a: int, b: int) -> int:
    return a + b

# tests/test_calc.py
from calc import add

def test_add_positives():
    assert add(2, 3) == 5

def test_add_negative():
    assert add(-1, 1) == 0
```

```bash
pytest
# or with verbose output and stop on first failure:
pytest -v -x
```

Project structure (from module 5 — same idea):

```
myproj/
├── pyproject.toml
├── src/
│   └── myproj/
│       └── ...
└── tests/
    ├── conftest.py
    └── test_*.py
```

Add to `pyproject.toml`:

```toml
[tool.pytest.ini_options]
testpaths = ["tests"]
addopts = "-ra --strict-markers"
```

`-ra` shows summary of all non-passing outcomes; `--strict-markers` fails on typos in `@pytest.mark.foo`.

## 2. Mechanism — fixtures, parametrize, and assertions

### Fixtures — reusable setup, scoped right

A fixture is a function decorated with `@pytest.fixture` whose return value is injected by name:

```python
import pytest
from pathlib import Path

@pytest.fixture
def sample_data():
    return [1, 2, 3, 4, 5]

def test_sum(sample_data):
    assert sum(sample_data) == 15

def test_max(sample_data):
    assert max(sample_data) == 5
```

Pytest sees `sample_data` in the test's parameter list and runs the fixture to provide it.

**Scopes** control how often a fixture runs:

```python
@pytest.fixture(scope="function")   # default — per test
@pytest.fixture(scope="module")     # per .py file
@pytest.fixture(scope="session")    # once per pytest run
```

Use module/session scopes for expensive setup (DB containers, large files). Default to `function` for isolation.

### Setup/teardown with `yield`

```python
@pytest.fixture
def temp_db(tmp_path):
    db = tmp_path / "test.db"
    conn = sqlite3.connect(db)
    conn.execute("CREATE TABLE x (id INTEGER)")
    yield conn                      # test runs here
    conn.close()                    # teardown
```

`tmp_path` is a built-in fixture giving you a fresh per-test directory. Combine your own fixtures with it for isolated, deterministic tests.

Common built-in fixtures: `tmp_path`, `tmp_path_factory`, `monkeypatch`, `capsys` (capture stdout/stderr), `caplog` (capture log records).

### `conftest.py` — shared fixtures

Fixtures in `tests/conftest.py` are auto-discovered by all tests in `tests/`. Put cross-file fixtures here:

```python
# tests/conftest.py
import pytest

@pytest.fixture
def user():
    return {"id": 1, "email": "test@example.com"}
```

### Parametrize — one test, many inputs

```python
import pytest
from calc import add

@pytest.mark.parametrize("a, b, expected", [
    (1, 1, 2),
    (0, 0, 0),
    (-1, 1, 0),
    (1_000_000, 1, 1_000_001),
])
def test_add(a, b, expected):
    assert add(a, b) == expected
```

Each tuple becomes a separate test case. Failures report which parameters failed. This is the single biggest leverage in pytest — never write five near-identical test functions.

You can stack parametrize for the cross-product:

```python
@pytest.mark.parametrize("x", [1, 2, 3])
@pytest.mark.parametrize("y", [10, 20])
def test_pair(x, y):
    # 6 tests: (1,10), (1,20), (2,10), (2,20), (3,10), (3,20)
    ...
```

### Asserting exceptions

```python
import pytest

def test_divide_by_zero():
    with pytest.raises(ZeroDivisionError):
        1 / 0

def test_validation_message():
    with pytest.raises(ValueError, match=r"must be positive"):
        validate(-1)
```

`match=` is a regex against the exception's string form — keeps the test honest.

## 3. Variations — mocking, marks, coverage

### Mocking — `unittest.mock` (still the standard) and `monkeypatch`

```python
from unittest.mock import patch, MagicMock

def fetch_user(client, user_id):
    return client.get(f"/users/{user_id}").json()

def test_fetch_user():
    client = MagicMock()
    client.get.return_value.json.return_value = {"id": 1, "name": "Yati"}
    user = fetch_user(client, 1)
    assert user["name"] == "Yati"
    client.get.assert_called_once_with("/users/1")
```

For patching globals/imports:

```python
def test_uses_uuid(monkeypatch):
    monkeypatch.setattr("mymodule.uuid4", lambda: "fixed-id")
    assert mymodule.new_id() == "fixed-id"
```

`monkeypatch` is the pytest-flavored, automatically-undone version of `mock.patch`. Use it for env vars and attribute swaps:

```python
def test_reads_env(monkeypatch):
    monkeypatch.setenv("API_KEY", "test-key")
    assert config.load().api_key == "test-key"
```

**Mock at the right boundary.** Mock the HTTP client, not your own service code. Mock the DB driver, not your repository. Tests that mock everything pass while the real code is broken.

### Marks — group and skip

```python
import pytest

@pytest.mark.slow
def test_big_thing(): ...

@pytest.mark.skip(reason="WIP")
def test_unfinished(): ...

@pytest.mark.skipif(sys.platform == "win32", reason="POSIX only")
def test_unix_only(): ...

@pytest.mark.xfail(reason="known bug, see #123")
def test_known_broken(): ...
```

Run only fast tests: `pytest -m "not slow"`. Register marks in `pyproject.toml` to satisfy `--strict-markers`:

```toml
[tool.pytest.ini_options]
markers = ["slow: integration-ish tests"]
```

### Coverage

```bash
pip install pytest-cov
pytest --cov=src --cov-report=term-missing --cov-report=html
```

`--cov-report=term-missing` shows the line numbers you didn't hit. `htmlcov/index.html` is browsable.

Coverage targets:

- **Don't chase 100%.** It rewards trivial tests and `# pragma: no cover` games.
- **Aim for 80–90% with meaningful assertions.** Coverage tells you what's *unexercised*, not what's *correct*.
- **Branch coverage** (`--cov-branch`) catches missed `if/else` branches that line coverage hides.

### Property-based testing — Hypothesis (worth knowing)

```python
from hypothesis import given, strategies as st

@given(xs=st.lists(st.integers()))
def test_sorted_idempotent(xs):
    assert sorted(sorted(xs)) == sorted(xs)
```

Hypothesis generates random inputs and shrinks failures to minimal examples. Excellent for parsers, serialization, and data transforms — anything with structural properties.

## 4. Practical Application — testing a service with fakes and parametrize

Realistic example: a `UserService` that depends on a `Repository` (from module 9). Test it with a fake repo, parametrized cases, and a mock for an external HTTP call:

```python
# src/myapp/users.py
from dataclasses import dataclass
from typing import Protocol

@dataclass(frozen=True)
class User:
    id: int
    email: str

class Repo(Protocol):
    def get(self, id: int) -> User | None: ...
    def add(self, u: User) -> None: ...

class EmailClient(Protocol):
    def send(self, to: str, subject: str, body: str) -> bool: ...

class UserService:
    def __init__(self, repo: Repo, email: EmailClient) -> None:
        self.repo = repo
        self.email = email

    def register(self, id: int, email: str) -> User:
        if self.repo.get(id) is not None:
            raise ValueError(f"user {id} already exists")
        if "@" not in email:
            raise ValueError("invalid email")
        u = User(id, email)
        self.repo.add(u)
        self.email.send(email, "Welcome", f"Hello {email}")
        return u
```

Tests:

```python
# tests/test_users.py
import pytest
from unittest.mock import MagicMock
from myapp.users import User, UserService

class FakeRepo:
    def __init__(self):
        self._db: dict[int, User] = {}
    def get(self, id):     return self._db.get(id)
    def add(self, u):      self._db[u.id] = u

@pytest.fixture
def repo():
    return FakeRepo()

@pytest.fixture
def email():
    m = MagicMock()
    m.send.return_value = True
    return m

@pytest.fixture
def service(repo, email):
    return UserService(repo, email)


def test_register_creates_user(service, repo, email):
    u = service.register(1, "yati@example.com")
    assert u == User(1, "yati@example.com")
    assert repo.get(1) == u
    email.send.assert_called_once()


def test_register_duplicate_raises(service):
    service.register(1, "a@example.com")
    with pytest.raises(ValueError, match="already exists"):
        service.register(1, "b@example.com")


@pytest.mark.parametrize("bad_email", ["", "no-at-sign", "missing-domain@"])
def test_register_invalid_email_raises(service, bad_email):
    with pytest.raises(ValueError, match="invalid email"):
        service.register(1, bad_email)


def test_email_failure_does_not_swallow(service, email):
    email.send.side_effect = RuntimeError("smtp down")
    with pytest.raises(RuntimeError):
        service.register(1, "y@example.com")
```

Why this is good:

- **Fake repo** (a hand-written class) for behavior the test cares about; **mock email client** for behavior we're just verifying calls on. Two tools, two purposes.
- **Fixtures compose:** `service` depends on `repo` and `email` — pytest wires them automatically.
- **Parametrize for invalid-input cases** — one function, three meaningful test cases, clean failure output.
- **`match=` on `pytest.raises`** so we test *which* error, not just that *some* error happened.
- **The mock's `side_effect`** simulates failures cleanly without monkey-patching real clients.

## 5. Common Mistakes & Gotchas

- **Tests that depend on each other / on order.** Fix the offending fixture scope or shared global state. Run with `pytest --random-order` (plugin) to catch this.
- **Mocking your own code.** You're now testing the mock. Mock at I/O boundaries (HTTP, DB, filesystem); test your code with the real thing.
- **Asserting `mock.assert_called`.** Doesn't exist as a method — typo → returns `MagicMock` and silently passes. Use `assert_called_once_with` etc., or run with `--strict-markers` and a recent mock that warns.
- **Sleeping in tests for async/eventing.** Flaky. Use proper sync points, fakes that resolve immediately, or `freezegun`/`time-machine` for time.
- **One giant test function.** If you find yourself writing `assert ...; assert ...; assert ...` covering different cases, split into multiple tests or parametrize.
- **No teardown.** Lingering files, DB rows, or env vars contaminate later tests. Use `tmp_path`, `monkeypatch`, and `yield` fixtures.
- **`time.sleep`, `requests.get` to real URLs, real DB calls** in unit tests. Tests should be hermetic. Network in tests = flaky CI.
- **Chasing coverage percentage with empty tests.** `def test_x(): assert True`. Reviewers spot it; mutation testing (`mutmut`, `cosmic-ray`) catches it.
- **`from x import *` in tests.** Same problem as anywhere. Use explicit imports.
- **Forgetting `pytest -ra`** — you'll miss skipped/xfailed test info that should prompt cleanup.

## 🎯 Key Takeaways

- **`pytest` + fixtures + `parametrize` + `tmp_path` + `monkeypatch` is the daily kit.** Master those five and you can test almost anything cleanly.
- **Inject dependencies (`Protocol` types, module 9) so tests can pass fakes.** Code that's hard to test is usually code that's tightly coupled — testing pressure improves design.
- **Mock at the boundary, not internally.** Mock the HTTP client; don't mock your own service. The line between "unit" and "integration" matters less than the line between "your code" and "the world."
- **`parametrize` is the leverage.** Five test functions doing the same thing with different inputs is a smell — one parametrized test is the cure.
- **Coverage tells you what's unexercised, not what's correct.** 85% with thoughtful assertions beats 100% with `assert True`. Aim there, then graduate to property-based testing for the parts that benefit.

*← [prev](./12_concurrency.md) | [next →](./14_packaging.md)*
