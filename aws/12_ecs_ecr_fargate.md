# 12 — ECS, ECR, Fargate: Containers on AWS

> **Goal:** Build, store, deploy, and operate container workloads on AWS without dealing with Kubernetes. (EKS/K8s is its own course.)

---

## 1. The container service landscape

**Mental model:**
- **ECR** = the image registry. Where your Docker images live.
- **ECS** = the orchestrator. Decides what runs where.
- **Fargate** = the serverless compute *capacity* that ECS (or EKS) can use, so you don't manage EC2.

You can also run ECS on EC2 capacity (you own the host fleet). And there's **EKS** (managed Kubernetes), covered separately.

For most teams without Kubernetes investment, **ECS on Fargate is the right answer**: simpler than K8s, integrates natively with AWS, no nodes to patch.

---

## 2. ECR — Elastic Container Registry

A private Docker registry per region per account. Supports image scanning, immutability, lifecycle rules.

### Push an image
```bash
aws ecr create-repository --repository-name myapp --image-scanning-configuration scanOnPush=true \
  --image-tag-mutability IMMUTABLE

aws ecr get-login-password --region us-east-1 | \
  docker login --username AWS --password-stdin 123456789012.dkr.ecr.us-east-1.amazonaws.com

docker build -t myapp:v1 .
docker tag myapp:v1 123456789012.dkr.ecr.us-east-1.amazonaws.com/myapp:v1
docker push 123456789012.dkr.ecr.us-east-1.amazonaws.com/myapp:v1
```

### Why `IMMUTABLE` tags
Once pushed, a tag can't be reused. Prevents the "redeploy `myapp:latest`, get different bits" surprise. **Always use immutable tags + content-addressed tags (commit SHA).**

### Scanning
- **Basic scanning** (free, daily, Clair-based): vulnerabilities.
- **Enhanced scanning** ($, continuous, Inspector-based): vulnerabilities + secrets + supply-chain.

### Lifecycle rules
Auto-delete old/untagged images.

```json
{
  "rules": [{
    "rulePriority": 1, "description": "Keep last 30 images",
    "selection": {"tagStatus": "any", "countType": "imageCountMoreThan", "countNumber": 30},
    "action": {"type": "expire"}
  }]
}
```

### ECR Public
A separate service (`public.ecr.aws`) for sharing images publicly. AWS hosts base images here (e.g., Amazon Linux, Lambda runtimes).

---

## 3. ECS Concepts — clusters, task definitions, services, tasks

### Cluster
A logical grouping. With Fargate, the cluster is just a name + IAM scope. With EC2, the cluster has registered container instances.

### Task Definition
A versioned JSON blueprint: image(s), CPU/memory, env vars, port mappings, IAM **task role**, networking mode, volumes, logging.

Tasks support multiple containers (sidecars: log shippers, proxies, agents). 1 task = 1 unit of scheduling.

```json
{
  "family": "myapp",
  "networkMode": "awsvpc",
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "512",
  "memory": "1024",
  "executionRoleArn": "arn:aws:iam::...:role/ecsTaskExecutionRole",
  "taskRoleArn": "arn:aws:iam::...:role/myapp-task-role",
  "containerDefinitions": [{
    "name": "web",
    "image": "123456789012.dkr.ecr.us-east-1.amazonaws.com/myapp:v1",
    "portMappings": [{"containerPort": 8080, "protocol": "tcp"}],
    "environment": [{"name": "ENV", "value": "prod"}],
    "secrets": [{"name": "DB_PASSWORD", "valueFrom": "arn:aws:secretsmanager:..."}],
    "logConfiguration": {
      "logDriver": "awslogs",
      "options": {
        "awslogs-group": "/ecs/myapp",
        "awslogs-region": "us-east-1",
        "awslogs-stream-prefix": "web"
      }
    },
    "healthCheck": {
      "command": ["CMD-SHELL", "curl -f http://localhost:8080/healthz || exit 1"],
      "interval": 30, "timeout": 5, "retries": 3, "startPeriod": 30
    }
  }]
}
```

### Two IAM roles per task — critical distinction
- **Task Execution Role**: used by ECS itself to pull the image from ECR, push logs to CloudWatch, read secrets. Always needed.
- **Task Role**: used *by your application code* to call AWS APIs. The role your app's SDK picks up via the credential chain.

### Service
A long-running, scaled task. Maintains N copies of a task definition, registers them with a load balancer, replaces unhealthy tasks.

```bash
aws ecs create-service \
  --cluster prod \
  --service-name myapp \
  --task-definition myapp:7 \
  --desired-count 2 \
  --launch-type FARGATE \
  --network-configuration "awsvpcConfiguration={subnets=[subnet-priv-a,subnet-priv-b],securityGroups=[$APP_SG],assignPublicIp=DISABLED}" \
  --load-balancers targetGroupArn=$TG_ARN,containerName=web,containerPort=8080 \
  --health-check-grace-period-seconds 60 \
  --deployment-configuration "maximumPercent=200,minimumHealthyPercent=100,deploymentCircuitBreaker={enable=true,rollback=true}"
```

### Standalone task
For batch / cron: `aws ecs run-task` runs a task definition once. Pair with EventBridge Scheduler.

---

## 4. Fargate vs EC2 launch type

| | Fargate | EC2 |
|---|---|---|
| Capacity management | None | You manage ASG of container instances |
| Cost | Higher per CPU/GB | Lower per CPU/GB; better with Spot |
| Patching | None | You patch hosts |
| Startup time | 30-60s | Seconds (host pre-warmed) |
| GPU | Limited | Full support |
| Daemon-set containers | Limited | Yes |
| Networking | awsvpc only (1 ENI/task) | bridge / awsvpc / host |

**Default to Fargate.** Switch to EC2 when:
- Per-task density is high (running 100s of tiny containers on a single host beats per-task Fargate billing).
- You need GPU.
- You need very low startup latency.
- You need privileged containers / daemons.

**Fargate Spot** offers ~70% discount with 2-min interruption notice. Great for stateless batch/processing.

---

## 5. Networking — `awsvpc` is the only sane mode

In `awsvpc` mode, each task gets its **own ENI** with a private IP in your VPC. The task is a first-class VPC citizen: SGs apply directly, VPC endpoints work, flow logs see it.

This also means:
- Subnet IP usage grows with task count.
- ENI creation adds ~10-30s to task start (Fargate has improved this drastically).
- You can use SGs to lock task-to-task traffic precisely.

### Service Discovery / Service Connect
- **AWS Cloud Map**: register tasks as `app.local` for internal DNS.
- **ECS Service Connect** (newer, recommended): managed Envoy sidecars, automatic service-to-service traffic with metrics/retries/circuit breaking. No load balancer needed between services.

```bash
aws ecs create-service --service-connect-configuration '{
  "enabled": true,
  "namespace": "prod",
  "services": [{
    "portName": "api",
    "clientAliases": [{"port": 8080, "dnsName": "api"}]
  }]
}'
```
Now other services in `prod` namespace can `http://api:8080`.

---

## 6. Load Balancing

**Application Load Balancer (ALB)** is the standard front for ECS services. HTTP/HTTPS, path/host routing, native auth (OIDC), WAF.

**Network Load Balancer (NLB)**: TCP/UDP, ultra-low latency, static IPs, preserves source IP. Use for non-HTTP, gaming, NLB→ALB chains.

ECS auto-registers/deregisters tasks with target groups as they scale.

### Dynamic port mapping (EC2 launch type only)
ECS picks an ephemeral host port; ALB target group is in "instance" mode. Lets multiple tasks of the same image run on one host. (Not applicable to Fargate — every task has its own ENI with the same port.)

---

## 7. Scaling

### Service auto-scaling
Target tracking on CPU, memory, ALB RequestCountPerTarget, or custom CloudWatch metric.

```bash
aws application-autoscaling register-scalable-target \
  --service-namespace ecs \
  --resource-id service/prod/myapp \
  --scalable-dimension ecs:service:DesiredCount \
  --min-capacity 2 --max-capacity 50

aws application-autoscaling put-scaling-policy \
  --policy-name keep-cpu-60 \
  --service-namespace ecs \
  --resource-id service/prod/myapp \
  --scalable-dimension ecs:service:DesiredCount \
  --policy-type TargetTrackingScaling \
  --target-tracking-scaling-policy-configuration '{
    "TargetValue": 60.0,
    "PredefinedMetricSpecification": {"PredefinedMetricType": "ECSServiceAverageCPUUtilization"}
  }'
```

### Capacity providers (EC2)
Mix Fargate / Fargate Spot / EC2 / EC2 Spot in one service with weights. ECS routes new tasks across them.

---

## 8. Deployments

### Rolling update (default)
ECS deploys new task def version gradually, keeping `minimumHealthyPercent` healthy and `maximumPercent` as ceiling. With **deployment circuit breaker**, ECS auto-rolls-back on failure.

### Blue/Green via CodeDeploy
For true zero-downtime + traffic-shifting deploys: CodeDeploy creates a parallel task set, shifts ALB target group from old to new in steps (canary 10/90 → 50/50 → 100), monitors CloudWatch alarms, rolls back automatically on alarm.

### External (Spinnaker, Argo Rollouts, etc.)
Possible via the "EXTERNAL" deployment controller. Less common on ECS.

---

## 9. Practical: a production ECS Fargate service

```typescript
// CDK with the high-level pattern
const cluster = new ecs.Cluster(this, "Prod", { vpc, containerInsights: true });

const fargateService = new ecs_patterns.ApplicationLoadBalancedFargateService(this, "Api", {
  cluster, cpu: 512, memoryLimitMiB: 1024,
  desiredCount: 2,
  taskImageOptions: {
    image: ecs.ContainerImage.fromEcrRepository(repo, "v1.2.3"),
    containerPort: 8080,
    environment: { ENV: "prod" },
    secrets: { DB_PASSWORD: ecs.Secret.fromSecretsManager(dbSecret, "password") },
    logDriver: ecs.LogDrivers.awsLogs({ streamPrefix: "api", logRetention: logs.RetentionDays.TWO_WEEKS }),
  },
  healthCheckGracePeriod: cdk.Duration.seconds(60),
  publicLoadBalancer: true,
  certificate: cert,
  redirectHTTP: true,
  circuitBreaker: { rollback: true },
  taskSubnets: { subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS },
});

fargateService.targetGroup.configureHealthCheck({
  path: "/healthz", healthyThresholdCount: 2, unhealthyThresholdCount: 3,
  interval: cdk.Duration.seconds(15),
});

fargateService.service.autoScaleTaskCount({ minCapacity: 2, maxCapacity: 20 })
  .scaleOnCpuUtilization("Cpu60", { targetUtilizationPercent: 60 });
```

---

## 10. Observability

- **CloudWatch Container Insights**: per-task/service CPU, memory, network. **Enable on every cluster.**
- **ECS Exec**: ssh-like shell into a running task (no public IP, no SSH). Requires `enableExecuteCommand=true` on the service + SSM permissions on task role.
  ```bash
  aws ecs execute-command --cluster prod --task <id> --container web --interactive --command "/bin/sh"
  ```
- **App Mesh / Service Connect** for service mesh-level metrics.
- **ADOT (AWS Distro for OpenTelemetry)** sidecar for traces/metrics/logs to your observability stack.

---

## 11. Common Mistakes & Gotchas

- **Task role vs execution role confused.** Most "can't pull image" issues = execution role missing `AmazonECSTaskExecutionRolePolicy`. Most "app can't access S3" issues = missing on task role.
- **Mutable tags (`:latest`).** Hard to reason about deploys, supports rollback only by chance. Use immutable + SHA tags.
- **No health check.** ECS doesn't know the container crashed; ALB serves errors. Container-level `HEALTHCHECK` + ALB target health check both.
- **Health check grace period too short.** Slow-booting apps killed mid-startup. Set 30-120s.
- **Logs not shipped.** Default no log driver. Set `awslogs` (or Firelens for advanced).
- **Log group retention forever.** Set it (`logRetention`).
- **`minimumHealthyPercent: 50` with desiredCount=2.** During deploy, you go down to 1 task = potential outage. Set `100` and `maximumPercent: 200` for true zero-downtime.
- **No deployment circuit breaker.** Bad deploy = down service. Always enable.
- **Public IP on Fargate task** (`assignPublicIp=ENABLED`). Bypasses your private-subnet design. Use NAT instead.
- **Subnet ENI exhaustion.** Each task = 1 ENI. /28 subnet ≈ 11 tasks. Use /22+.
- **Forgetting `enableExecuteCommand`** before you need to debug. Can't turn on retroactively for running tasks without an update.
- **Fargate Spot for stateful tasks.** 2-min notice = lost work. Stateless only.
- **Secrets as plaintext env vars.** Use the `secrets` field with Secrets Manager / Parameter Store ARNs.
- **Container can't reach Secrets Manager.** Without VPC endpoint, traffic goes through NAT (cost) or fails (no NAT). Add `secretsmanager` interface endpoint.
- **`awslogs-create-group: true` not set** → log group doesn't exist on first run → task fails. Either pre-create or enable auto-create.
- **GraphQL N+1 multiplied across containers.** Containers don't make bad code good. Profile before scaling.

---

## 🎯 Key Takeaways

- **ECS on Fargate is the simpler, AWS-native containers path.** Pick it unless you've already invested in Kubernetes — the operational savings (no node patching, no cluster autoscaler) are substantial.
- **Two roles per task, never confuse them.** Execution role = ECS pulls/logs/secrets. Task role = your app's AWS calls. They're orthogonal.
- **Immutable image tags + deployment circuit breaker + container health check + ALB health check** are the four-layer safety net. Skip any one and you'll learn why it existed.
- **`awsvpc` mode + tight SGs + Service Connect** gives you a service mesh-grade network without running a service mesh.
- **Fargate Spot for stateless batch, Fargate for stateful services** — the 70% discount on the right workload is one of AWS's biggest cost levers.

*← [prev](./11_cloudfront_route53.md) | [next →](./13_observability.md)*
