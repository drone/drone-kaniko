package output

import (
	"fmt"
	"github.com/joho/godotenv"
)

func WritePluginOutputFile(outputFilePath, digest string, pluginTarPath string) error {
	fmt.Printf("Debug: Writing output file with digest: %s and tar_path: %s\n", digest, pluginTarPath)
	output := make(map[string]string)
	if digest != "" {
		output["digest"] = digest
	}
	if pluginTarPath != "" {
		output["tar_path"] = pluginTarPath
		fmt.Printf("Debug: Added tar_path to output map: %s\n", pluginTarPath)
	} else {
		fmt.Printf("Warning: tar_path is empty, skipping\n")
	}

	// Verify we have at least one value to write
	if len(output) == 0 {
		return fmt.Errorf("no values to write to output file")
	}

	// Write the file
	if err := godotenv.Write(output, outputFilePath); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	// Verify the file was written correctly
	written, err := godotenv.Read(outputFilePath)
	if err != nil {
		return fmt.Errorf("failed to verify written output: %w", err)
	}

	fmt.Printf("Debug: Verified written output - tar_path: %s\n", written["tar_path"])
	return nil
}
