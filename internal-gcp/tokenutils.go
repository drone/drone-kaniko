package gcp

import (
	"encoding/json"
	"fmt"
	"os"
)

func WriteCredentialsToFile(idToken, projectNumber, workforcePoolID, providerID, serviceAccountEmail string) (string, error) {
	homeDir := os.Getenv("DRONE_WORKSPACE")

	if homeDir == "" || homeDir == "/" {
		fmt.Print("could not get home directory, using /home/harness as home directory")
		homeDir = "/home/harness"
	}

	idTokenDir := fmt.Sprintf("%s/tmp", homeDir)
	err := os.MkdirAll(idTokenDir, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create tmp directory: %w", err)
	}

	idTokenPath := fmt.Sprintf("%s/id_token", idTokenDir)
	if err := os.WriteFile(idTokenPath, []byte(idToken), 0644); err != nil {
		return "", fmt.Errorf("failed to write idToken to file: %w", err)
	}

	fmt.Printf("idTokenPath: %s\n", idTokenPath)
	credsPath := "/kaniko/config.json"

	data := map[string]interface{}{
		"type":                              "external_account",
		"audience":                          fmt.Sprintf("//iam.googleapis.com/projects/%s/locations/global/workloadIdentityPools/%s/providers/%s", projectNumber, workforcePoolID, providerID),
		"subject_token_type":                "urn:ietf:params:oauth:token-type:id_token",
		"token_url":                         "https://sts.googleapis.com/v1/token",
		"service_account_impersonation_url": fmt.Sprintf("https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken", serviceAccountEmail),
		"credential_source": map[string]string{
			"file": idTokenPath,
		},
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal json data: %w", err)
	}

	err = os.WriteFile(credsPath, jsonData, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write to credentials file: %w", err)
	}

	fmt.Printf("credsPath: %s\n", credsPath)

	return credsPath, nil
}
