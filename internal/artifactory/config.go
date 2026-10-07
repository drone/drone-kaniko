package artifactory

import (
	"fmt"
	"strings"
	"time"

	"github.com/drone/drone-kaniko/pkg/docker"
)

const (
	maxExecutionDuration   = 30 * time.Minute
	credentialSafetyMargin = 5 * time.Minute
)

// Inputs contains the OIDC-specific settings and conflicting standard Kaniko
// options that must be validated before configuring short-lived credentials.
type Inputs struct {
	ServerURL    string
	IDToken      string
	ProviderName string
	Registry     string
	Repository   string

	DockerConfigRoot     string
	DockerConfigOverride string
	Username             string
	Password             string

	BaseImageRegistry string
	BaseImageUsername string
	BaseImagePassword string

	EnableCache     bool
	CacheRepository string
	CacheTTL        int
	CacheDir        string
	CacheCopyLayers bool
	CacheRunLayers  bool
	CompressedCache bool

	RegistryMirrors       []string
	RegistryClientCert    string
	Insecure              bool
	InsecurePull          bool
	InsecureRegistry      string
	SkipTLSVerify         bool
	SkipTLSVerifyPull     bool
	SkipTLSVerifyRegistry bool
}

// ValidateInputs rejects options that conflict with OIDC-managed registry
// credentials and validates the required OIDC and destination values.
func ValidateInputs(inputs Inputs) error {
	if strings.TrimSpace(inputs.ServerURL) == "" {
		return fmt.Errorf("JFrog server URL is required")
	}
	if strings.TrimSpace(inputs.IDToken) == "" {
		return fmt.Errorf("OIDC ID token is required")
	}
	if strings.TrimSpace(inputs.ProviderName) == "" {
		return fmt.Errorf("OIDC provider name is required")
	}
	if strings.TrimSpace(inputs.Registry) == "" {
		return fmt.Errorf("registry is required")
	}
	if strings.TrimSpace(inputs.Repository) == "" {
		return fmt.Errorf("repository is required")
	}
	if strings.TrimSpace(inputs.DockerConfigRoot) == "" {
		return fmt.Errorf("Docker config root is required")
	}
	if inputs.DockerConfigOverride != "" || inputs.Username != "" || inputs.Password != "" ||
		inputs.RegistryClientCert != "" || inputs.Insecure || inputs.InsecurePull || inputs.InsecureRegistry != "" ||
		inputs.SkipTLSVerify || inputs.SkipTLSVerifyPull || inputs.SkipTLSVerifyRegistry {
		return fmt.Errorf("option is incompatible with JFrog OIDC credentials")
	}

	destination, err := ValidateDestination(inputs.ServerURL, inputs.Registry, inputs.Repository)
	if err != nil {
		return err
	}
	if inputs.CacheRepository != "" {
		if _, err := ValidateDestination(inputs.ServerURL, inputs.Registry, inputs.CacheRepository); err != nil {
			return fmt.Errorf("invalid cache repository: %w", err)
		}
	}
	if inputs.BaseImageRegistry == "" {
		return nil
	}
	baseRegistry, err := parseBaseImageRegistry(inputs.BaseImageRegistry)
	if err != nil {
		return fmt.Errorf("invalid base image registry")
	}
	baseOrigin, err := normalizedHTTPSOrigin(baseRegistry)
	if err != nil {
		return fmt.Errorf("invalid base image registry")
	}
	destinationRegistry, err := parseRegistry(destination.Registry)
	if err != nil {
		return fmt.Errorf("invalid registry")
	}
	destinationOrigin, err := normalizedHTTPSOrigin(destinationRegistry)
	if err != nil {
		return fmt.Errorf("invalid registry")
	}
	if baseOrigin == destinationOrigin &&
		(inputs.BaseImageUsername != "" || inputs.BaseImagePassword != "") {
		return fmt.Errorf("base image credentials conflict with JFrog OIDC credentials")
	}
	return nil
}

// SetupCredentials writes a private, attempt-local Docker config and returns
// an idempotent cleanup callback. It never exposes the exchanged token.
func SetupCredentials(inputs Inputs, destination Destination, credential Credential, now func() time.Time) (func() error, error) {
	if err := ValidateInputs(inputs); err != nil {
		return nil, err
	}
	inputRegistry, err := normalizedRegistryKey(inputs.Registry)
	if err != nil {
		return nil, fmt.Errorf("invalid registry")
	}
	destinationRegistry, err := normalizedRegistryKey(destination.Registry)
	if err != nil {
		return nil, fmt.Errorf("invalid destination registry")
	}
	if destinationRegistry != inputRegistry {
		return nil, fmt.Errorf("destination registry does not match validated inputs")
	}
	if now == nil {
		return nil, fmt.Errorf("clock is required")
	}
	if strings.TrimSpace(credential.Username) == "" || strings.TrimSpace(credential.AccessToken) == "" {
		return nil, fmt.Errorf("JFrog OIDC credential is incomplete")
	}
	if credential.ExpiresIn < int64((maxExecutionDuration+credentialSafetyMargin)/time.Second) {
		return nil, fmt.Errorf("JFrog OIDC credential lifetime is too short")
	}
	startedAt := now()
	if startedAt.Add(time.Duration(credential.ExpiresIn) * time.Second).Before(
		startedAt.Add(maxExecutionDuration + credentialSafetyMargin)) {
		return nil, fmt.Errorf("JFrog OIDC credential lifetime is too short")
	}
	credentials := []docker.RegistryCredentials{{
		Registry: destination.Registry,
		Username: credential.Username,
		Password: credential.AccessToken,
	}}
	if inputs.CacheRepository != "" {
		cacheDestination, err := ValidateDestination(inputs.ServerURL, inputs.Registry, inputs.CacheRepository)
		if err != nil {
			return nil, fmt.Errorf("invalid cache repository: %w", err)
		}
		if cacheDestination.Registry != destination.Registry {
			credentials = append(credentials, docker.RegistryCredentials{
				Registry: cacheDestination.Registry,
				Username: credential.Username,
				Password: credential.AccessToken,
			})
		}
	}
	if inputs.BaseImageRegistry != "" {
		credentials = append(credentials, docker.RegistryCredentials{
			Registry: inputs.BaseImageRegistry,
			Username: inputs.BaseImageUsername,
			Password: inputs.BaseImagePassword,
		})
	}
	return docker.CreateTemporaryDockerConfig(credentials, inputs.DockerConfigRoot)
}

func normalizedRegistryKey(registry string) (string, error) {
	parsed, err := parseRegistry(registry)
	if err != nil {
		return "", err
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if port := parsed.Port(); port != "" && port != "443" {
		return host + ":" + port, nil
	}
	return host, nil
}
