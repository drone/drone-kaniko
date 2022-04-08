package main

import (
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
