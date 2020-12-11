# Copyright 2019-2020 Hewlett Packard Enterprise Development LP

# Dockerfile for building HMS go http library code. Note that
# this image can't be run as these are just packages in this repo.

# Build base just has the packages installed we need.
FROM dtr.dev.cray.com/baseos/golang:1.14-alpine3.12 AS build-base

RUN set -ex \
    && apk update \
    && apk add build-base

# Copy the files in for the next stages to use.
FROM build-base AS base

COPY . $GOPATH/src/stash.us.cray.com/HMS/hms-go-http-lib
COPY vendor $GOPATH/src/stash.us.cray.com/HMS/hms-go-http-lib/vendor

# Now we can build.
FROM base

RUN set -ex \
    && go build -v stash.us.cray.com/HMS/hms-go-http-lib/./...
