#!/usr/bin/env sh
set -eu

unformatted="$(find . -name '*.go' -type f -not -path './gen/*' -not -path './.git/*' -print0 | xargs -0 gofmt -l)"
if [ -n "$unformatted" ]; then
  echo "Go files require formatting:" >&2
  echo "$unformatted" >&2
  exit 1
fi

go run ./cmd/releasepack audit --root .

go test -count=1 ./cmd/... ./db ./internal/... ./tests/...
go test -race -count=1 ./internal/... ./tests/...
go vet ./cmd/... ./db ./internal/... ./tests/...

bun run --cwd frontend test
bun run --cwd frontend check
bun run --cwd web/control-console check
