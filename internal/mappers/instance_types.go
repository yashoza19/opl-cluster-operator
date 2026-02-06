package mappers

// ClusterSizeConfig defines instance types and replica counts for different cluster sizes
type ClusterSizeConfig struct {
	ControlPlaneType     string
	ControlPlaneReplicas int
	WorkerType           string
	WorkerReplicas       int
	Description          string
}

var clusterSizeMap = map[string]ClusterSizeConfig{
	"small": {
		ControlPlaneType:     "m7i.large",
		ControlPlaneReplicas: 3,
		WorkerType:           "m7i.large",
		WorkerReplicas:       3,
		Description:          "Small Cluster - 3 masters, 3 workers (m7i.large)",
	},
	"medium": {
		ControlPlaneType:     "m7i.xlarge",
		ControlPlaneReplicas: 3,
		WorkerType:           "m7i.xlarge",
		WorkerReplicas:       3,
		Description:          "Medium Cluster - 3 masters, 3 workers (m7i.xlarge)",
	},
	"large": {
		ControlPlaneType:     "m7i.2xlarge",
		ControlPlaneReplicas: 3,
		WorkerType:           "m7i.2xlarge",
		WorkerReplicas:       3,
		Description:          "Large Cluster - 3 masters, 3 workers (m7i.2xlarge)",
	},
	"xl": {
		ControlPlaneType:     "m7i.4xlarge",
		ControlPlaneReplicas: 3,
		WorkerType:           "m7i.4xlarge",
		WorkerReplicas:       3,
		Description:          "XL Cluster - 16 vCPU 64GB RAM - 3 masters, 3 workers (m7i.4xlarge)",
	},
}

// GetClusterSizeConfig returns the instance and replica configuration for a given cluster size
func GetClusterSizeConfig(clusterSize string) (ClusterSizeConfig, bool) {
	config, ok := clusterSizeMap[clusterSize]
	return config, ok
}

// RequestTypeInstanceOverride defines instance type overrides for special request types
// These override the cluster size defaults when specified
type RequestTypeInstanceOverride struct {
	ControlPlaneType string
	WorkerType       string
	Description      string
}

var requestTypeInstanceOverrides = map[string]RequestTypeInstanceOverride{
	"ocpv": {
		ControlPlaneType: "m6a.metal",   // Bare metal for OpenShift Virtualization - 96 vCPU 192GB RAM
		WorkerType:       "m6a.metal",
		Description:      "Bare metal instances for OpenShift Virtualization (96 vCPU, 192GB RAM, 500GB Storage)",
	},
	"rhoai": {
		ControlPlaneType: "m7i.xlarge",
		WorkerType:       "g5.2xlarge",   // NVIDIA A10G GPU instance
		Description:      "GPU instances for Red Hat OpenShift AI",
	},
	"nvidia": {
		ControlPlaneType: "m7i.xlarge",
		WorkerType:       "g5.2xlarge",   // NVIDIA A10G GPU instance
		Description:      "GPU instances for NVIDIA workloads",
	},
}

// GetRequestTypeInstanceOverride returns instance type overrides for special request types
// Returns the override config and true if an override exists, empty config and false otherwise
func GetRequestTypeInstanceOverride(requestType string) (RequestTypeInstanceOverride, bool) {
	override, ok := requestTypeInstanceOverrides[requestType]
	return override, ok
}
