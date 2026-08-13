DROP INDEX IF EXISTS idx_leagues_search;
DROP INDEX IF EXISTS idx_coaches_name_trgm;
DROP INDEX IF EXISTS idx_coaches_search;
DROP INDEX IF EXISTS idx_teams_name_trgm;
DROP INDEX IF EXISTS idx_teams_search;
DROP INDEX IF EXISTS idx_players_name_trgm;
DROP INDEX IF EXISTS idx_players_search;

ALTER TABLE leagues DROP COLUMN IF EXISTS search_document;
ALTER TABLE coaches DROP COLUMN IF EXISTS search_document;
ALTER TABLE teams   DROP COLUMN IF EXISTS search_document;
ALTER TABLE players DROP COLUMN IF EXISTS search_document;

-- pg_trgm is left installed. Dropping an extension something else may have
-- come to depend on is riskier than leaving it, and it is inert when unused.
