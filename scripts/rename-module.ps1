param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[A-Za-z0-9._-]+/[A-Za-z0-9._/-]+$')]
    [string]$NewModule
)

$ErrorActionPreference = "Stop"
$OldModule = "github.com/0disoft/relaydock"
$TextExtensions = @(
    ".go", ".mod", ".work", ".sum", ".proto", ".sql", ".ts", ".js", ".svelte",
    ".json", ".yaml", ".yml", ".toml", ".md", ".html", ".css", ".ps1", ".sh",
    ".example", ".gateway", ".control", ".expert", ".outbox", ".webhook-sink"
)
$TextNames = @("Dockerfile", "Caddyfile", "Taskfile.yml", ".env.example", ".gitignore", ".gitattributes")

$files = Get-ChildItem -Recurse -File | Where-Object {
    $_.FullName -notmatch '[\\/](\.git|node_modules|bin|dist|build|\.svelte-kit)[\\/]' -and
    ($TextExtensions -contains $_.Extension -or $TextNames -contains $_.Name)
}

$updated = 0
foreach ($file in $files) {
    $content = [System.IO.File]::ReadAllText($file.FullName)
    if ($content.Contains($OldModule)) {
        [System.IO.File]::WriteAllText($file.FullName, $content.Replace($OldModule, $NewModule), [System.Text.UTF8Encoding]::new($false))
        $updated++
    }
}

Write-Host "Module renamed to $NewModule in $updated text files"
