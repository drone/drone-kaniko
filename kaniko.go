package kaniko

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/drone/drone-kaniko/pkg/artifact"
	"github.com/drone/drone-kaniko/pkg/output"
	"github.com/drone/drone-kaniko/pkg/tagger"
	"golang.org/x/mod/semver"
)

type (
	// Build defines Docker build parameters.
	Build struct {
		DroneCommitRef   string   // Drone git commit reference
		DroneRepoBranch  string   // Drone repo branch
		Dockerfile       string   // Docker build Dockerfile
		Context          string   // Docker build context
		Tags             []string // Docker build tags
		AutoTag          bool     // Set this to auto detect tags from git commits and semver-tagged labels
		AutoTagSuffix    string   // Suffix to append to the auto detect tags
		ExpandTag        bool     // Set this to expand the `Tags` into semver-tagged labels
		Args             []string // Docker build args
		Target           string   // Docker build target
		Repo             string   // Docker build repository
		Mirrors          []string // Docker repository mirrors
		Labels           []string // Label map
		SkipTlsVerify    bool     // Docker skip tls certificate verify for registry
		SnapshotMode     string   // Kaniko snapshot mode
		EnableCache      bool     // Whether to enable kaniko cache
		CacheRepo        string   // Remote repository that will be used to store cached layers
		CacheTTL         int      // Cache timeout in hours
		DigestFile       string   // Digest file location
		NoPush           bool     // Set this flag if you only want to build the image, without pushing to a registry
		Verbosity        string   // Log level
		Platform         string   // Allows to build with another default platform than the host, similarly to docker build --platform
		SkipUnusedStages bool     // Build only used stages
	}

	// Artifact defines content of artifact file
	Artifact struct {
		Tags         []string                  // Docker artifact tags
		Repo         string                    // Docker artifact repository
		Registry     string                    // Docker artifact registry
		RegistryType artifact.RegistryTypeEnum // Rocker artifact registry type
		ArtifactFile string                    // Artifact file location
	}

	// Output defines content of output file
	Output struct {
		OutputFile string // File where plugin output are saved
	}

	// Plugin defines the Docker plugin parameters.
	Plugin struct {
		Build    Build    // Docker build configuration
		Artifact Artifact // Artifact file content
		Output   Output   // Output file content
	}
)

// labelsForTag returns the labels to use for the given tag, subject to the value of ExpandTag.
//
// Build information (e.g. +linux_amd64) is carried through to all labels.
// Pre-release information (e.g. -rc1) suppresses major and major+minor auto-labels.
func (b Build) labelsForTag(tag string) (labels []string) {
	// We strip "v" off of the beginning of semantic versions, as they are not used in docker tags
	const VersionPrefix = "v"

	// Semantic Versions don't allow underscores, so replace them with dashes.
	//   https://semver.org/
	semverTag := strings.ReplaceAll(tag, "_", "-")

	// Allow tags of the form "1.2.3" as well as "v1.2.3" to avoid confusion.
	if withV := VersionPrefix + semverTag; !semver.IsValid(semverTag) && semver.IsValid(withV) {
		semverTag = withV
	}

	// Pass through tags if expand-tag is not set, or if the tag is not a semantic version
	if !b.ExpandTag || !semver.IsValid(semverTag) {
		return []string{tag}
	}
	tag = semverTag

	// If the version is pre-release, only the full release should be tagged, not the major/minor versions.
	if semver.Prerelease(tag) != "" {
		return []string{
			strings.TrimPrefix(tag, VersionPrefix),
		}
	}

	// tagFor carries any build information from the semantic version through to major and minor tags.
	labelFor := func(base string) string {
		return strings.TrimPrefix(base, VersionPrefix) + semver.Build(tag)
	}
	return []string{
		labelFor(semver.Major(tag)),
		labelFor(semver.MajorMinor(tag)),
		labelFor(semver.Canonical(tag)),
	}
}

// Returns the auto detected tags. See the AutoTag section of
// https://plugins.drone.io/drone-plugins/drone-docker/ for more info.
func (b Build) AutoTags() (tags []string, err error) {
	if len(b.Tags) > 1 || len(b.Tags) == 1 && b.Tags[0] != "latest" {
		err = fmt.Errorf("The auto-tag flag does not work with user provided tags %s", b.Tags)
		return
	}
	// We have tried the best to prevent enabling auto-tag and passing in
	// user specified at the same time. Starts to auto detect tags.
	// Note: passing in a "latest" tag with auto-tag enabled won't trigger the
	// early returns above, because we cannot tell if the tag is provided by
	// the default value or by the users.
	commitRef := b.DroneCommitRef
	if !tagger.UseAutoTag(commitRef, b.DroneRepoBranch) {
		err = fmt.Errorf("Could not auto detect the tag. Skipping automated docker build for commit %s", commitRef)
		return
	}
	tags, err = tagger.AutoTagsSuffix(commitRef, b.AutoTagSuffix)
	if err != nil {
		err = fmt.Errorf("Invalid semantic version when auto detecting the tag. Skipping automated docker build for %s.", commitRef)
	}
	return
}

// Exec executes the plugin step
func (p Plugin) Exec() error {
	if !p.Build.NoPush && p.Build.Repo == "" {
		return fmt.Errorf("repository name to publish image must be specified")
	}

	if _, err := os.Stat(p.Build.Dockerfile); os.IsNotExist(err) {
		return fmt.Errorf("dockerfile does not exist at path: %s", p.Build.Dockerfile)
	}

	var tags = p.Build.Tags
	if p.Build.AutoTag && p.Build.ExpandTag {
		return fmt.Errorf("The auto-tag flag conflicts with the expand-tag flag")
	}
	if p.Build.AutoTag {
		var err error
		tags, err = p.Build.AutoTags()
		if err != nil {
			return err
		}
	}

	cmdArgs := []string{
		fmt.Sprintf("--dockerfile=%s", p.Build.Dockerfile),
		fmt.Sprintf("--context=dir://%s", p.Build.Context),
	}

	// Set the destination repository
	if !p.Build.NoPush {
		for _, tag := range tags {
			for _, label := range p.Build.labelsForTag(tag) {
				cmdArgs = append(cmdArgs, fmt.Sprintf("--destination=%s:%s", p.Build.Repo, label))
			}
		}
	}
	// Set the build arguments
	for _, arg := range p.Build.Args {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--build-arg=%s", arg))
	}
	// Set the labels
	for _, label := range p.Build.Labels {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--label=%s", label))
	}
	// Set repository mirrors
	for _, mirror := range p.Build.Mirrors {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--registry-mirror=%s", mirror))
	}
	if p.Build.Target != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--target=%s", p.Build.Target))
	}

	if p.Build.SkipTlsVerify {
		cmdArgs = append(cmdArgs, "--skip-tls-verify=true")
	}

	if p.Build.SnapshotMode != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--snapshotMode=%s", p.Build.SnapshotMode))
	}

	if p.Build.EnableCache {
		cmdArgs = append(cmdArgs, "--cache=true")

		if p.Build.CacheRepo != "" {
			cmdArgs = append(cmdArgs, fmt.Sprintf("--cache-repo=%s", p.Build.CacheRepo))
		}
	}

	if p.Build.CacheTTL != 0 {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--cache-ttl=%dh", p.Build.CacheTTL))
	}

	if p.Build.DigestFile != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--digest-file=%s", p.Build.DigestFile))
	}

	if p.Build.NoPush {
		cmdArgs = append(cmdArgs, "--no-push")
	}

	if p.Build.Verbosity != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--verbosity=%s", p.Build.Verbosity))
	}

	if p.Build.Platform != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--customPlatform=%s", p.Build.Platform))
	}

	if p.Build.SkipUnusedStages {
		cmdArgs = append(cmdArgs, "--skip-unused-stages")
	}

	cmd := exec.Command("/kaniko/executor", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	trace(cmd)

	err := cmd.Run()
	if err != nil {
		return err
	}

	if p.Build.DigestFile != "" && p.Artifact.ArtifactFile != "" {
		err = artifact.WritePluginArtifactFile(p.Artifact.RegistryType, p.Artifact.ArtifactFile, p.Artifact.Registry, p.Artifact.Repo, getDigest(p.Build.DigestFile), p.Artifact.Tags)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to write plugin artifact file at path: %s with error: %s\n", p.Artifact.ArtifactFile, err)
		}
	}

	if p.Output.OutputFile != "" {
		if err = output.WritePluginOutputFile(p.Output.OutputFile, p.Build.Repo, getDigest(p.Build.DigestFile), p.Build.Tags); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write plugin output file at path: %s with error: %s\n", p.Output.OutputFile, err)
		}
	}

	return nil
}

func getDigest(digestFile string) string {
	content, err := ioutil.ReadFile(digestFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read digest file contents at path: %s with error: %s\n", digestFile, err)
	}
	return string(content)
}

// trace writes each command to stdout with the command wrapped in an xml
// tag so that it can be extracted and displayed in the logs.
func trace(cmd *exec.Cmd) {
	fmt.Fprintf(os.Stdout, "+ %s\n", strings.Join(cmd.Args, " "))
}
