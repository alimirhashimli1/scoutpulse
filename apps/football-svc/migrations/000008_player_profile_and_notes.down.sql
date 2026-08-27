DROP INDEX IF EXISTS idx_player_notes_author;
DROP INDEX IF EXISTS idx_player_notes_player;
DROP TABLE IF EXISTS player_notes;

ALTER TABLE players DROP CONSTRAINT IF EXISTS players_percentages_in_range;

ALTER TABLE players
    DROP COLUMN IF EXISTS weaknesses,
    DROP COLUMN IF EXISTS strengths,
    DROP COLUMN IF EXISTS aerial_duels_won_pct,
    DROP COLUMN IF EXISTS shots_on_target_pct,
    DROP COLUMN IF EXISTS pass_completion_pct,
    DROP COLUMN IF EXISTS duels_won_pct;

-- Restore the two nationality columns and put the first two entries back, so
-- rolling back does not silently discard the data the up migration moved.
ALTER TABLE players ADD COLUMN IF NOT EXISTS nationality VARCHAR(100);
ALTER TABLE players ADD COLUMN IF NOT EXISTS second_nationality VARCHAR(100);

UPDATE players
   SET nationality        = NULLIF(nationalities[1], ''),
       second_nationality = NULLIF(nationalities[2], '')
 WHERE array_length(nationalities, 1) IS NOT NULL;

-- The generated search column depends on the array columns, so it has to go
-- before they can, and be rebuilt against the restored ones -- the mirror of
-- what the up migration does, and for the same reason.
DROP INDEX IF EXISTS idx_players_search;
ALTER TABLE players DROP COLUMN IF EXISTS search_document;

DROP INDEX IF EXISTS idx_players_nationalities;
ALTER TABLE players DROP COLUMN IF EXISTS nationalities;
ALTER TABLE players DROP COLUMN IF EXISTS secondary_positions;

ALTER TABLE players
    ADD COLUMN search_document tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(first_name, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(last_name, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(nationality, '')), 'C') ||
        setweight(to_tsvector('simple', coalesce(position, '')), 'C')
    ) STORED;

CREATE INDEX IF NOT EXISTS idx_players_search ON players USING GIN (search_document);
