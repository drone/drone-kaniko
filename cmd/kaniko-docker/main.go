package main

import (
	"encoding/base64"
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
	// Docker config file path
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
			Usage:  "docker registry",
			Value:  "https://index.docker.io/v1/",
			EnvVar: "PLUGIN_REGISTRY",
		},
		cli.StringFlag{
			Name:   "username",
			Usage:  "docker username",
			EnvVar: "PLUGIN_USERNAME",
		},
		cli.StringFlag{
			Name:   "password",
			Usage:  "docker password",
			EnvVar: "PLUGIN_PASSWORD",
		},
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func run(c *cli.Context) error {
	err := createDockerCfgFile(c.String("username"), c.String("password"), c.String("registry"))
	if err != nil {
		return err
	}

	plugin := kaniko.Plugin{
		Build: kaniko.Build{
			Dockerfile: c.String("dockerfile"),
			Context:    c.String("context"),
			Tags:       c.StringSlice("tags"),
			Args:       c.StringSlice("args"),
			Target:     c.String("target"),
			Repo:       c.String("repo"),
			Labels:     c.StringSlice("custom-labels"),
		},
	}
	return plugin.Exec()
}

// Create the docker config file for authentication
func createDockerCfgFile(username, password, registry string) error {
	if username == "" {
		return fmt.Errorf("Username is not set")
	}
	if password == "" {
		return fmt.Errorf("Password is not set")
	}
	if registry == "" {
		return fmt.Errorf("Registry is not set")
	}

	authBytes := []byte(fmt.Sprintf("%s:%s", username, password))
	encodedString := base64.StdEncoding.EncodeToString(authBytes)
	jsonBytes := []byte(fmt.Sprintf(`{"auths": {"%s": {"auth": "%s"}}}`, registry, encodedString))
	err := ioutil.WriteFile(dockerConfigPath, jsonBytes, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to create docker config file")
	}
	return nil
}
