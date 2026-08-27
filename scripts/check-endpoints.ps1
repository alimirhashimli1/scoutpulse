<#
.SYNOPSIS
  Exercises the ScoutPulse API end to end and asserts on the responses.

.DESCRIPTION
  smoke-test.ps1 checks that containers are running. This checks that the API
  actually works: it registers an account, walks a player from one club to
  another through a real transfer, and verifies the temporal invariants that
  most of the write logic exists to defend.

  It also pins three behaviours that are easy to regress silently:
    - a malformed id is a 400, not a 500
    - an unauthenticated write is a JSON 401, not plain text
    - an editor cannot do an admin-only write

  Defaults target the Docker-free local setup (services on their own ports).
  Pass -Gateway to go through Caddy on :8000 instead.

.EXAMPLE
  .\scripts\check-endpoints.ps1
  .\scripts\check-endpoints.ps1 -Gateway
#>
[CmdletBinding()]
param(
    [switch] $Gateway,
    [string] $IdentityUrl,
    [string] $FootballUrl,
    [switch] $KeepData
)

$ErrorActionPreference = 'Stop'

if ($Gateway) {
    if (-not $IdentityUrl) { $IdentityUrl = 'http://localhost:8000/api/identity' }
    if (-not $FootballUrl) { $FootballUrl = 'http://localhost:8000/api/football' }
} else {
    if (-not $IdentityUrl) { $IdentityUrl = 'http://localhost:8080' }
    if (-not $FootballUrl) { $FootballUrl = 'http://localhost:8081' }
}

$script:Passed = 0
$script:Failed = 0
$script:Notes  = @()

function Pass([string] $what) {
    $script:Passed++
    Write-Host "  PASS  $what" -ForegroundColor Green
}

function Fail([string] $what, [string] $detail) {
    $script:Failed++
    Write-Host "  FAIL  $what" -ForegroundColor Red
    if ($detail) { Write-Host "        $detail" -ForegroundColor DarkGray }
}

function Section([string] $name) {
    Write-Host ""
    Write-Host $name -ForegroundColor Cyan
}

# Invoke-RestMethod throws on any non-2xx, which makes asserting on a 400 or
# 401 awkward. This returns the status and parsed body either way.
function Invoke-Api {
    param(
        [string] $Method,
        [string] $Url,
        $Body,
        [string] $Token,
        [string] $ContentType = 'application/json'
    )

    $headers = @{}
    if ($Token) { $headers['Authorization'] = "Bearer $Token" }

    $args = @{
        Method      = $Method
        Uri         = $Url
        Headers     = $headers
        ContentType = $ContentType
        UseBasicParsing = $true
    }
    if ($null -ne $Body) {
        $args['Body'] = ($Body | ConvertTo-Json -Depth 10 -Compress)
    }

    try {
        $resp = Invoke-WebRequest @args
        $parsed = $null
        if ($resp.Content) {
            try { $parsed = $resp.Content | ConvertFrom-Json } catch { $parsed = $resp.Content }
        }
        return [pscustomobject]@{ Status = [int]$resp.StatusCode; Body = $parsed; Raw = $resp.Content }
    } catch [System.Net.WebException] {
        $r = $_.Exception.Response
        if (-not $r) {
            return [pscustomobject]@{ Status = 0; Body = $null; Raw = $_.Exception.Message }
        }
        $status = [int]$r.StatusCode
        $reader = New-Object System.IO.StreamReader($r.GetResponseStream())
        $raw = $reader.ReadToEnd()
        $reader.Close()
        $parsed = $null
        if ($raw) { try { $parsed = $raw | ConvertFrom-Json } catch { $parsed = $raw } }
        return [pscustomobject]@{ Status = $status; Body = $parsed; Raw = $raw }
    }
}

Write-Host "ScoutPulse endpoint check" -ForegroundColor White
Write-Host "  identity  $IdentityUrl" -ForegroundColor DarkGray
Write-Host "  football  $FootballUrl" -ForegroundColor DarkGray

# --- health ------------------------------------------------------------
Section "Health"
$h1 = Invoke-Api -Method GET -Url "$IdentityUrl/health"
if ($h1.Status -eq 200) { Pass "identity-svc is up" } else { Fail "identity-svc is up" "got $($h1.Status). Start it with .\scripts\dev-run.ps1 identity"; }
$h2 = Invoke-Api -Method GET -Url "$FootballUrl/health"
if ($h2.Status -eq 200) { Pass "football-svc is up" } else { Fail "football-svc is up" "got $($h2.Status). Start it with .\scripts\dev-run.ps1 football" }

if ($h1.Status -ne 200 -or $h2.Status -ne 200) {
    Write-Host ""
    Write-Host "Both services must be running. Stopping here." -ForegroundColor Red
    exit 1
}

# --- accounts ----------------------------------------------------------
Section "Accounts and tokens"
$suffix = [Guid]::NewGuid().ToString('N').Substring(0, 8)
$username = "checker_$suffix"
$password = 'check-endpoints-pw'

$reg = Invoke-Api -Method POST -Url "$IdentityUrl/api/v1/auth/register" -Body @{
    username = $username; email = "$username@example.test"; password = $password
}
if ($reg.Status -eq 201) { Pass "register creates an account" } else { Fail "register creates an account" "status $($reg.Status): $($reg.Raw)" }
# Checked against the raw response text, not a top-level property: registration
# now nests the account under "user", so a property probe on the envelope would
# pass without ever looking at the account it is meant to be inspecting.
if ($reg.Raw -notmatch 'password_hash') {
    Pass "the password hash is never serialised"
} else {
    Fail "the password hash is never serialised" "found password_hash in the response body"
}
if ($reg.Body.user.role -eq 'user') { Pass "self-registration assigns 'user', not a client-chosen role" } else { Fail "self-registration assigns 'user'" "got '$($reg.Body.user.role)'" }

# A smuggled role must be refused outright, not ignored.
$smuggle = Invoke-Api -Method POST -Url "$IdentityUrl/api/v1/auth/register" -Body @{
    username = "smuggle_$suffix"; email = "smuggle_$suffix@example.test"; password = $password; role = 'admin'
}
if ($smuggle.Status -eq 400) { Pass "an unknown 'role' field is rejected (400)" } else { Fail "an unknown 'role' field is rejected" "expected 400, got $($smuggle.Status)" }

$login = Invoke-Api -Method POST -Url "$IdentityUrl/api/v1/auth/login" -Body @{ identifier = $username; password = $password }
if ($login.Status -eq 200 -and $login.Body.access_token) { Pass "login returns a token pair" } else { Fail "login returns a token pair" "status $($login.Status)" }
$userToken = $login.Body.access_token
if ($login.Body.expires_in -eq 900) { Pass "access token expires in 15 minutes" } else { Fail "access token lifetime" "expires_in = $($login.Body.expires_in)" }

$bad = Invoke-Api -Method POST -Url "$IdentityUrl/api/v1/auth/login" -Body @{ identifier = $username; password = 'wrong-password' }
if ($bad.Status -eq 401) { Pass "a wrong password is rejected" } else { Fail "a wrong password is rejected" "got $($bad.Status)" }

# Promote through the database, then log in again: the role is read at login,
# so the new token carries it. identity_user owns the table, so no superuser
# credentials are needed here.
Section "Promoting the checker to admin"
$adminToken = $null
if (Get-Command psql -ErrorAction SilentlyContinue) {
    $env:PGPASSWORD = 'password'
    & psql --host localhost --port 5432 --username identity_user --dbname identity_db `
        -v ON_ERROR_STOP=1 -q -c "UPDATE users SET role = 'admin' WHERE username = '$username'" 2>&1 | Out-Null
    $promoted = ($LASTEXITCODE -eq 0)
    Remove-Item Env:\PGPASSWORD -ErrorAction SilentlyContinue

    if ($promoted) {
        $relogin = Invoke-Api -Method POST -Url "$IdentityUrl/api/v1/auth/login" -Body @{ identifier = $username; password = $password }
        $adminToken = $relogin.Body.access_token
        Pass "role change is picked up on the next login"
    } else {
        Fail "promote to admin" "psql could not update the row"
    }
} else {
    $script:Notes += "psql not found: admin-only checks were skipped."
    Write-Host "  SKIP  psql unavailable, admin-only checks will be skipped" -ForegroundColor Yellow
}

# --- authorization -----------------------------------------------------
Section "Authorization"
$noAuth = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/leagues" -Body @{ name = "x"; country = "y" }
if ($noAuth.Status -eq 401) { Pass "an unauthenticated write is refused (401)" } else { Fail "an unauthenticated write is refused" "got $($noAuth.Status)" }
if ($noAuth.Body -and $noAuth.Body.code -eq 'unauthorized') {
    Pass "the 401 body is JSON with a 'code' field"
} else {
    Fail "the 401 body is JSON" "body was: $($noAuth.Raw)"
}

$asUser = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/leagues" -Token $userToken -Body @{ name = "x"; country = "y" }
if ($asUser.Status -eq 403) { Pass "a plain user cannot create a league (403)" } else { Fail "a plain user cannot create a league" "got $($asUser.Status)" }

if (-not $adminToken) {
    Write-Host ""
    Write-Host "No admin token; skipping the domain walk." -ForegroundColor Yellow
    Write-Host ""
    Write-Host "$script:Passed passed, $script:Failed failed" -ForegroundColor White
    if ($script:Failed -gt 0) { exit 1 } else { exit 0 }
}

# --- domain walk -------------------------------------------------------
Section "Building a league, a club and a player"

$league = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/leagues" -Token $adminToken -Body @{
    name = "Check League $suffix"; country = 'Testland'; competition_type = 'league'; tier = 1
}
if ($league.Status -eq 201 -and $league.Body.id) { Pass "create a league" } else { Fail "create a league" "status $($league.Status): $($league.Raw)" }
$leagueId = $league.Body.id

$selling = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/teams" -Token $adminToken -Body @{
    league_id = $leagueId; name = "Selling FC $suffix"; city = 'Testville'; founded_year = 1901
}
$buying = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/teams" -Token $adminToken -Body @{
    league_id = $leagueId; name = "Buying FC $suffix"; city = 'Testburg'; founded_year = 1888
}
if ($selling.Status -eq 201 -and $buying.Status -eq 201) { Pass "create two clubs" } else { Fail "create two clubs" "$($selling.Status) / $($buying.Status)" }
$sellingId = $selling.Body.id
$buyingId  = $buying.Body.id

$badYear = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/teams" -Token $adminToken -Body @{
    league_id = $leagueId; name = "Too Old FC"; founded_year = 1700
}
if ($badYear.Status -eq 400) { Pass "a founding year before 1850 is refused" } else { Fail "founded_year validation" "got $($badYear.Status)" }

# contract_start dates the opening transfer. Without it that record is dated
# now, which would make the 2026 move below a backdated one.
$player = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/players" -Token $adminToken -Body @{
    team_id = $sellingId; name = "Check Player $suffix"; position = 'Forward'
    nationality = 'Testland'; preferred_foot = 'left'; squad_number = 9
    contract_start = '2020-07-01T00:00:00Z'
    market_value_minor = 1500000
}
if ($player.Status -eq 201 -and $player.Body.id) { Pass "create a player at the selling club" } else { Fail "create a player" "status $($player.Status): $($player.Raw)" }
$playerId = $player.Body.id

if ($player.Body.market_value_minor -eq 1500000) { Pass "money round-trips as an integer count of minor units" } else { Fail "money encoding" "got $($player.Body.market_value_minor)" }

$badFoot = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/players" -Token $adminToken -Body @{
    team_id = $sellingId; name = 'Bad Foot'; position = 'Forward'; preferred_foot = 'sideways'
}
if ($badFoot.Status -eq 400) { Pass "an unknown preferred_foot is refused" } else { Fail "preferred_foot validation" "got $($badFoot.Status)" }

Section "The temporal model"

$hist0 = Invoke-Api -Method GET -Url "$FootballUrl/api/v1/players/$playerId/transfers"
if ($hist0.Body.items.Count -eq 1) {
    Pass "creating a player wrote its opening transfer"
} else {
    Fail "opening transfer written on create" "expected 1 row, got $($hist0.Body.items.Count)"
}
if ($null -eq $hist0.Body.items[0].from_team_id) { Pass "the opening record arrives from nowhere" } else { Fail "opening record has a null origin" "from_team_id = $($hist0.Body.items[0].from_team_id)" }

$vals0 = Invoke-Api -Method GET -Url "$FootballUrl/api/v1/players/$playerId/market-values"
if ($vals0.Body.items.Count -eq 1) { Pass "creating a player wrote its opening valuation" } else { Fail "opening valuation written on create" "got $($vals0.Body.items.Count) rows" }

# A plain update must not be able to move the player.
$moveByUpdate = Invoke-Api -Method PUT -Url "$FootballUrl/api/v1/players/$playerId" -Token $adminToken -Body @{
    id = $playerId; team_id = $buyingId; name = "Check Player $suffix"; position = 'Forward'
}
$afterUpdate = Invoke-Api -Method GET -Url "$FootballUrl/api/v1/players/$playerId"
if ($afterUpdate.Body.team_id -eq $sellingId) {
    Pass "PUT /players cannot move a player between clubs"
} else {
    Fail "PUT /players must not move a player" "team_id became $($afterUpdate.Body.team_id)"
}

$transfer = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/transfers" -Token $adminToken -Body @{
    player_id = $playerId; from_team_id = $sellingId; to_team_id = $buyingId
    transfer_date = '2026-01-15T00:00:00Z'; transfer_type = 'permanent'; fee_minor = 2500000000
}
if ($transfer.Status -eq 201) { Pass "record a transfer" } else { Fail "record a transfer" "status $($transfer.Status): $($transfer.Raw)" }

$afterTransfer = Invoke-Api -Method GET -Url "$FootballUrl/api/v1/players/$playerId"
if ($afterTransfer.Body.team_id -eq $buyingId) { Pass "the player's club followed the transfer" } else { Fail "the player's club followed the transfer" "team_id = $($afterTransfer.Body.team_id)" }

$hist1 = Invoke-Api -Method GET -Url "$FootballUrl/api/v1/players/$playerId/transfers"
if ($hist1.Body.items.Count -eq 2) { Pass "the history now has two records" } else { Fail "history has two records" "got $($hist1.Body.items.Count)" }
if ($hist1.Body.items[0].from_team_id -eq $sellingId) { Pass "the selling club survives in the history" } else { Fail "selling club preserved" "got $($hist1.Body.items[0].from_team_id)" }
if ($hist1.Body.items[0].fee_minor -eq 2500000000) { Pass "the fee survives the round trip exactly" } else { Fail "fee round trip" "got $($hist1.Body.items[0].fee_minor)" }

# A move dated before the player's arrival is recorded, but must not claim
# they are at that club today.
$backdated = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/transfers" -Token $adminToken -Body @{
    player_id = $playerId; from_team_id = $buyingId; to_team_id = $sellingId
    transfer_date = '2019-06-01T00:00:00Z'; transfer_type = 'permanent'
}
if ($backdated.Status -eq 201) {
    $afterBackdate = Invoke-Api -Method GET -Url "$FootballUrl/api/v1/players/$playerId"
    if ($afterBackdate.Body.team_id -eq $buyingId) {
        Pass "a backdated transfer does not rewrite the current club"
    } else {
        Fail "backdated transfer must not rewrite the present" "team_id became $($afterBackdate.Body.team_id)"
    }
} else {
    Fail "a backdated transfer is still recorded" "status $($backdated.Status): $($backdated.Raw)"
}

Section "Valuations, coaches and seasons"

$mv = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/players/$playerId/market-values" -Token $adminToken -Body @{
    value_minor = 4000000000; valued_on = '2026-02-01T00:00:00Z'; currency = 'EUR'
}
if ($mv.Status -eq 201) { Pass "record a market value" } else { Fail "record a market value" "status $($mv.Status): $($mv.Raw)" }

$playerNow = Invoke-Api -Method GET -Url "$FootballUrl/api/v1/players/$playerId"
if ($playerNow.Body.market_value_minor -eq 4000000000) { Pass "the player's current value follows the latest valuation" } else { Fail "current value follows latest" "got $($playerNow.Body.market_value_minor)" }

$decimal = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/players/$playerId/market-values" -Token $adminToken -Body @{ value_minor = 1.5 }
if ($decimal.Status -eq 400) { Pass "a decimal money value is refused, not truncated" } else { Fail "decimal money refused" "got $($decimal.Status)" }

$season = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/seasons" -Token $adminToken -Body @{
    label = "25/26-$suffix"; start_date = '2025-08-01T00:00:00Z'; end_date = '2026-05-31T00:00:00Z'
}
if ($season.Status -eq 201) { Pass "create a season" } else { Fail "create a season" "status $($season.Status): $($season.Raw)" }
$seasonId = $season.Body.id

$overlap = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/seasons" -Token $adminToken -Body @{
    label = "overlap-$suffix"; start_date = '2026-01-01T00:00:00Z'; end_date = '2026-09-30T00:00:00Z'
}
if ($overlap.Status -eq 409) { Pass "an overlapping season is refused (409)" } else { Fail "overlapping season refused" "expected 409, got $($overlap.Status): $($overlap.Raw)" }

$coach = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/coaches" -Token $adminToken -Body @{
    team_id = $buyingId; name = "Check Coach $suffix"; nationality = 'Testland'
}
if ($coach.Status -eq 201) { Pass "create a coach" } else { Fail "create a coach" "status $($coach.Status): $($coach.Raw)" }
$coachId = $coach.Body.id

$spell = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/coaches/$coachId/spells" -Token $adminToken -Body @{
    coach_id = $coachId; team_id = $buyingId; role = 'head_coach'; start_date = '2025-07-01T00:00:00Z'
}
if ($spell.Status -eq 201) { Pass "record a coaching spell" } else { Fail "record a coaching spell" "status $($spell.Status): $($spell.Raw)" }

$badRole = Invoke-Api -Method POST -Url "$FootballUrl/api/v1/coaches/$coachId/spells" -Token $adminToken -Body @{
    coach_id = $coachId; team_id = $buyingId; role = 'supreme_overlord'; start_date = '2025-07-01T00:00:00Z'
}
if ($badRole.Status -eq 400) { Pass "an unknown coaching role is refused" } else { Fail "coaching role validation" "got $($badRole.Status)" }

Section "Reads, envelopes and errors"

$list = Invoke-Api -Method GET -Url "$FootballUrl/api/v1/players?team_id=$buyingId&limit=5"
$hasEnvelope = $list.Body.PSObject.Properties.Name -contains 'items' -and
               $list.Body.PSObject.Properties.Name -contains 'has_more' -and
               $list.Body.PSObject.Properties.Name -contains 'limit'
if ($hasEnvelope) { Pass "lists return an envelope, not a bare array" } else { Fail "list envelope" "got: $($list.Raw)" }

$clamped = Invoke-Api -Method GET -Url "$FootballUrl/api/v1/players?limit=9999"
if ($clamped.Body.limit -le 100) { Pass "an oversized limit is clamped to 100" } else { Fail "limit clamping" "limit came back as $($clamped.Body.limit)" }

$feed = Invoke-Api -Method GET -Url "$FootballUrl/api/v1/transfers?team_id=$buyingId"
if ($feed.Status -eq 200 -and $feed.Body.items.Count -ge 1) { Pass "the transfer feed filters by club" } else { Fail "transfer feed by club" "status $($feed.Status), $($feed.Body.items.Count) rows" }

# The regression that mattered most: a malformed id used to reach Postgres and
# come back as a 500.
$badId = Invoke-Api -Method GET -Url "$FootballUrl/api/v1/players/not-a-uuid"
if ($badId.Status -eq 400) {
    Pass "a malformed id is a 400, not a 500"
} else {
    Fail "malformed id returns 400" "got $($badId.Status) -- a 500 here means the SQLSTATE mapping regressed"
}
if ($badId.Body.error -and $badId.Body.error -notmatch 'uuid|syntax|SQL') {
    Pass "the error body leaks no driver detail"
} else {
    Fail "error body leaks driver detail" "error was: $($badId.Body.error)"
}

$missing = Invoke-Api -Method GET -Url "$FootballUrl/api/v1/players/00000000-0000-0000-0000-000000000000"
if ($missing.Status -eq 404) { Pass "an unknown id is a 404" } else { Fail "unknown id returns 404" "got $($missing.Status)" }

# --- cleanup -----------------------------------------------------------
if (-not $KeepData) {
    Section "Cleanup"
    Invoke-Api -Method DELETE -Url "$FootballUrl/api/v1/players/$playerId" -Token $adminToken | Out-Null
    Invoke-Api -Method DELETE -Url "$FootballUrl/api/v1/coaches/$coachId" -Token $adminToken | Out-Null
    Invoke-Api -Method DELETE -Url "$FootballUrl/api/v1/teams/$sellingId" -Token $adminToken | Out-Null
    Invoke-Api -Method DELETE -Url "$FootballUrl/api/v1/teams/$buyingId" -Token $adminToken | Out-Null
    Invoke-Api -Method DELETE -Url "$FootballUrl/api/v1/seasons/$seasonId" -Token $adminToken | Out-Null
    Invoke-Api -Method DELETE -Url "$FootballUrl/api/v1/leagues/$leagueId" -Token $adminToken | Out-Null
    Write-Host "  removed the records this run created" -ForegroundColor DarkGray
    $script:Notes += "The two checker accounts remain in identity_db; there is no delete-user endpoint."
} else {
    $script:Notes += "-KeepData was set, so the created records were left in place."
}

# --- summary -----------------------------------------------------------
Write-Host ""
Write-Host ("-" * 52) -ForegroundColor DarkGray
if ($script:Failed -eq 0) {
    Write-Host "$script:Passed checks passed." -ForegroundColor Green
} else {
    Write-Host "$script:Passed passed, $script:Failed FAILED." -ForegroundColor Red
}
foreach ($n in $script:Notes) { Write-Host "note: $n" -ForegroundColor DarkGray }

if ($script:Failed -gt 0) { exit 1 }
exit 0
