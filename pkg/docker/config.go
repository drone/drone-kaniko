package docker

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
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

func WriteDockerConfig(data []byte, path string) (string error){
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