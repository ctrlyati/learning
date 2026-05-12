# 05 — Virtual Machines, Disks, and VMSS

> **Goal:** Pick the right VM SKU, attach the right disks, and stamp out fleets with Virtual Machine Scale Sets — with the right zone/availability strategy and without lighting cost on fire.

## 1. The VM as a composition — not one resource, several

An Azure VM is not a single resource. Creating one creates at least five ARM resources, all linked:

```
Microsoft.Compute/virtualMachines
        │
        ├─► Microsoft.Compute/disks (OS disk; one per VM)
        ├─► Microsoft.Compute/disks (data disks; 0..N)
        ├─► Microsoft.Network/networkInterfaces
        │       └─► Microsoft.Network/publicIPAddresses (optional)
        │       └─► Microsoft.Network/networkSecurityGroups (optional, at NIC scope)
        └─► (no boot diagnostics storage in modern setups; managed boot diag uses platform)
```

Each is independently lifecycled. You can detach a data disk, attach it to a different VM, and reattach later. Deleting the VM doesn't automatically delete its disks unless you opt in (`--os-disk-delete-option Delete` and `--data-disk-delete-option Delete`).

Smallest possible creation:

```bash
az vm create \
  --resource-group rg-learn-azure \
  --name vm-demo-01 \
  --image Ubuntu2204 \
  --size Standard_B2s \
  --admin-username azureuser \
  --ssh-key-values @~/.ssh/id_ed25519.pub \
  --public-ip-sku Standard \
  --os-disk-delete-option Delete \
  --nic-delete-option Delete
```

The `--*-delete-option Delete` flags are the right default for dev work — fewer orphaned resources.

## 2. Sizes and series — the alphabet soup

VM sizes follow a pattern: `Standard_<family><sub-family><CPUs><version>` like `Standard_D8s_v5` or `Standard_E32ads_v6`. Decode:

- **Family** — what it's optimized for:
  - **A** — entry-level, dev/test.
  - **B** — burstable. Earn CPU credits while idle, burn when busy. The cheap-but-deceptive family.
  - **D** — general purpose. The default for stateless web tiers.
  - **E** — memory-optimized. For SQL, in-memory caches, JVMs.
  - **F** — compute-optimized (high CPU, lower RAM). Batch processing.
  - **L** — storage-optimized (NVMe). Cassandra, MongoDB.
  - **M** — massive memory (up to 12 TB). SAP HANA territory.
  - **N** — GPU (`NC` for compute, `ND` for deep learning, `NV` for visualization).
  - **H** — HPC (high CPU clocks, RDMA).
- **Sub-family letters:**
  - **s** — premium-storage capable.
  - **a** — AMD CPU (otherwise Intel).
  - **d** — local-disk (ephemeral) included.
  - **p** — Ampere ARM CPU.
- **Version** — `_v5`, `_v6`. Newer is faster per dollar; older may be only option in some regions.

Practical defaults:

| Workload | Start here |
|----------|------------|
| Small Linux web app | `Standard_B2ms` or `Standard_D2s_v5` |
| Java / .NET API tier | `Standard_D4s_v5` |
| SQL Server / Postgres | `Standard_E4ds_v5` (the `d` gives a local cache disk for tempdb) |
| Batch / build agent | `Standard_F8s_v2` |
| ARM-based experimentation | `Standard_D2ps_v5` |

List sizes available in a region:

```bash
az vm list-sizes --location eastus2 -o table | head -40
```

Cost-aware tip: `Standard_D2as_v5` (AMD) is typically ~10-15% cheaper than `Standard_D2s_v5` (Intel) with very similar performance for general workloads.

## 3. Disks, images, and ephemeral OS

### Disk SKUs

- **Standard HDD (`Standard_LRS`)** — slow, dev/test only.
- **Standard SSD (`StandardSSD_LRS`)** — entry production, dev VMs.
- **Premium SSD v1 (`Premium_LRS`)** — old default for prod.
- **Premium SSD v2 (`PremiumV2_LRS`)** — *the* modern default. Independently configurable IOPS and throughput. Cheaper than v1 at most points. Zone-deployable (no ZRS yet for v2).
- **Ultra Disk (`UltraSSD_LRS`)** — for "I need 160k IOPS and 4 GB/s" workloads. Niche.
- **Zone-redundant variants** — `ZRS` suffix (`Premium_ZRS`, `StandardSSD_ZRS`). Disk lives in all 3 zones. Required for zone-redundant VMs.

```bash
az disk create -g rg-learn-azure -n disk-app-data-01 \
  --size-gb 256 --sku PremiumV2_LRS \
  --disk-iops-read-write 5000 --disk-mbps-read-write 200 \
  --zone 1

az vm disk attach -g rg-learn-azure --vm-name vm-demo-01 --name disk-app-data-01 --lun 0
```

### Ephemeral OS disk

If your OS disk fits in the local SSD attached to the VM (`Standard_D*ds_v5` and similar), you can use an **ephemeral OS disk**: faster, free, but data is *lost* on stop/dealloc or host failure. Perfect for stateless app tiers behind a load balancer. `--ephemeral-os-disk true --ephemeral-os-disk-placement ResourceDisk`.

### Images

Three sources:

- **Marketplace** — official OS images (`Ubuntu2204`, `Win2022Datacenter`, `RHEL9`). Friendly aliases or full URN like `Canonical:0001-com-ubuntu-server-jammy:22_04-lts-gen2:latest`.
- **Azure Compute Gallery (formerly Shared Image Gallery)** — your own baked images, versioned, replicated to multiple regions. The right place for golden images.
- **VHDs** — bring-your-own. Legacy.

Bake an image with Packer or `az image builder`; publish to a Compute Gallery; have VMSS pull from it.

## 3.5. Availability — zones, sets, and beyond

Three constructs you'll see in deployment templates:

- **Availability Zones** — modern. Spread VMs across 1/2/3 with `--zone N`. SLA: 99.99% for VMs across ≥2 zones.
- **Availability Sets** — legacy. Used in regions without zones, or for fault-domain/update-domain semantics. Don't pick this if zones are available.
- **VMSS Flexible orchestration** — the modern way; supersedes both for fleets.

```bash
az vm create -n vm-app-01 -g rg-app --image Ubuntu2204 --size Standard_D2s_v5 --zone 1
az vm create -n vm-app-02 -g rg-app --image Ubuntu2204 --size Standard_D2s_v5 --zone 2
az vm create -n vm-app-03 -g rg-app --image Ubuntu2204 --size Standard_D2s_v5 --zone 3
```

Behind a **Standard Load Balancer** (zone-redundant frontend), this gives you 99.99% SLA.

## 4. Virtual Machine Scale Sets — fleets done right

A **VMSS** is a managed group of identical VMs with autoscaling, rolling upgrades, and load-balancer integration. Two orchestration modes:

- **Uniform** — original. All instances identical. Faster scale, more constrained.
- **Flexible** — modern. Each instance is a full VM resource you can manipulate individually. Mix sizes and zones. Use Flexible for new builds.

```bash
az vmss create \
  --name vmss-web-prod \
  --resource-group rg-web-prod \
  --orchestration-mode Flexible \
  --image Ubuntu2204 \
  --instance-count 3 \
  --vm-sku Standard_D2s_v5 \
  --zones 1 2 3 \
  --vnet-name vnet-prod-eastus2 --subnet snet-app \
  --load-balancer lb-web-prod \
  --upgrade-policy-mode Rolling \
  --admin-username azureuser --ssh-key-values @~/.ssh/id_ed25519.pub
```

Add autoscale:

```bash
az monitor autoscale create \
  --resource-group rg-web-prod \
  --resource vmss-web-prod --resource-type Microsoft.Compute/virtualMachineScaleSets \
  --name autoscale-web --min-count 3 --max-count 10 --count 3

az monitor autoscale rule create \
  --resource-group rg-web-prod --autoscale-name autoscale-web \
  --condition "Percentage CPU > 70 avg 5m" --scale out 2

az monitor autoscale rule create \
  --resource-group rg-web-prod --autoscale-name autoscale-web \
  --condition "Percentage CPU < 30 avg 10m" --scale in 1
```

Health checks: pair with an Application Health Extension or a Load Balancer probe so unhealthy instances are auto-replaced. Critical for prod.

### Rolling upgrades

When you bump the model (new image version), VMSS rolls instances in batches respecting a `MaxBatchInstancePercent`, `MaxUnhealthyInstancePercent`, and `PauseTimeBetweenBatches`. Bake into your deploy pipeline so prod is updated without downtime.

## 4. Practical Application — production-ready web tier

Bicep for a three-zone VMSS Flex behind a zone-redundant Standard LB:

```bicep
param location string = resourceGroup().location
param adminPublicKey string
param vnetId string
param subnetName string = 'snet-app'

resource lb 'Microsoft.Network/loadBalancers@2023-09-01' = {
  name: 'lb-web-prod'
  location: location
  sku: { name: 'Standard' }
  properties: {
    frontendIPConfigurations: [{
      name: 'fe'
      zones: ['1','2','3']
      properties: { publicIPAddress: { id: pip.id } }
    }]
    backendAddressPools: [{ name: 'be-web' }]
    probes: [{
      name: 'http'
      properties: { protocol: 'Http', port: 80, requestPath: '/healthz', intervalInSeconds: 5, numberOfProbes: 2 }
    }]
    loadBalancingRules: [{
      name: 'http'
      properties: {
        frontendIPConfiguration: { id: resourceId('Microsoft.Network/loadBalancers/frontendIPConfigurations','lb-web-prod','fe') }
        backendAddressPool: { id: resourceId('Microsoft.Network/loadBalancers/backendAddressPools','lb-web-prod','be-web') }
        probe: { id: resourceId('Microsoft.Network/loadBalancers/probes','lb-web-prod','http') }
        protocol: 'Tcp', frontendPort: 80, backendPort: 80, idleTimeoutInMinutes: 4
      }
    }]
  }
}

resource pip 'Microsoft.Network/publicIPAddresses@2023-09-01' = {
  name: 'pip-web-prod'
  location: location
  sku: { name: 'Standard' }
  zones: ['1','2','3']
  properties: { publicIPAllocationMethod: 'Static' }
}

resource vmss 'Microsoft.Compute/virtualMachineScaleSets@2024-03-01' = {
  name: 'vmss-web-prod'
  location: location
  zones: ['1','2','3']
  sku: { name: 'Standard_D2s_v5', tier: 'Standard', capacity: 3 }
  properties: {
    orchestrationMode: 'Flexible'
    platformFaultDomainCount: 1
    upgradePolicy: { mode: 'Rolling' }
    virtualMachineProfile: {
      osProfile: {
        computerNamePrefix: 'web'
        adminUsername: 'azureuser'
        linuxConfiguration: {
          disablePasswordAuthentication: true
          ssh: { publicKeys: [{ path: '/home/azureuser/.ssh/authorized_keys', keyData: adminPublicKey }] }
        }
      }
      storageProfile: {
        imageReference: {
          publisher: 'Canonical', offer: '0001-com-ubuntu-server-jammy'
          sku: '22_04-lts-gen2', version: 'latest'
        }
        osDisk: { createOption: 'FromImage', managedDisk: { storageAccountType: 'Premium_LRS' }, diffDiskSettings: { option: 'Local' } }
      }
      networkProfile: {
        networkApiVersion: '2022-11-01'
        networkInterfaceConfigurations: [{
          name: 'nic-web'
          properties: {
            primary: true
            ipConfigurations: [{
              name: 'ipcfg'
              properties: {
                subnet: { id: '${vnetId}/subnets/${subnetName}' }
                loadBalancerBackendAddressPools: [{ id: resourceId('Microsoft.Network/loadBalancers/backendAddressPools','lb-web-prod','be-web') }]
              }
            }]
          }
        }]
      }
    }
  }
}
```

This is roughly the minimum acceptable production web tier — zonal, autoscale-ready, ephemeral OS for fast scale, behind a zone-redundant Standard LB. Layer Application Gateway or Front Door in front (module 11) for L7.

## 5. Common Mistakes & Gotchas

- **Picking B-series for prod.** Burstable VMs are great until they aren't. Once credits are exhausted, the CPU is throttled to baseline (often 10-20%). Acceptable for bursty internal tools; *never* for steady-state production.
- **Not zoning.** A single-zone VM has a 99.9% SLA. Two zones gets 99.99%. The price difference: a Standard LB. Free SLA upgrade.
- **Public IPs on VMs.** Should be rare. Use Bastion (module 04) for management, Load Balancer for ingress, NAT Gateway for outbound. Direct VM public IPs are a 2014 pattern.
- **Forgetting NAT Gateway for outbound.** With Standard LBs the default outbound SNAT is unreliable at scale (port exhaustion). Attach an `azure-nat-gateway` to outbound subnets. Cheap, deterministic.
- **Old VM series in interview-y regions.** `Standard_D2_v2` looks similar to `Standard_D2s_v5` but is *3x* the cost per CPU and slower. Always check `_v5`/`_v6`.
- **Ephemeral OS data loss.** If you `--ephemeral-os-disk true` and store state on `/`, a host reboot wipes it. Use only for stateless tiers.
- **VM identity vs VM admin credentials.** SSH/RDP credentials are for the OS. *Managed identity* is for the VM to call Azure. Don't conflate them.
- **Trusted Launch / Generation 2 confusion.** Most modern images are Gen2 / Trusted Launch. Some legacy images and some sizes (`*_v2` family) only support Gen1. Cross-checking image gen vs VM size has burned everyone.
- **Quota by family.** `vCPU quota` is per *family* per region, not global. You can have 100 D-family cores quota and zero E-family cores quota. Request specific increases.
- **Reserved Instances tied to the wrong scope.** A 1-yr `Standard_D4s_v5` RI bought at single-scope only applies to that sub; bought at shared scope it applies tenant-wide. Buy shared scope by default. (Module 17.)
- **VMSS Uniform vs Flexible deployment incompatibilities.** You cannot convert between modes. Pick Flexible for new builds.
- **Snapshot vs backup confusion.** A snapshot is a point-in-time copy of a single disk. Azure Backup is the orchestrated, retention-policy-driven, application-consistent solution. Don't rely on snapshots for DR.

## 🎯 Key Takeaways

- **A VM is several resources.** Treat disks and NICs as first-class — they outlive the VM by default unless you set `delete-option Delete`.
- **Size series matters.** Default to D-family for general workloads, E for memory, F for compute, `v5+` and `a` (AMD) for cost-perf.
- **Premium SSD v2 + Trusted Launch + Gen2 image + 3 zones + ephemeral OS** is the modern stateless tier default.
- **Use VMSS Flexible** for any fleet of 2+ identical VMs. It supersedes Availability Sets and Uniform VMSS.
- **Pair compute with Bastion (mgmt), Standard LB (ingress), NAT Gateway (egress)** — the standard four-resource recipe.

*← [prev](./04_networking.md) | [next → 06_storage.md](./06_storage.md)*
