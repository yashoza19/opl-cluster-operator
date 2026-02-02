package templates

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/yashoza19/opl-cluster-operator/internal/mappers"
)

// Generator handles template rendering for cluster configurations
type Generator struct{}

// NewGenerator creates a new template generator
func NewGenerator() *Generator {
	return &Generator{}
}

// GenerateClusterFiles generates all cluster configuration files
func (g *Generator) GenerateClusterFiles(config mappers.ClusterConfig) (map[string]string, error) {
	files := make(map[string]string)

	// Generate kustomization.yaml
	kustomization, err := g.generateKustomization(config)
	if err != nil {
		return nil, fmt.Errorf("failed to generate kustomization.yaml: %w", err)
	}
	files["kustomization.yaml"] = kustomization

	// Generate cluster-config.yaml
	clusterConfig, err := g.generateClusterConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to generate cluster-config.yaml: %w", err)
	}
	files["cluster-config.yaml"] = clusterConfig

	// Generate argocd-application.yaml
	argoCDApp, err := g.generateArgoCDApplication(config)
	if err != nil {
		return nil, fmt.Errorf("failed to generate argocd-application.yaml: %w", err)
	}
	files["argocd-application.yaml"] = argoCDApp

	return files, nil
}

// generateKustomization generates the kustomization.yaml file
func (g *Generator) generateKustomization(config mappers.ClusterConfig) (string, error) {
	zonesYAML := ""
	for _, zone := range config.WorkerZones {
		zonesYAML += fmt.Sprintf("                   - %s\n", zone)
	}
	zonesYAML = strings.TrimSuffix(zonesYAML, "\n")

	data := struct {
		mappers.ClusterConfig
		WorkerZonesYAML string
	}{
		ClusterConfig:   config,
		WorkerZonesYAML: zonesYAML,
	}

	tmpl := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: {{.ClusterName}}

resources:
  - ../../cluster-templates/aws-ha/base
  - cluster-config.yaml

# Patches to customize the cluster
patches:
  # Update ClusterDeployment with specific values
  - target:
      kind: ClusterDeployment
    patch: |-
     - op: replace
       path: /metadata/name
       value: {{.ClusterName}}
     - op: replace
       path: /metadata/namespace
       value: {{.ClusterName}}
     - op: replace
       path: /spec/clusterName
       value: {{.ClusterName}}
     - op: replace
       path: /spec/baseDomain
       value: {{.BaseDomain}}
     - op: replace
       path: /spec/platform/aws/region
       value: {{.Region}}
     - op: replace
       path: /spec/provisioning/imageSetRef/name
       value: {{.OpenshiftVersion}}
     - op: replace
       path: /spec/provisioning/installConfigSecretRef/name
       value: {{.ClusterName}}-install-config
     - op: replace
       path: /spec/provisioning/sshPrivateKeySecretRef/name
       value: {{.ClusterName}}-ssh-key
     - op: replace
       path: /spec/pullSecretRef/name
       value: {{.ClusterName}}-pull-secret
     - op: replace
       path: /spec/platform/aws/credentialsSecretRef/name
       value: {{.ClusterName}}-aws-credentials
     - op: remove
       path: /spec/provisioning/manifestsConfigMapRef

  # Update MachinePool with specific values
  - target:
      kind: MachinePool
    patch: |-
     - op: replace
       path: /metadata/name
       value: {{.ClusterName}}-worker
     - op: replace
       path: /metadata/namespace
       value: {{.ClusterName}}
     - op: replace
       path: /spec/clusterDeploymentRef/name
       value: {{.ClusterName}}
     - op: replace
       path: /spec/platform/aws/type
       value: {{.WorkerInstanceType}}
     - op: replace
       path: /spec/replicas
       value: {{.WorkerReplicas}}

  # Update ManagedCluster
  - target:
      kind: ManagedCluster
    patch: |-
     - op: replace
       path: /metadata/name
       value: {{.ClusterName}}
     - op: replace
       path: /metadata/labels/environment
       value: {{.Environment}}

  # Update KlusterletAddonConfig
  - target:
      kind: KlusterletAddonConfig
    patch: |-
     - op: replace
       path: /metadata/name
       value: {{.ClusterName}}
     - op: replace
       path: /metadata/namespace
       value: {{.ClusterName}}
     - op: replace
       path: /spec/clusterName
       value: {{.ClusterName}}
     - op: replace
       path: /spec/clusterNamespace
       value: {{.ClusterName}}

  # Update install-config Secret
  - target:
      kind: Secret
      name: cluster-placeholder-install-config
    patch: |-
     - op: replace
       path: /metadata/name
       value: {{.ClusterName}}-install-config
     - op: replace
       path: /metadata/namespace
       value: {{.ClusterName}}
     - op: replace
       path: /stringData/install-config.yaml
       value: |
         apiVersion: v1
         baseDomain: {{.BaseDomain}}
         metadata:
           name: {{.ClusterName}}

         controlPlane:
           name: master
           platform:
             aws:
               type: {{.ControlPlaneInstanceType}}
               rootVolume:
                 iops: 4000
                 size: 120
                 type: gp3
               zones:
                 - {{index .WorkerZones 0}}
           replicas: {{.ControlPlaneReplicas}}

         compute:
           - name: worker
             platform:
               aws:
                 type: {{.WorkerInstanceType}}
                 rootVolume:
                   iops: 2000
                   size: 100
                   type: gp3
                 zones:
{{.WorkerZonesYAML}}
             replicas: {{.WorkerReplicas}}

         networking:
           clusterNetwork:
             - cidr: {{.ClusterNetworkCIDR}}
               hostPrefix: 23
           machineNetwork:
             - cidr: {{.MachineNetworkCIDR}}
           serviceNetwork:
             - {{.ServiceNetworkCIDR}}
           networkType: OVNKubernetes

         platform:
           aws:
             region: {{.Region}}

         fips: false
         publish: External

# Hive will automatically populate clusterMetadata fields after provisioning
`

	return g.render(tmpl, data)
}

// generateClusterConfig generates the cluster-config.yaml file as a ConfigMap
func (g *Generator) generateClusterConfig(config mappers.ClusterConfig) (string, error) {
	// Join zones into a single string
	workerZones := strings.Join(config.WorkerZones, ",")

	data := struct {
		mappers.ClusterConfig
		WorkerZonesString string
	}{
		ClusterConfig:     config,
		WorkerZonesString: workerZones,
	}

	tmpl := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-config
  namespace: {{.ClusterName}}
data:
  clusterName: {{.ClusterName}}
  baseDomain: {{.BaseDomain}}
  region: {{.Region}}
  openshiftVersion: {{.OpenshiftVersion}}
  environment: {{.Environment}}

  # Control plane configuration
  controlPlaneInstanceType: {{.ControlPlaneInstanceType}}
  controlPlaneReplicas: "{{.ControlPlaneReplicas}}"

  # Worker configuration
  workerInstanceType: {{.WorkerInstanceType}}
  workerReplicas: "{{.WorkerReplicas}}"
  workerZones: "{{.WorkerZonesString}}"

  # Networking
  clusterNetworkCIDR: {{.ClusterNetworkCIDR}}
  serviceNetworkCIDR: {{.ServiceNetworkCIDR}}
  machineNetworkCIDR: {{.MachineNetworkCIDR}}
`

	return g.render(tmpl, data)
}

// generateArgoCDApplication generates the argocd-application.yaml file
func (g *Generator) generateArgoCDApplication(config mappers.ClusterConfig) (string, error) {
	tmpl := `---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: {{.ClusterName}}
  namespace: openshift-gitops
  annotations:
    argocd.argoproj.io/sync-wave: "10"
  labels:
    cluster-name: {{.ClusterName}}
    environment: {{.Environment}}
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: cluster-provisioning

  source:
    repoURL: https://github.com/yashoza19/opl-argocd.git
    targetRevision: master
    path: clusters/{{.ClusterName}}

  destination:
    server: https://kubernetes.default.svc
    namespace: {{.ClusterName}}

  syncPolicy:
    automated:
      prune: true
      selfHeal: false  # CRITICAL: Disable selfHeal to prevent deletion of Hive-created secrets
      allowEmpty: false
    syncOptions:
      - PrunePropagationPolicy=orphan
      - RespectIgnoreDifferences=true
    retry:
      limit: 5
      backoff:
        duration: 5s
        factor: 2
        maxDuration: 10m

  # Ignore differences in dynamic fields
  ignoreDifferences:
    - group: hive.openshift.io
      kind: ClusterDeployment
      jsonPointers:
        - /status
        - /spec/clusterMetadata  # Ignore entire clusterMetadata section (Hive-managed)
        - /spec/installed  # Ignore installed timestamp (Hive-managed)
    - group: cluster.open-cluster-management.io
      kind: ManagedCluster
      jsonPointers:
        - /status
        - /spec/managedClusterClientConfigs
`

	return g.render(tmpl, config)
}

// render executes a template with the given data
func (g *Generator) render(tmplStr string, data interface{}) (string, error) {
	tmpl, err := template.New("template").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}
