package output

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

type (
	Image struct {
		Image  string `json:"image"`
		Digest string `json:"digest"`
	}
	Data struct {
		Registry    string  `json:"registry"`
		RegistryUrl string  `json:"registryUrl"`
		Images      []Image `json:"images"`
	}
	Output struct {
		Kind string `json:"kind"`
		Data Data   `json:"data"`
	}
)

func WritePluginOutput(outputFilePath, kind, registry, repo, digest string, tags []string) error {
	images := make([]Image, len(tags))
	for i, tag := range tags {
		images[i] = Image{
			Image:  fmt.Sprintf("%s:%s", repo, tag),
			Digest: digest,
		}
	}
	data := Data{
		Registry:    kind,
		RegistryUrl: registry,
		Images:      images,
	}

	output := Output{
		Kind: "docker/v1",
		Data: data,
	}

	b, err := json.Marshal(output)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to marshal output %+v", output))
	}

	if _, err := os.Stat(outputFilePath); os.IsNotExist(err) {
		path := filepath.Dir(outputFilePath)
		err = os.MkdirAll(path, 0644)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to create %s directory", outputFilePath))
		}
	}

	err = ioutil.WriteFile(outputFilePath, b, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to create output file")
	}
	return nil
}
