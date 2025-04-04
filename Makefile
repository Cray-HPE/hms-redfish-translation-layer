# MIT License
#
# (C) Copyright [2021-2022,2025] Hewlett Packard Enterprise Development LP
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
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.  IN NO EVENT SHALL
# THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
# OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
# ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
# OTHER DEALINGS IN THE SOFTWARE.

# Service
NAME ?= hms-redfish-translation-service
VERSION ?= $(shell cat .version)

NAME_VAULT ?= vault-kv-enabler

all: image image-pprof unittest image-vault-kv-enabler snyk

image:
	docker build ${NO_CACHE} --pull ${DOCKER_ARGS} --tag '${NAME}:${VERSION}' -f Dockerfile .

image-pprof:
	docker build ${NO_CACHE} --pull ${DOCKER_ARGS} --tag '${NAME}-pprof:${VERSION}' -f Dockerfile.pprof .

unittest:
	./runUnitTest.sh

image-vault-kv-enabler:
	docker build ${NO_CACHE} --pull ${DOCKER_ARGS_VAULT} --tag '${NAME_VAULT}:${VERSION}' -f vault-kv-enabler.dockerfile .

snyk:
	./runSnyk.sh
