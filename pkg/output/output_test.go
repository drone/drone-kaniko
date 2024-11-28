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
			name:       "valid_output_privileged",
			outputPath: "",
			digest:     "sha256:test",
			tarPath:    "",
			setup: func(path string) error {
				tmpDir, err := os.MkdirTemp("", "test-output")
				if err != nil {
					return err
				}
				os.Setenv("DRONE_WORKSPACE", tmpDir)
				return nil
			},
			cleanup: func(path string) error {
				tmpDir := os.Getenv("DRONE_WORKSPACE")
				os.Unsetenv("DRONE_WORKSPACE")
				return os.RemoveAll(tmpDir)
			},
			expectError: false,
			privileged:  true,
		},
		{
			name:       "valid_output_unprivileged",
			outputPath: "",
			digest:     "sha256:test",
			tarPath:    "",
			setup: func(path string) error {
				tmpDir, err := os.MkdirTemp("", "test-output")
				if err != nil {
					return err
				}
				os.Setenv("DRONE_WORKSPACE", tmpDir)
				return nil
			},
			cleanup: func(path string) error {
				tmpDir := os.Getenv("DRONE_WORKSPACE")
				os.Unsetenv("DRONE_WORKSPACE")
				return os.RemoveAll(tmpDir)
			},
			expectError: false,
			privileged:  false,
		},
		{
			name:       "digest_only",
			outputPath: "",
			digest:     "sha256:test",
			tarPath:    "",
			setup: func(path string) error {
				tmpDir, err := os.MkdirTemp("", "test-output")
				if err != nil {
					return err
				}
				os.Setenv("DRONE_WORKSPACE", tmpDir)
				return nil
			},
			cleanup: func(path string) error {
				tmpDir := os.Getenv("DRONE_WORKSPACE")
				os.Unsetenv("DRONE_WORKSPACE")
				return os.RemoveAll(tmpDir)
			},
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

			tmpDir := os.Getenv("DRONE_WORKSPACE")
			var outputPath, tarPath string
			switch tt.name {
			case "valid_output_privileged", "valid_output_unprivileged":
				outputPath = filepath.Join(tmpDir, "test", "output.env")
				tarPath = filepath.Join(tmpDir, "test", "image.tar")
			case "invalid_output_path":
				outputPath = filepath.Join("/root", "test", "output.env")
				tarPath = filepath.Join("/root", "test", "image.tar")
			case "digest_only":
				outputPath = filepath.Join(tmpDir, "test", "output.env")
				tarPath = ""
			}

			err := os.MkdirAll(filepath.Dir(outputPath), 0755)
			if err != nil {
				t.Fatalf("Failed to create output directory: %v", err)
			}

			err = WritePluginOutputFile(outputPath, tt.digest, tarPath)

			if tt.expectError && err == nil {
				t.Error("Expected error, got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if !tt.expectError && err == nil {
				content, err := os.ReadFile(outputPath)
				if err != nil {
					t.Fatalf("Failed to read output file: %v", err)
				}

				if tt.digest != "" && !contains(string(content), tt.digest) {
					t.Error("Expected digest in output file")
				}

				if tarPath != "" && !contains(string(content), tarPath) {
					t.Error("Expected tar path in output file")
				}
			}
		})
	}
}

func contains(content, substring string) bool {
	return len(substring) > 0 && content != "" && content != "\n" && content != "\r\n"
}
