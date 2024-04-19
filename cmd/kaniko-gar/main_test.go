package main

import (
	"testing"

	"github.com/drone/drone-kaniko/pkg/docker"
)

func TestCreateDockerConfig(t *testing.T) {
	testCases := []struct {
		name           string
		dockerRegistry string
		dockerUsername string
		dockerPassword string
		expectedConfig *docker.Config
		expectedError  error
	}{
		{
			name:           "NoUsernameProvided",
			dockerRegistry: "https://index.docker.io/v1/",
			dockerUsername: "",
			dockerPassword: "",
			expectedConfig: docker.NewConfig(),
			expectedError:  nil,
		},
		{
			name:           "DockerHubWithCredentials",
			dockerRegistry: "",
			dockerUsername: "testuser",
			dockerPassword: "testpassword",
			expectedConfig: func() *docker.Config {
				config := docker.NewConfig()
				config.SetAuth("https://index.docker.io/v1/", "testuser", "testpassword")
				return config
			}(),
			expectedError: nil,
		},
		{
			name:           "PrivateRegistryWithCredentials",
			dockerRegistry: "example.azurecr.io",
			dockerUsername: "testuser",
			dockerPassword: "testpassword",
			expectedConfig: func() *docker.Config {
				config := docker.NewConfig()
				config.SetAuth("example.azurecr.io", "testuser", "testpassword")
				return config
			}(),
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config, err := createDockerConfig(tc.dockerRegistry, tc.dockerUsername, tc.dockerPassword)

			if tc.expectedError != nil {
				if err == nil {
					t.Errorf("Expected error '%v', but got nil", tc.expectedError)
				} else if err.Error() != tc.expectedError.Error() {
					t.Errorf("Expected error '%v', but got '%v'", tc.expectedError, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				if !configsEqual(config, tc.expectedConfig) {
					t.Errorf("Configs are not equal:\nExpected: %v\nGot: %v", tc.expectedConfig, config)
				}
			}
		})
	}
}

func configsEqual(c1, c2 *docker.Config) bool {
	if len(c1.Auths) != len(c2.Auths) {
		return false
	}

	for registry, auth1 := range c1.Auths {
		auth2, ok := c2.Auths[registry]
		if !ok || auth1 != auth2 {
			return false
		}
	}

	return true
}
