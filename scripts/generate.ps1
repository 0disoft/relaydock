$ErrorActionPreference = "Stop"

buf generate
sqlc generate
wails3 generate bindings

Get-ChildItem -Recurse -Filter *.go | ForEach-Object { gofmt -w $_.FullName }
Write-Host "Generated protobuf, sqlc, and Wails bindings."
