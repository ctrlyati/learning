# 09 — Static and Media Files

> **Goal:** Serve CSS/JS/images correctly in development and production — and store user uploads on S3 (not on the web server).

---

## 1. Static vs media — the conceptual split

| | Static | Media |
|---|---|---|
| What | Code: CSS, JS, images checked into the repo | User uploads: avatars, attachments |
| Lifetime | Bundled with each deploy; versioned | Persists across deploys |
| Source | Your codebase | `request.FILES` |
| URL prefix | `STATIC_URL` (default `/static/`) | `MEDIA_URL` (default `/media/`) |
| Server | Whitenoise / CDN / nginx | S3 / object storage / nginx |

Mixing the two is the most common mistake. Static files belong in git; media files **never** do.

---

## 2. Settings — the eight that matter

```python
# settings.py
STATIC_URL = "/static/"
STATIC_ROOT = BASE_DIR / "staticfiles"        # collectstatic output (prod)
STATICFILES_DIRS = [BASE_DIR / "static"]      # project-wide static sources (in addition to per-app)

MEDIA_URL = "/media/"
MEDIA_ROOT = BASE_DIR / "media"               # local uploads dir (dev)

# Django 4.2+ unified STORAGES setting
STORAGES = {
    "default": {
        "BACKEND": "django.core.files.storage.FileSystemStorage",   # media
    },
    "staticfiles": {
        "BACKEND": "django.contrib.staticfiles.storage.StaticFilesStorage",   # static
    },
}
```

- **`STATIC_URL`** — URL prefix the browser uses.
- **`STATIC_ROOT`** — where `collectstatic` *writes* (never edit by hand; it gets nuked).
- **`STATICFILES_DIRS`** — extra directories where Django *reads* static (alongside each app's `static/` folder).
- **`MEDIA_URL`** / **`MEDIA_ROOT`** — analogous for uploads.
- **`STORAGES`** (Django 4.2+) — replaces the older `DEFAULT_FILE_STORAGE` + `STATICFILES_STORAGE`. Each key has its own backend.

Convention for app-local static:

```
blog/
└── static/
    └── blog/                  # namespaced subdir to avoid collisions
        ├── css/blog.css
        └── js/blog.js
```

Then `{% static "blog/css/blog.css" %}` in templates.

---

## 3. `{% static %}` template tag — never hardcode

```django
{% load static %}
<link rel="stylesheet" href="{% static 'blog/css/blog.css' %}">
<img src="{% static 'images/logo.png' %}">
```

The tag prefixes `STATIC_URL` and, when using a hashed storage (§5), includes the content hash so cache-busting works.

---

## 4. Development — `runserver` serves static automatically

When `DEBUG=True`, `django.contrib.staticfiles` is in `INSTALLED_APPS`, and you use `runserver`, Django automatically serves files from each app's `static/` directory plus `STATICFILES_DIRS`. No collectstatic needed in dev.

Media is *not* automatic — add to your URL conf for dev convenience:

```python
# mysite/urls.py
from django.conf import settings
from django.conf.urls.static import static

urlpatterns = [...]

if settings.DEBUG:
    urlpatterns += static(settings.MEDIA_URL, document_root=settings.MEDIA_ROOT)
    urlpatterns += static(settings.STATIC_URL, document_root=settings.STATIC_ROOT)
```

**Never use this in production.** `django.views.static.serve` is single-threaded, no caching, no range requests — it exists for development convenience only.

---

## 5. Production static — `collectstatic` + Whitenoise

In production, `runserver` is gone, `DEBUG=False`, and Django no longer auto-serves anything. Two steps:

### Step 1: gather files

```bash
python manage.py collectstatic --noinput
```

This walks every app's `static/` dir + `STATICFILES_DIRS` and copies everything into `STATIC_ROOT` (`/path/to/staticfiles/`). That directory is what gets served.

### Step 2: serve them

Three common options:

**A. Whitenoise** — serve static from your Django process. Simplest. Good for most deployments.

```bash
pip install whitenoise
```

```python
# settings.py
MIDDLEWARE = [
    "django.middleware.security.SecurityMiddleware",
    "whitenoise.middleware.WhiteNoiseMiddleware",     # right after security
    ...
]

STORAGES = {
    "default": {"BACKEND": "django.core.files.storage.FileSystemStorage"},
    "staticfiles": {
        "BACKEND": "whitenoise.storage.CompressedManifestStaticFilesStorage",
    },
}
```

`CompressedManifestStaticFilesStorage` gives you:
- **Compression** (gzip + brotli) of static files at collectstatic time.
- **Content-hashed filenames** (`blog.css` → `blog.a1b2c3d4.css`) for forever-cacheable URLs.

**B. nginx** — front Django with nginx; serve `/static/` directly from disk.

```nginx
location /static/ {
    alias /app/staticfiles/;
    expires 1y;
    add_header Cache-Control "public, immutable";
}
```

**C. CDN** — push `STATIC_ROOT` to CloudFront/CloudFlare/S3 + CDN. The S3 storage backend (next section) handles this.

For most projects, **start with Whitenoise**. If you outgrow it, layer a CDN in front.

---

## 6. Media on S3 — `django-storages`

User uploads must not live on a single web server's disk — autoscale a second server and uploads disappear. The standard: S3 (or any S3-compatible: R2, Wasabi, MinIO, GCS).

```bash
pip install django-storages[s3] boto3
```

```python
# settings.py
STORAGES = {
    "default": {
        "BACKEND": "storages.backends.s3.S3Storage",
        "OPTIONS": {
            "bucket_name": "my-bucket",
            "region_name": "us-east-1",
            "default_acl": None,            # use bucket policy, not ACLs
            "querystring_auth": False,      # public URLs, no signed
            "location": "media",            # bucket subfolder
        },
    },
    "staticfiles": {
        "BACKEND": "whitenoise.storage.CompressedManifestStaticFilesStorage",
    },
}

AWS_ACCESS_KEY_ID = os.environ["AWS_ACCESS_KEY_ID"]
AWS_SECRET_ACCESS_KEY = os.environ["AWS_SECRET_ACCESS_KEY"]
```

Now any `ImageField`/`FileField` you save uploads to S3 transparently:

```python
class Profile(models.Model):
    avatar = models.ImageField(upload_to="avatars/")

# Behind the scenes:
profile.avatar.save("alice.jpg", file_obj)
profile.avatar.url   # https://my-bucket.s3.amazonaws.com/media/avatars/alice.jpg
```

For private files (medical records, contracts), set `querystring_auth=True` and `default_acl="private"` — `.url` then returns a short-lived signed URL.

---

## 7. `upload_to` patterns

You usually don't want every upload landing in one flat directory.

```python
class Post(models.Model):
    cover = models.ImageField(upload_to="covers/%Y/%m/")
    # → media/covers/2026/05/myfile.jpg

def post_image_path(instance, filename):
    return f"posts/{instance.post.id}/{filename}"

class PostImage(models.Model):
    post = models.ForeignKey(Post, on_delete=models.CASCADE)
    image = models.ImageField(upload_to=post_image_path)
```

The callable form is the senior pattern — it lets you namespace by tenant, user, or any model attribute.

---

## 8. Practical application — avatar upload

```python
# accounts/models.py
from django.conf import settings
from django.db import models

class Profile(models.Model):
    user = models.OneToOneField(settings.AUTH_USER_MODEL, on_delete=models.CASCADE, related_name="profile")
    avatar = models.ImageField(upload_to="avatars/%Y/", null=True, blank=True)
    bio = models.TextField(blank=True)

    def avatar_url(self):
        return self.avatar.url if self.avatar else "/static/img/default-avatar.png"
```

```python
# accounts/forms.py
from django import forms
from .models import Profile

class ProfileForm(forms.ModelForm):
    class Meta:
        model = Profile
        fields = ["avatar", "bio"]
```

```python
# accounts/views.py
from django.contrib.auth.decorators import login_required
from django.shortcuts import render, redirect

@login_required
def edit_profile(request):
    profile = request.user.profile
    form = ProfileForm(request.POST or None, request.FILES or None, instance=profile)
    if form.is_valid():
        form.save()
        return redirect("profile")
    return render(request, "accounts/edit_profile.html", {"form": form})
```

```django
{# templates/accounts/edit_profile.html #}
<form method="post" enctype="multipart/form-data">
  {% csrf_token %}
  {{ form.as_p }}
  <button type="submit">Save</button>
</form>
```

```django
{# templates/base.html #}
<img src="{{ user.profile.avatar_url }}" alt="avatar" width="40">
```

In dev, the file lands in `MEDIA_ROOT/avatars/2026/`. In production with `django-storages`, it lands in `s3://my-bucket/media/avatars/2026/`. **The model code is identical** — that's the win of pluggable storage.

---

## 9. Pillow, file validation, and storage gotchas

For `ImageField`, install Pillow:

```bash
pip install Pillow
```

Pillow validates that the uploaded file is actually an image (parsing the bytes, not trusting the filename). This is also where you can resize on upload:

```python
def save(self, *args, **kwargs):
    super().save(*args, **kwargs)
    if self.avatar:
        from PIL import Image
        img = Image.open(self.avatar.path)
        if img.height > 512 or img.width > 512:
            img.thumbnail((512, 512))
            img.save(self.avatar.path)
```

(`self.avatar.path` only works with local storage — on S3, use `self.avatar.open()`, process, write back.)

For arbitrary files, validate MIME and size in the form:

```python
class UploadForm(forms.Form):
    file = forms.FileField()

    def clean_file(self):
        f = self.cleaned_data["file"]
        if f.size > 5 * 1024 * 1024:
            raise forms.ValidationError("Max 5 MB.")
        if f.content_type not in {"application/pdf", "image/jpeg", "image/png"}:
            raise forms.ValidationError("Unsupported type.")
        return f
```

`content_type` is **client-supplied** — for stricter checks use `python-magic` to sniff the file bytes.

---

## 10. Common mistakes and gotchas

1. **Serving static with `runserver` in production.** No caching, no compression, single-threaded — your site dies under any load. Use Whitenoise or nginx.
2. **Forgetting `python manage.py collectstatic` in deploy.** `STATIC_ROOT` is empty → 404 on every CSS file. Should be in your Dockerfile / deploy script.
3. **`STATIC_ROOT == STATICFILES_DIRS[0]`.** Django refuses to start — output dir can't be one of the input dirs. Use different paths.
4. **Storing media on the web server.** First autoscale event = lost uploads. S3 from day one is fine.
5. **Hardcoding `/static/` paths.** `<link href="/static/foo.css">` skips hashing/cache-busting. Always `{% static %}`.
6. **Forgetting `enctype="multipart/form-data"`.** Files silently don't upload; form looks valid with empty fields.
7. **Not including `request.FILES` in the form constructor.** `Form(request.POST)` ignores uploads.
8. **Trusting `f.content_type`.** Attackers can lie. Sniff bytes for security-critical uploads.
9. **Public S3 bucket by default.** New AWS accounts often have block-public-access on. Set bucket policy explicitly and audit it.
10. **Caching hashed assets forever — and bumping `STATIC_URL` instead.** Hashed filenames mean you *can* set `Cache-Control: public, immutable, max-age=31536000`. Take advantage.
11. **Mixing `media/` into the repo.** A `media/` dir in git is a sign of a confused project. Ignore it in `.gitignore`.
12. **`collectstatic` slow on every deploy.** With `Manifest` storage, every run hashes everything. For large projects, look at `ManifestFilesMixin` config or upload pre-hashed via CI.

---

## 🎯 Key Takeaways

- **Static is your code; media is your users' data.** Different storage backends, different deploy lifecycles. Never commit media.
- **Always use `{% static %}` and `obj.image.url`** — both work transparently across local, Whitenoise, and S3 storage.
- **Whitenoise + S3 is the senior default:** Whitenoise serves hashed static from the app, S3 stores user uploads. Add a CDN only when needed.
- **`STORAGES` (Django 4.2+) is the new unified settings entry.** Old `DEFAULT_FILE_STORAGE`/`STATICFILES_STORAGE` settings are deprecated.
- **`collectstatic` is a deploy step**, not a runtime thing. Forget it once and your CSS is broken in production. Bake it into the Dockerfile.

*← [prev](./08_admin_site.md) | [next →](./10_middleware_and_signals.md)*
