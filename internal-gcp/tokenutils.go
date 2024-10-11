package gcp

import (
	"encoding/json"
	"fmt"
)

const (
	audienceFormat = "//iam.googleapis.com/projects/%s/locations/global/workloadIdentityPools/%s/providers/%s"
	scopeURL       = "https://www.googleapis.com/auth/cloud-platform"
)

func WriteCredentialsToFile(idToken, projectNumber, workforcePoolID, providerID, serviceAccountEmail string) (string, error) {
	// Creating the JSON structure for credentials
	data := map[string]interface{}{
		"type":                              "external_account",
		"audience":                          fmt.Sprintf("//iam.googleapis.com/projects/%s/locations/global/workloadIdentityPools/%s/providers/%s", projectNumber, workforcePoolID, providerID),
		"subject_token_type":                "urn:ietf:params:oauth:token-type:id_token",
		"token_url":                         "https://sts.googleapis.com/v1/token",
		"service_account_impersonation_url": fmt.Sprintf("https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken", serviceAccountEmail),
		"credential_source": map[string]string{
			"file": idToken, // Directly using idToken here instead of path
		},
	}

	// Marshal the JSON structure to return as a string
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal json data: %w", err)
	}

	// Return the JSON data as a string
	return string(jsonData), nil
}
