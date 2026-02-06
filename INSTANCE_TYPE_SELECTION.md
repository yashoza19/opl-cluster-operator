# Instance Type Selection Logic

This document explains how the operator selects EC2 instance types and replica counts for clusters based on various configuration sources.

## Core Principles

1. **Cluster Size determines Base Configuration** - The cluster size (small, medium, large, xl) determines both instance types AND replica counts
2. **Request Type can Override Instance Types** - Special workload types (ocpv, rhoai, nvidia) override instance types for specific hardware requirements
3. **Explicit Spec Overrides Everything** - Explicit values in the ClusterRequest spec always take highest priority

## Quick Reference Table

| Field | Controls | Values | Example |
|-------|----------|--------|---------|
| `clusterSize` | **Instance Types & Replicas** | `small`, `medium`, `large`, `xl` | `medium` → m7i.xlarge, 3 masters, 3 workers |
| `requestType` | **Override for Special Hardware** | `ocpv`, `rhoai`, `nvidia` | `ocpv` → m6a.metal (bare metal) |
| `spec.controlPlane.instanceType` | **Override CP Instance** | Any EC2 type | `m5.4xlarge` |
| `spec.workers.instanceType` | **Override Worker Instance** | Any EC2 type | `g5.4xlarge` |
| `spec.controlPlane.replicas` | **Override CP Replicas** | Integer | `5` |
| `spec.workers.replicas` | **Override Worker Replicas** | Integer | `10` |

## Selection Priority

### Instance Types (Highest to Lowest)
1. **Explicit CR Spec Override** - `spec.controlPlane.instanceType` / `spec.workers.instanceType`
2. **Request Type Override** - Only for special workloads (ocpv, rhoai, nvidia)
3. **Cluster Size** - Base instance types from cluster size
4. **Default Fallback** - Medium cluster size (m7i.xlarge)

### Replica Counts (Highest to Lowest)
1. **Explicit CR Spec Override** - `spec.controlPlane.replicas` / `spec.workers.replicas`
2. **Cluster Size** - Replica counts from cluster size
3. **Default Fallback** - Medium cluster size (3 control plane, 3 workers)

## Cluster Size Configurations

Cluster size determines the BASE instance types and replica counts:

### Small Cluster
- **Instance Type:** `m7i.large`
- **Control Plane:** 3 replicas
- **Worker:** 3 replicas
- **Resources:** Standard 3 masters, 3 workers

### Medium Cluster
- **Instance Type:** `m7i.xlarge`
- **Control Plane:** 3 replicas
- **Worker:** 3 replicas
- **Resources:** Standard 3 masters, 3 workers

### Large Cluster
- **Instance Type:** `m7i.2xlarge`
- **Control Plane:** 3 replicas
- **Worker:** 3 replicas
- **Resources:** Standard 3 masters, 3 workers

### XL Cluster
- **Instance Type:** `m7i.4xlarge`
- **Control Plane:** 3 replicas
- **Worker:** 3 replicas
- **Resources:** 16 vCPU, 64GB RAM per node - Standard 3 masters, 3 workers

## Request Type Instance Overrides

The `requestType` field OVERRIDES cluster size instance types for special hardware requirements:

### OCPV (OpenShift Virtualization)
- **Request Type:** `ocpv`
- **Control Plane:** `m6a.metal` (Bare metal)
- **Worker:** `m6a.metal` (Bare metal)
- **Resources:** 96 vCPU, 192GB RAM, 500GB Storage
- **Description:** Bare metal instances required for nested virtualization

### RHOAI / NVIDIA (GPU Workloads)
- **Request Types:** `rhoai`, `nvidia`
- **Control Plane:** `m7i.xlarge` (General purpose)
- **Worker:** `g5.2xlarge` (NVIDIA A10G GPU)
- **Description:** GPU instances for AI/ML workloads and NVIDIA CUDA applications

### Engineering / General (No Override)
- **Request Types:** `engineering`, `general`
- **Behavior:** Uses cluster size instance types (no override)
- **Description:** Standard workloads use cluster size configuration

## Examples

### Example 1: Standard Small Cluster (No Request Type)
```yaml
spec:
  clusterSize: small
# Result:
# - Instance Types: m7i.large control plane and workers (from cluster size)
# - Replicas: 3 control plane, 3 workers (from cluster size)
```

### Example 2: Medium Cluster for Engineering
```yaml
spec:
  clusterSize: medium
  requestType: engineering
# Result:
# - Instance Types: m7i.xlarge control plane and workers (from cluster size)
# - Replicas: 3 control plane, 3 workers (from cluster size)
# - Note: engineering request type doesn't override instance types
```

### Example 3: Large Cluster with OCPV (Bare Metal Override)
```yaml
spec:
  clusterSize: large
  requestType: ocpv
# Result:
# - Instance Types: m6a.metal control plane and workers (OVERRIDDEN by request type)
# - Replicas: 3 control plane, 3 workers (from cluster size)
# - Resources: 96 vCPU, 192GB RAM per node
```

### Example 4: XL Cluster with GPU for AI
```yaml
spec:
  clusterSize: xl
  requestType: rhoai
# Result:
# - Instance Types: m7i.xlarge control plane (OVERRIDDEN), g5.2xlarge workers (OVERRIDDEN with GPU)
# - Replicas: 3 control plane, 3 workers (from cluster size)
```

### Example 5: Explicit Instance Type Override
```yaml
spec:
  clusterSize: medium
  requestType: ocpv
  workers:
    instanceType: m6a.metal
    replicas: 6  # Override worker replicas
# Result:
# - Instance Types: m6a.metal control plane (from request type), m6a.metal workers (explicit spec)
# - Replicas: 3 control plane (from cluster size), 6 workers (OVERRIDDEN by explicit spec)
```

## Region Mapping

Database regions are automatically mapped to AWS regions:

| Database Region | AWS Region | Description |
|----------------|------------|-------------|
| `na1` | `us-east-1` | Eastern Time Zone |
| `na2` | `us-central-1` | Central Time Zone |
| `na3` | `us-west-1` | Pacific Time Zone |
| `apac` | `ap-southeast-1` | Asia Pacific |
| `emea` | `eu-west-1` | Europe, Middle East, Africa |

Availability zones are automatically generated based on the AWS region (e.g., `us-east-1a`, `us-east-1b`, `us-east-1c`).

## Related Documentation

- **[Version Mapping](VERSION_MAPPING.md)** - OpenShift version to ClusterImageSet mapping

## Code References

- Request type mappings: [internal/mappers/instance_types.go](internal/mappers/instance_types.go)
- Cluster size mappings: [internal/mappers/instance_types.go](internal/mappers/instance_types.go)
- Selection logic: [internal/mappers/cluster_config.go](internal/mappers/cluster_config.go)
- Region mapping: [internal/mappers/region_mapper.go](internal/mappers/region_mapper.go)
- Version mapping: [internal/mappers/version_mapper.go](internal/mappers/version_mapper.go)
