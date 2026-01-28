package mappers

import (
	"fmt"

	oplv1alpha1 "github.com/openshift-partner-labs/opl-cluster-operator/api/v1alpha1"
)

// ClusterConfig represents the configuration needed for cluster templates
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
	WorkerZones              []string
	ClusterNetworkCIDR       string
	ServiceNetworkCIDR       string
	MachineNetworkCIDR       string
	CompanyName              string
	RequestedBy              string
}

// ClusterSizeConfig defines instance types and replicas for different cluster sizes
type ClusterSizeConfig struct {
	ControlPlaneType     string
	ControlPlaneReplicas int
	WorkerType           string
	WorkerReplicas       int
}

var clusterSizeMap = map[string]ClusterSizeConfig{
	"small": {
		ControlPlaneType:     "m5.xlarge",
		ControlPlaneReplicas: 3,
		WorkerType:           "m5.2xlarge",
		WorkerReplicas:       2,
	},
	"medium": {
		ControlPlaneType:     "m5.xlarge",
		ControlPlaneReplicas: 3,
		WorkerType:           "m5.2xlarge",
		WorkerReplicas:       3,
	},
	"large": {
		ControlPlaneType:     "m5.2xlarge",
		ControlPlaneReplicas: 3,
		WorkerType:           "m5.4xlarge",
		WorkerReplicas:       6,
	},
}

// MapClusterRequest converts a ClusterRequest CR to a ClusterConfig
func MapClusterRequest(cr *oplv1alpha1.ClusterRequest) ClusterConfig {
	// Get size configuration
	sizeConfig, ok := clusterSizeMap[cr.Spec.ClusterSize]
	if !ok {
		// Default to medium if size not found
		sizeConfig = clusterSizeMap["medium"]
	}

	// Use explicit control plane config if provided, otherwise use size defaults
	controlPlaneType := sizeConfig.ControlPlaneType
	controlPlaneReplicas := sizeConfig.ControlPlaneReplicas
	if cr.Spec.ControlPlane != nil {
		if cr.Spec.ControlPlane.InstanceType != "" {
			controlPlaneType = cr.Spec.ControlPlane.InstanceType
		}
		if cr.Spec.ControlPlane.Replicas > 0 {
			controlPlaneReplicas = cr.Spec.ControlPlane.Replicas
		}
	}

	// Use explicit worker config if provided, otherwise use size defaults
	workerType := sizeConfig.WorkerType
	workerReplicas := sizeConfig.WorkerReplicas
	var workerZones []string
	if cr.Spec.Workers != nil {
		if cr.Spec.Workers.InstanceType != "" {
			workerType = cr.Spec.Workers.InstanceType
		}
		if cr.Spec.Workers.Replicas > 0 {
			workerReplicas = cr.Spec.Workers.Replicas
		}
		if len(cr.Spec.Workers.Zones) > 0 {
			workerZones = cr.Spec.Workers.Zones
		}
	}

	// Default zones if not specified
	if len(workerZones) == 0 {
		workerZones = []string{
			fmt.Sprintf("%sa", cr.Spec.Region),
			fmt.Sprintf("%sb", cr.Spec.Region),
			fmt.Sprintf("%sc", cr.Spec.Region),
		}
	}

	// Networking defaults
	clusterNetworkCIDR := "10.128.0.0/14"
	serviceNetworkCIDR := "172.30.0.0/16"
	machineNetworkCIDR := "10.0.0.0/16"
	if cr.Spec.Networking != nil {
		if cr.Spec.Networking.ClusterNetworkCIDR != "" {
			clusterNetworkCIDR = cr.Spec.Networking.ClusterNetworkCIDR
		}
		if cr.Spec.Networking.ServiceNetworkCIDR != "" {
			serviceNetworkCIDR = cr.Spec.Networking.ServiceNetworkCIDR
		}
		if cr.Spec.Networking.MachineNetworkCIDR != "" {
			machineNetworkCIDR = cr.Spec.Networking.MachineNetworkCIDR
		}
	}

	return ClusterConfig{
		ClusterName:              cr.Spec.ClusterName,
		Environment:              cr.Spec.Environment,
		CloudProvider:            "aws", // Currently only AWS is supported
		Region:                   cr.Spec.Region,
		BaseDomain:               cr.Spec.BaseDomain,
		OpenshiftVersion:         cr.Spec.OpenshiftVersion,
		ControlPlaneInstanceType: controlPlaneType,
		ControlPlaneReplicas:     controlPlaneReplicas,
		WorkerInstanceType:       workerType,
		WorkerReplicas:           workerReplicas,
		WorkerZones:              workerZones,
		ClusterNetworkCIDR:       clusterNetworkCIDR,
		ServiceNetworkCIDR:       serviceNetworkCIDR,
		MachineNetworkCIDR:       machineNetworkCIDR,
		CompanyName:              cr.Spec.CompanyName,
		RequestedBy:              cr.Spec.RequestedBy,
	}
}
