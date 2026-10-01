#!/bin/sh
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
export GOPATH="${GOPATH:-$project_dir/.cache/gopath}"
export GOMODCACHE="${GOMODCACHE:-$project_dir/.cache/gomod}"
export GOCACHE="${GOCACHE:-$project_dir/.cache/gobuild}"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"
export GOTMPDIR="$project_dir/.cache/tmp"
export TMPDIR="$GOTMPDIR"
mkdir -p "$GOTMPDIR"
cd "$project_dir"
if command -v go >/dev/null 2>&1; then
    exec go "$@"
fi
exec "$project_dir/.tools/go1.26.8/go/bin/go" "$@"
