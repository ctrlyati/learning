# 11 — Django REST Framework Essentials

> **Goal:** Build clean, secured, paginated, throttled JSON APIs using Django REST Framework's idiomatic stack.

---

## 1. Why DRF — and what it actually is

Django ships `JsonResponse`. You *could* build an API with FBVs returning JSON. You shouldn't. DRF gives you:

- **Serializers** — translate model instances ↔ Python dicts ↔ JSON, with validation.
- **Generic views / ViewSets** — list, retrieve, create, update, delete with ~3 lines.
- **Routers** — automatic URL wiring from a ViewSet.
- **Authentication** — Token, JWT, Session, OAuth, all pluggable.
- **Permissions** — `IsAuthenticated`, `IsOwner`, `DjangoModelPermissions`, custom.
- **Pagination, throttling, content negotiation, browsable API** — all default.

```bash
pip install djangorestframework
```

```python
# settings.py
INSTALLED_APPS += ["rest_framework"]
REST_FRAMEWORK = {
    "DEFAULT_AUTHENTICATION_CLASSES": ["rest_framework.authentication.SessionAuthentication"],
    "DEFAULT_PERMISSION_CLASSES": ["rest_framework.permissions.IsAuthenticated"],
    "DEFAULT_PAGINATION_CLASS": "rest_framework.pagination.PageNumberPagination",
    "PAGE_SIZE": 20,
    "DEFAULT_THROTTLE_CLASSES": ["rest_framework.throttling.UserRateThrottle", "rest_framework.throttling.AnonRateThrottle"],
    "DEFAULT_THROTTLE_RATES": {"user": "1000/day", "anon": "100/day"},
}
```

---

## 2. Serializers — the translation layer

### `Serializer` — the verbose form

```python
# blog/api/serializers.py
from rest_framework import serializers

class PostSerializer(serializers.Serializer):
    id = serializers.IntegerField(read_only=True)
    title = serializers.CharField(max_length=200)
    body = serializers.CharField()
    author = serializers.StringRelatedField(read_only=True)
    created_at = serializers.DateTimeField(read_only=True)

    def create(self, validated_data):
        return Post.objects.create(**validated_data)

    def update(self, instance, validated_data):
        for field, value in validated_data.items():
            setattr(instance, field, value)
        instance.save()
        return instance
```

### `ModelSerializer` — the 95% case

```python
from rest_framework import serializers
from blog.models import Post

class PostSerializer(serializers.ModelSerializer):
    author = serializers.StringRelatedField(read_only=True)

    class Meta:
        model = Post
        fields = ["id", "title", "slug", "body", "author", "status", "created_at"]
        read_only_fields = ["slug", "created_at"]
```

`ModelSerializer` auto-generates fields, validators, and `create`/`update` methods from the model. The relationship to `ModelForm` is intentional — same pattern, JSON instead of HTML.

Usage:

```python
# serialize (model → dict)
serializer = PostSerializer(post)
serializer.data    # {"id": 1, "title": ...}

# serialize many
PostSerializer(Post.objects.all(), many=True).data

# deserialize and validate (dict → validated_data)
serializer = PostSerializer(data=request.data)
if serializer.is_valid():
    post = serializer.save(author=request.user)    # pass extra fields here
else:
    errors = serializer.errors
```

### Validation hooks

Same shape as `Form`:

```python
class PostSerializer(serializers.ModelSerializer):
    class Meta:
        model = Post
        fields = "__all__"

    def validate_title(self, value):
        if "clickbait" in value.lower():
            raise serializers.ValidationError("No clickbait.")
        return value

    def validate(self, attrs):
        if attrs.get("status") == "published" and not attrs.get("body"):
            raise serializers.ValidationError({"body": "Can't publish empty body."})
        return attrs
```

### Nested serializers

```python
class CommentSerializer(serializers.ModelSerializer):
    author = serializers.StringRelatedField()
    class Meta:
        model = Comment
        fields = ["id", "author", "body", "created_at"]

class PostDetailSerializer(serializers.ModelSerializer):
    author = serializers.StringRelatedField()
    comments = CommentSerializer(many=True, read_only=True)

    class Meta:
        model = Post
        fields = ["id", "title", "body", "author", "comments"]
```

Use nested for *reads*. For writes, nested gets complex — usually you POST to a separate `/comments/` endpoint.

---

## 3. Views — generics and ViewSets

### Class-based generics

DRF has its own generic CBVs:

```python
# blog/api/views.py
from rest_framework import generics
from blog.models import Post
from .serializers import PostSerializer

class PostList(generics.ListCreateAPIView):
    queryset = Post.objects.all()
    serializer_class = PostSerializer

    def perform_create(self, serializer):
        serializer.save(author=self.request.user)

class PostDetail(generics.RetrieveUpdateDestroyAPIView):
    queryset = Post.objects.all()
    serializer_class = PostSerializer
```

```python
# blog/api/urls.py
from django.urls import path
from . import views

urlpatterns = [
    path("posts/", views.PostList.as_view()),
    path("posts/<int:pk>/", views.PostDetail.as_view()),
]
```

That's a full CRUD API in ~10 lines.

The generics hierarchy:

```
GenericAPIView
├── ListAPIView                  GET list
├── CreateAPIView                POST
├── RetrieveAPIView              GET one
├── UpdateAPIView                PUT/PATCH
├── DestroyAPIView               DELETE
├── ListCreateAPIView            (combined)
├── RetrieveUpdateAPIView
├── RetrieveDestroyAPIView
└── RetrieveUpdateDestroyAPIView
```

### ViewSets — even less code

A `ViewSet` bundles the five CRUD actions into one class. Combined with a `Router`, you write zero URLs:

```python
# blog/api/views.py
from rest_framework import viewsets, permissions
from blog.models import Post
from .serializers import PostSerializer

class PostViewSet(viewsets.ModelViewSet):
    queryset = Post.objects.select_related("author").prefetch_related("tags")
    serializer_class = PostSerializer
    permission_classes = [permissions.IsAuthenticatedOrReadOnly]

    def perform_create(self, serializer):
        serializer.save(author=self.request.user)

    def get_queryset(self):
        qs = super().get_queryset()
        if self.request.user.is_staff:
            return qs
        return qs.filter(status="published")
```

```python
# blog/api/urls.py
from rest_framework.routers import DefaultRouter
from . import views

router = DefaultRouter()
router.register(r"posts", views.PostViewSet, basename="post")

urlpatterns = router.urls
```

Mount at `path("api/", include("blog.api.urls"))` and you get:

```
GET  /api/posts/         → list
POST /api/posts/         → create
GET  /api/posts/{id}/    → retrieve
PUT  /api/posts/{id}/    → update
PATCH /api/posts/{id}/   → partial update
DELETE /api/posts/{id}/  → delete
```

### Custom actions

```python
from rest_framework.decorators import action
from rest_framework.response import Response

class PostViewSet(viewsets.ModelViewSet):
    ...
    @action(detail=True, methods=["post"], permission_classes=[permissions.IsAuthenticated])
    def publish(self, request, pk=None):
        post = self.get_object()
        post.status = "published"
        post.save()
        return Response(PostSerializer(post).data)

    @action(detail=False)
    def trending(self, request):
        qs = self.get_queryset().filter(view_count__gt=1000)
        page = self.paginate_queryset(qs)
        return self.get_paginated_response(PostSerializer(page, many=True).data)
```

Router auto-routes them to `/api/posts/{id}/publish/` and `/api/posts/trending/`.

---

## 4. Authentication classes

```python
REST_FRAMEWORK = {
    "DEFAULT_AUTHENTICATION_CLASSES": [
        "rest_framework.authentication.SessionAuthentication",      # browser SPA
        "rest_framework.authentication.TokenAuthentication",        # mobile / scripts
    ],
}

INSTALLED_APPS += ["rest_framework.authtoken"]
```

After `migrate`, generate a token:

```python
from rest_framework.authtoken.models import Token
token, _ = Token.objects.get_or_create(user=user)
print(token.key)
```

Client sends:

```
Authorization: Token 9944b09199c62bcf9418ad846dd0e4bbdfc6ee4b
```

For JWT (stateless tokens), install `djangorestframework-simplejwt`:

```bash
pip install djangorestframework-simplejwt
```

```python
REST_FRAMEWORK["DEFAULT_AUTHENTICATION_CLASSES"] = [
    "rest_framework_simplejwt.authentication.JWTAuthentication",
]

# urls.py
from rest_framework_simplejwt.views import TokenObtainPairView, TokenRefreshView

urlpatterns += [
    path("api/token/", TokenObtainPairView.as_view()),
    path("api/token/refresh/", TokenRefreshView.as_view()),
]
```

Token vs JWT decision: Token = DB-backed (revocable, slower); JWT = stateless (fast, hard to revoke without a denylist). Many production APIs use JWT for access + a refresh token cycle.

---

## 5. Permissions

Built-in:

| Class | Allows |
|-------|--------|
| `AllowAny` | Anyone |
| `IsAuthenticated` | Logged in |
| `IsAdminUser` | `user.is_staff` |
| `IsAuthenticatedOrReadOnly` | Anyone GET; logged-in for write |
| `DjangoModelPermissions` | Django model perms (`add_post` etc.) |
| `DjangoObjectPermissions` | Object-level (needs `django-guardian` or similar) |

Custom — implement `has_permission` and/or `has_object_permission`:

```python
from rest_framework import permissions

class IsAuthorOrReadOnly(permissions.BasePermission):
    def has_object_permission(self, request, view, obj):
        if request.method in permissions.SAFE_METHODS:
            return True
        return obj.author == request.user
```

Apply per-view:

```python
class PostViewSet(viewsets.ModelViewSet):
    permission_classes = [permissions.IsAuthenticatedOrReadOnly, IsAuthorOrReadOnly]
```

`has_permission` runs on the list/create. `has_object_permission` runs on retrieve/update/delete, after the view fetches the object.

---

## 6. Pagination

Globally:

```python
REST_FRAMEWORK = {
    "DEFAULT_PAGINATION_CLASS": "rest_framework.pagination.PageNumberPagination",
    "PAGE_SIZE": 25,
}
```

Now `GET /api/posts/` returns:

```json
{
  "count": 142,
  "next": "http://api.example.com/posts/?page=2",
  "previous": null,
  "results": [...]
}
```

Three built-in pagers:

| Class | Style |
|-------|-------|
| `PageNumberPagination` | `?page=3` |
| `LimitOffsetPagination` | `?limit=20&offset=40` |
| `CursorPagination` | `?cursor=...` — best for infinite scroll, deep pages, stable ordering |

Use `CursorPagination` for any list ordered by `-created_at` with > 10k rows — page-number gets slow at high offsets.

---

## 7. Throttling

Same pattern — global defaults plus per-view overrides:

```python
REST_FRAMEWORK = {
    "DEFAULT_THROTTLE_CLASSES": [
        "rest_framework.throttling.AnonRateThrottle",
        "rest_framework.throttling.UserRateThrottle",
    ],
    "DEFAULT_THROTTLE_RATES": {
        "anon": "100/day",
        "user": "1000/day",
        "login": "5/min",         # named scope for custom throttles
    },
}
```

Scoped throttle for a specific action:

```python
from rest_framework.throttling import ScopedRateThrottle

class LoginView(APIView):
    throttle_classes = [ScopedRateThrottle]
    throttle_scope = "login"
```

DRF stores counts in the cache backend — Redis recommended for any real deployment (module 12).

---

## 8. Filtering and search

Install `django-filter`:

```bash
pip install django-filter
```

```python
# settings.py
REST_FRAMEWORK["DEFAULT_FILTER_BACKENDS"] = [
    "django_filters.rest_framework.DjangoFilterBackend",
    "rest_framework.filters.SearchFilter",
    "rest_framework.filters.OrderingFilter",
]

# views
class PostViewSet(viewsets.ModelViewSet):
    queryset = Post.objects.all()
    serializer_class = PostSerializer
    filterset_fields = ["status", "category", "author__username"]
    search_fields = ["title", "body"]
    ordering_fields = ["created_at", "view_count"]
    ordering = ["-created_at"]   # default
```

Now `GET /api/posts/?status=published&search=django&ordering=-view_count` works.

---

## 9. Practical application — full blog API

```python
# blog/api/serializers.py
from rest_framework import serializers
from blog.models import Post, Comment

class CommentSerializer(serializers.ModelSerializer):
    author = serializers.StringRelatedField(read_only=True)
    class Meta:
        model = Comment
        fields = ["id", "author", "body", "created_at"]

class PostSerializer(serializers.ModelSerializer):
    author = serializers.StringRelatedField(read_only=True)
    comment_count = serializers.IntegerField(source="comments.count", read_only=True)

    class Meta:
        model = Post
        fields = ["id", "title", "slug", "body", "status", "author", "created_at", "comment_count"]
        read_only_fields = ["slug", "created_at"]
```

```python
# blog/api/views.py
from rest_framework import viewsets, permissions
from rest_framework.decorators import action
from rest_framework.response import Response
from blog.models import Post, Comment
from .serializers import PostSerializer, CommentSerializer

class IsAuthorOrReadOnly(permissions.BasePermission):
    def has_object_permission(self, request, view, obj):
        if request.method in permissions.SAFE_METHODS:
            return True
        return obj.author_id == request.user.id

class PostViewSet(viewsets.ModelViewSet):
    queryset = Post.objects.select_related("author").prefetch_related("comments", "tags")
    serializer_class = PostSerializer
    permission_classes = [permissions.IsAuthenticatedOrReadOnly, IsAuthorOrReadOnly]
    filterset_fields = ["status", "category"]
    search_fields = ["title", "body"]
    ordering_fields = ["created_at", "view_count"]

    def perform_create(self, serializer):
        serializer.save(author=self.request.user)

    @action(detail=True, methods=["get"])
    def comments(self, request, pk=None):
        post = self.get_object()
        comments = post.comments.select_related("author")
        page = self.paginate_queryset(comments)
        ser = CommentSerializer(page or comments, many=True)
        return self.get_paginated_response(ser.data) if page else Response(ser.data)
```

```python
# blog/api/urls.py
from rest_framework.routers import DefaultRouter
from . import views

router = DefaultRouter()
router.register(r"posts", views.PostViewSet)
urlpatterns = router.urls
```

```python
# mysite/urls.py
urlpatterns += [path("api/", include("blog.api.urls"))]
```

You now have a paginated, filterable, searchable, ownership-protected REST API at `/api/posts/`, with `/api/posts/{id}/comments/` for sub-resources. Browse it at `/api/posts/` — DRF's HTML browsable API works out of the box.

---

## 10. Common mistakes and gotchas

1. **N+1 queries in `ModelSerializer`.** `author = StringRelatedField()` on a list view does 1+N queries unless the queryset uses `select_related`. Always tune the queryset.
2. **Forgetting `many=True`.** `PostSerializer(qs)` on a queryset returns garbage; `PostSerializer(qs, many=True)` is correct.
3. **`save()` arguments.** `serializer.save(author=request.user)` injects fields that aren't in the request body — the canonical pattern for setting the owner.
4. **Mixing nested writes with read-only.** A nested serializer is read-only by default. Writable nested serializers require overriding `create()`/`update()` — usually not worth it; expose sub-resources separately.
5. **`PUT` vs `PATCH` confusion.** PUT replaces the entire resource (omitted fields → cleared or invalid). PATCH partial-updates. DRF's `update()` honors this via `partial=True`.
6. **`permission_classes` order doesn't matter.** All must pass. People sometimes try to "fall through" — that's not how it works.
7. **No throttle without cache.** Local-memory cache works in dev but breaks under multi-process Gunicorn. Use Redis.
8. **`IsAuthenticated` on session-only auth = CSRF still required.** `SessionAuthentication` enforces CSRF for unsafe methods. JWT/Token auth bypass it. Make sure clients send the right thing.
9. **Browsable API in production.** It exposes DELETE buttons and writeable forms to anyone hitting your API. Disable it in prod by setting `DEFAULT_RENDERER_CLASSES = ["rest_framework.renderers.JSONRenderer"]`.
10. **Using DRF for HTML.** DRF can render HTML but it's clunky. For HTML, use Django views. DRF for JSON.
11. **`source="comments.count"` on a queryset.** Triggers a `COUNT` per row on list views (N+1). Use `annotate(comment_count=Count("comments"))` instead.

---

## 🎯 Key Takeaways

- **`ModelSerializer` + `ModelViewSet` + `DefaultRouter`** is the DRF productivity trio. Three components, full CRUD API with permissions and pagination.
- **Tune the queryset on every ViewSet.** `select_related` and `prefetch_related` are not optional in API land — N+1 will crush you when a client iterates pages.
- **Permission stacking is AND.** All `permission_classes` must pass. Use `has_permission` for collection-level, `has_object_permission` for instance-level.
- **CursorPagination for big lists.** PageNumber is fine for small datasets; cursor scales to millions of rows and infinite scroll.
- **Throttling + Redis cache from day one.** Anonymous endpoints get scraped. Cheap insurance: 100/day for anon, 1000/day for users, custom scope for login.

*← [prev](./10_middleware_and_signals.md) | [next →](./12_caching_and_sessions.md)*
