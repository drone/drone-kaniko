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

func TestWriteDockerConfigUsesOwnerOnlyPermissions(t *testing.T) {
	tempRoot := t.TempDir()
	configDir := filepath.Join(tempRoot, "docker")

	if err := WriteDockerConfig([]byte(`{"auths":{}}`), configDir); err != nil {
		t.Fatalf("WriteDockerConfig() error = %v", err)
	}

	assertPermissions(t, configDir, 0700)
	assertPermissions(t, filepath.Join(configDir, "config.json"), 0600)
}

func TestCreateTemporaryDockerConfigLifecycle(t *testing.T) {
	previous, hadPrevious := os.LookupEnv("DOCKER_CONFIG")
	t.Cleanup(func() {
		if hadPrevious {
			_ = os.Setenv("DOCKER_CONFIG", previous)
		} else {
			_ = os.Unsetenv("DOCKER_CONFIG")
		}
	})
	if err := os.Setenv("DOCKER_CONFIG", "/previous/config"); err != nil {
		t.Fatal(err)
	}
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))

	cleanup, err := CreateTemporaryDockerConfig([]RegistryCredentials{{
		Registry: "registry.example.com",
		Username: "user",
		Password: "password",
	}}, tempRoot)
	if err != nil {
		t.Fatalf("CreateTemporaryDockerConfig() error = %v", err)
	}
	configDir := os.Getenv("DOCKER_CONFIG")
	if configDir == "" || configDir == "/previous/config" {
		t.Fatalf("DOCKER_CONFIG = %q, want temporary directory", configDir)
	}
	if filepath.Dir(configDir) != tempRoot {
		t.Fatalf("DOCKER_CONFIG parent = %q, want %q", filepath.Dir(configDir), tempRoot)
	}
	assertPermissions(t, configDir, 0700)
	assertPermissions(t, filepath.Join(configDir, "config.json"), 0600)

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatalf("temporary config still exists: %v", err)
	}
	if got := os.Getenv("DOCKER_CONFIG"); got != "/previous/config" {
		t.Fatalf("DOCKER_CONFIG after cleanup = %q", got)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("second cleanup() error = %v", err)
	}
}

func assertPermissions(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s permissions = %04o, want %04o", path, got, want)
	}
}
