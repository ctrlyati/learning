# 03 — Templates (Django Template Language)

> **Goal:** Render HTML safely and DRY-ly with DTL — inheritance, context, escaping, and custom tags/filters.

---

## 1. DTL — what it is and what it isn't

Django Template Language (DTL) is intentionally *less powerful* than Python. You cannot call arbitrary functions with arguments, do complex arithmetic, or import modules. This is a feature: templates stay reviewable by non-Pythonists and you can't put business logic where it doesn't belong.

The two syntaxes:

- `{{ variable }}` — *interpolate* a value (with autoescape).
- `{% tag %}` — *do* something (loop, branch, include, load).

Plus filters with `|`:

- `{{ name|upper }}` → uppercase.
- `{{ post.created_at|date:"Y-m-d" }}` → format date.
- `{{ body|truncatewords:30|safe }}` → chain filters.

---

## 2. Configuring templates

In `settings.py`:

```python
TEMPLATES = [
    {
        "BACKEND": "django.template.backends.django.DjangoTemplates",
        "DIRS": [BASE_DIR / "templates"],     # project-wide templates
        "APP_DIRS": True,                      # also look in each app's templates/ folder
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
```

Convention: namespace your app templates to avoid collisions:

```
blog/
└── templates/
    └── blog/                    # <-- nested folder named after app
        ├── index.html
        └── post_detail.html
```

Then `render(request, "blog/index.html", ctx)` — Django finds it because both `DIRS` and `APP_DIRS` are searched.

---

## 3. Template inheritance — the DRY engine

The single most important DTL feature. Define a base layout once, override **blocks** per page.

```django
{# templates/base.html #}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>{% block title %}My Site{% endblock %}</title>
  {% block extra_head %}{% endblock %}
</head>
<body>
  <nav>{% include "_nav.html" %}</nav>
  <main>
    {% block content %}{% endblock %}
  </main>
  <footer>&copy; 2026</footer>
</body>
</html>
```

```django
{# blog/templates/blog/index.html #}
{% extends "base.html" %}

{% block title %}Blog — {{ block.super }}{% endblock %}

{% block content %}
  <h1>Latest Posts</h1>
  <ul>
    {% for post in posts %}
      <li>
        <a href="{% url 'blog:detail' slug=post.slug %}">{{ post.title }}</a>
        <small>{{ post.created_at|date:"M j, Y" }}</small>
      </li>
    {% empty %}
      <li>No posts yet.</li>
    {% endfor %}
  </ul>

  {% if is_paginated %}
    <nav>
      {% if page_obj.has_previous %}<a href="?page={{ page_obj.previous_page_number }}">Prev</a>{% endif %}
      Page {{ page_obj.number }} of {{ paginator.num_pages }}
      {% if page_obj.has_next %}<a href="?page={{ page_obj.next_page_number }}">Next</a>{% endif %}
    </nav>
  {% endif %}
{% endblock %}
```

Key tags here:

- `{% extends "base.html" %}` — must be the first tag in a child template.
- `{% block name %}...{% endblock %}` — defines an override point; child can replace or extend.
- `{{ block.super }}` — within a child block, renders the parent block's contents.
- `{% include "partial.html" %}` — drop in another template (the includee inherits the current context).
- `{% for ... %}{% empty %}{% endfor %}` — `empty` runs when the iterable is empty.

---

## 4. Context — what's available in templates

The context dict you pass to `render()` is what the template sees. **Plus** anything injected by context processors (the `auth` processor adds `user`, `perms`; the `request` processor adds `request`).

```python
def index(request):
    return render(request, "blog/index.html", {
        "posts": Post.objects.filter(published=True),
        "site_name": "My Blog",
    })
```

```django
Hi {{ user.username }}! Welcome to {{ site_name }}.
```

Inside a template, **dot lookup** tries: dict key, then attribute, then method (called with no args), then list index. So `{{ post.title }}` works whether `post` is a dict, a model instance, or has a `title` method.

You **cannot** call methods with arguments in templates. `{{ post.publish(now) }}` doesn't work. That's a deliberate constraint — write a custom tag or do it in the view.

---

## 5. Escaping and autoescape — the security default

By default, every `{{ var }}` is HTML-escaped. So:

```django
{{ comment.body }}
```

If `comment.body == '<script>alert(1)</script>'`, the rendered HTML is `&lt;script&gt;alert(1)&lt;/script&gt;` — neutralized. **This is what protects you from XSS.**

To intentionally render raw HTML (a markdown-rendered blog body, say), use `|safe`:

```django
{{ post.rendered_html|safe }}
```

But `safe` means "I, the developer, swear this is already sanitized." If `rendered_html` was built by concatenating user input, you've reintroduced XSS. Sanitize with `bleach` or similar *before* marking safe.

To disable autoescape for a block:

```django
{% autoescape off %}
  This {{ html_var }} will not be escaped.
{% endautoescape %}
```

Avoid `autoescape off` — it's a foot-gun. Prefer `|safe` on specific variables you've audited.

In view code, mark a string safe with `mark_safe`:

```python
from django.utils.safestring import mark_safe
context["badge"] = mark_safe('<span class="badge">New</span>')
```

---

## 6. Built-in tags and filters worth memorizing

**Tags:**

- `{% if x %}` / `{% elif %}` / `{% else %}` / `{% endif %}`
- `{% for x in xs %}` / `{% empty %}` / `{% endfor %}`
- `{% url 'name' arg %}` — reverse a URL.
- `{% load static %}` then `{% static 'css/site.css' %}` — static file URL.
- `{% csrf_token %}` — must be inside every `<form method="post">`.
- `{% include "_partial.html" with var=value %}` — include with extra context.
- `{% with x=expensive_thing %}...{% endwith %}` — cache an expression.
- `{% verbatim %}{{ raw }}{% endverbatim %}` — when you don't want DTL to interpret braces (Vue/Angular interop).

**Filters:**

- `|default:"N/A"` — fallback for falsy values.
- `|length` — len of a list/string.
- `|date:"Y-m-d H:i"` — format dates.
- `|truncatewords:50` / `|truncatechars:100`
- `|linebreaks` — turn `\n` into `<p>` and `<br>`.
- `|pluralize` — `"{{ n }} item{{ n|pluralize }}"`.
- `|join:", "` — join a list.
- `|floatformat:2` — round to 2 decimals.
- `|safe`, `|escape` — toggle escaping.

In a `for` loop, the `forloop` variable is auto-injected:

```django
{% for post in posts %}
  {{ forloop.counter }}. {{ post.title }}
  {% if forloop.first %}(latest!){% endif %}
{% endfor %}
```

---

## 7. Custom template tags and filters

When DTL's built-ins aren't enough, write your own. Create `templatetags/` *inside an app* (must have `__init__.py`):

```
blog/
├── templatetags/
│   ├── __init__.py
│   └── blog_extras.py
```

A **filter** is a simple function:

```python
# blog/templatetags/blog_extras.py
from django import template
from django.utils.html import escape, mark_safe

register = template.Library()

@register.filter
def addclass(field, css_class):
    """Add a CSS class to a form field widget."""
    return field.as_widget(attrs={"class": css_class})

@register.filter
def reading_time(text):
    words = len(text.split())
    minutes = max(1, round(words / 200))
    return f"{minutes} min read"
```

Use it:

```django
{% load blog_extras %}
{{ form.title|addclass:"form-control" }}
{{ post.body|reading_time }}
```

A **simple tag**:

```python
@register.simple_tag
def current_year():
    from datetime import date
    return date.today().year

@register.simple_tag(takes_context=True)
def active(context, url_name):
    request = context["request"]
    from django.urls import resolve
    return "active" if resolve(request.path_info).url_name == url_name else ""
```

```django
{% load blog_extras %}
<footer>&copy; {% current_year %}</footer>
<a class="{% active 'blog:index' %}" href="{% url 'blog:index' %}">Blog</a>
```

An **inclusion tag** renders a sub-template with its own context:

```python
@register.inclusion_tag("blog/_recent_posts.html")
def recent_posts(count=5):
    return {"posts": Post.objects.order_by("-created_at")[:count]}
```

```django
{% load blog_extras %}
{% recent_posts 3 %}
```

After adding a new templatetags module, **restart `runserver`** — Django caches the registry at startup.

---

## 8. Practical application — full blog index template

```django
{# blog/templates/blog/index.html #}
{% extends "base.html" %}
{% load static blog_extras %}

{% block title %}Blog{% endblock %}

{% block extra_head %}
  <link rel="stylesheet" href="{% static 'blog/css/blog.css' %}">
{% endblock %}

{% block content %}
  <header>
    <h1>The Blog</h1>
    <p>{{ posts|length }} post{{ posts|length|pluralize }}</p>
  </header>

  <section class="posts">
    {% for post in posts %}
      <article>
        <h2><a href="{{ post.get_absolute_url }}">{{ post.title }}</a></h2>
        <p class="meta">
          By {{ post.author.username|default:"anon" }} ·
          {{ post.created_at|date:"M j, Y" }} ·
          {{ post.body|reading_time }}
        </p>
        <p>{{ post.body|truncatewords:30 }}</p>
      </article>
    {% empty %}
      <p>No posts yet — check back soon!</p>
    {% endfor %}
  </section>

  {% include "_pagination.html" %}
{% endblock %}
```

---

## 9. Template engines — yes, you can swap them

DTL is the default, but Django supports **Jinja2** out of the box:

```python
TEMPLATES = [
    {"BACKEND": "django.template.backends.django.DjangoTemplates", ...},
    {"BACKEND": "django.template.backends.jinja2.Jinja2",
     "DIRS": [BASE_DIR / "jinja2"], "APP_DIRS": True},
]
```

Jinja2 is faster and more Pythonic but loses DTL conveniences (`{% url %}`, `{% static %}` need shims). Use DTL unless you have a measured reason to switch.

---

## 10. Common mistakes and gotchas

1. **XSS from `|safe` on user input.** Marking unsanitized user content safe is *the* template-layer XSS vector. Sanitize first, mark safe second.
2. **Forgetting `{% csrf_token %}` in forms.** The form will POST and Django will return 403. Module 6 covers CSRF in depth.
3. **Logic in templates.** If you find yourself writing `{% if posts|length > 5 and user.is_staff and not request.GET.hide %}`, move that decision to the view.
4. **Calling expensive things repeatedly in a loop.** `{% for post in posts %}{{ post.author.profile.avatar }}{% endfor %}` triggers an N+1 (a query per row). Fix in the view with `select_related("author__profile")` (module 5).
5. **Forgetting to `{% load %}` your tag library.** Custom tags require `{% load blog_extras %}` at the top of the template that uses them.
6. **Confusing `{% include %}` and `{% extends %}`.** `extends` is for inheritance (one per template, first line). `include` is for partials (anywhere, repeatable).
7. **Editing `block.super` without thinking about MRO.** `{{ block.super }}` only works when extending — it pulls from the parent block. Get the order wrong and content disappears.
8. **Hardcoding URLs in `<a href>`.** Always `{% url 'name' %}`. Hardcoded paths break the day you change `urls.py`.
9. **Templatetag module not picked up.** You forgot `__init__.py` in `templatetags/`, or didn't restart the server, or the app isn't in `INSTALLED_APPS`.
10. **Mutating context inside a template.** You can't, and that's by design — but you'll occasionally write `{% with x=qs %}` and be surprised that `qs` was evaluated and cached. That's the whole point of `with`.

---

## 🎯 Key Takeaways

- **Autoescape is your XSS armor.** Default-on for every `{{ var }}` is the reason Django apps don't bleed `<script>` tags. Only `|safe` content you've sanitized.
- **Template inheritance + blocks** is the DRY mechanism. One `base.html`, dozens of children overriding `{% block %}`s.
- **DTL is intentionally restricted.** Heavy logic belongs in views, custom tags, or model methods — not templates.
- **Custom tags/filters** (in `app/templatetags/`) are how you extend DTL cleanly. Simple tags, filters, inclusion tags cover almost every need.
- **N+1 queries hide in templates.** Iterating `{{ post.author.name }}` over 100 posts is 101 queries unless the view used `select_related`. Templates expose ORM problems — fix them in views.

*← [prev](./02_urls_and_views.md) | [next →](./04_models_and_orm_basics.md)*
