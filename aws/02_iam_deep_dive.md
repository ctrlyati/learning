# 02 — IAM Deep Dive: Identity, Authorization, and the Trust Web

> **Goal:** Understand the IAM model deeply enough to write least-privilege policies, debug "Access Denied" errors in under a minute, and design role-based access for real workloads.

IAM is **the single most important AWS service**. Get it wrong and you'll either be breached or constantly fighting access denials. Most senior AWS engineers I've worked with say the same thing: "IAM is the API I'm most often debugging."

---

## 1. The IAM Model — Principals, Actions, Resources, Conditions

**Mental model:** Every API call to AWS is a question: *"Can this **principal** perform this **action** on this **resource** under these **conditions**?"* IAM is the engine that answers it.

The four-tuple `(principal, action, resource, condition)` is the entire mental model. Everything else is plumbing.

- **Principal**: who is asking. An IAM user, an IAM role (assumed by something), the root user, or an AWS service.
- **Action**: what API call. Format: `service:Operation`, e.g., `s3:GetObject`, `ec2:RunInstances`, `iam:PassRole`. There are ~14,000 of them.
- **Resource**: what ARN. e.g., `arn:aws:s3:::my-bucket/*`. Some APIs are unscoped (`*`-only).
- **Condition**: optional constraints. Source IP, MFA presence, requested encryption, tags on resource or principal, time of day.

### Try it
```bash
# What can I currently do? (Tells you who you are.)
aws sts get-caller-identity

# Simulate a specific call — IAM's "what if" engine, free and indispensable
aws iam simulate-principal-policy \
  --policy-source-arn arn:aws:iam::123456789012:user/alice \
  --action-names s3:GetObject \
  --resource-arns arn:aws:s3:::my-bucket/file.txt
# Returns "allowed" or "implicitDeny" or "explicitDeny", with which statement matched
```

---

## 2. Identities — Users, Groups, and Roles

### IAM Users
A long-lived identity with a password (for console) and/or access keys (for CLI/SDK). One human = one user. Used to be the only way to authenticate; now considered an anti-pattern for most workloads.

```bash
aws iam create-user --user-name alice
aws iam create-access-key --user-name alice    # the secret is shown ONCE. save it.
aws iam attach-user-policy --user-name alice \
  --policy-arn arn:aws:iam::aws:policy/ReadOnlyAccess
```

**Anti-pattern alert.** AWS now recommends:
- For human users → **IAM Identity Center (formerly SSO)** with federated identities, not IAM users.
- For workloads → **IAM Roles** (no long-lived keys at all).

### IAM Groups
A bag of users with shared policies. No identity of its own — you can't "log in as a group." Purely a policy-attachment convenience.

```bash
aws iam create-group --group-name developers
aws iam attach-group-policy --group-name developers \
  --policy-arn arn:aws:iam::aws:policy/PowerUserAccess
aws iam add-user-to-group --group-name developers --user-name alice
```

### IAM Roles — the most important identity type
A role is an identity *with no permanent credentials*. To use it, a **principal** (a user, a service, an external identity) calls `sts:AssumeRole` and gets back a temporary credential set (access key, secret, **session token**) that expires (15 min – 12 hr).

Roles have two policy types:
- **Trust policy** ("AssumeRolePolicyDocument") — who *can* assume the role.
- **Permission policies** — what the role *can do* once assumed.

```bash
# Trust policy: let EC2 instances assume this role
cat > trust.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "Service": "ec2.amazonaws.com" },
    "Action": "sts:AssumeRole"
  }]
}
EOF

aws iam create-role --role-name ec2-s3-reader --assume-role-policy-document file://trust.json
aws iam attach-role-policy --role-name ec2-s3-reader \
  --policy-arn arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess

# Create an instance profile (the wrapper EC2 uses to attach the role)
aws iam create-instance-profile --instance-profile-name ec2-s3-reader
aws iam add-role-to-instance-profile \
  --instance-profile-name ec2-s3-reader --role-name ec2-s3-reader
```

Now any EC2 instance launched with `--iam-instance-profile Name=ec2-s3-reader` can read S3 with zero credentials in the AMI.

### STS — the credential vending machine

`AssumeRole`, `AssumeRoleWithSAML`, `AssumeRoleWithWebIdentity`, `GetFederationToken`, `GetSessionToken`. The last is rare; the first three are everything.

```bash
# A user explicitly assumes a role:
aws sts assume-role \
  --role-arn arn:aws:iam::123456789012:role/admin \
  --role-session-name alice-admin-session \
  --duration-seconds 3600
# returns AccessKeyId / SecretAccessKey / SessionToken / Expiration
```

You typically don't do this manually. Configure it in `~/.aws/config`:

```ini
[profile sandbox]
sso_session = my-sso
sso_account_id = 123456789012
sso_role_name = AdministratorAccess
region = us-east-1

[profile prod-readonly]
source_profile = sandbox
role_arn = arn:aws:iam::999999999999:role/ReadOnly
mfa_serial = arn:aws:iam::123456789012:mfa/alice
```
Now `aws --profile prod-readonly s3 ls` auto-assumes the cross-account role, prompting for MFA.

---

## 3. Policies — the Language of Permissions

A policy is a JSON document with one or more **statements**. Each statement has an `Effect` (Allow/Deny), one or more `Action`s, one or more `Resource`s, and optional `Condition`s.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ReadProdLogs",
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:ListBucket"],
      "Resource": [
        "arn:aws:s3:::prod-logs",
        "arn:aws:s3:::prod-logs/*"
      ],
      "Condition": {
        "IpAddress": {"aws:SourceIp": "203.0.113.0/24"},
        "Bool":      {"aws:MultiFactorAuthPresent": "true"}
      }
    }
  ]
}
```

### Policy types — there are six and they all interact

| Type | Attached to | Purpose |
|---|---|---|
| **Identity-based** | User, group, role | What this identity can do |
| **Resource-based** | S3 bucket, KMS key, SQS queue, etc. | Who can access this resource |
| **Permissions boundary** | User or role | Max permissions the identity can ever have (a ceiling) |
| **SCP** (Service Control Policy) | OU/account in an Org | Account-wide ceiling on all principals |
| **Session policy** | Passed to `AssumeRole` | Reduce permissions for one session |
| **ACL** (legacy) | S3, etc. | Avoid — use bucket policies |

### The evaluation logic (memorize this)

For a request, IAM walks through:
1. If any **explicit Deny** matches anywhere → **DENY**. End of story.
2. Apply **SCPs** (if in an Org). If no Allow → **DENY**.
3. Apply **resource-based policy**. Allow here can grant access even without an identity-based allow (key trick for cross-account).
4. Apply **identity-based policy** + **permissions boundary** intersection.
5. Apply **session policy**.
6. If no Allow anywhere applicable → **DENY** (default deny).

**The single most useful rule:** *Explicit Deny always wins.* When access fails unexpectedly, look for a Deny statement (in identity policy, resource policy, SCP, or boundary) before assuming a missing Allow.

### AWS-managed vs customer-managed vs inline

- **AWS-managed** (`arn:aws:iam::aws:policy/...`) — curated by AWS. Convenient for getting started but often too broad. `AdministratorAccess`, `ReadOnlyAccess`, `PowerUserAccess`.
- **Customer-managed** — your own reusable policies. Versioned, attachable to many identities. **Prefer these.**
- **Inline** — embedded in a single user/role. Hard to audit. Avoid for anything you'll attach to >1 identity.

```bash
aws iam create-policy --policy-name S3ReadProdLogs \
  --policy-document file://policy.json
# Attach to a role:
aws iam attach-role-policy --role-name dev-role \
  --policy-arn arn:aws:iam::123456789012:policy/S3ReadProdLogs
```

---

## 4. Practical: Cross-account access done right

**Scenario:** Your data team in account `B` needs to read a specific S3 bucket in account `A`. They have an existing role they already use.

### Step 1: In account A (the bucket owner)
Attach a **bucket policy** allowing account B's role:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "AWS": "arn:aws:iam::222222222222:role/data-team" },
    "Action": ["s3:GetObject", "s3:ListBucket"],
    "Resource": [
      "arn:aws:s3:::shared-data",
      "arn:aws:s3:::shared-data/*"
    ]
  }]
}
```

### Step 2: In account B
The `data-team` role's identity policy must also allow:

```json
{
  "Effect": "Allow",
  "Action": ["s3:GetObject", "s3:ListBucket"],
  "Resource": ["arn:aws:s3:::shared-data", "arn:aws:s3:::shared-data/*"]
}
```

Both sides must allow. (This is unique to cross-account — within the same account, either side is enough.)

### Step 3: If using KMS (almost always)
The KMS key encrypting the bucket must also have a key policy that allows the cross-account role to `Decrypt`. KMS is famously a third gatekeeper that surprises people in cross-account flows.

---

## 5. Best Practices — what senior engineers actually do

- **No IAM users for humans.** Use **AWS IAM Identity Center** (SSO) backed by Okta/AzureAD/Google Workspace. Humans get SSO logins; the CLI uses `aws sso login`.
- **No IAM users for workloads.** Use roles. EC2 → instance profile. ECS/Fargate → task role. Lambda → execution role. EKS pods → IRSA. On-prem/CI → OIDC federation (Module 16).
- **Permissions boundaries for delegation.** When dev teams need to create roles, give them a boundary so their roles can't exceed your guardrails.
- **Use IAM Access Analyzer** (free) to detect resources shared outside your trust zone and to generate **least-privilege policies from CloudTrail history**.
- **Tag-based access (ABAC).** Allow `ec2:StartInstances` only if `aws:PrincipalTag/team` matches `aws:ResourceTag/team`. This scales without writing per-team policies.
- **MFA everywhere.** Condition policies on `aws:MultiFactorAuthPresent: true` for any sensitive operation.
- **Session duration**: short by default (1 hour). Increase only when needed.
- **Rotate keys** if you absolutely must have them. IAM has a "credential report" CSV that shows last-used and age:
  ```bash
  aws iam generate-credential-report
  aws iam get-credential-report --query 'Content' --output text | base64 -d
  ```
- **`iam:PassRole` is the privilege escalation primitive.** Any service that lets you specify a role you create (Lambda, EC2, ECS) requires `iam:PassRole` to pass an existing role. Grant it narrowly. A user who can `lambda:CreateFunction` + `iam:PassRole *` is effectively admin.

---

## 6. Debugging Access Denied — the workflow

```
$ aws s3 cp ./file s3://bucket/key
An error occurred (AccessDenied) when calling the PutObject operation: Access Denied
```

1. **Read the error.** Newer errors include the user/role ARN and sometimes which statement denied. If not, set `--debug` and look at the response body.
2. **Run the IAM simulator.**
   ```bash
   aws iam simulate-principal-policy \
     --policy-source-arn arn:aws:iam::123456789012:role/myrole \
     --action-names s3:PutObject \
     --resource-arns arn:aws:s3:::bucket/key
   ```
3. **Check CloudTrail.** The failed call is logged with `errorCode: AccessDenied` and (recently) the **reason** field tells you *which policy denied*.
4. **Walk the policy stack:** identity policies → SCPs → permissions boundary → bucket policy → KMS key policy → session policy.
5. **Don't forget `iam:PassRole`** for any service that consumes a role.
6. **VPC endpoints** with restrictive endpoint policies are a common silent denier (Module 03).

---

## 7. Common Mistakes & Gotchas

- **`"Action": "*"` "just to make it work."** This is the policy version of `chmod 777`. Audit and remove.
- **Forgetting `iam:PassRole`.** Half of "Lambda won't deploy" tickets in the wild.
- **Trusting wildcards in trust policies.** `"Principal": "*"` on a role means anyone on the internet can assume it given the role ARN. Use specific account IDs.
- **Confusing `Resource: "*"` with bucket-level vs object-level.** S3 actions need both `arn:aws:s3:::bucket` (ListBucket) and `arn:aws:s3:::bucket/*` (GetObject/PutObject).
- **Resource policies and identity policies being out of sync.** Cross-account flows need *both*.
- **KMS key policy forgotten.** Encrypted S3 bucket + cross-account access fails until the KMS key allows the consumer.
- **IAM is eventually consistent.** After creating a role, `AssumeRole` may fail for a few seconds. Add retries or sleeps in deploy scripts.
- **Inline policies that drift.** Inline policies on a role are invisible if you only audit attached managed policies. Use Access Analyzer or `aws iam list-role-policies` to find them.
- **Wildcard `aws:PrincipalAccount` conditions** are not the same as restricting the principal — they only constrain who. Always start from `Principal` first.
- **MFA-protected APIs** check `aws:MultiFactorAuthPresent: true`. SDKs don't add MFA automatically; you must explicitly call `GetSessionToken` with `--serial-number` and `--token-code`, or assume a role with MFA.
- **Permissions boundaries are not "permissions"** — they're a cap. Attaching a boundary doesn't grant anything; identity policies still must allow the action.

---

## 🎯 Key Takeaways

- **IAM is a four-tuple authorization engine:** principal × action × resource × condition. Internalize this and every policy puzzle becomes a join.
- **Explicit Deny always wins, default is Deny.** When debugging, hunt for the deniers (SCPs, bucket policies, KMS, boundaries) before suspecting a missing Allow.
- **Roles, not users.** Long-lived access keys are a liability. Every workload should run under a role; every human should federate through Identity Center.
- **`iam:PassRole` is the keystone of privilege escalation.** Anyone who can deploy compute + pass arbitrary roles is effectively admin. Scope `PassRole` resources tightly.
- **Use the IAM Simulator and Access Analyzer.** The simulator answers "would this work?" before you deploy; Access Analyzer generates least-privilege policies from real CloudTrail usage. Both are free and underused.

*← [prev](./01_account_and_iam.md) | [next →](./03_vpc_networking.md)*
