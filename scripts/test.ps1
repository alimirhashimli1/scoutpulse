<#
.SYNOPSIS
  Runs the test suite across all six Go modules.

.DESCRIPTION
  This repository is a Go workspace, not a single module, so `go test ./...`
  from the root fails with "directory prefix . does not contain modules listed
  in go.work". Each module has to be entered separately. That is what this
  does, plus a summary at the end.

  By default it runs with -short, which skips the suites needing a Docker
  daemon. Those run in CI.

.PARAMETER All
  Include the Docker-backed tests. Needs a running Docker daemon; without one
  they skip themselves after a delay.

.PARAMETER Fresh
  Ignore Go's test cache. Use this when you want to be certain a change was
  actually exercised rather than seeing a "(cached)" result.

.PARAMETER Detailed
  Print each test name instead of one line per package.

.PARAMETER Race
  Enable the race detector, as CI does. Slower.

.PARAMETER Module
  Test one module only, e.g. -Module apps/football-svc

.EXAMPLE
  .\scripts\test.ps1
  .\scripts\test.ps1 -Fresh -Detailed
  .\scripts\test.ps1 -Module apps/football-svc
#>
[CmdletBinding()]
param(
    [switch] $All,
    [switch] $Fresh,
    [switch] $Detailed,
    [switch] $Race,
    [string] $Module
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot

$modules = @(
    'libs/auth',
    'libs/db',
    'libs/platform',
    'apps/identity-svc',
    'apps/football-svc',
    'tests/integration'
)

if ($Module) {
    $trimmed = $Module.TrimEnd('/', '\') -replace '\\', '/'
    if ($modules -notcontains $trimmed) {
        throw "Unknown module '$Module'. Expected one of: $($modules -join ', ')"
    }
    $modules = @($trimmed)
}

# Build the argument list once so every module is run identically.
$goArgs = @('test')
if (-not $All)   { $goArgs += '-short' }
if ($Fresh)      { $goArgs += '-count=1' }
if ($Detailed)   { $goArgs += '-v' }
if ($Race)       { $goArgs += '-race' }
$goArgs += './...'

Write-Host "go $($goArgs -join ' ')" -ForegroundColor DarkGray
if (-not $All) {
    Write-Host "(-short: Docker-backed suites are skipped. Use -All to include them.)" -ForegroundColor DarkGray
}
Write-Host ""

$failed = @()

foreach ($m in $modules) {
    Write-Host $m -ForegroundColor Cyan

    Push-Location (Join-Path $repo ($m -replace '/', '\'))
    try {
        & go @goArgs 2>&1 | ForEach-Object {
            $line = "$_"
            if ($line -match '^ok\s')                  { Write-Host "  $line" -ForegroundColor Green }
            elseif ($line -match '^(FAIL|---\s+FAIL)') { Write-Host "  $line" -ForegroundColor Red }
            elseif ($line -match '\[no test files\]')  { Write-Host "  $line" -ForegroundColor DarkGray }
            else                                       { Write-Host "  $line" }
        }
        if ($LASTEXITCODE -ne 0) { $failed += $m }
    } finally {
        Pop-Location
    }
}

Write-Host ""
Write-Host ("-" * 52) -ForegroundColor DarkGray
if ($failed.Count -eq 0) {
    Write-Host "All $($modules.Count) module(s) passed." -ForegroundColor Green
    exit 0
}

Write-Host "FAILED in: $($failed -join ', ')" -ForegroundColor Red
exit 1
