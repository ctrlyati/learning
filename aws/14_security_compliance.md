# 14 — Security & Compliance: KMS, Secrets, GuardDuty, WAF, and the Defense Stack

> **Goal:** Build accounts that are secure by default — encryption everywhere, secrets managed, threats detected, and a runbook when something goes wrong.

---

## 1. The defense-in-depth stack

**Mental model:** AWS security isn't one service — it's a *stack of overlapping controls*. If one fails, another catches it. The stack:

```
Identity & access ────► IAM, Identity Center, SCPs (Modules 02, 18)
Network ─────────────► VPC, SGs, WAF, Shield (Module 03, here)
Data at rest ────────► KMS, S3 default encryption, EBS encryption
Data in transit ─────► ACM, TLS, VPC endpoints, PrivateLink
Secrets ─────────────► Secrets Manager, Parameter Store
Detection ───────────► GuardDuty, Security Hub, Inspector, Macie, Config
Audit ───────────────► CloudTrail, Config (Module 13, here)
Response ────────────► EventBridge → Lambda / SOAR, Detective
```

This module covers the data, secrets, detection, and response layers.

---

## 2. KMS — Key Management Service

KMS is the cryptographic backbone of AWS. Every encryption-at-rest feature you've seen (`--storage-encrypted`, S3 SSE-KMS) is KMS under the hood.

### Key types
- **AWS-managed keys** (`aws/<service>`): per-service, per-region, free. You can't change their policies. Fine for casual use.
- **Customer-managed keys (CMKs)**: you own them, you control the key policy, you can rotate manually or annually. $1/key/month + $0.03 per 10000 requests. **Use these for anything serious.**
- **AWS-owned keys**: used by AWS internally, invisible to you. Free, no control.
- **Imported key material**: bring your own, you control rotation. Niche.
- **External key store (XKS)**: key material in your HSM, KMS proxies. Compliance/sovereignty.

### Encrypt / decrypt flow — envelope encryption
KMS doesn't encrypt your gigabytes. It encrypts *data keys* you generate, which you then use to encrypt the data locally.

```python
kms = boto3.client("kms")
# 1. Generate a data key
r = kms.generate_data_key(KeyId="alias/myapp", KeySpec="AES_256")
plaintext_key   = r["Plaintext"]              # use to encrypt locally
encrypted_key   = r["CiphertextBlob"]         # store with the data

# 2. Encrypt locally
ct = AES_GCM_encrypt(plaintext_key, data)

# 3. Store (encrypted_key, ct)
# 4. To decrypt: kms.decrypt(CiphertextBlob=encrypted_key) → plaintext_key, then AES decrypt
```

For small data (≤4 KB), `kms.encrypt/decrypt` directly. For anything bigger, envelope encryption.

### Key Policy — IAM for keys
A KMS key has its own resource policy. **A key policy that doesn't allow your account's IAM is unusable from IAM.** Standard boilerplate:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "EnableIAM",
      "Effect": "Allow",
      "Principal": { "AWS": "arn:aws:iam::123456789012:root" },
      "Action": "kms:*",
      "Resource": "*"
    },
    {
      "Sid": "AllowAppUse",
      "Effect": "Allow",
      "Principal": { "AWS": "arn:aws:iam::123456789012:role/app" },
      "Action": ["kms:Encrypt","kms:Decrypt","kms:GenerateDataKey*","kms:DescribeKey"],
      "Resource": "*"
    }
  ]
}
```

Cross-account use → add the other account's role/account-root to the key policy AND grant on the IAM side.

### Rotation
Customer-managed keys support automatic yearly rotation (creates new key material, retains old for decryption). Free. **Always enable.**

```bash
aws kms enable-key-rotation --key-id $KEY_ID
```

### Aliases
Friendly names (`alias/orders-data`) that you reference instead of UUID key IDs. Aliases can be repointed — useful for key rotation patterns where you swap aliases between old and new keys.

---

## 3. Secrets Manager and Parameter Store

| | Secrets Manager | SSM Parameter Store |
|---|---|---|
| Cost | $0.40/secret/mo + $0.05/10k requests | Standard: free; Advanced: $0.05/parameter/mo |
| Auto-rotation | Built-in (Lambda-based) | None native |
| Size limit | 64 KB | Standard: 4 KB; Advanced: 8 KB |
| Versioning | Yes | Yes (history of 100) |
| Cross-account share | Yes (resource policy) | Limited |
| Use case | DB credentials, API keys, certs | Config, feature flags, low-sensitivity values |

### Secrets Manager — rotation
For RDS / Aurora, Secrets Manager has built-in rotation (a Lambda template that runs the password change on a schedule).

```bash
aws secretsmanager create-secret --name myapp/db \
  --secret-string '{"username":"app","password":"initial"}' \
  --kms-key-id alias/secrets-key

aws secretsmanager rotate-secret --secret-id myapp/db \
  --rotation-lambda-arn arn:aws:lambda:...:function:SecretsManagerRDSPostgreSQLRotationSingleUser \
  --rotation-rules AutomaticallyAfterDays=30
```

### Parameter Store
Cheap, tiered values for non-rotating config. Free for the first 10000 standard params.

```bash
aws ssm put-parameter --name /myapp/prod/feature-x --value "enabled" --type SecureString --key-id alias/secrets-key
```

### Caching in apps
Both services rate-limit. Wrap calls with a TTL cache (30s-5min). The official **AWS Parameters and Secrets Lambda Extension** does this automatically for Lambda.

---

## 4. GuardDuty — managed threat detection

Continuously analyzes CloudTrail, VPC Flow Logs, DNS logs, EKS audit logs, S3 data events, RDS logins, EBS volumes (for malware) — and flags suspicious behavior.

Findings include:
- Compromised credentials (e.g., access from Tor/anomalous geography).
- Crypto-mining EC2.
- Reconnaissance, brute force.
- Unusual API calls (e.g., user disables CloudTrail).
- Backdoored AMIs, suspicious DNS queries.

```bash
aws guardduty create-detector --enable
```

30-day free trial; after that $$ scales with CloudTrail volume + VPC traffic. Worth it on prod accounts.

**Forward findings to EventBridge → Slack/PagerDuty/SOAR**:
```json
{
  "source": ["aws.guardduty"],
  "detail-type": ["GuardDuty Finding"],
  "detail": { "severity": [{ "numeric": [">", 7.0] }] }
}
```

---

## 5. Security Hub — the aggregator

A single dashboard for security findings from GuardDuty, Inspector, Macie, IAM Access Analyzer, Config, and 3rd-party tools. Runs continuous compliance checks (AWS Foundational Security Best Practices, CIS Benchmark, PCI DSS).

```bash
aws securityhub enable-security-hub --enable-default-standards
```

Findings normalize to the **AWS Security Finding Format (ASFF)**, queryable, exportable, routable.

**Use Security Hub as your security single-pane-of-glass** — even if you use 3rd-party tools, integrate them with Security Hub for aggregation.

---

## 6. Inspector and Macie

### Inspector v2 — vulnerability management
Continuously scans EC2 instances, container images in ECR, and Lambda functions for CVEs. Uses the latest CISA KEV catalog.

```bash
aws inspector2 enable --resource-types EC2 ECR LAMBDA
```

### Macie — sensitive data discovery
Scans S3 for PII, credentials, financial data using ML. Costs per GB scanned. Use for compliance (GDPR, HIPAA) and uncovering "wait, that prod data ended up in this bucket?"

---

## 7. AWS WAF — web application firewall

Filters HTTP/S traffic to ALB / API Gateway / CloudFront / AppSync. Rule types:
- **Managed rule groups** (AWS- and vendor-provided): OWASP top 10, Linux/Unix, PHP, Wordpress, IP reputation, bot control.
- **Rate-based rules**: throttle by source IP (e.g., 1000 requests / 5 min).
- **Geo match, IP match, regex, size constraints**.

```bash
aws wafv2 create-web-acl --name myapp-waf --scope CLOUDFRONT --region us-east-1 \
  --default-action Allow={} \
  --rules '[
    {"Name":"core","Priority":1,
     "Statement":{"ManagedRuleGroupStatement":{"VendorName":"AWS","Name":"AWSManagedRulesCommonRuleSet"}},
     "OverrideAction":{"None":{}},"VisibilityConfig":{"SampledRequestsEnabled":true,"CloudWatchMetricsEnabled":true,"MetricName":"core"}}
  ]' \
  --visibility-config "SampledRequestsEnabled=true,CloudWatchMetricsEnabled=true,MetricName=myapp-waf"
```

### Bot Control & Captcha
Managed rule groups for sophisticated bot detection. Premium $$$.

### Shield Standard & Advanced
- **Standard**: free, auto-on, defends against common L3/L4 DDoS.
- **Advanced**: $3000/mo, sophisticated DDoS protections + DRT support + cost protections.

---

## 8. AWS Config — continuous compliance

Records the configuration history of every resource. Lets you write **rules** that check compliance ("all S3 buckets must have versioning enabled", "no security group allows 0.0.0.0/0 on port 22").

```bash
aws configservice put-config-rule --config-rule file://rule.json
# Use AWS-managed rules: s3-bucket-public-write-prohibited, rds-storage-encrypted, etc.
```

Costs $$ at scale. Use it for:
- Compliance reporting.
- Auto-remediation: Config rule + EventBridge → SSM Automation runbook.

### Conformance packs
Curated bundles of rules (PCI, HIPAA, NIST 800-53, Operational Best Practices).

---

## 9. IAM Access Analyzer (back to IAM)

Free. Two superpowers:
1. **External findings**: flags resources (S3 buckets, KMS keys, roles, secrets) shared outside your defined trust zone (zone of trust = your account or org). Catches "I made the bucket public by accident".
2. **Policy generation**: builds a least-privilege policy from a role's CloudTrail history. Magical for tightening overprivileged roles.

```bash
aws accessanalyzer create-analyzer --analyzer-name org-analyzer --type ORGANIZATION
```

Also: **Unused Access Analyzer** (newer) — finds unused IAM roles, users, permissions.

---

## 10. Practical: a secure-by-default account baseline

What every account should have configured (Module 18 covers automating this via Control Tower):

```typescript
// Pseudo-CDK for an "account guardrails" stack
new cloudtrail.Trail(this, "Trail", { isMultiRegionTrail: true, enableFileValidation: true });
new guardduty.CfnDetector(this, "GD", { enable: true });
new securityhub.CfnHub(this, "SH", { });
new inspector.CfnInspector2Enabler(this, "Insp", { accountIds: [accountId], resourceTypes: ["EC2","ECR","LAMBDA"] });
new accessanalyzer.CfnAnalyzer(this, "AA", { type: "ACCOUNT" });

// S3 account-level block public access
new s3.CfnAccountPublicAccessBlock(this, "BPA", {
  accountId, blockPublicAcls: true, blockPublicPolicy: true,
  ignorePublicAcls: true, restrictPublicBuckets: true,
});

// EBS default encryption
new ec2.CfnEC2Fleet(/* ... */); // not exact API; in console: EC2 → Settings → Always encrypt new EBS volumes
```

Also enable in console: **IMDSv2 required by default**, **default EBS encryption**, **default S3 SSE-S3 (or KMS)**.

---

## 11. Incident response

When GuardDuty fires "CompromisedCredentials":
1. **Don't panic; preserve.** Don't delete the role — you need it for forensics.
2. **Rotate.** Disable the access key, revoke role's active sessions:
   ```bash
   aws iam put-user-policy --user-name alice --policy-name DenyAll \
     --policy-document '{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"*","Resource":"*"}]}'
   ```
3. **Investigate.** CloudTrail Lake query: what API calls did the principal make recently?
4. **Contain.** Quarantine affected EC2 (Session Manager isolation SG), snapshot for forensics.
5. **Remediate.** Patch root cause: leaked key, missing MFA, vulnerable code path.
6. **Post-mortem.** Blameless.

**Detective** (AWS service) visualizes the relationship graph between findings, principals, resources — speeds investigation.

---

## 12. Common Mistakes & Gotchas

- **Using AWS-managed KMS keys for everything.** Free, easy — but you can't share cross-account, can't audit who used them, can't restrict access narrowly. Use CMKs for important data.
- **Forgetting `S3 Bucket Keys`** with SSE-KMS — ~99% reduction in KMS request charges.
- **Wide-open KMS key policy.** A key policy with `Principal: "*"` and a too-broad IAM condition = effectively public encryption (anyone in your account, or worse).
- **Secrets in env vars.** Show up in CloudTrail / logs / debug dumps. Always Secrets Manager / Parameter Store.
- **Rotating Secrets Manager secrets without rotating dependencies.** The DB password rotates; the app still has the old one cached. Cache TTL or graceful retry on auth failure.
- **GuardDuty findings ignored.** All that money to detect threats no one acts on. Route to PagerDuty for sev ≥ 7.
- **WAF logs to CloudWatch by default = $$$.** Send to S3 + Athena instead.
- **WAF managed rules in COUNT mode forever.** Easy to test, terrible if you forget to switch to BLOCK.
- **No WAF rate-based rule.** A misbehaving client or bot can bill you into oblivion before manual rate limit kicks in.
- **CloudTrail Data Events on all S3 buckets.** Huge bill — scope tightly.
- **Macie / Inspector on every account.** Costly at scale. Pilot on prod, evaluate.
- **Forgetting Shield Standard exists.** It does, it's free, it's on. Don't pay for Advanced unless you're a tier-1 target.
- **No KMS key rotation enabled.** Compliance auditors love this.
- **Cross-account key access without grants.** Sometimes you need `kms:CreateGrant` for services like EBS / RDS / S3 replication to work cross-account.
- **Forgetting `kms:ViaService` condition** to scope key use to specific services.

---

## 🎯 Key Takeaways

- **KMS + customer-managed keys + bucket keys + rotation** is the data-at-rest baseline for serious workloads. AWS-managed keys are fine for low-stakes, but not for compliance-grade data.
- **Secrets Manager for credentials with rotation; Parameter Store for config.** Use them as the credential chain endpoints, not env vars.
- **GuardDuty + Security Hub + Inspector + Access Analyzer** form the detection baseline. Enable all four; they're cheap relative to a breach.
- **WAF managed rules + rate-based rules + Shield Standard** front every internet-facing endpoint. The cost is small; the cost of a 1k RPS bot is not.
- **Compliance ≠ security, but it's a useful checklist.** Security Hub's AWS Foundational Security Best Practices standard gives a 200+ rule baseline for free — start there, then go beyond.

*← [prev](./13_observability.md) | [next →](./15_infrastructure_as_code.md)*
