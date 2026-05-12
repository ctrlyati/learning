# 14 — Testing Django

> **Goal:** Build a fast, reliable test suite using `TestCase`, `Client`, factories, and `pytest-django` — at every layer.

---

## 1. Django's testing toolbox

Django ships a substantial testing layer:

- **`SimpleTestCase`** — no DB access; for pure-logic tests.
- **`TestCase`** — wraps each test in a transaction that rolls back; fastest with DB.
- **`TransactionTestCase`** — truncates tables between tests; needed when your code uses `transaction.atomic()` you want to observe commits of.
- **`LiveServerTestCase`** — launches a real WSGI server for browser tests.
- **`Client`** — fake HTTP client for end-to-end view tests.
- **`RequestFactory`** — synthesizes a `request` object without going through the middleware stack — fast unit tests for views.

Run them:

```bash
python manage.py test                          # discovers tests/ or tests.py
python manage.py test blog                     # one app
python manage.py test blog.tests.test_views    # one module
python manage.py test --parallel               # parallel runner
python manage.py test --keepdb                 # reuse the test DB (faster re-runs)
```

The test runner creates a separate test database (named `test_<your_db>`), runs migrations, executes tests, drops the DB. `--keepdb` skips creation/drop on subsequent runs.

---

## 2. A first model + view test

```python
# blog/tests/test_models.py
from django.test import TestCase
from django.contrib.auth import get_user_model
from blog.models import Post

User = get_user_model()

class PostModelTests(TestCase):
    def setUp(self):
        self.user = User.objects.create_user("alice", password="x")

    def test_get_absolute_url(self):
        post = Post.objects.create(
            title="Hi", slug="hi", body="...", author=self.user, status="published"
        )
        self.assertEqual(post.get_absolute_url(), "/blog/post/hi/")

    def test_str(self):
        post = Post.objects.create(title="Hi", slug="hi", body="...", author=self.user)
        self.assertEqual(str(post), "Hi")
```

```python
# blog/tests/test_views.py
from django.test import TestCase
from django.urls import reverse
from django.contrib.auth import get_user_model
from blog.models import Post

User = get_user_model()

class PostViewTests(TestCase):
    @classmethod
    def setUpTestData(cls):
        # Created once for the entire class — much faster than setUp for read-only fixtures.
        cls.user = User.objects.create_user("alice", password="x")
        cls.post = Post.objects.create(
            title="Hi", slug="hi", body="...", author=cls.user, status="published"
        )

    def test_index_renders(self):
        url = reverse("blog:index")
        response = self.client.get(url)
        self.assertEqual(response.status_code, 200)
        self.assertContains(response, "Hi")

    def test_detail_404_for_draft(self):
        Post.objects.create(title="D", slug="d", body=".", author=self.user, status="draft")
        url = reverse("blog:detail", kwargs={"slug": "d"})
        self.assertEqual(self.client.get(url).status_code, 404)

    def test_login_required_for_create(self):
        url = reverse("blog:create")
        response = self.client.get(url)
        self.assertRedirects(response, f"/accounts/login/?next={url}")

    def test_create_requires_owner(self):
        self.client.login(username="alice", password="x")
        url = reverse("blog:create")
        response = self.client.post(url, {"title": "T", "slug": "t", "body": "b", "status": "draft"})
        self.assertEqual(Post.objects.filter(slug="t").count(), 1)
        post = Post.objects.get(slug="t")
        self.assertEqual(post.author, self.user)
```

Key idioms here:

- **`setUpTestData`** vs **`setUp`** — `setUpTestData` runs once per class and lives in the surrounding test transaction; `setUp` runs per test. Use `setUpTestData` for read-only baseline data — huge speed win.
- **`self.client`** — Django's `Client`, already wired with sessions and CSRF.
- **`self.client.login(...)`** — log in as a user; sets the session cookie.
- **`reverse(name)`** — never hardcode URLs in tests either.

---

## 3. `Client` — the full request/response harness

```python
client.get("/path/", {"q": "foo"})            # GET with query params
client.post("/path/", {"title": "x"})         # POST form data
client.post("/path/", data=json.dumps({...}), content_type="application/json")
client.put(...), client.patch(...), client.delete(...)

response = client.get("/path/")
response.status_code
response.content                              # bytes
response.json()                               # if JSON
response.context["form"]                      # the rendered template's context
response.templates                            # list of templates used
response.redirect_chain                       # if follow=True

# Helpers
self.assertContains(response, "Hello", count=1)
self.assertNotContains(response, "Secret")
self.assertRedirects(response, expected_url)
self.assertTemplateUsed(response, "blog/index.html")
```

For DRF, use the DRF client (handles authentication classes):

```python
from rest_framework.test import APIClient, APITestCase

class PostAPITests(APITestCase):
    def setUp(self):
        self.user = User.objects.create_user("alice", password="x")
        self.client.force_authenticate(self.user)   # skip auth flow

    def test_create_post(self):
        response = self.client.post("/api/posts/", {"title": "t", "body": "b"}, format="json")
        self.assertEqual(response.status_code, 201)
        self.assertEqual(response.data["author"], "alice")
```

---

## 4. `RequestFactory` — unit-test views without the middleware

For tight unit tests on a view function's logic (no middleware, no DB query overhead through full request cycle):

```python
from django.test import RequestFactory
from django.contrib.auth.models import AnonymousUser

class IndexViewUnitTest(TestCase):
    def setUp(self):
        self.factory = RequestFactory()

    def test_anonymous_redirects(self):
        request = self.factory.get("/some-protected-path/")
        request.user = AnonymousUser()
        response = some_view(request)
        self.assertEqual(response.status_code, 302)
```

You construct the `request` object directly. No URL resolution, no middleware run, no session. Faster but skips end-to-end realism. Use `Client` for "does this URL work?", `RequestFactory` for "does this view function behave?"

---

## 5. Fixtures vs factories

### Django fixtures — declarative JSON

```bash
python manage.py dumpdata blog > blog/fixtures/initial.json
```

```python
class MyTest(TestCase):
    fixtures = ["initial.json"]    # loaded before each test
```

Fixtures are fine for tiny lookup tables but become brittle as models evolve — a model field rename breaks every fixture.

### Factories — `factory_boy`

The senior pattern. Define factories that produce test objects with sensible defaults:

```bash
pip install factory_boy
```

```python
# blog/tests/factories.py
import factory
from django.contrib.auth import get_user_model
from blog.models import Post

User = get_user_model()

class UserFactory(factory.django.DjangoModelFactory):
    class Meta:
        model = User

    username = factory.Sequence(lambda n: f"user{n}")
    email = factory.LazyAttribute(lambda o: f"{o.username}@example.com")
    password = factory.PostGenerationMethodCall("set_password", "x")

class PostFactory(factory.django.DjangoModelFactory):
    class Meta:
        model = Post

    title = factory.Faker("sentence", nb_words=4)
    slug = factory.Sequence(lambda n: f"post-{n}")
    body = factory.Faker("paragraph", nb_sentences=5)
    author = factory.SubFactory(UserFactory)
    status = "published"
```

```python
# Usage in tests
from .factories import PostFactory, UserFactory

post = PostFactory()                              # all defaults
post = PostFactory(title="Specific", author=u)    # override
posts = PostFactory.create_batch(20)              # 20 posts at once
```

Factories scale: every test gets exactly the data it needs without manual setup churn.

---

## 6. `pytest-django` — the senior default

`pytest` has cleaner assertions (`assert x == y` instead of `self.assertEqual`), better fixtures, parameterization, and a huge plugin ecosystem.

```bash
pip install pytest pytest-django
```

```ini
# pytest.ini
[pytest]
DJANGO_SETTINGS_MODULE = mysite.settings.test
python_files = test_*.py *_tests.py
```

```python
# blog/tests/test_views_pytest.py
import pytest
from django.urls import reverse
from .factories import PostFactory, UserFactory

@pytest.fixture
def alice(db):
    return UserFactory(username="alice")

@pytest.mark.django_db
def test_index_lists_published(client):
    PostFactory.create_batch(3, status="published")
    PostFactory(status="draft")
    response = client.get(reverse("blog:index"))
    assert response.status_code == 200
    assert response.context["posts"].count() == 3

@pytest.mark.django_db
def test_create_requires_login(client):
    response = client.get(reverse("blog:create"))
    assert response.status_code == 302
    assert "/login/" in response["Location"]

@pytest.mark.parametrize("status,expected", [
    ("published", 200),
    ("draft", 404),
])
@pytest.mark.django_db
def test_detail_status(client, status, expected):
    post = PostFactory(status=status)
    response = client.get(reverse("blog:detail", kwargs={"slug": post.slug}))
    assert response.status_code == expected
```

Common pytest-django fixtures:

- `db` — gives the test DB access (rolls back).
- `client` — Django test client.
- `admin_client` — logged-in superuser client.
- `django_user_model` — `User`, lazily resolved.
- `rf` — `RequestFactory`.
- `settings` — modify settings for one test: `settings.DEBUG = True`.

Run:

```bash
pytest                  # all tests
pytest -k auth          # tests with "auth" in the name
pytest -x               # stop on first failure
pytest --reuse-db       # like --keepdb
pytest -n 4             # parallel (pip install pytest-xdist)
```

---

## 7. Mocking external services

Don't hit real APIs in tests. Use `unittest.mock` or `responses`:

```python
from unittest.mock import patch

@patch("blog.services.send_email")
def test_publish_sends_email(self, mock_send):
    post = PostFactory(status="draft")
    post.publish()
    mock_send.assert_called_once_with(post.author.email, "...")
```

For HTTP-level mocking:

```python
import responses

@responses.activate
def test_fetches_data():
    responses.add(responses.GET, "https://api.example.com/x", json={"ok": True})
    result = my_service_that_calls_api()
    assert result == {"ok": True}
```

---

## 8. Database transactions in tests

Each `TestCase` test runs inside a transaction that rolls back at teardown. That's why tests are fast — no `DELETE FROM ...` between cases.

But: if your *production code* calls `transaction.on_commit`, the callbacks **don't fire** in `TestCase` because the outer transaction never commits. To test them:

```python
class MyTest(TestCase):
    def test_on_commit(self):
        with self.captureOnCommitCallbacks(execute=True) as callbacks:
            do_thing_that_queues_callback()
        self.assertEqual(len(callbacks), 1)
```

Or use `TransactionTestCase` if you need real commits (slow — truncates tables per test).

---

## 9. Coverage and CI

```bash
pip install coverage
coverage run --source=. manage.py test
coverage report -m
coverage html
```

`pytest-cov` integrates with pytest:

```bash
pip install pytest-cov
pytest --cov=blog --cov-report=term-missing
```

In CI, gate PRs on test passage + coverage threshold + `makemigrations --check`:

```yaml
- run: python manage.py makemigrations --check --dry-run
- run: pytest --cov=. --cov-fail-under=80
```

---

## 10. Practical application — a complete test module

```python
# blog/tests/test_post_lifecycle.py
import pytest
from django.urls import reverse
from blog.models import Post
from .factories import PostFactory, UserFactory

pytestmark = pytest.mark.django_db

class TestPostLifecycle:
    def test_anon_sees_only_published(self, client):
        PostFactory.create_batch(2, status="published")
        PostFactory.create_batch(3, status="draft")
        r = client.get(reverse("blog:index"))
        assert r.context["posts"].count() == 2

    def test_owner_can_edit(self, client):
        author = UserFactory(username="alice")
        post = PostFactory(author=author)
        client.force_login(author)
        r = client.post(
            reverse("blog:edit", kwargs={"pk": post.pk}),
            {"title": "Updated", "slug": post.slug, "body": "...", "status": "published"},
        )
        post.refresh_from_db()
        assert post.title == "Updated"

    def test_non_owner_cannot_edit(self, client):
        bob = UserFactory(username="bob")
        post = PostFactory()                  # owned by a fresh user
        client.force_login(bob)
        r = client.post(
            reverse("blog:edit", kwargs={"pk": post.pk}),
            {"title": "Hijacked", "slug": "x", "body": "x", "status": "published"},
        )
        assert r.status_code in (403, 404)

    @pytest.mark.parametrize("status,visible", [("published", True), ("draft", False)])
    def test_detail_visibility(self, client, status, visible):
        post = PostFactory(status=status)
        r = client.get(reverse("blog:detail", kwargs={"slug": post.slug}))
        assert (r.status_code == 200) is visible
```

This covers happy path, authorization, parametrized variants — about a third of a real production test suite for a feature.

---

## 11. Common mistakes and gotchas

1. **Hitting real services in tests.** Slow, flaky, costs money. Mock or use a fake.
2. **Per-test `setUp` for read-only data.** Each test reseeds the same rows. Use `setUpTestData` (or pytest class/session fixtures) — 10× faster.
3. **No `--parallel` / `pytest -n`.** Single-process test runs scale linearly with test count. Parallel cuts run time on multi-core.
4. **Brittle assertions on response bytes.** `assertEqual(response.content, b"...big html...")` breaks on any whitespace change. Use `assertContains`/`assertTemplateUsed`/`response.context`.
5. **Tests that depend on order.** A passing-by-accident test that fails when run alone is a bug. Tests must be independent.
6. **Forgetting `@pytest.mark.django_db`.** Pytest-django won't give you DB access without the marker — symptoms: `DatabaseError` in tests.
7. **Mixing test class data and module-level data.** A `Post.objects.create(...)` at module scope runs at import — before the test DB is set up. Wrap in factories or fixtures.
8. **Real network in `setUp`.** Yes, this happens. `requests.get(...)` in `setUp` makes your test suite depend on the internet. Mock at module level.
9. **Not testing migrations.** `makemigrations --check` in CI catches "you forgot to commit a migration."
10. **Testing internals instead of behavior.** `assertEqual(view._queryset.query, "...")` ties tests to implementation. Test the response and DB state, not internal attrs.
11. **`transaction.on_commit` callbacks not running.** Use `captureOnCommitCallbacks(execute=True)` or `TransactionTestCase`.
12. **Forgetting to test the negative path.** "Owner can edit" is fine; "non-owner cannot edit" is the actually-important test.

---

## 🎯 Key Takeaways

- **Use `pytest-django` + `factory_boy` for new projects.** Cleaner assertions, better fixtures, easier parameterization. Django's built-in `TestCase` works too — but pytest is the senior default.
- **`setUpTestData` for read-only baseline data.** Class-scoped, one transaction — usually 5-10× faster than per-test `setUp`.
- **`Client` for end-to-end view tests, `RequestFactory` for unit-level view logic.** Both have their place.
- **Factories beat fixtures.** Brittle JSON fixtures break on schema changes; factories adapt because they're Python code.
- **Mock external services and test the negative paths.** "It works when everything is happy" is the easy half. "It refuses when unauthorized" is the half that catches breaches.

*← [prev](./13_async_and_channels.md) | [next →](./15_security.md)*
