package digest

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const (
	defaultDigestFile string = "/kaniko/.docker/digest-file"
)

func GetDigestFileName(digestFile, outputFile string) (string, error) {
	if digestFile == "" && outputFile == "" {
		return "", nil
	}

	var fileName string
	if digestFile != "" {
		fileName = digestFile
	} else {
		fileName = defaultDigestFile
	}

	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		path := filepath.Dir(fileName)
		err = os.MkdirAll(path, 0644)
		if err != nil {
			return "", errors.Wrap(err, fmt.Sprintf("failed to create %s directory", fileName))
		}
	}

	return fileName, nil
}

func ReadDigestFile(digestFile string) (string, error) {
	content, err := ioutil.ReadFile(digestFile)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
