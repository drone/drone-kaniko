package docker

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
)

// Create the docker config file for authentication
func CreateDockerCfgFile(username, password, registry, path string) error {
	if username == "" {
		return fmt.Errorf("Username must be specified")
	}
	if password == "" {
		return fmt.Errorf("Password must be specified")
	}

	err := os.MkdirAll(path, 0600)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to create %s directory", path))
	}

	authBytes := []byte(fmt.Sprintf("%s:%s", username, password))
	encodedString := base64.StdEncoding.EncodeToString(authBytes)
	jsonBytes := []byte(fmt.Sprintf(`{"auths": {"%s": {"auth": "%s"}}}`, "https://"+registry, encodedString))
	err = ioutil.WriteFile(path, jsonBytes, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to create docker config file")
	}
	return nil
}
