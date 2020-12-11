# Copyright 2020 Hewlett Packard Enterprise Development LP

FROM dtr.dev.cray.com/library/vault
LABEL maintainer="Cray, Inc."

RUN apk add --no-cache \
    bash

# Vault
ENV VAULT_ADDR http://localhost:8200

# Default KV Store
ENV KV_STORES hms-creds

COPY scripts/wait-for.sh /
COPY scripts /scripts
COPY .version /

ENTRYPOINT ["/scripts/vault-kv-enable.sh"]
