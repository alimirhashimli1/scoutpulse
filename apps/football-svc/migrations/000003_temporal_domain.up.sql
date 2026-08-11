-- Temporal domain model.
--
-- The original schema was a flat snapshot: a transfer overwrote players.team_id
-- and the previous fact was lost. This migration adds the history that a
-- Transfermarkt-style product is actually about -- transfers, market value over
-- time, and seasons -- while keeping the denormalised "current club" pointers
-- that reads and authorization depend on.
--
-- Source of truth vs. derived state:
--   transfers            is the source of truth for where a player has played.
--   players.team_id      is derived: maintained by the transfer flow.
--   player_market_values is the source of truth for valuation history.
--   players.market_value_minor is derived: the latest recorded value.
-- Derived columns exist so the common reads stay a single-table lookup.

-- ---------------------------------------------------------------- seasons

CREATE TABLE IF NOT EXISTS seasons (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label      VARCHAR(20)  NOT NULL UNIQUE,   -- e.g. '2025/26'
    start_date DATE         NOT NULL,
    end_date   DATE         NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT seasons_dates_ordered CHECK (end_date > start_date)
);

-- ----------------------------------------------------------- competitions

-- A league row is really a competition: leagues, domestic cups and continental
-- cups all need the same shape, and a club contests several in one season.
ALTER TABLE leagues
    ADD COLUMN IF NOT EXISTS tier             SMALLINT,
    ADD COLUMN IF NOT EXISTS competition_type VARCHAR(20) NOT NULL DEFAULT 'league';

ALTER TABLE leagues DROP CONSTRAINT IF EXISTS leagues_competition_type_valid;
ALTER TABLE leagues ADD CONSTRAINT leagues_competition_type_valid
    CHECK (competition_type IN ('league', 'domestic_cup', 'international_cup', 'super_cup'));

-- ------------------------------------------------------------------ clubs

ALTER TABLE teams
    ADD COLUMN IF NOT EXISTS short_name   VARCHAR(50),
    ADD COLUMN IF NOT EXISTS founded_year SMALLINT,
    ADD COLUMN IF NOT EXISTS stadium      VARCHAR(255),
    ADD COLUMN IF NOT EXISTS city         VARCHAR(255),
    ADD COLUMN IF NOT EXISTS country      VARCHAR(255);

-- Which competitions a club contests in a given season. teams.league_id
-- remains the club's current primary league for fast reads.
CREATE TABLE IF NOT EXISTS team_seasons (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id    UUID NOT NULL REFERENCES teams(id)   ON DELETE CASCADE,
    season_id  UUID NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    league_id  UUID NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (team_id, season_id, league_id)
);

-- ---------------------------------------------------------------- players

ALTER TABLE players
    ADD COLUMN IF NOT EXISTS first_name         VARCHAR(255),
    ADD COLUMN IF NOT EXISTS last_name          VARCHAR(255),
    ADD COLUMN IF NOT EXISTS date_of_birth      DATE,
    ADD COLUMN IF NOT EXISTS nationality        VARCHAR(255),
    ADD COLUMN IF NOT EXISTS second_nationality VARCHAR(255),
    ADD COLUMN IF NOT EXISTS height_cm          SMALLINT,
    ADD COLUMN IF NOT EXISTS preferred_foot     VARCHAR(10),
    ADD COLUMN IF NOT EXISTS agent              VARCHAR(255),
    ADD COLUMN IF NOT EXISTS squad_number       SMALLINT,
    ADD COLUMN IF NOT EXISTS contract_start     DATE;

ALTER TABLE players DROP CONSTRAINT IF EXISTS players_preferred_foot_valid;
ALTER TABLE players ADD CONSTRAINT players_preferred_foot_valid
    CHECK (preferred_foot IS NULL OR preferred_foot IN ('left', 'right', 'both'));

-- Money moves from NUMERIC-read-as-float64 to integer minor units. Binary
-- floating point cannot represent decimal currency exactly, and every
-- arithmetic step compounds the error.
ALTER TABLE players
    ADD COLUMN IF NOT EXISTS market_value_minor BIGINT   NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS currency           CHAR(3)  NOT NULL DEFAULT 'EUR';

-- Carry existing values across before the old column goes away.
UPDATE players
   SET market_value_minor = ROUND(market_value * 100)::BIGINT
 WHERE market_value IS NOT NULL
   AND market_value_minor = 0;

ALTER TABLE players DROP COLUMN IF EXISTS market_value;

ALTER TABLE players DROP CONSTRAINT IF EXISTS players_market_value_non_negative;
ALTER TABLE players ADD CONSTRAINT players_market_value_non_negative
    CHECK (market_value_minor >= 0);

-- -------------------------------------------------------------- transfers

-- The core historical record: every move a player has made.
CREATE TABLE IF NOT EXISTS transfers (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id     UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    -- A club may be deleted without erasing the transfer that references it.
    from_team_id  UUID REFERENCES teams(id)   ON DELETE SET NULL,
    to_team_id    UUID REFERENCES teams(id)   ON DELETE SET NULL,
    season_id     UUID REFERENCES seasons(id) ON DELETE SET NULL,
    transfer_date DATE        NOT NULL,
    transfer_type VARCHAR(20) NOT NULL,
    -- NULL fee means undisclosed, which is different from a free transfer.
    fee_minor     BIGINT,
    currency      CHAR(3)     NOT NULL DEFAULT 'EUR',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT transfers_type_valid CHECK (transfer_type IN (
        'permanent', 'loan', 'loan_return', 'free', 'youth_promotion', 'released', 'retired')),
    -- A transfer must move a player from somewhere or to somewhere.
    CONSTRAINT transfers_has_direction CHECK (from_team_id IS NOT NULL OR to_team_id IS NOT NULL),
    CONSTRAINT transfers_fee_non_negative CHECK (fee_minor IS NULL OR fee_minor >= 0)
);

-- Backfill: every player currently at a club gets an opening record, so the
-- history is not silently empty for pre-existing data.
INSERT INTO transfers (player_id, from_team_id, to_team_id, transfer_date, transfer_type)
SELECT p.id, NULL, p.team_id, p.created_at::DATE, 'permanent'
  FROM players p
 WHERE p.team_id IS NOT NULL
   AND NOT EXISTS (SELECT 1 FROM transfers t WHERE t.player_id = p.id);

-- --------------------------------------------------- market value history

CREATE TABLE IF NOT EXISTS player_market_values (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id   UUID   NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    valued_on   DATE   NOT NULL,
    value_minor BIGINT NOT NULL,
    currency    CHAR(3) NOT NULL DEFAULT 'EUR',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT player_market_values_non_negative CHECK (value_minor >= 0),
    -- One valuation per player per day; re-recording replaces it.
    UNIQUE (player_id, valued_on)
);

INSERT INTO player_market_values (player_id, valued_on, value_minor, currency)
SELECT p.id, p.created_at::DATE, p.market_value_minor, p.currency
  FROM players p
 WHERE p.market_value_minor > 0
ON CONFLICT (player_id, valued_on) DO NOTHING;

-- ---------------------------------------------------------------- coaches

ALTER TABLE coaches
    ADD COLUMN IF NOT EXISTS first_name    VARCHAR(255),
    ADD COLUMN IF NOT EXISTS last_name     VARCHAR(255),
    ADD COLUMN IF NOT EXISTS date_of_birth DATE,
    ADD COLUMN IF NOT EXISTS nationality   VARCHAR(255);

-- coaches.team_id was UNIQUE, which made it impossible to record that a club
-- has had more than one coach over time. History now lives in coach_spells and
-- the column stays as the current appointment.
ALTER TABLE coaches DROP CONSTRAINT IF EXISTS coaches_team_id_key;

CREATE TABLE IF NOT EXISTS coach_spells (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    coach_id   UUID NOT NULL REFERENCES coaches(id) ON DELETE CASCADE,
    team_id    UUID REFERENCES teams(id) ON DELETE SET NULL,
    role       VARCHAR(30) NOT NULL DEFAULT 'head_coach',
    start_date DATE NOT NULL,
    end_date   DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT coach_spells_dates_ordered CHECK (end_date IS NULL OR end_date >= start_date)
);

INSERT INTO coach_spells (coach_id, team_id, start_date)
SELECT c.id, c.team_id, c.created_at::DATE
  FROM coaches c
 WHERE c.team_id IS NOT NULL
   AND NOT EXISTS (SELECT 1 FROM coach_spells s WHERE s.coach_id = c.id);

-- ---------------------------------------------------------------- indexes

CREATE INDEX IF NOT EXISTS idx_transfers_player_date ON transfers (player_id, transfer_date DESC);
CREATE INDEX IF NOT EXISTS idx_transfers_to_team     ON transfers (to_team_id, transfer_date DESC);
CREATE INDEX IF NOT EXISTS idx_transfers_from_team   ON transfers (from_team_id, transfer_date DESC);
CREATE INDEX IF NOT EXISTS idx_transfers_date        ON transfers (transfer_date DESC);
CREATE INDEX IF NOT EXISTS idx_transfers_season      ON transfers (season_id);

CREATE INDEX IF NOT EXISTS idx_market_values_player  ON player_market_values (player_id, valued_on DESC);

CREATE INDEX IF NOT EXISTS idx_coach_spells_coach    ON coach_spells (coach_id, start_date DESC);
CREATE INDEX IF NOT EXISTS idx_coach_spells_team     ON coach_spells (team_id, start_date DESC);

CREATE INDEX IF NOT EXISTS idx_team_seasons_season   ON team_seasons (season_id);
CREATE INDEX IF NOT EXISTS idx_team_seasons_league   ON team_seasons (league_id, season_id);

CREATE INDEX IF NOT EXISTS idx_players_nationality   ON players (nationality);
CREATE INDEX IF NOT EXISTS idx_players_market_value  ON players (market_value_minor DESC);
