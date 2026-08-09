#!/usr/bin/env sh
set -eu

buf generate
sqlc generate
wails3 generate bindings
find . -name '*.go' -type f -print0 | xargs -0 gofmt -w
