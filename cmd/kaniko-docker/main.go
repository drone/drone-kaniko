package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	kaniko "github.com/drone/drone-kaniko"
	"github.com/drone/drone-kaniko/internal/artifactory"
	"github.com/drone/drone-kaniko/pkg/artifact"
	"github.com/drone/drone-kaniko/pkg/docker"
	"github.com/drone/drone-kaniko/pkg/utils"
)

const (
	// Docker file path
	dockerPath       string = "/kaniko/.docker"
	dockerConfigPath string = "/kaniko/.docker/config.json"

	v1RegistryURL string = "https://index.docker.io/v1/" // Default registry

	defaultDigestFile string = "/kaniko/digest-file"
)

var (
	version = "unknown"

	setupDockerAuth        = setDockerAuth
	executeKaniko          = func(plugin kaniko.Plugin) error { return plugin.Exec() }
	setupOIDCAuth          = configureOIDCAuth
	exchangeOIDCCredential = exchangeOIDC
)

func main() {
	// Load env-file if it exists first
	if env := os.Getenv("PLUGIN_ENV_FILE"); env != "" {
		if err := godotenv.Load(env); err != nil {
			logrus.Fatal(err)
		}
	}

	app := cli.NewApp()
	app.Name = "kaniko docker plugin"
	app.Usage = "kaniko docker plugin"
	app.Action = run
	app.Version = version
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
			Name:   "expand-repo",
			Usage:  "Prepends the registry url to the repo if registry url is not specified in repo name",
			EnvVar: "PLUGIN_EXPAND_REPO",
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
			Name:   "dockerconfig",
			Usage:  "docker json dockerconfig",
			EnvVar: "PLUGIN_CONFIG",
		},
		cli.StringFlag{
			Name:   "url",
			Usage:  "JFrog Artifactory URL for OIDC authentication",
			EnvVar: "PLUGIN_URL",
		},
		cli.StringFlag{
			Name:   "artifactory-oidc-token",
			Usage:  "OIDC token for JFrog Artifactory authentication",
			EnvVar: "ARTIFACTORY_OIDC_TOKEN",
		},
		cli.StringFlag{
			Name:   "artifactory-oidc-provider-name",
			Usage:  "JFrog OIDC provider name",
			EnvVar: "ARTIFACTORY_OIDC_PROVIDER_NAME",
		},
		cli.StringFlag{
			Name:   "artifactory-oidc-project-key",
			Usage:  "JFrog OIDC project key",
			EnvVar: "ARTIFACTORY_OIDC_PROJECT_KEY",
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
		cli.StringSliceFlag{
			Name:   "custom-labels",
			Usage:  "additional k=v labels",
			EnvVar: "PLUGIN_CUSTOM_LABELS",
		},
		cli.StringFlag{
			Name:   "registry",
			Usage:  "docker registry of registry to push image to",
			Value:  v1RegistryURL,
			EnvVar: "PLUGIN_REGISTRY",
		},
		cli.StringFlag{
			Name:   "base-image-registry",
			Usage:  "Docker registry for base image",
			EnvVar: "PLUGIN_DOCKER_REGISTRY,PLUGIN_BASE_IMAGE_REGISTRY,DOCKER_REGISTRY",
		},
		cli.StringSliceFlag{
			Name:   "registry-mirrors",
			Usage:  "docker registry mirrors",
			EnvVar: "PLUGIN_REGISTRY_MIRRORS",
		},
		cli.StringFlag{
			Name:   "username",
			Usage:  "docker username of registry to push image to",
			EnvVar: "PLUGIN_USERNAME",
		},
		cli.StringFlag{
			Name:   "base-image-username",
			Usage:  "Docker username for base image registry",
			EnvVar: "PLUGIN_DOCKER_USERNAME,PLUGIN_BASE_IMAGE_USERNAME,DOCKER_USERNAME",
		},
		cli.StringFlag{
			Name:   "password",
			Usage:  "docker password of registry to push image to",
			EnvVar: "PLUGIN_PASSWORD",
		},
		cli.StringFlag{
			Name:   "base-image-password",
			Usage:  "Docker password for base image registry",
			EnvVar: "PLUGIN_DOCKER_PASSWORD,PLUGIN_BASE_IMAGE_PASSWORD,DOCKER_PASSWORD",
		},
		cli.BoolFlag{
			Name:   "skip-tls-verify",
			Usage:  "Skip registry tls verify",
			EnvVar: "PLUGIN_SKIP_TLS_VERIFY",
		},
		cli.StringFlag{
			Name:   "snapshot-mode",
			Usage:  "Specify one of full, redo or time as snapshot mode",
			EnvVar: "PLUGIN_SNAPSHOT_MODE",
		},
		cli.BoolFlag{
			Name:   "enable-cache",
			Usage:  "Set this flag to opt into caching with kaniko",
			EnvVar: "PLUGIN_ENABLE_CACHE",
		},
		cli.StringFlag{
			Name:   "cache-repo",
			Usage:  "Remote repository that will be used to store cached layers. enable-cache needs to be set to use this flag",
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
		cli.StringFlag{
			Name:   "tar-path",
			Usage:  "Set this flag to save the image as a tarball at path",
			EnvVar: "PLUGIN_TAR_PATH, PLUGIN_DESTINATION_TAR_PATH",
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
			Name:   "output-file",
			Usage:  "Output file location that will be generated by the plugin. This file will include information of the output that are exported by the plugin.",
			EnvVar: "DRONE_OUTPUT",
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
		cli.StringFlag{
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
		cli.StringSliceFlag{
			Name:   "ignore-paths",
			Usage:  "Path to ignore during the build.",
			EnvVar: "PLUGIN_IGNORE_PATHS",
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
		cli.StringFlag{
			Name:   "source-tar-path",
			Usage:  "Set this flag for the source tarball during push operations.",
			EnvVar: "PLUGIN_SOURCE_TAR_PATH",
		},
		cli.BoolFlag{
			Name:   "push-only",
			Usage:  "Specify if the operation is push-only",
			EnvVar: "PLUGIN_PUSH_ONLY",
		},
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func run(c *cli.Context) error {
	username := c.String("username")
	noPush := c.Bool("no-push")
	configOverride := c.String("dockerconfig")
	registryCertificate := c.String("registry-certificate")
	if hasArtifactoryOIDCInputs(c) {
		if registryCertificate != "" {
			return fmt.Errorf("Artifactory OIDC authentication does not support registry certificates")
		}
		resources, err := setupOIDCAuth(context.Background(), oidcInputs(c), c.String("artifactory-oidc-project-key"))
		if err != nil {
			return err
		}
		defer func() {
			if resources.cleanup == nil {
				return
			}
			if cleanupErr := resources.cleanup(); cleanupErr != nil {
				logrus.WithError(cleanupErr).Error("failed to clean up Artifactory OIDC authentication material")
			}
		}()
	} else {
		// if configOverride is provided, use this directly to write to docker config file
		if len(configOverride) > 0 {
			if err := docker.WriteDockerConfig([]byte(configOverride), dockerPath); err != nil {
				return err
			}
		} else if !noPush || username != "" {
			// setup auth when pushing/pulling or credentials are defined and docker config override is false
			err := setupDockerAuth(
				c.String("username"),
				c.String("password"),
				c.String("registry"),
				c.String("base-image-username"),
				c.String("base-image-password"),
				c.String("base-image-registry"),
			)
			if err != nil {
				return errors.Wrap(err, "failed to create docker config")
			}
		}
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
			Repo:                        buildRepo(c.String("registry"), c.String("repo"), c.Bool("expand-repo")),
			Mirrors:                     c.StringSlice("registry-mirrors"),
			Labels:                      c.StringSlice("custom-labels"),
			SkipTlsVerify:               c.Bool("skip-tls-verify"),
			SnapshotMode:                c.String("snapshot-mode"),
			EnableCache:                 c.Bool("enable-cache"),
			CacheRepo:                   effectiveCacheRepository(c),
			CacheTTL:                    c.Int("cache-ttl"),
			DigestFile:                  defaultDigestFile,
			NoPush:                      noPush,
			TarPath:                     c.String("tar-path"),
			Verbosity:                   c.String("verbosity"),
			CustomPlatform:              c.String("platform"),
			PushOnly:                    c.Bool("push-only"),
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
			RegistryCertificate:         registryCertificate,
			RegistryClientCert:          c.String("registry-client-cert"),
			SkipDefaultRegistryFallback: c.Bool("skip-default-registry-fallback"),
			Reproducible:                c.Bool("reproducible"),
			SingleSnapshot:              c.Bool("single-snapshot"),
			SkipTLSVerify:               c.Bool("skip-tls-verify"),
			SkipPushPermissionCheck:     c.Bool("skip-push-permission-check"),
			SkipTLSVerifyPull:           c.Bool("skip-tls-verify-pull"),
			SkipTLSVerifyRegistry:       c.Bool("skip-tls-verify-registry"),
			SourceTarPath:               c.String("source-tar-path"),
			UseNewRun:                   c.Bool("use-new-run"),
			IgnorePath:                  c.String("ignore-path"),
			IgnorePaths:                 c.StringSlice("ignore-paths"),

			ImageFSExtractRetry: c.Int("image-fs-extract-retry"),
			ImageDownloadRetry:  c.Int("image-download-retry"),
		},
		Artifact: kaniko.Artifact{
			Tags:         c.StringSlice("tags"),
			Repo:         buildRepo(c.String("registry"), c.String("repo"), c.Bool("expand-repo")),
			Registry:     c.String("registry"),
			ArtifactFile: c.String("artifact-file"),
			RegistryType: artifact.Docker,
		},
		Output: kaniko.Output{
			OutputFile: c.String("output-file"),
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
	return executeKaniko(plugin)
}

type authResources struct {
	cleanup func() error
}

func hasArtifactoryOIDCInputs(c *cli.Context) bool {
	return c.String("url") != "" ||
		c.String("artifactory-oidc-token") != "" ||
		c.String("artifactory-oidc-provider-name") != "" ||
		c.String("artifactory-oidc-project-key") != ""
}

func oidcInputs(c *cli.Context) artifactory.Inputs {
	return artifactory.Inputs{
		ServerURL:             c.String("url"),
		IDToken:               c.String("artifactory-oidc-token"),
		ProviderName:          c.String("artifactory-oidc-provider-name"),
		Registry:              c.String("registry"),
		Repository:            buildRepo(c.String("registry"), c.String("repo"), c.Bool("expand-repo")),
		DockerConfigRoot:      filepath.Dir(dockerPath),
		DockerConfigOverride:  c.String("dockerconfig"),
		Username:              c.String("username"),
		Password:              c.String("password"),
		BaseImageRegistry:     c.String("base-image-registry"),
		BaseImageUsername:     c.String("base-image-username"),
		BaseImagePassword:     c.String("base-image-password"),
		EnableCache:           c.Bool("enable-cache"),
		CacheRepository:       effectiveCacheRepository(c),
		CacheTTL:              c.Int("cache-ttl"),
		CacheDir:              c.String("cache-dir"),
		CacheCopyLayers:       c.Bool("cache-copy-layers"),
		CacheRunLayers:        c.Bool("cache-run-layers"),
		CompressedCache:       c.Bool("compressed-caching"),
		RegistryMirrors:       c.StringSlice("registry-mirrors"),
		RegistryClientCert:    c.String("registry-client-cert"),
		Insecure:              c.Bool("insecure"),
		InsecurePull:          c.Bool("insecure-pull"),
		InsecureRegistry:      c.String("insecure-registry"),
		SkipTLSVerify:         c.Bool("skip-tls-verify"),
		SkipTLSVerifyPull:     c.Bool("skip-tls-verify-pull"),
		SkipTLSVerifyRegistry: c.Bool("skip-tls-verify-registry"),
	}
}

func configureOIDCAuth(
	ctx context.Context, inputs artifactory.Inputs, projectKey string,
) (authResources, error) {
	if err := artifactory.ValidateInputs(inputs); err != nil {
		return authResources{}, err
	}
	destination, err := artifactory.ValidateDestination(inputs.ServerURL, inputs.Registry, inputs.Repository)
	if err != nil {
		return authResources{}, err
	}

	credential, err := exchangeOIDCCredential(
		ctx,
		&http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone(), Timeout: 30 * time.Second},
		mustTokenEndpoint(inputs.ServerURL),
		inputs.IDToken,
		inputs.ProviderName,
		projectKey,
	)
	if err != nil {
		return authResources{}, err
	}
	dockerCleanup, err := artifactory.SetupCredentials(inputs, destination, credential, time.Now)
	if err != nil {
		return authResources{}, err
	}
	return authResources{
		cleanup: dockerCleanup,
	}, nil
}

func exchangeOIDC(
	ctx context.Context, client *http.Client, endpoint, idToken, providerName, projectKey string,
) (artifactory.Credential, error) {
	return (artifactory.ExchangeClient{HTTPClient: client}).Exchange(ctx, endpoint, idToken, providerName, projectKey)
}

func mustTokenEndpoint(serverURL string) string {
	endpoint, err := artifactory.ResolveTokenEndpoint(serverURL)
	if err != nil {
		logrus.WithError(err).Error("failed to resolve JFrog OIDC token endpoint")
	}
	return endpoint
}

func setDockerAuth(username, password, registry, baseImageUsername, baseImagePassword, baseImageRegistry string) error {
	dockerConfig := docker.NewConfig()
	pushToRegistryCreds := docker.RegistryCredentials{
		Registry: registry,
		Username: username,
		Password: password,
	}
	credentials := []docker.RegistryCredentials{pushToRegistryCreds}

	if baseImageRegistry != "" {
		pullFromRegistryCreds := docker.RegistryCredentials{
			Registry: baseImageRegistry,
			Username: baseImageUsername,
			Password: baseImagePassword,
		}
		credentials = append(credentials, pullFromRegistryCreds)
	} else {
		fmt.Println("\033[33mTo ensure consistent and reliable pipeline execution, we recommend setting up a Base Image Connector.\033[0m\n" +
			"\033[33mWhile optional at this time, configuring it helps prevent failures caused by Docker Hub's rate limits.\033[0m")
	}
	// Creates docker config for both the regustries used for authentication
	return dockerConfig.CreateDockerConfig(credentials, dockerPath)
}

func buildRepo(registry, repo string, expandRepo bool) string {
	if !expandRepo || registry == "" || registry == v1RegistryURL {
		// No custom registry, just return the repo name
		return repo
	}
	// Trim off trailing slash to prevent double slash when combining with repo
	registry = strings.TrimSuffix(registry, "/")
	if strings.HasPrefix(repo, registry+"/") {
		// Repo already includes the registry prefix
		// For backward compatibility, we won't add the prefix again.
		return repo
	}
	// Prefix the repo with the registry
	return registry + "/" + repo
}

func effectiveCacheRepository(c *cli.Context) string {
	repository := c.String("cache-repo")
	if repository == "" {
		return ""
	}
	if hasArtifactoryOIDCInputs(c) && isQualifiedRepository(repository) {
		return repository
	}
	return buildRepo(c.String("registry"), repository, c.Bool("expand-repo") || hasArtifactoryOIDCInputs(c))
}

func isQualifiedRepository(repository string) bool {
	firstComponent, _, found := strings.Cut(repository, "/")
	if !found {
		return false
	}
	return strings.EqualFold(firstComponent, "localhost") ||
		strings.Contains(firstComponent, ".") ||
		strings.Contains(firstComponent, ":")
}
