#!/usr/bin/env sh
set -eu

WAILS_VERSION="v3.0.0-alpha2.119"
TASK_VERSION="v3.51.1"
BUF_VERSION="v1.72.0"
SQLC_VERSION="v1.31.1"

go version
bun --version
go install "github.com/wailsapp/wails/v3/cmd/wails3@${WAILS_VERSION}"
go install "github.com/go-task/task/v3/cmd/task@${TASK_VERSION}"
go install "github.com/bufbuild/buf/cmd/buf@${BUF_VERSION}"
go install "github.com/sqlc-dev/sqlc/cmd/sqlc@${SQLC_VERSION}"

bun install
echo "Bootstrap complete. Run ./scripts/generate.sh next."
