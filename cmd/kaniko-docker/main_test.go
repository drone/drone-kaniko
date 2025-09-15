package main

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/drone/drone-kaniko/pkg/docker"
	"github.com/drone/drone-kaniko/pkg/utils"
	"github.com/urfave/cli"
)

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
