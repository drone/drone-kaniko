package output

import (
	"github.com/joho/godotenv"
)

func WritePluginOutputFile(outputFilePath, digest string, pluginTarPath string) error {
	output := map[string]string{
		"digest":   digest,
		"tar_path": pluginTarPath,
	}
	return godotenv.Write(output, outputFilePath)
}
