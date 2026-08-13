-- Full-text search over players and clubs.
--
-- Until now the only way to reach a player was to know their club, and the
-- only way to reach a club was to know its competition. That is a workable
-- browse hierarchy but it is not how anyone actually uses a site like this --
-- they type a name.
--
-- Postgres FTS rather than a separate search service: the dataset is small
-- enough that a GIN index answers in single-digit milliseconds, and adding
-- Meilisearch or Elasticsearch would mean another container, another failure
-- mode, and keeping a second copy of the data in step. Revisit that when the
-- data outgrows this, not before.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ---------------------------------------------------------------- players

-- A generated column rather than a trigger-maintained one: Postgres keeps it
-- in step automatically, so it cannot drift when a row is updated by any
-- path that forgets to fire the trigger.
--
-- Weighting: the display name matters most, then the given and family names,
-- then nationality and position -- so searching "brazil forward" finds
-- Brazilian forwards without those fields outranking an actual name match.
ALTER TABLE players
    ADD COLUMN IF NOT EXISTS search_document tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(first_name, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(last_name, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(nationality, '')), 'C') ||
        setweight(to_tsvector('simple', coalesce(position, '')), 'C')
    ) STORED;

CREATE INDEX IF NOT EXISTS idx_players_search ON players USING GIN (search_document);

-- Trigram index for the partial and misspelt case. FTS matches whole lexemes,
-- so "messi" finds Messi but "mess" finds nothing; a trigram similarity search
-- covers the type-ahead behaviour people expect.
CREATE INDEX IF NOT EXISTS idx_players_name_trgm ON players USING GIN (name gin_trgm_ops);

-- ------------------------------------------------------------------ clubs

ALTER TABLE teams
    ADD COLUMN IF NOT EXISTS search_document tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(short_name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(city, '')), 'C') ||
        setweight(to_tsvector('simple', coalesce(country, '')), 'C') ||
        setweight(to_tsvector('simple', coalesce(stadium, '')), 'D')
    ) STORED;

CREATE INDEX IF NOT EXISTS idx_teams_search ON teams USING GIN (search_document);
CREATE INDEX IF NOT EXISTS idx_teams_name_trgm ON teams USING GIN (name gin_trgm_ops);

-- ---------------------------------------------------------------- coaches

ALTER TABLE coaches
    ADD COLUMN IF NOT EXISTS search_document tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(first_name, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(last_name, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(nationality, '')), 'C')
    ) STORED;

CREATE INDEX IF NOT EXISTS idx_coaches_search ON coaches USING GIN (search_document);
CREATE INDEX IF NOT EXISTS idx_coaches_name_trgm ON coaches USING GIN (name gin_trgm_ops);

-- ----------------------------------------------------------- competitions

ALTER TABLE leagues
    ADD COLUMN IF NOT EXISTS search_document tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(country, '')), 'B')
    ) STORED;

CREATE INDEX IF NOT EXISTS idx_leagues_search ON leagues USING GIN (search_document);

-- The 'simple' configuration is deliberate throughout. English stemming would
-- mangle proper nouns -- it strips what it takes for suffixes, so surnames
-- ending in -s, -ing or -ed match the wrong stem. Names are not English words,
-- and this dataset is multilingual by nature.
