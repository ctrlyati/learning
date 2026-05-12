# 11 — Front Door, Application Gateway, Traffic Manager, DNS

> **Goal:** Pick the right Azure traffic service for global anycast, regional L7, geo-routed DNS, and authoritative DNS — and stack them sensibly without paying twice for the same job.

## 1. The four services and what each is for

| Service | Layer | Scope | Picks the closest endpoint by | Use it for |
|---------|-------|-------|--------------------------------|------------|
| **Azure DNS** | L7 (DNS) | Global | DNS resolution only | Hosting authoritative DNS zones |
| **Traffic Manager** | DNS-level | Global | DNS-based routing (performance, priority, weighted, geographic, multivalue, subnet) | Geo/priority routing without proxying |
| **Application Gateway** | L7 (HTTPS) | Regional | URL-path, host, cookie affinity, headers — *with* WAF | Regional reverse proxy with WAF for backend pools in a VNet |
| **Front Door** | L7 (HTTPS) | Global anycast | Latency-based + global health probing + WAF + CDN cache | Global edge, multi-region failover, WAF, public-facing |

Two rules of thumb:

1. **Public global HTTPS** → Front Door.
2. **Private regional L7 (inside a VNet)** → Application Gateway.

You often use *both*: Front Door at the edge → Application Gateway in-region → backends. That's expensive — only do it if you actually need both WAFs and both feature sets. Common setups use Front Door alone (Premium tier with WAF) or App Gateway alone, not both.

## 2. Azure DNS — authoritative DNS, nothing more

Hosted DNS. Public zones (`acme.com`) and Private zones (`internal.acme.com`, resolves only inside linked VNets). Backed by Azure's anycast nameservers (you'll be assigned NS records like `ns1-XX.azure-dns.com`).

```bash
RG=rg-dns-prod

az network dns zone create -g $RG -n acme.com
az network dns record-set a add-record -g $RG -z acme.com -n www -a 203.0.113.10

# Private zone for internal services
az network private-dns zone create -g $RG -n internal.acme.com
az network private-dns link vnet create -g $RG -n internal-link \
  --zone-name internal.acme.com --virtual-network vnet-prod-eastus2 \
  --registration-enabled false
```

For public zones, you must change your registrar's NS records to Azure's nameservers once. After that, all record management is `az` or Bicep.

**Alias records** are an Azure-specific RR type — they point at an Azure resource (a Public IP, Traffic Manager profile, Front Door endpoint, Storage static website) and *follow* that resource as its IP/CNAME changes. Use them for apex records (`acme.com` → Front Door endpoint).

```bash
az network dns record-set a create -g $RG -z acme.com -n @ \
  --target-resource $(az afd endpoint show -g rg-edge -n endpt-acme --profile-name afd-acme --query id -o tsv)
```

## 3. Traffic Manager — DNS-level global routing

Traffic Manager (ATM) routes by *DNS resolution*. Clients resolve `acme.trafficmanager.net` (or your CNAMEd `acme.com`); Traffic Manager returns the IP/CNAME of the chosen endpoint; the client connects directly. ATM never sees the actual traffic.

Routing methods:

- **Priority** — primary endpoint; failover if unhealthy.
- **Weighted** — round-robin with weights.
- **Performance** — closest endpoint by latency map (Microsoft's measured network).
- **Geographic** — country/state of the resolver decides.
- **MultiValue** — return multiple healthy IPs.
- **Subnet** — match resolver's IP subnet to a specific endpoint.

```bash
az network traffic-manager profile create \
  -g $RG -n tm-acme \
  --routing-method Priority \
  --unique-dns-name tm-acme \
  --ttl 60 \
  --monitor-protocol HTTPS --monitor-port 443 --monitor-path /healthz

az network traffic-manager endpoint create \
  -g $RG --profile-name tm-acme --name eastus2 \
  --type externalEndpoints --target www-eastus2.acme.com --priority 1

az network traffic-manager endpoint create \
  -g $RG --profile-name tm-acme --name westeurope \
  --type externalEndpoints --target www-westeurope.acme.com --priority 2
```

DNS-level routing has caveats:

- **TTL** — clients cache the DNS answer for the TTL. Failover is not instant; it's TTL-bound.
- **No traffic inspection.** It can't do WAF or path-based routing.
- **Clients hit endpoints directly** — public IPs needed on backends.

ATM is fine for "DR failover between regions" or "geo-route to nearest data center" when you don't need a global proxy. For modern public web traffic, Front Door is usually a better answer.

## 4. Application Gateway — regional L7 with WAF

Application Gateway (AppGw) is a reverse proxy. SSL termination, URL routing, cookie-based sticky sessions, host-based routing, WebSocket, HTTP/2. Backed by VM scale set under the hood.

Two SKUs:

- **Standard_v2** — production L7 ingress.
- **WAF_v2** — Standard + integrated Web Application Firewall (OWASP CRS, bot protection, custom rules).

AppGw lives in your VNet, gets a public and/or private IP, and routes to backend pools (NICs, IPs/FQDNs, App Service apps, AKS services via App Gateway Ingress Controller).

```bash
az network public-ip create -g $RG -n pip-appgw --sku Standard --zone 1 2 3
az network application-gateway create \
  -g $RG -n appgw-acme \
  --location eastus2 \
  --vnet-name vnet-prod-eastus2 --subnet snet-appgw \
  --capacity 2 --sku WAF_v2 \
  --http-settings-cookie-based-affinity Enabled \
  --frontend-port 443 \
  --public-ip-address pip-appgw \
  --priority 100 \
  --servers app-orders-prod.azurewebsites.net \
  --waf-policy $(az network application-gateway waf-policy show -g $RG -n waf-default --query id -o tsv)
```

AppGw needs:

- Its **own subnet** (commonly `snet-appgw`, `/26` or larger).
- A WAF policy (separate resource) attached for WAF SKU.
- Listeners + rules + backend pools + HTTP settings — four concepts you wire together. Bicep is far cleaner than CLI for non-trivial setups.

Use AppGw when:

- Your backends are inside the VNet and must not be public.
- You need path-based routing for a single region.
- You need WAF in a region where Front Door doesn't suffice (e.g., regulatory data-residency).
- You're integrating with AKS via AGIC (Application Gateway Ingress Controller).

## 4.5. Front Door — global anycast L7

Azure Front Door (AFD) is Microsoft's global edge. Anycast IPs in 100+ POPs. SSL termination at the edge, route to backend "origins" anywhere (Azure, on-prem, other clouds). Three things bundled:

- **Edge load balancing** with latency-based routing and active health probes.
- **WAF** (Front Door Standard/Premium) — same rule families as App Gateway WAF.
- **CDN** caching (rule-based).

Tiers:

- **Standard** — most features, custom rules.
- **Premium** — adds managed rule sets (Microsoft + OWASP), private-link origins (reach origins in private VNets without public IPs), bot manager.

```bash
RG=rg-edge-prod
az afd profile create -g $RG -n afd-acme --sku Premium_AzureFrontDoor

az afd endpoint create -g $RG --profile-name afd-acme --endpoint-name endpt-acme

az afd origin-group create -g $RG --profile-name afd-acme -n og-app \
  --probe-path /healthz --probe-protocol Https --probe-request-type GET --probe-interval-in-seconds 30 \
  --sample-size 4 --successful-samples-required 3 --additional-latency-in-milliseconds 50

az afd origin create -g $RG --profile-name afd-acme --origin-group-name og-app -n eastus2 \
  --host-name app-orders-eastus2.azurewebsites.net --origin-host-header app-orders-eastus2.azurewebsites.net \
  --priority 1 --weight 1000 --enabled-state Enabled --enable-private-link false

az afd origin create -g $RG --profile-name afd-acme --origin-group-name og-app -n westus3 \
  --host-name app-orders-westus3.azurewebsites.net --origin-host-header app-orders-westus3.azurewebsites.net \
  --priority 1 --weight 1000

az afd route create -g $RG --profile-name afd-acme --endpoint-name endpt-acme -n default \
  --origin-group og-app --supported-protocols Https --forwarding-protocol HttpsOnly \
  --link-to-default-domain Enabled --https-redirect Enabled
```

Notes worth memorizing:

- **Private Link origins (Premium)** — Front Door reaches App Service / AppGw / internal Load Balancer through Private Link without exposing them publicly. The cleanest pattern for "global edge + private backends."
- **Custom domains + managed certs** — Front Door provisions free DigiCert certificates with auto-rotation. No more cert headaches.
- **Caching is rule-based.** You can cache by path pattern, query string keys, headers. Purge with `az afd endpoint purge`.
- **Restrict backends to AFD.** On App Service / AppGw, restrict access to service tag `AzureFrontDoor.Backend` *and* require `X-Azure-FDID: <your-profile-id>` header — without this, anyone with the backend hostname can bypass Front Door.

## 4.6. Practical Application — public web app, two regions, fronted globally

```
acme.com                            ┌──────────┐
   │                                │ Azure DNS │  alias → AFD endpoint
   ▼                                └─────┬─────┘
┌─────────────────────────────┐           │
│      Front Door Premium     │ ◄─── public anycast IP
│      (WAF, cache, route)    │
└───────┬─────────────┬───────┘
        │             │ Private Link origin (Premium)
        ▼             ▼
   App Service       App Service
   eastus2 (prod)    westus3 (prod)
        │             │
        ▼             ▼
   private endpoints to SQL, Cosmos, Storage
```

Bicep sketch (origin only):

```bicep
resource afd 'Microsoft.Cdn/profiles@2024-02-01' = {
  name: 'afd-acme'
  location: 'global'
  sku: { name: 'Premium_AzureFrontDoor' }
}

resource endpoint 'Microsoft.Cdn/profiles/afdEndpoints@2024-02-01' = {
  parent: afd
  name: 'endpt-acme'
  location: 'global'
  properties: { enabledState: 'Enabled' }
}

resource og 'Microsoft.Cdn/profiles/originGroups@2024-02-01' = {
  parent: afd
  name: 'og-app'
  properties: {
    loadBalancingSettings: { sampleSize: 4, successfulSamplesRequired: 3, additionalLatencyInMilliseconds: 50 }
    healthProbeSettings: { probePath: '/healthz', probeRequestType: 'GET', probeProtocol: 'Https', probeIntervalInSeconds: 30 }
  }
}

resource originEastUs2 'Microsoft.Cdn/profiles/originGroups/origins@2024-02-01' = {
  parent: og
  name: 'eastus2'
  properties: {
    hostName: 'app-orders-eastus2.azurewebsites.net'
    originHostHeader: 'app-orders-eastus2.azurewebsites.net'
    httpsPort: 443
    priority: 1
    weight: 1000
    enabledState: 'Enabled'
    sharedPrivateLinkResource: {
      privateLink: { id: appServiceEastUs2Id }
      groupId: 'sites'
      requestMessage: 'Front Door access for orders app'
      privateLinkLocation: 'eastus2'
    }
  }
}
```

(`sharedPrivateLinkResource` is the Front-Door-to-App-Service Private Link pattern.)

## 5. Common Mistakes & Gotchas

- **Stacking AppGw inside AFD without need.** Each adds latency, cost, and operational complexity. Default: AFD alone unless you have a regional WAF or path-routing requirement AFD can't meet.
- **DNS apex record on a non-Azure CNAME.** You can't put a CNAME at the apex (`acme.com`). Use Azure DNS **alias records** to point to AFD/Traffic Manager.
- **Traffic Manager probe failure** because backends aren't returning 200 on the probe path. Add a `/healthz` endpoint that touches all critical dependencies (DB ping, cache ping) and returns 200/503.
- **AppGw subnet too small or has other resources.** Must be empty (except other AppGws), `/26`+, and *cannot* have NSG rules that block `65200-65535` (probe management ports).
- **Backend not accepting the host header.** AppGw and AFD by default send the original host header to the backend. If your backend is `app-orders-eastus2.azurewebsites.net` but the client requested `acme.com`, App Service may 404. Either override host header at the proxy or configure custom domain bindings on the backend.
- **TLS termination behavior.** Both AFD and AppGw terminate TLS at the edge and *can* re-encrypt to origin. Default re-encryption is on; verify your origin trusts the cert chain.
- **WAF in detection-only mode forever.** Detection mode logs but doesn't block. Move to prevention mode after baseline; otherwise the WAF is decoration.
- **Front Door cache poisoning.** Caching responses keyed only on path means user-specific responses can leak across users. Set `Cache-Control: private` for personalized responses or vary cache keys on auth headers.
- **AFD restrict-by-FDID forgotten.** Without `X-Azure-FDID` enforcement, your "Front Door fronted" backend is still directly addressable on its public hostname. Combine NSG/access-restriction on the service tag with the FDID header check.
- **Traffic Manager with non-public endpoints.** ATM resolves DNS to IPs that clients must reach. Putting it in front of internal load balancers is wrong — use AppGw or AFD with Private Link instead.
- **Premium AFD egress costs.** AFD bills per-GB egress from the edge to the user. Cache aggressively; offload static assets; otherwise the bill stings.
- **AppGw v1 (classic).** Deprecated. Use v2 only.

## 🎯 Key Takeaways

- **Front Door = global anycast L7 + WAF + CDN.** The right default for public web apps. Premium tier unlocks private-link origins — use it.
- **Application Gateway = regional L7 + WAF inside your VNet.** Use it when backends must be private and you don't need global edge, or paired with AGIC for AKS.
- **Traffic Manager = DNS-level geo/priority routing.** No traffic interception. Useful for DR failover and geographic routing without proxying.
- **Azure DNS hosts your zones.** Use alias records at the apex to point at Azure-resource endpoints — they follow the resource.
- **Lock backends to the edge.** `AzureFrontDoor.Backend` service tag + `X-Azure-FDID` header check (or Private Link origins on Premium). Without it, your edge is decorative.

*← [prev](./10_messaging.md) | [next → 12_containers.md](./12_containers.md)*
