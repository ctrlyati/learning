# 13 — Async Django and Channels

> **Goal:** Know when async views earn their complexity, deploy under ASGI, and reach for Channels when you need websockets.

---

## 1. Why async at all

Django has been async-capable since 3.0 (ASGI) and async-views-capable since 3.1. Django 5 extends async to more of the ORM, signals, and middleware. The motivation:

- **High-fanout I/O.** A view that calls 3 external APIs serially takes 3× the latency. Run them concurrently with `asyncio.gather` and you cut that to 1×.
- **Long-lived connections.** Server-sent events, websockets, long polling — sync workers tie up a thread per connection, async workers can hold thousands.
- **Streaming responses.** LLM token streams, log tails, etc. — natural fit for async generators.

Async **does not** make CPU-bound code faster. It makes I/O-bound code more concurrent. If your view spends 100ms in Python doing math, async won't help.

---

## 2. WSGI vs ASGI

| | WSGI | ASGI |
|---|---|---|
| Protocol | Sync, request → response | Async, supports streams |
| Servers | gunicorn, uwsgi, mod_wsgi | uvicorn, daphne, hypercorn |
| Django entry | `wsgi.py` | `asgi.py` |
| Supports websockets? | No | Yes |
| Supports async views? | Wraps them in a thread (works but inefficient) | Native |

For Django 5, ASGI is the forward path. Deploy with `uvicorn` (FastAPI's server) or `daphne` (Channels' server):

```bash
pip install uvicorn[standard]
uvicorn mysite.asgi:application --workers 4
```

```python
# mysite/asgi.py (default generated)
import os
from django.core.asgi import get_asgi_application

os.environ.setdefault("DJANGO_SETTINGS_MODULE", "mysite.settings")
application = get_asgi_application()
```

Sync views still work under ASGI — Django wraps them in a threadpool. So you can migrate gradually: switch the server first, async-ify views as needed.

---

## 3. Async views

Just `async def`:

```python
# blog/views.py
import asyncio
import httpx
from django.http import JsonResponse

async def aggregate(request):
    async with httpx.AsyncClient() as client:
        results = await asyncio.gather(
            client.get("https://api.a.com/data"),
            client.get("https://api.b.com/data"),
            client.get("https://api.c.com/data"),
        )
    return JsonResponse({"results": [r.json() for r in results]})
```

That view is concurrent across the three external calls. Under WSGI it'd take ~3× the longest call's duration; under ASGI it takes ~1×.

Class-based:

```python
from django.views import View
from django.http import JsonResponse

class AggregateView(View):
    async def get(self, request):
        ...
        return JsonResponse(...)
```

Django dispatches to `aget`, `apost`, etc. when defined on a CBV.

---

## 4. Async ORM (Django 4.1+, expanded in 5)

The ORM is still synchronous at heart (most drivers are blocking). Django wraps queries in `sync_to_async` automatically when called from async code, *but provides async API variants*:

```python
# Reads
post = await Post.objects.aget(pk=1)
count = await Post.objects.acount()
exists = await Post.objects.filter(...).aexists()
first = await Post.objects.afirst()

# Iteration
async for post in Post.objects.filter(status="published"):
    print(post.title)

# Writes
post = await Post.objects.acreate(title="Hi", body="...", author=user)
await post.asave()
await post.adelete()

# Bulk
await Post.objects.abulk_create([Post(...), Post(...)])
```

Under the hood Django still uses a thread pool for the actual driver call. Real-async DB drivers (`psycopg3` async, `asyncpg`) work but the ORM doesn't yet thread them through end-to-end. For now: use the `a*` API for ergonomics, expect modest concurrency wins, watch for the thread-pool limit.

---

## 5. Mixing sync and async — `sync_to_async` / `async_to_sync`

Sometimes you need to call sync code from async (or vice versa):

```python
from asgiref.sync import sync_to_async, async_to_sync

# Call sync function in an async view
async def view(request):
    user = await sync_to_async(get_user_sync)(request)
    return JsonResponse({"name": user.name})

# Call async function from sync code
def script_entry():
    result = async_to_sync(fetch_external_api)()
```

Async views can call sync code — Django wraps it for you with `sync_to_async`. But each wrapped call has overhead and consumes a threadpool slot. If you wrap dozens of sync ORM calls in an async view, you've reinvented sync with extra steps.

**Rule of thumb:** if your view is 80% ORM, it might as well be sync. Async wins when you're doing real I/O concurrency (parallel HTTP, streaming, long-lived sockets).

---

## 6. Streaming responses

```python
from django.http import StreamingHttpResponse
import asyncio

async def slow_stream(request):
    async def gen():
        for i in range(10):
            yield f"chunk {i}\n"
            await asyncio.sleep(0.5)
    return StreamingHttpResponse(gen())
```

`StreamingHttpResponse` accepts a sync or async generator. The client sees bytes as they're yielded, not after the response completes. Useful for:

- Server-sent events (SSE) — `content_type="text/event-stream"`.
- LLM token streaming.
- Large CSV/JSON exports without buffering all rows in memory.

---

## 7. Channels — when you need websockets

Django itself can do request/response HTTP and SSE. For **websockets** (full duplex) and background **workers tied to your Django models**, install `channels`:

```bash
pip install channels channels-redis
```

```python
# settings.py
INSTALLED_APPS += ["channels", "chat"]
ASGI_APPLICATION = "mysite.asgi.application"
CHANNEL_LAYERS = {
    "default": {
        "BACKEND": "channels_redis.core.RedisChannelLayer",
        "CONFIG": {"hosts": [("127.0.0.1", 6379)]},
    },
}
```

```python
# mysite/asgi.py
import os
import django
from channels.routing import ProtocolTypeRouter, URLRouter
from channels.auth import AuthMiddlewareStack
from django.core.asgi import get_asgi_application

os.environ.setdefault("DJANGO_SETTINGS_MODULE", "mysite.settings")
django.setup()

from chat.routing import websocket_urlpatterns

application = ProtocolTypeRouter({
    "http": get_asgi_application(),
    "websocket": AuthMiddlewareStack(URLRouter(websocket_urlpatterns)),
})
```

---

## 8. A websocket consumer

```python
# chat/consumers.py
import json
from channels.generic.websocket import AsyncJsonWebsocketConsumer

class ChatConsumer(AsyncJsonWebsocketConsumer):
    async def connect(self):
        self.room = self.scope["url_route"]["kwargs"]["room"]
        self.group = f"chat_{self.room}"
        await self.channel_layer.group_add(self.group, self.channel_name)
        await self.accept()

    async def disconnect(self, code):
        await self.channel_layer.group_discard(self.group, self.channel_name)

    async def receive_json(self, content):
        # Broadcast to the group
        await self.channel_layer.group_send(self.group, {
            "type": "chat.message",
            "message": content["message"],
            "user": self.scope["user"].username,
        })

    # Handler invoked when the group sends a "chat.message" event
    async def chat_message(self, event):
        await self.send_json({"message": event["message"], "user": event["user"]})
```

```python
# chat/routing.py
from django.urls import re_path
from . import consumers

websocket_urlpatterns = [
    re_path(r"ws/chat/(?P<room>\w+)/$", consumers.ChatConsumer.as_asgi()),
]
```

Client:

```js
const ws = new WebSocket(`ws://${location.host}/ws/chat/general/`);
ws.onmessage = (e) => console.log(JSON.parse(e.data));
ws.send(JSON.stringify({message: "hi"}));
```

Deploy with `daphne` (Channels' reference server) or `uvicorn` plus a workers config:

```bash
daphne -b 0.0.0.0 -p 8000 mysite.asgi:application
```

---

## 9. Sending to a websocket from a view (server push)

The pattern: HTTP request comes in (sync view), it pushes a message to a Channels group, all subscribed websockets get it.

```python
# blog/views.py
from asgiref.sync import async_to_sync
from channels.layers import get_channel_layer

def publish_post(request, pk):
    post = Post.objects.get(pk=pk)
    post.status = "published"
    post.save()

    channel_layer = get_channel_layer()
    async_to_sync(channel_layer.group_send)("posts_feed", {
        "type": "post.published",
        "post_id": post.pk,
        "title": post.title,
    })
    return redirect("blog:detail", pk=pk)
```

```python
class FeedConsumer(AsyncJsonWebsocketConsumer):
    async def connect(self):
        await self.channel_layer.group_add("posts_feed", self.channel_name)
        await self.accept()
    async def post_published(self, event):
        await self.send_json({"new_post": event})
```

This is the classic "live updates" pattern — pageless reactivity without polling.

---

## 10. Practical application — SSE for live trending count

A simpler alternative to Channels when you don't need bidirectional.

```python
# blog/views.py
import asyncio
from django.http import StreamingHttpResponse
from .models import Post

async def trending_stream(request):
    async def event_stream():
        while True:
            count = await Post.objects.filter(status="published").acount()
            yield f"data: {count}\n\n"
            await asyncio.sleep(5)

    return StreamingHttpResponse(event_stream(), content_type="text/event-stream")
```

```html
<script>
  const es = new EventSource("/trending-stream/");
  es.onmessage = (e) => document.getElementById("count").textContent = e.data;
</script>
```

Run under `uvicorn` and you're done — one async view, one SSE-aware browser API, live updates.

---

## 11. Should you go async?

A senior decision framework:

**Stay sync if:**
- Your views are CRUD-shaped (ORM in, template out).
- You have <100 concurrent users.
- Most of your latency is DB-bound and you're already tuning queries.

**Go async if:**
- You make multiple parallel external HTTP/RPC calls in a view.
- You need websockets, SSE, or long-polling.
- You're streaming large responses (LLM tokens, large CSV).
- You're running into "running out of WSGI workers under load" with mostly-idle connections.

**Don't go async if:**
- You haven't profiled. Most "slow Django" problems are missing indexes and N+1s, not the runtime.

---

## 12. Common mistakes and gotchas

1. **Calling sync ORM in an async view without `sync_to_async`.** Django will raise `SynchronousOnlyOperation`. Use the `a*` methods or wrap in `sync_to_async`.
2. **Using `requests` library in async views.** `requests` is sync — it blocks the event loop. Use `httpx.AsyncClient` or `aiohttp`.
3. **Thinking async makes everything faster.** It makes I/O-concurrent things concurrent. CPU-bound code is just as slow.
4. **Mixing `async def` with sync middleware that doesn't declare async-capable.** Django wraps but emits warnings; performance suffers. Mark middleware `async_capable=True` or use `sync_and_async_middleware`.
5. **Forgetting `django.setup()` in `asgi.py` before importing apps.** Channels' `ProtocolTypeRouter` imports app routes — these must run *after* settings load.
6. **Channels without Redis.** Default in-memory channel layer is single-process — group messages don't reach other workers. Always use Redis (or another layer) in production.
7. **Using `async_to_sync` inside an event loop.** Causes a deadlock. From async code, just `await`.
8. **Holding DB transactions across `await`.** The transaction is per thread, but `await` may resume on a different thread. Mostly fine with `sync_to_async(thread_sensitive=True)`, but a known footgun.
9. **Streaming large querysets without `iterator()`.** Streams in Django are easy to make non-streaming if you accidentally `list(qs)` the entire result first.
10. **Forgetting to deploy under ASGI.** Async views under WSGI/gunicorn-sync workers run *one at a time* in a thread — you've added complexity for no concurrency.
11. **Channels consumer not closing connections.** Without `disconnect` cleanup, the consumer stays subscribed; ghost messages leak. Always `group_discard` in `disconnect`.
12. **Auth in websockets.** `AuthMiddlewareStack` reads the session cookie. Cross-origin websockets need `OriginValidator` and an appropriate `ALLOWED_HOSTS`.

---

## 🎯 Key Takeaways

- **Async Django needs ASGI to be useful.** Deploy with `uvicorn` or `daphne`; async views under WSGI workers gain you nothing.
- **Async ORM `a*` methods exist** but use a thread pool under the hood. The wins are in I/O concurrency (`asyncio.gather` over external calls), not in raw DB throughput.
- **`StreamingHttpResponse` + async generators** is the simplest path to SSE and token streaming — no Channels required.
- **Channels for websockets and groups.** `channels-redis` for production; `AuthMiddlewareStack` wraps the URL router for `scope["user"]`.
- **Most apps don't need async.** Profile first, fix N+1 and missing indexes, *then* consider whether async fixes the remaining latency. Async is not a substitute for a tuned ORM.

*← [prev](./12_caching_and_sessions.md) | [next →](./14_testing.md)*
