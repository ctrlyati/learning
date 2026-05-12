# 08 — The Admin Site

> **Goal:** Turn `django.contrib.admin` into a polished, safe, productivity-multiplying back-office in 30 minutes.

---

## 1. Why the admin matters

The Django admin is the most underrated feature of the framework. For zero code beyond `admin.site.register(Post)`, you get:

- A login-protected web UI.
- CRUD pages for every registered model.
- Search, filter, pagination, sort.
- Bulk delete and custom actions.
- Inline editing of related objects.
- Object history (`LogEntry`).

It is **not** an end-user UI. It's for staff who run the business — content editors, ops, customer support. Treat it as a serious internal tool.

---

## 2. Register your first model

```python
# blog/admin.py
from django.contrib import admin
from .models import Post, Category, Tag

admin.site.register(Post)
admin.site.register(Category)
admin.site.register(Tag)
```

```bash
python manage.py createsuperuser
python manage.py runserver
```

Visit `/admin/`, sign in, and your models are there. That alone is impressive — but the defaults are barely usable for any model with more than a few fields.

---

## 3. `ModelAdmin` — the customization surface

The 80/20:

```python
# blog/admin.py
from django.contrib import admin
from django.utils.html import format_html
from .models import Post, Category, Tag

@admin.register(Post)
class PostAdmin(admin.ModelAdmin):
    list_display = ("title", "author", "status", "created_at", "view_link")
    list_display_links = ("title",)
    list_filter = ("status", "category", "created_at")
    search_fields = ("title", "body", "author__username")
    list_per_page = 25
    list_select_related = ("author", "category")   # JOIN, no N+1 on the changelist
    ordering = ("-created_at",)
    prepopulated_fields = {"slug": ("title",)}
    autocomplete_fields = ("category", "tags")
    readonly_fields = ("created_at", "updated_at", "view_count")
    date_hierarchy = "created_at"
    fieldsets = (
        ("Content", {"fields": ("title", "slug", "body")}),
        ("Classification", {"fields": ("category", "tags", "status")}),
        ("Metadata", {"fields": ("author", "view_count", "created_at", "updated_at"), "classes": ("collapse",)}),
    )

    def view_link(self, obj):
        return format_html('<a href="{}" target="_blank">view</a>', obj.get_absolute_url())
    view_link.short_description = "Live"
```

What each does:

- **`list_display`** — columns on the changelist. Can be a model field, an admin method, or any callable.
- **`list_filter`** — sidebar filters. Use a model field or a custom `SimpleListFilter` subclass.
- **`search_fields`** — adds a search box. Use `field__icontains`-style lookups by listing related fields with `__`.
- **`list_select_related`** / **`list_prefetch_related`** (Django 5) — avoid N+1 on the changelist.
- **`prepopulated_fields`** — JS-auto-fills slug from title.
- **`autocomplete_fields`** — replaces the slow default FK/M2M widget with an AJAX search. The related `ModelAdmin` needs `search_fields`.
- **`readonly_fields`** — display but don't allow editing.
- **`date_hierarchy`** — adds the year/month/day drilldown bar at the top.
- **`fieldsets`** — group fields into named sections; `classes=("collapse",)` makes a section collapsible.

---

## 4. Inline editing — child rows inside the parent

Edit a post's `Section`s on the post's edit page:

```python
# blog/admin.py
from .models import Post, Section

class SectionInline(admin.TabularInline):     # or StackedInline
    model = Section
    extra = 1
    fields = ("title", "body", "order")
    ordering = ("order",)

@admin.register(Post)
class PostAdmin(admin.ModelAdmin):
    inlines = [SectionInline]
    ...
```

`TabularInline` is a compact row-per-record table. `StackedInline` is a vertical form-per-record. Pick based on the number of fields.

---

## 5. Custom admin actions — bulk operations

Built-in is "Delete selected." Add your own:

```python
@admin.register(Post)
class PostAdmin(admin.ModelAdmin):
    ...
    actions = ["mark_published", "mark_draft", "export_csv"]

    @admin.action(description="Mark selected as published")
    def mark_published(self, request, queryset):
        updated = queryset.update(status="published")
        self.message_user(request, f"{updated} posts marked published.")

    @admin.action(description="Export selected as CSV")
    def export_csv(self, request, queryset):
        import csv
        from django.http import HttpResponse
        response = HttpResponse(content_type="text/csv")
        response["Content-Disposition"] = 'attachment; filename="posts.csv"'
        writer = csv.writer(response)
        writer.writerow(["id", "title", "status", "created_at"])
        for p in queryset:
            writer.writerow([p.id, p.title, p.status, p.created_at])
        return response
```

Selected rows arrive as a queryset — use it with the same ORM tools you know.

---

## 6. Customizing the form

To override the form used by the admin:

```python
from django import forms
from .models import Post

class PostAdminForm(forms.ModelForm):
    class Meta:
        model = Post
        fields = "__all__"

    def clean_title(self):
        title = self.cleaned_data["title"]
        if "click here" in title.lower():
            raise forms.ValidationError("No clickbait.")
        return title

@admin.register(Post)
class PostAdmin(admin.ModelAdmin):
    form = PostAdminForm
    ...
```

Or override `get_form()` for per-request logic (e.g. limit choices based on `request.user`).

---

## 7. Custom list filters

Beyond simple `list_filter = ("status",)`:

```python
from django.contrib import admin
from django.utils import timezone
from datetime import timedelta

class RecencyFilter(admin.SimpleListFilter):
    title = "recency"
    parameter_name = "recency"

    def lookups(self, request, model_admin):
        return [("today", "Today"), ("week", "Past 7 days"), ("month", "Past 30 days")]

    def queryset(self, request, queryset):
        now = timezone.now()
        if self.value() == "today":
            return queryset.filter(created_at__gte=now - timedelta(days=1))
        if self.value() == "week":
            return queryset.filter(created_at__gte=now - timedelta(days=7))
        if self.value() == "month":
            return queryset.filter(created_at__gte=now - timedelta(days=30))

@admin.register(Post)
class PostAdmin(admin.ModelAdmin):
    list_filter = ("status", RecencyFilter)
```

---

## 8. Per-user / per-row access control

By default, anyone with `is_staff=True` and the right permissions can edit anything. Restrict:

```python
@admin.register(Post)
class PostAdmin(admin.ModelAdmin):
    def get_queryset(self, request):
        qs = super().get_queryset(request)
        if request.user.is_superuser:
            return qs
        return qs.filter(author=request.user)    # only own posts

    def has_change_permission(self, request, obj=None):
        if obj is None:
            return True
        return request.user.is_superuser or obj.author_id == request.user.id

    has_delete_permission = has_change_permission

    def save_model(self, request, obj, form, change):
        if not change:
            obj.author = request.user
        super().save_model(request, obj, form, change)
```

`save_model` is the admin's `form_valid` — runs on every admin save. Use it to inject the current user, audit logs, etc.

---

## 9. Admin security — the things to lock down

The admin is a high-value target. Bare minimums for production:

1. **Move the URL off `/admin/`.** Default-path admins get hammered by bots. Use a non-obvious path:

   ```python
   urlpatterns = [path("ops-cake/", admin.site.urls)]   # something nonsense
   ```

2. **Force HTTPS.** Set `SECURE_SSL_REDIRECT = True`, `SESSION_COOKIE_SECURE = True`.

3. **Enforce 2FA.** `django-otp` + `django-two-factor-auth` give the admin TOTP-based MFA.

4. **Restrict by IP / VPN.** Front the admin with nginx `allow`/`deny` rules or a VPN-only subdomain.

5. **Audit logging.** Built-in `LogEntry` records who changed what. For a fuller audit, use `django-simple-history` or `django-auditlog`.

6. **Don't grant `is_superuser` casually.** Superusers bypass `has_perm` entirely. Use granular permissions and groups.

7. **Rate-limit the login.** `django-axes` locks brute-force attempts.

---

## 10. Admin theming and `AdminSite`

For multi-brand projects, subclass `AdminSite`:

```python
# myproject/admin.py
from django.contrib.admin import AdminSite

class MySiteAdmin(AdminSite):
    site_header = "Acme Internal Tools"
    site_title = "Acme Admin"
    index_title = "Operations Console"

admin_site = MySiteAdmin(name="myadmin")

# urls.py
urlpatterns = [path("ops/", admin_site.urls)]
```

Or just tweak the default:

```python
admin.site.site_header = "Acme Internal Tools"
admin.site.site_title = "Acme"
admin.site.index_title = "Welcome"
```

For visual polish, `django-jazzmin` or `django-grappelli` are popular skins.

---

## 11. Practical application — full-featured `PostAdmin`

```python
# blog/admin.py
from django.contrib import admin
from django.urls import reverse
from django.utils.html import format_html
from django.utils.safestring import mark_safe
from .models import Post, Category, Tag, Comment

class CommentInline(admin.TabularInline):
    model = Comment
    extra = 0
    readonly_fields = ("author", "body", "created_at")
    can_delete = True
    show_change_link = True

@admin.register(Post)
class PostAdmin(admin.ModelAdmin):
    list_display = ("title", "author", "status_badge", "category", "created_at", "view_count")
    list_display_links = ("title",)
    list_filter = ("status", "category", "created_at")
    search_fields = ("title", "body", "author__username")
    list_select_related = ("author", "category")
    autocomplete_fields = ("category", "tags")
    prepopulated_fields = {"slug": ("title",)}
    readonly_fields = ("view_count", "created_at", "updated_at")
    date_hierarchy = "created_at"
    inlines = [CommentInline]
    actions = ["publish_selected"]
    fieldsets = (
        ("Content", {"fields": ("title", "slug", "body")}),
        ("Classification", {"fields": ("category", "tags", "status")}),
        ("Audit", {"fields": ("view_count", "created_at", "updated_at"), "classes": ("collapse",)}),
    )

    def status_badge(self, obj):
        colors = {"draft": "#999", "published": "#28a745"}
        color = colors.get(obj.status, "#000")
        return mark_safe(f'<span style="color:{color};font-weight:bold">{obj.get_status_display()}</span>')
    status_badge.short_description = "Status"
    status_badge.admin_order_field = "status"

    @admin.action(description="Publish selected posts")
    def publish_selected(self, request, queryset):
        count = queryset.update(status="published")
        self.message_user(request, f"Published {count} posts.")

    def get_queryset(self, request):
        return super().get_queryset(request).select_related("author", "category").prefetch_related("tags")

    def save_model(self, request, obj, form, change):
        if not change and not obj.author_id:
            obj.author = request.user
        super().save_model(request, obj, form, change)


@admin.register(Category)
class CategoryAdmin(admin.ModelAdmin):
    list_display = ("name", "slug", "post_count")
    search_fields = ("name",)
    prepopulated_fields = {"slug": ("name",)}

    def post_count(self, obj):
        return obj.posts.count()


@admin.register(Tag)
class TagAdmin(admin.ModelAdmin):
    search_fields = ("name",)
```

You now have a back-office that supports search, filter, bulk publish, inline comment moderation, colored status badges, and a clean fieldset layout — all from a single file.

---

## 12. Common mistakes and gotchas

1. **Forgetting `search_fields` on a related admin you `autocomplete_fields` to.** `autocomplete_fields = ("category",)` fails with `E040` unless `CategoryAdmin.search_fields` exists.
2. **N+1 in `list_display`.** A `def author_name(self, obj): return obj.author.username` adds a query per row. Fix with `list_select_related` or override `get_queryset`.
3. **Leaving the admin at `/admin/` in prod.** Bot traffic eats CPU just from failed logins. Move it.
4. **Granting `is_superuser` to all staff.** Superusers bypass *every* permission check. Use granular permissions + groups.
5. **`fields` and `fieldsets` together.** Pick one. `fieldsets` wins if both are set.
6. **Editing migrations to add admin fields.** Admin lives in `admin.py`; it doesn't generate migrations. If you find yourself touching migrations because of admin, you're confused.
7. **Heavy logic in `list_display` callables.** They run for every row on every changelist page load. Push computation into the queryset (`annotate`).
8. **Trusting `readonly_fields` for security.** A determined user with form-level access could still POST a value. For real "no one but admins can set this," enforce in `save_model` or use `has_change_permission`.
9. **Not realizing `ModelAdmin.save_model` runs on bulk-edit too.** If you do per-row logic on save, bulk admin edits will hit it once per row.
10. **Ignoring `LogEntry`.** Django records every admin change. `from django.contrib.admin.models import LogEntry` and query it — it's a free audit trail.

---

## 🎯 Key Takeaways

- **The admin is your CRUD UI for free.** `@admin.register` + a 20-line `ModelAdmin` produces a tool worth thousands of dollars of custom UI work.
- **Tune the changelist:** `list_display`, `list_filter`, `search_fields`, `list_select_related`, `autocomplete_fields`, `date_hierarchy`. These six bring the admin from "barely usable" to "delightful."
- **Inlines and actions** scale the admin from one-row CRUD to real workflow tooling (moderate comments inline, bulk publish, export CSV).
- **Lock the admin down in production:** non-default URL, HTTPS-only sessions, 2FA, IP allow-listing, audit logging, minimal superusers.
- **`save_model` is your hook** for "set author = current user," "log this change," "trigger downstream side effect." It runs on every admin save (with `change=False` on create, `True` on update).

*← [prev](./07_auth.md) | [next →](./09_static_and_media.md)*
