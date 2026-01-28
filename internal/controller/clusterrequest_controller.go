/*
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
*/

package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	oplv1alpha1 "github.com/openshift-partner-labs/opl-cluster-operator/api/v1alpha1"
	"github.com/openshift-partner-labs/opl-cluster-operator/internal/git"
	"github.com/openshift-partner-labs/opl-cluster-operator/internal/mappers"
	"github.com/openshift-partner-labs/opl-cluster-operator/internal/templates"
)

const (
	clusterRequestFinalizer = "opl.openshiftpartnerlabs.com/finalizer"

	// State constants
	StatePending       = "pending"
	StateApproved      = "approved"
	StateGitCommitted  = "git-committed"
	StateProvisioning  = "provisioning"
	StateComplete      = "complete"
	StateFailed        = "failed"

	// Condition types
	ConditionTypeReady              = "Ready"
	ConditionTypeGitCommitted       = "GitCommitted"
	ConditionTypeArgocdAppCreated   = "ArgocdAppCreated"
	ConditionTypeHiveProvisioning   = "HiveProvisioning"
	ConditionTypeClusterProvisioned = "ClusterProvisioned"

	// Requeue intervals
	requeueAfterSuccess = 30 * time.Second
	requeueAfterError   = 1 * time.Minute
)

// ClusterRequestReconciler reconciles a ClusterRequest object
type ClusterRequestReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	GitClient   *git.Client
	TemplateGen *templates.Generator
	// TODO: Add SlackNotifier
}

// +kubebuilder:rbac:groups=opl.openshiftpartnerlabs.com,resources=clusterrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opl.openshiftpartnerlabs.com,resources=clusterrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opl.openshiftpartnerlabs.com,resources=clusterrequests/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ClusterRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling ClusterRequest", "name", req.Name, "namespace", req.Namespace)

	// Fetch the ClusterRequest instance
	clusterRequest := &oplv1alpha1.ClusterRequest{}
	if err := r.Get(ctx, req.NamespacedName, clusterRequest); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ClusterRequest resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ClusterRequest")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !clusterRequest.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, clusterRequest)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(clusterRequest, clusterRequestFinalizer) {
		controllerutil.AddFinalizer(clusterRequest, clusterRequestFinalizer)
		if err := r.Update(ctx, clusterRequest); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize state if empty
	if clusterRequest.Status.State == "" {
		logger.Info("Initializing ClusterRequest state to pending")
		clusterRequest.Status.State = StatePending
		if err := r.Status().Update(ctx, clusterRequest); err != nil {
			logger.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// State machine reconciliation
	switch clusterRequest.Status.State {
	case StatePending:
		logger.Info("ClusterRequest is pending approval")
		// Wait for external approval (Buffalo app will update state to 'approved')
		return ctrl.Result{RequeueAfter: requeueAfterSuccess}, nil

	case StateApproved:
		logger.Info("Processing approved ClusterRequest")
		return r.handleApproved(ctx, clusterRequest)

	case StateGitCommitted:
		logger.Info("Cluster configuration committed to Git, monitoring ArgoCD sync")
		return r.handleGitCommitted(ctx, clusterRequest)

	case StateProvisioning:
		logger.Info("Cluster is provisioning via Hive")
		// Hive watcher will update status when provisioning completes
		return ctrl.Result{RequeueAfter: requeueAfterSuccess}, nil

	case StateComplete:
		logger.Info("Cluster provisioning complete")
		return ctrl.Result{}, nil

	case StateFailed:
		logger.Info("ClusterRequest failed", "error", clusterRequest.Status.ErrorMessage)
		// Check if we should retry
		if clusterRequest.Status.RetryCount < 3 {
			logger.Info("Retrying failed ClusterRequest", "retryCount", clusterRequest.Status.RetryCount)
			clusterRequest.Status.State = StateApproved
			clusterRequest.Status.RetryCount++
			if err := r.Status().Update(ctx, clusterRequest); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: requeueAfterError}, nil
		}
		return ctrl.Result{}, nil

	default:
		logger.Info("Unknown state", "state", clusterRequest.Status.State)
		return ctrl.Result{}, nil
	}
}

// handleApproved processes an approved ClusterRequest by generating cluster configs and committing to Git
func (r *ClusterRequestReconciler) handleApproved(ctx context.Context, cr *oplv1alpha1.ClusterRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Set processing started timestamp
	if cr.Status.ProcessingStartedAt == nil {
		now := metav1.Now()
		cr.Status.ProcessingStartedAt = &now
		if err := r.Status().Update(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Map ClusterRequest spec to cluster configuration
	logger.Info("Mapping ClusterRequest to cluster configuration", "cluster", cr.Spec.ClusterName)
	clusterConfig := mappers.MapClusterRequest(cr)

	// Generate cluster configuration files from templates
	logger.Info("Generating cluster configuration files", "cluster", cr.Spec.ClusterName)
	files, err := r.TemplateGen.GenerateClusterFiles(clusterConfig)
	if err != nil {
		logger.Error(err, "Failed to generate cluster configuration files")
		r.setCondition(cr, ConditionTypeGitCommitted, metav1.ConditionFalse, "TemplateFailed",
			fmt.Sprintf("Failed to generate templates: %v", err))
		if setErr := r.setErrorState(ctx, cr, err); setErr != nil {
			return ctrl.Result{}, setErr
		}
		return ctrl.Result{RequeueAfter: requeueAfterError}, nil
	}

	// Commit cluster configuration to Git repository
	logger.Info("Committing cluster configuration to Git", "cluster", cr.Spec.ClusterName)
	commitSHA, err := r.GitClient.CommitCluster(cr.Spec.ClusterName, files)
	if err != nil {
		logger.Error(err, "Failed to commit cluster configuration to Git")
		r.setCondition(cr, ConditionTypeGitCommitted, metav1.ConditionFalse, "GitCommitFailed",
			fmt.Sprintf("Failed to commit to Git: %v", err))
		if setErr := r.setErrorState(ctx, cr, err); setErr != nil {
			return ctrl.Result{}, setErr
		}
		return ctrl.Result{RequeueAfter: requeueAfterError}, nil
	}

	// Update status with successful commit
	logger.Info("Successfully committed cluster configuration to Git", "cluster", cr.Spec.ClusterName, "commitSHA", commitSHA)
	cr.Status.State = StateGitCommitted
	cr.Status.GitCommitSHA = commitSHA
	r.setCondition(cr, ConditionTypeGitCommitted, metav1.ConditionTrue, "GitCommitted",
		fmt.Sprintf("Cluster configuration committed to Git: %s", commitSHA))

	if err := r.Status().Update(ctx, cr); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

// handleGitCommitted monitors ArgoCD synchronization and transitions to provisioning
func (r *ClusterRequestReconciler) handleGitCommitted(ctx context.Context, cr *oplv1alpha1.ClusterRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// TODO: Check if ArgoCD Application exists and is synced
	logger.Info("TODO: Monitor ArgoCD Application sync status")

	// Placeholder: assume ArgoCD sync is complete
	cr.Status.ArgocdAppCreated = true
	cr.Status.State = StateProvisioning
	r.setCondition(cr, ConditionTypeArgocdAppCreated, metav1.ConditionTrue, "AppCreated", "ArgoCD Application created and syncing")
	r.setCondition(cr, ConditionTypeHiveProvisioning, metav1.ConditionUnknown, "Provisioning", "Waiting for Hive to start provisioning")

	if err := r.Status().Update(ctx, cr); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

// handleDeletion handles cleanup when a ClusterRequest is deleted
func (r *ClusterRequestReconciler) handleDeletion(ctx context.Context, cr *oplv1alpha1.ClusterRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(cr, clusterRequestFinalizer) {
		// TODO: Cleanup logic (e.g., delete Git files, notify Slack)
		logger.Info("Cleaning up ClusterRequest resources")

		// Remove finalizer
		controllerutil.RemoveFinalizer(cr, clusterRequestFinalizer)
		if err := r.Update(ctx, cr); err != nil {
			logger.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// setCondition updates or adds a condition to the ClusterRequest status
func (r *ClusterRequestReconciler) setCondition(cr *oplv1alpha1.ClusterRequest, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: cr.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&cr.Status.Conditions, condition)
}

// setErrorState updates the ClusterRequest to failed state with error message
func (r *ClusterRequestReconciler) setErrorState(ctx context.Context, cr *oplv1alpha1.ClusterRequest, err error) error {
	logger := log.FromContext(ctx)

	cr.Status.State = StateFailed
	cr.Status.ErrorMessage = err.Error()
	now := metav1.Now()
	cr.Status.ProcessingCompletedAt = &now

	r.setCondition(cr, ConditionTypeReady, metav1.ConditionFalse, "Failed", fmt.Sprintf("ClusterRequest failed: %v", err))

	if updateErr := r.Status().Update(ctx, cr); updateErr != nil {
		logger.Error(updateErr, "Failed to update error status")
		return updateErr
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&oplv1alpha1.ClusterRequest{}).
		Named("clusterrequest").
		Complete(r)
}
