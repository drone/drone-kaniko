package output

import (
	"github.com/joho/godotenv"
)

func WritePluginOutputFile(outputFilePath, imageName, digest string, tags []string) error {
	output := map[string]string{
		"digest": digest,
	}
	return godotenv.Write(output, outputFilePath)
}
