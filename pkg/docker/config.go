package docker

import (
	"encoding/base64"
	"fmt"
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
	authBytes := []byte(fmt.Sprintf("%s:%s", username, password))
	encodedString := base64.StdEncoding.EncodeToString(authBytes)
	c.Auths[registry] = Auth{Auth: encodedString}
}

func (c *Config) SetCredHelper(registry, helper string) {
	c.CredHelpers[registry] = helper
}