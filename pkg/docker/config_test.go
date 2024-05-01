package docker

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	c := NewConfig()
	assert.NotNil(t, c.Auths)
	assert.NotNil(t, c.CredHelpers)

	c.SetAuth(RegistryV1, "test", "password")
	expectedAuth := Auth{Auth: "dGVzdDpwYXNzd29yZA=="}
	assert.Equal(t, expectedAuth, c.Auths[RegistryV1])

	c.SetCredHelper(RegistryECRPublic, "ecr-login")
	assert.Equal(t, "ecr-login", c.CredHelpers[RegistryECRPublic])

	tempDir, err := ioutil.TempDir("", "docker-config-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	credentials := []RegistryCredentials{
		{
			Registry: "https://index.docker.io/v1/",
			Username: "user1",
			Password: "pass1",
		},
		{
			Registry: "gcr.io",
			Username: "user2",
			Password: "pass2",
		},
	}

	err = c.CreateDockerConfig(credentials, tempDir)
	assert.NoError(t, err)

	configPath := filepath.Join(tempDir, "config.json")
	data, err := ioutil.ReadFile(configPath)
	assert.NoError(t, err)

	var configFromFile Config
	err = json.Unmarshal(data, &configFromFile)
	assert.NoError(t, err)

	assert.Equal(t, c.Auths, configFromFile.Auths)
	assert.Equal(t, c.CredHelpers, configFromFile.CredHelpers)
}
