<#
.SYNOPSIS
  Restores one database from a backup made by backup-db.ps1.

.DESCRIPTION
  Two uses:

  1. Proving a backup works -- restore it into a local Postgres and look at the
     data. A backup that has never been restored is a hope, not a backup. Do
     this after the first backup and every so often after that.

  2. Disaster recovery -- putting production back from a backup. The target is
     then a remote database, which this script refuses unless -AllowRemote is
     given and the database name is typed back to confirm.

  The restore replaces every table in the target's `public` schema that the
  backup contains, inside a single transaction: either the whole backup lands
  or nothing changes.

.EXAMPLE
  .\scripts\restore-db.ps1 -Database football -TargetUrl 'postgresql://postgres:postgres@localhost:5432/football_restore_test'
  Restores the newest football backup into a local test database (created if
  missing).

.EXAMPLE
  .\scripts\restore-db.ps1 -Dump 'C:\...\2026-10-21_093000\identity.dump' -TargetUrl $url -AllowRemote
  Puts a specific backup back into a remote database, after confirmation.
#>
[CmdletBinding(DefaultParameterSetName = 'Latest')]
param(
    # Which database's newest backup to restore.
    [Parameter(Mandatory = $true, ParameterSetName = 'Latest')]
    [ValidateSet('football', 'identity')]
    [string] $Database,

    # Folder backup-db.ps1 writes to.
    [Parameter(ParameterSetName = 'Latest')]
    [string] $BackupDir = (Join-Path ([Environment]::GetFolderPath('MyDocuments')) 'ScoutPulse-backups'),

    # Or an exact dump file.
    [Parameter(Mandatory = $true, ParameterSetName = 'File')]
    [string] $Dump,

    # postgresql://user:password@host:port/dbname
    [Parameter(Mandatory = $true)]
    [string] $TargetUrl,

    # Required for anything that is not localhost.
    [switch] $AllowRemote
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Web

function Find-PgTool([string] $name) {
    $installed = Get-ChildItem 'C:\Program Files\PostgreSQL\*\bin\' -Filter "$name.exe" -ErrorAction SilentlyContinue |
        Sort-Object { [int]($_.Directory.Parent.Name -replace '\D', '') } -Descending |
        Select-Object -First 1
    if ($installed) { return $installed.FullName }
    $onPath = Get-Command $name -ErrorAction SilentlyContinue
    if ($onPath) { return $onPath.Source }
    throw "$name was not found. Install PostgreSQL client tools, version 17 or newer."
}
$pgRestore = Find-PgTool 'pg_restore'
$psql = Find-PgTool 'psql'

# ------------------------------------------------------------ the backup

if ($PSCmdlet.ParameterSetName -eq 'Latest') {
    $latest = Get-ChildItem $BackupDir -Directory -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^\d{4}-\d{2}-\d{2}_\d{6}$' -and (Test-Path (Join-Path $_.FullName "$Database.dump")) } |
        Sort-Object Name -Descending |
        Select-Object -First 1
    if (-not $latest) { throw "No $Database backup found in $BackupDir." }
    $Dump = Join-Path $latest.FullName "$Database.dump"
}
if (-not (Test-Path $Dump)) { throw "Backup file not found: $Dump" }
$Dump = (Resolve-Path $Dump).Path

# ------------------------------------------------------------ the target

$uri = [uri]($TargetUrl -replace '^postgres(ql)?://', 'http://')
$user, $password = $uri.UserInfo -split ':', 2
$query = [System.Web.HttpUtility]::ParseQueryString($uri.Query)
$target = @{
    Host     = $uri.Host
    Port     = $(if ($uri.Port -gt 0) { $uri.Port } else { 5432 })
    User     = [uri]::UnescapeDataString($user)
    Password = $(if ($password) { [uri]::UnescapeDataString($password) } else { '' })
    Database = $uri.AbsolutePath.TrimStart('/')
    SslMode  = $(if ($query['sslmode']) { $query['sslmode'] } else { 'prefer' })
}
if (-not $target.Database) { throw 'The target URL must name a database.' }

$isLocal = $target.Host -in 'localhost', '127.0.0.1', '::1'
Write-Host "Backup : $Dump"
Write-Host "Target : $($target.User)@$($target.Host):$($target.Port)/$($target.Database)"

if (-not $isLocal) {
    if (-not $AllowRemote) {
        throw 'The target is not localhost. Pass -AllowRemote if you really mean to restore into a remote database.'
    }
    Write-Host ''
    Write-Host 'This REPLACES the tables in that remote database with the backup.' -ForegroundColor Yellow
    Write-Host 'Anything written there since the backup was taken will be lost.' -ForegroundColor Yellow
    $typed = Read-Host "Type the database name ($($target.Database)) to continue"
    if ($typed -ne $target.Database) { throw 'Not confirmed. Nothing was changed.' }
}

$env:PGPASSWORD = $target.Password
$env:PGSSLMODE = $target.SslMode
$env:PGCONNECT_TIMEOUT = '20'
$conn = @('--host', $target.Host, '--port', $target.Port, '--username', $target.User, '--no-password')
$tocFile = [IO.Path]::GetTempFileName()

try {
    # A local test database is created on demand, so trying a backup is one
    # command. A remote one must already exist -- that is not something to
    # create by accident.
    if ($isLocal) {
        $dbLiteral = $target.Database -replace "'", "''"
        $exists = & $psql @conn --dbname postgres -tAc "SELECT 1 FROM pg_database WHERE datname = '$dbLiteral'"
        if ($LASTEXITCODE -ne 0) { throw 'Could not connect to the local Postgres server.' }
        if ($exists -ne '1') {
            $dbIdent = '"' + ($target.Database -replace '"', '""') + '"'
            & $psql @conn --dbname postgres -v ON_ERROR_STOP=1 -q -c "CREATE DATABASE $dbIdent"
            if ($LASTEXITCODE -ne 0) {
                throw "Could not create database $($target.Database). Connect as a user that may create databases (usually 'postgres'), or create it first."
            }
            Write-Host "Created local database $($target.Database)"
        }
    }

    # The backup holds the public schema only, so extensions are not in it.
    # The search indexes need pg_trgm, installed in the same schema it had in
    # production -- Supabase keeps extensions in `extensions`, and the dumped
    # index definitions name that schema explicitly.
    $schemaSql = & $pgRestore --schema-only --file - $Dump | Out-String
    $trgmSchema = 'public'
    if ($schemaSql -match '([A-Za-z_][A-Za-z0-9_]*)\.gin_trgm_ops') { $trgmSchema = $Matches[1] }
    $prep = "SET client_min_messages = warning; CREATE SCHEMA IF NOT EXISTS ""$trgmSchema""; CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA ""$trgmSchema"";"
    & $psql @conn --dbname $target.Database -v ON_ERROR_STOP=1 -q -c $prep
    if ($LASTEXITCODE -ne 0) { throw 'Could not install the pg_trgm extension on the target.' }

    # --clean drops each object before recreating it. The dump also carries an
    # entry for the public schema itself, which --clean would turn into
    # DROP SCHEMA public -- so that entry is left out of the restore list.
    & $pgRestore --list $Dump |
        Where-Object { $_ -notmatch ' SCHEMA - public ' -and $_ -notmatch ' COMMENT - SCHEMA public ' } |
        Set-Content -Encoding ascii $tocFile

    & $pgRestore @conn --dbname $target.Database `
        --use-list $tocFile --clean --if-exists `
        --no-owner --no-privileges --single-transaction `
        $Dump
    if ($LASTEXITCODE -ne 0) { throw "pg_restore failed (exit $LASTEXITCODE). The transaction was rolled back; the target is unchanged." }

    # Row counts, so "it worked" is something to look at rather than assume.
    Write-Host ''
    Write-Host 'Restored. Rows per table:' -ForegroundColor Green
    $countSql = @'
SELECT format('  %-28s %s', table_name,
       (xpath('/row/c/text()', query_to_xml(format('SELECT count(*) AS c FROM public.%I', table_name), false, true, '')))[1]::text)
  FROM information_schema.tables
 WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
 ORDER BY table_name
'@
    & $psql @conn --dbname $target.Database -tA -c $countSql
} finally {
    Remove-Item $tocFile -ErrorAction SilentlyContinue
    Remove-Item Env:PGPASSWORD, Env:PGSSLMODE, Env:PGCONNECT_TIMEOUT -ErrorAction SilentlyContinue
}
