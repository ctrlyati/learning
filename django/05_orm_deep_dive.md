# 05 — ORM Deep Dive

> **Goal:** Write ORM code that emits efficient SQL — kill N+1s, compose with `F`/`Q`, annotate aggregates, and reach for raw SQL only when justified.

---

## 1. Querysets are lazy — internalize this

A `QuerySet` is a *description* of a SQL query, not the result. It doesn't execute until you:

- iterate it (`for p in qs:`)
- slice with a step or convert (`list(qs)`, `tuple(qs)`)
- call `len(qs)`, `bool(qs)`, `repr(qs)`
- call `.exists()`, `.count()`, `.first()`, `.last()`, `.get()`
- pickle it
- pass it to `{% for %}` in a template

Everything else — `.filter()`, `.exclude()`, `.order_by()`, `.annotate()`, `.values()` — builds a *new* queryset without touching the database.

```python
qs = Post.objects.filter(status="published")    # no SQL yet
qs = qs.order_by("-created_at")                  # still no SQL
qs = qs.exclude(author__is_active=False)         # still no SQL
print(qs.query)                                  # prints the SQL
posts = list(qs)                                 # NOW it runs (one SELECT)
```

QuerySets also **cache results**. After the first iteration, repeated iteration uses the cache:

```python
qs = Post.objects.all()
list(qs)              # query runs
list(qs)              # uses cache, no query
qs2 = qs.filter(...)  # new queryset, fresh query
```

But `if qs:` followed by `for p in qs:` runs *two* queries (bool then iter) unless you `list()` once or use `.exists()` for the test.

---

## 2. The N+1 problem and `select_related` / `prefetch_related`

The single most common Django performance bug:

```python
# Bad — 1 query for posts, then 1 per post for the author
for post in Post.objects.all():
    print(post.author.username)
# 1 + N queries
```

Each `post.author` triggers a fresh `SELECT * FROM auth_user WHERE id = ?`. With 1000 posts, that's 1001 queries.

### `select_related` — for forward FK and OneToOne (single-valued)

Joins the related table in the same query:

```python
for post in Post.objects.select_related("author"):
    print(post.author.username)
# 1 query — JOIN auth_user
```

The SQL:

```sql
SELECT post.*, user.*
FROM blog_post
INNER JOIN auth_user ON post.author_id = user.id
```

Chain across relations:

```python
Post.objects.select_related("author", "category", "author__profile")
```

### `prefetch_related` — for reverse FK and ManyToMany (multi-valued)

Can't be a single JOIN (you'd duplicate rows). Django runs a *second* query and stitches in Python:

```python
for user in User.objects.prefetch_related("posts"):
    for post in user.posts.all():
        print(post.title)
# 2 queries: 1 for users, 1 for posts WHERE author_id IN (...)
```

Combine them:

```python
Post.objects.select_related("author").prefetch_related("tags", "comments__author")
```

### `Prefetch` for filtered prefetches

You want each user's *published* posts only:

```python
from django.db.models import Prefetch

User.objects.prefetch_related(
    Prefetch("posts", queryset=Post.objects.filter(status="published"), to_attr="published_posts")
)
# Now: user.published_posts (a list, prefetched)
```

**Rule of thumb:** `select_related` for the FK/OneToOne you're following downward; `prefetch_related` for the reverse/M2M you're walking back through.

---

## 3. `F` expressions — operate on database columns

`F("field")` references a column on the **server side**, so you can do atomic, race-free updates without round-tripping:

```python
from django.db.models import F

# Increment view_count without a SELECT-then-UPDATE race
Post.objects.filter(pk=p.pk).update(view_count=F("view_count") + 1)
# SQL: UPDATE blog_post SET view_count = view_count + 1 WHERE id = ?

# Compare two columns of the same row
Post.objects.filter(updated_at__gt=F("created_at"))
# Posts that have been edited since creation
```

After an `F` update, **the in-memory instance is stale** — call `.refresh_from_db()` to reload.

---

## 4. `Q` objects — OR, NOT, complex boolean

`filter()` ANDs its kwargs. For OR and NOT, use `Q`:

```python
from django.db.models import Q

Post.objects.filter(Q(status="published") | Q(author=request.user))
# WHERE status = 'published' OR author_id = ?

Post.objects.filter(~Q(status="draft"))
# WHERE NOT (status = 'draft')

# Combine with regular kwargs (still ANDed):
Post.objects.filter(Q(title__icontains=q) | Q(body__icontains=q), status="published")
```

Build queries dynamically:

```python
filters = Q()
if request.GET.get("q"):
    filters &= (Q(title__icontains=request.GET["q"]) | Q(body__icontains=request.GET["q"]))
if request.GET.get("author"):
    filters &= Q(author__username=request.GET["author"])
posts = Post.objects.filter(filters)
```

---

## 5. Aggregations and annotations

`.aggregate()` collapses the queryset to one row of scalars:

```python
from django.db.models import Count, Sum, Avg, Max, Min

Post.objects.aggregate(total=Count("id"), avg_views=Avg("view_count"))
# {"total": 142, "avg_views": 87.3}
```

`.annotate()` adds a computed column **per row**:

```python
from django.db.models import Count

users = User.objects.annotate(post_count=Count("posts"))
for u in users:
    print(u.username, u.post_count)
```

Filter on annotations:

```python
User.objects.annotate(post_count=Count("posts")).filter(post_count__gte=10)
```

Conditional aggregates (Django 2+):

```python
from django.db.models import Count, Q

User.objects.annotate(
    published_count=Count("posts", filter=Q(posts__status="published")),
    draft_count=Count("posts", filter=Q(posts__status="draft")),
)
```

Combine `F` and aggregates:

```python
from django.db.models import F, ExpressionWrapper, DurationField

Post.objects.annotate(
    edit_lag=ExpressionWrapper(F("updated_at") - F("created_at"), output_field=DurationField())
).order_by("-edit_lag")
```

---

## 6. `values()`, `values_list()`, and `.only()`/`.defer()`

By default a queryset hydrates full model instances. Sometimes you only want a few columns:

```python
Post.objects.values("id", "title")
# QuerySet of dicts: [{"id": 1, "title": "..."}, ...]

Post.objects.values_list("id", flat=True)
# QuerySet of scalars: [1, 2, 3, ...]

Post.objects.values_list("id", "title")
# QuerySet of tuples
```

`only()` and `defer()` still return model instances but constrain which columns load:

```python
Post.objects.only("id", "title")        # load only these, others lazy-load
Post.objects.defer("body")              # load everything except body
```

Useful when a `TextField` is huge and you don't need it on a list page.

---

## 7. Bulk operations

`save()` in a loop is 1 query per row. For thousands of rows:

```python
posts = [Post(title=f"Post {i}", slug=f"p{i}", body="...", author=u) for i in range(1000)]
Post.objects.bulk_create(posts, batch_size=500)
# 2 queries (with batching), not 1000
```

```python
for p in posts:
    p.view_count *= 2
Post.objects.bulk_update(posts, ["view_count"], batch_size=500)
```

**Caveats:** `bulk_create` doesn't fire signals, doesn't populate `pk` on the original objects (unless on Postgres with `RETURNING`), and skips `save()` overrides.

---

## 8. Transactions

By default, each `save()` is its own transaction. To wrap multiple writes:

```python
from django.db import transaction

with transaction.atomic():
    post.save()
    Notification.objects.create(...)
    AuditLog.objects.create(...)
# All commit together, or all roll back on exception.
```

Decorator form:

```python
@transaction.atomic
def transfer(from_user, to_user, amount):
    ...
```

Database-level constraints fire at COMMIT, not at `save()`. If you want a constraint failure to raise inside the `with` block, use `transaction.atomic()` nested or `set_deferred(False)`.

To run something only on successful commit (e.g. enqueue a Celery task that needs the row to exist):

```python
transaction.on_commit(lambda: send_welcome_email.delay(user.pk))
```

This is one of the most underrated Django features. Use it whenever you write a row and then want side effects.

---

## 9. Raw SQL — the escape hatch

The ORM covers ~95% of cases. For the rest:

### `Model.objects.raw()` — returns model instances

```python
Post.objects.raw("SELECT * FROM blog_post WHERE LENGTH(body) > %s", [500])
```

### Cursor — for arbitrary SQL

```python
from django.db import connection

with connection.cursor() as cur:
    cur.execute("SELECT category_id, COUNT(*) FROM blog_post GROUP BY category_id")
    rows = cur.fetchall()
```

Both `raw()` and cursor are subject to SQL injection if you string-interpolate user input — **always use parameter substitution** (`%s` placeholders) so the driver escapes for you.

Prefer ORM. Reach for raw SQL when:
- You're using a DB feature with no ORM equivalent (window functions before Django 2 had them; certain CTEs)
- You need to read query plans and tune to the byte
- You're running a one-off data migration

---

## 10. Django 5 async ORM

Django 4.1+ added async variants. Django 5 extends them. The pattern is `aget`, `acount`, `afirst`, `acreate`, `asave`, plus `async for` over querysets:

```python
async def post_count():
    return await Post.objects.acount()

async def published():
    async for post in Post.objects.filter(status="published"):
        ...

async def create_post(**kwargs):
    return await Post.objects.acreate(**kwargs)
```

Behind the scenes Django still uses a thread pool to wrap sync drivers (psycopg, mysqlclient). True async drivers exist (`psycopg3` async) but the ORM doesn't yet thread them all the way through. Module 13 covers when async pays off.

---

## 11. Practical application — a "trending posts" view

A senior, query-tuned implementation:

```python
# blog/views.py
from django.db.models import Count, Q, F
from django.utils import timezone
from datetime import timedelta
from django.views.generic import ListView
from .models import Post

class TrendingPostsView(ListView):
    template_name = "blog/trending.html"
    context_object_name = "posts"
    paginate_by = 20

    def get_queryset(self):
        last_week = timezone.now() - timedelta(days=7)
        return (
            Post.objects
            .filter(status="published", created_at__gte=last_week)
            .select_related("author", "category")
            .prefetch_related("tags")
            .annotate(
                comment_count=Count("comments", filter=Q(comments__approved=True)),
                score=F("view_count") + F("comment_count") * 5,
            )
            .order_by("-score", "-created_at")
        )
```

Inspect the SQL:

```python
print(TrendingPostsView().get_queryset().query)
```

You get one query with two JOINs (`select_related`) plus a second query for tags (`prefetch_related`). Without these, the template loop would emit hundreds of queries. With them, a constant 2.

---

## 12. Common mistakes and gotchas

1. **N+1 queries in templates.** `{% for post in posts %}{{ post.author.profile.bio }}{% endfor %}` is the classic. Use `select_related("author__profile")` in the view.
2. **`len(qs)` vs `qs.count()`.** `len()` evaluates the whole queryset and caches it. `count()` runs `SELECT COUNT(*)`. Use `count()` for size checks; `len()` if you're going to iterate anyway.
3. **`if qs:` triggers evaluation.** It's effectively `bool(qs)` → executes the query (with `LIMIT 1`). For just an existence check, use `.exists()`.
4. **Forgetting to `.refresh_from_db()` after `F` updates.** The in-memory object still has the old value.
5. **`.update()` skips `save()` and signals.** `auto_now` doesn't update; `post_save` doesn't fire. If business logic lives in `save()`, `update()` bypasses it.
6. **Mixing `aggregate` and `annotate`.** `aggregate()` returns a dict; `annotate()` returns a queryset. They don't compose the way you'd naively expect — re-read the docs each time.
7. **Filtering across M2M creates duplicates.** `Post.objects.filter(tags__name__in=["py", "django"])` may return the same post twice. Add `.distinct()`.
8. **Slicing then filtering.** `Post.objects.all()[:10].filter(...)` raises — you can't filter a sliced queryset. Filter first, slice last.
9. **Querysets aren't thread-safe to share.** Two threads iterating the same `qs` will both hit the cache or both run queries — undefined. Build fresh per request.
10. **Forgetting `transaction.atomic()` on multi-row writes.** Half-applied state survives the exception.
11. **Trusting `bulk_create` to populate `pk` everywhere.** Only Postgres returns IDs by default (Django 4+). On MySQL/SQLite without manual tweaks, the inserted objects have `pk=None`.
12. **Not using `django-debug-toolbar` in development.** This is non-negotiable for any serious project — it shows every query per request, with line numbers and `EXPLAIN` output.

---

## 🎯 Key Takeaways

- **Querysets are descriptions; they execute only on evaluation.** Print `.query`, check `django-debug-toolbar` — never guess what SQL ran.
- **`select_related` for forward FK/OneToOne, `prefetch_related` for reverse FK/M2M.** This pair kills 95% of N+1 bugs.
- **`F` for atomic column expressions, `Q` for OR/NOT** — combined they let you express any SQL `WHERE` clause.
- **`annotate` + `Count(..., filter=Q(...))`** is the senior pattern for dashboards: per-row aggregates with conditional filtering, one query.
- **Wrap writes in `transaction.atomic()` and side effects in `on_commit()`.** Half-written rows + already-fired emails = the nastiest production bugs.

*← [prev](./04_models_and_orm_basics.md) | [next →](./06_forms_and_validation.md)*
