package kaniko

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/drone/drone-kaniko/cmd/artifact"
)

type (
	// Build defines Docker build parameters.
	Build struct {
		Dockerfile    string   // Docker build Dockerfile
		Context       string   // Docker build context
		Tags          []string // Docker build tags
		Args          []string // Docker build args
		Target        string   // Docker build target
		Repo          string   // Docker build repository
		Labels        []string // Label map
		SkipTlsVerify bool     // Docker skip tls certificate verify for registry
		SnapshotMode  string   // Kaniko snapshot mode
		EnableCache   bool     // Whether to enable kaniko cache
		CacheRepo     string   // Remote repository that will be used to store cached layers
		CacheTTL      int      // Cache timeout in hours
		DigestFile    string   // Digest file location
	}
	// Artifact defines content of artifact file
	Artifact struct {
		Tags         []string // Docker artifact tags
		Repo         string   // Docker artifact repository
		Registry     string   // Docker artifact registry
		ArtifactFile string   // Artifact file location

	}

	// Plugin defines the Docker plugin parameters.
	Plugin struct {
		Build    Build    // Docker build configuration
		Artifact Artifact // Artifact file content
	}
)

// Exec executes the plugin step
func (p Plugin) Exec() error {
	if p.Build.Repo == "" {
		return fmt.Errorf("repository name to publish image must be specified")
	}

	if _, err := os.Stat(p.Build.Dockerfile); os.IsNotExist(err) {
		return fmt.Errorf("dockerfile does not exist at path: %s", p.Build.Dockerfile)
	}

	cmdArgs := []string{
		fmt.Sprintf("--dockerfile=%s", p.Build.Dockerfile),
		fmt.Sprintf("--context=dir://%s", p.Build.Context),
	}

	// Set the destination repository
	for _, tag := range p.Build.Tags {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--destination=%s:%s", p.Build.Repo, tag))
	}
	// Set the build arguments
	for _, arg := range p.Build.Args {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--build-arg=%s", arg))
	}
	// Set the labels
	for _, label := range p.Build.Labels {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--label=%s", label))
	}

	if p.Build.Target != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--target=%s", p.Build.Target))
	}

	if p.Build.SkipTlsVerify {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--skip-tls-verify=true"))
	}

	if p.Build.SnapshotMode != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--snapshotMode=%s", p.Build.SnapshotMode))
	}

	if p.Build.EnableCache == true {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--cache=true"))
	}

	if p.Build.CacheRepo != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--cache-repo=%s", p.Build.CacheRepo))
	}

	if p.Build.CacheTTL != 0 {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--cache-ttl=%d", p.Build.CacheTTL))
	}

	if p.Build.DigestFile != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--digest-file=%s", p.Build.DigestFile))
	}

	cmd := exec.Command("/kaniko/executor", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	trace(cmd)

	err := cmd.Run()
	if err != nil {
		return err
	}

	if p.Build.DigestFile != "" {
		content, err := ioutil.ReadFile(p.Build.DigestFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
		}
		err = artifact.WritePluginArtifactFile(artifact.Docker, p.Artifact.ArtifactFile, p.Artifact.Registry, p.Artifact.Repo, string(content), p.Artifact.Tags)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
		}
	}

	return nil
}

// trace writes each command to stdout with the command wrapped in an xml
// tag so that it can be extracted and displayed in the logs.
func trace(cmd *exec.Cmd) {
	fmt.Fprintf(os.Stdout, "+ %s\n", strings.Join(cmd.Args, " "))
}
