# 15 — Infrastructure as Code: CloudFormation, CDK, Terraform

> **Goal:** Pick the right IaC tool for your team, write maintainable templates/code, and understand drift, state, and the gotchas that bite scaled-up IaC.

---

## 1. Why IaC

**Mental model:** Click-ops doesn't scale — across environments, teams, time. IaC turns infrastructure into versioned code: peer-reviewed, reproducible, auditable.

Three dominant choices on AWS:
- **CloudFormation (CFN)** — AWS's native templating service. YAML/JSON.
- **AWS CDK** — high-level imperative wrapper over CFN. TypeScript/Python/Java/.NET/Go.
- **Terraform** — HashiCorp's multi-cloud IaC. HCL.

Plus Pulumi (CDK-like with state model), SAM (CFN preset for serverless), Serverless Framework (legacy).

---

## 2. CloudFormation — the foundation

A CFN template describes resources as a YAML doc. AWS materializes it as a **stack** — a managed set of resources with a single lifecycle.

```yaml
AWSTemplateFormatVersion: "2010-09-09"
Parameters:
  EnvName: { Type: String, Default: dev }
Resources:
  Bucket:
    Type: AWS::S3::Bucket
    Properties:
      BucketName: !Sub "myapp-${EnvName}-${AWS::AccountId}"
      VersioningConfiguration: { Status: Enabled }
      PublicAccessBlockConfiguration:
        BlockPublicAcls: true
        IgnorePublicAcls: true
        BlockPublicPolicy: true
        RestrictPublicBuckets: true
Outputs:
  BucketName:
    Value: !Ref Bucket
    Export: { Name: !Sub "${EnvName}-bucket-name" }
```

```bash
aws cloudformation deploy --template-file template.yaml --stack-name myapp-dev \
  --parameter-overrides EnvName=dev \
  --capabilities CAPABILITY_IAM
```

### Strengths
- Native — no extra runtime, no state file to manage (AWS stores state for you).
- **Stack drift detection** — `aws cloudformation detect-stack-drift` tells you what's changed manually.
- **Stack policies** to prevent accidental deletion of stateful resources.
- **Change sets**: preview what will change before applying.
- Rich service coverage (AWS adds CFN support on launch ~ usually).

### Weaknesses
- YAML/JSON is verbose. Loops? Use macros or CDK.
- Slow updates compared to Terraform (often).
- Drift detection covers only some property types.
- Limited cross-region/cross-account in a single template (use StackSets).

### StackSets
Deploy the same template across many accounts/regions from a central management account. Essential for multi-account.

```bash
aws cloudformation create-stack-set --stack-set-name baseline --template-body file://template.yaml
aws cloudformation create-stack-instances --stack-set-name baseline \
  --accounts 111111111111 222222222222 --regions ap-southeast-1 eu-west-1
```

---

## 3. AWS CDK — IaC as real code

CDK lets you write CloudFormation in TypeScript/Python/Java/.NET/Go using object-oriented constructs. Synthesizes to a CFN template; deployed via CFN.

```typescript
import * as cdk from "aws-cdk-lib";
import * as s3 from "aws-cdk-lib/aws-s3";
import * as iam from "aws-cdk-lib/aws-iam";

class MyAppStack extends cdk.Stack {
  constructor(scope: cdk.App, id: string, props?: cdk.StackProps) {
    super(scope, id, props);

    const bucket = new s3.Bucket(this, "Data", {
      versioned: true,
      blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
      encryption: s3.BucketEncryption.S3_MANAGED,
      lifecycleRules: [{
        abortIncompleteMultipartUploadAfter: cdk.Duration.days(7),
        noncurrentVersionExpiration: cdk.Duration.days(30),
        transitions: [{ storageClass: s3.StorageClass.INTELLIGENT_TIERING, transitionAfter: cdk.Duration.days(0) }],
      }],
    });

    const role = new iam.Role(this, "AppRole", {
      assumedBy: new iam.ServicePrincipal("lambda.amazonaws.com"),
    });
    bucket.grantReadWrite(role);
  }
}

const app = new cdk.App();
new MyAppStack(app, "MyApp-Dev", { env: { account: "123", region: "ap-southeast-1" } });
new MyAppStack(app, "MyApp-Prod", { env: { account: "456", region: "ap-southeast-1" } });
```

```bash
cdk bootstrap aws://123/ap-southeast-1   # one-time per account/region
cdk diff
cdk deploy MyApp-Dev
```

### Strengths
- Real language: loops, conditionals, types, abstractions, tests.
- **L2 constructs** like `s3.Bucket`, `lambda.Function` encode best practices and sensible defaults.
- **Patterns library** (`aws-cdk-lib/aws-ecs-patterns`, `aws-apigatewayv2-integrations`, etc.) for common architectures.
- Reusable internal constructs (build your team's "ApprovedBucket" once).

### Weaknesses
- Magic — generated CFN can be confusing; `cdk synth` to inspect.
- Tied to CFN under the hood (so subject to CFN limits/quirks).
- Version churn (cdk-lib v2 was already a big migration).
- Logical IDs auto-generated; renaming a construct can force-replace resources.

### Aspects and Custom Resources
**Aspects** apply visitor-pattern transforms across a tree (e.g., "tag every resource", "remove public access from every bucket").

**Custom Resources** let you back a CFN/CDK resource with a Lambda — for AWS APIs that don't have native CFN support, or for orchestration steps.

---

## 4. Terraform — the multi-cloud option

Terraform uses HCL and is provider-based (AWS, GCP, Azure, Datadog, GitHub, etc., all via providers).

```hcl
terraform {
  required_version = ">= 1.6"
  required_providers {
    aws = { source = "hashicorp/aws", version = "~> 5.0" }
  }
  backend "s3" {
    bucket         = "myorg-tf-state"
    key            = "myapp/prod/terraform.tfstate"
    region         = "ap-southeast-1"
    dynamodb_table = "tf-lock"
    encrypt        = true
  }
}

provider "aws" { region = "ap-southeast-1" }

resource "aws_s3_bucket" "data" {
  bucket = "myapp-prod-${data.aws_caller_identity.this.account_id}"
}

resource "aws_s3_bucket_public_access_block" "data" {
  bucket                  = aws_s3_bucket.data.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_versioning" "data" {
  bucket = aws_s3_bucket.data.id
  versioning_configuration { status = "Enabled" }
}

data "aws_caller_identity" "this" {}
```

```bash
terraform init
terraform plan
terraform apply
```

### Strengths
- Multi-cloud, multi-provider — one tool for everything.
- Mature ecosystem: thousands of modules in Terraform Registry.
- HCL is reasonably terse and readable.
- **Plan** output is clear and reviewable.
- Big community + corporate adoption.

### Weaknesses
- **State file** is a real thing — must be stored remotely (S3 + DynamoDB lock), backed up, never lost. Lose state → can't manage resources from Terraform without `import` gymnastics.
- HCL's limitations: weak typing, no loops outside `count`/`for_each`, expression-heavy.
- AWS coverage lags CloudFormation slightly for brand-new features.
- License change (BSL) in 2023 was contentious; **OpenTofu** is a fork worth tracking.

### Workspaces & remote state
- **Workspaces**: separate state for dev/staging/prod from one config (often combined with `-var-file`).
- **Terraform Cloud / Enterprise**: HCP-hosted state, policy (Sentinel), runs, secrets. $$.
- **S3 backend + DynamoDB locks** is the OSS-friendly setup.

---

## 5. Picking a tool

| Criterion | CFN | CDK | Terraform |
|---|---|---|---|
| AWS-only shop, small | ✓ | ✓ | ✓ |
| Multi-cloud or many providers | | | ✓ |
| Team prefers programming language | | ✓ | |
| Team prefers config | ✓ | | ✓ |
| Sophisticated abstractions | | ✓ | partial (modules) |
| New AWS service on day one | ✓ | ✓ | sometimes lags |
| Need state file ownership | | | ✓ (you own it) |
| Want AWS to own state | ✓ | ✓ | |

**Pragmatic patterns:**
- **AWS-only, dev-heavy team** → CDK.
- **AWS-only, ops/SRE team** → CloudFormation directly or Terraform.
- **Multi-cloud or multi-tool** → Terraform.
- Many orgs use **CDK for app stacks + Terraform for foundational/networking** — a reasonable mixed approach.

---

## 6. State, Drift, and Reconciliation

### State drift
Someone clicks in the console, modifying a resource. Now your IaC and reality disagree. Three approaches:
1. **Detect + alert** — CFN drift detection, Terraform `plan` showing diffs.
2. **Auto-revert** — re-run apply on a schedule.
3. **Prevent** — SCPs that deny console actions in prod, or read-only IAM for humans.

The last is the only durable answer at scale.

### Resource imports
You have an existing manually-created resource. To bring under IaC:
- **Terraform**: `terraform import aws_s3_bucket.data my-bucket-name` → then write matching HCL.
- **CFN**: `aws cloudformation create-change-set --change-set-type IMPORT`.
- **CDK**: import then represent via L1 constructs or `Fn.importValue`.

### Refactoring without recreation
Renaming a resource in code may force destroy + recreate (replacement). Watch for it:
- Terraform: use `moved` blocks (since 1.1).
- CFN: use stack policies + careful logical ID management.
- CDK: be aware that renaming a construct id changes logical ID → replacement. Use `Override.logicalId` to preserve.

---

## 7. Practical: a real deploy structure

A typical CDK + multi-account setup:

```
/iac
  bin/
    app.ts            # entry point
  lib/
    constructs/       # reusable abstractions
      approved-bucket.ts
      compliant-lambda.ts
    stacks/
      network-stack.ts
      data-stack.ts
      app-stack.ts
  cdk.json
  package.json
```

```typescript
// bin/app.ts
const app = new cdk.App();
const envs = {
  dev:  { account: "111", region: "ap-southeast-1" },
  prod: { account: "222", region: "ap-southeast-1" },
};
for (const [name, env] of Object.entries(envs)) {
  const network = new NetworkStack(app, `Network-${name}`, { env });
  const data    = new DataStack(app, `Data-${name}`, { env, vpc: network.vpc });
  new AppStack(app, `App-${name}`, { env, vpc: network.vpc, db: data.db });
}
```

Deployments run via CodePipeline / GitHub Actions with OIDC (Module 16) — never from a developer laptop in prod.

---

## 8. Testing IaC

- **`cdk diff` / `terraform plan`** in PRs → require human review.
- **Static checks**: `cfn-lint`, `cfn-nag`, `cdk-nag`, `tflint`, `checkov`, `kics`.
- **Policy as code**: Sentinel (Terraform Cloud), Open Policy Agent (OPA), `cfn-guard`. Block PRs that violate guardrails (no public buckets, all RDS encrypted, etc.).
- **Unit tests** (CDK): `Template.fromStack(stack).hasResourceProperties("AWS::S3::Bucket", { ... })`.
- **Integration tests**: deploy to a sandbox, run smoke tests, tear down. Tools like `terratest` (Terraform) and CDK's `integ-tests`.

---

## 9. Common Mistakes & Gotchas

- **State file in git.** Terraform state contains secrets in plaintext. NEVER. Use remote state.
- **No state locking.** Two concurrent applies = corruption. DynamoDB locks for S3 backend, mandatory.
- **`cdk deploy` with elevated creds from a laptop.** Use CI with OIDC + scoped role.
- **Long-running stacks with deletion-protected resources you delete.** CFN gets stuck `DELETE_FAILED`. Manually clean stuck resources, retry.
- **Implicit dependencies across stacks.** Stack A produces an output, Stack B imports it — but you can't delete A while B exists. Use SSM Parameter Store as the indirection, or carefully manage stack ordering.
- **Custom resources that fail.** A Lambda-backed CFN resource that hangs blocks the stack for an hour. Always add error handling + signal failure quickly.
- **Renaming constructs in CDK.** Logical ID changes → resource replacement → data loss on stateful resources.
- **Mixing CDK versions across stacks** in one app — synth errors.
- **No `cdk bootstrap` updates.** Old bootstrap stacks lack newer features (e.g., new IAM permissions). Re-run `cdk bootstrap` regularly.
- **Terraform `count` for resources that should be `for_each`.** `count` makes the list ordering critical — removing an early element shifts indices, recreating everything.
- **Hardcoded ARNs in templates.** Cross-environment promotion breaks. Use parameters / SSM lookups.
- **Drift not detected.** Schedule periodic drift detection + alarms.
- **No retention policies on stateful resources.** Stack delete = data delete. Set `DeletionPolicy: Retain` (CFN), `removalPolicy: RETAIN` (CDK), `lifecycle { prevent_destroy = true }` (Terraform) on prod DBs / S3 buckets / KMS keys.
- **Embedding secrets in templates.** Use Secrets Manager / SSM Parameter Store + reference, not literal strings.
- **Provider version drift.** Pin major versions; review minor bumps in PR.

---

## 🎯 Key Takeaways

- **All infrastructure as code, no exceptions.** Click-ops creates drift, untestability, and unauditable changes. The IaC tool matters less than the discipline.
- **CDK = best DX on AWS-only stacks; Terraform = best fit for multi-cloud or polyglot.** Both are excellent; pick by team and scope, not religion.
- **State management is the silent killer.** Lost state, unlocked state, secrets in state — these wreck Terraform deployments. CFN sidesteps this by owning state, at the cost of flexibility.
- **Apply IaC from CI with OIDC roles, never from developer laptops.** This single discipline eliminates "works on my machine" infrastructure mysteries.
- **Policy-as-code (cfn-nag, cdk-nag, checkov, OPA) in PR checks** stops the biggest misconfigurations — public buckets, unencrypted volumes, wide-open SGs — before they merge.

*← [prev](./14_security_compliance.md) | [next →](./16_cicd_on_aws.md)*
