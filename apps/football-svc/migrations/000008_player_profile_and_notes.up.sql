-- Scouting detail on a player, and one note per member.

-- --------------------------------------------------------------- positions
-- The main position stays exactly where it is. This is the *other* positions
-- someone can fill, which is a different fact: a player has one listed
-- position and a set of roles they are usable in.
ALTER TABLE players
    ADD COLUMN IF NOT EXISTS secondary_positions TEXT[] NOT NULL DEFAULT '{}';

-- ------------------------------------------------------------ nationalities
-- Replaces nationality and second_nationality with one ordered list.
--
-- Two columns could only ever express two, and dual nationality is common
-- enough in football that the second column was already a workaround. An array
-- also removes the question of what "second" means when someone holds three.
-- The first entry is the primary one, which is what a profile shows.
ALTER TABLE players
    ADD COLUMN IF NOT EXISTS nationalities TEXT[] NOT NULL DEFAULT '{}';

-- Carry the existing values across before the old columns go. array_remove
-- drops the NULLs, so a player with no second nationality gets a one-element
-- list rather than one containing a hole.
UPDATE players
   SET nationalities = array_remove(ARRAY[nationality, second_nationality], NULL)
 WHERE nationalities = '{}';

-- The generated search column from migration 000007 reads `nationality`, and
-- Postgres refuses to drop a column a generated column depends on:
--
--   cannot drop column nationality of table players because other objects
--   depend on it
--
-- So it is dropped and rebuilt around the array. Rebuilding is cheap here and
-- unavoidable in any case: a search index still keyed on a column that no
-- longer exists could not be maintained.
DROP INDEX IF EXISTS idx_players_search;
ALTER TABLE players DROP COLUMN IF EXISTS search_document;

ALTER TABLE players DROP COLUMN IF EXISTS nationality;
ALTER TABLE players DROP COLUMN IF EXISTS second_nationality;

-- Same weighting as before: name first, then given and family names, then
-- nationality and position.
--
-- The array elements are spelled out by subscript rather than joined with
-- array_to_string, which would be the obvious way and does not work:
--
--   ERROR: generation expression is not immutable
--
-- array_to_string is only STABLE, because it depends on the element type's
-- output function. A generated column requires IMMUTABLE. Subscripting,
-- coalesce and || all are, so the expression is written out to the caps the
-- service enforces (3 nationalities, 6 secondary positions).
--
-- That couples this expression to those caps: raising one means extending
-- this list, or entries past the limit stop being searchable.
ALTER TABLE players
    ADD COLUMN search_document tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(first_name, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(last_name, '')), 'B') ||
        setweight(to_tsvector('simple',
            coalesce(nationalities[1], '') || ' ' ||
            coalesce(nationalities[2], '') || ' ' ||
            coalesce(nationalities[3], '')
        ), 'C') ||
        setweight(to_tsvector('simple', coalesce(position, '')), 'C') ||
        -- Secondary positions are indexed too, so searching "left back" also
        -- finds the players who can fill the role rather than only those
        -- listed as it.
        setweight(to_tsvector('simple',
            coalesce(secondary_positions[1], '') || ' ' ||
            coalesce(secondary_positions[2], '') || ' ' ||
            coalesce(secondary_positions[3], '') || ' ' ||
            coalesce(secondary_positions[4], '') || ' ' ||
            coalesce(secondary_positions[5], '') || ' ' ||
            coalesce(secondary_positions[6], '')
        ), 'D')
    ) STORED;

CREATE INDEX IF NOT EXISTS idx_players_search ON players USING GIN (search_document);

-- Finding every Turkish player is a containment query now, which GIN answers
-- without scanning the table.
CREATE INDEX IF NOT EXISTS idx_players_nationalities ON players USING GIN (nationalities);

-- ------------------------------------------------------------- percentages
-- Manually entered season or career averages, NOT computed from matches --
-- there is no match data in this system, and there is no honest way to derive
-- these without it. Nullable because "not recorded" is the normal state and is
-- a different fact from zero.
--
-- DOUBLE PRECISION rather than the integer-minor-units approach money uses:
-- these are ratios that are read and compared, never summed, so binary
-- floating point costs nothing here. The CHECK is what actually matters.
ALTER TABLE players
    ADD COLUMN IF NOT EXISTS duels_won_pct        DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS pass_completion_pct  DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS shots_on_target_pct  DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS aerial_duels_won_pct DOUBLE PRECISION;

ALTER TABLE players
    ADD CONSTRAINT players_percentages_in_range CHECK (
        (duels_won_pct        IS NULL OR (duels_won_pct        >= 0 AND duels_won_pct        <= 100)) AND
        (pass_completion_pct  IS NULL OR (pass_completion_pct  >= 0 AND pass_completion_pct  <= 100)) AND
        (shots_on_target_pct  IS NULL OR (shots_on_target_pct  >= 0 AND shots_on_target_pct  <= 100)) AND
        (aerial_duels_won_pct IS NULL OR (aerial_duels_won_pct >= 0 AND aerial_duels_won_pct <= 100))
    );

-- ------------------------------------------------------- strengths/weaknesses
-- Two lists rather than free prose, so they render as bullets and stay
-- comparable between players. Length is bounded in the service, not here: a
-- CHECK on array element length would report a constraint name to a user
-- rather than a sentence they can act on.
ALTER TABLE players
    ADD COLUMN IF NOT EXISTS strengths  TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS weaknesses TEXT[] NOT NULL DEFAULT '{}';

-- -------------------------------------------------------------------- notes
-- One note per member per player, editable afterwards.
--
-- The UNIQUE constraint is the anti-spam rule, and it lives here rather than
-- only in the handler: two simultaneous posts would both pass a "does one
-- exist?" check and both insert. The database is the only place that can
-- decide this once.
CREATE TABLE IF NOT EXISTS player_notes (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id  UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,

    -- Accounts live in identity-svc's database, which no foreign key here can
    -- reach. The same situation as team_editors.
    author_id  UUID NOT NULL,
    -- Denormalised at write time from the token, because this service cannot
    -- resolve a user id to a name. It is also the right call independently:
    -- the note keeps the name it was written under even if the account is
    -- later deleted, which is what the rest of this schema does with history.
    author_name TEXT NOT NULL DEFAULT '',

    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT player_notes_one_per_author UNIQUE (player_id, author_id),
    CONSTRAINT player_notes_body_not_empty CHECK (length(btrim(body)) > 0)
);

-- The listing is always "the notes on this player, newest first".
CREATE INDEX IF NOT EXISTS idx_player_notes_player ON player_notes (player_id, created_at DESC);
-- "Have I already written one?" and account cleanup both go by author.
CREATE INDEX IF NOT EXISTS idx_player_notes_author ON player_notes (author_id);
