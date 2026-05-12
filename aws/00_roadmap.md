# AWS Deep-Dive Course — Roadmap

> **Goal:** Take a working developer from "I have an AWS account" to "I can architect, build, secure, and operate production systems on AWS" — with cost, security, and operational excellence baked in.

This course is designed for **professional upskilling**: every concept is tied to a runnable command, an IaC snippet, or a production gotcha. You won't just learn what S3 is — you'll know why your `s3:GetObject` deny is overriding your bucket policy at 2am.

---

## Module Table

| # | Title | Focus Areas |
|---|-------|-------------|
| 01 | Account & IAM Basics, Console + CLI + SDKs | Root vs IAM, billing alerts, regions/AZs, AWS CLI v2 setup, SDK auth chain, MFA |
| 02 | IAM Deep Dive | Users, groups, roles, policies, STS, permission boundaries, policy evaluation logic, best practices |
| 03 | VPC & Networking | Subnets, route tables, IGW/NAT, SGs vs NACLs, VPC endpoints, peering, Transit Gateway |
| 04 | EC2 Compute | Instance families, AMIs, EBS volumes, key pairs, user data, ASG fundamentals, Spot |
| 05 | S3 Object Storage | Buckets, storage classes, versioning, lifecycle, presigned URLs, SSE-S3/KMS, replication |
| 06 | RDS & Aurora | Engines, Multi-AZ vs Read Replicas, parameter/option groups, backups, Aurora Serverless v2 |
| 07 | DynamoDB | Partition/sort keys, GSI/LSI, on-demand vs provisioned, single-table design, Streams, DAX |
| 08 | Lambda & Serverless | Invocation models, layers, cold starts, limits, event sources, SnapStart, Lambda URLs |
| 09 | API Gateway, AppSync, Lambda Integrations | REST vs HTTP API, WebSocket, AppSync GraphQL, auth, throttling, custom domains |
| 10 | SQS, SNS, EventBridge | Queues vs topics vs buses, FIFO, DLQs, fan-out, event-driven patterns, schemas |
| 11 | CloudFront & Route 53 | OAC, cache behaviors, signed URLs/cookies, Lambda@Edge, DNS routing policies, health checks |
| 12 | ECS, ECR, Fargate | Task defs, services, capacity providers, ALB integration, image scanning, Fargate vs EC2 launch |
| 13 | Observability | CloudWatch Logs/Metrics/Alarms, Logs Insights, X-Ray tracing, CloudTrail, Container Insights |
| 14 | Security & Compliance | KMS, Secrets Manager, GuardDuty, Security Hub, WAF, Shield, Inspector, Macie |
| 15 | Infrastructure as Code | CloudFormation, AWS CDK, Terraform — comparison, drift, state, when to use what |
| 16 | CI/CD on AWS | CodePipeline/Build/Deploy, GitHub Actions + OIDC, blue/green, canary, secrets in pipelines |
| 17 | Cost Optimization | Pricing models, Savings Plans vs RIs, Cost Explorer, AWS Budgets, common waste patterns |
| 18 | Well-Architected & Production Patterns | 6 pillars, Control Tower, landing zones, multi-account, DR strategies, scaling stories |

---

## Suggested Timelines

### Sprint pace — 3 weeks (1 module/day)
For someone with a deadline (interview, project, certification soon).
- **Week 1 (foundations):** Modules 01–06
- **Week 2 (core services):** Modules 07–12
- **Week 3 (production):** Modules 13–18

### Steady pace — 9 weeks (2 modules/week)
For working professionals adding AWS depth alongside a day job. Pair theory with weekend hands-on time in a sandbox account.

### Marathon pace — 18 weeks (1 module/week)
For a side-of-desk learning track. Spend extra time on labs and external links.

---

## How to Use This Course

1. **Read the module top-to-bottom** — concepts build on each other within a module.
2. **Run the commands.** Every `aws` CLI snippet and CDK/Terraform block is meant to be executed in a sandbox account. Reading without running is half the value.
3. **Tear down what you build.** AWS happily bills you for the EBS volume you forgot. The Cost Optimization module (17) drills this — but practice it from day one.
4. **Cross-reference IAM (Module 02) and VPC (Module 03)** as you go. They underpin everything else.
5. **Skim the "Common Mistakes & Gotchas" first** if you're senior — it's the highest-density section per module.

---

## Prerequisites & Setup

### You need
- A working laptop with **administrative access** (to install CLI tools).
- An **AWS account**. The free tier covers most of this course; budget < $20 if you're sloppy with EC2/NAT Gateway/Aurora.
- Comfort with the command line (bash or PowerShell), basic networking (TCP, DNS, HTTP), and at least one programming language (Python, Node, or Go preferred for SDK examples).

### Sandbox account checklist
1. **Create a fresh AWS account** — do **not** use your employer's prod account.
2. **Enable MFA on the root user.** Lock the credentials away.
3. **Create an IAM admin user** (we cover this properly in Module 02). Never use root for daily work.
4. **Set a billing alarm** at $20 (Module 01 walks through this).
5. **Install the tooling:**

```bash
# AWS CLI v2 — required
aws --version    # should report aws-cli/2.x

# Configure a named profile
aws configure --profile sandbox
# enter Access Key, Secret, default region (e.g. us-east-1), output (json)

# Verify
aws sts get-caller-identity --profile sandbox

# Optional but recommended
brew install awscli session-manager-plugin   # macOS
choco install awscli                         # Windows
# Node + AWS CDK (for IaC module)
npm install -g aws-cdk
cdk --version
# Terraform
brew install terraform     # or choco install terraform
```

6. **Pick a default region** and stick with it. `us-east-1` has the most services first but also the most outages; `us-west-2` is a popular alternative. EU readers: `eu-west-1` or `eu-central-1`.

---

## Core Mental Models

These six ideas explain 80% of AWS behavior. Internalize them before you memorize service names.

### 1. The Shared Responsibility Model
AWS is responsible for the security **of** the cloud (hardware, hypervisor, physical datacenters). You are responsible for security **in** the cloud (your IAM policies, your patched EC2, your S3 bucket policies, your encrypted data). Most breaches blamed on "AWS" are customer-side misconfigurations.

### 2. Everything is an API
The console is just a web app that calls the same APIs your CLI/SDK calls. Every action — `RunInstances`, `PutObject`, `CreateRole` — is an API call, logged in CloudTrail, and gated by IAM. When something is broken, ask: "what API call would this be, what's its IAM action, and did CloudTrail log it?"

### 3. IAM Denies by Default
With no policy attached, an identity can do **nothing**. Allows are additive. An explicit `Deny` anywhere in the evaluation chain (identity policy, resource policy, SCP, permission boundary, session policy) overrides any `Allow`. When access fails, the question is almost never "what's allowing me?" — it's "what's denying me, or what allow is missing?"

### 4. Eventually Consistent vs Strongly Consistent
S3 reads after writes are strongly consistent (since Dec 2020). DynamoDB reads default to eventually consistent (you can opt into strong reads at 2x cost). IAM changes propagate eventually. Cross-region replication is eventually consistent. Build with the assumption that "I just wrote it" doesn't mean "everyone can read it" — unless the docs explicitly say strong.

### 5. Regions are Independent
A region (`us-east-1`, `eu-west-1`) is a separate failure domain. Your IAM users are global; your VPC, EC2, S3 bucket, RDS instance are regional. Replication is **never automatic** — you configure it. A region outage doesn't (usually) affect another region. Design for the blast radius you need.

### 6. You Pay for What You Don't Turn Off
Every resource is metered until destroyed. An unattached EBS volume bills. An idle NAT Gateway bills (~$32/mo doing nothing). An idle Aurora cluster bills. A forgotten ELB bills. Free tier ends, snapshots persist, log groups grow. Treat infrastructure as a subscription, not a purchase.

---

## Curated External Resources

- **[AWS Documentation](https://docs.aws.amazon.com/)** — the source of truth. Service-specific developer guides are usually excellent.
- **[AWS Well-Architected Framework](https://docs.aws.amazon.com/wellarchitected/latest/framework/welcome.html)** — the official AWS opinion on how to build well. Covered in Module 18.
- **[AWS Workshops](https://workshops.aws/)** — free, self-paced, hands-on labs maintained by AWS.
- **[Last Week in AWS](https://www.lastweekinaws.com/)** (Corey Quinn) — newsletter and blog. Snarky, accurate, often the first to call out billing/feature absurdity.
- **[AWS Builders' Library](https://aws.amazon.com/builders-library/)** — papers by Amazon principal engineers on how they actually build at scale. Read "Avoiding insurmountable queue backlogs" and "Reliability, constant work, and a good cup of coffee."
- **[AWSGeek Diagrams](https://www.awsgeek.com/)** — Jerry Hargrove's visual service summaries. Great for revising before a certification or interview.

---

## Closing Note

By Module 18 you should be able to walk into any AWS-centric engineering conversation — design review, on-call escalation, cost review, interview loop — and engage at depth. The certifications (Solutions Architect Associate/Professional, Developer, SysOps) are achievable side-effects of this material, not the goal. The goal is **operating production systems with confidence**.

Build things. Break them. Read the bills. Welcome to AWS.

*[next → 01_account_and_iam.md](./01_account_and_iam.md)*
