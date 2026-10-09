package artifactory

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

const tokenEndpointPath = "/access/api/v1/oidc/token"

// Destination is a validated registry and untagged repository pair.
type Destination struct {
	Registry   string
	Repository string
}

// ResolveTokenEndpoint converts an approved JFrog connector URL into its OIDC
// token exchange endpoint.
func ResolveTokenEndpoint(serverURL string) (string, error) {
	parsed, err := parseServerURL(serverURL)
	if err != nil {
		return "", err
	}
	parsed.Path = tokenEndpointPath
	parsed.RawPath = ""
	return parsed.String(), nil
}

// ValidateDestination verifies that the configured registry and repository
// resolve to the same HTTPS origin as the JFrog connector.
func ValidateDestination(serverURL, registry, repo string) (Destination, error) {
	server, err := parseServerURL(serverURL)
	if err != nil {
		return Destination{}, err
	}
	serverOrigin, err := normalizedHTTPSOrigin(server)
	if err != nil {
		return Destination{}, fmt.Errorf("invalid JFrog server origin")
	}

	registryURL, err := parseRegistry(registry)
	if err != nil {
		return Destination{}, err
	}
	registryOrigin, err := normalizedHTTPSOrigin(registryURL)
	if err != nil {
		return Destination{}, fmt.Errorf("invalid registry origin")
	}
	if registryOrigin != serverOrigin {
		return Destination{}, fmt.Errorf("registry origin does not match JFrog server")
	}

	if strings.TrimSpace(repo) != repo || strings.Contains(repo, "://") ||
		strings.ContainsAny(repo, "@?#") {
		return Destination{}, fmt.Errorf("invalid repository")
	}
	repository, err := name.NewRepository(repo, name.StrictValidation)
	if err != nil {
		return Destination{}, fmt.Errorf("invalid repository")
	}
	if repository.RepositoryStr() == repository.RegistryStr() {
		return Destination{}, fmt.Errorf("repository must contain an untagged path")
	}

	repoRegistry, err := parseRegistry(repository.RegistryStr())
	if err != nil {
		return Destination{}, fmt.Errorf("invalid repository registry")
	}
	repoOrigin, err := normalizedHTTPSOrigin(repoRegistry)
	if err != nil || repoOrigin != serverOrigin {
		return Destination{}, fmt.Errorf("repository origin does not match JFrog server")
	}

	return Destination{Registry: repository.RegistryStr(), Repository: repo}, nil
}

func parseServerURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) != raw {
		return nil, fmt.Errorf("invalid JFrog server URL")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
		strings.HasSuffix(parsed.Host, ":") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.Opaque != "" {
		return nil, fmt.Errorf("invalid JFrog server URL")
	}
	if parsed.Path != "" && parsed.Path != "/" &&
		parsed.Path != "/artifactory" && parsed.Path != "/artifactory/" {
		return nil, fmt.Errorf("unsupported JFrog server URL path")
	}
	if parsed.RawPath != "" && parsed.EscapedPath() != parsed.Path {
		return nil, fmt.Errorf("invalid JFrog server URL path")
	}
	if _, err := normalizedHTTPSOrigin(parsed); err != nil {
		return nil, fmt.Errorf("invalid JFrog server URL")
	}
	return parsed, nil
}

func parseRegistry(registry string) (*url.URL, error) {
	if registry == "" || strings.TrimSpace(registry) != registry ||
		strings.ContainsAny(registry, "/@?#") || strings.Contains(registry, "://") {
		return nil, fmt.Errorf("invalid registry")
	}
	parsed, err := url.Parse("https://" + registry)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil ||
		strings.HasSuffix(parsed.Host, ":") ||
		parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("invalid registry")
	}
	return parsed, nil
}

func parseBaseImageRegistry(registry string) (*url.URL, error) {
	if registry == "" || strings.TrimSpace(registry) != registry {
		return nil, fmt.Errorf("invalid base image registry")
	}
	if !strings.Contains(registry, "://") {
		parsed, err := parseRegistry(registry)
		if err != nil {
			return nil, fmt.Errorf("invalid base image registry")
		}
		return parsed, nil
	}

	parsed, err := url.Parse(registry)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" ||
		strings.HasSuffix(parsed.Host, ":") || parsed.User != nil || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" || parsed.Opaque != "" {
		return nil, fmt.Errorf("invalid base image registry")
	}
	if _, err := normalizedHTTPSOrigin(parsed); err != nil {
		return nil, fmt.Errorf("invalid base image registry")
	}
	return parsed, nil
}

func normalizedHTTPSOrigin(parsed *url.URL) (string, error) {
	if parsed.Scheme != "https" || parsed.Hostname() == "" {
		return "", fmt.Errorf("HTTPS origin required")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return "", fmt.Errorf("invalid host")
	}
	port := parsed.Port()
	if port == "" {
		port = "443"
	} else {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number == 0 {
			return "", fmt.Errorf("invalid port")
		}
		port = strconv.FormatUint(number, 10)
	}
	return host + "\x00" + port, nil
}
