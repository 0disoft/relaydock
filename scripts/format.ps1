$ErrorActionPreference = "Stop"

Get-ChildItem -Recurse -Filter *.go |
    Where-Object {
        $_.FullName -notmatch '[\\/]gen[\\/]' -and
        $_.FullName -notmatch '[\\/]\.git[\\/]'
    } |
    ForEach-Object { gofmt -w $_.FullName }

Write-Host "Go sources formatted."
