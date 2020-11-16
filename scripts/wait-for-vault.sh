#!/usr/bin/env sh
# Copyright 2019 Cray Inc.  All Rights Reserved

until $(kubectl -n vault get statefulsets.apps cray-vault >/dev/null 2>&1); do
  echo "Waiting for Vault to get created."
  sleep 1
done

while [ -z "$(kubectl -n vault get statefulsets.apps cray-vault -o jsonpath='{.status.readyReplicas}')" ]; do
  echo "Waiting for Vault to become ready."
  sleep 1
done

echo "Vault is ready!"
