package main

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/drone/drone-kaniko/pkg/docker"
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
