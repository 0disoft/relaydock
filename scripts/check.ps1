$ErrorActionPreference = "Stop"

$Unformatted = Get-ChildItem -Recurse -Filter *.go |
    Where-Object {
        $_.FullName -notmatch '[\\/]gen[\\/]' -and
        $_.FullName -notmatch '[\\/]\.git[\\/]'
    } |
    ForEach-Object { gofmt -l $_.FullName }

if ($Unformatted) {
    Write-Error ("Go files require formatting:`n" + ($Unformatted -join "`n"))
}

go run ./cmd/releasepack audit --root .

go test -count=1 ./cmd/... ./db ./internal/... ./tests/...
go test -race -count=1 ./internal/... ./tests/...
go vet ./cmd/... ./db ./internal/... ./tests/...

bun --cwd frontend run test
bun --cwd frontend run check
bun --cwd web/control-console run check
