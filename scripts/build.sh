#!/bin/sh

# force go modules
export GOPATH=""

# disable cgo
export CGO_ENABLED=0

set -e
set -x

# linux
GOOS=linux GOARCH=amd64 go build -o release/linux/amd64/kaniko-gcr    ./cmd/kaniko-gcr
GOOS=linux GOARCH=amd64 go build -o release/linux/amd64/kaniko-acr    ./cmd/kaniko-acr
GOOS=linux GOARCH=amd64 go build -o release/linux/amd64/kaniko-ecr    ./cmd/kaniko-ecr
GOOS=linux GOARCH=amd64 go build -o release/linux/amd64/kaniko-docker ./cmd/kaniko-docker
GOOS=linux GOARCH=amd64 go build -o release/linux/amd64/kaniko-gar    ./cmd/kaniko-gar

GOOS=linux GOARCH=arm64 go build -o release/linux/arm64/kaniko-gcr    ./cmd/kaniko-gcr
GOOS=linux GOARCH=arm64 go build -o release/linux/arm64/kaniko-acr    ./cmd/kaniko-acr
GOOS=linux GOARCH=arm64 go build -o release/linux/arm64/kaniko-ecr    ./cmd/kaniko-ecr
GOOS=linux GOARCH=arm64 go build -o release/linux/arm64/kaniko-docker ./cmd/kaniko-docker
GOOS=linux GOARCH=arm64 go build -o release/linux/arm64/kaniko-gar    ./cmd/kaniko-gar

GOOS=linux GOARCH=arm   go build -o release/linux/arm/kaniko-gcr      ./cmd/kaniko-gcr
GOOS=linux GOARCH=arm   go build -o release/linux/arm/kaniko-acr      ./cmd/kaniko-acr
GOOS=linux GOARCH=arm   go build -o release/linux/arm/kaniko-ecr      ./cmd/kaniko-ecr
GOOS=linux GOARCH=arm   go build -o release/linux/arm/kaniko-docker   ./cmd/kaniko-docker
GOOS=linux GOARCH=arm   go build -o release/linux/arm/kaniko-gar      ./cmd/kaniko-gar
