-- Match performance, and a way to load a real dataset without duplicating it.
--
-- Until now the only performance data was four hand-entered percentages. This
-- adds what a public dataset can actually supply -- appearances, minutes,
-- goals, assists and cards per season and competition -- plus a player's
-- international record.

-- ---------------------------------------------------------- season stats
-- One row per player, season, competition and club. The club is part of the
-- key because a player who moves in January has two lines for the same
-- league season, and adding them together would credit one club with goals
-- scored for the other.
--
-- team_id is nullable and SET NULL on delete: the club may be outside the
-- dataset (a season spent at a club since relegated out of it), and deleting
-- a club should not erase what its players did there.
--
-- These rows are written by the importer, not through the API. They are
-- aggregates of match data this system does not hold, so there is nothing an
-- editor could check a correction against.
CREATE TABLE IF NOT EXISTS player_season_stats (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id    UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    season_id    UUID NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    league_id    UUID NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    team_id      UUID REFERENCES teams(id) ON DELETE SET NULL,
    appearances  INTEGER NOT NULL DEFAULT 0,
    minutes      INTEGER NOT NULL DEFAULT 0,
    goals        INTEGER NOT NULL DEFAULT 0,
    assists      INTEGER NOT NULL DEFAULT 0,
    yellow_cards INTEGER NOT NULL DEFAULT 0,
    red_cards    INTEGER NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT player_season_stats_non_negative CHECK (
        appearances >= 0 AND minutes >= 0 AND goals >= 0 AND assists >= 0
        AND yellow_cards >= 0 AND red_cards >= 0)
);

-- The only read is "this player's seasons".
CREATE INDEX IF NOT EXISTS idx_player_season_stats_player
    ON player_season_stats (player_id);

-- ------------------------------------------------------ international record
-- Nullable: "not recorded" is the normal state for a player entered by hand,
-- and is a different fact from never having been capped.
ALTER TABLE players
    ADD COLUMN IF NOT EXISTS international_caps  INTEGER,
    ADD COLUMN IF NOT EXISTS international_goals INTEGER;

ALTER TABLE players DROP CONSTRAINT IF EXISTS players_international_non_negative;
ALTER TABLE players ADD CONSTRAINT players_international_non_negative CHECK (
    (international_caps  IS NULL OR international_caps  >= 0) AND
    (international_goals IS NULL OR international_goals >= 0));

-- ----------------------------------------------------------- external refs
-- Which local row an outside record became.
--
-- Re-running an import must update what the last run created rather than add
-- a second copy, and names are not keys: there are two Bundesligas, and more
-- than one player called Rodri. The source's own id is the only stable thing
-- to match on, and it belongs here rather than as a column on every table,
-- which would tie the schema to one provider.
--
-- No foreign key on local_id, because it points into whichever table `kind`
-- names. A ref whose row was deleted is treated by the importer as absent.
CREATE TABLE IF NOT EXISTS external_refs (
    source      TEXT NOT NULL,
    kind        TEXT NOT NULL,
    external_id TEXT NOT NULL,
    local_id    UUID NOT NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (source, kind, external_id)
);
