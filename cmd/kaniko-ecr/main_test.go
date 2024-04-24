package main

import (
	"encoding/base64"
	"io/ioutil"
	"os"
	"testing"

	"github.com/drone/drone-kaniko/pkg/docker"
	"github.com/stretchr/testify/assert"
)

func TestCreateDockerConfigForECRWithBaseRegistry(t *testing.T) {
	accessKey := "access-key"
	secretKey := "secret-key"
	ecrRegistry := "ecr-registry"
	dockerUsername := "dockeruser"
	dockerPassword := "dockerpass"
	dockerRegistry := "https://index.docker.io/v1/"

	tempDir, err := ioutil.TempDir("", "docker-config-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config := docker.NewConfig()

	pullFromRegistryCreds := docker.RegistryCredentials{
		Registry: dockerRegistry,
		Username: dockerUsername,
		Password: dockerPassword,
	}
	credentials := []docker.RegistryCredentials{
		{Registry: ecrRegistry, Username: accessKey, Password: secretKey},
		pullFromRegistryCreds,
	}

	err = config.CreateDockerConfig(credentials, tempDir)
	assert.NoError(t, err)

	expectedECRAuth := docker.Auth{Auth: base64.StdEncoding.EncodeToString([]byte(accessKey + ":" + secretKey))}
	assert.Equal(t, expectedECRAuth, config.Auths[ecrRegistry])

	expectedDockerAuth := docker.Auth{Auth: base64.StdEncoding.EncodeToString([]byte(dockerUsername + ":" + dockerPassword))}
	assert.Equal(t, expectedDockerAuth, config.Auths[dockerRegistry])
}