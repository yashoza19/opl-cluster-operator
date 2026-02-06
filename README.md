# OPL Cluster Operator

A Kubernetes operator that automates OpenShift cluster provisioning through GitOps. This operator watches `ClusterRequest` custom resources, generates cluster configurations, commits them to Git, and monitors Hive-based cluster provisioning.

## Features

- **Declarative API**: Define cluster requests as Kubernetes custom resources
- **Database Sync Controller**: Automatically syncs cluster requests from PostgreSQL database
- **GitOps Integration**: Automatically commits cluster configs to Git repository
- **State Management**: Full state machine from pending → complete/failed
- **ArgoCD Sync**: Generated configs are automatically synced by ArgoCD
- **Hive Provisioning**: Integrates with Hive for actual cluster creation on AWS
- **Size Presets**: Pre-configured cluster sizes (small, medium, large, xl)
- **Smart Mapping**: Automatic instance type, region, and version mapping
- **Secret Management**: Automatic secret copying to cluster namespaces
- **Retry Logic**: Automatic retry on failures (up to 3 attempts)

## Quick Start

### Prerequisites

- Kubernetes/OpenShift cluster (v1.25+)
- Hive operator installed
- ArgoCD installed and configured
- Git repository for cluster configurations
- SSH key with write access to Git repo

### Installation

```bash
# Install CRDs
make install

# Create SSH secret for Git access
kubectl create secret generic git-ssh-key \
  --from-file=id_rsa=$HOME/.ssh/id_rsa \
  -n opl-cluster-operator-system

# Create database secret (optional, for database sync feature)
kubectl create secret generic database-sync-secret \
  --from-literal=host=your-postgres-host \
  --from-literal=port=5432 \
  --from-literal=database=your-database \
  --from-literal=username=your-username \
  --from-literal=password=your-password \
  -n opl-cluster-operator-system

# Deploy the operator
make deploy IMG=quay.io/yoza/opl-cluster-operator:latest

# Create required secrets in default namespace (will be copied to cluster namespaces)
kubectl create secret generic pull-secret --from-file=.dockerconfigjson=$HOME/.docker/config.json --type=kubernetes.io/dockerconfigjson
kubectl create secret generic ssh-key --from-file=ssh-privatekey=$HOME/.ssh/id_rsa --type=kubernetes.io/ssh-auth
kubectl create secret generic aws-credentials --from-literal=aws_access_key_id=YOUR_KEY --from-literal=aws_secret_access_key=YOUR_SECRET
```

### Create a Cluster

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
```

Apply and approve:
```bash
kubectl apply -f cluster-request.yaml
kubectl patch clusterrequest dev-cluster-01 --subresource=status --type=merge -p '{"status":{"state":"approved"}}'
```

Monitor progress:
```bash
kubectl get clusterrequests
kubectl describe clusterrequest dev-cluster-01
```

## Environment Variables

### Core Operator

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GIT_REPO_URL` | ✅ | - | Git repository URL |
| `GIT_BRANCH` | ❌ | `main` | Git branch |
| `GIT_SSH_KEY_PATH` | ❌ | `/etc/git-secret/id_rsa` | SSH key path |
| `GIT_AUTHOR_NAME` | ❌ | `OPL Cluster Operator` | Commit author name |
| `GIT_AUTHOR_EMAIL` | ❌ | `cluster-operator@openshiftpartnerlabs.com` | Commit author email |

### Database Sync Controller (Optional)

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DB_HOST` | ✅ | - | PostgreSQL host (from secret) |
| `DB_PORT` | ✅ | `5432` | PostgreSQL port (from secret) |
| `DB_NAME` | ✅ | - | Database name (from secret) |
| `DB_USER` | ✅ | - | Database username (from secret) |
| `DB_PASSWORD` | ✅ | - | Database password (from secret) |
| `DB_SYNC_INTERVAL` | ❌ | `30s` | Sync interval (from configmap) |

## Architecture

### Direct API Flow
```
Buffalo App → ClusterRequest CR → Operator → Git Commit → ArgoCD → Hive → Cluster
```

### Database Sync Flow
```
Buffalo App → PostgreSQL → DB Sync Controller → ClusterRequest CR → Operator → Git Commit → ArgoCD → Hive → Cluster
```

**State Flow:**
```
pending → approved → git-committed → provisioning → complete/failed
```

## Implementation Status

### ✅ Completed
- ClusterRequest CRD with full spec and status
- Controller with state machine reconciliation
- Database sync controller for PostgreSQL integration
- Instance type, region, and version mapping utilities
- Git client with SSH/HTTPS authentication
- Template generator (kustomization, cluster-config, argocd-app)
- Cluster config mapper with size presets (small, medium, large, xl)
- Namespace and secret management
- Retry logic and error handling
- RBAC and deployment manifests
- Helper scripts for database management

### 🚧 Planned
- Hive watcher for status updates
- Slack notifications
- Comprehensive tests
- Metrics and observability

## Documentation

- **[Design Document](CLUSTER_OPERATOR_DESIGN.md)** - Complete architecture and implementation details
- **[Database Sync Design](DATABASE_SYNC_DESIGN.md)** - Database sync controller architecture
- **[Instance Type Selection](INSTANCE_TYPE_SELECTION.md)** - Instance type mapping logic
- **[Version Mapping](VERSION_MAPPING.md)** - OpenShift version mapping
- **[Deployment Notes](config/manager/DEPLOYMENT_NOTES.md)** - Database sync deployment guide
- **[Sample ClusterRequest](config/samples/opl_v1alpha1_clusterrequest.yaml)** - Example CR
- **[API Reference](api/v1alpha1/clusterrequest_types.go)** - CRD field definitions

## Development

### Build and Test Locally

```bash
# Download dependencies
go mod tidy

# Generate code
make generate

# Generate CRDs
make manifests

# Run tests
make test

# Build binary
make build

# Run locally (requires kubeconfig)
export GIT_REPO_URL="git@github.com:yashoza19/opl-argocd.git"
export GIT_SSH_KEY_PATH="$HOME/.ssh/id_rsa"
make run
```

### Build Docker Image

```bash
make docker-build docker-push IMG=<registry>/opl-cluster-operator:tag
```

### Uninstall

```bash
# Delete ClusterRequests
kubectl delete clusterrequests --all

# Uninstall operator
make undeploy

# Remove CRDs
make uninstall
```

## Project Structure

```
.
├── api/v1alpha1/              # CRD definitions
├── cmd/                       # Main entry point
├── config/                    # Kubernetes manifests
│   ├── crd/                   # CRD YAML files
│   ├── rbac/                  # RBAC manifests
│   ├── manager/               # Operator deployment & DB sync config
│   └── samples/               # Example ClusterRequests
├── internal/
│   ├── controller/            # Reconciliation logic
│   │   ├── clusterrequest_controller.go
│   │   └── dbsync_controller.go
│   ├── database/              # Database client and models
│   ├── git/                   # Git client
│   ├── templates/             # Template generator
│   └── mappers/               # Config mappers (cluster, instance, region, version)
├── hack/                      # Helper scripts
└── test/                      # Tests
```

## Contributing

Contributions welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

Apache License 2.0 - See LICENSE file for details

## Links

- **Repository**: https://github.com/yashoza19/opl-cluster-operator
- **Issues**: https://github.com/yashoza19/opl-cluster-operator/issues
- **ArgoCD Repo**: https://github.com/yashoza19/opl-argocd

---

**Version**: v0.2.3
**Status**: Core implementation and database sync integration complete
