package artifact

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const (
	dockerArtifactV1 string = "docker/v1"
)

type RegistryTypeEnum string

const (
	Docker RegistryTypeEnum = "Docker"
	ECR    RegistryTypeEnum = "ECR"
	GCR    RegistryTypeEnum = "GCR"
)

type (
	Image struct {
		Image  string `json:"image"`
		Digest string `json:"digest"`
	}
	Data struct {
		RegistryType RegistryTypeEnum `json:"registryType"`
		RegistryUrl  string           `json:"registryUrl"`
		Images       []Image          `json:"images"`
	}
	DockerArtifact struct {
		Kind string `json:"kind"`
		Data Data   `json:"data"`
	}
)

func WritePluginArtifactFile(registryType RegistryTypeEnum, artifactFilePath, registryUrl, imageName, digest string, tags []string) error {
	var images []Image
	for _, tag := range tags {
		images = append(images, Image{
			Image:  fmt.Sprintf("%s:%s", imageName, tag),
			Digest: digest,
		})
	}
	data := Data{
		RegistryType: registryType,
		RegistryUrl:  registryUrl,
		Images:       images,
	}

	dockerArtifact := DockerArtifact{
		Kind: dockerArtifactV1,
		Data: data,
	}

	b, err := json.MarshalIndent(dockerArtifact, "", "\t")
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to marshal output %+v", dockerArtifact))
	}

	dir := filepath.Dir(artifactFilePath)
	err = os.MkdirAll(dir, 0644)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to create %s directory for artifact file", dir))
	}

	err = ioutil.WriteFile(artifactFilePath, b, 0644)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to write artifact to artifact file %s", artifactFilePath))
	}
	return nil
}
