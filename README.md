# drone-kaniko

Drone kaniko plugin uses [kaniko](https://github.com/GoogleContainerTools/kaniko) to build and publish Docker images to a container registry.

Plugin images are published with 1.6.0 as well as 1.9.1 kaniko version from 1.5.1 release tag. `plugins/kaniko:<release-tag>` uses 1.6.0 version while `plugins/kaniko:<release-tag>-kaniko1.9.1` uses 1.9.1 version. Similar convention is used for plugins/kaniko-ecr & plugins/kaniko-gcr images as well.

Run the following script to install git-leaks support to this repo.

```
chmod +x ./git-hooks/install.sh
./git-hooks/install.sh
```

## Build

Build the binaries with the following commands:

```console
export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0
export GO111MODULE=on

go build -v -a -tags netgo -o release/linux/amd64/kaniko-docker ./cmd/kaniko-docker
go build -v -a -tags netgo -o release/linux/amd64/kaniko-gcr ./cmd/kaniko-gcr
go build -v -a -tags netgo -o release/linux/amd64/kaniko-ecr ./cmd/kaniko-ecr
go build -v -a -tags netgo -o release/linux/amd64/kaniko-acr ./cmd/kaniko-acr
```

## Docker

Build the Docker images with the following commands:

```console
docker build \
  --label org.label-schema.build-date=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  --label org.label-schema.vcs-ref=$(git rev-parse --short HEAD) \
  --file docker/docker/Dockerfile.linux.amd64 --tag plugins/kaniko .

docker build \
  --label org.label-schema.build-date=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  --label org.label-schema.vcs-ref=$(git rev-parse --short HEAD) \
  --file docker/acr/Dockerfile.linux.amd64 --tag plugins/kaniko-acr .

docker build \
  --label org.label-schema.build-date=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  --label org.label-schema.vcs-ref=$(git rev-parse --short HEAD) \
  --file docker/gcr/Dockerfile.linux.amd64 --tag plugins/kaniko-gcr .

docker build \
  --label org.label-schema.build-date=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  --label org.label-schema.vcs-ref=$(git rev-parse --short HEAD) \
  --file docker/ecr/Dockerfile.linux.amd64 --tag plugins/kaniko-ecr .
```

## Usage

### Operation Modes

Default Mode (Build and Push):

When neither `no_push` nor `push_only` is provided. Plugin builds and pushes the Docker image to a container registry.

Build-Only Mode (no-push):

When `no_push` is true and `destination_tar_path` is defined.
Plugin performs only the image build operation and saves the resulting image tarball to the specified `destination_tar_path`
It does not push the image to any registry.

Push-Only Mode (push-only):

When `push_only` is true and `source_tar_path` is defined.
Plugin loads an existing image tarball from the specified `source_tar_path`
and pushes the loaded image to a Container Registry.
It skips the build process.

### Mutually Exclusive Inputs

If both `no_push` and `push_only` inputs are provided, the plugin will:

Terminate the operation and
throw an error with the message: "Inputs no-push and push-only cannot be used together. Please define only one."

### Manual Tagging

```console
docker run --rm \
    -e PLUGIN_TAGS=1.2,latest \
    -e PLUGIN_DOCKERFILE=/drone/Dockerfile \
    -e PLUGIN_REPO=foo/bar \
    -e PLUGIN_USERNAME=foo \
    -e PLUGIN_PASSWORD=bar \
    -v $(pwd):/drone \
    -w /drone \
    plugins/kaniko:linux-amd64
```

With expanded tagging enabled, semantic versions can be passed to PLUGIN_TAGS directly for expansion.

**Note**: this feature only works for build labels. Artifact labels are not supported.

```console
docker run --rm \
    -e PLUGIN_TAGS=v1.2.3,latest \
    -e PLUGIN_EXPAND_TAG=true \
    -v $(pwd):/drone \
    -w /drone \
    plugins/kaniko:linux-amd64
```

would both be equivalent to

```
PLUGIN_TAGS=1,1.2,1.2.3,latest
```

This allows for passing `$DRONE_TAG` directly as a tag for repos that use [semver](https://semver.org) tags.

To avoid confusion between repo tags and image tags, `PLUGIN_EXPAND_TAG` also recognizes a semantic version
without the `v` prefix. As such, the following is also equivalent to the above:

```console
docker run --rm \
    -e PLUGIN_TAGS=1.2.3,latest \
    -e PLUGIN_EXPAND_TAG=true \
    -v $(pwd):/drone \
    -w /drone \
    plugins/kaniko:linux-amd64
```

### Auto Tagging

The [auto tag feature](https://plugins.drone.io/drone-plugins/drone-docker) of docker plugin is also supported.

When auto tagging is enabled, if any of the case is matched below, a docker build will be pushed with auto generated tags. Otherwise the docker build will be skipped.

**Note**: this feature only works for build labels. Artifact labels are not supported.

#### Git Tag Push:

```console
docker run --rm \
    -e DRONE_COMMIT_REF=refs/tags/v1.2.3 \
    -e PLUGIN_REPO=foo/bar \
    -e PLUGIN_USERNAME=foo \
    -e PLUGIN_PASSWORD=bar \
    -e PLUGIN_AUTO_TAG=true \
    -v $(pwd):/drone \
    -w /drone \
    plugins/kaniko:linux-amd64
```

Tags to push:

- 1.2.3
- 1.2
- 1

#### Git Commit Push in default branch:

```console
docker run --rm \
    -e DRONE_COMMIT_REF=refs/heads/master \
    -e DRONE_REPO_BRANCH=main \
    -e PLUGIN_REPO=foo/bar \
    -e PLUGIN_USERNAME=foo \
    -e PLUGIN_PASSWORD=bar \
    -e PLUGIN_AUTO_TAG=true \
    -v $(pwd):/drone \
    -w /drone \
    plugins/kaniko:linux-amd64
```

Tags to push:

- latest
