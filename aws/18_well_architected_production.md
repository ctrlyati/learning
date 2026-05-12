# 18 — Well-Architected & Production Patterns: Organizations, Landing Zones, DR, Scaling

> **Goal:** Pull everything together into the patterns that real AWS shops run — multi-account organizations, automated landing zones, disaster recovery strategies, and the lessons from scaling stories.

---

## 1. The Well-Architected Framework (WAF, no relation to the firewall)

AWS's distilled wisdom across thousands of customer engagements: 6 pillars + design principles + service-specific lenses.

### The 6 pillars
1. **Operational Excellence** — run and monitor systems, evolve procedures.
2. **Security** — protect data, systems, assets; manage permissions.
3. **Reliability** — recover from failures, handle changes, scale.
4. **Performance Efficiency** — use compute resources efficiently to meet requirements.
5. **Cost Optimization** — avoid unnecessary cost (Module 17).
6. **Sustainability** — minimize environmental impact (added 2021).

The WA Framework comes with a free **WA Tool** in the AWS console where you walk through ~50-100 questions per workload and get scored + remediation suggestions. Worth running annually on critical workloads — especially before launch.

### Design principles worth memorizing
- **Stop guessing capacity.** Use auto-scaling; pay for what you use.
- **Test systems at production scale.** With on-demand infra it's cheap.
- **Automate everything reproducible.**
- **Allow for evolutionary architectures.** Don't lock yourself into year-one decisions.
- **Drive architectures using data.** Metrics, logs, traces.
- **Improve through game days.** Practice failures.

---

## 2. Multi-Account Organizations

**Mental model:** A single AWS account is fine for learning, dangerous for production. As you scale, blast radius (one misconfigured IAM = whole account compromised) and management overhead force a multi-account model. AWS Organizations is the parent management surface.

### Why multi-account
- **Blast radius isolation**: prod break ≠ dev break.
- **Cost & billing separation**: consolidated billing with per-account budgets.
- **Compliance isolation**: PCI workload in its own account.
- **Quota isolation**: hitting service limits in dev doesn't kill prod.
- **IAM simplicity**: per-account guardrails > complex cross-team policies.

### Typical account structure
```
Root Organization (mgmt account)
├── Security OU
│   ├── log-archive       (centralized CloudTrail/Config logs)
│   └── audit             (Security Hub, GuardDuty, Inspector aggregator)
├── Infrastructure OU
│   ├── network           (Transit Gateway, shared VPCs)
│   └── tooling           (CI/CD pipelines, artifact registries)
├── Workloads OU
│   ├── prod
│   ├── staging
│   └── dev
└── Sandbox OU
    ├── per-developer accounts
```

The **management account** does billing + Organizations admin only. **Never run workloads in it.**

### Service Control Policies (SCPs)
Org-wide IAM guardrails. SCPs don't grant; they cap. Example: "no one (including root) can disable CloudTrail, leave the org, or use regions outside the approved list."

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "DenyDisableCloudTrail",
      "Effect": "Deny",
      "Action": ["cloudtrail:StopLogging","cloudtrail:DeleteTrail"],
      "Resource": "*"
    },
    {
      "Sid": "RestrictRegions",
      "Effect": "Deny",
      "NotAction": ["iam:*","s3:*","cloudfront:*","route53:*","support:*"],
      "Resource": "*",
      "Condition": {
        "StringNotEquals": { "aws:RequestedRegion": ["us-east-1","eu-west-1"] }
      }
    }
  ]
}
```

Attach SCPs to OUs to apply broadly. SCPs are the **strongest** blast-radius control AWS gives you.

---

## 3. Control Tower & Landing Zones

A **landing zone** is the baseline setup of an org: account vending machine, mandatory guardrails, log aggregation, identity federation, networking.

### AWS Control Tower
A managed landing zone. Sets up:
- Multi-account org + OUs.
- Account Factory (vend new accounts with a baseline).
- Mandatory and elective guardrails (SCPs + Config rules).
- Centralized log archive + audit account.
- IAM Identity Center for human SSO.

Control Tower is opinionated; if you need more flexibility, build with **AWS Organizations + StackSets + Customizations for Control Tower (CfCT)** or use **Account Factory for Terraform (AFT)**.

### Identity Center (formerly AWS SSO)
The recommended human-access path: federate from your IdP (Okta/AzureAD/Google Workspace), assign permission sets per account/group, users log in once at `https://<your>.awsapps.com/start`.

```bash
# Configure profile from Identity Center
aws configure sso
# Browser opens; pick account + role; CLI uses temporary creds
aws sso login --profile sandbox
```

---

## 4. Networking at scale: Transit Gateway and Shared VPCs

When you have many VPCs:

### Transit Gateway (TGW)
A regional hub. Attach each VPC, on-prem VPN, Direct Connect Gateway. Routes between any two attached networks. Transitive (unlike VPC peering). ~$36/mo per attachment + $0.02/GB processed.

**Use it when:** > ~5 VPCs, hybrid network, or you need transitive routing.

### Shared VPCs (RAM-shared)
Create a VPC in one account, share subnets with other accounts (AWS Resource Access Manager). All accounts launch resources into the same VPC — no peering, no TGW. Best for tightly-coupled teams.

### TGW Network Manager
Visualize and centrally manage TGWs + on-prem connectivity.

---

## 5. Disaster Recovery Strategies

Four standard tiers (AWS docs use these names; memorize them).

| Strategy | RPO | RTO | Cost | Complexity |
|---|---|---|---|---|
| **Backup & Restore** | hours | hours | $ | Low |
| **Pilot Light** | minutes | tens of minutes | $$ | Medium |
| **Warm Standby** | seconds-minutes | minutes | $$$ | High |
| **Multi-region Active/Active** | seconds | seconds | $$$$ | Highest |

### Backup & Restore
- All data backed up to another region (S3 cross-region replication, RDS cross-region snapshots, EBS snapshot copy).
- On failure: provision infra (IaC), restore data, switch DNS.
- Recovery in hours.

### Pilot Light
- Critical core (RDS replicas, EBS snapshots, key roles) lives in DR region, but the rest is "off" (no running compute).
- On failure: promote replicas, scale up compute, switch DNS.

### Warm Standby
- A scaled-down full stack runs in DR region.
- On failure: scale it up, switch DNS. Minutes.

### Active/Active
- Full prod in two regions, serving simultaneously. Route 53 latency-based + health checks.
- Needs careful data architecture (Aurora Global, DynamoDB Global Tables, multi-region replication design).
- Most expensive, most complex, fastest recovery.

### AWS Backup
Centralized backup service across EC2/EBS/RDS/DynamoDB/S3/EFS/FSx/Storage Gateway. Cross-region, cross-account, immutable Vault Lock. **The right place to centralize backups** vs per-service tooling.

```bash
aws backup put-backup-plan --backup-plan '{
  "BackupPlanName": "daily-prod",
  "Rules": [{
    "RuleName": "daily",
    "TargetBackupVaultName": "prod-vault",
    "ScheduleExpression": "cron(0 5 ? * * *)",
    "Lifecycle": { "DeleteAfterDays": 30, "MoveToColdStorageAfterDays": 7 },
    "CopyActions": [{
      "DestinationBackupVaultArn": "arn:aws:backup:eu-west-1:...:vault/dr-vault",
      "Lifecycle": { "DeleteAfterDays": 90 }
    }]
  }]
}'
```

---

## 6. Scaling Stories — what changes at each order of magnitude

### 0-10 RPS — early stage
- Single account, single region, single AZ for everything except DB (Multi-AZ).
- ECS Fargate or Lambda + Aurora Serverless v2 + S3 + CloudFront.
- Cost: $50-500/mo.

### 10-100 RPS — first product-market fit
- Multi-AZ everything, ALB + ASG / Fargate service auto-scaling.
- ElastiCache for hot reads.
- RDS Multi-AZ → consider Aurora.
- Cost: $500-5000/mo.

### 100-1000 RPS — meaningful traffic
- Multi-account (prod isolated).
- DynamoDB or sharded Postgres for high-write tables.
- CloudFront for all egress.
- SQS / EventBridge for async work.
- Cost: $5k-50k/mo.

### 1000-10000 RPS — serious operations
- Multi-region warm standby.
- Aurora Global / DynamoDB Global Tables for critical data.
- Service-to-service mesh (App Mesh / Service Connect).
- Dedicated FinOps function.
- Cost: $50k-500k/mo.

### 10000+ RPS — Hyperscale
- Multi-region active/active.
- Custom data sharding.
- Edge compute (Lambda@Edge, CloudFront Functions).
- AWS Enterprise Support + TAM.
- Cost: $500k+/mo.

### The Amazon Builder's Library is gold
Read these as you scale:
- **"Avoiding insurmountable queue backlogs"** — keep queues bounded.
- **"Reliability, constant work, and a good cup of coffee"** — design for constant work patterns to avoid retry storms.
- **"Caching challenges and strategies"** — cache invalidation, thundering herds.
- **"Avoiding fallback in distributed systems"** — fallback is often worse than failure.
- **"Workload isolation using shuffle sharding"** — limit blast radius algorithmically.

---

## 7. Operational Patterns That Pay Off

### Game days / Chaos engineering
Deliberately break things in production to verify your runbooks and recovery. **AWS Fault Injection Service (FIS)** is a managed chaos tool — kill instances, throttle EC2, drop network packets.

```bash
# Stop random EC2 instances
aws fis start-experiment --experiment-template-id $TEMPLATE
```

### Runbooks for every alarm
Each CloudWatch alarm → links to a runbook (SSM Documents, Confluence pages). On-call should never wonder "what do I do?"

### Pre-mortems & post-mortems
- Pre-mortem before a big launch: "imagine this fails — what went wrong?"
- Blameless post-mortem after incidents: timelines, root cause, action items with owners.

### Service health monitoring
Maintain a public status page (Statuspage, AWS Service Health Dashboard for AWS-side issues).

### Capacity planning
Capacity isn't infinite even on AWS. Some services have account-level limits that take days to raise (RDS storage, EC2 vCPUs, Network Load Balancers per region, Route 53 hosted zones). **Check limits before scaling events.**

### Tagging strategy
Enforce via SCP / Config rules:
- `project` — the system this is part of.
- `env` — dev/staging/prod.
- `owner` — team or email.
- `cost-center` — for chargeback.
- `data-classification` — public/internal/sensitive.

---

## 8. Practical: a production-grade reference architecture

```
                          ┌─────────────────────┐
                          │ Route 53 (latency + │
                          │  health checks)     │
                          └──────────┬──────────┘
                                     │
                ┌────────────────────┴────────────────────┐
                ▼                                         ▼
        ┌──────────────┐                          ┌──────────────┐
        │  us-east-1   │                          │  eu-west-1   │
        │              │                          │              │
        │  CloudFront  │                          │  CloudFront  │
        │      │       │                          │      │       │
        │     WAF      │                          │     WAF      │
        │      │       │                          │      │       │
        │  API Gateway │                          │  API Gateway │
        │      │       │                          │      │       │
        │   Lambda/    │                          │   Lambda/    │
        │   ECS Fargate│                          │   ECS Fargate│
        │      │       │                          │      │       │
        │  Aurora      │◄────── Global DB ────────►│  Aurora     │
        │  DynamoDB    │◄──── Global Tables ──────►│  DynamoDB   │
        │  S3 (CRR) ───┼──────────────────────────►│  S3 (CRR)   │
        └──────────────┘                          └──────────────┘
```

**Underneath, in every account:**
- CloudTrail multi-region → central audit account.
- GuardDuty, Security Hub, Inspector → central audit account.
- Config + SCP guardrails enforced by Control Tower.
- AWS Backup → cross-region vault.
- CI/CD account → OIDC roles per environment.
- Cost Anomaly Detection + Budgets per account.
- IAM Identity Center for all human access.

That's a senior-engineer's mental picture of "AWS done right."

---

## 9. Common Mistakes & Gotchas

- **Running workloads in the management account.** Never. The mgmt account has uplifted privileges (Organizations admin); a compromise = org-wide compromise.
- **Single account for everything.** Doesn't scale, blast radius huge.
- **No SCPs.** A misconfigured IAM in one account can be exploited globally. SCPs are the only real prevention.
- **DR strategy implicit.** "We have Multi-AZ" ≠ "we can survive region loss." Be explicit about RTO/RPO and test it.
- **Backups in the same account/region as primary data.** Ransomware deletes both. Cross-account + cross-region, with Vault Lock if available.
- **Identity Center not used; IAM users for humans.** Long-lived creds, no MFA enforcement, painful offboarding.
- **No game days.** Untested recovery procedures fail at the worst time.
- **No tags.** Cost management impossible; ownership unclear in incidents.
- **CloudTrail centralized but missing some accounts.** Account Factory must enforce.
- **Quota limits hit during a launch.** Check Service Quotas, raise proactively.
- **Multi-region without thought to data sovereignty.** GDPR-regulated EU data replicated to US = compliance breach.
- **Active-active without conflict resolution.** Last-writer-wins eats data. Aurora Global, DynamoDB Global Tables have specific semantics; understand them before relying.
- **Manual processes "just for now"** that become permanent. Automate the second time you do it.
- **No runbooks.** New on-call eng gets paged at 3 AM, panics.
- **WA reviews skipped.** It's free, takes a day, finds real issues.

---

## 🎯 Key Takeaways

- **Multi-account from the beginning.** A management account that does nothing but Organizations + billing, with workload accounts isolated by environment and team, with SCPs as guardrails — this is the single biggest architectural decision in AWS at scale.
- **Identity Center + OIDC + roles** eliminate long-lived credentials for humans and CI alike. Combined with SCPs, you get a defensible identity story without the burden of per-user IAM.
- **Be explicit about DR strategy.** Pick a tier (backup-and-restore through active-active), measure your actual RTO/RPO with game days, and use AWS Backup to operationalize it.
- **Read the Builder's Library and run Well-Architected reviews annually.** The patterns Amazon learned through pain are written down — most architectural failures are predicted in those papers.
- **Tagging + cost allocation + budgets + anomaly detection + monthly waste hunts** is the FinOps discipline. Combined with SPs and right-sizing, this is what turns AWS from a runaway expense into a managed line item.

---

You've now walked from "I have an AWS account" through every major service and through the patterns that make systems work in production. The remaining growth is **practice**: build, deploy, page yourself, fix it, repeat. Run game days. Read post-mortems. Lurk in the AWS Community Builders Slack, the r/aws subreddit, the Last Week in AWS newsletter. The technology will keep evolving; the principles in these 19 files won't.

Welcome to senior AWS engineering.

*← [prev](./17_cost_optimization.md) | [roadmap →](./00_roadmap.md)*
