#!/bin/sh

# force go modules
export GOPATH=""

# disable cgo
export CGO_ENABLED=0

# force linux amd64 platform
export GOOS=linux
export GOARCH=amd64 

set -e
set -x

# build the binary
go build -o release/linux/amd64/kaniko-gcr    ./cmd/kaniko-gcr
go build -o release/linux/amd64/kaniko-gar    ./cmd/kaniko-gar
go build -o release/linux/amd64/kaniko-ecr    ./cmd/kaniko-ecr
go build -o release/linux/amd64/kaniko-docker ./cmd/kaniko-docker

# build the docker image
docker build -f docker/gcr/Dockerfile.linux.amd64    -t plugins/kaniko-gcr .
docker build -f docker/gar/Dockerfile.linux.amd64    -t plugins/kaniko-gar .
docker build -f docker/ecr/Dockerfile.linux.amd64    -t plugins/kaniko-ecr .
docker build -f docker/docker/Dockerfile.linux.amd64 -t plugins/kaniko .
