package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	kaniko "github.com/drone/drone-kaniko"
	azureutil "github.com/drone/drone-kaniko/internal/azure"
	"github.com/drone/drone-kaniko/pkg/artifact"
	"github.com/drone/drone-kaniko/pkg/docker"
	"github.com/drone/drone-kaniko/pkg/utils"
)

const (
	clientIdEnv        string = "AZURE_CLIENT_ID"
	clientSecretKeyEnv string = "AZURE_CLIENT_SECRET"
	dockerConfigPath   string = "/kaniko/.docker"
	tenantKeyEnv       string = "AZURE_TENANT_ID"
	certPathEnv        string = "AZURE_CLIENT_CERTIFICATE_PATH"
	defaultDigestFile  string = "/kaniko/digest-file"
	finalUrl           string = "https://portal.azure.com/#view/Microsoft_Azure_ContainerRegistries/TagMetadataBlade/registryId/"
)

var (
	ACRCertPath   = "/kaniko/acr-cert.pem"
	pluginVersion = "unknown"
	username      = "00000000-0000-0000-0000-000000000000"
	maxPageCount  = 1000 // maximum count of pages to cycle through before we break out
)

func main() {
	// TODO Add the env file functionality
	app := cli.NewApp()
	app.Name = "kaniko docker plugin"
	app.Usage = "kaniko docker plugin"
	app.Action = run
	app.Version = pluginVersion
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "dockerfile",
			Usage:  "build dockerfile",
			Value:  "Dockerfile",
			EnvVar: "PLUGIN_DOCKERFILE",
		},
		cli.StringFlag{
			Name:   "context",
			Usage:  "build context",
			Value:  ".",
			EnvVar: "PLUGIN_CONTEXT",
		},
		cli.StringFlag{
			Name:   "drone-commit-ref",
			Usage:  "git commit ref passed by Drone",
			EnvVar: "DRONE_COMMIT_REF",
		},
		cli.StringFlag{
			Name:   "drone-repo-branch",
			Usage:  "git repository default branch passed by Drone",
			EnvVar: "DRONE_REPO_BRANCH",
		},
		cli.StringSliceFlag{
			Name:     "tags",
			Usage:    "build tags",
			Value:    &cli.StringSlice{"latest"},
			EnvVar:   "PLUGIN_TAGS",
			FilePath: ".tags",
		},
		cli.BoolFlag{
			Name:   "expand-tag",
			Usage:  "enable for semver tagging",
			EnvVar: "PLUGIN_EXPAND_TAG",
		},
		cli.BoolFlag{
			Name:   "auto-tag",
			Usage:  "enable auto generation of build tags",
			EnvVar: "PLUGIN_AUTO_TAG",
		},
		cli.StringFlag{
			Name:   "auto-tag-suffix",
			Usage:  "the suffix of auto build tags",
			EnvVar: "PLUGIN_AUTO_TAG_SUFFIX",
		},
		cli.StringSliceFlag{
			Name:   "args",
			Usage:  "build args",
			EnvVar: "PLUGIN_BUILD_ARGS",
		},
		cli.GenericFlag{
			Name:   "args-new",
			Usage:  "build args new",
			EnvVar: "PLUGIN_BUILD_ARGS_NEW",
			Value:  new(utils.CustomStringSliceFlag),
		},
		cli.BoolFlag{
			Name:   "plugin-multiple-build-agrs",
			Usage:  "plugin multiple build agrs",
			EnvVar: "PLUGIN_MULTIPLE_BUILD_ARGS",
		},
		cli.StringFlag{
			Name:   "target",
			Usage:  "build target",
			EnvVar: "PLUGIN_TARGET",
		},
		cli.StringFlag{
			Name:   "repo",
			Usage:  "docker repository",
			EnvVar: "PLUGIN_REPO",
		},
		cli.BoolFlag{
			Name:   "create-repository",
			Usage:  "create ACR repository",
			EnvVar: "PLUGIN_CREATE_REPOSITORY",
		},
		cli.StringSliceFlag{
			Name:   "custom-labels",
			Usage:  "additional k=v labels",
			EnvVar: "PLUGIN_CUSTOM_LABELS",
		},
		cli.StringFlag{
			Name:   "registry",
			Usage:  "ACR registry",
			EnvVar: "PLUGIN_REGISTRY",
		},
		cli.StringFlag{
			Name:   "base-image-registry",
			Usage:  "Docker registry for base image",
			EnvVar: "PLUGIN_DOCKER_REGISTRY,PLUGIN_BASE_IMAGE_REGISTRY,DOCKER_REGISTRY",
		},
		cli.StringFlag{
			Name:   "base-image-username",
			Usage:  "Docker username for base image registry",
			EnvVar: "PLUGIN_DOCKER_USERNAME,PLUGIN_BASE_IMAGE_USERNAME,DOCKER_USERNAME",
		},
		cli.StringFlag{
			Name:   "base-image-password",
			Usage:  "Docker password for base image registry",
			EnvVar: "PLUGIN_DOCKER_PASSWORD,PLUGIN_BASE_IMAGE_PASSWORD,DOCKER_PASSWORD",
		},
		cli.StringSliceFlag{
			Name:   "registry-mirrors",
			Usage:  "docker registry mirrors",
			EnvVar: "PLUGIN_REGISTRY_MIRRORS",
		},
		cli.StringFlag{
			Name:   "client-secret",
			Usage:  "Azure client secret",
			EnvVar: "CLIENT_SECRET",
		},
		cli.StringFlag{
			Name:   "client-cert",
			Usage:  "Azure client certificate encoded in base64 format",
			EnvVar: "CLIENT_CERTIFICATE",
		},
		cli.StringFlag{
			Name:   "tenant-id",
			Usage:  "Azure Tenant Id",
			EnvVar: "TENANT_ID,AZURE_TENANT_ID,PLUGIN_TENANT_ID",
		},
		cli.StringFlag{
			Name:   "subscription-id",
			Usage:  "Azure Subscription Id",
			EnvVar: "SUBSCRIPTION_ID",
		},
		cli.StringFlag{
			Name:   "client-id",
			Usage:  "Azure Client ID (also called App ID)",
			EnvVar: "CLIENT_ID,AZURE_CLIENT_ID,PLUGIN_CLIENT_ID,AZURE_APP_ID",
		},
		cli.StringFlag{
			Name:   "oidc-token-id",
			Usage:  "OIDC ID token to exchange for Azure AD access token (federated credentials)",
			EnvVar: "PLUGIN_OIDC_TOKEN_ID",
		},
		cli.StringFlag{
			Name:   "azure-authority-host",
			Usage:  "Azure authority host base URL (e.g., https://login.microsoftonline.com, https://login.microsoftonline.us)",
			EnvVar: "AZURE_AUTHORITY_HOST",
		},
		cli.StringFlag{
			Name:   "snapshot-mode",
			Usage:  "Specify one of full, redo or time as snapshot mode",
			EnvVar: "PLUGIN_SNAPSHOT_MODE",
		},
		cli.StringFlag{
			Name:   "lifecycle-policy",
			Usage:  "Path to lifecycle policy file",
			EnvVar: "PLUGIN_LIFECYCLE_POLICY",
		},
		cli.StringFlag{
			Name:   "repository-policy",
			Usage:  "Path to repository policy file",
			EnvVar: "PLUGIN_REPOSITORY_POLICY",
		},
		cli.BoolFlag{
			Name:   "enable-cache",
			Usage:  "Set this flag to opt into caching with kaniko",
			EnvVar: "PLUGIN_ENABLE_CACHE",
		},
		cli.StringFlag{
			Name:   "cache-repo",
			Usage:  "Remote repository that will be used to store cached layers. Cache repo should be present in specified registry. enable-cache needs to be set to use this flag",
			EnvVar: "PLUGIN_CACHE_REPO",
		},
		cli.IntFlag{
			Name:   "cache-ttl",
			Usage:  "Cache timeout in hours. Defaults to two weeks.",
			EnvVar: "PLUGIN_CACHE_TTL",
		},
		cli.StringFlag{
			Name:   "artifact-file",
			Usage:  "Artifact file location that will be generated by the plugin. This file will include information of docker images that are uploaded by the plugin.",
			EnvVar: "PLUGIN_ARTIFACT_FILE",
		},
		cli.BoolFlag{
			Name:   "no-push",
			Usage:  "Set this flag if you only want to build the image, without pushing to a registry",
			EnvVar: "PLUGIN_NO_PUSH",
		},
		cli.BoolFlag{
			Name:   "push-only",
			Usage:  "Set this flag if you only want to push a pre-built image from a tarball",
			EnvVar: "PLUGIN_PUSH_ONLY",
		},
		cli.StringFlag{
			Name:   "source-tar-path",
			Usage:  "Path to the local tarball to be pushed when push-only is set",
			EnvVar: "PLUGIN_SOURCE_TAR_PATH",
		},
		cli.StringFlag{
			Name:   "tar-path",
			Usage:  "Set this flag to save the image as a tarball at path",
			EnvVar: "PLUGIN_TAR_PATH,PLUGIN_DESTINATION_TAR_PATH",
		},
		cli.StringFlag{
			Name:   "verbosity",
			Usage:  "Set this flag with value as oneof <panic|fatal|error|warn|info|debug|trace> to set the logging level for kaniko. Defaults to info.",
			EnvVar: "PLUGIN_VERBOSITY",
		},
		cli.StringFlag{
			Name:   "platform",
			Usage:  "Allows to build with another default platform than the host, similarly to docker build --platform",
			EnvVar: "PLUGIN_PLATFORM,PLUGIN_CUSTOM_PLATFORM",
		},
		cli.BoolFlag{
			Name:   "skip-unused-stages",
			Usage:  "build only used stages",
			EnvVar: "PLUGIN_SKIP_UNUSED_STAGES",
		},
		cli.StringFlag{
			Name:   "cache-dir",
			Usage:  "Set this flag to specify a local directory cache for base images",
			EnvVar: "PLUGIN_CACHE_DIR",
		},

		cli.BoolFlag{
			Name:   "cache-copy-layers",
			Usage:  "Enable or disable copying layers from the cache.",
			EnvVar: "PLUGIN_CACHE_COPY_LAYERS",
		},
		cli.BoolFlag{
			Name:   "cache-run-layers",
			Usage:  "Enable or disable running layers from the cache.",
			EnvVar: "PLUGIN_CACHE_RUN_LAYERS",
		},
		cli.BoolFlag{
			Name:   "cleanup",
			Usage:  "Enable or disable cleanup of temporary files.",
			EnvVar: "PLUGIN_CLEANUP",
		},
		cli.BoolFlag{
			Name:   "compressed-caching",
			Usage:  "Enable or disable compressed caching.",
			EnvVar: "PLUGIN_COMPRESSED_CACHING",
		},
		cli.StringFlag{
			Name:   "context-sub-path",
			Usage:  "Sub-path within the context to build.",
			EnvVar: "PLUGIN_CONTEXT_SUB_PATH",
		},
		cli.BoolFlag{
			Name:   "force",
			Usage:  "Force building the image even if it already exists.",
			EnvVar: "PLUGIN_FORCE",
		},
		cli.StringSliceFlag{
			Name:   "image-name-with-digest-file",
			Usage:  "Write image name with digest to a file.",
			EnvVar: "PLUGIN_IMAGE_NAME_WITH_DIGEST_FILE",
		},
		cli.StringFlag{
			Name:   "image-name-tag-with-digest-file",
			Usage:  "Write image name with tag and digest to a file.",
			EnvVar: "PLUGIN_IMAGE_NAME_TAG_WITH_DIGEST_FILE",
		},
		cli.BoolFlag{
			Name:   "insecure",
			Usage:  "Allow connecting to registries without TLS.",
			EnvVar: "PLUGIN_INSECURE",
		},
		cli.BoolFlag{
			Name:   "insecure-pull",
			Usage:  "Allow insecure pulls from the registry.",
			EnvVar: "PLUGIN_INSECURE_PULL",
		},
		cli.StringFlag{
			Name:   "insecure-registry",
			Usage:  "Use plain HTTP for registry communication.",
			EnvVar: "PLUGIN_INSECURE_REGISTRY",
		},
		cli.StringFlag{
			Name:   "log-format",
			Usage:  "Set the log format for build output.",
			EnvVar: "PLUGIN_LOG_FORMAT",
		},
		cli.BoolFlag{
			Name:   "log-timestamp",
			Usage:  "Show timestamps in build output.",
			EnvVar: "PLUGIN_LOG_TIMESTAMP",
		},
		cli.StringFlag{
			Name:   "oci-layout-path",
			Usage:  "Directory to store OCI layout.",
			EnvVar: "PLUGIN_OCI_LAYOUT_PATH",
		},
		cli.IntFlag{
			Name:   "push-retry",
			Usage:  "Number of times to retry pushing an image.",
			EnvVar: "PLUGIN_PUSH_RETRY",
		},
		cli.StringFlag{
			Name:   "registry-certificate",
			Usage:  "Path to a file containing a registry certificate.",
			EnvVar: "PLUGIN_REGISTRY_CERTIFICATE",
		},
		cli.StringFlag{
			Name:   "registry-client-cert",
			Usage:  "Path to a file containing a registry client certificate.",
			EnvVar: "PLUGIN_REGISTRY_CLIENT_CERT",
		},
		cli.BoolFlag{
			Name:   "skip-default-registry-fallback",
			Usage:  "Skip Docker Hub and default registry fallback.",
			EnvVar: "PLUGIN_SKIP_DEFAULT_REGISTRY_FALLBACK",
		},
		cli.BoolFlag{
			Name:   "reproducible",
			Usage:  "Create a reproducible image.",
			EnvVar: "PLUGIN_REPRODUCIBLE",
		},
		cli.BoolFlag{
			Name:   "single-snapshot",
			Usage:  "Only create a single snapshot of the image.",
			EnvVar: "PLUGIN_SINGLE_SNAPSHOT",
		},
		cli.BoolFlag{
			Name:   "skip-push-permission-check",
			Usage:  "Skip permission check when pushing.",
			EnvVar: "PLUGIN_SKIP_PUSH_PERMISSION_CHECK",
		},
		cli.BoolFlag{
			Name:   "skip-tls-verify-pull",
			Usage:  "Skip TLS verification when pulling.",
			EnvVar: "PLUGIN_SKIP_TLS_VERIFY_PULL",
		},
		cli.BoolFlag{
			Name:   "skip-tls-verify-registry",
			Usage:  "Skip TLS verification when connecting to a registry.",
			EnvVar: "PLUGIN_SKIP_TLS_VERIFY_REGISTRY",
		},
		cli.BoolFlag{
			Name:   "use-new-run",
			Usage:  "Skip TLS verification when connecting to a registry.",
			EnvVar: "PLUGIN_USE_NEW_RUN",
		},
		cli.BoolFlag{
			Name:   "ignore-var-run",
			Usage:  "Ignore the /var/run directory during build.",
			EnvVar: "PLUGIN_IGNORE_VAR_RUN",
		},
		cli.StringFlag{
			Name:   "ignore-path",
			Usage:  "Path to ignore during the build.",
			EnvVar: "PLUGIN_IGNORE_PATH",
		},
		cli.IntFlag{
			Name:   "image-fs-extract-retry",
			Usage:  "Number of retries for extracting filesystem layers.",
			EnvVar: "PLUGIN_IMAGE_FS_EXTRACT_RETRY",
		},
		cli.IntFlag{
			Name:   "image-download-retry",
			Usage:  "Number of retries for downloading base images.",
			EnvVar: "PLUGIN_IMAGE_DOWNLOAD_RETRY",
		},
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func run(c *cli.Context) error {
	// Check if push-only flag is set
	if c.Bool("push-only") {
		return handlePushOnly(c)
	}

	registry := c.String("registry")
	noPush := c.Bool("no-push")

	clientID := c.String("client-id")
	tenantID := c.String("tenant-id")
	oidcIdToken := c.String("oidc-token-id")
	authorityHost := c.String("azure-authority-host")

	var publicUrl string
	var err error
	publicUrl, err = setupAuth(
		tenantID,
		clientID,
		oidcIdToken,
		c.String("client-cert"),
		c.String("client-secret"),
		c.String("subscription-id"),
		registry,
		c.String("base-image-username"),
		c.String("base-image-password"),
		c.String("base-image-registry"),
		authorityHost,
		noPush,
	)
	if err != nil {
		return err
	}

	plugin := kaniko.Plugin{
		Build: kaniko.Build{
			DroneCommitRef:              c.String("drone-commit-ref"),
			DroneRepoBranch:             c.String("drone-repo-branch"),
			Dockerfile:                  c.String("dockerfile"),
			Context:                     c.String("context"),
			Tags:                        c.StringSlice("tags"),
			AutoTag:                     c.Bool("auto-tag"),
			AutoTagSuffix:               c.String("auto-tag-suffix"),
			ExpandTag:                   c.Bool("expand-tag"),
			Args:                        c.StringSlice("args"),
			ArgsNew:                     c.Generic("args-new").(*utils.CustomStringSliceFlag).GetValue(),
			IsMultipleBuildArgs:         c.Bool("plugin-multiple-build-agrs"),
			Target:                      c.String("target"),
			Repo:                        c.String("repo"),
			Mirrors:                     c.StringSlice("registry-mirrors"),
			Labels:                      c.StringSlice("custom-labels"),
			SnapshotMode:                c.String("snapshot-mode"),
			EnableCache:                 c.Bool("enable-cache"),
			CacheRepo:                   fmt.Sprintf("%s/%s", c.String("registry"), c.String("cache-repo")),
			CacheTTL:                    c.Int("cache-ttl"),
			DigestFile:                  defaultDigestFile,
			NoPush:                      noPush,
			Verbosity:                   c.String("verbosity"),
			CustomPlatform:              c.String("platform"),
			SkipUnusedStages:            c.Bool("skip-unused-stages"),
			CacheDir:                    c.String("cache-dir"),
			CacheCopyLayers:             c.Bool("cache-copy-layers"),
			CacheRunLayers:              c.Bool("cache-run-layers"),
			Cleanup:                     c.Bool("cleanup"),
			ContextSubPath:              c.String("context-sub-path"),
			Force:                       c.Bool("force"),
			ImageNameWithDigestFile:     c.String("image-name-with-digest-file"),
			ImageNameTagWithDigestFile:  c.String("image-name-tag-with-digest-file"),
			Insecure:                    c.Bool("insecure"),
			InsecurePull:                c.Bool("insecure-pull"),
			InsecureRegistry:            c.String("insecure-registry"),
			Label:                       c.String("label"),
			LogFormat:                   c.String("log-format"),
			LogTimestamp:                c.Bool("log-timestamp"),
			OCILayoutPath:               c.String("oci-layout-path"),
			PushRetry:                   c.Int("push-retry"),
			RegistryCertificate:         c.String("registry-certificate"),
			RegistryClientCert:          c.String("registry-client-cert"),
			SkipDefaultRegistryFallback: c.Bool("skip-default-registry-fallback"),
			Reproducible:                c.Bool("reproducible"),
			SingleSnapshot:              c.Bool("single-snapshot"),
			SkipTLSVerify:               c.Bool("skip-tls-verify"),
			SkipPushPermissionCheck:     c.Bool("skip-push-permission-check"),
			SkipTLSVerifyPull:           c.Bool("skip-tls-verify-pull"),
			SkipTLSVerifyRegistry:       c.Bool("skip-tls-verify-registry"),
			UseNewRun:                   c.Bool("use-new-run"),
			IgnorePath:                  c.String("ignore-path"),
			IgnorePaths:                 c.StringSlice("ignore-paths"),
			ImageFSExtractRetry:         c.Int("image-fs-extract-retry"),
			ImageDownloadRetry:          c.Int("image-download-retry"),
		},
		Artifact: kaniko.Artifact{
			Tags:         c.StringSlice("tags"),
			Repo:         c.String("repo"),
			Registry:     publicUrl, // this is public url on which the artifact can be seen
			ArtifactFile: c.String("artifact-file"),
			RegistryType: artifact.Docker,
		},
	}
	if c.IsSet("compressed-caching") {
		flag := c.Bool("compressed-caching")
		plugin.Build.CompressedCaching = &flag
	}
	if c.IsSet("ignore-var-run") {
		flag := c.Bool("ignore-var-run")
		plugin.Build.IgnoreVarRun = &flag
	}

	// Set tar-path if provided
	if c.IsSet("tar-path") {
		plugin.Build.TarPath = c.String("tar-path")
	}

	return plugin.Exec()
}

func setupAuth(tenantId, clientId, oidcIdToken, cert,
	clientSecret, subscriptionId, registry, dockerUsername, dockerPassword, dockerRegistry, authorityHost string, noPush bool) (string, error) {
	if registry == "" {
		return "", fmt.Errorf("registry must be specified")
	}

	var aadAccessToken string
	var acrToken string
	var publicUrl string
	var err error

	if oidcIdToken != "" {
		// OIDC authentication flow requires tenantId and clientId
		if tenantId == "" || clientId == "" {
			if noPush {
				logrus.Warnf("NO_PUSH mode: tenantId or clientId not provided for OIDC")
				return "", nil
			}
			return "", fmt.Errorf("tenantId and clientId must be provided for OIDC authentication")
		}
		logrus.Debug("Using OIDC authentication flow")
		// Exchange OIDC ID token for AAD access token via client_assertion
		aadAccessToken, err = azureutil.GetAADAccessTokenViaClientAssertion(context.Background(), tenantId, clientId, oidcIdToken, authorityHost)
		if err != nil {
			return handleError(noPush, err, "failed to get AAD token via OIDC")
		}
		publicUrl, err = getPublicUrl(aadAccessToken, registry, subscriptionId)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get public url with error: %s\n", err)
		}
		// Exchange AAD access token to ACR refresh token
		acrToken, err = fetchACRToken(tenantId, aadAccessToken, registry)
		if err != nil {
			return handleError(noPush, err, "failed to fetch ACR token")
		}
	} else {
		logrus.Debug("Using traditional Azure AD authentication flow")
		// Validate that if tenantId is provided, clientId must also be provided
		// (unless using managed identity with no explicit tenantId)
		if tenantId != "" && clientId == "" && clientSecret == "" && cert == "" {
			if noPush {
				logrus.Warnf("NO_PUSH mode: tenantId provided but clientId is missing")
				return "", nil
			}
			return "", fmt.Errorf("tenantId and clientId must be provided")
		}
		acrToken, publicUrl, err = getACRToken(subscriptionId, tenantId, clientId, clientSecret, cert, registry)
		if err != nil {
			return handleError(noPush, err, "failed to fetch ACR Token")
		}
	}

	if err := setDockerAuth(username, acrToken, registry, dockerUsername, dockerPassword, dockerRegistry); err != nil {
		return handleError(noPush, err, "failed to create docker config")
	}
	return publicUrl, nil
}

// Error handling
func handleError(noPush bool, err error, msg string) (string, error) {
	if noPush {
		logrus.Warnf("NO_PUSH mode: %s: %v", msg, err)
		return "", nil
	}
	return "", errors.Wrap(err, msg)
}

func getACRToken(subscriptionId, tenantId, clientId, clientSecret, cert, registry string) (string, string, error) {
	// Handle managed identity (when no clientSecret or cert provided)
	if clientSecret == "" && cert == "" {
		if tenantId == "" {
			tenantId = os.Getenv("AZURE_TENANT_ID")
			if tenantId == "" {
				tenantId = os.Getenv("TENANT_ID")
			}
		}
		opts := &azidentity.DefaultAzureCredentialOptions{}
		if tenantId != "" {
			opts.TenantID = tenantId
		}
		cred, err := azidentity.NewDefaultAzureCredential(opts)
		if err != nil {
			return "", "", errors.Wrap(err, "failed to get credentials")
		}
		policy := policy.TokenRequestOptions{
			Scopes: []string{"https://management.azure.com/.default"},
		}
		azToken, err := cred.GetToken(context.Background(), policy)
		if err != nil {
			return "", "", errors.Wrap(err, "failed to fetch access token")
		}
		publicUrl, err := getPublicUrl(azToken.Token, registry, subscriptionId)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get public url with error: %s\n", err)
		}
		if tenantId == "" {
			return "", "", fmt.Errorf("tenantId cannot be empty for ACR token exchange")
		}
		ACRToken, err := fetchACRToken(tenantId, azToken.Token, registry)
		if err != nil {
			return "", "", errors.Wrap(err, "failed to fetch ACR token")
		}
		return ACRToken, publicUrl, nil
	}

	if tenantId == "" {
		return "", "", fmt.Errorf("tenantId can't be empty for AAD authentication")
	}
	if clientId == "" {
		return "", "", fmt.Errorf("clientId can't be empty for AAD authentication")
	}

	if clientSecret == "" && cert == "" {
		return "", "", fmt.Errorf("one of client secret or cert should be defined")
	}

	// in case of authentication via cert
	if cert != "" {
		err := setupACRCert(cert)
		if err != nil {
			errors.Wrap(err, "failed to push setup cert file")
		}
	}

	if err := os.Setenv(clientIdEnv, clientId); err != nil {
		return "", "", errors.Wrap(err, "failed to set env variable client Id")
	}
	if err := os.Setenv(clientSecretKeyEnv, clientSecret); err != nil {
		return "", "", errors.Wrap(err, "failed to set env variable client secret")
	}
	if err := os.Setenv(tenantKeyEnv, tenantId); err != nil {
		return "", "", errors.Wrap(err, "failed to set env variable tenant Id")
	}
	if err := os.Setenv(certPathEnv, ACRCertPath); err != nil {
		return "", "", errors.Wrap(err, "failed to set env variable cert path")
	}
	env, err := azidentity.NewEnvironmentCredential(nil)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get env credentials from azure")
	}

	policy := policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	}
	os.Unsetenv(clientIdEnv)
	os.Unsetenv(clientSecretKeyEnv)
	os.Unsetenv(tenantKeyEnv)
	os.Unsetenv(certPathEnv)

	azToken, err := env.GetToken(context.Background(), policy)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to fetch access token")
	}

	publicUrl, err := getPublicUrl(azToken.Token, registry, subscriptionId)
	if err != nil {
		// execution should not fail because of this error.
		fmt.Fprintf(os.Stderr, "failed to get public url with error: %s\n", err)
	}

	ACRToken, err := fetchACRToken(tenantId, azToken.Token, registry)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to fetch ACR token")
	}
	return ACRToken, publicUrl, nil
}

func fetchACRToken(tenantId, token, registry string) (string, error) {
	formData := url.Values{
		"grant_type":   {"access_token"},
		"service":      {registry},
		"tenant":       {tenantId},
		"access_token": {token},
	}
	jsonResponse, err := http.PostForm(fmt.Sprintf("https://%s/oauth2/exchange", registry), formData)
	if err != nil {
		return "", errors.Wrap(err, "failed to fetch ACR token")
	}
	var response map[string]interface{}
	err = json.NewDecoder(jsonResponse.Body).Decode(&response)
	if err != nil {
		return "", errors.Wrap(err, "failed to decode oauth exchange response")
	}

	if x, found := response["refresh_token"]; found {
		s, ok := x.(string)
		if !ok {
			errors.New("failed to cast refresh token from acr")
		} else {
			return s, nil
		}
	} else {
		return "", errors.Wrap(err, "refresh token not found in response of oauth exchange call")
	}
	return "", errors.New("failed to get refresh token from acr")
}

func setupACRCert(cert string) error {
	decoded, err := base64.StdEncoding.DecodeString(cert)
	if err != nil {
		return errors.Wrap(err, "failed to base64 decode ACR certificate")
	}
	err = ioutil.WriteFile(ACRCertPath, []byte(decoded), 0644)
	if err != nil {
		return errors.Wrap(err, "failed to write ACR certificate")
	}
	return nil
}

func getPublicUrl(token, registryUrl, subscriptionId string) (string, error) {
	// for backward compatibilty, if the subscription id is not defined, do not fail step.
	if len(subscriptionId) == 0 {
		return "", nil
	}

	registry := strings.Split(registryUrl, ".")[0]
	baseURL := "https://management.azure.com/subscriptions/" +
		subscriptionId + "/resources?$filter=resourceType%20eq%20'Microsoft.ContainerRegistry/registries'%20and%20name%20eq%20'" +
		registry + "'&api-version=2021-04-01&$select=id"

	method := "GET"
	client := &http.Client{}

	cnt := 0

	for {
		// this is just in case we end up cycling through nextLink's infinitely.
		// this should not happen - added as a precaution.
		if cnt > maxPageCount {
			break
		}
		cnt++
		req, err := http.NewRequest(method, baseURL, nil)
		if err != nil {
			return "", errors.Wrap(err, "failed to create request for getting container registry setting")
		}

		req.Header.Add("Authorization", "Bearer "+token)
		res, err := client.Do(req)
		if err != nil {
			return "", errors.Wrap(err, "failed to send request for getting container registry setting")
		}
		defer res.Body.Close()

		var response strct
		err = json.NewDecoder(res.Body).Decode(&response)
		if err != nil {
			return "", errors.Wrap(err, "failed to send request for getting container registry setting")
		}

		if len(response.Value) > 0 {
			if response.Value[0].ID == "" { // should not happen
				return "", errors.New("received empty registry ID from /subscriptions API")
			}
			return finalUrl + encodeParam(response.Value[0].ID), nil
		}

		if response.NextLink == "" {
			// No more pages, break the loop
			break
		}

		baseURL = response.NextLink
	}

	return "", errors.New("did not receive any registry information from /subscriptions API")
}

func setDockerAuth(username, password, registry, dockerUsername, dockerPassword, dockerRegistry string) error {
	dockerConfig := docker.NewConfig()
	pushToRegistryCreds := docker.RegistryCredentials{
		Registry: registry,
		Username: username,
		Password: password,
	}

	credentials := []docker.RegistryCredentials{pushToRegistryCreds}

	if dockerRegistry != "" {
		pullFromRegistryCreds := docker.RegistryCredentials{
			Registry: dockerRegistry,
			Username: dockerUsername,
			Password: dockerPassword,
		}
		credentials = append(credentials, pullFromRegistryCreds)
	} else {
		fmt.Println("\033[33mTo ensure consistent and reliable pipeline execution, we recommend setting up a Base Image Connector.\033[0m\n" +
			"\033[33mWhile optional at this time, configuring it helps prevent failures caused by Docker Hub's rate limits.\033[0m")
	}
	return dockerConfig.CreateDockerConfig(credentials, dockerConfigPath)
}

func encodeParam(s string) string {
	return url.QueryEscape(s)
}

func handlePushOnly(c *cli.Context) error {
	// Validate inputs for push-only operation
	sourceTarPath := c.String("source-tar-path")
	if sourceTarPath == "" {
		return fmt.Errorf("source_tar_path is required when push_only is set")
	}

	if _, err := os.Stat(sourceTarPath); os.IsNotExist(err) {
		return fmt.Errorf("image tarball does not exist at path: %s", sourceTarPath)
	}

	repo := c.String("repo")
	registry := c.String("registry")
	if repo == "" || registry == "" {
		return fmt.Errorf("repository and registry must be specified for push-only operation")
	}

	// Resolve Azure client/tenant and OIDC via CLI flags
	clientID := c.String("client-id")
	tenantID := c.String("tenant-id")
	oidcIdToken := c.String("oidc-token-id")
	authorityHost := c.String("azure-authority-host")

	var publicUrl string
	var err error
	publicUrl, err = setupAuth(
		tenantID,
		clientID,
		oidcIdToken,
		c.String("client-cert"),
		c.String("client-secret"),
		c.String("subscription-id"),
		registry,
		c.String("base-image-username"),
		c.String("base-image-password"),
		c.String("base-image-registry"),
		authorityHost,
		false,
	)
	if err != nil {
		return err
	}

	// Load the image from the tarball
	logrus.Infof("Loading image from tarball: %s", sourceTarPath)

	img, err := crane.Load(sourceTarPath)
	if err != nil {
		return fmt.Errorf("failed to load image from tarball: %v", err)
	}

	// Check if the Docker config directory exists (should have been created by setupAuth)
	if _, err := os.Stat(dockerConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("Docker config directory does not exist: %v", err)
	} else if err != nil {
		return fmt.Errorf("error checking Docker config directory: %v", err)
	}

	// Explicitly set DOCKER_CONFIG environment variable to ensure crane finds the config
	if err := os.Setenv("DOCKER_CONFIG", dockerConfigPath); err != nil {
		return fmt.Errorf("failed to set DOCKER_CONFIG environment variable: %v", err)
	}

	// Setup crane options
	opts := []crane.Option{
		crane.WithAuthFromKeychain(authn.DefaultKeychain),
	}

	// Push for each tag
	tags := c.StringSlice("tags")
	if len(tags) == 0 {
		tags = []string{"latest"}
	}

	// Use the registry from setupAuth if publicUrl is available, otherwise use the provided registry
	pushRegistry := registry
	if publicUrl != "" {
		logrus.Infof("Using public URL for pushing: %s", publicUrl)
		// Extract just the registry part from the full URL if needed
		// This depends on the format of publicUrl, adjust parsing as needed
		pushRegistry = publicUrl
	}

	for _, tag := range tags {
		dest := fmt.Sprintf("%s/%s:%s", pushRegistry, repo, tag)
		logrus.Infof("Pushing image to: %s", dest)

		if err := crane.Push(img, dest, opts...); err != nil {
			return fmt.Errorf("failed to push image to %s: %v", dest, err)
		}

		logrus.Infof("Successfully pushed image to %s", dest)
	}

	return nil
}

type strct struct {
	Value []struct {
		ID string `json:"id"`
	} `json:"value"`
	NextLink string `json:"nextLink"` // for pagination
}
