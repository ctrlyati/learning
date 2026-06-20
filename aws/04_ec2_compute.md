# 04 — EC2: Virtual Machines, AMIs, EBS, and Autoscaling

> **Goal:** Pick the right EC2 instance, build a hardened AMI, attach storage thoughtfully, and scale horizontally with confidence — including the cost knobs (Spot, Savings Plans) that move the needle.

---

## 1. EC2 — the original AWS service

**Mental model:** EC2 is a giant fleet of hypervisors (KVM/Nitro). When you `RunInstances`, AWS places a VM on a hypervisor that has the requested CPU/RAM, boots it from an **AMI** (machine image), wires it to your VPC subnet via an **ENI** (elastic network interface), and attaches an **EBS** root volume. You pay per second for the VM time plus per-GB-month for the storage.

### Launch one (the long way, instructive)
```bash
# 1. Find a current Amazon Linux 2023 AMI in your region
AMI=$(aws ec2 describe-images --owners amazon \
  --filters "Name=name,Values=al2023-ami-*-x86_64" "Name=state,Values=available" \
  --query 'Images | sort_by(@, &CreationDate)[-1].ImageId' --output text)

# 2. Create a key pair (download .pem and chmod 400)
aws ec2 create-key-pair --key-name dev --query KeyMaterial --output text > dev.pem
chmod 400 dev.pem

# 3. Launch
aws ec2 run-instances \
  --image-id $AMI \
  --instance-type t3.micro \
  --key-name dev \
  --subnet-id subnet-pub-a \
  --security-group-ids $WEB_SG \
  --iam-instance-profile Name=ec2-s3-reader \
  --user-data file://bootstrap.sh \
  --metadata-options HttpTokens=required \
  --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=demo}]'
```

Note `HttpTokens=required` → enforces **IMDSv2**, which prevents SSRF-style metadata theft (a real attack vector that hit Capital One). Always set it.

### user-data — the boot script
```bash
#!/bin/bash
dnf update -y
dnf install -y nginx
systemctl enable --now nginx
```

`user-data` runs as root on first boot. For complex bootstrapping, prefer baking an AMI (Packer) over long user-data scripts — faster boots, deterministic.

---

## 2. Instance families & sizing

Instance type = `[family][generation][.size]`, e.g., `m6i.large`.

| Family | Optimized for | Examples |
|---|---|---|
| **t** | Burstable, general | `t3`, `t4g`. Cheap, accrue CPU credits. Great for low-utilization workloads. |
| **m** | Balanced general | `m6i`, `m7g`. The "default" workhorse. |
| **c** | Compute (high CPU/RAM ratio) | `c6i`, `c7g`. Web servers, batch, HPC. |
| **r** | Memory-optimized | `r6i`, `r7g`. Databases, caches, in-memory analytics. |
| **x / u** | Huge memory | `x2idn` (4 TB RAM), `u-` (24 TB). SAP HANA. |
| **i / d** | Storage-optimized (NVMe / HDD) | `i4i`, `d3en`. Databases, big data. |
| **g / p** | GPU | `g5`, `p4d`. ML training/inference, graphics. |
| **inf / trn** | AWS custom silicon | `inf2` (inference), `trn1` (training). |

### Generations and architectures
- **i = Intel**, **a = AMD**, **g = AWS Graviton (ARM)**. Graviton is typically 20-40% cheaper and faster per dollar. **Default to Graviton** unless you have x86 binaries that won't recompile.
- Generations: higher = newer = better $/perf. Don't launch `m4` in 2026.

### Right-sizing
Use **Compute Optimizer** (free) — it watches CloudWatch metrics for 14+ days and recommends size changes. Most fleets are 30%+ overprovisioned.

```bash
aws compute-optimizer get-ec2-instance-recommendations \
  --instance-arns arn:aws:ec2:ap-southeast-1:123456789012:instance/i-xxx
```

---

## 3. AMIs — the machine image

An AMI is a snapshot of a root volume + metadata (kernel, block device mapping, architecture). When you launch, AWS clones the snapshot to a new EBS volume for the instance.

### Sources
- **Amazon-owned**: Amazon Linux 2023, Ubuntu, Windows Server, etc.
- **AWS Marketplace**: vendor AMIs (often $$$).
- **Community AMIs**: untrusted. Avoid.
- **Your own**: build with **EC2 Image Builder** or **Packer**.

### Baking an AMI with Packer
```hcl
source "amazon-ebs" "app" {
  region        = "ap-southeast-1"
  source_ami_filter {
    filters = { name = "al2023-ami-*-x86_64", state = "available" }
    owners  = ["amazon"]
    most_recent = true
  }
  instance_type = "t3.micro"
  ssh_username  = "ec2-user"
  ami_name      = "app-{{timestamp}}"
}
build {
  sources = ["source.amazon-ebs.app"]
  provisioner "shell" {
    inline = ["sudo dnf install -y nginx", "sudo systemctl enable nginx"]
  }
}
```

Baking gives reproducible, fast-booting instances. Pair with **Auto Scaling Group launch templates** referring to AMI ID.

---

## 4. EBS — block storage that lives past the instance

EBS volumes are network-attached block devices, replicated within an AZ. They survive instance termination if `DeleteOnTermination=false`. They're AZ-scoped — can't move directly across AZs (must snapshot → restore).

### Volume types

| Type | Use case | Performance | Cost |
|---|---|---|---|
| **gp3** | Default for almost everything | 3000 IOPS / 125 MB/s baseline, scalable | $0.08/GB-mo |
| **gp2** | Legacy general purpose | Tied to volume size | $0.10/GB-mo — switch to gp3 |
| **io2** / **io2 Block Express** | High-perf databases | up to 256k IOPS, 99.999% durability | $$$ |
| **st1** | Throughput HDD (logs, big data) | High MB/s | $0.045/GB-mo |
| **sc1** | Cold HDD (archive) | Low | $0.015/GB-mo |

**Default to gp3.** Cheaper than gp2, faster, and you can dial IOPS/throughput independently from size.

### Snapshots
Incremental, stored in S3 (you can't see the bucket), regional. Cross-region copy supported.

```bash
aws ec2 create-snapshot --volume-id vol-xxx --description "pre-upgrade"
aws ec2 copy-snapshot --source-region ap-southeast-1 --source-snapshot-id snap-xxx --region ap-east-1
```

**EBS Snapshot Lifecycle Manager** (DLM) automates daily/weekly snapshots + retention.

### Instance store (NVMe)
Physically attached to the host. Blazing fast, *ephemeral*: data is lost on stop/terminate. Use for scratch, caches, replicated DBs. Free (included with instance price).

---

## 5. Connecting to instances — three ways

### a) SSH (the classic)
```bash
ssh -i dev.pem ec2-user@<public-ip>
```
Requires public IP, security group port 22 open, key in PEM. Don't put port 22 open to `0.0.0.0/0` in prod.

### b) Session Manager (the modern way)
**No SSH, no key, no public IP, no port 22.** Uses SSM Agent (preinstalled on Amazon Linux 2/2023, Windows, Ubuntu) which establishes an outbound connection to the SSM service. Auditable via CloudTrail.

```bash
# Requires the instance to have an IAM role with AmazonSSMManagedInstanceCore
aws ssm start-session --target i-xxx
```

For Session Manager to work in a private subnet without a NAT, add Interface endpoints for `ssm`, `ssmmessages`, and `ec2messages`.

### c) EC2 Instance Connect Endpoint
A managed endpoint that lets `ssh` reach instances over a tunnel — pure SSH UX, no public IP needed. Newer, cleaner than Session Manager for SSH-needing tooling (rsync, port forwarding).

---

## 6. Auto Scaling — horizontal scale

An **Auto Scaling Group (ASG)** maintains a desired number of EC2 instances by launching/terminating them. Combined with a **Launch Template** (the AMI, instance type, user-data, IAM role, SG, key) and one or more **scaling policies**.

```bash
# Launch template
aws ec2 create-launch-template \
  --launch-template-name app-lt \
  --launch-template-data '{
    "ImageId": "ami-xxx",
    "InstanceType": "t3.small",
    "IamInstanceProfile": {"Name": "app-role"},
    "SecurityGroupIds": ["sg-xxx"],
    "UserData": "'$(base64 -w 0 < bootstrap.sh)'",
    "MetadataOptions": {"HttpTokens": "required"}
  }'

# ASG
aws autoscaling create-auto-scaling-group \
  --auto-scaling-group-name app-asg \
  --launch-template LaunchTemplateName=app-lt \
  --min-size 2 --max-size 10 --desired-capacity 2 \
  --vpc-zone-identifier "subnet-priv-a,subnet-priv-b" \
  --target-group-arns arn:aws:elasticloadbalancing:...:targetgroup/app/xxx \
  --health-check-type ELB --health-check-grace-period 60
```

### Scaling policy types
- **Target tracking** — "keep CPU at 60%". Easiest, recommended.
- **Step scaling** — alarm-driven adds/removes.
- **Simple scaling** — legacy.
- **Scheduled** — cron-like (predictable bursts).
- **Predictive** — ML-based forecast (works well for daily/weekly patterns).

### Health checks
- **EC2 health**: AWS detects underlying failures.
- **ELB health**: ASG also checks if the ALB considers the instance healthy. **Use this for app failure detection.**

### Lifecycle hooks
Pause launch/terminate so you can warm caches, drain connections, or run final logs export. Critical for graceful deployments.

---

## 7. Practical: a simple HA web tier

```typescript
// CDK
const vpc = new ec2.Vpc(this, "Vpc", { maxAzs: 2, natGateways: 1 });

const asg = new autoscaling.AutoScalingGroup(this, "WebAsg", {
  vpc,
  vpcSubnets: { subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS },
  instanceType: ec2.InstanceType.of(ec2.InstanceClass.T4G, ec2.InstanceSize.SMALL), // Graviton
  machineImage: ec2.MachineImage.latestAmazonLinux2023({ cpuType: ec2.AmazonLinuxCpuType.ARM_64 }),
  minCapacity: 2,
  maxCapacity: 10,
  requireImdsv2: true,
  userData: ec2.UserData.custom(fs.readFileSync("bootstrap.sh", "utf8")),
});

const alb = new elbv2.ApplicationLoadBalancer(this, "Alb", { vpc, internetFacing: true });
const listener = alb.addListener("Https", {
  port: 443, certificates: [{ certificateArn: cert.certificateArn }],
});
listener.addTargets("WebTargets", { port: 80, targets: [asg] });

asg.scaleOnCpuUtilization("KeepCpu60", { targetUtilizationPercent: 60 });
```

---

## 8. Pricing models — where senior engineers earn their salary

| Model | Discount vs On-Demand | Commitment | Use case |
|---|---|---|---|
| **On-Demand** | 0% | None | Bursty, unpredictable, dev |
| **Spot** | up to ~90% | None (2-min interruption notice) | Stateless workloads (batch, CI, big-data, fault-tolerant fleets) |
| **Savings Plans (Compute)** | ~27-66% | 1 or 3 yr, $/hr commit | Steady-state compute across EC2/Fargate/Lambda |
| **EC2 Instance Savings Plans** | up to ~72% | 1 or 3 yr, family-locked | Steady-state EC2 in a specific family |
| **Reserved Instances** | up to ~75% | 1 or 3 yr, AZ/region/family-locked | Legacy alternative to SPs; SPs are usually preferred |
| **Dedicated Hosts** | varies | Per-host | BYOL Windows, compliance |

**Practical mix** for a typical SaaS: baseline on SP/RI, burst on On-Demand, batch/CI on Spot.

### Spot
```bash
aws ec2 run-instances --instance-market-options 'MarketType=spot,SpotOptions={MaxPrice=0.05}' ...
```
EC2 Auto Scaling Groups support **mixed instance policies** — e.g., 30% on-demand, 70% spot, across multiple instance types and AZs. The diversification protects against spot interruptions.

---

## 9. Common Mistakes & Gotchas

- **IMDSv1 still enabled.** SSRF → credential theft. Always launch with `HttpTokens=required`.
- **`0.0.0.0/0` on port 22.** Use Session Manager or restrict to your bastion/VPN IP.
- **Saving the .pem file in git.** Worst case scenario. AWS will detect and notify you, but the cat's out of the bag.
- **Forgetting `DeleteOnTermination`** on EBS volumes. Terminated instance, orphaned volume, $$$ for years.
- **Snapshots forever.** Snapshots are incremental but bill per-GB-month. Set lifecycle policies.
- **Wrong AZ for an EBS volume.** Can't attach `vol-xxx` in `ap-southeast-1a` to an instance in `ap-southeast-1b` — snapshot and restore.
- **Single ASG in a single AZ.** Outage takes the whole tier.
- **ASG health-check-type=EC2 only.** Misses app crashes that don't kill the OS. Use `ELB`.
- **No `HealthCheckGracePeriod`.** ASG kills instances mid-boot. Set 60-300s based on bootstrap time.
- **Launching `t3` in unlimited mode** without realizing — credits exhausted, surprise CPU bill. Default is unlimited; flip to standard for predictability.
- **Mixing Graviton AMI with x86 instance type.** Boot fails silently or in confusing ways.
- **Spot without diversification.** All instances of one type in one AZ get pulled at once. Use 4+ instance types across 2+ AZs.
- **No tags on instances.** Cost Explorer is useless. Tag with `project`, `env`, `owner`, `cost-center`. Enforce with SCPs.
- **Public IPs auto-assigned in private subnets.** Possible if `MapPublicIpOnLaunch=true` on the subnet — confusing.
- **Forgetting metadata-options on a launch template.** ASG-launched instances run with IMDSv1.
- **GPU instances on by mistake.** A `p4d.24xlarge` is $33/hr. A weekend of forgetfulness = a car payment.

---

## 🎯 Key Takeaways

- **Default to Graviton** (`t4g`, `m7g`, `c7g`, `r7g`). 20-40% cheaper/faster per dollar than equivalent x86 unless you're locked to x86 binaries.
- **Session Manager replaces SSH** in production — no port 22, no key management, full audit log. Combine with private subnets + interface endpoints.
- **gp3 is the new default volume.** Switch any gp2 you find — same or better performance, ~20% cheaper, IOPS/throughput dialable independently.
- **Auto Scaling done right requires both ELB health checks and lifecycle hooks** — without these, you get either zombie instances stuck in load balancers or in-flight requests dropped mid-deploy.
- **Capacity strategy = mix.** Steady-state on Savings Plans, burst on On-Demand, batch/stateless on Spot with instance/AZ diversification. This single decision can cut compute bills by 50%+.

*← [prev](./03_vpc_networking.md) | [next →](./05_s3_object_storage.md)*
