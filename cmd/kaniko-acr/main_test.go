package main

import (
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/drone/drone-kaniko/pkg/docker"
	"github.com/stretchr/testify/assert"
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