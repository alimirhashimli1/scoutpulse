<#
.SYNOPSIS
  Fills a local ScoutPulse database with a demo dataset: five leagues, three
  clubs each, three players per club, and a head coach per club.

.DESCRIPTION
  Talks to the two services directly (identity 8080, football 8081), the same
  way api.http does, so it needs .\scripts\dev-run.ps1 running for both.

  Safe to re-run. Every step looks the entity up by name first and creates only
  what is missing, so a second run reports "exists" rather than producing
  duplicates or a wall of 409s.

  Everything created here is admin-only at the API, so the account used must
  hold the admin role.

.EXAMPLE
  .\scripts\seed-demo.ps1 -User aligsbsu1 -Pass 'your-password'
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)] [string] $User,
    [Parameter(Mandatory = $true)] [string] $Pass,
    [string] $Identity = 'http://localhost:8080',
    [string] $Football = 'http://localhost:8081'
)

$ErrorActionPreference = 'Stop'

# PowerShell 5.1 sends a string body as ISO-8859-1, which turns "München" into
# mojibake on the way in. Encoding to UTF-8 bytes here is the fix, and it has
# to be done on every write rather than once.
function Invoke-Json {
    param([string] $Method, [string] $Url, $Body, $Headers)
    $call = @{ Uri = $Url; Method = $Method; ContentType = 'application/json; charset=utf-8' }
    if ($Headers) { $call.Headers = $Headers }
    if ($null -ne $Body) {
        $json = $Body | ConvertTo-Json -Depth 6 -Compress
        $call.Body = [System.Text.Encoding]::UTF8.GetBytes($json)
    }
    return Invoke-RestMethod @call
}

Write-Host "Signing in as $User..." -ForegroundColor Cyan
$login = Invoke-Json -Method Post -Url "$Identity/api/v1/auth/login" -Body @{ identifier = $User; password = $Pass }
$H = @{ Authorization = "Bearer $($login.access_token)" }

$me = Invoke-RestMethod -Uri "$Identity/api/v1/users/me" -Headers $H
if ($me.role -ne 'admin') {
    throw "$User has role '$($me.role)'. Everything this script creates is admin-only."
}
Write-Host "  signed in, role=$($me.role)" -ForegroundColor DarkGray

$created  = @{ season = 0; leagues = 0; teams = 0; players = 0; coaches = 0; spells = 0; entries = 0 }
$existing = @{ season = 0; leagues = 0; teams = 0; players = 0; coaches = 0 }

# --- season ---------------------------------------------------------------
# Created first, so the opening transfer each player gets on creation is filed
# against it. A missing season is not an error; it just leaves moves unfiled.
$seasonLabel = '2026/27'
$season = (Invoke-RestMethod "$Football/api/v1/seasons?limit=100").items |
    Where-Object { $_.label -eq $seasonLabel } | Select-Object -First 1
if ($season) {
    Write-Host "season $seasonLabel exists" -ForegroundColor DarkGray
    $existing.season++
} else {
    $season = Invoke-Json -Method Post -Url "$Football/api/v1/seasons" -Headers $H -Body @{
        label = $seasonLabel; start_date = '2026-07-01T00:00:00Z'; end_date = '2027-06-30T00:00:00Z'
    }
    Write-Host "season $seasonLabel created" -ForegroundColor Green
    $created.season++
}

# --- the dataset ----------------------------------------------------------
# Squads and coaching appointments are as good as the author's knowledge and
# will drift; this is demo data for exercising the app, not a source of truth.
$data = @(
  @{ league = @{ name = 'Premier League'; country = 'England'; tier = 1 }
     clubs = @(
       @{ name = 'Manchester City'; short_name = 'Man City'; city = 'Manchester'; country = 'England'; stadium = 'Etihad Stadium'; founded_year = 1880
          coach = @{ name = 'Pep Guardiola'; nationality = 'Spain' }
          players = @(
            @{ name = 'Erling Haaland'; position = 'Centre-Forward'; nationality = 'Norway'; squad_number = 9; preferred_foot = 'left'; date_of_birth = '2000-07-21T00:00:00Z'; market_value_minor = 18000000000 }
            @{ name = 'Phil Foden'; position = 'Attacking Midfield'; nationality = 'England'; squad_number = 47; preferred_foot = 'left'; date_of_birth = '2000-05-28T00:00:00Z'; market_value_minor = 11000000000 }
            @{ name = 'Rodri'; position = 'Defensive Midfield'; nationality = 'Spain'; squad_number = 16; preferred_foot = 'right'; date_of_birth = '1996-06-22T00:00:00Z'; market_value_minor = 11000000000 }
          ) }
       @{ name = 'Arsenal FC'; short_name = 'Arsenal'; city = 'London'; country = 'England'; stadium = 'Emirates Stadium'; founded_year = 1886
          coach = @{ name = 'Mikel Arteta'; nationality = 'Spain' }
          players = @(
            @{ name = 'Bukayo Saka'; position = 'Right Winger'; nationality = 'England'; squad_number = 7; preferred_foot = 'left'; date_of_birth = '2001-09-05T00:00:00Z'; market_value_minor = 14000000000 }
            @{ name = 'Martin Ødegaard'; position = 'Attacking Midfield'; nationality = 'Norway'; squad_number = 8; preferred_foot = 'left'; date_of_birth = '1998-12-17T00:00:00Z'; market_value_minor = 10000000000 }
            @{ name = 'William Saliba'; position = 'Centre-Back'; nationality = 'France'; squad_number = 2; preferred_foot = 'right'; date_of_birth = '2001-03-24T00:00:00Z'; market_value_minor = 8000000000 }
          ) }
       @{ name = 'Liverpool FC'; short_name = 'Liverpool'; city = 'Liverpool'; country = 'England'; stadium = 'Anfield'; founded_year = 1892
          coach = @{ name = 'Arne Slot'; nationality = 'Netherlands' }
          players = @(
            @{ name = 'Mohamed Salah'; position = 'Right Winger'; nationality = 'Egypt'; squad_number = 11; preferred_foot = 'left'; date_of_birth = '1992-06-15T00:00:00Z'; market_value_minor = 5500000000 }
            @{ name = 'Virgil van Dijk'; position = 'Centre-Back'; nationality = 'Netherlands'; squad_number = 4; preferred_foot = 'right'; date_of_birth = '1991-07-08T00:00:00Z'; market_value_minor = 2800000000 }
            @{ name = 'Alexis Mac Allister'; position = 'Central Midfield'; nationality = 'Argentina'; squad_number = 10; preferred_foot = 'right'; date_of_birth = '1998-12-24T00:00:00Z'; market_value_minor = 7000000000 }
          ) }
     ) }

  @{ league = @{ name = 'LaLiga'; country = 'Spain'; tier = 1 }
     clubs = @(
       @{ name = 'Real Madrid'; short_name = 'Real Madrid'; city = 'Madrid'; country = 'Spain'; stadium = 'Estadio Santiago Bernabéu'; founded_year = 1902
          coach = @{ name = 'Xabi Alonso'; nationality = 'Spain' }
          players = @(
            @{ name = 'Kylian Mbappé'; position = 'Centre-Forward'; nationality = 'France'; squad_number = 9; preferred_foot = 'right'; date_of_birth = '1998-12-20T00:00:00Z'; market_value_minor = 18000000000 }
            @{ name = 'Jude Bellingham'; position = 'Attacking Midfield'; nationality = 'England'; squad_number = 5; preferred_foot = 'right'; date_of_birth = '2003-06-29T00:00:00Z'; market_value_minor = 18000000000 }
            @{ name = 'Vinícius Júnior'; position = 'Left Winger'; nationality = 'Brazil'; squad_number = 7; preferred_foot = 'right'; date_of_birth = '2000-07-12T00:00:00Z'; market_value_minor = 17000000000 }
          ) }
       @{ name = 'FC Barcelona'; short_name = 'Barcelona'; city = 'Barcelona'; country = 'Spain'; stadium = 'Spotify Camp Nou'; founded_year = 1899
          coach = @{ name = 'Hansi Flick'; nationality = 'Germany' }
          players = @(
            @{ name = 'Lamine Yamal'; position = 'Right Winger'; nationality = 'Spain'; squad_number = 10; preferred_foot = 'left'; date_of_birth = '2007-07-13T00:00:00Z'; market_value_minor = 20000000000 }
            @{ name = 'Pedri'; position = 'Central Midfield'; nationality = 'Spain'; squad_number = 8; preferred_foot = 'right'; date_of_birth = '2002-11-25T00:00:00Z'; market_value_minor = 14000000000 }
            @{ name = 'Robert Lewandowski'; position = 'Centre-Forward'; nationality = 'Poland'; squad_number = 9; preferred_foot = 'right'; date_of_birth = '1988-08-21T00:00:00Z'; market_value_minor = 1500000000 }
          ) }
       @{ name = 'Atlético de Madrid'; short_name = 'Atlético'; city = 'Madrid'; country = 'Spain'; stadium = 'Riyadh Air Metropolitano'; founded_year = 1903
          coach = @{ name = 'Diego Simeone'; nationality = 'Argentina' }
          players = @(
            @{ name = 'Julián Álvarez'; position = 'Centre-Forward'; nationality = 'Argentina'; squad_number = 19; preferred_foot = 'right'; date_of_birth = '2000-01-31T00:00:00Z'; market_value_minor = 9000000000 }
            @{ name = 'Antoine Griezmann'; position = 'Second Striker'; nationality = 'France'; squad_number = 7; preferred_foot = 'left'; date_of_birth = '1991-03-21T00:00:00Z'; market_value_minor = 1500000000 }
            @{ name = 'Jan Oblak'; position = 'Goalkeeper'; nationality = 'Slovenia'; squad_number = 13; preferred_foot = 'right'; date_of_birth = '1993-01-07T00:00:00Z'; market_value_minor = 2000000000 }
          ) }
     ) }

  @{ league = @{ name = 'Bundesliga'; country = 'Germany'; tier = 1 }
     clubs = @(
       @{ name = 'FC Bayern München'; short_name = 'Bayern'; city = 'Munich'; country = 'Germany'; stadium = 'Allianz Arena'; founded_year = 1900
          coach = @{ name = 'Vincent Kompany'; nationality = 'Belgium' }
          players = @(
            @{ name = 'Harry Kane'; position = 'Centre-Forward'; nationality = 'England'; squad_number = 9; preferred_foot = 'right'; date_of_birth = '1993-07-28T00:00:00Z'; market_value_minor = 9000000000 }
            @{ name = 'Jamal Musiala'; position = 'Attacking Midfield'; nationality = 'Germany'; squad_number = 42; preferred_foot = 'right'; date_of_birth = '2003-02-26T00:00:00Z'; market_value_minor = 14000000000 }
            @{ name = 'Joshua Kimmich'; position = 'Defensive Midfield'; nationality = 'Germany'; squad_number = 6; preferred_foot = 'right'; date_of_birth = '1995-02-08T00:00:00Z'; market_value_minor = 5000000000 }
          ) }
       @{ name = 'Borussia Dortmund'; short_name = 'Dortmund'; city = 'Dortmund'; country = 'Germany'; stadium = 'Signal Iduna Park'; founded_year = 1909
          coach = @{ name = 'Niko Kovač'; nationality = 'Croatia' }
          players = @(
            @{ name = 'Serhou Guirassy'; position = 'Centre-Forward'; nationality = 'Guinea'; squad_number = 9; preferred_foot = 'right'; date_of_birth = '1996-03-12T00:00:00Z'; market_value_minor = 4000000000 }
            @{ name = 'Julian Brandt'; position = 'Attacking Midfield'; nationality = 'Germany'; squad_number = 19; preferred_foot = 'left'; date_of_birth = '1996-05-02T00:00:00Z'; market_value_minor = 2500000000 }
            @{ name = 'Nico Schlotterbeck'; position = 'Centre-Back'; nationality = 'Germany'; squad_number = 4; preferred_foot = 'left'; date_of_birth = '1999-12-01T00:00:00Z'; market_value_minor = 4000000000 }
          ) }
       @{ name = 'Bayer 04 Leverkusen'; short_name = 'Leverkusen'; city = 'Leverkusen'; country = 'Germany'; stadium = 'BayArena'; founded_year = 1904
          coach = @{ name = 'Kasper Hjulmand'; nationality = 'Denmark' }
          players = @(
            @{ name = 'Patrik Schick'; position = 'Centre-Forward'; nationality = 'Czech Republic'; squad_number = 14; preferred_foot = 'left'; date_of_birth = '1996-01-24T00:00:00Z'; market_value_minor = 3000000000 }
            @{ name = 'Alejandro Grimaldo'; position = 'Left-Back'; nationality = 'Spain'; squad_number = 20; preferred_foot = 'left'; date_of_birth = '1995-09-20T00:00:00Z'; market_value_minor = 3500000000 }
            @{ name = 'Exequiel Palacios'; position = 'Central Midfield'; nationality = 'Argentina'; squad_number = 25; preferred_foot = 'right'; date_of_birth = '1998-10-05T00:00:00Z'; market_value_minor = 3000000000 }
          ) }
     ) }

  @{ league = @{ name = 'Serie A'; country = 'Italy'; tier = 1 }
     clubs = @(
       @{ name = 'Inter Milan'; short_name = 'Inter'; city = 'Milan'; country = 'Italy'; stadium = 'Giuseppe Meazza'; founded_year = 1908
          coach = @{ name = 'Cristian Chivu'; nationality = 'Romania' }
          players = @(
            @{ name = 'Lautaro Martínez'; position = 'Centre-Forward'; nationality = 'Argentina'; squad_number = 10; preferred_foot = 'right'; date_of_birth = '1997-08-22T00:00:00Z'; market_value_minor = 9000000000 }
            @{ name = 'Nicolò Barella'; position = 'Central Midfield'; nationality = 'Italy'; squad_number = 23; preferred_foot = 'right'; date_of_birth = '1997-02-07T00:00:00Z'; market_value_minor = 7000000000 }
            @{ name = 'Alessandro Bastoni'; position = 'Centre-Back'; nationality = 'Italy'; squad_number = 95; preferred_foot = 'left'; date_of_birth = '1999-04-13T00:00:00Z'; market_value_minor = 6500000000 }
          ) }
       @{ name = 'AC Milan'; short_name = 'Milan'; city = 'Milan'; country = 'Italy'; stadium = 'Giuseppe Meazza'; founded_year = 1899
          coach = @{ name = 'Massimiliano Allegri'; nationality = 'Italy' }
          players = @(
            @{ name = 'Rafael Leão'; position = 'Left Winger'; nationality = 'Portugal'; squad_number = 10; preferred_foot = 'right'; date_of_birth = '1999-06-10T00:00:00Z'; market_value_minor = 7500000000 }
            @{ name = 'Christian Pulisic'; position = 'Right Winger'; nationality = 'United States'; squad_number = 11; preferred_foot = 'right'; date_of_birth = '1998-09-18T00:00:00Z'; market_value_minor = 4500000000 }
            @{ name = 'Mike Maignan'; position = 'Goalkeeper'; nationality = 'France'; squad_number = 16; preferred_foot = 'right'; date_of_birth = '1995-07-03T00:00:00Z'; market_value_minor = 3000000000 }
          ) }
       @{ name = 'SSC Napoli'; short_name = 'Napoli'; city = 'Naples'; country = 'Italy'; stadium = 'Stadio Diego Armando Maradona'; founded_year = 1926
          coach = @{ name = 'Antonio Conte'; nationality = 'Italy' }
          players = @(
            @{ name = 'Kevin De Bruyne'; position = 'Attacking Midfield'; nationality = 'Belgium'; squad_number = 11; preferred_foot = 'right'; date_of_birth = '1991-06-28T00:00:00Z'; market_value_minor = 2000000000 }
            @{ name = 'Scott McTominay'; position = 'Central Midfield'; nationality = 'Scotland'; squad_number = 8; preferred_foot = 'right'; date_of_birth = '1996-12-08T00:00:00Z'; market_value_minor = 4500000000 }
            @{ name = 'Alex Meret'; position = 'Goalkeeper'; nationality = 'Italy'; squad_number = 1; preferred_foot = 'right'; date_of_birth = '1997-03-22T00:00:00Z'; market_value_minor = 1200000000 }
          ) }
     ) }

  @{ league = @{ name = 'Ligue 1'; country = 'France'; tier = 1 }
     clubs = @(
       @{ name = 'Paris Saint-Germain'; short_name = 'PSG'; city = 'Paris'; country = 'France'; stadium = 'Parc des Princes'; founded_year = 1970
          coach = @{ name = 'Luis Enrique'; nationality = 'Spain' }
          players = @(
            @{ name = 'Ousmane Dembélé'; position = 'Right Winger'; nationality = 'France'; squad_number = 10; preferred_foot = 'both'; date_of_birth = '1997-05-15T00:00:00Z'; market_value_minor = 9000000000 }
            @{ name = 'Vitinha'; position = 'Central Midfield'; nationality = 'Portugal'; squad_number = 17; preferred_foot = 'right'; date_of_birth = '2000-02-13T00:00:00Z'; market_value_minor = 9000000000 }
            @{ name = 'Achraf Hakimi'; position = 'Right-Back'; nationality = 'Morocco'; squad_number = 2; preferred_foot = 'right'; date_of_birth = '1998-11-04T00:00:00Z'; market_value_minor = 6500000000 }
          ) }
       @{ name = 'Olympique de Marseille'; short_name = 'Marseille'; city = 'Marseille'; country = 'France'; stadium = 'Orange Vélodrome'; founded_year = 1899
          coach = @{ name = 'Roberto De Zerbi'; nationality = 'Italy' }
          players = @(
            @{ name = 'Mason Greenwood'; position = 'Right Winger'; nationality = 'England'; squad_number = 10; preferred_foot = 'left'; date_of_birth = '2001-10-01T00:00:00Z'; market_value_minor = 4000000000 }
            @{ name = 'Pierre-Emerick Aubameyang'; position = 'Centre-Forward'; nationality = 'Gabon'; squad_number = 9; preferred_foot = 'right'; date_of_birth = '1989-06-18T00:00:00Z'; market_value_minor = 500000000 }
            @{ name = 'Leonardo Balerdi'; position = 'Centre-Back'; nationality = 'Argentina'; squad_number = 5; preferred_foot = 'right'; date_of_birth = '1999-01-26T00:00:00Z'; market_value_minor = 2500000000 }
          ) }
       @{ name = 'AS Monaco'; short_name = 'Monaco'; city = 'Monaco'; country = 'Monaco'; stadium = 'Stade Louis II'; founded_year = 1924
          coach = @{ name = 'Adi Hütter'; nationality = 'Austria' }
          players = @(
            @{ name = 'Maghnes Akliouche'; position = 'Attacking Midfield'; nationality = 'France'; squad_number = 8; preferred_foot = 'right'; date_of_birth = '2002-02-25T00:00:00Z'; market_value_minor = 4000000000 }
            @{ name = 'Denis Zakaria'; position = 'Defensive Midfield'; nationality = 'Switzerland'; squad_number = 20; preferred_foot = 'right'; date_of_birth = '1996-11-20T00:00:00Z'; market_value_minor = 3000000000 }
            @{ name = 'Folarin Balogun'; position = 'Centre-Forward'; nationality = 'United States'; squad_number = 29; preferred_foot = 'right'; date_of_birth = '2001-07-03T00:00:00Z'; market_value_minor = 2500000000 }
          ) }
     ) }
)

foreach ($block in $data) {
    # --- league ---
    $l = $block.league
    $league = (Invoke-RestMethod "$Football/api/v1/leagues?limit=100").items |
        Where-Object { $_.name -eq $l.name } | Select-Object -First 1
    if ($league) {
        Write-Host "league   $($l.name) exists" -ForegroundColor DarkGray
        $existing.leagues++
    } else {
        $league = Invoke-Json -Method Post -Url "$Football/api/v1/leagues" -Headers $H -Body @{
            name = $l.name; country = $l.country; tier = $l.tier; competition_type = 'league'
        }
        Write-Host "league   $($l.name)" -ForegroundColor Green
        $created.leagues++
    }

    foreach ($c in $block.clubs) {
        # --- club ---
        $team = (Invoke-RestMethod "$Football/api/v1/teams?limit=100").items |
            Where-Object { $_.name -eq $c.name } | Select-Object -First 1
        if ($team) {
            Write-Host "  club   $($c.name) exists" -ForegroundColor DarkGray
            $existing.teams++
        } else {
            $team = Invoke-Json -Method Post -Url "$Football/api/v1/teams" -Headers $H -Body @{
                name = $c.name; short_name = $c.short_name; league_id = $league.id
                city = $c.city; country = $c.country; stadium = $c.stadium; founded_year = $c.founded_year
            }
            Write-Host "  club   $($c.name)" -ForegroundColor Green
            $created.teams++
        }

        # Enter the club in this season's competition. That is the history the
        # club page reads back; league_id alone only says where they are now.
        # A repeat run hits the unique constraint, which is the intended no-op.
        try {
            Invoke-Json -Method Post -Url "$Football/api/v1/teams/$($team.id)/seasons" -Headers $H -Body @{
                team_id = $team.id; season_id = $season.id; league_id = $league.id
            } | Out-Null
            $created.entries++
        } catch { }

        # --- players ---
        # team_id on create is meaningful: the repository records an opening
        # transfer into that club and an opening valuation. It is ignored on
        # update, which is why the edit form has no club field.
        $squad = (Invoke-RestMethod "$Football/api/v1/players?team_id=$($team.id)&limit=100").items
        foreach ($p in $c.players) {
            if ($squad | Where-Object { $_.name -eq $p.name }) {
                Write-Host "    player $($p.name) exists" -ForegroundColor DarkGray
                $existing.players++
                continue
            }
            $body = @{ team_id = $team.id } + $p
            Invoke-Json -Method Post -Url "$Football/api/v1/players" -Headers $H -Body $body | Out-Null
            Write-Host "    player $($p.name)" -ForegroundColor Green
            $created.players++
        }

        # --- head coach ---
        $coach = $null
        try { $coach = Invoke-RestMethod "$Football/api/v1/coaches?team_id=$($team.id)" } catch { }
        if ($coach -and $coach.name -eq $c.coach.name) {
            Write-Host "    coach  $($c.coach.name) exists" -ForegroundColor DarkGray
            $existing.coaches++
        } else {
            $coach = Invoke-Json -Method Post -Url "$Football/api/v1/coaches" -Headers $H -Body @{
                name = $c.coach.name; nationality = $c.coach.nationality; team_id = $team.id
            }
            Write-Host "    coach  $($c.coach.name)" -ForegroundColor Green
            $created.coaches++

            # A coach's club is derived from their spells, so the appointment is
            # recorded as well as the pointer. An open spell — no end_date — is
            # what makes this their current club.
            Invoke-Json -Method Post -Url "$Football/api/v1/coaches/$($coach.id)/spells" -Headers $H -Body @{
                coach_id = $coach.id; team_id = $team.id; role = 'head_coach'; start_date = '2026-07-01T00:00:00Z'
            } | Out-Null
            $created.spells++
        }
    }
}

Write-Host ""
Write-Host "created:  $($created.leagues) leagues, $($created.teams) clubs, $($created.players) players, $($created.coaches) coaches, $($created.spells) spells, $($created.entries) season entries, $($created.season) season" -ForegroundColor Cyan
Write-Host "existing: $($existing.leagues) leagues, $($existing.teams) clubs, $($existing.players) players, $($existing.coaches) coaches" -ForegroundColor DarkGray
