package main

import "testing"

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

func TestCreateDockerConfigFromGivenRegistry(t *testing.T) {
	tests := []struct {
		name           string
		username       string
		password       string
		registry       string
		dockerUsername string
		dockerPassword string
		dockerRegistry string
		wantErr        bool
	}{
		{
			name:     "valid credentials",
			username: "testuser",
			password: "testpassword",
			registry: "https://index.docker.io/v1/",
			wantErr:  false,
		},
		{
			name:     "v2 registry",
			username: "testuser",
			password: "testpassword",
			registry: "https://index.docker.io/v2/",
			wantErr:  false,
		},
		{
			name:           "docker registry credentials",
			username:       "testuser",
			password:       "testpassword",
			registry:       "https://index.docker.io/v1/",
			dockerUsername: "dockeruser",
			dockerPassword: "dockerpassword",
			dockerRegistry: "https://docker.io",
			wantErr:        false,
		},
		{
			name:           "empty docker registry",
			username:       "testuser",
			password:       "testpassword",
			registry:       "https://index.docker.io/v1/",
			dockerUsername: "dockeruser",
			dockerPassword: "dockerpassword",
			dockerRegistry: "",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := createDockerCfgFile(tt.username, tt.password, tt.registry, tt.dockerUsername, tt.dockerPassword, tt.dockerRegistry)
			if (err != nil) != tt.wantErr {
				t.Errorf("createDockerCfgFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
