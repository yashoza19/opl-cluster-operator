package templates

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/openshift-partner-labs/opl-cluster-operator/internal/mappers"
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

	// Generate cluster-deployment.yaml
	clusterDeployment, err := g.generateClusterDeployment(config)
	if err != nil {
		return nil, fmt.Errorf("failed to generate cluster-deployment.yaml: %w", err)
	}
	files["cluster-deployment.yaml"] = clusterDeployment

	return files, nil
}

// generateKustomization generates the kustomization.yaml file
func (g *Generator) generateKustomization(config mappers.ClusterConfig) (string, error) {
	tmpl := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: {{.ClusterName}}

resources:
  - cluster-deployment.yaml

configMapGenerator:
  - name: cluster-config
    files:
      - cluster-config.yaml

labels:
  - pairs:
      environment: {{.Environment}}
      company: {{.CompanyName}}
      requested-by: {{.RequestedBy}}
`

	return g.render(tmpl, config)
}

// generateClusterConfig generates the cluster-config.yaml file
func (g *Generator) generateClusterConfig(config mappers.ClusterConfig) (string, error) {
	zonesYAML := ""
	for _, zone := range config.WorkerZones {
		zonesYAML += fmt.Sprintf("  - %s\n", zone)
	}
	zonesYAML = strings.TrimSuffix(zonesYAML, "\n")

	data := struct {
		mappers.ClusterConfig
		WorkerZonesYAML string
	}{
		ClusterConfig:   config,
		WorkerZonesYAML: zonesYAML,
	}

	tmpl := `# Cluster Configuration for {{.ClusterName}}
clusterName: {{.ClusterName}}
environment: {{.Environment}}
cloudProvider: {{.CloudProvider}}
region: {{.Region}}
baseDomain: {{.BaseDomain}}
openshiftVersion: {{.OpenshiftVersion}}

controlPlane:
  instanceType: {{.ControlPlaneInstanceType}}
  replicas: {{.ControlPlaneReplicas}}

workers:
  instanceType: {{.WorkerInstanceType}}
  replicas: {{.WorkerReplicas}}
  zones:
{{.WorkerZonesYAML}}

networking:
  clusterNetworkCIDR: {{.ClusterNetworkCIDR}}
  serviceNetworkCIDR: {{.ServiceNetworkCIDR}}
  machineNetworkCIDR: {{.MachineNetworkCIDR}}

metadata:
  companyName: {{.CompanyName}}
  requestedBy: {{.RequestedBy}}
`

	return g.render(tmpl, data)
}

// generateClusterDeployment generates the Hive ClusterDeployment manifest
func (g *Generator) generateClusterDeployment(config mappers.ClusterConfig) (string, error) {
	tmpl := `apiVersion: hive.openshift.io/v1
kind: ClusterDeployment
metadata:
  name: {{.ClusterName}}
  namespace: {{.ClusterName}}
  labels:
    environment: {{.Environment}}
    company: {{.CompanyName}}
spec:
  baseDomain: {{.BaseDomain}}
  clusterName: {{.ClusterName}}
  platform:
    aws:
      region: {{.Region}}
      credentialsSecretRef:
        name: aws-creds
  provisioning:
    imageSetRef:
      name: {{.OpenshiftVersion}}
    installConfigSecretRef:
      name: install-config
  pullSecretRef:
    name: pull-secret
---
apiVersion: v1
kind: Secret
metadata:
  name: install-config
  namespace: {{.ClusterName}}
type: Opaque
stringData:
  install-config.yaml: |
    apiVersion: v1
    baseDomain: {{.BaseDomain}}
    metadata:
      name: {{.ClusterName}}
    platform:
      aws:
        region: {{.Region}}
    controlPlane:
      name: master
      platform:
        aws:
          type: {{.ControlPlaneInstanceType}}
      replicas: {{.ControlPlaneReplicas}}
    compute:
    - name: worker
      platform:
        aws:
          type: {{.WorkerInstanceType}}
          zones:{{range .WorkerZones}}
          - {{.}}{{end}}
      replicas: {{.WorkerReplicas}}
    networking:
      clusterNetwork:
      - cidr: {{.ClusterNetworkCIDR}}
        hostPrefix: 23
      serviceNetwork:
      - {{.ServiceNetworkCIDR}}
      machineNetwork:
      - cidr: {{.MachineNetworkCIDR}}
      networkType: OVNKubernetes
    pullSecret: ""
    sshKey: ""
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
