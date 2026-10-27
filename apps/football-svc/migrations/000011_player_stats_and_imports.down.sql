DROP TABLE IF EXISTS external_refs;

ALTER TABLE players DROP CONSTRAINT IF EXISTS players_international_non_negative;
ALTER TABLE players
    DROP COLUMN IF EXISTS international_goals,
    DROP COLUMN IF EXISTS international_caps;

DROP TABLE IF EXISTS player_season_stats;
