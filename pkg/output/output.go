package output

import (
	"github.com/joho/godotenv"
)

func WritePluginOutputFile(outputFilePath, digest string) error {
	output := map[string]string{
		"digest": digest,
	}
	return godotenv.Write(output, outputFilePath)
}
