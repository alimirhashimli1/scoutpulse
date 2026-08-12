<#
.SYNOPSIS
  Prepares a local (Docker-free) ScoutPulse stack against an already-running
  PostgreSQL server.

.DESCRIPTION
  The documented path is `make keys && make up`, which needs make, openssl and
  Docker. This script needs none of them -- only Go, psql, and a Postgres
  server you can already connect to. It is idempotent: run it as often as you
  like.

  It does three things:
    1. Generates the JWT key pair into .env (skipped if already present).
    2. Creates a login role and a database per service.
    3. Applies every migration, in order, to each database.

.PARAMETER SuperUser
  A Postgres superuser. Defaults to 'postgres'.

.PARAMETER SuperPassword
  That user's password. Prompted for if omitted.

.EXAMPLE
  .\scripts\dev-setup.ps1
#>
[CmdletBinding()]
param(
    [string] $PgHost   = 'localhost',
    [int]    $PgPort   = 5432,
    [string] $SuperUser = 'postgres',
    [string] $SuperPassword
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot

function Say([string] $msg, [string] $colour = 'Cyan') {
    Write-Host $msg -ForegroundColor $colour
}

# --- preflight ---------------------------------------------------------
foreach ($tool in 'go', 'psql') {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        throw "$tool is not on PATH. Install it, or use the Docker path instead (docker compose up)."
    }
}

if (-not $SuperPassword) {
    $secure = Read-Host "Password for Postgres user '$SuperUser'" -AsSecureString
    $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    $SuperPassword = [Runtime.InteropServices.Marshal]::PtrToStringAuto($bstr)
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
}

# psql reads this rather than prompting. Scoped to this process only.
$env:PGPASSWORD = $SuperPassword

# --- 1. keys -----------------------------------------------------------
$envFile = Join-Path $repo '.env'
if (-not (Test-Path $envFile)) {
    Copy-Item (Join-Path $repo '.env.example') $envFile
    Say "Created .env from .env.example"
}

if (Select-String -Path $envFile -Pattern '^JWT_PRIVATE_KEY=' -Quiet) {
    Say "JWT key pair already in .env, leaving it alone." 'DarkGray'
} else {
    Say "Generating JWT key pair..."
    Push-Location (Join-Path $repo 'libs\auth')
    try {
        $keys = & go run ./cmd/genkeys
        if ($LASTEXITCODE -ne 0) { throw "genkeys failed" }
    } finally { Pop-Location }
    Add-Content -Path $envFile -Value $keys -Encoding utf8
    Say "Key pair written to .env" 'Green'
}

# --- 2. roles and databases -------------------------------------------
# Identifiers and passwords go through psql variables and format()'s %I/%L,
# never string-pasted into the statement -- the same rule the compose
# bootstrap script follows, and this runs as superuser too.
$services = @(
    @{ Db = 'identity_db'; Role = 'identity_user'; Password = 'password'; Migrations = 'apps\identity-svc\migrations' },
    @{ Db = 'football_db'; Role = 'football_user'; Password = 'password'; Migrations = 'apps\football-svc\migrations' }
)

# Two things to be careful of below, both of which silently produce garbage
# rather than failing loudly:
#
#   1. In an argument to a NATIVE command, "role=$svc.Role" expands only $svc
#      and leaves ".Role" as literal text -- yielding
#      "role=System.Collections.Hashtable.Role". Only a bare $svc.Role token
#      evaluates the property. The subexpression form "$($svc.Role)" is
#      unambiguous, so locals are used throughout instead.
#
#   2. \gexec runs the *current query buffer*. Ending the SELECT with a
#      semicolon sends and clears that buffer first, so \gexec then has nothing
#      to run and the generated statement is merely printed. The SELECT before
#      a \gexec must NOT be semicolon-terminated.
foreach ($svc in $services) {
    $db   = $svc.Db
    $role = $svc.Role
    $pw   = $svc.Password

    Say "Ensuring role $role and database $db..."

    $roleSql = @'
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'role', :'pw')
 WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'role')
\gexec
'@
    $roleSql | & psql --host $PgHost --port $PgPort --username $SuperUser --dbname postgres `
        -v ON_ERROR_STOP=1 -v "role=$role" -v "pw=$pw" -q
    if ($LASTEXITCODE -ne 0) { throw "creating role $role failed" }

    $dbSql = @'
SELECT format('CREATE DATABASE %I OWNER %I', :'db', :'role')
 WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'db')
\gexec
'@
    $dbSql | & psql --host $PgHost --port $PgPort --username $SuperUser --dbname postgres `
        -v ON_ERROR_STOP=1 -v "db=$db" -v "role=$role" -q
    if ($LASTEXITCODE -ne 0) { throw "creating database $db failed" }

    # Verify rather than trust: if \gexec silently did nothing, everything
    # downstream fails with a confusing error instead of this clear one.
    $exists = & psql --host $PgHost --port $PgPort --username $SuperUser --dbname postgres `
        -tAc "SELECT 1 FROM pg_database WHERE datname = '$db'"
    if ($exists -ne '1') { throw "database $db was not created -- check the psql output above" }

    $grantSql = @'
SELECT format('GRANT ALL ON SCHEMA public TO %I', :'role')
\gexec
'@
    $grantSql | & psql --host $PgHost --port $PgPort --username $SuperUser --dbname $db `
        -v ON_ERROR_STOP=1 -v "role=$role" -q
    if ($LASTEXITCODE -ne 0) { throw "granting on $db failed" }

    Say "  role and database ready" 'Green'
}

# --- 3. migrations -----------------------------------------------------
# golang-migrate is not required: the .up.sql files are plain SQL and are
# applied in filename order. A simple applied-versions table keeps this
# idempotent, mirroring what golang-migrate does in the Docker path.
foreach ($svc in $services) {
    $db   = $svc.Db
    $role = $svc.Role

    Say "Applying migrations to $db..."

    $track = "CREATE TABLE IF NOT EXISTS schema_migrations_local (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW());"
    $track | & psql --host $PgHost --port $PgPort --username $SuperUser --dbname $db -v ON_ERROR_STOP=1 -q
    if ($LASTEXITCODE -ne 0) { throw "creating the migration tracking table failed" }

    $files = Get-ChildItem (Join-Path $repo $svc.Migrations) -Filter '*.up.sql' | Sort-Object Name
    foreach ($file in $files) {
        $version = $file.BaseName

        $already = & psql --host $PgHost --port $PgPort --username $SuperUser --dbname $db `
            -tAc "SELECT 1 FROM schema_migrations_local WHERE version = '$version'"
        if ($already -eq '1') {
            Say "  skip $version" 'DarkGray'
            continue
        }

        & psql --host $PgHost --port $PgPort --username $SuperUser --dbname $db `
            -v ON_ERROR_STOP=1 -q -f $file.FullName
        if ($LASTEXITCODE -ne 0) { throw "applying $version failed" }

        & psql --host $PgHost --port $PgPort --username $SuperUser --dbname $db `
            -v ON_ERROR_STOP=1 -q -c "INSERT INTO schema_migrations_local (version) VALUES ('$version')"
        if ($LASTEXITCODE -ne 0) { throw "recording $version failed" }

        Say "  applied $version" 'Green'
    }

    # The service connects as its own role, so it must own what it will write.
    # No trailing semicolon: see the note above the previous loop.
    $reassign = @'
SELECT format('ALTER TABLE %I OWNER TO %I', tablename, :'role')
  FROM pg_tables WHERE schemaname = 'public'
\gexec
'@
    $reassign | & psql --host $PgHost --port $PgPort --username $SuperUser --dbname $db `
        -v ON_ERROR_STOP=1 -v "role=$role" -q
    if ($LASTEXITCODE -ne 0) { throw "reassigning table ownership in $db failed" }

    $tables = & psql --host $PgHost --port $PgPort --username $SuperUser --dbname $db `
        -tAc "SELECT count(*) FROM pg_tables WHERE schemaname = 'public'"
    Say "  $db has $($tables.Trim()) tables" 'Green'
}

Remove-Item Env:\PGPASSWORD -ErrorAction SilentlyContinue

Say ""
Say "Setup complete." 'Green'
Say ""
Say "Next, start the two services in separate terminals:" 'White'
Say "  .\scripts\dev-run.ps1 identity" 'White'
Say "  .\scripts\dev-run.ps1 football" 'White'
Say ""
Say "Then exercise the API:" 'White'
Say "  .\scripts\check-endpoints.ps1" 'White'
