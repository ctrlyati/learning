# 12 — Caching and Sessions

> **Goal:** Speed up reads, scale sessions, and use Django's cache framework at every granularity — site, view, template fragment, low-level.

---

## 1. The cache framework — one API, many backends

```python
# settings.py
CACHES = {
    "default": {
        "BACKEND": "django.core.cache.backends.redis.RedisCache",
        "LOCATION": "redis://127.0.0.1:6379/1",
    }
}
```

Built-in backends:

| Backend | When |
|---------|------|
| `redis.RedisCache` | Production default. Persistent, distributed, supports TTL. |
| `memcached.PyMemcacheCache` | Equally valid; simpler, no persistence. |
| `db.DatabaseCache` | Cache table inside your DB. Fine for low traffic. |
| `filebased.FileBasedCache` | Single-machine, mostly for tests. |
| `locmem.LocMemCache` | Per-process in-memory. Default; do **not** rely on across processes. |
| `dummy.DummyCache` | No-op; useful in tests. |

You can register multiple named caches:

```python
CACHES = {
    "default": {"BACKEND": "...redis.RedisCache", "LOCATION": "redis://.../1"},
    "sessions": {"BACKEND": "...redis.RedisCache", "LOCATION": "redis://.../2"},
}
```

Then `cache = caches["sessions"]`.

---

## 2. The low-level cache API

```python
from django.core.cache import cache

cache.set("key", value, timeout=300)        # 300s; None = forever; 0 = don't cache
value = cache.get("key")                    # None if missing
value = cache.get("key", default="fallback")
cache.add("key", value)                     # only set if not already present (atomic)
cache.delete("key")
cache.delete_many(["k1", "k2"])
cache.clear()                               # nuke everything (dangerous!)

cache.get_or_set("key", lambda: expensive_compute(), timeout=60)

# Atomic counters
cache.set("hits", 0)
cache.incr("hits")    # returns 1
cache.decr("hits")
```

The single most useful pattern — *compute-on-miss*:

```python
def get_trending_posts():
    key = "blog:trending"
    posts = cache.get(key)
    if posts is None:
        posts = list(Post.objects.filter(...).order_by("-score")[:10])
        cache.set(key, posts, timeout=300)
    return posts
```

Or, cleaner:

```python
return cache.get_or_set(
    "blog:trending",
    lambda: list(Post.objects.filter(...).order_by("-score")[:10]),
    timeout=300,
)
```

---

## 3. Per-view caching

Decorator:

```python
from django.views.decorators.cache import cache_page

@cache_page(60 * 15)       # 15 minutes
def article_list(request):
    ...
```

Class-based:

```python
from django.utils.decorators import method_decorator

@method_decorator(cache_page(60 * 15), name="dispatch")
class ArticleListView(ListView):
    ...
```

This caches the **entire HTTP response** keyed by URL + query params + a few headers. The view doesn't even run on cache hit.

URL-level (cleaner for many views):

```python
from django.views.decorators.cache import cache_page

urlpatterns = [
    path("articles/", cache_page(60 * 15)(article_list)),
]
```

**Caveats:** `cache_page` keys by URL — different users get the same response. Don't cache pages that show personalized content (the user's name, cart count, etc.). Use `Vary: Cookie` or per-user keys for those.

---

## 4. `Vary` headers and per-user variants

By default, Django adds `Vary: Cookie` and `Vary: Accept-Language` for safety, which means caches respect those headers. But for shared HTTP caches (CDNs), be explicit:

```python
from django.views.decorators.vary import vary_on_cookie, vary_on_headers

@cache_page(60 * 5)
@vary_on_cookie
def my_view(request):
    ...    # cached per session
```

For pages that differ by language, currency, or A/B test cohort, vary on the relevant header.

---

## 5. Template fragment caching

Don't want to cache the whole page (header has the user's name, but the article list is shared)?

```django
{% load cache %}
{% cache 300 article_list %}
  {% for a in articles %}
    <article>...</article>
  {% endfor %}
{% endcache %}
```

Cache by variant:

```django
{% cache 300 sidebar request.user.id %}
  ...
{% endcache %}
```

The cache key combines the literal name (`sidebar`) plus each `vary_on` argument. Different users get different cache entries.

---

## 6. The site-wide cache middleware

You can cache *every* page with two middlewares:

```python
MIDDLEWARE = [
    "django.middleware.cache.UpdateCacheMiddleware",     # outermost
    ...
    "django.middleware.cache.FetchFromCacheMiddleware",  # innermost
]
CACHE_MIDDLEWARE_SECONDS = 600
CACHE_MIDDLEWARE_KEY_PREFIX = "mysite"
```

Use this for read-mostly sites (blog, docs) only. Anything user-specific bypasses the cache or you'll leak data between users.

---

## 7. Cache invalidation — the hard part

"There are only two hard things in Computer Science: cache invalidation and naming things." Strategies:

### TTL — let it expire

The default. Set `timeout=300` and accept up to 5 minutes of staleness. Simple, works for most data.

### Explicit invalidation on write

```python
def save_post(post):
    post.save()
    cache.delete(f"post:{post.pk}")
    cache.delete("blog:trending")
```

Or via a signal:

```python
@receiver(post_save, sender=Post)
def invalidate_post_cache(sender, instance, **kwargs):
    cache.delete(f"post:{instance.pk}")
```

Use `transaction.on_commit` (module 10) — invalidating *before* commit can serve stale data if the transaction rolls back.

### Versioned keys

Bump a version number to invalidate everything at once:

```python
def get_articles_cache_key(version):
    return f"articles:v{version}"

version = cache.get_or_set("articles:version", 1)
cache.get(get_articles_cache_key(version))

# To invalidate all:
cache.incr("articles:version")
```

Old entries linger until TTL but are unreachable.

---

## 8. Sessions — what they are, where they live

Django's session framework stores arbitrary per-user data:

```python
request.session["cart"] = [101, 102, 103]
request.session["last_viewed"] = post.id
del request.session["cart"]
request.session.flush()       # delete the session and rotate the cookie
```

The session ID lives in a `sessionid` cookie. The data lives… where?

### Session engines

```python
# settings.py
SESSION_ENGINE = "django.contrib.sessions.backends.db"  # default
```

Options:

| Engine | Storage | Notes |
|--------|---------|-------|
| `backends.db` | Django DB table | Default, durable, slow at scale. |
| `backends.cached_db` | Cache (read), DB (write-through) | Best of both — fast reads, durable writes. |
| `backends.cache` | Cache only | Fast, lost on cache flush. Use only if you're OK losing sessions. |
| `backends.file` | Files on disk | Fine for single-machine apps. |
| `backends.signed_cookies` | Inside the cookie itself, signed | No server state. Limit 4KB, every request carries the data. |

For any real app: `cached_db` with Redis. Pure `cache` is fine if you accept that a Redis restart logs everyone out.

### Important session settings

```python
SESSION_COOKIE_AGE = 60 * 60 * 24 * 14         # 2 weeks
SESSION_COOKIE_HTTPONLY = True                  # no JS access — XSS protection
SESSION_COOKIE_SECURE = True                    # HTTPS only (prod)
SESSION_COOKIE_SAMESITE = "Lax"                 # CSRF defense in depth
SESSION_EXPIRE_AT_BROWSER_CLOSE = False
SESSION_SAVE_EVERY_REQUEST = False              # set True to slide expiration
```

---

## 9. Cache for DRF throttling and other framework features

DRF's throttle classes store counts in the default cache:

```python
REST_FRAMEWORK = {
    "DEFAULT_THROTTLE_CLASSES": [...],
    "DEFAULT_THROTTLE_RATES": {"anon": "100/day", "user": "1000/day"},
}
```

This is **why a Redis cache matters in production** — with `locmem`, each Gunicorn worker has its own counter, throttle quotas are effectively N× what you configured.

---

## 10. Practical application — caching a trending posts widget

```python
# blog/services.py
from django.core.cache import cache
from django.db import transaction
from django.db.models.signals import post_save
from django.dispatch import receiver
from .models import Post

TRENDING_KEY = "blog:trending"
TRENDING_TTL = 60 * 5     # 5 min

def get_trending_posts():
    return cache.get_or_set(
        TRENDING_KEY,
        lambda: list(
            Post.objects.filter(status="published")
                .select_related("author")
                .order_by("-view_count")[:10]
        ),
        timeout=TRENDING_TTL,
    )

@receiver(post_save, sender=Post)
def invalidate_trending(sender, instance, **kwargs):
    transaction.on_commit(lambda: cache.delete(TRENDING_KEY))
```

```python
# blog/views.py
from django.shortcuts import render
from .services import get_trending_posts

def trending(request):
    return render(request, "blog/trending.html", {"posts": get_trending_posts()})
```

```django
{# blog/templates/blog/trending.html — partial fragment cache for the user-personalized header #}
{% extends "base.html" %}
{% load cache %}

{% block content %}
  {# user-specific, not cached #}
  <p>Hi {{ user.username|default:"there" }}, here's what's hot:</p>

  {# shared widget, fragment-cached 5 min #}
  {% cache 300 trending_widget %}
    <ul>
      {% for p in posts %}<li>{{ p.title }} — {{ p.view_count }} views</li>{% endfor %}
    </ul>
  {% endcache %}
{% endblock %}
```

The view query result is cached (5 min) *and* the rendered HTML fragment is cached. Cache miss on save invalidates both.

---

## 11. Cache warmup and the thundering herd

When a popular key expires, every request that arrives in the next few ms triggers the recompute simultaneously — the "thundering herd." Mitigation:

- **Jittered TTLs** — `timeout=300 + random.randint(0, 60)` so keys don't all expire at the same instant.
- **Probabilistic early refresh** — refresh in the background before TTL ends; libraries like `django-cacheops` and `cachetools` help.
- **Lock around expensive computes**:

```python
from django.core.cache import cache

def get_trending():
    val = cache.get("trending")
    if val is not None:
        return val
    # try to acquire a soft lock
    if cache.add("trending:lock", 1, timeout=10):
        try:
            val = compute_trending()
            cache.set("trending", val, timeout=300)
        finally:
            cache.delete("trending:lock")
        return val
    # someone else is computing — wait briefly or serve stale
    return compute_trending()    # or sleep + retry
```

Most apps don't need this until they hit scale. But know it exists.

---

## 12. Common mistakes and gotchas

1. **Using `locmem` in production.** Each process has its own cache; counters and throttles desync. Use Redis.
2. **Caching pages that show user-specific content.** `@cache_page` keyed by URL only — different users see the same response. Recipe for data leaks.
3. **Caching empty/error responses.** If your view returns 500 once, you've cached a 500 for 15 minutes. Use `@cache_control(no_cache=True)` for error paths or wrap in try/except.
4. **No cache invalidation strategy at all.** TTL works until it doesn't (stale UI for hours). Pair cache with signals + `on_commit` for write-through invalidation.
5. **Caching querysets.** `cache.set("posts", Post.objects.all())` pickles the *queryset* (description), not the rows. On retrieval it re-executes. Cache `list(qs)` instead.
6. **Pickling errors.** Cache backends pickle the value. Custom classes must be picklable; lambdas can't be cached as values.
7. **Cache stampede on hot key.** First miss → 100 simultaneous DB queries. Add jitter or a lock.
8. **Session bloat.** Stuffing large objects (full querysets, file contents) into `request.session` makes every request load/save it. Keep sessions small.
9. **`signed_cookies` session engine + secret data.** The cookie is signed but **not encrypted** — anyone with the cookie can read its contents in base64. Never store sensitive data there.
10. **Forgetting `cache.clear()` is global.** Hits every key, including the ones other apps own. Use targeted `delete()` or version prefixes.
11. **Site-wide cache middleware on a logged-in site.** Authenticated pages get cached and served to the wrong users. Catastrophic — use per-view caching instead.
12. **Cache TTLs that are forever.** `timeout=None` means it lives until evicted. In Redis with memory limits, that's eviction policy roulette.

---

## 🎯 Key Takeaways

- **Pick a serious cache backend (Redis) from day one in production.** `locmem` and `db` cache won't scale past one process.
- **Four granularities of caching: site (middleware), view (`cache_page`), fragment (`{% cache %}`), and low-level (`cache.get`/`set`).** Use the smallest one that fits.
- **Cache invalidation = TTL + `on_commit` deletion.** Bare TTL drifts; bare deletion races with rollbacks. Combine them.
- **`SESSION_ENGINE = "cached_db"` + Redis** is the production session sweet spot — fast reads, durable writes.
- **Never cache user-specific content with URL-only keys.** Either skip caching, use fragment caching for the shared parts, or include user ID in the key.

*← [prev](./11_drf_essentials.md) | [next →](./13_async_and_channels.md)*
