# 03 — VPC & Networking: Building Your Own Private Cloud

> **Goal:** Design VPCs that are secure, multi-AZ, cost-aware, and connect cleanly to the internet, on-prem, and other AWS accounts — and know exactly which firewall denied that packet.

---

## 1. The VPC — a software-defined private datacenter

**Mental model:** A VPC is a private, isolated slice of the AWS network you own. It has its own RFC 1918 IP range. Inside it, you carve subnets, route packets, and attach firewalls. To AWS, you're a tenant in a giant switching fabric; the VPC is your tenant view.

A VPC is **regional**. It spans all AZs in a region. Subnets within it are **AZ-scoped**.

```bash
# Create a /16 VPC (~65,000 IPs)
aws ec2 create-vpc --cidr-block 10.0.0.0/16 \
  --tag-specifications 'ResourceType=vpc,Tags=[{Key=Name,Value=demo-vpc}]'
# returns VpcId vpc-0abc...

# Enable DNS resolution (almost always wanted)
aws ec2 modify-vpc-attribute --vpc-id vpc-0abc --enable-dns-support
aws ec2 modify-vpc-attribute --vpc-id vpc-0abc --enable-dns-hostnames
```

### Default VPC vs custom VPC

Every region ships with a default VPC (`172.31.0.0/16`) with one public subnet per AZ. Fine for first experiments, terrible for anything real. Build custom VPCs.

### CIDR planning rules of thumb
- Pick a /16 for the VPC.
- Reserve enough space for **future AZs** and for **peering** (no overlapping CIDRs!).
- Plan for at least 3 AZs.
- Don't use `10.0.0.0/16` if you might peer with another account that already uses it.

---

## 2. Subnets, Route Tables, Gateways — the packet's journey

A **subnet** is a slice of the VPC's CIDR bound to a single AZ. Each subnet has an associated **route table**, which determines where packets go.

### Public vs Private subnets

The label is a convention, not a setting. A subnet is "public" if its route table has `0.0.0.0/0 → IGW` (Internet Gateway). It's "private" if it routes `0.0.0.0/0` through a NAT or has no internet route.

### Internet Gateway (IGW)
Attached to a VPC. Provides bidirectional internet connectivity to resources with public IPs. **Free.**

### NAT Gateway
A managed service in a *public* subnet that lets *private* subnet resources reach the internet outbound, while remaining unreachable inbound. **$$$**: ~$0.045/hr (~$32/mo) **per gateway** + $0.045/GB processed. Common $$$ surprise.

For HA, deploy **one NAT per AZ**, with private subnets routing to the NAT in their own AZ (avoids inter-AZ data transfer charges).

### Egress-only IGW (IPv6)
The IPv6 equivalent of NAT for IPv6 traffic.

### Build it
```bash
VPC=vpc-0abc

# Two public subnets (one per AZ)
aws ec2 create-subnet --vpc-id $VPC --cidr-block 10.0.1.0/24 \
  --availability-zone ap-southeast-1a --tag-specifications 'ResourceType=subnet,Tags=[{Key=Name,Value=public-a}]'
aws ec2 create-subnet --vpc-id $VPC --cidr-block 10.0.2.0/24 \
  --availability-zone ap-southeast-1b --tag-specifications 'ResourceType=subnet,Tags=[{Key=Name,Value=public-b}]'

# Two private subnets
aws ec2 create-subnet --vpc-id $VPC --cidr-block 10.0.11.0/24 --availability-zone ap-southeast-1a
aws ec2 create-subnet --vpc-id $VPC --cidr-block 10.0.12.0/24 --availability-zone ap-southeast-1b

# IGW
IGW=$(aws ec2 create-internet-gateway --query InternetGateway.InternetGatewayId --output text)
aws ec2 attach-internet-gateway --internet-gateway-id $IGW --vpc-id $VPC

# Public route table
PUB_RT=$(aws ec2 create-route-table --vpc-id $VPC --query RouteTable.RouteTableId --output text)
aws ec2 create-route --route-table-id $PUB_RT --destination-cidr-block 0.0.0.0/0 --gateway-id $IGW
aws ec2 associate-route-table --route-table-id $PUB_RT --subnet-id subnet-pub-a
aws ec2 associate-route-table --route-table-id $PUB_RT --subnet-id subnet-pub-b

# NAT in public-a, private route table pointing to it
EIP=$(aws ec2 allocate-address --domain vpc --query AllocationId --output text)
NAT=$(aws ec2 create-nat-gateway --subnet-id subnet-pub-a --allocation-id $EIP \
  --query NatGateway.NatGatewayId --output text)
PRIV_RT=$(aws ec2 create-route-table --vpc-id $VPC --query RouteTable.RouteTableId --output text)
aws ec2 create-route --route-table-id $PRIV_RT --destination-cidr-block 0.0.0.0/0 --nat-gateway-id $NAT
```

The same thing in **CDK** (TypeScript) — much terser:

```typescript
new ec2.Vpc(this, "DemoVpc", {
  ipAddresses: ec2.IpAddresses.cidr("10.0.0.0/16"),
  maxAzs: 2,
  natGateways: 1,  // 1 saves money; 2+ for HA
  subnetConfiguration: [
    { name: "public",  subnetType: ec2.SubnetType.PUBLIC,                       cidrMask: 24 },
    { name: "private", subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS,          cidrMask: 24 },
    { name: "isolated",subnetType: ec2.SubnetType.PRIVATE_ISOLATED,             cidrMask: 24 },
  ],
});
```

---

## 3. Firewalls: Security Groups vs Network ACLs

Two layers, easy to confuse.

| | Security Group | Network ACL |
|---|---|---|
| **Scope** | ENI (per instance) | Subnet |
| **Type** | **Stateful** | **Stateless** |
| **Rules** | Allow only | Allow & Deny |
| **Default** | Deny all in, allow all out | Allow all in/out |
| **Evaluation** | All rules considered | Rules numbered, first match wins |
| **Use for** | App-level firewall (90% of needs) | Subnet-wide blanket deny (block IPs) |

### Security Group — the workhorse
Stateful means: if you allow inbound on port 443, the response packet egresses automatically. You only write rules for what you're allowing **in**.

```bash
SG=$(aws ec2 create-security-group --group-name web-sg --description "web" --vpc-id $VPC \
  --query GroupId --output text)
aws ec2 authorize-security-group-ingress --group-id $SG --protocol tcp --port 443 --cidr 0.0.0.0/0
aws ec2 authorize-security-group-ingress --group-id $SG --protocol tcp --port 80  --cidr 0.0.0.0/0
```

**Pro tip:** SGs can reference *other SGs* as a source. Best practice: app-tier SG allows traffic only from web-tier SG, not from a CIDR.

```bash
aws ec2 authorize-security-group-ingress --group-id $APP_SG \
  --protocol tcp --port 8080 --source-group $WEB_SG
```

### NACL — the heavy artillery
Use NACLs for things SGs can't do: subnet-wide IP blocks, ephemeral-port awareness for stateless protocols, compliance requirements. Be cautious — NACLs are easy to misconfigure (stateless = you must explicitly allow both directions).

---

## 4. VPC Endpoints — keep AWS traffic off the internet

By default, an EC2 instance in a private subnet calling `s3.amazonaws.com` goes out through the NAT Gateway, across the internet, to S3 — paying NAT charges and exposing traffic externally.

**VPC Endpoints** let your instances reach AWS services over the AWS backbone, no NAT, no internet.

### Two types

**Gateway endpoints** (S3, DynamoDB only): free. A route table entry that points specific prefixes (S3's published IP ranges) to the AWS network.

```bash
aws ec2 create-vpc-endpoint --vpc-id $VPC \
  --service-name com.amazonaws.ap-southeast-1.s3 \
  --route-table-ids $PRIV_RT
```

**Interface endpoints** (everything else: SQS, SNS, Secrets Manager, ECR, KMS, STS, Lambda, ...): ~$0.01/hr per endpoint per AZ + $0.01/GB. Powered by **PrivateLink**. Each endpoint creates ENIs in subnets you specify, with private DNS that overrides the public service DNS.

```bash
aws ec2 create-vpc-endpoint --vpc-id $VPC \
  --service-name com.amazonaws.ap-southeast-1.secretsmanager \
  --vpc-endpoint-type Interface \
  --subnet-ids subnet-priv-a subnet-priv-b \
  --security-group-ids $ENDPOINT_SG \
  --private-dns-enabled
```

When is it worth the cost? When you'd otherwise pay more for NAT, or for security/compliance reasons (data must not transit the internet).

### Endpoint policies
Each endpoint can carry a policy restricting which API calls/resources it permits. A common pattern: restrict S3 endpoint to only buckets in your account.

---

## 5. Connecting VPCs and on-prem

### VPC Peering
1-to-1, non-transitive, free within a region (charged across regions). Easy for connecting two VPCs.

```bash
aws ec2 create-vpc-peering-connection --vpc-id $VPC_A --peer-vpc-id $VPC_B
# Accept it from the peer side, then add routes pointing the peer's CIDR to the peering connection.
```

### Transit Gateway (TGW)
A regional hub for connecting many VPCs and on-prem networks. Transitive. Costs ~$36/mo per attachment + data. Use it when you have >2-3 VPCs.

### AWS Site-to-Site VPN
Encrypted tunnel to on-prem. ~$36/mo per connection.

### AWS Direct Connect
Dedicated private circuit (50 Mbps to 100 Gbps). Expensive, low-latency, predictable. For serious hybrid.

### PrivateLink
Expose your own service privately to other VPCs/accounts — same machinery interface endpoints use. The clean way to provide SaaS-style services between accounts.

---

## 6. Practical: a production-shaped 3-tier VPC

```
                              Internet
                                  │
                                IGW
                                  │
              ┌───────────────────┴───────────────────┐
              │                                       │
        public-a (10.0.1.0/24)                  public-b (10.0.2.0/24)
        [ALB, NAT-a]                             [ALB, NAT-b]
              │                                       │
        private-app-a (10.0.11.0/24)             private-app-b (10.0.12.0/24)
        [ECS tasks / EC2]                        [ECS tasks / EC2]
              │                                       │
        private-data-a (10.0.21.0/24)            private-data-b (10.0.22.0/24)
        [RDS, ElastiCache]                       [RDS, ElastiCache]
```

- **public**: ALB only, plus NAT Gateways. No app instances. Route `0.0.0.0/0 → IGW`.
- **private-app**: app tier. Route `0.0.0.0/0 → NAT in same AZ`. Gateway endpoints to S3/DynamoDB. Interface endpoints to KMS/Secrets/ECR if needed.
- **private-data**: DBs and caches. *No internet route at all.* `PRIVATE_ISOLATED` in CDK terms.

SG chain:
- `alb-sg`: 80/443 from `0.0.0.0/0`.
- `app-sg`: 8080 from `alb-sg`.
- `db-sg`: 5432 from `app-sg`.

This setup costs ~$32-64/mo at idle (NAT(s)). Endpoints add ~$7/mo each.

---

## 7. Debugging: the packet flow checklist

When traffic isn't getting through, walk this:
1. **Route table.** Is there a route for the destination? IGW, NAT, peering, TGW?
2. **NACL ingress** on the destination subnet. Remember NACL is stateless — must allow ephemeral ports back.
3. **Security group ingress** on the destination ENI.
4. **The application itself listening on the right port/interface?**
5. **NACL egress** on the source subnet.
6. **Security group egress** on the source ENI (default is allow-all; rarely the issue).
7. **DNS resolution.** Is the FQDN resolving privately or publicly?

### Tools
```bash
# VPC Flow Logs (turn on per VPC/subnet/ENI)
aws ec2 create-flow-logs --resource-type VPC --resource-ids $VPC \
  --traffic-type ALL --log-destination-type cloud-watch-logs \
  --log-group-name /aws/vpc/flowlogs --deliver-logs-permission-arn $ROLE_ARN

# VPC Reachability Analyzer — pathfinding between two ENIs
aws ec2 create-network-insights-path \
  --source $SRC_ENI --destination $DST_ENI --protocol tcp --destination-port 443
aws ec2 start-network-insights-analysis --network-insights-path-id nip-xxx
# Tells you exactly which SG/NACL/route blocked
```

VPC Flow Logs + Reachability Analyzer have saved more careers than any other AWS feature.

---

## 8. Common Mistakes & Gotchas

- **NAT Gateway forgotten in a private dev VPC.** $32/mo doing nothing. Use a NAT *instance* (single EC2) for non-prod, or just route `0.0.0.0/0` to a single shared NAT.
- **Single NAT for prod.** AZ outage = entire app degrades. NAT-per-AZ is the production pattern.
- **NACLs as your primary firewall.** They're stateless and easy to misconfigure. SGs first; NACLs only for blanket subnet rules.
- **SG referencing CIDR instead of SG.** Causes maintenance pain when IPs change and breaks when ALB is in front (use SG-from-SG).
- **Overlapping CIDRs.** Two VPCs you want to peer both using `10.0.0.0/16`. Can't peer. Plan up front.
- **Public subnet without route to IGW.** Resources get public IPs that don't route anywhere.
- **Forgetting to enable DNS hostnames** before launching instances — they won't get public DNS names.
- **VPC endpoint without `--private-dns-enabled`** — your code still hits the public endpoint over NAT.
- **Endpoint policy too restrictive.** Easy to write an endpoint policy that breaks STS, killing all auth.
- **Cross-AZ data transfer charges.** $0.01/GB each way. A misplaced read replica or chatty microservice across AZs surprises people.
- **`0.0.0.0/0` egress allowed on SGs.** Default but worth tightening for sensitive workloads (data exfiltration vector).
- **IPv6 forgotten.** Modern VPCs should dual-stack. Costs nothing extra, future-proofs you.
- **Subnet CIDR too small.** A /28 has only 11 usable IPs (AWS reserves 5). EKS pods or large ASGs can chew through that.
- **DHCP options set is rarely the culprit** but worth knowing about for custom DNS / Directory Service setups.

---

## 🎯 Key Takeaways

- **VPC is regional; subnets are AZ-scoped.** Architect every tier across at least 2 AZs from day one; retrofitting HA into a single-AZ design is painful.
- **Security Groups (stateful, per-ENI) and NACLs (stateless, per-subnet) are independent layers.** Master SGs first — reference SGs from SGs to compose tiers without IP coupling.
- **NAT Gateway is one of AWS's most expensive idle services.** ~$32/mo per gateway plus per-GB processing. VPC Gateway endpoints (S3, DynamoDB) are free and should always be enabled.
- **VPC Endpoints + PrivateLink are the secure-by-default communication fabric.** Production workloads should not be reaching AWS APIs over public endpoints.
- **Flow Logs + Reachability Analyzer turn "the network is broken" from a black box into a debuggable system.** Set up Flow Logs the day you set up the VPC.

*← [prev](./02_iam_deep_dive.md) | [next →](./04_ec2_compute.md)*
