package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
)

func CreateTemporaryJsonKey(ctx context.Context, serviceAccountEmail string) (string, error) {
	// Create a temporary credentials file
	credsFile, err := os.CreateTemp("", "temp_creds.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary credentials file: %w", err)
	}
	defer credsFile.Close()

	// Use the Google Cloud SDK to create a temporary JSON key
	cmd := exec.Command("gcloud", "auth", "activate-service-account", "--key-file", credsFile.Name(), "--impersonated-service-account", serviceAccountEmail)
	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to create temporary JSON key: %w", err)
	}

	// Read the contents of the temporary JSON key file
	_, err = os.ReadFile(credsFile.Name())
	if err != nil {
		return "", fmt.Errorf("failed to read temporary JSON key: %w", err)
	}

	return credsFile.Name(), nil
}

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

	// Don't write the service account email to the file
	// Instead, configure kaniko/config.json to use the OIDC access token

	data := map[string]interface{}{
		"type":               "external_account",
		"audience":           fmt.Sprintf("//iam.googleapis.com/projects/%s/locations/global/workloadIdentityPools/%s/providers/%s", projectNumber, workforcePoolID, providerID),
		"subject_token_type": "urn:ietf:params:oauth:token-type:id_token",
		"token_url":          "https://sts.googleapis.com/v1/token",
		"credential_source": map[string]string{
			"file": idTokenPath, // Point to the stored OIDC token
		},
	}

	// Create temporary JSON key
	tempJsonKeyPath, err := CreateTemporaryJsonKey(context.Background(), serviceAccountEmail)
	if err != nil {
		return "", fmt.Errorf("failed to create temporary JSON key: %w", err)
	}

	// Read temporary JSON key content
	tempKeyData, err := os.ReadFile(tempJsonKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read temporary JSON key: %w", err)
	}

	// Combine temporary key data with OIDC token configuration
	var combinedData map[string]interface{}
	if err := json.Unmarshal(tempKeyData, &combinedData); err != nil {
		return "", fmt.Errorf("failed to unmarshal temporary JSON key data: %w", err)
	}
	for key, value := range data {
		combinedData[key] = value
	}

	jsonData, err := json.MarshalIndent(combinedData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON data: %w", err)
	}

	err = os.WriteFile(credsPath, jsonData, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write to credentials file: %w", err)
	}

	fmt.Printf("credsPath: %s\n", credsPath)
	log.Printf("Credentials (OIDC token) written to file: %s\n", idTokenPath)
	log.Printf("File content (kaniko/config.json): %s\n", string(jsonData))

	// Return the final JSON data
	return string(jsonData), nil
}
