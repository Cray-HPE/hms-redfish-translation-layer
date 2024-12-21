# MIT License
#
# (C) Copyright [2019-2022,2024-2025] Hewlett Packard Enterprise Development LP
#
# Permission is hereby granted, free of charge, to any person obtaining a
# copy of this software and associated documentation files (the "Software"),
# to deal in the Software without restriction, including without limitation
# the rights to use, copy, modify, merge, publish, distribute, sublicense,
# and/or sell copies of the Software, and to permit persons to whom the
# Software is furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included
# in all copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
# THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
# OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
# ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
# OTHER DEALINGS IN THE SOFTWARE.

# Dockerfile for building the HMS Redfish Translation Service (RTS).

FROM artifactory.algol60.net/docker.io/library/golang:1.23-alpine AS builder

RUN go env -w GO111MODULE=auto

# Copy all the necessary files to the image.
COPY cmd $GOPATH/src/github.com/Cray-HPE/hms-redfish-translation-service/cmd
COPY internal $GOPATH/src/github.com/Cray-HPE/hms-redfish-translation-service/internal
COPY vendor $GOPATH/src/github.com/Cray-HPE/hms-redfish-translation-service/vendor

# Set profiling to disabled by default
ARG ENABLE_PPROF=false

# Conditionally build with the pprof tag if profiling is enabled
RUN if [ "$ENABLE_PPROF" = "true" ]; then \
	set -ex \
	    && go build -v -tags pprof github.com/Cray-HPE/hms-redfish-translation-service/cmd/rfdispatcher \
	    && go build -v -tags pprof github.com/Cray-HPE/hms-redfish-translation-service/cmd/vault_loader
    else \
	set -ex \
	    && go build -v github.com/Cray-HPE/hms-redfish-translation-service/cmd/rfdispatcher \
	    && go build -v github.com/Cray-HPE/hms-redfish-translation-service/cmd/vault_loader
    fi

### Final Stage ###

FROM artifactory.algol60.net/docker.io/alpine:3.15
LABEL maintainer="Hewlett Packard Enterprise"
EXPOSE 8082
STOPSIGNAL SIGTERM

# DMTF
ENV SCHEMA_VERSION 2019.1
ENV PRIVILEGE_REGISTRY_VERSION 1.0.4

# Runtime
ENV LOG_LEVEL INFO
ENV CONTRIB_DIR_PREFIX /
ENV SCRIPT_DIR_PREFIX /scripts
ENV RTS_DNS_PROVIDER none

# Redis
ENV REDIS_HOSTNAME localhost
ENV REDIS_PORT 6379

# Vault
ENV VAULT_ADDR http://localhost:8200
ENV HMS_VAULT_KEYPATH hms-creds
ENV JAWS_VAULT_KEYPATH pdu-creds

# HSM
ENV HSM_URL http://localhost:27779

# HSM
ENV SLS_URL http://localhost:8376

# Refish Eventing
ENV COLLECTOR_URL http://hms-hmcollector

# JAWS Backend helper
ENV JAWS_POLLING_ENABLED true
ENV JAWS_POLLING_INTERVAL 10
ENV JAWS_POLLING_WORKERS 30

RUN set -ex \
    && apk -U upgrade \
    && apk add redis curl \
    && mkdir /rts-logs \
    && chown 65534:65534 /rts-logs

COPY --from=builder /go/rfdispatcher /usr/local/bin
COPY --from=builder /go/vault_loader /usr/local/bin

COPY scripts/wait-for.sh /
COPY docker-entrypoint.sh /
COPY contrib ${CONTRIB_DIR_PREFIX}contrib
COPY configs ${CONFIGS_DIR_PREFIX}configs
COPY scripts /scripts
COPY .version /

# nobody 65534:65534
USER 65534:65534

ENTRYPOINT ["/docker-entrypoint.sh"]

CMD ["rfdispatcher"]
