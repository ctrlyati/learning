# 00 — Django 5.x Deep-Dive Roadmap

> **Goal:** Take a working Python developer from "I've never typed `django-admin startproject`" to confidently shipping, securing, scaling, and operating Django 5.x applications in production.

Django is opinionated, mature, and "batteries included" — which means there is a *correct* Django way to do most things, and the fastest path to senior-level fluency is learning those defaults cold, then learning when to deviate. This course walks the full arc: from the first `manage.py runserver` through DRF APIs, async views, Channels, security hardening, and deploying behind nginx with gunicorn in Docker.

---

## Prerequisites

You should be comfortable with:

- **Python 3.10+** — classes, decorators, context managers, virtual environments. Brush up here if needed: [`../python/00_roadmap.md`](../python/00_roadmap.md).
- **HTML/CSS basics** — Django templates render HTML; you don't need to be a designer, but `<form>`, `<input>`, and `<a>` should not feel foreign.
- **SQL fluency** — Django's ORM abstracts SQL, but seniors *read the queries it emits*. If JOIN, GROUP BY, and indexes are fuzzy, run through [`../mysql/00_roadmap.md`](../mysql/00_roadmap.md) first.
- **The command line** — `pip`, `python -m venv`, `git`, and your shell of choice.

---

## Module Table

| #  | File | Topic | Time |
|----|------|-------|------|
| 00 | [`00_roadmap.md`](./00_roadmap.md) | This roadmap | 30 min |
| 01 | [`01_setup_and_project.md`](./01_setup_and_project.md) | Setup, virtualenv, `startproject` vs `startapp`, `settings.py` | 1 day |
| 02 | [`02_urls_and_views.md`](./02_urls_and_views.md) | URL routing, function vs class-based views, generics | 1 day |
| 03 | [`03_templates.md`](./03_templates.md) | DTL syntax, inheritance, custom tags/filters, escaping | 1 day |
| 04 | [`04_models_and_orm_basics.md`](./04_models_and_orm_basics.md) | Fields, Meta, migrations workflow | 1 day |
| 05 | [`05_orm_deep_dive.md`](./05_orm_deep_dive.md) | Lazy querysets, `select_related`, `prefetch_related`, `F`/`Q`, annotations | 1 day |
| 06 | [`06_forms_and_validation.md`](./06_forms_and_validation.md) | `Form` vs `ModelForm`, `clean_*`, formsets, CSRF | 1 day |
| 07 | [`07_auth.md`](./07_auth.md) | User model, permissions, groups, custom user | 1 day |
| 08 | [`08_admin_site.md`](./08_admin_site.md) | `ModelAdmin`, inlines, actions, admin security | 1 day |
| 09 | [`09_static_and_media.md`](./09_static_and_media.md) | `STATIC` vs `MEDIA`, `collectstatic`, S3 storage | 1 day |
| 10 | [`10_middleware_and_signals.md`](./10_middleware_and_signals.md) | Request lifecycle, signals, when *not* to use them | 1 day |
| 11 | [`11_drf_essentials.md`](./11_drf_essentials.md) | DRF serializers, viewsets, routers, perms, throttling | 1-2 days |
| 12 | [`12_caching_and_sessions.md`](./12_caching_and_sessions.md) | Cache backends, per-view, fragment, low-level API | 1 day |
| 13 | [`13_async_and_channels.md`](./13_async_and_channels.md) | Async views, ASGI, Channels, websockets | 1 day |
| 14 | [`14_testing.md`](./14_testing.md) | `TestCase`, `Client`, factories, `pytest-django` | 1 day |
| 15 | [`15_security.md`](./15_security.md) | CSRF, XSS, SQLi, clickjacking, secrets | 1 day |
| 16 | [`16_production.md`](./16_production.md) | gunicorn/uvicorn + nginx, Docker, Sentry, tuning | 1-2 days |

**Total:** ~2.5 weeks at 1 module/day. Comfortable pros can compress to 1 week; new-to-web devs should expect 4 weeks with practice projects.

---

## Core Mental Models

Internalize these six and 80% of Django stops feeling magical:

### 1. Apps are reusable plug-ins

A Django *project* is the deployment unit; *apps* are the reusable components inside it (e.g. `accounts`, `blog`, `payments`). A well-written app could be pip-installed into a different project tomorrow. This is why apps have their own `models.py`, `urls.py`, `admin.py`, `migrations/` — they're self-contained.

### 2. Models are the source of truth

Define your data once in `models.py`. Django generates: the database schema (via migrations), the admin forms, the `ModelForm`, DRF `ModelSerializer`, and the queryset API. Change the model, run `makemigrations`, and everything downstream updates. Resist the urge to define schema in two places.

### 3. Querysets are lazy

`Article.objects.filter(published=True)` does *not* hit the database. It builds a SQL query that executes only when you iterate, slice, call `list()`, `len()`, `exists()`, or `count()`. This is the single biggest source of "why is this view slow" bugs — and the single biggest superpower once you learn to chain and compose them.

### 4. The request/response middleware sandwich

A request enters at the top of `MIDDLEWARE`, passes down through each layer (auth, CSRF, sessions, ...), hits your view, and the response bubbles back up through the same stack in reverse. If you understand this sandwich, you understand 90% of "why is `request.user` an `AnonymousUser`" type questions.

### 5. Batteries included — favor Django's tools

Need auth? Use `django.contrib.auth`. Need an admin? It's already there. Need forms? `ModelForm`. Need pagination? `Paginator`. New devs reinvent these; seniors reach for them first and only roll their own when there's a concrete reason. The framework has 20 years of edge cases baked in.

### 6. Migrations are code

`makemigrations` writes a Python file under `app/migrations/`. Read it. Commit it. Review it like any other code. Treating migrations as "auto-generated noise to ignore" is how production databases get destroyed at 2am.

---

## External Resources

- **Official docs** — [docs.djangoproject.com](https://docs.djangoproject.com/) — best-in-class framework docs. The tutorial ("Writing your first Django app") is mandatory pre-reading.
- **Django REST Framework** — [django-rest-framework.org](https://www.django-rest-framework.org/) — module 11 leans on this heavily.
- **Two Scoops of Django** (Audrey & Daniel Roy Greenfeld) — the canonical "best practices" book; reads like a checklist of seniority.
- **Classy Class-Based Views** — [ccbv.co.uk](https://ccbv.co.uk/) — searchable, sourced reference for every attribute/method on every Django generic view. Indispensable.
- **MDN's Django tutorial** — [developer.mozilla.org/en-US/docs/Learn/Server-side/Django](https://developer.mozilla.org/en-US/docs/Learn/Server-side/Django) — a structured second pass for the official tutorial concepts.
- **Django Forum + r/django** — for "is this idiomatic?" questions once you've got the basics.

---

## How to use this course

1. **Read in order.** Each module assumes the previous. The ORM deep dive (05) is meaningless without models (04); DRF (11) builds on views, models, and forms.
2. **Type the code.** Don't copy-paste. Muscle memory matters for `manage.py`, `makemigrations`, `runserver`, and the import paths.
3. **Build something alongside.** A blog, a todo API, a tiny e-commerce — anything you'll iterate on across modules. By module 16 you should be deploying it.
4. **Run `.query` constantly.** When you write a queryset, print `str(qs.query)` and read the SQL. This is how you become the engineer who finds the N+1 in code review.
5. **Read the source.** Django's source is exceptionally readable. When something feels magical, `Ctrl+click` into it.

This is professional upskilling material — by the end you should be the person on the team your peers ask "is this the Django way?" Welcome in.

*[next →](./01_setup_and_project.md)*
