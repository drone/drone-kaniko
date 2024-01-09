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
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	kaniko "github.com/drone/drone-kaniko"
	"github.com/drone/drone-kaniko/pkg/artifact"
	"github.com/drone/drone-kaniko/pkg/docker"
)

const (
	dockerPath         string = "/kaniko/.docker"
	clientIdEnv        string = "AZURE_CLIENT_ID"
	clientSecretKeyEnv string = "AZURE_CLIENT_SECRET"
	tenantKeyEnv       string = "AZURE_TENANT_ID"
	certPathEnv        string = "AZURE_CLIENT_CERTIFICATE_PATH"
	dockerConfigPath   string = "/kaniko/.docker"
	defaultDigestFile  string = "/kaniko/digest-file"
	finalUrl           string = "https://portal.azure.com/#view/Microsoft_Azure_ContainerRegistries/TagMetadataBlade/registryId/"
)

var (
	ACRCertPath   = "/kaniko/acr-cert.pem"
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
			Usage:  "Azure client certificate encoded in base64 format",
			EnvVar: "CLIENT_CERTIFICATE",
		},
		cli.StringFlag{
			Name:   "tenant-id",
			Usage:  "Azure Tenant Id",
			EnvVar: "TENANT_ID",
		},
		cli.StringFlag{
			Name:   "subscription-id",
			Usage:  "Azure Subscription Id",
			EnvVar: "SUBSCRIPTION_ID",
		},
		cli.StringFlag{
			Name:   "client-id",
			Usage:  "Azure Client Id",
			EnvVar: "CLIENT_ID",
		},
		cli.StringFlag{
			Name:   "single-snapshot",
			Usage:  "Takes a single snapshot of the filesystem at the end of the build, only that will be appended to the base image",
			EnvVar: "PLUGIN_SINGLE_SNAPSHOT",
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

	publicUrl, err := setupAuth(
		c.String("tenant-id"),
		c.String("client-id"),
		c.String("client-cert"),
		c.String("client-secret"),
		c.String("subscription-id"),
		registry,
		noPush,
	)
	if err != nil {
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
			Repo:             c.String("repo"),
			Mirrors:          c.StringSlice("registry-mirrors"),
			Labels:           c.StringSlice("custom-labels"),
			SingleSnapshot:   c.Bool("single-snapshot"),
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
			Registry:     publicUrl, // this is public url on which the artifact can be seen
			ArtifactFile: c.String("artifact-file"),
			RegistryType: artifact.Docker,
		},
	}
	return plugin.Exec()
}

func setupAuth(tenantId, clientId, cert,
	clientSecret, subscriptionId, registry string, noPush bool) (string, error) {
	if registry == "" {
		return "", fmt.Errorf("registry must be specified")
	}

	if noPush {
		return "", nil
	}

	// case of client secret or cert based auth
	if clientId != "" {
		// only setup auth when pushing or credentials are defined

		token, publicUrl, err := getACRToken(subscriptionId, tenantId, clientId, clientSecret, cert, registry)
		if err != nil {
			return "", errors.Wrap(err, "failed to fetch ACR Token")
		}
		err = docker.CreateDockerCfgFile(username, token, registry, dockerConfigPath)
		if err != nil {
			return "", errors.Wrap(err, "failed to create docker config")
		}
		return publicUrl, nil
	} else {
		return "", fmt.Errorf("managed authentication is not supported")
	}
}

func getACRToken(subscriptionId, tenantId, clientId, clientSecret, cert, registry string) (string, string, error) {
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
	burl := "https://management.azure.com/subscriptions/" +
		subscriptionId + "/resources?$filter=resourceType%20eq%20'Microsoft.ContainerRegistry/registries'%20and%20name%20eq%20'" +
		registry + "'&api-version=2021-04-01&$select=id"

	method := "GET"
	client := &http.Client{}
	req, err := http.NewRequest(method, burl, nil)
	if err != nil {
		fmt.Println(err)
		return "", errors.Wrap(err, "failed to create request for getting container registry setting")
	}

	req.Header.Add("Authorization", "Bearer "+token)
	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return "", errors.Wrap(err, "failed to send request for getting container registry setting")
	}
	defer res.Body.Close()

	var response strct
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return "", errors.Wrap(err, "failed to send request for getting container registry setting")
	}
	return finalUrl + encodeParam(response.Value[0].ID), nil
}

func encodeParam(s string) string {
	return url.QueryEscape(s)
}

type strct struct {
	Value []struct {
		ID string `json:"id"`
	} `json:"value"`
}
