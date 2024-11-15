package output

import (
	"github.com/joho/godotenv"
)

func WritePluginOutputFile(outputFilePath, digest string, pluginTarPath string) error {
	output := map[string]string{
		"digest":          digest,
		"PLUGIN_TAR_PATH": pluginTarPath,
	}
	return godotenv.Write(output, outputFilePath)
}
