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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	oplv1alpha1 "github.com/yashoza19/opl-cluster-operator/api/v1alpha1"
	"github.com/yashoza19/opl-cluster-operator/internal/database"
)

// DatabaseSyncReconciler syncs labs from PostgreSQL to ClusterRequests
type DatabaseSyncReconciler struct {
	client.Client
	DBClient *database.Client
	Config   DatabaseSyncConfig
}

// DatabaseSyncConfig holds configuration for database sync
type DatabaseSyncConfig struct {
	SyncNamespace   string        // Namespace where CRs are created
	BaseDomain      string        // Default base domain for clusters
	PollingInterval time.Duration // How often to poll database
	BatchSize       int           // Max labs per sync batch
}

// Start implements manager.Runnable (Mode 1: Polling Loop for DB → K8s sync)
func (r *DatabaseSyncReconciler) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("database-sync-poller")
	logger.Info("Starting database sync polling loop",
		"interval", r.Config.PollingInterval,
		"namespace", r.Config.SyncNamespace,
		"batchSize", r.Config.BatchSize)

	ticker := time.NewTicker(r.Config.PollingInterval)
	defer ticker.Stop()

	// Initial sync on startup
	if err := r.syncDatabaseToKubernetes(ctx); err != nil {
		logger.Error(err, "Initial database sync failed")
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping database sync polling loop")
			return nil
		case <-ticker.C:
			if err := r.syncDatabaseToKubernetes(ctx); err != nil {
				logger.Error(err, "Database sync failed")
				// Continue polling despite errors
			}
		}
	}
}

// syncDatabaseToKubernetes polls database and creates CRs (DB → K8s)
func (r *DatabaseSyncReconciler) syncDatabaseToKubernetes(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("db-to-k8s-sync")

	// 1. Fetch unsynced labs from database
	labs, err := r.DBClient.GetUnsyncedLabs(ctx, r.Config.BatchSize)
	if err != nil {
		return fmt.Errorf("failed to fetch unsynced labs: %w", err)
	}

	if len(labs) == 0 {
		logger.V(1).Info("No unsynced labs found")
		return nil
	}

	logger.Info("Found unsynced labs", "count", len(labs))

	// 2. Process each lab
	successCount := 0
	errorCount := 0
	for _, lab := range labs {
		if err := r.processLab(ctx, lab); err != nil {
			logger.Error(err, "Failed to process lab",
				"labID", lab.ID, "clusterName", lab.ClusterName)
			errorCount++
			// Continue processing other labs (don't fail entire batch)
			continue
		}
		successCount++
	}

	logger.Info("Database sync completed", "success", successCount, "errors", errorCount)
	return nil
}

// processLab handles a single lab sync
func (r *DatabaseSyncReconciler) processLab(ctx context.Context, lab database.Lab) error {
	logger := log.FromContext(ctx).WithValues("labID", lab.ID, "clusterName", lab.ClusterName)

	// 1. Validate lab data
	if err := database.ValidateLab(lab); err != nil {
		logger.Error(err, "Lab validation failed")
		// Mark as synced with error so we don't retry invalid data
		return r.DBClient.UpdateLabK8sStatus(ctx, lab.ID, database.LabK8sStatus{
			State:        "failed",
			ErrorMessage: stringPtr(err.Error()),
		})
	}

	// 2. Idempotency check: CR exists by name?
	existingCR := &oplv1alpha1.ClusterRequest{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      lab.ClusterName,
		Namespace: r.Config.SyncNamespace,
	}, existingCR)

	if err == nil {
		// CR exists - check if it's the same lab
		if existingCR.Spec.LabID == lab.ID {
			logger.Info("ClusterRequest already exists for this lab, marking as synced")
			return r.DBClient.MarkLabAsSynced(ctx, lab.ID, existingCR.Namespace)
		} else if existingCR.Spec.LabID != 0 {
			// Conflict: different lab with same cluster name
			errMsg := fmt.Sprintf("Name conflict: existing CR has labID=%d, new lab has labID=%d",
				existingCR.Spec.LabID, lab.ID)
			logger.Error(fmt.Errorf(errMsg), "Duplicate cluster name detected")
			return r.DBClient.UpdateLabK8sStatus(ctx, lab.ID, database.LabK8sStatus{
				State:        "failed",
				ErrorMessage: stringPtr(errMsg),
			})
		}
		// If existingCR.Spec.LabID == 0, it was created manually - allow duplicate
	}

	if !errors.IsNotFound(err) && err != nil {
		return fmt.Errorf("failed to check for existing CR: %w", err)
	}

	// 3. Idempotency check: CR exists with lab-id label?
	crList := &oplv1alpha1.ClusterRequestList{}
	err = r.List(ctx, crList, client.InNamespace(r.Config.SyncNamespace),
		client.MatchingLabels{
			"opl.openshiftpartnerlabs.com/lab-id": fmt.Sprintf("%d", lab.ID),
		})
	if err != nil {
		return fmt.Errorf("failed to list CRs by label: %w", err)
	}

	if len(crList.Items) > 0 {
		logger.Info("ClusterRequest with matching lab-id label found, marking as synced")
		return r.DBClient.MarkLabAsSynced(ctx, lab.ID, crList.Items[0].Namespace)
	}

	// 4. Fetch company name if company_id is set
	var companyName string
	if lab.CompanyID != nil {
		company, err := r.DBClient.GetCompanyByID(ctx, *lab.CompanyID)
		if err != nil {
			logger.Error(err, "Failed to fetch company", "companyID", *lab.CompanyID)
			// Continue with empty company name (non-critical error)
		} else {
			companyName = company.CompanyName
		}
	}

	// 5. Map lab to ClusterRequest
	cr := database.MapLabToClusterRequest(lab, companyName, r.Config.BaseDomain, r.Config.SyncNamespace)

	// 6. Create ClusterRequest
	if err := r.Create(ctx, cr); err != nil {
		return fmt.Errorf("failed to create ClusterRequest: %w", err)
	}

	logger.Info("Created ClusterRequest", "cr", cr.Name, "namespace", cr.Namespace)

	// 7. Mark lab as synced in database
	if err := r.DBClient.MarkLabAsSynced(ctx, lab.ID, cr.Namespace); err != nil {
		logger.Error(err, "Failed to mark lab as synced (CR was created but DB not updated)")
		// Don't return error - CR was created successfully
	}

	return nil
}

// Reconcile handles CR status updates (Mode 2: K8s → DB Sync)
func (r *DatabaseSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch ClusterRequest
	cr := &oplv1alpha1.ClusterRequest{}
	if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Only process database-sourced CRs (labId != 0)
	if cr.Spec.LabID == 0 {
		logger.V(1).Info("Skipping CR without labId (not from database)",
			"cr", cr.Name)
		return ctrl.Result{}, nil
	}

	// Only sync specific states back to database
	shouldSync := false
	switch cr.Status.State {
	case StateGitCommitted, StateProvisioning, StateComplete, StateFailed:
		shouldSync = true
	}

	if !shouldSync {
		logger.V(1).Info("State not synced back to database",
			"cr", cr.Name, "state", cr.Status.State)
		return ctrl.Result{}, nil
	}

	// Update database with current status
	status := database.LabK8sStatus{
		State:         cr.Status.State,
		GitCommitSHA:  stringPtr(cr.Status.GitCommitSHA),
		HiveClusterID: stringPtr(cr.Status.HiveClusterID),
	}

	if cr.Status.State == StateFailed {
		status.ErrorMessage = stringPtr(cr.Status.ErrorMessage)
	}

	if err := r.DBClient.UpdateLabK8sStatus(ctx, cr.Spec.LabID, status); err != nil {
		logger.Error(err, "Failed to sync CR status to database",
			"cr", cr.Name, "labID", cr.Spec.LabID, "state", cr.Status.State)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	logger.Info("Synced CR status to database",
		"cr", cr.Name, "labID", cr.Spec.LabID, "state", cr.Status.State)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *DatabaseSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&oplv1alpha1.ClusterRequest{}).
		Named("database-sync").
		Complete(r)
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
