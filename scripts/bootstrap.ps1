$ErrorActionPreference = "Stop"

$WailsVersion = "v3.0.0-alpha2.119"
$TaskVersion = "v3.51.1"
$BufVersion = "v1.72.0"
$SqlcVersion = "v1.31.1"

go version
bun --version
go install "github.com/wailsapp/wails/v3/cmd/wails3@$WailsVersion"
go install "github.com/go-task/task/v3/cmd/task@$TaskVersion"
go install "github.com/bufbuild/buf/cmd/buf@$BufVersion"
go install "github.com/sqlc-dev/sqlc/cmd/sqlc@$SqlcVersion"

bun install
Write-Host "Bootstrap complete. Run ./scripts/generate.ps1 next."
