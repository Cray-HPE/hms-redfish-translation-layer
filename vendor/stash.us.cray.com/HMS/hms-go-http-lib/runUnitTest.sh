#!/usr/bin/env bash

# Build the build base image
docker build -t cray/hms-go-http-lib-build-base -f Dockerfile.build-base .

docker build -t cray/hms-go-http-lib-testing -f Dockerfile.testing .
