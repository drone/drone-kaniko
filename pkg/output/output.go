package output

import (
	"github.com/joho/godotenv"
)

func WritePluginOutputFile(outputFilePath, digest string, pluginTarPath string) error {
	output := make(map[string]string)
	if digest != "" {
		output["digest"] = digest
	}

	if pluginTarPath != "" {
		output["IMAGE_TAR_PATH"] = pluginTarPath
	}

	return godotenv.Write(output, outputFilePath)
}
