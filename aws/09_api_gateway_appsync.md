# 09 — API Gateway, AppSync, and Lambda Integrations

> **Goal:** Stand up APIs (REST, HTTP, WebSocket, GraphQL) in front of Lambda and other backends — with auth, throttling, custom domains, and the right cost/latency profile.

---

## 1. The API front-door choices

**Mental model:** You have application logic in Lambda/ECS/EC2. You need to expose it as HTTP/GraphQL to clients. AWS offers three front-doors:

| Service | Best for | Pricing | Complexity |
|---|---|---|---|
| **API Gateway HTTP API** | Modern REST APIs, Lambda backends | $1.00 / million requests | Low |
| **API Gateway REST API** | Mature REST APIs needing every feature (request validation, transformation, API keys, edge optimization) | $3.50 / million requests | High |
| **API Gateway WebSocket API** | Real-time bidirectional (chat, gaming) | $1.00 / million msgs + connection-mins | Medium |
| **AppSync** | GraphQL APIs, real-time subscriptions | $4.00 / million queries + cache | Medium |
| **ALB → Lambda** | Internal APIs, no auth needs | ALB hourly + per-LCU | Low |
| **Lambda Function URL** | Simplest possible, no infra | Free (just Lambda price) | Lowest |
| **CloudFront → Lambda@Edge / Functions** | Global, edge-compute | CDN $ + invocations | Medium |

### When to use what
- "Just give me HTTPS for my Lambda" → **Function URL** + CloudFront if you need a custom domain/WAF.
- "REST API for a SaaS" → **HTTP API** (cheaper, simpler). Move to REST API only if you need WAF integration before WAF for HTTP API existed (now it does), Cognito user pools as authorizer types, request/response transformation, or API keys.
- "Need WAF and custom domains" → either HTTP or REST API + CloudFront.
- "Real-time chat / live dashboard" → **AppSync subscriptions** or **WebSocket API**.
- "Complex data graph, multiple data sources" → **AppSync**.

---

## 2. API Gateway HTTP API — the modern default

```typescript
// CDK (v2 HTTP API)
const api = new apigwv2.HttpApi(this, "Api", {
  corsPreflight: {
    allowOrigins: ["https://app.example.com"],
    allowMethods: [apigwv2.CorsHttpMethod.ANY],
    allowHeaders: ["content-type", "authorization"],
  },
});

const handler = new lambda.Function(this, "Fn", { /*...*/ });
api.addRoutes({
  path: "/orders/{orderId}",
  methods: [apigwv2.HttpMethod.GET],
  integration: new HttpLambdaIntegration("orders", handler),
});

// Cognito authorizer
const authz = new HttpJwtAuthorizer("CognitoAuth", `https://cognito-idp.us-east-1.amazonaws.com/${userPool.userPoolId}`, {
  jwtAudience: [appClient.userPoolClientId],
});
api.addRoutes({
  path: "/admin",
  methods: [apigwv2.HttpMethod.ANY],
  integration: new HttpLambdaIntegration("admin", adminFn),
  authorizer: authz,
});
```

### Stages and deployments
A **stage** = a versioned snapshot of routes (`prod`, `staging`). API Gateway has built-in **canary deployments** — route X% to a new version.

### Throttling
- **Account level**: 10000 RPS default, can be raised.
- **Stage / route level**: rate limit + burst limit per second.
- **Usage plans** (REST API only): per-API-key limits.

```bash
aws apigatewayv2 update-stage --api-id $ID --stage-name prod \
  --default-route-settings 'ThrottlingRateLimit=500,ThrottlingBurstLimit=1000'
```

### Authorizers
- **JWT / OIDC**: validate tokens (Cognito, Auth0, Okta).
- **Lambda authorizer**: custom logic. Slower (extra Lambda call) but flexible.
- **IAM**: SigV4-signed requests (great for internal service-to-service).
- **None**: public.

---

## 3. API Gateway REST API — when you need more

Older, more feature-rich, more expensive. Worth it when:
- You need **request/response transformation** via Velocity Templates (legacy but powerful).
- You need **API keys + usage plans** with throttling tiers.
- You need **request validation** schemas.
- You need **edge-optimized** endpoints (REST API supports `EDGE` type that uses CloudFront automatically; HTTP API doesn't, but you can add CloudFront yourself).

```bash
aws apigateway create-rest-api --name MyApi --endpoint-configuration types=REGIONAL
```

### Integration types
- **Lambda Proxy** (recommended): event passed as-is, response shaped automatically.
- **Lambda non-proxy**: VTL transformations on input/output.
- **HTTP** / **HTTP_PROXY**: forward to any HTTP backend.
- **AWS**: direct integration to AWS services (you can build a "no-Lambda" API that just talks to DynamoDB / Step Functions).
- **Mock**: returns a canned response. Useful for stubs.

---

## 4. AppSync — managed GraphQL

GraphQL: clients declare what they need, server resolves from multiple data sources.

### Why AppSync (vs running Apollo on Lambda)
- Built-in resolvers (mapping templates or JavaScript) for DynamoDB, RDS Data API, OpenSearch, HTTP, Lambda — no boilerplate.
- **Subscriptions** over WebSocket out of the box.
- Cognito / IAM / API key / OIDC / Lambda auth.
- Schema-driven, with caching.

```graphql
type Order @aws_cognito_user_pools {
  orderId: ID!
  customerId: ID!
  total: Float!
  status: String!
}

type Query {
  getOrder(orderId: ID!): Order
}

type Mutation {
  updateStatus(orderId: ID!, status: String!): Order
}

type Subscription {
  onStatusChanged(orderId: ID!): Order
    @aws_subscribe(mutations: ["updateStatus"])
}
```

### Resolvers
Map a GraphQL field to a data source. JavaScript resolvers (since 2022) replace VTL for most cases.

```javascript
// getOrder resolver — DynamoDB
import { util } from "@aws-appsync/utils";
export function request(ctx) {
  return {
    operation: "GetItem",
    key: util.dynamodb.toMapValues({ PK: `ORDER#${ctx.args.orderId}` }),
  };
}
export function response(ctx) {
  return ctx.result;
}
```

### Subscriptions
The killer feature. Clients subscribe; when the matching mutation fires, AppSync pushes via WebSocket. Use for live dashboards, collaborative editing, multiplayer games.

### Pipeline resolvers
Chain multiple resolvers (e.g., fetch user → enrich with orders → return). Avoids "Lambda just to orchestrate".

---

## 5. WebSocket API (API Gateway)

Bidirectional. Use when AppSync subscriptions don't fit (custom protocols, non-GraphQL).

Routes are based on the message body's `action` field:
- `$connect`: client opened.
- `$disconnect`: client closed.
- `$default`: catch-all.
- Custom routes (e.g., `sendMessage`) chosen by `action` in the JSON body.

Server-side push: backend calls `PostToConnection` against a connection ID stored at `$connect`.

```python
mgmt = boto3.client("apigatewaymanagementapi",
                    endpoint_url=f"https://{api_id}.execute-api.us-east-1.amazonaws.com/prod")
mgmt.post_to_connection(ConnectionId=conn_id, Data=b'{"event":"new_message"}')
```

---

## 6. Custom Domains, TLS, and WAF

- **ACM certs**: free, auto-renewed. Must be in `us-east-1` for CloudFront-fronted APIs; same region as the API otherwise.
- **Custom domain mapping**: `api.example.com` → API Gateway domain.
- **Route 53 ALIAS record** points to the custom domain endpoint.
- **AWS WAF** in front of API Gateway / AppSync / CloudFront for SQLi / XSS / rate / geo / managed rule groups.

```bash
aws apigatewayv2 create-domain-name --domain-name api.example.com \
  --domain-name-configurations CertificateArn=$CERT_ARN,EndpointType=REGIONAL
aws apigatewayv2 create-api-mapping --domain-name api.example.com --api-id $ID --stage prod
```

---

## 7. Practical: a serverless REST + WebSocket stack

```typescript
// Authenticated CRUD + live updates
const userPool = new cognito.UserPool(this, "Users", { selfSignUpEnabled: true });
const appClient = userPool.addClient("Web");

const httpApi = new apigwv2.HttpApi(this, "OrdersApi");
const authz = new HttpJwtAuthorizer("Auth",
  `https://cognito-idp.${region}.amazonaws.com/${userPool.userPoolId}`,
  { jwtAudience: [appClient.userPoolClientId] });

httpApi.addRoutes({
  path: "/orders/{orderId}",
  methods: [apigwv2.HttpMethod.GET, apigwv2.HttpMethod.PUT],
  integration: new HttpLambdaIntegration("orders", ordersFn),
  authorizer: authz,
});

// AppSync for real-time order status
const gql = new appsync.GraphqlApi(this, "OrdersGraph", {
  name: "orders-gql",
  schema: appsync.SchemaFile.fromAsset("schema.graphql"),
  authorizationConfig: {
    defaultAuthorization: {
      authorizationType: appsync.AuthorizationType.USER_POOL,
      userPoolConfig: { userPool },
    },
  },
});
const ordersDs = gql.addDynamoDbDataSource("orders", ordersTable);
ordersDs.createResolver("GetOrder", {
  typeName: "Query", fieldName: "getOrder",
  requestMappingTemplate: appsync.MappingTemplate.dynamoDbGetItem("PK", "orderId"),
  responseMappingTemplate: appsync.MappingTemplate.dynamoDbResultItem(),
});
```

---

## 8. CORS — the eternal source of pain

Browsers enforce CORS; API Gateway can return CORS headers automatically when you configure it.

```typescript
new apigwv2.HttpApi(this, "Api", {
  corsPreflight: {
    allowOrigins: ["https://app.example.com"],
    allowMethods: [apigwv2.CorsHttpMethod.GET, apigwv2.CorsHttpMethod.POST],
    allowHeaders: ["Content-Type", "Authorization"],
    allowCredentials: false,
    maxAge: cdk.Duration.hours(1),
  },
});
```

**Common CORS bug**: backend errors return 500 without CORS headers → browser reports the error as "CORS issue" rather than 500. Misleading. Add CORS to error responses too.

---

## 9. Common Mistakes & Gotchas

- **Choosing REST API for new projects.** HTTP API is cheaper, faster, simpler. Pick HTTP unless you need a specific REST-only feature.
- **API Gateway as a generic proxy.** It's not designed to forward arbitrary HTTP — use ALB or CloudFront.
- **No throttling configured.** First DDoS = surprise bill. Set stage-level limits.
- **Lambda authorizer with no caching.** Each request = extra Lambda call. Enable result caching (300s default).
- **CORS configured only on success path.** Errors return without headers; clients see "CORS error" instead of 5xx.
- **`AWS_IAM` auth without SigV4 signing on client.** Frustration. Use `aws-amplify` or sign manually.
- **Cognito JWT audience mismatch.** Token has `aud=app-client-A`, authorizer expects `aud=B`. Silent 401s.
- **Stage variables not used.** Hardcoded `dev/prod` Lambda ARNs lead to bad deploys. Use stage variables + aliases.
- **WebSocket connection IDs not persisted.** Server can't push without storing them (typically in DynamoDB at `$connect`).
- **AppSync VTL leftover.** Modernize to JS resolvers — much easier to read.
- **GraphQL N+1.** Each field triggering a resolver per item. Use pipeline resolvers + BatchGetItem.
- **Custom domain ACM cert in wrong region.** `EDGE` type needs `us-east-1`; `REGIONAL` needs the API's region.
- **WAF charges per million requests.** Throttle first to avoid WAF-evaluating bot traffic at full price.
- **Lambda integration timeout = 29s for API Gateway.** Longer Lambda = client timeout.
- **API keys treated as auth.** They're a usage-tracking mechanism, not authentication. Real auth = Cognito/IAM/JWT.
- **403 with no useful body.** Often means resource policy or WAF denied. Check API Gateway execution logs (separate from access logs).

---

## 🎯 Key Takeaways

- **HTTP API is the new default.** Cheaper, faster, native JWT auth. REST API only for niche features.
- **AppSync subscriptions deliver real-time without you operating a WebSocket fleet.** For greenfield real-time apps, start there before reaching for raw WebSocket APIs.
- **Authorizers and throttling are non-optional.** Public endpoints without throttling are bills waiting to happen; auth without caching is a latency tax.
- **Custom domain + ACM cert + Route 53 ALIAS** is the production hat trick — set it up day one. The free APIGW URL embarrasses your brand.
- **CORS misdiagnoses 80% of "broken API" tickets.** Configure preflight properly, return headers on errors, and verify with `curl -v -H "Origin: https://app.example.com"` before blaming the network.

*← [prev](./08_lambda_serverless.md) | [next →](./10_sqs_sns_eventbridge.md)*
