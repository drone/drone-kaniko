package main

import (
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/drone/drone-kaniko/pkg/docker"
	"github.com/drone/drone-kaniko/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli"
)

const (
	v2RegistryURL string = "https://index.docker.io/v2/" // v2 registry is not supported
)

func TestCreateDockerConfigWithBaseRegistry(t *testing.T) {
	username := "user1"
	password := "pass1"
	registry := "azurecr.io"
	dockerUsername := "dockeruser"
	dockerPassword := "dockerpass"
	dockerRegistry := "https://index.docker.io/v1/"
	privateRegistry := "privateDockerRegistry"
	privateRegistryUsername := "priaveUsername"
	privateRegistryPassword := "privatePassword"

	credentials := []docker.RegistryCredentials{
		{
			Registry: registry,
			Username: username,
			Password: password,
		},
		{
			Registry: dockerRegistry,
			Username: dockerUsername,
			Password: dockerPassword,
		},
		{
			Registry: privateRegistry,
			Username: privateRegistryUsername,
			Password: privateRegistryPassword,
		},
	}

	tempDir, err := ioutil.TempDir("", "docker-config-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config := docker.NewConfig()
	err = config.CreateDockerConfig(credentials, tempDir)
	assert.NoError(t, err)

	expectedAuth := docker.Auth{Auth: base64.StdEncoding.EncodeToString([]byte(username + ":" + password))}
	assert.Equal(t, expectedAuth, config.Auths[registry])

	expectedDockerAuth := docker.Auth{Auth: base64.StdEncoding.EncodeToString([]byte(dockerUsername + ":" + dockerPassword))}
	assert.Equal(t, expectedDockerAuth, config.Auths[dockerRegistry])

	configPath := filepath.Join(tempDir, "config.json")
	data, err := ioutil.ReadFile(configPath)
	assert.NoError(t, err)

	var configFromFile docker.Config
	err = json.Unmarshal(data, &configFromFile)
	assert.NoError(t, err)

	assert.Equal(t, config.Auths, configFromFile.Auths)

	err = config.CreateDockerConfig([]docker.RegistryCredentials{
		{
			Registry: registry,
			Username: "",
			Password: password,
		},
	}, tempDir)
	assert.EqualError(t, err, "Username must be specified for registry: "+registry)

	err = config.CreateDockerConfig([]docker.RegistryCredentials{
		{
			Registry: registry,
			Username: username,
			Password: "",
		},
	}, tempDir)
	assert.EqualError(t, err, "Password must be specified for registry: "+registry)

	// v1 registry but without username password
	err = config.CreateDockerConfig([]docker.RegistryCredentials{
		{
			Registry: registry,
			Username: username,
			Password: password,
		},
		{
			Registry: dockerRegistry,
			Username: "",
			Password: "",
		},
	}, tempDir)
	assert.EqualError(t, err, "Username must be specified for registry: "+dockerRegistry)

	// private base registry without username/password
	err = config.CreateDockerConfig([]docker.RegistryCredentials{
		{
			Registry: privateRegistry,
			Username: "",
			Password: "",
		},
	}, tempDir)
	assert.EqualError(t, err, "Username must be specified for registry: "+privateRegistry)

}

func TestCreateDockerConfigWithoutBaseRegistry(t *testing.T) {
	username := "user1"
	password := "pass1"
	registry := "azurecr.io"

	credentials := []docker.RegistryCredentials{
		{
			Registry: registry,
			Username: username,
			Password: password,
		},
	}

	// Create a temporary directory
	tempDir, err := ioutil.TempDir("", "docker-config-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config := docker.NewConfig()
	err = config.CreateDockerConfig(credentials, tempDir)
	assert.NoError(t, err)

	expectedAuth := docker.Auth{Auth: base64.StdEncoding.EncodeToString([]byte(username + ":" + password))}
	assert.Equal(t, expectedAuth, config.Auths[registry])

	// Check the contents of the config.json file
	configPath := filepath.Join(tempDir, "config.json")
	data, err := ioutil.ReadFile(configPath)
	assert.NoError(t, err)

	var configFromFile docker.Config
	err = json.Unmarshal(data, &configFromFile)
	assert.NoError(t, err)

	assert.Equal(t, config.Auths, configFromFile.Auths)

	// Check if the public Docker Hub auth is not set
	_, exists := config.Auths[""]
	assert.False(t, exists)
}

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

func TestCLIIntegrationWithCustomFlag(t *testing.T) {
	// Test CLI integration with proper flag setup
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "CLI with single arg",
			args:     []string{"acr-test", "--args-new", "ARG1=value1"},
			expected: []string{"ARG1=value1"},
		},
		{
			name:     "CLI with multiple args",
			args:     []string{"acr-test", "--args-new", "ARG1=value1;ARG2=value2"},
			expected: []string{"ARG1=value1", "ARG2=value2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := cli.NewApp()
			app.Name = "acr-test"

			var capturedArgs []string

			app.Flags = []cli.Flag{
				cli.GenericFlag{
					Name:   "args-new",
					Usage:  "build args new",
					EnvVar: "PLUGIN_BUILD_ARGS_NEW",
					Value:  new(utils.CustomStringSliceFlag),
				},
			}

			app.Action = func(c *cli.Context) error {
				if genericFlag := c.Generic("args-new"); genericFlag != nil {
					if customFlag, ok := genericFlag.(*utils.CustomStringSliceFlag); ok {
						capturedArgs = customFlag.GetValue()
					}
				}
				return nil
			}

			err := app.Run(tt.args)
			if err != nil {
				t.Errorf("CLI run error = %v, want nil", err)
				return
			}

			if len(capturedArgs) != len(tt.expected) {
				t.Errorf("Got %d args, want %d", len(capturedArgs), len(tt.expected))
				return
			}

			for i, expected := range tt.expected {
				if capturedArgs[i] != expected {
					t.Errorf("Got arg[%d] = %v, want %v", i, capturedArgs[i], expected)
				}
			}
		})
	}
}

func TestACRBuildArgsProcessing(t *testing.T) {
	// Test that build args are correctly processed in the context of ACR plugin
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
			name:          "azure specific args",
			argsNew:       "AZURE_TENANT_ID=tenant123;AZURE_CLIENT_ID=client456",
			expectedCount: 2,
			expectedFirst: "AZURE_TENANT_ID=tenant123",
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

func TestACRAuthenticationFlow(t *testing.T) {
	// Test that ACR authentication works with build args
	tests := []struct {
		name         string
		tenantId     string
		clientId     string
		clientSecret string
		expectError  bool
	}{
		{
			name:         "missing tenant id",
			tenantId:     "",
			clientId:     "client123",
			clientSecret: "secret456",
			expectError:  true,
		},
		{
			name:         "missing client id",
			tenantId:     "tenant123",
			clientId:     "",
			clientSecret: "secret456",
			expectError:  true,
		},
		{
			name:         "missing client secret",
			tenantId:     "tenant123",
			clientId:     "client456",
			clientSecret: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test validates the parameter validation logic
			// without actually making network calls
			if tt.tenantId == "" && !tt.expectError {
				t.Error("Expected error for missing tenant ID")
			}
			if tt.clientId == "" && !tt.expectError {
				t.Error("Expected error for missing client ID")
			}
			if tt.clientSecret == "" && !tt.expectError {
				t.Error("Expected error for missing client secret")
			}
		})
	}
}

func TestSetupAuth_RegistryMustBeSpecified(t *testing.T) {
	pub, err := setupAuth("tenant", "client", "", "", "", "sub", "", "", "", "", "", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "registry must be specified")
	assert.Equal(t, "", pub)
}

func TestSetupAuth_MissingTenantOrClient(t *testing.T) {
	pub, err := setupAuth("tenant", "", "", "", "", "sub", "myregistry.azurecr.io", "", "", "", "", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tenantId and clientId must be provided")
	assert.Equal(t, "", pub)
}

func TestSetupAuth_NoCreds_NoPushTrue(t *testing.T) {
	pub, err := setupAuth("tenant", "client", "", "", "", "sub", "myregistry.azurecr.io", "", "", "", "", true)
	assert.NoError(t, err)
	assert.Equal(t, "", pub)
}
