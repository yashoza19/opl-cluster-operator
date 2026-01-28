# Cluster Provisioning Operator - Design Document (CRD-Based)

## Executive Summary

This document describes a **true Kubernetes operator** that automates OpenShift cluster provisioning via ArgoCD. The operator manages a custom `ClusterRequest` CRD and integrates with the existing **OpenShift Partner Labs** Buffalo application.

**Architecture:**
- **Custom Resource:** `ClusterRequest` CRD for declarative cluster specifications
- **Operator:** Go-based controller using Operator SDK/Kubebuilder
- **Integration:** Buffalo app creates `ClusterRequest` CRs (minimal code change)
- **Output:** Automated GitOps-based cluster provisioning

---

## System Architecture

```
┌───────────────────────────────────────────────────────────────────┐
│          Existing: OpenShift Partner Labs Application             │
│                    (Buffalo/Go - Minimal Changes)                  │
│                                                                    │
│  ┌──────────┐         ┌────────────────┐         ┌──────────┐   │
│  │  Web UI  │────────→│   PostgreSQL   │────────→│   REST   │   │
│  │  (Plush) │         │  labs table    │         │   API    │   │
│  └──────────┘         └────────────────┘         └─────┬────┘   │
│                                                         │         │
│                     MODIFIED: On approval, create CR ──┘         │
└──────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼ kubectl apply -f ClusterRequest
                         ┌─────────────────────┐
                         │   Kubernetes API    │
                         │   ClusterRequest CR │
                         └─────────────────────┘
                                    │
                                    ▼ Watch
                         ┌─────────────────────┐
                         │  Cluster Operator   │
                         │  (Go/controller-    │
                         │   runtime)          │
                         └─────────────────────┘
                                    │
                         ┌──────────┴──────────┐
                         ▼                     ▼
                 ┌──────────────┐      ┌──────────────┐
                 │  Git Repo    │      │ Hive Watcher │
                 │              │      │              │
                 │  clusters/   │      │ ClusterDep   │
                 │  └─ new-*/   │      │ Status       │
                 └──────┬───────┘      └──────┬───────┘
                        │                     │
                        ▼                     ▼
                 ┌──────────────┐      ┌──────────────┐
                 │   ArgoCD     │      │    Slack     │
                 │ Auto-Discover│      │ Notifications│
                 └──────────────┘      └──────────────┘
```

---

## Custom Resource Definition (CRD)

### ClusterRequest CRD

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: clusterrequests.opl.openshiftpartnerlabs.com
spec:
  group: opl.openshiftpartnerlabs.com
  names:
    kind: ClusterRequest
    listKind: ClusterRequestList
    plural: clusterrequests
    singular: clusterrequest
    shortNames:
      - cr
      - creq
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              required:
                - clusterName
                - openshiftVersion
                - clusterSize
                - region
              properties:
                # Cluster Identity
                clusterName:
                  type: string
                  description: User-provided cluster name (4-12 chars, alphanumeric)
                  pattern: '^[a-z][a-z0-9-]{3,11}$'
                generatedName:
                  type: string
                  description: Auto-generated unique name (uuid-prefix + cluster-name)

                # Cluster Configuration
                openshiftVersion:
                  type: string
                  description: OpenShift version (e.g., "4.14.0")
                clusterSize:
                  type: string
                  enum: [small, medium, large]
                  description: Cluster size determining instance types and replicas
                cloudProvider:
                  type: string
                  enum: [aws, azure, gcp]
                  default: aws
                region:
                  type: string
                  description: Cloud provider region (e.g., "us-east-1")

                # Organization
                companyName:
                  type: string
                  description: Company name
                projectName:
                  type: string
                  description: Project identifier

                # Contact Information
                primaryContact:
                  type: object
                  properties:
                    firstName:
                      type: string
                    lastName:
                      type: string
                    email:
                      type: string
                      format: email
                secondaryContact:
                  type: object
                  properties:
                    firstName:
                      type: string
                    lastName:
                      type: string
                    email:
                      type: string
                      format: email

                # Request Metadata
                requestType:
                  type: string
                  enum: [workshop, demo, poc, training, development]
                  description: Type of cluster request
                partner:
                  type: boolean
                  description: Whether this is a partner request
                sponsor:
                  type: string
                  description: Sponsoring person/team
                description:
                  type: string
                  description: Detailed description of cluster purpose

                # Lifecycle
                leaseTime:
                  type: string
                  enum: ["1d", "1w", "2w", "1m", "2m", "3m", "6m"]
                  description: Cluster lease duration
                startDate:
                  type: string
                  format: date-time
                  description: When cluster should be provisioned
                endDate:
                  type: string
                  format: date-time
                  description: When cluster lease expires
                alwaysOn:
                  type: boolean
                  default: false
                  description: Whether cluster should persist beyond lease

                # Advanced Options (optional)
                baseDomain:
                  type: string
                  default: "openshiftpartnerlabs.com"
                networking:
                  type: object
                  properties:
                    clusterNetworkCIDR:
                      type: string
                      default: "10.128.0.0/14"
                    serviceNetworkCIDR:
                      type: string
                      default: "172.30.0.0/16"
                    machineNetworkCIDR:
                      type: string
                      default: "10.0.0.0/16"

            status:
              type: object
              properties:
                # State Machine
                state:
                  type: string
                  enum: [pending, approved, provisioning, complete, failed, denied]
                  description: Current state of the cluster request

                # Conditions (standard K8s pattern)
                conditions:
                  type: array
                  items:
                    type: object
                    properties:
                      type:
                        type: string
                      status:
                        type: string
                        enum: ["True", "False", "Unknown"]
                      lastTransitionTime:
                        type: string
                        format: date-time
                      reason:
                        type: string
                      message:
                        type: string

                # Provisioning Details
                gitCommitSHA:
                  type: string
                  description: Git commit SHA after cluster files created
                argocdApplicationCreated:
                  type: boolean
                  description: Whether ArgoCD Application was created
                hiveClusterID:
                  type: string
                  description: Hive ClusterDeployment UID
                clusterProvisioned:
                  type: boolean
                  description: Whether Hive completed provisioning

                # Timestamps
                approvedAt:
                  type: string
                  format: date-time
                provisioningStartedAt:
                  type: string
                  format: date-time
                provisioningCompletedAt:
                  type: string
                  format: date-time

                # Error Tracking
                errorMessage:
                  type: string
                retryCount:
                  type: integer
                  default: 0
                maxRetries:
                  type: integer
                  default: 3

      # Additional printer columns for kubectl get
      additionalPrinterColumns:
        - name: State
          type: string
          jsonPath: .status.state
        - name: Cluster
          type: string
          jsonPath: .spec.generatedName
        - name: Size
          type: string
          jsonPath: .spec.clusterSize
        - name: Region
          type: string
          jsonPath: .spec.region
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp

      # Subresources
      subresources:
        status: {}  # Enable status subresource
```

### Example ClusterRequest CR

```yaml
apiVersion: opl.openshiftpartnerlabs.com/v1alpha1
kind: ClusterRequest
metadata:
  name: lab-a1b2c3d4-testcluster
  namespace: cluster-requests
  labels:
    company: acme-corp
    environment: development
    request-type: workshop
spec:
  clusterName: testcluster
  generatedName: a1b2c3d4-testcluster
  openshiftVersion: "4.14.0"
  clusterSize: medium
  cloudProvider: aws
  region: us-east-1

  companyName: Acme Corp
  projectName: Q1-Workshop

  primaryContact:
    firstName: John
    lastName: Doe
    email: john.doe@acme.com

  requestType: workshop
  partner: true
  sponsor: Jane Smith
  description: Workshop cluster for Q1 partner training

  leaseTime: "2w"
  startDate: "2026-01-25T09:00:00Z"
  endDate: "2026-02-08T17:00:00Z"
  alwaysOn: false

  baseDomain: openshiftpartnerlabs.com

status:
  state: approved
  conditions:
    - type: Approved
      status: "True"
      lastTransitionTime: "2026-01-22T10:30:00Z"
      reason: AdminApproved
      message: Approved by admin@acme.com
    - type: Provisioning
      status: "False"
      lastTransitionTime: "2026-01-22T10:30:00Z"
      reason: AwaitingReconciliation
      message: Waiting for operator to process
  approvedAt: "2026-01-22T10:30:00Z"
```

---

## Operator Implementation

### Technology Stack

- **Language:** Go 1.22+
- **Framework:** Kubebuilder v3 / Operator SDK
- **Libraries:**
  - `sigs.k8s.io/controller-runtime` - K8s controller framework
  - `github.com/go-git/go-git/v5` - Git operations
  - `text/template` - Template rendering (Go stdlib)
  - `github.com/jackc/pgx/v5` - PostgreSQL (optional, for DB sync)
  - `github.com/slack-go/slack` - Slack integration

### Project Structure

```
cluster-operator/
├── api/
│   └── v1alpha1/
│       ├── clusterrequest_types.go      # CRD type definitions
│       ├── groupversion_info.go
│       └── zz_generated.deepcopy.go
├── controllers/
│   ├── clusterrequest_controller.go     # Main reconciliation logic
│   ├── hive_watcher.go                  # Watch Hive ClusterDeployments
│   └── suite_test.go
├── internal/
│   ├── git/
│   │   └── client.go                    # Git operations
│   ├── templates/
│   │   ├── generator.go                 # Template rendering
│   │   ├── kustomization.yaml.tmpl
│   │   ├── cluster-config.yaml.tmpl
│   │   └── argocd-application.yaml.tmpl
│   ├── mappers/
│   │   └── cluster_config.go            # Spec to config mapping
│   ├── slack/
│   │   └── notifier.go                  # Slack notifications
│   └── database/
│       └── sync.go                      # Optional: sync status to PostgreSQL
├── config/
│   ├── crd/                             # CRD manifests
│   ├── rbac/                            # RBAC manifests
│   ├── manager/                         # Operator deployment
│   └── samples/                         # Example CRs
├── hack/
├── Dockerfile
├── Makefile
├── go.mod
├── go.sum
└── README.md
```

### Core Controller Logic

#### ClusterRequest Controller (`controllers/clusterrequest_controller.go`)

```go
package controllers

import (
    "context"
    "fmt"
    "time"

    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    oplv1alpha1 "github.com/openshift-partner-labs/cluster-operator/api/v1alpha1"
    "github.com/openshift-partner-labs/cluster-operator/internal/git"
    "github.com/openshift-partner-labs/cluster-operator/internal/mappers"
    "github.com/openshift-partner-labs/cluster-operator/internal/slack"
    "github.com/openshift-partner-labs/cluster-operator/internal/templates"
)

// ClusterRequestReconciler reconciles a ClusterRequest object
type ClusterRequestReconciler struct {
    client.Client
    Scheme        *runtime.Scheme
    GitClient     *git.Client
    TemplateGen   *templates.Generator
    SlackNotifier *slack.Notifier
}

//+kubebuilder:rbac:groups=opl.openshiftpartnerlabs.com,resources=clusterrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=opl.openshiftpartnerlabs.com,resources=clusterrequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=opl.openshiftpartnerlabs.com,resources=clusterrequests/finalizers,verbs=update
//+kubebuilder:rbac:groups=hive.openshift.io,resources=clusterdeployments,verbs=get;list;watch

// Reconcile is the main reconciliation loop
func (r *ClusterRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    // Fetch the ClusterRequest instance
    clusterReq := &oplv1alpha1.ClusterRequest{}
    if err := r.Get(ctx, req.NamespacedName, clusterReq); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Check if the CR is being deleted
    if !clusterReq.ObjectMeta.DeletionTimestamp.IsZero() {
        return r.handleDeletion(ctx, clusterReq)
    }

    // State machine reconciliation
    switch clusterReq.Status.State {
    case "approved":
        return r.handleApproved(ctx, clusterReq)
    case "provisioning":
        return r.handleProvisioning(ctx, clusterReq)
    case "complete", "failed", "denied":
        // Terminal states - no action needed
        return ctrl.Result{}, nil
    default:
        // Pending state - wait for approval
        return ctrl.Result{}, nil
    }
}

// handleApproved processes approved ClusterRequests
func (r *ClusterRequestReconciler) handleApproved(ctx context.Context, cr *oplv1alpha1.ClusterRequest) (ctrl.Result, error) {
    logger := log.FromContext(ctx)
    logger.Info("Processing approved cluster request", "cluster", cr.Spec.GeneratedName)

    // Update status to provisioning
    cr.Status.State = "provisioning"
    cr.Status.ProvisioningStartedAt = time.Now().Format(time.RFC3339)
    if err := r.updateStatus(ctx, cr, "Provisioning", "InProgress", "Operator processing request"); err != nil {
        return ctrl.Result{}, err
    }

    // Map spec to cluster configuration
    clusterConfig := mappers.MapClusterRequest(cr)

    // Generate cluster files from templates
    files, err := r.TemplateGen.GenerateClusterFiles(clusterConfig)
    if err != nil {
        r.updateStatus(ctx, cr, "Provisioning", "False", fmt.Sprintf("Template generation failed: %v", err))
        cr.Status.State = "failed"
        cr.Status.ErrorMessage = err.Error()
        r.Status().Update(ctx, cr)
        return ctrl.Result{}, err
    }

    // Commit to Git repository
    commitSHA, err := r.GitClient.CommitCluster(cr.Spec.GeneratedName, files)
    if err != nil {
        r.updateStatus(ctx, cr, "Provisioning", "False", fmt.Sprintf("Git commit failed: %v", err))
        cr.Status.State = "failed"
        cr.Status.ErrorMessage = err.Error()
        r.Status().Update(ctx, cr)
        return ctrl.Result{}, err
    }

    // Update status with Git commit SHA
    cr.Status.GitCommitSHA = commitSHA
    cr.Status.ArgocdApplicationCreated = true
    if err := r.updateStatus(ctx, cr, "GitCommitted", "True", fmt.Sprintf("Committed to Git: %s", commitSHA)); err != nil {
        return ctrl.Result{}, err
    }

    // Send Slack notification
    r.SlackNotifier.NotifyProvisioningStarted(cr)

    logger.Info("Cluster files committed to Git", "cluster", cr.Spec.GeneratedName, "commit", commitSHA)

    // Requeue to check Hive status
    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// handleProvisioning monitors provisioning status
func (r *ClusterRequestReconciler) handleProvisioning(ctx context.Context, cr *oplv1alpha1.ClusterRequest) (ctrl.Result, error) {
    // The Hive watcher will update the status when ClusterDeployment completes
    // This handler just ensures we keep checking
    return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

// handleDeletion handles cleanup when CR is deleted
func (r *ClusterRequestReconciler) handleDeletion(ctx context.Context, cr *oplv1alpha1.ClusterRequest) (ctrl.Result, error) {
    logger := log.FromContext(ctx)
    logger.Info("Cluster request being deleted", "cluster", cr.Spec.GeneratedName)

    // TODO: Optionally delete cluster files from Git
    // TODO: Optionally trigger cluster deprovisioning

    return ctrl.Result{}, nil
}

// updateStatus updates the status conditions
func (r *ClusterRequestReconciler) updateStatus(ctx context.Context, cr *oplv1alpha1.ClusterRequest,
    conditionType string, status string, message string) error {

    condition := oplv1alpha1.Condition{
        Type:               conditionType,
        Status:             status,
        LastTransitionTime: time.Now().Format(time.RFC3339),
        Reason:             conditionType,
        Message:            message,
    }

    cr.Status.Conditions = append(cr.Status.Conditions, condition)
    return r.Status().Update(ctx, cr)
}

// SetupWithManager sets up the controller with the Manager
func (r *ClusterRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&oplv1alpha1.ClusterRequest{}).
        Complete(r)
}
```

#### Hive Watcher (`controllers/hive_watcher.go`)

```go
package controllers

import (
    "context"
    "fmt"

    hivev1 "github.com/openshift/hive/apis/hive/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    oplv1alpha1 "github.com/openshift-partner-labs/cluster-operator/api/v1alpha1"
)

// HiveWatcher watches Hive ClusterDeployments and updates ClusterRequest status
type HiveWatcher struct {
    client.Client
}

//+kubebuilder:rbac:groups=hive.openshift.io,resources=clusterdeployments,verbs=get;list;watch
//+kubebuilder:rbac:groups=hive.openshift.io,resources=clusterdeployments/status,verbs=get

func (h *HiveWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    // Fetch ClusterDeployment
    clusterDep := &hivev1.ClusterDeployment{}
    if err := h.Get(ctx, req.NamespacedName, clusterDep); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Find corresponding ClusterRequest by matching generated name
    clusterReqList := &oplv1alpha1.ClusterRequestList{}
    if err := h.List(ctx, clusterReqList); err != nil {
        return ctrl.Result{}, err
    }

    var matchingCR *oplv1alpha1.ClusterRequest
    for i := range clusterReqList.Items {
        if clusterReqList.Items[i].Spec.GeneratedName == clusterDep.Name {
            matchingCR = &clusterReqList.Items[i]
            break
        }
    }

    if matchingCR == nil {
        // No matching ClusterRequest found
        return ctrl.Result{}, nil
    }

    // Check ClusterDeployment conditions
    for _, condition := range clusterDep.Status.Conditions {
        switch condition.Type {
        case hivev1.ProvisionedCondition:
            if condition.Status == metav1.ConditionTrue {
                // Provisioning completed
                logger.Info("Cluster provisioning completed", "cluster", matchingCR.Spec.GeneratedName)
                matchingCR.Status.State = "complete"
                matchingCR.Status.ClusterProvisioned = true
                matchingCR.Status.ProvisioningCompletedAt = condition.LastTransitionTime.Format(time.RFC3339)
                matchingCR.Status.HiveClusterID = string(clusterDep.UID)

                if err := h.Status().Update(ctx, matchingCR); err != nil {
                    return ctrl.Result{}, err
                }

                // Send Slack notification
                // TODO: integrate slack notifier
            }
        case hivev1.ProvisionFailedCondition:
            if condition.Status == metav1.ConditionTrue {
                // Provisioning failed
                logger.Error(nil, "Cluster provisioning failed", "cluster", matchingCR.Spec.GeneratedName)
                matchingCR.Status.State = "failed"
                matchingCR.Status.ErrorMessage = condition.Message

                if err := h.Status().Update(ctx, matchingCR); err != nil {
                    return ctrl.Result{}, err
                }

                // Send Slack notification
                // TODO: integrate slack notifier
            }
        }
    }

    return ctrl.Result{}, nil
}

func (h *HiveWatcher) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&hivev1.ClusterDeployment{}).
        Complete(h)
}
```

#### Cluster Config Mapper (`internal/mappers/cluster_config.go`)

```go
package mappers

import (
    oplv1alpha1 "github.com/openshift-partner-labs/cluster-operator/api/v1alpha1"
)

type ClusterConfig struct {
    ClusterName              string
    Environment              string
    CloudProvider            string
    Region                   string
    BaseDomain               string
    OpenshiftVersion         string
    ControlPlaneInstanceType string
    ControlPlaneReplicas     int
    WorkerInstanceType       string
    WorkerReplicas           int
    WorkerZones              string
    ClusterNetworkCIDR       string
    ServiceNetworkCIDR       string
    MachineNetworkCIDR       string
    CompanyName              string
    ProjectName              string
    RequestedBy              string
    RequestType              string
}

var clusterSizeMap = map[string]struct {
    ControlPlaneType string
    ControlPlaneReplicas int
    WorkerType       string
    WorkerReplicas   int
}{
    "small": {
        ControlPlaneType: "m5.xlarge",
        ControlPlaneReplicas: 3,
        WorkerType:       "m5.2xlarge",
        WorkerReplicas:   2,
    },
    "medium": {
        ControlPlaneType: "m5.xlarge",
        ControlPlaneReplicas: 3,
        WorkerType:       "m5.2xlarge",
        WorkerReplicas:   3,
    },
    "large": {
        ControlPlaneType: "m5.2xlarge",
        ControlPlaneReplicas: 3,
        WorkerType:       "m5.4xlarge",
        WorkerReplicas:   6,
    },
}

var versionMap = map[string]string{
    "4.14.0": "openshift-v4.14.0",
    "4.15.0": "openshift-v4.15.0",
    "4.20.10": "img4.20.10-x86-64-appsub",
}

func MapClusterRequest(cr *oplv1alpha1.ClusterRequest) ClusterConfig {
    sizeConfig := clusterSizeMap[cr.Spec.ClusterSize]

    // Determine environment
    env := "development"
    if cr.Spec.AlwaysOn {
        env = "production"
    } else if cr.Spec.RequestType == "poc" {
        env = "staging"
    }

    // Map region to availability zones
    region := cr.Spec.Region
    zones := fmt.Sprintf("%sa,%sb,%sc", region, region, region)

    // Map version
    version := versionMap[cr.Spec.OpenshiftVersion]
    if version == "" {
        version = cr.Spec.OpenshiftVersion
    }

    return ClusterConfig{
        ClusterName:              cr.Spec.GeneratedName,
        Environment:              env,
        CloudProvider:            cr.Spec.CloudProvider,
        Region:                   region,
        BaseDomain:               cr.Spec.BaseDomain,
        OpenshiftVersion:         version,
        ControlPlaneInstanceType: sizeConfig.ControlPlaneType,
        ControlPlaneReplicas:     sizeConfig.ControlPlaneReplicas,
        WorkerInstanceType:       sizeConfig.WorkerType,
        WorkerReplicas:           sizeConfig.WorkerReplicas,
        WorkerZones:              zones,
        ClusterNetworkCIDR:       cr.Spec.Networking.ClusterNetworkCIDR,
        ServiceNetworkCIDR:       cr.Spec.Networking.ServiceNetworkCIDR,
        MachineNetworkCIDR:       cr.Spec.Networking.MachineNetworkCIDR,
        CompanyName:              cr.Spec.CompanyName,
        ProjectName:              cr.Spec.ProjectName,
        RequestedBy:              cr.Spec.PrimaryContact.Email,
        RequestType:              cr.Spec.RequestType,
    }
}
```

---

## Buffalo Application Changes

### Minimal Integration Required

The Buffalo app needs a small modification to create ClusterRequest CRs when admins approve lab requests.

#### Add Kubernetes Client

**`actions/kubernetes.go`** (NEW FILE)

```go
package actions

import (
    "context"
    "fmt"
    "os"

    oplv1alpha1 "github.com/openshift-partner-labs/cluster-operator/api/v1alpha1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/kubernetes/scheme"
    "k8s.io/client-go/rest"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

var k8sClient client.Client

func init() {
    // Initialize Kubernetes client
    config, err := rest.InClusterConfig()
    if err != nil {
        fmt.Printf("Failed to get in-cluster config: %v\n", err)
        return
    }

    // Add ClusterRequest scheme
    oplv1alpha1.AddToScheme(scheme.Scheme)

    k8sClient, err = client.New(config, client.Options{Scheme: scheme.Scheme})
    if err != nil {
        fmt.Printf("Failed to create Kubernetes client: %v\n", err)
    }
}

// CreateClusterRequest creates a ClusterRequest CR from a Lab
func CreateClusterRequest(lab *models.Lab) error {
    if k8sClient == nil {
        return fmt.Errorf("Kubernetes client not initialized")
    }

    cr := &oplv1alpha1.ClusterRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      lab.GeneratedName,
            Namespace: os.Getenv("CLUSTER_REQUEST_NAMESPACE"), // e.g., "cluster-requests"
            Labels: map[string]string{
                "company":      lab.CompanyName,
                "request-type": lab.RequestType,
            },
        },
        Spec: oplv1alpha1.ClusterRequestSpec{
            ClusterName:      lab.ClusterName,
            GeneratedName:    lab.GeneratedName,
            OpenshiftVersion: lab.OpenshiftVersion,
            ClusterSize:      lab.ClusterSize,
            CloudProvider:    lab.CloudProvider,
            Region:           lab.Region,
            CompanyName:      lab.CompanyName,
            ProjectName:      lab.ProjectName,
            PrimaryContact: oplv1alpha1.Contact{
                FirstName: lab.PrimaryFirst,
                LastName:  lab.PrimaryLast,
                Email:     lab.PrimaryEmail,
            },
            SecondaryContact: oplv1alpha1.Contact{
                FirstName: lab.SecondaryFirst,
                LastName:  lab.SecondaryLast,
                Email:     lab.SecondaryEmail,
            },
            RequestType: lab.RequestType,
            Partner:     lab.Partner,
            Sponsor:     lab.Sponsor,
            Description: lab.Description,
            LeaseTime:   lab.LeaseTime,
            StartDate:   lab.StartDate.Format(time.RFC3339),
            EndDate:     lab.EndDate.Format(time.RFC3339),
            AlwaysOn:    lab.AlwaysOn,
            BaseDomain:  "openshiftpartnerlabs.com",
        },
    }

    ctx := context.Background()
    if err := k8sClient.Create(ctx, cr); err != nil {
        return fmt.Errorf("failed to create ClusterRequest: %w", err)
    }

    // Update CR status to approved
    cr.Status.State = "approved"
    cr.Status.ApprovedAt = time.Now().Format(time.RFC3339)
    if err := k8sClient.Status().Update(ctx, cr); err != nil {
        return fmt.Errorf("failed to update ClusterRequest status: %w", err)
    }

    return nil
}
```

#### Modify Approve Handler

**`actions/labs.go`** (MODIFY EXISTING)

```go
// LabsApprove approves a lab request
func LabsApprove(c buffalo.Context) error {
    tx := c.Value("tx").(*pop.Connection)
    lab := &models.Lab{}

    if err := tx.Find(lab, c.Param("lab_id")); err != nil {
        return c.Error(http.StatusNotFound, err)
    }

    // Update database state
    lab.State = "approved"
    if err := tx.Update(lab); err != nil {
        return err
    }

    // NEW: Create ClusterRequest CR in Kubernetes
    if err := CreateClusterRequest(lab); err != nil {
        // Log error but don't fail the approval
        c.Logger().Errorf("Failed to create ClusterRequest CR: %v", err)
        // Optionally: retry logic or alert
    }

    // Existing notification code
    notifier.SendSlackMessage(lab)

    return c.Redirect(http.StatusSeeOther, "/labs/%s", lab.ID)
}
```

---

## Deployment

### Install CRD

```bash
kubectl apply -f config/crd/opl.openshiftpartnerlabs.com_clusterrequests.yaml
```

### Deploy Operator

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cluster-operator
  namespace: cluster-operator
spec:
  replicas: 1
  selector:
    matchLabels:
      app: cluster-operator
  template:
    metadata:
      labels:
        app: cluster-operator
    spec:
      serviceAccountName: cluster-operator
      containers:
      - name: manager
        image: quay.io/opl/cluster-operator:v1.0.0
        env:
          - name: GIT_REPO_URL
            value: "git@github.com:yashoza19/opl-argocd.git"
          - name: GIT_BRANCH
            value: "master"
          - name: SLACK_WEBHOOK_URL
            valueFrom:
              secretKeyRef:
                name: cluster-operator-secrets
                key: slack_webhook_url
        volumeMounts:
          - name: git-ssh-key
            mountPath: /etc/git-secret
            readOnly: true
      volumes:
        - name: git-ssh-key
          secret:
            secretName: cluster-operator-secrets
            items:
              - key: git_ssh_key
                path: id_rsa
                mode: 0600
```

---

## Workflow

### End-to-End Flow

1. **User submits request** → Buffalo UI creates `labs` record (state=pending)
2. **Admin approves** → Buffalo updates DB + creates `ClusterRequest` CR (status.state=approved)
3. **Operator reconciles** → Watches `ClusterRequest` CRs
4. **Generates files** → Renders templates from CR spec
5. **Commits to Git** → Creates `clusters/<generated-name>/` directory
6. **ArgoCD syncs** → Discovers new application, creates ClusterDeployment
7. **Hive provisions** → Provisions OpenShift cluster
8. **Operator updates CR** → Watches ClusterDeployment, updates ClusterRequest status
9. **Buffalo displays** → UI shows cluster status from CR

---

## Advantages of CRD Approach

✅ **True Kubernetes Operator** - Follows standard operator pattern
✅ **Declarative** - Cluster requests are Kubernetes resources
✅ **GitOps Compatible** - CRs can be version-controlled
✅ **Event-Driven** - Immediate reconciliation (no polling delay)
✅ **Standard Tools** - `kubectl get clusterrequests`, `kubectl describe`, etc.
✅ **Extensible** - Easy to add validating/mutating webhooks
✅ **RBAC Integration** - Kubernetes-native access control
✅ **Scalable** - Controller-runtime handles watch efficiency

---

## Success Criteria

- ✅ CRD installed and validated
- ✅ Operator reconciles ClusterRequest CRs
- ✅ Buffalo app creates CRs on approval
- ✅ Git commits contain valid cluster configurations
- ✅ ArgoCD discovers and syncs clusters
- ✅ Hive status updates ClusterRequest status
- ✅ Database and CR status stay in sync
- ✅ Standard `kubectl` commands work

---

**Document Version:** 3.0 (CRD-Based)
**Date:** 2026-01-22
**Status:** Design/Proposal
**Approach:** True Kubernetes Operator with Custom Resource Definition
