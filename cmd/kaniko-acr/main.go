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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	kaniko "github.com/drone/drone-kaniko"
	"github.com/drone/drone-kaniko/pkg/artifact"
	"github.com/drone/drone-kaniko/pkg/docker"
)

const (
	dockerPath        string = "/kaniko/.docker"
	accessKeyEnv      string = "AZURE_CLIENT_ID"
	secretKeyEnv      string = "AZURE_CLIENT_SECRET"
	tenantKeyEnv      string = "AZURE_TENANT_ID"
	certPathEnv       string = "AZURE_CLIENT_CERTIFICATE_PATH"
	dockerConfigPath  string = "/kaniko/.docker/acr/config-acr.json"
	kanikoVersionEnv  string = "KANIKO_VERSION"
	defaultDigestFile string = "/kaniko/digest-file"
)

var (
	acrCertPath   = "/kaniko/acr-cert.pem"
	pluginVersion = "unknown"
	username      = "00000000-0000-0000-0000-000000000000"
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
			Name:   "docker-username",
			Usage:  "docker username",
			EnvVar: "PLUGIN_USERNAME,DOCKER_USERNAME",
		},
		cli.StringFlag{
			Name:   "docker-password",
			Usage:  "docker password",
			EnvVar: "PLUGIN_PASSWORD,DOCKER_PASSWORD",
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
			Usage:  "Azure client certificate",
			EnvVar: "CLIENT_CERTIFICATE",
		},
		cli.StringFlag{
			Name:   "tenant-id",
			Usage:  "Azure Tenant Id",
			EnvVar: "TENANT_ID",
		},
		cli.StringFlag{
			Name:   "client-id",
			Usage:  "Azure Client Id",
			EnvVar: "CLIENT_ID",
		},
		cli.StringFlag{
			Name:   "assume-role",
			Usage:  "Assume a role",
			EnvVar: "PLUGIN_ASSUME_ROLE",
		},
		cli.StringFlag{
			Name:   "external-id",
			Usage:  "Used along with assume role to assume a role",
			EnvVar: "PLUGIN_EXTERNAL_ID",
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
		cli.StringFlag{
			Name:   "verbosity",
			Usage:  "Set this flag with value as oneof <panic|fatal|error|warn|info|debug|trace> to set the logging level for kaniko. Defaults to info.",
			EnvVar: "PLUGIN_VERBOSITY",
		},
		cli.StringFlag{
			Name:   "platform",
			Usage:  "Allows to build with another default platform than the host, similarly to docker build --platform",
			EnvVar: "PLUGIN_PLATFORM",
		},
		cli.BoolFlag{
			Name:   "skip-unused-stages",
			Usage:  "build only used stages",
			EnvVar: "PLUGIN_SKIP_UNUSED_STAGES",
		},
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func run(c *cli.Context) error {
	registry := c.String("registry")
	noPush := c.Bool("no-push")

	dockerConfig, err := createDockerConfig(
		c.String("tenant-id"),
		c.String("client-id"),
		c.String("client-cert"),
		c.String("client-secret"),
		registry,
		noPush,
	)
	if err != nil {
		return err
	}

	jsonBytes, err := json.Marshal(dockerConfig)
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(dockerConfigPath, jsonBytes, 0644); err != nil {
		return err
	}

	plugin := kaniko.Plugin{
		Build: kaniko.Build{
			DroneCommitRef:   c.String("drone-commit-ref"),
			DroneRepoBranch:  c.String("drone-repo-branch"),
			Dockerfile:       c.String("dockerfile"),
			Context:          c.String("context"),
			Tags:             c.StringSlice("tags"),
			AutoTag:          c.Bool("auto-tag"),
			AutoTagSuffix:    c.String("auto-tag-suffix"),
			ExpandTag:        c.Bool("expand-tag"),
			Args:             c.StringSlice("args"),
			Target:           c.String("target"),
			Repo:             fmt.Sprintf("%s/%s", c.String("registry"), c.String("repo")),
			Mirrors:          c.StringSlice("registry-mirrors"),
			Labels:           c.StringSlice("custom-labels"),
			SnapshotMode:     c.String("snapshot-mode"),
			EnableCache:      c.Bool("enable-cache"),
			CacheRepo:        fmt.Sprintf("%s/%s", c.String("registry"), c.String("cache-repo")),
			CacheTTL:         c.Int("cache-ttl"),
			DigestFile:       defaultDigestFile,
			NoPush:           noPush,
			Verbosity:        c.String("verbosity"),
			Platform:         c.String("platform"),
			SkipUnusedStages: c.Bool("skip-unused-stages"),
		},
		Artifact: kaniko.Artifact{
			Tags:         c.StringSlice("tags"),
			Repo:         c.String("repo"),
			Registry:     c.String("registry"),
			ArtifactFile: c.String("artifact-file"),
			RegistryType: artifact.Docker,
		},
	}
	return plugin.Exec()
}

func createDockerConfig(tenantId, clientId, cert,
	clientSecret, registry string, noPush bool) (*docker.Config, error) {
	dockerConfig := docker.NewConfig()
	if registry == "" {
		return nil, fmt.Errorf("registry must be specified")
	}

	if noPush {
		return dockerConfig, nil
	}

	// case of client secret or cert based auth
	if clientId != "" {
		// only setup auth when pushing or credentials are defined

		token, err := getAcrToken(tenantId, clientId, clientSecret, cert, registry)
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch acrToken")
		}
		err = createDockerCfgFile(username, token, registry)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create docker config")
		}
	} else {
		return nil, fmt.Errorf("managed authentication is not supported")
	}

	return dockerConfig, nil
}

func getAcrToken(tenantId, clientId, clientSecret, cert, registry string) (string, error) {
	if tenantId == "" {
		return "", fmt.Errorf("tenantId can't be empty foe AAD authentication")
	}

	if clientId == "" {
		return "", fmt.Errorf("clientId can't be empty foe AAD authentication")
	}

	if clientSecret == "" && cert == "" {
		return "", fmt.Errorf("one of accessKey or secretKey should be defined")
	}

	// in case of authentication via cert
	if cert != "" {
		err := setupACRCert(cert)
		if err != nil {
			errors.Wrap(err, "failed to push setup cert file")
		}
	}

	// TODO check for presence of file as well.
	os.Setenv(accessKeyEnv, clientId)
	os.Setenv(secretKeyEnv, clientSecret)
	os.Setenv(tenantKeyEnv, tenantId)
	env, err := azidentity.NewEnvironmentCredential(nil)
	context.Background()
	policy := policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	}

	os.Unsetenv(accessKeyEnv)
	os.Unsetenv(secretKeyEnv)
	os.Unsetenv(tenantKeyEnv)
	os.Unsetenv(certPathEnv)

	azToken, err := env.GetToken(context.Background(), policy)
	if err != nil {
		return "", errors.Wrap(err, "failed to fetch access token")
	}

	acrToken, err := fetchAcrToken(tenantId, azToken.Token, registry)
	if err != nil {
		return "", errors.Wrap(err, "failed to fetch acr token")
	}
	return acrToken, nil
}

func fetchAcrToken(tenantId, token, registry string) (string, error) {
	formData := url.Values{
		"grant_type":   {"access_token"},
		"service":      {registry},
		"tenant":       {tenantId},
		"access_token": {token},
	}
	jsonResponse, err := http.PostForm(fmt.Sprintf("https://%s/oauth2/exchange", registry), formData)
	if err != nil {
		return "", errors.Wrap(err, "failed to fetch acr token")
	}
	var response map[string]interface{}
	json.NewDecoder(jsonResponse.Body).Decode(&response)
	return response["refresh_token"].(string), nil
}

// Create the docker config file for authentication
func createDockerCfgFile(username, password, registry string) error {
	if username == "" {
		return fmt.Errorf("Username must be specified")
	}
	if password == "" {
		return fmt.Errorf("Password must be specified")
	}

	err := os.MkdirAll(dockerPath, 0600)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to create %s directory", dockerPath))
	}

	authBytes := []byte(fmt.Sprintf("%s:%s", username, password))
	encodedString := base64.StdEncoding.EncodeToString(authBytes)
	jsonBytes := []byte(fmt.Sprintf(`{"auths": {"%s": {"auth": "%s"}}}`, "https://"+registry, encodedString))
	err = ioutil.WriteFile(dockerConfigPath, jsonBytes, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to create docker config file")
	}
	return nil
}

func setupACRCert(jsonKey string) error {
	err := ioutil.WriteFile(acrCertPath, []byte(jsonKey), 0644)
	if err != nil {
		return errors.Wrap(err, "failed to write ACR certificate")
	}
	err = os.Setenv(certPathEnv, acrCertPath)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to set %s environment variable", certPathEnv))
	}
	return nil
}
