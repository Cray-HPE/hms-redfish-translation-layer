#!/usr/bin/env sh
# MIT License
#
# (C) Copyright [2021] Hewlett Packard Enterprise Development LP
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

REDIS_CLI=$(command -v redis-cli)
REDIS_CMD="${REDIS_CLI} -h ${REDIS_HOSTNAME} -p ${REDIS_PORT}"

set -e

setup_redis() {
  # Make sure redis is around before we go any further
  until ${REDIS_CMD} ping >/dev/null; do
    echo "Waiting for Redis..."
    sleep 1
  done
  echo "Redis is alive and well."

  # This method is likely temporary, but for now we need to populate redis if it's not already.
  redis_keys="$(${REDIS_CMD} --no-raw KEYS "*")"
  if [ "$redis_keys" = "(empty list or set)" ]; then
    echo "Redis database empty...populating..."
    /scripts/redfish-pdu-schema-load.sh 2>&1 | tee schema_load.log
    result=$?

    if [ $result -ne 0 ]; then
      return 1
    fi
  else
    echo "Redis contains keys, not touching."
  fi

  return 0
}

if [ "$1" = 'rfdispatcher' ]; then
  setup_redis
elif [ "$1" = 'setup_redis' ]; then
  setup_redis
  exit $?
elif [ "$1" = 'vault_loader' ]; then
  vault_host_port=$(echo "$VAULT_ADDR" | awk -F[/] '{print $3}')
  vault_host=$(echo "$vault_host_port" | cut -d ':' -f 1)
  vault_port=$(echo "$vault_host_port" | cut -d ':' -f 2)

  echo "Waiting for Vault ($vault_host:$vault_port) to become ready..."

  ./wait-for.sh "$vault_host":"$vault_port" -- echo 'Vault ready.'
fi

exec "$@"
