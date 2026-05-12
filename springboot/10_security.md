# 10 — Spring Security

> **Goal:** Lock down a REST API with Spring Security — understand the filter chain, do password hashing right, issue/verify JWTs, and stand up an OAuth2 resource server.

---

## 1. Spring Security — mental model + working code

Adding `spring-boot-starter-security` to a project secures **every** endpoint by default:

- All HTTP requests require authentication.
- A user `user` is generated with a random password (logged at startup).
- A basic-auth login is wired automatically.

That default is a deliberate fail-safe. Your job is to configure it to match reality.

### Add the starter

```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-security</artifactId>
</dependency>
```

Boot prints something like:

```
Using generated security password: 3a1c1ef2-...-...
```

Hit any endpoint → 401 unless you send `Authorization: Basic dXNlcjozYTFj...`.

### Minimal custom config — open everything

```java
package com.example.bookstore.security;

import org.springframework.context.annotation.*;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.config.annotation.web.configuration.EnableWebSecurity;
import org.springframework.security.web.SecurityFilterChain;

@Configuration
@EnableWebSecurity
public class SecurityConfig {

    @Bean
    public SecurityFilterChain filterChain(HttpSecurity http) throws Exception {
        http
            .authorizeHttpRequests(auth -> auth.anyRequest().permitAll())
            .csrf(csrf -> csrf.disable());     // OK for stateless JSON APIs
        return http.build();
    }
}
```

That's the simplest "secure everything" → "open everything" override. We layer rules back in next.

---

## 2. The filter chain — what Spring Security actually does

Spring Security inserts a **chain of `Filter`s** into the servlet pipeline. Every HTTP request walks through:

```
Request
  ↓
[SecurityContextPersistenceFilter]   loads SecurityContext if any
  ↓
[CsrfFilter]                          CSRF token check (not on stateless APIs)
  ↓
[LogoutFilter]                        intercepts /logout
  ↓
[UsernamePasswordAuthenticationFilter] processes /login form
  ↓
[BearerTokenAuthenticationFilter]     processes Authorization: Bearer ...
  ↓
[BasicAuthenticationFilter]           processes Authorization: Basic ...
  ↓
[ExceptionTranslationFilter]          turns AccessDeniedException → 403, AuthenticationException → 401
  ↓
[FilterSecurityInterceptor / AuthorizationFilter]  enforces access rules
  ↓
[DispatcherServlet]                   if authorized, hand off to your controller
```

Each `SecurityFilterChain` bean configures one chain. You can have multiple, matched by path:

```java
@Bean
@Order(1)
public SecurityFilterChain apiChain(HttpSecurity http) throws Exception {
    http.securityMatcher("/api/**")
        .authorizeHttpRequests(a -> a.anyRequest().hasRole("USER"))
        .oauth2ResourceServer(o -> o.jwt(j -> {}));
    return http.build();
}

@Bean
@Order(2)
public SecurityFilterChain actuatorChain(HttpSecurity http) throws Exception {
    http.securityMatcher("/actuator/**")
        .authorizeHttpRequests(a -> a
            .requestMatchers("/actuator/health").permitAll()
            .anyRequest().hasRole("ADMIN"))
        .httpBasic(b -> {});
    return http.build();
}
```

---

## 3. Authentication mechanisms — depth

### Password encoding

**Never store plaintext passwords. Never use MD5 or SHA without a salt. Use BCrypt (or Argon2).**

```java
@Bean
public PasswordEncoder passwordEncoder() {
    return new BCryptPasswordEncoder();
}
```

Storing a user:

```java
String hash = encoder.encode("plaintextPassword");
// hash = "$2a$10$..." — store this in the DB
```

Verifying:

```java
boolean matches = encoder.matches("plaintextPassword", storedHash);
```

### `UserDetailsService` — Spring's user lookup contract

```java
@Service
public class JpaUserDetailsService implements UserDetailsService {

    private final UserRepository repo;

    public JpaUserDetailsService(UserRepository repo) { this.repo = repo; }

    @Override
    public UserDetails loadUserByUsername(String username) {
        var user = repo.findByUsername(username)
            .orElseThrow(() -> new UsernameNotFoundException(username));

        return User.withUsername(user.getUsername())
            .password(user.getPasswordHash())
            .authorities(user.getRoles().stream()
                .map(r -> "ROLE_" + r).toArray(String[]::new))
            .build();
    }
}
```

### JWT — stateless authentication

JWT (JSON Web Token) is a signed, base64-encoded token the client sends in `Authorization: Bearer <jwt>`. The server verifies the signature and trusts the claims (subject, roles, expiry).

Add the resource-server starter:
```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-oauth2-resource-server</artifactId>
</dependency>
```

Configure:
```yaml
spring:
  security:
    oauth2:
      resourceserver:
        jwt:
          issuer-uri: https://auth.example.com/realms/bookstore
```

Spring will fetch the JWKS, validate tokens, and populate `Authentication` with claims. No code needed beyond enabling the resource server in the chain:

```java
http.oauth2ResourceServer(o -> o.jwt(Customizer.withDefaults()));
```

### Issuing your own JWTs (self-contained auth)

```java
@Service
public class JwtIssuer {

    private final JwtEncoder encoder;

    public JwtIssuer(JwtEncoder encoder) { this.encoder = encoder; }

    public String issue(String username, List<String> roles) {
        Instant now = Instant.now();
        JwtClaimsSet claims = JwtClaimsSet.builder()
            .issuer("bookstore-api")
            .issuedAt(now)
            .expiresAt(now.plus(1, ChronoUnit.HOURS))
            .subject(username)
            .claim("roles", roles)
            .build();
        return encoder.encode(JwtEncoderParameters.from(claims)).getTokenValue();
    }
}

@Bean
JwtDecoder jwtDecoder(@Value("${security.jwt.secret}") String secret) {
    SecretKeySpec key = new SecretKeySpec(secret.getBytes(), "HmacSHA256");
    return NimbusJwtDecoder.withSecretKey(key).build();
}

@Bean
JwtEncoder jwtEncoder(@Value("${security.jwt.secret}") String secret) {
    SecretKeySpec key = new SecretKeySpec(secret.getBytes(), "HmacSHA256");
    JWKSource<SecurityContext> jwks = new ImmutableSecret<>(key);
    return new NimbusJwtEncoder(jwks);
}
```

---

## 4. Practical application — JWT-secured bookstore API

```java
package com.example.bookstore.security;

import org.springframework.context.annotation.*;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.config.annotation.web.configuration.EnableWebSecurity;
import org.springframework.security.config.annotation.method.configuration.EnableMethodSecurity;
import org.springframework.security.config.http.SessionCreationPolicy;
import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.security.oauth2.server.resource.authentication.JwtAuthenticationConverter;
import org.springframework.security.oauth2.server.resource.authentication.JwtGrantedAuthoritiesConverter;
import org.springframework.security.web.SecurityFilterChain;

@Configuration
@EnableWebSecurity
@EnableMethodSecurity              // enables @PreAuthorize on methods
public class SecurityConfig {

    @Bean
    public SecurityFilterChain api(HttpSecurity http) throws Exception {
        http
            .sessionManagement(s -> s.sessionCreationPolicy(SessionCreationPolicy.STATELESS))
            .csrf(csrf -> csrf.disable())
            .authorizeHttpRequests(auth -> auth
                .requestMatchers("/auth/login", "/actuator/health").permitAll()
                .requestMatchers("/api/v1/admin/**").hasRole("ADMIN")
                .requestMatchers("/api/v1/**").authenticated()
                .anyRequest().denyAll())
            .oauth2ResourceServer(o -> o.jwt(j -> j
                .jwtAuthenticationConverter(jwtAuthConverter())));
        return http.build();
    }

    @Bean
    public PasswordEncoder passwordEncoder() { return new BCryptPasswordEncoder(); }

    private JwtAuthenticationConverter jwtAuthConverter() {
        JwtGrantedAuthoritiesConverter granted = new JwtGrantedAuthoritiesConverter();
        granted.setAuthoritiesClaimName("roles");
        granted.setAuthorityPrefix("ROLE_");

        JwtAuthenticationConverter conv = new JwtAuthenticationConverter();
        conv.setJwtGrantedAuthoritiesConverter(granted);
        return conv;
    }
}
```

### The login endpoint

```java
@RestController
@RequestMapping("/auth")
public class AuthController {

    private final AuthenticationManager authManager;
    private final JwtIssuer issuer;

    public AuthController(AuthenticationManager authManager, JwtIssuer issuer) {
        this.authManager = authManager;
        this.issuer = issuer;
    }

    @PostMapping("/login")
    public TokenResponse login(@RequestBody @Valid LoginRequest req) {
        Authentication auth = authManager.authenticate(
            new UsernamePasswordAuthenticationToken(req.username(), req.password()));
        List<String> roles = auth.getAuthorities().stream()
            .map(a -> a.getAuthority().replace("ROLE_", ""))
            .toList();
        return new TokenResponse(issuer.issue(auth.getName(), roles));
    }

    record LoginRequest(@NotBlank String username, @NotBlank String password) {}
    record TokenResponse(String token) {}
}
```

### Method-level security

```java
@Service
public class BookService {

    @PreAuthorize("hasRole('ADMIN')")
    public void delete(Long id) { ... }

    @PreAuthorize("hasRole('USER') and #userId == authentication.name")
    public Order placeOrder(String userId, Long bookId, int qty) { ... }
}
```

### Accessing the current user in a controller

```java
@GetMapping("/me")
public Map<String, Object> me(@AuthenticationPrincipal Jwt jwt) {
    return Map.of(
        "username", jwt.getSubject(),
        "roles", jwt.getClaimAsStringList("roles"));
}
```

---

## 5. Common Mistakes & Gotchas

- **Disabling CSRF without thinking.** OK on a stateless JSON API with token auth. **Not** OK on a server-rendered web app with cookie sessions. Know which you have.

- **Storing JWT secrets in source.** Treat secrets like passwords — env vars, secret manager, never in YAML.

- **Long-lived JWTs.** A leaked 30-day token is a 30-day problem. Use short-lived access tokens (5–15 min) + refresh tokens. Or revocation via a token blocklist.

- **Putting passwords in JWT claims.** Don't. The token is base64, not encrypted by default. Anyone with the token can read the claims.

- **Forgetting `sessionCreationPolicy(STATELESS)`.** Spring still creates an `HttpSession` per request by default. For pure JWT APIs, that's a memory leak.

- **Custom filter inserted at the wrong position.** Use `http.addFilterBefore(...)` / `addFilterAfter(...)` with a specific filter class as reference. Custom auth filters often need to run before `UsernamePasswordAuthenticationFilter`.

- **`hasRole("USER")` vs `hasAuthority("USER")`.** `hasRole` prepends `ROLE_`. So your authorities must be `ROLE_USER` for `hasRole("USER")` to match. Use one convention consistently.

- **CORS done in Security alone.** CORS and authentication interact in subtle ways. Configure CORS via `WebMvcConfigurer` (or a `CorsConfigurationSource` bean) **and** enable it in the security chain: `http.cors(Customizer.withDefaults())`.

- **Defaulting to `permitAll()` everywhere "for now".** That "for now" ships to production. Default deny is non-negotiable.

- **Logging request bodies that contain passwords.** A debug log line can leak credentials into a SIEM. Mask sensitive fields in your logging filter.

---

## 🎯 Key Takeaways

- **Spring Security defaults to deny.** Override deliberately, not lazily.
- **Filter chain is the model.** Visualize requests walking through it. Knowing the order makes debugging tractable.
- **BCrypt or Argon2 for password storage.** Plus `PasswordEncoder` always — never compare bytes yourself.
- **Stateless JWT is the default for modern APIs.** Use a real issuer (Keycloak, Auth0, AWS Cognito) for anything more than a toy.
- **`@PreAuthorize` on services**, not just `@RequestMapping` rules. Defense in depth.

*[← prev](./09_database_migrations.md) | [next →](./11_testing.md)*
