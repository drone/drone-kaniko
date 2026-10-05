package artifactory

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drone/drone-kaniko/pkg/docker"
)

func TestValidateInputsRejectsConflicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Inputs)
	}{
		{name: "explicit Docker config", mutate: func(in *Inputs) { in.DockerConfigOverride = "{}" }},
		{name: "destination username", mutate: func(in *Inputs) { in.Username = "user" }},
		{name: "destination password", mutate: func(in *Inputs) { in.Password = "password" }},
		{name: "same-host base-image credentials", mutate: func(in *Inputs) {
			in.BaseImageRegistry = "https://JFROG.example.com:443/artifactory"
			in.BaseImageUsername = "base-user"
			in.BaseImagePassword = "base-password"
		}},
		{name: "remote cache enabled", mutate: func(in *Inputs) { in.EnableCache = true }},
		{name: "remote cache repository", mutate: func(in *Inputs) { in.CacheRepository = "jfrog.example.com/cache" }},
		{name: "no push", mutate: func(in *Inputs) { in.NoPush = true }},
		{name: "push only", mutate: func(in *Inputs) { in.PushOnly = true }},
		{name: "tar path", mutate: func(in *Inputs) { in.TarPath = "/tmp/image.tar" }},
		{name: "source tar path", mutate: func(in *Inputs) { in.SourceTarPath = "/tmp/source.tar" }},
		{name: "registry mirror", mutate: func(in *Inputs) { in.RegistryMirror = "mirror.example.com" }},
		{name: "registry mirrors", mutate: func(in *Inputs) { in.RegistryMirrors = []string{"mirror.example.com"} }},
		{name: "registry client certificate", mutate: func(in *Inputs) { in.RegistryClientCert = "/certs/client.pem" }},
		{name: "insecure", mutate: func(in *Inputs) { in.Insecure = true }},
		{name: "insecure pull", mutate: func(in *Inputs) { in.InsecurePull = true }},
		{name: "insecure registry", mutate: func(in *Inputs) { in.InsecureRegistry = "jfrog.example.com" }},
		{name: "skip TLS verify", mutate: func(in *Inputs) { in.SkipTLSVerify = true }},
		{name: "skip TLS verify pull", mutate: func(in *Inputs) { in.SkipTLSVerifyPull = true }},
		{name: "skip TLS verify registry", mutate: func(in *Inputs) { in.SkipTLSVerifyRegistry = true }},
		{name: "missing URL", mutate: func(in *Inputs) { in.ServerURL = "" }},
		{name: "missing ID token", mutate: func(in *Inputs) { in.IDToken = "" }},
		{name: "missing provider", mutate: func(in *Inputs) { in.ProviderName = "" }},
		{name: "missing registry", mutate: func(in *Inputs) { in.Registry = "" }},
		{name: "missing repository", mutate: func(in *Inputs) { in.Repository = "" }},
		{name: "missing Docker config root", mutate: func(in *Inputs) { in.DockerConfigRoot = "" }},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inputs := validInputs()
			tt.mutate(&inputs)
			if err := ValidateInputs(inputs); err == nil {
				t.Fatal("ValidateInputs() error = nil, want conflict error")
			}
		})
	}
}

func TestValidateInputsAllowsDifferentHostBaseImageCredentials(t *testing.T) {
	t.Parallel()

	inputs := validInputs()
	inputs.BaseImageRegistry = "https://index.docker.io/v1/"
	inputs.BaseImageUsername = "base-user"
	inputs.BaseImagePassword = "base-password"
	if err := ValidateInputs(inputs); err != nil {
		t.Fatalf("ValidateInputs() error = %v", err)
	}
}

func TestValidateInputsRejectsInsecureBaseImageRegistryURL(t *testing.T) {
	t.Parallel()

	inputs := validInputs()
	inputs.BaseImageRegistry = "http://base.example.com"
	inputs.BaseImageUsername = "base-user"
	inputs.BaseImagePassword = "base-password"
	if err := ValidateInputs(inputs); err == nil || !strings.Contains(err.Error(), "invalid base image registry") {
		t.Fatalf("ValidateInputs() error = %v, want invalid base image registry", err)
	}
}

func TestSetupCredentialsWritesAttemptLocalConfigAndCleansUp(t *testing.T) {
	original, hadOriginal := os.LookupEnv("DOCKER_CONFIG")
	t.Cleanup(func() {
		if hadOriginal {
			_ = os.Setenv("DOCKER_CONFIG", original)
		} else {
			_ = os.Unsetenv("DOCKER_CONFIG")
		}
	})
	if err := os.Setenv("DOCKER_CONFIG", "/previous/docker/config"); err != nil {
		t.Fatal(err)
	}

	inputs := validInputs()
	inputs.BaseImageRegistry = "https://index.docker.io/v1/"
	inputs.BaseImageUsername = "base-user"
	inputs.BaseImagePassword = "base-password"
	destination, err := ValidateDestination(inputs.ServerURL, inputs.Registry, inputs.Repository)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	cleanup, err := SetupCredentials(
		inputs,
		destination,
		Credential{Username: "oidc-user", AccessToken: "short-lived-token", ExpiresIn: 35 * 60},
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("SetupCredentials() error = %v", err)
	}

	configDir := os.Getenv("DOCKER_CONFIG")
	if configDir == "" || configDir == "/previous/docker/config" {
		t.Fatalf("DOCKER_CONFIG = %q, want attempt-local directory", configDir)
	}
	assertMode(t, configDir, 0700)
	configPath := filepath.Join(configDir, "config.json")
	assertMode(t, configPath, 0600)

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config docker.Config
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Auths) != 2 {
		t.Fatalf("auth count = %d, want 2", len(config.Auths))
	}
	assertAuth(t, config, "jfrog.example.com", "oidc-user:short-lived-token")
	assertAuth(t, config, "https://index.docker.io/v1/", "base-user:base-password")
	for key := range config.Auths {
		if key == inputs.BaseImageRegistry {
			continue
		}
		if strings.Contains(key, "artifactory") || strings.Contains(key, "/") {
			t.Fatalf("auth key %q contains an API or repository path", key)
		}
	}

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	if _, err := os.Stat(configDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config directory still exists after cleanup: %v", err)
	}
	if got := os.Getenv("DOCKER_CONFIG"); got != "/previous/docker/config" {
		t.Fatalf("DOCKER_CONFIG after cleanup = %q", got)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("second cleanup() error = %v", err)
	}
}

func TestSetupCredentialsCleanupAfterOperationFailure(t *testing.T) {
	_ = os.Unsetenv("DOCKER_CONFIG")
	inputs := validInputs()
	destination, err := ValidateDestination(inputs.ServerURL, inputs.Registry, inputs.Repository)
	if err != nil {
		t.Fatal(err)
	}
	cleanup, err := SetupCredentials(
		inputs,
		destination,
		Credential{Username: "oidc-user", AccessToken: "short-lived-token", ExpiresIn: 3600},
		time.Now,
	)
	if err != nil {
		t.Fatal(err)
	}
	configDir := os.Getenv("DOCKER_CONFIG")

	operationErr := errors.New("kaniko failed")
	if operationErr == nil {
		t.Fatal("expected simulated operation failure")
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	if _, err := os.Stat(configDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config directory still exists after failed operation cleanup: %v", err)
	}
}

func TestSetupCredentialsPreservesExplicitDefaultPortAuthKey(t *testing.T) {
	original, hadOriginal := os.LookupEnv("DOCKER_CONFIG")
	t.Cleanup(func() {
		if hadOriginal {
			_ = os.Setenv("DOCKER_CONFIG", original)
		} else {
			_ = os.Unsetenv("DOCKER_CONFIG")
		}
	})

	inputs := validInputs()
	inputs.Repository = "jfrog.example.com:443/team/image"
	destination, err := ValidateDestination(inputs.ServerURL, inputs.Registry, inputs.Repository)
	if err != nil {
		t.Fatal(err)
	}
	cleanup, err := SetupCredentials(
		inputs,
		destination,
		Credential{Username: "oidc-user", AccessToken: "short-lived-token", ExpiresIn: 35 * 60},
		time.Now,
	)
	if err != nil {
		t.Fatalf("SetupCredentials() error = %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	data, err := os.ReadFile(filepath.Join(os.Getenv("DOCKER_CONFIG"), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config docker.Config
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	assertAuth(t, config, "jfrog.example.com:443", "oidc-user:short-lived-token")
}

func TestSetupCredentialsEnforcesLifetimeBeforeWritingAuth(t *testing.T) {
	tests := []struct {
		name      string
		expiresIn int64
		wantErr   bool
	}{
		{name: "exactly 35 minutes accepted", expiresIn: 35 * 60},
		{name: "one second below rejected", expiresIn: 35*60 - 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Unsetenv("DOCKER_CONFIG")
			inputs := validInputs()
			destination, err := ValidateDestination(inputs.ServerURL, inputs.Registry, inputs.Repository)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
			cleanup, err := SetupCredentials(
				inputs,
				destination,
				Credential{Username: "user", AccessToken: "token", ExpiresIn: tt.expiresIn},
				func() time.Time { return now },
			)
			if tt.wantErr {
				if err == nil {
					_ = cleanup()
					t.Fatal("SetupCredentials() error = nil, want lifetime error")
				}
				if got := os.Getenv("DOCKER_CONFIG"); got != "" {
					t.Fatalf("DOCKER_CONFIG = %q; auth was written before lifetime rejection", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetupCredentials() error = %v", err)
			}
			if err := cleanup(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSetupCredentialsRejectsMismatchedDestinationBeforeWritingAuth(t *testing.T) {
	previous, hadPrevious := os.LookupEnv("DOCKER_CONFIG")
	t.Cleanup(func() {
		if hadPrevious {
			_ = os.Setenv("DOCKER_CONFIG", previous)
		} else {
			_ = os.Unsetenv("DOCKER_CONFIG")
		}
	})
	if err := os.Unsetenv("DOCKER_CONFIG"); err != nil {
		t.Fatal(err)
	}

	inputs := validInputs()
	cleanup, err := SetupCredentials(
		inputs,
		Destination{Registry: "attacker.example.com", Repository: "attacker.example.com/team/image"},
		Credential{Username: "oidc-user", AccessToken: "short-lived-token", ExpiresIn: 35 * 60},
		time.Now,
	)
	if err == nil {
		_ = cleanup()
		t.Fatal("SetupCredentials() error = nil, want destination mismatch error")
	}
	if got := os.Getenv("DOCKER_CONFIG"); got != "" {
		t.Fatalf("DOCKER_CONFIG = %q; auth was written for mismatched destination", got)
	}
}

func validInputs() Inputs {
	return Inputs{
		ServerURL:        "https://JFROG.example.com/artifactory",
		IDToken:          "harness-id-token",
		ProviderName:     "provider",
		Registry:         "jfrog.example.com:443",
		Repository:       "jfrog.example.com/team/image",
		DockerConfigRoot: os.TempDir(),
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
	}
}

func assertAuth(t *testing.T, config docker.Config, registry, want string) {
	t.Helper()
	entry, ok := config.Auths[registry]
	if !ok {
		t.Fatalf("missing auth key %q in %#v", registry, config.Auths)
	}
	decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(decoded); got != want {
		t.Fatalf("decoded auth for %q = %q, want %q", registry, got, want)
	}
}
