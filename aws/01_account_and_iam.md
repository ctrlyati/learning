# 01 — AWS Account, Billing, Regions, and the Three Front Doors

> **Goal:** Get a sandbox AWS account set up safely, understand the global geography, and know which of the console / CLI / SDK to reach for in any given task.

---

## 1. The AWS Account — your billing boundary, your blast radius

**Mental model:** An AWS account is a *tenant* — a tenant in a multi-tenant data center. One bill, one set of IAM users, one isolated namespace for resources. Your S3 bucket name has to be globally unique because the bucket lives in someone's account, but the URL only resolves through the global S3 namespace.

In real organizations, you have **many** accounts — one for prod, one for staging, one per team, one for security tooling — held under an **AWS Organization**. We cover multi-account properly in Module 18. For the course, you use one personal sandbox.

### Creating the account (one-time)
1. Go to https://aws.amazon.com/ → "Create an AWS Account".
2. Email + a strong password + a credit card. Yes, the credit card is required even for free tier.
3. Pick the **Basic Support** plan (free). You can upgrade later.
4. AWS will verify by SMS or call. This usually completes in minutes.

### The root user is special — and dangerous
The email + password you signed up with is the **root user**. It can do anything, including close the account, change the email, and bypass any IAM policy. Treat it like the master key to a building.

**Immediately:**
- Enable **MFA** on the root user (Settings → Security credentials → MFA → assign a hardware key or authenticator app).
- Set a long, unique password and put it in a password manager.
- **Never** create access keys for root. If you already have, delete them.
- **Never** log in as root for daily work. Create an IAM admin user (Module 02) and use that.

### Verify it
```bash
# Once you've set up your IAM admin user and an AWS CLI profile:
aws sts get-caller-identity --profile sandbox
# {
#   "UserId": "AIDAEXAMPLE",
#   "Account": "123456789012",
#   "Arn": "arn:aws:iam::123456789012:user/admin"
# }
```
The 12-digit account number is your tenant ID. It appears in every ARN.

---

## 2. Billing — the system that makes you cry if you ignore it

Every API call that creates a resource starts a meter. Most meters are per-second or per-request, billed in arrears, surfaced in the next bill. The bill arrives **after** the damage is done. So the discipline is: budgets, alarms, and an instinct for what's expensive.

### Set a billing alarm (do this now)
Billing alarms live in CloudWatch in **us-east-1 specifically** (a historical quirk — billing data is only published there).

```bash
# Enable billing metrics first (one-time, console only):
#   Account → Billing Preferences → "Receive Billing Alerts" ✅

# Then create a $20 monthly alarm
aws cloudwatch put-metric-alarm \
  --alarm-name billing-monthly-20usd \
  --alarm-description "Alert when estimated charges exceed $20" \
  --metric-name EstimatedCharges \
  --namespace AWS/Billing \
  --statistic Maximum \
  --period 21600 \
  --evaluation-periods 1 \
  --threshold 20 \
  --comparison-operator GreaterThanThreshold \
  --dimensions Name=Currency,Value=USD \
  --alarm-actions arn:aws:sns:us-east-1:123456789012:billing-alerts \
  --region us-east-1
```

You need an SNS topic with your email subscribed for the `--alarm-actions` to actually notify you (Module 10 covers SNS — for now, create one in the console: SNS → Topics → Create, subscribe your email, click the confirmation link).

### AWS Budgets — better than CloudWatch billing alarms

AWS Budgets supports forecasted spend, RI/SP utilization, and credit-aware budgets.

```bash
aws budgets create-budget \
  --account-id 123456789012 \
  --budget '{
    "BudgetName": "monthly-sandbox-cap",
    "BudgetLimit": {"Amount": "20", "Unit": "USD"},
    "TimeUnit": "MONTHLY",
    "BudgetType": "COST"
  }' \
  --notifications-with-subscribers '[{
    "Notification": {
      "NotificationType": "ACTUAL",
      "ComparisonOperator": "GREATER_THAN",
      "Threshold": 80,
      "ThresholdType": "PERCENTAGE"
    },
    "Subscribers": [{"SubscriptionType": "EMAIL", "Address": "you@example.com"}]
  }]'
```

### Free Tier
- **12-month free**: things like 750 EC2 t2.micro hours/month, 5GB S3, 750 hours RDS db.t2.micro.
- **Always free**: things like 1M Lambda invocations/month, 25GB DynamoDB.
- **Trials**: 30-day free, like GuardDuty.

The free tier is generous but **doesn't cover everything**. NAT Gateway, Aurora Serverless v2 minimum capacity, KMS keys, and CloudWatch dashboards are not free. We flag this in every module.

---

## 3. Regions, AZs, Edge Locations — AWS geography

An **AWS Region** is a cluster of data centers in a geographic area (e.g., `us-east-1` in Northern Virginia, `eu-west-1` in Ireland, `ap-south-1` in Mumbai). At time of writing there are 30+ regions.

A region is composed of **Availability Zones (AZs)** — typically 3–6 per region. Each AZ is one or more physically distinct data centers with independent power, cooling, and networking. AZs within a region are connected by low-latency (<1ms RTT) private fiber. AZs are **the unit of fault isolation** within a region.

Then there are **Edge Locations** (~400+ globally) used by CloudFront, Route 53, Global Accelerator, and S3 Transfer Acceleration.

### Region naming
```
us-east-1   = N. Virginia (the granddaddy; cheapest; first to get new services; also most outage-prone)
us-east-2   = Ohio
us-west-2   = Oregon (popular alternative to us-east-1, often cheaper for some services)
eu-west-1   = Ireland
eu-central-1 = Frankfurt
ap-south-1  = Mumbai
ap-southeast-1 = Singapore
```

AZ names are scoped per-account — `us-east-1a` in your account may be a different physical AZ than `us-east-1a` in mine (AWS does this to balance load across AZs). To get the stable name, use the AZ ID (e.g., `use1-az1`):
```bash
aws ec2 describe-availability-zones --region us-east-1 \
  --query "AvailabilityZones[].[ZoneName,ZoneId]" --output table
```

### How to choose a region
1. **Latency to users.** Test with https://cloudping.info.
2. **Service availability.** New services launch in `us-east-1` first, then trickle out. Check the AWS Regional Services List.
3. **Compliance.** Data residency (GDPR, India's DPDPA) often forces a region.
4. **Cost.** Prices vary by region (often by 10–30%). `us-east-1` and `us-east-2` are typically cheapest.

### List enabled regions
```bash
aws ec2 describe-regions --query "Regions[].RegionName" --output table
```
Some newer regions (`me-south-1`, `af-south-1`, `eu-south-1`) are **opt-in** — you must enable them in Account Settings before using.

---

## 4. The Three Front Doors — Console, CLI, SDKs

Everything in AWS is an API. The console, CLI, and SDKs are all just clients hitting the same APIs over HTTPS, signed with **SigV4** using your credentials.

### a) The Console — for exploration and rare operations
Good for: poking at a service you've never used, looking at metrics graphs, reading the docs side-by-side, emergency interventions.

Bad for: anything you'll do twice, anything you need to reproduce, anything you need to audit.

### b) The AWS CLI v2 — for daily ops
```bash
# Configure a named profile
aws configure --profile sandbox
# AWS Access Key ID:     AKIA...
# AWS Secret Access Key: ****
# Default region name:   us-east-1
# Default output format: json

# Use it
aws s3 ls --profile sandbox
aws ec2 describe-instances --profile sandbox --output table
```

**Profile selection**: `--profile` flag, or `AWS_PROFILE=sandbox` env var. The env var is cleaner for scripts.

**Output formats**: `json` (default, machine-readable), `table` (human-readable), `text` (great for piping to `grep`/`awk`), `yaml` (for CloudFormation work).

**Filtering with `--query` (JMESPath)**:
```bash
aws ec2 describe-instances \
  --query "Reservations[].Instances[].[InstanceId,InstanceType,State.Name]" \
  --output table
```

**Pagination**: AWS APIs paginate. CLI v2 auto-paginates by default; for very large result sets use `--max-items` and `--page-size`.

### c) The SDKs — for application code

Each SDK speaks SigV4 and uses the **credential provider chain** in this order:
1. Explicit credentials passed to the client constructor.
2. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`).
3. `AWS_PROFILE` → `~/.aws/credentials` and `~/.aws/config`.
4. SSO cache (if `aws sso login`).
5. EC2 IMDS / ECS task role / Lambda execution role / EKS IRSA.

This is why your Lambda just *works* without credentials — it picks up its execution role from the runtime.

#### Python (boto3)
```python
import boto3
s3 = boto3.client("s3", region_name="us-east-1")
for b in s3.list_buckets()["Buckets"]:
    print(b["Name"])
```

#### Node.js (AWS SDK v3 — modular)
```javascript
import { S3Client, ListBucketsCommand } from "@aws-sdk/client-s3";
const s3 = new S3Client({ region: "us-east-1" });
const out = await s3.send(new ListBucketsCommand({}));
console.log(out.Buckets.map(b => b.Name));
```

#### Go (aws-sdk-go-v2)
```go
cfg, _ := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
s3c := s3.NewFromConfig(cfg)
out, _ := s3c.ListBuckets(ctx, &s3.ListBucketsInput{})
for _, b := range out.Buckets { fmt.Println(*b.Name) }
```

The SDK names and patterns are very consistent: `<ServiceClient>` and per-operation Inputs/Outputs.

---

## 5. AWS CloudShell — the zero-setup terminal

Click the `>_` icon in the top bar of the console. You get a free Linux shell running in your account, pre-authenticated with your console identity, with the CLI, Python, Node, and 1GB of persistent home directory storage. Excellent for "I just need to run one command and I don't want to set up creds on this laptop." Free tier.

---

## 6. Practical: bootstrap a sandbox account in 10 minutes

```bash
# 1. Verify identity
aws sts get-caller-identity

# 2. List your current region's AZs
aws ec2 describe-availability-zones --output table

# 3. Confirm free-tier eligible regions you can use
aws ec2 describe-regions --output table

# 4. Check current month's spend
aws ce get-cost-and-usage \
  --time-period Start=$(date -u +%Y-%m-01),End=$(date -u +%Y-%m-%d) \
  --granularity MONTHLY \
  --metrics UnblendedCost
# Cost Explorer must be enabled once via console; first activation takes ~24h to populate.

# 5. Tag every resource you create with a "project" tag — makes Cost Explorer usable later
aws ec2 create-tags --resources i-xxx --tags Key=project,Value=aws-course

# 6. Set up a default region so you don't repeat --region everywhere
export AWS_DEFAULT_REGION=us-east-1
export AWS_PROFILE=sandbox
```

---

## 7. Common Mistakes & Gotchas

- **Using the root user for daily work.** The single biggest beginner mistake. Create an IAM admin in Module 02 and lock root away.
- **Forgetting MFA on root.** A leaked root password without MFA = account takeover. AWS now nags you, but enforce it day one.
- **Billing alarms in the wrong region.** They *must* be in `us-east-1`. Putting them in `eu-west-1` silently does nothing.
- **No budget set.** A botched Lambda recursion or a misconfigured CloudFront cache can run up four-figure bills in hours. A $20 budget alarm gives you a chance to catch it.
- **Working in random regions accidentally.** The console remembers the last region you used. You'll spend an hour wondering where your EC2 went, then realize you switched to Ohio. Pin a region, and use `--region` explicitly in scripts.
- **Hard-coded access keys in code.** Never. Use the credential chain. For local dev, use named profiles. For CI, use OIDC-federated roles (Module 16). For workloads on AWS, use instance/task/execution roles.
- **Confusing AZ names across accounts.** `us-east-1a` in your account ≠ `us-east-1a` in your colleague's. Use AZ IDs (`use1-az1`) when comparing.
- **Old CLI v1.** If `aws --version` reports 1.x, upgrade. v2 has critical features like SSO support, auto-pagination, and the `--no-cli-pager` flag.
- **Region-disabled APIs.** Some new opt-in regions reject calls until you enable them. The error is `OptInRequired`. Fix in Account Settings.
- **Free tier hours are per *account*, not per resource.** Running two t2.micro instances simultaneously uses 1500 hours/month, doubling the free 750. The 751st hour bills.

---

## 🎯 Key Takeaways

- The **AWS account = a billing and IAM boundary**. In production, you'll have many accounts under an Organization (Module 18); for learning, one is enough — just secure root with MFA and never use it.
- **Set a budget alarm before doing anything else.** A $20 monthly budget catches almost any runaway sandbox mistake. Remember: billing alarms must live in `us-east-1`.
- **Regions are independent failure domains; AZs are independent failure domains within a region.** Anything you build that needs HA must span AZs; anything that needs disaster-recovery must span regions.
- **Everything is an API.** The console, CLI, SDKs, and your Terraform provider all call the same SigV4-signed REST endpoints. Mastery means thinking in terms of API calls, IAM actions, and CloudTrail events — not "where's the button?"
- **The SDK credential chain is your friend.** Code that uses the default chain (no hardcoded keys) works identically on your laptop (profile), in CI (OIDC role), and in Lambda (execution role). This is the foundation of secure AWS development.

*← [prev](./00_roadmap.md) | [next →](./02_iam_deep_dive.md)*
