<#
.SYNOPSIS
  Downloads a backup of both production databases (football and identity) to
  this machine.

.DESCRIPTION
  Production data lives in two Supabase Postgres databases, one per service.
  This script runs pg_dump against each and writes one compressed, restorable
  file per database into a timestamped folder:

      <OutDir>\2026-10-21_093000\football.dump
      <OutDir>\2026-10-21_093000\identity.dump
      <OutDir>\2026-10-21_093000\manifest.json

  Credentials are not stored anywhere. By default they are read at run time
  from the Railway services' own DB_* variables through the Railway CLI, so the
  backup always uses exactly what production uses. Run `railway login` once
  first. Without the Railway CLI, pass connection strings instead (-FootballUrl
  / -IdentityUrl, or the FOOTBALL_DB_URL / IDENTITY_DB_URL environment
  variables) -- Supabase shows them under Project Settings -> Database ->
  Connection string -> "Session pooler".

  Only the `public` schema is dumped. That is where every table the services
  own lives, including golang-migrate's schema_migrations; the other schemas in
  a Supabase database (auth, storage, realtime, ...) belong to Supabase and are
  neither ours to back up nor restorable elsewhere.

  Each file is checked after download (pg_restore --list must read it), and a
  dump is only given its final name once that check passes, so a file in the
  backup folder is never a half-written one. Old backups beyond -Keep are
  deleted.

  The dumps contain users' e-mail addresses and password hashes. The default
  folder is outside the repository on purpose -- the repository is public.

.EXAMPLE
  .\scripts\backup-db.ps1
  Backs up both databases to Documents\ScoutPulse-backups.

.EXAMPLE
  .\scripts\backup-db.ps1 -Only football -OutDir D:\Backups\scoutpulse -Keep 60

.EXAMPLE
  $env:FOOTBALL_DB_URL = 'postgresql://postgres.xxxx:PASSWORD@aws-1-eu-west-1.pooler.supabase.com:5432/postgres'
  .\scripts\backup-db.ps1 -Only football
#>
[CmdletBinding()]
param(
    # Where backups are written. One sub-folder per run.
    [string] $OutDir = (Join-Path ([Environment]::GetFolderPath('MyDocuments')) 'ScoutPulse-backups'),

    # How many backup folders to keep. Older ones are deleted. 0 keeps all.
    [int] $Keep = 30,

    # Back up just one of the two databases.
    [ValidateSet('all', 'football', 'identity')]
    [string] $Only = 'all',

    # Explicit connection strings. When set, Railway is not consulted for that
    # database.
    [string] $FootballUrl = $env:FOOTBALL_DB_URL,
    [string] $IdentityUrl = $env:IDENTITY_DB_URL,

    # Where the Railway CLI finds each service's DB_* variables. These are
    # project ids, not secrets.
    [string] $FootballRailwayProject = 'e26f2935-e69e-4686-be78-9a7f889ce997',
    [string] $IdentityRailwayProject = '863b9ba5-f58b-4381-affc-8da8e3c10263',
    [string] $RailwayService = 'scoutpulse',
    [string] $RailwayEnvironment = 'production'
)

$ErrorActionPreference = 'Stop'

# ------------------------------------------------------------------ tools

# pg_dump must be at least as new as the server. Prefer the newest installed
# PostgreSQL over whatever happens to be first on PATH.
function Find-PgTool([string] $name) {
    $installed = Get-ChildItem 'C:\Program Files\PostgreSQL\*\bin\' -Filter "$name.exe" -ErrorAction SilentlyContinue |
        Sort-Object { [int]($_.Directory.Parent.Name -replace '\D', '') } -Descending |
        Select-Object -First 1
    if ($installed) { return $installed.FullName }

    $onPath = Get-Command $name -ErrorAction SilentlyContinue
    if ($onPath) { return $onPath.Source }

    throw "$name was not found. Install PostgreSQL client tools (https://www.postgresql.org/download/windows/) -- version 17 or newer."
}

$pgDump = Find-PgTool 'pg_dump'
$pgRestore = Find-PgTool 'pg_restore'

# ------------------------------------------------------------ credentials

# Reads one service's DB_* variables from Railway. Values are kept in memory
# only and never printed.
function Get-RailwayDb([string] $project, [string] $label) {
    if (-not (Get-Command railway -ErrorAction SilentlyContinue)) {
        throw "No connection string for $label and the Railway CLI is not installed. Pass -$($label)Url or set $($label.ToUpper())_DB_URL."
    }
    $json = railway variables -s $RailwayService -p $project -e $RailwayEnvironment --json 2>$null
    if ($LASTEXITCODE -ne 0 -or -not $json) {
        throw "Could not read $label variables from Railway. Run 'railway login' and try again."
    }
    $v = ($json | Out-String) | ConvertFrom-Json
    foreach ($k in 'DB_HOST', 'DB_PORT', 'DB_USER', 'DB_PASSWORD', 'DB_NAME') {
        if (-not $v.$k) { throw "Railway service for $label has no $k variable." }
    }
    $sslmode = if ($v.DB_SSLMODE) { $v.DB_SSLMODE } else { 'require' }
    return @{ Host = $v.DB_HOST; Port = $v.DB_PORT; User = $v.DB_USER; Password = $v.DB_PASSWORD; Database = $v.DB_NAME; SslMode = $sslmode }
}

# Turns a postgres:// URL into the same shape. The password is URL-decoded, so
# one containing @ or : can be pasted percent-encoded as Supabase shows it.
function ConvertFrom-DbUrl([string] $url) {
    $uri = [uri]($url -replace '^postgres(ql)?://', 'http://')
    $user, $password = $uri.UserInfo -split ':', 2
    $query = [System.Web.HttpUtility]::ParseQueryString($uri.Query)
    $sslmode = if ($query['sslmode']) { $query['sslmode'] } else { 'require' }
    $port = if ($uri.Port -gt 0) { $uri.Port } else { 5432 }
    return @{
        Host     = $uri.Host
        Port     = $port
        User     = [uri]::UnescapeDataString($user)
        Password = [uri]::UnescapeDataString($password)
        Database = $uri.AbsolutePath.TrimStart('/')
        SslMode  = $sslmode
    }
}
Add-Type -AssemblyName System.Web

# ----------------------------------------------------------------- backup

function Backup-Database([string] $label, [hashtable] $db, [string] $folder) {
    $final = Join-Path $folder "$label.dump"
    $partial = "$final.partial"

    Write-Host "-> $label  ($($db.User)@$($db.Host):$($db.Port)/$($db.Database))"

    # The password goes through the environment, not the command line, where
    # it would be visible to every other process on the machine.
    $env:PGPASSWORD = $db.Password
    $env:PGSSLMODE = $db.SslMode
    $env:PGCONNECT_TIMEOUT = '20'
    try {
        $started = Get-Date
        & $pgDump --host $db.Host --port $db.Port --username $db.User --dbname $db.Database `
            --schema=public --format=custom --compress=9 `
            --no-owner --no-privileges --no-password `
            --file $partial
        if ($LASTEXITCODE -ne 0) { throw "pg_dump failed for $label (exit $LASTEXITCODE)." }
    } finally {
        Remove-Item Env:PGPASSWORD, Env:PGSSLMODE, Env:PGCONNECT_TIMEOUT -ErrorAction SilentlyContinue
    }

    # A dump that pg_restore cannot read is not a backup.
    $toc = & $pgRestore --list $partial
    if ($LASTEXITCODE -ne 0) { throw "The $label dump was written but cannot be read back." }
    $tables = @($toc | Where-Object { $_ -match ' TABLE DATA ' }).Count
    if ($tables -eq 0) { throw "The $label dump contains no table data -- refusing to keep it." }

    Move-Item $partial $final -Force
    $size = (Get-Item $final).Length
    $seconds = [math]::Round(((Get-Date) - $started).TotalSeconds, 1)
    Write-Host ("   ok: {0} tables, {1:N1} MB, {2}s" -f $tables, ($size / 1MB), $seconds) -ForegroundColor Green

    return [ordered]@{ file = "$label.dump"; tables = $tables; bytes = $size; host = $db.Host; database = $db.Database }
}

$targets = [ordered]@{}
if ($Only -in 'all', 'football') {
    $targets['football'] = if ($FootballUrl) { ConvertFrom-DbUrl $FootballUrl } else { Get-RailwayDb $FootballRailwayProject 'Football' }
}
if ($Only -in 'all', 'identity') {
    $targets['identity'] = if ($IdentityUrl) { ConvertFrom-DbUrl $IdentityUrl } else { Get-RailwayDb $IdentityRailwayProject 'Identity' }
}

$stamp = Get-Date -Format 'yyyy-MM-dd_HHmmss'
$folder = Join-Path $OutDir $stamp
New-Item -ItemType Directory -Force -Path $folder | Out-Null
Write-Host "Backing up to $folder"

$results = [ordered]@{}
$failed = @()
foreach ($label in $targets.Keys) {
    try {
        $results[$label] = Backup-Database $label $targets[$label] $folder
    } catch {
        Write-Host "   FAILED: $($_.Exception.Message)" -ForegroundColor Red
        Remove-Item (Join-Path $folder "$label.dump.partial") -ErrorAction SilentlyContinue
        $failed += $label
    }
}

[ordered]@{
    created_at = (Get-Date).ToString('o')
    pg_dump    = (& $pgDump --version)
    databases  = $results
    failed     = $failed
} | ConvertTo-Json -Depth 4 | Set-Content -Encoding utf8 (Join-Path $folder 'manifest.json')

# ------------------------------------------------------------- retention

# Only folders this script made are considered, and only after a run that
# fully succeeded -- a failing run must never delete the last good backups.
if ($Keep -gt 0 -and $failed.Count -eq 0) {
    $old = Get-ChildItem $OutDir -Directory |
        Where-Object { $_.Name -match '^\d{4}-\d{2}-\d{2}_\d{6}$' } |
        Sort-Object Name -Descending |
        Select-Object -Skip $Keep
    foreach ($dir in $old) {
        Remove-Item $dir.FullName -Recurse -Force
        Write-Host "   removed old backup $($dir.Name)" -ForegroundColor DarkGray
    }
}

if ($failed.Count -gt 0) {
    Write-Host "Backup incomplete: $($failed -join ', ') failed." -ForegroundColor Red
    exit 1
}
Write-Host "Done." -ForegroundColor Green
