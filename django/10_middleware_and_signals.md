# 10 — Middleware and Signals

> **Goal:** Understand the request/response sandwich, write your own middleware, and know when (and when *not*) to use signals.

---

## 1. The request/response sandwich — Django's core lifecycle

When a request hits Django:

```
HTTP request
   │
   ▼
[SecurityMiddleware] ─────────────────┐
[SessionMiddleware]                   │
[CsrfViewMiddleware]                  │  request phase
[AuthenticationMiddleware]            │  (top → bottom)
[MessageMiddleware]                   │
   │                                  │
   ▼
URL resolver → view → response        │
   │                                  │
   ▲                                  │
[MessageMiddleware]                   │
[AuthenticationMiddleware]            │  response phase
[CsrfViewMiddleware]                  │  (bottom → top)
[SessionMiddleware]                   │
[SecurityMiddleware] ─────────────────┘
   │
   ▼
HTTP response
```

Every middleware wraps the next. Each can:
- **Inspect or modify the request** before passing it down.
- **Short-circuit** by returning a response without calling the next layer.
- **Inspect or modify the response** on the way back out.

This is why middleware **order matters in `MIDDLEWARE`**. The first entry is the outermost wrapper. Move `AuthenticationMiddleware` above `SessionMiddleware` and `request.session` doesn't exist when auth runs — your site breaks.

---

## 2. The default stack — what each does

```python
MIDDLEWARE = [
    "django.middleware.security.SecurityMiddleware",            # HTTPS redirects, HSTS, XSS headers
    "django.contrib.sessions.middleware.SessionMiddleware",     # populates request.session
    "django.middleware.common.CommonMiddleware",                # URL_PREFIX, APPEND_SLASH, etc.
    "django.middleware.csrf.CsrfViewMiddleware",                # CSRF token check on POST
    "django.contrib.auth.middleware.AuthenticationMiddleware",  # populates request.user
    "django.contrib.messages.middleware.MessageMiddleware",     # flash messages
    "django.middleware.clickjacking.XFrameOptionsMiddleware",   # X-Frame-Options header
]
```

A request must pass top-to-bottom; a response bubbles bottom-to-top. So in the *response phase*, `XFrameOptionsMiddleware` runs first (innermost), `SecurityMiddleware` runs last (outermost). That's why security-related headers can be added by middleware at any layer.

---

## 3. Writing custom middleware

Modern Django middleware is a callable that takes `get_response` (the next layer in the stack):

```python
# myapp/middleware.py
import time
import logging

logger = logging.getLogger(__name__)

class RequestTimingMiddleware:
    def __init__(self, get_response):
        # one-time init at server startup
        self.get_response = get_response

    def __call__(self, request):
        start = time.perf_counter()

        # request phase: before view
        response = self.get_response(request)

        # response phase: after view
        elapsed = (time.perf_counter() - start) * 1000
        response["X-Response-Time-ms"] = f"{elapsed:.1f}"
        if elapsed > 500:
            logger.warning("Slow request: %s %s — %.0f ms", request.method, request.path, elapsed)
        return response
```

Register it:

```python
# settings.py
MIDDLEWARE = [
    "django.middleware.security.SecurityMiddleware",
    ...
    "myapp.middleware.RequestTimingMiddleware",     # near the top for accurate timing
]
```

That's it — every request now logs its duration and gets an `X-Response-Time-ms` header.

Where to slot it: place security/auth-related middleware near the top, observability/header injection near the top (so they wrap everything), and view-specific tweaks near the bottom.

---

## 4. Class-based middleware hooks — optional but powerful

In addition to `__call__`, middleware can implement these optional hooks:

```python
class FancyMiddleware:
    def __init__(self, get_response):
        self.get_response = get_response

    def __call__(self, request):
        return self.get_response(request)

    def process_view(self, request, view_func, view_args, view_kwargs):
        # After URL resolution, before view runs.
        # Return None to continue; return HttpResponse to short-circuit.
        ...

    def process_exception(self, request, exception):
        # View raised. Return None to let Django handle, or a response to recover.
        ...

    def process_template_response(self, request, response):
        # Response has a .render() method (TemplateResponse).
        # Tweak context here.
        ...
```

`process_view` is where decorators like `@login_required` ultimately run their check (via `LoginRequiredMixin.dispatch`-style logic in the view, but you can do similar things globally here).

---

## 5. Short-circuiting and async support

You can return a response from `__call__` *without* calling `get_response`:

```python
class MaintenanceMiddleware:
    def __init__(self, get_response):
        self.get_response = get_response
        import os
        self.enabled = os.environ.get("MAINTENANCE", "0") == "1"

    def __call__(self, request):
        if self.enabled and not request.user.is_staff:
            from django.shortcuts import render
            return render(request, "maintenance.html", status=503)
        return self.get_response(request)
```

Now flipping `MAINTENANCE=1` returns a 503 for non-staff users, and the view never runs.

**Async middleware** (Django 3.1+): if you mark it `async_capable=True` and write an `async def __call__`, it can be efficient in ASGI apps. Pure-sync middleware is wrapped by Django automatically but loses some efficiency under ASGI.

```python
from asgiref.sync import iscoroutinefunction
from django.utils.decorators import sync_and_async_middleware

@sync_and_async_middleware
def hybrid_middleware(get_response):
    if iscoroutinefunction(get_response):
        async def middleware(request):
            response = await get_response(request)
            return response
    else:
        def middleware(request):
            return get_response(request)
    return middleware
```

Module 13 covers async views in depth.

---

## 6. Signals — Django's pub/sub

Signals let receivers react to events emitted by sender code, **decoupling** the two. The signals you'll meet:

| Signal | Sent when |
|--------|-----------|
| `pre_save` / `post_save` | Before/after `Model.save()` |
| `pre_delete` / `post_delete` | Before/after `Model.delete()` |
| `m2m_changed` | M2M relations change (`.add`, `.remove`, `.clear`, `.set`) |
| `request_started` / `request_finished` | Request lifecycle |
| `user_logged_in` / `user_logged_out` / `user_login_failed` | Auth events |

### Connecting a receiver

```python
# blog/signals.py
from django.db.models.signals import post_save
from django.dispatch import receiver
from django.conf import settings
from .models import Profile

@receiver(post_save, sender=settings.AUTH_USER_MODEL)
def create_profile_for_new_user(sender, instance, created, **kwargs):
    if created:
        Profile.objects.create(user=instance)
```

For the receiver to fire, the module must be **imported at startup**. The idiomatic way:

```python
# blog/apps.py
from django.apps import AppConfig

class BlogConfig(AppConfig):
    name = "blog"
    default_auto_field = "django.db.models.BigAutoField"

    def ready(self):
        from . import signals     # noqa: register receivers
```

---

## 7. Custom signals

You can define your own:

```python
# blog/signals.py
import django.dispatch

post_published = django.dispatch.Signal()

# blog/models.py
class Post(models.Model):
    ...
    def publish(self):
        self.status = "published"
        self.save()
        from .signals import post_published
        post_published.send(sender=self.__class__, post=self)

# blog/handlers.py
from django.dispatch import receiver
from .signals import post_published

@receiver(post_published)
def notify_subscribers(sender, post, **kwargs):
    # email subscribers, ping search index, etc.
    ...
```

This is the right shape for "publish action" decoupled from "email subscribers." But — see the next section before reaching for signals.

---

## 8. When NOT to use signals

Signals are *implicit*. Reading the model code, you can't see what runs on `save()` — a receiver in some other module is silently invoked. This implicitness has bitten every Django dev I know. **Prefer explicit code paths when you can.**

Use a signal when:
- The handler lives in a different *app* (decoupling reusable apps).
- The sender code can't import the handler (would create a circular import).
- Multiple unrelated handlers care about the event.

**Don't** use a signal when:
- The handler logic is "what should happen on save" — that's the model's `save()` method or `form_valid()`.
- You're trying to be clever. Explicit method calls are easier to test and debug.
- The handler does I/O (DB, network) inside a `pre_save` — you've serialized that I/O into every save call.

Common foot-gun: `post_save` handler that creates *another* row, which fires its own `post_save`, recursively. Or `post_save` that writes back to the same instance, causing infinite recursion. Add a guard:

```python
@receiver(post_save, sender=Post)
def cache_summary(sender, instance, **kwargs):
    if getattr(instance, "_skip_signal", False):
        return
    ...
```

### `transaction.on_commit` — the often-correct alternative

You want "after this post is saved, send an email." If you use `post_save`, the email is sent *inside* the DB transaction. If the transaction rolls back, the email already went out — a desync bug. Use `on_commit`:

```python
from django.db import transaction

class Post(models.Model):
    def publish(self):
        self.status = "published"
        self.save()
        transaction.on_commit(lambda: send_announcement.delay(self.pk))
```

Now `send_announcement` (a Celery task) runs only after the transaction commits. If it rolls back, no email. This pattern is the senior default for "do X after save."

---

## 9. Practical application — request audit middleware + post-creation signal

### Middleware: log every authenticated request

```python
# audit/middleware.py
import logging
logger = logging.getLogger("audit")

class AuthAuditMiddleware:
    def __init__(self, get_response):
        self.get_response = get_response

    def __call__(self, request):
        response = self.get_response(request)
        if request.user.is_authenticated and request.method != "GET":
            logger.info(
                "AUDIT user=%s method=%s path=%s status=%d",
                request.user.username, request.method, request.path, response.status_code,
            )
        return response
```

```python
# settings.py
MIDDLEWARE = [
    "django.middleware.security.SecurityMiddleware",
    "django.contrib.sessions.middleware.SessionMiddleware",
    "django.middleware.common.CommonMiddleware",
    "django.middleware.csrf.CsrfViewMiddleware",
    "django.contrib.auth.middleware.AuthenticationMiddleware",   # must run before AuditMiddleware
    "audit.middleware.AuthAuditMiddleware",
    "django.contrib.messages.middleware.MessageMiddleware",
    "django.middleware.clickjacking.XFrameOptionsMiddleware",
]
```

### Signal: auto-create profile on user signup

```python
# accounts/signals.py
from django.conf import settings
from django.db.models.signals import post_save
from django.dispatch import receiver
from .models import Profile

@receiver(post_save, sender=settings.AUTH_USER_MODEL)
def ensure_profile(sender, instance, created, **kwargs):
    if created:
        Profile.objects.create(user=instance)
```

```python
# accounts/apps.py
class AccountsConfig(AppConfig):
    name = "accounts"
    def ready(self):
        from . import signals
```

---

## 10. Common mistakes and gotchas

1. **Middleware order.** `AuthenticationMiddleware` *requires* `SessionMiddleware` to run first. Swap them and `request.user` blows up. Read the docstrings of any middleware before adding it.
2. **Stateful middleware.** `__init__` runs once at startup. Instance attributes persist across requests, **across threads**. Don't store per-request state on `self`.
3. **Exceptions in middleware leak through.** A middleware that raises crashes every request. Wrap risky logic in try/except.
4. **`get_response()` not awaited in async.** If you're writing async middleware and forget `await`, you get coroutine objects in your response chain. Use `sync_and_async_middleware` to be safe.
5. **Signals not firing.** Three usual causes: (a) the module isn't imported (forgot `ready()`), (b) sender mismatch (`sender=User` vs `sender=settings.AUTH_USER_MODEL` — they may not be the same class), (c) `Model.objects.update()` doesn't fire `pre_save`/`post_save` (it's bulk SQL).
6. **`post_save` recursion.** Handler that saves the same instance — endless loop. Use `update_fields` or a sentinel attribute.
7. **`pre_save` doing I/O.** Every save waits on your network call. Move to `on_commit` + Celery.
8. **Signal handlers in views.py.** They get imported on first request and may not fire on early requests. Always use `ready()` in `apps.py`.
9. **Using signals for what should be explicit code.** "After ordering, send receipt" — just call `send_receipt()` in the checkout view. A `post_save` receiver buries that intent.
10. **`m2m_changed` complexity.** It fires for `pre_add`, `post_add`, `pre_remove`, `post_remove`, `pre_clear`, `post_clear` — handle the right action. Doing all four wrong is easier than you'd think.
11. **Forgetting `transaction.on_commit` semantics.** If you're outside a transaction, `on_commit` runs immediately. Wrap in `transaction.atomic` explicitly for predictable behavior.

---

## 🎯 Key Takeaways

- **Middleware is a sandwich, not a list.** Order in `MIDDLEWARE` is the wrap order; entries at the top wrap entries below. Mental model: outer = top, inner = bottom.
- **Custom middleware is a 10-line class.** Time requests, inject headers, enforce maintenance mode, audit auth events — all clean concerns that don't belong in a view.
- **Signals decouple, but obscure.** Use them for cross-app reactions and for "auto-create related rows on user create." For business logic on save, prefer explicit method calls or `transaction.on_commit`.
- **`transaction.on_commit` over `post_save` for side effects.** If your handler does I/O or queues a task, it must run *after* the DB commit — otherwise rollbacks cause desync bugs.
- **Receivers must be registered at startup.** Put `from . import signals` inside your `AppConfig.ready()` — anywhere else and they may not fire.

*← [prev](./09_static_and_media.md) | [next →](./11_drf_essentials.md)*
