# 05 — S3: Object Storage, Storage Classes, Versioning, and the Quiet Footguns

> **Goal:** Use S3 confidently for every object-storage need from static sites to data lakes — and never be the engineer who made the bucket public.

S3 is the oldest AWS service (2006) and arguably the most important: 11 nines of durability, infinite scale, $0.023/GB/mo. The default datastore for everything that isn't a database row.

---

## 1. Buckets and Objects — flat key space, not a filesystem

**Mental model:** S3 is a giant key→blob hash table. "Folders" are illusions created by `/` characters in keys. There is no `mkdir`. A bucket is a globally-unique namespace; an object is `(bucket, key, version)` → bytes (up to 5 TB each) + metadata.

A bucket name must be globally unique across **all AWS accounts in the world**. Choose something prefixed with your org.

### Create one and put something in it
```bash
BUCKET=acme-yati-aws-course-$(openssl rand -hex 4)

aws s3api create-bucket --bucket $BUCKET --region us-east-1
# Note: outside us-east-1, you must add --create-bucket-configuration LocationConstraint=<region>

# Block public access (do this FIRST, every time)
aws s3api put-public-access-block --bucket $BUCKET --public-access-block-configuration \
  BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true

# Upload
echo "hello s3" > hi.txt
aws s3 cp hi.txt s3://$BUCKET/greetings/hi.txt

# List
aws s3 ls s3://$BUCKET/ --recursive

# Read
aws s3 cp s3://$BUCKET/greetings/hi.txt -    # to stdout
```

### Two CLIs in one
- `aws s3 …` — high-level (cp/sync/mv/rm), great for daily use.
- `aws s3api …` — low-level (one API call per command), required for advanced settings.

### Bucket regions
A bucket lives in one region. `s3://bucket/key` doesn't say which — the SDK does a redirect on first access. To make this explicit (and avoid latency), set `region_name` in your SDK config to the bucket's region.

---

## 2. The Access Story — five gatekeepers

S3 access is decided by an evaluation of *all* of:
1. **Block Public Access** (account-level + bucket-level) — overrides everything else; if set, public access is denied even if a policy allows it.
2. **IAM identity policy** (the caller's permissions).
3. **Bucket policy** (resource-based).
4. **Bucket ACLs / Object ACLs** (legacy; AWS now recommends disabling ACLs entirely with "Bucket Owner Enforced").
5. **VPC Endpoint policy** (if accessed via a Gateway endpoint).

### Block Public Access — the firebreak
Every new bucket since 2018 ships with BPA on. **Leave it on.** Almost no production bucket should be truly public; serve content through CloudFront with OAC instead (Module 11).

### Bucket policy example — grant CloudFront OAC access
```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "Service": "cloudfront.amazonaws.com" },
    "Action": "s3:GetObject",
    "Resource": "arn:aws:s3:::acme-site/*",
    "Condition": {
      "StringEquals": {
        "AWS:SourceArn": "arn:aws:cloudfront::123456789012:distribution/E2XXXXX"
      }
    }
  }]
}
```

---

## 3. Storage Classes — paying for what you actually need

S3 has 8+ storage classes with different cost/latency profiles. Pick wrong and you either overpay or hit retrieval cliffs.

| Class | $/GB/mo (us-east-1) | Retrieval | First-byte latency | Min duration | Use case |
|---|---|---|---|---|---|
| **Standard** | $0.023 | Free | ms | none | Hot data, daily access |
| **Intelligent-Tiering** | tiered automatically | Free | ms (frequent), 12hr (deep archive) | none | Unknown/changing access patterns — set-and-forget |
| **Standard-IA** | $0.0125 | $0.01/GB | ms | 30 days | Infrequent but immediate access |
| **One Zone-IA** | $0.01 | $0.01/GB | ms | 30 days | Re-creatable IA data |
| **Glacier Instant Retrieval** | $0.004 | $0.03/GB | ms | 90 days | Quarterly access |
| **Glacier Flexible Retrieval** | $0.0036 | $0.01/GB + min/hr retrieval delay | 1 min – 12 hr | 90 days | Backups |
| **Glacier Deep Archive** | $0.00099 | $0.02/GB + 12-48 hr | 12-48 hr | 180 days | Compliance archive |

**The minimum duration matters.** Move an object to Glacier Deep Archive and delete it after a week → you still pay 180 days of storage. Treat lifecycle transitions as commitments.

### Intelligent-Tiering — usually the right default
A small monitoring fee ($0.0025 per 1000 objects/mo) lets S3 automatically move objects between Frequent / Infrequent / Archive Instant / Archive / Deep Archive tiers based on access pattern. Pays for itself on any non-trivial workload with mixed access.

```bash
aws s3 cp big.parquet s3://$BUCKET/data/ --storage-class INTELLIGENT_TIERING
```

---

## 4. Versioning, Lifecycle, Replication, Object Lock

### Versioning
Off by default. Once enabled, every PUT creates a new version; DELETE creates a "delete marker" (the object isn't gone — set `?versionId=`). One-way: you can't un-enable, only suspend.

```bash
aws s3api put-bucket-versioning --bucket $BUCKET --versioning-configuration Status=Enabled
```

Versioning is the **first line of defense** against accidental deletes and ransomware. Combine with **MFA Delete** for high-stakes buckets.

### Lifecycle rules
Automate transitions and expirations.

```json
{
  "Rules": [
    {
      "ID": "archive-logs",
      "Status": "Enabled",
      "Filter": { "Prefix": "logs/" },
      "Transitions": [
        { "Days": 30,  "StorageClass": "STANDARD_IA" },
        { "Days": 90,  "StorageClass": "GLACIER" }
      ],
      "Expiration": { "Days": 365 },
      "NoncurrentVersionExpiration": { "NoncurrentDays": 30 },
      "AbortIncompleteMultipartUpload": { "DaysAfterInitiation": 7 }
    }
  ]
}
```
```bash
aws s3api put-bucket-lifecycle-configuration --bucket $BUCKET --lifecycle-configuration file://lifecycle.json
```

**The `AbortIncompleteMultipartUpload` rule is mandatory.** Failed multipart uploads accumulate as invisible storage you keep paying for. Anecdote: a customer was billed $40k/yr for orphaned multipart parts they didn't know existed.

### Replication
- **CRR** (Cross-Region Replication): for DR.
- **SRR** (Same-Region Replication): for compliance separation, log aggregation.
- **Replication Time Control (RTC)**: 15-min SLA + metrics, ~$0.015/GB.

```bash
# Both source and destination must have versioning enabled
aws s3api put-bucket-replication --bucket $BUCKET --replication-configuration file://rep.json
```

### Object Lock — WORM
Write-Once-Read-Many for compliance (SEC 17a-4, HIPAA). Two modes:
- **Governance** — admins with `BypassGovernanceRetention` can override.
- **Compliance** — *no one* can override, including root. Use carefully — you can rack up storage you literally cannot delete.

Object Lock must be enabled at bucket creation time.

---

## 5. Encryption

Every object is encrypted at rest. Choices:
- **SSE-S3** (default since 2023): AES-256, AWS manages keys, free.
- **SSE-KMS**: Encrypted with a KMS key. Auditable in CloudTrail, lets you enforce key policy. Costs $0.03 per 10,000 requests against the KMS key.
- **SSE-C**: You supply the key on every request. Rare.
- **DSSE-KMS**: Double encryption for compliance regimes.

```bash
# Force KMS encryption on every PUT
aws s3api put-bucket-encryption --bucket $BUCKET --server-side-encryption-configuration '{
  "Rules": [{
    "ApplyServerSideEncryptionByDefault": {
      "SSEAlgorithm": "aws:kms",
      "KMSMasterKeyID": "arn:aws:kms:us-east-1:123456789012:key/xxx"
    },
    "BucketKeyEnabled": true
  }]
}'
```

**Always enable Bucket Keys** with SSE-KMS — drops KMS request costs ~99%.

### Encryption in transit
Always TLS. Block plaintext with a bucket policy `aws:SecureTransport: false → Deny`.

---

## 6. Practical: Pre-signed URLs and Multipart Uploads

### Pre-signed URLs
A short-lived signed URL that lets a holder GET or PUT an object without AWS credentials. Pattern: backend signs, frontend uploads directly to S3, bandwidth bypasses your app servers.

```python
import boto3
s3 = boto3.client("s3", region_name="us-east-1")
url = s3.generate_presigned_url(
    "put_object",
    Params={"Bucket": "acme-uploads", "Key": "user-123/avatar.png", "ContentType": "image/png"},
    ExpiresIn=900,  # 15 min
)
# Front-end does: PUT <url>  with binary body
```

For >5MB uploads, use **pre-signed multipart**:
```python
upload = s3.create_multipart_upload(Bucket="acme-uploads", Key="big.zip")
# return upload["UploadId"]; pre-sign each PartNumber for upload_part
```

### Multipart from CLI (automatic)
`aws s3 cp` automatically multiparts files >8MB into 8MB chunks, in parallel. You typically don't have to think about it.

### Transfer Acceleration
For large uploads from far-flung clients, enable Transfer Acceleration to route through the nearest CloudFront edge:
```bash
aws s3api put-bucket-accelerate-configuration --bucket $BUCKET --accelerate-configuration Status=Enabled
# then use https://$BUCKET.s3-accelerate.amazonaws.com
```
Costs $0.04/GB on top of normal egress. Worth it for global users uploading multi-GB files.

---

## 7. S3 as a Data Lake Foundation

- **Partitioned layouts**: `s3://lake/events/year=2026/month=05/day=11/file.parquet`. Athena/Glue/EMR push down partition filters.
- **Parquet/ORC** columnar formats: 10-100x cheaper scans than JSON/CSV.
- **S3 Select / Glacier Select**: server-side filtering on CSV/JSON/Parquet — pay for less data transferred.
- **Athena** queries S3 directly. Pricing: $5/TB scanned. Compress + columnar + partition = drastic savings.

---

## 8. Common Mistakes & Gotchas

- **Public buckets.** The classic data-breach headline. Always Block Public Access. Serve through CloudFront with OAC.
- **Wide-open bucket policy** like `Principal: "*"` without conditions.
- **ACLs left on.** Set "Bucket owner enforced" object ownership to disable ACLs and avoid the "uploaded by another account, owned by them, you can't read it" trap.
- **Forgot to abort incomplete multipart uploads.** Silent cost. Add a lifecycle rule.
- **Versioning enabled, no lifecycle for old versions.** Costs balloon. Add `NoncurrentVersionExpiration`.
- **Lifecycle to Glacier for files < 128KB.** Below this size, transitions can cost more than they save (~$0.05 per 1000 transitions).
- **Cross-region read in app code.** Reading `eu-west-1` bucket from `us-east-1` code = inter-region transfer at $0.02/GB and high latency. Replicate, or put compute next to data.
- **SSE-KMS without bucket keys** — KMS request charges dominate.
- **Logs to the same bucket they're logging.** Recursive write loop. Use a separate logs bucket.
- **Strong consistency assumed for *List* across regions** — S3 read-after-write is strong; replication is eventual.
- **403 on cross-account write** — uploader writes object owned by themselves; bucket owner can't read. Fix with `ObjectOwnership = BucketOwnerEnforced`.
- **Pre-signed URL leaks.** A URL valid for 7 days, posted on Slack/email, granting PUT/GET — it's a credential. Keep TTLs short.
- **Glacier retrieval surprise.** Cheap storage, expensive retrievals — both $/GB and time. Model your retrieval needs before archiving.
- **Bucket naming**: `.` in bucket names breaks TLS wildcards (`*.s3.amazonaws.com`). Use hyphens.
- **`s3:ListBucket` vs `s3:GetObject`** — separate actions, separate ARN scopes. Both needed for `aws s3 ls && aws s3 cp`.
- **Request rate cliffs**: a single prefix can sustain 3500 PUT/s and 5500 GET/s. Beyond that, partition by prefix.

---

## 🎯 Key Takeaways

- **Block Public Access at the account level**, and rely on bucket policies + CloudFront OAC for distribution. Public buckets are now a known antipattern, not a tool.
- **Intelligent-Tiering is the right default** for unknown access patterns. The $0.0025/1000 objects monitoring fee is trivial vs the savings on a real workload.
- **Lifecycle is mandatory**: noncurrent version expiration, multipart abort, and tiered transitions. Without it, your bill grows monotonically forever.
- **Enable Bucket Keys with SSE-KMS** every time. The 99% drop in KMS request costs is the difference between $50/mo and $5000/mo on a chatty bucket.
- **S3 is a hash table, not a filesystem.** Design key schemes for partitioning (data lake), uniqueness (idempotency), and request distribution from day one — retrofitting hurts.

*← [prev](./04_ec2_compute.md) | [next →](./06_rds_aurora.md)*
