-- Foreign keys are not indexed automatically in PostgreSQL, and these three
-- columns carry the service's hottest lookups: a team's squad, a league's
-- teams, and a team's head coach. Without them every such read is a
-- sequential scan over the whole table.
CREATE INDEX IF NOT EXISTS idx_players_team_id ON players (team_id);
CREATE INDEX IF NOT EXISTS idx_teams_league_id ON teams (league_id);

-- Free-agent listings filter on team_id IS NULL, which the index above does
-- not serve well once most players belong to a team. A partial index keeps
-- that query cheap while staying small.
CREATE INDEX IF NOT EXISTS idx_players_free_agents ON players (name) WHERE team_id IS NULL;

-- Position is the other supported filter on the player listing.
CREATE INDEX IF NOT EXISTS idx_players_position ON players (position);

-- List endpoints order by (name, id) so paging stays stable; matching indexes
-- let PostgreSQL satisfy the ORDER BY without a sort.
CREATE INDEX IF NOT EXISTS idx_players_name_id ON players (name, id);
CREATE INDEX IF NOT EXISTS idx_teams_name_id ON teams (name, id);
CREATE INDEX IF NOT EXISTS idx_leagues_name_id ON leagues (name, id);
