package mappers

import (
	"log"

	oplv1alpha1 "github.com/yashoza19/opl-cluster-operator/api/v1alpha1"
)

// ClusterConfig represents the configuration needed for cluster templates
type ClusterConfig struct {
	ClusterName              string
	Environment              string
	CloudProvider            string
	Region                   string
	DatabaseRegion           string // Original database region (na1, na2, etc.)
	BaseDomain               string
	OpenshiftVersion         string // Semantic version (e.g., "4.20.10")
	ImageSetRef              string // ClusterImageSet reference (e.g., "img4.20.10-x86-64-appsub")
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

// MapClusterRequest converts a ClusterRequest CR to a ClusterConfig
func MapClusterRequest(cr *oplv1alpha1.ClusterRequest) ClusterConfig {
	// Map database region to AWS region
	databaseRegion := cr.Spec.Region
	awsRegion := MapRegionToAWS(databaseRegion)

	log.Printf("Mapping region: database=%s -> aws=%s", databaseRegion, awsRegion)

	// Map OpenShift version to ClusterImageSet reference
	imageSetRef, semanticVersion := MapVersionToImageSet(cr.Spec.OpenshiftVersion)
	log.Printf("Mapping version: database=%s -> imageSetRef=%s, semanticVersion=%s",
		cr.Spec.OpenshiftVersion, imageSetRef, semanticVersion)

	// Get instance types and replica counts from cluster size (base configuration)
	sizeConfig, ok := GetClusterSizeConfig(cr.Spec.ClusterSize)
	if !ok {
		// Default to medium if size not found
		sizeConfig, _ = GetClusterSizeConfig("medium")
		log.Printf("Warning: Unknown cluster size '%s', defaulting to medium", cr.Spec.ClusterSize)
	}

	controlPlaneType := sizeConfig.ControlPlaneType
	controlPlaneReplicas := sizeConfig.ControlPlaneReplicas
	workerType := sizeConfig.WorkerType
	workerReplicas := sizeConfig.WorkerReplicas

	log.Printf("Cluster size selection: size=%s, controlPlane=%s, worker=%s, controlPlaneReplicas=%d, workerReplicas=%d",
		cr.Spec.ClusterSize, controlPlaneType, workerType, controlPlaneReplicas, workerReplicas)

	// Check for request type override (only for special workloads like OCPV, GPU, etc.)
	if cr.Spec.RequestType != "" {
		if override, ok := GetRequestTypeInstanceOverride(cr.Spec.RequestType); ok {
			log.Printf("Applying request type override: requestType=%s, controlPlane=%s -> %s, worker=%s -> %s",
				cr.Spec.RequestType, controlPlaneType, override.ControlPlaneType, workerType, override.WorkerType)
			controlPlaneType = override.ControlPlaneType
			workerType = override.WorkerType
		}
	}

	// Use explicit control plane config if provided (highest priority)
	if cr.Spec.ControlPlane != nil {
		if cr.Spec.ControlPlane.InstanceType != "" {
			log.Printf("Overriding control plane instance type from spec: %s -> %s",
				controlPlaneType, cr.Spec.ControlPlane.InstanceType)
			controlPlaneType = cr.Spec.ControlPlane.InstanceType
		}
		if cr.Spec.ControlPlane.Replicas > 0 {
			log.Printf("Overriding control plane replicas from spec: %d -> %d",
				controlPlaneReplicas, cr.Spec.ControlPlane.Replicas)
			controlPlaneReplicas = cr.Spec.ControlPlane.Replicas
		}
	}

	// Use explicit worker config if provided (highest priority)
	var workerZones []string
	if cr.Spec.Workers != nil {
		if cr.Spec.Workers.InstanceType != "" {
			log.Printf("Overriding worker instance type from spec: %s -> %s",
				workerType, cr.Spec.Workers.InstanceType)
			workerType = cr.Spec.Workers.InstanceType
		}
		// Worker replicas can be 0 (valid for clusters without workers)
		// Always use the explicit value when Workers config is provided
		log.Printf("Using worker replicas from spec: %d", cr.Spec.Workers.Replicas)
		workerReplicas = cr.Spec.Workers.Replicas
		if len(cr.Spec.Workers.Zones) > 0 {
			workerZones = cr.Spec.Workers.Zones
		}
	}

	// Default zones if not specified - use AWS availability zones
	if len(workerZones) == 0 {
		workerZones = GetAWSAvailabilityZones(awsRegion)
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
		Region:                   awsRegion,
		DatabaseRegion:           databaseRegion,
		BaseDomain:               cr.Spec.BaseDomain,
		OpenshiftVersion:         semanticVersion,
		ImageSetRef:              imageSetRef,
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
