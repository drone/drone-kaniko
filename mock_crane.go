package kaniko

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

func MockCraneLoad(path string, loadErr error) func(string) (v1.Image, error) {
	return func(inputPath string) (v1.Image, error) {
		if loadErr != nil {
			return nil, loadErr
		}
		return &mockImage{}, nil
	}
}

func MockCranePush(pushErr error) func(v1.Image, string) error {
	return func(img v1.Image, dest string) error {
		if pushErr != nil {
			return pushErr
		}
		return nil
	}
}

// mockImage is a mock implementation of v1.Image interface
type mockImage struct{}

func (m *mockImage) Size() (int64, error) {
	return 0, nil
}

func (m *mockImage) RawConfigFile() ([]byte, error) {
	return nil, nil
}

func (m *mockImage) Digest() (v1.Hash, error) {
	return v1.Hash{}, nil
}

func (m *mockImage) Manifest() (*v1.Manifest, error) {
	return nil, nil
}

func (m *mockImage) RawManifest() ([]byte, error) {
	return nil, nil
}

func (m *mockImage) LayerByDigest(hash v1.Hash) (v1.Layer, error) {
	return nil, nil
}

func (m *mockImage) LayerByDiffID(hash v1.Hash) (v1.Layer, error) {
	return nil, nil
}

func (m *mockImage) Layers() ([]v1.Layer, error) {
	return nil, nil
}

func (m *mockImage) MediaType() (types.MediaType, error) {
	return "", nil
}

func (m *mockImage) ConfigFile() (*v1.ConfigFile, error) {
	return nil, nil
}

func (m *mockImage) ConfigName() (v1.Hash, error) {
	return v1.Hash{}, nil
}
