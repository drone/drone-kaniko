#!/bin/sh

# force go modules
export GOPATH=""

# disable cgo
export CGO_ENABLED=0

set -e
set -x

# linux
GOOS=linux GOARCH=amd64 go build -o release/linux/amd64/kaniko-gcr
GOOS=linux GOARCH=amd64 go build -o release/linux/amd64/kaniko-ecr
GOOS=linux GOARCH=amd64 go build -o release/linux/amd64/kaniko-docker

GOOS=linux GOARCH=arm64 go build -o release/linux/arm64/kaniko-gcr
GOOS=linux GOARCH=arm64 go build -o release/linux/arm64/kaniko-ecr
GOOS=linux GOARCH=arm64 go build -o release/linux/arm64/kaniko-docker

GOOS=linux GOARCH=arm   go build -o release/linux/arm/kaniko-gcr
GOOS=linux GOARCH=arm   go build -o release/linux/arm/kaniko-ecr
GOOS=linux GOARCH=arm   go build -o release/linux/arm/kaniko-docker
