# OpenShift Version Mapping

This document explains how the operator maps OpenShift version strings to ClusterImageSet references.

## Overview

In Hive-based cluster provisioning, the OpenShift version is specified via a ClusterImageSet resource. The operator automatically maps user-friendly version strings from the database to the corresponding ClusterImageSet names.

## Mapping Process

When a ClusterRequest is created:

1. **Database Version** - The version string from the database (e.g., "4.20.10", "4.20")
2. **ImageSetRef** - Mapped to a ClusterImageSet name (e.g., "img4.20.10-x86-64-appsub")
3. **Semantic Version** - Extracted semantic version (e.g., "4.20.10")

## Supported Version Mappings

| Database Version | ClusterImageSet Ref | Semantic Version | Description |
|------------------|---------------------|------------------|-------------|
| `4.20.10` | `img4.20.10-x86-64-appsub` | `4.20.10` | OpenShift 4.20.10 |
| `4.20.9` | `img4.20.9-x86-64-appsub` | `4.20.9` | OpenShift 4.20.9 |
| `4.20` | `img4.20.10-x86-64-appsub` | `4.20.10` | OpenShift 4.20 (latest) |
| `4.19.15` | `img4.19.15-x86-64-appsub` | `4.19.15` | OpenShift 4.19.15 |
| `4.19` | `img4.19.15-x86-64-appsub` | `4.19.15` | OpenShift 4.19 (latest) |

## Automatic Mapping

If a version is not in the predefined mapping table, the operator uses intelligent defaults:

### Already an ImageSet Reference
If the version string already looks like a ClusterImageSet name (starts with "img"):
```
Input: "img4.21.0-x86-64-appsub"
Output: Uses as-is → "img4.21.0-x86-64-appsub"
```

### Version Number Format
If the version is a semantic version number (contains dots and digits):
```
Input: "4.21.5"
Output: Constructs → "img4.21.5-x86-64-appsub"
```

### Pass-Through
For any other format, the operator uses the value as-is.

## Examples

### Example 1: Using a Predefined Version
```yaml
spec:
  openshiftVersion: "4.20.10"
# Result:
# - ImageSetRef: img4.20.10-x86-64-appsub
# - Semantic Version: 4.20.10
```

### Example 2: Using Major.Minor Version
```yaml
spec:
  openshiftVersion: "4.20"
# Result:
# - ImageSetRef: img4.20.10-x86-64-appsub (maps to latest 4.20.x)
# - Semantic Version: 4.20.10
```

### Example 3: Using Direct ImageSet Reference
```yaml
spec:
  openshiftVersion: "img4.21.0-x86-64-appsub"
# Result:
# - ImageSetRef: img4.21.0-x86-64-appsub (uses as-is)
# - Semantic Version: 4.21.0 (extracted)
```

### Example 4: New Version Not in Mapping
```yaml
spec:
  openshiftVersion: "4.22.3"
# Result:
# - ImageSetRef: img4.22.3-x86-64-appsub (auto-constructed)
# - Semantic Version: 4.22.3
```

## ClusterImageSet Resources

ClusterImageSet resources must exist in the cluster before provisioning. The operator references these by name but does not create them.

### Example ClusterImageSet
```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterImageSet
metadata:
  name: img4.20.10-x86-64-appsub
spec:
  releaseImage: quay.io/openshift-release-dev/ocp-release:4.20.10-x86_64
```

## Adding New Versions

To add support for a new OpenShift version:

1. **Create ClusterImageSet** - Ensure the ClusterImageSet resource exists in the cluster
2. **Add to Mapping** (optional) - Add an entry to `version_mapper.go` if you want a friendly alias:
   ```go
   {
       DatabaseVersion:  "4.21",
       ImageSetRef:      "img4.21.5-x86-64-appsub",
       OpenshiftVersion: "4.21.5",
       Description:      "OpenShift 4.21 (latest)",
   }
   ```
3. **Or Use Auto-Mapping** - The operator will automatically construct the ImageSet name for semantic versions

## Database Integration

When syncing from the database, the `openshift_version` column value is used:

```sql
SELECT openshift_version FROM labs WHERE id = 123;
-- Result: "4.20.10" or "img4.20.10-x86-64-appsub"
```

The operator handles both formats automatically.

## Code References

- Version mapper: [internal/mappers/version_mapper.go](internal/mappers/version_mapper.go)
- Cluster config mapper: [internal/mappers/cluster_config.go](internal/mappers/cluster_config.go)
- Template generation: [internal/templates/generator.go](internal/templates/generator.go)
