<#
.SYNOPSIS
  Runs one ScoutPulse service against the local Postgres, without Docker.

.DESCRIPTION
  Reads the JWT keys from .env and sets the environment each service expects,
  then runs it in the foreground. Ctrl-C stops it.

  Note there is no gateway in this mode, so services are reached on their own
  ports (identity 8080, football 8081) rather than through :8000. Paths are the
  service's own -- /api/v1/... with no /api/identity or /api/football prefix.

.EXAMPLE
  .\scripts\dev-run.ps1 identity
  .\scripts\dev-run.ps1 football
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('identity', 'football')]
    [string] $Service,

    [string] $PgHost = 'localhost',
    [int]    $PgPort = 5432
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $repo '.env'

if (-not (Test-Path $envFile)) {
    throw ".env not found. Run .\scripts\dev-setup.ps1 first."
}

# Pull just the two JWT values out of .env. Everything else is set below, so
# the file's other contents (including unrelated secrets) are left alone.
function Get-EnvValue([string] $name) {
    $line = Select-String -Path $envFile -Pattern "^$name=" | Select-Object -First 1
    if (-not $line) { return $null }
    $raw = $line.Line.Substring($name.Length + 1)
    # Strip surrounding quotes and turn the escaped newlines back into real ones.
    $raw = $raw.Trim()
    if ($raw.StartsWith('"') -and $raw.EndsWith('"')) {
        $raw = $raw.Substring(1, $raw.Length - 2)
    }
    return $raw.Replace('\n', "`n")
}

$privateKey = Get-EnvValue 'JWT_PRIVATE_KEY'
$publicKey  = Get-EnvValue 'JWT_PUBLIC_KEY'

if (-not $privateKey -or -not $publicKey) {
    throw "JWT keys missing from .env. Run .\scripts\dev-setup.ps1 first."
}

# Browser origin for the future frontend. There is no default any more: an
# unset value denies every cross-origin request rather than allowing all.
$env:CORS_ALLOWED_ORIGINS = 'http://localhost:4200'
$env:DB_HOST = $PgHost
$env:DB_PORT = "$PgPort"
$env:DB_SSLMODE = 'disable'

# No NATS locally, so events are disabled and the services run normally.
Remove-Item Env:\NATS_URL -ErrorAction SilentlyContinue
Remove-Item Env:\OTEL_EXPORTER_OTLP_ENDPOINT -ErrorAction SilentlyContinue

if ($Service -eq 'identity') {
    $env:DB_USER = 'identity_user'
    $env:DB_PASSWORD = 'password'
    $env:DB_NAME = 'identity_db'
    # The token issuer, and the only process given the private key.
    $env:JWT_PRIVATE_KEY = $privateKey
    Remove-Item Env:\JWT_PUBLIC_KEY -ErrorAction SilentlyContinue

    Write-Host "identity-svc -> http://localhost:8080" -ForegroundColor Cyan
    Push-Location (Join-Path $repo 'apps\identity-svc')
    try { & go run ./cmd/server } finally { Pop-Location }
} else {
    $env:DB_USER = 'football_user'
    $env:DB_PASSWORD = 'password'
    $env:DB_NAME = 'football_db'
    # Verification only: this service holds no private key and cannot mint.
    $env:JWT_PUBLIC_KEY = $publicKey
    Remove-Item Env:\JWT_PRIVATE_KEY -ErrorAction SilentlyContinue

    Write-Host "football-svc -> http://localhost:8081" -ForegroundColor Cyan
    Push-Location (Join-Path $repo 'apps\football-svc')
    try { & go run . } finally { Pop-Location }
}
