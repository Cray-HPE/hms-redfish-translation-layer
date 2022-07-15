#! /usr/bin/env bash

set -xe


export VAULT_TOKEN=hms
export VAULT_ADDR=http://localhost:8200
export CRAY_VAULT_JWT_FILE=configs/token
export CRAY_VAULT_ROLE_FILE=configs/namespace
export CRAY_VAULT_AUTH_PATH=auth/token/create
export JAWS_VAULT_KEYPATH=pdu-creds

go run github.com/Cray-HPE/hms-redfish-translation-service/cmd/pdu_loader
