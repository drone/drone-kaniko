package main

import (
	"encoding/base64"
	"testing"

	"github.com/drone/drone-kaniko/pkg/docker"
	"github.com/stretchr/testify/assert"
)

func TestCreateDockerConfigForACR(t *testing.T) {
	username := "user1"
	password := "pass1"
	registry := "azurecr.io"
	dockerUsername := "dockeruser"
	dockerPassword := "dockerpass"
	dockerRegistry := "https://index.docker.io/v1/"

	// Test with valid inputs
	config, err := createDockerConfig(username, password, registry, dockerUsername, dockerPassword, dockerRegistry)
	assert.NoError(t, err)
	assert.NotNil(t, config)

	expectedAuth := docker.Auth{Auth: base64.StdEncoding.EncodeToString([]byte(username + ":" + password))}
	assert.Equal(t, expectedAuth, config.Auths[registry])

	expectedDockerAuth := docker.Auth{Auth: base64.StdEncoding.EncodeToString([]byte(dockerUsername + ":" + dockerPassword))}
	assert.Equal(t, expectedDockerAuth, config.Auths[dockerRegistry])

	// Test with empty username
	_, err = createDockerConfig("", password, registry, dockerUsername, dockerPassword, dockerRegistry)
	assert.EqualError(t, err, "Username must be specified")

	// Test with empty password
	_, err = createDockerConfig(username, "", registry, dockerUsername, dockerPassword, dockerRegistry)
	assert.EqualError(t, err, "Password must be specified")

	// Test with empty docker username and password
	config, err = createDockerConfig(username, password, registry, "", "", "")
	assert.NoError(t, err)
	assert.NotNil(t, config)
	_, exists := config.Auths[""]
	assert.False(t, exists)
}

func TestSetAuth(t *testing.T) {
	config := docker.NewConfig()

	// Set auth for a registry
	registry := "azurecr.io"
	username := "user1"
	password := "pass1"
	config.SetAuth(registry, username, password)

	expectedAuth := docker.Auth{Auth: base64.StdEncoding.EncodeToString([]byte(username + ":" + password))}
	assert.Equal(t, expectedAuth, config.Auths[registry])

	registry2 := "gcr.io"
	username2 := "user2"
	password2 := "pass2"
	config.SetAuth(registry2, username2, password2)

	assert.Equal(t, expectedAuth, config.Auths[registry])
	expectedAuth2 := docker.Auth{Auth: base64.StdEncoding.EncodeToString([]byte(username2 + ":" + password2))}
	assert.Equal(t, expectedAuth2, config.Auths[registry2])
}
