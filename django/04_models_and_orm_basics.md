# 04 — Models and ORM Basics

> **Goal:** Define a schema once in Python, generate it as SQL via migrations, and CRUD it via the ORM.

---

## 1. A model is a Python class that mirrors a database table

```python
# blog/models.py
from django.db import models
from django.conf import settings

class Category(models.Model):
    name = models.CharField(max_length=80, unique=True)
    slug = models.SlugField(unique=True)

    class Meta:
        verbose_name_plural = "Categories"
        ordering = ["name"]

    def __str__(self):
        return self.name


class Post(models.Model):
    STATUS_CHOICES = [
        ("draft", "Draft"),
        ("published", "Published"),
    ]

    title = models.CharField(max_length=200)
    slug = models.SlugField(unique=True)
    body = models.TextField()
    status = models.CharField(max_length=10, choices=STATUS_CHOICES, default="draft")
    author = models.ForeignKey(
        settings.AUTH_USER_MODEL,
        on_delete=models.CASCADE,
        related_name="posts",
    )
    category = models.ForeignKey(
        Category, on_delete=models.SET_NULL, null=True, blank=True, related_name="posts"
    )
    tags = models.ManyToManyField("Tag", blank=True, related_name="posts")
    created_at = models.DateTimeField(auto_now_add=True)
    updated_at = models.DateTimeField(auto_now=True)
    view_count = models.PositiveIntegerField(default=0)

    class Meta:
        ordering = ["-created_at"]
        indexes = [models.Index(fields=["status", "-created_at"])]
        constraints = [
            models.UniqueConstraint(fields=["author", "slug"], name="unique_author_slug"),
        ]

    def __str__(self):
        return self.title


class Tag(models.Model):
    name = models.CharField(max_length=40, unique=True)

    def __str__(self):
        return self.name
```

A few things to notice immediately:

- Every model implicitly gets an `id` primary key (`BigAutoField` by default in Django 5).
- `ForeignKey` requires `on_delete=` — Django won't guess what should happen when the parent is deleted.
- `related_name` is what you call the **reverse** accessor: `user.posts.all()`, not `user.post_set.all()`.
- `class Meta` is where ordering, indexes, constraints, table name, and admin labels live.
- `__str__` is what `repr(obj)` and the admin show.

---

## 2. The field catalogue

The most-used fields:

| Field | Backing SQL | Notes |
|-------|-------------|-------|
| `CharField(max_length=N)` | `varchar(N)` | Required `max_length` |
| `TextField()` | `text` | No `max_length` |
| `SlugField()` | `varchar(50)` | URL-safe; auto-indexed |
| `IntegerField`, `BigIntegerField`, `PositiveIntegerField` | `integer` variants | |
| `FloatField`, `DecimalField(max_digits, decimal_places)` | `float`/`numeric` | Money → `Decimal` always |
| `BooleanField(default=False)` | `bool` | Don't leave `default` unset |
| `DateField`, `DateTimeField`, `TimeField` | `date`/`timestamp` | `auto_now`/`auto_now_add` for audit |
| `EmailField`, `URLField`, `UUIDField`, `IPAddressField` | `varchar` with validators | |
| `FileField(upload_to=)`, `ImageField` | `varchar` (the path) | File lives in `MEDIA_ROOT` |
| `JSONField()` | `jsonb` on Postgres | Filter with `data__key="x"` |
| `ForeignKey(To, on_delete=)` | `integer + FK` | Many-to-one |
| `OneToOneField(To, on_delete=)` | `integer + FK + unique` | Profile pattern |
| `ManyToManyField(To)` | Implicit through-table | |

Common per-field options:

- `null=True` — DB allows `NULL`. **Avoid on string fields** — use `blank=True` instead.
- `blank=True` — form/admin allows empty. Form-layer, not DB-layer.
- `default=...` — default value. **Use a callable**, not a mutable, for lists/dicts.
- `unique=True` — DB unique constraint, indexed.
- `db_index=True` — explicit index. (FKs are auto-indexed.)
- `choices=[...]` — admin renders a select, validator enforces membership.
- `help_text="..."` — shown in forms/admin.
- `verbose_name="..."` — human label.

---

## 3. Migrations — the schema-as-code workflow

You change a model. You run `makemigrations`. Django diff-compares the current state of your models to the last migration and writes a new migration file:

```bash
python manage.py makemigrations blog
# Migrations for 'blog':
#   blog/migrations/0002_post_view_count.py
#     + Add field view_count to post
```

Open it:

```python
# blog/migrations/0002_post_view_count.py
from django.db import migrations, models

class Migration(migrations.Migration):
    dependencies = [("blog", "0001_initial")]
    operations = [
        migrations.AddField(
            model_name="post",
            name="view_count",
            field=models.PositiveIntegerField(default=0),
        ),
    ]
```

**Read this file.** Commit it. Then apply:

```bash
python manage.py migrate
# Operations to perform:
#   Apply all migrations: admin, auth, blog, contenttypes, sessions
# Running migrations:
#   Applying blog.0002_post_view_count... OK
```

`migrate` writes a row into `django_migrations` so it knows what's been applied.

To see what SQL Django will run *without* executing it:

```bash
python manage.py sqlmigrate blog 0002
# ALTER TABLE "blog_post" ADD COLUMN "view_count" integer DEFAULT 0 NOT NULL;
```

To check unmigrated changes:

```bash
python manage.py makemigrations --check --dry-run
```

To roll back to a previous migration (during dev):

```bash
python manage.py migrate blog 0001
```

To remove all migrations (nuclear, dev only):

```bash
rm blog/migrations/0*.py
python manage.py makemigrations blog
```

**`makemigrations` is local, deterministic; `migrate` touches the database.** Keep them separate in your head.

---

## 4. The ORM CRUD vocabulary

In `python manage.py shell`:

### Create

```python
from blog.models import Post, Category
from django.contrib.auth import get_user_model
User = get_user_model()

u = User.objects.create_user("alice", password="x")
c = Category.objects.create(name="Tech", slug="tech")

p = Post.objects.create(
    title="Hello",
    slug="hello",
    body="...",
    author=u,
    category=c,
    status="published",
)
# Or two-step:
p = Post(title="Hi", slug="hi", body="...", author=u)
p.save()
```

### Read

```python
Post.objects.all()                                  # SELECT * FROM blog_post
Post.objects.filter(status="published")             # WHERE status = 'published'
Post.objects.filter(title__icontains="hello")       # WHERE title ILIKE '%hello%'
Post.objects.filter(created_at__gte="2026-01-01")
Post.objects.exclude(status="draft")
Post.objects.get(pk=1)                              # exactly one; raises DoesNotExist/MultipleObjectsReturned
Post.objects.first()
Post.objects.count()
Post.objects.exists()
Post.objects.order_by("-created_at")[:10]           # LIMIT 10
```

Common lookups: `__exact` (default), `__iexact`, `__contains`, `__icontains`, `__startswith`, `__istartswith`, `__endswith`, `__in=[...]`, `__gt`, `__gte`, `__lt`, `__lte`, `__isnull=True`, `__year`, `__month`, `__date`, `__range=(a, b)`.

### Update

```python
p.title = "Hello (edited)"
p.save()                                            # UPDATE blog_post SET ... WHERE id = 1

# Bulk update — single query, no signals fired
Post.objects.filter(status="draft").update(status="archived")
```

### Delete

```python
p.delete()
Post.objects.filter(status="archived").delete()
```

### Relationships

```python
# Forward
p.author              # User instance
p.category.name

# Reverse (by related_name)
u.posts.all()         # all posts by this user
u.posts.filter(status="published")
c.posts.count()

# Many-to-many
p.tags.add(t1, t2)
p.tags.remove(t1)
p.tags.set([t1, t3])  # replace
p.tags.clear()
p.tags.all()
```

---

## 5. `on_delete` — the FK behavior you must pick

When the referenced row is deleted, what happens to this row?

| Option | Effect |
|--------|--------|
| `CASCADE` | Delete this row too. Default for tightly-coupled children (a post's comments). |
| `PROTECT` | Raise `ProtectedError` and refuse the parent delete. Use when losing data is unacceptable. |
| `SET_NULL` | Set this FK to NULL. Requires `null=True`. |
| `SET_DEFAULT` | Set to the field's default. |
| `SET(value_or_callable)` | Set to a specific value. |
| `RESTRICT` | Similar to `PROTECT` but allows cascades through other paths (Django 3.1+). |
| `DO_NOTHING` | DB will raise an integrity error unless you've added an FK action at the DB level. Use only when you know exactly what you're doing. |

The thought process: "if the parent vanishes, is this child still meaningful?" If yes → `SET_NULL`. If no → `CASCADE`. If never → `PROTECT`.

---

## 6. Inspecting the generated SQL

Every queryset has a `.query` attribute that shows the SQL it would run:

```python
qs = Post.objects.filter(status="published").order_by("-created_at")
print(qs.query)
# SELECT "blog_post"."id", "blog_post"."title", ...
# FROM "blog_post"
# WHERE "blog_post"."status" = 'published'
# ORDER BY "blog_post"."created_at" DESC
```

Even better, install **`django-debug-toolbar`** — every page shows you the exact SQL it ran, how long each query took, and lets you click "EXPLAIN" on slow ones. This single tool turns ORM black magic into a transparent layer.

```bash
pip install django-debug-toolbar
```

```python
# settings/dev.py
INSTALLED_APPS += ["debug_toolbar"]
MIDDLEWARE = ["debug_toolbar.middleware.DebugToolbarMiddleware"] + MIDDLEWARE
INTERNAL_IPS = ["127.0.0.1"]
```

```python
# urls.py (dev only)
if settings.DEBUG:
    import debug_toolbar
    urlpatterns += [path("__debug__/", include(debug_toolbar.urls))]
```

---

## 7. Practical application — the blog model end-to-end

Combine model, migration, admin registration, and a view:

```python
# blog/models.py (extracted from above)
class Post(models.Model):
    title = models.CharField(max_length=200)
    slug = models.SlugField(unique=True)
    body = models.TextField()
    status = models.CharField(max_length=10, choices=Post.STATUS_CHOICES, default="draft")
    author = models.ForeignKey(settings.AUTH_USER_MODEL, on_delete=models.CASCADE, related_name="posts")
    created_at = models.DateTimeField(auto_now_add=True)
```

```bash
python manage.py makemigrations blog
python manage.py migrate
```

```python
# blog/admin.py
from django.contrib import admin
from .models import Post, Category, Tag

@admin.register(Post)
class PostAdmin(admin.ModelAdmin):
    list_display = ("title", "author", "status", "created_at")
    list_filter = ("status",)
    search_fields = ("title", "body")
    prepopulated_fields = {"slug": ("title",)}

admin.site.register([Category, Tag])
```

```python
# blog/views.py
from django.views.generic import ListView
from .models import Post

class PostListView(ListView):
    queryset = Post.objects.filter(status="published").order_by("-created_at")
    template_name = "blog/index.html"
    context_object_name = "posts"
    paginate_by = 10
```

Now you have a fully working model, admin, list view, and template — driven by ~30 lines of model code.

---

## 8. Model methods, `get_absolute_url`, and properties

Models can carry behavior. The most common pattern:

```python
from django.urls import reverse

class Post(models.Model):
    ...
    def get_absolute_url(self):
        return reverse("blog:detail", kwargs={"slug": self.slug})

    @property
    def is_recent(self):
        from django.utils import timezone
        return (timezone.now() - self.created_at).days < 7

    def publish(self):
        self.status = "published"
        self.save(update_fields=["status"])
```

`get_absolute_url` is special — admin uses it for the "View on site" button; generic `CreateView`/`UpdateView` redirect to it by default after save.

---

## 9. Custom managers and query methods

When the same filter shows up across views, push it onto the manager:

```python
class PublishedManager(models.Manager):
    def get_queryset(self):
        return super().get_queryset().filter(status="published")

class Post(models.Model):
    ...
    objects = models.Manager()       # default
    published = PublishedManager()   # custom

# Usage
Post.published.all()                 # only published posts
```

Or use `QuerySet.as_manager()` for chainable custom methods:

```python
class PostQuerySet(models.QuerySet):
    def published(self):
        return self.filter(status="published")
    def by_author(self, user):
        return self.filter(author=user)

class Post(models.Model):
    ...
    objects = PostQuerySet.as_manager()

# Chainable
Post.objects.published().by_author(request.user)
```

This is the senior pattern — it keeps view code readable and centralizes business filters.

---

## 10. Common mistakes and gotchas

1. **Mutable default arguments.** `default=[]` for a `JSONField` shares the list across rows. Use `default=list` (callable).
2. **Forgetting `on_delete`.** Django 2+ requires it; you'll get a clear error. But picking the *wrong* `on_delete` (CASCADE-ing a critical reference) destroys data silently the day someone deletes a user.
3. **`null=True` on `CharField`/`TextField`.** Now you have two "empty" states: empty string and `NULL`. Use `blank=True` only — the DB stores `""`.
4. **Migrations not committed.** Migrations are source code. Commit them. CI should run `makemigrations --check`.
5. **Squashing migrations carelessly.** It's a real tool (`squashmigrations`) but doing it on a project with multiple deployed environments is a coordination problem. Plan it.
6. **`Model.objects.update()` doesn't call `save()`.** It also doesn't fire `pre_save`/`post_save` signals or run `auto_now`. Surprise data drift territory.
7. **Adding a non-null field without a default.** `makemigrations` asks "provide a one-off default" — picking "1" silently inserts 1 into every existing row. Think before pressing enter.
8. **Forgetting `related_name`.** `Post.author = FK(User)` makes the reverse `user.post_set`. Two FKs from `Post` to `User` (author + editor) without `related_name` fails with a clash error. Always set it.
9. **Using `__str__` to do queries.** `def __str__(self): return self.author.username` triggers a query every time the object is `repr`'d. Cache or avoid.
10. **Thinking `save()` is atomic across many rows.** It saves one row. For multi-row updates, wrap in `transaction.atomic()`.
11. **`Meta.ordering` makes every query ORDER BY.** Convenient, but it can hurt query plans. Sometimes explicit `order_by()` is better.

---

## 🎯 Key Takeaways

- **Models are the single source of truth.** Schema, admin, forms, serializers, and the queryset API all derive from them. Edit the model first, regenerate everything else.
- **Migrations are commit-able code.** Read them before applying. Review them in PRs. Never edit a migration that's been deployed.
- **`on_delete` is a design decision, not a default.** Pick `CASCADE`, `PROTECT`, or `SET_NULL` deliberately per FK.
- **Use `null=True` for non-string fields, `blank=True` for strings.** Avoid double-empty states.
- **Custom managers + `QuerySet.as_manager()`** are how you keep view code clean. `Post.objects.published().by_author(u)` reads like a sentence.

*← [prev](./03_templates.md) | [next →](./05_orm_deep_dive.md)*
