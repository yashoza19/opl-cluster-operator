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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ClusterRequestSpec defines the desired state of ClusterRequest.
type ClusterRequestSpec struct {
	// ClusterName is the name of the OpenShift cluster to provision
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	ClusterName string `json:"clusterName"`

	// Environment specifies the environment type (development, staging, production)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=development;staging;production
	Environment string `json:"environment"`

	// OpenshiftVersion specifies the OpenShift version image reference
	// +kubebuilder:validation:Required
	// +kubebuilder:default="img4.20.10-x86-64-appsub"
	OpenshiftVersion string `json:"openshiftVersion"`

	// ClusterSize defines the cluster configuration size (small, medium, large)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=small;medium;large
	ClusterSize string `json:"clusterSize"`

	// Region specifies the AWS region for cluster deployment
	// +kubebuilder:validation:Required
	Region string `json:"region"`

	// BaseDomain is the base DNS domain for the cluster
	// +kubebuilder:validation:Required
	// +kubebuilder:default="openshiftpartnerlabs.com"
	BaseDomain string `json:"baseDomain"`

	// ControlPlane defines control plane node configuration
	// +optional
	ControlPlane *ControlPlaneConfig `json:"controlPlane,omitempty"`

	// Workers defines worker node configuration
	// +optional
	Workers *WorkersConfig `json:"workers,omitempty"`

	// Networking defines cluster networking configuration
	// +optional
	Networking *NetworkingConfig `json:"networking,omitempty"`

	// RequestedBy is the username/email of the person requesting the cluster
	// +kubebuilder:validation:Required
	RequestedBy string `json:"requestedBy"`

	// CompanyName is the company/organization name
	// +optional
	CompanyName string `json:"companyName,omitempty"`

	// LabID is the reference to the database lab ID
	// +optional
	LabID int `json:"labId,omitempty"`
}

// ControlPlaneConfig defines control plane node configuration
type ControlPlaneConfig struct {
	// InstanceType is the AWS instance type for control plane nodes
	// +kubebuilder:default="m5.xlarge"
	InstanceType string `json:"instanceType,omitempty"`

	// Replicas is the number of control plane nodes
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=5
	// +kubebuilder:default=3
	Replicas int `json:"replicas,omitempty"`
}

// WorkersConfig defines worker node configuration
type WorkersConfig struct {
	// InstanceType is the AWS instance type for worker nodes
	// +kubebuilder:default="m5.2xlarge"
	InstanceType string `json:"instanceType,omitempty"`

	// Replicas is the number of worker nodes
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=3
	Replicas int `json:"replicas,omitempty"`

	// Zones is the list of availability zones for worker nodes
	// +optional
	Zones []string `json:"zones,omitempty"`
}

// NetworkingConfig defines cluster networking configuration
type NetworkingConfig struct {
	// ClusterNetworkCIDR is the CIDR for pod networking
	// +kubebuilder:default="10.128.0.0/14"
	ClusterNetworkCIDR string `json:"clusterNetworkCIDR,omitempty"`

	// ServiceNetworkCIDR is the CIDR for service networking
	// +kubebuilder:default="172.30.0.0/16"
	ServiceNetworkCIDR string `json:"serviceNetworkCIDR,omitempty"`

	// MachineNetworkCIDR is the CIDR for machine networking
	// +kubebuilder:default="10.0.0.0/16"
	MachineNetworkCIDR string `json:"machineNetworkCIDR,omitempty"`
}

// ClusterRequestStatus defines the observed state of ClusterRequest.
type ClusterRequestStatus struct {
	// State represents the current state of the cluster request
	// +kubebuilder:validation:Enum=pending;approved;git-committed;provisioning;complete;failed
	State string `json:"state,omitempty"`

	// Conditions represent the latest available observations of the cluster request's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// GitCommitSHA is the SHA of the git commit that created the cluster configuration
	// +optional
	GitCommitSHA string `json:"gitCommitSHA,omitempty"`

	// ArgocdAppCreated indicates if the ArgoCD Application was created
	// +optional
	ArgocdAppCreated bool `json:"argocdAppCreated,omitempty"`

	// HiveClusterID is the Hive cluster ID from ClusterDeployment
	// +optional
	HiveClusterID string `json:"hiveClusterID,omitempty"`

	// HiveInfraID is the infrastructure ID from Hive
	// +optional
	HiveInfraID string `json:"hiveInfraID,omitempty"`

	// ClusterProvisioned indicates if the cluster has been successfully provisioned
	// +optional
	ClusterProvisioned bool `json:"clusterProvisioned,omitempty"`

	// ErrorMessage contains error details if the request failed
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`

	// ProcessingStartedAt is when processing began
	// +optional
	ProcessingStartedAt *metav1.Time `json:"processingStartedAt,omitempty"`

	// ProcessingCompletedAt is when processing completed
	// +optional
	ProcessingCompletedAt *metav1.Time `json:"processingCompletedAt,omitempty"`

	// RetryCount tracks the number of reconciliation retries
	// +optional
	RetryCount int `json:"retryCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=cr;creq,scope=Namespaced
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.spec.clusterSize`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="GitCommit",type=string,JSONPath=`.status.gitCommitSHA`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterRequest is the Schema for the clusterrequests API.
type ClusterRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterRequestSpec   `json:"spec,omitempty"`
	Status ClusterRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterRequestList contains a list of ClusterRequest.
type ClusterRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterRequest{}, &ClusterRequestList{})
}
