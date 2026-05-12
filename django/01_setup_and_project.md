# 01 — Setup and Project Layout

> **Goal:** Stand up a Django 5.x project in an isolated environment and understand every file `startproject` creates.

---

## 1. Virtualenv and install — the first five minutes

Every Django project lives in its own virtual environment. Sharing a global `site-packages` across projects guarantees version conflicts within six months.

```bash
# Create and activate a venv (use the python you intend to deploy with)
python -m venv .venv

# Activate
# macOS/Linux:
source .venv/bin/activate
# Windows PowerShell:
.\.venv\Scripts\Activate.ps1

# Install Django 5.x and pin it
pip install "Django>=5.0,<6.0"
pip freeze > requirements.txt
```

Verify:

```bash
python -c "import django; print(django.get_version())"
# 5.0.x
```

Why pin to `>=5.0,<6.0`? Django ships a major release every ~8 months and an LTS every ~2 years. Lock to a known-good major to avoid surprise breakage on `pip install -U`.

---

## 2. `startproject` vs `startapp` — the conceptual split

These two commands confuse beginners. Memorize the distinction *once*:

| Command | Creates | Purpose |
|---------|---------|---------|
| `django-admin startproject mysite .` | Project: `settings.py`, `urls.py`, `wsgi.py`, `asgi.py`, `manage.py` | The deployment unit — one per repo |
| `python manage.py startapp blog` | App: `models.py`, `views.py`, `admin.py`, `apps.py`, `migrations/` | A reusable feature module — many per project |

A **project** is what you deploy. An **app** is a self-contained component (auth, blog, payments) you could in principle copy into a different project. One project, many apps.

```bash
django-admin startproject mysite .   # the trailing dot creates files in cwd, not a nested folder
python manage.py startapp blog
```

Resulting tree:

```
.
├── manage.py
├── mysite/
│   ├── __init__.py
│   ├── asgi.py
│   ├── settings.py
│   ├── urls.py
│   └── wsgi.py
└── blog/
    ├── __init__.py
    ├── admin.py
    ├── apps.py
    ├── migrations/
    │   └── __init__.py
    ├── models.py
    ├── tests.py
    └── views.py
```

Now wire the app into the project — Django doesn't auto-discover apps; you must list it in `INSTALLED_APPS`:

```python
# mysite/settings.py
INSTALLED_APPS = [
    "django.contrib.admin",
    "django.contrib.auth",
    "django.contrib.contenttypes",
    "django.contrib.sessions",
    "django.contrib.messages",
    "django.contrib.staticfiles",
    "blog.apps.BlogConfig",   # <-- add your app
]
```

---

## 3. `manage.py` and `django-admin` — what they actually do

`django-admin` is the global CLI. `manage.py` is a thin wrapper that does *one thing*: sets `DJANGO_SETTINGS_MODULE=mysite.settings` before delegating to `django-admin`. Open it:

```python
#!/usr/bin/env python
import os
import sys

def main():
    os.environ.setdefault("DJANGO_SETTINGS_MODULE", "mysite.settings")
    from django.core.management import execute_from_command_line
    execute_from_command_line(sys.argv)

if __name__ == "__main__":
    main()
```

That's it. Every `python manage.py X` is "run Django command `X` with *my project's* settings."

Common commands you'll live in:

```bash
python manage.py runserver           # dev server on :8000
python manage.py runserver 0.0.0.0:8080
python manage.py makemigrations      # generate migration files from model changes
python manage.py migrate             # apply migrations to the DB
python manage.py createsuperuser     # admin user
python manage.py shell               # interactive Python with Django loaded
python manage.py shell_plus          # (django-extensions) auto-imports your models
python manage.py check               # static config check, no DB
python manage.py collectstatic       # gather static files for prod
python manage.py test                # run the test suite
```

---

## 4. `settings.py` — the dials that matter

`settings.py` is 100+ lines, but only a handful matter day-to-day. Open it and find these:

```python
# mysite/settings.py
from pathlib import Path
import os

BASE_DIR = Path(__file__).resolve().parent.parent

SECRET_KEY = os.environ.get("DJANGO_SECRET_KEY", "dev-only-do-not-use-in-prod")

DEBUG = os.environ.get("DJANGO_DEBUG", "False") == "True"

ALLOWED_HOSTS = os.environ.get("DJANGO_ALLOWED_HOSTS", "localhost,127.0.0.1").split(",")

INSTALLED_APPS = [
    "django.contrib.admin",
    "django.contrib.auth",
    "django.contrib.contenttypes",
    "django.contrib.sessions",
    "django.contrib.messages",
    "django.contrib.staticfiles",
    "blog.apps.BlogConfig",
]

MIDDLEWARE = [
    "django.middleware.security.SecurityMiddleware",
    "django.contrib.sessions.middleware.SessionMiddleware",
    "django.middleware.common.CommonMiddleware",
    "django.middleware.csrf.CsrfViewMiddleware",
    "django.contrib.auth.middleware.AuthenticationMiddleware",
    "django.contrib.messages.middleware.MessageMiddleware",
    "django.middleware.clickjacking.XFrameOptionsMiddleware",
]

ROOT_URLCONF = "mysite.urls"

TEMPLATES = [
    {
        "BACKEND": "django.template.backends.django.DjangoTemplates",
        "DIRS": [BASE_DIR / "templates"],
        "APP_DIRS": True,
        "OPTIONS": {
            "context_processors": [
                "django.template.context_processors.debug",
                "django.template.context_processors.request",
                "django.contrib.auth.context_processors.auth",
                "django.contrib.messages.context_processors.messages",
            ],
        },
    },
]

WSGI_APPLICATION = "mysite.wsgi.application"

DATABASES = {
    "default": {
        "ENGINE": "django.db.backends.sqlite3",
        "NAME": BASE_DIR / "db.sqlite3",
    }
}

LANGUAGE_CODE = "en-us"
TIME_ZONE = "UTC"
USE_I18N = True
USE_TZ = True

STATIC_URL = "static/"
STATIC_ROOT = BASE_DIR / "staticfiles"
MEDIA_URL = "media/"
MEDIA_ROOT = BASE_DIR / "media"

DEFAULT_AUTO_FIELD = "django.db.models.BigAutoField"
```

What each does:

- **`BASE_DIR`** — root of your repo, useful for path-relative configs.
- **`SECRET_KEY`** — signs sessions, password reset tokens, CSRF. Must be set via env in prod. Leaking it = full session hijack.
- **`DEBUG`** — when `True`, Django shows tracebacks with stack frames and local variables. Setting this to `True` in prod is the most common Django security incident.
- **`ALLOWED_HOSTS`** — list of host headers Django will respond to. Prevents Host header attacks. Must be set when `DEBUG=False`.
- **`INSTALLED_APPS`** — every app, including third-party (DRF, allauth, etc.) and your own.
- **`MIDDLEWARE`** — request/response sandwich (see module 10). Order matters: security first, clickjacking last.
- **`DATABASES`** — start with SQLite, swap to PostgreSQL for any real project.
- **`USE_TZ = True`** — store datetimes in UTC. Always. Off-by-timezone bugs are some of the nastiest in production.

---

## 5. Practical application — a working "hello, world"

Add a route and a view to confirm the wiring:

```python
# blog/views.py
from django.http import HttpResponse

def hello(request):
    return HttpResponse("Hello from blog app!")
```

```python
# blog/urls.py  (create this file)
from django.urls import path
from . import views

urlpatterns = [
    path("", views.hello, name="hello"),
]
```

```python
# mysite/urls.py
from django.contrib import admin
from django.urls import path, include

urlpatterns = [
    path("admin/", admin.site.urls),
    path("blog/", include("blog.urls")),
]
```

Run migrations (Django ships with built-in apps that need tables) and start the server:

```bash
python manage.py migrate
python manage.py runserver
```

Visit `http://127.0.0.1:8000/blog/` — you should see "Hello from blog app!"

---

## 6. Splitting settings for environments

Beyond toy projects, you'll outgrow a single `settings.py`. The idiomatic split:

```
mysite/settings/
├── __init__.py
├── base.py        # shared defaults
├── dev.py         # from .base import *; DEBUG = True; SQLite
├── prod.py        # from .base import *; DEBUG = False; Postgres; Sentry
└── test.py        # from .base import *; in-memory DB
```

Then run with:

```bash
DJANGO_SETTINGS_MODULE=mysite.settings.dev python manage.py runserver
```

Or use `django-environ` / `python-decouple` to load secrets from `.env`. Module 16 covers production env layout in depth.

---

## 7. Common mistakes and gotchas

1. **Forgetting to activate the venv.** You install `Django`, then `python manage.py runserver` errors with `ModuleNotFoundError: No module named 'django'` — because you're in system Python. Always check your prompt.
2. **`DEBUG=True` in production.** The yellow Django error page leaks `SECRET_KEY`, env vars, source code. We mention this in *every* module for a reason.
3. **Committing `SECRET_KEY`.** Read it from env: `os.environ["DJANGO_SECRET_KEY"]`. If it leaks in git history, rotate it — it's used to sign session cookies and password reset tokens.
4. **Forgetting to add the app to `INSTALLED_APPS`.** Symptoms: `makemigrations` says "No changes detected", admin doesn't show your models, signals don't fire. Django literally does not know your app exists.
5. **Naming an app the same as a stdlib/third-party package.** Don't name an app `test`, `email`, `auth`, `os`. Import shadows will eat your debugging time.
6. **Running `manage.py` from the wrong directory.** `manage.py` must be invoked from its containing folder, or you must `PYTHONPATH=. python mysite/manage.py ...` — easier to just `cd` first.
7. **Mixing `django-admin` and `manage.py`.** Outside of `startproject`, always use `manage.py` — it sets `DJANGO_SETTINGS_MODULE` for you.
8. **Storing absolute paths in settings.** Use `BASE_DIR / "subdir"`. Hard-coding `/Users/yati/...` breaks in CI and Docker.

---

## 🎯 Key Takeaways

- **One project, many apps.** `startproject` is once per repo; `startapp` is once per logical feature. Apps must be added to `INSTALLED_APPS` to exist.
- **`manage.py` is just `django-admin` + your settings module.** Everything else is a Django management command — and you can write your own (covered in module 16).
- **`DEBUG`, `SECRET_KEY`, `ALLOWED_HOSTS`** are the three settings whose misconfiguration causes the most production incidents. Make them env-driven from day one.
- **`USE_TZ = True` and store UTC.** Convert to user timezone only at the display layer. This is a forever-correct decision.
- **Split settings per environment** as soon as you have more than one environment. A monolithic `settings.py` is fine for tutorials but starts hurting at deploy time.

*← [prev](./00_roadmap.md) | [next →](./02_urls_and_views.md)*
