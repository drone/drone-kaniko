package main

import (
	"os"
	"reflect"
	"testing"

	"github.com/drone/drone-kaniko/pkg/docker"
)

func TestCreateDockerConfig(t *testing.T) {
	got, err := createDockerConfig(
		"docker-username",
		"docker-password",
		"access-key",
		"secret-key",
		"ecr-registry",
		"",
		"",
		"",
		false,
	)
	if err != nil {
		t.Error("failed to create docker config")
	}

	want := docker.NewConfig()
	want.SetAuth(docker.RegistryV1, "docker-username", "docker-password")
	want.SetCredHelper(docker.RegistryECRPublic, "ecr-login")
	want.SetCredHelper("ecr-registry", "ecr-login")

	if !reflect.DeepEqual(want, got) {
		t.Errorf("not equal:\n  want: %#v\n   got: %#v", want, got)
	}
}

func TestCreateDockerConfigKanikoOneDotEight(t *testing.T) {
	os.Setenv(kanikoVersionEnv, "1.8.1")
	defer os.Setenv(kanikoVersionEnv, "")
	got, err := createDockerConfig(
		"docker-username",
		"docker-password",
		"access-key",
		"secret-key",
		"ecr-registry",
		false,
	)
	if err != nil {
		t.Error("failed to create docker config")
	}

	want := docker.NewConfig()
	want.SetAuth(docker.RegistryV1, "docker-username", "docker-password")

	if !reflect.DeepEqual(want, got) {
		t.Errorf("not equal:\n  want: %#v\n   got: %#v", want, got)
	}
}

func TestVersionComparison(t *testing.T) {
	tests := []struct {
		title    string
		version  string
		expected bool
	}{
		{
			title:    "Kaniko 1.6.0 version",
			version:  "1.6.0",
			expected: true,
		},
		{
			title:    "Kaniko 1.8.0 version",
			version:  "1.8.0",
			expected: false,
		},
		{
			title:    "Kaniko 1.8.1 version",
			version:  "1.8.1",
			expected: false,
		},
		{
			title:    "Empty kaniko version",
			version:  "",
			expected: true,
		},
		{
			title:    "Kaniko version 1.10.0",
			version:  "1.10.0",
			expected: false,
		},
	}
	for _, test := range tests {
		got := isKanikoVersionBelowOneDotEight(test.version)
		if got != test.expected {
			t.Fatalf("test name: %s, expected: %v, got: %v", test.title, test.expected, got)
		}
	}
}
