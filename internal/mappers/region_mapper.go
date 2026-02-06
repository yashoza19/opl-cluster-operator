package mappers

import (
	"fmt"
)

// RegionMapping maps database regions to AWS regions
var regionMappings = map[string]string{
	"na1":  "us-east-1",     // Eastern Time Zone
	"na2":  "us-central-1",  // Central Time Zone
	"na3":  "us-west-1",     // Pacific Time Zone
	"apac": "ap-southeast-1", // Asia Pacific
	"emea": "eu-west-1",     // Europe, Middle East, Africa
}

// MapRegionToAWS maps a database region (na1, na2, etc.) to an AWS region
func MapRegionToAWS(databaseRegion string) string {
	if awsRegion, ok := regionMappings[databaseRegion]; ok {
		return awsRegion
	}

	// If no mapping found, assume it's already an AWS region
	// (for backwards compatibility or direct region specifications)
	return databaseRegion
}

// GetAWSAvailabilityZones returns the availability zones for a given AWS region
func GetAWSAvailabilityZones(awsRegion string) []string {
	// AWS uses letter suffixes (a, b, c)
	return []string{
		fmt.Sprintf("%sa", awsRegion),
		fmt.Sprintf("%sb", awsRegion),
		fmt.Sprintf("%sc", awsRegion),
	}
}
