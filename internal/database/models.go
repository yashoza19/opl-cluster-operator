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

import "time"

// Lab represents a row from the labs table
type Lab struct {
	ID               int        `db:"id"`
	ClusterID        string     `db:"cluster_id"`
	GeneratedName    string     `db:"generated_name"`
	State            string     `db:"state"`
	ClusterName      string     `db:"cluster_name"`
	OpenshiftVersion string     `db:"openshift_version"`
	ClusterSize      string     `db:"cluster_size"`
	CompanyID        *int       `db:"company_id"` // nullable
	RequestType      string     `db:"request_type"`
	Partner          int        `db:"partner"`
	Sponsor          string     `db:"sponsor"`
	CloudProvider    string     `db:"cloud_provider"`
	PrimaryFirst     string     `db:"primary_first"`
	PrimaryLast      string     `db:"primary_last"`
	PrimaryEmail     string     `db:"primary_email"`
	SecondaryFirst   string     `db:"secondary_first"`
	SecondaryLast    string     `db:"secondary_last"`
	SecondaryEmail   string     `db:"secondary_email"`
	Region           string     `db:"region"`
	AlwaysOn         int        `db:"always_on"`
	ProjectName      string     `db:"project_name"`
	LeaseTime        string     `db:"lease_time"`
	Description      string     `db:"description"`
	Notes            string     `db:"notes"`
	StartDate        time.Time  `db:"start_date"`
	EndDate          time.Time  `db:"end_date"`
	Hold             int        `db:"hold"`
	CreatedAt        time.Time  `db:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at"`
	K8sSynced        bool       `db:"k8s_synced"`
	K8sNamespace     *string    `db:"k8s_namespace"`
	K8sCreatedAt     *time.Time `db:"k8s_created_at"`
}

// Company represents a row from the companies table
type Company struct {
	ID          int    `db:"id"`
	CompanyName string `db:"company_name"`
	// Add other fields as needed when accessed
}

// LabK8sStatus holds Kubernetes-related status for a lab
type LabK8sStatus struct {
	State         string
	GitCommitSHA  *string
	HiveClusterID *string
	ErrorMessage  *string
}
