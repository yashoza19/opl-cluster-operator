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

package database

import (
	"context"
	"fmt"
)

// GetUnsyncedLabs retrieves labs that haven't been synced to Kubernetes yet
// Uses FOR UPDATE SKIP LOCKED for multi-replica safety
func (c *Client) GetUnsyncedLabs(ctx context.Context, batchSize int) ([]Lab, error) {
	if c.pool == nil {
		return nil, fmt.Errorf("connection pool not initialized")
	}

	query := `
		SELECT
			id, cluster_id, generated_name, state, cluster_name,
			openshift_version, cluster_size, company_id, request_type,
			primary_email, region, hold, created_at, updated_at,
			k8s_synced, k8s_namespace, k8s_created_at
		FROM labs
		WHERE k8s_synced = false
		  AND state IN ('pending', 'approved')
		  AND hold = 0
		ORDER BY created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT $1
	`

	rows, err := c.pool.Query(ctx, query, batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to query unsynced labs: %w", err)
	}
	defer rows.Close()

	var labs []Lab
	for rows.Next() {
		var lab Lab
		err := rows.Scan(
			&lab.ID, &lab.ClusterID, &lab.GeneratedName, &lab.State,
			&lab.ClusterName, &lab.OpenshiftVersion, &lab.ClusterSize,
			&lab.CompanyID, &lab.RequestType, &lab.PrimaryEmail,
			&lab.Region, &lab.Hold, &lab.CreatedAt, &lab.UpdatedAt,
			&lab.K8sSynced, &lab.K8sNamespace, &lab.K8sCreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lab row: %w", err)
		}
		labs = append(labs, lab)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating lab rows: %w", err)
	}

	return labs, nil
}

// GetLabByID retrieves a specific lab by its ID
func (c *Client) GetLabByID(ctx context.Context, labID int) (*Lab, error) {
	if c.pool == nil {
		return nil, fmt.Errorf("connection pool not initialized")
	}

	query := `
		SELECT
			id, cluster_id, generated_name, state, cluster_name,
			openshift_version, cluster_size, company_id, request_type,
			primary_email, region, hold, created_at, updated_at,
			k8s_synced, k8s_namespace, k8s_created_at
		FROM labs
		WHERE id = $1
	`

	var lab Lab
	err := c.pool.QueryRow(ctx, query, labID).Scan(
		&lab.ID, &lab.ClusterID, &lab.GeneratedName, &lab.State,
		&lab.ClusterName, &lab.OpenshiftVersion, &lab.ClusterSize,
		&lab.CompanyID, &lab.RequestType, &lab.PrimaryEmail,
		&lab.Region, &lab.Hold, &lab.CreatedAt, &lab.UpdatedAt,
		&lab.K8sSynced, &lab.K8sNamespace, &lab.K8sCreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get lab %d: %w", labID, err)
	}

	return &lab, nil
}

// GetCompanyByID retrieves company information
func (c *Client) GetCompanyByID(ctx context.Context, companyID int) (*Company, error) {
	if c.pool == nil {
		return nil, fmt.Errorf("connection pool not initialized")
	}

	query := `
		SELECT id, company_name
		FROM companies
		WHERE id = $1
	`

	var company Company
	err := c.pool.QueryRow(ctx, query, companyID).Scan(
		&company.ID,
		&company.CompanyName,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get company %d: %w", companyID, err)
	}

	return &company, nil
}

// MarkLabAsSynced marks a lab as synced to Kubernetes (transactional)
func (c *Client) MarkLabAsSynced(ctx context.Context, labID int, namespace string) error {
	if c.pool == nil {
		return fmt.Errorf("connection pool not initialized")
	}

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // Rollback if not committed

	query := `
		UPDATE labs
		SET k8s_synced = true,
		    k8s_namespace = $1,
		    k8s_created_at = NOW(),
		    updated_at = NOW()
		WHERE id = $2
	`

	result, err := tx.Exec(ctx, query, namespace, labID)
	if err != nil {
		return fmt.Errorf("failed to mark lab as synced: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("lab ID %d not found", labID)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// UpdateLabK8sStatus updates the lab state and Kubernetes-related metadata
func (c *Client) UpdateLabK8sStatus(ctx context.Context, labID int, status LabK8sStatus) error {
	if c.pool == nil {
		return fmt.Errorf("connection pool not initialized")
	}

	query := `
		UPDATE labs
		SET state = $1,
		    updated_at = NOW()
		WHERE id = $2
	`

	result, err := c.pool.Exec(ctx, query, status.State, labID)
	if err != nil {
		return fmt.Errorf("failed to update lab status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("lab ID %d not found", labID)
	}

	return nil
}
