# 16 — CI/CD on AWS: CodePipeline, GitHub Actions OIDC, Blue/Green

> **Goal:** Build reliable, auditable, secure pipelines from commit to production — using AWS-native services and/or GitHub Actions with OIDC federation (no long-lived keys).

---

## 1. The CI/CD landscape on AWS

**Mental model:** A pipeline takes source code, builds artifacts, tests them, and deploys them — with gates between stages. Different teams pick:

- **AWS-native: CodePipeline + CodeBuild + CodeDeploy** — tight AWS integration, no third-party.
- **GitHub Actions + OIDC** — code lives on GitHub anyway, runners on GitHub or self-hosted on EC2/Fargate.
- **GitLab CI / Jenkins / Buildkite + OIDC** — same pattern as GitHub.
- **Hybrid**: GitHub Actions builds + tests, CodePipeline orchestrates deployment.

The shift in the last 3 years is decisive: **OIDC federation to short-lived AWS roles** has replaced long-lived access keys for CI. Everyone should be doing this.

---

## 2. CodeBuild — managed build runner

A managed CI runner. Pulls source, runs commands defined in `buildspec.yml`, produces artifacts. Pay per build minute.

```yaml
# buildspec.yml
version: 0.2
env:
  variables:
    NODE_ENV: production
phases:
  install:
    runtime-versions: { nodejs: 20 }
    commands:
      - npm ci
  pre_build:
    commands:
      - aws ecr get-login-password --region $AWS_REGION | \
          docker login --username AWS --password-stdin $REPO_URI
  build:
    commands:
      - npm test
      - docker build -t myapp:$CODEBUILD_RESOLVED_SOURCE_VERSION .
      - docker tag myapp:$CODEBUILD_RESOLVED_SOURCE_VERSION $REPO_URI:$CODEBUILD_RESOLVED_SOURCE_VERSION
      - docker push $REPO_URI:$CODEBUILD_RESOLVED_SOURCE_VERSION
artifacts:
  files:
    - appspec.yml
    - taskdef.json
```

CodeBuild can use:
- Managed images (Ubuntu, Amazon Linux, Windows) with common toolchains.
- Custom Docker images.
- ARM and x86 instances; Lambda compute mode for short builds.

### Caching
`cache: { paths: [/root/.npm/**/*] }` or S3 cache → big speedups.

### Reports
Emit JUnit/Cobertura → CodeBuild test reports UI.

---

## 3. CodeDeploy — managed deployment

CodeDeploy orchestrates **deployment** to EC2/ASG, ECS, or Lambda — with health checks, traffic shifting strategies, and automatic rollback.

### Deployment types
- **In-place** (EC2/ASG only): replace on the same instances. Cheap but downtime risk.
- **Blue/Green** (ECS, Lambda, EC2/ASG): stand up new fleet, shift traffic, terminate old.

### Traffic shifting (ECS / Lambda)
- **AllAtOnce**: 100% switch.
- **Linear10PercentEvery1Minute**, etc.
- **Canary10Percent5Minutes**: 10% → wait → 100%.

```yaml
# appspec.yml for ECS
version: 0.0
Resources:
  - TargetService:
      Type: AWS::ECS::Service
      Properties:
        TaskDefinition: <TASK_DEFINITION>
        LoadBalancerInfo:
          ContainerName: web
          ContainerPort: 8080
Hooks:
  - BeforeAllowTraffic: arn:aws:lambda:...:smoke-test
  - AfterAllowTraffic: arn:aws:lambda:...:integration-test
```

Add CloudWatch alarms as **deployment alarms** → CodeDeploy rolls back automatically if the alarm fires during/after shift.

---

## 4. CodePipeline — the orchestrator

Stages of actions, with manual approvals, parallel branches, cross-region/cross-account targets.

```bash
# Conceptually: source → build → test → deploy-staging → manual approval → deploy-prod
```

A pipeline definition:

```typescript
const pipeline = new codepipeline.Pipeline(this, "Cd", {
  pipelineName: "myapp",
  pipelineType: codepipeline.PipelineType.V2,
});

const source = new codepipeline.Artifact("Source");
pipeline.addStage({
  stageName: "Source",
  actions: [new actions.GitHubSourceAction({
    actionName: "GitHub",
    output: source,
    owner: "myorg", repo: "myapp", branch: "main",
    oauthToken: cdk.SecretValue.secretsManager("github-token"),  // or use Connections for OIDC
    trigger: actions.GitHubTrigger.WEBHOOK,
  })],
});

const built = new codepipeline.Artifact("Built");
pipeline.addStage({
  stageName: "Build",
  actions: [new actions.CodeBuildAction({
    actionName: "Build", project: buildProject,
    input: source, outputs: [built],
  })],
});

pipeline.addStage({
  stageName: "DeployStaging",
  actions: [new actions.EcsDeployAction({
    actionName: "DeployECS",
    service: stagingService,
    imageFile: built.atPath("imagedefinitions.json"),
  })],
});

pipeline.addStage({
  stageName: "ApproveProd",
  actions: [new actions.ManualApprovalAction({ actionName: "Approve" })],
});

pipeline.addStage({
  stageName: "DeployProd",
  actions: [new actions.CodeDeployEcsDeployAction({
    actionName: "BlueGreen",
    deploymentGroup: prodDeploymentGroup,
    taskDefinitionTemplateFile: built.atPath("taskdef.json"),
    appSpecTemplateFile: built.atPath("appspec.yaml"),
  })],
});
```

### CodeStar Connections
The modern way to wire CodePipeline to GitHub/Bitbucket/GitLab without storing tokens — OAuth-style consent, AWS-managed.

---

## 5. GitHub Actions with OIDC — the modern path

OpenID Connect lets GitHub Actions assume an AWS role for a single workflow run — no long-lived AWS keys stored in GitHub.

### 1. Create the OIDC provider in AWS (one-time per account)
```bash
aws iam create-open-id-connect-provider \
  --url https://token.actions.githubusercontent.com \
  --client-id-list sts.amazonaws.com \
  --thumbprint-list 6938fd4d98bab03faadb97b34396831e3780aea1
```

### 2. Create a role that GitHub Actions can assume
Trust policy:
```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "Federated": "arn:aws:iam::123456789012:oidc-provider/token.actions.githubusercontent.com" },
    "Action": "sts:AssumeRoleWithWebIdentity",
    "Condition": {
      "StringEquals": { "token.actions.githubusercontent.com:aud": "sts.amazonaws.com" },
      "StringLike":   { "token.actions.githubusercontent.com:sub": "repo:myorg/myapp:ref:refs/heads/main" }
    }
  }]
}
```
The `sub` condition is your **security boundary** — only workflows on `main` of `myorg/myapp` can assume this role. Scope tightly.

### 3. Use in the workflow
```yaml
name: deploy
on:
  push: { branches: [main] }
permissions:
  id-token: write   # required for OIDC
  contents: read
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::123456789012:role/gh-actions-deploy
          aws-region: ap-southeast-1
      - name: Login ECR
        uses: aws-actions/amazon-ecr-login@v2
      - name: Build & push
        run: |
          docker build -t $ECR_URI:$GITHUB_SHA .
          docker push $ECR_URI:$GITHUB_SHA
      - name: Deploy ECS (CDK)
        run: |
          npm ci
          npx cdk deploy MyAppProd --require-approval never
```

**No `AWS_ACCESS_KEY_ID` anywhere.** No GitHub secret of long-lived AWS creds. Audit lives in CloudTrail.

### Per-branch / per-environment roles
Different roles for `main` vs `feature/*` vs `release/*`, each with appropriate permission scope (e.g., feature branches deploy only to dev account).

---

## 6. Deployment strategies revisited

| Strategy | Risk | Cost | Complexity |
|---|---|---|---|
| **All at once** | Highest | None | Lowest |
| **Rolling** | Medium | None | Low |
| **Blue/Green** | Low | 2× during deploy | Medium |
| **Canary** | Lowest | Slight overhead | High |
| **Feature flags** | App-level rollout | Modest infra | Highest (app-side discipline) |

Real production teams combine: **Blue/Green deploys + feature flags** for fine-grained release control + canary metrics for auto-rollback.

### LaunchDarkly / AWS AppConfig / Unleash
Feature flag services. AppConfig is AWS-native — configuration with safe deployment strategies (gradual rollout + alarms + rollback).

```bash
# AppConfig: deploy a config update to 10% over 10 minutes with alarm rollback
aws appconfig start-deployment --application-id $APP --environment-id $ENV \
  --deployment-strategy-id AppConfig.Linear50PercentEvery30Seconds \
  --configuration-profile-id $PROFILE --configuration-version $VERSION
```

---

## 7. Multi-account pipelines

Best practice: **pipelines run in a dedicated CI/CD account** and deploy to dev/staging/prod accounts by assuming cross-account roles.

```
[CI/CD Account]                    [Dev Account]         [Prod Account]
 CodePipeline ─────► sts:AssumeRole ─► deploy-role         deploy-role
 GitHub Actions ───► sts:AssumeRole ────────────────────►  deploy-role
```

Each target account has a `deploy-role` with a trust policy allowing the CI/CD account, plus permission policies scoped to what the pipeline needs to change.

---

## 8. Practical: a complete GitHub Actions → AWS workflow

```yaml
name: ci-cd
on:
  push:
    branches: [main]
  pull_request:

permissions:
  id-token: write
  contents: read
  pull-requests: write

concurrency:
  group: ${{ github.ref }}
  cancel-in-progress: false

jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: 20, cache: npm }
      - run: npm ci
      - run: npm test
      - run: npm run lint
      - run: npm run build

  cdk-diff:
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    needs: ci
    steps:
      - uses: actions/checkout@v4
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::111:role/gh-actions-readonly
          aws-region: ap-southeast-1
      - run: npx cdk diff > diff.txt || true
      - uses: marocchino/sticky-pull-request-comment@v2
        with: { path: diff.txt }

  deploy-dev:
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    needs: ci
    environment: dev
    steps:
      - uses: actions/checkout@v4
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::111:role/gh-actions-deploy-dev
          aws-region: ap-southeast-1
      - run: npx cdk deploy MyApp-Dev --require-approval never

  deploy-prod:
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    needs: deploy-dev
    environment: prod    # requires manual approval via GitHub environment protection
    steps:
      - uses: actions/checkout@v4
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::222:role/gh-actions-deploy-prod
          aws-region: ap-southeast-1
      - run: npx cdk deploy MyApp-Prod --require-approval never
```

Pair the `prod` environment with **required reviewers** in GitHub repo settings. Result: every prod deploy needs human approval, deploy runs in CI without long-lived secrets, full audit in CloudTrail.

---

## 9. Secrets in pipelines

- **Never** put AWS keys in repo / GitHub secrets / env files. OIDC instead.
- For non-AWS secrets (npm tokens, vendor API keys): GitHub Encrypted Secrets, scoped per environment.
- For runtime secrets (DB passwords): the deployed app reads from Secrets Manager via task role — pipeline never sees them.

---

## 10. Common Mistakes & Gotchas

- **Long-lived AWS keys in GitHub Secrets.** The pre-2022 default. Replace with OIDC. There is no acceptable excuse anymore.
- **`role-to-assume` trust policy with `sub: *`.** Any GitHub repo on the internet can assume your role. Scope `sub` to your repo + branch/environment.
- **No branch protection.** Pipeline auto-deploys from `main`, anyone can push to main. Branch protection + required reviews.
- **No manual approval before prod.** Auto-deploy is great in dev; in prod, at least one human eyeball.
- **Deploy on every commit, regardless of tests passing.** Test failure must block deploy.
- **No deployment alarms wired to CodeDeploy.** Bad deploy goes live and stays. Always wire CW alarms (error rate, latency) for auto-rollback.
- **`cdk deploy --require-approval never` everywhere.** Fine in CI, dangerous locally — `cdk deploy` from a laptop changes prod in seconds.
- **Mutable image tags from CI** (`:latest`). Production drift; rollback impossible. Use commit SHA tags + ECS task definition revisions.
- **One pipeline deploying to all accounts** without per-environment guardrails. Test failure in dev should not block prod hotfix; structure pipelines accordingly.
- **Forgot CodeBuild's `privileged: true`** when building Docker → "docker daemon not running".
- **CodeBuild VPC config wrong** → can't pull from private NPM/ECR. Match subnets + SGs.
- **GitHub Actions runner exposed to PRs from forks** with secrets enabled. Forks can run malicious code with your secrets. `pull_request_target` is dangerous — read the docs.
- **No concurrency control.** Two deploys racing = corrupt state. Use GitHub `concurrency` or CodePipeline serial execution.
- **Pipeline rolls back app but not DB migrations.** Schema is forward-only; ensure migrations are backward compatible.
- **No SBOM, no image signing.** Modern supply-chain attacks demand them. `cosign`, AWS Signer for Lambda, etc.

---

## 🎯 Key Takeaways

- **OIDC federation kills the access-key problem.** Every CI system worth using (GitHub, GitLab, Buildkite, CircleCI) supports it; configure once, never store an AWS key in CI again.
- **Pipelines run in a dedicated CI/CD account; deploys assume roles into target accounts.** This is the multi-account pattern that scales.
- **Blue/Green + canary + deployment alarms + automatic rollback** is the production-grade deploy. The infrastructure for it is already built (CodeDeploy, ALB target groups) — you just have to wire it.
- **Feature flags decouple deploy from release.** Code goes to prod dark; flag flips control activation. Pair with AppConfig or LaunchDarkly for safe, granular rollouts.
- **Branch protection + required reviews + manual approval to prod + alarms wired to rollback** is the four-part minimum for any serious CI/CD. Skipping any one of these is a near-future incident.

*← [prev](./15_infrastructure_as_code.md) | [next →](./17_cost_optimization.md)*
