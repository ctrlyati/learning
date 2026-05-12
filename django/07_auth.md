# 07 — Authentication and Authorization

> **Goal:** Use Django's auth system idiomatically — login/logout, permissions, groups, custom user models — without rolling your own.

---

## 1. `django.contrib.auth` — what you get for free

Out of the box, `auth` ships with:

- A `User` model with username, password (hashed), email, first/last name, `is_active`, `is_staff`, `is_superuser`.
- A `Group` model and a `Permission` model.
- Login/logout/password-change/password-reset views and URL patterns.
- `request.user` — the authenticated user, or `AnonymousUser`.
- Decorators (`@login_required`) and mixins (`LoginRequiredMixin`).
- A pluggable backend interface for SSO, LDAP, OAuth, etc.

It's already in `INSTALLED_APPS` by default. The middleware that populates `request.user` is `AuthenticationMiddleware`.

---

## 2. The built-in `User`

```python
from django.contrib.auth import get_user_model
User = get_user_model()

u = User.objects.create_user("alice", email="alice@example.com", password="x")
u.set_password("new-password")      # hashes via PBKDF2 by default
u.save()
u.check_password("new-password")    # True
```

**Never store passwords in plaintext.** `set_password()` hashes; assigning `u.password = "..."` directly stores the literal string — broken.

**Use `get_user_model()`, never `from django.contrib.auth.models import User`.** Why: you may swap in a custom user model later (§7), and direct imports lock you in.

---

## 3. Login, logout, and session lifecycle

### Built-in views — just plug them in

```python
# mysite/urls.py
from django.contrib.auth import views as auth_views

urlpatterns += [
    path("accounts/login/",  auth_views.LoginView.as_view(),  name="login"),
    path("accounts/logout/", auth_views.LogoutView.as_view(), name="logout"),
    path("accounts/password_change/", auth_views.PasswordChangeView.as_view(), name="password_change"),
    path("accounts/password_change/done/", auth_views.PasswordChangeDoneView.as_view(), name="password_change_done"),
    path("accounts/password_reset/", auth_views.PasswordResetView.as_view(), name="password_reset"),
    # ... and three more for reset confirm/done
]
```

Or just `path("accounts/", include("django.contrib.auth.urls"))` to mount them all.

Templates Django looks for: `registration/login.html`, `registration/password_reset_form.html`, etc. Create them in your project's `templates/` directory.

```django
{# templates/registration/login.html #}
{% extends "base.html" %}
{% block content %}
  <h1>Sign in</h1>
  <form method="post">
    {% csrf_token %}
    {{ form.as_p }}
    <button type="submit">Sign in</button>
  </form>
{% endblock %}
```

Settings you'll want:

```python
LOGIN_URL = "/accounts/login/"          # where @login_required redirects
LOGIN_REDIRECT_URL = "/"                # where login succeeds to
LOGOUT_REDIRECT_URL = "/"
```

### Manual login flow (when needed)

```python
from django.contrib.auth import authenticate, login, logout

def custom_login(request):
    user = authenticate(request, username=request.POST["u"], password=request.POST["p"])
    if user:
        login(request, user)            # writes session
        return redirect("home")
    return render(request, "login.html", {"error": "Invalid credentials"})

def custom_logout(request):
    logout(request)                     # clears session
    return redirect("home")
```

`authenticate()` returns the user if creds check, else `None`. `login(request, user)` writes the user's ID into the session.

---

## 4. Permissions and groups

Every model gets four auto-generated permissions: `add_<model>`, `change_<model>`, `delete_<model>`, `view_<model>`.

```python
u.user_permissions.add(Permission.objects.get(codename="add_post"))
u.has_perm("blog.add_post")             # True
u.has_perms(["blog.add_post", "blog.change_post"])  # check multiple
```

**Groups** are bundles of permissions. The senior pattern is: assign permissions to groups, users to groups, never users to permissions directly.

```python
from django.contrib.auth.models import Group, Permission

editors = Group.objects.create(name="Editors")
editors.permissions.add(
    Permission.objects.get(codename="add_post"),
    Permission.objects.get(codename="change_post"),
)

u.groups.add(editors)
u.has_perm("blog.add_post")             # True via group
```

Custom permissions on a model:

```python
class Post(models.Model):
    ...
    class Meta:
        permissions = [
            ("can_publish", "Can publish post"),
            ("can_feature", "Can feature post on homepage"),
        ]
```

Run `migrate`, then `u.has_perm("blog.can_publish")` works.

---

## 5. Protecting views — decorators and mixins

### Decorators for FBVs

```python
from django.contrib.auth.decorators import login_required, permission_required, user_passes_test

@login_required
def dashboard(request):
    ...

@permission_required("blog.add_post", raise_exception=True)
def create_post(request):
    ...

@user_passes_test(lambda u: u.email.endswith("@company.com"))
def internal_view(request):
    ...
```

`raise_exception=True` returns 403 instead of redirecting to login when the user is logged in but lacks the permission.

### Mixins for CBVs

```python
from django.contrib.auth.mixins import LoginRequiredMixin, PermissionRequiredMixin, UserPassesTestMixin
from django.views.generic import CreateView, UpdateView

class PostCreateView(LoginRequiredMixin, PermissionRequiredMixin, CreateView):
    model = Post
    fields = ["title", "body"]
    permission_required = "blog.add_post"
    raise_exception = True

class PostUpdateView(LoginRequiredMixin, UserPassesTestMixin, UpdateView):
    model = Post
    fields = ["title", "body"]

    def test_func(self):
        return self.get_object().author == self.request.user
```

Order matters: `LoginRequiredMixin` should be leftmost so anonymous users are bounced to login before any other check runs.

---

## 6. Object-level permissions

Built-in `has_perm` is **model-level** ("can edit any post"). For "can edit *this* post" (object-level), implement yourself:

```python
def can_edit_post(user, post):
    return user.is_superuser or post.author_id == user.id

@login_required
def edit_post(request, pk):
    post = get_object_or_404(Post, pk=pk)
    if not can_edit_post(request.user, post):
        raise PermissionDenied
    ...
```

Or use a library — `django-guardian` is the standard for per-object permissions. DRF (module 11) has its own `IsAuthenticatedOrReadOnly`, `IsOwnerOrReadOnly` pattern.

---

## 7. Custom user models — the right time and the wrong time

**Decide at project start, not later.** Swapping the user model on a live project is a multi-week migration.

Two valid approaches:

### A. `AbstractUser` — keep username, add fields

```python
# accounts/models.py
from django.contrib.auth.models import AbstractUser
from django.db import models

class User(AbstractUser):
    bio = models.TextField(blank=True)
    avatar = models.ImageField(upload_to="avatars/", null=True, blank=True)
    timezone = models.CharField(max_length=64, default="UTC")
```

```python
# settings.py
AUTH_USER_MODEL = "accounts.User"
```

This is the safe default — you keep everything the built-in user has, just with extra fields.

### B. `AbstractBaseUser` — replace the auth model entirely (e.g. email-as-username)

```python
# accounts/models.py
from django.contrib.auth.models import AbstractBaseUser, BaseUserManager, PermissionsMixin
from django.db import models

class UserManager(BaseUserManager):
    def create_user(self, email, password=None, **extra):
        if not email:
            raise ValueError("Email is required")
        email = self.normalize_email(email)
        user = self.model(email=email, **extra)
        user.set_password(password)
        user.save(using=self._db)
        return user

    def create_superuser(self, email, password=None, **extra):
        extra.setdefault("is_staff", True)
        extra.setdefault("is_superuser", True)
        return self.create_user(email, password, **extra)

class User(AbstractBaseUser, PermissionsMixin):
    email = models.EmailField(unique=True)
    name = models.CharField(max_length=120, blank=True)
    is_active = models.BooleanField(default=True)
    is_staff = models.BooleanField(default=False)
    date_joined = models.DateTimeField(auto_now_add=True)

    objects = UserManager()

    USERNAME_FIELD = "email"
    REQUIRED_FIELDS = []     # asked by createsuperuser besides email + password

    def __str__(self):
        return self.email
```

Then `AUTH_USER_MODEL = "accounts.User"`. Also register a `UserAdmin` for the admin (otherwise admin breaks).

**Rule:** if your app might *ever* need email-as-login, do this on day one.

In all model FKs to user, use:

```python
from django.conf import settings
author = models.ForeignKey(settings.AUTH_USER_MODEL, on_delete=models.CASCADE)
```

Never hardcode `User` — `AUTH_USER_MODEL` makes the indirection portable.

---

## 8. Authentication backends — SSO, LDAP, OAuth

The default backend is `django.contrib.auth.backends.ModelBackend` (checks username/password against the User table). You can stack backends:

```python
AUTHENTICATION_BACKENDS = [
    "django.contrib.auth.backends.ModelBackend",
    "myapp.backends.EmailBackend",   # custom
    "social_core.backends.google.GoogleOAuth2",   # social-auth-app-django
]
```

A custom backend:

```python
# accounts/backends.py
from django.contrib.auth import get_user_model
from django.contrib.auth.backends import ModelBackend

User = get_user_model()

class EmailBackend(ModelBackend):
    def authenticate(self, request, username=None, password=None, **kwargs):
        try:
            user = User.objects.get(email=username)
        except User.DoesNotExist:
            return None
        if user.check_password(password) and self.user_can_authenticate(user):
            return user
```

For SSO/OAuth, **use a library**:

- `django-allauth` — the standard for social login (Google, GitHub, Facebook, etc.).
- `social-auth-app-django` — alternative.
- `django-simple-sso` / `mozilla-django-oidc` — for OIDC.
- For SAML: `python3-saml` or `djangosaml2`.

---

## 9. Password hashing

Django uses **PBKDF2** with SHA-256 by default — secure but slow on purpose. Other options:

```python
PASSWORD_HASHERS = [
    "django.contrib.auth.hashers.Argon2PasswordHasher",     # recommended, install argon2-cffi
    "django.contrib.auth.hashers.PBKDF2PasswordHasher",
    "django.contrib.auth.hashers.PBKDF2SHA1PasswordHasher",
    "django.contrib.auth.hashers.BCryptSHA256PasswordHasher",
]
```

The first hasher in the list is used for new passwords. Older hashes are re-hashed on successful login. **Argon2 is the modern preference** — install `argon2-cffi` and put it at the top of the list for new projects.

Password validators (enforced by `UserCreationForm` and `set_password`):

```python
AUTH_PASSWORD_VALIDATORS = [
    {"NAME": "django.contrib.auth.password_validation.UserAttributeSimilarityValidator"},
    {"NAME": "django.contrib.auth.password_validation.MinimumLengthValidator", "OPTIONS": {"min_length": 10}},
    {"NAME": "django.contrib.auth.password_validation.CommonPasswordValidator"},
    {"NAME": "django.contrib.auth.password_validation.NumericPasswordValidator"},
]
```

---

## 10. Sessions — how auth state actually persists

When `login(request, user)` runs:
1. Django creates a session row in `django_session` (default DB backend).
2. The session ID is stored in a `sessionid` cookie on the response.
3. On subsequent requests, `SessionMiddleware` reads the cookie, looks up the session, attaches `request.session`. Then `AuthenticationMiddleware` reads `request.session["_auth_user_id"]` and sets `request.user`.

Settings worth knowing:

```python
SESSION_ENGINE = "django.contrib.sessions.backends.db"       # default
# Other backends: cache, cached_db, file, signed_cookies
SESSION_COOKIE_AGE = 60 * 60 * 24 * 14                       # 2 weeks
SESSION_COOKIE_SECURE = True                                  # HTTPS only (prod)
SESSION_COOKIE_HTTPONLY = True                                # no JS access
SESSION_COOKIE_SAMESITE = "Lax"
SESSION_EXPIRE_AT_BROWSER_CLOSE = False
```

For high-traffic apps, switch to `cached_db` (reads from cache, writes through to DB) or pure `cache` (Redis).

Module 12 covers session backends and caching in depth.

---

## 11. Practical application — login-protected post creation

```python
# blog/views.py
from django.contrib.auth.mixins import LoginRequiredMixin, UserPassesTestMixin
from django.views.generic import CreateView, UpdateView
from django.urls import reverse_lazy
from .models import Post
from .forms import PostForm

class PostCreateView(LoginRequiredMixin, CreateView):
    model = Post
    form_class = PostForm
    template_name = "blog/post_form.html"
    success_url = reverse_lazy("blog:index")

    def form_valid(self, form):
        form.instance.author = self.request.user
        return super().form_valid(form)

class PostUpdateView(LoginRequiredMixin, UserPassesTestMixin, UpdateView):
    model = Post
    form_class = PostForm
    template_name = "blog/post_form.html"

    def test_func(self):
        return self.get_object().author == self.request.user
```

```python
# mysite/urls.py
urlpatterns += [
    path("accounts/", include("django.contrib.auth.urls")),  # all 7 auth views
]
```

```django
{# base.html — show user state #}
{% if user.is_authenticated %}
  Signed in as {{ user.username }} · <a href="{% url 'logout' %}">Sign out</a>
{% else %}
  <a href="{% url 'login' %}">Sign in</a>
{% endif %}
```

You now have signup-style flows (via `UserCreationForm`), login, logout, password change/reset, and per-user authorization — without writing more than ~40 lines of code.

---

## 12. Common mistakes and gotchas

1. **Importing `User` directly.** `from django.contrib.auth.models import User` breaks if you swap user models. Always `get_user_model()` (or `settings.AUTH_USER_MODEL` for FK targets).
2. **Setting `user.password = "..."` directly.** Stores the literal string. Use `set_password()`.
3. **Swapping user models mid-project.** It's a major migration — plan it at project birth or live with `AbstractUser`.
4. **Forgetting `AUTHENTICATION_BACKENDS` when adding a custom backend.** Just adding the class isn't enough.
5. **Order of mixins on CBVs.** `LoginRequiredMixin` must be left of `PermissionRequiredMixin` or anonymous users get a 403 instead of a redirect to login.
6. **Confusing `is_staff`, `is_superuser`, `is_authenticated`.** `is_authenticated` is "logged in"; `is_staff` is "can access admin"; `is_superuser` is "bypasses all permission checks." Not interchangeable.
7. **`request.user.is_authenticated()` — that's the old syntax.** In modern Django it's a property: `request.user.is_authenticated` (no parens).
8. **Skipping object-level auth.** `@permission_required("blog.change_post")` lets any editor edit *any* post. Object-level check needed for ownership.
9. **Logging out without invalidating sessions.** `logout()` does invalidate; manually deleting the cookie alone leaves the session row in the DB until cleanup. Use `logout(request)`.
10. **Trusting `request.user` before middleware ran.** In custom middleware that runs *before* `AuthenticationMiddleware`, `request.user` doesn't exist. Order matters.
11. **Storing sensitive data in the session.** Sessions are server-side by default, but `SESSION_ENGINE="signed_cookies"` puts them on the client — readable to anyone who has the cookie.

---

## 🎯 Key Takeaways

- **Use Django's auth — don't roll your own.** `login_required`, groups, permissions, password reset, session management — all hardened over 20 years. Reinventing is how breaches happen.
- **Custom user model at day one or never.** `AUTH_USER_MODEL = "accounts.User"` extending `AbstractUser` is the safe default; `AbstractBaseUser` for email-as-username.
- **Permissions go on groups, users go in groups.** Direct user-to-permission assignments scale terribly.
- **Object-level auth is on you.** Built-in `has_perm` is model-level. For "owner can edit", check in the view or use `django-guardian`.
- **Switch to Argon2** in `PASSWORD_HASHERS` for new projects. PBKDF2 is fine but Argon2 is the modern choice.

*← [prev](./06_forms_and_validation.md) | [next →](./08_admin_site.md)*
