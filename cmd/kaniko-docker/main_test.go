package main

import (
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"

	kaniko "github.com/drone/drone-kaniko"
	"github.com/drone/drone-kaniko/internal/artifactory"
	"github.com/drone/drone-kaniko/pkg/docker"
	"github.com/drone/drone-kaniko/pkg/utils"
	"github.com/urfave/cli"
)

func TestConfigureAuthRoutesLegacyAndOIDC(t *testing.T) {
	t.Run("missing OIDC inputs preserves legacy setup", func(t *testing.T) {
		var legacyCalls int
		originalLegacy := legacyDockerAuth
		originalExec := executeKaniko
		t.Cleanup(func() {
			legacyDockerAuth = originalLegacy
			executeKaniko = originalExec
		})
		legacyDockerAuth = func(_, _, _, _, _, _ string) error {
			legacyCalls++
			return nil
		}
		executeKaniko = func(kaniko.Plugin) error { return nil }

		if err := newApp().Run([]string{"kaniko-docker", "--repo", "team/image"}); err != nil {
			t.Fatalf("run() error = %v", err)
		}
		if legacyCalls != 1 {
			t.Fatalf("legacy auth calls = %d, want 1", legacyCalls)
		}
	})

	t.Run("OIDC inputs use OIDC setup", func(t *testing.T) {
		var oidcCalls int
		originalOIDC := setupOIDCAuth
		originalExec := executeKaniko
		t.Cleanup(func() {
			setupOIDCAuth = originalOIDC
			executeKaniko = originalExec
		})
		setupOIDCAuth = func(_ context.Context, inputs artifactory.Inputs, projectKey string) (authResources, error) {
			oidcCalls++
			if inputs.ServerURL != "https://jfrog.example.com" || inputs.IDToken != "id-token" ||
				inputs.ProviderName != "provider" || projectKey != "project" {
				t.Fatalf("unexpected OIDC inputs: %#v, project=%q", inputs, projectKey)
			}
			return authResources{cleanup: func() error { return nil }}, nil
		}
		executeKaniko = func(kaniko.Plugin) error { return nil }

		err := newApp().Run([]string{
			"kaniko-docker",
			"--url", "https://jfrog.example.com",
			"--artifactory-oidc-token", "id-token",
			"--artifactory-oidc-provider-name", "provider",
			"--artifactory-oidc-project-key", "project",
			"--registry", "jfrog.example.com",
			"--repo", "jfrog.example.com/team/image",
		})
		if err != nil {
			t.Fatalf("run() error = %v", err)
		}
		if oidcCalls != 1 {
			t.Fatalf("OIDC setup calls = %d, want 1", oidcCalls)
		}
	})
}

func TestPartialOIDCInputsFailInsteadOfFallingBack(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "URL only",
			args: []string{"kaniko-docker", "--url", "https://jfrog.example.com"},
			want: "OIDC ID token is required",
		},
		{
			name: "token only",
			args: []string{"kaniko-docker", "--artifactory-oidc-token", "id-token"},
			want: "JFrog server URL is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := newApp().Run(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("run error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCanonicalOIDCEnvironmentVariablesPopulateInputs(t *testing.T) {
	t.Setenv("PLUGIN_URL", "https://jfrog.example.com")
	t.Setenv("ARTIFACTORY_OIDC_TOKEN", "id-token")
	t.Setenv("ARTIFACTORY_OIDC_PROVIDER_NAME", "provider")
	t.Setenv("ARTIFACTORY_OIDC_PROJECT_KEY", "project")
	t.Setenv("PLUGIN_REGISTRY", "jfrog.example.com")
	t.Setenv("PLUGIN_REPO", "jfrog.example.com/team/image")
	originalOIDC := setupOIDCAuth
	originalExec := executeKaniko
	t.Cleanup(func() {
		setupOIDCAuth = originalOIDC
		executeKaniko = originalExec
	})
	setupOIDCAuth = func(_ context.Context, inputs artifactory.Inputs, projectKey string) (authResources, error) {
		if inputs.ServerURL != "https://jfrog.example.com" || inputs.IDToken != "id-token" ||
			inputs.ProviderName != "provider" || inputs.Registry != "jfrog.example.com" ||
			inputs.Repository != "jfrog.example.com/team/image" || inputs.DockerConfigRoot != "/kaniko" ||
			projectKey != "project" {
			t.Fatalf("canonical environment variables were not mapped: inputs=%#v project=%q", inputs, projectKey)
		}
		return authResources{cleanup: func() error { return nil }}, nil
	}
	executeKaniko = func(kaniko.Plugin) error { return nil }

	if err := newApp().Run([]string{"kaniko-docker"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
}

func TestRegistryCertificateLegacyPathAndOIDCRejection(t *testing.T) {
	t.Run("legacy mode passes the configured file path to Kaniko", func(t *testing.T) {
		originalExec := executeKaniko
		t.Cleanup(func() { executeKaniko = originalExec })
		executeKaniko = func(plugin kaniko.Plugin) error {
			if plugin.Build.RegistryCertificate != "/certs/registry-ca.pem" {
				t.Fatalf("registry certificate = %q", plugin.Build.RegistryCertificate)
			}
			return nil
		}

		if err := newApp().Run(
			[]string{
				"kaniko-docker", "--registry-certificate", "/certs/registry-ca.pem", "--repo", "team/image", "--no-push",
			},
		); err != nil {
			t.Fatalf("run() error = %v", err)
		}
	})

	t.Run("OIDC mode rejects registry certificate input", func(t *testing.T) {
		err := newApp().Run(append(oidcCLIArgs(), "--registry-certificate", "/certs/registry-ca.pem"))
		if err == nil || !strings.Contains(err.Error(), "does not support registry certificates") {
			t.Fatalf("run() error = %v, want unsupported registry certificate", err)
		}
	})

	t.Run("OIDC mode rejects registry client certificate input", func(t *testing.T) {
		err := newApp().Run(append(oidcCLIArgs(), "--registry-client-cert", "/certs/client.pem"))
		if err == nil || !strings.Contains(err.Error(), "incompatible with JFrog OIDC credentials") {
			t.Fatalf("run() error = %v, want unsupported registry client certificate", err)
		}
	})
}

func TestOIDCAuthRejectsInsecureBeforeExchange(t *testing.T) {
	originalExchange := exchangeOIDCCredential
	t.Cleanup(func() { exchangeOIDCCredential = originalExchange })
	exchangeOIDCCredential = func(context.Context, *http.Client, string, string, string, string) (artifactory.Credential, error) {
		t.Fatal("exchange must not run after local validation failure")
		return artifactory.Credential{}, nil
	}

	inputs := validOIDCInputs()
	inputs.Insecure = true
	if _, err := configureOIDCAuth(context.Background(), inputs, ""); err == nil {
		t.Fatal("insecure configuration error = nil")
	}
}

func TestOIDCAuthCleanupRunsAfterKanikoSuccessAndFailure(t *testing.T) {
	for _, execErr := range []error{nil, errors.New("kaniko failed")} {
		t.Run("execution", func(t *testing.T) {
			var cleanupCalls int
			originalOIDC := setupOIDCAuth
			originalExec := executeKaniko
			t.Cleanup(func() {
				setupOIDCAuth = originalOIDC
				executeKaniko = originalExec
			})
			setupOIDCAuth = func(context.Context, artifactory.Inputs, string) (authResources, error) {
				return authResources{cleanup: func() error {
					cleanupCalls++
					return nil
				}}, nil
			}
			executeKaniko = func(kaniko.Plugin) error { return execErr }

			err := newApp().Run(oidcCLIArgs())
			if !errors.Is(err, execErr) {
				t.Fatalf("run() error = %v, want %v", err, execErr)
			}
			if cleanupCalls != 1 {
				t.Fatalf("cleanup calls = %d, want 1", cleanupCalls)
			}
		})
	}
}

func validOIDCInputs() artifactory.Inputs {
	return artifactory.Inputs{
		ServerURL:        "https://jfrog.example.com",
		IDToken:          "id-token",
		ProviderName:     "provider",
		Registry:         "jfrog.example.com",
		Repository:       "jfrog.example.com/team/image",
		DockerConfigRoot: os.TempDir(),
	}
}

func oidcCLIArgs() []string {
	return []string{
		"kaniko-docker", "--url", "https://jfrog.example.com",
		"--artifactory-oidc-token", "id-token", "--artifactory-oidc-provider-name", "provider",
		"--registry", "jfrog.example.com", "--repo", "jfrog.example.com/team/image",
	}
}

func Test_buildRepo(t *testing.T) {
	tests := []struct {
		name     string
		registry string
		repo     string
		want     string
	}{
		{
			name: "dockerhub",
			repo: "golang",
			want: "golang",
		},
		{
			name:     "internal",
			registry: "artifactory.example.com",
			repo:     "service",
			want:     "artifactory.example.com/service",
		},
		{
			name:     "backward_compatibility",
			registry: "artifactory.example.com",
			repo:     "artifactory.example.com/service",
			want:     "artifactory.example.com/service",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildRepo(tt.registry, tt.repo, true); got != tt.want {
				t.Errorf("buildRepo(%q, %q) = %v, want %v", tt.registry, tt.repo, got, tt.want)
			}
		})
	}
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
			args:     []string{"docker-test", "--args-new", "ARG1=value1"},
			expected: []string{"ARG1=value1"},
		},
		{
			name:     "CLI with multiple args",
			args:     []string{"docker-test", "--args-new", "ARG1=value1;ARG2=value2"},
			expected: []string{"ARG1=value1", "ARG2=value2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := cli.NewApp()
			app.Name = "docker-test"

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

func TestDockerBuildArgsProcessing(t *testing.T) {
	// Test that build args are correctly processed in the context of Docker plugin
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
			name:          "single complex arg with special characters",
			argsNew:       "BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')",
			expectedCount: 1,
			expectedFirst: "BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')",
		},
		{
			name:          "args with equals and semicolons",
			argsNew:       "API_URL=https://api.example.com;DEBUG=true;VERSION=1.0.0",
			expectedCount: 3,
			expectedFirst: "API_URL=https://api.example.com",
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

func TestPlatformEnvVarMapping(t *testing.T) {
	tests := []struct {
		name          string
		envVar        string
		envValue      string
		expectedValue string
	}{
		{
			name:          "PLUGIN_PLATFORM env var",
			envVar:        "PLUGIN_PLATFORM",
			envValue:      "linux/amd64",
			expectedValue: "linux/amd64",
		},
		{
			name:          "PLUGIN_CUSTOM_PLATFORM env var",
			envVar:        "PLUGIN_CUSTOM_PLATFORM",
			envValue:      "linux/arm64",
			expectedValue: "linux/arm64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the environment variable
			os.Setenv(tt.envVar, tt.envValue)
			defer os.Unsetenv(tt.envVar)

			app := cli.NewApp()
			app.Name = "kaniko-docker-test"

			var capturedPlatform string

			app.Flags = []cli.Flag{
				cli.StringFlag{
					Name:   "platform",
					Usage:  "Allows to build with another default platform than the host",
					EnvVar: "PLUGIN_PLATFORM,PLUGIN_CUSTOM_PLATFORM",
				},
			}

			app.Action = func(c *cli.Context) error {
				capturedPlatform = c.String("platform")
				return nil
			}

			err := app.Run([]string{"kaniko-docker-test"})
			if err != nil {
				t.Errorf("CLI run error = %v, want nil", err)
				return
			}

			if capturedPlatform != tt.expectedValue {
				t.Errorf("Got platform = %v, want %v", capturedPlatform, tt.expectedValue)
			}
		})
	}
}

func TestCreateDockerConfig(t *testing.T) {
	config := docker.NewConfig()
	tempDir, err := ioutil.TempDir("", "docker-config-test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name        string
		credentials []docker.RegistryCredentials
		wantErr     bool
	}{
		{
			name: "valid credentials",
			credentials: []docker.RegistryCredentials{
				{
					Registry: "https://index.docker.io/v1/",
					Username: "testuser",
					Password: "testpassword",
				},
			},
			wantErr: false,
		},
		{
			name: "v2 registry",
			credentials: []docker.RegistryCredentials{
				{
					Registry: "https://index.docker.io/v2/",
					Username: "testuser",
					Password: "testpassword",
				},
			},
			wantErr: false,
		},
		{
			name: "docker registry credentials",
			credentials: []docker.RegistryCredentials{
				{
					Registry: "https://index.docker.io/v1/",
					Username: "testuser",
					Password: "testpassword",
				},
				{
					Registry: "https://docker.io",
					Username: "dockeruser",
					Password: "dockerpassword",
				},
			},
			wantErr: false,
		},
		{
			name: "empty docker registry",
			credentials: []docker.RegistryCredentials{
				{
					Registry: "https://index.docker.io/v1/",
					Username: "testuser",
					Password: "testpassword",
				},
				{
					Registry: "https://docker.io",
					Username: "dockeruser",
					Password: "",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.CreateDockerConfig(tt.credentials, tempDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateDockerConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
