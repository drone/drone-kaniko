package output

import (
	"fmt"
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

	if len(output) == 0 {
		return fmt.Errorf("no values to write to output file")
	}

	return godotenv.Write(output, outputFilePath)
}
