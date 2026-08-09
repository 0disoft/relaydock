#!/usr/bin/env sh
set -eu

find . -name '*.go' -type f -not -path './gen/*' -not -path './.git/*' -print0 | xargs -0 gofmt -w

echo "Go sources formatted."
