#!/usr/bin/env sh
# MIT License
#
# (C) Copyright [2019, 2021] Hewlett Packard Enterprise Development LP
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

if ! [[ -x "$REDIS_CLI" ]]; then
    echo "Redis CLI not found!"
    exit 1
fi

REDIS_LOG="/rts-logs/schema_load.out"
root_uri="/redfish/v1"

service_root_keyspace="/redfish/v1"
rackpdus_keyspace="${service_root_keyspace}/PowerEquipment/RackPDUs"
accont_service_keyspace="${service_root_keyspace}/AccountService"
roles_keyspace="${accont_service_keyspace}/Roles"
accounts_keyspace="${accont_service_keyspace}/Accounts"

: "${REDIS_HOSTNAME:?Need to set REDIS_HOSTNAME}"
: "${REDIS_PORT:?Need to set REDIS_PORT}"
REDIS_CMD="${REDIS_CLI} -h ${REDIS_HOSTNAME} -p ${REDIS_PORT}"

SET_CMD="${REDIS_CMD}      SET"
SADD_CMD="${REDIS_CMD}     SADD"
SCARD_CMD="${REDIS_CMD}    SCARD"
RPUSH_CMD="${REDIS_CMD}    RPUSH"
LRANGE_CMD="${REDIS_CMD}   LRANGE"
LLEN_CMD="${REDIS_CMD}     LLEN"
FLUSHALL_CMD="${REDIS_CMD} FLUSHALL"

# Global Functions

SET_OBJ_KEY() {
    local keyspace="${1}"
    local property="${2}"
    ${SADD_CMD} ${keyspace} ${property}  2>&1 >> ${REDIS_LOG}
    echo "${keyspace}/${property}"
}

SET_VAL_KEY() {
    local keyspace="${1}"
    local property="${2}"
    local value="${3}"
    local key="${keyspace}/${property}"
    ${SET_CMD} ${key} "${value}" 2>&1 >> ${REDIS_LOG}
    ${SADD_CMD} ${keyspace} ${property} 2>&1 >> ${REDIS_LOG}
}

SET_ARR_KEY() {
    local keyspace="${1}"
    local property="${2}"
    members_keyspace="${keyspace}/${property}"
    index=$(${LLEN_CMD} ${members_keyspace} | cut -f2)
    if [[ "${index}" == "0" ]] ; then
	    ${SADD_CMD} ${keyspace} ${property} 2>&1 >> ${REDIS_LOG}
    fi
    ${RPUSH_CMD} ${members_keyspace} ${index} >> ${REDIS_LOG}
    echo "${members_keyspace}/${index}"
}

ADD_DISPATCHER() {
    local keyspace="${1}"
    local property="${2}"
    local key="${keyspace}/${property}"
    ${SADD_CMD} ${keyspace} ${property} 2>&1 >> ${REDIS_LOG}
}

# Setup Functions

init_service_root() {
    keyspace=$(SET_OBJ_KEY "/redfish" "v1")
    SET_VAL_KEY ${keyspace} "Name"            "Service Root"
    SET_VAL_KEY ${keyspace} "Description"     "The Redfish ServiceRoot"

    # These are prototype only and should be inserted from schema tree
    ADD_DISPATCHER ${keyspace} "@odata.id"
    ADD_DISPATCHER ${keyspace} "@odata.type"
    ADD_DISPATCHER ${keyspace} "@odata.context"
}

init_json_schemas_collection() {
    keyspace=$(SET_OBJ_KEY ${service_root_keyspace} "JsonSchemas")
    SET_VAL_KEY ${keyspace} "Name"            "Json Schemas Collection"
    SET_VAL_KEY ${keyspace} "Description"     "A collection of all JSON Schemas"

    # These are prototype only and should be inserted from schema tree
    ADD_DISPATCHER ${keyspace} "@odata.id"
    ADD_DISPATCHER ${keyspace} "@odata.type"
    ADD_DISPATCHER ${keyspace} "@odata.context"
}

init_powerequipment() {
    keyspace=$(SET_OBJ_KEY ${service_root_keyspace} "PowerEquipment")
    SET_VAL_KEY ${keyspace} "Name"            "DCIM Energy Equipment"

    # These are prototype only and should be inserted from schema tree
    ADD_DISPATCHER ${keyspace} "@odata.id"
    ADD_DISPATCHER ${keyspace} "@odata.type"
    ADD_DISPATCHER ${keyspace} "@odata.context"

    # Put everything under RackPDUs
    SET_OBJ_KEY ${keyspace} "RackPDUs"
}


# Account Service Initialization

init_account_service() {
    # Account service
    keyspace=$(SET_OBJ_KEY ${service_root_keyspace} "AccountService")

    ADD_DISPATCHER ${keyspace} "@odata.id"
    ADD_DISPATCHER ${keyspace} "@odata.type"
    ADD_DISPATCHER ${keyspace} "@odata.context"

    SET_VAL_KEY ${keyspace} AccountLockoutDuration "0"
    SET_VAL_KEY ${keyspace} AccountLockoutCounterResetAfter "0"
    SET_VAL_KEY ${keyspace} AccountLockoutThreshold "0"

    SET_VAL_KEY ${keyspace} MinPasswordLength "8"
    SET_VAL_KEY ${keyspace} MaxPasswordLength "64"
    SET_VAL_KEY ${keyspace} ServiceEnabled "true"
}

init_roles_collection() {
    keyspace=$(SET_OBJ_KEY ${accont_service_keyspace} "Roles")
    SET_VAL_KEY ${keyspace} "Description"     "A collection of all Role instances on this host"
    SET_VAL_KEY ${keyspace} "Name"            "Role Collection"

    # These are prototype only and should be inserted from schema tree
    ADD_DISPATCHER ${keyspace} "@odata.id"
    ADD_DISPATCHER ${keyspace} "@odata.type"
    ADD_DISPATCHER ${keyspace} "@odata.context"
}

init_role() {
    local instance_name="${1}"
    local predefined_role="${2}"
    # Setup role
    instance_keyspace="${roles_keyspace}/${instance_name}"
    ADD_DISPATCHER ${instance_keyspace} "@odata.id"
    ADD_DISPATCHER ${instance_keyspace} "@odata.type"
    ADD_DISPATCHER ${instance_keyspace} "@odata.context"
    SET_VAL_KEY ${instance_keyspace} "Name" "${instance_name}"
    SET_VAL_KEY ${instance_keyspace} "Id" "${instance_name}"
    SET_VAL_KEY ${instance_keyspace} "RoleId" "${instance_name}"
    SET_VAL_KEY ${instance_keyspace} "IsPredefined" "${predefined_role}"

    # TODO wrap this in a function at somepoint. This sets all assigned
    # privileges at once
    ${SADD_CMD} ${instance_keyspace} "AssignedPrivileges" 2>&1 >> ${REDIS_LOG}
    ${RPUSH_CMD} "${instance_keyspace}/AssignedPrivileges" "${@:3}"

    # Add role to collection
    members_keyspace=$(SET_ARR_KEY ${roles_keyspace} "Members")
    SET_VAL_KEY ${members_keyspace} "@odata.id" "${instance_keyspace}"
}

init_accounts_collection() {
    keyspace=$(SET_OBJ_KEY ${accont_service_keyspace} "Accounts")
    SET_VAL_KEY ${keyspace} "Description"     "A collection of all Account instances on this host"
    SET_VAL_KEY ${keyspace} "Name"            "Account Collection"

    # These are prototype only and should be inserted from schema tree
    ADD_DISPATCHER ${keyspace} "@odata.id"  
    ADD_DISPATCHER ${keyspace} "@odata.type"  
    ADD_DISPATCHER ${keyspace} "@odata.context"  
}



# Main

set -ex

rm -f ${REDIS_LOG}
${FLUSHALL_CMD}

init_json_schemas_collection


init_account_service
init_roles_collection
init_role "Administrator" "true" "Login" "ConfigureManager" "ConfigureUsers" "ConfigureComponents" "ConfigureSelf"
init_role "Operator" "true" "Login" "ConfigureComponents" "ConfigureSelf"
init_role "ReadOnly" "true" "Login" "ConfigureSelf"
init_accounts_collection

exit 0
