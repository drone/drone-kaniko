package gcp

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
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

	// Debug: print idToken
	fmt.Printf("idToken: %s\n", idToken)

	idTokenPath := fmt.Sprintf("%s/id_token", idTokenDir)
	if err := os.WriteFile(idTokenPath, []byte(idToken), 0644); err != nil {
		return "", fmt.Errorf("failed to write idToken to file: %w", err)
	}

	fmt.Printf("idTokenPath: %s\n", idTokenPath)

	credsDir := fmt.Sprintf("%s/.config/gcloud", homeDir)
	err = os.MkdirAll(credsDir, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create gcloud directory: %w", err)
	}

	// Create application default credentials file at $HOME/.config/gcloud/application_default_credentials.json
	credsPath := fmt.Sprintf("%s/application_default_credentials.json", credsDir)

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

	// Debug: print generated JSON
	fmt.Printf("Generated JSON: %s\n", string(jsonData))

	err = os.WriteFile(credsPath, jsonData, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write to credentials file: %w", err)
	}

	fmt.Printf("credsPath: %s\n", credsPath)

	// Run the gcloud auth login command using the generated credentials file
	loginCmd := exec.Command("gcloud", "auth", "login", "--brief", "--cred-file", credsPath)
	loginCmd.Stdout = os.Stdout
	loginCmd.Stderr = os.Stderr
	err = loginCmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to execute 'gcloud auth login': %w", err)
	}

	// Run the gcloud config config-helper command to get the authenticated token
	helperCmd := exec.Command("gcloud", "config", "config-helper", "--format=json(credential)")
	helperCmd.Stdout = os.Stdout
	helperCmd.Stderr = os.Stderr
	err = helperCmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to execute 'gcloud config config-helper': %w", err)
	}

	// Read and return the content of the credentials JSON file
	credsData, err := ioutil.ReadFile(credsPath)
	if err != nil {
		return "", fmt.Errorf("failed to read the credentials file: %w", err)
	}

	return string(credsData), nil
}
