#! /usr/bin/env bash
#
# MIT License
#
# (C) Copyright 2022 Hewlett Packard Enterprise Development LP
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
#

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
