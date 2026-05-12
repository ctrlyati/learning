# 06 — Forms and Validation

> **Goal:** Accept user input safely — validate, sanitize, and persist with `Form`, `ModelForm`, formsets, and CSRF.

---

## 1. Why Django forms exist

A web form is a four-step pipeline:

1. **Render** the HTML inputs.
2. **Receive** the POST data.
3. **Validate** each field, then the form as a whole.
4. **Process** — save to DB, send an email, etc.

You *could* do this manually with HTML and request parsing. Django forms automate all four and ship with built-in protection against malformed inputs, type errors, and CSRF.

```python
# blog/forms.py
from django import forms

class ContactForm(forms.Form):
    name = forms.CharField(max_length=100)
    email = forms.EmailField()
    subject = forms.CharField(max_length=200)
    body = forms.CharField(widget=forms.Textarea)
    cc_myself = forms.BooleanField(required=False)
```

```python
# blog/views.py
from django.shortcuts import render, redirect
from .forms import ContactForm

def contact(request):
    if request.method == "POST":
        form = ContactForm(request.POST)
        if form.is_valid():
            # form.cleaned_data is a dict of validated, typed values
            send_email(form.cleaned_data)
            return redirect("contact-thanks")
    else:
        form = ContactForm()
    return render(request, "contact.html", {"form": form})
```

```django
{# templates/contact.html #}
<form method="post">
  {% csrf_token %}
  {{ form.as_p }}
  <button type="submit">Send</button>
</form>
```

`{{ form.as_p }}` renders each field wrapped in a `<p>` (also `as_div`, `as_table`, `as_ul`, or manual rendering). The form handles labels, help text, error display, and HTML escaping.

---

## 2. The full field catalogue

Mirrors model fields but with form-layer concerns:

| Field | What it accepts | Notes |
|-------|-----------------|-------|
| `CharField` | String | `max_length`, `min_length`, `strip=True` |
| `IntegerField` | Int | `min_value`, `max_value` |
| `DecimalField`, `FloatField` | Number | |
| `BooleanField` | Checkbox | Always pass `required=False` for optional |
| `DateField`, `DateTimeField` | Date | `input_formats=[...]` |
| `EmailField`, `URLField`, `UUIDField` | Validated string | |
| `ChoiceField`, `MultipleChoiceField` | Select | `choices=[...]` |
| `ModelChoiceField`, `ModelMultipleChoiceField` | Select tied to queryset | |
| `FileField`, `ImageField` | Uploaded file | Need `enctype="multipart/form-data"` |
| `JSONField` | Validated JSON | |
| `RegexField` | String matching regex | |

Field options worth knowing:

- `required=True` (default) — submitting empty raises validation error.
- `label="..."`, `help_text="..."` — UI text.
- `initial=...` — pre-fill when form is unbound.
- `widget=forms.Textarea(attrs={"rows": 4, "class": "form-control"})` — override widget and HTML attrs.
- `validators=[...]` — additional validators (see §4).
- `error_messages={"required": "Please enter your name"}` — override per-field errors.

---

## 3. `is_valid()`, `cleaned_data`, and `errors`

The cycle is always the same:

```python
form = ContactForm(request.POST)         # bound
if form.is_valid():                       # runs full validation
    name = form.cleaned_data["name"]      # typed values
else:
    print(form.errors)                    # dict of field -> [messages]
    print(form.non_field_errors())        # form-wide errors
```

After `is_valid()`, `cleaned_data` exists and contains *converted* values — `cleaned_data["age"]` is `int`, `cleaned_data["email"]` is a normalized email string, `cleaned_data["birthdate"]` is a `date` object.

A form without data is **unbound** (`form = ContactForm()`) and just renders. A form with data is **bound** (`form = ContactForm(request.POST)`) and can be validated.

---

## 4. Validation — three layers

### Layer 1: field-level (`clean_<fieldname>`)

```python
class ContactForm(forms.Form):
    email = forms.EmailField()

    def clean_email(self):
        email = self.cleaned_data["email"]
        if email.endswith("@competitor.com"):
            raise forms.ValidationError("Sorry, we don't accept those.")
        return email.lower()    # return the cleaned value
```

The method must be `clean_<fieldname>`, must `return` the value (cleaned or as-is), and may raise `ValidationError`.

### Layer 2: form-level (`clean`)

For cross-field rules:

```python
def clean(self):
    cleaned = super().clean()
    pw1, pw2 = cleaned.get("password1"), cleaned.get("password2")
    if pw1 and pw2 and pw1 != pw2:
        raise forms.ValidationError("Passwords don't match.")
    return cleaned
```

Or attach the error to a specific field:

```python
def clean(self):
    cleaned = super().clean()
    if cleaned.get("end") < cleaned.get("start"):
        self.add_error("end", "End must be after start.")
```

### Layer 3: validators (reusable)

```python
from django.core.validators import RegexValidator, MinLengthValidator

phone_validator = RegexValidator(r"^\+?1?\d{9,15}$", "Invalid phone number")

class ProfileForm(forms.Form):
    phone = forms.CharField(validators=[phone_validator, MinLengthValidator(10)])
```

Validators are reusable callables — a senior pattern for sharing rules across forms and models.

---

## 5. `ModelForm` — derive form from model

When the form persists to a model, don't repeat field definitions:

```python
# blog/forms.py
from django import forms
from .models import Post

class PostForm(forms.ModelForm):
    class Meta:
        model = Post
        fields = ["title", "slug", "body", "category", "tags"]
        # or: exclude = ["author", "created_at"]
        widgets = {
            "body": forms.Textarea(attrs={"rows": 10, "class": "form-control"}),
        }
        labels = {"body": "Content"}
        help_texts = {"slug": "URL-safe identifier"}
```

```python
# blog/views.py
def post_create(request):
    if request.method == "POST":
        form = PostForm(request.POST)
        if form.is_valid():
            post = form.save(commit=False)    # don't hit DB yet
            post.author = request.user        # fill in fields not in the form
            post.save()
            form.save_m2m()                   # save tags (M2M needs explicit call)
            return redirect(post.get_absolute_url())
    else:
        form = PostForm()
    return render(request, "blog/post_form.html", {"form": form})
```

`form.save(commit=False)` is the senior pattern when the form doesn't capture every required field. `save_m2m()` follows because M2M can't be saved until the parent has a `pk`.

`ModelForm` automatically runs `Model.full_clean()`, which validates field types, choices, and `clean_*` methods on the model — so model-level validation is honored.

---

## 6. Manual field rendering

When `{{ form.as_p }}` isn't enough:

```django
<form method="post" class="space-y-4">
  {% csrf_token %}

  {{ form.non_field_errors }}

  <div class="form-group {% if form.title.errors %}has-error{% endif %}">
    <label for="{{ form.title.id_for_label }}">{{ form.title.label }}</label>
    {{ form.title }}
    {% if form.title.errors %}
      <small class="error">{{ form.title.errors.0 }}</small>
    {% endif %}
    {% if form.title.help_text %}
      <small class="help">{{ form.title.help_text }}</small>
    {% endif %}
  </div>

  <button type="submit">Save</button>
</form>
```

Or loop:

```django
{% for field in form %}
  <div class="form-group">
    {{ field.label_tag }} {{ field }}
    {{ field.errors }}
  </div>
{% endfor %}
```

For real projects, install **`django-crispy-forms`** or **`django-widget-tweaks`** — they make styling forms with Bootstrap/Tailwind a one-liner.

---

## 7. Formsets — N forms on one page

A `FormSet` lets you submit a variable number of forms together — useful for "add multiple line items" UIs:

```python
from django.forms import formset_factory

ContactFormSet = formset_factory(ContactForm, extra=3, max_num=10)

def bulk_contact(request):
    if request.method == "POST":
        formset = ContactFormSet(request.POST)
        if formset.is_valid():
            for form in formset:
                if form.cleaned_data:        # skip empty extras
                    save_contact(form.cleaned_data)
            return redirect("done")
    else:
        formset = ContactFormSet()
    return render(request, "bulk_contact.html", {"formset": formset})
```

```django
<form method="post">
  {% csrf_token %}
  {{ formset.management_form }}        {# REQUIRED hidden fields #}
  {% for form in formset %}
    {{ form.as_p }}
  {% endfor %}
  <button type="submit">Save all</button>
</form>
```

`{{ formset.management_form }}` is mandatory — it carries `TOTAL_FORMS`, `INITIAL_FORMS`, etc., so the server knows how many to expect. Forget it and you'll see a `ManagementForm data is missing` error.

### `inlineformset_factory` — child rows of a parent model

The classic "edit a post + its tags/sections on one page":

```python
from django.forms import inlineformset_factory
from .models import Post, Section

SectionFormSet = inlineformset_factory(Post, Section, fields=["title", "body"], extra=2, can_delete=True)

def edit_post(request, pk):
    post = get_object_or_404(Post, pk=pk)
    if request.method == "POST":
        form = PostForm(request.POST, instance=post)
        formset = SectionFormSet(request.POST, instance=post)
        if form.is_valid() and formset.is_valid():
            form.save()
            formset.save()
            return redirect(post.get_absolute_url())
    else:
        form = PostForm(instance=post)
        formset = SectionFormSet(instance=post)
    return render(request, "blog/edit.html", {"form": form, "formset": formset})
```

---

## 8. CSRF — what `{% csrf_token %}` actually does

Cross-Site Request Forgery: a malicious site submits a form to *your* site using the user's logged-in cookie. Without CSRF protection, the bank transfer goes through.

Django's defense:
1. On every form, embed a random token in a hidden input (`{% csrf_token %}`).
2. Also set a cookie with the same token (`csrftoken`).
3. On POST, `CsrfViewMiddleware` checks that the form's token matches the cookie's token. Mismatch → 403.

A cross-site attacker can read the cookie (it's set on your domain) but can't read it from their attacker site (same-origin policy). So they can't put it in the form.

**Always include `{% csrf_token %}` inside every `<form method="post">`.** For AJAX, include the token as an `X-CSRFToken` header (Django docs have a JS snippet for this).

To exempt a view (e.g., a webhook receiver with its own auth):

```python
from django.views.decorators.csrf import csrf_exempt

@csrf_exempt
def stripe_webhook(request):
    # verify Stripe signature instead
    ...
```

Use `csrf_exempt` only when you have an alternative authentication mechanism. It's not "off because forms are annoying."

---

## 9. File uploads

Need `enctype="multipart/form-data"` on the form and `request.FILES` in the view:

```python
class AvatarForm(forms.Form):
    avatar = forms.ImageField()

def upload(request):
    if request.method == "POST":
        form = AvatarForm(request.POST, request.FILES)    # note request.FILES
        if form.is_valid():
            f = form.cleaned_data["avatar"]
            # save somewhere — see module 9 for MEDIA_ROOT
```

For `ModelForm` with an `ImageField`/`FileField`:

```python
form = ProfileForm(request.POST, request.FILES, instance=request.user.profile)
form.save()    # writes file to MEDIA_ROOT
```

Module 9 covers storage backends (S3, GCS) so uploaded files don't live on a single web server.

---

## 10. Practical application — post create/edit flow

```python
# blog/forms.py
from django import forms
from django.utils.text import slugify
from .models import Post

class PostForm(forms.ModelForm):
    class Meta:
        model = Post
        fields = ["title", "slug", "body", "category", "tags", "status"]
        widgets = {
            "body": forms.Textarea(attrs={"rows": 12, "class": "form-control"}),
            "title": forms.TextInput(attrs={"class": "form-control"}),
            "slug": forms.TextInput(attrs={"class": "form-control"}),
        }

    def clean_slug(self):
        slug = self.cleaned_data["slug"] or slugify(self.cleaned_data["title"])
        qs = Post.objects.filter(slug=slug)
        if self.instance.pk:
            qs = qs.exclude(pk=self.instance.pk)
        if qs.exists():
            raise forms.ValidationError("That slug is taken.")
        return slug

    def clean(self):
        cleaned = super().clean()
        if cleaned.get("status") == "published" and not cleaned.get("body"):
            raise forms.ValidationError("Can't publish an empty post.")
        return cleaned
```

```python
# blog/views.py
from django.contrib.auth.decorators import login_required
from django.shortcuts import render, redirect, get_object_or_404
from .forms import PostForm
from .models import Post

@login_required
def post_create(request):
    form = PostForm(request.POST or None)
    if request.method == "POST" and form.is_valid():
        post = form.save(commit=False)
        post.author = request.user
        post.save()
        form.save_m2m()
        return redirect(post.get_absolute_url())
    return render(request, "blog/post_form.html", {"form": form, "mode": "create"})

@login_required
def post_edit(request, pk):
    post = get_object_or_404(Post, pk=pk, author=request.user)
    form = PostForm(request.POST or None, instance=post)
    if request.method == "POST" and form.is_valid():
        form.save()
        return redirect(post.get_absolute_url())
    return render(request, "blog/post_form.html", {"form": form, "mode": "edit"})
```

```django
{# blog/templates/blog/post_form.html #}
{% extends "base.html" %}
{% block content %}
  <h1>{{ mode|title }} Post</h1>
  <form method="post" novalidate>
    {% csrf_token %}
    {{ form.non_field_errors }}
    {% for field in form %}
      <div class="form-group">
        {{ field.label_tag }}
        {{ field }}
        {{ field.errors }}
      </div>
    {% endfor %}
    <button type="submit" class="btn btn-primary">Save</button>
  </form>
{% endblock %}
```

---

## 11. Common mistakes and gotchas

1. **Forgetting `{% csrf_token %}`.** Form POST returns 403. Look for it in every `<form method="post">`.
2. **Forgetting `request.FILES` for uploads.** Files silently don't bind to the form. Check `form.is_valid()` says false with no obvious error → it's this.
3. **Forgetting `form.save_m2m()` after `save(commit=False)`.** Tags/M2M relations silently drop.
4. **Returning the wrong value from `clean_<field>`.** Forgetting to `return` means `cleaned_data[field]` becomes `None`. Always `return value`.
5. **Catching `ValidationError` somewhere that swallows it.** A field's `clean_*` should `raise` — Django collects them into `form.errors`. Don't try/except inside `clean_*`.
6. **Field order in `Meta.fields`.** It defines render order. Putting `fields = "__all__"` accepts every model field including sensitive ones (`is_staff`, etc.) — dangerous for user-facing forms.
7. **Trusting `BooleanField` default = `required=True`.** Checkboxes that aren't checked send nothing → validation fails. Always `required=False` for optional checkboxes.
8. **Sharing form instances across requests.** Forms have state. Build a new one per request.
9. **`csrf_exempt` for "convenience".** Now you have CSRF nowhere. Only exempt when you have an alternative auth (webhook signatures, token auth).
10. **Forgetting `{{ formset.management_form }}`.** Formsets explode without it.
11. **Validating in the view instead of the form.** Then you can't reuse the form, and validation logic scatters. Forms are the right home for "is this input acceptable."

---

## 🎯 Key Takeaways

- **`ModelForm` is the default for model-backed forms.** Don't repeat schema. Use `commit=False` + `save_m2m()` when filling in fields the form doesn't expose.
- **Three validation layers:** `clean_<field>` for per-field rules, `clean()` for cross-field rules, `validators=[...]` for reusable rules.
- **`{% csrf_token %}` belongs in every POST form.** Django's CSRF defense is automatic if you don't fight it. Never `csrf_exempt` for convenience.
- **Formsets and `inlineformset_factory`** handle "edit N child rows on one page" — the parent-children CRUD pattern that's tedious to roll by hand.
- **Forms own input validation; views own orchestration.** Keep "is this input legal?" inside the form, and "what happens after it's saved?" inside the view.

*← [prev](./05_orm_deep_dive.md) | [next →](./07_auth.md)*
