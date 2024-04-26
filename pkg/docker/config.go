package docker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
)

const (
	v2HubRegistryURL string = "https://registry.hub.docker.com/v2/"
	v1RegistryURL    string = "https://index.docker.io/v1/" // Default registry
	v2RegistryURL    string = "https://index.docker.io/v2/" // v2 registry is not supported
)

type (
	Auth struct {
		Auth string `json:"auth"`
	}

	Config struct {
		Auths       map[string]Auth   `json:"auths"`
		CredHelpers map[string]string `json:"credHelpers,omitempty"`
	}
)

type RegistryCredentials struct {
	Registry string
	Username string
	Password string
}

func NewConfig() *Config {
	return &Config{
		Auths:       make(map[string]Auth),
		CredHelpers: make(map[string]string),
	}
}

func (c *Config) SetAuth(registry, username, password string) {
	authBytes := []byte(username + ":" + password)
	encodedString := base64.StdEncoding.EncodeToString(authBytes)
	c.Auths[registry] = Auth{Auth: encodedString}
}

func (c *Config) SetCredHelper(registry, helper string) {
	c.CredHelpers[registry] = helper
}

func (c *Config) CreateDockerConfig(credentials []RegistryCredentials, dockerPath string) error {
	for _, cred := range credentials {
		if cred.Registry != "" {
			// update v2 docker registry to v1
			if cred.Registry == v2RegistryURL || cred.Registry == v2HubRegistryURL {
				fmt.Printf("Docker v2 registry '%s' is not supported in kaniko. Refer issue: https://github.com/GoogleContainerTools/kaniko/issues/1209\n", cred.Registry)
				fmt.Printf("Using v1 registry instead: %s\n", v1RegistryURL)
				cred.Registry = v1RegistryURL
			}

			if cred.Username == "" {
				return fmt.Errorf("Username must be specified for registry: %s", cred.Registry)
			}
			if cred.Password == "" {
				return fmt.Errorf("Password must be specified for registry: %s", cred.Registry)
			}
			c.SetAuth(cred.Registry, cred.Username, cred.Password)
		}
	}
	jsonBytes, err := json.Marshal(c)
	if err != nil {
		return errors.Wrap(err, "failed to serialize docker config json")
	}
	if err := WriteDockerConfig(jsonBytes, dockerPath); err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to write docker config to path: %s", dockerPath))
	}
	return nil
}

func WriteDockerConfig(data []byte, path string) (string error) {
	err := os.MkdirAll(path, 0600)
	if err != nil {
		if !os.IsExist(err) {
			return errors.Wrap(err, fmt.Sprintf("failed to create %s directory", path))
		}
	}

	filePath := path + "/config.json"

	err = ioutil.WriteFile(filePath, data, 0644)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to create docker config file at %s", path))
	}
	return nil
}
