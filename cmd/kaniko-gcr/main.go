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
	// GCR JSON key file path
	gcrKeyPath string = "/kaniko/gcr.json"
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
	app.Name = "kaniko gcr plugin"
	app.Usage = "kaniko gcr plugin"
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
			Usage:  "gcr repository",
			EnvVar: "PLUGIN_REPO",
		},
		cli.StringSliceFlag{
			Name:   "custom-labels",
			Usage:  "additional k=v labels",
			EnvVar: "PLUGIN_CUSTOM_LABELS",
		},
		cli.StringFlag{
			Name:   "registry",
			Usage:  "gcr registry",
			Value:  "gcr.io",
			EnvVar: "PLUGIN_REGISTRY",
		},
		cli.StringFlag{
			Name:   "json-key",
			Usage:  "docker username",
			EnvVar: "PLUGIN_JSON_KEY",
		},
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func run(c *cli.Context) error {
	err := setupGCRAuth(c.String("json-key"))
	if err != nil {
		return err
	}

	if c.String("repo") == "" {
		return fmt.Errorf("repo must be specified")
	}

	plugin := kaniko.Plugin{
		Build: kaniko.Build{
			Dockerfile: c.String("dockerfile"),
			Context:    c.String("context"),
			Tags:       c.StringSlice("tags"),
			Args:       c.StringSlice("args"),
			Target:     c.String("target"),
			Repo:       fmt.Sprintf("%s/%s", c.String("registry"), c.String("repo")),
			Labels:     c.StringSlice("custom-labels"),
		},
	}
	return plugin.Exec()
}

// Create the docker config file for authentication
func setupGCRAuth(jsonKey string) error {
	if jsonKey == "" {
		return fmt.Errorf("GCR JSON key must be specified")
	}

	err := ioutil.WriteFile(gcrKeyPath, []byte(jsonKey), 0644)
	if err != nil {
		return errors.Wrap(err, "failed to write GCR JSON key")
	}

	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", gcrKeyPath)
	return nil
}
