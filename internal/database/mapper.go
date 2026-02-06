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
	"fmt"
	"regexp"

	oplv1alpha1 "github.com/yashoza19/opl-cluster-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MapLabToClusterRequest converts a Lab record to ClusterRequest CR
func MapLabToClusterRequest(lab Lab, companyName string, baseDomain string, namespace string) *oplv1alpha1.ClusterRequest {
	// Default cloud provider to AWS if not specified
	cloudProvider := lab.CloudProvider
	if cloudProvider == "" {
		cloudProvider = "aws"
	}

	return &oplv1alpha1.ClusterRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lab.ClusterName,
			Namespace: namespace,
			Labels: map[string]string{
				"opl.openshiftpartnerlabs.com/lab-id":       fmt.Sprintf("%d", lab.ID),
				"opl.openshiftpartnerlabs.com/source":       "database",
				"opl.openshiftpartnerlabs.com/request-type": lab.RequestType,
			},
		},
		Spec: oplv1alpha1.ClusterRequestSpec{
			ClusterName:      lab.ClusterName,
			Environment:      mapEnvironment(lab.RequestType),
			OpenshiftVersion: lab.OpenshiftVersion,
			ClusterSize:      lab.ClusterSize,
			CloudProvider:    cloudProvider,
			Region:           lab.Region,
			BaseDomain:       baseDomain,
			RequestedBy:      lab.PrimaryEmail,
			CompanyName:      companyName,
			LabID:            lab.ID,
			RequestType:      lab.RequestType,
		},
	}
}

// mapEnvironment maps database request_type to environment enum
func mapEnvironment(requestType string) string {
	switch requestType {
	case "demo", "trial":
		return "development"
	case "production":
		return "production"
	default:
		return "staging"
	}
}

// ValidateLab validates lab data before creating ClusterRequest
func ValidateLab(lab Lab) error {
	// Validate cluster name (DNS compliance)
	if !isDNSCompliant(lab.ClusterName) {
		return fmt.Errorf("cluster_name not DNS-compliant: %s (must be 1-63 lowercase alphanumeric characters, hyphens, start/end with alphanumeric)", lab.ClusterName)
	}

	// Validate cluster size
	validSizes := map[string]bool{"small": true, "medium": true, "large": true}
	if !validSizes[lab.ClusterSize] {
		return fmt.Errorf("invalid cluster_size: %s (must be small, medium, or large)", lab.ClusterSize)
	}

	// Validate region
	if lab.Region == "" {
		return fmt.Errorf("region cannot be empty")
	}

	// Validate email
	if lab.PrimaryEmail == "" {
		return fmt.Errorf("primary_email cannot be empty")
	}

	// Check hold flag
	if lab.Hold != 0 {
		return fmt.Errorf("lab is on hold, skipping sync")
	}

	// Validate openshift version
	if lab.OpenshiftVersion == "" {
		return fmt.Errorf("openshift_version cannot be empty")
	}

	return nil
}

// isDNSCompliant checks if a string is DNS-1123 compliant (Kubernetes resource name)
func isDNSCompliant(name string) bool {
	// DNS-1123 label: 1-63 characters, lowercase alphanumeric or '-', start and end with alphanumeric
	dnsPattern := `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	matched, _ := regexp.MatchString(dnsPattern, name)
	return matched && len(name) <= 63 && len(name) >= 1
}
