# OPL Cluster Operator - Design & Architecture

## Executive Summary

A Kubernetes operator that automates OpenShift cluster provisioning through GitOps. The operator watches `ClusterRequest` custom resources, generates cluster configurations, commits them to Git, and monitors Hive-based provisioning status.

**Architecture Components:**
- **ClusterRequest CRD** - Declarative cluster specifications
- **Operator Controller** - Go-based reconciliation logic (Kubebuilder/controller-runtime)
- **Git Integration** - Automated commits to ArgoCD repository
- **Hive Integration** - OpenShift cluster provisioning on AWS
- **Buffalo App** - Creates ClusterRequest CRs when lab requests are approved

---

## System Architecture

```
┌────────────────────────────────────────────────────────────────┐
│        OpenShift Partner Labs Buffalo Application              │
│        (PostgreSQL-backed web app)                             │
│                                                                 │
│  Admin approves lab request → Create ClusterRequest CR         │
└────────────────────────────┬───────────────────────────────────┘
                             │
                             ▼
                  ┌──────────────────────┐
                  │   Kubernetes API     │
                  │  ClusterRequest CR   │
                  └──────────┬───────────┘
                             │
                             ▼ Watch & Reconcile
                  ┌──────────────────────┐
                  │  Cluster Operator    │
                  │  (Controller)        │
                  └──────────┬───────────┘
                             │
               ┌─────────────┴─────────────┐
               ▼                           ▼
    ┌──────────────────┐        ┌──────────────────┐
    │  Git Repository  │        │  Hive Watcher    │
    │  (opl-argocd)    │        │  (future)        │
    │                  │        │                  │
    │  clusters/       │        │  ClusterDep      │
    │    └─ name/      │        │  Status Monitor  │
    └────────┬─────────┘        └────────┬─────────┘
             │                           │
             ▼                           ▼
    ┌──────────────────┐        ┌──────────────────┐
    │     ArgoCD       │        │  Slack Notifier  │
    │   Auto-Sync      │        │  (future)        │
    └────────┬─────────┘        └──────────────────┘
             │
             ▼
    ┌──────────────────┐
    │      Hive        │
    │  Cluster Deploy  │
    └────────┬─────────┘
             │
             ▼
    ┌──────────────────┐
    │  OpenShift       │
    │  Cluster (AWS)   │
    └──────────────────┘
```

---

## ClusterRequest Custom Resource

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `clusterName` | string | ✅ | DNS-compatible cluster name (e.g., `dev-cluster-01`) |
| `environment` | string | ✅ | `development`, `staging`, or `production` |
| `openshiftVersion` | string | ✅ | OpenShift version image reference (e.g., `img4.20.10-x86-64-appsub`) |
| `clusterSize` | string | ✅ | `small`, `medium`, or `large` (determines instance types) |
| `region` | string | ✅ | AWS region (e.g., `us-east-1`) |
| `baseDomain` | string | ✅ | Base DNS domain (default: `openshiftpartnerlabs.com`) |
| `requestedBy` | string | ✅ | Email of requester |
| `companyName` | string | ❌ | Company/organization name |
| `labId` | int | ❌ | Reference to Buffalo app lab ID |
| `controlPlane` | object | ❌ | Control plane overrides (instanceType, replicas) |
| `workers` | object | ❌ | Worker node overrides (instanceType, replicas, zones) |
| `networking` | object | ❌ | Network CIDR overrides |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `state` | string | Current state: `pending`, `approved`, `git-committed`, `provisioning`, `complete`, `failed` |
| `gitCommitSHA` | string | Git commit SHA after files are committed |
| `argocdAppCreated` | bool | Whether ArgoCD Application was created |
| `hiveClusterID` | string | Hive ClusterDeployment UID |
| `clusterProvisioned` | bool | Whether provisioning completed successfully |
| `errorMessage` | string | Error details if state is `failed` |
| `conditions` | []Condition | Standard Kubernetes conditions |
| `processingStartedAt` | timestamp | When operator started processing |
| `processingCompletedAt` | timestamp | When processing finished |
| `retryCount` | int | Number of retry attempts (max 3) |

### Cluster Size Presets

| Size | Control Plane | Workers | Use Case |
|------|--------------|---------|----------|
| **small** | 3 × m5.xlarge | 2 × m5.2xlarge | Development/Testing |
| **medium** | 3 × m5.xlarge | 3 × m5.2xlarge | Staging/Demos |
| **large** | 3 × m5.2xlarge | 6 × m5.4xlarge | Production/Training |

---

## Controller State Machine

```
┌──────────┐
│ pending  │ ← Created by Buffalo app, awaiting approval
└────┬─────┘
     │ Buffalo app updates status.state = "approved"
     ▼
┌──────────┐
│ approved │ ← Controller generates configs, commits to Git
└────┬─────┘
     │ Git commit succeeds
     ▼
┌─────────────────┐
│ git-committed   │ ← ArgoCD syncs cluster configs
└────┬────────────┘
     │ Hive starts provisioning
     ▼
┌──────────────┐
│ provisioning │ ← Waiting for Hive to complete
└────┬─────────┘
     │ Hive finishes (success or failure)
     ▼
┌──────────┐     ┌──────────┐
│ complete │  or │ failed   │ ← Terminal states
└──────────┘     └──────────┘
                      │ Retry up to 3 times
                      └──► Back to "approved" if retryCount < 3
```

---

## Implementation Status

### ✅ Completed Components

#### 1. ClusterRequest CRD
- [x] Full CRD definition with spec and status
- [x] Kubebuilder validation markers
- [x] Printer columns for `kubectl get`
- [x] Status subresource
- [x] Nested configuration objects (ControlPlane, Workers, Networking)

#### 2. Controller Logic
- [x] State machine reconciliation loop
- [x] Finalizer support for cleanup
- [x] Condition management (Kubernetes-standard)
- [x] Retry logic with exponential backoff
- [x] Error handling and status updates
- [x] Namespace creation for clusters
- [x] Secret copying (pull-secret, ssh-key, aws-credentials)

#### 3. Git Integration
- [x] Git client package (`internal/git/client.go`)
  - SSH and HTTPS authentication
  - Clone, pull, commit, push operations
  - Creates `clusters/<cluster-name>/` directory structure
  - Cleanup support (DeleteCluster)

#### 4. Template Generator
- [x] Template generator package (`internal/templates/generator.go`)
- [x] Generates 3 files per cluster:
  - `kustomization.yaml` - Kustomize patches for ClusterDeployment, MachinePool, etc.
  - `cluster-config.yaml` - ConfigMap with human-readable cluster metadata
  - `argocd-application.yaml` - ArgoCD Application manifest

#### 5. Configuration Mapper
- [x] ClusterRequest → ClusterConfig mapper (`internal/mappers/cluster_config.go`)
- [x] Cluster size preset mapping
- [x] Instance type and replica overrides
- [x] Availability zone mapping
- [x] Network CIDR defaults and overrides

#### 6. Project Infrastructure
- [x] Kubebuilder project scaffolding
- [x] Makefile targets (build, deploy, install, etc.)
- [x] RBAC manifests
- [x] Sample ClusterRequest
- [x] Deployment manifests
- [x] OLM bundle generation

### 🚧 In Progress / Planned

- [ ] **Hive Watcher** - Monitor ClusterDeployment status and update ClusterRequest
- [ ] **Buffalo App Integration** - Create ClusterRequest CRs from Buffalo app
- [ ] **Database Sync** - Optional: sync ClusterRequest status back to PostgreSQL
- [ ] **Slack Notifications** - Send notifications on state changes
- [ ] **Webhooks** - Validating and mutating webhooks for ClusterRequest
- [ ] **Unit Tests** - Comprehensive controller tests
- [ ] **E2E Tests** - Full workflow integration tests
- [ ] **Metrics** - Prometheus metrics for monitoring
- [ ] **Multi-cloud Support** - Azure and GCP providers

---

## Generated File Structure

When a ClusterRequest is approved, the operator creates this Git structure:

```
clusters/
└── <cluster-name>/
    ├── kustomization.yaml          # Kustomize configuration
    ├── cluster-config.yaml         # ConfigMap with metadata
    └── argocd-application.yaml     # ArgoCD Application
```

### File Purposes

**kustomization.yaml**
- References base templates from `cluster-templates/aws-ha/base`
- Applies JSON patches to customize ClusterDeployment
- Sets namespace, labels, and configuration

**cluster-config.yaml**
- ConfigMap containing cluster metadata
- Human-readable reference for operators
- Can be mounted in pods that need cluster info

**argocd-application.yaml**
- ArgoCD Application manifest
- Points to the cluster directory
- Configures sync policy and ignore rules
- Prevents ArgoCD from deleting Hive-managed secrets

---

## Environment Variables

The operator requires these environment variables:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GIT_REPO_URL` | ✅ | - | Git repository URL (SSH or HTTPS) |
| `GIT_BRANCH` | ❌ | `main` | Git branch to commit to |
| `GIT_SSH_KEY_PATH` | ❌ | `/etc/git-secret/id_rsa` | Path to SSH private key |
| `GIT_LOCAL_PATH` | ❌ | `/tmp/opl-argocd` | Local clone directory |
| `GIT_AUTHOR_NAME` | ❌ | `OPL Cluster Operator` | Git commit author name |
| `GIT_AUTHOR_EMAIL` | ❌ | `cluster-operator@openshiftpartnerlabs.com` | Git commit author email |

---

## Deployment

### Prerequisites
- OpenShift/Kubernetes cluster
- Hive operator installed
- ArgoCD installed and configured
- Git repository with cluster templates
- SSH key with write access to Git repo

### Installation Steps

1. **Install CRDs**
   ```bash
   make install
   ```

2. **Create Git SSH Secret**
   ```bash
   kubectl create secret generic git-ssh-key \
     --from-file=id_rsa=/path/to/ssh/key \
     -n opl-cluster-operator-system
   ```

3. **Deploy Operator**
   ```bash
   make deploy IMG=quay.io/yoza/opl-cluster-operator:latest
   ```

4. **Create Secrets in Default Namespace**
   ```bash
   # These will be copied to each cluster namespace
   kubectl create secret generic pull-secret --from-file=...
   kubectl create secret generic ssh-key --from-file=...
   kubectl create secret generic aws-credentials --from-literal=...
   ```

### Creating a ClusterRequest

```yaml
apiVersion: opl.openshiftpartnerlabs.com/v1alpha1
kind: ClusterRequest
metadata:
  name: dev-cluster-01
  namespace: default
spec:
  clusterName: dev-cluster-01
  environment: development
  openshiftVersion: img4.20.10-x86-64-appsub
  clusterSize: small
  region: us-east-1
  baseDomain: openshiftpartnerlabs.com
  requestedBy: user@example.com
  companyName: Example Corp
```

Approve it by updating the status:
```bash
kubectl patch clusterrequest dev-cluster-01 \
  --subresource=status \
  --type=merge \
  -p '{"status":{"state":"approved"}}'
```

---

## Workflow

### End-to-End Flow

1. **User Request** → Buffalo web UI
2. **Admin Approval** → Buffalo updates database + creates ClusterRequest CR
3. **CR Created** → Status: `pending` (waiting for explicit approval flag)
4. **Buffalo Updates CR** → Status: `approved`
5. **Operator Reconciles:**
   - Creates cluster namespace
   - Copies secrets (pull-secret, ssh-key, aws-credentials)
   - Maps ClusterRequest spec → ClusterConfig
   - Generates templates (kustomization.yaml, cluster-config.yaml, argocd-application.yaml)
   - Commits to Git: `clusters/<cluster-name>/`
   - Updates status: `git-committed`
6. **ArgoCD Detects** → Auto-syncs cluster directory
7. **Hive Provisions** → Creates ClusterDeployment
8. **Cluster Ready** → Hive updates ClusterDeployment status
9. **Hive Watcher (future)** → Updates ClusterRequest status: `complete`
10. **Buffalo Polls** → Displays cluster status to users

---

## Integration Points

### Buffalo Application

**Minimal changes required:**

1. Add Kubernetes client library
2. Create ClusterRequest CR when admin approves lab
3. Poll ClusterRequest status for UI updates
4. Optionally: Delete ClusterRequest when lab expires

### Git Repository (opl-argocd)

**Required structure:**

```
opl-argocd/
├── cluster-templates/
│   └── aws-ha/
│       └── base/
│           ├── cluster-deployment.yaml
│           ├── machine-pool.yaml
│           ├── managed-cluster.yaml
│           └── klusterlet-addon-config.yaml
└── clusters/
    ├── cluster-01/    # Created by operator
    ├── cluster-02/    # Created by operator
    └── ...
```

### ArgoCD Configuration

**Required AppProject:**

```yaml
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: cluster-provisioning
  namespace: openshift-gitops
spec:
  sourceRepos:
    - 'https://github.com/yashoza19/opl-argocd.git'
  destinations:
    - namespace: '*'
      server: 'https://kubernetes.default.svc'
  clusterResourceWhitelist:
    - group: '*'
      kind: '*'
```

**Auto-discovery:** ArgoCD should be configured to auto-discover Applications in the Git repo.

---

## RBAC Requirements

The operator service account needs these permissions:

- **ClusterRequests**: Full access (get, list, watch, create, update, patch, delete, status)
- **Namespaces**: Create, get, list, watch, update
- **Secrets**: Create, get, list, watch, update (for copying secrets)
- **ClusterDeployments** (Hive): Get, list, watch (for future Hive watcher)

---

## Advantages of This Architecture

✅ **Declarative** - Cluster requests are Kubernetes resources
✅ **GitOps Native** - All cluster configs versioned in Git
✅ **Event-Driven** - Immediate reconciliation (no polling delays)
✅ **Standard Tools** - Works with `kubectl`, `oc`, `k9s`, etc.
✅ **Extensible** - Easy to add webhooks, policies, quotas
✅ **RBAC Compatible** - Kubernetes-native access control
✅ **Scalable** - Controller-runtime handles watch efficiency
✅ **Auditable** - Git history provides full audit trail

---

## Next Steps

1. **Test End-to-End Workflow**
   - Deploy operator to cluster
   - Create sample ClusterRequest
   - Verify Git commit
   - Verify ArgoCD sync
   - Verify Hive provisioning

2. **Implement Hive Watcher**
   - Watch ClusterDeployment resources
   - Update ClusterRequest status based on Hive conditions
   - Handle provisioning failures

3. **Buffalo App Integration**
   - Add Kubernetes client to Buffalo
   - Create ClusterRequest on approval
   - Poll status for UI updates

4. **Production Readiness**
   - Add comprehensive tests
   - Set up CI/CD pipeline
   - Create operator bundle for OperatorHub
   - Document runbooks for operations

---

**Document Version:** 4.0 (Implementation-Updated)
**Last Updated:** 2026-02-02
**Status:** Core implementation complete, integrations pending
**Repository:** https://github.com/yashoza19/opl-cluster-operator
