#!/bin/bash -e
if [[ -z "${GOBIN}" ]]; then
    export GOBIN=`pwd`/bin
    export PATH=$GOBIN:$PATH
fi
