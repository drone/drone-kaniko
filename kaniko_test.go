package kaniko

import (
	"archive/tar"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBuild_labelsForTag(t *testing.T) {
	tests := []struct {
		name       string
		tag        string
		expandTags []string
	}{
		{
			name:       "semver",
			tag:        "v1.2.3",
			expandTags: []string{"1", "1.2", "1.2.3"},
		},
		{
			name:       "no_patch",
			tag:        "v1.2",
			expandTags: []string{"1", "1.2", "1.2.0"},
		},
		{
			name:       "only_major",
			tag:        "v1",
			expandTags: []string{"1", "1.0", "1.0.0"},
		},
		{
			name:       "full_with_build",
			tag:        "v1.2.3+build-info",
			expandTags: []string{"1+build-info", "1.2+build-info", "1.2.3+build-info"},
		},
		{
			name:       "build_with_underscores",
			tag:        "v1.2.3+linux_amd64",
			expandTags: []string{"1+linux-amd64", "1.2+linux-amd64", "1.2.3+linux-amd64"},
		},
		{
			name:       "prerelease",
			tag:        "v1.2.3-rc1",
			expandTags: []string{"1.2.3-rc1"},
		},
		{
			name:       "prerelease_with_build",
			tag:        "v1.2.3-rc1+bld",
			expandTags: []string{"1.2.3-rc1+bld"},
		},
		{
			name:       "invalid_build",
			tag:        "v1+bld", // can only include build detail with all three elements
			expandTags: []string{"v1+bld"},
		},
		{
			name:       "accidental_non_semver",
			tag:        "1.2.3",
			expandTags: []string{"1", "1.2", "1.2.3"},
		},
		{
			name:       "non_semver",
			tag:        "latest",
			expandTags: []string{"latest"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := Build{ExpandTag: true}.labelsForTag(tt.tag)
			if got, want := tags, tt.expandTags; !cmp.Equal(got, want) {
				t.Errorf("tagsFor(%q) = %q, want %q", tt.tag, got, want)
			}
		})
	}
}

func TestBuild_AutoTags(t *testing.T) {
	tests := []struct {
		name          string
		repoBranch    string
		commitRef     string
		autoTagSuffix string
		expectedTags  []string
	}{
		{
			name:          "commit push",
			repoBranch:    "master",
			commitRef:     "refs/heads/master",
			autoTagSuffix: "",
			expectedTags:  []string{"latest"},
		},
		{
			name:          "tag push",
			repoBranch:    "master",
			commitRef:     "refs/tags/v1.0.0",
			autoTagSuffix: "",
			expectedTags: []string{
				"1",
				"1.0",
				"1.0.0",
			},
		},
		{
			name:          "beta tag push",
			repoBranch:    "master",
			commitRef:     "refs/tags/v1.0.0-beta.1",
			autoTagSuffix: "",
			expectedTags: []string{
				"1.0.0-beta.1",
			},
		},
		{
			name:          "tag push with suffix",
			repoBranch:    "master",
			commitRef:     "refs/tags/v1.0.0",
			autoTagSuffix: "linux-amd64",
			expectedTags: []string{
				"1-linux-amd64",
				"1.0-linux-amd64",
				"1.0.0-linux-amd64",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := Build{DroneCommitRef: tt.commitRef, DroneRepoBranch: tt.repoBranch, AutoTag: true}
			if tt.autoTagSuffix != "" {
				b.AutoTagSuffix = tt.autoTagSuffix
			}
			tags, err := b.AutoTags()
			if err != nil {
				t.Errorf("Unexpected err %q", err)
			}
			if got, want := tags, tt.expectedTags; !cmp.Equal(got, want) {
				t.Errorf("auto detected tags = %q, wanted = %q", got, want)
			}
		})
	}
	t.Run("auto-tag cannot be enabled with user provided tags", func(t *testing.T) {
		b := Build{
			DroneCommitRef:  "refs/tags/v1.0.0",
			DroneRepoBranch: "master",
			AutoTag:         true,
			Tags:            []string{"v1"},
		}
		_, err := b.AutoTags()
		if err == nil {
			t.Errorf("Expect error for invalid flags")
		}
	})
}

func TestTarPathValidation(t *testing.T) {
	tests := []struct {
		name          string
		tarPath       string
		setup         func(string) error
		cleanup       func(string) error
		expectSuccess bool
		privileged    bool
	}{
		{
			name:    "valid_path_privileged",
			tarPath: "",
			setup: func(path string) error {
				tmpDir, err := os.MkdirTemp("", "test-image-tar")
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
			expectSuccess: true,
			privileged:    true,
		},
		{
			name:    "valid_path_unprivileged",
			tarPath: "",
			setup: func(path string) error {
				tmpDir, err := os.MkdirTemp("", "test-image-tar")
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
			expectSuccess: true,
			privileged:    false,
		},
		{
			name:          "empty_path",
			tarPath:       "",
			setup:         func(path string) error { return nil },
			cleanup:       func(path string) error { return nil },
			expectSuccess: false,
			privileged:    false,
		},
		{
			name:    "relative_path_dots",
			tarPath: "",
			setup: func(path string) error {
				tmpDir, err := os.MkdirTemp("", "test-image-tar")
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
			expectSuccess: true,
			privileged:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip privileged tests if not running as root
			if tt.privileged && os.Getuid() != 0 {
				t.Skip("Skipping privileged test as not running as root")
			}

			if err := tt.setup(tt.tarPath); err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer tt.cleanup(tt.tarPath)

			// Determine tar path based on test case
			var tarPath string
			tmpDir := os.Getenv("DRONE_WORKSPACE")
			switch tt.name {
			case "valid_path_privileged", "valid_path_unprivileged":
				tarPath = filepath.Join(tmpDir, "test", "image.tar")
			case "invalid_path_no_permissions":
				tarPath = "/test/image.tar"
			case "relative_path_dots":
				tarPath = filepath.Join("..", "test", "image.tar")
			default:
				tarPath = tt.tarPath
			}

			p := Plugin{
				Build: Build{
					TarPath: tarPath,
				},
			}

			tarDir := filepath.Dir(p.Build.TarPath)
			err := os.MkdirAll(tarDir, 0755)
			if tt.expectSuccess {
				if err != nil {
					t.Errorf("Expected directory creation to succeed, got error: %v", err)
				}
				if _, err := os.Stat(tarDir); err != nil {
					t.Errorf("Expected directory to exist after creation, got error: %v", err)
				}
			}

			result := getTarPath(p.Build.TarPath)
			if tt.expectSuccess && result == "" {
				t.Error("Expected non-empty tar path, got empty string")
			}
			if !tt.expectSuccess && result != "" {
				t.Error("Expected empty tar path, got non-empty string")
			}
		})
	}
}

func TestSourceTarballPush(t *testing.T) {
	tests := []struct {
		name          string
		sourceTarPath string
		repo          string
		autoTag       bool
		tags          []string
		commitRef     string
		repoBranch    string
		expectedError bool
		expectedTags  []string
		mockLoadErr   error
		mockPushErr   error
	}{
		{
			name:          "empty_repo_fails",
			sourceTarPath: "/path/to/image.tar",
			repo:          "",
			expectedError: true,
		},
		{
			name:          "nonexistent_tarball_fails",
			sourceTarPath: "/path/that/does/not/exist/image.tar",
			repo:          "test-repo",
			expectedError: true,
		},
		{
			name:          "load_image_fails",
			sourceTarPath: createTestTarball(t),
			repo:          "test-repo",
			expectedError: true,
			mockLoadErr:   fmt.Errorf("load failed"),
		},
		{
			name:          "push_image_fails",
			sourceTarPath: createTestTarball(t),
			repo:          "test-repo",
			expectedError: true,
			expectedTags:  []string{"latest"},
			mockPushErr:   fmt.Errorf("push failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPlugin := Plugin{
				Build: Build{
					SourceTarPath:   tt.sourceTarPath,
					Repo:            tt.repo,
					Tags:            tt.tags,
					AutoTag:         tt.autoTag,
					DroneCommitRef:  tt.commitRef,
					DroneRepoBranch: tt.repoBranch,
				},
				LoadImageFromTarball: MockCraneLoad(tt.sourceTarPath, tt.mockLoadErr),
				PushImageToRegistry:  MockCranePush(tt.mockPushErr),
			}

			err := mockPlugin.Exec()

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected an error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// Helper function to create a test tarball
func createTestTarball(t *testing.T) string {
	// Create a temporary directory for the tarball contents
	tmpDir, err := os.MkdirTemp("", "test-tarball-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a minimal `manifest.json` file with a valid hash
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	manifestContent := `[{
		"Config": "config.json",
		"RepoTags": ["test-repo:latest"],
		"Layers": ["layer.tar"]
	}]`
	err = os.WriteFile(manifestPath, []byte(manifestContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create manifest.json: %v", err)
	}

	// Create a valid `config.json` file with a dummy hash
	configPath := filepath.Join(tmpDir, "config.json")
	configContent := `{
		"architecture": "amd64",
		"os": "linux",
		"rootfs": {
			"type": "layers",
			"diff_ids": ["sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]
		}
	}`
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config.json: %v", err)
	}

	// Create a dummy `layer.tar` file
	layerPath := filepath.Join(tmpDir, "layer.tar")
	layerFile, err := os.Create(layerPath)
	if err != nil {
		t.Fatalf("Failed to create layer.tar: %v", err)
	}
	defer layerFile.Close()
	_, err = layerFile.Write([]byte("dummy layer content"))
	if err != nil {
		t.Fatalf("Failed to write to layer.tar: %v", err)
	}

	// Create a tarball from the temp directory
	tarballPath := filepath.Join(os.TempDir(), "test-image.tar")
	tarballFile, err := os.Create(tarballPath)
	if err != nil {
		t.Fatalf("Failed to create tarball: %v", err)
	}
	defer tarballFile.Close()

	tw := tar.NewWriter(tarballFile)
	defer tw.Close()

	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(tmpDir, path)
		if relPath == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, path)
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if !info.IsDir() {
			fileContent, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, err = tw.Write(fileContent)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to write tarball: %v", err)
	}

	return tarballPath
}
