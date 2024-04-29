package main

import (
	"context"
	"encoding/base64"
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
	ecrpublicv1 "github.com/aws/aws-sdk-go/service/ecrpublic"
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
	dockerConfigPath string = "/kaniko/.docker"
	secretKeyEnv     string = "AWS_SECRET_ACCESS_KEY"
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
			Name:   "docker-registry",
			Usage:  "Docker registry for base image",
			EnvVar: "PLUGIN_DOCKER_REGISTRY,DOCKER_REGISTRY,PLUGIN_BASE_IMAGE_REGISTRY",
		},
		cli.StringFlag{
			Name:   "docker-username",
			Usage:  "Docker username for base image registry",
			EnvVar: "PLUGIN_USERNAME,PLUGIN_BASE_IMAGE_USERNAME,DOCKER_USERNAME",
		},
		cli.StringFlag{
			Name:   "docker-password",
			Usage:  "Docker password for base image registry",
			EnvVar: "PLUGIN_PASSWORD,PLUGIN_BASE_IMAGE_PASSWORD,DOCKER_PASSWORD",
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
		cli.StringFlag{
			Name:   "custom-platform",
			Usage:  "Platform to use for building.",
			EnvVar: "PLUGIN_CUSTOM_PLATFORM",
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
	repo := c.String("repo")
	registry := c.String("registry")
	region := c.String("region")
	noPush := c.Bool("no-push")
	assumeRole := c.String("assume-role")
	externalId := c.String("external-id")

	// setup docker config for azure registry and base image docker registry
	err := setDockerAuth(
		c.String("docker-registry"),
		c.String("docker-username"),
		c.String("docker-password"),
		c.String("access-key"),
		c.String("secret-key"),
		registry,
		assumeRole,
		externalId,
		region,
		noPush,
	)
	if err != nil {
		return errors.Wrap(err, "failed to create docker config")
	}

	// only create repository when pushing and create-repository is true
	if !noPush && c.Bool("create-repository") {
		if err := createRepository(region, repo, registry, assumeRole, externalId); err != nil {
			return err
		}
	}

	if c.IsSet("lifecycle-policy") {
		contents, err := ioutil.ReadFile(c.String("lifecycle-policy"))
		if err != nil {
			logrus.Fatal(err)
		}
		if err := uploadLifeCyclePolicy(region, repo, string(contents), assumeRole, externalId); err != nil {
			logrus.Fatal(fmt.Sprintf("error uploading ECR lifecycle policy: %v", err))
		}
	}

	if c.IsSet("repository-policy") {
		contents, err := ioutil.ReadFile(c.String("repository-policy"))
		if err != nil {
			logrus.Fatal(err)
		}
		if err := uploadRepositoryPolicy(region, repo, registry, string(contents), assumeRole, externalId); err != nil {
			logrus.Fatal(fmt.Sprintf("error uploading ECR lifecycle policy: %v", err))
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
			Target:                      c.String("target"),
			Repo:                        fmt.Sprintf("%s/%s", c.String("registry"), c.String("repo")),
			Mirrors:                     c.StringSlice("registry-mirrors"),
			Labels:                      c.StringSlice("custom-labels"),
			SnapshotMode:                c.String("snapshot-mode"),
			EnableCache:                 c.Bool("enable-cache"),
			CacheRepo:                   fmt.Sprintf("%s/%s", c.String("registry"), c.String("cache-repo")),
			CacheTTL:                    c.Int("cache-ttl"),
			DigestFile:                  defaultDigestFile,
			NoPush:                      noPush,
			Verbosity:                   c.String("verbosity"),
			Platform:                    c.String("platform"),
			SkipUnusedStages:            c.Bool("skip-unused-stages"),
			CacheDir:                    c.String("cache-dir"),
			CacheCopyLayers:             c.Bool("cache-copy-layers"),
			CacheRunLayers:              c.Bool("cache-run-layers"),
			Cleanup:                     c.Bool("cleanup"),
			ContextSubPath:              c.String("context-sub-path"),
			CustomPlatform:              c.String("custom-platform"),
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
			ImageFSExtractRetry:         c.Int("image-fs-extract-retry"),
			ImageDownloadRetry:          c.Int("image-download-retry"),
		},
		Artifact: kaniko.Artifact{
			Tags:         c.StringSlice("tags"),
			Repo:         c.String("repo"),
			Registry:     c.String("registry"),
			ArtifactFile: c.String("artifact-file"),
			RegistryType: artifact.ECR,
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
	return plugin.Exec()
}

func setDockerAuth(dockerRegistry, dockerUsername, dockerPassword, accessKey, secretKey,
	registry, assumeRole, externalId, region string, noPush bool) error {
	dockerConfig := docker.NewConfig()
	credentials := []docker.RegistryCredentials{}
	// set docker credentials for base image registry
	if dockerRegistry != "" {
		pullFromRegistryCreds := docker.RegistryCredentials{
			Registry: dockerRegistry,
			Username: dockerUsername,
			Password: dockerPassword,
		}
		credentials = append(credentials, pullFromRegistryCreds)
	}

	if assumeRole != "" {
		var err error
		username, password, registry, err := getAssumeRoleCreds(region, assumeRole, externalId, "")
		if err != nil {
			return err
		}
		pushToRegistryCreds := docker.RegistryCredentials{
			Registry: registry,
			Username: username,
			Password: password,
		}
		credentials = append(credentials, pushToRegistryCreds)

	} else if !noPush || accessKey != "" {
		// only setup auth when pushing or credentials are defined
		if registry == "" {
			return fmt.Errorf("registry must be specified")
		}

		// If IAM role is used, access key & secret key are not required
		if accessKey != "" && secretKey != "" {
			err := os.Setenv(accessKeyEnv, accessKey)
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("failed to set %s environment variable", accessKeyEnv))
			}

			err = os.Setenv(secretKeyEnv, secretKey)
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("failed to set %s environment variable", secretKeyEnv))
			}
		}

		// kaniko-executor >=1.8.0 does not require additional cred helper logic for ECR,
		// as it discovers ECR repositories automatically and acts accordingly.
		if isKanikoVersionBelowOneDotEight(os.Getenv(kanikoVersionEnv)) {
			dockerConfig.SetCredHelper(ecrPublicDomain, "ecr-login")
			dockerConfig.SetCredHelper(registry, "ecr-login")
		}
	}
	return dockerConfig.CreateDockerConfig(credentials, dockerConfigPath)
}

func createRepository(region, repo, registry, assumeRole, externalId string) error {
	if registry == "" {
		return fmt.Errorf("registry must be specified")
	}

	if repo == "" {
		return fmt.Errorf("repo must be specified")
	}

	var createErr error

	if assumeRole != "" {
		if isRegistryPublic(registry) {
			_, createErr = getAssumeRoleEcrPublicSvc(region, assumeRole, externalId).CreateRepository(&ecrpublicv1.CreateRepositoryInput{RepositoryName: &repo})
		} else {
			_, createErr = getAssumeRoleEcrSvc(region, assumeRole, externalId).CreateRepository(&ecrv1.CreateRepositoryInput{RepositoryName: &repo})
		}
	} else {
		cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
		if err != nil {
			return errors.Wrap(err, "failed to load aws config")
		}
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
	}

	var apiError smithy.APIError
	if errors.As(createErr, &apiError) && apiError.ErrorCode() != "RepositoryAlreadyExistsException" {
		return errors.Wrap(createErr, "failed to create repository")
	}

	return nil
}

func uploadLifeCyclePolicy(region, repo, lifecyclePolicy, assumeRole, externalId string) (err error) {
	if assumeRole != "" {
		input := &ecrv1.PutLifecyclePolicyInput{
			LifecyclePolicyText: aws.String(lifecyclePolicy),
			RepositoryName:      aws.String(repo),
		}
		_, err = getAssumeRoleEcrSvc(region, assumeRole, externalId).PutLifecyclePolicy(input)
	} else {
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
	}

	return err
}

func uploadRepositoryPolicy(region, repo, registry, repositoryPolicy, assumeRole, externalId string) (err error) {
	if assumeRole != "" {
		if isRegistryPublic(registry) {
			input := &ecrpublicv1.SetRepositoryPolicyInput{
				PolicyText:     aws.String(repositoryPolicy),
				RepositoryName: aws.String(repo),
			}
			_, err = getAssumeRoleEcrPublicSvc(region, assumeRole, externalId).SetRepositoryPolicy(input)
		} else {
			input := &ecrv1.SetRepositoryPolicyInput{
				PolicyText:     aws.String(repositoryPolicy),
				RepositoryName: aws.String(repo),
			}
			_, err = getAssumeRoleEcrSvc(region, assumeRole, externalId).SetRepositoryPolicy(input)
		}
	} else {
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
		return "", "", "", errors.Wrap(err, "failed to get ECR auth: no basic auth credentials")
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

func getAssumeRoleEcrSvc(region, assumeRole, externalId string) *ecrv1.ECR {
	sess, err := session.NewSession(&awsv1.Config{Region: &region})
	if err != nil {
		logrus.Fatal(err, "failed to create aws session")
	}

	return ecrv1.New(sess, &awsv1.Config{
		Credentials: stscreds.NewCredentials(sess, assumeRole, func(p *stscreds.AssumeRoleProvider) {
			if externalId != "" {
				p.ExternalID = &externalId
			}
		}),
	})
}

func getAssumeRoleEcrPublicSvc(region, assumeRole, externalId string) *ecrpublicv1.ECRPublic {
	sess, err := session.NewSession(&awsv1.Config{Region: &region})
	if err != nil {
		logrus.Fatal(err, "failed to create aws session")
	}

	return ecrpublicv1.New(sess, &awsv1.Config{
		Credentials: stscreds.NewCredentials(sess, assumeRole, func(p *stscreds.AssumeRoleProvider) {
			if externalId != "" {
				p.ExternalID = &externalId
			}
		}),
	})
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
