# 15 — Security

> **Goal:** Know the threat model Django defends against by default, what you must still do, and how to avoid the seven classes of web vulnerability.

---

## 1. The OWASP top, mapped to Django

| Threat | Django's default defense | What you must still do |
|--------|--------------------------|------------------------|
| SQL injection | ORM parameterizes queries | Don't use `raw()` with string interpolation |
| XSS (Cross-Site Scripting) | Templates autoescape | Don't `\|safe` user content; sanitize HTML if you must |
| CSRF | `CsrfViewMiddleware` + `{% csrf_token %}` | Include token in every POST form; for AJAX, send `X-CSRFToken` |
| Clickjacking | `XFrameOptionsMiddleware` sets `X-Frame-Options: DENY` | Don't unset it without a reason |
| Insecure cookies | `SESSION_COOKIE_SECURE`, `HTTPONLY` available | Set them in prod |
| Password storage | PBKDF2 hashing | Upgrade to Argon2; enforce password validators |
| Session fixation | Session ID rotates on login | Don't reuse session IDs in custom auth |
| Open redirect | Built-in `redirect()` validates URL targets | Don't trust `?next=` without validation |
| Mass assignment | Forms restrict `fields` / `exclude` | Never `fields = "__all__"` on user-facing forms |
| Host header attacks | `ALLOWED_HOSTS` enforced when `DEBUG=False` | Set it explicitly in prod |

Django defends against most of these *by default*, but the defenses can be bypassed by writing un-idiomatic code (raw SQL, `|safe`, `csrf_exempt`, etc.). Understanding what each defense actually does is what makes you the engineer who doesn't bypass it accidentally.

---

## 2. SQL injection — and why the ORM saves you

Bad (and impossible to write idiomatically in Django):

```python
# DON'T
cursor.execute(f"SELECT * FROM users WHERE name = '{request.GET['name']}'")
# Attacker sends ?name=' OR 1=1 -- → returns every user
```

Good:

```python
cursor.execute("SELECT * FROM users WHERE name = %s", [request.GET["name"]])
```

The ORM always parameterizes:

```python
User.objects.filter(name=request.GET["name"])
# SQL: SELECT * FROM users WHERE name = %s [params: 'whatever']
```

**Where you can still shoot yourself:**

- `Model.objects.raw("... %s" % user_input, ...)` — string-formatting before parameterization
- `cursor.execute("... " + user_input)` — concatenation
- `Model.objects.extra(where=[f"col = {user_input}"])` — `extra()` is dangerous and largely deprecated; avoid
- Building `order_by(user_input)` from query params — Django allows column-name strings; whitelist them

Treat anything from `request.GET`, `request.POST`, `request.headers`, `request.body`, or external APIs as untrusted. The ORM safely parameterizes *values*; you parameterize *identifiers* with allowlists.

---

## 3. XSS — what autoescape protects

Templates escape `<`, `>`, `'`, `"`, `&` by default:

```django
{{ user.bio }}          {# safe: bio="<script>" → rendered as &lt;script&gt; #}
```

You break this when you:

- Use `|safe` on user input.
- Use `{% autoescape off %}{{ user_input }}{% endautoescape %}`.
- Call `mark_safe(user_input)` in view code.
- Render `format_html("<div>{}</div>", user_input)` — actually this *does* escape with `{}` placeholders. **But** `format_html_join` and manual string concat into HTML do not.
- Set `innerHTML = $variable` in templated JavaScript (your template engine doesn't see inside the JS).

If you must render user-supplied HTML (a blog body), sanitize with `bleach`:

```bash
pip install bleach
```

```python
import bleach

ALLOWED_TAGS = ["p", "br", "strong", "em", "a", "ul", "ol", "li", "code", "pre", "blockquote"]
ALLOWED_ATTRS = {"a": ["href", "title", "rel"]}

def sanitize(html):
    return bleach.clean(html, tags=ALLOWED_TAGS, attributes=ALLOWED_ATTRS, strip=True)

# In your model save / view:
post.body = sanitize(form.cleaned_data["body"])
```

Then `{{ post.body|safe }}` is reasonable — you've already removed `<script>`.

For **JavaScript-context** templating (rare):

```django
<script>
  const name = "{{ name|escapejs }}";
</script>
```

`escapejs` escapes per JS rules, not HTML rules.

---

## 4. CSRF — Django's mechanism

A malicious site `evil.com` includes `<form action="https://bank.com/transfer" method="POST">`. The user, logged into `bank.com`, clicks submit — browser sends the bank's session cookie automatically. Without CSRF protection, the transfer goes through.

Django's defense:

1. On every form, embed a random token via `{% csrf_token %}`.
2. Set a `csrftoken` cookie with the same value.
3. On POST, `CsrfViewMiddleware` checks the form token matches the cookie token.

`evil.com` cannot read the `csrftoken` cookie (same-origin policy), so it can't include the token, so its forged form is rejected.

**Always:**
- Include `{% csrf_token %}` inside every `<form method="post">`.
- For AJAX, include `X-CSRFToken: <cookie value>` header.
- Set `SESSION_COOKIE_SAMESITE = "Lax"` (default in Django 4+) — second layer of CSRF defense at the browser.

**Carefully:**
- `@csrf_exempt` for webhook receivers — only when you authenticate them another way (signature header, secret, mTLS).
- DRF views using `TokenAuthentication`/`JWT` are auto-exempt from CSRF (auth is in the header, not the cookie).

**Never:**
- Disable `CsrfViewMiddleware` globally.

---

## 5. Clickjacking and `X-Frame-Options`

Attacker iframes your bank's "transfer money" page, overlays an innocent UI, tricks the user into clicking through the invisible iframe. Defense: refuse to be iframed.

```python
MIDDLEWARE = [..., "django.middleware.clickjacking.XFrameOptionsMiddleware"]
X_FRAME_OPTIONS = "DENY"     # or "SAMEORIGIN"
```

Per-view override (rarely needed):

```python
from django.views.decorators.clickjacking import xframe_options_sameorigin, xframe_options_exempt

@xframe_options_exempt
def embeddable_widget(request):
    ...
```

Modern alternative: `Content-Security-Policy: frame-ancestors 'none'` — same effect, more powerful overall. The `django-csp` package adds full CSP support.

---

## 6. Secrets management

**Never commit:**
- `SECRET_KEY`
- DB passwords
- API keys (AWS, Stripe, SendGrid, etc.)
- OAuth client secrets

Strategies:

1. **Environment variables** — load from `os.environ` in settings. Use `python-decouple` or `django-environ`:

   ```python
   import environ
   env = environ.Env()
   environ.Env.read_env(BASE_DIR / ".env")
   SECRET_KEY = env("SECRET_KEY")
   DEBUG = env.bool("DEBUG", default=False)
   ```

2. **Secret managers** — AWS Secrets Manager, GCP Secret Manager, HashiCorp Vault. Best for production.

3. **`.env` files in development**, never in production images.

If `SECRET_KEY` leaks (commit history, log file), **rotate it**. Existing sessions invalidate; password reset tokens become invalid; signed cookies stop verifying. Plan a rotation procedure.

---

## 7. `SECURE_*` settings — the production checklist

```python
# settings/prod.py
DEBUG = False
ALLOWED_HOSTS = ["myapp.com", "www.myapp.com"]

# HTTPS
SECURE_SSL_REDIRECT = True              # redirect HTTP → HTTPS
SECURE_HSTS_SECONDS = 31536000           # 1 year HSTS
SECURE_HSTS_INCLUDE_SUBDOMAINS = True
SECURE_HSTS_PRELOAD = True
SECURE_PROXY_SSL_HEADER = ("HTTP_X_FORWARDED_PROTO", "https")    # if behind a TLS-terminating proxy

# Cookies
SESSION_COOKIE_SECURE = True
SESSION_COOKIE_HTTPONLY = True
SESSION_COOKIE_SAMESITE = "Lax"
CSRF_COOKIE_SECURE = True
CSRF_COOKIE_HTTPONLY = True

# Headers
SECURE_CONTENT_TYPE_NOSNIFF = True
SECURE_BROWSER_XSS_FILTER = True         # legacy, but harmless
SECURE_REFERRER_POLICY = "same-origin"
X_FRAME_OPTIONS = "DENY"
```

Run Django's built-in audit:

```bash
python manage.py check --deploy
```

It surfaces missing security settings.

---

## 8. Authorization mistakes — the real-world ones

The OWASP `Broken Access Control` category is the most common modern vulnerability:

### IDOR (Insecure Direct Object Reference)

```python
# DON'T
def post_detail(request, pk):
    post = get_object_or_404(Post, pk=pk)
    return render(request, "blog/post_detail.html", {"post": post})
```

If `Post` should only be visible to the owner, the URL `/posts/123/` is exploitable by anyone with the ID. Fix:

```python
def post_detail(request, pk):
    post = get_object_or_404(Post, pk=pk, author=request.user)
    ...
```

Same applies to API endpoints — DRF `ModelViewSet` exposes by PK unless `get_queryset()` filters.

### Function-level access

`@login_required` ≠ "this admin function is admin-only." Always layer:

```python
@login_required
@permission_required("blog.can_publish", raise_exception=True)
def publish(...):
    ...
```

### Mass assignment via `fields = "__all__"`

```python
class UserForm(forms.ModelForm):
    class Meta:
        model = User
        fields = "__all__"      # includes is_staff, is_superuser, password
```

An attacker POSTs `is_superuser=on`, becomes admin. **Always explicit-list `fields`.**

---

## 9. Open redirect

```python
# DON'T
def post_login_redirect(request):
    return redirect(request.GET["next"])
```

Attacker sends `https://myapp.com/login/?next=https://evil.com` — phishing.

Use Django's helper:

```python
from django.utils.http import url_has_allowed_host_and_scheme

next_url = request.GET.get("next")
if next_url and url_has_allowed_host_and_scheme(next_url, allowed_hosts={request.get_host()}):
    return redirect(next_url)
return redirect("home")
```

Or trust Django's built-in `LoginView` which already does this.

---

## 10. Password hashing and validators

```python
PASSWORD_HASHERS = [
    "django.contrib.auth.hashers.Argon2PasswordHasher",     # install argon2-cffi
    "django.contrib.auth.hashers.PBKDF2PasswordHasher",
    "django.contrib.auth.hashers.BCryptSHA256PasswordHasher",
]

AUTH_PASSWORD_VALIDATORS = [
    {"NAME": "django.contrib.auth.password_validation.UserAttributeSimilarityValidator"},
    {"NAME": "django.contrib.auth.password_validation.MinimumLengthValidator", "OPTIONS": {"min_length": 12}},
    {"NAME": "django.contrib.auth.password_validation.CommonPasswordValidator"},
    {"NAME": "django.contrib.auth.password_validation.NumericPasswordValidator"},
]
```

Argon2 is the modern recommendation. Old PBKDF2 hashes auto-upgrade on next login (the first hasher in the list is the active one).

Throttle login attempts: `django-axes` locks the account after N failed tries.

---

## 11. Rate limiting and brute-force

`django-axes` for login throttling. DRF `AnonRateThrottle`/`UserRateThrottle` for API throttling. Add `django-ratelimit` for arbitrary views:

```python
from ratelimit.decorators import ratelimit

@ratelimit(key="ip", rate="5/m", method="POST", block=True)
def password_reset(request):
    ...
```

---

## 12. Logging and sensitive data

Don't log secrets, passwords, tokens, full request bodies, or full cookie headers. Django filters sensitive fields out of debug error pages automatically — but your *own* logging code can leak them.

```python
from django.views.decorators.debug import sensitive_post_parameters, sensitive_variables

@sensitive_post_parameters("password")
def login(request):
    ...

@sensitive_variables("password", "credit_card")
def process_payment(user, password, credit_card):
    ...
```

These decorators tell Django's error pages to redact those values.

---

## 13. Dependency hygiene

Outdated dependencies are a huge attack surface. Tools:

- `pip-audit` — scans `requirements.txt` against known CVEs.
- `safety` — same idea.
- GitHub Dependabot — automated PRs to bump vulnerable deps.

```bash
pip install pip-audit
pip-audit
```

Run in CI. Block PRs that introduce vulnerable deps.

---

## 14. Practical application — secure post detail view

```python
# blog/views.py
from django.shortcuts import get_object_or_404, redirect
from django.contrib.auth.decorators import login_required
from django.core.exceptions import PermissionDenied
from django.utils.http import url_has_allowed_host_and_scheme
from .models import Post

@login_required
def post_detail(request, pk):
    post = get_object_or_404(Post, pk=pk)
    if post.status != "published" and post.author_id != request.user.id and not request.user.is_staff:
        raise PermissionDenied
    return render(request, "blog/post_detail.html", {"post": post})

@login_required
def post_published_redirect(request, pk):
    post = get_object_or_404(Post, pk=pk, author=request.user)
    post.status = "published"
    post.save()
    next_url = request.GET.get("next")
    if next_url and url_has_allowed_host_and_scheme(next_url, allowed_hosts={request.get_host()}):
        return redirect(next_url)
    return redirect("blog:detail", pk=pk)
```

```python
# settings/prod.py — defense-in-depth checklist
DEBUG = False
ALLOWED_HOSTS = env.list("ALLOWED_HOSTS")
SECRET_KEY = env("SECRET_KEY")
SECURE_SSL_REDIRECT = True
SECURE_HSTS_SECONDS = 31536000
SECURE_HSTS_INCLUDE_SUBDOMAINS = True
SECURE_HSTS_PRELOAD = True
SECURE_PROXY_SSL_HEADER = ("HTTP_X_FORWARDED_PROTO", "https")
SESSION_COOKIE_SECURE = True
SESSION_COOKIE_HTTPONLY = True
SESSION_COOKIE_SAMESITE = "Lax"
CSRF_COOKIE_SECURE = True
SECURE_CONTENT_TYPE_NOSNIFF = True
SECURE_REFERRER_POLICY = "same-origin"
X_FRAME_OPTIONS = "DENY"
```

---

## 15. Common mistakes and gotchas

1. **`DEBUG=True` in production.** Yellow error page leaks `SECRET_KEY`, env vars, source code, DB credentials. The most common Django security incident.
2. **Committing `SECRET_KEY` to git.** It's there forever in history. Rotate it; use env vars.
3. **`ALLOWED_HOSTS = ["*"]` in production.** Bypasses Host header validation. Attackers can spoof for password reset URL generation.
4. **`fields = "__all__"` in user-facing forms.** Mass assignment to `is_staff` / `is_superuser`. Always explicit-list.
5. **`|safe` on unsanitized user input.** XSS one-shot. Sanitize with `bleach` before marking safe.
6. **Custom raw SQL with string interpolation.** SQL injection. Always parameterize with `%s`.
7. **`@csrf_exempt` for "convenience".** Whole-class CSRF protection gone. Only for endpoints with alternative auth.
8. **Open redirects via `?next=`.** Phishing. Use `url_has_allowed_host_and_scheme`.
9. **IDOR — fetching by PK without ownership check.** `/posts/{id}/` exposes everything. Filter by `author=request.user` or use `has_object_permission`.
10. **Storing JWTs in `localStorage`.** XSS-readable. Use HttpOnly cookies for browser apps.
11. **Forgetting `manage.py check --deploy`.** Run it in CI; treat warnings as failures.
12. **Logging sensitive request bodies.** Login forms, payment forms — wrap views with `@sensitive_post_parameters`.
13. **Trusting `request.META["REMOTE_ADDR"]` behind a proxy.** It's the proxy's IP. Use `X-Forwarded-For` carefully (it's spoofable from the client) and configure middleware to trust only your known proxy.
14. **Skipping HTTPS in staging.** Bugs found only in production. Use Let's Encrypt; HTTPS everywhere.

---

## 🎯 Key Takeaways

- **Django defends against the OWASP top by default.** Your job is to not break those defenses (no `|safe` on input, no `csrf_exempt` for convenience, no `fields = "__all__"`, no string-interpolated SQL).
- **Authorization is on you.** Authentication ("who are you?") is Django's. Authorization ("can *you* do *this* to *that* object?") is yours. IDOR is the #1 modern web vuln.
- **`manage.py check --deploy` + the `SECURE_*` settings checklist** is the production security baseline. Run it in CI.
- **Rotate `SECRET_KEY` if it leaks; use env vars + a secret manager.** Never commit secrets — and assume any committed secret is permanently compromised.
- **Defense in depth.** CSRF token + SameSite cookie. HTTPS + HSTS. Argon2 hashing + password validators + rate limiting + 2FA on the admin. Each layer covers another's failure mode.

*← [prev](./14_testing.md) | [next →](./16_production.md)*
