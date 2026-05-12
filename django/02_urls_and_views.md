# 02 — URLs and Views

> **Goal:** Route any URL to any callable, fluently — function views, class-based views, and Django's generic CBVs.

---

## 1. URL routing — the mental model

A Django request arrives, and Django walks `ROOT_URLCONF` (default `mysite.urls`) top-to-bottom looking for the first `path()` whose pattern matches. The matched view is called with `request` and any captured kwargs.

```python
# mysite/urls.py
from django.contrib import admin
from django.urls import path, include

urlpatterns = [
    path("admin/", admin.site.urls),
    path("blog/", include("blog.urls")),       # delegate to app
    path("", include("home.urls")),
]
```

`include()` is how you mount an app's `urls.py` at a prefix. This is the modular Django way — each app owns its routes.

---

## 2. `path` vs `re_path` — converters and regex

`path()` uses **path converters** for type-safe captures:

```python
# blog/urls.py
from django.urls import path
from . import views

urlpatterns = [
    path("", views.index, name="blog-index"),
    path("post/<int:pk>/", views.post_detail, name="post-detail"),
    path("post/<slug:slug>/", views.post_by_slug, name="post-slug"),
    path("archive/<int:year>/<int:month>/", views.archive, name="archive"),
]
```

Built-in converters: `str` (default), `int`, `slug`, `uuid`, `path`. Captures arrive as kwargs in the view:

```python
# blog/views.py
def post_detail(request, pk):
    # pk is already an int — Django converted it
    ...
```

For anything `path` can't express (regex), use `re_path`:

```python
from django.urls import re_path
re_path(r"^archive/(?P<year>[0-9]{4})/$", views.archive_year),
```

Rule of thumb: prefer `path()` 95% of the time. `re_path` is for legacy URL formats and complex patterns.

You can also write **custom converters**:

```python
# blog/converters.py
class FourDigitYearConverter:
    regex = "[0-9]{4}"
    def to_python(self, value): return int(value)
    def to_url(self, value): return f"{value:04d}"

# blog/urls.py
from django.urls import register_converter
from . import converters
register_converter(converters.FourDigitYearConverter, "yyyy")

urlpatterns = [path("archive/<yyyy:year>/", views.archive_year)]
```

---

## 3. Named URLs and `reverse()` — never hardcode paths

Every `path()` should have a `name=`. Then in templates and views you reference the *name*, not the URL:

```python
# In a view
from django.shortcuts import redirect
from django.urls import reverse

def create_post(request):
    # ... save ...
    return redirect("post-detail", pk=new_post.pk)
    # or: return redirect(reverse("post-detail", kwargs={"pk": new_post.pk}))
```

```django
{# In a template #}
<a href="{% url 'post-detail' pk=post.pk %}">{{ post.title }}</a>
```

When URLs are namespaced via `include()`, add an `app_name`:

```python
# blog/urls.py
app_name = "blog"
urlpatterns = [path("post/<int:pk>/", views.post_detail, name="detail")]
```

Now refer to it as `"blog:detail"`. This prevents collision if two apps both define a `detail` route.

---

## 4. Function-based views (FBVs) — the simplest unit

A view is just a callable: `request -> HttpResponse`.

```python
# blog/views.py
from django.shortcuts import render, get_object_or_404
from django.http import HttpResponse, JsonResponse
from .models import Post

def index(request):
    posts = Post.objects.filter(published=True).order_by("-created_at")
    return render(request, "blog/index.html", {"posts": posts})

def post_detail(request, pk):
    post = get_object_or_404(Post, pk=pk, published=True)
    return render(request, "blog/post_detail.html", {"post": post})

def api_post(request, pk):
    post = get_object_or_404(Post, pk=pk)
    return JsonResponse({"id": post.pk, "title": post.title})
```

Helpful shortcuts:

- `render(request, template, context)` — load template, render with context, return `HttpResponse`.
- `get_object_or_404(Model, **kwargs)` — `.get()` but raises `Http404` instead of `DoesNotExist`.
- `redirect(name_or_url_or_obj)` — returns `HttpResponseRedirect`; accepts URL name, URL string, or any model with `get_absolute_url()`.

For different HTTP methods:

```python
from django.views.decorators.http import require_http_methods, require_POST

@require_http_methods(["GET", "POST"])
def create_post(request):
    if request.method == "POST":
        # handle form
        ...
    return render(request, "blog/create.html")
```

---

## 5. Class-based views (CBVs) — when state and inheritance pay off

CBVs are classes with HTTP methods as members. Django calls `as_view()` to produce a view callable:

```python
# blog/views.py
from django.views import View
from django.shortcuts import render

class PostListView(View):
    def get(self, request):
        posts = Post.objects.filter(published=True)
        return render(request, "blog/index.html", {"posts": posts})

    def post(self, request):
        # handle creation
        ...
```

```python
# blog/urls.py
path("", PostListView.as_view(), name="index"),
```

The win: instead of `if request.method == "POST"` branches, you get a method per verb. The bigger win is **generic CBVs**, which encapsulate 90% of CRUD.

---

## 6. Generic CBVs — the productivity multiplier

Django ships generic views for the patterns you write 50 times a year. The hierarchy:

```
View
├── TemplateView          # render a template
├── RedirectView          # redirect to a URL
├── ListView              # list of objects
├── DetailView            # single object
├── CreateView            # form to create
├── UpdateView            # form to update
├── DeleteView            # confirmation + delete
└── FormView              # arbitrary form
```

A `ListView` is literally three lines:

```python
from django.views.generic import ListView, DetailView, CreateView
from django.urls import reverse_lazy
from .models import Post

class PostListView(ListView):
    model = Post
    template_name = "blog/index.html"    # default: <app>/<model>_list.html
    context_object_name = "posts"        # default: object_list
    paginate_by = 10                     # adds pagination free

    def get_queryset(self):
        return Post.objects.filter(published=True).order_by("-created_at")

class PostDetailView(DetailView):
    model = Post
    # default URL kwarg: <int:pk>; default template: blog/post_detail.html

class PostCreateView(CreateView):
    model = Post
    fields = ["title", "body"]
    success_url = reverse_lazy("blog:index")
```

```python
# blog/urls.py
urlpatterns = [
    path("", PostListView.as_view(), name="index"),
    path("post/<int:pk>/", PostDetailView.as_view(), name="detail"),
    path("new/", PostCreateView.as_view(), name="create"),
]
```

That replaces ~40 lines of FBV code. The catch: you must learn the **MRO** (method resolution order) of generic views. When you need to customize, you override the right method:

- `get_queryset()` — filter what's listed/detailed.
- `get_context_data(**kwargs)` — add to the template context.
- `form_valid(form)` — runs after form passes validation.
- `get_success_url()` — dynamic redirect after success.

```python
class PostDetailView(DetailView):
    model = Post

    def get_context_data(self, **kwargs):
        ctx = super().get_context_data(**kwargs)
        ctx["recent_posts"] = Post.objects.order_by("-created_at")[:5]
        return ctx
```

**Bookmark [ccbv.co.uk](https://ccbv.co.uk/)** — it lists every attribute and method on every generic CBV with line-numbered source. Indispensable.

---

## 7. FBV vs CBV — when to use which

| Use FBV when | Use CBV when |
|---|---|
| Logic is short and linear | You're doing CRUD on a model |
| Multiple unrelated concerns in one view | Multiple views share setup logic (use a mixin) |
| You want maximum readability | You want to override one piece of a standard flow |
| Decorators (`@login_required`) feel natural | Mixins (`LoginRequiredMixin`) feel natural |

There's no holy war here. Senior codebases use both — FBVs for one-off endpoints, CBVs for the dozen CRUD pages around each model. DRF (module 11) leans entirely on CBV-style `ViewSets`.

---

## 8. Practical application — a blog detail page wired end-to-end

```python
# blog/models.py
from django.db import models
from django.urls import reverse

class Post(models.Model):
    title = models.CharField(max_length=200)
    slug = models.SlugField(unique=True)
    body = models.TextField()
    published = models.BooleanField(default=False)
    created_at = models.DateTimeField(auto_now_add=True)

    def get_absolute_url(self):
        return reverse("blog:detail", kwargs={"slug": self.slug})

    def __str__(self):
        return self.title
```

```python
# blog/views.py
from django.views.generic import ListView, DetailView

class PostListView(ListView):
    model = Post
    template_name = "blog/index.html"
    context_object_name = "posts"
    paginate_by = 10

    def get_queryset(self):
        return Post.objects.filter(published=True).order_by("-created_at")

class PostDetailView(DetailView):
    model = Post
    slug_url_kwarg = "slug"
    template_name = "blog/post_detail.html"
```

```python
# blog/urls.py
from django.urls import path
from . import views

app_name = "blog"
urlpatterns = [
    path("", views.PostListView.as_view(), name="index"),
    path("post/<slug:slug>/", views.PostDetailView.as_view(), name="detail"),
]
```

```python
# mysite/urls.py
urlpatterns = [
    path("admin/", admin.site.urls),
    path("blog/", include("blog.urls", namespace="blog")),
]
```

Now `Post.objects.first().get_absolute_url()` returns `/blog/post/my-first-post/` and the admin's "View on site" button works.

---

## 9. Decorators and mixins for cross-cutting concerns

For FBVs, decorators stack:

```python
from django.contrib.auth.decorators import login_required, permission_required
from django.views.decorators.cache import cache_page
from django.views.decorators.http import require_GET

@require_GET
@cache_page(60 * 15)        # 15 min
@login_required
def dashboard(request):
    ...
```

For CBVs, use mixins (in MRO order — leftmost wins):

```python
from django.contrib.auth.mixins import LoginRequiredMixin, PermissionRequiredMixin

class DashboardView(LoginRequiredMixin, PermissionRequiredMixin, TemplateView):
    template_name = "dashboard.html"
    permission_required = "blog.view_post"
    login_url = "/login/"
```

Or, on a method:

```python
from django.utils.decorators import method_decorator

@method_decorator(cache_page(60), name="dispatch")
class PostListView(ListView):
    ...
```

---

## 10. Common mistakes and gotchas

1. **Forgetting `as_view()` in `urls.py`.** `path("", PostListView)` silently does the wrong thing — it stores the class, not an instance. Always `PostListView.as_view()`.
2. **Trailing slashes.** Django's `APPEND_SLASH=True` redirects `/blog/post/1` to `/blog/post/1/`. This redirect drops POST bodies. Define both or be consistent.
3. **Hardcoding URLs.** Writing `<a href="/blog/post/1/">` instead of `{% url 'blog:detail' pk=1 %}` means refactoring URL structure breaks every template. Always use `{% url %}`.
4. **Wrong URL kwarg name in CBV.** `DetailView` looks for `pk` or `slug` by default. If your `path()` captures `<int:post_id>`, set `pk_url_kwarg = "post_id"` on the view.
5. **`reverse()` at import time.** `reverse()` requires URL config to be loaded, which isn't true at module import. Use `reverse_lazy()` for class attributes.
6. **Overusing CBVs.** Inheritance chains of 5 mixins are unreadable. If you're overriding `dispatch()` and `get_queryset()` and `form_valid()`, you might as well write the FBV.
7. **Confusing `path` converters with regex.** `<int:pk>` is *not* `<\d+:pk>`. There's no regex inside `path()` — use `re_path` for that.
8. **Forgetting `name=` on routes.** Without it, you can't use `{% url %}` or `reverse()`. Always name your routes.
9. **MRO surprises with mixins.** `class V(SomeMixin, LoginRequiredMixin, ListView)` — the auth check might run *after* `get_queryset()` if the mixin is ordered wrong. `LoginRequiredMixin` should be first (leftmost) so it short-circuits early.

---

## 🎯 Key Takeaways

- **`path()` is your default**, `re_path` is the regex escape hatch. Converters (`<int:>`, `<slug:>`) give you typed kwargs for free.
- **Name every URL** and reference by name (`{% url %}`, `reverse()`). Namespaces (`app_name`) prevent collisions across apps.
- **Generic CBVs eat CRUD boilerplate.** `ListView`, `DetailView`, `CreateView`, `UpdateView`, `DeleteView` — learn their hook methods (`get_queryset`, `get_context_data`, `form_valid`) and you'll write 70% less code.
- **FBV vs CBV is taste, not theology.** Use FBVs for one-off endpoints, CBVs when you're doing standard model CRUD or sharing logic via mixins.
- **`ccbv.co.uk` is your senior cheatsheet.** When in doubt about which method runs when in a CBV, look it up there before guessing.

*← [prev](./01_setup_and_project.md) | [next →](./03_templates.md)*
