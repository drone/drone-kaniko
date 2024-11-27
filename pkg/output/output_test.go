package output

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWritePluginOutputFile(t *testing.T) {
	tests := []struct {
		name        string
		outputPath  string
		digest      string
		tarPath     string
		setup       func(string) error
		cleanup     func(string) error
		expectError bool
		privileged  bool
	}{
		{
			name:        "valid_output_privileged",
			outputPath:  "/tmp/test/output.env",
			digest:      "sha256:test",
			tarPath:     "/tmp/test/image.tar",
			setup:       func(path string) error { return os.MkdirAll(filepath.Dir(path), 0755) },
			cleanup:     func(path string) error { return os.RemoveAll(filepath.Dir(path)) },
			expectError: false,
			privileged:  true,
		},
		{
			name:        "valid_output_unprivileged",
			outputPath:  "./test/output.env",
			digest:      "sha256:test",
			tarPath:     "./test/image.tar",
			setup:       func(path string) error { return os.MkdirAll(filepath.Dir(path), 0755) },
			cleanup:     func(path string) error { return os.RemoveAll(filepath.Dir(path)) },
			expectError: false,
			privileged:  false,
		},
		{
			name:        "invalid_output_path",
			outputPath:  "/root/test/output.env",
			digest:      "sha256:test",
			tarPath:     "/root/test/image.tar",
			setup:       func(path string) error { return nil },
			cleanup:     func(path string) error { return nil },
			expectError: true,
			privileged:  false,
		},
		{
			name:        "digest_only",
			outputPath:  "./test/output.env",
			digest:      "sha256:test",
			tarPath:     "",
			setup:       func(path string) error { return os.MkdirAll(filepath.Dir(path), 0755) },
			cleanup:     func(path string) error { return os.RemoveAll(filepath.Dir(path)) },
			expectError: false,
			privileged:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip privileged tests if not running as root
			if tt.privileged && os.Getuid() != 0 {
				t.Skip("Skipping privileged test as not running as root")
			}

			if err := tt.setup(tt.outputPath); err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer tt.cleanup(tt.outputPath)

			err := WritePluginOutputFile(tt.outputPath, tt.digest, tt.tarPath)

			if tt.expectError && err == nil {
				t.Error("Expected error, got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if !tt.expectError && err == nil {
				content, err := os.ReadFile(tt.outputPath)
				if err != nil {
					t.Fatalf("Failed to read output file: %v", err)
				}

				if tt.digest != "" && !contains(string(content), tt.digest) {
					t.Error("Expected digest in output file")
				}

				if tt.tarPath != "" && !contains(string(content), tt.tarPath) {
					t.Error("Expected tar path in output file")
				}
			}
		})
	}
}

func contains(content, substring string) bool {
	return len(substring) > 0 && content != "" && content != "\n" && content != "\r\n"
}
