package mappers

import (
	"fmt"
	"strings"
)

// VersionMapping maps database OpenShift version strings to ClusterImageSet names
type VersionMapping struct {
	DatabaseVersion  string // Version string from database (e.g., "4.20.10")
	ImageSetRef      string // ClusterImageSet name (e.g., "img4.20.10-x86-64-appsub")
	OpenshiftVersion string // Semantic version (e.g., "4.20.10")
	Description      string
}

var versionMappings = []VersionMapping{
	{
		DatabaseVersion:  "4.20.10",
		ImageSetRef:      "img4.20.10-x86-64-appsub",
		OpenshiftVersion: "4.20.10",
		Description:      "OpenShift 4.20.10",
	},
	{
		DatabaseVersion:  "4.20.9",
		ImageSetRef:      "img4.20.9-x86-64-appsub",
		OpenshiftVersion: "4.20.9",
		Description:      "OpenShift 4.20.9",
	},
	{
		DatabaseVersion:  "4.20",
		ImageSetRef:      "img4.20.10-x86-64-appsub",
		OpenshiftVersion: "4.20.10",
		Description:      "OpenShift 4.20 (latest)",
	},
	{
		DatabaseVersion:  "4.19.15",
		ImageSetRef:      "img4.19.15-x86-64-appsub",
		OpenshiftVersion: "4.19.15",
		Description:      "OpenShift 4.19.15",
	},
	{
		DatabaseVersion:  "4.19",
		ImageSetRef:      "img4.19.15-x86-64-appsub",
		OpenshiftVersion: "4.19.15",
		Description:      "OpenShift 4.19 (latest)",
	},
}

// MapVersionToImageSet maps a database version string to a ClusterImageSet reference
// Returns the imageSetRef name and semantic version
func MapVersionToImageSet(databaseVersion string) (imageSetRef string, semanticVersion string) {
	// Clean up the version string (remove whitespace, convert to lowercase)
	cleanVersion := strings.TrimSpace(databaseVersion)

	// Try exact match first
	for _, mapping := range versionMappings {
		if mapping.DatabaseVersion == cleanVersion {
			return mapping.ImageSetRef, mapping.OpenshiftVersion
		}
	}

	// If the version already looks like an imageSetRef (starts with "img"), use it as-is
	if strings.HasPrefix(cleanVersion, "img") {
		return cleanVersion, extractVersionFromImageSet(cleanVersion)
	}

	// If no mapping found and it's a version number, construct the imageSetRef
	// This handles cases where new versions aren't in the mapping yet
	if isVersionNumber(cleanVersion) {
		imageSetRef := fmt.Sprintf("img%s-x86-64-appsub", cleanVersion)
		return imageSetRef, cleanVersion
	}

	// Default fallback - use as-is
	return cleanVersion, cleanVersion
}

// extractVersionFromImageSet extracts the version number from an imageSetRef
// e.g., "img4.20.10-x86-64-appsub" -> "4.20.10"
func extractVersionFromImageSet(imageSetRef string) string {
	// Remove "img" prefix
	version := strings.TrimPrefix(imageSetRef, "img")

	// Find the first non-version character (usually a dash)
	for i, char := range version {
		if char != '.' && (char < '0' || char > '9') {
			return version[:i]
		}
	}

	return version
}

// isVersionNumber checks if a string looks like a version number (e.g., "4.20" or "4.20.10")
func isVersionNumber(s string) bool {
	// Simple check: contains at least one dot and only contains digits and dots
	hasDot := false
	for _, char := range s {
		if char == '.' {
			hasDot = true
		} else if char < '0' || char > '9' {
			return false
		}
	}
	return hasDot
}

// GetSupportedVersions returns a list of all supported OpenShift versions
func GetSupportedVersions() []VersionMapping {
	return versionMappings
}
