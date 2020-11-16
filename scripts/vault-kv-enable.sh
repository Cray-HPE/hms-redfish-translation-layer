#!/usr/bin/env bash
# Copyright 2020 Hewlett Packard Enterprise Development LP

IFS=','

vault_host_port=$(echo "$VAULT_ADDR" | awk -F[/] '{print $3}')
vault_host=$(echo "$vault_host_port" | cut -d ':' -f 1)
vault_port=$(echo "$vault_host_port" | cut -d ':' -f 2)

echo "Waiting for Vault ($vault_host:$vault_port) to become ready..."

./wait-for.sh "$vault_host":"$vault_port" -- echo 'Vault ready.'

set -ex

vault login "$VAULT_TOKEN"

# Rename the v2 kv-store, so we can create a v1 kv-store named secret
vault secrets move secret/ secret-v2/

read -ra STORE <<<"$KV_STORES"
for store in "${STORE[@]}"; do
  echo "Enabling $store..."
  vault secrets enable -path="$store" kv
done