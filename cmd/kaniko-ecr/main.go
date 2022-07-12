package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecrpublic"
	awsv1 "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	ecrv1 "github.com/aws/aws-sdk-go/service/ecr"
	"github.com/aws/smithy-go"
	"github.com/hashicorp/go-version"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	kaniko "github.com/drone/drone-kaniko"
	"github.com/drone/drone-kaniko/pkg/artifact"
	"github.com/drone/drone-kaniko/pkg/docker"
)

const (
	accessKeyEnv     string = "AWS_ACCESS_KEY_ID"
	secretKeyEnv     string = "AWS_SECRET_ACCESS_KEY"
	dockerConfigPath string = "/kaniko/.docker/config.json"
	ecrPublicDomain  string = "public.ecr.aws"
	kanikoVersionEnv string = "KANIKO_VERSION"

	oneDotEightVersion string = "1.8.0"
	defaultDigestFile  string = "/kaniko/digest-file"
)

var (
	pluginVersion = "unknown"
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
			Usage:  "create ECR repository",
			EnvVar: "PLUGIN_CREATE_REPOSITORY",
		},
		cli.StringFlag{
			Name:   "region",
			Usage:  "AWS region",
			Value:  "us-east-1",
			EnvVar: "PLUGIN_REGION",
		},
		cli.StringSliceFlag{
			Name:   "custom-labels",
			Usage:  "additional k=v labels",
			EnvVar: "PLUGIN_CUSTOM_LABELS",
		},
		cli.StringFlag{
			Name:   "registry",
			Usage:  "ECR registry",
			EnvVar: "PLUGIN_REGISTRY",
		},
		cli.StringSliceFlag{
			Name:   "registry-mirrors",
			Usage:  "docker registry mirrors",
			EnvVar: "PLUGIN_REGISTRY_MIRRORS",
		},
		cli.StringFlag{
			Name:   "access-key",
			Usage:  "ECR access key",
			EnvVar: "PLUGIN_ACCESS_KEY",
		},
		cli.StringFlag{
			Name:   "secret-key",
			Usage:  "ECR secret key",
			EnvVar: "PLUGIN_SECRET_KEY",
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
	repo := c.String("repo")
	registry := c.String("registry")
	region := c.String("region")
	noPush := c.Bool("no-push")

	dockerConfig, err := createDockerConfig(
		c.String("docker-username"),
		c.String("docker-password"),
		c.String("access-key"),
		c.String("secret-key"),
		registry,
		c.String("assume-role"),
		c.String("external-id"),
		region,
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

	// only create repository when pushing and create-repository is true
	if !noPush && c.Bool("create-repository") {
		if err := createRepository(region, repo, registry); err != nil {
			return err
		}
	}

	if c.IsSet("lifecycle-policy") {
		contents, err := ioutil.ReadFile(c.String("lifecycle-policy"))
		if err != nil {
			logrus.Fatal(err)
		}
		if err := uploadLifeCyclePolicy(region, repo, string(contents)); err != nil {
			logrus.Fatal(fmt.Sprintf("error uploading ECR lifecycle policy: %v", err))
		}
	}

	if c.IsSet("repository-policy") {
		contents, err := ioutil.ReadFile(c.String("repository-policy"))
		if err != nil {
			logrus.Fatal(err)
		}
		if err := uploadRepositoryPolicy(region, repo, registry, string(contents)); err != nil {
			logrus.Fatal(fmt.Sprintf("error uploading ECR lifecycle policy: %v", err))
		}
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
			RegistryType: artifact.ECR,
		},
	}
	return plugin.Exec()
}

func createDockerConfig(dockerUsername, dockerPassword, accessKey, secretKey,
	registry, assumeRole, externalId, region string, noPush bool) (*docker.Config, error) {
	dockerConfig := docker.NewConfig()

	if dockerUsername != "" {
		dockerConfig.SetAuth(docker.RegistryV1, dockerUsername, dockerPassword)
	}

	if assumeRole != "" {
		var err error
		username, password, registry, err := getAssumeRoleCreds(region, assumeRole, externalId, "")
		if err != nil {
			return nil, err
		}
		dockerConfig.SetAuth(registry, username, password)
	} else if !noPush || accessKey != "" {
		// only setup auth when pushing or credentials are defined
		if registry == "" {
			return nil, fmt.Errorf("registry must be specified")
		}

		// If IAM role is used, access key & secret key are not required
		if accessKey != "" && secretKey != "" {
			err := os.Setenv(accessKeyEnv, accessKey)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("failed to set %s environment variable", accessKeyEnv))
			}

			err = os.Setenv(secretKeyEnv, secretKey)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("failed to set %s environment variable", secretKeyEnv))
			}
		}

		// kaniko-executor >=1.8.0 does not require additional cred helper logic for ECR,
		// as it discovers ECR repositories automatically and acts accordingly.
		if isKanikoVersionBelowOneDotEight(os.Getenv(kanikoVersionEnv)) {
			dockerConfig.SetCredHelper(ecrPublicDomain, "ecr-login")
			dockerConfig.SetCredHelper(registry, "ecr-login")
		}
	}

	return dockerConfig, nil
}

func createRepository(region, repo, registry string) error {
	if registry == "" {
		return fmt.Errorf("registry must be specified")
	}

	if repo == "" {
		return fmt.Errorf("repo must be specified")
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return errors.Wrap(err, "failed to load aws config")
	}

	var createErr error

	//create public repo
	//if registry string starts with public domain (ex: public.ecr.aws/example-registry)
	if isRegistryPublic(registry) {
		svc := ecrpublic.NewFromConfig(cfg)
		_, createErr = svc.CreateRepository(context.TODO(), &ecrpublic.CreateRepositoryInput{RepositoryName: &repo})
		//create private repo
	} else {
		svc := ecr.NewFromConfig(cfg)
		_, createErr = svc.CreateRepository(context.TODO(), &ecr.CreateRepositoryInput{RepositoryName: &repo})
	}

	var apiError smithy.APIError
	if errors.As(createErr, &apiError) && apiError.ErrorCode() != "RepositoryAlreadyExistsException" {
		return errors.Wrap(createErr, "failed to create repository")
	}

	return nil
}

func uploadLifeCyclePolicy(region, repo, lifecyclePolicy string) (err error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return errors.Wrap(err, "failed to load aws config")
	}

	svc := ecr.NewFromConfig(cfg)

	input := &ecr.PutLifecyclePolicyInput{
		LifecyclePolicyText: aws.String(lifecyclePolicy),
		RepositoryName:      aws.String(repo),
	}
	_, err = svc.PutLifecyclePolicy(context.TODO(), input)

	return err
}

func uploadRepositoryPolicy(region, repo, registry, repositoryPolicy string) (err error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return errors.Wrap(err, "failed to load aws config")
	}

	if isRegistryPublic(registry) {
		svc := ecrpublic.NewFromConfig(cfg)

		input := &ecrpublic.SetRepositoryPolicyInput{
			PolicyText:     aws.String(repositoryPolicy),
			RepositoryName: aws.String(repo),
		}
		_, err = svc.SetRepositoryPolicy(context.TODO(), input)
	} else {

		svc := ecr.NewFromConfig(cfg)

		input := &ecr.SetRepositoryPolicyInput{
			PolicyText:     aws.String(repositoryPolicy),
			RepositoryName: aws.String(repo),
		}
		_, err = svc.SetRepositoryPolicy(context.TODO(), input)
	}

	return err
}

func getAssumeRoleCreds(region, roleArn, externalId, roleSessionName string) (string, string, string, error) {
	sess, err := session.NewSession(&awsv1.Config{Region: &region})
	if err != nil {
		return "", "", "", errors.Wrap(err, "failed to create aws session")
	}

	svc := ecrv1.New(sess, &awsv1.Config{
		Credentials: stscreds.NewCredentials(sess, roleArn, func(p *stscreds.AssumeRoleProvider) {
			if externalId != "" {
				p.ExternalID = &externalId
			}
		}),
	})

	username, password, registry, err := getAuthInfo(svc)
	if err != nil {
		return "", "", "", errors.Wrap(err, "failed to get ECR auth")
	}
	return username, password, registry, nil
}

func getAuthInfo(svc *ecrv1.ECR) (username, password, registry string, err error) {
	var result *ecrv1.GetAuthorizationTokenOutput
	var decoded []byte

	result, err = svc.GetAuthorizationToken(&ecrv1.GetAuthorizationTokenInput{})
	if err != nil {
		return
	}

	auth := result.AuthorizationData[0]
	token := *auth.AuthorizationToken
	decoded, err = base64.StdEncoding.DecodeString(token)
	if err != nil {
		return
	}

	registry = strings.TrimPrefix(*auth.ProxyEndpoint, "https://")
	creds := strings.Split(string(decoded), ":")
	username = creds[0]
	password = creds[1]
	return
}

func isRegistryPublic(registry string) bool {
	return strings.HasPrefix(registry, ecrPublicDomain)
}

func isKanikoVersionBelowOneDotEight(v string) bool {
	currVer, err := version.NewVersion(v)
	if err != nil {
		return true
	}
	oneEightVer, err := version.NewVersion(oneDotEightVersion)
	if err != nil {
		return true
	}

	return currVer.LessThan(oneEightVer)
}
