#!/bin/bash -e
if [[ -z "${GOBIN}" ]]; then
    export GOPATH=~/gopath
    export GOBIN=`pwd`/bin
    export PATH=$GOBIN:$PATH
fi
