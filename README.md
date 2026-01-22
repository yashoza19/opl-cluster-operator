# OpenShift Partner Labs - Cluster Operator

A Kubernetes operator that automates OpenShift cluster provisioning through a GitOps workflow. This operator watches `ClusterRequest` custom resources, generates cluster configurations, commits them to Git, and monitors Hive-based provisioning.

## Description

This operator implements an automated cluster provisioning pipeline that integrates with:
- **Buffalo Web Application** - Creates ClusterRequest CRs when lab requests are approved
- **Git Repository** - Stores cluster configurations (opl-argocd repo)
- **ArgoCD** - Syncs cluster configurations to deploy Hive resources
- **Hive** - Provisions actual OpenShift clusters on AWS
- **ACM/MCE** - Manages provisioned clusters

### Architecture

```
Buffalo App → ClusterRequest CR → Operator → Git Commit → ArgoCD → Hive → OpenShift Cluster
    ↓                                                                          ↓
PostgreSQL DB ← Status Updates ←──────────────────── Hive Watcher ←──────────┘
```

### State Machine

ClusterRequests progress through these states:
1. **pending** - Created by Buffalo app, awaiting approval
2. **approved** - Approved by admin, ready for processing
3. **git-committed** - Cluster config committed to Git
4. **provisioning** - Hive is provisioning the cluster
5. **complete** - Cluster successfully provisioned
6. **failed** - Provisioning failed (retries up to 3 times)

## ClusterRequest Custom Resource

### Spec Fields

```yaml
apiVersion: opl.openshiftpartnerlabs.com/v1alpha1
kind: ClusterRequest
metadata:
  name: dev-cluster-01
  namespace: cluster-provisioning
spec:
  # Required fields
  clusterName: dev-cluster-01              # DNS-compatible cluster name
  environment: development                 # development | staging | production
  openshiftVersion: img4.20.10-x86-64-appsub
  clusterSize: small                       # small | medium | large
  region: us-east-1
  baseDomain: openshiftpartnerlabs.com
  requestedBy: user@example.com

  # Optional company info
  companyName: Example Corp
  labId: 123                               # Reference to Buffalo app lab ID

  # Optional overrides (auto-configured based on clusterSize if not specified)
  controlPlane:
    instanceType: m5.xlarge
    replicas: 3

  workers:
    instanceType: m5.2xlarge
    replicas: 3
    zones:
      - us-east-1a

  networking:
    clusterNetworkCIDR: 10.128.0.0/14
    serviceNetworkCIDR: 172.30.0.0/16
    machineNetworkCIDR: 10.0.0.0/16
```

### Cluster Size Mappings

| Size   | Control Plane      | Workers         | Use Case           |
|--------|-------------------|-----------------|--------------------|
| small  | m5.xlarge x 3     | m5.2xlarge x 3  | Development/testing|
| medium | m5.xlarge x 3     | m5.2xlarge x 5  | Staging            |
| large  | m5.2xlarge x 3    | m5.4xlarge x 7  | Production         |

### Status Fields

```yaml
status:
  state: provisioning                    # Current state
  gitCommitSHA: abc123def                # Git commit SHA
  argocdAppCreated: true                 # ArgoCD app created
  hiveClusterID: cluster-abc123          # Hive cluster ID
  hiveInfraID: infra-abc123             # AWS infrastructure ID
  clusterProvisioned: false              # Provisioning complete
  errorMessage: ""                       # Error if failed
  processingStartedAt: "2026-01-22T10:00:00Z"
  retryCount: 0

  conditions:
    - type: GitCommitted
      status: "True"
      reason: GitCommitted
      message: Cluster configuration committed to Git
    - type: ArgocdAppCreated
      status: "True"
      reason: AppCreated
      message: ArgoCD Application created
    - type: HiveProvisioning
      status: Unknown
      reason: Provisioning
      message: Waiting for Hive provisioning
```

## Implementation Status

### ✅ Completed

- [x] ClusterRequest CRD with comprehensive spec and status fields
- [x] Kubebuilder validation markers and printer columns
- [x] Controller state machine (pending → approved → git-committed → provisioning → complete)
- [x] Finalizer support for cleanup
- [x] Condition management
- [x] Retry logic (max 3 attempts)
- [x] Sample ClusterRequest

### 🚧 In Progress / TODO

- [ ] **Git Client** - Generate cluster configs and commit to opl-argocd repo
  - Template generator for kustomization.yaml, cluster-config.yaml, argocd-application.yaml
  - Git operations (clone, commit, push)
  - SSH key authentication
- [ ] **Hive Watcher** - Monitor ClusterDeployment status
  - Watch Hive ClusterDeployment resources
  - Update ClusterRequest status based on provisioning state
  - Extract cluster metadata (clusterID, infraID)
- [ ] **Buffalo App Integration** - Bridge between PostgreSQL and Kubernetes
  - Create ClusterRequest CRs on lab approval
  - Update database with CR status
- [ ] **Slack Notifications** - Notify on state changes
- [ ] **OLM Bundle** - Package operator for OpenShift OperatorHub
- [ ] **Unit Tests** - Controller logic tests
- [ ] **E2E Tests** - Full workflow tests

## Getting Started

### Prerequisites
- go version v1.25.0+
- operator-sdk v1.42.0+
- docker version 17.03+
- kubectl version v1.11.3+
- Access to an OpenShift cluster with Hive and ArgoCD installed

### Development Setup

1. **Clone the repository**
   ```bash
   git clone https://github.com/openshift-partner-labs/opl-cluster-operator.git
   cd opl-cluster-operator
   ```

2. **Install dependencies**
   ```bash
   go mod tidy
   ```

3. **Generate CRD manifests**
   ```bash
   make manifests
   ```

4. **Run locally against a cluster**
   ```bash
   make install  # Install CRDs
   make run      # Run controller locally
   ```

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/opl-cluster-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/opl-cluster-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/opl-cluster-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/opl-cluster-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
operator-sdk edit --plugins=helm/v1-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

