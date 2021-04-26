package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	kaniko "github.com/drone/drone-kaniko"
)

const (
	accessKeyEnv     string = "AWS_ACCESS_KEY_ID"
	secretKeyEnv     string = "AWS_SECRET_ACCESS_KEY"
	dockerConfigPath string = "/kaniko/.docker/config.json"
)

var (
	version = "unknown"
)

func main() {
	// Load env-file if it exists first
	if env := os.Getenv("PLUGIN_ENV_FILE"); env != "" {
		godotenv.Load(env)
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
		cli.StringSliceFlag{
			Name:     "tags",
			Usage:    "build tags",
			Value:    &cli.StringSlice{"latest"},
			EnvVar:   "PLUGIN_TAGS",
			FilePath: ".tags",
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
			Usage:  "Remote repository that will be used to store cached layers. Cache repo should be present in specified registry. enable-cache needs to be set to use this flag",
			EnvVar: "PLUGIN_CACHE_REPO",
		},
		cli.IntFlag{
			Name:   "cache-ttl",
			Usage:  "Cache timeout in hours. Defaults to two weeks.",
			EnvVar: "PLUGIN_CACHE_TTL",
		},
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func run(c *cli.Context) error {
	err := setupECRAuth(c.String("access-key"), c.String("secret-key"), c.String("registry"))
	if err != nil {
		return err
	}

	plugin := kaniko.Plugin{
		Build: kaniko.Build{
			Dockerfile:   c.String("dockerfile"),
			Context:      c.String("context"),
			Tags:         c.StringSlice("tags"),
			Args:         c.StringSlice("args"),
			Target:       c.String("target"),
			Repo:         fmt.Sprintf("%s/%s", c.String("registry"), c.String("repo")),
			Labels:       c.StringSlice("custom-labels"),
			SnapshotMode: c.String("snapshot-mode"),
			EnableCache:  c.Bool("enable-cache"),
			CacheRepo:    fmt.Sprintf("%s/%s", c.String("registry"), c.String("cache-repo")),
			CacheTTL:     c.Int("cache-ttl"),
		},
	}
	return plugin.Exec()
}

func setupECRAuth(accessKey, secretKey, registry string) error {
	if registry == "" {
		return fmt.Errorf("Registry must be specified")
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

	jsonBytes := []byte(fmt.Sprintf(`{"credStore": "ecr-login", "credHelpers": {"public.ecr.aws": "ecr-login", "%s": "ecr-login"}}`, registry))
	err := ioutil.WriteFile(dockerConfigPath, jsonBytes, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to create docker config file")
	}
	return nil
}
