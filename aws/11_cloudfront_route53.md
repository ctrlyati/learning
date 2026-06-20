# 11 — CloudFront & Route 53: Edge Caching and DNS

> **Goal:** Put a global CDN in front of every web property, host DNS with intelligent routing, and understand the edge-compute hooks that can save latency and money.

---

## 1. CloudFront — AWS's CDN

**Mental model:** Your origin (S3 bucket, ALB, API Gateway, custom HTTP) lives in one region. CloudFront has ~600 edge locations + ~13 regional edge caches worldwide. Users hit the nearest edge; CloudFront serves from cache or fetches from origin. Reduces origin load, latency, and egress (egress to CloudFront is free).

### Distribution components
- **Origin**: where the content lives.
- **Behaviors**: path patterns → which origin, what cache policy, what allowed methods.
- **Cache policy**: TTLs, headers/cookies/query strings to include in the cache key.
- **Origin request policy**: which headers/cookies/qs to forward to origin.
- **Response headers policy**: HSTS, CSP, CORS — add at the edge.

### Create a basic distribution (S3 + OAC)
```typescript
const bucket = new s3.Bucket(this, "Site", {
  blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
  encryption: s3.BucketEncryption.S3_MANAGED,
});

const distribution = new cloudfront.Distribution(this, "Cdn", {
  defaultBehavior: {
    origin: origins.S3BucketOrigin.withOriginAccessControl(bucket),
    viewerProtocolPolicy: cloudfront.ViewerProtocolPolicy.REDIRECT_TO_HTTPS,
    cachePolicy: cloudfront.CachePolicy.CACHING_OPTIMIZED,
    responseHeadersPolicy: cloudfront.ResponseHeadersPolicy.SECURITY_HEADERS,
  },
  defaultRootObject: "index.html",
  domainNames: ["www.example.com"],
  certificate: cert,    // ACM cert in us-east-1!
  priceClass: cloudfront.PriceClass.PRICE_CLASS_100,
});
```

### Origin Access Control (OAC)
Replaces the legacy OAI (Origin Access Identity). Lets CloudFront fetch from a private S3 bucket using SigV4 — the bucket stays Block-All-Public. **Always use OAC for S3 origins.**

### Cache policies — the cache key matters
CloudFront's cache key by default = URL + method. If two users get different responses based on a header (e.g., `Authorization`), you must include that header in the cache key or you'll serve A's data to B (catastrophic).

Use AWS-managed policies as starting points:
- `CACHING_OPTIMIZED` — long TTL, ignores all headers/cookies/qs (great for static).
- `CACHING_DISABLED` — never cache (APIs).
- `Managed-CachingOptimizedForUncompressedObjects` — etc.

### Invalidations
When you deploy new content with the same URL, invalidate to force CloudFront to refresh:
```bash
aws cloudfront create-invalidation --distribution-id E2XXXX --paths "/*"
```
**First 1000 invalidation paths/month free; $0.005 per path after.** Wildcards (`/*`) count as one path. Don't invalidate constantly — use versioned filenames (`app.a3f7d.js`) instead.

### Signed URLs / Signed Cookies
Restrict CloudFront content access (e.g., video paywall, time-limited downloads). Sign a URL with a CloudFront key; only holders with the signed URL can fetch.

### Compression
Enable `Compress: true` on behaviors — CloudFront auto-gzips/brotlis text content. Free win.

### Pricing
- **Data out**: regional rates, ~$0.085/GB in NA/EU, more elsewhere. **Free** from S3/EC2 to CloudFront in the same region (huge benefit vs. direct S3 egress at $0.09/GB).
- **Requests**: ~$0.0075-0.01 per 10000 HTTPS requests.
- **Price classes**: limit which edge locations are used (100 = NA/EU only, 200 = + South America/Asia, All = global). Cheaper price classes = higher latency in excluded regions.

---

## 2. Lambda@Edge and CloudFront Functions

Edge compute for request/response manipulation.

### CloudFront Functions
- JavaScript only, runs at the edge (all locations).
- Sub-millisecond cold start.
- Tight limits: 10 KB code, 2 MB memory, 1 ms CPU.
- Triggers: viewer request, viewer response.
- Use cases: URL rewrites, header rewrites, A/B routing, simple auth checks.
- **Very cheap**: $0.10 per million invocations.

```javascript
function handler(event) {
  const req = event.request;
  // Rewrite /old/* to /new/*
  if (req.uri.startsWith("/old/")) req.uri = req.uri.replace("/old/", "/new/");
  // Add a header
  req.headers["x-edge"] = { value: "1" };
  return req;
}
```

### Lambda@Edge
- Full Lambda (Node/Python), runs at regional edge caches.
- Up to 5 sec viewer-side, 30 sec origin-side.
- Triggers: viewer request, viewer response, origin request, origin response.
- Use cases: dynamic origin selection, full auth, image processing, A/B testing.
- More expensive than CloudFront Functions; deployed only from `us-east-1`.

Rule of thumb: **CloudFront Functions if it fits; Lambda@Edge if you need more.**

---

## 3. Route 53 — DNS, plus routing intelligence

**Mental model:** A managed authoritative DNS service. You delegate your domain to it, define records, and Route 53 answers DNS queries globally with high availability. Beyond plain DNS, it offers traffic management (latency, geo, failover) and health checks.

### Hosted zones
A zone hosts records for a domain.

```bash
ZONE=$(aws route53 create-hosted-zone --name example.com \
  --caller-reference $(date +%s) --query HostedZone.Id --output text)
# Update your registrar's nameservers to the four AWS NS records shown
```

### Record types
- A / AAAA: IPv4 / IPv6.
- CNAME: aliases (NOT allowed at apex `example.com`).
- ALIAS: AWS-specific extension — like CNAME but works at apex and integrates with health checks. Free of charge (regular DNS queries are charged).
- MX, TXT, NS, SOA, SRV, CAA.

### ALIAS records — use these for AWS targets
```bash
aws route53 change-resource-record-sets --hosted-zone-id $ZONE --change-batch '{
  "Changes": [{
    "Action": "UPSERT",
    "ResourceRecordSet": {
      "Name": "www.example.com",
      "Type": "A",
      "AliasTarget": {
        "DNSName": "d111111abcdef8.cloudfront.net",
        "HostedZoneId": "Z2FDTNDATAQYW2",
        "EvaluateTargetHealth": false
      }
    }
  }]
}'
```

`HostedZoneId` for CloudFront is always `Z2FDTNDATAQYW2`. ALB / API Gateway / S3 website endpoints have their own.

---

## 4. Routing Policies — what makes Route 53 special

Each record can have a routing policy. Multiple records with the same name+type can coexist with different policies.

| Policy | Behavior |
|---|---|
| **Simple** | One target; classic DNS. |
| **Weighted** | Split traffic by weight (e.g., 90/10 for canary). |
| **Latency-based** | Send users to the region with lowest measured latency. |
| **Geolocation** | Route by continent/country/state. |
| **Geoproximity** | Like geo but with adjustable bias toward regions (requires traffic flow). |
| **Failover** | Primary + secondary; secondary returned when primary's health check fails. |
| **Multivalue answer** | Return up to 8 random healthy targets — poor-man's load balancer. |
| **IP-based** | Route by client IP range. |

### Health checks
Probe an endpoint (HTTP/HTTPS/TCP) from multiple regions. Use to drive failover, multivalue, or alarms.

```bash
aws route53 create-health-check --caller-reference $(date +%s) --health-check-config '{
  "Type": "HTTPS",
  "FullyQualifiedDomainName": "www.example.com",
  "ResourcePath": "/healthz",
  "Port": 443,
  "RequestInterval": 30,
  "FailureThreshold": 3
}'
```

### Practical: multi-region active-active with failover
```
www.example.com (latency-based, with health checks)
  ├── A: ap-southeast-1 ALB (health: /healthz)
  ├── A: eu-west-1 ALB (health: /healthz)
  └── A: us-east-1 ALB (health: /healthz)
```

If `ap-southeast-1` is unhealthy, latency-based won't return it; traffic shifts to next-fastest healthy region. No app changes.

---

## 5. Route 53 Resolver, Private Hosted Zones, DNSSEC

### Private hosted zones
DNS that resolves only inside specific VPCs. Use for internal service discovery (`db.internal.example.com`).

### Route 53 Resolver
The default DNS resolver for VPCs. Resolver endpoints let you:
- **Inbound endpoint**: on-prem DNS resolves AWS records.
- **Outbound endpoint**: AWS workloads resolve on-prem records via forwarding rules.

### DNSSEC
Route 53 supports DNSSEC signing (since 2020) and validation. Important for security-sensitive domains.

---

## 6. Practical: a global static site with API

```
                                     ┌─── /api/* ──► API Gateway (regional, eu-west-1)
                Route 53                                           ▲
   www.example.com (ALIAS)                                          │
                  ▼                                                 │
            CloudFront ──────► /* ──► S3 (eu-west-1, OAC)          │
                  │                                                 │
                  └─── ACM cert (us-east-1)                          │
                  └─── WAF (us-east-1, CloudFront scope)             │
                  └─── /api/* behavior with CACHING_DISABLED, forwards Authorization header
```

CloudFront in front of API Gateway gives you: WAF integration, edge TLS termination, optional caching for GETs, custom error pages.

---

## 7. Common Mistakes & Gotchas

- **ACM cert in the wrong region.** CloudFront *requires* certs in `us-east-1` regardless of where your origin lives. ALB requires same-region.
- **CNAME at apex.** Not allowed by DNS spec. Use ALIAS for AWS targets.
- **Forgetting to forward `Authorization` header.** API behind CloudFront receives no auth — 401s. Use an origin request policy that forwards auth.
- **Auth header in cache key by default.** With managed `CACHING_DISABLED` it's fine, but a custom policy that includes the header creates a per-user cache (giant, wasteful).
- **Long TTL on dynamic content.** Stale data served for hours. Set short TTLs or `CACHING_DISABLED`.
- **Invalidating on every deploy.** Wasteful + slow. Use versioned filenames + `Cache-Control: max-age=31536000, immutable`. Invalidate only `index.html`.
- **`PriceClass_100` set for a global audience.** Asian/Australian users get worse latency. Use `PriceClass_All` if budget allows.
- **OAI instead of OAC.** OAI is legacy; OAC supports SSE-KMS and is recommended.
- **Health check checks `/` of an SPA** that returns 200 always — fails to detect backend death. Build a real `/healthz` that checks downstream.
- **Failover routing without health checks.** Failover never happens; primary always returned.
- **Latency routing in front of single-region target.** Pointless overhead.
- **Private hosted zone shared without VPC association.** Resolution fails confusingly. Associate every VPC that needs it.
- **DNS TTL of 0** thinking it bypasses cache. Doesn't; many resolvers ignore <30s and use their own min.
- **Domain transfers** to Route 53 take days. Plan ahead; you can host the zone before transferring registration.
- **CloudFront logging not enabled.** When a customer says "your site is broken", you have no idea. Enable real-time logs to Firehose → S3.
- **WAF scope mismatch.** WAF for CloudFront must be in `us-east-1`; regional services have regional WAF.

---

## 🎯 Key Takeaways

- **CloudFront in front of *everything* user-facing** — static sites, APIs, dynamic apps. The egress savings + edge TLS + WAF integration are worth the small setup cost.
- **Use OAC + S3 + Block Public Access** to serve static content. Public S3 buckets for websites are a 2015-era pattern.
- **Versioned asset filenames + `immutable` Cache-Control + invalidate only `index.html`** is the canonical static-site deploy pattern.
- **Route 53 ALIAS records are free and apex-capable** — always prefer them over CNAMEs for AWS resources.
- **Route 53 routing policies + health checks** turn DNS into a global traffic director. Latency-based + multi-region origins + failover is the simplest "global active-active" most teams need.

*← [prev](./10_sqs_sns_eventbridge.md) | [next →](./12_ecs_ecr_fargate.md)*
