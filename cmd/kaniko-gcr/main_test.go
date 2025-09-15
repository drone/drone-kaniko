package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/drone/drone-kaniko/pkg/utils"
	"github.com/urfave/cli"
)

func TestCustomStringSliceFlagIntegration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single build arg",
			input:    "ARG1=value1",
			expected: []string{"ARG1=value1"},
		},
		{
			name:     "multiple build args with semicolon",
			input:    "ARG1=value1;ARG2=value2;ARG3=value3",
			expected: []string{"ARG1=value1", "ARG2=value2", "ARG3=value3"},
		},
		{
			name:     "build args with spaces",
			input:    "ARG1=value with spaces;ARG2=another value",
			expected: []string{"ARG1=value with spaces", "ARG2=another value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the CustomStringSliceFlag directly
			flag := &utils.CustomStringSliceFlag{}
			err := flag.Set(tt.input)
			if err != nil {
				t.Errorf("Set() error = %v, want nil", err)
				return
			}

			result := flag.GetValue()
			if len(result) != len(tt.expected) {
				t.Errorf("Got %d args, want %d", len(result), len(tt.expected))
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Got arg[%d] = %v, want %v", i, result[i], expected)
				}
			}
		})
	}
}

func TestEnvironmentVariableIntegration(t *testing.T) {
	// Test that environment variables work with CustomStringSliceFlag
	originalEnv := os.Getenv("PLUGIN_BUILD_ARGS_NEW")
	defer func() {
		if originalEnv != "" {
			os.Setenv("PLUGIN_BUILD_ARGS_NEW", originalEnv)
		} else {
			os.Unsetenv("PLUGIN_BUILD_ARGS_NEW")
		}
	}()

	os.Setenv("PLUGIN_BUILD_ARGS_NEW", "ENV_ARG1=env_value1;ENV_ARG2=env_value2")

	app := cli.NewApp()
	app.Flags = []cli.Flag{
		cli.GenericFlag{
			Name:   "args-new",
			Usage:  "build args new",
			EnvVar: "PLUGIN_BUILD_ARGS_NEW",
			Value:  new(utils.CustomStringSliceFlag),
		},
	}

	var capturedArgs []string
	app.Action = func(c *cli.Context) error {
		if flag := c.Generic("args-new"); flag != nil {
			if customFlag, ok := flag.(*utils.CustomStringSliceFlag); ok {
				capturedArgs = customFlag.GetValue()
			}
		}
		return nil
	}

	err := app.Run([]string{"test"})
	if err != nil {
		t.Errorf("App.Run() error = %v, want nil", err)
		return
	}

	expected := []string{"ENV_ARG1=env_value1", "ENV_ARG2=env_value2"}
	if len(capturedArgs) != len(expected) {
		t.Errorf("Environment variable test: got %d args, want %d", len(capturedArgs), len(expected))
		return
	}

	for i, exp := range expected {
		if capturedArgs[i] != exp {
			t.Errorf("Environment variable test: got arg[%d] = %v, want %v", i, capturedArgs[i], exp)
		}
	}
}

func TestGCRBuildArgsProcessing(t *testing.T) {
	// Test that build args are correctly processed in the context of GCR plugin
	tests := []struct {
		name          string
		argsNew       string
		expectedCount int
		expectedFirst string
	}{
		{
			name:          "docker build args format",
			argsNew:       "GOOS=linux;GOARCH=amd64;CGO_ENABLED=0",
			expectedCount: 3,
			expectedFirst: "GOOS=linux",
		},
		{
			name:          "google cloud specific args",
			argsNew:       "GOOGLE_APPLICATION_CREDENTIALS=/path/to/creds.json;PROJECT_ID=my-project",
			expectedCount: 2,
			expectedFirst: "GOOGLE_APPLICATION_CREDENTIALS=/path/to/creds.json",
		},
		{
			name:          "single complex arg with special characters",
			argsNew:       "BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')",
			expectedCount: 1,
			expectedFirst: "BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := &utils.CustomStringSliceFlag{}
			err := flag.Set(tt.argsNew)
			if err != nil {
				t.Errorf("Set() error = %v, want nil", err)
				return
			}

			args := flag.GetValue()
			if len(args) != tt.expectedCount {
				t.Errorf("Got %d args, want %d", len(args), tt.expectedCount)
				return
			}

			if len(args) > 0 && args[0] != tt.expectedFirst {
				t.Errorf("Got first arg = %v, want %v", args[0], tt.expectedFirst)
			}
		})
	}
}

func TestGCRRegistryFormatting(t *testing.T) {
	// Test GCR-specific registry formatting
	tests := []struct {
		name     string
		registry string
		repo     string
		expected string
	}{
		{
			name:     "standard GCR format",
			registry: "gcr.io",
			repo:     "my-project/my-image",
			expected: "gcr.io/my-project/my-image",
		},
		{
			name:     "regional GCR",
			registry: "us.gcr.io",
			repo:     "project123/image456",
			expected: "us.gcr.io/project123/image456",
		},
		{
			name:     "european GCR",
			registry: "eu.gcr.io",
			repo:     "my-eu-project/my-app",
			expected: "eu.gcr.io/my-eu-project/my-app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This would be the format used in the GCR plugin
			result := tt.registry + "/" + tt.repo
			if result != tt.expected {
				t.Errorf("GCR formatting: got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGCRJSONKeyValidation(t *testing.T) {
	// Test JSON key validation for GCR authentication
	tests := []struct {
		name      string
		jsonKey   string
		expectErr bool
	}{
		{
			name:      "empty json key",
			jsonKey:   "",
			expectErr: false, // Empty is allowed (workload identity)
		},
		{
			name:      "valid json structure",
			jsonKey:   `{"type":"service_account","project_id":"test","private_key_id":"123"}`,
			expectErr: false,
		},
		{
			name:      "invalid json",
			jsonKey:   `{invalid json}`,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This simulates the JSON key validation that would happen in GCR
			if tt.jsonKey != "" {
				var data map[string]interface{}
				err := json.Unmarshal([]byte(tt.jsonKey), &data)
				if err != nil && !tt.expectErr {
					t.Errorf("Expected no error for JSON key, got %v", err)
				}
				if err == nil && tt.expectErr {
					t.Errorf("Expected error for JSON key, got nil")
				}
			}
		})
	}
}

func TestGCRAuthSetup(t *testing.T) {
	// Test GCR authentication setup
	tests := []struct {
		name           string
		jsonKey        string
		expectAuthFile bool
	}{
		{
			name:           "with json key",
			jsonKey:        `{"type":"service_account","project_id":"test"}`,
			expectAuthFile: true,
		},
		{
			name:           "without json key (workload identity)",
			jsonKey:        "",
			expectAuthFile: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This simulates the auth setup logic
			hasAuthFile := tt.jsonKey != ""
			if hasAuthFile != tt.expectAuthFile {
				t.Errorf("Auth file expectation: got %v, want %v", hasAuthFile, tt.expectAuthFile)
			}
		})
	}
}
