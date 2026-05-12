# 10 — Authentication and Authorization

> **Goal:** Implement JWT-based auth, understand OAuth2 flows, build session-cookie auth, and wire RBAC patterns — with the security details that audits actually check.

---

## 1. Auth in Gin — mental model + working code

Two concerns, often conflated:

- **Authentication (AuthN)** — "who is this user?" (identity).
- **Authorization (AuthZ)** — "what can they do?" (permissions).

In Gin both happen in middleware: AuthN parses a credential (JWT, session cookie, API key) and stashes the user (`c.Set("user", u)`); AuthZ inspects the stashed user and decides allow/deny.

Three common credential mechanisms:

| Mechanism | Best for | Trade-off |
|-----------|----------|-----------|
| **JWT bearer token** | APIs, mobile, SPAs | Stateless, but rotating/revoking is painful |
| **Server-side session cookie** | Web apps with browsers | Easy to revoke, harder to scale across regions |
| **API key** | Service-to-service | Simple, but long-lived; rotate carefully |

Most production services I see use **session cookies for the browser app + JWTs for mobile/API**, with API keys for internal callers.

### Minimal JWT middleware (using golang-jwt)

```bash
go get github.com/golang-jwt/jwt/v5
```

```go
// internal/http/middleware/jwt.go
package middleware

import (
    "errors"
    "net/http"
    "strings"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/golang-jwt/jwt/v5"
)

type Claims struct {
    UserID int64    `json:"uid"`
    Role   string   `json:"role"`
    jwt.RegisteredClaims
}

func IssueToken(secret []byte, userID int64, role string, ttl time.Duration) (string, error) {
    claims := Claims{
        UserID: userID,
        Role:   role,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
            Issuer:    "hello-gin",
            Subject:   "auth",
        },
    }
    t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return t.SignedString(secret)
}

func RequireJWT(secret []byte) gin.HandlerFunc {
    return func(c *gin.Context) {
        h := c.GetHeader("Authorization")
        if !strings.HasPrefix(h, "Bearer ") {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
            return
        }
        raw := strings.TrimPrefix(h, "Bearer ")

        claims := &Claims{}
        tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
            if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
                return nil, errors.New("unexpected signing method")
            }
            return secret, nil
        })
        if err != nil || !tok.Valid {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
            return
        }
        c.Set("user_id", claims.UserID)
        c.Set("role", claims.Role)
        c.Next()
    }
}
```

Use it:

```go
api := r.Group("/api/v1", middleware.RequireJWT(secret))
api.GET("/me", func(c *gin.Context) {
    c.JSON(200, gin.H{"user_id": c.GetInt64("user_id"), "role": c.GetString("role")})
})
```

```bash
TOKEN=$(curl -s -X POST -d '{"email":"y@x.com","password":"correct"}' \
  -H 'Content-Type: application/json' http://localhost:8080/login | jq -r .token)

curl -i -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/me
```

---

## 2. How JWT validation works under the hood

A JWT has three base64url-encoded parts joined by dots: `header.payload.signature`. The signature is HMAC-SHA256 (or RSA/ECDSA) over `header.payload`.

```
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1aWQiOjEsImV4cCI6MTcwMDAwMDAwMH0.<sig>
```

`ParseWithClaims` decodes header + payload, looks up the algorithm, calls your keyfunc for the verification key, recomputes the signature, and verifies. Then it checks `exp`, `nbf`, `iat` against the current time.

Two critical security points:

- **Pin the algorithm.** A famous CVE was "alg: none" — a token claiming no signature would validate. Always verify `t.Method.Alg() == expected` inside your keyfunc. The example above does this for HS256.
- **Don't use symmetric (HS*) keys for tokens issued by one service and validated by another** unless they truly share a secret. Use RS256/ES256 when the validator is separate from the issuer (e.g., third parties verifying your tokens against a JWKS).

### JWT trade-offs to know

- **Stateless ✓ / Revocation ✗.** A leaked JWT is valid until expiry. Options: short TTLs (5–15 min) + refresh tokens; or a server-side allow/deny list (which gives up statelessness).
- **Don't put secrets in the payload.** The payload is base64-encoded, not encrypted. Anyone with the token can read it.
- **Token size.** JWTs in headers add bytes to every request. Keep claims minimal.

### Refresh tokens

Pattern: short-lived access token (5–15 min) + long-lived refresh token (days/weeks) stored server-side. Client exchanges refresh for a new access token. If a refresh token is stolen, you invalidate it server-side. Don't put the refresh token in `localStorage` — store it in an `HttpOnly Secure SameSite=Strict` cookie.

---

## 3. Sessions, OAuth2, and RBAC

### Session cookies with `gin-contrib/sessions`

```bash
go get github.com/gin-contrib/sessions
go get github.com/gin-contrib/sessions/cookie
```

```go
import (
    "github.com/gin-contrib/sessions"
    "github.com/gin-contrib/sessions/cookie"
)

store := cookie.NewStore([]byte(os.Getenv("SESSION_KEY")))
store.Options(sessions.Options{
    Path:     "/",
    MaxAge:   3600 * 24 * 7, // 1 week
    HttpOnly: true,
    Secure:   true,
    SameSite: http.SameSiteLaxMode,
})
r.Use(sessions.Sessions("session", store))

r.POST("/login", func(c *gin.Context) {
    // ...verify password...
    s := sessions.Default(c)
    s.Set("user_id", userID)
    _ = s.Save()
    c.JSON(200, gin.H{"ok": true})
})

r.GET("/me", RequireSession, meHandler)

func RequireSession(c *gin.Context) {
    s := sessions.Default(c)
    uid := s.Get("user_id")
    if uid == nil {
        c.AbortWithStatusJSON(401, gin.H{"error": "not signed in"})
        return
    }
    c.Set("user_id", uid.(int64))
    c.Next()
}
```

The cookie store keeps the entire session encrypted in the cookie. For server-side state (revocation, server-issued IDs), swap in `redis` or `postgres` stores from `gin-contrib/sessions`. Don't roll your own session store.

### OAuth2 — terminology and flows

If your app needs "Sign in with Google/GitHub/Microsoft," you're an OAuth2 *client*. The flows:

- **Authorization Code** — for web servers. Redirect user to the provider, get an auth code back, exchange it for a token server-side. Use this.
- **Authorization Code + PKCE** — for mobile/SPAs (any "public" client). Adds a code verifier so a stolen auth code can't be exchanged. Use this for SPAs.
- **Implicit** — deprecated. Don't use.
- **Client Credentials** — for service-to-service (no user). Provider issues a token directly to a client ID + secret.
- **Refresh Token** — the grant type for exchanging a refresh token for a new access token.

Use the `golang.org/x/oauth2` package:

```go
import "golang.org/x/oauth2"
import "golang.org/x/oauth2/github"

conf := &oauth2.Config{
    ClientID:     os.Getenv("GH_CLIENT_ID"),
    ClientSecret: os.Getenv("GH_CLIENT_SECRET"),
    RedirectURL:  "https://example.com/auth/github/callback",
    Scopes:       []string{"read:user", "user:email"},
    Endpoint:     github.Endpoint,
}

r.GET("/auth/github/login", func(c *gin.Context) {
    state := randomString()           // store in session
    sessions.Default(c).Set("oauth_state", state)
    _ = sessions.Default(c).Save()
    c.Redirect(http.StatusFound, conf.AuthCodeURL(state))
})

r.GET("/auth/github/callback", func(c *gin.Context) {
    if c.Query("state") != sessions.Default(c).Get("oauth_state") {
        c.AbortWithStatusJSON(400, gin.H{"error": "bad state"})
        return
    }
    tok, err := conf.Exchange(c.Request.Context(), c.Query("code"))
    if err != nil {
        c.AbortWithStatusJSON(400, gin.H{"error": err.Error()})
        return
    }
    // tok.AccessToken — use to call GitHub API for user info
    client := conf.Client(c.Request.Context(), tok)
    resp, _ := client.Get("https://api.github.com/user")
    _ = resp
    // ...create or look up the user in your DB, set session, redirect
})
```

Key security points: **always validate `state`** to defeat CSRF on the callback, and **always use PKCE for public clients**.

### RBAC — role-based access control

The simplest model: each user has a role; each route requires a role.

```go
func RequireRole(roles ...string) gin.HandlerFunc {
    allowed := make(map[string]struct{}, len(roles))
    for _, r := range roles {
        allowed[r] = struct{}{}
    }
    return func(c *gin.Context) {
        role := c.GetString("role")
        if _, ok := allowed[role]; !ok {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
            return
        }
        c.Next()
    }
}

admin := r.Group("/admin", middleware.RequireJWT(secret), RequireRole("admin"))
admin.DELETE("/users/:id", deleteUser)
```

When roles grow into 20+ permissions, switch to permissions-not-roles:

```go
// user has []Permission e.g. {"user:read", "user:write", "order:read"}
func RequirePerm(perm string) gin.HandlerFunc {
    return func(c *gin.Context) {
        perms, _ := c.Get("permissions")
        if !contains(perms.([]string), perm) {
            c.AbortWithStatusJSON(403, gin.H{"error": "forbidden"})
            return
        }
        c.Next()
    }
}
```

For object-level (row-level) authz — "user X can read order Y because they own it" — middleware is not the right tool. Push that check into the service or store layer, where it has access to the data.

### Password hashing

`bcrypt` is the default in Go:

```go
import "golang.org/x/crypto/bcrypt"

hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost) // cost=10
err = bcrypt.CompareHashAndPassword(hash, []byte(input))
```

`argon2id` is technically stronger but bcrypt is what every Go team uses. Never store plaintext, never store unsalted hashes, and never compare with `==`/`bytes.Equal` (timing attack — `bcrypt.CompareHashAndPassword` is constant-time).

---

## 4. Practical application — full login + JWT + RBAC

A complete slice. User signs up, logs in, gets a JWT, accesses a protected route, hits a 403 on an admin route.

```go
// internal/http/handlers/auth.go
package handlers

import (
    "context"
    "errors"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "golang.org/x/crypto/bcrypt"

    "github.com/you/hello-gin/internal/apperr"
    "github.com/you/hello-gin/internal/http/middleware"
    "github.com/you/hello-gin/internal/service"
)

type AuthHandler struct {
    Users     *service.UserService
    JWTSecret []byte
    JWTTTL    time.Duration
}

type signupReq struct {
    Email    string `json:"email"    binding:"required,email"`
    Password string `json:"password" binding:"required,min=8,max=72"`
    Name     string `json:"name"     binding:"required,min=1,max=100"`
}

type loginReq struct {
    Email    string `json:"email"    binding:"required,email"`
    Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Signup(c *gin.Context) error {
    var req signupReq
    if err := c.ShouldBindJSON(&req); err != nil {
        return apperr.BadRequest("invalid body", err)
    }
    hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
    if err != nil {
        return apperr.Internal(err)
    }
    u := &service.User{Email: req.Email, Name: req.Name, PasswordHash: hash, Role: "user"}
    if err := h.Users.Create(c.Request.Context(), u); err != nil {
        if errors.Is(err, service.ErrEmailTaken) {
            return apperr.Conflict("email taken", err)
        }
        return apperr.Internal(err)
    }
    return h.issue(c, u)
}

func (h *AuthHandler) Login(c *gin.Context) error {
    var req loginReq
    if err := c.ShouldBindJSON(&req); err != nil {
        return apperr.BadRequest("invalid body", err)
    }
    u, err := h.Users.GetByEmail(c.Request.Context(), req.Email)
    if err != nil {
        // Same error message for missing user and bad password — don't leak which.
        return apperr.Unauthorized("invalid credentials")
    }
    if err := bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(req.Password)); err != nil {
        return apperr.Unauthorized("invalid credentials")
    }
    return h.issue(c, u)
}

func (h *AuthHandler) issue(c *gin.Context, u *service.User) error {
    _ = context.Background()
    tok, err := middleware.IssueToken(h.JWTSecret, u.ID, u.Role, h.JWTTTL)
    if err != nil {
        return apperr.Internal(err)
    }
    c.JSON(http.StatusOK, gin.H{
        "token":   tok,
        "expires": time.Now().Add(h.JWTTTL).Unix(),
        "user":    u,
    })
    return nil
}
```

Wire it:

```go
// cmd/api/main.go
r := gin.Default()
authH := &handlers.AuthHandler{Users: svc, JWTSecret: secret, JWTTTL: 15 * time.Minute}

r.POST("/signup", ginx.Wrap(log, authH.Signup))
r.POST("/login",  ginx.Wrap(log, authH.Login))

api := r.Group("/api/v1", middleware.RequireJWT(secret))
api.GET("/me", ginx.Wrap(log, userH.Me))

admin := api.Group("/admin", middleware.RequireRole("admin"))
admin.DELETE("/users/:id", ginx.Wrap(log, userH.Delete))
```

```bash
# Sign up
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"email":"y@x.com","password":"correcthorse","name":"yati"}' \
  http://localhost:8080/signup
# {"token":"eyJ...","expires":...}

TOKEN="eyJ..."

# Authenticated route
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/me
# 200

# Admin route — 403 (user role)
curl -i -X DELETE -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/admin/users/2
# 403
```

---

## 5. Common mistakes & gotchas

- **`alg: none` JWT vulnerability.** Always pin the signing algorithm in the keyfunc. Don't trust `t.Method`; whitelist it.
- **Leaking which credential was wrong.** "Email not found" vs "wrong password" lets an attacker enumerate users. Return the same generic message for both.
- **Storing JWTs in `localStorage`.** XSS → token theft. For browser clients use `HttpOnly Secure SameSite=Strict` cookies for the refresh token, and access-token-in-memory.
- **Forgetting `Secure` + `HttpOnly` + `SameSite` on session cookies.** All three are non-negotiable in production. `Secure=true` requires HTTPS; use it.
- **Comparing password hashes with `==`.** Timing attack. Use `bcrypt.CompareHashAndPassword` (constant time). Same rule: any cryptographic comparison goes through `subtle.ConstantTimeCompare`.
- **bcrypt cost too low.** Cost 4 is fine for tests. Default 10 is the production floor. Cost 12+ if your hardware can handle it without slowing login below ~250ms.
- **No CSRF protection on cookie-based mutating endpoints.** Sessions in cookies + state-changing POSTs without a CSRF token = classic vulnerability. Use `gin-contrib/csrf` or set `SameSite=Strict`.
- **No state validation in OAuth callback.** CSRF on the callback URL. Always issue a random `state`, store in the session, verify on return.
- **Role check on the wrong layer.** Middleware can check "is this user an admin"; it cannot check "does this user own order #42." Object-level permission checks live in the service layer, near the data.
- **Long-lived JWTs without revocation.** A 24-hour JWT can't be revoked if the device is lost. Shorten TTL (15 min), use a refresh token, allow server-side refresh-token revocation.

---

## 🎯 Key Takeaways

1. **AuthN sets identity; AuthZ checks permissions.** Both are middleware, but only AuthZ that is purely role-based belongs in middleware. Object-level "can this user act on *that* resource" lives in the service layer.
2. **Pin JWT algorithms inside your keyfunc.** "alg: none" and HS-as-RS confusion attacks are real CVEs. The two-line check (`if t.Method.Alg() != expected { return nil, err }`) is mandatory.
3. **Use `HttpOnly Secure SameSite` cookies for browsers, JWTs for APIs/mobile.** Don't put JWTs in `localStorage`. Don't reuse the same token type across both surfaces.
4. **Use `golang.org/x/oauth2` for OAuth2 client flows** — Authorization Code (+ PKCE for public clients), validated `state`, exchange on the server. Don't roll your own OAuth.
5. **bcrypt at cost 10+, constant-time comparison, identical error messages for "user not found" and "wrong password."** These three details are the security-review difference between "looks fine" and "rejected."

*← [09 — Database Integration](./09_database_integration.md) | [11 — Testing →](./11_testing.md)*
