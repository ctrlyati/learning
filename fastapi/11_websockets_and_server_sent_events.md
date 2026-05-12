# 11 — WebSockets & Server-Sent Events

> **Goal:** Build real-time features in FastAPI — bidirectional WebSockets and one-way Server-Sent Events — and understand the scaling considerations that bite at the second-server boundary.

---

## 1. Concept — Two patterns, different shapes

| Pattern   | Direction         | Transport     | Use when                                              |
| --------- | ----------------- | ------------- | ----------------------------------------------------- |
| **WebSocket** | Bidirectional | One TCP / WS | Chat, multiplayer, live cursors, anything client-talks-too |
| **SSE**   | Server → Client   | HTTP stream   | Notifications, log tails, LLM token streaming         |
| Long-poll | Client polls      | HTTP          | Legacy fallback, rarely intentional choice today      |

WebSockets give you full duplex but require sticky connections, careful framing, and reconnect logic. SSE rides on plain HTTP, auto-reconnects in browsers, and is half the code — choose it whenever you only need server-to-client.

```python
from fastapi import FastAPI, WebSocket

app = FastAPI()


@app.websocket("/ws/echo")
async def echo(ws: WebSocket) -> None:
    await ws.accept()
    try:
        while True:
            msg = await ws.receive_text()
            await ws.send_text(f"echo: {msg}")
    except Exception:
        await ws.close()
```

That's a working bidirectional endpoint. Connect from a browser, send a string, get one back.

---

## 2. Mechanism — WebSocket lifecycle and Starlette's API

A FastAPI WebSocket lifecycle:

1. Client sends an HTTP `Upgrade: websocket` request.
2. Server (Uvicorn) accepts and hands you a `WebSocket` object.
3. You call `await ws.accept()` to complete the handshake (or `await ws.close(code)` to reject).
4. You loop on `await ws.receive_text() / receive_json() / receive_bytes()`.
5. You call `await ws.send_text() / send_json() / send_bytes()`.
6. Either side closes with `await ws.close(code)` or the connection drops.

Disconnects throw `WebSocketDisconnect`. Handle it.

```python
from fastapi import WebSocket, WebSocketDisconnect


@app.websocket("/ws/chat")
async def chat(ws: WebSocket) -> None:
    await ws.accept()
    try:
        while True:
            data = await ws.receive_json()
            # broadcast, persist, etc.
            await ws.send_json({"ok": True, "echo": data})
    except WebSocketDisconnect:
        # client went away
        pass
```

For dependencies in WebSocket endpoints, `Depends` works but with a twist: you can't raise `HTTPException` (no HTTP response to send). Raise `WebSocketException(code=1008, reason="...")` instead, or `await ws.close(1008)` manually.

---

## 3. Variations & Depth

### Connection management — broadcast to many clients

You need to track who's connected to deliver "user A sent a message → push to users B, C, D."

```python
from collections import defaultdict

from fastapi import WebSocket


class ConnectionManager:
    def __init__(self) -> None:
        self._rooms: dict[str, set[WebSocket]] = defaultdict(set)

    async def connect(self, room: str, ws: WebSocket) -> None:
        await ws.accept()
        self._rooms[room].add(ws)

    def disconnect(self, room: str, ws: WebSocket) -> None:
        self._rooms[room].discard(ws)

    async def broadcast(self, room: str, message: dict) -> None:
        for ws in list(self._rooms[room]):
            try:
                await ws.send_json(message)
            except Exception:
                self._rooms[room].discard(ws)


manager = ConnectionManager()


@app.websocket("/ws/rooms/{room}")
async def room_endpoint(ws: WebSocket, room: str) -> None:
    await manager.connect(room, ws)
    try:
        while True:
            msg = await ws.receive_json()
            await manager.broadcast(room, {"room": room, "msg": msg})
    except WebSocketDisconnect:
        manager.disconnect(room, ws)
```

This works on **one process**. The moment you scale to two pods, user A on pod-1 broadcasts to room "X" and user B on pod-2 receives nothing. The manager dict is per-process.

### Scaling — pub/sub backplane

For multi-process broadcast, push messages through Redis / NATS / Kafka:

```
client → pod-1 → publish to redis channel "room:X" → all pods subscribed deliver to their connected clients
```

Sketch:

```python
import asyncio
import json
import redis.asyncio as redis


@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.redis = redis.from_url("redis://localhost", decode_responses=True)
    yield
    await app.state.redis.aclose()


async def _redis_subscriber(app):
    pubsub = app.state.redis.pubsub()
    await pubsub.psubscribe("room:*")
    async for message in pubsub.listen():
        if message["type"] != "pmessage":
            continue
        room = message["channel"].split(":", 1)[1]
        await manager.broadcast(room, json.loads(message["data"]))


@app.on_event("startup")  # or in lifespan
async def start_sub():
    asyncio.create_task(_redis_subscriber(app))


@app.websocket("/ws/rooms/{room}")
async def room_endpoint(ws: WebSocket, room: str, request_app=...):
    await manager.connect(room, ws)
    try:
        while True:
            msg = await ws.receive_json()
            await app.state.redis.publish(f"room:{room}", json.dumps(msg))
    except WebSocketDisconnect:
        manager.disconnect(room, ws)
```

This is the rough shape every "real" WebSocket app converges on. The details (sharding, presence, ordering, backpressure) are full-time work; consider a managed service (Ably, Pusher, AWS API Gateway WebSocket) if it's not your core competency.

### Authentication for WebSockets

WS clients can't easily send custom headers from a browser. Common patterns:

- **Token as query param**: `ws://api/ws?token=...`. Verify with a dep:

```python
async def get_ws_user(token: str = Query(...)) -> dict:
    try:
        payload = decode_access_token(token)
    except Exception:
        raise WebSocketException(code=1008, reason="invalid token")
    return {"id": int(payload["sub"])}


@app.websocket("/ws")
async def ws_endpoint(ws: WebSocket, user: dict = Depends(get_ws_user)) -> None:
    await ws.accept()
    ...
```

- **First message is the token**: client connects, sends `{"auth": "..."}` as its first frame, server verifies before processing anything else.
- **Cookie-based session**: works only if browser is same-site.

### Server-Sent Events (SSE)

For one-way push, SSE is simpler and more robust:

```python
import asyncio
from fastapi.responses import StreamingResponse


async def event_stream():
    while True:
        # in real life: pop from a queue, await on an asyncio.Event, etc.
        yield f"data: {{\"time\": \"{datetime.utcnow().isoformat()}\"}}\n\n"
        await asyncio.sleep(1)


@app.get("/events/clock")
async def clock_stream() -> StreamingResponse:
    return StreamingResponse(event_stream(), media_type="text/event-stream")
```

Browser side:

```js
const es = new EventSource("/events/clock");
es.onmessage = (e) => console.log(JSON.parse(e.data));
```

`EventSource` auto-reconnects, deduplicates by `id:`, supports `event:` types. For LLM token streaming, this is what OpenAI and Anthropic APIs use under the hood.

### LLM streaming pattern

```python
@app.post("/chat")
async def chat(prompt: str) -> StreamingResponse:
    async def gen():
        async for token in llm.astream(prompt):
            yield f"data: {json.dumps({'token': token})}\n\n"
        yield "data: [DONE]\n\n"
    return StreamingResponse(gen(), media_type="text/event-stream")
```

The async generator yields chunks; FastAPI flushes each as it comes. Total response time = time-to-last-token; time-to-first-byte = time-to-first-token.

---

## 4. Practical Application — A scoped notification stream

Users connect to `/ws/notifications` and receive notifications targeted to them. Behind the scenes, other services push notifications via an internal HTTP endpoint, which broadcasts to the right user's WebSocket.

```python
# app/realtime/manager.py
import asyncio
from collections import defaultdict
from fastapi import WebSocket


class UserConnections:
    def __init__(self) -> None:
        self._conns: dict[int, set[WebSocket]] = defaultdict(set)
        self._lock = asyncio.Lock()

    async def add(self, user_id: int, ws: WebSocket) -> None:
        async with self._lock:
            self._conns[user_id].add(ws)

    async def remove(self, user_id: int, ws: WebSocket) -> None:
        async with self._lock:
            self._conns[user_id].discard(ws)

    async def push(self, user_id: int, payload: dict) -> int:
        targets = list(self._conns.get(user_id, ()))
        sent = 0
        for ws in targets:
            try:
                await ws.send_json(payload)
                sent += 1
            except Exception:
                await self.remove(user_id, ws)
        return sent


connections = UserConnections()
```

```python
# app/api/v1/realtime.py
from fastapi import APIRouter, Depends, Query, WebSocket, WebSocketDisconnect, WebSocketException

from app.realtime.manager import connections
from app.core.security import decode_access_token

router = APIRouter()


async def get_ws_user(token: str = Query(...)) -> int:
    try:
        payload = decode_access_token(token)
        return int(payload["sub"])
    except Exception:
        raise WebSocketException(code=1008)


@router.websocket("/ws/notifications")
async def notifications(ws: WebSocket, user_id: int = Depends(get_ws_user)) -> None:
    await ws.accept()
    await connections.add(user_id, ws)
    try:
        while True:
            # Optional: receive heartbeats/acks from the client
            await ws.receive_text()
    except WebSocketDisconnect:
        await connections.remove(user_id, ws)
```

```python
# app/api/v1/internal.py — only callable by internal services
from fastapi import APIRouter, HTTPException
from pydantic import BaseModel

from app.realtime.manager import connections

internal_router = APIRouter(prefix="/internal", tags=["internal"])


class Notification(BaseModel):
    user_id: int
    title: str
    body: str


@internal_router.post("/notify")
async def notify(n: Notification) -> dict:
    delivered = await connections.push(n.user_id, n.model_dump())
    return {"delivered_to": delivered}
```

**Smoke test from JS**

```js
const token = "Bearer eyJ...";
const ws = new WebSocket(`ws://localhost:8000/ws/notifications?token=${token.replace("Bearer ", "")}`);
ws.onmessage = (e) => console.log("notification:", JSON.parse(e.data));
ws.onopen = () => setInterval(() => ws.send("ping"), 30000);
```

And from another shell:

```bash
curl -X POST http://127.0.0.1:8000/internal/notify \
     -H "Content-Type: application/json" \
     -d '{"user_id": 1, "title": "Hi", "body": "Welcome"}'
# {"delivered_to": 1}
```

For multi-pod: replace `connections.push()` with a Redis publish, and have every pod subscribe and dispatch.

---

## 5. Common Mistakes & Gotchas

- **Forgetting `await ws.accept()`.** Connection silently fails the handshake.
- **No `WebSocketDisconnect` handling.** Endpoint loop crashes on disconnect → connection leak in your manager dict.
- **Storing connections in module-level dict** then scaling to 2 pods. Each pod sees half the users. Pub/sub fix or use a managed service.
- **Trying to use a sync DB driver in a WS endpoint.** Each frame await goes back to the event loop; blocking calls there hold up *all* other connections on that worker.
- **Sticky sessions assumed.** Without sticky load balancing (or a backplane), reconnects can land on a different pod that doesn't know about the user.
- **No heartbeat / ping-pong.** Idle connections get dropped by load balancers (often after 60s). Send a ping frame every 30s, or use Starlette's built-in `keepalive_timeout`.
- **Memory leaks via cancelled tasks holding refs.** Use `finally` to clean up, not just `except WebSocketDisconnect`.
- **Treating SSE like WebSockets** — SSE is server→client only. If your client wants to *send* anything, you need a normal POST (or use WebSockets).
- **Browsers cap parallel SSE connections** (~6 per origin in HTTP/1.1). Use HTTP/2 (handled by your reverse proxy) to remove the cap.
- **`StreamingResponse` with a sync generator** — runs in the threadpool, can starve other requests if the generator blocks. Use async generators for streaming.

---

## 🎯 Key Takeaways

- **Pick SSE when you can, WebSockets when you must.** SSE is one-way, auto-reconnecting, half the code.
- **`WebSocket` endpoints are coroutines that own a long-running connection.** Resource lifecycle (connect, disconnect, cleanup) is your responsibility.
- **A per-process connection manager works until your second pod.** Plan for a pub/sub backplane (Redis, NATS) from day one if you'll scale.
- **Auth via query token or first-frame handshake.** Headers aren't an option from browsers.
- **Heartbeats and reconnects are not optional.** Proxies kill idle connections; clients will retry — make sure your server can handle the churn.

*← [prev](./10_middleware_cors_and_exception_handlers.md) | [next →](./12_testing.md)*
