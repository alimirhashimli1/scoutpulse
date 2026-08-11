DROP INDEX IF EXISTS idx_players_market_value;
DROP INDEX IF EXISTS idx_players_nationality;
DROP INDEX IF EXISTS idx_team_seasons_league;
DROP INDEX IF EXISTS idx_team_seasons_season;
DROP INDEX IF EXISTS idx_coach_spells_team;
DROP INDEX IF EXISTS idx_coach_spells_coach;
DROP INDEX IF EXISTS idx_market_values_player;
DROP INDEX IF EXISTS idx_transfers_season;
DROP INDEX IF EXISTS idx_transfers_date;
DROP INDEX IF EXISTS idx_transfers_from_team;
DROP INDEX IF EXISTS idx_transfers_to_team;
DROP INDEX IF EXISTS idx_transfers_player_date;

DROP TABLE IF EXISTS coach_spells;
DROP TABLE IF EXISTS player_market_values;
DROP TABLE IF EXISTS transfers;
DROP TABLE IF EXISTS team_seasons;

ALTER TABLE coaches
    DROP COLUMN IF EXISTS nationality,
    DROP COLUMN IF EXISTS date_of_birth,
    DROP COLUMN IF EXISTS last_name,
    DROP COLUMN IF EXISTS first_name;

-- Restore the UNIQUE constraint the up migration dropped, so down really is
-- the inverse of up. Without this, a down-then-up cycle silently leaves the
-- schema different from where it started -- and CI's up/down/up check cannot
-- detect the difference, because re-applying the up migration drops the
-- constraint again either way.
--
-- Duplicate team_id values are possible once coach_spells has been in use, and
-- they would make the constraint unaddable. The later duplicates are released
-- to NULL, which is the reversible choice: it loses the current appointment,
-- which coach_spells recorded anyway, rather than deleting a coach.
UPDATE coaches SET team_id = NULL
 WHERE id IN (
     SELECT id FROM (
         SELECT id, ROW_NUMBER() OVER (PARTITION BY team_id ORDER BY created_at) AS rn
           FROM coaches
          WHERE team_id IS NOT NULL
     ) ranked
     WHERE ranked.rn > 1
 );

ALTER TABLE coaches DROP CONSTRAINT IF EXISTS coaches_team_id_key;
ALTER TABLE coaches ADD CONSTRAINT coaches_team_id_key UNIQUE (team_id);

-- Restore the float column. Precision lost in the minor-unit conversion cannot
-- be recovered, but the shape is restored.
ALTER TABLE players ADD COLUMN IF NOT EXISTS market_value NUMERIC(15, 2) NOT NULL DEFAULT 0;
UPDATE players SET market_value = market_value_minor / 100.0;

ALTER TABLE players DROP CONSTRAINT IF EXISTS players_market_value_non_negative;
ALTER TABLE players DROP CONSTRAINT IF EXISTS players_preferred_foot_valid;
ALTER TABLE players
    DROP COLUMN IF EXISTS currency,
    DROP COLUMN IF EXISTS market_value_minor,
    DROP COLUMN IF EXISTS contract_start,
    DROP COLUMN IF EXISTS squad_number,
    DROP COLUMN IF EXISTS agent,
    DROP COLUMN IF EXISTS preferred_foot,
    DROP COLUMN IF EXISTS height_cm,
    DROP COLUMN IF EXISTS second_nationality,
    DROP COLUMN IF EXISTS nationality,
    DROP COLUMN IF EXISTS date_of_birth,
    DROP COLUMN IF EXISTS last_name,
    DROP COLUMN IF EXISTS first_name;

ALTER TABLE teams
    DROP COLUMN IF EXISTS country,
    DROP COLUMN IF EXISTS city,
    DROP COLUMN IF EXISTS stadium,
    DROP COLUMN IF EXISTS founded_year,
    DROP COLUMN IF EXISTS short_name;

ALTER TABLE leagues DROP CONSTRAINT IF EXISTS leagues_competition_type_valid;
ALTER TABLE leagues
    DROP COLUMN IF EXISTS competition_type,
    DROP COLUMN IF EXISTS tier;

DROP TABLE IF EXISTS seasons;
