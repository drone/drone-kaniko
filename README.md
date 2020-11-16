# drone-kaniko

Drone kaniko plugin uses [kaniko](https://github.com/GoogleContainerTools/kaniko) to build and publish Docker images to a container registry.

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
```

## Docker

Build the Docker images with the following commands:

```console
docker build \
  --label org.label-schema.build-date=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  --label org.label-schema.vcs-ref=$(git rev-parse --short HEAD) \
  --file docker/docker/Dockerfile.linux.amd64 --tag plugins/kaniko-docker .

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

> Notice: Be aware that the Docker plugin currently requires privileged capabilities, otherwise the integrated Docker daemon is not able to start.

```console
docker run --rm \
  -e PLUGIN_TAG=latest \
  -e PLUGIN_REPO=octocat/hello-world \
  -v $(pwd):$(pwd) \
  -w $(pwd) \
  plugins/kaniko-docker
```
