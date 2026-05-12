# 04 — Azure Networking: VNets, NSGs, Private Link, Bastion

> **Goal:** Be able to design, draw, and `az`-deploy a multi-tier Azure network — VNets, subnets, NSGs, route tables, peering, Private Link, Bastion — and know which Azure design choice differs from AWS VPCs.

## 1. Virtual Network — the L3 boundary

An **Azure Virtual Network (VNet)** is the L3 isolation primitive. Like an AWS VPC, it is a private IPv4 (optionally IPv6) address space confined to a single region and a single subscription. Resources placed in a VNet can talk to each other; resources in different VNets cannot — unless you peer them or route through a hub.

A VNet has:

- One or more **address spaces** (CIDR blocks, e.g. `10.50.0.0/16`). You can add or remove blocks any time — non-disruptive.
- One or more **subnets** (e.g. `10.50.1.0/24`). Subnets are *not* zone-bound — they span all zones of the region.
- Optional **DDoS protection** (Basic is free and on by default; Standard is paid per-VNet).
- Optional **encryption** of VM-to-VM traffic within the VNet (newer feature, off by default).

Create one:

```bash
az network vnet create \
  --name vnet-prod-eastus2 \
  --resource-group rg-network-prod \
  --address-prefixes 10.50.0.0/16 \
  --subnet-name snet-app \
  --subnet-prefixes 10.50.1.0/24
```

### Address planning — the most important non-fun task

If you pick `10.0.0.0/16` and your company already has `10.0.0.0/16` in another VNet or on-prem, you cannot peer or VPN them. This is the #1 unforced error in Azure networking. Adopt an IP plan *before* you create your first VNet. A safe scheme for a learning sub:

| Environment | VNet CIDR | Typical subnets |
|-------------|-----------|-----------------|
| hub-prod    | `10.0.0.0/22`   | GatewaySubnet, AzureFirewallSubnet, AzureBastionSubnet |
| dev         | `10.10.0.0/16`  | snet-app, snet-data, snet-pe |
| prod        | `10.20.0.0/16`  | snet-app, snet-data, snet-pe, snet-mgmt |

Use the `172.16.0.0/12` or `100.64.0.0/10` spaces if 10.0/8 is fragmented at your company. Avoid `192.168.0.0/16` (home-router collision).

## 2. Subnets, NSGs, and route tables — the L3/L4 mechanism

A **subnet** is a CIDR sub-block of a VNet. Resources (NICs, app gateways, function apps with VNet integration, etc.) attach to a subnet. Three subnet rules that will bite you:

- **Azure reserves the first 4 and last 1 address** of every subnet (network address, default gateway, DNS proxy, broadcast). A `/29` gives you 3 usable IPs, not 6.
- **Some services require dedicated subnets** with reserved names: `GatewaySubnet`, `AzureFirewallSubnet`, `AzureBastionSubnet`, `AzureFirewallManagementSubnet`, `RouteServerSubnet`. Don't put VMs in them.
- **Resizing a subnet** is possible but only if no resources occupy the IPs being removed.

### Network Security Groups

An **NSG** is a stateful firewall — a list of allow/deny rules evaluated in priority order. You can attach NSGs to **subnets**, to **NICs**, or both. If both, rules apply in sequence (subnet first for inbound, NIC first for outbound).

Each rule has: priority (100-4096, lower = first), direction, source, destination, port, protocol, action. Azure adds a baseline of default rules (`AllowVnetInBound`, `AllowAzureLoadBalancerInBound`, `DenyAllInBound`, etc.) at priority 65000+ — you can't delete them but you can override with lower-priority rules.

```bash
az network nsg create --name nsg-snet-app --resource-group rg-network-prod
az network nsg rule create --nsg-name nsg-snet-app --resource-group rg-network-prod \
  --name AllowHttpsFromVnet --priority 100 \
  --source-address-prefixes VirtualNetwork --source-port-ranges '*' \
  --destination-address-prefixes '*' --destination-port-ranges 443 \
  --access Allow --protocol Tcp --direction Inbound

az network vnet subnet update --vnet-name vnet-prod-eastus2 --name snet-app \
  --resource-group rg-network-prod \
  --network-security-group nsg-snet-app
```

**Service Tags** (`VirtualNetwork`, `AzureLoadBalancer`, `Internet`, `AzureCloud.<region>`, `Storage`, `Sql`, etc.) are Microsoft-maintained dynamic IP lists you reference in rules. They beat hand-maintaining IP ranges.

### Route tables (User-Defined Routes)

Azure injects a **system route table** into every subnet:

- `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16` → "VNet" (next-hop = within VNet/peering).
- `0.0.0.0/0` → "Internet" (next-hop = direct egress).

You override with a **UDR**: e.g., force `0.0.0.0/0` through an Azure Firewall in the hub.

```bash
az network route-table create --name rt-snet-app --resource-group rg-network-prod
az network route-table route create \
  --route-table-name rt-snet-app --resource-group rg-network-prod \
  --name default-via-fw --address-prefix 0.0.0.0/0 \
  --next-hop-type VirtualAppliance --next-hop-ip-address 10.0.1.4

az network vnet subnet update --vnet-name vnet-prod-eastus2 --name snet-app \
  --resource-group rg-network-prod \
  --route-table rt-snet-app
```

Order: NSG → UDR. NSG can block traffic the UDR sends — debugging requires both.

## 3. Variations and depth — peering, Private Link, Service Endpoints, Bastion

### VNet peering

Connect two VNets so they can communicate as if they were one. Peering is:

- **Non-transitive.** A↔B and B↔C does not give you A↔C. You either mesh-peer or route through a hub (hub-spoke topology).
- **Cross-region capable** (global peering) and **cross-subscription/tenant capable**. Latency = inter-region link.
- **Cheap but not free.** Charged per GB in *both* directions.

```bash
az network vnet peering create \
  --name vnet-prod-to-hub \
  --resource-group rg-network-prod \
  --vnet-name vnet-prod-eastus2 \
  --remote-vnet $(az network vnet show -g rg-hub -n vnet-hub-eastus2 --query id -o tsv) \
  --allow-vnet-access --allow-forwarded-traffic
# Then create the reciprocal peering on the hub.
```

The two `--allow-*` flags govern hub-spoke patterns. Set them deliberately.

### Service Endpoints vs Private Endpoints — the only diagram you need

PaaS services like Storage and SQL have public endpoints. To keep traffic on the Microsoft backbone (and off the public internet), you have two mechanisms:

| | Service Endpoint | Private Endpoint (Private Link) |
|--|--|--|
| What it is | Subnet attribute that adds the service's *public IP range* to your VNet routing table | A NIC in your subnet with a *private* IP that fronts the PaaS service |
| IP address | Still resolves to public IP | Resolves to **your** RFC1918 IP via Private DNS |
| Cross-region | Same region only | Any region (charged per hour + per GB) |
| On-prem reachable | No | Yes (via VPN/ExpressRoute + Private DNS forwarding) |
| Cost | Free | ~USD 0.01/hour + data |
| Use today? | Legacy, fine for simple cases | The modern default |

Private Endpoint example for a storage account:

```bash
PE_SUBNET=$(az network vnet subnet show -g rg-network-prod -n snet-pe --vnet-name vnet-prod-eastus2 --query id -o tsv)
SA_ID=$(az storage account show -n stappdataprod -g rg-data --query id -o tsv)

az network private-endpoint create \
  --name pe-stappdataprod-blob \
  --resource-group rg-network-prod \
  --subnet $PE_SUBNET \
  --private-connection-resource-id $SA_ID \
  --group-id blob \
  --connection-name pe-stappdataprod-blob-conn

# Attach Private DNS zone for blob.core.windows.net so name resolution works.
az network private-dns zone create -g rg-network-prod -n privatelink.blob.core.windows.net
az network private-dns link vnet create -g rg-network-prod -n pdz-blob-link \
  --zone-name privatelink.blob.core.windows.net \
  --virtual-network vnet-prod-eastus2 --registration-enabled false
az network private-endpoint dns-zone-group create -g rg-network-prod \
  --endpoint-name pe-stappdataprod-blob --name default \
  --private-dns-zone privatelink.blob.core.windows.net --zone-name blob
```

That last block looks long but is the *complete* Private Link recipe. Memorize it.

### Azure Bastion

SSH/RDP to VMs without giving them public IPs. Bastion is a managed jumpbox you reach via portal HTTPS, that then connects to your private VMs over the VNet.

```bash
az network public-ip create -g rg-hub -n pip-bastion --sku Standard --zone 1 2 3
az network bastion create -g rg-hub -n bast-hub \
  --vnet-name vnet-hub-eastus2 --public-ip-address pip-bastion --sku Basic
```

Bastion needs a subnet named exactly `AzureBastionSubnet` of size `/26` or larger. SKU choices: Developer (free, single-vnet, single-session), Basic, Standard, Premium. Standard supports native client (`az network bastion ssh ...`).

### Hub-spoke topology (the standard enterprise pattern)

```
       ┌──────────────────┐
       │   on-prem DC     │
       └────────┬─────────┘
              VPN/ER
                │
         ┌──────▼──────┐
         │  Hub VNet   │  ← Azure Firewall, Bastion, VPN GW, DNS, monitoring
         └──┬───┬───┬──┘
            │   │   │  peering
   ┌────────▼┐ ┌▼───┐ ┌▼────────┐
   │ prod   │ │dev │ │ shared  │  spokes (workloads)
   └────────┘ └────┘ └─────────┘
```

Every spoke peers to the hub, never to other spokes. The hub holds shared services (firewall, gateway, Bastion, Private DNS). Traffic between spokes is routed through the hub's firewall. This is module 18's territory and the de-facto enterprise pattern.

## 4. Practical Application — a two-tier VNet with private SQL

Goal: an app subnet talking to a Private-Endpoint-fronted Azure SQL.

```bicep
param location string = resourceGroup().location
var vnetName = 'vnet-app-prod'

resource vnet 'Microsoft.Network/virtualNetworks@2023-09-01' = {
  name: vnetName
  location: location
  properties: {
    addressSpace: { addressPrefixes: ['10.20.0.0/16'] }
    subnets: [
      {
        name: 'snet-app'
        properties: { addressPrefix: '10.20.1.0/24' }
      }
      {
        name: 'snet-pe'
        properties: {
          addressPrefix: '10.20.2.0/24'
          privateEndpointNetworkPolicies: 'Disabled'
        }
      }
    ]
  }
}

resource pdz 'Microsoft.Network/privateDnsZones@2024-06-01' = {
  name: 'privatelink.database.windows.net'
  location: 'global'
}

resource pdzLink 'Microsoft.Network/privateDnsZones/virtualNetworkLinks@2024-06-01' = {
  parent: pdz
  name: '${vnetName}-link'
  location: 'global'
  properties: {
    virtualNetwork: { id: vnet.id }
    registrationEnabled: false
  }
}

resource sql 'Microsoft.Sql/servers@2023-08-01-preview' = {
  name: 'sql-app-prod-${uniqueString(resourceGroup().id)}'
  location: location
  properties: {
    administratorLogin: 'sqladmin'
    administratorLoginPassword: 'P@ssw0rdRotateMe!'
    publicNetworkAccess: 'Disabled'
    minimalTlsVersion: '1.2'
  }
}

resource pe 'Microsoft.Network/privateEndpoints@2023-09-01' = {
  name: 'pe-sql-app-prod'
  location: location
  properties: {
    subnet: { id: '${vnet.id}/subnets/snet-pe' }
    privateLinkServiceConnections: [{
      name: 'sql-conn'
      properties: {
        privateLinkServiceId: sql.id
        groupIds: ['sqlServer']
      }
    }]
  }
}

resource peDns 'Microsoft.Network/privateEndpoints/privateDnsZoneGroups@2023-09-01' = {
  parent: pe
  name: 'default'
  properties: {
    privateDnsZoneConfigs: [{
      name: 'sql'
      properties: { privateDnsZoneId: pdz.id }
    }]
  }
}
```

Deploy → SQL is unreachable from the public internet, but resolves to a 10.20.2.x address from anything in the VNet. Combined with Entra-only auth (module 14) you've got a strong baseline.

## 5. Common Mistakes & Gotchas

- **IP overlap with on-prem or another VNet.** You won't notice until you try to peer/VPN, at which point Azure rejects the connection. Plan IP space first.
- **NSG blocks `AzureLoadBalancer` or `VirtualNetwork` traffic.** Adding a deny-all rule at low priority can break health probes, intra-VNet traffic, even App Service VNet integration. Always order: specific allows (high priority) → specific denies (lower) → leave default rules alone.
- **UDR + NSG interaction.** A UDR redirects traffic to a firewall, but the NSG on the firewall subnet must allow that traffic *and* the NSG on the destination subnet must allow the traffic with the firewall's IP as source.
- **Forgetting `--allow-forwarded-traffic` on peering.** Spoke can't talk through hub firewall to another spoke; classic hub-spoke debug session.
- **Service Endpoint vs Private Endpoint confusion.** Service Endpoints leave the destination's IP *public*; only access is restricted to the VNet. Private Endpoint puts an IP *in your VNet*. They are not equivalent.
- **Private DNS zones not linked.** Private Endpoint creates a NIC but DNS resolution still goes to the public IP. Without the Private DNS zone + VNet link, your apps will connect to the public endpoint and hit firewall denies. Always pair PE + Private DNS.
- **Bastion subnet wrong name or wrong size.** `AzureBastionSubnet` *exactly*, `/26` or larger. Anything else: deployment fails.
- **Public IP SKU mismatches.** Standard SKU Public IPs are zonal-aware and *deny by default* (need NSG allow); Basic is being retired. Always use Standard.
- **VNet peering region for global peering costs more.** Within-region peering is cheap; cross-region inter-VNet egress is at the inter-region rate.
- **DDoS Standard cost.** Per-VNet, not per-IP. Cheap for one VNet; expensive when you have 50. Apply at hub VNets, not every spoke.
- **`privateEndpointNetworkPolicies` defaults to `Disabled` historically.** In some recent API versions it defaults to `Enabled`, which can interact with NSGs on the PE subnet in surprising ways. Set it explicitly.
- **Azure vs AWS:** Subnets are *not* AZ-bound in Azure. Resources within a subnet can land in any zone — you pick zone at *resource* creation. Subnet itself spans zones. This is the opposite of AWS.

## 🎯 Key Takeaways

- **VNet + subnet + NSG + (optional) UDR is the minimum-viable Azure network.** Internalize the order: NSG filters, UDR routes.
- **Private Endpoint + Private DNS is the modern way to lock down PaaS.** Service Endpoints are legacy; Private Endpoints are the answer.
- **Peering is non-transitive.** Hub-spoke with shared firewall is the standard enterprise topology — every interview-relevant design.
- **Plan IP space before you create the first VNet.** Overlap with on-prem or sibling VNets is the most expensive mistake to fix.
- **Subnets in Azure span zones**, unlike AWS. Zone-awareness is at the resource (VM, disk, IP, LB) level.

*← [prev](./03_rbac.md) | [next → 05_vms.md](./05_vms.md)*
