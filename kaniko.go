package kaniko

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
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
	}

	// Plugin defines the Docker plugin parameters.
	Plugin struct {
		Build Build // Docker build configuration
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

	cmd := exec.Command("/kaniko/executor", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	trace(cmd)

	err := cmd.Run()
	return err
}

// trace writes each command to stdout with the command wrapped in an xml
// tag so that it can be extracted and displayed in the logs.
func trace(cmd *exec.Cmd) {
	fmt.Fprintf(os.Stdout, "+ %s\n", strings.Join(cmd.Args, " "))
}
