$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
Set-Location -LiteralPath $repo
if (Get-Process -Name 'relaydock','relaydock-dev' -ErrorAction SilentlyContinue) {
    throw 'Close the existing RelayDock instance before this isolated smoke test.'
}
$runKey = Get-ItemProperty -LiteralPath 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -ErrorAction SilentlyContinue
if ($null -ne $runKey.'com.0disoft.relaydock') {
    throw 'An existing RelayDock autostart entry must not be changed by the smoke test.'
}
$state = Join-Path $repo '.tmp/desktop-smoke-20260912'
New-Item -ItemType Directory -Force -Path $state | Out-Null
foreach ($name in @('stop','restart')) {
    $marker = Join-Path $state $name
    if (Test-Path -LiteralPath $marker) { Remove-Item -LiteralPath $marker }
}
$env:ARG_DESKTOP_DATA_DIR = Join-Path $state 'data'
$env:PATH = (Join-Path $env:USERPROFILE 'go/bin') + ';' + $env:PATH
$env:GOWORK = 'off'
$env:GOPROXY = 'off'
$env:GOFLAGS = '-mod=readonly'
$cli = Join-Path $env:USERPROFILE 'go/bin/wails3.exe'
$deadline = [DateTime]::UtcNow.AddMinutes(8)
$child = $null
$cycle = 0
try {
    while ([DateTime]::UtcNow -lt $deadline -and !(Test-Path -LiteralPath (Join-Path $state 'stop'))) {
        if ($null -eq $child) {
            $cycle++
            $child = Start-Process -FilePath $cli -ArgumentList @('dev','--port','19245') -WorkingDirectory $repo -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $state "stdout-$cycle.log") -RedirectStandardError (Join-Path $state "stderr-$cycle.log")
            Write-Output "Smoke cycle $cycle started; supervisor PID=$($child.Id); evidence=$state"
        }
        if ($child.HasExited) { throw "Wails supervisor exited with code $($child.ExitCode); inspect the smoke logs." }
        $restart = Join-Path $state 'restart'
        if (Test-Path -LiteralPath $restart) {
            & taskkill.exe /PID $child.Id /T /F | Out-Null
            $child.WaitForExit(10000) | Out-Null
            $child = $null
            Remove-Item -LiteralPath $restart
        }
        Start-Sleep -Milliseconds 250
    }
} finally {
    if ($null -ne $child -and !$child.HasExited) {
        & taskkill.exe /PID $child.Id /T /F | Out-Null
        $child.WaitForExit(10000) | Out-Null
    }
    Write-Output 'Smoke supervisor and its child processes stopped.'
}
