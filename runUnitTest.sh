#!/usr/bin/env bash
# Copyright 2020 Hewlett Packard Enterprise Development LP

# Run the RTS unit tests in an automoated fashion

docker build --rm --no-cache -f Dockerfile.testing .
test_result=$?

if [[ $test_result -ne 0 ]]; then
    echo "Unit tests FAILED!"
else
    echo "Unit tests PASSED!"
fi

exit ${test_result}