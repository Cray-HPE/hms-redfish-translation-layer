# Copyright 2019-2020 Hewlett Packard Enterprise Development LP

# Dockerfile for building the HMS Redfish Translation Service (RTS).

FROM dtr.dev.cray.com/baseos/golang:1.14-alpine3.12 AS builder

# Copy all the necessary files to the image.
COPY cmd $GOPATH/src/stash.us.cray.com/HMS/hms-redfish-translation-service/cmd
COPY internal $GOPATH/src/stash.us.cray.com/HMS/hms-redfish-translation-service/internal
COPY vendor $GOPATH/src/stash.us.cray.com/HMS/hms-redfish-translation-service/vendor

RUN set -ex \
    && go build -v -i stash.us.cray.com/HMS/hms-redfish-translation-service/cmd/rfdispatcher \
    && go build -v -i stash.us.cray.com/HMS/hms-redfish-translation-service/cmd/vault_loader

### Final Stage ###

FROM dtr.dev.cray.com/baseos/alpine:3.12
LABEL maintainer="Cray, Inc."
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

# Refish Eventing
ENV COLLECTOR_URL http://hms-hmcollector

# JAWS Backend helper
ENV JAWS_POLLING_ENABLED true
ENV JAWS_POLLING_INTERVAL 10
ENV JAWS_POLLING_WORKERS 30

RUN set -ex \
    && apk update \
    && apk add redis curl

COPY --from=builder /go/rfdispatcher /usr/local/bin
COPY --from=builder /go/vault_loader /usr/local/bin

COPY scripts/wait-for.sh /
COPY docker-entrypoint.sh /
COPY contrib ${CONTRIB_DIR_PREFIX}contrib
COPY configs ${CONFIGS_DIR_PREFIX}configs
COPY scripts /scripts
COPY .version /

ENTRYPOINT ["/docker-entrypoint.sh"]

CMD ["rfdispatcher"]
