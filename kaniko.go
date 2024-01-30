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
		TarPath          string   // Set this flag to save the image as a tarball at path
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

const kanikoArgsEnabled = "DRONE_KANIKO_ADDIONAL_ARGS_ENABLED"

// Excluded variables
var excludeList = []string{"PLUGIN_PIPELINE", "PLUGIN_USERNAME", "PLUGIN_PASSWORD", "PLUGIN_TAGS", "PLUGIN_REGISTRY", "PLUGIN_ARTIFACT_FILE", "PLUGIN_REPO", "PLUGIN_BUILD_ARGS"}

// Allowed variables
var allowList = []string{"PLUGIN_BUILD_ARG", "PLUGIN_CACHE", "PLUGIN_CACHE_DIR", "PLUGIN_CACHE_REPO", "PLUGIN_CACHE_COPY_LAYERS", "PLUGIN_CACHE_RUN_LAYERS", "PLUGIN_CACHE_TTL", "PLUGIN_CLEANUP", "PLUGIN_COMPRESSED_CACHING", "PLUGIN_CONTEXT_SUB_PATH", "PLUGIN_CUSTOM_PLATFORM", "PLUGIN_DIGEST_FILE", "PLUGIN_DOCKERFILE", "PLUGIN_FORCE", "PLUGIN_GIT", "PLUGIN_IMAGE_NAME_WITH_DIGEST_FILE", "PLUGIN_IMAGE_NAME_TAG_WITH_DIGEST_FILE", "PLUGIN_INSECURE", "PLUGIN_INSECURE_PULL", "PLUGIN_INSECURE_REGISTRY", "PLUGIN_LABEL", "PLUGIN_LOG_FORMAT", "PLUGIN_LOG_TIMESTAMP", "PLUGIN_NO_PUSH", "PLUGIN_OCI_LAYOUT_PATH", "PLUGIN_PUSH_RETRY", "PLUGIN_REGISTRY_CERTIFICATE", "PLUGIN_REGISTRY_CLIENT_CERT", "PLUGIN_REGISTRY_MIRROR", "PLUGIN_SKIP_DEFAULT_REGISTRY_FALLBACK", "PLUGIN_REPRODUCIBLE", "PLUGIN_SINGLE_SNAPSHOT", "PLUGIN_SKIP_TLS_VERIFY", "PLUGIN_SKIP_PUSH_PERMISSION_CHECK", "PLUGIN_SKIP_TLS_VERIFY_PULL", "PLUGIN_SKIP_TLS_VERIFY_REGISTRY", "PLUGIN_SKIP_UNUSED_STAGES", "PLUGIN_SNAPSHOT_MODE", "PLUGIN_TAR_PATH", "PLUGIN_TARGET", "PLUGIN_USE_NEW_RUN", "PLUGIN_VERBOSITY", "PLUGIN_IGNORE_VAR_RUN", "PLUGIN_IGNORE_PATH", "PLUGIN_IMAGE_FS_EXTRACT_RETRY", "PLUGIN_IMAGE_DOWNLOAD_RETRY"}

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

	// Set the destination repository only when we push or save to tarball
	if !p.Build.NoPush || p.Build.TarPath != "" {
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

	if p.Build.TarPath != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--tar-path=%s", p.Build.TarPath))
	}

	//Read all PLUGIN_ env vars if FF is enabled
	//parse them such that PLUGIN_ENV_ARG is set to the value of --env-arg
	//Add the value of --env-arg to cmdArgs if it does not exist
	argsEnabled, ok := os.LookupEnv(kanikoArgsEnabled)
	if ok {
		fmt.Fprintf(os.Stdout, "%s env is set with value: %s ", kanikoArgsEnabled, argsEnabled)
	}
	if argsEnabled == "true" {
		cmdArgs = getPluginEnvVars(cmdArgs)
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
		if err = output.WritePluginOutputFile(p.Output.OutputFile, getDigest(p.Build.DigestFile)); err != nil {
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

func getPluginEnvVars(cmdArgs []string) []string {
	envVars := os.Environ()

	// Iterate through environment variables
	for _, envVar := range envVars {
		// Check if the variable starts with PLUGIN_
		if strings.HasPrefix(envVar, "PLUGIN_") && !contains(excludeList, envVar) && contains(allowList, envVar) {
			// Split the variable into key and value
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := parts[0]
			value := parts[1]

			// Trim the "PLUGIN_" prefix
			flagName := strings.TrimPrefix(key, "PLUGIN_")

			// Replace underscores with hyphens and convert to lowercase
			flagName = strings.ReplaceAll(flagName, "_", "-")
			flagName = strings.ToLower(flagName)

			// Format the flag name with "--" prefix
			flag := "--" + flagName

			// Check if the flag already exists in cmdArgs
			exists := false
			for _, arg := range cmdArgs {
				if strings.HasPrefix(arg, flag) {
					exists = true
					break
				}
			}

			// If the flag does not exist, add it to cmdArgs
			if !exists {
				if value == "" {
					cmdArgs = append(cmdArgs, flag)
				} else {
					cmdArgs = append(cmdArgs, fmt.Sprintf("%s=%s", flag, value))
				}
			}
		}
	}
	return cmdArgs
}

// Function to check if a string is in a slice
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if strings.HasPrefix(str, s) {
			return true
		}
	}
	return false
}
