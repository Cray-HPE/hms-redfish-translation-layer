#! /usr/bin/env bash

set -xe

export SCHEMA_VERSION=2019.1
export PRIVILEGE_REGISTRY_VERSION=1.0.4
export SCRIPT_DIR_PREFIX=/tmp/redfish
export REDIS_HOSTNAME=localhost
export REDIS_PORT=6379
export VAULT_ADDR=http://localhost:8200
export HTTPS_CERT=configs/rts.crt
export HTTPS_KEY=configs/rts.key
export HSM_URL=http://localhost:27779
export PERIODIC_SLEEP=60
export COLLECTOR_URL=http://localhost:8080


export LOG_LEVEL=INFO
export VAULT_TOKEN=hms
export CRAY_VAULT_JWT_FILE=configs/token
export CRAY_VAULT_ROLE_FILE=configs/namespace
export CRAY_VAULT_AUTH_PATH=auth/token/create
export BACKEND_HELPER=Mock

export CERTIFICATE_SERVICE_ENABLED=true
export CERTIFICATE_VAULT_KEYPATH=pdu-creds/certs

# The following password is not used in production anywhere!!!
export MOCKBACKEND_ROOT_PASSWORD=root_password


redis-cli FLUSHALL
docker-compose -f docker-compose.devel.yaml up redis-prep 
go run github.com/Cray-HPE/hms-redfish-translation-service/cmd/rfdispatcher
